/**
 * Presentation-only formatting helpers. These must never be used to mutate
 * values used for calculations, filters, exports, or persistence — only for
 * display strings (e.g. chart tooltips, labels).
 */

/**
 * Formats a health score for display with a fixed number of decimal places.
 * Defaults to 1 decimal place (e.g. 2.388888888888889 -> "2.4", 2 -> "2.0").
 *
 * Uses Intl.NumberFormat rather than Number.prototype.toFixed, which rounds
 * half-way values like 1.65 down to "1.6" due to binary floating-point
 * representation error; Intl.NumberFormat rounds it correctly to "1.7".
 */
export function formatScore(score: number, digits: number = 1): string {
  return new Intl.NumberFormat('en-US', {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  }).format(score);
}
