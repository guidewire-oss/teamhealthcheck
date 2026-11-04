-- Refuses to roll back while the schedule is enabled or automatic runs exist:
-- dropping the table loses the saved schedule (re-applying re-seeds it
-- disabled, so auto sync would silently stop), and dropping trigger_type loses
-- which runs were automatic.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM org_sync_schedule WHERE enabled) THEN
        RAISE EXCEPTION 'cannot roll back 000031: auto sync is enabled; disable it first';
    END IF;
    IF EXISTS (SELECT 1 FROM org_sync_runs WHERE trigger_type = 'SCHEDULED') THEN
        RAISE EXCEPTION 'cannot roll back 000031: automatic runs exist; their trigger_type would be lost';
    END IF;
END $$;

DROP TABLE IF EXISTS org_sync_schedule;
DROP INDEX IF EXISTS idx_org_sync_runs_trigger_created;
ALTER TABLE org_sync_runs DROP CONSTRAINT IF EXISTS org_sync_runs_trigger_type_check;
ALTER TABLE org_sync_runs DROP COLUMN IF EXISTS trigger_type;
