// Package manager provides task management shared by CLI and WebUI.
package manager

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"sproutai-cron/internal/lock"
	"sproutai-cron/internal/schedule"
	"sproutai-cron/internal/task"
)

var taskIDRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)

var nonTaskRootDirs = map[string]bool{
	"cron-tasks": true, "lib": true, "webui": true, "skills": true,
	"mcps": true, "terminals": true, "scripts": true, "internal": true,
	"cmd": true, "bin": true, "config": true,
}

// TaskInfo is the public task view for CLI/API.
type TaskInfo struct {
	TaskID       string   `json:"task_id"`
	Enabled      bool     `json:"enabled"`
	Running      bool     `json:"running"`
	Schedule     *string  `json:"schedule"`
	ScheduleKind *string  `json:"schedule_kind,omitempty"`
	NextRunAt    *string  `json:"next_run_at,omitempty"`
	Description  *string  `json:"description"`
	LogSize      int64    `json:"log_size"`
	LogUpdated   *string  `json:"log_updated"`
	TaskDir      string   `json:"task_dir"`
	Runtime      string   `json:"runtime"`
	Tags         []string `json:"tags"`
}

// IterTaskIDs lists all known task IDs.
func IterTaskIDs(cronRoot string) []string {
	_ = MigrateLegacy(cronRoot)
	ids := map[string]bool{}
	container := task.TasksRoot(cronRoot)
	if entries, err := os.ReadDir(container); err == nil {
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			name := e.Name()
			if strings.HasSuffix(name, ".disabled") {
				ids[strings.TrimSuffix(name, ".disabled")] = true
				continue
			}
			p := filepath.Join(container, name)
			if task.IsValidTaskDir(p) {
				ids[name] = true
			}
		}
		dis := task.DisabledRoot(cronRoot)
		if entries, err := os.ReadDir(dis); err == nil {
			for _, e := range entries {
				if e.IsDir() && task.IsValidTaskDir(filepath.Join(dis, e.Name())) {
					ids[e.Name()] = true
				}
			}
		}
	}
	// legacy root layout
	if entries, err := os.ReadDir(cronRoot); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if nonTaskRootDirs[name] || (strings.HasPrefix(name, ".") && name != task.DisabledDirName) {
				continue
			}
			if strings.HasSuffix(name, ".disabled") {
				ids[strings.TrimSuffix(name, ".disabled")] = true
				continue
			}
			if name == task.DisabledDirName {
				sub, _ := os.ReadDir(filepath.Join(cronRoot, name))
				for _, c := range sub {
					if c.IsDir() && task.IsValidTaskDir(filepath.Join(cronRoot, name, c.Name())) {
						ids[c.Name()] = true
					}
				}
				continue
			}
			if task.IsValidTaskDir(filepath.Join(cronRoot, name)) {
				ids[name] = true
			}
		}
	}
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// GetContext validates task exists.
func GetContext(cronRoot, taskID string) (task.Paths, error) {
	p, err := task.Resolve(cronRoot, taskID)
	if err != nil {
		return p, err
	}
	st, err := os.Stat(p.TaskDir)
	if err != nil || !st.IsDir() {
		return p, fmt.Errorf("任务不存在: %s", taskID)
	}
	if !task.IsValidTaskDir(p.TaskDir) {
		return p, fmt.Errorf("任务目录缺少 schedule.cron 或入口脚本: %s", taskID)
	}
	return p, nil
}

// BuildTaskInfo builds TaskInfo for one task.
func BuildTaskInfo(cronRoot, taskID string) (TaskInfo, error) {
	p, err := GetContext(cronRoot, taskID)
	if err != nil {
		return TaskInfo{}, err
	}
	expr, desc := task.ParseSchedule(p.TaskDir)
	var sched, description, kind, nextRun *string
	if expr != "" {
		sched = &expr
		if spec, err := schedule.Parse(expr); err == nil {
			k := string(spec.Kind)
			kind = &k
			st := schedule.LoadState(cronRoot, taskID)
			st = schedule.EnsureNext(spec, time.Now(), st)
			n := schedule.NextAfter(spec, time.Now(), st)
			if !n.IsZero() {
				s := n.Local().Format("2006-01-02 15:04:05")
				nextRun = &s
			}
		}
	}
	if desc != "" {
		description = &desc
	}
	var logSize int64
	var logUpdated *string
	if st, err := os.Stat(p.LogFile); err == nil {
		logSize = st.Size()
		s := st.ModTime().Format("2006-01-02 15:04:05")
		logUpdated = &s
	}
	m, err := task.LoadManifest(p.TaskDir)
	if err != nil {
		return TaskInfo{}, err
	}
	return TaskInfo{
		TaskID:       taskID,
		Enabled:      p.Enabled,
		Running:      lock.IsLocked(p.LockFile),
		Schedule:     sched,
		ScheduleKind: kind,
		NextRunAt:    nextRun,
		Description:  description,
		LogSize:      logSize,
		LogUpdated:   logUpdated,
		TaskDir:      p.TaskDir,
		Runtime:      m.Runtime,
		Tags:         task.EffectiveTags(p.TaskDir, m),
	}, nil
}

