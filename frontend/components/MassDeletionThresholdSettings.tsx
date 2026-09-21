"use client";

import { useEffect, useState } from "react";
import { AlertCircle, CheckCircle2, Lock, ShieldAlert } from "lucide-react";
import {
  getOrgSyncDeletionThreshold,
  updateOrgSyncDeletionThreshold,
  OrgSyncDeletionThreshold,
  THRESHOLD_LOCKED_CODE,
} from "@/lib/api/admin";

/**
 * Admin control for the organization-sync mass-deletion threshold.
 *
 * The threshold is the share of the existing non-protected users -- or teams --
 * that one sync may remove before the whole sync is held for review. It is a
 * safety limit, never a credential, and saving it neither runs a sync nor
 * waives a hold: only the explicit Sync Anyway action does that.
 *
 * Saving is deliberately explicit. A slider that autosaved would let a stray
 * drag quietly widen a destructive-action guard.
 */

/** Values admins reach for most often, offered as one-click shortcuts. */
const PRESETS = [10, 20, 50, 100];

/** At or above this, the threshold stops being much of a guard -- say so. */
const HIGH_THRESHOLD = 50;

/** Explains why editing is disabled, in the terms the admin is looking at. */
function lockExplanation(config: OrgSyncDeletionThreshold): string {
  const at =
    config.activeSyncThreshold !== undefined
      ? ` That sync is being judged at ${config.activeSyncThreshold}%.`
      : "";
  if (config.lockReason === "syncing") {
    return `A sync is running, so the threshold cannot be changed until it finishes.${at}`;
  }
  return `A sync is held for mass-deletion review, so the threshold is frozen until that hold is resolved -- apply it with Sync Anyway, or dismiss it.${at}`;
}

/** Explains where the value currently in force came from. */
function sourceHint(config: OrgSyncDeletionThreshold): string {
  switch (config.source) {
    case "admin":
      return "Currently using the value saved here.";
    case "environment":
      return "Currently using ORG_SYNC_MAX_DELETE_PERCENT from the API service. Saving here overrides it.";
    default:
      return `Currently using the built-in default of ${config.defaultPercent}%.`;
  }
}

/**
 * `syncActivity` lets the parent tell this card that a sync it started has
 * begun or ended, so the lock reflects the current tab immediately instead of
 * waiting for the next fetch. The backend remains the source of truth: the
 * card re-reads it whenever that signal changes, on mount, and on window
 * focus, which is how a reload or a second tab learns about a lock it did not
 * cause.
 */
