"use client";

import { useState, useEffect } from "react";
import { AlertCircle, CheckCircle2, Loader2, RefreshCw, ShieldAlert } from "lucide-react";
import {
  getOrganizationProviderSettings,
  getMassDeletionHold,
  dismissMassDeletionHold,
  clearAdminCache,
  ConfirmedMassDeletion,
  DeletionMetric,
  MassDeletionReport,
  OrganizationProviderSettings,
} from "@/lib/api/admin";
import { useSyncRun } from "@/lib/useSyncRun";
import { isActive } from "@/lib/sync-progress";
import SyncProgressBar from "./SyncProgressBar";
import MassDeletionThresholdSettings from "./MassDeletionThresholdSettings";

/**
 * Manual trigger for the external organization-data provider sync.
 *
 * The provider credential (DATA_PROVIDER_BASE_URL / DATA_PROVIDER_API_TOKEN)
 * is environment configuration on the backend -- there is deliberately no
 * token-entry UI here. A sync upserts users, teams and memberships and marks
 * anyone no longer reported as inactive; it never deletes people. It is
 * deliberately manual: an admin decides when the organization changes shape.
 *
 * The sync runs in the background on the server and its status is persisted, so
 * this component only starts it and follows the stored run.
 */
/** Renders a percentage the way the backend calculated it, e.g. "25.0%". */
function formatPercent(value: number): string {
  return `${value.toFixed(1)}%`;
}

/**
 * Renders one metric as "deleted / existing = percent%", using the backend's
 * own label for what the count means. A cascaded count is never called a
 * deletion: those rows go because the database removes them with their owner.
 */
function metricSentence(metric: DeletionMetric): string {
  const label = metric.kind === "cascaded" ? "cascaded" : "deactivated";
  return `${metric.deleting} ${label} / ${metric.existing} existing = ${formatPercent(metric.percent)}`;
}

/**
 * Explains WHY a count is what it is: how many records the provider actually
 * sent for this entity type. A big deletion next to a small incoming count is
 * an incomplete payload, not a real mass departure -- which is the single most
 * common cause of a surprising hold, and is otherwise invisible from the UI.
 */
function incomingSentence(metric: DeletionMetric): string {
  return `provider sent ${metric.incoming}`;
}

