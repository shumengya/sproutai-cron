// Package runtimeprobe detects available task interpreters.
package runtimeprobe

import (
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"sproutai-cron/internal/interp"
)

// Probe is one runtime availability result.
type Probe struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Available bool    `json:"available"`
	Version   *string `json:"version"`
	Path      *string `json:"path"`
}

// All probes python/node/bash/powershell.
func All() []Probe {
	return []Probe{probePython(), probeNode(), probeBash(), probePowerShell()}
}

func probePython() Probe {
	path := interp.FindPython()
	if path == "" {
		return Probe{ID: "python", Name: "Python", Available: false}
	}
	args := []string{"--version"}
	base := strings.ToLower(filepath.Base(path))
	if base == "py" || base == "py.exe" {
		args = []string{"-3", "--version"}
	}
	out, _ := exec.Command(path, args...).CombinedOutput()
	ver := firstLine(string(out))
	ver = strings.TrimPrefix(ver, "Python ")
	ver = strings.TrimSpace(ver)
	p := path
	if ver == "" {
		return Probe{ID: "python", Name: "Python", Available: true, Path: &p}
	}
	v := ver
	return Probe{ID: "python", Name: "Python", Available: true, Version: &v, Path: &p}
}

func probeNode() Probe {
	path := interp.FindNode()
	if path == "" {
		return Probe{ID: "nodejs", Name: "JavaScript", Available: false}
	}
	out, err := exec.Command(path, "--version").CombinedOutput()
	if err != nil {
		p := path
		return Probe{ID: "nodejs", Name: "JavaScript", Available: false, Path: &p}
	}
	ver := strings.TrimLeft(strings.TrimSpace(firstLine(string(out))), "vV")
	p, v := path, ver
	return Probe{ID: "nodejs", Name: "JavaScript", Available: true, Version: &v, Path: &p}
}

func probeBash() Probe {
	// Same discovery as task runner: Git Bash / MSYS before WSL System32 stub.
	path := interp.FindBash()
	if path == "" {
		return Probe{ID: "bash", Name: "Bash", Available: false}
	}
	out, err := exec.Command(path, "--version").CombinedOutput()
	if err != nil {
		// Still mark available if -c probe already passed in FindBash
		p := path
		return Probe{ID: "bash", Name: "Bash", Available: true, Path: &p}
	}
	re := regexp.MustCompile(`(?i)version\s+([^\s(]+)`)
	ver := firstLine(string(out))
	if m := re.FindStringSubmatch(ver); m != nil {
		ver = m[1]
	}
	p := path
	if ver == "" {
		return Probe{ID: "bash", Name: "Bash", Available: true, Path: &p}
	}
	v := ver
	return Probe{ID: "bash", Name: "Bash", Available: true, Version: &v, Path: &p}
}

func probePowerShell() Probe {
	path := interp.FindPowerShell()
	if path == "" {
		return Probe{ID: "powershell", Name: "PowerShell", Available: false}
	}
	out, err := exec.Command(path, "-NoProfile", "-Command", "$PSVersionTable.PSVersion.ToString()").CombinedOutput()
	if err != nil {
		p := path
		return Probe{ID: "powershell", Name: "PowerShell", Available: false, Path: &p}
	}
	ver := strings.TrimSpace(firstLine(string(out)))
	if ver == "" {
		p := path
		return Probe{ID: "powershell", Name: "PowerShell", Available: true, Path: &p}
	}
	p, v := path, ver
	return Probe{ID: "powershell", Name: "PowerShell", Available: true, Version: &v, Path: &p}
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
