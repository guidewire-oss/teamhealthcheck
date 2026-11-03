/**
 * Persists Sync Now state across tab switches, SPA navigation, and browser
 * tabs open to the same admin page at once.
 *
 * The record lives in one shared localStorage slot and is a lease:
 *
 *  - acquireSyncLease claims the slot only when it is free (empty, finished,
 *    or stale); a tab that finds a live sync already running is refused.
 *  - the owner keeps the lease alive with a heartbeat (startSyncHeartbeat)
 *    that refreshes `updatedAt` while its request runs, so a sync that takes
 *    longer than SYNC_STALE_TIMEOUT_MS is never mistaken for an abandoned one.
 *  - a crashed or reloaded tab stops heartbeating, so its record goes stale
 *    after SYNC_STALE_TIMEOUT_MS and any tab may reclaim the slot.
 *  - every record carries a requestId, and refresh/clear only act on the
 *    record they own.
 *
 * ATOMICITY. Separate localStorage reads and writes are NOT atomic across
 * tabs: another tab can write between a getItem and the following
 * setItem/removeItem. Every read-compare-write on the slot therefore runs
 * inside a Web Lock (navigator.locks, exclusive, same-origin), which
 * serializes the tabs. Locks are held only for the few synchronous
 * statements of a critical section and the browser releases them if the tab
 * dies, so they cannot deadlock or outlive a crash. Where Web Locks are
 * unavailable (older browsers, insecure contexts) the same critical sections
 * run unserialized. That path is best effort: the compare-before-write still
 * stops a stale request from clobbering a record it can see, but a write by
 * another tab landing inside that window can still be lost. The lease expiry
 * bounds the damage: a lost record is restored by the owner's next heartbeat.
 *
 * onSyncStateChange complements this: the browser never fires `storage` in
 * the tab that made the change, so it is how a tab that mounted before a
 * sync started learns about it (and about it finishing) in real time,
 * disabling its own button instead of only finding out on its next remount.
 *
 * Hard reloads abort the synchronous request, so stale state is cleared
 * after the timeout to avoid locking out admins.
 */

const STORAGE_KEY = "adminSyncState";
const LOCK_NAME = "adminSyncState:lock";
export const SYNC_STALE_TIMEOUT_MS = 60_000;
/** How often the owner refreshes its lease. Several beats fit inside
 * SYNC_STALE_TIMEOUT_MS so a throttled background timer does not forfeit it. */
export const SYNC_HEARTBEAT_MS = 10_000;

export type AdminSyncStatus = "in_progress" | "completed" | "failed";

export interface AdminSyncState {
  status: AdminSyncStatus;
  startedAt: number;
  /** Last heartbeat. A record is stale when this stops advancing. */
  updatedAt: number;
  /** Identifies the lease holder, so refresh and clear can be scoped to it. */
  requestId: string;
}

function createRequestId(): string {
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
}

function parseSyncState(raw: string | null): AdminSyncState | null {
  if (!raw) return null;
  try {
    const parsed = JSON.parse(raw);
    if (
      !parsed ||
      typeof parsed.startedAt !== "number" ||
      typeof parsed.updatedAt !== "number" ||
      typeof parsed.requestId !== "string" ||
      (parsed.status !== "in_progress" && parsed.status !== "completed" && parsed.status !== "failed")
    ) {
      return null;
    }
    return parsed as AdminSyncState;
  } catch {
    return null;
  }
}

export function readSyncState(): AdminSyncState | null {
  try {
    return parseSyncState(localStorage.getItem(STORAGE_KEY));
  } catch {
    // Corrupt or inaccessible storage is treated the same as "no state".
    return null;
  }
}

/**
 * Runs a read-compare-write on the slot under an exclusive Web Lock when the
 * browser has one. `critical` must not throw (each caller catches storage
 * errors itself), so a rejection here can only mean the lock could not be
 * taken, in which case it runs unserialized (see the module comment).
 */
async function withSyncLock<T>(critical: () => T): Promise<T> {
  const locks = typeof navigator !== "undefined" ? navigator.locks : undefined;
  if (locks && typeof locks.request === "function") {
    try {
      return await locks.request(LOCK_NAME, () => critical());
    } catch {
      // Lock unavailable (e.g. opaque origin): fall back below.
    }
  }
  return critical();
}

/**
 * Claims the slot for a new sync and returns its requestId, or null when
 * another sync is genuinely running (a fresh in-progress record belonging to
 * someone else). A finished, stale, corrupt, or missing record is free to
 * take. When storage is unavailable persistence is best effort, so a
 * requestId is still returned and the sync proceeds untracked.
 */
export function acquireSyncLease(): Promise<string | null> {
  return withSyncLock(() => {
    const requestId = createRequestId();
    try {
      const now = Date.now();
      const current = parseSyncState(localStorage.getItem(STORAGE_KEY));
      if (current && current.status === "in_progress" && !isStale(current, now)) {
        return null;
      }
      const state: AdminSyncState = { status: "in_progress", startedAt: now, updatedAt: now, requestId };
      localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
    } catch {
      // Persistence is best effort
    }
    return requestId;
  });
}

