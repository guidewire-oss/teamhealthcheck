package healthcheck

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

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

// --- Period eligibility ------------------------------------------------------------------
//
// The survey experience allows exactly one submission per (user, survey type, year,
// half-year) for the Individual Survey, and per (team, survey type, year, half-year) for the
// Post-Workshop Survey. Persisted records (the assessment periods already submitted for that
// scope) are the source of truth -- not submission dates or elapsed time.

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
	// PeriodReasonPast means the candidate period is from an earlier year than the current
	// one (a stale period that is no longer open, even if it was never submitted).
	PeriodReasonPast PeriodEligibilityReason = "past_period"
	// PeriodReasonInvalidFormat means the candidate string isn't a recognized half-year
	// period at all.
	PeriodReasonInvalidFormat PeriodEligibilityReason = "invalid_period"
)

// CurrentlyOpenPeriods returns the half-year periods (storage format "YYYY H1"/"YYYY H2") a
// brand-new submission may target as of now, before checking whether they were already
// submitted:
//   - During H1 (January - June), only the current year's H1 is open.
//   - During H2 (July - December), both the current year's H1 (a catch-up submission for a
//     period the caller may have missed) and H2 are open.
//
// No other period -- a previous year, or a half-year further in the future -- is ever open.
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
// A duplicate always takes precedence over a timing rejection: resubmitting a period that
// happens to also be stale is reported as a duplicate, since that is the more specific and
// actionable reason.
func CheckPeriodEligibility(period string, now time.Time, submittedPeriods []string) PeriodEligibilityReason {
	half, year, ok := ParsePeriodHalfYear(period)
	if !ok {
		return PeriodReasonInvalidFormat
	}

	for _, submitted := range submittedPeriods {
		if sHalf, sYear, sOK := ParsePeriodHalfYear(submitted); sOK && sHalf == half && sYear == year {
			return PeriodReasonDuplicate
		}
	}

	for _, open := range CurrentlyOpenPeriods(now) {
		if oHalf, oYear, _ := ParsePeriodHalfYear(open); oHalf == half && oYear == year {
			return PeriodEligible
		}
	}

	curHalf, curYear := HalfYearOf(now)
	if year > curYear || (year == curYear && half > curHalf) {
		return PeriodReasonFuture
	}
	return PeriodReasonPast
}

// NextEligiblePeriod returns the half-year label (display format "H1 <year>" / "H2 <year>")
// immediately following the most recent (highest year, then highest half) period in
// submittedPeriods -- H1 rolls to H2 of the same year; H2 rolls to H1 of the following year.
// Used to tell the caller when they will next be eligible after a duplicate-submission
// rejection. Returns "" if submittedPeriods contains no recognizable half-year period.
func NextEligiblePeriod(submittedPeriods []string) string {
	var maxYear, maxHalf int
	found := false
	for _, submitted := range submittedPeriods {
		if half, year, ok := ParsePeriodHalfYear(submitted); ok {
			if !found || year > maxYear || (year == maxYear && half > maxHalf) {
				maxYear, maxHalf, found = year, half, true
			}
		}
	}
	if !found {
		return ""
	}
	if maxHalf == 1 {
		return fmt.Sprintf("H2 %d", maxYear)
	}
	return fmt.Sprintf("H1 %d", maxYear+1)
}
