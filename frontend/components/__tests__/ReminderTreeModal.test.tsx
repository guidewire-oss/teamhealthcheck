import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ReminderTreeModal from '@/components/ReminderTreeModal';
import type { ReminderPlan } from '@/lib/reminder-tree';

function plan(): ReminderPlan {
  return {
    root: {
      type: 'person',
      id: 'mgr-1',
      name: 'Manager One',
      level: 'Manager',
      children: [{ type: 'team', id: 't1', name: 'Pod A', teamLeadName: 'Alice', status: 'not_started' }],
    },
    podCount: 1,
    subtitle: 'Reminding 1 pod under Manager One',
    toastMessage: 'Reminder sent to Manager One and 1 team lead.',
  };
}

// Regression coverage: the modal previously had no dialog semantics, no
// Escape handling, and no focus management at all -- unlike its sibling,
// CompletionByTeamFullscreenModal.
describe('ReminderTreeModal — accessibility', () => {
  it('exposes dialog semantics with an accessible name from the title', () => {
    render(<ReminderTreeModal plan={plan()} onCancel={() => {}} onSend={() => {}} />);

    const dialog = screen.getByRole('dialog');
    expect(dialog).toHaveAttribute('aria-modal', 'true');
    expect(dialog).toHaveAccessibleName('Who will get this reminder?');
  });

  it('closes on Escape', async () => {
    const user = userEvent.setup();
    const onCancel = vi.fn();
    render(<ReminderTreeModal plan={plan()} onCancel={onCancel} onSend={() => {}} />);

    await user.keyboard('{Escape}');

    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it('moves focus to the close button on open and restores it to the trigger on close', async () => {
    const opener = document.createElement('button');
    opener.textContent = 'Open';
    document.body.appendChild(opener);
    opener.focus();
    expect(document.activeElement).toBe(opener);

    const { unmount } = render(<ReminderTreeModal plan={plan()} onCancel={() => {}} onSend={() => {}} />);

    expect(document.activeElement).toBe(screen.getByTestId('reminder-tree-close'));

    unmount();

    expect(document.activeElement).toBe(opener);
    opener.remove();
  });

  it('locks page scrolling while open and restores it on close', () => {
    const { unmount } = render(<ReminderTreeModal plan={plan()} onCancel={() => {}} onSend={() => {}} />);

    expect(document.body.style.overflow).toBe('hidden');

    unmount();

    expect(document.body.style.overflow).not.toBe('hidden');
  });
});
