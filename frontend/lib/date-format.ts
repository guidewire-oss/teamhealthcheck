/**
 * Explicit, timezone-safe conversion between the UI's dd/mm/yyyy display
 * format and the API/storage boundary's canonical ISO date-only format
 * (YYYY-MM-DD).
 *
 * Deliberately avoids `new Date(dateString)` for parsing: browsers disagree
 * on whether a slash-separated string like "06/12/2026" means 6 June or
 * 12 June, and `new Date("YYYY-MM-DD")` parses as UTC midnight, which can
 * shift a calendar date backward by a day in timezones behind UTC. Both
 * directions here work on the three numeric components directly, so the
 * intended calendar date is preserved regardless of the viewer's locale or
 * timezone.
 */

const DAYS_IN_MONTH = [31, 29, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];

function isLeapYear(year: number): boolean {
  return (year % 4 === 0 && year % 100 !== 0) || year % 400 === 0;
}

function isValidCalendarDate(year: number, month: number, day: number): boolean {
  if (month < 1 || month > 12) return false;
  const maxDay = month === 2 && !isLeapYear(year) ? 28 : DAYS_IN_MONTH[month - 1];
  return day >= 1 && day <= maxDay;
}

/**
 * Parses a dd/mm/yyyy string entered in the UI into a canonical
 * YYYY-MM-DD string. Returns null if the input isn't a real calendar date.
 */
export function displayDateToIso(display: string): string | null {
  const match = display.trim().match(/^(\d{1,2})\/(\d{1,2})\/(\d{4})$/);
  if (!match) return null;

  const day = Number(match[1]);
  const month = Number(match[2]);
  const year = Number(match[3]);
  if (!isValidCalendarDate(year, month, day)) return null;

  return `${String(year).padStart(4, '0')}-${String(month).padStart(2, '0')}-${String(day).padStart(2, '0')}`;
}

/**
 * Formats a canonical YYYY-MM-DD string for dd/mm/yyyy display. Returns an
 * empty string for null/invalid input.
 */
export function isoDateToDisplay(iso: string | null | undefined): string {
  if (!iso) return '';
  const match = iso.match(/^(\d{4})-(\d{2})-(\d{2})$/);
  if (!match) return '';
  const [, year, month, day] = match;
  return `${day}/${month}/${year}`;
}

const MONTH_ABBREVIATIONS = [
  'Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun',
  'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec',
];

/**
 * Formats a canonical YYYY-MM-DD string as "D Mon YYYY" (e.g. "4 May 2026")
 * for card display. Parses the three numeric components directly rather
 * than through `Date#toLocaleDateString`, so the result doesn't depend on
 * the viewer's locale or timezone. Returns null for null/invalid input —
 * callers should skip rendering rather than show "Invalid Date".
 */
export function formatDueDateLong(iso: string | null | undefined): string | null {
  if (!iso) return null;
  const match = iso.match(/^(\d{4})-(\d{2})-(\d{2})$/);
  if (!match) return null;
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  if (!isValidCalendarDate(year, month, day)) return null;
  return `${day} ${MONTH_ABBREVIATIONS[month - 1]} ${year}`;
}

/**
 * True when a canonical YYYY-MM-DD due date is strictly before today (local
 * calendar date, not UTC midnight, to avoid an off-by-one in timezones
 * behind UTC). Invalid/null input is never overdue.
 */
export function isDueDateOverdue(iso: string | null | undefined): boolean {
  if (!iso) return false;
  const match = iso.match(/^(\d{4})-(\d{2})-(\d{2})$/);
  if (!match) return false;
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  if (!isValidCalendarDate(year, month, day)) return false;

  const due = new Date(year, month - 1, day);
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  return due < today;
}
