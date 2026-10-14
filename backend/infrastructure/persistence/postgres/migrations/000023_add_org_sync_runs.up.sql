-- Durable log of organization-sync activity: ONE ROW PER SYNC REQUEST.
--
-- Every sync attempt that ran and reached a conclusion (success, blocked,
-- failed) and every skipped scheduled occurrence adds a row, identified by
-- run_id -- the same UUID written on that request's log lines and returned to
-- the API caller. Rows are never overwritten; "newest" means the latest
-- finished_at. The application keeps the newest 1000 rows and prunes older
-- ones on insert.
--
-- This is an event log written by the sync itself, not an admin setting, so
-- it deliberately does NOT live on app_settings: a completed sync would
-- otherwise bump app_settings.updated_at, the column the settings endpoints
-- use to reason about admin edits.
--
-- There is deliberately no "running" status: an in-flight run is tracked only
-- in memory by the sync service, and its row is written when it finishes.
--
-- A table has no inherent order. To read newest-first, ORDER BY finished_at
-- DESC, or select from the org_sync_runs_latest view below.
CREATE TABLE IF NOT EXISTS org_sync_runs (
    -- One row per request, so the run ID is the key: it names exactly one row.
    run_id UUID PRIMARY KEY,

    trigger TEXT NOT NULL
        CONSTRAINT org_sync_runs_trigger_allowed
            CHECK (trigger IN ('manual', 'scheduled')),
    status TEXT NOT NULL
        CONSTRAINT org_sync_runs_status_allowed
            CHECK (status IN ('success', 'blocked', 'failed', 'skipped')),
    started_at  TIMESTAMP WITH TIME ZONE NOT NULL,
    finished_at TIMESTAMP WITH TIME ZONE NOT NULL,
    instance_id TEXT DEFAULT NULL,

    -- The threshold THAT attempt was judged against, stored rather than
    -- re-resolved, so changing the setting cannot retroactively re-label a
    -- blocked run. NULL when the attempt failed before a threshold was ever
    -- evaluated, and for skips.
    threshold_percent DOUBLE PRECISION DEFAULT NULL
        CONSTRAINT org_sync_runs_threshold_range
            CHECK (threshold_percent IS NULL
                   OR (threshold_percent >= 1 AND threshold_percent <= 100)),

    -- Nullable, never 0: a failed attempt never computed these, and 0 would
    -- read as a safe sync -- a lie. Set only for blocked/success outcomes.
    users_existing INTEGER DEFAULT NULL,
    users_deleting INTEGER DEFAULT NULL,
    users_percent  DOUBLE PRECISION DEFAULT NULL,
    teams_existing INTEGER DEFAULT NULL,
    teams_deleting INTEGER DEFAULT NULL,
    teams_percent  DOUBLE PRECISION DEFAULT NULL,

    -- Tri-state, not boolean: 'none' is a confirmed rollback, a blocked
    -- attempt or a skip (no write was ever attempted); 'applied' means
    -- Commit() returned successfully (independent of whether any row actually
    -- changed); 'unknown' means Commit() itself errored or the connection was
    -- lost at that exact moment, so the outcome cannot be proven either way.
    writes_status TEXT NOT NULL DEFAULT 'none'
        CONSTRAINT org_sync_runs_writes_status_allowed
            CHECK (writes_status IN ('none', 'applied', 'unknown')),
    override_used BOOLEAN NOT NULL DEFAULT false,

    users_synced INTEGER DEFAULT NULL,
    teams_synced INTEGER DEFAULT NULL,

    -- Set exactly when status = 'skipped'.
    skip_reason TEXT DEFAULT NULL
        CONSTRAINT org_sync_runs_skip_reason_allowed
            CHECK (skip_reason IS NULL OR skip_reason IN
                   ('manual_sync_running', 'scheduled_sync_running', 'hold_unresolved', 'overdue_catch_up_skipped')),
    message TEXT DEFAULT NULL,

    -- Durable mass-deletion hold: freezes threshold editing (via LockState's
    -- fail-closed OR with the in-memory hold) on every replica, closing the
    -- bypass where an admin resolves a hold on one replica by editing the
    -- threshold on another. Set true only on a blocked row; cleared (on every
    -- row that holds it) by a successful sync or an explicit admin dismissal,
    -- or ignored once it is older than the hold TTL -- never cleared by an
    -- unrelated later failure, which would silently reopen the bypass.
    hold_active      BOOLEAN NOT NULL DEFAULT false
        CONSTRAINT org_sync_runs_hold_only_on_blocked
            CHECK (NOT hold_active OR status = 'blocked'),
    hold_recorded_at TIMESTAMP WITH TIME ZONE DEFAULT NULL,

    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT org_sync_runs_skip_reason_matches_status
        CHECK ((status = 'skipped') = (skip_reason IS NOT NULL))
);

-- The newest rows are read constantly (last result, hold lookup, history).
-- created_at and run_id break a tie between rows with the same finished_at.
CREATE INDEX IF NOT EXISTS idx_org_sync_runs_newest ON org_sync_runs (finished_at DESC, created_at DESC, run_id DESC);
CREATE INDEX IF NOT EXISTS idx_org_sync_runs_hold_active ON org_sync_runs (finished_at DESC) WHERE hold_active;

-- Newest sync first, for anyone browsing the table in a SQL client.
CREATE OR REPLACE VIEW org_sync_runs_latest AS
    SELECT * FROM org_sync_runs ORDER BY finished_at DESC, created_at DESC, run_id DESC;
