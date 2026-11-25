"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { AlertCircle, CheckCircle2, Loader2 } from "lucide-react";
import {
  getAutoSyncSchedule,
  updateAutoSyncSchedule,
  type AutoSyncFrequency,
  type AutoSyncSchedule,
} from "@/lib/api/admin";

/** Status refresh cadence: quick while an automatic run is in flight. */
export const AUTO_SYNC_POLL_MS = 15000;
export const AUTO_SYNC_ACTIVE_POLL_MS = 2000;

const OPTIONS: { value: AutoSyncFrequency; label: string }[] = [
  { value: "DAILY", label: "Daily" },
  { value: "WEEKLY", label: "Weekly" },
  { value: "MONTHLY", label: "Monthly" },
];

/** "4 Nov 2026, 02:00 UTC" -- always UTC, always labelled. */
export function formatUtc(iso: string | null | undefined): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const date = d.toLocaleDateString("en-GB", { day: "numeric", month: "short", year: "numeric", timeZone: "UTC" });
  const time = d.toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit", hour12: false, timeZone: "UTC" });
  return `${date}, ${time} UTC`;
}

const STATUS_LABEL = { PENDING: "Running", RUNNING: "Running", SUCCESS: "Succeeded", FAILED: "Failed" } as const;

/**
 * Auto Sync settings, shown beside the manual Sync Now controls. The schedule
 * and its history live on the server: this component only edits and displays
 * them. Saving never starts a sync.
 */
