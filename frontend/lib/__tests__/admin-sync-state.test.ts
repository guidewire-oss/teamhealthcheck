import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import {
  acquireSyncLease,
  clearSyncState,
  isStale,
  msUntilStale,
  onSyncStateChange,
  readSyncState,
  refreshSyncLease,
  startSyncHeartbeat,
  SYNC_HEARTBEAT_MS,
  SYNC_STALE_TIMEOUT_MS,
  type AdminSyncState,
} from '../admin-sync-state';

const KEY = 'adminSyncState';

function seed(state: Partial<AdminSyncState> & { requestId: string }): void {
  const now = Date.now();
  localStorage.setItem(
    KEY,
    JSON.stringify({ status: 'in_progress', startedAt: now, updatedAt: now, ...state })
  );
}

/** A minimal exclusive, FIFO Web Locks stand-in, enough to serialize critical sections. */
function installWebLocks() {
  let tail: Promise<unknown> = Promise.resolve();
  const request = vi.fn((_name: string, callback: () => unknown) => {
    const run = tail.then(() => callback());
    tail = run.catch(() => undefined);
    return run;
  });
  Object.defineProperty(navigator, 'locks', { configurable: true, value: { request } });
  return request;
}

function removeWebLocks() {
  delete (navigator as unknown as { locks?: unknown }).locks;
}

function stateAt(startedAt: number): AdminSyncState {
  return { status: 'in_progress', startedAt, updatedAt: startedAt, requestId: 'r' };
}

describe('isStale', () => {
  const now = 1_700_000_000_000;

  it('is not stale just under the timeout', () => {
    expect(isStale(stateAt(now - (SYNC_STALE_TIMEOUT_MS - 1)), now)).toBe(false);
  });

  it('is stale just past the timeout', () => {
    expect(isStale(stateAt(now - (SYNC_STALE_TIMEOUT_MS + 1)), now)).toBe(true);
  });

  it('treats a future startedAt as stale rather than never-expiring', () => {
    expect(isStale(stateAt(now + 60_000), now)).toBe(true);
  });

  it('treats NaN startedAt as stale', () => {
    expect(isStale(stateAt(NaN), now)).toBe(true);
  });

  it('treats Infinity startedAt as stale', () => {
    expect(isStale(stateAt(Infinity), now)).toBe(true);
  });

  it('treats -Infinity startedAt as stale', () => {
    expect(isStale(stateAt(-Infinity), now)).toBe(true);
  });
});

describe('isStale heartbeat semantics', () => {
  const now = 1_700_000_000_000;

  it('measures age from the last heartbeat, not from startedAt', () => {
    const state: AdminSyncState = {
      status: 'in_progress',
      startedAt: now - 10 * SYNC_STALE_TIMEOUT_MS,
      updatedAt: now - 1_000,
      requestId: 'r',
    };
    expect(isStale(state, now)).toBe(false);
  });

  it('is stale once the heartbeat stops advancing', () => {
    const state: AdminSyncState = { status: 'in_progress', startedAt: now - 5_000, updatedAt: now - (SYNC_STALE_TIMEOUT_MS + 1), requestId: 'r' };
    expect(isStale(state, now)).toBe(true);
  });

  it('treats a non-finite or future updatedAt as stale', () => {
    expect(isStale({ status: 'in_progress', startedAt: now - 1, updatedAt: NaN, requestId: 'r' }, now)).toBe(true);
    expect(isStale({ status: 'in_progress', startedAt: now - 1, updatedAt: now + 5_000, requestId: 'r' }, now)).toBe(true);
  });

  it('msUntilStale counts down from the last heartbeat', () => {
    const state: AdminSyncState = { status: 'in_progress', startedAt: now - 30_000, updatedAt: now - 20_000, requestId: 'r' };
    expect(msUntilStale(state, now)).toBe(SYNC_STALE_TIMEOUT_MS - 20_000);
    expect(msUntilStale({ ...state, updatedAt: now - 10 * SYNC_STALE_TIMEOUT_MS }, now)).toBe(0);
  });
});

