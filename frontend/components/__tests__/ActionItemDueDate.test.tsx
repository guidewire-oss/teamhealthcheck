import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import ActionItemDueDate from '../ActionItemDueDate';

afterEach(() => {
  vi.useRealTimers();
});

describe('ActionItemDueDate', () => {
  it('formats a future date as "Due D Mon YYYY" in ordinary gray, with no warning', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 0, 1)); // 1 Jan 2026 (local)

    render(<ActionItemDueDate dueDate="2026-05-04" status="open" />);
    const text = screen.getByTestId('action-item-due-date-text');

    expect(text).toHaveTextContent('Due 4 May 2026');
    expect(text.className).toContain('text-gray-400');
    expect(text.className).not.toContain('text-red-600');
  });

  it('applies red text and the warning glyph when the due date is overdue and the item is not Done', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 5, 1)); // 1 Jun 2026 (local) — after the due date

    render(<ActionItemDueDate dueDate="2026-05-04" status="open" />);
    const text = screen.getByTestId('action-item-due-date-text');

    expect(text.className).toContain('text-red-600');
    expect(text).toHaveTextContent('Due 4 May 2026 ⚠');
  });

  it('does not treat a Done item as overdue, even past its due date', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 5, 1));

    render(<ActionItemDueDate dueDate="2026-05-04" status="done" />);
    const text = screen.getByTestId('action-item-due-date-text');

    expect(text.className).not.toContain('text-red-600');
    expect(text).not.toHaveTextContent('⚠');
  });

  it('renders nothing for a null due date (no "Invalid Date")', () => {
    render(<ActionItemDueDate dueDate={null} status="open" />);
    expect(screen.queryByTestId('action-item-due-date-text')).not.toBeInTheDocument();
  });

  it('renders nothing for a malformed due date instead of showing "Invalid Date"', () => {
    render(<ActionItemDueDate dueDate="not-a-date" status="open" />);
    expect(screen.queryByTestId('action-item-due-date-text')).not.toBeInTheDocument();
    expect(screen.queryByText(/invalid date/i)).not.toBeInTheDocument();
  });

  it('is timezone-safe: parses YYYY-MM-DD components directly without shifting the calendar day', () => {
    // A UTC-midnight interpretation (e.g. `new Date("2026-05-04")`) would
    // display as 3 May in any timezone behind UTC. Explicit component
    // parsing must always show 4 May regardless of the host timezone.
    render(<ActionItemDueDate dueDate="2026-05-04" status="open" />);
    expect(screen.getByTestId('action-item-due-date-text')).toHaveTextContent('Due 4 May 2026');
  });

  it('uses whitespace-nowrap so the due date never squeezes mid-text when wrapping to a new line', () => {
    render(<ActionItemDueDate dueDate="2026-05-04" status="open" />);
    expect(screen.getByTestId('action-item-due-date-text').className).toContain('whitespace-nowrap');
  });
});
