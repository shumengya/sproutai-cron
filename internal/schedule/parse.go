// Package schedule parses flexible task schedules (cron, @every, @random, @on, @holiday).
package schedule

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Kind identifies schedule type.
type Kind string

const (
	KindEvery   Kind = "every"
	KindRandom  Kind = "random"
	KindCron    Kind = "cron"
	KindOn      Kind = "on"
	KindHoliday Kind = "holiday"
)

// Spec is a normalized schedule.
type Spec struct {
	Kind    Kind
	Raw     string // original expression
	Every   time.Duration
	RandMin time.Duration
	RandMax time.Duration
	// CronExpr is 5 or 6 fields (normalized).
	CronExpr string
	// HasSeconds true if 6-field cron.
	HasSeconds bool
	// On: annual month-day + clock
	Month int
	Day   int
	Hour  int
	Min   int
	Sec   int
	// Holiday name if KindHoliday
	Holiday string
}

// holidays maps alias → month, day, default hour/min/sec (midnight).
var holidays = map[string]struct{ month, day int }{
	"christmas":         {12, 25},
	"xmas":              {12, 25},
	"cn-national-day":   {10, 1},
	"national-day-cn":   {10, 1},
	"china-national-day": {10, 1},
	"国庆":               {10, 1},
	"国庆节":              {10, 1},
	"圣诞":               {12, 25},
	"圣诞节":              {12, 25},
}

// Parse validates and normalizes a schedule expression line.
func Parse(expression string) (Spec, error) {
	raw := strings.TrimSpace(expression)
	if raw == "" {
		return Spec{}, fmt.Errorf("调度表达式为空")
	}
	// strip inline comment
	if i := strings.Index(raw, " #"); i >= 0 {
		raw = strings.TrimSpace(raw[:i])
	}
	low := strings.ToLower(raw)

	switch {
	case strings.HasPrefix(low, "@every"):
		return parseEvery(raw)
	case strings.HasPrefix(low, "@random"):
		return parseRandom(raw)
	case strings.HasPrefix(low, "@on"):
		return parseOn(raw)
	case strings.HasPrefix(low, "@holiday"):
		return parseHoliday(raw)
	case strings.HasPrefix(low, "@weekly"):
		return parseWeekly(raw)
	case strings.HasPrefix(low, "@monthly"):
		return parseMonthly(raw)
	case strings.HasPrefix(low, "@yearly") || strings.HasPrefix(low, "@annually"):
		return parseYearly(raw)
	default:
		return parseCron(raw)
	}
}

// Validate is an alias of Parse that returns normalized Raw string.
func Validate(expression string) (string, error) {
	s, err := Parse(expression)
	if err != nil {
		return "", err
	}
	return s.Raw, nil
}

func parseEvery(raw string) (Spec, error) {
	parts := strings.Fields(raw)
	if len(parts) != 2 {
		return Spec{}, fmt.Errorf("@every 用法: @every 10s | @every 5m | @every 2h")
	}
	d, err := parseDuration(parts[1])
	if err != nil {
		return Spec{}, err
	}
	if d < time.Second {
		return Spec{}, fmt.Errorf("@every 最小间隔为 1s")
	}
	norm := "@every " + formatDuration(d)
	return Spec{Kind: KindEvery, Raw: norm, Every: d}, nil
}

func parseRandom(raw string) (Spec, error) {
	parts := strings.Fields(raw)
	if len(parts) != 3 {
		return Spec{}, fmt.Errorf("@random 用法: @random 1s 60s | @random 30m 3h")
	}
	a, err1 := parseDuration(parts[1])
	b, err2 := parseDuration(parts[2])
	if err1 != nil || err2 != nil {
		return Spec{}, fmt.Errorf("无法解析时长: %s %s", parts[1], parts[2])
	}
	if a < time.Second || b < time.Second {
		return Spec{}, fmt.Errorf("@random 最小间隔为 1s")
	}
	if a > b {
		a, b = b, a
	}
	norm := fmt.Sprintf("@random %s %s", formatDuration(a), formatDuration(b))
	return Spec{Kind: KindRandom, Raw: norm, RandMin: a, RandMax: b}, nil
}

func parseOn(raw string) (Spec, error) {
	// @on 12-25 00:00   or  @on 12-25 00:00:00
	parts := strings.Fields(raw)
	if len(parts) < 2 || len(parts) > 3 {
		return Spec{}, fmt.Errorf("@on 用法: @on 12-25 00:00  或  @on 10-01 08:00")
	}
	md := parts[1]
	clock := "00:00"
	if len(parts) == 3 {
		clock = parts[2]
	}
	month, day, err := parseMonthDay(md)
	if err != nil {
		return Spec{}, err
	}
	h, m, s, err := parseClock(clock)
	if err != nil {
		return Spec{}, err
	}
	norm := fmt.Sprintf("@on %02d-%02d %02d:%02d", month, day, h, m)
	if s != 0 {
		norm = fmt.Sprintf("@on %02d-%02d %02d:%02d:%02d", month, day, h, m, s)
	}
	return Spec{Kind: KindOn, Raw: norm, Month: month, Day: day, Hour: h, Min: m, Sec: s}, nil
}

