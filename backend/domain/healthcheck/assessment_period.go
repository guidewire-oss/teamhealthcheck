package healthcheck

import (
	"regexp"
	"sort"
	"strconv"
)

// periodFormatRank orders the known period formats so that, when two
// different formats resolve to the same AssessmentPeriodSortKey (e.g.
// "2026", "2026 Jan", and "2026 Q1" all start at month 0 of 2026), sorting
// is still a deterministic total order instead of depending on whatever
// order the caller (e.g. an unordered SQL DISTINCT query) happened to
// supply. Finer-grained formats rank before coarser ones so "2026 Jan"
// (the most specific statement of "start of 2026") is treated as the most
// recent among ties.
const (
	rankMonthly = iota
	rankQuarterly
	rankHalfYearly
	rankLegacyHalfYearly
	rankYearly
	rankUnknown
)

func periodFormatRank(period string) int {
	switch {
	case monthlyPeriodPattern.MatchString(period):
		return rankMonthly
	case quarterlyPeriodPattern.MatchString(period):
		return rankQuarterly
	case halfYearlyPeriodPattern.MatchString(period):
		return rankHalfYearly
	case legacyPeriodPattern.MatchString(period):
		return rankLegacyHalfYearly
	case yearlyPeriodPattern.MatchString(period):
		return rankYearly
	default:
		return rankUnknown
	}
}

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
//
// When two parseable periods share the same chronological key (different
// formats describing the same start date), order is broken deterministically
// by periodFormatRank and then lexically, so repeated calls on the same
// input — regardless of the input's original order — always produce the
// same result. This matters because FindDistinctAssessmentPeriods feeds
// this function rows from an unordered SQL DISTINCT query.
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
		if keyI != keyJ {
			return keyI > keyJ
		}
		rankI, rankJ := periodFormatRank(periods[i]), periodFormatRank(periods[j])
		if rankI != rankJ {
			return rankI < rankJ
		}
		return periods[i] < periods[j]
	})
}

// IsSelectableAssessmentPeriod reports whether period may be offered in a
// dashboard period selector: exactly the canonical half-year form "YYYY H1" or
// "YYYY H2" (see halfYearlyPeriodPattern). It is a format rule, not a list of
// years, so a future half-year (e.g. "2027 H1") becomes selectable as soon as
// completed sessions exist for it, with no code change.
//
// Every other string that may exist in health_check_sessions.assessment_period
// -- quarterly ("2026 Q3"), monthly ("2026 Mar"), yearly, "YYYY - 1st Half" or
// free text -- is deliberately NOT selectable. Those rows are left untouched
// in the database; they are simply never offered as a filter choice, so no
// period-filtered view can match them (the filter compares the stored string
// exactly).
func IsSelectableAssessmentPeriod(period string) bool {
	return halfYearlyPeriodPattern.MatchString(period)
}

// SelectableAssessmentPeriods returns the distinct selectable periods from
// raw, most recent first. raw is not modified.
func SelectableAssessmentPeriods(raw []string) []string {
	seen := make(map[string]bool, len(raw))
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		if IsSelectableAssessmentPeriod(p) && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	SortAssessmentPeriodsDescending(out)
	return out
}
