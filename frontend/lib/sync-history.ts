import type {
  SyncRunHistoryItem,
  SyncRunSnapshotType,
  SyncRunStatus,
  SyncRunTrigger,
} from '@/lib/api/admin';

/**
 * The one place backend sync enums become human-readable text. Keys are the
 * exact values the backend persists (domain/orgsyncrun); an unknown value falls
 * back to itself rather than breaking the UI.
 */
export const STATUS_LABELS: Record<SyncRunStatus, string> = {
  PENDING: 'Queued',
  RUNNING: 'In progress',
  SUCCESS: 'Succeeded',
  FAILED: 'Failed',
};

export const STATUS_OPTIONS: SyncRunStatus[] = ['PENDING', 'RUNNING', 'SUCCESS', 'FAILED'];

export const TRIGGER_LABELS: Record<SyncRunTrigger, string> = {
  MANUAL: 'Manual',
  SCHEDULED: 'Scheduled',
};

export const TRIGGER_OPTIONS: SyncRunTrigger[] = ['MANUAL', 'SCHEDULED'];

export const SNAPSHOT_LABELS: Record<SyncRunSnapshotType, string> = {
  COMPLETE: 'Complete snapshot',
  PARTIAL: 'Partial snapshot',
};

export const STAGE_LABELS: Record<string, string> = {
  QUEUED: 'Queued',
  ACQUIRING_LOCK: 'Acquiring lock',
  FETCHING: 'Fetching from provider',
  VALIDATING: 'Validating',
  UPSERTING_USERS: 'Updating users',
  UPSERTING_TEAMS: 'Updating teams',
  SYNCING_MEMBERSHIPS: 'Syncing memberships',
  RECONCILING_REMOVALS: 'Reconciling removals',
  COMMITTING: 'Committing',
  COMPLETED: 'Completed',
  FAILED: 'Failed',
};

export const ERROR_CODE_LABELS: Record<string, string> = {
  PROVIDER_NOT_CONFIGURED: 'Provider not configured',
  LOCK_UNAVAILABLE: 'Another sync held the lock',
  PROVIDER_FETCH_FAILED: 'Could not fetch from provider',
  INVALID_SNAPSHOT: 'Provider data was invalid',
  STALE_SNAPSHOT: 'Provider data was older than the last sync',
  MASS_DEACTIVATION_HOLD: 'Held: too many records would be deactivated',
  DATABASE_ERROR: 'Database error',
  STALE_RUN: 'Stopped responding',
  INTERNAL_ERROR: 'Internal error',
};

export const NOT_AVAILABLE = '—';

/**
 * A count for display. Only null/undefined mean "not available"; a real 0 is
 * shown as "0" (no truthiness check).
 */
export const formatCount = (n: number | null | undefined, unavailable: string = NOT_AVAILABLE): string =>
  n === null || n === undefined ? unavailable : String(n);

const label = <T extends string>(map: Record<string, string>, v: T | null | undefined): string =>
  v ? (map[v] ?? v) : NOT_AVAILABLE;

export const statusLabel = (s: string | null | undefined) => label(STATUS_LABELS, s);
export const triggerLabel = (s: string | null | undefined) => label(TRIGGER_LABELS, s);
export const snapshotLabel = (s: string | null | undefined) => label(SNAPSHOT_LABELS, s);
export const stageLabel = (s: string | null | undefined) => label(STAGE_LABELS, s);
export const errorCodeLabel = (s: string | null | undefined) => label(ERROR_CODE_LABELS, s);

/** A run is in progress until it reaches a terminal status. */
export const isActive = (s: SyncRunStatus) => s === 'PENDING' || s === 'RUNNING';

/** "Manual · Complete snapshot" */
export const syncTypeLabel = (r: Pick<SyncRunHistoryItem, 'trigger' | 'snapshotType'>) =>
  `${triggerLabel(r.trigger)} · ${snapshotLabel(r.snapshotType)}`;

/** The backend records "scheduler" for automatic runs; anything else is a user id. */
export function initiatorLabel(initiatedBy: string | null | undefined): string {
  if (!initiatedBy) return NOT_AVAILABLE;
  return initiatedBy === 'scheduler' ? 'Scheduler (automatic)' : initiatedBy;
}

/** "a1b2c3d4…" -- for dense tables only; details show the full id. */
export function shortId(id: string, length = 8): string {
  return id.length > length ? `${id.slice(0, length)}…` : id;
}

/**
 * "12 Nov 2026, 14:05:09 GMT+1" in the browser's timezone. The zone name is
 * always included so the time is unambiguous.
 */
export function formatTimestamp(iso: string | null | undefined): string {
  if (!iso) return NOT_AVAILABLE;
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return NOT_AVAILABLE;
  return new Intl.DateTimeFormat('en-GB', {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
    timeZoneName: 'short',
  }).format(d);
}

/** Exact UTC instant, used as a hover title. */
export const exactTimestamp = (iso: string | null | undefined): string | undefined =>
  iso && !Number.isNaN(new Date(iso).getTime()) ? new Date(iso).toISOString() : undefined;

/** "5 min ago" / "in 2 h"; empty string when the input is unusable. */
export function relativeTime(iso: string | null | undefined, now: number = Date.now()): string {
  if (!iso) return '';
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return '';
  const secs = Math.round((t - now) / 1000);
  const abs = Math.abs(secs);
  const rtf = new Intl.RelativeTimeFormat('en', { numeric: 'auto' });
  if (abs < 60) return rtf.format(secs, 'second');
  if (abs < 3600) return rtf.format(Math.round(secs / 60), 'minute');
  if (abs < 86400) return rtf.format(Math.round(secs / 3600), 'hour');
  return rtf.format(Math.round(secs / 86400), 'day');
}

/** "1 h 2 min 5 s". Null (not calculable) renders as an em dash, never "0 s". */
export function formatDuration(seconds: number | null | undefined): string {
  if (seconds === null || seconds === undefined || seconds < 0) return NOT_AVAILABLE;
  if (seconds === 0) return '< 1 s';
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;
  return [h && `${h} h`, m && `${m} min`, s && `${s} s`].filter(Boolean).join(' ');
}
