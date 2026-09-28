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

func TestParsePeriodHalfYear(t *testing.T) {
	cases := []struct {
		name     string
		period   string
		wantHalf int
		wantYear int
		wantOK   bool
	}{
		{"storage half-yearly H1", "2026 H1", 1, 2026, true},
		{"storage half-yearly H2", "2026 H2", 2, 2026, true},
		{"legacy quarterly Q1 collapses to H1", "2026 Q1", 1, 2026, true},
		{"legacy quarterly Q2 collapses to H1", "2026 Q2", 1, 2026, true},
		{"legacy quarterly Q3 collapses to H2", "2026 Q3", 2, 2026, true},
		{"legacy quarterly Q4 collapses to H2", "2026 Q4", 2, 2026, true},
		{"legacy 1st half maps to H2 of the same year", "2024 - 1st Half", 2, 2024, true},
		{"legacy 2nd half maps to H1 of the following year", "2024 - 2nd Half", 1, 2025, true},
		{"monthly periods are not half-year labels", "2026 Mar", 0, 0, false},
		{"yearly periods are not half-year labels", "2026", 0, 0, false},
		{"unrecognized periods are not half-year labels", "not-a-period", 0, 0, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			half, year, ok := ParsePeriodHalfYear(tc.period)
			if ok != tc.wantOK || (ok && (half != tc.wantHalf || year != tc.wantYear)) {
				t.Errorf("ParsePeriodHalfYear(%q) = (%d, %d, %v), want (%d, %d, %v)",
					tc.period, half, year, ok, tc.wantHalf, tc.wantYear, tc.wantOK)
			}
		})
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

func TestCurrentlyOpenPeriods(t *testing.T) {
	cases := []struct {
		name string
		now  string
		want []string
	}{
		{"during H1, only the current H1 is open", "2026-03-15", []string{"2026 H1"}},
		{"during H2, both the current H1 (catch-up) and H2 are open", "2026-11-20", []string{"2026 H1", "2026 H2"}},
		{"the first day of H1", "2026-01-01", []string{"2026 H1"}},
		{"the first day of H2", "2026-07-01", []string{"2026 H1", "2026 H2"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CurrentlyOpenPeriods(mustParseDate(t, tc.now))
			if len(got) != len(tc.want) {
				t.Fatalf("CurrentlyOpenPeriods(%s) = %v, want %v", tc.now, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("CurrentlyOpenPeriods(%s) = %v, want %v", tc.now, got, tc.want)
				}
			}
		})
	}
}

func TestCheckPeriodEligibility(t *testing.T) {
	cases := []struct {
		name             string
		period           string
		now              string
		submittedPeriods []string
		want             PeriodEligibilityReason
	}{
		{
			name:   "during H1, H1 of the current year is eligible when never submitted",
			period: "2026 H1", now: "2026-03-15", submittedPeriods: nil,
			want: PeriodEligible,
		},
		{
			name:   "H1 duplicate is blocked",
			period: "2026 H1", now: "2026-03-15", submittedPeriods: []string{"2026 H1"},
			want: PeriodReasonDuplicate,
		},
		{
			name:   "H2 duplicate is blocked",
			period: "2026 H2", now: "2026-11-20", submittedPeriods: []string{"2026 H2"},
			want: PeriodReasonDuplicate,
		},
		{
			name:   "H1 submitted, H2 of the same year is then eligible",
			period: "2026 H2", now: "2026-11-20", submittedPeriods: []string{"2026 H1"},
			want: PeriodEligible,
		},
		{
			name:   "during H1, H2 of the same year is a future period and is blocked",
			period: "2026 H2", now: "2026-03-15", submittedPeriods: nil,
			want: PeriodReasonFuture,
		},
		{
			name:   "during H2, H1 of the next year is a future period and is blocked",
			period: "2027 H1", now: "2026-11-20", submittedPeriods: []string{"2026 H2"},
			want: PeriodReasonFuture,
		},
		{
			name:   "during H2, an unsubmitted H1 of the current year is a catch-up and is eligible",
			period: "2026 H1", now: "2026-11-20", submittedPeriods: nil,
			want: PeriodEligible,
		},
		{
			name:   "during H2, an unsubmitted H1 of the current year is eligible even if H2 is already submitted",
			period: "2026 H1", now: "2026-11-20", submittedPeriods: []string{"2026 H2"},
			want: PeriodEligible,
		},
		{
			name:   "a period from a previous year is blocked as past, even if never submitted",
			period: "2025 H2", now: "2026-03-15", submittedPeriods: nil,
			want: PeriodReasonPast,
		},
		{
			name:   "once January begins, H1 of the new year is eligible",
			period: "2027 H1", now: "2027-01-01", submittedPeriods: []string{"2026 H2"},
			want: PeriodEligible,
		},
		{
			name:   "a duplicate takes precedence even when the period is also stale",
			period: "2025 H1", now: "2026-11-20", submittedPeriods: []string{"2025 H1"},
			want: PeriodReasonDuplicate,
		},
		{
			name:   "legacy quarter-labeled submissions are recognized for duplicate detection",
			period: "2026 H1", now: "2026-03-15", submittedPeriods: []string{"2026 Q1"},
			want: PeriodReasonDuplicate,
		},
		{
			name:   "an unrecognized period format is reported distinctly",
			period: "2026 Mar", now: "2026-03-15", submittedPeriods: nil,
			want: PeriodReasonInvalidFormat,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CheckPeriodEligibility(tc.period, mustParseDate(t, tc.now), tc.submittedPeriods)
			if got != tc.want {
				t.Errorf("CheckPeriodEligibility(%q, %s, %v) = %q, want %q",
					tc.period, tc.now, tc.submittedPeriods, got, tc.want)
			}
		})
	}
}

func TestNextEligiblePeriod(t *testing.T) {
	cases := []struct {
		name             string
		submittedPeriods []string
		want             string
	}{
		{"H1 submitted, H2 not submitted -> H2 of the same year", []string{"2026 H1"}, "H2 2026"},
		{"H1 and H2 both submitted -> H1 of the next year", []string{"2026 H1", "2026 H2"}, "H1 2027"},
		{"only H2 submitted -> H1 of the next year", []string{"2026 H2"}, "H1 2027"},
		{"order of the input slice does not matter", []string{"2026 H2", "2026 H1"}, "H1 2027"},
		{"legacy quarter-labeled submissions are normalized first", []string{"2026 Q1"}, "H2 2026"},
		{"no recognizable period returns empty", []string{"2026 Mar"}, ""},
		{"empty input returns empty", nil, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NextEligiblePeriod(tc.submittedPeriods)
			if got != tc.want {
				t.Errorf("NextEligiblePeriod(%v) = %q, want %q", tc.submittedPeriods, got, tc.want)
			}
		})
	}
}
