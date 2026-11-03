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
  takeAbandoned,
  onSyncLeaseExpired,
  getActiveRequestId,
  getActiveSyncPromise,
  setActiveRequestId,
  setActiveSyncPromise,
  SYNC_HEARTBEAT_MS,
  SYNC_MAX_LEASE_MS,
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

  // Each case must do more than return an id: the slot another tab reads has
  // to now hold that id, in progress, replacing whatever was there.
  const expectClaimed = (requestId: string | null, replaced: string | null) => {
    expect(requestId).not.toBeNull();
    expect(requestId).not.toBe(replaced);
    const state = readSyncState();
    expect(state?.requestId).toBe(requestId);
    expect(state?.status).toBe('in_progress');
  };

  it('reclaims a finished record and replaces it in the slot', async () => {
    seed({ requestId: 'done', status: 'completed' });

    expectClaimed(await acquireSyncLease(), 'done');
  });

  it('reclaims a stale record and replaces it in the slot', async () => {
    const old = Date.now() - 10 * SYNC_STALE_TIMEOUT_MS;
    seed({ requestId: 'old', startedAt: old, updatedAt: old });

    expectClaimed(await acquireSyncLease(), 'old');
  });

  it('reclaims a failed record and replaces it in the slot', async () => {
    seed({ requestId: 'failed-run', status: 'failed' });

    expectClaimed(await acquireSyncLease(), 'failed-run');
  });

  it('reclaims a corrupt record and replaces it with a readable one', async () => {
    localStorage.setItem(KEY, '{not json');
    expect(readSyncState()).toBeNull(); // unreadable before

    expectClaimed(await acquireSyncLease(), null);
  });

  it('the reclaimed lease then excludes other tabs', async () => {
    seed({ requestId: 'done', status: 'completed' });
    const mine = await acquireSyncLease();

    expect(await acquireSyncLease()).toBeNull();
    expect(readSyncState()?.requestId).toBe(mine);
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

  it('refresh reports a lost lease, and writes nothing, when the record has vanished', async () => {
    const id = (await acquireSyncLease())!;
    localStorage.removeItem(KEY);

    expect(await refreshSyncLease(id)).toBe(false);
    expect(readSyncState()).toBeNull(); // not resurrected
  });

  it('refresh reports a lost lease for an unreadable record and leaves it alone', async () => {
    const id = (await acquireSyncLease())!;
    localStorage.setItem(KEY, '{not json');

    expect(await refreshSyncLease(id)).toBe(false);
    expect(localStorage.getItem(KEY)).toBe('{not json');
  });

  it('refresh reports a lost lease when this request\'s own record is no longer in progress', async () => {
    const id = (await acquireSyncLease())!;
    seed({ requestId: id, status: 'completed' });

    expect(await refreshSyncLease(id)).toBe(false);
    expect(readSyncState()?.status).toBe('completed'); // not flipped back to in_progress
  });

  it('refresh preserves startedAt and every other field of the record it renews', async () => {
    vi.useFakeTimers();
    const id = (await acquireSyncLease())!;
    const before = readSyncState()!;
    vi.advanceTimersByTime(3_000);

    expect(await refreshSyncLease(id)).toBe(true);

    expect(readSyncState()).toEqual({ ...before, updatedAt: before.updatedAt + 3_000 });
  });

  it('an old request cannot resurrect its lease after a newer tab acquired and cleared the slot', async () => {
    vi.useFakeTimers();
    const old = (await acquireSyncLease())!; // tab A, its heartbeat is throttled
    vi.advanceTimersByTime(SYNC_STALE_TIMEOUT_MS + 1_000);

    const newer = (await acquireSyncLease())!; // tab B reclaims the stale lease
    expect(newer).not.toBe(old);
    await clearSyncState(newer); // ...and finishes, clearing the slot
    expect(readSyncState()).toBeNull();

    expect(await refreshSyncLease(old)).toBe(false); // A's late heartbeat
    expect(readSyncState()).toBeNull(); // slot stays free

    // So the next Sync Now is not blocked by A's dead lease.
    const next = await acquireSyncLease();
    expect(next).not.toBeNull();
    expect(readSyncState()?.requestId).toBe(next);
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

  it('runs a single heartbeat per requestId even if started twice (Strict Mode)', async () => {
    const id = (await acquireSyncLease())!;
    const baseline = vi.getTimerCount();
    const stopA = startSyncHeartbeat(id);
    const stopB = startSyncHeartbeat(id);

    // One heartbeat = one write interval plus one expiry timer, never doubled.
    expect(vi.getTimerCount()).toBe(baseline + 2);
    stopB();
    expect(vi.getTimerCount()).toBe(baseline);
    stopA(); // idempotent
    expect(vi.getTimerCount()).toBe(baseline);
  });

  it('stops and reports when the record vanishes, instead of recreating it', async () => {
    const id = (await acquireSyncLease())!;
    const onLost = vi.fn();
    const stopLater = startSyncHeartbeat(id, onLost);

    localStorage.removeItem(KEY);
    await vi.advanceTimersByTimeAsync(SYNC_HEARTBEAT_MS);

    expect(onLost).toHaveBeenCalledTimes(1);
    expect(readSyncState()).toBeNull();
    await vi.advanceTimersByTimeAsync(SYNC_HEARTBEAT_MS * 5);
    expect(readSyncState()).toBeNull(); // and it never comes back
    stopLater();
  });

  it('stops writing and reports when another request has taken the lease', async () => {
    const id = (await acquireSyncLease())!;
    const onLost = vi.fn();
    const stop = startSyncHeartbeat(id, onLost);

    seed({ requestId: 'newer-request' });
    await vi.advanceTimersByTimeAsync(SYNC_HEARTBEAT_MS);

    expect(onLost).toHaveBeenCalledTimes(1);
    const writes = vi.spyOn(Storage.prototype, 'setItem');
    await vi.advanceTimersByTimeAsync(SYNC_HEARTBEAT_MS * 5);
    expect(writes).not.toHaveBeenCalled(); // heartbeat no longer writes
    writes.mockRestore();
    expect(readSyncState()?.requestId).toBe('newer-request');
    stop();
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

describe('startSyncHeartbeat -- bounded lease for a request that never settles', () => {
  beforeEach(() => {
    localStorage.clear();
    removeWebLocks();
    vi.useFakeTimers();
    setActiveSyncPromise(null);
    setActiveRequestId(null);
  });
  afterEach(() => {
    vi.useRealTimers();
    setActiveSyncPromise(null);
    setActiveRequestId(null);
  });

  it('keeps refreshing right up to the bound', async () => {
    const id = (await acquireSyncLease())!;
    startSyncHeartbeat(id, undefined, undefined, 5 * SYNC_HEARTBEAT_MS);

    await vi.advanceTimersByTimeAsync(4 * SYNC_HEARTBEAT_MS);

    expect(isStale(readSyncState()!)).toBe(false);
    expect(await acquireSyncLease()).toBeNull();
  });

  it('at the bound it stops refreshing, releases the lease, and calls onExpired once', async () => {
    const id = (await acquireSyncLease())!;
    const onExpired = vi.fn();
    startSyncHeartbeat(id, undefined, onExpired, 5 * SYNC_HEARTBEAT_MS);

    await vi.advanceTimersByTimeAsync(5 * SYNC_HEARTBEAT_MS);

    expect(onExpired).toHaveBeenCalledTimes(1);
    expect(readSyncState()).toBeNull(); // released immediately, not left to age out

    const writes = vi.spyOn(Storage.prototype, 'setItem');
    await vi.advanceTimersByTimeAsync(10 * SYNC_HEARTBEAT_MS);
    expect(writes).not.toHaveBeenCalled(); // no heartbeat writes afterwards
    expect(onExpired).toHaveBeenCalledTimes(1);
    writes.mockRestore();
  });

  it('after the bound another tab can start a sync straight away', async () => {
    const id = (await acquireSyncLease())!;
    startSyncHeartbeat(id, undefined, undefined, 3 * SYNC_HEARTBEAT_MS);

    await vi.advanceTimersByTimeAsync(3 * SYNC_HEARTBEAT_MS);

    const next = await acquireSyncLease();
    expect(next).not.toBeNull();
    expect(next).not.toBe(id);
    expect(readSyncState()?.requestId).toBe(next);
  });

  it('defaults the bound to SYNC_MAX_LEASE_MS, well beyond one stale timeout', async () => {
    expect(SYNC_MAX_LEASE_MS).toBeGreaterThan(SYNC_STALE_TIMEOUT_MS);
    const id = (await acquireSyncLease())!;
    startSyncHeartbeat(id);

    await vi.advanceTimersByTimeAsync(SYNC_MAX_LEASE_MS - SYNC_HEARTBEAT_MS);
    expect(isStale(readSyncState()!)).toBe(false); // still held just under the default bound

    await vi.advanceTimersByTimeAsync(SYNC_HEARTBEAT_MS);
    expect(readSyncState()).toBeNull(); // released at the bound
  });

  it('expiry drops the active-request handles when they still name this request', async () => {
    const id = (await acquireSyncLease())!;
    setActiveRequestId(id);
    setActiveSyncPromise(new Promise(() => {}));
    startSyncHeartbeat(id, undefined, undefined, 2 * SYNC_HEARTBEAT_MS);

    await vi.advanceTimersByTimeAsync(2 * SYNC_HEARTBEAT_MS);

    expect(getActiveRequestId()).toBeNull();
    expect(getActiveSyncPromise()).toBeNull();
  });

  it('expiry leaves the handles alone when a newer request already owns them', async () => {
    const id = (await acquireSyncLease())!;
    startSyncHeartbeat(id, undefined, undefined, 2 * SYNC_HEARTBEAT_MS);
    const newerPromise = new Promise(() => {});
    setActiveRequestId('newer-request');
    setActiveSyncPromise(newerPromise);

    await vi.advanceTimersByTimeAsync(2 * SYNC_HEARTBEAT_MS);

    expect(getActiveRequestId()).toBe('newer-request');
    expect(getActiveSyncPromise()).toBe(newerPromise);
  });

  it('expiry never clears a newer request\'s lease record', async () => {
    const id = (await acquireSyncLease())!;
    startSyncHeartbeat(id, undefined, undefined, 2 * SYNC_HEARTBEAT_MS);
    seed({ requestId: 'newer-request' }); // another tab took the slot

    await vi.advanceTimersByTimeAsync(2 * SYNC_HEARTBEAT_MS);

    expect(readSyncState()?.requestId).toBe('newer-request');
  });

  it('marks the request abandoned exactly once, so its late settlement can be recognised', async () => {
    const id = (await acquireSyncLease())!;
    startSyncHeartbeat(id, undefined, undefined, SYNC_HEARTBEAT_MS);
    expect(takeAbandoned(id)).toBe(false); // not yet

    await vi.advanceTimersByTimeAsync(SYNC_HEARTBEAT_MS);

    expect(takeAbandoned(id)).toBe(true);
    expect(takeAbandoned(id)).toBe(false); // consumed
    expect(takeAbandoned(null)).toBe(false);
  });

  it('a request that settles in time is never marked abandoned', async () => {
    const id = (await acquireSyncLease())!;
    const stop = startSyncHeartbeat(id, undefined, undefined, 5 * SYNC_HEARTBEAT_MS);

    await vi.advanceTimersByTimeAsync(2 * SYNC_HEARTBEAT_MS);
    stop(); // settled
    await vi.advanceTimersByTimeAsync(10 * SYNC_HEARTBEAT_MS);

    expect(takeAbandoned(id)).toBe(false);
  });

  it('notifies expiry listeners, and stops notifying after unsubscribe', async () => {
    const listener = vi.fn();
    const unsubscribe = onSyncLeaseExpired(listener);
    const id = (await acquireSyncLease())!;
    startSyncHeartbeat(id, undefined, undefined, SYNC_HEARTBEAT_MS);

    await vi.advanceTimersByTimeAsync(SYNC_HEARTBEAT_MS);
    expect(listener).toHaveBeenCalledExactlyOnceWith(id);

    unsubscribe();
    const second = (await acquireSyncLease())!;
    startSyncHeartbeat(second, undefined, undefined, SYNC_HEARTBEAT_MS);
    await vi.advanceTimersByTimeAsync(SYNC_HEARTBEAT_MS);
    expect(listener).toHaveBeenCalledTimes(1);
  });

  it('the bound still applies after the lease was lost: a hung request is not unbounded', async () => {
    const id = (await acquireSyncLease())!;
    const onLost = vi.fn();
    const onExpired = vi.fn();
    startSyncHeartbeat(id, onLost, onExpired, 6 * SYNC_HEARTBEAT_MS);

    seed({ requestId: 'newer-request' });
    await vi.advanceTimersByTimeAsync(2 * SYNC_HEARTBEAT_MS);
    expect(onLost).toHaveBeenCalledTimes(1);
    expect(onExpired).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(4 * SYNC_HEARTBEAT_MS);
    expect(onExpired).toHaveBeenCalledTimes(1);
    expect(takeAbandoned(id)).toBe(true);
  });
});