describe.each([
  ['with Web Locks', installWebLocks],
  ['without Web Locks (best-effort fallback)', removeWebLocks],
])('lease protocol %s', (_label, setup) => {
  beforeEach(() => {
    localStorage.clear();
    setup();
  });
  afterEach(() => {
    removeWebLocks();
    vi.useRealTimers();
  });

  it('acquires the slot when nothing is persisted', async () => {
    const id = await acquireSyncLease();
    expect(id).toEqual(expect.any(String));
    expect(readSyncState()).toMatchObject({ status: 'in_progress', requestId: id });
  });

  it('refuses a second acquire while a live sync holds the lease', async () => {
    const first = await acquireSyncLease();
    const second = await acquireSyncLease();
    expect(second).toBeNull();
    expect(readSyncState()?.requestId).toBe(first);
  });

  it('reclaims a finished, stale, or corrupt record', async () => {
    seed({ requestId: 'done', status: 'completed' });
    expect(await acquireSyncLease()).not.toBeNull();

    seed({ requestId: 'old', startedAt: Date.now() - 10 * SYNC_STALE_TIMEOUT_MS, updatedAt: Date.now() - 10 * SYNC_STALE_TIMEOUT_MS });
    expect(await acquireSyncLease()).not.toBeNull();

    localStorage.setItem(KEY, '{not json');
    expect(await acquireSyncLease()).not.toBeNull();
  });

  it('clears the record when the requestId still matches', async () => {
    const id = (await acquireSyncLease())!;
    await clearSyncState(id);
    expect(readSyncState()).toBeNull();
  });

  it('does not clear a newer record a different request now owns', async () => {
    seed({ requestId: 'tab-b' });
    await clearSyncState('tab-a');
    expect(readSyncState()?.requestId).toBe('tab-b');
  });

  it('is a no-op when nothing is persisted', async () => {
    await expect(clearSyncState('some-request-id')).resolves.toBeUndefined();
    expect(readSyncState()).toBeNull();
  });

  it('removes unreadable garbage instead of leaving the slot wedged', async () => {
    localStorage.setItem(KEY, 'garbage');
    await clearSyncState('anything');
    expect(localStorage.getItem(KEY)).toBeNull();
  });

  it('refresh advances updatedAt only on the owned record', async () => {
    vi.useFakeTimers();
    const id = (await acquireSyncLease())!;
    const startedAt = readSyncState()!.startedAt;
    vi.advanceTimersByTime(5_000);

    expect(await refreshSyncLease(id)).toBe(true);
    expect(readSyncState()).toMatchObject({ requestId: id, startedAt, updatedAt: startedAt + 5_000 });

    seed({ requestId: 'someone-else' });
    expect(await refreshSyncLease(id)).toBe(false);
    expect(readSyncState()?.requestId).toBe('someone-else');
  });

  it('refresh restores the record if another tab\'s stale recovery cleared it mid-sync', async () => {
    const id = (await acquireSyncLease())!;
    localStorage.removeItem(KEY);
    expect(await refreshSyncLease(id)).toBe(true);
    expect(readSyncState()?.requestId).toBe(id);
  });

  it('refresh skips the write when shouldWrite says the sync has settled', async () => {
    const id = (await acquireSyncLease())!;
    await clearSyncState(id);
    expect(await refreshSyncLease(id, () => false)).toBe(true);
    expect(readSyncState()).toBeNull();
  });

  it('degrades to a usable requestId when localStorage throws', async () => {
    const spy = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('denied');
    });
    await expect(acquireSyncLease()).resolves.toEqual(expect.any(String));
    await expect(clearSyncState('x')).resolves.toBeUndefined();
    await expect(refreshSyncLease('x')).resolves.toBe(true);
    spy.mockRestore();
  });
});

describe('cross-tab interleaving (Web Locks)', () => {
  beforeEach(() => {
    localStorage.clear();
    installWebLocks();
  });
  afterEach(removeWebLocks);

  // Tab C found tab A's abandoned record and is clearing it. Tab B claims the
  // slot at exactly the moment C has finished its ownership check but not yet
  // removed anything. Unserialized, C's removal would delete B's brand-new
  // record. Under the lock, B waits for C's critical section to finish.
  it("a newer request written after the old request's ownership check is not erased by its cleanup", async () => {
    const old = Date.now() - 10 * SYNC_STALE_TIMEOUT_MS;
    seed({ requestId: 'tab-a-abandoned', startedAt: old, updatedAt: old });

    let tabB: Promise<string | null> | undefined;
    const realGetItem = Storage.prototype.getItem;
    const spy = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(function (this: Storage, key: string) {
      const value = realGetItem.call(this, key);
      if (key === KEY && !tabB) {
        tabB = acquireSyncLease(); // the competing tab fires right after C's read
      }
      return value;
    });

    await clearSyncState('tab-a-abandoned'); // tab C
    spy.mockRestore();
    const tabBId = await tabB!;

    expect(tabBId).not.toBeNull();
    expect(readSyncState()?.requestId).toBe(tabBId);
  });

  it('an older request can no longer overwrite or clear a newer lease', async () => {
    const older = (await acquireSyncLease())!;
    // The older request stalls past the timeout; a newer one takes over.
    const t = Date.now() - 10 * SYNC_STALE_TIMEOUT_MS;
    seed({ requestId: older, startedAt: t, updatedAt: t });
    const newer = (await acquireSyncLease())!;
    expect(newer).not.toBe(older);

    expect(await refreshSyncLease(older)).toBe(false);
    await clearSyncState(older);

    expect(readSyncState()?.requestId).toBe(newer);
  });

  it('serializes every critical section through one named lock', async () => {
    const request = installWebLocks();
    await acquireSyncLease();
    await refreshSyncLease('x');
    await clearSyncState('x');
    expect(request).toHaveBeenCalledTimes(3);
    expect(new Set(request.mock.calls.map((c) => c[0])).size).toBe(1);
  });

  it('falls back to running unserialized when the lock request itself rejects', async () => {
    Object.defineProperty(navigator, 'locks', {
      configurable: true,
      value: { request: () => Promise.reject(new DOMException('opaque origin', 'SecurityError')) },
    });
    await expect(acquireSyncLease()).resolves.toEqual(expect.any(String));
  });
});

