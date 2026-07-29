// SPDX-License-Identifier: Apache-2.0

/**
 * Utility functions for cadence-driven assessment period detection.
 *
 * Assessment period format depends on team cadence:
 *   - Monthly:     "YYYY Mon"  (e.g., "2026 Mar")
 *   - Quarterly:   "YYYY Q1"   (e.g., "2026 Q1")
 *   - Half-yearly: "YYYY H1"   (e.g., "2026 H1")
 *   - Yearly:      "YYYY"      (e.g., "2026")
 *
 * Legacy format "YYYY - 1st/2nd Half" is still parsed for backward compatibility.
 */

export type Cadence = 'monthly' | 'quarterly' | 'half-yearly' | 'yearly';

const VALID_CADENCES: ReadonlySet<string> = new Set(['monthly', 'quarterly', 'half-yearly', 'yearly']);

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
    case 'quarterly':
      return `${year} Q${Math.floor(month / 3) + 1}`;
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
 */
export type ParsedPeriod =
  | { type: 'monthly'; year: number; month: number }       // month: 0-indexed
  | { type: 'quarterly'; year: number; quarter: number }    // quarter: 1-4
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
