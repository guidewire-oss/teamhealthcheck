// Package scheduler owns the automatic organization-sync schedule: a
// process-lifecycle component (a goroutine, a clock, a shutdown contract),
// which is why it is a separate package from application/services rather
// than another file there -- everything in services is request-scoped and
// returns to a handler.
//
// The dependency direction is deliberately one-way: scheduler imports
// services, never the reverse. That is also why trigger attribution
// (orgprovider.TriggerScheduled) lives in services/orgprovider, not here --
// services cannot depend on a type this package would need to name.
package scheduler

import (
	"time"

	"github.com/agopalakrishnan/teams360/backend/domain/organization"
)

// RunHour is the local hour every frequency runs at. Not admin-configurable
// -- only the frequency is.
const RunHour = 2

// DefaultFrequency is used the first time a schedule is ever enabled with no
// frequency specified. Weekly, not Daily, deliberately: a daily
// hard-deleting sync gives an operator one day to notice bad provider data;
// weekly gives a week.
const DefaultFrequency = organization.OrgSyncFrequencyWeekly

// Interval is one nominal period for a frequency, used only by the bounded
// catch-up check and nowhere that needs a precise duration. Monthly uses 28
// days -- the SHORTEST possible month -- so the catch-up bound errs
// conservatively: a 29-day-overdue monthly schedule classifies as "more than
// one interval" and skips forward rather than firing a stale destructive run.
func Interval(frequency string) time.Duration {
	switch frequency {
	case organization.OrgSyncFrequencyDaily:
		return 24 * time.Hour
	case organization.OrgSyncFrequencyMonthly:
		return 28 * 24 * time.Hour
	default: // Weekly
		return 7 * 24 * time.Hour
	}
}

// ValidFrequency reports whether frequency is one of the three fixed
// choices. Never true for "", which means "not configured", not "invalid" --
// callers that need to distinguish those check for "" separately.
func ValidFrequency(frequency string) bool {
	switch frequency {
	case organization.OrgSyncFrequencyDaily, organization.OrgSyncFrequencyWeekly, organization.OrgSyncFrequencyMonthly:
		return true
	default:
		return false
	}
}

// NextOccurrence returns the first occurrence STRICTLY after "after", at
// RunHour local time in loc, as a UTC instant. Strict inequality is the
// property that stops one occurrence firing twice. Each frequency evaluates
// exactly two candidates (this period's, then next period's) rather than
// looping, so termination is obvious -- the hour is fixed, so there is never
// a third candidate to consider.
func NextOccurrence(frequency string, after time.Time, loc *time.Location) time.Time {
	local := after.In(loc)

	switch frequency {
	case organization.OrgSyncFrequencyDaily:
		y, m, d := local.Date()
		if t := resolveLocalRun(y, m, d, loc); t.After(after) {
			return t
		}
		y, m, d = local.AddDate(0, 0, 1).Date()
		return resolveLocalRun(y, m, d, loc)

	case organization.OrgSyncFrequencyMonthly:
		// Targeting the 1st removes month-length as a problem rather than
		// solving it: day 1 exists in every month, so there is no clamping
		// rule, no 29/30/31 branch, and no February special case. The
		// natural-seeming generalization -- a configurable day-of-month --
		// reintroduces all of that plus "what does the 31st mean in
		// February"; that needs its own design, not an extra parameter here.
		if t := resolveLocalRun(local.Year(), local.Month(), 1, loc); t.After(after) {
			return t
		}
		y, m := nextMonth(local.Year(), local.Month())
		return resolveLocalRun(y, m, 1, loc)

	default: // Weekly
		// Sunday == 0, so subtracting Weekday() lands on this local week's
		// Sunday.
		sun := local.AddDate(0, 0, -int(local.Weekday()))
		y, m, d := sun.Date()
		if t := resolveLocalRun(y, m, d, loc); t.After(after) {
			return t
		}
		y, m, d = sun.AddDate(0, 0, 7).Date()
		return resolveLocalRun(y, m, d, loc)
	}
}

// nextMonth advances by field, never via AddDate(0, 1, 0). AddDate normalizes
// overflow, so Jan 31 + 1 month is Mar 3 -- skipping February entirely. That
// bug passes every test written in March and fires on Jan 31.
func nextMonth(y int, m time.Month) (int, time.Month) {
	if m == time.December {
		return y + 1, time.January
	}
	return y, m + 1
}

// resolveLocalRun returns the earliest instant on local calendar day y-m-d in
// loc whose local wall-clock hour is at least RunHour, as a UTC instant.
//
// That single rule gives the right answer for all three day shapes with no
// special-casing: a normal day gives RunHour:00 local. A spring-forward day
// where RunHour:00 does not exist gives the first instant past the gap
// (whatever hour that normalizes to). A fall-back day where RunHour:00
// occurs twice gives the EARLIER of the two, because it is earliest.
//
// Two walk phases, forward then backward, because time.Date's normalization
// for a non-existent local time is not guaranteed to land on or after
// RunHour -- verified empirically, not merely assumed: for the 2026-03-08
// America/New_York gap, time.Date(2026,3,8,2,0,0,0,loc) returns the instant
// that DISPLAYS as 01:00 EST (an hour BEFORE RunHour), not something at or
// past the gap. A backward-only walk can never correct that -- it only ever
// moves earlier -- so the forward phase runs first to reach a reading that
// is actually >= RunHour on the right calendar day, and only then does the
// backward phase look for an earlier instant that reads the same (the
// fall-back overlap case).
func resolveLocalRun(y int, m time.Month, d int, loc *time.Location) time.Time {
	t := time.Date(y, m, d, RunHour, 0, 0, 0, loc)

	// Phase 1: cross a gap. Walk forward while the current reading is NOT
	// yet >= RunHour on the target calendar day. Bounded at 4 iterations:
	// real DST transitions are 30 minutes to 2 hours, and an unbounded loop
	// here would be a liveness bug in a scheduler.
	for i := 0; i < 4; i++ {
		lt := t.In(loc)
		ly, lm, ld := lt.Date()
		if ly == y && lm == m && ld == d && lt.Hour() >= RunHour {
			break
		}
		t = t.Add(time.Hour)
	}

	// Phase 2: resolve a fall-back overlap deterministically. Walk back
	// while the previous hour is still on this calendar day and still reads
	// RunHour or later.
	//
	// Determinism argument: if time.Date (or phase 1) landed on the LATER of
	// two RunHour:00 instants on a fall-back day, one step back reaches the
	// EARLIER one (same day, hour still >= RunHour) and the next step (hour
	// RunHour-1) stops -- result: the earlier instant. If it landed on the
	// earlier one directly, the first step back already reads an hour below
	// RunHour and stops immediately -- same result either way.
	for i := 0; i < 4; i++ {
		prev := t.Add(-time.Hour)
		pl := prev.In(loc)
		py, pm, pd := pl.Date()
		if py != y || pm != m || pd != d || pl.Hour() < RunHour {
			break
		}
		t = prev
	}

	// Guard the calendar day: a zone whose gap spans midnight can leave this
	// normalized onto the following day (America/Santiago transitions at
	// midnight and is the zone that exercises this), or phase 1's bounded
	// walk could exhaust its 4 iterations without ever reaching the target
	// day for a pathological gap wider than 4 hours (none exist in practice,
	// but the guard costs nothing and removes the need to prove it).
	if ly, lm, ld := t.In(loc).Date(); ly != y || lm != m || ld != d {
		t = time.Date(y, m, d, 0, 0, 0, 0, loc).Add(RunHour * time.Hour)
	}

	return t.UTC()
}