export default function AutoSyncSettings() {
  const [schedule, setSchedule] = useState<AutoSyncSchedule | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [enabled, setEnabled] = useState(false);
  const [frequency, setFrequency] = useState<AutoSyncFrequency>("DAILY");
  const [dirty, setDirty] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const dirtyRef = useRef(false);
  dirtyRef.current = dirty;
  const mounted = useRef(true);

  const apply = useCallback((next: AutoSyncSchedule) => {
    setSchedule(next);
    // Never overwrite edits in progress with a background refresh.
    if (!dirtyRef.current) {
      setEnabled(next.enabled);
      setFrequency(next.frequency);
    }
  }, []);

  const refresh = useCallback(async () => {
    try {
      const next = await getAutoSyncSchedule();
      if (!mounted.current) return;
      setLoadError(null);
      apply(next);
    } catch (err) {
      if (mounted.current) setLoadError(err instanceof Error ? err.message : "Could not load auto sync settings");
    } finally {
      if (mounted.current) setLoading(false);
    }
  }, [apply]);

  const running =
    schedule?.lastAttempt?.status === "PENDING" || schedule?.lastAttempt?.status === "RUNNING";

  useEffect(() => {
    mounted.current = true;
    refresh();
    return () => {
      mounted.current = false;
    };
  }, [refresh]);

  useEffect(() => {
    const id = setInterval(refresh, running ? AUTO_SYNC_ACTIVE_POLL_MS : AUTO_SYNC_POLL_MS);
    return () => clearInterval(id);
  }, [refresh, running]);

  const onSave = async () => {
    setSaving(true);
    setSaveError(null);
    setSaved(false);
    try {
      const next = await updateAutoSyncSchedule(enabled, frequency);
      if (!mounted.current) return;
      dirtyRef.current = false;
      setDirty(false);
      setEnabled(next.enabled);
      setFrequency(next.frequency);
      setSchedule(next);
      setSaved(true);
    } catch (err) {
      if (mounted.current) setSaveError(err instanceof Error ? err.message : "Could not save auto sync settings");
    } finally {
      if (mounted.current) setSaving(false);
    }
  };

  if (loading) {
    return (
      <div className="mt-8" data-testid="auto-sync-loading">
        <p className="text-sm text-gray-500">Loading auto sync settings...</p>
      </div>
    );
  }

  const attempt = schedule?.lastAttempt ?? null;

  return (
    <div className="mt-8 border-t border-gray-200 pt-6" data-testid="auto-sync-settings">
      <h3 className="text-lg font-medium text-gray-900">Auto Sync</h3>
      <p className="text-sm text-gray-500 mt-1 mb-4">
        Runs the same sync as Sync Now on a schedule, with the same safeguards. Saving does not start a sync.
        Syncs missed while the service was down are skipped, not replayed.
      </p>

      {loadError && (
        <div role="alert" data-testid="auto-sync-load-error" className="flex items-start gap-2 text-sm text-red-700 mb-3">
          <AlertCircle className="w-4 h-4 mt-0.5" />
          <span>{loadError}</span>
        </div>
      )}

      {schedule && (
        <>
          <div className="flex flex-wrap items-center gap-4 mb-3">
            <label className="flex items-center gap-2 text-sm text-gray-800">
              <input
                type="checkbox"
                data-testid="auto-sync-toggle"
                checked={enabled}
                disabled={saving}
                onChange={(e) => {
                  setEnabled(e.target.checked);
                  setDirty(true);
                  setSaved(false);
                }}
              />
              Enable Auto Sync
            </label>
            <label className="flex items-center gap-2 text-sm text-gray-800">
              Frequency
              <select
                data-testid="auto-sync-frequency"
                value={frequency}
                disabled={saving}
                onChange={(e) => {
                  setFrequency(e.target.value as AutoSyncFrequency);
                  setDirty(true);
                  setSaved(false);
                }}
                className="border border-gray-300 rounded px-2 py-1"
              >
                {OPTIONS.map((o) => (
                  <option key={o.value} value={o.value}>
                    {o.label}
                  </option>
                ))}
              </select>
            </label>
            <button
              data-testid="auto-sync-save"
              onClick={onSave}
              disabled={saving || !dirty}
              className="flex items-center gap-2 px-4 py-2 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {saving && <Loader2 className="w-4 h-4 animate-spin" />}
              {saving ? "Saving..." : "Save"}
            </button>
          </div>

          {saveError && (
            <div role="alert" data-testid="auto-sync-save-error" className="flex items-start gap-2 text-sm text-red-700 mb-3">
              <AlertCircle className="w-4 h-4 mt-0.5" />
              <span>{saveError}</span>
            </div>
          )}
          {saved && !dirty && (
            <p role="status" data-testid="auto-sync-saved" className="flex items-center gap-2 text-sm text-green-700 mb-3">
              <CheckCircle2 className="w-4 h-4" /> Auto sync settings saved.
            </p>
          )}

          <dl className="text-sm text-gray-700 space-y-1" data-testid="auto-sync-status">
            <div className="flex gap-2">
              <dt className="font-medium">Schedule:</dt>
              <dd data-testid="auto-sync-schedule">
                {schedule.enabled ? `${schedule.description} (${schedule.timezone})` : `Not scheduled (${schedule.timezone})`}
              </dd>
            </div>
            <div className="flex gap-2">
              <dt className="font-medium">Last automatic attempt:</dt>
              <dd data-testid="auto-sync-last-attempt">
                {attempt
                  ? `${formatUtc(attempt.startedAt)} - ${STATUS_LABEL[attempt.status]}`
                  : "Never synced automatically"}
              </dd>
            </div>
            {attempt?.status === "FAILED" && attempt.errorMessage && (
              <div className="flex gap-2 text-red-700">
                <dt className="font-medium">Error:</dt>
                <dd data-testid="auto-sync-last-error">{attempt.errorMessage}</dd>
              </div>
            )}
            <div className="flex gap-2">
              <dt className="font-medium">Last successful automatic sync:</dt>
              <dd data-testid="auto-sync-last-success">
                {schedule.lastSuccessAt ? formatUtc(schedule.lastSuccessAt) : "Never synced automatically"}
              </dd>
            </div>
            <div className="flex gap-2">
              <dt className="font-medium">Next automatic sync:</dt>
              <dd data-testid="auto-sync-next">
                {schedule.enabled && schedule.nextRunAt ? formatUtc(schedule.nextRunAt) : "Auto-sync disabled"}
              </dd>
            </div>
          </dl>
        </>
      )}
    </div>
  );
}
