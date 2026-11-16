/**
 * Pure percentage formatting for the status breakdown pie chart. Split out
 * so it can be unit-tested without rendering Recharts — same pattern as
 * lib/team-completion-filter.ts.
 */

/**
 * Formats one status's share of the total as a whole-number percentage
 * string (e.g. "25%"), matching the app's existing rounding convention
 * (status badges, the "Completion by team" bars, this chart's own
 * tooltip). Safe when total is 0 (no data at all yet) — returns "0%"
 * rather than NaN/Infinity/"NaN%".
 */
export function formatSlicePercent(value: number, total: number): string {
  if (!total) return '0%';
  return `${Math.round((value / total) * 100)}%`;
}
