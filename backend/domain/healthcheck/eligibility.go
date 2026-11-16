package healthcheck

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// monthAbbreviations are the month names used in monthly period labels ("2026 Mar").
var monthAbbreviations = [...]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

// HalfYearOf returns the half-year (1 or 2) and calendar year that t falls into.
// H1 runs January 1 - June 30; H2 runs July 1 - December 31. The year is always derived
// from t -- never hardcoded -- so this works correctly for any year.
func HalfYearOf(t time.Time) (half int, year int) {
	year = t.Year()
	if t.Month() <= time.June {
		return 1, year
	}
	return 2, year
}

// ActiveHalfYearPeriod returns the half-year assessment-period label ("H1 <year>" /
// "H2 <year>") that t falls into, deriving the year dynamically from t.
func ActiveHalfYearPeriod(t time.Time) string {
	half, year := HalfYearOf(t)
	return fmt.Sprintf("H%d %d", half, year)
}

// --- Shared period parsing ---------------------------------------------------------------
//
// ParsePeriodHalfYear is the single place that maps any recognized raw assessment-period
// string -- current half-yearly storage format ("YYYY H1"/"YYYY H2"), or legacy
// quarterly/"1st-2nd Half" formats from data predating this model -- to the (half, year) it
// represents. FormatPeriodForDisplay and every eligibility function below build on this one
// parser rather than re-implementing quarter/legacy-format recognition.

var (
	periodPatternQuarterly  = regexp.MustCompile(`^(\d{4}) Q([1-4])$`)
	periodPatternHalfYearly = regexp.MustCompile(`^(\d{4}) H([12])$`)
	periodPatternLegacyHalf = regexp.MustCompile(`^(\d{4}) - (1st|2nd) Half$`)
	periodPatternMonthly    = regexp.MustCompile(`^(\d{4}) (Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)$`)
	periodPatternYearly     = regexp.MustCompile(`^(\d{4})$`)
)

// ParsePeriodHalfYear extracts the half (1 or 2) and year a raw assessment-period string
// represents. Returns ok=false for monthly/yearly periods or anything unrecognized -- those
// are not half-year labels and never participate in half-year eligibility.
func ParsePeriodHalfYear(period string) (half int, year int, ok bool) {
	if m := periodPatternHalfYearly.FindStringSubmatch(period); m != nil {
		year, _ = strconv.Atoi(m[1])
		half, _ = strconv.Atoi(m[2])
		return half, year, true
	}
	if m := periodPatternQuarterly.FindStringSubmatch(period); m != nil {
		year, _ = strconv.Atoi(m[1])
		quarter, _ := strconv.Atoi(m[2])
		if quarter <= 2 {
			return 1, year, true
		}
		return 2, year, true
	}
	if m := periodPatternLegacyHalf.FindStringSubmatch(period); m != nil {
		year, _ = strconv.Atoi(m[1])
		if m[2] == "1st" {
			// "YYYY - 1st Half" covers Jul-Dec of YYYY (= YYYY H2)
			return 2, year, true
		}
		// "YYYY - 2nd Half" covers Jan-Jun of YYYY+1 (= (YYYY+1) H1)
		return 1, year + 1, true
	}
	return 0, 0, false
}

// FormatPeriodForDisplay renders any assessment-period string as a user-facing label in the
// dynamic format "H1 <year>" / "H2 <year>". Quarter-derived periods -- legacy
// quarterly-cadence periods like "2026 Q1", and legacy "YYYY - 1st/2nd Half" periods -- are
// collapsed into the half-year they fall in; a quarter number is never exposed to users.
// Monthly and yearly periods, which are not quarter/half labels, are returned unchanged. This
// is the single reusable H1/H2 period formatter for the backend; every user-facing message
// renders periods through this function rather than the raw assessment period string.
func FormatPeriodForDisplay(period string) string {
	if half, year, ok := ParsePeriodHalfYear(period); ok {
		return fmt.Sprintf("H%d %d", half, year)
	}
	return period
}

// --- Supported period formats ----------------------------------------------------------------
//
// A team's cadence decides which kind of period it surveys for. Three kinds are supported:
//   - half-yearly: "YYYY H1" / "YYYY H2" (and the legacy quarterly and "1st/2nd Half" labels
//     that older data used, which all collapse into a half-year);
//   - monthly:     "YYYY Mon", e.g. "2026 Mar";
//   - yearly:      "YYYY".
// Anything else, including an empty string, is not a period at all.

