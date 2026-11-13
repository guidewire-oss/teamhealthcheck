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
--
-- The DEFAULT 0 is kept on purpose, so this migration is safe for a rolling
-- deploy. The application applies migrations at startup, so a pre-000032 replica
-- can still be running and inserting runs after this migration commits. That
-- binary omits these columns from its INSERT and scans them into a plain int; a
-- dropped default would hand it NULL rows it cannot read. With the default kept,
-- its inserts stay 0 and stay readable. The current code does not rely on the
-- default: CreatePending writes NULL explicitly, so a new run starts unknown.
-- Only the columns' NOT NULL constraint is removed here.
--
-- Known limit: a pre-000032 replica still cannot read a row that holds NULL,
-- which includes the rows the UPDATE below marks unknown and any run a new
-- replica creates. That window lasts only while old replicas are still serving.
-- If it matters for a deployment, drain the old replicas before running this.
-- A later migration can drop the default once no pre-000032 binary remains.
ALTER TABLE org_sync_runs
    ALTER COLUMN unchanged_users DROP NOT NULL,
    ALTER COLUMN unchanged_teams DROP NOT NULL;

UPDATE org_sync_runs
   SET unchanged_users = NULL, unchanged_teams = NULL
 WHERE status <> 'SUCCESS'
    OR (unchanged_users = 0 AND unchanged_teams = 0);
