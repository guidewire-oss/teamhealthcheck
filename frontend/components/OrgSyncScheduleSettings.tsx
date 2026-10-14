"use client";

import { useEffect, useRef, useState } from "react";
import { AlertCircle, CheckCircle2, Clock } from "lucide-react";
import {
  getOrgSyncSchedule,
  updateOrgSyncSchedule,
  getOrgSyncLastRun,
  OrgSyncSchedule,
  OrgSyncFrequency,
  OrgSyncLastRun,
} from "@/lib/api/admin";

/**
 * Admin control for the automatic organization-sync schedule, plus the
 * persisted result of the most recent attempt.
 *
 * Disabled by default. Enabling reveals a fixed choice of three cadences --
 * never raw cron syntax -- and saving never runs a sync itself: the schedule
 * only decides WHEN the existing sync path runs, unattended, with no
 * automatic override of a mass-deletion hold, ever.
 *
 * The last-result panel is independent of this settings form: it reads the
 * durably persisted outcome of the most recent attempt (survives refresh,
 * logout/login, and a server restart), which is the entire point of storing
 * it server-side rather than only showing it right after a manual click.
 */

const FREQUENCY_LABELS: Record<OrgSyncFrequency, string> = {
  daily: "Daily",
  weekly: "Weekly",
  monthly: "Monthly",
};

function frequencyDescription(frequency: OrgSyncFrequency): string {
  switch (frequency) {
    case "daily":
      return "Runs every day at 02:00.";
    case "monthly":
      return "Runs on the 1st of each month at 02:00.";
    default:
      return "Runs every Sunday at 02:00.";
  }
}

