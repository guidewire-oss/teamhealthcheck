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
