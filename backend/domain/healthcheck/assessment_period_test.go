package healthcheck

import "testing"

func TestAssessmentPeriodSortKey(t *testing.T) {
	cases := []struct {
		name   string
		period string
		key    int
		ok     bool
	}{
		{"monthly Jan", "2026 Jan", 202600, true},
		{"monthly Dec", "2026 Dec", 202611, true},
		{"quarterly Q1", "2026 Q1", 202600, true},
		{"quarterly Q4", "2026 Q4", 202609, true},
		{"half-yearly H1", "2026 H1", 202600, true},
		{"half-yearly H2", "2026 H2", 202606, true},
		{"yearly", "2026", 202600, true},
		{"legacy 1st half maps to same-year H2", "2025 - 1st Half", 202506, true},
		{"legacy 2nd half maps to next-year H1", "2025 - 2nd Half", 202600, true},
		{"unparseable", "not a period", 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			key, ok := AssessmentPeriodSortKey(c.period)
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v", ok, c.ok)
			}
			if ok && key != c.key {
				t.Fatalf("key = %d, want %d", key, c.key)
			}
		})
	}
}

// TestSortAssessmentPeriodsDescending is the regression test for the bug a
// plain `ORDER BY assessment_period DESC` SQL sort has: it's a lexicographic
// STRING sort, which has no concept of chronological order once the app's
// five different period formats are mixed together (e.g. "H1" < "Mar" <
// "Q1" alphabetically, unrelated to actual month/quarter order).
func TestSortAssessmentPeriodsDescending(t *testing.T) {
	// Deliberately mixes all five formats with keys chosen to be pairwise
	// distinct (202606 > 202600 > 202506 > 202406), so the expected order
	// tests real chronological sorting rather than a same-key tie-break. A
	// plain alphabetical string sort would get this badly wrong: e.g.
	// "2024 - 1st Half" > "2025 H2" > "2026 Jan" > "2026 Q3" lexicographically
	// ('-' and '2' < 'H' < 'J' < 'Q' as leading bytes after the year),
	// nothing like the actual chronological order below.
	periods := []string{
		"2025 H2",         // 202506
		"2026 Jan",        // 202600
		"2024 - 1st Half", // 202406 (= 2024 H2)
		"2026 Q3",         // 202606
	}
	SortAssessmentPeriodsDescending(periods)

	want := []string{
		"2026 Q3",
		"2026 Jan",
		"2025 H2",
		"2024 - 1st Half",
	}
	if len(periods) != len(want) {
		t.Fatalf("got %d periods, want %d", len(periods), len(want))
	}
	for i := range want {
		if periods[i] != want[i] {
			t.Fatalf("index %d: got %q, want %q (full result: %v)", i, periods[i], want[i], periods)
		}
	}
}

func TestSortAssessmentPeriodsDescending_UnparseableSortsLast(t *testing.T) {
	periods := []string{"garbage", "2026 H1", "2025 H2"}
	SortAssessmentPeriodsDescending(periods)

	want := []string{"2026 H1", "2025 H2", "garbage"}
	for i := range want {
		if periods[i] != want[i] {
			t.Fatalf("index %d: got %q, want %q (full result: %v)", i, periods[i], want[i], periods)
		}
	}
}

func TestSortAssessmentPeriodsDescending_TieBreaksDeterministicallyAcrossInputOrders(t *testing.T) {
	// "2026", "2026 Jan", and "2026 Q1" all resolve to the same chronological
	// key (start of 2026), since FindDistinctAssessmentPeriods feeds this
	// function rows from an unordered SQL DISTINCT query, the result must not
	// depend on the slice's initial order.
	tied := []string{"2026", "2026 Jan", "2026 Q1"}
	want := []string{"2026 Jan", "2026 Q1", "2026"}

	orderings := [][]string{
		{"2026", "2026 Jan", "2026 Q1"},
		{"2026 Q1", "2026", "2026 Jan"},
		{"2026 Jan", "2026 Q1", "2026"},
	}
	for _, input := range orderings {
		periods := append([]string(nil), input...)
		SortAssessmentPeriodsDescending(periods)
		for i := range want {
			if periods[i] != want[i] {
				t.Fatalf("input order %v: index %d: got %q, want %q (full result: %v)", input, i, periods[i], want[i], periods)
			}
		}
	}

	// Sanity: all three really do share one key.
	for _, p := range tied {
		if _, ok := AssessmentPeriodSortKey(p); !ok {
			t.Fatalf("expected %q to be parseable", p)
		}
	}
}