type periodKind int

const (
	periodKindHalfYear periodKind = iota + 1
	periodKindMonthly
	periodKindYearly
)

// classifyPeriod returns the kind of a period and its position: year, and within the year the
// half (1-2) or month (1-12); unit is 0 for yearly periods. ok is false for anything that is
// not a supported period.
func classifyPeriod(period string) (kind periodKind, year, unit int, ok bool) {
	if half, y, isHalf := ParsePeriodHalfYear(period); isHalf {
		return periodKindHalfYear, y, half, true
	}
	if m := periodPatternMonthly.FindStringSubmatch(period); m != nil {
		y, _ := strconv.Atoi(m[1])
		for i, name := range monthAbbreviations {
			if name == m[2] {
				return periodKindMonthly, y, i + 1, true
			}
		}
	}
	if m := periodPatternYearly.FindStringSubmatch(period); m != nil {
		y, _ := strconv.Atoi(m[1])
		return periodKindYearly, y, 0, true
	}
	return 0, 0, 0, false
}

// CanonicalPeriod returns the single stored form of a supported assessment period, so that
// two labels for the same period are always equal strings. Half-year periods become
// "YYYY H1" / "YYYY H2" -- the legacy quarterly ("2026 Q1") and "YYYY - 1st Half" labels are
// folded into the half-year they fall in -- while monthly ("2026 Mar") and yearly ("2026")
// periods are already canonical. ok is false when period is empty or not a supported period.
//
// Storing the canonical form is what lets the database's unique index catch two concurrent
// submissions for the same period even when they arrived under different labels.
func CanonicalPeriod(period string) (canonical string, ok bool) {
	kind, year, unit, ok := classifyPeriod(period)
	if !ok {
		return "", false
	}
	switch kind {
	case periodKindHalfYear:
		return fmt.Sprintf("%d H%d", year, unit), true
	case periodKindMonthly:
		return fmt.Sprintf("%d %s", year, monthAbbreviations[unit-1]), true
	default:
		return strconv.Itoa(year), true
	}
}

// --- Period eligibility ------------------------------------------------------------------
//
// The survey experience allows exactly one submission per (user, survey type, period) for the
// Individual Survey, and per (team, survey type, period) for the Post-Workshop Survey, where
// the period is a half-year, a month or a year depending on the team's cadence. Persisted
// records (the assessment periods already submitted for that scope) are the source of truth
// -- not submission dates or elapsed time.

// PeriodEligibilityReason classifies why a candidate assessment period is, or is not, a valid
// target for a brand-new submission right now.
type PeriodEligibilityReason string

const (
	// PeriodEligible means the candidate period is currently open and not yet submitted.
	PeriodEligible PeriodEligibilityReason = "eligible"
	// PeriodReasonDuplicate means the caller's scope already has a completed submission for
	// this exact survey type and period.
	PeriodReasonDuplicate PeriodEligibilityReason = "duplicate"
	// PeriodReasonFuture means the candidate period has not started yet.
	PeriodReasonFuture PeriodEligibilityReason = "future_period"
	// PeriodReasonPast means the candidate period has already ended and is no longer open for
	// a new submission, even if it was never submitted.
	PeriodReasonPast PeriodEligibilityReason = "past_period"
	// PeriodReasonInvalidFormat means the candidate string is empty or isn't a supported
	// period (half-yearly, monthly or yearly) at all.
	PeriodReasonInvalidFormat PeriodEligibilityReason = "invalid_period"
)

// CurrentlyOpenPeriods returns the half-year periods (storage format "YYYY H1"/"YYYY H2") a
// brand-new submission may target as of now, before checking whether they were already
// submitted:
//   - During H1 (January - June), only the current year's H1 is open.
//   - During H2 (July - December), both the current year's H1 (a catch-up submission for a
//     period the caller may have missed) and H2 are open.
//
// No other half-year -- a previous year, or one further in the future -- is ever open.
// Monthly and yearly teams have exactly one open period each: the current month, or the
// current year.
func CurrentlyOpenPeriods(now time.Time) []string {
	half, year := HalfYearOf(now)
	h1 := fmt.Sprintf("%d H1", year)
	if half == 1 {
		return []string{h1}
	}
	return []string{h1, fmt.Sprintf("%d H2", year)}
}

