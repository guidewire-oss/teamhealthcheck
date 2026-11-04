package orgsyncschedule_test

import (
	"testing"
	"time"

	"github.com/agopalakrishnan/teams360/backend/domain/orgsyncschedule"
)

func TestNextAfter(t *testing.T) {
	cases := []struct {
		name string
		f    orgsyncschedule.Frequency
		from string
		want string
	}{
		{"daily before 02:00", orgsyncschedule.Daily, "2026-11-04T01:00:00Z", "2026-11-04T02:00:00Z"},
		{"daily exactly 02:00 is strictly after", orgsyncschedule.Daily, "2026-11-04T02:00:00Z", "2026-11-05T02:00:00Z"},
		{"daily after 02:00", orgsyncschedule.Daily, "2026-11-04T10:00:00Z", "2026-11-05T02:00:00Z"},
		{"daily month boundary", orgsyncschedule.Daily, "2026-01-31T12:00:00Z", "2026-02-01T02:00:00Z"},
		{"daily year boundary", orgsyncschedule.Daily, "2026-12-31T12:00:00Z", "2027-01-01T02:00:00Z"},
		{"daily leap day", orgsyncschedule.Daily, "2028-02-28T12:00:00Z", "2028-02-29T02:00:00Z"},
		{"weekly midweek", orgsyncschedule.Weekly, "2026-11-04T10:00:00Z", "2026-11-09T02:00:00Z"},
		{"weekly Monday before 02:00", orgsyncschedule.Weekly, "2026-11-09T01:00:00Z", "2026-11-09T02:00:00Z"},
		{"weekly Monday after 02:00", orgsyncschedule.Weekly, "2026-11-09T03:00:00Z", "2026-11-16T02:00:00Z"},
		{"weekly Sunday", orgsyncschedule.Weekly, "2026-11-08T23:59:00Z", "2026-11-09T02:00:00Z"},
		{"weekly year boundary", orgsyncschedule.Weekly, "2026-12-30T10:00:00Z", "2027-01-04T02:00:00Z"},
		{"monthly midmonth", orgsyncschedule.Monthly, "2026-11-04T10:00:00Z", "2026-12-01T02:00:00Z"},
		{"monthly 1st before 02:00", orgsyncschedule.Monthly, "2026-11-01T01:00:00Z", "2026-11-01T02:00:00Z"},
		{"monthly 1st after 02:00", orgsyncschedule.Monthly, "2026-11-01T02:00:01Z", "2026-12-01T02:00:00Z"},
		{"monthly is calendar month (31-day Jan)", orgsyncschedule.Monthly, "2026-01-15T00:00:00Z", "2026-02-01T02:00:00Z"},
		{"monthly year boundary", orgsyncschedule.Monthly, "2026-12-15T00:00:00Z", "2027-01-01T02:00:00Z"},
		{"non-UTC input is converted", orgsyncschedule.Daily, "2026-11-04T23:00:00-05:00", "2026-11-06T02:00:00Z"},
	}
	for _, c := range cases {
		from, _ := time.Parse(time.RFC3339, c.from)
		got, err := orgsyncschedule.NextAfter(c.f, from)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		want, _ := time.Parse(time.RFC3339, c.want)
		if !got.Equal(want) || got.Location() != time.UTC {
			t.Errorf("%s: got %v, want %v", c.name, got, want)
		}
	}
}

func TestNextAfterRejectsUnknownFrequency(t *testing.T) {
	if _, err := orgsyncschedule.NextAfter("HOURLY", time.Now()); err == nil {
		t.Fatal("want error")
	}
	if orgsyncschedule.Frequency("hourly").Valid() {
		t.Fatal("hourly must be invalid")
	}
}
