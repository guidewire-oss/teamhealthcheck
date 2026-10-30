"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  getLatestSyncRun,
  getSyncRun,
  syncOrganizationProvider,
  type ConfirmedMassDeletion,
  type SyncRun,
} from "@/lib/api/admin";
import { isActive, mergeRun } from "@/lib/sync-progress";

export const SYNC_POLL_INTERVAL_MS = 1500;

interface StartOptions {
  overrideMassDeletion?: boolean;
  confirmedMassDeletion?: ConfirmedMassDeletion;
}

/**
 * Follows the persisted organization sync run.
 *
 * The backend row is the only source of truth, so there is no localStorage
 * record to go stale: on mount the latest run is simply read again, which is
 * what restores the right state after a browser refresh or a second tab.
 *
 * Polling runs only while the run is PENDING or RUNNING and stops at SUCCESS or
 * FAILED. A failed poll never changes the run, so a network error can never
 * look like success; it is surfaced separately as `pollError` and polling
 * continues.
 */
export function useSyncRun(pollMs: number = SYNC_POLL_INTERVAL_MS) {
  const [run, setRun] = useState<SyncRun | null>(null);
  const [loading, setLoading] = useState(true);
  const [starting, setStarting] = useState(false);
  const [startError, setStartError] = useState<string | null>(null);
  const [pollError, setPollError] = useState<string | null>(null);
  // Bumped after a failed poll so the effect below schedules the next attempt.
  const [retryTick, setRetryTick] = useState(0);
  const mounted = useRef(true);

  useEffect(() => {
    mounted.current = true;
    getLatestSyncRun()
      .then((latest) => {
        // If the admin already started a run while this was loading, that run is
        // newer than "the last one" and must not be replaced by it.
        if (mounted.current) setRun((prev) => prev ?? latest);
      })
      .catch((err) => {
        if (mounted.current) setPollError(err?.message || "Could not load the last sync status");
      })
      .finally(() => {
        if (mounted.current) setLoading(false);
      });
    return () => {
      mounted.current = false;
    };
  }, []);

  const runId = run?.runId;
  const active = isActive(run);
  useEffect(() => {
    if (!runId || !active) return;
    const timer = setTimeout(async () => {
      try {
        const next = await getSyncRun(runId);
        if (!mounted.current) return;
        setPollError(null);
        // A late or out-of-order answer may only move the run forward.
        setRun((prev) => mergeRun(prev, next));
        // Always schedule the next look: the merge can keep the same object, and
        // an unchanged identity would otherwise stall polling of a live run.
        setRetryTick((n) => n + 1);
      } catch (err: any) {
        if (!mounted.current) return;
        // Keep the last known run untouched and try again.
        setPollError(err?.message || "Lost contact with the server; retrying");
        setRetryTick((n) => n + 1);
      }
    }, pollMs);
    return () => clearTimeout(timer);
    // `run` identity changes on every successful poll, which is what schedules the next one.
  }, [runId, active, run, retryTick, pollMs]);

  const start = useCallback(
    async (options: StartOptions = {}) => {
      setStarting(true);
      setStartError(null);
      try {
        const created = await syncOrganizationProvider(options);
        if (mounted.current) setRun((prev) => (prev && prev.runId === created.runId ? mergeRun(prev, created) : created));
      } catch (err: any) {
        // Nothing was accepted, so the previous run is left exactly as it was.
        if (mounted.current) setStartError(err?.message || "Could not start the synchronization");
      } finally {
        if (mounted.current) setStarting(false);
      }
    },
    []
  );

  return { run, loading, starting, startError, pollError, start };
}
