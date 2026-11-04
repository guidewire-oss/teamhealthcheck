import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, act, fireEvent } from '@testing-library/react';

const getSchedule = vi.fn();
const putSchedule = vi.fn();

vi.mock('@/lib/api/admin', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api/admin')>();
  return {
    ...actual,
    getAutoSyncSchedule: (...a: any[]) => getSchedule(...a),
    updateAutoSyncSchedule: (...a: any[]) => putSchedule(...a),
  };
});

import AutoSyncSettings, { AUTO_SYNC_POLL_MS, AUTO_SYNC_ACTIVE_POLL_MS } from '../AutoSyncSettings';

const make = (over: Record<string, any> = {}) => ({
  enabled: false, frequency: 'DAILY', description: 'Daily at 02:00 UTC', timezone: 'UTC',
  nextRunAt: null, lastAttempt: null, lastSuccessAt: null, ...over,
});

const flush = () => act(async () => { await vi.advanceTimersByTimeAsync(0); });
const advance = (ms: number) => act(async () => { await vi.advanceTimersByTimeAsync(ms); });
const text = (id: string) => screen.getByTestId(id).textContent;

const renderWith = async (schedule: any) => {
  getSchedule.mockResolvedValue(schedule);
  render(<AutoSyncSettings />);
  await flush();
};

beforeEach(() => { vi.clearAllMocks(); vi.useFakeTimers(); });
afterEach(() => vi.useRealTimers());

