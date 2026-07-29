// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

/**
 * Tests for cadence-driven assessment period utility functions.
 *
 * Period format depends on team cadence:
 *   - Monthly:     "YYYY Mon"  (e.g., "2026 Mar")
 *   - Quarterly:   "YYYY Q1"   (e.g., "2026 Q1")
 *   - Half-yearly: "YYYY H1"   (e.g., "2026 H1")
 *   - Yearly:      "YYYY"      (e.g., "2026")
 *
 * Legacy format "YYYY - 1st/2nd Half" is parsed for backward compatibility.
 */

import { getAssessmentPeriod, getCurrentAssessmentPeriod, parseAssessmentPeriod, compareAssessmentPeriods, Cadence } from '../assessment-period';

// Helper to create dates in local timezone (avoids UTC parsing issues)
const createDate = (year: number, month: number, day: number) => new Date(year, month - 1, day);

describe('getAssessmentPeriod', () => {
  describe('monthly cadence', () => {
    it('should return "2026 Jan" for January 2026', () => {
      expect(getAssessmentPeriod(createDate(2026, 1, 15), 'monthly')).toBe('2026 Jan');
    });

    it('should return "2026 Jun" for June 2026', () => {
      expect(getAssessmentPeriod(createDate(2026, 6, 30), 'monthly')).toBe('2026 Jun');
    });

    it('should return "2026 Dec" for December 2026', () => {
      expect(getAssessmentPeriod(createDate(2026, 12, 1), 'monthly')).toBe('2026 Dec');
    });

    it('should return correct month for each month of the year', () => {
      const months = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
      months.forEach((name, i) => {
        expect(getAssessmentPeriod(createDate(2026, i + 1, 15), 'monthly')).toBe(`2026 ${name}`);
      });
    });
  });

  describe('quarterly cadence', () => {
    it('should return Q1 for Jan-Mar', () => {
      expect(getAssessmentPeriod(createDate(2026, 1, 1), 'quarterly')).toBe('2026 Q1');
      expect(getAssessmentPeriod(createDate(2026, 2, 15), 'quarterly')).toBe('2026 Q1');
      expect(getAssessmentPeriod(createDate(2026, 3, 31), 'quarterly')).toBe('2026 Q1');
    });

    it('should return Q2 for Apr-Jun', () => {
      expect(getAssessmentPeriod(createDate(2026, 4, 1), 'quarterly')).toBe('2026 Q2');
      expect(getAssessmentPeriod(createDate(2026, 5, 15), 'quarterly')).toBe('2026 Q2');
      expect(getAssessmentPeriod(createDate(2026, 6, 30), 'quarterly')).toBe('2026 Q2');
    });

    it('should return Q3 for Jul-Sep', () => {
      expect(getAssessmentPeriod(createDate(2026, 7, 1), 'quarterly')).toBe('2026 Q3');
      expect(getAssessmentPeriod(createDate(2026, 9, 30), 'quarterly')).toBe('2026 Q3');
    });

    it('should return Q4 for Oct-Dec', () => {
      expect(getAssessmentPeriod(createDate(2026, 10, 1), 'quarterly')).toBe('2026 Q4');
      expect(getAssessmentPeriod(createDate(2026, 12, 31), 'quarterly')).toBe('2026 Q4');
    });
  });

  describe('half-yearly cadence', () => {
    it('should return H1 for Jan-Jun', () => {
      expect(getAssessmentPeriod(createDate(2026, 1, 1), 'half-yearly')).toBe('2026 H1');
      expect(getAssessmentPeriod(createDate(2026, 3, 15), 'half-yearly')).toBe('2026 H1');
      expect(getAssessmentPeriod(createDate(2026, 6, 30), 'half-yearly')).toBe('2026 H1');
    });

    it('should return H2 for Jul-Dec', () => {
      expect(getAssessmentPeriod(createDate(2026, 7, 1), 'half-yearly')).toBe('2026 H2');
      expect(getAssessmentPeriod(createDate(2026, 9, 15), 'half-yearly')).toBe('2026 H2');
      expect(getAssessmentPeriod(createDate(2026, 12, 31), 'half-yearly')).toBe('2026 H2');
    });
  });

  describe('yearly cadence', () => {
    it('should return just the year', () => {
      expect(getAssessmentPeriod(createDate(2025, 3, 15), 'yearly')).toBe('2025');
      expect(getAssessmentPeriod(createDate(2026, 9, 15), 'yearly')).toBe('2026');
    });
  });

  describe('default cadence (half-yearly)', () => {
    it('should default to half-yearly when no cadence provided', () => {
      expect(getAssessmentPeriod(createDate(2026, 3, 15))).toBe('2026 H1');
      expect(getAssessmentPeriod(createDate(2026, 9, 15))).toBe('2026 H2');
    });

    it('should use current date when no arguments provided', () => {
      const result = getAssessmentPeriod();
      expect(result).toMatch(/^\d{4} H[12]$/);
    });
  });

  describe('different years', () => {
    it('should work correctly across years', () => {
      expect(getAssessmentPeriod(createDate(2024, 3, 15), 'quarterly')).toBe('2024 Q1');
      expect(getAssessmentPeriod(createDate(2025, 8, 15), 'monthly')).toBe('2025 Aug');
      expect(getAssessmentPeriod(createDate(2027, 11, 1), 'half-yearly')).toBe('2027 H2');
    });
  });

  describe('time of day should not matter', () => {
    it('should return same period for start and end of day', () => {
      const startOfDay = new Date(2026, 2, 31, 0, 0, 0);   // Mar 31 00:00:00
      const endOfDay = new Date(2026, 2, 31, 23, 59, 59);   // Mar 31 23:59:59
      expect(getAssessmentPeriod(startOfDay, 'quarterly')).toBe('2026 Q1');
      expect(getAssessmentPeriod(endOfDay, 'quarterly')).toBe('2026 Q1');
    });
  });
});

