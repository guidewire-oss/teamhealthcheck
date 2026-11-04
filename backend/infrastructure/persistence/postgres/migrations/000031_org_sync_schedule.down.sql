DROP TABLE IF EXISTS org_sync_schedule;
DROP INDEX IF EXISTS idx_org_sync_runs_trigger_created;
ALTER TABLE org_sync_runs DROP CONSTRAINT IF EXISTS org_sync_runs_trigger_type_check;
ALTER TABLE org_sync_runs DROP COLUMN IF EXISTS trigger_type;
