package schedule

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// MatchesCron reports whether when hits a 5- or 6-field cron expression.
func MatchesCron(expr string, when time.Time, hasSeconds bool) bool {
	parts := strings.Fields(expr)
	if hasSeconds && len(parts) == 6 {
		secs, _ := expandField(parts[0], 0, 59)
		if !secs[when.Second()] {
			return false
		}
		return matchYMDHMinDOW(parts[1], parts[2], parts[3], parts[4], parts[5], when)
	}
	if len(parts) == 5 {
		// 5-field: ignore second for match (whole minute)
		return matchYMDHMinDOW(parts[0], parts[1], parts[2], parts[3], parts[4], when)
	}
	return false
}

func matchYMDHMinDOW(minute, hour, dom, month, dow string, when time.Time) bool {
	mins, _ := expandField(minute, 0, 59)
	if !mins[when.Minute()] {
		return false
	}
	hours, _ := expandField(hour, 0, 23)
	if !hours[when.Hour()] {
		return false
	}
	months, _ := expandField(month, 1, 12)
	if !months[int(when.Month())] {
		return false
	}

	domVals, _ := expandField(dom, 1, 31)
	pyDow := int(when.Weekday())
	dowVals, _ := expandField(dow, 0, 7)
	if dowVals[7] {
		dowVals[0] = true
		delete(dowVals, 7)
	}

	domStar := dom == "*"
	dowStar := dow == "*"
	if !domStar && !dowStar {
		return domVals[when.Day()] || dowVals[pyDow]
	}
	if !domStar && !domVals[when.Day()] {
		return false
	}
	if !dowStar && !dowVals[pyDow] {
		return false
	}
	return true
}

// nextCronAfter finds the next fire time strictly after `from` (or equal if inclusive and matches).
func nextCronAfter(expr string, hasSeconds bool, from time.Time, inclusive bool) time.Time {
	t := from.Truncate(time.Second)
	if !inclusive {
		if hasSeconds {
			t = t.Add(time.Second)
		} else {
			// jump to next minute start
			t = t.Truncate(time.Minute).Add(time.Minute)
		}
	} else if !hasSeconds {
		t = t.Truncate(time.Minute)
	}

	// search window: up to ~2 years of minutes, or seconds for 6-field (cap steps)
	maxSteps := 366 * 24 * 60
	if hasSeconds {
		maxSteps = 366 * 24 * 60 * 2 // 2 days of seconds is too small; use minute steps with sec scan
		// For 6-field walk second by second up to 8 days
		maxSteps = 8 * 24 * 3600
		for i := 0; i < maxSteps; i++ {
			if MatchesCron(expr, t, true) {
				return t
			}
			t = t.Add(time.Second)
		}
		return time.Time{}
	}
	for i := 0; i < maxSteps; i++ {
		if MatchesCron(expr, t, false) {
			return t
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}
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
