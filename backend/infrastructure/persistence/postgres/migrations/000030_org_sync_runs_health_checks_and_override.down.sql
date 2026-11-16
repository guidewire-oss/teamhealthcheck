ALTER TABLE org_sync_runs
    DROP COLUMN IF EXISTS mass_deletion_overridden,
    DROP COLUMN IF EXISTS health_checks_enabled,
    DROP COLUMN IF EXISTS health_checks_disabled;
