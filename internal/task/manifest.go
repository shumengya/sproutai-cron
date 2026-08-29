package task

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	MaxTaskTags  = 4
	MaxTagLength = 32
)

// Runtime identifiers.
const (
	RuntimePython     = "python"
	RuntimeJavaScript = "javascript"
	RuntimeBash       = "bash"
	RuntimePowerShell = "powershell"
)

var (
	RuntimeDefaultEntry = map[string]string{
		RuntimePython:     "run.py",
		RuntimeJavaScript: "run.js",
		RuntimeBash:       "run.sh",
		RuntimePowerShell: "run.ps1",
	}
	RuntimeLabel = map[string]string{
		RuntimePython:     "Python",
		RuntimeJavaScript: "JavaScript",
		RuntimeBash:       "Bash",
		RuntimePowerShell: "PowerShell",
	}
	TemplateDirs = map[string]string{
		RuntimePython:     "template-python",
		RuntimeJavaScript: "template-javascript",
		RuntimeBash:       "template-bash",
		RuntimePowerShell: "template-powershell",
	}
	entryToRuntime = map[string]string{
		"run.py":  RuntimePython,
		"run.js":  RuntimeJavaScript,
		"run.sh":  RuntimeBash,
		"run.ps1": RuntimePowerShell,
	}
	runtimeAliases = map[string]string{
		"python": RuntimePython, "py": RuntimePython,
		"javascript": RuntimeJavaScript, "js": RuntimeJavaScript, "node": RuntimeJavaScript,
		"bash": RuntimeBash, "sh": RuntimeBash, "shell": RuntimeBash,
		"powershell": RuntimePowerShell, "ps1": RuntimePowerShell, "pwsh": RuntimePowerShell,
	}
)

// Manifest is task.json content.
type Manifest struct {
	Runtime string   `json:"runtime"`
	Entry   string   `json:"entry"`
	Tags    []string `json:"tags,omitempty"`
}

// LoadManifest reads task.json or infers from entry files.
func LoadManifest(taskDir string) (Manifest, error) {
	mf := filepath.Join(taskDir, "task.json")
	if data, err := os.ReadFile(mf); err == nil {
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			return Manifest{}, err
		}
		rtRaw := "python"
		if v, ok := raw["runtime"].(string); ok && v != "" {
			rtRaw = v
		}
		rt, ok := runtimeAliases[strings.ToLower(strings.TrimSpace(rtRaw))]
		if !ok {
			return Manifest{}, fmt.Errorf("不支持的 runtime: %s", rtRaw)
		}
		entry := RuntimeDefaultEntry[rt]
		if v, ok := raw["entry"].(string); ok && strings.TrimSpace(v) != "" {
			entry = strings.TrimSpace(v)
		}
		if _, err := os.Stat(filepath.Join(taskDir, entry)); err != nil {
			return Manifest{}, fmt.Errorf("task.json 指定的入口不存在: %s", entry)
		}
		return Manifest{Runtime: rt, Entry: entry, Tags: NormalizeTags(raw["tags"])}, nil
	}

	for entry, rt := range entryToRuntime {
		if _, err := os.Stat(filepath.Join(taskDir, entry)); err == nil {
			return Manifest{Runtime: rt, Entry: entry}, nil
		}
	}
	return Manifest{}, fmt.Errorf("未找到 task.json 或可识别的入口脚本")
}

// IsValidTaskDir reports whether dir looks like a cron task.
func IsValidTaskDir(taskDir string) bool {
	st, err := os.Stat(taskDir)
	if err != nil || !st.IsDir() {
		return false
	}
	if _, err := os.Stat(filepath.Join(taskDir, "schedule.cron")); err != nil {
		return false
	}
	_, err = LoadManifest(taskDir)
	return err == nil
}

// NormalizeTags dedupes and truncates tags.
func NormalizeTags(raw any) []string {
	var list []any
	switch v := raw.(type) {
	case []any:
		list = v
	case []string:
		for _, s := range v {
			list = append(list, s)
		}
	default:
		return nil
	}
	var tags []string
	seen := map[string]bool{}
	for _, item := range list {
		s := strings.TrimSpace(fmt.Sprint(item))
		if s == "" || len(s) > MaxTagLength || seen[s] {
			continue
		}
		seen[s] = true
		tags = append(tags, s)
		if len(tags) >= MaxTaskTags {
			break
		}
	}
	return tags
}

// EffectiveTags returns stored tags or default runtime label.
func EffectiveTags(taskDir string, m Manifest) []string {
	if len(m.Tags) > 0 {
		return m.Tags
	}
	// reload tags only from file if LoadManifest already filled
	mf := filepath.Join(taskDir, "task.json")
	if data, err := os.ReadFile(mf); err == nil {
		var raw map[string]any
		if json.Unmarshal(data, &raw) == nil {
			if t := NormalizeTags(raw["tags"]); len(t) > 0 {
				return t
			}
		}
	}
	if label, ok := RuntimeLabel[m.Runtime]; ok {
		return []string{label}
	}
	return []string{m.Runtime}
}

// SaveTags writes tags into task.json.
func SaveTags(taskDir string, tags []string) error {
	m, err := LoadManifest(taskDir)
	if err != nil {
		return err
	}
	normalized := NormalizeTags(tags)
	mf := filepath.Join(taskDir, "task.json")
	data := map[string]any{
		"runtime": m.Runtime,
		"entry":   m.Entry,
		"tags":    normalized,
	}
	if existing, err := os.ReadFile(mf); err == nil {
		var raw map[string]any
		if json.Unmarshal(existing, &raw) == nil {
			for k, v := range raw {
				if k != "runtime" && k != "entry" && k != "tags" {
					data[k] = v
				}
			}
		}
	}
	data["runtime"] = m.Runtime
	data["entry"] = m.Entry
	data["tags"] = normalized
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(mf, append(out, '\n'), 0o644)
}
