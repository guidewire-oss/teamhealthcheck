ALTER TABLE app_settings DROP COLUMN IF EXISTS org_sync_last_source_generated_at;
DROP INDEX IF EXISTS uq_org_sync_runs_idempotency;
DROP INDEX IF EXISTS uq_org_sync_runs_single_active;
DROP INDEX IF EXISTS idx_org_sync_runs_created;
DROP TABLE IF EXISTS org_sync_runs;
DROP INDEX IF EXISTS idx_team_members_active;
ALTER TABLE teams DROP COLUMN IF EXISTS deactivated_at, DROP COLUMN IF EXISTS is_active;
ALTER TABLE users DROP COLUMN IF EXISTS deactivated_at, DROP COLUMN IF EXISTS is_active;
ALTER TABLE team_members DROP COLUMN IF EXISTS deactivated_at, DROP COLUMN IF EXISTS is_active;
