ALTER TABLE app_settings DROP CONSTRAINT IF EXISTS org_sync_schedule_complete;
ALTER TABLE app_settings DROP COLUMN IF EXISTS org_sync_next_run_at;
ALTER TABLE app_settings DROP COLUMN IF EXISTS org_sync_schedule_frequency;
ALTER TABLE app_settings DROP COLUMN IF EXISTS org_sync_schedule_enabled;