// CheckPeriodEligibility decides whether period is a valid target for a brand-new submission
// as of now, given submittedPeriods -- every period already on record for the caller's scope
// (same survey type, same user for Individual Survey / same team for Post-Workshop Survey).
//
// Periods are compared in canonical form (see CanonicalPeriod), so "2026 Q1" and "2026 H1"
// count as the same period. A duplicate always takes precedence over a timing rejection:
// resubmitting a period that happens to also be stale is reported as a duplicate, since that
// is the more specific and actionable reason.
func CheckPeriodEligibility(period string, now time.Time, submittedPeriods []string) PeriodEligibilityReason {
	canonical, ok := CanonicalPeriod(period)
	if !ok {
		return PeriodReasonInvalidFormat
	}

	for _, submitted := range submittedPeriods {
		if sCanonical, sOK := CanonicalPeriod(submitted); sOK && sCanonical == canonical {
			return PeriodReasonDuplicate
		}
	}

	kind, year, unit, _ := classifyPeriod(canonical)
	curHalf, curYear := HalfYearOf(now)
	curMonth := int(now.Month())

	switch kind {
	case periodKindMonthly:
		switch {
		case year == curYear && unit == curMonth:
			return PeriodEligible
		case year > curYear || (year == curYear && unit > curMonth):
			return PeriodReasonFuture
		}
		return PeriodReasonPast
	case periodKindYearly:
		switch {
		case year == curYear:
			return PeriodEligible
		case year > curYear:
			return PeriodReasonFuture
		}
		return PeriodReasonPast
	}

	// Half-year.
	for _, open := range CurrentlyOpenPeriods(now) {
		if open == canonical {
			return PeriodEligible
		}
	}
	if year > curYear || (year == curYear && unit > curHalf) {
		return PeriodReasonFuture
	}
	return PeriodReasonPast
}

// NextEligiblePeriod returns the half-year label (display format "H1 <year>" / "H2 <year>")
// immediately following the most recent (highest year, then highest half) period in
// submittedPeriods -- H1 rolls to H2 of the same year; H2 rolls to H1 of the following year.
// Returns "" if submittedPeriods contains no recognizable half-year period. For a scope that
// surveys monthly or yearly, use NextEligiblePeriodAfter.
func NextEligiblePeriod(submittedPeriods []string) string {
	return latestNext(periodKindHalfYear, submittedPeriods)
}

// NextEligiblePeriodAfter returns the label of the period a caller becomes eligible for next
// after a duplicate of duplicatedPeriod, following the most recent submitted period of the
// same kind: the next half-year, the next month, or the next year. Returns "" when
// nothing of its kind has been submitted. A duplicatedPeriod that is not a supported period
// falls back to the half-year sequence.
func NextEligiblePeriodAfter(duplicatedPeriod string, submittedPeriods []string) string {
	kind, _, _, ok := classifyPeriod(duplicatedPeriod)
	if !ok {
		// Not a recognizable period (for example an already-formatted display label): fall
		// back to the half-year sequence, which is the model's default.
		return NextEligiblePeriod(submittedPeriods)
	}
	return latestNext(kind, submittedPeriods)
}

// latestNext finds the most recent submitted period of the given kind and returns the label of
// the period that follows it.
func latestNext(kind periodKind, submittedPeriods []string) string {
	var maxYear, maxUnit int
	found := false
	for _, submitted := range submittedPeriods {
		k, year, unit, ok := classifyPeriod(submitted)
		if !ok || k != kind {
			continue
		}
		if !found || year > maxYear || (year == maxYear && unit > maxUnit) {
			maxYear, maxUnit, found = year, unit, true
		}
	}
	if !found {
		return ""
	}

	switch kind {
	case periodKindMonthly:
		if maxUnit == 12 {
			return fmt.Sprintf("%d %s", maxYear+1, monthAbbreviations[0])
		}
		return fmt.Sprintf("%d %s", maxYear, monthAbbreviations[maxUnit])
	case periodKindYearly:
		return strconv.Itoa(maxYear + 1)
	default:
		if maxUnit == 1 {
			return fmt.Sprintf("H2 %d", maxYear)
		}
		return fmt.Sprintf("H1 %d", maxYear+1)
	}
}
