package scheduler_test

import (
	"testing"
	"time"

	"github.com/agopalakrishnan/teams360/backend/application/scheduler"
	"github.com/agopalakrishnan/teams360/backend/domain/organization"
)

func mustLoadLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("failed to load location %q: %v", name, err)
	}
	return loc
}

func TestNextOccurrenceDailyIsStrictlyAfter(t *testing.T) {
	after := time.Date(2026, 3, 10, 2, 0, 0, 0, time.UTC) // exactly 02:00 UTC
	next := scheduler.NextOccurrence(organization.OrgSyncFrequencyDaily, after, time.UTC)
	want := time.Date(2026, 3, 11, 2, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("an occurrence exactly at 02:00 must not fire again the same day; got %v, want %v", next, want)
	}
}

func TestNextOccurrenceDailyLaterSameDayGivesTomorrow(t *testing.T) {
	after := time.Date(2026, 3, 10, 5, 0, 0, 0, time.UTC)
	next := scheduler.NextOccurrence(organization.OrgSyncFrequencyDaily, after, time.UTC)
	want := time.Date(2026, 3, 11, 2, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("got %v, want %v", next, want)
	}
}

func TestNextOccurrenceWeeklyIsSunday(t *testing.T) {
	// 2026-03-10 is a Tuesday.
	after := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	next := scheduler.NextOccurrence(organization.OrgSyncFrequencyWeekly, after, time.UTC)
	if next.Weekday() != time.Sunday {
		t.Fatalf("expected a Sunday, got %v (%v)", next, next.Weekday())
	}
	if !next.After(after) {
		t.Fatalf("occurrence must be strictly after %v, got %v", after, next)
	}
}

func TestNextOccurrenceMonthlyIsTheFirst(t *testing.T) {
	after := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	next := scheduler.NextOccurrence(organization.OrgSyncFrequencyMonthly, after, time.UTC)
	if next.Day() != 1 {
		t.Fatalf("expected the 1st, got day %d (%v)", next.Day(), next)
	}
	want := time.Date(2026, 4, 1, 2, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("got %v, want %v", next, want)
	}
}

// TestNextOccurrenceMonthlyJanuary31DoesNotSkipFebruary is the canonical
// AddDate(0,1,0) trap: that call normalizes Jan 31 + 1 month to Mar 3,
// skipping February. NextOccurrence must advance by field instead.
func TestNextOccurrenceMonthlyJanuary31DoesNotSkipFebruary(t *testing.T) {
	after := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)
	next := scheduler.NextOccurrence(organization.OrgSyncFrequencyMonthly, after, time.UTC)
	want := time.Date(2026, 2, 1, 2, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("expected Feb 1 (not skipped to March), got %v", next)
	}
}

func TestNextOccurrenceMonthlyDecemberRollsOverToJanuary(t *testing.T) {
	after := time.Date(2026, 12, 15, 12, 0, 0, 0, time.UTC)
	next := scheduler.NextOccurrence(organization.OrgSyncFrequencyMonthly, after, time.UTC)
	want := time.Date(2027, 1, 1, 2, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("got %v, want %v", next, want)
	}
}

// TestNextOccurrenceSpringForwardGap exercises a real DST gap: America/New_York
// springs forward at 2026-03-08 02:00 local -> 03:00 local. 02:00 does not
// exist that day, so the occurrence must be the first instant past the gap.
func TestNextOccurrenceSpringForwardGap(t *testing.T) {
	loc := mustLoadLocation(t, "America/New_York")
	after := time.Date(2026, 3, 7, 12, 0, 0, 0, loc)
	next := scheduler.NextOccurrence(organization.OrgSyncFrequencyDaily, after, loc)

	local := next.In(loc)
	if local.Year() != 2026 || local.Month() != 3 || local.Day() != 8 {
		t.Fatalf("expected March 8 local, got %v", local)
	}
	if local.Hour() < 2 {
		t.Fatalf("expected the resolved hour to be at or past the gap (>= 2), got %v", local)
	}
}

// TestNextOccurrenceFallBackDayIsUnambiguousAtRunHour exercises the US
// fall-back transition, America/New_York, 2026-11-01: clocks read 02:00:00
// EDT and are set back to 01:00:00 EST. The REPEATED hour is 01:00-01:59, not
// 02:00 -- RunHour (2) therefore occurs exactly once that day, as EST
// (UTC-5), with no ambiguity for resolveLocalRun to resolve. This is the
// correctness of the calendar-day guard on an ordinary (if unusual) day, not
// a test of the backward-walk determinism logic -- an IANA zone with a
// verified fall-back transition strictly AFTER RunHour would be needed for
// that, and none is asserted here without being certain of its exact rule.
func TestNextOccurrenceFallBackDayIsUnambiguousAtRunHour(t *testing.T) {
	loc := mustLoadLocation(t, "America/New_York")
	after := time.Date(2026, 10, 31, 12, 0, 0, 0, loc)
	next := scheduler.NextOccurrence(organization.OrgSyncFrequencyDaily, after, loc)

	local := next.In(loc)
	if local.Year() != 2026 || local.Month() != 11 || local.Day() != 1 || local.Hour() != 2 {
		t.Fatalf("expected Nov 1 02:00 local, got %v", local)
	}
	if next.UTC().Hour() != 7 {
		t.Fatalf("expected the single, unambiguous 02:00 EST (UTC-5) instant -> 07:00 UTC, got %v UTC", next.UTC())
	}
}

func TestIntervalMonthlyUsesTheShortestMonth(t *testing.T) {
	got := scheduler.Interval(organization.OrgSyncFrequencyMonthly)
	want := 28 * 24 * time.Hour
	if got != want {
		t.Fatalf("monthly interval must be the shortest possible month (28 days) for a conservative catch-up bound; got %v", got)
	}
}

func TestValidFrequency(t *testing.T) {
	for _, f := range []string{organization.OrgSyncFrequencyDaily, organization.OrgSyncFrequencyWeekly, organization.OrgSyncFrequencyMonthly} {
		if !scheduler.ValidFrequency(f) {
			t.Fatalf("expected %q to be valid", f)
		}
	}
	for _, f := range []string{"", "hourly", "yearly", "Daily"} {
		if scheduler.ValidFrequency(f) {
			t.Fatalf("expected %q to be invalid", f)
		}
	}
}
