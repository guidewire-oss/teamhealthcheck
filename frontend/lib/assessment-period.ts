/**
 * Utility functions for cadence-driven assessment period detection.
 *
 * Assessment period format depends on team cadence:
 *   - Monthly:     "YYYY Mon"  (e.g., "2026 Mar")
 *   - Half-yearly: "YYYY H1"   (e.g., "2026 H1")
 *   - Yearly:      "YYYY"      (e.g., "2026")
 *
 * Quarter-based periods ("YYYY Q1"-"YYYY Q4") and legacy "YYYY - 1st/2nd Half" periods are
 * no longer produced by this codebase -- quarterly cadence has been removed -- but are still
 * *parsed* so any pre-existing historical data can still be rendered correctly (always
 * collapsed to a safe H1/H2 label; see `formatPeriodLabel`, and the eligibility model no
 * longer depends on quarter buckets at all).
 */

export type Cadence = 'monthly' | 'half-yearly' | 'yearly';

const VALID_CADENCES: ReadonlySet<string> = new Set(['monthly', 'half-yearly', 'yearly']);

/**
 * Validate and narrow a string to a Cadence type.
 * Returns the cadence if valid, or 'half-yearly' as a safe default.
 */
export function toCadence(value: string | undefined | null): Cadence {
  if (value && VALID_CADENCES.has(value)) return value as Cadence;
  return 'half-yearly';
}

const MONTH_NAMES = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

const MONTH_NAME_TO_INDEX: Record<string, number> = {
  Jan: 0, Feb: 1, Mar: 2, Apr: 3, May: 4, Jun: 5,
  Jul: 6, Aug: 7, Sep: 8, Oct: 9, Nov: 10, Dec: 11,
};

/**
 * Get the assessment period for a given date and cadence.
 * @param date - Date object or ISO string (defaults to current date)
 * @param cadence - Team cadence (defaults to 'half-yearly' for backward compat)
 */
export function getAssessmentPeriod(date?: Date | string, cadence?: Cadence): string {
  const d = date ? (typeof date === 'string' ? new Date(date) : date) : new Date();
  const year = d.getFullYear();
  const month = d.getMonth(); // 0-indexed

  switch (cadence) {
    case 'monthly':
      return `${year} ${MONTH_NAMES[month]}`;
    case 'yearly':
      return `${year}`;
    case 'half-yearly':
    default:
      return `${year} H${month < 6 ? 1 : 2}`;
  }
}

/**
 * Get the current assessment period (convenience function).
 */
export function getCurrentAssessmentPeriod(cadence?: Cadence): string {
  return getAssessmentPeriod(new Date(), cadence);
}

/**
 * Parsed assessment period — discriminated union by type.
 *
 * 'quarterly' and 'legacy' are parsed only for backward compatibility with periods stored
 * before quarterly cadence was removed; no new period of either type is ever produced.
 */
export type ParsedPeriod =
  | { type: 'monthly'; year: number; month: number }       // month: 0-indexed
  | { type: 'quarterly'; year: number; quarter: number }    // quarter: 1-4 (legacy only)
  | { type: 'half-yearly'; year: number; half: number }     // half: 1-2
  | { type: 'yearly'; year: number }
  | { type: 'legacy'; year: number; half: '1st' | '2nd' };

/**
 * Parse an assessment period string into its components.
 * @returns ParsedPeriod or null if format is invalid
 */
export function parseAssessmentPeriod(period: string): ParsedPeriod | null {
  // Monthly: "2026 Mar"
  const monthlyMatch = period.match(/^(\d{4}) (Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)$/);
  if (monthlyMatch) {
    return { type: 'monthly', year: parseInt(monthlyMatch[1], 10), month: MONTH_NAME_TO_INDEX[monthlyMatch[2]] };
  }

  // Quarterly: "2026 Q1"
  const quarterlyMatch = period.match(/^(\d{4}) Q([1-4])$/);
  if (quarterlyMatch) {
    return { type: 'quarterly', year: parseInt(quarterlyMatch[1], 10), quarter: parseInt(quarterlyMatch[2], 10) };
  }

  // Half-yearly: "2026 H1"
  const halfYearlyMatch = period.match(/^(\d{4}) H([12])$/);
  if (halfYearlyMatch) {
    return { type: 'half-yearly', year: parseInt(halfYearlyMatch[1], 10), half: parseInt(halfYearlyMatch[2], 10) };
  }

  // Yearly: "2026"
  const yearlyMatch = period.match(/^(\d{4})$/);
  if (yearlyMatch) {
    return { type: 'yearly', year: parseInt(yearlyMatch[1], 10) };
  }

  // Legacy: "2024 - 1st Half"
  const legacyMatch = period.match(/^(\d{4}) - (1st|2nd) Half$/);
  if (legacyMatch) {
    return { type: 'legacy', year: parseInt(legacyMatch[1], 10), half: legacyMatch[2] as '1st' | '2nd' };
  }

  return null;
}