// ListTasks returns all tasks.
func ListTasks(cronRoot string) ([]TaskInfo, error) {
	_ = MigrateLegacy(cronRoot)
	ids := IterTaskIDs(cronRoot)
	out := make([]TaskInfo, 0, len(ids))
	for _, id := range ids {
		info, err := BuildTaskInfo(cronRoot, id)
		if err != nil {
			continue
		}
		out = append(out, info)
	}
	return out, nil
}

// SetTaskState enables or disables a task.
func SetTaskState(cronRoot, taskID string, enabled bool) (TaskInfo, string, error) {
	if _, err := GetContext(cronRoot, taskID); err != nil {
		return TaskInfo{}, "", err
	}
	if err := task.SetEnabled(cronRoot, taskID, enabled); err != nil {
		return TaskInfo{}, "", err
	}
	info, err := BuildTaskInfo(cronRoot, taskID)
	if err != nil {
		return TaskInfo{}, "", err
	}
	label := "已禁用"
	hint := "serve 将跳过此任务"
	if enabled {
		label = "已启用"
		hint = "由 serve 到点触发"
	}
	return info, fmt.Sprintf("%s（%s）", label, hint), nil
}

// ToggleTask flips enable state.
func ToggleTask(cronRoot, taskID string) (TaskInfo, string, error) {
	p, err := GetContext(cronRoot, taskID)
	if err != nil {
		return TaskInfo{}, "", err
	}
	return SetTaskState(cronRoot, taskID, !p.Enabled)
}