/**
 * Heartbeat: advances `updatedAt` on the record this requestId owns. Returns
 * false once a different request owns the slot (the lease was lost and must
 * not be written over). If the record has simply vanished (another tab's
 * stale recovery cleared it while this request was still running) the owner
 * restores it. `shouldWrite` is checked inside the critical section so a
 * heartbeat already queued when the sync settles cannot resurrect a record
 * that settlement has just cleared.
 */
export function refreshSyncLease(requestId: string, shouldWrite: () => boolean = () => true): Promise<boolean> {
  return withSyncLock(() => {
    try {
      if (!shouldWrite()) return true;
      const now = Date.now();
      const current = parseSyncState(localStorage.getItem(STORAGE_KEY));
      if (current && current.requestId !== requestId) return false;
      const state: AdminSyncState = {
        status: "in_progress",
        startedAt: current?.startedAt ?? now,
        updatedAt: now,
        requestId,
      };
      localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
      return true;
    } catch {
      // Storage unavailable: nothing to refresh, and nothing to lose.
      return true;
    }
  });
}

/**
 * Removes the persisted record, but only if it is still the exact record
 * identified by requestId (or is unreadable garbage). If a different tab has
 * since written a newer record this is a no-op, and because the check and
 * the removal share one critical section that newer write cannot land in
 * between them (see the module comment for the no-Web-Locks caveat).
 */
export function clearSyncState(requestId: string): Promise<void> {
  return withSyncLock(() => {
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      if (raw === null) return;
      const current = parseSyncState(raw);
      if (current && current.requestId !== requestId) return;
      localStorage.removeItem(STORAGE_KEY);
    } catch {
      // Persistence is best effort
    }
  });
}

/**
 * A record is stale once its heartbeat (`updatedAt`) has not advanced for
 * SYNC_STALE_TIMEOUT_MS. Measuring from the last heartbeat rather than from
 * `startedAt` is what lets a long but healthy sync keep its lease.
 */
export function isStale(state: AdminSyncState, now: number = Date.now()): boolean {
  // A non-finite (NaN/Infinity) or future timestamp can never age past the
  // timeout through subtraction alone, which would lock the Sync Now button
  // forever instead of recovering. Treat those as stale so recovery still runs.
  if (
    !Number.isFinite(state.startedAt) ||
    !Number.isFinite(state.updatedAt) ||
    state.startedAt > now ||
    state.updatedAt > now
  ) {
    return true;
  }
  return now - state.updatedAt > SYNC_STALE_TIMEOUT_MS;
}

/** How long until a record would go stale if it is never refreshed again. */
export function msUntilStale(state: AdminSyncState, now: number = Date.now()): number {
  return Math.max(0, SYNC_STALE_TIMEOUT_MS - (now - state.updatedAt));
}

const heartbeats = new Map<string, ReturnType<typeof setInterval>>();

/**
 * Keeps the lease for requestId alive until the returned stop function runs
 * (call it when the request settles, however it settles). One interval per
 * requestId: starting again returns a stop for the existing one. The timer
 * belongs to the request, not to a component, so it survives an SPA
 * unmount/remount while the request is genuinely still running; a hard
 * reload destroys it along with the request, which is exactly what lets the
 * lease expire. onLost fires if another request has taken the slot.
 */
export function startSyncHeartbeat(requestId: string, onLost?: () => void): () => void {
  const stop = () => {
    const timer = heartbeats.get(requestId);
    if (timer !== undefined) {
      clearInterval(timer);
      heartbeats.delete(requestId);
    }
  };
  if (heartbeats.has(requestId)) return stop;

  const timer = setInterval(() => {
    void refreshSyncLease(requestId, () => heartbeats.has(requestId)).then((stillOwner) => {
      if (!stillOwner) {
        stop();
        onLost?.();
      }
    });
  }, SYNC_HEARTBEAT_MS);
  heartbeats.set(requestId, timer);
  return stop;
}

/**
 * Subscribes to the persisted record changing from another tab (the browser
 * never fires `storage` in the tab that made the change). Returns an
 * unsubscribe function. Calls back with the new state, or null if the
 * record was removed or a full localStorage.clear() ran.
 */
export function onSyncStateChange(callback: (state: AdminSyncState | null) => void): () => void {
  if (typeof window === "undefined") {
    return () => {};
  }
  const handler = (event: StorageEvent) => {
    if (event.key !== null && event.key !== STORAGE_KEY) return;
    callback(parseSyncState(event.newValue));
  };
  window.addEventListener("storage", handler);
  return () => window.removeEventListener("storage", handler);
}

/**
 * Tracks the active request so remounted components can reattach to it.
 * A hard reload clears this module state.
 */
let activeSyncPromise: Promise<unknown> | null = null;
let activeRequestId: string | null = null;

export function getActiveSyncPromise(): Promise<unknown> | null {
  return activeSyncPromise;
}

export function setActiveSyncPromise(promise: Promise<unknown> | null): void {
  activeSyncPromise = promise;
}

/** The requestId of the write behind getActiveSyncPromise(), so a remounted
 * component that reattaches to it can still scope its eventual clear. */
export function getActiveRequestId(): string | null {
  return activeRequestId;
}

export function setActiveRequestId(requestId: string | null): void {
  activeRequestId = requestId;
}
