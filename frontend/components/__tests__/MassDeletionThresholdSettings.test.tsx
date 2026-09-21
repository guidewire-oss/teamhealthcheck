import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

// The component reaches the backend only through these two functions.
const getThreshold = vi.fn();
const updateThreshold = vi.fn();

vi.mock('@/lib/api/admin', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api/admin')>();
  return {
    ...actual,
    getOrgSyncDeletionThreshold: (...a: any[]) => getThreshold(...a),
    updateOrgSyncDeletionThreshold: (...a: any[]) => updateThreshold(...a),
  };
});

import MassDeletionThresholdSettings from '../MassDeletionThresholdSettings';
import type { OrgSyncDeletionThreshold } from '@/lib/api/admin';

const threshold = (overrides: Partial<OrgSyncDeletionThreshold> = {}): OrgSyncDeletionThreshold => ({
  maxDeletePercent: 20,
  source: 'default',
  defaultPercent: 20,
  minPercent: 1,
  maxPercent: 100,
  locked: false,
  ...overrides,
});

describe('MassDeletionThresholdSettings', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getThreshold.mockResolvedValue(threshold());
    updateThreshold.mockImplementation(async (percent: number) =>
      threshold({ maxDeletePercent: percent, source: 'admin' })
    );
  });

  const slider = () => screen.getByTestId('threshold-slider') as HTMLInputElement;
  const input = () => screen.getByTestId('threshold-input') as HTMLInputElement;
  const saveBtn = () => screen.getByTestId('threshold-save-btn') as HTMLButtonElement;

  it('renders the threshold currently in force and where it came from', async () => {
    getThreshold.mockResolvedValue(threshold({ maxDeletePercent: 45, source: 'environment' }));
    render(<MassDeletionThresholdSettings />);

    await waitFor(() => expect(input().value).toBe('45'));
    expect(slider().value).toBe('45');
    expect(screen.getByTestId('threshold-source').textContent).toContain(
      'ORG_SYNC_MAX_DELETE_PERCENT'
    );
    expect(screen.getByTestId('threshold-recommended-hint').textContent).toContain('20%');
  });

  it('keeps the slider and the numeric input showing the same value', async () => {
    const user = userEvent.setup();
    render(<MassDeletionThresholdSettings />);
    await waitFor(() => expect(input().value).toBe('20'));

    await user.clear(input());
    await user.type(input(), '37');
    expect(slider().value).toBe('37');

    await user.click(screen.getByTestId('threshold-preset-10'));
    expect(input().value).toBe('10');
    expect(slider().value).toBe('10');
  });

  it('saves the chosen value and confirms it', async () => {
    const user = userEvent.setup();
    render(<MassDeletionThresholdSettings />);
    await waitFor(() => expect(input().value).toBe('20'));

    await user.click(screen.getByTestId('threshold-preset-50'));
    await user.click(saveBtn());

    await waitFor(() => expect(updateThreshold).toHaveBeenCalledWith(50));
    expect(await screen.findByTestId('threshold-save-success')).toBeTruthy();
    expect(screen.getByTestId('threshold-source').textContent).toContain('saved here');
  });

  it('rejects empty, out-of-range and non-numeric input without calling the backend', async () => {
    const user = userEvent.setup();
    render(<MassDeletionThresholdSettings />);
    await waitFor(() => expect(input().value).toBe('20'));

    for (const bad of ['', '0', '101', '-5', '2.5']) {
      await user.clear(input());
      if (bad !== '') await user.type(input(), bad);
      expect(screen.getByTestId('threshold-validation-error')).toBeTruthy();
      expect(saveBtn().disabled).toBe(true);
    }

    expect(updateThreshold).not.toHaveBeenCalled();
  });

  it('warns about high thresholds without blocking them', async () => {
    const user = userEvent.setup();
    render(<MassDeletionThresholdSettings />);
    await waitFor(() => expect(input().value).toBe('20'));

    expect(screen.queryByTestId('threshold-high-warning')).toBeNull();

    await user.click(screen.getByTestId('threshold-preset-100'));
    expect(screen.getByTestId('threshold-high-warning').textContent).toContain('100%');
    expect(saveBtn().disabled).toBe(false);

    await user.click(saveBtn());
    await waitFor(() => expect(updateThreshold).toHaveBeenCalledWith(100));
  });

  it('surfaces a save failure instead of claiming success', async () => {
    const user = userEvent.setup();
    updateThreshold.mockRejectedValue(new Error('maxDeletePercent must be between 1 and 100'));
    render(<MassDeletionThresholdSettings />);
    await waitFor(() => expect(input().value).toBe('20'));

    await user.click(screen.getByTestId('threshold-preset-50'));
    await user.click(saveBtn());

    expect(await screen.findByTestId('threshold-save-error')).toBeTruthy();
    expect(screen.queryByTestId('threshold-save-success')).toBeNull();
  });

  it('offers a retry when the current value cannot be loaded', async () => {
    const user = userEvent.setup();
    getThreshold.mockRejectedValueOnce(new Error('network down'));
    render(<MassDeletionThresholdSettings />);

    const failure = await screen.findByTestId('threshold-load-error');
    expect(failure.textContent).toContain('network down');

    await user.click(screen.getByText('Retry'));
    await waitFor(() => expect(input().value).toBe('20'));
  });

  it('reloads the saved value on remount rather than keeping a local edit', async () => {
    const user = userEvent.setup();
    const { unmount } = render(<MassDeletionThresholdSettings />);
    await waitFor(() => expect(input().value).toBe('20'));

    await user.click(screen.getByTestId('threshold-preset-50'));
    expect(input().value).toBe('50');
    unmount();

    getThreshold.mockResolvedValue(threshold({ maxDeletePercent: 20, source: 'default' }));
    render(<MassDeletionThresholdSettings />);
    await waitFor(() => expect(input().value).toBe('20'));
  });

  describe('while a sync is running or held', () => {
    const presets = () => [10, 20, 50, 100].map((p) => screen.getByTestId(`threshold-preset-${p}`) as HTMLButtonElement);

    it('disables every control when the backend reports a held sync', async () => {
      getThreshold.mockResolvedValue(
        threshold({ maxDeletePercent: 20, source: 'admin', locked: true, lockReason: 'held', activeSyncThreshold: 20 })
      );
      render(<MassDeletionThresholdSettings />);

      await waitFor(() => expect(input().value).toBe('20'));
      expect(screen.getByTestId('threshold-locked-notice').textContent).toContain('held');
      expect(slider().disabled).toBe(true);
      expect(input().disabled).toBe(true);
      expect(saveBtn().disabled).toBe(true);
      presets().forEach((button) => expect(button.disabled).toBe(true));
    });

    it('stays disabled after a reload, because the lock lives on the server', async () => {
      getThreshold.mockResolvedValue(
        threshold({ maxDeletePercent: 20, source: 'admin', locked: true, lockReason: 'held' })
      );
      const { unmount } = render(<MassDeletionThresholdSettings />);
      await waitFor(() => expect(input().disabled).toBe(true));
      unmount();

      // A fresh mount is what a page reload or a second tab looks like here.
      render(<MassDeletionThresholdSettings />);
      await waitFor(() => expect(input().disabled).toBe(true));
      expect(screen.getByTestId('threshold-locked-notice')).toBeTruthy();
    });

    it('disables immediately when this tab starts a sync, before the server is re-read', async () => {
      const { rerender } = render(<MassDeletionThresholdSettings syncActivity={false} />);
      await waitFor(() => expect(input().disabled).toBe(false));

      rerender(<MassDeletionThresholdSettings syncActivity={true} />);
      expect(input().disabled).toBe(true);
      expect(saveBtn().disabled).toBe(true);
      expect(screen.getByTestId('threshold-locked-notice')).toBeTruthy();
    });

    it('re-enables once the sync finishes and the backend reports no lock', async () => {
      getThreshold.mockResolvedValue(
        threshold({ maxDeletePercent: 20, source: 'admin', locked: true, lockReason: 'syncing' })
      );
      const { rerender } = render(<MassDeletionThresholdSettings syncActivity={true} />);
      await waitFor(() => expect(input().disabled).toBe(true));

      getThreshold.mockResolvedValue(threshold({ maxDeletePercent: 20, source: 'admin', locked: false }));
      rerender(<MassDeletionThresholdSettings syncActivity={false} />);

      await waitFor(() => expect(input().disabled).toBe(false));
      expect(screen.queryByTestId('threshold-locked-notice')).toBeNull();
    });

    it('locks itself when a save is refused because another tab started a sync', async () => {
      const user = userEvent.setup();
      render(<MassDeletionThresholdSettings />);
      await waitFor(() => expect(input().value).toBe('20'));

      const refusal: any = new Error('A synchronization is held for mass-deletion review.');
      refusal.statusCode = 409;
      refusal.apiError = { error: 'Deletion threshold is locked', code: 'threshold_locked' };
      updateThreshold.mockRejectedValue(refusal);
      // The next read reflects the lock the other tab created.
      getThreshold.mockResolvedValue(
        threshold({ maxDeletePercent: 20, source: 'admin', locked: true, lockReason: 'held' })
      );

      await user.click(screen.getByTestId('threshold-preset-100'));
      await user.click(saveBtn());

      expect(await screen.findByTestId('threshold-save-error')).toBeTruthy();
      await waitFor(() => expect(screen.getByTestId('threshold-locked-notice')).toBeTruthy());
      expect(input().disabled).toBe(true);
    });

    it('re-reads the server when the window regains focus', async () => {
      render(<MassDeletionThresholdSettings />);
      await waitFor(() => expect(input().disabled).toBe(false));

      getThreshold.mockResolvedValue(
        threshold({ maxDeletePercent: 20, source: 'admin', locked: true, lockReason: 'held' })
      );
      window.dispatchEvent(new Event('focus'));

      await waitFor(() => expect(input().disabled).toBe(true));
    });
  });
});