// ReadLogTail returns the last n lines of the task log.
func ReadLogTail(cronRoot, taskID string, lines int) (string, error) {
	p, err := GetContext(cronRoot, taskID)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(p.LogFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	content := string(data)
	parts := strings.Split(content, "\n")
	// trailing empty from final newline
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	if lines <= 0 || len(parts) <= lines {
		return content, nil
	}
	return strings.Join(parts[len(parts)-lines:], "\n") + "\n", nil
}

// UpdateTaskSchedule updates schedule.cron and tags.
func UpdateTaskSchedule(cronRoot, taskID, description, schedule string, tags []string, hasTags bool) (TaskInfo, string, error) {
	p, err := GetContext(cronRoot, taskID)
	if err != nil {
		return TaskInfo{}, "", err
	}
	if err := task.UpdateSchedule(p.TaskDir, description, schedule); err != nil {
		return TaskInfo{}, "", err
	}
	if hasTags {
		if err := task.SaveTags(p.TaskDir, tags); err != nil {
			return TaskInfo{}, "", err
		}
	}
	info, err := BuildTaskInfo(cronRoot, taskID)
	if err != nil {
		return TaskInfo{}, "", err
	}
	return info, "已保存（serve 将使用新表达式）", nil
}

// CreateTask copies a template into a new task directory.
func CreateTask(cronRoot, taskID, runtime string, enable bool) (TaskInfo, string, string, error) {
	if !taskIDRe.MatchString(taskID) {
		return TaskInfo{}, "", "", fmt.Errorf("task_id 仅允许字母数字、点、连字符与下划线，且不能以特殊符号开头")
	}
	templateName, ok := task.TemplateDirs[runtime]
	if !ok {
		return TaskInfo{}, "", "", fmt.Errorf("不支持的 runtime: %s", runtime)
	}
	for _, id := range IterTaskIDs(cronRoot) {
		if id == taskID {
			return TaskInfo{}, "", "", fmt.Errorf("任务已存在: %s", taskID)
		}
	}
	templateDir, err := resolveTemplateDir(cronRoot, runtime)
	if err != nil {
		return TaskInfo{}, "", "", err
	}
	container := task.TasksRoot(cronRoot)
	if err := os.MkdirAll(container, 0o755); err != nil {
		return TaskInfo{}, "", "", err
	}
	target := filepath.Join(container, taskID)
	if err := copyDir(templateDir, target); err != nil {
		return TaskInfo{}, "", "", err
	}
	sched := filepath.Join(target, "schedule.cron")
	if _, err := os.Stat(sched); err == nil {
		_ = task.RewriteScheduleOnCreate(sched, templateName, taskID)
	}
	if runtime == task.RuntimeBash {
		runSh := filepath.Join(target, "run.sh")
		_ = os.Chmod(runSh, 0o755)
	}
	info, msg, err := SetTaskState(cronRoot, taskID, enable)
	return info, msg, templateName, err
}

func resolveTemplateDir(cronRoot, runtime string) (string, error) {
	name, ok := task.TemplateDirs[runtime]
	if !ok {
		return "", fmt.Errorf("不支持的 runtime: %s", runtime)
	}
	p := filepath.Join(task.TasksRoot(cronRoot), name)
	if st, err := os.Stat(p); err == nil && st.IsDir() {
		return p, nil
	}
	legacy := filepath.Join(cronRoot, name)
	if st, err := os.Stat(legacy); err == nil && st.IsDir() {
		return legacy, nil
	}
	return "", fmt.Errorf("模板不存在: cron-tasks/%s", name)
}

// DeleteTask removes a task directory.
func DeleteTask(cronRoot, taskID string, force bool) (map[string]any, error) {
	if isTemplateTaskID(taskID) {
		return nil, fmt.Errorf("禁止删除模板任务: %s", taskID)
	}
	if !taskIDRe.MatchString(taskID) {
		return nil, fmt.Errorf("task_id 仅允许字母数字、点、连字符与下划线，且不能以特殊符号开头")
	}
	p, err := GetContext(cronRoot, taskID)
	if err != nil {
		return nil, err
	}
	if lock.IsLocked(p.LockFile) && !force {
		return nil, fmt.Errorf("任务正在运行，如需强制删除请加 --force: %s", taskID)
	}
	if err := os.RemoveAll(p.TaskDir); err != nil {
		return nil, err
	}
	return map[string]any{
		"task_id":  taskID,
		"task_dir": p.TaskDir,
		"message":  fmt.Sprintf("已删除任务目录 %s", p.TaskDir),
	}, nil
}

func isTemplateTaskID(taskID string) bool {
	for _, v := range task.TemplateDirs {
		if taskID == v {
			return true
		}
	}
	return strings.HasPrefix(taskID, "template-")
}

// ListTemplates lists language templates.
func ListTemplates(cronRoot string) []map[string]any {
	var out []map[string]any
	// stable order
	order := []string{task.RuntimePython, task.RuntimeJavaScript, task.RuntimeBash, task.RuntimePowerShell}
	for _, rt := range order {
		dirname := task.TemplateDirs[rt]
		path := filepath.Join(task.TasksRoot(cronRoot), dirname)
		exists := false
		if st, err := os.Stat(path); err == nil && st.IsDir() {
			exists = true
		} else if st, err := os.Stat(filepath.Join(cronRoot, dirname)); err == nil && st.IsDir() {
			exists = true
		}
		out = append(out, map[string]any{
			"runtime":      rt,
			"template_dir": "cron-tasks/" + dirname,
			"exists":       exists,
			"entry":        task.RuntimeDefaultEntry[rt],
		})
	}
	return out
}

// MigrateLegacy moves old layouts into cron-tasks/. Best-effort.
func MigrateLegacy(cronRoot string) error {
	dest := task.TasksRoot(cronRoot)
	_ = os.MkdirAll(dest, 0o755)
	disabledDest := task.DisabledRoot(cronRoot)
	_ = os.MkdirAll(disabledDest, 0o755)

	// root/<id>.disabled → cron-tasks/.disabled/<id>
	if entries, err := os.ReadDir(cronRoot); err == nil {
		for _, e := range entries {
			if !e.IsDir() || !strings.HasSuffix(e.Name(), ".disabled") {
				continue
			}
			id := strings.TrimSuffix(e.Name(), ".disabled")
			target := filepath.Join(disabledDest, id)
			if _, err := os.Stat(target); err == nil {
				continue
			}
			_ = os.Rename(filepath.Join(cronRoot, e.Name()), target)
		}
	}
	// root/.disabled → cron-tasks/.disabled
	legacyDis := filepath.Join(cronRoot, task.DisabledDirName)
	if entries, err := os.ReadDir(legacyDis); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			target := filepath.Join(disabledDest, e.Name())
			if _, err := os.Stat(target); err == nil {
				continue
			}
			_ = os.Rename(filepath.Join(legacyDis, e.Name()), target)
		}
		_ = os.Remove(legacyDis) // only if empty
	}
	// root valid tasks → cron-tasks/
	if entries, err := os.ReadDir(cronRoot); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if nonTaskRootDirs[name] || strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".disabled") {
				continue
			}
			src := filepath.Join(cronRoot, name)
			if !task.IsValidTaskDir(src) {
				continue
			}
			target := filepath.Join(dest, name)
			if _, err := os.Stat(target); err == nil {
				continue
			}
			_ = os.Rename(src, target)
		}
	}
	return nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		// skip junk
		base := info.Name()
		if base == "__pycache__" || strings.HasSuffix(base, ".pyc") || strings.HasSuffix(base, ".lock") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(base, "logs") && rel == "logs" {
			// create empty logs dir without copying old logs
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if strings.Contains(rel, string(filepath.Separator)+"logs"+string(filepath.Separator)) || rel == filepath.Join("logs", base) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			// skip log files
			if strings.HasPrefix(rel, "logs") {
				return nil
			}
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// Ptr helpers for tests — unused.
var _ = time.Now