describe('getCurrentAssessmentPeriod', () => {
  it('should return a valid half-yearly period by default', () => {
    const result = getCurrentAssessmentPeriod();
    expect(result).toMatch(/^\d{4} H[12]$/);
  });

  it('should accept a cadence parameter', () => {
    const result = getCurrentAssessmentPeriod('quarterly');
    expect(result).toMatch(/^\d{4} Q[1-4]$/);
  });
});

describe('parseAssessmentPeriod', () => {
  describe('monthly format', () => {
    it('should parse "2026 Mar"', () => {
      expect(parseAssessmentPeriod('2026 Mar')).toEqual({ type: 'monthly', year: 2026, month: 2 });
    });

    it('should parse all month names', () => {
      const months = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
      months.forEach((name, i) => {
        expect(parseAssessmentPeriod(`2026 ${name}`)).toEqual({ type: 'monthly', year: 2026, month: i });
      });
    });
  });

  describe('quarterly format', () => {
    it('should parse "2026 Q1" through "2026 Q4"', () => {
      expect(parseAssessmentPeriod('2026 Q1')).toEqual({ type: 'quarterly', year: 2026, quarter: 1 });
      expect(parseAssessmentPeriod('2026 Q4')).toEqual({ type: 'quarterly', year: 2026, quarter: 4 });
    });

    it('should reject Q0 and Q5', () => {
      expect(parseAssessmentPeriod('2026 Q0')).toBeNull();
      expect(parseAssessmentPeriod('2026 Q5')).toBeNull();
    });
  });

  describe('half-yearly format', () => {
    it('should parse "2026 H1" and "2026 H2"', () => {
      expect(parseAssessmentPeriod('2026 H1')).toEqual({ type: 'half-yearly', year: 2026, half: 1 });
      expect(parseAssessmentPeriod('2026 H2')).toEqual({ type: 'half-yearly', year: 2026, half: 2 });
    });

    it('should reject H0 and H3', () => {
      expect(parseAssessmentPeriod('2026 H0')).toBeNull();
      expect(parseAssessmentPeriod('2026 H3')).toBeNull();
    });
  });

  describe('yearly format', () => {
    it('should parse "2026"', () => {
      expect(parseAssessmentPeriod('2026')).toEqual({ type: 'yearly', year: 2026 });
    });
  });

  describe('legacy format', () => {
    it('should parse "2024 - 1st Half" and "2024 - 2nd Half"', () => {
      expect(parseAssessmentPeriod('2024 - 1st Half')).toEqual({ type: 'legacy', year: 2024, half: '1st' });
      expect(parseAssessmentPeriod('2024 - 2nd Half')).toEqual({ type: 'legacy', year: 2024, half: '2nd' });
    });
  });

  describe('invalid formats', () => {
    it('should return null for invalid strings', () => {
      expect(parseAssessmentPeriod('2024 - 3rd Half')).toBeNull();
      expect(parseAssessmentPeriod('2024-first-half')).toBeNull();
      expect(parseAssessmentPeriod('invalid')).toBeNull();
      expect(parseAssessmentPeriod('')).toBeNull();
      expect(parseAssessmentPeriod('abcd')).toBeNull();
    });
  });
});

describe('compareAssessmentPeriods', () => {
  it('should compare same-format periods', () => {
    expect(compareAssessmentPeriods('2026 Q1', '2026 Q2')).toBeLessThan(0);
    expect(compareAssessmentPeriods('2026 Q3', '2026 Q1')).toBeGreaterThan(0);
    expect(compareAssessmentPeriods('2026 H1', '2026 H1')).toBe(0);
  });

  it('should compare periods across different years', () => {
    expect(compareAssessmentPeriods('2025 Q4', '2026 Q1')).toBeLessThan(0);
    expect(compareAssessmentPeriods('2026 H1', '2025 H2')).toBeGreaterThan(0);
  });

  it('should compare cross-format periods using sort keys', () => {
    // Q1 starts at month 0, H1 starts at month 0 → equal sort key
    expect(compareAssessmentPeriods('2026 Q1', '2026 H1')).toBe(0);
    // Q3 starts at month 6, H2 starts at month 6 → equal sort key
    expect(compareAssessmentPeriods('2026 Q3', '2026 H2')).toBe(0);
    // Monthly Mar (month 2) vs Q1 (month 0) → Mar > Q1
    expect(compareAssessmentPeriods('2026 Mar', '2026 Q1')).toBeGreaterThan(0);
  });

  it('should compare legacy format with new formats', () => {
    // Legacy "YYYY - 1st Half" covers Jul-Dec of YYYY = equivalent to "YYYY H2"
    expect(compareAssessmentPeriods('2024 - 1st Half', '2024 H2')).toBe(0);
    // Legacy "YYYY - 2nd Half" covers Jan-Jun of YYYY+1 = equivalent to "(YYYY+1) H1"
    expect(compareAssessmentPeriods('2024 - 2nd Half', '2025 H1')).toBe(0);
    // "2024 - 2nd Half" (Jan-Jun 2025) should sort AFTER "2024 - 1st Half" (Jul-Dec 2024)
    expect(compareAssessmentPeriods('2024 - 1st Half', '2024 - 2nd Half')).toBeLessThan(0);
  });

  it('should return 0 for invalid periods', () => {
    expect(compareAssessmentPeriods('invalid', '2026 Q1')).toBe(0);
    expect(compareAssessmentPeriods('2026 Q1', 'invalid')).toBe(0);
  });
});
