import { describe, it, expect } from 'vitest';
import { getDimensionTrendDisplay } from '../dimension-trend';

describe('getDimensionTrendDisplay', () => {
  it('renders "Improving" with the green improving style', () => {
    const result = getDimensionTrendDisplay('improving');
    expect(result).toEqual({
      trend: 'improving',
      label: 'Improving',
      colorClass: 'text-green-600',
      icon: 'up',
    });
  });

  it('renders "Stable" with the neutral stable style', () => {
    const result = getDimensionTrendDisplay('stable');
    expect(result).toEqual({
      trend: 'stable',
      label: 'Stable',
      colorClass: 'text-gray-400',
      icon: 'stable',
    });
  });

  it('renders "Declining" with the red declining style', () => {
    const result = getDimensionTrendDisplay('declining');
    expect(result).toEqual({
      trend: 'declining',
      label: 'Declining',
      colorClass: 'text-red-600',
      icon: 'down',
    });
  });

  it('hides the indicator when trend is undefined', () => {
    expect(getDimensionTrendDisplay(undefined)).toBeNull();
  });

  it('hides the indicator when trend is null', () => {
    expect(getDimensionTrendDisplay(null)).toBeNull();
  });

  it('hides the indicator when trend is an empty string', () => {
    expect(getDimensionTrendDisplay('')).toBeNull();
  });

  it('hides the indicator for an unsupported trend value', () => {
    expect(getDimensionTrendDisplay('skyrocketing')).toBeNull();
  });

  it('is case-sensitive and hides indicator for mismatched casing', () => {
    // Backend always sends lowercase (DB CHECK constraint enforces this);
    // an unexpected casing should be treated as unsupported rather than guessed at.
    expect(getDimensionTrendDisplay('Improving')).toBeNull();
  });
});
