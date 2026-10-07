import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import PodActionBoard from '../PodActionBoard';
import { ActionItem } from '@/lib/api/action-items';

function mockTruncated(scrollHeight: number, clientHeight: number) {
  vi.spyOn(HTMLElement.prototype, 'scrollHeight', 'get').mockReturnValue(scrollHeight);
  vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(clientHeight);
}

afterEach(() => {
  vi.restoreAllMocks();
});

const item: ActionItem = {
  id: 'item-1',
  teamId: 'team-1',
  dimensionId: 'delivering-value',
  dimensionName: 'Delivering Value',
  createdBy: 'lead-1',
  createdByName: 'Lead One',
  assignedTo: null,
  assigneeName: null,
  title: 'hello',
  description: 'testing checking if expired date works and if manager is able to get assigned. my manager is john.',
  status: 'on_hold',
  dueDate: null,
  assessmentPeriod: null,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
};

describe('PodActionBoard', () => {
  it('uses the shared ActionItemDescription component for a truncated description', async () => {
    mockTruncated(100, 40);
    const user = userEvent.setup();
    render(<PodActionBoard items={[item]} />);

    expect(screen.getByText('hello')).toBeInTheDocument();
    const toggle = screen.getByTestId('action-item-description-toggle');
    expect(toggle).toHaveTextContent('Show more');

    await user.click(toggle);
    expect(toggle).toHaveTextContent('Show less');
    expect(screen.getByTestId('action-item-description-text')).toHaveTextContent(item.description);
  });

  it('renders no action controls on the read-only board (no edit, start, hold, done, or delete)', () => {
    mockTruncated(40, 40);
    render(<PodActionBoard items={[item]} />);

    expect(screen.queryByText('Start')).not.toBeInTheDocument();
    expect(screen.queryByText('Move to Hold')).not.toBeInTheDocument();
    expect(screen.queryByText('Mark Done')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Edit action item')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Delete')).not.toBeInTheDocument();
  });

  it('applies the same overdue red/warning treatment as the Team Lead board', () => {
    mockTruncated(40, 40);
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 5, 1)); // 1 Jun 2026, after the due date below
    render(<PodActionBoard items={[{ ...item, status: 'on_hold', dueDate: '2026-05-04' }]} />);

    const dueDateText = screen.getByTestId('action-item-due-date-text');
    expect(dueDateText).toHaveTextContent('Due 4 May 2026 ⚠');
    expect(dueDateText.className).toContain('text-red-600');
    vi.useRealTimers();
  });

  it('keeps the assignee/due-date metadata row wrapping-safe (break-words on assignee, whitespace-nowrap on due date)', () => {
    mockTruncated(40, 40);
    render(
      <PodActionBoard
        items={[{ ...item, assigneeName: 'A Very Long Assignee Name That Could Wrap', dueDate: '2026-05-04' }]}
      />
    );

    expect(screen.getByText(/A Very Long Assignee Name/).className).toContain('break-words');
    expect(screen.getByTestId('action-item-due-date-text').className).toContain('whitespace-nowrap');
  });
});
