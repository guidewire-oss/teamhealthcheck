-- Unchanged = existing records the sync looked at and found identical. Stored
-- beside added/updated so a re-sync of identical data reads "0 added, 0 updated,
-- N unchanged" instead of counting every record it merely read as an update.
ALTER TABLE org_sync_runs
    ADD COLUMN IF NOT EXISTS unchanged_users INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS unchanged_teams INT NOT NULL DEFAULT 0;
