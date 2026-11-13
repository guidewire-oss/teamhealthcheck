-- Drops the flag. unchanged_users and unchanged_teams were never altered by the
-- up migration, so nothing else needs restoring. After a rollback the counts read
-- as plain numbers again, as they did before 000032; the unknown-versus-zero
-- distinction exists only while the flag does.
ALTER TABLE org_sync_runs DROP COLUMN IF EXISTS unchanged_recorded;
