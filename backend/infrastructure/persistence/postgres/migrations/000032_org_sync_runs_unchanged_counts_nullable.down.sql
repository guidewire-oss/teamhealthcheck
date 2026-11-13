-- Restores the 000028 shape: unchanged_users / unchanged_teams NOT NULL DEFAULT 0.
--
-- Unlike 000031's down migration this does not refuse when data would be lost,
-- and that is deliberate. The pre-000032 schema, and the application code that
-- goes with it, cannot represent "not recorded": the columns were NOT NULL and
-- 0 was already how unknown was stored, so restoring NOT NULL requires giving
-- every NULL a value. Refusing instead would block every rollback as soon as
-- one failed or pending run exists, because those rows are NULL after 000032.
--
-- What is lost is the distinction 000032 added: afterwards a NULL row reads as
-- "0 unchanged" again. A re-apply of 000032 marks only SUCCESS rows whose
-- counts are both 0 as unknown again, plus all non-SUCCESS rows, so a genuine
-- zero on a SUCCESS row with a non-zero sibling count is not affected. The
-- notice below reports how many rows were coerced so it is not silent.
DO $$
DECLARE
    coerced BIGINT;
BEGIN
    SELECT count(*) INTO coerced
      FROM org_sync_runs
     WHERE unchanged_users IS NULL OR unchanged_teams IS NULL;
    RAISE NOTICE '000032 down: % org_sync_runs row(s) had unknown unchanged counts and are now stored as 0', coerced;
END $$;

UPDATE org_sync_runs SET unchanged_users = 0 WHERE unchanged_users IS NULL;
UPDATE org_sync_runs SET unchanged_teams = 0 WHERE unchanged_teams IS NULL;
ALTER TABLE org_sync_runs
    ALTER COLUMN unchanged_users SET DEFAULT 0,
    ALTER COLUMN unchanged_users SET NOT NULL,
    ALTER COLUMN unchanged_teams SET DEFAULT 0,
    ALTER COLUMN unchanged_teams SET NOT NULL;
