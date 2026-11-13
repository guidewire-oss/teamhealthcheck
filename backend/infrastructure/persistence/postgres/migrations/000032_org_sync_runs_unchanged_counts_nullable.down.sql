UPDATE org_sync_runs SET unchanged_users = 0 WHERE unchanged_users IS NULL;
UPDATE org_sync_runs SET unchanged_teams = 0 WHERE unchanged_teams IS NULL;
ALTER TABLE org_sync_runs
    ALTER COLUMN unchanged_users SET DEFAULT 0,
    ALTER COLUMN unchanged_users SET NOT NULL,
    ALTER COLUMN unchanged_teams SET DEFAULT 0,
    ALTER COLUMN unchanged_teams SET NOT NULL;