export default function DataProviderConfig() {
  const [settings, setSettings] = useState<OrganizationProviderSettings | null>(null);
  const [loading, setLoading] = useState(true);
  // Distinguish an unfetched/failed settings request from an unconfigured provider
  const [settingsLoadError, setSettingsLoadError] = useState<string | null>(null);

  const { run, loading: runLoading, starting, startError, pollError, start } = useSyncRun();

  // Two-step confirmation for the override, matching the destructive-action
  // pattern used elsewhere in admin: clicking Sync Anyway reveals a confirm
  // button rather than firing the request.
  const [confirmingOverride, setConfirmingOverride] = useState(false);
  const [dismissingHold, setDismissingHold] = useState(false);
  const [dismissError, setDismissError] = useState<string | null>(null);
  // The run whose hold the admin dismissed, so the banner stays hidden for
  // exactly that run after a poll or refresh re-reads it.
  const [dismissedRunId, setDismissedRunId] = useState<string | null>(null);

  const syncing = starting || isActive(run);
  // A sync must not be started until the latest persisted run is known: until
  // then an already-active run is invisible, so a click could start a duplicate.
  // This only blocks starting; it is not "syncing", so the label stays "Sync Now".
  const startBlocked = syncing || runLoading;
  const rawHold = getMassDeletionHold(run);
  const hold = rawHold && run && run.runId !== dismissedRunId ? rawHold : null;

  useEffect(() => {
    loadSettings();
  }, []);

  // The admin client caches users and teams for two minutes. Without this the
  // screen would keep showing pre-sync counts once a run succeeds.
  const succeededRunId = run?.status === "SUCCESS" ? run.runId : null;
  useEffect(() => {
    if (succeededRunId) clearAdminCache();
  }, [succeededRunId]);

  const loadSettings = async () => {
    setLoading(true);
    setSettingsLoadError(null);
    try {
      setSettings(await getOrganizationProviderSettings());
    } catch (err: any) {
      setSettingsLoadError(err.message || "Failed to load provider settings");
    } finally {
      setLoading(false);
    }
  };

  /**
   * Starts a sync. `overrideMassDeletion` is only ever true on the explicit,
   * confirmed Sync Anyway path -- a hold is never retried automatically, and
   * the plain Sync Now button always sends a normal request.
   *
   * `confirmedReport` is the exact hold this override is confirming. Every sync
   * re-fetches the provider snapshot fresh, so the backend needs these numbers
   * to verify nothing changed between the hold and this confirmation.
   */
  const handleSync = async (overrideMassDeletion = false, confirmedReport?: MassDeletionReport) => {
    // Guard as well as disable: a double-submit must not reach the backend.
    if (startBlocked) return;
    setConfirmingOverride(false);
    setDismissError(null);

    const confirmedMassDeletion: ConfirmedMassDeletion | undefined = confirmedReport
      ? {
          usersExisting: confirmedReport.users.existing,
          usersDeleting: confirmedReport.users.deleting,
          teamsExisting: confirmedReport.teams.existing,
          teamsDeleting: confirmedReport.teams.deleting,
        }
      : undefined;

    await start({ overrideMassDeletion, confirmedMassDeletion });
  };

  /**
   * Resolves a hold the admin has decided not to override -- because the
   * provider's data is what needs fixing. It applies nothing; it only clears
   * the review, which is also what unfreezes the deactivation threshold.
   */
  const handleDismissHold = async () => {
    if (!run) return;
    setDismissingHold(true);
    setDismissError(null);
    try {
      await dismissMassDeletionHold();
      setDismissedRunId(run.runId);
      setConfirmingOverride(false);
    } catch (err: any) {
      setDismissError(err?.message || "Failed to dismiss the hold");
    } finally {
      setDismissingHold(false);
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
  // Only render the "not configured" guidance once settings actually loaded
  // and said so -- a failed fetch (settingsLoadError) means readiness is
  // simply unknown, not that the provider is unconfigured.
  const showNotConfigured = !!settings && !readyToSync;

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <h3 className="text-lg font-medium text-gray-900">Organization Data Provider</h3>
        <button
          data-testid="sync-now-btn"
          onClick={() => handleSync()}
          disabled={startBlocked || !readyToSync || !!settingsLoadError}
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

      <SyncProgressBar run={run} errorMessage={run?.status === "FAILED" && !hold ? run.errorMessage : null} />

      <p className="text-sm text-gray-500 my-4">
        Pulls people, teams and team membership from Data Provider and applies them to Team360. Existing records are
        updated and new ones added. Anyone no longer reported by a complete Data Provider snapshot is marked inactive,
        never deleted, so past surveys and history are kept.
      </p>

      <p className="sr-only" role="status" aria-live="polite" data-testid="sync-last-status">
        {runLoading ? "Loading last synchronization status" : ""}
      </p>

      {settingsLoadError && (
        <div
          data-testid="provider-settings-load-error"
          className="mb-4 flex items-start gap-3 p-4 bg-red-50 border border-red-200 rounded-lg"
        >
          <AlertCircle className="w-5 h-5 text-red-600 mt-0.5 flex-shrink-0" />
          <div>
            <p className="text-sm font-medium text-red-900">Couldn&apos;t load provider settings</p>
            <p className="text-sm text-red-700 mt-1">{settingsLoadError}</p>
            <button
              onClick={loadSettings}
              className="text-sm text-red-800 underline mt-2"
            >
              Retry
            </button>
          </div>
        </div>
      )}

      {showNotConfigured && (
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

      {hold && (
        <div
          data-testid="sync-mass-deletion-hold"
          role="alert"
          className="mb-4 p-4 bg-red-50 border-2 border-red-400 rounded-lg flex items-start gap-3"
        >
          <ShieldAlert className="w-5 h-5 text-red-600 mt-0.5 flex-shrink-0" />
          <div className="w-full">
            <p className="font-medium text-red-900">Sync held for review</p>
            <ul className="text-sm text-red-800 mt-2 space-y-1">
              <li data-testid="sync-hold-users">
                <strong>Users:</strong> {metricSentence(hold.users)}{" "}
                <span className="text-red-600">({incomingSentence(hold.users)})</span>
              </li>
              <li data-testid="sync-hold-teams">
                <strong>Teams:</strong> {metricSentence(hold.teams)}{" "}
                <span className="text-red-600">({incomingSentence(hold.teams)})</span>
              </li>
              {hold.memberships && (
                <li data-testid="sync-hold-memberships">
                  <strong>Memberships:</strong> {metricSentence(hold.memberships)}{" "}
                  <span className="text-red-600">({incomingSentence(hold.memberships)})</span>
                </li>
              )}
            </ul>
            <p className="text-xs text-red-700 mt-2" data-testid="sync-hold-threshold">
              &quot;Existing&quot; counts only the users/teams the provider is allowed to manage &mdash; it
              excludes the permanent admin and the fixed demo/E2E accounts and teams, which a sync never changes. Configured threshold: {formatPercent(hold.threshold)}.
            </p>
            <p className="text-xs text-red-700 mt-2" data-testid="sync-hold-incoming-hint">
              A low incoming count means the provider&apos;s snapshot is missing those records.
            </p>
            <p className="text-xs text-red-700 mt-1" data-testid="sync-hold-not-applied">
              This sync has not been applied; no data was changed.
            </p>

            <p className="text-xs text-red-700 mt-1" data-testid="sync-hold-threshold-locked">
              The deactivation threshold is frozen at this sync&apos;s value until the hold is resolved,
              so raising it is not a way past this review.
            </p>

            {!confirmingOverride ? (
              <div className="mt-3 flex items-center gap-2">
                <button
                  data-testid="sync-anyway-btn"
                  onClick={() => setConfirmingOverride(true)}
                  disabled={syncing}
                  className="px-3 py-1.5 text-sm bg-red-600 text-white rounded-lg hover:bg-red-700 disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  Sync Anyway
                </button>
                <button
                  data-testid="sync-hold-dismiss-btn"
                  onClick={handleDismissHold}
                  disabled={syncing || dismissingHold}
                  className="px-3 py-1.5 text-sm text-red-800 underline disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {dismissingHold ? "Dismissing..." : "Dismiss without syncing"}
                </button>
              </div>
            ) : (
              <div data-testid="sync-anyway-confirm" className="mt-3">
                <p className="text-sm font-medium text-red-900">
                  Apply this sync and mark {hold.users.deleting} user(s) and{" "}
                  {hold.teams.deleting} team(s) inactive? Their history is kept, and they are reactivated if the provider reports them again.
                </p>
                <div className="flex gap-2 mt-2">
                  <button
                    data-testid="sync-anyway-confirm-btn"
                    onClick={() => handleSync(true, hold)}
                    disabled={syncing}
                    className="px-3 py-1.5 text-sm bg-red-600 text-white rounded-lg hover:bg-red-700 disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    Yes, sync anyway
                  </button>
                  <button
                    data-testid="sync-anyway-cancel-btn"
                    onClick={() => setConfirmingOverride(false)}
                    className="px-3 py-1.5 text-sm text-red-800 underline"
                  >
                    Cancel
                  </button>
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      {(startError || dismissError) && (
        <div
          data-testid="provider-error"
          className="mb-4 p-4 bg-red-50 border border-red-200 rounded-lg flex items-start gap-3"
        >
          <AlertCircle className="w-5 h-5 text-red-600 mt-0.5 flex-shrink-0" />
          <div>
            <p className="font-medium text-red-900">Error</p>
            <p className="text-sm text-red-700">{startError || dismissError}</p>
          </div>
        </div>
      )}

      {pollError && syncing && (
        <p data-testid="sync-poll-error" role="status" className="mb-4 text-sm text-amber-700">
          Lost contact with the server while checking progress. Retrying&hellip; ({pollError})
        </p>
      )}

      {run?.status === "SUCCESS" && (
        <div
          data-testid="sync-result"
          data-run-id={run.runId}
          className="mb-4 p-4 bg-green-50 border border-green-200 rounded-lg flex items-start gap-3"
        >
          <CheckCircle2 className="w-5 h-5 text-green-600 mt-0.5 flex-shrink-0" />
          <div>
            <p className="font-medium text-green-900">Sync complete</p>
            {run.completedAt && (
              <p className="text-sm text-green-700" data-testid="sync-completed-at">
                Completed {new Date(run.completedAt).toLocaleString()}
              </p>
            )}
            <p className="text-sm text-green-700" data-testid="sync-counts">
              Users: {run.insertedUsers} added, {run.updatedUsers} updated, {run.unchangedUsers} unchanged; Teams:{" "}
              {run.insertedTeams} added, {run.updatedTeams} updated, {run.unchangedTeams} unchanged; Membership
              changes: {run.membershipChanges}.
            </p>
            {run.deactivatedRecords > 0 && (
              <p className="text-sm text-green-700 mt-1" data-testid="sync-deactivated">
                {run.deactivatedRecords} user(s) or team(s) marked inactive (no longer reported by the provider).
                Their history is kept.
              </p>
            )}
            {run.healthChecksDisabled > 0 && (
              <p className="text-sm text-amber-700 mt-1" data-testid="sync-health-checks-disabled">
                Health checks were switched off for {run.healthChecksDisabled} team(s). They are excluded from
                survey-completion figures until the provider enables them again.
              </p>
            )}
            {run.healthChecksEnabled > 0 && (
              <p className="text-sm text-green-700 mt-1" data-testid="sync-health-checks-enabled">
                Health checks were switched on for {run.healthChecksEnabled} team(s).
              </p>
            )}
            {run.massDeletionOverridden && (
              <p className="text-sm text-amber-700 mt-1" data-testid="sync-override-applied">
                Applied with the mass-deletion hold overridden.
              </p>
            )}
          </div>
        </div>
      )}

      {/* Configures the hold above: the percentage at which a sync stops and
          asks for review, rather than applying its deletions. */}
      <MassDeletionThresholdSettings syncActivity={syncing || !!hold} />
    </div>
  );
}
