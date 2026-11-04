-- Scheduled (automatic) organization sync.
--
-- Runs get a trigger type so automatic and manual attempts are tracked
-- separately while sharing one run history, one single-active-run index and one
-- advisory lock.
ALTER TABLE org_sync_runs
    ADD COLUMN IF NOT EXISTS trigger_type VARCHAR(16) NOT NULL DEFAULT 'MANUAL';
ALTER TABLE org_sync_runs
    DROP CONSTRAINT IF EXISTS org_sync_runs_trigger_type_check;
ALTER TABLE org_sync_runs
    ADD CONSTRAINT org_sync_runs_trigger_type_check CHECK (trigger_type IN ('MANUAL','SCHEDULED'));
CREATE INDEX IF NOT EXISTS idx_org_sync_runs_trigger_created
    ON org_sync_runs(trigger_type, created_at DESC);

-- Singleton schedule row (id pinned to 1). Disabled by default. next_run_at is
-- the next UTC occurrence; replicas claim an occurrence by compare-and-swapping
-- it forward, so each occurrence fires at most once.
CREATE TABLE IF NOT EXISTS org_sync_schedule (
    id          SMALLINT PRIMARY KEY CONSTRAINT org_sync_schedule_singleton CHECK (id = 1),
    enabled     BOOLEAN NOT NULL DEFAULT false,
    frequency   VARCHAR(16) NOT NULL DEFAULT 'DAILY'
        CONSTRAINT org_sync_schedule_frequency_check CHECK (frequency IN ('DAILY','WEEKLY','MONTHLY')),
    next_run_at TIMESTAMPTZ NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by  VARCHAR(255) NULL
);
INSERT INTO org_sync_schedule (id) VALUES (1) ON CONFLICT (id) DO NOTHING;