function formatDateTime(iso?: string): string {
  if (!iso) return "";
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

// How often the page polls for a newly completed/skipped scheduled run while
// the admin stays on this page. A scheduled run is server-initiated -- there
// is no client-side event to react to -- and a same-tab manual sync has no
// shared state with this component either, so polling is the only mechanism
// that covers both without depending on localStorage/onSyncStateChange
// (cross-tab only, and never fires in the tab that made the change).
const POLL_INTERVAL_MS = 15000;

const SKIP_REASON_LABELS: Record<string, string> = {
  manual_sync_running: "a manual sync was already running",
  scheduled_sync_running: "another scheduled sync was already running",
  hold_unresolved: "an unresolved mass-deletion hold was still frozen",
  overdue_catch_up_skipped: "the schedule was overdue by more than one interval after a restart",
};

export default function OrgSyncScheduleSettings() {
  const [schedule, setSchedule] = useState<OrgSyncSchedule | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);

  const [enabled, setEnabled] = useState(false);
  const [frequency, setFrequency] = useState<OrgSyncFrequency>("weekly");

  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  const [lastRun, setLastRun] = useState<OrgSyncLastRun | null>(null);
  const [lastRunError, setLastRunError] = useState<string | null>(null);

  const load = async () => {
    setLoadError(null);
    try {
      const loaded = await getOrgSyncSchedule();
      setSchedule(loaded);
      setEnabled(loaded.enabled);
      setFrequency(loaded.frequency ?? loaded.defaultFrequency);
    } catch (err: any) {
      setLoadError(err?.message || "Failed to load the organization sync schedule");
    } finally {
      setLoading(false);
    }
  };

  const loadLastRun = async () => {
    setLastRunError(null);
    try {
      setLastRun(await getOrgSyncLastRun());
    } catch (err: any) {
      setLastRunError(err?.message || "Failed to load the last sync result");
    }
  };

  const dirty =
    !!schedule && (enabled !== schedule.enabled || (enabled && frequency !== (schedule.frequency ?? schedule.defaultFrequency)));

  // Latest-value refs for the poll below: the interval is set up once (empty
  // dependency array) so its closure would otherwise see stale values of
  // `dirty`/`saving` from the render that created it. Assigning on every
  // render keeps them current without recreating the interval.
  const dirtyRef = useRef(dirty);
  dirtyRef.current = dirty;
  const savingRef = useRef(saving);
  savingRef.current = saving;
  // Guards against a poll tick overlapping a still-in-flight previous one
  // (e.g. a slow response outlasting the interval).
  const pollInFlightRef = useRef(false);

  useEffect(() => {
    const poll = async () => {
      if (pollInFlightRef.current) return;
      pollInFlightRef.current = true;
      try {
        await Promise.all([
          loadLastRun(),
          // Skip while the admin has an unsaved edit or a save in flight, so
          // a background poll never overwrites a draft they haven't submitted.
          dirtyRef.current || savingRef.current ? Promise.resolve() : load(),
        ]);
      } finally {
        pollInFlightRef.current = false;
      }
    };

    poll(); // initial fetch, also the first tick
    const intervalId = setInterval(poll, POLL_INTERVAL_MS);
    return () => clearInterval(intervalId);
  }, []);

  const handleSave = async () => {
    if (saving) return;
    setSaving(true);
    setSaveError(null);
    setSaved(false);
    try {
      const updated = await updateOrgSyncSchedule(enabled, enabled ? frequency : undefined);
      setSchedule(updated);
      setEnabled(updated.enabled);
      setFrequency(updated.frequency ?? updated.defaultFrequency);
      setSaved(true);
    } catch (err: any) {
      setSaveError(err?.message || "Failed to save the organization sync schedule");
    } finally {
      setSaving(false);
    }
  };

  const attempt = lastRun?.lastAttempt;
  const skip = lastRun?.lastSkip;

  return (
    <div className="mt-6 pt-6 border-t" data-testid="org-sync-schedule-settings">
      <h4 className="text-base font-medium text-gray-900">Automatic organization sync</h4>
      <p className="text-sm text-gray-500 mt-1">
        Runs the same organization sync as the Sync Now button, unattended, on a schedule. It never
        automatically approves a mass-deletion hold -- a hold still requires an administrator to
        review it manually.
      </p>

      {loading ? (
        <p className="text-sm text-gray-500 mt-4" data-testid="schedule-loading">
          Loading current schedule...
        </p>
      ) : loadError ? (
        <div
          data-testid="schedule-load-error"
          className="mt-4 flex items-start gap-3 p-4 bg-red-50 border border-red-200 rounded-lg"
        >
          <AlertCircle className="w-5 h-5 text-red-600 mt-0.5 flex-shrink-0" />
          <div>
            <p className="text-sm font-medium text-red-900">Couldn&apos;t load the schedule</p>
            <p className="text-sm text-red-700 mt-1">{loadError}</p>
            <button onClick={load} className="text-sm text-red-800 underline mt-2">
              Retry
            </button>
          </div>
        </div>
      ) : (
        <div className="mt-4 space-y-4">
          <div className="flex items-center gap-3">
            <label htmlFor="org-sync-schedule-enabled" className="flex items-center gap-2 cursor-pointer">
              <input
                id="org-sync-schedule-enabled"
                type="checkbox"
                data-testid="schedule-enabled-toggle"
                checked={enabled}
                onChange={(e) => {
                  setEnabled(e.target.checked);
                  setSaved(false);
                  setSaveError(null);
                }}
                className="h-4 w-4 accent-indigo-600"
              />
              <span className="text-sm text-gray-900">Automatic sync: {enabled ? "Enabled" : "Disabled"}</span>
            </label>
          </div>

          {enabled && (
            <div className="space-y-2" data-testid="schedule-frequency-picker">
              <label className="text-sm text-gray-700">Sync frequency</label>
              <div className="flex flex-wrap gap-2">
                {(["daily", "weekly", "monthly"] as OrgSyncFrequency[]).map((f) => (
                  <button
                    key={f}
                    type="button"
                    data-testid={`schedule-frequency-${f}`}
                    aria-pressed={frequency === f}
                    onClick={() => {
                      setFrequency(f);
                      setSaved(false);
                      setSaveError(null);
                    }}
                    className={`px-3 py-1.5 text-sm rounded-lg border transition-colors ${
                      frequency === f
                        ? "bg-indigo-600 text-white border-indigo-600"
                        : "bg-white text-gray-700 border-gray-300 hover:bg-gray-50"
                    }`}
                  >
                    {FREQUENCY_LABELS[f]}
                  </button>
                ))}
              </div>
              <p className="text-xs text-gray-500">{frequencyDescription(frequency)}</p>
            </div>
          )}

          {schedule?.enabled && schedule.nextRunAt && (
            <p className="text-xs text-gray-500 flex items-center gap-1" data-testid="schedule-next-run">
              <Clock className="w-3.5 h-3.5" />
              Next run: {formatDateTime(schedule.nextRunAt)}
            </p>
          )}

          {saveError && (
            <p className="text-sm text-red-600" data-testid="schedule-save-error" role="alert">
              {saveError}
            </p>
          )}

          {saved && (
            <p className="text-sm text-green-700 flex items-center gap-1" data-testid="schedule-save-success">
              <CheckCircle2 className="w-4 h-4" />
              Schedule saved
            </p>
          )}

          <button
            type="button"
            onClick={handleSave}
            disabled={saving || !dirty}
            data-testid="schedule-save-btn"
            className="px-4 py-2 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {saving ? "Saving..." : "Save schedule"}
          </button>
        </div>
      )}

      <div className="mt-6 pt-4 border-t border-gray-100">
        <h5 className="text-sm font-medium text-gray-900">Last sync result</h5>

        {lastRunError ? (
          <p className="text-sm text-red-600 mt-2" data-testid="last-run-load-error">
            {lastRunError}
          </p>
        ) : !attempt ? (
          <p className="text-sm text-gray-500 mt-2" data-testid="last-run-never">
            Never run.
          </p>
        ) : (
          <div className="mt-2 space-y-1" data-testid="last-run-attempt">
            <p className="text-sm text-gray-900">
              {attempt.status === "success" && "Success"}
              {attempt.status === "blocked" && "Blocked — deletion threshold exceeded"}
              {attempt.status === "failed" && "Failed — provider or system error"}
              {" · "}
              {attempt.trigger === "scheduled" ? "Automatic" : "Manual"}
              {" · "}
              {formatDateTime(attempt.finishedAt)}
            </p>
            {attempt.usersDeleting !== undefined && attempt.usersPercent !== undefined && (
              <p className="text-sm text-gray-700">
                User deletions: {attempt.usersDeleting} ({attempt.usersPercent.toFixed(1)}%)
              </p>
            )}
            {attempt.teamsDeleting !== undefined && attempt.teamsPercent !== undefined && (
              <p className="text-sm text-gray-700">
                Team deletions: {attempt.teamsDeleting} ({attempt.teamsPercent.toFixed(1)}%)
              </p>
            )}
            {attempt.thresholdPercent !== undefined && (
              <p className="text-xs text-gray-500">Configured threshold: {attempt.thresholdPercent}%</p>
            )}
            <p className="text-xs text-gray-500">
              {attempt.writesStatus === "applied" && "Changes were written."}
              {attempt.writesStatus === "none" && "No changes were written."}
              {attempt.writesStatus === "unknown" &&
                "The outcome could not be confirmed. Verify current user and team counts directly."}
            </p>
            {attempt.message && <p className="text-xs text-gray-500 mt-1">{attempt.message}</p>}
          </div>
        )}

        {/* A skip is shown as a secondary line and never replaces the headline
            result above -- a skip physically cannot overwrite the last attempt
            that actually ran, and the display must not either. */}
        {skip && (
          <p className="text-xs text-amber-700 mt-2" data-testid="last-run-skip">
            Most recently skipped {formatDateTime(skip.at)}: {SKIP_REASON_LABELS[skip.reason] ?? skip.reason}.
          </p>
        )}
      </div>
    </div>
  );
}
