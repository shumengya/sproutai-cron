// Package daemon implements the cronctl serve loop.
package daemon

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"sproutai-cron/internal/lock"
	"sproutai-cron/internal/manager"
	"sproutai-cron/internal/runner"
	"sproutai-cron/internal/schedule"
	"sproutai-cron/internal/task"
)

// IsRunning reports whether a serve process appears alive via PID file.
func IsRunning(cronRoot string) bool {
	path := task.DaemonPIDPath(cronRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return false
	}
	if !pidAlive(pid) {
		_ = os.Remove(path)
		return false
	}
	return true
}

// ClearStaleState removes lock/pid files when no live serve is detected.
func ClearStaleState(cronRoot string) {
	if IsRunning(cronRoot) {
		return
	}
	if lock.IsLocked(task.DaemonLockPath(cronRoot)) {
		return
	}
	_ = os.Remove(task.DaemonPIDPath(cronRoot))
	_ = os.Remove(task.DaemonLockPath(cronRoot))
}

// RunServeLoop starts the scheduler. once=true evaluates due tasks once then exits.
func RunServeLoop(cronRoot string, once bool) error {
	if err := os.MkdirAll(task.DaemonStateDir(cronRoot), 0o755); err != nil {
		return err
	}

	lf, err := acquireServeLock(cronRoot)
	if err != nil {
		return err
	}
	defer func() {
		_ = lf.Unlock()
		_ = os.Remove(task.DaemonPIDPath(cronRoot))
	}()

	pid := os.Getpid()
	if err := os.WriteFile(task.DaemonPIDPath(cronRoot), []byte(fmt.Sprintf("%d\n", pid)), 0o644); err != nil {
		return fmt.Errorf("写入 serve pid 失败: %w", err)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	var (
		mu      sync.Mutex
		running = map[string]bool{}
	)

	fmt.Printf("[serve] sprout-cron 调度器已启动 pid=%d root=%s\n", pid, cronRoot)
	fmt.Println("[serve] 支持 @every / @random / cron / @on / @holiday；Ctrl+C 退出")

	for {
		now := time.Now()
		due, nearest := scanTasks(cronRoot, now, &mu, running)
		if len(due) > 0 {
			for _, d := range due {
				fmt.Printf("[serve] 命中 %s (%s)\n", d.taskID, d.expr)
				spawnRun(cronRoot, d.taskID, &mu, running)
			}
		} else if once {
			fmt.Printf("[serve] %s 无到期任务\n", now.Format("2006-01-02 15:04:05"))
		}

		if once {
			deadline := time.Now().Add(2 * time.Second)
			for time.Now().Before(deadline) {
				mu.Lock()
				empty := len(running) == 0
				mu.Unlock()
				if empty {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			break
		}

		wait := sleepUntil(now, nearest)
		timer := time.NewTimer(wait)
		select {
		case <-stop:
			timer.Stop()
			fmt.Println("[serve] 正在退出…")
			fmt.Println("[serve] 已停止")
			return nil
		case <-timer.C:
		}
	}
	fmt.Println("[serve] 正在退出…")
	fmt.Println("[serve] 已停止")
	return nil
}

type dueItem struct {
	taskID string
	expr   string
}

// scanTasks returns due items and the nearest next fire time among enabled tasks.
func scanTasks(cronRoot string, now time.Time, mu *sync.Mutex, running map[string]bool) ([]dueItem, time.Time) {
	var due []dueItem
	var nearest time.Time

	for _, id := range manager.IterTaskIDs(cronRoot) {
		info, err := manager.BuildTaskInfo(cronRoot, id)
		if err != nil || !info.Enabled || info.Schedule == nil || *info.Schedule == "" {
			continue
		}
		spec, err := schedule.Parse(*info.Schedule)
		if err != nil {
			fmt.Printf("[serve] 跳过无效 schedule: %s → %s (%v)\n", id, *info.Schedule, err)
			continue
		}

		st := schedule.LoadState(cronRoot, id)
		st = schedule.EnsureNext(spec, now, st)
		// persist init for every/random
		if (spec.Kind == schedule.KindEvery || spec.Kind == schedule.KindRandom) && st.NextRunAt != nil {
			_ = schedule.SaveState(cronRoot, id, st)
		}

		if schedule.IsDue(spec, now, st) {
			mu.Lock()
			busy := running[id]
			mu.Unlock()
			if busy {
				fmt.Printf("[serve] 跳过（仍在运行）: %s\n", id)
			} else {
				// mark fired + advance next before spawn to avoid double-hit
				st = schedule.MarkFired(spec, now, st)
				_ = schedule.SaveState(cronRoot, id, st)
				due = append(due, dueItem{taskID: id, expr: spec.Raw})
			}
		}

		// next wake candidate
		n := schedule.NextAfter(spec, now, st)
		if n.IsZero() {
			continue
		}
		if nearest.IsZero() || n.Before(nearest) {
			nearest = n
		}
	}
	return due, nearest
}

func sleepUntil(now, nearest time.Time) time.Duration {
	// max poll 1s for hot reload of schedules; min 50ms
	maxWait := time.Second
	minWait := 50 * time.Millisecond
	if nearest.IsZero() {
		return maxWait
	}
	d := nearest.Sub(now)
	if d < minWait {
		return minWait
	}
	if d > maxWait {
		return maxWait
	}
	return d
}

func acquireServeLock(cronRoot string) (*lock.File, error) {
	lockPath := task.DaemonLockPath(cronRoot)
	try := func() (*lock.File, bool, error) {
		return lock.TryLock(lockPath)
	}

	lf, ok, err := try()
	if err != nil {
		return nil, err
	}
	if ok {
		return lf, nil
	}

	if IsRunning(cronRoot) {
		return nil, fmt.Errorf(
			"sprout-cron serve 已在运行。可用 `cronctl status` 查看；root=%s",
			cronRoot,
		)
	}

	ClearStaleState(cronRoot)
	if !IsRunning(cronRoot) && !lock.IsLocked(lockPath) {
		_ = os.Remove(lockPath)
		_ = os.Remove(task.DaemonPIDPath(cronRoot))
	}

	lf, ok, err = try()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf(
			"sprout-cron serve 锁被占用但无法确认 PID。若确认已退出请删除 %s 与 %s 后重试",
			task.DaemonLockName, task.DaemonPIDName,
		)
	}
	return lf, nil
}

// StartInBackground starts serve in a goroutine if not already running.
func StartInBackground(cronRoot string) bool {
	if IsRunning(cronRoot) {
		return true
	}
	ClearStaleState(cronRoot)

	errCh := make(chan error, 1)
	go func() {
		errCh <- RunServeLoop(cronRoot, false)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-errCh:
			if err != nil {
				fmt.Printf("[serve] scheduler not started: %v\n", err)
			}
			return IsRunning(cronRoot)
		default:
		}
		if IsRunning(cronRoot) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return IsRunning(cronRoot)
}

func spawnRun(cronRoot, taskID string, mu *sync.Mutex, running map[string]bool) {
	mu.Lock()
	if running[taskID] {
		mu.Unlock()
		fmt.Printf("[serve] 跳过（仍在运行）: %s\n", taskID)
		return
	}
	running[taskID] = true
	mu.Unlock()

	go func() {
		defer func() {
			mu.Lock()
			delete(running, taskID)
			mu.Unlock()
		}()
		fmt.Printf("[serve] 触发 %s @ %s\n", taskID, time.Now().Format("2006-01-02 15:04:05"))
		code, err := runner.Run(cronRoot, taskID)
		if err != nil {
			fmt.Printf("[serve] 失败 %s: %v\n", taskID, err)
			return
		}
		fmt.Printf("[serve] 完成 %s (exit=%d)\n", taskID, code)
	}()
}
