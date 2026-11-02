-- Refuses to roll back while anything is soft-deactivated. The is_active flags
-- dropped below are the only record of a deactivation, so dropping them would
-- silently reactivate every former user (sign-in, survey eligibility), team and
-- membership. Reactivate or hard-delete those rows deliberately first, then
-- re-run this migration.
DO $$
DECLARE
    inactive_users   BIGINT;
    inactive_teams   BIGINT;
    inactive_members BIGINT;
BEGIN
    SELECT COUNT(*) INTO inactive_users   FROM users        WHERE NOT is_active;
    SELECT COUNT(*) INTO inactive_teams   FROM teams        WHERE NOT is_active;
    SELECT COUNT(*) INTO inactive_members FROM team_members WHERE NOT is_active;

    IF inactive_users > 0 OR inactive_teams > 0 OR inactive_members > 0 THEN
        RAISE EXCEPTION
            'cannot roll back 000026: % inactive user(s), % inactive team(s) and % inactive membership(s) exist; rolling back would reactivate them',
            inactive_users, inactive_teams, inactive_members;
    END IF;
END $$;

ALTER TABLE app_settings DROP COLUMN IF EXISTS org_sync_last_source_generated_at;
DROP INDEX IF EXISTS uq_org_sync_runs_idempotency;
DROP INDEX IF EXISTS uq_org_sync_runs_single_active;
DROP INDEX IF EXISTS idx_org_sync_runs_created;
DROP TABLE IF EXISTS org_sync_runs;
DROP INDEX IF EXISTS idx_team_members_active;
ALTER TABLE teams DROP COLUMN IF EXISTS deactivated_at, DROP COLUMN IF EXISTS is_active;
ALTER TABLE users DROP COLUMN IF EXISTS deactivated_at, DROP COLUMN IF EXISTS is_active;
ALTER TABLE team_members DROP COLUMN IF EXISTS deactivated_at, DROP COLUMN IF EXISTS is_active;
