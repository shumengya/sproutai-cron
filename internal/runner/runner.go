// Package runner executes cron tasks via subprocess with lock and logging.
package runner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"sproutai-cron/internal/interp"
	"sproutai-cron/internal/lock"
	"sproutai-cron/internal/manager"
	"sproutai-cron/internal/task"
)

// Run executes a task once. Disabled tasks return 0 without running.
func Run(cronRoot, taskID string) (int, error) {
	p, err := manager.GetContext(cronRoot, taskID)
	if err != nil {
		return 1, err
	}
	if !p.Enabled {
		return 0, nil
	}
	m, err := task.LoadManifest(p.TaskDir)
	if err != nil {
		return 1, err
	}

	if err := os.MkdirAll(p.LogDir, 0o755); err != nil {
		return 1, err
	}
	rotateLog(p.LogFile)

	lf, ok, err := lock.TryLock(p.LockFile)
	if err != nil {
		return 1, err
	}
	if !ok {
		appendLog(p, "已有同名任务在运行，跳过本次。")
		return 0, nil
	}
	defer lf.Unlock()

	appendLog(p, "start")
	appendLog(p, fmt.Sprintf("runtime=%s entry=%s", m.Runtime, m.Entry))

	cmdArgs, err := buildCommand(m, p)
	if err != nil {
		appendLog(p, err.Error())
		appendLog(p, "end")
		return 1, nil
	}
	appendLog(p, "exec="+strings.Join(cmdArgs, " "))

	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Dir = p.TaskDir
	cmd.Env = taskEnv(cronRoot, taskID, p.TaskDir)

	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		appendRaw(p.LogFile, string(out))
	}
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			appendLog(p, fmt.Sprintf("error: %v", err))
			code = 1
		}
	}
	if code != 0 {
		appendLog(p, fmt.Sprintf("exit=%d", code))
	}
	appendLog(p, "end")
	return code, nil
}

func taskEnv(cronRoot, taskID, taskDir string) []string {
	env := append(os.Environ(),
		"CRON_TASK_ID="+taskID,
		"CRON_TASK_DIR="+taskDir,
		"CRON_ROOT="+cronRoot,
		// Prefer UTF-8 logs from Python / modern runtimes
		"PYTHONIOENCODING=utf-8",
		"PYTHONUTF8=1",
		"PYTHONUNBUFFERED=1",
	)
	return env
}

func buildCommand(m task.Manifest, p task.Paths) ([]string, error) {
	entry := filepath.Join(p.TaskDir, m.Entry)
	switch m.Runtime {
	case task.RuntimePython:
		py := interp.FindPython()
		if py == "" {
			return nil, fmt.Errorf("未找到 python 解释器，请确认已安装并在 PATH 中")
		}
		base := strings.ToLower(filepath.Base(py))
		if base == "py" || base == "py.exe" {
			return []string{py, "-3", entry}, nil
		}
		return []string{py, entry}, nil
	case task.RuntimeJavaScript:
		node := interp.FindNode()
		if node == "" {
			return nil, fmt.Errorf("未找到 node 解释器，请确认已安装并在 PATH 中")
		}
		return []string{node, entry}, nil
	case task.RuntimeBash:
		bash := interp.FindBash()
		if bash == "" {
			return nil, fmt.Errorf("未找到可用的 bash（Windows 请安装 Git for Windows 或 MSYS2；勿使用未配置发行版的 WSL bash.exe）")
		}
		// Git Bash accepts Windows paths; forward slashes are more portable
		return []string{bash, filepath.ToSlash(entry)}, nil
	case task.RuntimePowerShell:
		ps := interp.FindPowerShell()
		if ps == "" {
			return nil, fmt.Errorf("未找到 powershell 解释器，请确认已安装并在 PATH 中")
		}
		// -NoProfile: avoid user profile noise/encoding issues
		// -ExecutionPolicy Bypass: allow local task scripts
		return []string{ps, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", entry}, nil
	default:
		return nil, fmt.Errorf("不支持的 runtime: %s", m.Runtime)
	}
}

func taskOutputPrefix(taskID string) string {
	parts := strings.SplitN(taskID, "-", 2)
	family := parts[0]
	suffix := taskID
	if len(parts) > 1 {
		suffix = parts[1]
	}
	acronyms := map[string]bool{"ai": true, "cli": true, "ssh": true, "api": true, "db": true, "url": true}
	var display []string
	for _, p := range strings.Split(suffix, "-") {
		if acronyms[strings.ToLower(p)] {
			display = append(display, strings.ToUpper(p))
		} else if p != "" {
			display = append(display, strings.ToUpper(p[:1])+p[1:])
		}
	}
	return fmt.Sprintf("[%s][%s]", family, strings.Join(display, "-"))
}

func appendLog(p task.Paths, message string) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	line := fmt.Sprintf("[%s] %s %s\n", ts, taskOutputPrefix(p.TaskID), message)
	appendRaw(p.LogFile, line)
}

func appendRaw(logFile, text string) {
	if text == "" {
		return
	}
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(text)
	if !strings.HasSuffix(text, "\n") {
		_, _ = f.WriteString("\n")
	}
}

func rotateLog(logFile string) {
	maxBytes := int64(10 * 1024 * 1024)
	if v := os.Getenv("CRON_LOG_MAX_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			maxBytes = n
		}
	}
	st, err := os.Stat(logFile)
	if err != nil || st.Size() <= maxBytes {
		return
	}
	stamp := time.Now().Format("2006-01-02-150405")
	_ = os.Rename(logFile, logFile+"."+stamp)
}
