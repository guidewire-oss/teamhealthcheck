"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { AlertCircle, CheckCircle2, Clock, Loader2, RefreshCw, X, XCircle } from "lucide-react";
import {
  listSyncRunHistory,
  type PaginationMeta,
  type SyncRunHistoryItem,
  type SyncRunStatus,
  type SyncRunTrigger,
} from "@/lib/api/admin";
import {
  NOT_AVAILABLE,
  STATUS_OPTIONS,
  TRIGGER_OPTIONS,
  errorCodeLabel,
  exactTimestamp,
  formatCount,
  formatDuration,
  formatTimestamp,
  initiatorLabel,
  isActive,
  relativeTime,
  shortId,
  snapshotLabel,
  stageLabel,
  statusLabel,
  syncTypeLabel,
  triggerLabel,
} from "@/lib/sync-history";

export const SYNC_HISTORY_PAGE_SIZE = 20;

/** Colour is reinforced by an icon and the status word, never colour alone. */
const STATUS_STYLE: Record<SyncRunStatus, string> = {
  SUCCESS: "bg-green-100 text-green-800",
  FAILED: "bg-red-100 text-red-800",
  RUNNING: "bg-blue-100 text-blue-800",
  PENDING: "bg-amber-100 text-amber-800",
};

function StatusBadge({ status }: { status: SyncRunStatus }) {
  const Icon = status === "SUCCESS" ? CheckCircle2 : status === "FAILED" ? XCircle : status === "RUNNING" ? Loader2 : Clock;
  return (
    <span
      data-testid="sync-status-badge"
      data-status={status}
      className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium ${STATUS_STYLE[status] ?? "bg-gray-100 text-gray-800"}`}
    >
      <Icon aria-hidden="true" className={`w-3.5 h-3.5 ${status === "RUNNING" ? "animate-spin" : ""}`} />
      {statusLabel(status)}
    </span>
  );
}

function Time({ iso }: { iso: string | null | undefined }) {
  return <time dateTime={iso ?? undefined} title={exactTimestamp(iso)}>{formatTimestamp(iso)}</time>;
}

type Summary = {
  latest: SyncRunHistoryItem | null;
  success: SyncRunHistoryItem | null;
  failed: SyncRunHistoryItem | null;
};

function SummaryCard({ id, title, children }: { id: string; title: string; children: React.ReactNode }) {
  return (
    <div className="border rounded-lg p-3 bg-gray-50" data-testid={id}>
      <h4 className="text-xs font-medium uppercase tracking-wide text-gray-500">{title}</h4>
      <div className="mt-1 text-sm text-gray-900">{children}</div>
    </div>
  );
}

function RunLine({ run, at }: { run: SyncRunHistoryItem; at: string | null }) {
  return (
    <>
      <div className="font-medium">{syncTypeLabel(run)}</div>
      <div className="text-gray-600">
        <Time iso={at} /> <span className="text-gray-500">({relativeTime(at)})</span>
      </div>
    </>
  );
}

function DetailsDialog({ run, onClose }: { run: SyncRunHistoryItem; onClose: () => void }) {
  const closeRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const opener = document.activeElement as HTMLElement | null;
    closeRef.current?.focus();
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("keydown", onKey);
      opener?.focus?.();
    };
  }, [onClose]);

  const rows: [string, React.ReactNode][] = [
    ["Run ID", <span key="id" className="font-mono break-all">{run.runId}</span>],
    ["Sync type", syncTypeLabel(run)],
    ["Status", <StatusBadge key="s" status={run.status} />],
    ["Stage", stageLabel(run.stage)],
    ["Initiated by", <span key="i" className="break-all">{initiatorLabel(run.initiatedBy)}</span>],
    ["Queued at", <Time key="c" iso={run.createdAt} />],
    ["Started at", run.startedAt ? <Time key="st" iso={run.startedAt} /> : NOT_AVAILABLE],
    [
      "Finished at",
      run.completedAt ? <Time key="f" iso={run.completedAt} /> : isActive(run.status) ? "In progress" : NOT_AVAILABLE,
    ],
    ["Duration", formatDuration(run.durationSeconds)],
    ["Provider snapshot generated at", run.sourceGeneratedAt ? <Time key="g" iso={run.sourceGeneratedAt} /> : NOT_AVAILABLE],
  ];

  // Same wording as Settings > "Sync complete". Only a succeeded run has counts.
  const na = "Not available";
  const c = (n: number | null) => formatCount(n, na);
  const countRows: [string, string, string][] = [
    ["Users added", "users-added", c(run.insertedUsers)],
    ["Users updated", "users-updated", c(run.updatedUsers)],
    ["Users unchanged", "users-unchanged", c(run.unchangedUsers)],
    ["Teams added", "teams-added", c(run.insertedTeams)],
    ["Teams updated", "teams-updated", c(run.updatedTeams)],
    ["Teams unchanged", "teams-unchanged", c(run.unchangedTeams)],
    ["Membership changes", "membership-changes", c(run.membershipChanges)],
  ];

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" onClick={onClose}>
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="sync-run-details-title"
        data-testid="sync-run-details"
        className="bg-white rounded-xl shadow-xl w-full max-w-xl max-h-[90vh] overflow-y-auto p-6"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-start justify-between gap-4 mb-4">
          <h3 id="sync-run-details-title" className="text-lg font-semibold text-gray-900">Sync run details</h3>
          <button ref={closeRef} onClick={onClose} aria-label="Close details" className="p-1 rounded hover:bg-gray-100">
            <X className="w-5 h-5" />
          </button>
        </div>
        <dl className="grid grid-cols-1 sm:grid-cols-3 gap-x-4 gap-y-2 text-sm">
          {rows.map(([k, v]) => (
            <div key={k} className="contents">
              <dt className="font-medium text-gray-600">{k}</dt>
              <dd className="sm:col-span-2 text-gray-900 min-w-0">{v}</dd>
            </div>
          ))}
        </dl>
        <div className="mt-4" data-testid="sync-run-counts">
          <h4 className="text-sm font-medium text-gray-900">Records changed by this run</h4>
          <dl className="mt-1 grid grid-cols-1 sm:grid-cols-3 gap-x-4 gap-y-1 text-sm">
            {countRows.map(([k, id, v]) => (
              <div key={id} className="contents">
                <dt className="font-medium text-gray-600">{k}</dt>
                <dd className="sm:col-span-2 text-gray-900" data-testid={`detail-${id}`}>{v}</dd>
              </div>
            ))}
          </dl>
          {run.status !== "SUCCESS" && (
            <p className="mt-1 text-xs text-gray-500">
              Counts are recorded only when a run completes successfully. {isActive(run.status) ? "This run has not finished." : "This run did not apply any changes."}
            </p>
          )}
        </div>
        {run.status === "FAILED" && (
          <div className="mt-4 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-900" data-testid="sync-run-failure">
            <h4 className="font-medium">Failure</h4>
            <dl className="mt-1 space-y-1">
              <div><dt className="inline font-medium">Failed during: </dt><dd className="inline">{stageLabel(run.failedStage)}</dd></div>
              <div><dt className="inline font-medium">Reason: </dt><dd className="inline">{errorCodeLabel(run.errorCode)}{run.errorCode ? <span className="text-red-700"> ({run.errorCode})</span> : null}</dd></div>
              <div><dt className="font-medium">Details</dt><dd className="break-words whitespace-pre-wrap">{run.errorMessage ?? "Not available"}</dd></div>
            </dl>
          </div>
        )}
      </div>
    </div>
  );
}

/**
 * Read-only history of organization sync runs for Admin > Settings. All
 * filtering, ordering and paging happen on the server. There are deliberately
 * no controls that start, retry or modify a sync.
 */
export default function SyncHistory() {
  const [status, setStatus] = useState<SyncRunStatus | "">("");
  const [trigger, setTrigger] = useState<SyncRunTrigger | "">("");
  const [page, setPage] = useState(1);
  const [runs, setRuns] = useState<SyncRunHistoryItem[]>([]);
  const [pagination, setPagination] = useState<PaginationMeta | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selected, setSelected] = useState<SyncRunHistoryItem | null>(null);
  const [summary, setSummary] = useState<Summary | null>(null);
  const [summaryError, setSummaryError] = useState(false);
  const seq = useRef(0);

  const load = useCallback(async () => {
    const mine = ++seq.current;
    setLoading(true);
    setError(null);
    try {
      const res = await listSyncRunHistory({
        page, pageSize: SYNC_HISTORY_PAGE_SIZE,
        status: status || undefined, trigger: trigger || undefined,
      });
      if (mine !== seq.current) return;
      setRuns(res.runs);
      setPagination(res.pagination);
    } catch (err) {
      if (mine !== seq.current) return;
      setError(err instanceof Error ? err.message : "Could not load sync history");
    } finally {
      if (mine === seq.current) setLoading(false);
    }
  }, [page, status, trigger]);

  // The summary asks the server for the newest run overall, newest success and
  // newest failure, so "None" for failures means there really are none -- not
  // merely none on the current table page. It ignores the table filters.
  const loadSummary = useCallback(async () => {
    try {
      const first = (r: Awaited<ReturnType<typeof listSyncRunHistory>>) => r.runs[0] ?? null;
      const [latest, success, failed] = await Promise.all([
        listSyncRunHistory({ pageSize: 1 }),
        listSyncRunHistory({ pageSize: 1, status: "SUCCESS" }),
        listSyncRunHistory({ pageSize: 1, status: "FAILED" }),
      ]);
      setSummary({ latest: first(latest), success: first(success), failed: first(failed) });
      setSummaryError(false);
    } catch {
      setSummaryError(true);
    }
  }, []);

  useEffect(() => { load(); }, [load]);
  useEffect(() => { loadSummary(); }, [loadSummary]);

  const refresh = () => { load(); loadSummary(); };
  const closeDetails = useCallback(() => setSelected(null), []);
  const noRunsEver = !loading && !error && runs.length === 0 && !status && !trigger && page === 1;

  return (
    <div data-testid="sync-history">
      <div className="flex items-center justify-between gap-2">
        <h2 className="text-xl font-semibold text-gray-900">Sync History</h2>
        <button onClick={refresh} className="flex items-center gap-1 text-sm text-indigo-700 hover:underline" data-testid="sync-history-refresh">
          <RefreshCw className="w-4 h-4" aria-hidden="true" /> Refresh
        </button>
      </div>
      <p className="text-sm text-gray-500 mt-1 mb-4">
        Read-only record of organization sync runs, newest first. Times are shown in your local timezone.
      </p>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-3 mb-4" data-testid="sync-history-summary">
        {summaryError ? (
          <p role="alert" className="md:col-span-3 text-sm text-gray-600" data-testid="sync-summary-error">Summary unavailable.</p>
        ) : !summary ? (
          <p className="md:col-span-3 text-sm text-gray-500" data-testid="sync-summary-loading">Loading summary...</p>
        ) : (
          <>
            <SummaryCard id="sync-summary-success" title="Last successful sync">
              {summary.success ? <RunLine run={summary.success} at={summary.success.completedAt ?? summary.success.createdAt} /> : "No successful sync yet"}
            </SummaryCard>
            <SummaryCard id="sync-summary-failed" title="Last failed sync">
              {summary.failed ? <RunLine run={summary.failed} at={summary.failed.completedAt ?? summary.failed.createdAt} /> : "None"}
            </SummaryCard>
            <SummaryCard id="sync-summary-latest" title="Latest sync status">
              {summary.latest ? (
                <>
                  <StatusBadge status={summary.latest.status} />
                  <div className="mt-1 text-gray-600">{syncTypeLabel(summary.latest)}</div>
                  <div className="text-gray-600"><Time iso={summary.latest.completedAt ?? summary.latest.createdAt} /></div>
                </>
              ) : "No sync has run yet"}
            </SummaryCard>
          </>
        )}
      </div>

      <div className="flex flex-wrap items-center gap-4 mb-3">
        <label className="flex items-center gap-2 text-sm text-gray-800">
          Status
          <select data-testid="sync-filter-status" value={status} className="border border-gray-300 rounded px-2 py-1"
            onChange={(e) => { setStatus(e.target.value as SyncRunStatus | ""); setPage(1); }}>
            <option value="">All</option>
            {STATUS_OPTIONS.map((s) => <option key={s} value={s}>{statusLabel(s)}</option>)}
          </select>
        </label>
        <label className="flex items-center gap-2 text-sm text-gray-800">
          Sync type
          <select data-testid="sync-filter-trigger" value={trigger} className="border border-gray-300 rounded px-2 py-1"
            onChange={(e) => { setTrigger(e.target.value as SyncRunTrigger | ""); setPage(1); }}>
            <option value="">All</option>
            {TRIGGER_OPTIONS.map((t) => <option key={t} value={t}>{triggerLabel(t)}</option>)}
          </select>
        </label>
      </div>

      {error ? (
        <div role="alert" data-testid="sync-history-error" className="flex items-center gap-3 text-sm text-red-700">
          <AlertCircle className="w-4 h-4" aria-hidden="true" />
          <span>{error}</span>
          <button onClick={load} className="px-3 py-1 border border-red-300 rounded hover:bg-red-50" data-testid="sync-history-retry">Retry</button>
        </div>
      ) : loading && runs.length === 0 ? (
        <p className="text-sm text-gray-500" data-testid="sync-history-loading">Loading sync history...</p>
      ) : runs.length === 0 ? (
        <p className="text-sm text-gray-600" data-testid="sync-history-empty">
          {noRunsEver ? "No sync runs have been recorded yet." : "No sync runs match these filters."}
        </p>
      ) : (
        <div className={loading ? "opacity-60" : ""} aria-busy={loading}>
          <div className="overflow-x-auto">
            <table className="min-w-full text-sm text-left" data-testid="sync-history-table">
              <caption className="sr-only">Organization sync runs, newest first</caption>
              <thead className="text-xs uppercase text-gray-500 border-b">
                <tr>
                  <th scope="col" className="py-2 pr-3">Sync type</th>
                  <th scope="col" className="py-2 pr-3">Status</th>
                  <th scope="col" className="py-2 pr-3">Users added</th>
                  <th scope="col" className="py-2 pr-3">Teams added</th>
                  <th scope="col" className="py-2 pr-3 hidden md:table-cell">Stage</th>
                  <th scope="col" className="py-2 pr-3 hidden lg:table-cell">Initiated by</th>
                  <th scope="col" className="py-2 pr-3">Started</th>
                  <th scope="col" className="py-2 pr-3 hidden md:table-cell">Finished</th>
                  <th scope="col" className="py-2 pr-3 hidden lg:table-cell">Duration</th>
                  <th scope="col" className="py-2"><span className="sr-only">Actions</span></th>
                </tr>
              </thead>
              <tbody>
                {runs.map((r) => (
                  <tr key={r.runId} data-testid="sync-history-row" className="border-b hover:bg-gray-50 cursor-pointer" onClick={() => setSelected(r)}>
                    <td className="py-2 pr-3">
                      <div>{triggerLabel(r.trigger)}</div>
                      <div className="text-xs text-gray-500">{snapshotLabel(r.snapshotType)} · <span title={r.runId} className="font-mono">{shortId(r.runId)}</span></div>
                    </td>
                    <td className="py-2 pr-3"><StatusBadge status={r.status} /></td>
                    <td className="py-2 pr-3" data-testid="sync-users-added" title={r.insertedUsers === null ? "Not available" : undefined}>{formatCount(r.insertedUsers)}</td>
                    <td className="py-2 pr-3" data-testid="sync-teams-added" title={r.insertedTeams === null ? "Not available" : undefined}>{formatCount(r.insertedTeams)}</td>
                    <td className="py-2 pr-3 hidden md:table-cell">{stageLabel(r.stage)}</td>
                    <td className="py-2 pr-3 hidden lg:table-cell break-all max-w-[12rem]">{initiatorLabel(r.initiatedBy)}</td>
                    <td className="py-2 pr-3">
                      {r.startedAt ? <Time iso={r.startedAt} /> : <><Time iso={r.createdAt} /> <span className="text-xs text-gray-500">(queued)</span></>}
                    </td>
                    <td className="py-2 pr-3 hidden md:table-cell">
                      {r.completedAt ? <Time iso={r.completedAt} /> : isActive(r.status) ? "In progress" : NOT_AVAILABLE}
                    </td>
                    <td className="py-2 pr-3 hidden lg:table-cell">{formatDuration(r.durationSeconds)}</td>
                    <td className="py-2">
                      <button
                        className="text-indigo-700 hover:underline"
                        aria-label={`View details for run ${shortId(r.runId)}`}
                        onClick={(e) => { e.stopPropagation(); setSelected(r); }}
                      >
                        View details
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {pagination && (
            <nav className="flex items-center justify-between gap-3 mt-3 text-sm" aria-label="Sync history pagination">
              <span data-testid="sync-history-page-info">
                Page {pagination.page} of {Math.max(pagination.totalPages, 1)} · {pagination.totalItems} run{pagination.totalItems === 1 ? "" : "s"}
              </span>
              <span className="flex gap-2">
                <button disabled={!pagination.hasPreviousPage || loading} onClick={() => setPage((p) => p - 1)}
                  className="px-3 py-1 border rounded disabled:opacity-50" data-testid="sync-history-prev">Previous</button>
                <button disabled={!pagination.hasNextPage || loading} onClick={() => setPage((p) => p + 1)}
                  className="px-3 py-1 border rounded disabled:opacity-50" data-testid="sync-history-next">Next</button>
              </span>
            </nav>
          )}
        </div>
      )}

      {selected && <DetailsDialog run={selected} onClose={closeDetails} />}
    </div>
  );
}
