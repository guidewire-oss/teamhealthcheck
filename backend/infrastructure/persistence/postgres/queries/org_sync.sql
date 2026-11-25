-- Queries for the org-sync stores:
--   org_sync_hold_repository.go, org_sync_schedule_repository.go
--   (org_sync_run_repository.go queries are in org_sync_run.sql)
-- Do not hand-edit sqlcgen/*.go - regenerate with `sqlc generate` (see Makefile).

-- name: GetOrgSyncHold :one
SELECT threshold, report, held_at FROM org_sync_hold WHERE id = 1;

-- name: UpsertOrgSyncHold :exec
INSERT INTO org_sync_hold (id, threshold, report, held_at)
VALUES (1, sqlc.arg(threshold), sqlc.arg(report), sqlc.arg(held_at))
ON CONFLICT (id) DO UPDATE
SET threshold = EXCLUDED.threshold, report = EXCLUDED.report, held_at = EXCLUDED.held_at;

-- name: DeleteOrgSyncHold :execrows
DELETE FROM org_sync_hold WHERE id = 1;

-- name: ClearLatestOrgSyncRunHoldReport :exec
-- Drops the hold report of the run that raised the hold (the newest run
-- carrying one). Older runs keep their reports as history.
UPDATE org_sync_runs SET hold_report = NULL
WHERE id = (SELECT id FROM org_sync_runs WHERE hold_report IS NOT NULL
            ORDER BY created_at DESC, id DESC LIMIT 1);

-- name: GetOrgSyncSchedule :one
SELECT enabled, frequency, next_run_at, updated_at, updated_by FROM org_sync_schedule WHERE id = 1;

-- name: UpdateOrgSyncSchedule :exec
UPDATE org_sync_schedule
SET enabled = sqlc.arg(enabled), frequency = sqlc.arg(frequency), next_run_at = sqlc.narg(next_run_at),
    updated_at = CURRENT_TIMESTAMP, updated_by = sqlc.narg(updated_by)
WHERE id = 1;

-- name: ClaimOrgSyncSchedule :execrows
-- Compare-and-swap on next_run_at. Re-checking enabled means a registration
-- that outlived a "disable" cannot launch a run.
UPDATE org_sync_schedule SET next_run_at = sqlc.arg(next_run_at)
WHERE id = 1 AND enabled AND next_run_at = sqlc.arg(due);
