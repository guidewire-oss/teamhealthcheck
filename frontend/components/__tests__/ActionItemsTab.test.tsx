import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const listActionItems = vi.fn();
const updateActionItem = vi.fn().mockResolvedValue(undefined);
const deleteActionItem = vi.fn().mockResolvedValue(undefined);
const getDirectManager = vi.fn().mockResolvedValue(null);

vi.mock('@/lib/api/action-items', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api/action-items')>();
  return {
    ...actual,
    listActionItems: (...a: any[]) => listActionItems(...a),
    updateActionItem: (...a: any[]) => updateActionItem(...a),
    deleteActionItem: (...a: any[]) => deleteActionItem(...a),
    getDirectManager: (...a: any[]) => getDirectManager(...a),
  };
});

import ActionItemsTab from '../ActionItemsTab';

const baseItem = {
  id: 'item-1',
  teamId: 'team-1',
  dimensionId: null,
  dimensionName: null,
  createdBy: 'lead-1',
  createdByName: 'Lead One',
  assignedTo: null,
  assigneeName: null,
  title: 'Improve standups',
  description: '',
  dueDate: null,
  assessmentPeriod: null,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
};

describe('ActionItemsTab', () => {
  beforeEach(() => {
    listActionItems.mockReset();
    updateActionItem.mockClear();
    deleteActionItem.mockClear();
  });

  afterEach(() => {
    // Safety net: no test in this file is expected to leave fake timers
    // installed, but restoring real timers unconditionally keeps a stray
    // vi.useFakeTimers() in one test from stalling async findBy*/waitFor
    // polling in a later one.
    vi.useRealTimers();
  });

  it('renders an overdue due date in red with the warning glyph', async () => {
    // Real timers only: findByTestId/waitFor poll via real timers, and
    // combining that with vi.useFakeTimers() here previously stalled this
    // test. An unambiguously historical due date makes the item overdue
    // against the real current date without needing to fake "now".
    listActionItems.mockResolvedValue([{ ...baseItem, status: 'open', dueDate: '2020-05-04' }]);
    render(
      <ActionItemsTab
        teamId="team-1"
        assessmentPeriod="2026 - 1st Half"
        teamMembers={[]}
        canEdit={true}
        currentUserId="lead-1"
      />
    );
    const dueDateText = await screen.findByTestId('action-item-due-date-text');
    expect(dueDateText).toHaveTextContent('Due 4 May 2020 ⚠');
    expect(dueDateText.className).toContain('text-red-600');
  });

  it('renders Open, On Hold, In Progress, and Done columns left-to-right', async () => {
    listActionItems.mockResolvedValue([]);
    render(
      <ActionItemsTab
        teamId="team-1"
        assessmentPeriod="2026 - 1st Half"
        teamMembers={[]}
        canEdit={true}
        currentUserId="lead-1"
      />
    );
    await waitFor(() => expect(listActionItems).toHaveBeenCalled());

    const headers = screen.getAllByText(/^(Open|On Hold|In Progress|Done)$/);
    expect(headers.map((h) => h.textContent)).toEqual(['Open', 'On Hold', 'In Progress', 'Done']);
  });

  it('shows Move to Hold and Mark Done on an In Progress card, and transitions correctly', async () => {
    listActionItems.mockResolvedValue([{ ...baseItem, status: 'in_progress' }]);
    const user = userEvent.setup();
    render(
      <ActionItemsTab
        teamId="team-1"
        assessmentPeriod="2026 - 1st Half"
        teamMembers={[]}
        canEdit={true}
        currentUserId="lead-1"
      />
    );
    await waitFor(() => expect(screen.getByText('Improve standups')).toBeInTheDocument());

    expect(screen.getByText('Move to Hold')).toBeInTheDocument();
    expect(screen.getByText('Mark Done')).toBeInTheDocument();

    await user.click(screen.getByText('Move to Hold'));
    expect(updateActionItem).toHaveBeenCalledWith('team-1', 'item-1', { status: 'on_hold' });
  });

  it('shows Resume on an On Hold card', async () => {
    listActionItems.mockResolvedValue([{ ...baseItem, status: 'on_hold' }]);
    render(
      <ActionItemsTab
        teamId="team-1"
        assessmentPeriod="2026 - 1st Half"
        teamMembers={[]}
        canEdit={true}
        currentUserId="lead-1"
      />
    );
    await waitFor(() => expect(screen.getByText('Resume')).toBeInTheDocument());
  });

  it('only shows the edit icon for the creator, and never for a Done item', async () => {
    listActionItems.mockResolvedValue([{ ...baseItem, status: 'open' }]);
    const { rerender } = render(
      <ActionItemsTab
        teamId="team-1"
        assessmentPeriod="2026 - 1st Half"
        teamMembers={[]}
        canEdit={true}
        currentUserId="lead-1"
      />
    );
    await waitFor(() => expect(screen.getByTestId('edit-action-item-1')).toBeInTheDocument());

    listActionItems.mockResolvedValue([{ ...baseItem, status: 'open' }]);
    rerender(
      <ActionItemsTab
        teamId="team-1"
        assessmentPeriod="2026 - 1st Half"
        teamMembers={[]}
        canEdit={true}
        currentUserId="someone-else"
      />
    );
    await waitFor(() => expect(screen.getByText('Improve standups')).toBeInTheDocument());
    expect(screen.queryByTestId('edit-action-item-1')).not.toBeInTheDocument();
  });
});
