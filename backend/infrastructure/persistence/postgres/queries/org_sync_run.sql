-- Queries for OrgSyncRunRepository (org_sync_run_repository.go).
-- SELECT * is deliberate: sqlc expands it to every org_sync_runs column, so all
-- run reads share the generated OrgSyncRun model instead of one row type each.
-- Do not hand-edit sqlcgen/*.go - regenerate with `sqlc generate` (see Makefile).

-- name: FindOrgSyncRunByIdempotencyKey :one
SELECT * FROM org_sync_runs WHERE idempotency_key = sqlc.arg(idempotency_key);

-- name: InsertPendingOrgSyncRun :exec
-- unchanged_recorded keeps its default FALSE: a new run has not recorded its
-- unchanged counts yet (see migration 000033).
INSERT INTO org_sync_runs (id, status, stage, initiated_by, idempotency_key, snapshot_type, trigger_type)
VALUES (sqlc.arg(id), 'PENDING', sqlc.arg(stage), sqlc.arg(initiated_by), sqlc.narg(idempotency_key), sqlc.arg(snapshot_type), sqlc.arg(trigger_type));

-- name: FindActiveOrgSyncRun :one
SELECT * FROM org_sync_runs
WHERE status = ANY(sqlc.arg(active_statuses)::text[])
ORDER BY created_at DESC LIMIT 1;

-- name: GetOrgSyncRun :one
SELECT * FROM org_sync_runs WHERE id = sqlc.arg(id);

-- name: FindLatestOrgSyncRun :one
SELECT * FROM org_sync_runs ORDER BY created_at DESC LIMIT 1;

-- name: FindLatestOrgSyncRunByTrigger :one
SELECT * FROM org_sync_runs WHERE trigger_type = sqlc.arg(trigger_type) ORDER BY created_at DESC LIMIT 1;

-- name: FindLatestSuccessfulOrgSyncRunByTrigger :one
SELECT * FROM org_sync_runs
WHERE trigger_type = sqlc.arg(trigger_type) AND status = 'SUCCESS'
ORDER BY completed_at DESC NULLS LAST, created_at DESC LIMIT 1;

-- name: CountOrgSyncRuns :one
-- A NULL filter disables its predicate, so one static query serves every
-- combination. The table holds one row per sync attempt, so filtered scans stay small.
SELECT COUNT(*) FROM org_sync_runs
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
  AND (sqlc.narg(trigger_type)::text IS NULL OR trigger_type = sqlc.narg(trigger_type)::text)
  AND (sqlc.narg(snapshot_type)::text IS NULL OR snapshot_type = sqlc.narg(snapshot_type)::text);

-- name: ListOrgSyncRuns :many
-- Newest first; id is the stable tiebreaker. Covered by idx_org_sync_runs_created
-- for the unfiltered case.
SELECT * FROM org_sync_runs
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
  AND (sqlc.narg(trigger_type)::text IS NULL OR trigger_type = sqlc.narg(trigger_type)::text)
  AND (sqlc.narg(snapshot_type)::text IS NULL OR snapshot_type = sqlc.narg(snapshot_type)::text)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit)::bigint OFFSET sqlc.arg(page_offset)::bigint;

-- name: SetOrgSyncRunStage :execrows
-- Records progress and refreshes the heartbeat on a non-terminal run.
UPDATE org_sync_runs SET stage = sqlc.arg(stage), heartbeat_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id) AND status = ANY(sqlc.arg(allowed_statuses)::text[]);

-- name: MarkOrgSyncRunRunning :execrows
UPDATE org_sync_runs SET status = 'RUNNING', started_at = CURRENT_TIMESTAMP,
    heartbeat_at = CURRENT_TIMESTAMP, worker_id = sqlc.arg(worker_id)
WHERE id = sqlc.arg(id) AND status = ANY(sqlc.arg(allowed_statuses)::text[]);

-- name: FailOrgSyncRun :execrows
UPDATE org_sync_runs SET status = 'FAILED', stage = sqlc.arg(stage), failed_stage = sqlc.arg(failed_stage),
    error_code = sqlc.arg(error_code), error_message = sqlc.arg(error_message), hold_report = sqlc.narg(hold_report),
    completed_at = CURRENT_TIMESTAMP, heartbeat_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id) AND status = ANY(sqlc.arg(allowed_statuses)::text[]);

-- name: RecoverStaleOrgSyncRuns :execrows
-- Fails runs abandoned by a crashed worker so the single-active index frees up.
UPDATE org_sync_runs SET status = 'FAILED', failed_stage = stage, stage = sqlc.arg(failed_stage_name),
    error_code = sqlc.arg(error_code), error_message = 'The synchronization stopped responding and was marked failed. It is safe to run it again.',
    completed_at = CURRENT_TIMESTAMP
WHERE (status = 'RUNNING' AND COALESCE(heartbeat_at, started_at, created_at) < NOW() - make_interval(secs => sqlc.arg(ttl_seconds)::float8))
   OR (status = 'PENDING' AND created_at < NOW() - make_interval(secs => sqlc.arg(ttl_seconds)::float8));

-- name: TryOrgSyncAdvisoryLock :one
-- Session-level lock: the caller must run this and the unlock on the SAME
-- connection (sqlcgen.New over a *sql.Conn).
SELECT pg_try_advisory_lock(sqlc.arg(lock_key)::bigint);

-- name: OrgSyncAdvisoryUnlock :one
SELECT pg_advisory_unlock(sqlc.arg(lock_key)::bigint);
