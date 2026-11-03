-- A sync can switch team health checks on or off and can be run with an
-- explicit mass-deletion override. Both are recorded on the run so the admin
-- UI, which reads the persisted run, can report them.
ALTER TABLE org_sync_runs
    ADD COLUMN IF NOT EXISTS health_checks_disabled   INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS health_checks_enabled    INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS mass_deletion_overridden BOOLEAN NOT NULL DEFAULT false;
