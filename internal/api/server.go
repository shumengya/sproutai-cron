// Package api implements the Gin HTTP API for the WebUI.
package api

import (
	"io/fs"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"

	"sproutai-cron/internal/daemon"
	"sproutai-cron/internal/manager"
	"sproutai-cron/internal/runtimeprobe"
	"sproutai-cron/webui"
)

// Server is the HTTP application.
type Server struct {
	Root     string
	assets   fs.FS // embedded frontend files (index.html, app.js, …)
	staticFS http.FileSystem
	runJobs  map[string]*os.Process
	runMu    sync.Mutex
}

// New creates a server with embedded frontend assets.
func New(root string) *Server {
	sub, err := fs.Sub(webui.Frontend, "frontend")
	if err != nil {
		// Should never happen when embed is correct; fall back empty.
		sub = webui.Frontend
	}
	return &Server{
		Root:     root,
		assets:   sub,
		staticFS: http.FS(sub),
		runJobs:  map[string]*os.Process{},
	}
}

// Engine builds the gin engine with dual /api and /cron/api prefixes.
func (s *Server) Engine() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), gin.Logger())

	register := func(g *gin.RouterGroup) {
		g.GET("/health", s.health)
		g.GET("/serve", s.serveStatus)
		g.GET("/runtimes", s.runtimes)
		g.GET("/tasks", s.listTasks)
		g.GET("/tasks/:task_id", s.getTask)
		g.POST("/tasks/:task_id/enable", s.enableTask)
		g.POST("/tasks/:task_id/disable", s.disableTask)
		g.POST("/tasks/:task_id/toggle", s.toggleTask)
		g.PATCH("/tasks/:task_id/schedule", s.updateSchedule)
		g.POST("/tasks/:task_id/run", s.runTask)
		g.GET("/tasks/:task_id/log", s.taskLog)
	}
	register(r.Group("/api"))
	register(r.Group("/cron/api"))

	// Serve index from embed bytes (avoid FileFromFS/FileServer redirect quirks).
	index := func(c *gin.Context) {
		data, err := fs.ReadFile(s.assets, "index.html")
		if err != nil {
			c.String(http.StatusInternalServerError, "index.html missing from embed")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	}
	r.GET("/", index)
	r.GET("/cron", index)
	r.GET("/cron/", index)

	// Static assets embedded in the binary (no disk dependency).
	r.StaticFS("/static", s.staticFS)
	r.StaticFS("/cron/static", s.staticFS)
	return r
}

// EnsureScheduler starts serve in-process (same as cronctl serve) if not running.
// Waits briefly so status /api/serve can see serve_running=true after web starts.
func (s *Server) EnsureScheduler() {
	if daemon.IsRunning(s.Root) {
		println("[webui] scheduler already running (pid file ok)")
		return
	}
	ok := daemon.StartInBackground(s.Root)
	if ok {
		println("[webui] scheduler started (in-process serve)")
		return
	}
	println("[webui] WARNING: scheduler failed to start — timed tasks will not fire")
	println("[webui] check lock/pid under cron-tasks/ or run: cronctl serve")
}

func (s *Server) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":        "ok",
		"serve_running": daemon.IsRunning(s.Root),
	})
}

func (s *Server) serveStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"serve_running": daemon.IsRunning(s.Root)})
}

func (s *Server) runtimes(c *gin.Context) {
	c.JSON(http.StatusOK, runtimeprobe.All())
}

func (s *Server) listTasks(c *gin.Context) {
	tasks, err := manager.ListTasks(s.Root)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tasks)
}

func (s *Server) getTask(c *gin.Context) {
	info, err := manager.BuildTaskInfo(s.Root, c.Param("task_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, info)
}

func (s *Server) enableTask(c *gin.Context) {
	info, _, err := manager.SetTaskState(s.Root, c.Param("task_id"), true)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, info)
}

func (s *Server) disableTask(c *gin.Context) {
	info, _, err := manager.SetTaskState(s.Root, c.Param("task_id"), false)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, info)
}

func (s *Server) toggleTask(c *gin.Context) {
	info, _, err := manager.ToggleTask(s.Root, c.Param("task_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, info)
}

type scheduleBody struct {
	Description string   `json:"description"`
	Schedule    string   `json:"schedule" binding:"required"`
	Tags        []string `json:"tags"`
}

func (s *Server) updateSchedule(c *gin.Context) {
	var body scheduleBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	if body.Tags == nil {
		body.Tags = []string{}
	}
	info, message, err := manager.UpdateTaskSchedule(
		s.Root, c.Param("task_id"), body.Description, body.Schedule, body.Tags, true,
	)
	if err != nil {
		if isNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"detail": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	if message == "" {
		message = "已保存"
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": message, "task": info})
}

func (s *Server) runTask(c *gin.Context) {
	taskID := c.Param("task_id")
	if _, err := manager.GetContext(s.Root, taskID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": err.Error()})
		return
	}

	s.runMu.Lock()
	defer s.runMu.Unlock()

	if proc, ok := s.runJobs[taskID]; ok && proc != nil {
		if processAlive(proc.Pid) {
			c.JSON(http.StatusOK, gin.H{
				"ok":      false,
				"message": "任务已在后台运行",
				"pid":     proc.Pid,
			})
			return
		}
		delete(s.runJobs, taskID)
	}

	exe, err := os.Executable()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error()})
		return
	}
	env := append(os.Environ(), "SPROUTAI_CRON_ROOT="+s.Root)
	proc, err := os.StartProcess(exe, []string{exe, "run", taskID}, &os.ProcAttr{
		Dir:   s.Root,
		Env:   env,
		Files: []*os.File{nil, os.Stdout, os.Stderr},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error()})
		return
	}
	s.runJobs[taskID] = proc
	go func(p *os.Process, id string) {
		_, _ = p.Wait()
		s.runMu.Lock()
		if s.runJobs[id] == p {
			delete(s.runJobs, id)
		}
		s.runMu.Unlock()
	}(proc, taskID)

	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "已在后台启动", "pid": proc.Pid})
}

func (s *Server) taskLog(c *gin.Context) {
	lines := 200
	if v := c.Query("lines"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			if n < 1 {
				n = 1
			}
			if n > 2000 {
				n = 2000
			}
			lines = n
		}
	}
	content, err := manager.ReadLogTail(s.Root, c.Param("task_id"), lines)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"task_id": c.Param("task_id"), "content": content})
}

func isNotFound(err error) bool {
	s := err.Error()
	return strings.Contains(s, "不存在") || strings.Contains(s, "未找到") || strings.Contains(s, "缺少")
}
