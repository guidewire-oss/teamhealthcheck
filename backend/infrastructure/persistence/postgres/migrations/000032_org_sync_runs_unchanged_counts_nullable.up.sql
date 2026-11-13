-- Unchanged counts were added (migration 000028) after runs already existed, and
-- those older rows were back-filled with 0. A stored 0 is therefore ambiguous:
-- "nothing was unchanged" or "this run never recorded it". Make the columns
-- nullable so unknown is distinct from zero, and mark the ambiguous rows unknown.
--
-- Only SUCCESS rows where BOTH unchanged counts are 0 are ambiguous. A real run
-- that read any existing record has a non-zero unchanged count on at least one
-- side, so those rows keep their values. Marking an all-new first import unknown
-- as well is deliberate: it errs toward "not available", never toward a guess.
-- Runs that did not succeed never committed counts, so they are unknown too.
-- inserted_*/updated_*/membership_changes were written from the first version of
-- the table, in the apply transaction, so they need no change.
ALTER TABLE org_sync_runs
    ALTER COLUMN unchanged_users DROP NOT NULL,
    ALTER COLUMN unchanged_users DROP DEFAULT,
    ALTER COLUMN unchanged_teams DROP NOT NULL,
    ALTER COLUMN unchanged_teams DROP DEFAULT;

UPDATE org_sync_runs
   SET unchanged_users = NULL, unchanged_teams = NULL
 WHERE status <> 'SUCCESS'
    OR (unchanged_users = 0 AND unchanged_teams = 0);