describe('startSyncHeartbeat', () => {
  beforeEach(() => {
    localStorage.clear();
    removeWebLocks();
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('keeps a sync alive far past SYNC_STALE_TIMEOUT_MS', async () => {
    const id = (await acquireSyncLease())!;
    const stop = startSyncHeartbeat(id);

    await vi.advanceTimersByTimeAsync(5 * SYNC_STALE_TIMEOUT_MS);

    const state = readSyncState()!;
    expect(state.requestId).toBe(id);
    expect(isStale(state)).toBe(false);
    // ...so another tab is still refused.
    expect(await acquireSyncLease()).toBeNull();
    stop();
  });

  it('without a heartbeat the same record is recovered after the timeout (crashed tab)', async () => {
    const id = (await acquireSyncLease())!;

    await vi.advanceTimersByTimeAsync(SYNC_STALE_TIMEOUT_MS + 1);

    expect(isStale(readSyncState()!)).toBe(true);
    const next = await acquireSyncLease();
    expect(next).not.toBeNull();
    expect(next).not.toBe(id);
  });

  it('stops writing once stopped (settle, failure, or unmount path)', async () => {
    const id = (await acquireSyncLease())!;
    const stop = startSyncHeartbeat(id);
    await vi.advanceTimersByTimeAsync(SYNC_HEARTBEAT_MS * 2);

    stop();
    const writes = vi.spyOn(Storage.prototype, 'setItem');
    await vi.advanceTimersByTimeAsync(SYNC_HEARTBEAT_MS * 5);

    expect(writes).not.toHaveBeenCalled();
    expect(vi.getTimerCount()).toBe(0);
    writes.mockRestore();
  });

  it('never resurrects a record that settlement already cleared', async () => {
    const id = (await acquireSyncLease())!;
    const stop = startSyncHeartbeat(id);
    await vi.advanceTimersByTimeAsync(SYNC_HEARTBEAT_MS);

    stop();
    await clearSyncState(id);
    await vi.advanceTimersByTimeAsync(SYNC_HEARTBEAT_MS * 3);

    expect(readSyncState()).toBeNull();
  });

  it('runs a single interval per requestId even if started twice (Strict Mode)', async () => {
    const id = (await acquireSyncLease())!;
    const baseline = vi.getTimerCount();
    const stopA = startSyncHeartbeat(id);
    const stopB = startSyncHeartbeat(id);

    expect(vi.getTimerCount()).toBe(baseline + 1);
    stopB();
    expect(vi.getTimerCount()).toBe(baseline);
    stopA(); // idempotent
    expect(vi.getTimerCount()).toBe(baseline);
  });

  it('stops and reports when another request has taken the lease', async () => {
    const id = (await acquireSyncLease())!;
    const onLost = vi.fn();
    startSyncHeartbeat(id, onLost);

    seed({ requestId: 'newer-request' });
    await vi.advanceTimersByTimeAsync(SYNC_HEARTBEAT_MS);

    expect(onLost).toHaveBeenCalledTimes(1);
    expect(vi.getTimerCount()).toBe(0);
    expect(readSyncState()?.requestId).toBe('newer-request');
  });
});

describe('onSyncStateChange', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('notifies with the new state when the record is written', () => {
    const seen: (AdminSyncState | null)[] = [];
    const unsubscribe = onSyncStateChange((state) => seen.push(state));

    window.dispatchEvent(
      new StorageEvent('storage', {
        key: 'adminSyncState',
        newValue: JSON.stringify({ status: 'in_progress', startedAt: Date.now(), updatedAt: Date.now(), requestId: 'r1' }),
      })
    );

    expect(seen).toHaveLength(1);
    expect(seen[0]?.status).toBe('in_progress');
    unsubscribe();
  });

  it('notifies with null when the record is removed', () => {
    const seen: (AdminSyncState | null)[] = [];
    const unsubscribe = onSyncStateChange((state) => seen.push(state));

    window.dispatchEvent(new StorageEvent('storage', { key: 'adminSyncState', newValue: null }));

    expect(seen).toEqual([null]);
    unsubscribe();
  });

  it('ignores changes to unrelated storage keys', () => {
    const seen: (AdminSyncState | null)[] = [];
    const unsubscribe = onSyncStateChange((state) => seen.push(state));

    window.dispatchEvent(new StorageEvent('storage', { key: 'someOtherKey', newValue: 'x' }));

    expect(seen).toHaveLength(0);
    unsubscribe();
  });

  it('stops notifying after unsubscribe', () => {
    const seen: (AdminSyncState | null)[] = [];
    const unsubscribe = onSyncStateChange((state) => seen.push(state));
    unsubscribe();

    window.dispatchEvent(new StorageEvent('storage', { key: 'adminSyncState', newValue: null }));

    expect(seen).toHaveLength(0);
  });
});
