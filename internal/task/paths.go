package task

import (
	"os"
	"path/filepath"
	"strings"

	"sproutai-cron/internal/root"
)

const (
	DisabledDirName = ".disabled"
	DaemonLockName  = ".sprout-cron.serve.lock"
	DaemonPIDName   = ".sprout-cron.serve.pid"
)

// Paths holds resolved paths for one task.
type Paths struct {
	TaskID   string
	Root     string
	TaskDir  string
	LogDir   string
	LogFile  string
	LockFile string
	Enabled  bool
}

// TasksRoot returns cron-tasks directory.
func TasksRoot(cronRoot string) string {
	return root.TasksRoot(cronRoot)
}

// DisabledRoot returns cron-tasks/.disabled.
func DisabledRoot(cronRoot string) string {
	return filepath.Join(TasksRoot(cronRoot), DisabledDirName)
}

// ActiveDir returns cron-tasks/<id>.
func ActiveDir(cronRoot, taskID string) string {
	return filepath.Join(TasksRoot(cronRoot), taskID)
}

// DisabledDir returns cron-tasks/.disabled/<id>.
func DisabledDir(cronRoot, taskID string) string {
	return filepath.Join(DisabledRoot(cronRoot), taskID)
}

// Resolve finds an existing task directory (enabled or disabled, including legacy layouts).
func Resolve(cronRoot, taskID string) (Paths, error) {
	candidates := []struct {
		dir     string
		enabled bool
	}{
		{DisabledDir(cronRoot, taskID), false},
		{ActiveDir(cronRoot, taskID), true},
		{filepath.Join(cronRoot, DisabledDirName, taskID), false},
		{filepath.Join(cronRoot, taskID+".disabled"), false},
		{filepath.Join(cronRoot, taskID), true},
	}
	taskDir := ActiveDir(cronRoot, taskID)
	enabled := true
	for _, c := range candidates {
		if st, err := os.Stat(c.dir); err == nil && st.IsDir() {
			taskDir, enabled = c.dir, c.enabled
			// refine enabled from path
			enabled = isEnabledPath(cronRoot, taskDir)
			break
		}
	}
	abs, err := filepath.Abs(taskDir)
	if err == nil {
		taskDir = abs
	}
	logDir := filepath.Join(taskDir, "logs")
	return Paths{
		TaskID:   taskID,
		Root:     cronRoot,
		TaskDir:  taskDir,
		LogDir:   logDir,
		LogFile:  filepath.Join(logDir, taskID+".log"),
		LockFile: filepath.Join(taskDir, taskID+".lock"),
		Enabled:  enabled,
	}, nil
}

func isEnabledPath(cronRoot, taskDir string) bool {
	base := filepath.Base(taskDir)
	if strings.HasSuffix(base, ".disabled") {
		return false
	}
	abs, err := filepath.Abs(taskDir)
	if err != nil {
		return true
	}
	disabledRoots := []string{
		DisabledRoot(cronRoot),
		filepath.Join(cronRoot, DisabledDirName),
	}
	for _, dr := range disabledRoots {
		dAbs, err := filepath.Abs(dr)
		if err != nil {
			continue
		}
		if abs == dAbs || strings.HasPrefix(abs, dAbs+string(os.PathSeparator)) {
			return false
		}
	}
	return true
}

// SetEnabled moves the task directory between active and disabled locations.
func SetEnabled(cronRoot, taskID string, enabled bool) error {
	p, err := Resolve(cronRoot, taskID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(p.TaskDir); err != nil {
		return err
	}
	active := ActiveDir(cronRoot, taskID)
	disabled := DisabledDir(cronRoot, taskID)
	target := active
	if !enabled {
		target = disabled
	}
	curAbs, _ := filepath.Abs(p.TaskDir)
	tgtAbs, _ := filepath.Abs(target)
	if curAbs == tgtAbs {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(target); err == nil {
		// target exists — do not overwrite
		return nil
	}
	return os.Rename(p.TaskDir, target)
}

// DaemonStateDir is cron-tasks/.
func DaemonStateDir(cronRoot string) string {
	return TasksRoot(cronRoot)
}

// DaemonLockPath returns serve lock file path.
func DaemonLockPath(cronRoot string) string {
	return filepath.Join(DaemonStateDir(cronRoot), DaemonLockName)
}

// DaemonPIDPath returns serve pid file path.
func DaemonPIDPath(cronRoot string) string {
	return filepath.Join(DaemonStateDir(cronRoot), DaemonPIDName)
}
