package schedule

import (
	"testing"
	"time"
)

func TestParseEvery(t *testing.T) {
	s, err := Parse("@every 10s")
	if err != nil {
		t.Fatal(err)
	}
	if s.Kind != KindEvery || s.Every != 10*time.Second {
		t.Fatalf("%+v", s)
	}
}

func TestParseRandom(t *testing.T) {
	s, err := Parse("@random 1s 60s")
	if err != nil {
		t.Fatal(err)
	}
	if s.RandMin != time.Second || s.RandMax != 60*time.Second {
		t.Fatalf("%+v", s)
	}
	s, err = Parse("@random 30m 3h")
	if err != nil {
		t.Fatal(err)
	}
	if s.RandMin != 30*time.Minute || s.RandMax != 3*time.Hour {
		t.Fatalf("%+v", s)
	}
}

func TestParseCronAndWeekly(t *testing.T) {
	s, err := Parse("0 10 * * 1")
	if err != nil || s.Kind != KindCron {
		t.Fatalf("%v %+v", err, s)
	}
	s, err = Parse("@weekly mon 10:00")
	if err != nil {
		t.Fatal(err)
	}
	if s.CronExpr != "0 10 * * 1" {
		t.Fatalf("got %s", s.CronExpr)
	}
}

func TestParseOnHoliday(t *testing.T) {
	s, err := Parse("@on 12-25 00:00")
	if err != nil || s.Month != 12 || s.Day != 25 {
		t.Fatalf("%v %+v", err, s)
	}
	s, err = Parse("@holiday cn-national-day")
	if err != nil || s.Month != 10 || s.Day != 1 {
		t.Fatalf("%v %+v", err, s)
	}
	s, err = Parse("@holiday christmas")
	if err != nil || s.Month != 12 || s.Day != 25 {
		t.Fatalf("%v %+v", err, s)
	}
}

func TestIsDueEvery(t *testing.T) {
	spec, _ := Parse("@every 10s")
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)
	next := now.Add(-time.Second)
	st := State{NextRunAt: &next}
	if !IsDue(spec, now, st) {
		t.Fatal("expected due")
	}
	future := now.Add(5 * time.Second)
	st.NextRunAt = &future
	if IsDue(spec, now, st) {
		t.Fatal("not due yet")
	}
}

func TestCronMatchesMonday(t *testing.T) {
	// 2026-07-27 is Monday
	mon := time.Date(2026, 7, 27, 10, 0, 0, 0, time.Local)
	if !MatchesCron("0 10 * * 1", mon, false) {
		t.Fatal("monday 10:00 should match")
	}
	if MatchesCron("0 10 * * 1", mon.Add(time.Hour), false) {
		t.Fatal("11:00 should not match")
	}
}
