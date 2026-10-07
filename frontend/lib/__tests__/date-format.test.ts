import { describe, it, expect } from 'vitest';
import { displayDateToIso, isoDateToDisplay } from '../date-format';

describe('displayDateToIso', () => {
  it('parses the 06/12/2026 regression case (6 December 2026, dd/mm/yyyy) without using locale-dependent Date parsing', () => {
    // The field is explicitly dd/mm/yyyy: day=06, month=12, so this is
    // 6 December 2026, not 12 June 2026.
    expect(displayDateToIso('06/12/2026')).toBe('2026-12-06');
  });

  it('round-trips through isoDateToDisplay', () => {
    const iso = displayDateToIso('06/12/2026');
    expect(isoDateToDisplay(iso)).toBe('06/12/2026');
  });

  it('accepts single-digit day/month', () => {
    expect(displayDateToIso('1/2/2026')).toBe('2026-02-01');
  });

  it('rejects an impossible calendar date (31 February)', () => {
    expect(displayDateToIso('31/02/2026')).toBeNull();
  });

  it('rejects 29 February on a non-leap year', () => {
    expect(displayDateToIso('29/02/2025')).toBeNull();
  });

  it('accepts 29 February on a leap year', () => {
    expect(displayDateToIso('29/02/2024')).toBe('2024-02-29');
  });

  it('rejects malformed input', () => {
    expect(displayDateToIso('2026-06-12')).toBeNull();
    expect(displayDateToIso('not a date')).toBeNull();
    expect(displayDateToIso('')).toBeNull();
  });
});

describe('isoDateToDisplay', () => {
  it('formats an ISO date as dd/mm/yyyy', () => {
    expect(isoDateToDisplay('2026-06-12')).toBe('12/06/2026');
  });

  it('returns an empty string for null/invalid input', () => {
    expect(isoDateToDisplay(null)).toBe('');
    expect(isoDateToDisplay(undefined)).toBe('');
    expect(isoDateToDisplay('garbage')).toBe('');
  });
});
