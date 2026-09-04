/**
 * Persists Sync Now state across tab switches and SPA navigation.
 * Hard reloads abort the synchronous request, so stale state is cleared
 * after the timeout to avoid locking out admins.
 */

const STORAGE_KEY = "adminSyncState";
export const SYNC_STALE_TIMEOUT_MS = 60_000;

export type AdminSyncStatus = "in_progress" | "completed" | "failed";

export interface AdminSyncState {
  status: AdminSyncStatus;
  startedAt: number;
  updatedAt: number;
}

export function readSyncState(): AdminSyncState | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw);
    if (
      !parsed ||
      typeof parsed.startedAt !== "number" ||
      typeof parsed.updatedAt !== "number" ||
      (parsed.status !== "in_progress" && parsed.status !== "completed" && parsed.status !== "failed")
    ) {
      return null;
    }
    return parsed as AdminSyncState;
  } catch {
    // Corrupt or inaccessible storage is treated the same as "no state".
    return null;
  }
}

export function writeSyncState(status: AdminSyncStatus, startedAt: number = Date.now()): void {
  try {
    const state: AdminSyncState = { status, startedAt, updatedAt: Date.now() };
    localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
  } catch {
    // Persistence is best effort
  }
}

export function clearSyncState(): void {
  try {
    localStorage.removeItem(STORAGE_KEY);
  } catch {
    // See writeSyncState.
  }
}

export function isStale(state: AdminSyncState, now: number = Date.now()): boolean {
  return now - state.startedAt > SYNC_STALE_TIMEOUT_MS;
}

/**
 * Tracks the active request so remounted components can reattach to it.
 * A hard reload clears this module state.
 */
let activeSyncPromise: Promise<unknown> | null = null;

export function getActiveSyncPromise(): Promise<unknown> | null {
  return activeSyncPromise;
}

export function setActiveSyncPromise(promise: Promise<unknown> | null): void {
  activeSyncPromise = promise;
}
