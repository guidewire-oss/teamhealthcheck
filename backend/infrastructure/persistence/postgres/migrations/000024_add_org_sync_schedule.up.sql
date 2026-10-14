-- Admin-configurable automatic organization-sync schedule. Disabled by
-- default for both new and existing deployments.
ALTER TABLE app_settings
    ADD COLUMN IF NOT EXISTS org_sync_schedule_enabled BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE app_settings
    ADD COLUMN IF NOT EXISTS org_sync_schedule_frequency TEXT DEFAULT NULL
    CONSTRAINT org_sync_schedule_frequency_allowed
        CHECK (org_sync_schedule_frequency IS NULL
               OR org_sync_schedule_frequency IN ('daily', 'weekly', 'monthly'));

-- The instant the scheduler should next consider running, as a UTC instant
-- (not a wall-clock local time -- see the scheduler package). NULL whenever
-- disabled: there is no next run to compute, and this column is cleared on
-- every disable so the UI/API never has to explain a stale next-run date on
-- a schedule that is off.
ALTER TABLE app_settings
    ADD COLUMN IF NOT EXISTS org_sync_next_run_at TIMESTAMP WITH TIME ZONE DEFAULT NULL;

-- Enabled implies a frequency and a next run. Enforced in the database so no
-- code path can leave the scheduler "on, but with no idea when".
ALTER TABLE app_settings
    ADD CONSTRAINT org_sync_schedule_complete
        CHECK (org_sync_schedule_enabled = false
               OR (org_sync_schedule_frequency IS NOT NULL
                   AND org_sync_next_run_at IS NOT NULL));
