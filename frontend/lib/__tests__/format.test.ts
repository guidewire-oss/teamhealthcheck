import { formatScore } from '../format';

describe('formatScore', () => {
  it('formats a repeating-decimal average to one decimal place', () => {
    expect(formatScore(2.388888888888889)).toBe('2.4');
  });

  it('formats an integer with a trailing .0', () => {
    expect(formatScore(2)).toBe('2.0');
  });

  it('rounds to the nearest tenth', () => {
    expect(formatScore(1.65)).toBe('1.7');
  });

  it('supports a custom number of digits', () => {
    expect(formatScore(2.388888888888889, 2)).toBe('2.39');
  });
});
