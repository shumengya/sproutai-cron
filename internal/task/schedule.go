package task

import (
	"os"
	"path/filepath"
	"strings"

	"sproutai-cron/internal/schedule"
)

// ParseSchedule reads schedule.cron for expression and description.
// Expression may be classic cron or modern DSL (@every / @random / …).
func ParseSchedule(taskDir string) (expression, description string) {
	data, err := os.ReadFile(filepath.Join(taskDir, "schedule.cron"))
	if err != nil {
		return "", ""
	}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if description == "" {
				text := strings.TrimSpace(strings.TrimPrefix(line, "#"))
				if isDescriptionComment(text) {
					description = text
				}
			}
			continue
		}
		// first non-comment non-empty line is the schedule expression
		if i := strings.Index(line, " #"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		expression = line
		break
	}
	return expression, description
}

func isDescriptionComment(text string) bool {
	if text == "" {
		return false
	}
	if strings.HasPrefix(text, "开关") {
		return false
	}
	if strings.Contains(text, "cronctl") {
		return false
	}
	return true
}

// UpdateSchedule rewrites description and schedule expression in schedule.cron.
func UpdateSchedule(taskDir, description, scheduleExpr string) error {
	path := filepath.Join(taskDir, "schedule.cron")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	spec, err := schedule.Parse(scheduleExpr)
	if err != nil {
		return err
	}
	newSchedule := spec.Raw
	descText := strings.TrimSpace(description)
	rawLines := strings.Split(string(data), "\n")
	if len(rawLines) > 0 && rawLines[len(rawLines)-1] == "" {
		rawLines = rawLines[:len(rawLines)-1]
	}

	descUpdated := false
	schedUpdated := false
	var newLines []string
	for _, line := range rawLines {
		stripped := strings.TrimSpace(line)
		if !descUpdated && strings.HasPrefix(stripped, "#") {
			text := strings.TrimSpace(strings.TrimPrefix(stripped, "#"))
			if isDescriptionComment(text) {
				if descText != "" {
					newLines = append(newLines, "# "+descText)
				} else {
					newLines = append(newLines, "#")
				}
				descUpdated = true
				continue
			}
		}
		if !schedUpdated && stripped != "" && !strings.HasPrefix(stripped, "#") {
			newLines = append(newLines, newSchedule)
			schedUpdated = true
			continue
		}
		newLines = append(newLines, line)
	}
	if !schedUpdated {
		// append schedule if file had no expression line
		if descText != "" && !descUpdated {
			newLines = append([]string{"# " + descText, ""}, newLines...)
		}
		newLines = append(newLines, newSchedule)
	} else if !descUpdated && descText != "" {
		var inserted []string
		for _, line := range newLines {
			if strings.TrimSpace(line) == newSchedule {
				if len(inserted) == 0 || inserted[len(inserted)-1] != "" {
					inserted = append(inserted, "# "+descText, "")
				}
			}
			inserted = append(inserted, line)
		}
		newLines = inserted
	}
	return os.WriteFile(path, []byte(strings.Join(newLines, "\n")+"\n"), 0o644)
}

// RewriteScheduleOnCreate replaces template name; keeps schedule expression lines.
func RewriteScheduleOnCreate(scheduleFile, templateName, taskID string) error {
	data, err := os.ReadFile(scheduleFile)
	if err != nil {
		return err
	}
	var out []string
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.ReplaceAll(raw, templateName, taskID)
		out = append(out, line)
	}
	if len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return os.WriteFile(scheduleFile, []byte(strings.Join(out, "\n")+"\n"), 0o644)
}
