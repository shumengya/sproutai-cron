// Package cronexpr validates and matches 5-field cron expressions.
// Fields: minute hour day-of-month month day-of-week
// Supports: *  n  n-m  */n  n-m/step  comma lists; DOW 0/7 = Sunday.
package cronexpr

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Validate checks and normalizes a 5-field cron expression.
func Validate(expression string) (string, error) {
	parts := strings.Fields(strings.TrimSpace(expression))
	if len(parts) != 5 {
		return "", fmt.Errorf("cron 表达式须为 5 段：分 时 日 月 周")
	}
	minute, hour, dom, month, dow := parts[0], parts[1], parts[2], parts[3], parts[4]
	if _, err := expandField(minute, 0, 59); err != nil {
		return "", err
	}
	if _, err := expandField(hour, 0, 23); err != nil {
		return "", err
	}
	if _, err := expandField(dom, 1, 31); err != nil {
		return "", err
	}
	if _, err := expandField(month, 1, 12); err != nil {
		return "", err
	}
	if _, err := expandField(dow, 0, 7); err != nil {
		return "", err
	}
	return strings.Join(parts, " "), nil
}

// Matches reports whether when hits the expression.
func Matches(expression string, when time.Time) (bool, error) {
	norm, err := Validate(expression)
	if err != nil {
		return false, err
	}
	parts := strings.Fields(norm)
	minute, hour, dom, month, dow := parts[0], parts[1], parts[2], parts[3], parts[4]

	mins, _ := expandField(minute, 0, 59)
	if !mins[when.Minute()] {
		return false, nil
	}
	hours, _ := expandField(hour, 0, 23)
	if !hours[when.Hour()] {
		return false, nil
	}
	months, _ := expandField(month, 1, 12)
	if !months[int(when.Month())] {
		return false, nil
	}

	domVals, _ := expandField(dom, 1, 31)
	// Go Weekday: Sunday=0 … Saturday=6 — same as cron 0=Sunday
	pyDow := int(when.Weekday())
	dowVals, _ := expandField(dow, 0, 7)
	if dowVals[7] {
		dowVals[0] = true
		delete(dowVals, 7)
	}

	domStar := dom == "*"
	dowStar := dow == "*"
	if !domStar && !dowStar {
		return domVals[when.Day()] || dowVals[pyDow], nil
	}
	if !domStar && !domVals[when.Day()] {
		return false, nil
	}
	if !dowStar && !dowVals[pyDow] {
		return false, nil
	}
	return true, nil
}

func expandField(field string, minV, maxV int) (map[int]bool, error) {
	values := make(map[int]bool)
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("空 cron 字段: %q", field)
		}
		step := 1
		rangePart := part
		if i := strings.Index(part, "/"); i >= 0 {
			rangePart = part[:i]
			s, err := strconv.Atoi(part[i+1:])
			if err != nil || s < 1 {
				return nil, fmt.Errorf("cron 步长须 ≥ 1: %q", part)
			}
			step = s
		}

		var start, end int
		if rangePart == "*" {
			start, end = minV, maxV
		} else if j := strings.Index(rangePart, "-"); j >= 0 {
			a, err1 := strconv.Atoi(rangePart[:j])
			b, err2 := strconv.Atoi(rangePart[j+1:])
			if err1 != nil || err2 != nil {
				return nil, fmt.Errorf("无法解析 cron 字段: %q", part)
			}
			start, end = a, b
		} else {
			n, err := strconv.Atoi(rangePart)
			if err != nil {
				return nil, fmt.Errorf("无法解析 cron 字段: %q", part)
			}
			start, end = n, n
		}
		if start < minV || end > maxV || start > end {
			return nil, fmt.Errorf("cron 字段超出范围 [%d,%d]: %q", minV, maxV, part)
		}
		for v := start; v <= end; v += step {
			values[v] = true
		}
	}
	return values, nil
}