func parseHoliday(raw string) (Spec, error) {
	parts := strings.Fields(raw)
	if len(parts) < 2 {
		return Spec{}, fmt.Errorf("@holiday 用法: @holiday christmas | @holiday cn-national-day")
	}
	name := strings.ToLower(parts[1])
	// allow Chinese names as second token(s)
	if len(parts) > 2 {
		name = strings.ToLower(strings.Join(parts[1:], ""))
	}
	// try original second field and joined
	h, ok := holidays[name]
	if !ok {
		// try original case for Chinese
		key := strings.Join(parts[1:], "")
		h, ok = holidays[key]
		if !ok {
			h, ok = holidays[strings.ToLower(key)]
		}
	}
	if !ok {
		return Spec{}, fmt.Errorf("未知节日 %q（支持: christmas, cn-national-day, 国庆, 圣诞）", parts[1])
	}
	hour, min, sec := 0, 0, 0
	if len(parts) >= 3 && looksLikeClock(parts[len(parts)-1]) {
		var err error
		hour, min, sec, err = parseClock(parts[len(parts)-1])
		if err != nil {
			return Spec{}, err
		}
	}
	// normalize name
	canon := parts[1]
	if name == "xmas" || name == "christmas" || strings.Contains(name, "圣诞") {
		canon = "christmas"
	}
	if strings.Contains(name, "national") || strings.Contains(name, "国庆") {
		canon = "cn-national-day"
	}
	norm := "@holiday " + canon
	if hour != 0 || min != 0 || sec != 0 {
		norm = fmt.Sprintf("@holiday %s %02d:%02d", canon, hour, min)
	}
	return Spec{
		Kind: KindHoliday, Raw: norm, Holiday: canon,
		Month: h.month, Day: h.day, Hour: hour, Min: min, Sec: sec,
	}, nil
}

func parseWeekly(raw string) (Spec, error) {
	// @weekly mon 10:00
	parts := strings.Fields(raw)
	if len(parts) != 3 {
		return Spec{}, fmt.Errorf("@weekly 用法: @weekly mon 10:00")
	}
	dow, err := parseWeekday(parts[1])
	if err != nil {
		return Spec{}, err
	}
	h, m, _, err := parseClock(parts[2])
	if err != nil {
		return Spec{}, err
	}
	// 5-field: min hour * * dow
	expr := fmt.Sprintf("%d %d * * %d", m, h, dow)
	return Spec{Kind: KindCron, Raw: expr, CronExpr: expr, HasSeconds: false}, nil
}

func parseMonthly(raw string) (Spec, error) {
	// @monthly 1 00:00
	parts := strings.Fields(raw)
	if len(parts) != 3 {
		return Spec{}, fmt.Errorf("@monthly 用法: @monthly 1 00:00")
	}
	day, err := strconv.Atoi(parts[1])
	if err != nil || day < 1 || day > 31 {
		return Spec{}, fmt.Errorf("@monthly 日期须为 1–31")
	}
	h, m, _, err := parseClock(parts[2])
	if err != nil {
		return Spec{}, err
	}
	expr := fmt.Sprintf("%d %d %d * *", m, h, day)
	return Spec{Kind: KindCron, Raw: expr, CronExpr: expr, HasSeconds: false}, nil
}

func parseYearly(raw string) (Spec, error) {
	// @yearly 12-25 00:00
	parts := strings.Fields(raw)
	if len(parts) < 2 {
		return Spec{}, fmt.Errorf("@yearly 用法: @yearly 12-25 00:00")
	}
	// reuse @on
	onLine := "@on " + strings.Join(parts[1:], " ")
	s, err := parseOn(onLine)
	if err != nil {
		return Spec{}, err
	}
	s.Raw = strings.Replace(s.Raw, "@on", "@yearly", 1)
	return s, nil
}

func parseCron(raw string) (Spec, error) {
	parts := strings.Fields(raw)
	switch len(parts) {
	case 5:
		if err := validateCronFields(parts, false); err != nil {
			return Spec{}, err
		}
		norm := strings.Join(parts, " ")
		return Spec{Kind: KindCron, Raw: norm, CronExpr: norm, HasSeconds: false}, nil
	case 6:
		if err := validateCronFields(parts, true); err != nil {
			return Spec{}, err
		}
		norm := strings.Join(parts, " ")
		return Spec{Kind: KindCron, Raw: norm, CronExpr: norm, HasSeconds: true}, nil
	default:
		return Spec{}, fmt.Errorf(
			"无法解析调度 %q\n支持: 五段/六段 cron、@every、@random、@on、@holiday、@weekly、@monthly",
			raw,
		)
	}
}

