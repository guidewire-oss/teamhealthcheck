/**
 * Maps a persisted sync run onto the six-step progress bar.
 *
 * The backend run row is the single source of truth (status + stage +
 * failedStage), so the same run always renders the same bar -- including after
 * a browser refresh, which is just a re-read of that row.
 */

export type SyncStatus = "PENDING" | "RUNNING" | "SUCCESS" | "FAILED";

export const SYNC_STEPS = [
  "Sync started",
  "Data loaded",
  "Running sync",
  "Queries updated",
  "Checking values",
  "Success",
] as const;

export type StepState = "pending" | "active" | "complete" | "failed";

/** The piece of a run the bar needs. */
export interface ProgressRun {
  status: SyncStatus;
  stage: string;
  failedStage?: string | null;
}

/**
 * Index (0-4) of the step a backend stage belongs to. The last step, "Success",
 * has no stage of its own: it is reached only by status SUCCESS.
 */
const STAGE_TO_STEP: Record<string, number> = {
  QUEUED: 0,
  ACQUIRING_LOCK: 0,
  FETCHING: 1,
  VALIDATING: 1,
  UPSERTING_USERS: 2,
  UPSERTING_TEAMS: 2,
  SYNCING_MEMBERSHIPS: 3,
  RECONCILING_REMOVALS: 3,
  COMMITTING: 4,
};

function stepForStage(stage: string | null | undefined): number {
  if (stage && stage in STAGE_TO_STEP) return STAGE_TO_STEP[stage];
  return 0;
}

/** The state of each of the six steps for a run. */
export function stepStates(run: ProgressRun | null): StepState[] {
  const states: StepState[] = SYNC_STEPS.map(() => "pending");
  if (!run) return states;

  if (run.status === "SUCCESS") {
    return states.map(() => "complete");
  }

  if (run.status === "FAILED") {
    // The step the run died in turns red; everything before it stays green and
    // everything after stays incomplete. A failure with no recorded stage is
    // attributed to the first step rather than guessed further along.
    const failedAt = stepForStage(run.failedStage);
    return states.map((_, i) => (i < failedAt ? "complete" : i === failedAt ? "failed" : "pending"));
  }

  const current = stepForStage(run.stage);
  return states.map((_, i) => (i < current ? "complete" : i === current ? "active" : "pending"));
}

/**
 * State of the line segment AFTER step `i` (between step i and i+1).
 * It turns green once the step before it is complete, animates while that step
 * is in flight, and stays gray otherwise -- which gives the left-to-right fill.
 */
export type SegmentState = "gray" | "progress" | "green";

export function segmentStates(run: ProgressRun | null): SegmentState[] {
  const steps = stepStates(run);
  return steps.slice(0, -1).map((s) => (s === "complete" ? "green" : s === "active" ? "progress" : "gray"));
}

/** Human description for screen readers and the status line. */
export function describeRun(run: ProgressRun | null): string {
  if (!run) return "No synchronization has run yet";
  switch (run.status) {
    case "PENDING":
      return "Pending: waiting to start";
    case "RUNNING": {
      const step = SYNC_STEPS[stepForStage(run.stage)];
      return `Running: ${step}`;
    }
    case "SUCCESS":
      return "Success: synchronization completed";
    case "FAILED": {
      const step = SYNC_STEPS[stepForStage(run.failedStage)];
      return `Failed at step: ${step}`;
    }
  }
}

export function isActive(run: ProgressRun | null): boolean {
  return !!run && (run.status === "PENDING" || run.status === "RUNNING");
}

const STATUS_RANK: Record<SyncStatus, number> = { PENDING: 0, RUNNING: 1, SUCCESS: 2, FAILED: 2 };

export function isTerminal(run: { status: SyncStatus } | null): boolean {
  return !!run && (run.status === "SUCCESS" || run.status === "FAILED");
}

/**
 * Decides what to show when a response about a run arrives. Responses can be
 * late or out of order, so a response may only move the screen forward:
 *
 *  - a response about a different run than the one being followed is ignored;
 *  - once the followed run is SUCCESS or FAILED nothing may change it, so a slow
 *    "RUNNING" answer can never bring the stuck-running state back;
 *  - a response that would move the same run backwards (RUNNING -> PENDING) is
 *    ignored.
 */
export function mergeRun<T extends { runId: string; status: SyncStatus }>(current: T | null, incoming: T): T | null {
  if (!current || current.runId !== incoming.runId) return current;
  if (isTerminal(current)) return current;
  if (STATUS_RANK[incoming.status] < STATUS_RANK[current.status]) return current;
  return incoming;
}
