// Package root locates the sproutai-cron repository root (CRON_ROOT).
package root

import (
	"fmt"
	"os"
	"path/filepath"
)

const TasksDirName = "cron-tasks"

// Find returns the cron project root directory.
// Order: SPROUTAI_CRON_ROOT / CRON_ROOT env → executable dir → cwd walk-up.
func Find() (string, error) {
	for _, key := range []string{"SPROUTAI_CRON_ROOT", "CRON_ROOT"} {
		if v := os.Getenv(key); v != "" {
			abs, err := filepath.Abs(v)
			if err != nil {
				return "", err
			}
			if isRoot(abs) {
				return abs, nil
			}
			return abs, nil // honor explicit env even if tasks dir missing yet
		}
	}

	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		if resolved, err := filepath.EvalSymlinks(dir); err == nil {
			dir = resolved
		}
		if isRoot(dir) {
			return dir, nil
		}
		// install may put binary elsewhere; try parent of bin layout
		parent := filepath.Dir(dir)
		if isRoot(parent) {
			return parent, nil
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for {
		if isRoot(dir) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("无法定位 sproutai-cron 根目录（需含 %s/）；可设置 SPROUTAI_CRON_ROOT", TasksDirName)
}

func isRoot(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, TasksDirName))
	return err == nil && st.IsDir()
}

// TasksRoot returns <root>/cron-tasks.
func TasksRoot(root string) string {
	return filepath.Join(root, TasksDirName)
}
