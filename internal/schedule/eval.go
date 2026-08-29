package schedule

import (
	"math/rand"
	"time"
)

// IsDue reports whether the task should fire at `now` given prior state.
func IsDue(spec Spec, now time.Time, st State) bool {
	now = now.Truncate(time.Second)

	switch spec.Kind {
	case KindEvery, KindRandom:
		if st.NextRunAt == nil {
			// first schedule: not due until next is initialized (caller should init)
			return false
		}
		return !now.Before(st.NextRunAt.Truncate(time.Second))

	case KindCron:
		if !MatchesCron(spec.CronExpr, now, spec.HasSeconds) {
			return false
		}
		// prevent double-fire in same window
		if st.LastRunAt != nil {
			last := st.LastRunAt.In(now.Location())
			if spec.HasSeconds {
				if last.Truncate(time.Second).Equal(now) {
					return false
				}
			} else if last.Truncate(time.Minute).Equal(now.Truncate(time.Minute)) {
				return false
			}
		}
		return true

	case KindOn, KindHoliday:
		if now.Month() != time.Month(spec.Month) || now.Day() != spec.Day {
			return false
		}
		if now.Hour() != spec.Hour || now.Minute() != spec.Min || now.Second() != spec.Sec {
			return false
		}
		if st.LastRunAt != nil {
			last := st.LastRunAt.In(now.Location())
			if last.Year() == now.Year() && last.Month() == now.Month() && last.Day() == now.Day() {
				return false // once per year
			}
		}
		return true
	}
	return false
}

// NextAfter returns the next planned fire time at or after `from` (for sleep scheduling).
// For every/random uses state.NextRunAt when set and still in future; otherwise computes.
func NextAfter(spec Spec, from time.Time, st State) time.Time {
	from = from.Truncate(time.Second)

	switch spec.Kind {
	case KindEvery, KindRandom:
		if st.NextRunAt != nil {
			n := st.NextRunAt.In(from.Location()).Truncate(time.Second)
			if !n.Before(from) {
				return n
			}
			// overdue: fire ASAP
			return from
		}
		// not initialized → first delay from now
		return AdvanceNext(spec, from, st)

	case KindCron:
		return nextCronAfter(spec.CronExpr, spec.HasSeconds, from, true)

	case KindOn, KindHoliday:
		return nextAnnual(spec, from, true)
	}
	return time.Time{}
}

// AdvanceNext computes the next run after a fire at `firedAt` (or first schedule).
func AdvanceNext(spec Spec, firedAt time.Time, st State) time.Time {
	firedAt = firedAt.Truncate(time.Second)

	switch spec.Kind {
	case KindEvery:
		base := firedAt
		if st.NextRunAt != nil && !st.NextRunAt.After(firedAt) {
			// catch up: next = last planned + n*every until > now
			base = st.NextRunAt.In(firedAt.Location()).Truncate(time.Second)
		}
		next := base.Add(spec.Every)
		for !next.After(firedAt) {
			next = next.Add(spec.Every)
		}
		return next

	case KindRandom:
		d := randomBetween(spec.RandMin, spec.RandMax)
		return firedAt.Add(d)

	case KindCron:
		return nextCronAfter(spec.CronExpr, spec.HasSeconds, firedAt, false)

	case KindOn, KindHoliday:
		return nextAnnual(spec, firedAt, false)
	}
	return time.Time{}
}

// EnsureNext initializes next_run_at if missing for interval kinds.
func EnsureNext(spec Spec, now time.Time, st State) State {
	now = now.Truncate(time.Second)
	switch spec.Kind {
	case KindEvery, KindRandom:
		if st.NextRunAt == nil {
			var next time.Time
			if spec.Kind == KindEvery {
				next = now.Add(spec.Every) // first run after one interval
			} else {
				next = now.Add(randomBetween(spec.RandMin, spec.RandMax))
			}
			st.NextRunAt = &next
			st.Kind = string(spec.Kind)
		}
	}
	return st
}

// MarkFired updates state after a successful schedule hit (before or after run).
func MarkFired(spec Spec, firedAt time.Time, st State) State {
	firedAt = firedAt.Truncate(time.Second)
	st.LastRunAt = &firedAt
	st.Kind = string(spec.Kind)
	next := AdvanceNext(spec, firedAt, st)
	if !next.IsZero() {
		st.NextRunAt = &next
	}
	return st
}

func nextAnnual(spec Spec, from time.Time, inclusive bool) time.Time {
	loc := from.Location()
	year := from.Year()
	candidate := time.Date(year, time.Month(spec.Month), spec.Day, spec.Hour, spec.Min, spec.Sec, 0, loc)
	if inclusive {
		if !candidate.Before(from) {
			return candidate
		}
	} else {
		if candidate.After(from) {
			return candidate
		}
	}
	return time.Date(year+1, time.Month(spec.Month), spec.Day, spec.Hour, spec.Min, spec.Sec, 0, loc)
}

func randomBetween(minD, maxD time.Duration) time.Duration {
	if maxD <= minD {
		return minD
	}
	span := maxD - minD
	// nanoseconds
	n := rand.Int63n(int64(span) + 1)
	return minD + time.Duration(n)
}
