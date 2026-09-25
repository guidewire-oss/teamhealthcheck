package healthcheck

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// EligibilityCooldownMonths is the minimum number of calendar months that must elapse
// between two completed submissions of the same survey type -- scoped per user for the
// Individual Survey, per team for the Post-Workshop Survey -- before another submission
// becomes eligible on cooldown grounds alone. This rule is preserved unchanged from its
// original form: a fixed rolling window measured strictly from the actual prior submission
// date, independent of the team's configured cadence.
//
// It is combined with the calendar half-year boundary rule below: a submission is eligible
// as soon as EITHER the cooldown has elapsed OR the calendar has moved into a later
// half-year period than the prior submission's. See NextEligibleDate.
const EligibilityCooldownMonths = 6

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

// NextHalfYearStart returns the start of the half-year immediately following t's half-year:
// July 1 of the same year if t falls in H1 (Jan-Jun), or January 1 of the following year if
// t falls in H2 (Jul-Dec). The returned year is always derived from t.
func NextHalfYearStart(t time.Time) time.Time {
	half, year := HalfYearOf(t)
	if half == 1 {
		return time.Date(year, time.July, 1, 0, 0, 0, 0, t.Location())
	}
	return time.Date(year+1, time.January, 1, 0, 0, 0, 0, t.Location())
}

// NextEligibleDate returns the earliest date another submission is eligible after
// lastSubmissionDate. This is the single centralized rule combining both eligibility paths:
//
//   - The existing six-month cooldown: lastSubmissionDate + EligibilityCooldownMonths.
//   - The new calendar half-year boundary: the start of the half-year following
//     lastSubmissionDate's half-year (see NextHalfYearStart) -- so a new half-year period
//     (e.g. H1 2027 starting January 1, 2027) is never blocked just because six months
//     haven't elapsed since a submission made earlier in the prior half-year (e.g. H2 2026
//     submitted in November 2026).
//
// The earlier of the two dates wins, so eligibility can only ever open up sooner than the
// six-month cooldown alone would allow -- the cooldown's existing blocking behavior for
// submissions within the same half-year period is unchanged, since six calendar months
// never elapse before that period's next boundary is reached.
func NextEligibleDate(lastSubmissionDate time.Time) time.Time {
	cooldownEnd := lastSubmissionDate.AddDate(0, EligibilityCooldownMonths, 0)
	nextPeriodStart := NextHalfYearStart(lastSubmissionDate)
	if nextPeriodStart.Before(cooldownEnd) {
		return nextPeriodStart
	}
	return cooldownEnd
}

// IsWithinCooldown reports whether candidateDate falls before the combined eligibility
// window (six-month cooldown or half-year boundary, whichever comes first) started by
// lastSubmissionDate has elapsed, i.e. whether a submission dated candidateDate must be
// rejected.
func IsWithinCooldown(lastSubmissionDate, candidateDate time.Time) bool {
	return candidateDate.Before(NextEligibleDate(lastSubmissionDate))
}

// --- Display formatting -----------------------------------------------------------------
//
// The patterns below exist only to recognize legacy/quarterly-cadence period strings that
// may already be stored from before half-year-only periods were adopted, so they can still
// be rendered safely. They are not used anywhere in the eligibility calculations above, and
// no new quarter-labeled period is ever produced by this codebase.

var (
	periodPatternQuarterly  = regexp.MustCompile(`^(\d{4}) Q([1-4])$`)
	periodPatternHalfYearly = regexp.MustCompile(`^(\d{4}) H([12])$`)
	periodPatternLegacyHalf = regexp.MustCompile(`^(\d{4}) - (1st|2nd) Half$`)
)

// FormatPeriodForDisplay renders any assessment-period string as a user-facing label in the
// dynamic format "H1 <year>" / "H2 <year>". Quarter-derived periods -- legacy
// quarterly-cadence periods like "2026 Q1", and legacy "YYYY - 1st/2nd Half" periods -- are
// collapsed into the half-year they fall in; a quarter number is never exposed to users.
// Half-yearly periods are re-ordered from "YYYY H1" to "H1 YYYY". Monthly and yearly
// periods, which are not quarter/half labels, are returned unchanged. This is the single
// reusable H1/H2 period formatter for the backend; every user-facing message renders
// periods through this function rather than the raw assessment period string.
func FormatPeriodForDisplay(period string) string {
	if m := periodPatternQuarterly.FindStringSubmatch(period); m != nil {
		year, _ := strconv.Atoi(m[1])
		quarter, _ := strconv.Atoi(m[2])
		if quarter <= 2 {
			return fmt.Sprintf("H1 %d", year)
		}
		return fmt.Sprintf("H2 %d", year)
	}
	if m := periodPatternHalfYearly.FindStringSubmatch(period); m != nil {
		year, _ := strconv.Atoi(m[1])
		return fmt.Sprintf("H%s %d", m[2], year)
	}
	if m := periodPatternLegacyHalf.FindStringSubmatch(period); m != nil {
		year, _ := strconv.Atoi(m[1])
		if m[2] == "1st" {
			return fmt.Sprintf("H2 %d", year)
		}
		return fmt.Sprintf("H1 %d", year+1)
	}
	return period
}
