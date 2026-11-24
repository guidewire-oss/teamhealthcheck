import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, waitFor, act } from '@testing-library/react';

const listManagerTeamsActionSummary = vi.fn();
vi.mock('@/lib/api/action-items', () => ({
  listManagerTeamsActionSummary: (...a: any[]) => listManagerTeamsActionSummary(...a),
}));

import { useManagerActionSummary } from '../useManagerActionSummary';

const row = (teamId: string) => ({
  teamId,
  teamName: teamId,
  openCount: 0,
  statusCounts: { open: 0, onHold: 0, inProgress: 0, completed: 0 },
});

function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe('useManagerActionSummary', () => {
  beforeEach(() => {
    listManagerTeamsActionSummary.mockReset();
  });

  it('does not fetch while disabled or without a manager id', () => {
    renderHook(() => useManagerActionSummary('m1', false));
    renderHook(() => useManagerActionSummary(undefined, true));
    expect(listManagerTeamsActionSummary).not.toHaveBeenCalled();
  });

  it('reports loading on the very first render, before any request settles', () => {
    listManagerTeamsActionSummary.mockReturnValue(new Promise(() => {}));
    const { result } = renderHook(() => useManagerActionSummary('m1', true));
    expect(result.current).toEqual({ summary: [], loading: true, error: false });
  });

  it('is not loading while disabled', () => {
    const { result } = renderHook(() => useManagerActionSummary('m1', false));
    expect(result.current.loading).toBe(false);
  });

  it('loads the summary when enabled', async () => {
    listManagerTeamsActionSummary.mockResolvedValue([row('t1')]);
    const { result } = renderHook(() => useManagerActionSummary('m1', true));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current).toEqual({ summary: [row('t1')], loading: false, error: false });
  });

  it('reports an error and an empty summary when the request fails', async () => {
    listManagerTeamsActionSummary.mockRejectedValue(new Error('boom'));
    const { result } = renderHook(() => useManagerActionSummary('m1', true));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current).toEqual({ summary: [], loading: false, error: true });
  });

  it('ignores a stale failure that arrives after a newer success', async () => {
    const first = deferred<any[]>();
    const second = deferred<any[]>();
    listManagerTeamsActionSummary.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);

    const { result, rerender } = renderHook(({ id }) => useManagerActionSummary(id, true), {
      initialProps: { id: 'm1' },
    });
    rerender({ id: 'm2' }); // newer request supersedes the first

    await act(async () => second.resolve([row('t2')]));
    await waitFor(() => expect(result.current.summary).toEqual([row('t2')]));

    await act(async () => first.reject(new Error('late failure')));
    expect(result.current).toEqual({ summary: [row('t2')], loading: false, error: false });
  });

  it('ignores a stale success that arrives after a newer failure', async () => {
    const first = deferred<any[]>();
    const second = deferred<any[]>();
    listManagerTeamsActionSummary.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);

    const { result, rerender } = renderHook(({ id }) => useManagerActionSummary(id, true), {
      initialProps: { id: 'm1' },
    });
    rerender({ id: 'm2' });

    await act(async () => second.reject(new Error('boom')));
    await act(async () => first.resolve([row('old')]));
    expect(result.current).toEqual({ summary: [], loading: false, error: true });
  });

  it('refetches, showing loading rather than stale counts, when re-enabled after leaving the tab', async () => {
    listManagerTeamsActionSummary.mockResolvedValueOnce([row('t1')]);
    const { result, rerender } = renderHook(({ on }) => useManagerActionSummary('m1', on), {
      initialProps: { on: true },
    });
    await waitFor(() => expect(result.current.summary).toEqual([row('t1')]));

    rerender({ on: false });
    await waitFor(() => expect(result.current.summary).toEqual([]));

    listManagerTeamsActionSummary.mockReturnValueOnce(new Promise(() => {}));
    rerender({ on: true });
    expect(result.current.loading).toBe(true);
    expect(listManagerTeamsActionSummary).toHaveBeenCalledTimes(2);
  });
});