func validateCronFields(parts []string, withSec bool) error {
	// expand via matching logic
	idx := 0
	if withSec {
		if _, err := expandField(parts[0], 0, 59); err != nil {
			return fmt.Errorf("秒: %w", err)
		}
		idx = 1
	}
	if _, err := expandField(parts[idx], 0, 59); err != nil {
		return fmt.Errorf("分: %w", err)
	}
	if _, err := expandField(parts[idx+1], 0, 23); err != nil {
		return fmt.Errorf("时: %w", err)
	}
	if _, err := expandField(parts[idx+2], 1, 31); err != nil {
		return fmt.Errorf("日: %w", err)
	}
	if _, err := expandField(parts[idx+3], 1, 12); err != nil {
		return fmt.Errorf("月: %w", err)
	}
	if _, err := expandField(parts[idx+4], 0, 7); err != nil {
		return fmt.Errorf("周: %w", err)
	}
	return nil
}

func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, fmt.Errorf("空时长")
	}
	// pure number → seconds
	if onlyDigits(s) {
		n, _ := strconv.Atoi(s)
		return time.Duration(n) * time.Second, nil
	}
	// support 0.5h
	var numStr string
	var unit string
	for i, r := range s {
		if unicode.IsDigit(r) || r == '.' {
			continue
		}
		numStr = s[:i]
		unit = s[i:]
		break
	}
	if numStr == "" || unit == "" {
		// try time.ParseDuration (1h30m etc.)
		d, err := time.ParseDuration(s)
		if err != nil {
			return 0, err
		}
		return d, nil
	}
	f, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, err
	}
	var mult time.Duration
	switch unit {
	case "s", "sec", "secs", "second", "seconds":
		mult = time.Second
	case "m", "min", "mins", "minute", "minutes":
		mult = time.Minute
	case "h", "hr", "hrs", "hour", "hours":
		mult = time.Hour
	case "d", "day", "days":
		mult = 24 * time.Hour
	default:
		return 0, fmt.Errorf("未知时长单位 %q", unit)
	}
	return time.Duration(f * float64(mult)), nil
}

func formatDuration(d time.Duration) string {
	if d%(24*time.Hour) == 0 && d >= 24*time.Hour {
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	}
	if d%time.Hour == 0 && d >= time.Hour {
		return fmt.Sprintf("%dh", int(d/time.Hour))
	}
	if d%time.Minute == 0 && d >= time.Minute {
		return fmt.Sprintf("%dm", int(d/time.Minute))
	}
	return fmt.Sprintf("%ds", int(d/time.Second))
}

func onlyDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

func parseMonthDay(s string) (month, day int, err error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "/", "-")
	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("月-日格式须为 MM-DD: %q", s)
	}
	month, err1 := strconv.Atoi(parts[0])
	day, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || month < 1 || month > 12 || day < 1 || day > 31 {
		return 0, 0, fmt.Errorf("无效月日: %q", s)
	}
	return month, day, nil
}

func parseClock(s string) (h, m, sec int, err error) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, 0, 0, fmt.Errorf("时间格式须为 HH:MM 或 HH:MM:SS: %q", s)
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	sec = 0
	if len(parts) == 3 {
		sec, err = strconv.Atoi(parts[2])
		if err != nil {
			return 0, 0, 0, err
		}
	}
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 || sec < 0 || sec > 59 {
		return 0, 0, 0, fmt.Errorf("无效时间: %q", s)
	}
	return h, m, sec, nil
}

func looksLikeClock(s string) bool {
	return strings.Contains(s, ":")
}

func parseWeekday(s string) (int, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	m := map[string]int{
		"0": 0, "7": 0, "sun": 0, "sunday": 0, "日": 0, "周日": 0, "星期日": 0,
		"1": 1, "mon": 1, "monday": 1, "一": 1, "周一": 1, "星期一": 1,
		"2": 2, "tue": 2, "tues": 2, "tuesday": 2, "二": 2, "周二": 2,
		"3": 3, "wed": 3, "wednesday": 3, "三": 3, "周三": 3,
		"4": 4, "thu": 4, "thur": 4, "thursday": 4, "四": 4, "周四": 4,
		"5": 5, "fri": 5, "friday": 5, "五": 5, "周五": 5,
		"6": 6, "sat": 6, "saturday": 6, "六": 6, "周六": 6,
	}
	if v, ok := m[s]; ok {
		return v, nil
	}
	return 0, fmt.Errorf("未知星期: %q", s)
}
