import { describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ReminderTreeModal from '@/components/ReminderTreeModal';
import type { ReminderPlan } from '@/lib/reminder-tree';

function plan(): ReminderPlan {
  return {
    request: { scope: 'person', personId: 'mgr-1' },
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

describe('ReminderTreeModal — focus trap', () => {
  function renderWithOutsideButtons() {
    const before = document.createElement('button');
    before.textContent = 'Before';
    const after = document.createElement('button');
    after.textContent = 'After';
    document.body.prepend(before);
    document.body.append(after);
    const utils = render(<ReminderTreeModal plan={plan()} onCancel={() => {}} onSend={() => {}} />);
    return {
      ...utils,
      before,
      after,
      cleanup: () => {
        before.remove();
        after.remove();
      },
    };
  }

  it('wraps Tab from the last control (Send) back to the first (Close)', async () => {
    const user = userEvent.setup();
    const { cleanup, after } = renderWithOutsideButtons();

    screen.getByTestId('reminder-tree-send').focus();
    await user.tab();

    expect(document.activeElement).toBe(screen.getByTestId('reminder-tree-close'));
    expect(document.activeElement).not.toBe(after);
    cleanup();
  });

  it('wraps Shift+Tab from the first control (Close) to the last (Send)', async () => {
    const user = userEvent.setup();
    const { cleanup, before } = renderWithOutsideButtons();

    expect(document.activeElement).toBe(screen.getByTestId('reminder-tree-close'));
    await user.tab({ shift: true });

    expect(document.activeElement).toBe(screen.getByTestId('reminder-tree-send'));
    expect(document.activeElement).not.toBe(before);
    cleanup();
  });

  it('advances normally between controls inside the dialog', async () => {
    const user = userEvent.setup();
    const { cleanup } = renderWithOutsideButtons();

    await user.tab();

    expect(document.activeElement).toBe(screen.getByTestId('reminder-tree-cancel'));
    cleanup();
  });

  it('pulls focus back inside when it has escaped the dialog', async () => {
    const user = userEvent.setup();
    const { cleanup, after } = renderWithOutsideButtons();

    after.focus();
    await user.tab();

    expect(screen.getByRole('dialog').contains(document.activeElement)).toBe(true);
    cleanup();
  });

  it('keeps focus on the dialog when it has no focusable descendants', () => {
    render(<ReminderTreeModal plan={plan()} onCancel={() => {}} onSend={() => {}} />);
    const dialog = screen.getByRole('dialog');
    dialog.querySelectorAll('button').forEach((b) => b.setAttribute('disabled', ''));
    dialog.focus();

    const event = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true });
    document.dispatchEvent(event);

    expect(event.defaultPrevented).toBe(true);
    expect(document.activeElement).toBe(dialog);
  });

  it('does not re-steal focus when the parent re-renders with a new onCancel', () => {
    const { rerender } = render(<ReminderTreeModal plan={plan()} onCancel={() => {}} onSend={() => {}} />);
    screen.getByTestId('reminder-tree-send').focus();

    rerender(<ReminderTreeModal plan={plan()} onCancel={() => {}} onSend={() => {}} />);

    expect(document.activeElement).toBe(screen.getByTestId('reminder-tree-send'));
  });

  it('stops intercepting Tab once closed', () => {
    const { unmount } = render(<ReminderTreeModal plan={plan()} onCancel={() => {}} onSend={() => {}} />);
    unmount();

    const event = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true });
    document.dispatchEvent(event);

    expect(event.defaultPrevented).toBe(false);
  });
});

describe('ReminderTreeModal — status exposed to assistive tech', () => {
  it('gives each team status dot an accessible name from STATUS_LABEL', () => {
    render(<ReminderTreeModal plan={plan()} onCancel={() => {}} onSend={() => {}} />);

    const node = screen.getByTestId('reminder-tree-team-node');
    expect(within(node).getByRole('img', { name: 'Not started' })).toBeInTheDocument();
  });

  it('hides the decorative legend dots so the status is not announced twice', () => {
    render(<ReminderTreeModal plan={plan()} onCancel={() => {}} onSend={() => {}} />);

    const legend = screen.getAllByTestId('reminder-tree-legend')[0];
    expect(within(legend).queryByRole('img')).toBeNull();
    expect(legend).toHaveTextContent('Not started');
  });
});

describe('ReminderTreeModal — sending state', () => {
  it('enables Send and calls onSend when idle', async () => {
    const user = userEvent.setup();
    const onSend = vi.fn();
    render(<ReminderTreeModal plan={plan()} onCancel={() => {}} onSend={onSend} />);

    const send = screen.getByTestId('reminder-tree-send');
    expect(send).toBeEnabled();
    expect(send).toHaveTextContent('Send reminder');

    await user.click(send);
    expect(onSend).toHaveBeenCalledTimes(1);
  });

  it('disables Send and says Sending while the request is in flight', async () => {
    const user = userEvent.setup();
    const onSend = vi.fn();
    render(<ReminderTreeModal plan={plan()} onCancel={() => {}} onSend={onSend} isSending />);

    const send = screen.getByTestId('reminder-tree-send');
    expect(send).toBeDisabled();
    expect(send).toHaveTextContent('Sending…');

    await user.click(send);
    expect(onSend).not.toHaveBeenCalled();
  });
});
