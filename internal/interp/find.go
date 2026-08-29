// Package interp locates task interpreters (python/node/bash/powershell).
// Shared by runner and runtimeprobe so WebUI detection matches execution.
package interp

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// FindPython returns a usable python executable path, or "".
func FindPython() string {
	if runtime.GOOS == "windows" {
		for _, c := range windowsPythonCandidates() {
			if fileExists(c) && IsUsablePython(c) {
				return c
			}
		}
	}
	for _, name := range []string{"python3", "python", "py"} {
		if p, err := exec.LookPath(name); err == nil {
			if isWindowsAppsStub(p) {
				continue
			}
			if IsUsablePython(p) {
				return p
			}
		}
	}
	if p, err := exec.LookPath("py"); err == nil {
		return p
	}
	return ""
}

// FindNode returns node path or "".
func FindNode() string {
	return lookPathFirst("node")
}

// FindBash returns a real bash (prefers Git Bash / MSYS on Windows; skips WSL stub).
func FindBash() string {
	if runtime.GOOS == "windows" {
		for _, c := range windowsBashCandidates() {
			if fileExists(c) && IsUsableBash(c) {
				return c
			}
		}
	}
	if p, err := exec.LookPath("bash"); err == nil {
		for _, cand := range pathCandidates("bash") {
			if isWSLOrStoreStub(cand) {
				continue
			}
			if IsUsableBash(cand) {
				return cand
			}
		}
		if !isWSLOrStoreStub(p) && IsUsableBash(p) {
			return p
		}
	}
	for _, p := range pathCandidates("bash") {
		if !isWSLOrStoreStub(p) && fileExists(p) {
			return p
		}
	}
	return ""
}

// FindPowerShell returns pwsh or powershell path, or "".
func FindPowerShell() string {
	return lookPathFirst("pwsh", "powershell")
}

// IsUsableBash reports whether path runs as a real bash (not WSL install stub).
func IsUsableBash(path string) bool {
	cmd := exec.Command(path, "-c", "echo __sprout_ok__")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "__sprout_ok__")
}

// IsUsablePython reports whether path can run a one-liner.
func IsUsablePython(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	args := []string{"-c", "print(1)"}
	if base == "py" || base == "py.exe" {
		args = []string{"-3", "-c", "print(1)"}
	}
	cmd := exec.Command(path, args...)
	out, err := cmd.CombinedOutput()
	return err == nil && strings.Contains(string(out), "1")
}

func windowsPythonCandidates() []string {
	var out []string
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out,
			filepath.Join(home, "scoop", "apps", "python", "current", "python.exe"),
			filepath.Join(home, "scoop", "shims", "python.exe"),
			filepath.Join(home, "AppData", "Local", "Programs", "Python", "Python314", "python.exe"),
			filepath.Join(home, "AppData", "Local", "Programs", "Python", "Python313", "python.exe"),
			filepath.Join(home, "AppData", "Local", "Programs", "Python", "Python312", "python.exe"),
			filepath.Join(home, "AppData", "Local", "Programs", "Python", "Python311", "python.exe"),
		)
	}
	return out
}

func windowsBashCandidates() []string {
	var out []string
	pf := os.Getenv("ProgramFiles")
	if pf == "" {
		pf = `C:\Program Files`
	}
	pf86 := os.Getenv("ProgramFiles(x86)")
	out = append(out,
		filepath.Join(pf, "Git", "bin", "bash.exe"),
		filepath.Join(pf, "Git", "usr", "bin", "bash.exe"),
		`C:\msys64\usr\bin\bash.exe`,
		`C:\msys64\bin\bash.exe`,
		`C:\cygwin64\bin\bash.exe`,
	)
	if pf86 != "" {
		out = append(out,
			filepath.Join(pf86, "Git", "bin", "bash.exe"),
			filepath.Join(pf86, "Git", "usr", "bin", "bash.exe"),
		)
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		out = append(out, filepath.Join(local, "Programs", "Git", "bin", "bash.exe"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out, filepath.Join(home, "scoop", "apps", "git", "current", "bin", "bash.exe"))
	}
	return out
}

func isWSLOrStoreStub(path string) bool {
	lower := strings.ToLower(filepath.Clean(path))
	sep := string(filepath.Separator)
	if strings.Contains(lower, `system32`+sep+`bash`) {
		return true
	}
	if strings.Contains(lower, `windowsapps`+sep+`bash`) {
		return true
	}
	if strings.Contains(lower, `system32/bash`) || strings.Contains(lower, `windowsapps/bash`) {
		return true
	}
	return isWindowsAppsStub(path)
}

func isWindowsAppsStub(path string) bool {
	lower := strings.ToLower(filepath.Clean(path))
	return strings.Contains(lower, `windowsapps`+string(filepath.Separator)) ||
		strings.Contains(lower, `windowsapps/`)
}

func lookPathFirst(names ...string) string {
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			if isWindowsAppsStub(p) {
				continue
			}
			return p
		}
	}
	return ""
}

func pathCandidates(name string) []string {
	var out []string
	seen := map[string]bool{}
	pathEnv := os.Getenv("PATH")
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			continue
		}
		cands := []string{filepath.Join(dir, name)}
		if runtime.GOOS == "windows" {
			cands = []string{
				filepath.Join(dir, name+".exe"),
				filepath.Join(dir, name+".cmd"),
				filepath.Join(dir, name+".bat"),
				filepath.Join(dir, name),
			}
		}
		for _, c := range cands {
			if !fileExists(c) {
				continue
			}
			abs, err := filepath.Abs(c)
			if err != nil {
				abs = c
			}
			key := strings.ToLower(abs)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, abs)
		}
	}
	return out
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
