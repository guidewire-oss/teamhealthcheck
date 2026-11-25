import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const updateActionItem = vi.fn().mockResolvedValue(undefined);
const createActionItem = vi.fn().mockResolvedValue({ id: 'new' });

vi.mock('@/lib/api/action-items', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api/action-items')>();
  return {
    ...actual,
    updateActionItem: (...a: any[]) => updateActionItem(...a),
    createActionItem: (...a: any[]) => createActionItem(...a),
  };
});

import ActionItemModal from '../ActionItemModal';
import type { ActionItem } from '@/lib/api/action-items';

const members = [{ id: 'u1', name: 'Alice' }];

const fullItem: ActionItem = {
  id: 'item-1',
  teamId: 'team-1',
  dimensionId: 'mission',
  dimensionName: 'Mission',
  createdBy: 'lead-1',
  createdByName: 'Lead One',
  assignedTo: 'u1',
  assigneeName: 'Alice',
  title: 'Improve standups',
  description: 'desc',
  status: 'open',
  dueDate: '2026-05-04',
  assessmentPeriod: '2026 - 1st Half',
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
};

function renderEdit(item: ActionItem) {
  return render(
    <ActionItemModal
      teamId="team-1"
      assessmentPeriod="2026 - 1st Half"
      teamMembers={members}
      editItem={item}
      onSaved={vi.fn()}
      onClose={vi.fn()}
    />
  );
}

describe('ActionItemModal clearing optional fields', () => {
  beforeEach(() => {
    updateActionItem.mockClear();
    createActionItem.mockClear();
  });

  it('sends null for assignee, dimension and due date when the user clears them', async () => {
    const user = userEvent.setup();
    renderEdit(fullItem);

    await user.selectOptions(screen.getByTestId('action-item-assignee-select'), '');
    await user.selectOptions(screen.getByTestId('action-item-dimension-select'), '');
    await user.clear(screen.getByTestId('action-item-due-date'));
    await user.click(screen.getByTestId('action-item-save-btn'));

    await waitFor(() => expect(updateActionItem).toHaveBeenCalledTimes(1));
    const payload = updateActionItem.mock.calls[0][2];
    // null survives JSON.stringify; undefined would be dropped and the server would keep the old value.
    expect(payload.assignedTo).toBeNull();
    expect(payload.dimensionId).toBeNull();
    expect(payload.dueDate).toBeNull();
    expect(JSON.parse(JSON.stringify(payload))).toMatchObject({
      assignedTo: null,
      dimensionId: null,
      dueDate: null,
    });
  });

  it('sends the existing values unchanged when nothing is cleared', async () => {
    const user = userEvent.setup();
    renderEdit(fullItem);

    await user.click(screen.getByTestId('action-item-save-btn'));

    await waitFor(() => expect(updateActionItem).toHaveBeenCalledTimes(1));
    expect(updateActionItem.mock.calls[0][2]).toMatchObject({
      assignedTo: 'u1',
      dimensionId: 'mission',
      dueDate: '2026-05-04',
    });
  });

  it('does not send null fields when creating a new item', async () => {
    const user = userEvent.setup();
    render(
      <ActionItemModal
        teamId="team-1"
        assessmentPeriod="2026 - 1st Half"
        teamMembers={members}
        onSaved={vi.fn()}
        onClose={vi.fn()}
      />
    );

    await user.type(screen.getByTestId('action-item-title-input'), 'New item');
    await user.click(screen.getByTestId('action-item-save-btn'));

    await waitFor(() => expect(createActionItem).toHaveBeenCalledTimes(1));
    const payload = createActionItem.mock.calls[0][1];
    expect(payload).not.toHaveProperty('assignedTo');
    expect(payload).not.toHaveProperty('dimensionId');
    expect(payload).not.toHaveProperty('dueDate');
  });
});
