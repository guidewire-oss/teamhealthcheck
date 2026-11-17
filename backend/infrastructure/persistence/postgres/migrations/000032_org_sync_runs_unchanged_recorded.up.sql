-- Unchanged counts were added (migration 000028) after runs already existed, and
-- those older rows were back-filled with 0. A stored 0 is therefore ambiguous:
-- "nothing was unchanged" or "this run never recorded it".
--
-- This adds a flag that says whether a run's unchanged counts are real, instead
-- of making the count columns nullable. unchanged_users and unchanged_teams stay
-- NOT NULL DEFAULT 0 and are never set to NULL, so a replica still running the
-- previous release, which scans them into a plain int and omits them from its
-- INSERT, keeps working through a rolling deploy. That binary ignores the new
-- column. Rows it writes keep the default FALSE, so a run it records reads as
-- "not available": it errs toward unknown, never toward a wrong 0.
--
-- Only a SUCCESS row where the counts are not both 0 is known to be real. A real
-- run that read any existing record has a non-zero unchanged count on at least
-- one side. SUCCESS rows where both are 0 stay unknown: that may be an all-new
-- first import or a back-filled row, and guessing would be wrong. Runs that did
-- not succeed never committed counts, so they stay unknown too.
-- inserted_*/updated_*/membership_changes were written from the first version of
-- the table, in the apply transaction, so they need no change.
ALTER TABLE org_sync_runs
    ADD COLUMN IF NOT EXISTS unchanged_recorded BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE org_sync_runs
   SET unchanged_recorded = TRUE
 WHERE status = 'SUCCESS'
   AND NOT (unchanged_users = 0 AND unchanged_teams = 0);
