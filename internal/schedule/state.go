package schedule

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// State tracks last/next run for interval and random schedules.
type State struct {
	LastRunAt *time.Time `json:"last_run_at,omitempty"`
	NextRunAt *time.Time `json:"next_run_at,omitempty"`
	Kind      string     `json:"kind,omitempty"`
}

// StateDir is cron-tasks/.schedule-state
func StateDir(cronRoot string) string {
	return filepath.Join(cronRoot, "cron-tasks", ".schedule-state")
}

// StatePath returns path for one task.
func StatePath(cronRoot, taskID string) string {
	return filepath.Join(StateDir(cronRoot), taskID+".json")
}

// LoadState reads state file; missing → empty.
func LoadState(cronRoot, taskID string) State {
	data, err := os.ReadFile(StatePath(cronRoot, taskID))
	if err != nil {
		return State{}
	}
	var st State
	if json.Unmarshal(data, &st) != nil {
		return State{}
	}
	return st
}

// SaveState writes state atomically.
func SaveState(cronRoot, taskID string, st State) error {
	dir := StateDir(cronRoot)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	path := StatePath(cronRoot, taskID)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	_ = os.Remove(path)
	return os.Rename(tmp, path)
}
