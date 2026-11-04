"use client";

import { Check, Loader2, X } from "lucide-react";
import {
  SYNC_STEPS,
  describeRun,
  segmentStates,
  stepStates,
  type ProgressRun,
  type SegmentState,
  type StepState,
} from "@/lib/sync-progress";

interface Props {
  run: ProgressRun | null;
  /** Shown beneath the bar when the run failed. Already safe for admins. */
  errorMessage?: string | null;
}

const STEP_WORDS: Record<StepState, string> = {
  pending: "not started",
  active: "in progress",
  complete: "complete",
  failed: "failed",
};

function Dot({ state }: { state: StepState }) {
  const base = "flex h-8 w-8 items-center justify-center rounded-full border-2 transition-colors duration-300";
  if (state === "complete") {
    return (
      <span className={`${base} border-green-600 bg-green-600 text-white`} data-testid="step-icon-complete">
        <Check className="h-4 w-4" aria-hidden="true" />
      </span>
    );
  }
  if (state === "failed") {
    return (
      <span className={`${base} border-red-600 bg-red-600 text-white`} data-testid="step-icon-failed">
        <X className="h-4 w-4" aria-hidden="true" />
      </span>
    );
  }
  if (state === "active") {
    return (
      <span className={`${base} border-green-600 bg-white text-green-600`} data-testid="step-icon-active">
        <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
      </span>
    );
  }
  return <span className={`${base} border-gray-300 bg-white`} data-testid="step-icon-pending" />;
}

function Segment({ state }: { state: SegmentState }) {
  // Gray until its left step completes; fills green; while that step's work is
  // in flight it is half-green and pulsing, so "in progress" reads as motion.
  return (
    <span
      data-testid="progress-segment"
      data-state={state}
      aria-hidden="true"
      className="relative mx-1 mb-6 h-1 flex-1 overflow-hidden rounded bg-gray-300"
    >
      <span
        className={`absolute inset-y-0 left-0 rounded bg-green-600 transition-all duration-500 ${
          state === "green" ? "w-full" : state === "progress" ? "w-1/2 animate-pulse" : "w-0"
        }`}
      />
    </span>
  );
}

/**
 * Horizontal six-step progress bar for a sync run. It is a pure function of the
 * run, so a refreshed page that reloads the run renders the identical bar.
 */
export default function SyncProgressBar({ run, errorMessage }: Props) {
  const steps = stepStates(run);
  const segments = segmentStates(run);

  return (
    <div data-testid="sync-progress" className="mt-4">
      <p className="sr-only" role="status" aria-live="polite" data-testid="sync-progress-status">
        {describeRun(run)}
      </p>
      <ol aria-label="Synchronization progress" className="flex items-start">
        {SYNC_STEPS.map((label, i) => (
          <li
            key={label}
            className={`flex items-start ${i < SYNC_STEPS.length - 1 ? "flex-1" : ""}`}
            data-testid={`sync-step-${i}`}
            data-state={steps[i]}
            aria-current={steps[i] === "active" ? "step" : undefined}
          >
            <div className="flex w-20 flex-col items-center text-center">
              <Dot state={steps[i]} />
              <span
                className={`mt-1 text-xs ${
                  steps[i] === "failed" ? "font-medium text-red-700" : steps[i] === "pending" ? "text-gray-500" : "text-gray-900"
                }`}
              >
                {label}
                {/* Words, not just colour, so state is clear to screen readers. */}
                <span className="sr-only"> — {STEP_WORDS[steps[i]]}</span>
              </span>
            </div>
            {i < SYNC_STEPS.length - 1 && <Segment state={segments[i]} />}
          </li>
        ))}
      </ol>
      {run?.status === "FAILED" && errorMessage && (
        <p role="alert" data-testid="sync-progress-error" className="mt-2 text-sm text-red-700">
          {errorMessage}
        </p>
      )}
    </div>
  );
}
