"use client";

import { useState, useEffect, useRef } from "react";
import { AlertCircle, CheckCircle2, Loader2, RefreshCw } from "lucide-react";
import {
  getOrganizationProviderSettings,
  syncOrganizationProvider,
  clearAdminCache,
  OrganizationProviderSettings,
  OrganizationSyncResult,
} from "@/lib/api/admin";
import { readSyncState, writeSyncState, clearSyncState, isStale, SYNC_STALE_TIMEOUT_MS, getActiveSyncPromise, setActiveSyncPromise } from "@/lib/admin-sync-state";

/**
 * Manual trigger for the external organization-data provider sync.
 *
 * The provider credential (DATA_PROVIDER_BASE_URL / DATA_PROVIDER_API_TOKEN)
 * is environment configuration on the backend -- there is deliberately no
 * token-entry UI here. Syncing can create, update, and hard-delete users,
 * teams, and memberships, so it is deliberately manual: an admin decides when
 * the organization changes shape.
 */
export default function DataProviderConfig() {
  const [settings, setSettings] = useState<OrganizationProviderSettings | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Initialized synchronously from localStorage so a remount (tab switch,
  // navigation, or page reload) never paints an enabled button before the
  // persisted in-progress state has a chance to disable it.
  const [syncing, setSyncing] = useState(() => {
    const persisted = readSyncState();
    return !!persisted && persisted.status === "in_progress" && !isStale(persisted);
  });
  const [syncResult, setSyncResult] = useState<OrganizationSyncResult | null>(null);
  const staleTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Shared by the original click and by a remount that reattached to the
  // still-running request, so both paths clear state and update the UI
  // identically regardless of which component instance observes completion.
  const settleSync = (outcome: { ok: true; result: OrganizationSyncResult } | { ok: false; error: any }) => {
    if (staleTimerRef.current) clearTimeout(staleTimerRef.current);
    clearSyncState();
    setActiveSyncPromise(null);
    setSyncing(false);
    if (outcome.ok) {
      setSyncResult(outcome.result);
      // The admin client caches users and teams for two minutes. Without this
      // the screen would keep showing pre-sync counts.
      clearAdminCache();
    } else {
      setError(outcome.error?.message || "Synchronization failed");
    }
  };

  useEffect(() => {
    loadSettings();

    let cancelled = false;

    const liveSync = getActiveSyncPromise();
    if (liveSync) {
      // SPA navigation/tab switch: the JS module graph survived, so the
      // original request is still genuinely running. Reattach to it instead
      // of guessing from a snapshot -- this is authoritative, not a timeout.
      setSyncing(true);
      liveSync.then(
        (result) => {
          if (cancelled) return;
          settleSync({ ok: true, result: result as OrganizationSyncResult });
        },
        (err) => {
          if (cancelled) return;
          settleSync({ ok: false, error: err });
        }
      );
    } else {
      // No live promise in this session -- either nothing is running, or a
      // hard reload destroyed the module graph along with the real request
      // (which the backend also aborts server-side; see admin-sync-state.ts).
      // Fall back to the persisted snapshot, bounded by a stale timeout so a
      // reload that outlives the real sync never disables the button forever.
      const persisted = readSyncState();
      if (persisted && persisted.status === "in_progress") {
        if (isStale(persisted)) {
          clearSyncState();
          setSyncing(false);
        } else {
          setSyncing(true);
          const remaining = SYNC_STALE_TIMEOUT_MS - (Date.now() - persisted.startedAt);
          staleTimerRef.current = setTimeout(() => {
            clearSyncState();
            setSyncing(false);
          }, remaining);
        }
      }
    }

    return () => {
      cancelled = true;
      if (staleTimerRef.current) clearTimeout(staleTimerRef.current);
    };
  }, []);

  const loadSettings = async () => {
    setLoading(true);
    setError(null);
    try {
      setSettings(await getOrganizationProviderSettings());
    } catch (err: any) {
      setError(err.message || "Failed to load provider settings");
    } finally {
      setLoading(false);
    }
  };

  const handleSync = async () => {
    // Guard as well as disable: a double-submit must not reach the backend,
    // which would answer the second call with a 409.
    if (syncing) return;

    // Persisted before the request starts so a reload/navigation immediately
    // after the click still finds an in-progress record to restore.
    writeSyncState("in_progress");
    setSyncing(true);
    setError(null);
    setSyncResult(null);

    const promise = syncOrganizationProvider();
    setActiveSyncPromise(promise);
    try {
      const result = await promise;
      settleSync({ ok: true, result });
    } catch (err: any) {
      settleSync({ ok: false, error: err });
    }
  };

  if (loading) {
    return (
      <div className="text-center py-8">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-indigo-600 mx-auto mb-4"></div>
        <p className="text-gray-500">Loading provider settings...</p>
      </div>
    );
  }

  const readyToSync = settings?.readyToSync ?? false;
  const isMassDeletionHold = !!error?.toLowerCase().includes("held for review");

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <h3 className="text-lg font-medium text-gray-900">Organization Data Provider</h3>
        <button
          data-testid="sync-now-btn"
          onClick={handleSync}
          disabled={syncing || !readyToSync}
          aria-label="Sync organization data from provider"
          aria-busy={syncing}
          className="flex items-center gap-2 px-4 py-2 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {syncing ? (
            <Loader2 className="w-4 h-4 animate-spin" />
          ) : (
            <RefreshCw className="w-4 h-4" />
          )}
          {syncing ? "Syncing..." : "Sync Now"}
        </button>
      </div>

      <p className="text-sm text-gray-500 mb-4">
        Pulls people, teams and team membership from Data Provider and applies them to Team360. The Data Provider is
        authoritative: a user or team no longer reported by Data Provider is removed from Team360.
      </p>

      {!readyToSync && (
        <div
          data-testid="provider-not-configured"
          className="mb-4 flex items-start gap-3 p-4 bg-amber-50 border border-amber-200 rounded-lg"
        >
          <AlertCircle className="w-5 h-5 text-amber-600 mt-0.5 flex-shrink-0" />
          <div>
            <p className="text-sm font-medium text-amber-800">Provider Not Ready</p>
            <ul className="text-sm text-amber-700 mt-1 list-disc list-inside space-y-0.5">
              {!settings?.baseUrlConfigured && (
                <li>
                  Set <code className="bg-amber-100 px-1 rounded">DATA_PROVIDER_BASE_URL</code> on
                  the API service.
                </li>
              )}
              {!settings?.tokenConfigured && (
                <li>
                  Set <code className="bg-amber-100 px-1 rounded">DATA_PROVIDER_API_TOKEN</code> on
                  the API service.
                </li>
              )}
            </ul>
          </div>
        </div>
      )}

      {error && (
        <div
          data-testid={isMassDeletionHold ? "sync-mass-deletion-hold" : "provider-error"}
          className="mb-4 p-4 bg-red-50 border border-red-200 rounded-lg flex items-start gap-3"
        >
          <AlertCircle className="w-5 h-5 text-red-600 mt-0.5 flex-shrink-0" />
          <div>
            <p className="font-medium text-red-900">{isMassDeletionHold ? "Sync held for review" : "Error"}</p>
            <p className="text-sm text-red-700">{error}</p>
          </div>
        </div>
      )}

      {syncResult && (
        <div
          data-testid="sync-result"
          className="mb-4 p-4 bg-green-50 border border-green-200 rounded-lg flex items-start gap-3"
        >
          <CheckCircle2 className="w-5 h-5 text-green-600 mt-0.5 flex-shrink-0" />
          <div>
            <p className="font-medium text-green-900">Sync complete</p>
            <p className="text-sm text-green-700">
              {syncResult.usersSynced} users, {syncResult.teamsSynced} teams and{" "}
              {syncResult.membershipsSynced} team memberships synchronized.
            </p>
            {(syncResult.usersDeleted > 0 || syncResult.teamsDeleted > 0) && (
              <p className="text-sm text-green-700 mt-1" data-testid="sync-deleted">
                {syncResult.usersDeleted} user(s) and {syncResult.teamsDeleted} team(s) removed
                (no longer reported by the provider).
              </p>
            )}
            {syncResult.membershipsRemoved > 0 && (
              <p className="text-sm text-green-700 mt-1">
                {syncResult.membershipsRemoved} stale team membership(s) removed.
              </p>
            )}
            {syncResult.actionItemsDeleted > 0 && (
              <p className="text-sm text-amber-700 mt-1" data-testid="sync-action-items-deleted">
                {syncResult.actionItemsDeleted} action item(s) removed along with their deleted
                user or team.
              </p>
            )}
          </div>
        </div>
      )}

      {syncResult && syncResult.healthChecksDisabled > 0 && (
        <div
          data-testid="sync-health-check-warning"
          className="mb-4 flex items-start gap-3 p-4 bg-amber-50 border border-amber-200 rounded-lg"
        >
          <AlertCircle className="w-5 h-5 text-amber-600 mt-0.5 flex-shrink-0" />
          <div>
            <p className="text-sm font-medium text-amber-800">
              Health checks switched off for {syncResult.healthChecksDisabled} team(s)
            </p>
            <p className="text-sm text-amber-700 mt-1">
              The provider reported these teams as not participating in health checks.
            </p>
          </div>
        </div>
      )}
    </div>
  );
}
