//go:build !windows

package install

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ensureUserPath(binDir string) (added bool, err error) {
	// Current process PATH
	if pathContains(os.Getenv("PATH"), binDir) {
		// Still ensure shell rc for future sessions if missing
		_, _ = appendPathToShellRC(binDir)
		return false, nil
	}
	// Persist for interactive shells
	changed, err := appendPathToShellRC(binDir)
	if err != nil {
		return false, err
	}
	return changed, nil
}

func setUserEnv(key, value string) error {
	// Unix: SPROUTAI_CRON_ROOT is set by the wrapper script.
	// Also append export to shell rc if not present (optional convenience).
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	line := fmt.Sprintf("export %s=%q", key, value)
	for _, name := range []string{".bashrc", ".zshrc", ".profile"} {
		rc := filepath.Join(home, name)
		if _, err := os.Stat(rc); err != nil {
			continue
		}
		data, err := os.ReadFile(rc)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), "export "+key+"=") {
			continue
		}
		f, err := os.OpenFile(rc, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			continue
		}
		_, _ = fmt.Fprintf(f, "\n# sproutai-cron\n%s\n", line)
		_ = f.Close()
	}
	return nil
}

func appendPathToShellRC(binDir string) (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}
	exportLine := `export PATH="$HOME/.local/bin:$PATH"`
	changed := false
	for _, name := range []string{".bashrc", ".zshrc", ".profile"} {
		rc := filepath.Join(home, name)
		st, err := os.Stat(rc)
		if err != nil || st.IsDir() {
			continue
		}
		data, err := os.ReadFile(rc)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), ".local/bin") {
			continue
		}
		f, err := os.OpenFile(rc, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			continue
		}
		if _, err := fmt.Fprintf(f, "\n# sproutai-cron\n%s\n", exportLine); err == nil {
			changed = true
		}
		_ = f.Close()
	}
	_ = binDir // documented as ~/.local/bin
	return changed, nil
}

func pathContains(pathEnv, dir string) bool {
	dir = filepath.Clean(dir)
	for _, p := range strings.Split(pathEnv, string(os.PathListSeparator)) {
		if filepath.Clean(p) == dir {
			return true
		}
	}
	return false
}