export default function MassDeletionThresholdSettings({
  syncActivity = false,
}: {
  syncActivity?: boolean;
}) {
  const [config, setConfig] = useState<OrgSyncDeletionThreshold | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);

  // `percent` is the canonical, always-valid value the slider renders and Save
  // sends. `draft` is the text in the numeric field, which can be mid-edit and
  // therefore temporarily invalid; the two are reconciled on every valid edit,
  // so what the slider shows and what a valid field says are never different.
  const [percent, setPercent] = useState(20);
  const [draft, setDraft] = useState("20");

  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  const min = config?.minPercent ?? 1;
  const max = config?.maxPercent ?? 100;

  // Locked by the server's own view, or by a sync this tab has just started
  // and whose result the server has not been asked about yet.
  const locked = syncActivity || !!config?.locked;

  /**
   * Reads the server's answer. Only the first read shows a loading state: a
   * refresh triggered by a sync starting or the window regaining focus must
   * not blank the card the admin is looking at.
   */
  const load = async () => {
    setLoadError(null);
    try {
      const loaded = await getOrgSyncDeletionThreshold();
      setConfig(loaded);
      setPercent(loaded.maxDeletePercent);
      setDraft(String(loaded.maxDeletePercent));
    } catch (err: any) {
      setLoadError(err?.message || "Failed to load the deletion threshold");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    // Re-read on mount and whenever this tab's sync starts or finishes, so a
    // reload, a navigation, or a sync in another tab all land on the server's
    // current answer rather than on local memory.
    load();
  }, [syncActivity]);

  useEffect(() => {
    // Another tab may have started, resolved, or overridden a sync while this
    // one sat in the background; refreshing on focus picks that up.
    const onFocus = () => load();
    window.addEventListener("focus", onFocus);
    return () => window.removeEventListener("focus", onFocus);
  }, []);

  /** Whether `draft` is a whole percentage inside the allowed range. */
  const draftValid = (() => {
    const trimmed = draft.trim();
    if (trimmed === "") return false;
    const value = Number(trimmed);
    return Number.isFinite(value) && Number.isInteger(value) && value >= min && value <= max;
  })();

  const applyValue = (next: number) => {
    setPercent(next);
    setDraft(String(next));
    setSaved(false);
    setSaveError(null);
  };

  const handleDraftChange = (text: string) => {
    setDraft(text);
    setSaved(false);
    setSaveError(null);
    const value = Number(text.trim());
    if (text.trim() !== "" && Number.isFinite(value) && Number.isInteger(value) && value >= min && value <= max) {
      setPercent(value);
    }
  };

  const handleSave = async () => {
    if (!draftValid || saving) return;
    setSaving(true);
    setSaveError(null);
    setSaved(false);
    try {
      const updated = await updateOrgSyncDeletionThreshold(percent);
      setConfig(updated);
      setPercent(updated.maxDeletePercent);
      setDraft(String(updated.maxDeletePercent));
      setSaved(true);
    } catch (err: any) {
      setSaveError(err?.message || "Failed to save the deletion threshold");
      if (err?.apiError?.code === THRESHOLD_LOCKED_CODE || err?.statusCode === 409) {
        // A sync started in another tab between this tab's last read and the
        // click. Re-read so the controls disable rather than inviting a retry
        // the backend will refuse again.
        load();
      }
    } finally {
      setSaving(false);
    }
  };

  const dirty = !!config && draftValid && percent !== config.maxDeletePercent;

  return (
    <div className="mt-6 pt-6 border-t" data-testid="mass-deletion-threshold-settings">
      <h4 className="text-base font-medium text-gray-900">Mass-deletion protection</h4>
      <p className="text-sm text-gray-500 mt-1">
        A sync is held for review when it would delete more than this share of the existing users
        &mdash; or of the existing teams. Either one going over holds the whole sync, and nothing is
        written until an administrator reviews the counts.
      </p>

      {loading ? (
        <p className="text-sm text-gray-500 mt-4" data-testid="threshold-loading">
          Loading current threshold...
        </p>
      ) : loadError ? (
        <div
          data-testid="threshold-load-error"
          className="mt-4 flex items-start gap-3 p-4 bg-red-50 border border-red-200 rounded-lg"
        >
          <AlertCircle className="w-5 h-5 text-red-600 mt-0.5 flex-shrink-0" />
          <div>
            <p className="text-sm font-medium text-red-900">Couldn&apos;t load the threshold</p>
            <p className="text-sm text-red-700 mt-1">{loadError}</p>
            <button onClick={load} className="text-sm text-red-800 underline mt-2">
              Retry
            </button>
          </div>
        </div>
      ) : (
        <div className="mt-4 space-y-4">
          {locked && (
            <div
              data-testid="threshold-locked-notice"
              role="status"
              className="flex items-start gap-3 p-3 bg-amber-50 border border-amber-200 rounded-lg"
            >
              <Lock className="w-5 h-5 text-amber-600 mt-0.5 flex-shrink-0" />
              <p className="text-sm text-amber-800">
                {config ? lockExplanation(config) : "A sync is running, so the threshold cannot be changed until it finishes."}
              </p>
            </div>
          )}

          <div className="flex items-center gap-4">
            <input
              type="range"
              min={min}
              max={max}
              step={1}
              value={percent}
              data-testid="threshold-slider"
              aria-label="Maximum deletion percentage"
              aria-valuetext={`${percent} percent`}
              onChange={(e) => applyValue(Number(e.target.value))}
              disabled={locked}
              className="flex-1 accent-indigo-600 disabled:opacity-50 disabled:cursor-not-allowed"
            />
            <div className="flex items-center gap-2">
              <label htmlFor="threshold-percent-input" className="sr-only">
                Maximum deletion percentage
              </label>
              <input
                id="threshold-percent-input"
                type="number"
                inputMode="numeric"
                min={min}
                max={max}
                step={1}
                value={draft}
                data-testid="threshold-input"
                aria-invalid={!draftValid}
                onChange={(e) => handleDraftChange(e.target.value)}
                disabled={locked}
                className="w-20 px-3 py-2 border border-gray-300 rounded-lg text-gray-900 focus:ring-2 focus:ring-indigo-500 focus:border-transparent disabled:bg-gray-100 disabled:opacity-50 disabled:cursor-not-allowed"
              />
              <span className="text-gray-700">%</span>
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            {PRESETS.map((preset) => (
              <button
                key={preset}
                type="button"
                data-testid={`threshold-preset-${preset}`}
                onClick={() => applyValue(preset)}
                aria-pressed={percent === preset}
                disabled={locked}
                className={`px-2.5 py-1 text-sm rounded-lg border transition-colors disabled:opacity-50 disabled:cursor-not-allowed ${
                  percent === preset
                    ? "bg-indigo-600 text-white border-indigo-600"
                    : "bg-white text-gray-700 border-gray-300 hover:bg-gray-50"
                }`}
              >
                {preset}%
              </button>
            ))}
            <span className="text-xs text-gray-500 ml-1" data-testid="threshold-recommended-hint">
              {config?.defaultPercent ?? 20}% recommended
            </span>
          </div>

          {!draftValid && (
            <p className="text-sm text-red-600" data-testid="threshold-validation-error" role="alert">
              Enter a whole number between {min} and {max}.
            </p>
          )}

          {draftValid && percent >= HIGH_THRESHOLD && (
            <div
              data-testid="threshold-high-warning"
              role="alert"
              className="flex items-start gap-3 p-3 bg-amber-50 border border-amber-200 rounded-lg"
            >
              <ShieldAlert className="w-5 h-5 text-amber-600 mt-0.5 flex-shrink-0" />
              <p className="text-sm text-amber-800">
                {percent >= 100
                  ? "At 100% a sync can delete every user and team the provider stops reporting, without being held for review."
                  : `At ${percent}% a sync can delete up to ${percent}% of your users or teams before anyone is asked to review it.`}
              </p>
            </div>
          )}

          <p className="text-xs text-gray-500" data-testid="threshold-source">
            {config ? sourceHint(config) : ""} The permanent admin and the fixed demo/E2E accounts
            and teams are never deleted by a sync and are excluded from this calculation.
          </p>

          {saveError && (
            <p className="text-sm text-red-600" data-testid="threshold-save-error" role="alert">
              {saveError}
            </p>
          )}

          {saved && (
            <p
              className="text-sm text-green-700 flex items-center gap-1"
              data-testid="threshold-save-success"
            >
              <CheckCircle2 className="w-4 h-4" />
              Threshold saved
            </p>
          )}

          <button
            type="button"
            onClick={handleSave}
            disabled={locked || saving || !draftValid || !dirty}
            data-testid="threshold-save-btn"
            className="px-4 py-2 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {saving ? "Saving..." : "Save threshold"}
          </button>
        </div>
      )}
    </div>
  );
}
