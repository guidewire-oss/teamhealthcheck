package healthcheck

import (
	"regexp"
	"testing"
	"time"
)

func mustParseDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("failed to parse date %q: %v", s, err)
	}
	return d
}

func TestHalfYearOf(t *testing.T) {
	cases := []struct {
		name     string
		date     string
		wantHalf int
		wantYear int
	}{
		{"January 1 is the start of H1", "2026-01-01", 1, 2026},
		{"June 30 is the end of H1", "2026-06-30", 1, 2026},
		{"July 1 is the start of H2", "2026-07-01", 2, 2026},
		{"December 31 is the end of H2", "2026-12-31", 2, 2026},
		{"works for any year", "1999-03-15", 1, 1999},
		{"works for any year (H2)", "2101-11-02", 2, 2101},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			half, year := HalfYearOf(mustParseDate(t, tc.date))
			if half != tc.wantHalf || year != tc.wantYear {
				t.Errorf("HalfYearOf(%s) = (%d, %d), want (%d, %d)", tc.date, half, year, tc.wantHalf, tc.wantYear)
			}
		})
	}
}

func TestActiveHalfYearPeriod(t *testing.T) {
	cases := []struct {
		date string
		want string
	}{
		{"2026-03-15", "H1 2026"},
		{"2026-11-20", "H2 2026"},
		{"1999-01-01", "H1 1999"},
		{"2101-12-31", "H2 2101"},
	}

	for _, tc := range cases {
		t.Run(tc.date, func(t *testing.T) {
			if got := ActiveHalfYearPeriod(mustParseDate(t, tc.date)); got != tc.want {
				t.Errorf("ActiveHalfYearPeriod(%s) = %q, want %q", tc.date, got, tc.want)
			}
		})
	}
}

func TestNextHalfYearStart(t *testing.T) {
	cases := []struct {
		name string
		date string
		want string
	}{
		{"from the start of H1", "2026-01-01", "2026-07-01"},
		{"from the middle of H1", "2026-03-15", "2026-07-01"},
		{"from the last day of H1", "2026-06-30", "2026-07-01"},
		{"from the start of H2", "2026-07-01", "2027-01-01"},
		{"from the middle of H2", "2026-11-20", "2027-01-01"},
		{"from the last day of H2", "2026-12-31", "2027-01-01"},
		{"works for any year", "1999-08-01", "2000-01-01"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NextHalfYearStart(mustParseDate(t, tc.date))
			if got.Format("2006-01-02") != tc.want {
				t.Errorf("NextHalfYearStart(%s) = %s, want %s", tc.date, got.Format("2006-01-02"), tc.want)
			}
		})
	}
}

func TestNextEligibleDate(t *testing.T) {
	cases := []struct {
		name string
		from string
		want string
	}{
		// Submitting exactly on a half-year boundary: the six-month cooldown and the next
		// half-year start coincide, so either rule alone would give the same answer.
		{"exactly at the start of H1: both rules agree", "2026-01-01", "2026-07-01"},
		{"exactly at the start of H2: both rules agree", "2026-07-01", "2027-01-01"},

		// Submitting mid-period or late in a period: the half-year boundary is reached well
		// before six calendar months would elapse, so it becomes the binding (earlier) rule.
		// This is the acceptance-criteria example: submitting H2 in November must not force a
		// wait until the six-month mark in May -- H1 of the following year opens January 1.
		{"mid H2 (November): next half-year wins over the six-month cooldown", "2026-11-15", "2027-01-01"},
		{"mid H1 (February): next half-year wins over the six-month cooldown", "2026-02-10", "2026-07-01"},
		{"last day of H1: next half-year is the very next day", "1999-06-30", "1999-07-01"},
		{"last day of H2: next half-year is the very next day, rolling the year", "2026-12-31", "2027-01-01"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NextEligibleDate(mustParseDate(t, tc.from))
			if got.Format("2006-01-02") != tc.want {
				t.Errorf("NextEligibleDate(%s) = %s, want %s", tc.from, got.Format("2006-01-02"), tc.want)
			}
		})
	}
}

