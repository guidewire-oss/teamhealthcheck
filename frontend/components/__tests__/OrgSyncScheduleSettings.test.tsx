import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor, act } from '@testing-library/react';

// The component reaches the backend only through these functions.
const getSchedule = vi.fn();
const updateSchedule = vi.fn();
const getLastRun = vi.fn();

vi.mock('@/lib/api/admin', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api/admin')>();
  return {
    ...actual,
    getOrgSyncSchedule: (...a: any[]) => getSchedule(...a),
    updateOrgSyncSchedule: (...a: any[]) => updateSchedule(...a),
    getOrgSyncLastRun: (...a: any[]) => getLastRun(...a),
  };
});

import OrgSyncScheduleSettings from '../OrgSyncScheduleSettings';
import type { OrgSyncSchedule, OrgSyncLastRun } from '@/lib/api/admin';

const POLL_INTERVAL_MS = 15000;

const schedule = (overrides: Partial<OrgSyncSchedule> = {}): OrgSyncSchedule => ({
  enabled: false,
  availableFrequencies: ['daily', 'weekly', 'monthly'],
  defaultFrequency: 'weekly',
  ...overrides,
});

const emptyLastRun: OrgSyncLastRun = {};

afterEach(() => {
  vi.useRealTimers();
});

describe('OrgSyncScheduleSettings polling', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getSchedule.mockResolvedValue(schedule());
    getLastRun.mockResolvedValue(emptyLastRun);
  });

  it('shows a newly completed/skipped result on the next poll without remounting', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });

    render(<OrgSyncScheduleSettings />);
    await waitFor(() => expect(screen.getByTestId('last-run-never')).toBeInTheDocument());
    expect(getLastRun).toHaveBeenCalledTimes(1);

    getLastRun.mockResolvedValue({
      lastSkip: { reason: 'manual_sync_running', at: '2026-09-23T18:09:22Z' },
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS);
    });

    await waitFor(() => expect(screen.getByTestId('last-run-skip')).toBeInTheDocument());
    expect(screen.getByTestId('last-run-skip').textContent).toContain('a manual sync was already running');
  });

  it('does not start an overlapping poll while the previous one is still in flight', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });

    let resolveSecondCall: (v: OrgSyncLastRun) => void = () => {};
    let callCount = 0;
    getLastRun.mockImplementation(() => {
      callCount++;
      if (callCount === 1) return Promise.resolve(emptyLastRun);
      return new Promise((resolve) => {
        resolveSecondCall = resolve;
      });
    });

    render(<OrgSyncScheduleSettings />);
    await waitFor(() => expect(getLastRun).toHaveBeenCalledTimes(1));

    // First poll tick starts a request that never resolves yet.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS);
    });
    expect(getLastRun).toHaveBeenCalledTimes(2);

    // A second tick fires while the first poll's request is still pending --
    // the in-flight guard must skip starting another one.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS);
    });
    expect(getLastRun).toHaveBeenCalledTimes(2);

    await act(async () => {
      resolveSecondCall(emptyLastRun);
      await Promise.resolve();
    });

    // Now that the in-flight poll has settled, the next tick may proceed.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS);
    });
    expect(getLastRun).toHaveBeenCalledTimes(3);
  });

  it('cleans up the poll interval on unmount', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });

    const { unmount } = render(<OrgSyncScheduleSettings />);
    await waitFor(() => expect(getLastRun).toHaveBeenCalledTimes(1));

    unmount();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 3);
    });

    // No further calls after unmount -- the interval was cleared.
    expect(getLastRun).toHaveBeenCalledTimes(1);
  });

  it('does not overwrite an unsaved schedule edit with a background poll', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    getSchedule.mockResolvedValue(schedule({ enabled: false }));

    render(<OrgSyncScheduleSettings />);
    await waitFor(() => expect(screen.getByTestId('schedule-enabled-toggle')).toBeInTheDocument());
    expect(getSchedule).toHaveBeenCalledTimes(1);

    const toggle = screen.getByTestId('schedule-enabled-toggle') as HTMLInputElement;
    await act(async () => {
      toggle.click();
    });
    expect(toggle.checked).toBe(true);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS);
    });

    // The schedule GET must not be re-issued while the toggle has an unsaved change.
    expect(getSchedule).toHaveBeenCalledTimes(1);
    expect((screen.getByTestId('schedule-enabled-toggle') as HTMLInputElement).checked).toBe(true);
  });
});
