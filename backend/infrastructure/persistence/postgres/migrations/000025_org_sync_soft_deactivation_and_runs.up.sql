-- Organization sync no longer hard-deletes people or teams. Removal from a
-- verified complete provider snapshot is recorded as inactivity instead, so
-- survey sessions, responses and action items keep resolving.
--
-- Inactivity is per MEMBERSHIP: a person who leaves Hopefarm for SRE keeps a
-- Hopefarm team_members row (is_active = false) as history and gets an active
-- SRE row. users.is_active is only false when the person has no active
-- membership anywhere after a sync (and was a member of something before).
ALTER TABLE team_members
    ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS deactivated_at TIMESTAMP NULL;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS deactivated_at TIMESTAMP NULL;

ALTER TABLE teams
    ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS deactivated_at TIMESTAMP NULL;

CREATE INDEX IF NOT EXISTS idx_team_members_active ON team_members(user_id) WHERE is_active = true;

-- One row per sync attempt. Status is persisted so the UI can recover it after
-- a refresh and so a crashed worker's run can be recovered by heartbeat expiry.
CREATE TABLE IF NOT EXISTS org_sync_runs (
    id                   VARCHAR(64) PRIMARY KEY,
    status               VARCHAR(16)  NOT NULL
        CONSTRAINT org_sync_runs_status_check CHECK (status IN ('PENDING','RUNNING','SUCCESS','FAILED')),
    stage                VARCHAR(32)  NOT NULL DEFAULT 'QUEUED',
    initiated_by         VARCHAR(255) NOT NULL DEFAULT 'scheduler',
    idempotency_key      VARCHAR(128) NULL,
    snapshot_type        VARCHAR(16)  NOT NULL DEFAULT 'COMPLETE'
        CONSTRAINT org_sync_runs_snapshot_type_check CHECK (snapshot_type IN ('COMPLETE','PARTIAL')),
    source_generated_at  TIMESTAMP NULL,
    created_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at           TIMESTAMP NULL,
    completed_at         TIMESTAMP NULL,
    heartbeat_at         TIMESTAMP NULL,
    inserted_users       INT NOT NULL DEFAULT 0,
    updated_users        INT NOT NULL DEFAULT 0,
    inserted_teams       INT NOT NULL DEFAULT 0,
    updated_teams        INT NOT NULL DEFAULT 0,
    membership_changes   INT NOT NULL DEFAULT 0,
    deactivated_records  INT NOT NULL DEFAULT 0,
    retry_count          INT NOT NULL DEFAULT 0,
    error_code           VARCHAR(64) NULL,
    error_message        VARCHAR(500) NULL,
    failed_stage         VARCHAR(32) NULL,
    worker_id            VARCHAR(128) NULL,
    hold_report          JSONB NULL
);

CREATE INDEX IF NOT EXISTS idx_org_sync_runs_created ON org_sync_runs(created_at DESC);
-- At most one non-terminal run at a time, enforced by the database so two API
-- replicas cannot both admit a run.
CREATE UNIQUE INDEX IF NOT EXISTS uq_org_sync_runs_single_active
    ON org_sync_runs ((true)) WHERE status IN ('PENDING','RUNNING');
CREATE UNIQUE INDEX IF NOT EXISTS uq_org_sync_runs_idempotency
    ON org_sync_runs (idempotency_key) WHERE idempotency_key IS NOT NULL;

-- Newest-wins ordering: the provider timestamp of the last committed sync.
ALTER TABLE app_settings
    ADD COLUMN IF NOT EXISTS org_sync_last_source_generated_at TIMESTAMP NULL;
