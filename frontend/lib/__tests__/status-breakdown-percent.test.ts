import { describe, expect, it } from 'vitest';
import { formatSlicePercent } from '@/lib/status-breakdown-percent';

describe('formatSlicePercent', () => {
  it('computes value/total * 100, rounded, with a trailing %', () => {
    expect(formatSlicePercent(5, 10)).toBe('50%');
    expect(formatSlicePercent(3, 10)).toBe('30%');
    expect(formatSlicePercent(1, 3)).toBe('33%'); // 33.33... rounds to 33
    expect(formatSlicePercent(2, 3)).toBe('67%'); // 66.67... rounds to 67
  });

  it('returns "0%" (not NaN/Infinity) when total is zero', () => {
    expect(formatSlicePercent(0, 0)).toBe('0%');
    expect(formatSlicePercent(5, 0)).toBe('0%');
  });

  it('returns "0%" for a zero-count status against a non-zero total', () => {
    expect(formatSlicePercent(0, 10)).toBe('0%');
  });

  it('returns "100%" when the value equals the total', () => {
    expect(formatSlicePercent(10, 10)).toBe('100%');
  });

  it('never produces the literal string "NaN%"', () => {
    const result = formatSlicePercent(0, 0);
    expect(result).not.toContain('NaN');
  });
});