func TestIsWithinCooldown(t *testing.T) {
	cases := []struct {
		name      string
		last      string
		candidate string
		want      bool
	}{
		{"same day is blocked", "2026-01-15", "2026-01-15", true},
		{"later the same half-year is still blocked (duplicate-period prevention)", "2026-01-15", "2026-06-30", true},
		{"the moment the next half-year starts is eligible", "2026-01-15", "2026-07-01", false},
		{"well into the next half-year is eligible", "2026-01-15", "2026-07-14", false},
		{"well after six months is eligible", "2026-01-15", "2027-01-15", false},

		// The acceptance-criteria scenario: a November H2 submission must not block January's
		// H1, even though only ~6 weeks (nowhere near six months) have passed.
		{"a November H2 submission does not block the following January (H1)", "2026-11-15", "2027-01-01", false},
		{"the day before the following January is still blocked (still H2)", "2026-11-15", "2026-12-31", true},
		{"under six months and still the same half-year remains blocked", "2025-11-20", "2025-12-31", true},
		{"well under six months but into a new half-year is eligible", "2025-11-20", "2026-05-19", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsWithinCooldown(mustParseDate(t, tc.last), mustParseDate(t, tc.candidate))
			if got != tc.want {
				t.Errorf("IsWithinCooldown(%s, %s) = %v, want %v", tc.last, tc.candidate, got, tc.want)
			}
		})
	}
}

// TestIsWithinCooldown_NeverAllowsADuplicateWithinTheSameHalfYear is a property-style check
// that submitting again anywhere within the same half-year period as the last submission is
// always blocked, regardless of how many days remain in that period -- i.e. the six-month
// cooldown and the half-year-boundary rule never combine to allow a same-period duplicate.
func TestIsWithinCooldown_NeverAllowsADuplicateWithinTheSameHalfYear(t *testing.T) {
	lastSubmissions := []string{"2026-01-01", "2026-03-15", "2026-06-30", "2026-07-01", "2026-09-20", "2026-12-31"}

	for _, last := range lastSubmissions {
		lastDate := mustParseDate(t, last)
		lastHalf, lastYear := HalfYearOf(lastDate)

		// Probe every day from the submission date through the last day of its half-year.
		periodEnd := NextHalfYearStart(lastDate).AddDate(0, 0, -1)
		for d := lastDate; !d.After(periodEnd); d = d.AddDate(0, 0, 1) {
			half, year := HalfYearOf(d)
			if half != lastHalf || year != lastYear {
				t.Fatalf("test setup error: %s is not in the same half-year as %s", d.Format("2006-01-02"), last)
			}
			if !IsWithinCooldown(lastDate, d) {
				t.Errorf("IsWithinCooldown(%s, %s) = false, want true (same half-year as the last submission)",
					last, d.Format("2006-01-02"))
			}
		}
	}
}

func TestFormatPeriodForDisplay(t *testing.T) {
	cases := []struct {
		name   string
		period string
		want   string
	}{
		{"half-yearly is re-ordered to H<n> <year>", "2026 H1", "H1 2026"},
		{"half-yearly H2", "2026 H2", "H2 2026"},
		{"legacy quarterly Q1 collapses to H1", "2026 Q1", "H1 2026"},
		{"legacy quarterly Q2 collapses to H1", "2026 Q2", "H1 2026"},
		{"legacy quarterly Q3 collapses to H2", "2026 Q3", "H2 2026"},
		{"legacy quarterly Q4 collapses to H2", "2026 Q4", "H2 2026"},
		{"legacy 1st half maps to H2 of the same year", "2024 - 1st Half", "H2 2024"},
		{"legacy 2nd half maps to H1 of the following year", "2024 - 2nd Half", "H1 2025"},
		{"monthly periods are unchanged", "2026 Mar", "2026 Mar"},
		{"yearly periods are unchanged", "2026", "2026"},
		{"unrecognized periods are returned unchanged", "not-a-period", "not-a-period"},
		{"multiple years: quarterly", "2099 Q1", "H1 2099"},
		{"multiple years: half-yearly", "1999 H2", "H2 1999"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatPeriodForDisplay(tc.period)
			if got != tc.want {
				t.Errorf("FormatPeriodForDisplay(%q) = %q, want %q", tc.period, got, tc.want)
			}
			if matched, _ := regexp.MatchString(`Q[1-4]`, got); matched {
				t.Errorf("FormatPeriodForDisplay(%q) = %q must never contain a quarter label", tc.period, got)
			}
		})
	}
}
