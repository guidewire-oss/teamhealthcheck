import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';

const getLatest = vi.fn();
const getRun = vi.fn();
const startSync = vi.fn();
vi.mock('@/lib/api/admin', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api/admin')>();
  return {
    ...actual,
    getLatestSyncRun: (...a: any[]) => getLatest(...a),
    getSyncRun: (...a: any[]) => getRun(...a),
    syncOrganizationProvider: (...a: any[]) => startSync(...a),
  };
});

import { useSyncRun, SYNC_POLL_INTERVAL_MS } from '../useSyncRun';

const run = (over: Record<string, any> = {}) => ({
  runId: 'r1', status: 'PENDING', stage: 'QUEUED', ...over,
});
const deferred = <T,>() => {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((res) => { resolve = res; });
  return { promise, resolve };
};
const tick = (ms = 0) => act(async () => { await vi.advanceTimersByTimeAsync(ms); });

beforeEach(() => { vi.clearAllMocks(); vi.useFakeTimers(); getLatest.mockResolvedValue(null); });
afterEach(() => vi.useRealTimers());

describe('useSyncRun', () => {
  it('stops polling the moment the run reaches SUCCESS', async () => {
    getLatest.mockResolvedValue(run({ status: 'RUNNING', stage: 'UPSERTING_USERS' }));
    getRun.mockResolvedValue(run({ status: 'SUCCESS', stage: 'COMPLETED' }));
    const { result } = renderHook(() => useSyncRun());
    await tick();
    await tick(SYNC_POLL_INTERVAL_MS + 5);
    expect(result.current.run?.status).toBe('SUCCESS');
    const calls = getRun.mock.calls.length;
    await tick(SYNC_POLL_INTERVAL_MS * 5);
    expect(getRun.mock.calls.length).toBe(calls);
  });

  it('stops polling the moment the run reaches FAILED', async () => {
    getLatest.mockResolvedValue(run({ status: 'RUNNING' }));
    getRun.mockResolvedValue(run({ status: 'FAILED', stage: 'FAILED', failedStage: 'FETCHING' }));
    const { result } = renderHook(() => useSyncRun());
    await tick();
    await tick(SYNC_POLL_INTERVAL_MS + 5);
    expect(result.current.run?.status).toBe('FAILED');
    const calls = getRun.mock.calls.length;
    await tick(SYNC_POLL_INTERVAL_MS * 5);
    expect(getRun.mock.calls.length).toBe(calls);
  });

  it('keeps polling while a run is genuinely still active', async () => {
    getLatest.mockResolvedValue(run({ status: 'RUNNING' }));
    getRun.mockResolvedValue(run({ status: 'RUNNING', stage: 'UPSERTING_TEAMS' }));
    renderHook(() => useSyncRun());
    await tick();
    await tick(SYNC_POLL_INTERVAL_MS + 5);
    await tick(SYNC_POLL_INTERVAL_MS + 5);
    await tick(SYNC_POLL_INTERVAL_MS + 5);
    expect(getRun.mock.calls.length).toBeGreaterThanOrEqual(3);
  });

  it('a slow poll answering RUNNING after SUCCESS was applied cannot bring RUNNING back', async () => {
    getLatest.mockResolvedValue(run({ status: 'RUNNING' }));
    const slow = deferred<any>();
    getRun.mockReturnValueOnce(slow.promise); // first poll hangs
    const { result } = renderHook(() => useSyncRun());
    await tick();
    await tick(SYNC_POLL_INTERVAL_MS + 5); // poll #1 is in flight (and slow)

    // Meanwhile another path observes the terminal state.
    startSync.mockResolvedValue(run({ status: 'SUCCESS', stage: 'COMPLETED' }));
    await act(async () => { await result.current.start(); });
    expect(result.current.run?.status).toBe('SUCCESS');

    // The slow, stale RUNNING answer finally lands.
    await act(async () => { slow.resolve(run({ status: 'RUNNING', stage: 'UPSERTING_USERS' })); });
    await tick(10);
    expect(result.current.run?.status).toBe('SUCCESS');
  });

  it('a poll answer for a previous run cannot overwrite the run now being followed', async () => {
    getLatest.mockResolvedValue(run({ runId: 'old', status: 'RUNNING' }));
    const slow = deferred<any>();
    getRun.mockReturnValueOnce(slow.promise);
    const { result } = renderHook(() => useSyncRun());
    await tick();
    await tick(SYNC_POLL_INTERVAL_MS + 5);

    startSync.mockResolvedValue(run({ runId: 'new', status: 'PENDING' }));
    await act(async () => { await result.current.start(); });
    expect(result.current.run?.runId).toBe('new');

    await act(async () => { slow.resolve(run({ runId: 'old', status: 'SUCCESS' })); });
    await tick(10);
    expect(result.current.run?.runId).toBe('new');
    expect(result.current.run?.status).toBe('PENDING');
  });

  it('follows a live run the backend started when the start request itself failed', async () => {
    startSync.mockRejectedValue(new Error('network dropped'));
    const { result } = renderHook(() => useSyncRun());
    await tick();
    getLatest.mockResolvedValue(run({ runId: 'lost-reply', status: 'RUNNING', stage: 'UPSERTING_USERS' }));
    await act(async () => { await result.current.start(); });
    expect(result.current.run?.runId).toBe('lost-reply');
    expect(result.current.startError).toBeNull();
    expect(result.current.starting).toBe(false);
  });

  it('reports the start failure when no run is active after it', async () => {
    startSync.mockRejectedValue(new Error('network dropped'));
    const { result } = renderHook(() => useSyncRun());
    await tick();
    getLatest.mockResolvedValue(run({ runId: 'old', status: 'SUCCESS', stage: 'COMPLETED' }));
    await act(async () => { await result.current.start(); });
    expect(result.current.startError).toBe('network dropped');
    expect(result.current.run?.runId).not.toBe('lost-reply');
  });

  it('a late "latest" load cannot replace a run the admin already started', async () => {
    const late = deferred<any>();
    getLatest.mockReturnValue(late.promise);
    startSync.mockResolvedValue(run({ runId: 'mine', status: 'PENDING' }));
    const { result } = renderHook(() => useSyncRun());
    await act(async () => { await result.current.start(); });
    await act(async () => { late.resolve(run({ runId: 'previous', status: 'SUCCESS' })); });
    expect(result.current.run?.runId).toBe('mine');
  });

  it('a network error never changes the run or reports success, and polling recovers', async () => {
    getLatest.mockResolvedValue(run({ status: 'RUNNING' }));
    getRun.mockRejectedValueOnce(new Error('offline')).mockResolvedValue(run({ status: 'SUCCESS', stage: 'COMPLETED' }));
    const { result } = renderHook(() => useSyncRun());
    await tick();
    await tick(SYNC_POLL_INTERVAL_MS + 5);
    expect(result.current.pollError).toMatch(/offline/);
    expect(result.current.run?.status).toBe('RUNNING');
    await tick(SYNC_POLL_INTERVAL_MS + 5);
    expect(result.current.run?.status).toBe('SUCCESS');
    expect(result.current.pollError).toBeNull();
  });
});