func TestSortAssessmentPeriodsDescending_RepeatedCallsAreStable(t *testing.T) {
	base := []string{"2026 Q1", "2026", "2026 Jan", "2025 Q4", "2025"}
	first := append([]string(nil), base...)
	SortAssessmentPeriodsDescending(first)

	for i := 0; i < 20; i++ {
		got := append([]string(nil), base...)
		SortAssessmentPeriodsDescending(got)
		for j := range first {
			if got[j] != first[j] {
				t.Fatalf("iteration %d: non-deterministic result: got %v, want %v", i, got, first)
			}
		}
	}
}

// TestSortAssessmentPeriodsDescending_SameYearChronologicalDiffersFromLexical
// pins the claimed regression: within ONE year, string order and
// chronological order disagree. Lexically (descending) these sort
// Sep, Q4, Mar, H2, Dec; chronologically they are Dec (Dec = month 11), Q4
// (starts Oct), Sep, H2 (starts Jul), Mar. A string sort, or a sort that only
// compared the year, fails this.
func TestSortAssessmentPeriodsDescending_SameYearChronologicalDiffersFromLexical(t *testing.T) {
	periods := []string{"2026 Mar", "2026 Q4", "2026 Dec", "2026 H2", "2026 Sep"}
	SortAssessmentPeriodsDescending(periods)

	want := []string{"2026 Dec", "2026 Q4", "2026 Sep", "2026 H2", "2026 Mar"}
	for i := range want {
		if periods[i] != want[i] {
			t.Fatalf("index %d: got %q, want %q (full result: %v)", i, periods[i], want[i], periods)
		}
	}
}

func TestSortAssessmentPeriodsDescending_ChronologicalAcrossYearBoundary(t *testing.T) {
	// "2025 Dec" is later than "2025 Q1" but earlier than "2026 Jan".
	periods := []string{"2025 Q1", "2026 Jan", "2025 Dec"}
	SortAssessmentPeriodsDescending(periods)

	want := []string{"2026 Jan", "2025 Dec", "2025 Q1"}
	for i := range want {
		if periods[i] != want[i] {
			t.Fatalf("index %d: got %q, want %q (full result: %v)", i, periods[i], want[i], periods)
		}
	}
}

func TestAssessmentPeriodSortKey_RejectsMalformedValues(t *testing.T) {
	for _, bad := range []string{"", "2026 Q5", "2026 Q0", "2026 H3", "2026 Foo", "26 Jan", "2026-Jan", "Jan 2026"} {
		if key, ok := AssessmentPeriodSortKey(bad); ok {
			t.Errorf("AssessmentPeriodSortKey(%q) = %d, true; want not ok", bad, key)
		}
	}
}

// Only half-years are selectable; every quarterly, monthly or yearly value
// seen in the DEV data (2026 Q1, 2026 Q3, 2026 Mar) must be rejected.
func TestIsSelectableAssessmentPeriod(t *testing.T) {
	cases := []struct {
		period string
		want   bool
	}{
		{"2026 H1", true},
		{"2026 H2", true},
		{"2027 H1", true}, // future half-years need no code change
		{"2026 Q1", false},
		{"2026 Q3", false},
		{"2026 Mar", false},
		{"2026", false},
		{"2024 - 1st Half", true}, // older half-year label, still a real half-year
		{"2023 - 2nd Half", true},
		{"2024 - 3rd Half", false},
		{"2026 H3", false},
		{"2026 h1", false},
		{" 2026 H1", false},
		{"", false},
		{"free text", false},
	}
	for _, c := range cases {
		if got := IsSelectableAssessmentPeriod(c.period); got != c.want {
			t.Errorf("IsSelectableAssessmentPeriod(%q) = %v, want %v", c.period, got, c.want)
		}
	}
}

func TestSelectableAssessmentPeriods_ExposesOnlyHalfYearsAndLeavesInputAlone(t *testing.T) {
	raw := []string{"2026 Q3", "2026 H1", "2026 Mar", "2026 H2", "2026 Q1", "2026 H1"}
	original := append([]string(nil), raw...)

	got := SelectableAssessmentPeriods(raw)

	want := []string{"2026 H2", "2026 H1"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v, want %v (deduplicated, most recent first, no Q1/Q3/Mar)", got, want)
	}
	for i := range raw {
		if raw[i] != original[i] {
			t.Fatalf("input was modified: %v, want %v", raw, original)
		}
	}
	if empty := SelectableAssessmentPeriods(nil); empty == nil || len(empty) != 0 {
		t.Fatalf("got %v for no input, want an empty non-nil slice (the API must return [] not null)", empty)
	}
}