/**
 * Build a list of selectable periods for manual override, most recent first.
 * Always includes the current auto-detected period, followed by `count - 1`
 * preceding periods for the given cadence (e.g., previous half-years).
 *
 * @param cadence - Team cadence
 * @param count - Number of periods to return (default 6)
 * @param referenceDate - Date to anchor "current" (defaults to now)
 */
export function getSelectablePeriods(cadence: Cadence, count = 6, referenceDate?: Date): string[] {
  const base = referenceDate ?? new Date();
  let year = base.getFullYear();
  let month = base.getMonth(); // 0-indexed

  const stepSize = cadence === 'monthly' ? 1 : cadence === 'yearly' ? 12 : 6;

  const periods: string[] = [];
  for (let i = 0; i < count; i++) {
    periods.push(getAssessmentPeriod(new Date(year, month, 1), cadence));
    month -= stepSize;
    while (month < 0) {
      month += 12;
      year -= 1;
    }
  }

  return periods;
}

/**
 * Render an assessment period as a user-facing label, in the dynamic format
 * `H1 <year>` / `H2 <year>`.
 *
 * Quarter-derived periods (quarterly-cadence periods like "2026 Q1", and legacy
 * "YYYY - 1st/2nd Half" periods) are never shown to users as quarter numbers —
 * they are always collapsed into the half-year they fall in (Q1/Q2 -> H1,
 * Q3/Q4 -> H2). Half-yearly periods are simply re-ordered from "YYYY H1" to
 * "H1 YYYY". Monthly and yearly periods are not quarter/half labels and are
 * returned unchanged.
 *
 * This is the single place quarter-to-half-year conversion happens for display;
 * every UI surface (dropdowns, tables, charts, tooltips, exports, summaries,
 * empty states) must render periods through this helper rather than the raw
 * assessment period string.
 */
export function formatPeriodLabel(period: string): string {
  const parsed = parseAssessmentPeriod(period);
  if (!parsed) return period;

  switch (parsed.type) {
    case 'half-yearly':
      return `H${parsed.half} ${parsed.year}`;
    case 'quarterly':
      return parsed.quarter <= 2 ? `H1 ${parsed.year}` : `H2 ${parsed.year}`;
    case 'legacy':
      // "YYYY - 1st Half" covers Jul-Dec of YYYY (= YYYY H2),
      // "YYYY - 2nd Half" covers Jan-Jun of YYYY+1 (= (YYYY+1) H1)
      return parsed.half === '1st' ? `H2 ${parsed.year}` : `H1 ${parsed.year + 1}`;
    case 'monthly':
    case 'yearly':
      return period;
  }
}

/**
 * Render a date as a user-facing month + year value, e.g. "March 2026". The survey
 * experience never displays day-, hour-, minute-, second-, or timezone-level precision --
 * eligibility, duplicate checks, and history/last-updated displays are all based on the
 * calendar month and year alone. Returns the input unchanged if it isn't a valid date.
 */
export function formatMonthYear(date: Date | string): string {
  const d = typeof date === 'string' ? new Date(date) : date;
  if (Number.isNaN(d.getTime())) return typeof date === 'string' ? date : '';
  return d.toLocaleDateString(undefined, { month: 'long', year: 'numeric', timeZone: 'UTC' });
}

/**
 * Convert a parsed period to a numeric sort key for chronological ordering.
 */
function periodSortKey(parsed: ParsedPeriod): number {
  switch (parsed.type) {
    case 'monthly':
      return parsed.year * 100 + parsed.month;
    case 'quarterly':
      return parsed.year * 100 + (parsed.quarter - 1) * 3;
    case 'half-yearly':
      return parsed.year * 100 + (parsed.half - 1) * 6;
    case 'yearly':
      return parsed.year * 100;
    case 'legacy':
      // Legacy mapping: "YYYY - 1st Half" covers Jul-Dec of YYYY (= YYYY H2),
      // "YYYY - 2nd Half" covers Jan-Jun of YYYY+1 (= (YYYY+1) H1)
      return parsed.half === '1st' ? parsed.year * 100 + 6 : (parsed.year + 1) * 100;
  }
}

/**
 * Compare two assessment periods chronologically.
 * @returns Negative if p1 < p2, positive if p1 > p2, 0 if equal
 */
export function compareAssessmentPeriods(period1: string, period2: string): number {
  const parsed1 = parseAssessmentPeriod(period1);
  const parsed2 = parseAssessmentPeriod(period2);

  if (!parsed1 || !parsed2) return 0;

  return periodSortKey(parsed1) - periodSortKey(parsed2);
}