describe('AutoSyncSettings', () => {
  it('shows a loading state, then the disabled / never-synced state', async () => {
    let resolve!: (v: any) => void;
    getSchedule.mockReturnValue(new Promise((r) => { resolve = r; }));
    render(<AutoSyncSettings />);
    expect(screen.getByTestId('auto-sync-loading')).toBeInTheDocument();
    resolve(make());
    await flush();
    expect(screen.getByTestId('auto-sync-toggle')).not.toBeChecked();
    expect(text('auto-sync-next')).toBe('Auto-sync disabled');
    expect(text('auto-sync-last-attempt')).toBe('Never synced automatically');
    expect(text('auto-sync-last-success')).toBe('Never synced automatically');
    expect(screen.getByTestId('auto-sync-save')).toBeDisabled();
  });

  it('displays the schedule with its timezone and the next run in UTC', async () => {
    await renderWith(make({ enabled: true, frequency: 'WEEKLY', description: 'Weekly on Monday at 02:00 UTC', nextRunAt: '2026-11-09T02:00:00Z' }));
    expect(text('auto-sync-schedule')).toBe('Weekly on Monday at 02:00 UTC (UTC)');
    expect(text('auto-sync-next')).toBe('9 Nov 2026, 02:00 UTC');
    expect(screen.getByTestId('auto-sync-toggle')).toBeChecked();
    expect(screen.getByTestId('auto-sync-frequency')).toHaveValue('WEEKLY');
  });

  it('saves the chosen enablement and frequency, then shows the returned next run', async () => {
    await renderWith(make());
    putSchedule.mockResolvedValue(make({ enabled: true, frequency: 'MONTHLY', description: 'Monthly on the 1st at 02:00 UTC', nextRunAt: '2026-12-01T02:00:00Z' }));
    fireEvent.click(screen.getByTestId('auto-sync-toggle'));
    fireEvent.change(screen.getByTestId('auto-sync-frequency'), { target: { value: 'MONTHLY' } });
    expect(screen.getByTestId('auto-sync-save')).toBeEnabled();
    fireEvent.click(screen.getByTestId('auto-sync-save'));
    await flush();
    expect(putSchedule).toHaveBeenCalledWith(true, 'MONTHLY');
    expect(text('auto-sync-next')).toBe('1 Dec 2026, 02:00 UTC');
    expect(screen.getByTestId('auto-sync-saved')).toBeInTheDocument();
    expect(screen.getByTestId('auto-sync-save')).toBeDisabled();
  });

  it('shows a safe error and keeps the edit when saving fails', async () => {
    await renderWith(make());
    putSchedule.mockRejectedValue(new Error('Invalid frequency'));
    fireEvent.click(screen.getByTestId('auto-sync-toggle'));
    fireEvent.click(screen.getByTestId('auto-sync-save'));
    await flush();
    expect(text('auto-sync-save-error')).toBe('Invalid frequency');
    expect(screen.getByTestId('auto-sync-toggle')).toBeChecked();
    expect(screen.getByTestId('auto-sync-save')).toBeEnabled();
  });

  it('disabling shows no next run', async () => {
    await renderWith(make({ enabled: true, nextRunAt: '2026-11-05T02:00:00Z' }));
    putSchedule.mockResolvedValue(make({ enabled: false, nextRunAt: null }));
    fireEvent.click(screen.getByTestId('auto-sync-toggle'));
    fireEvent.click(screen.getByTestId('auto-sync-save'));
    await flush();
    expect(putSchedule).toHaveBeenCalledWith(false, 'DAILY');
    expect(text('auto-sync-next')).toBe('Auto-sync disabled');
  });

  it('keeps last success separate from a failed latest attempt', async () => {
    await renderWith(make({
      enabled: true, nextRunAt: '2026-11-07T02:00:00Z',
      lastAttempt: { runId: 'r2', status: 'FAILED', startedAt: '2026-11-06T02:00:00Z', completedAt: '2026-11-06T02:00:05Z', errorMessage: 'The organization provider could not be reached.' },
      lastSuccessAt: '2026-11-05T02:00:04Z',
    }));
    expect(text('auto-sync-last-attempt')).toBe('6 Nov 2026, 02:00 UTC - Failed');
    expect(text('auto-sync-last-error')).toContain('could not be reached');
    expect(text('auto-sync-last-success')).toBe('5 Nov 2026, 02:00 UTC');
  });

  it('refreshes status automatically and polls faster while a run is active', async () => {
    await renderWith(make({
      enabled: true, nextRunAt: '2026-11-05T02:00:00Z',
      lastAttempt: { runId: 'r1', status: 'RUNNING', startedAt: '2026-11-05T02:00:00Z', completedAt: null, errorMessage: null },
    }));
    expect(text('auto-sync-last-attempt')).toContain('Running');
    getSchedule.mockResolvedValue(make({
      enabled: true, nextRunAt: '2026-11-06T02:00:00Z',
      lastAttempt: { runId: 'r1', status: 'SUCCESS', startedAt: '2026-11-05T02:00:00Z', completedAt: '2026-11-05T02:00:04Z', errorMessage: null },
      lastSuccessAt: '2026-11-05T02:00:04Z',
    }));
    await advance(AUTO_SYNC_ACTIVE_POLL_MS + 10);
    expect(text('auto-sync-last-attempt')).toContain('Succeeded');
    expect(text('auto-sync-last-success')).toBe('5 Nov 2026, 02:00 UTC');
    expect(text('auto-sync-next')).toBe('6 Nov 2026, 02:00 UTC');

    const calls = getSchedule.mock.calls.length;
    await advance(AUTO_SYNC_ACTIVE_POLL_MS + 10); // idle now: slow cadence
    expect(getSchedule.mock.calls.length).toBe(calls);
    await advance(AUTO_SYNC_POLL_MS);
    expect(getSchedule.mock.calls.length).toBeGreaterThan(calls);
  });

  it('does not clobber unsaved edits on a background refresh', async () => {
    await renderWith(make());
    fireEvent.click(screen.getByTestId('auto-sync-toggle'));
    await advance(AUTO_SYNC_POLL_MS + 10);
    expect(screen.getByTestId('auto-sync-toggle')).toBeChecked();
  });

  it('shows a load error without crashing', async () => {
    getSchedule.mockRejectedValue(new Error('Forbidden'));
    render(<AutoSyncSettings />);
    await flush();
    expect(text('auto-sync-load-error')).toBe('Forbidden');
  });
});
