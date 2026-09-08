package healthcheck

import (
	"regexp"
	"sort"
	"strconv"
)

var (
	monthlyPeriodPattern    = regexp.MustCompile(`^(\d{4}) (Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)$`)
	quarterlyPeriodPattern  = regexp.MustCompile(`^(\d{4}) Q([1-4])$`)
	halfYearlyPeriodPattern = regexp.MustCompile(`^(\d{4}) H([12])$`)
	yearlyPeriodPattern     = regexp.MustCompile(`^(\d{4})$`)
	legacyPeriodPattern     = regexp.MustCompile(`^(\d{4}) - (1st|2nd) Half$`)
)

var monthIndexByName = map[string]int{
	"Jan": 0, "Feb": 1, "Mar": 2, "Apr": 3, "May": 4, "Jun": 5,
	"Jul": 6, "Aug": 7, "Sep": 8, "Oct": 9, "Nov": 10, "Dec": 11,
}

// AssessmentPeriodSortKey returns a numeric key that sorts assessment
// period strings chronologically ascending, mirroring
// frontend/lib/assessment-period.ts's parseAssessmentPeriod/periodSortKey
// exactly (same five formats, same legacy-period year-boundary mapping),
// so the two implementations can never disagree about ordering.
//
// ok is false when period matches none of the known formats — the caller
// decides how to handle that; SortAssessmentPeriodsDescending sorts such
// entries after every parseable one.
func AssessmentPeriodSortKey(period string) (key int, ok bool) {
	if m := monthlyPeriodPattern.FindStringSubmatch(period); m != nil {
		year, _ := strconv.Atoi(m[1])
		return year*100 + monthIndexByName[m[2]], true
	}
	if m := quarterlyPeriodPattern.FindStringSubmatch(period); m != nil {
		year, _ := strconv.Atoi(m[1])
		quarter, _ := strconv.Atoi(m[2])
		return year*100 + (quarter-1)*3, true
	}
	if m := halfYearlyPeriodPattern.FindStringSubmatch(period); m != nil {
		year, _ := strconv.Atoi(m[1])
		half, _ := strconv.Atoi(m[2])
		return year*100 + (half-1)*6, true
	}
	if m := yearlyPeriodPattern.FindStringSubmatch(period); m != nil {
		year, _ := strconv.Atoi(m[1])
		return year * 100, true
	}
	if m := legacyPeriodPattern.FindStringSubmatch(period); m != nil {
		// "YYYY - 1st Half" covers Jul-Dec of YYYY (= YYYY H2); "YYYY - 2nd
		// Half" covers Jan-Jun of YYYY+1 (= (YYYY+1) H1) — matches this
		// app's own getAssessmentPeriod() half-year boundary exactly.
		year, _ := strconv.Atoi(m[1])
		if m[2] == "1st" {
			return year*100 + 6, true
		}
		return (year + 1) * 100, true
	}
	return 0, false
}

// SortAssessmentPeriodsDescending sorts period strings most-recent-first
// using AssessmentPeriodSortKey, so a caller picking periods[0] as "the
// current/most recent period" gets a genuinely chronological answer
// instead of relying on an alphabetical string sort — which silently
// breaks across this app's five different period formats (e.g. "H1" <
// "Mar" < "Q1" alphabetically, unrelated to actual month/quarter order).
// A period that doesn't match any known format sorts after every
// parseable one and keeps its relative order among other unparseable
// entries (stable sort), rather than being interleaved arbitrarily.
func SortAssessmentPeriodsDescending(periods []string) {
	sort.SliceStable(periods, func(i, j int) bool {
		keyI, okI := AssessmentPeriodSortKey(periods[i])
		keyJ, okJ := AssessmentPeriodSortKey(periods[j])
		if okI != okJ {
			return okI
		}
		if !okI {
			return false
		}
		return keyI > keyJ
	})
}
