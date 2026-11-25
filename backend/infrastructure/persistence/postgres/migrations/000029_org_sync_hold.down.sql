-- Refuses to roll back while a hold exists: dropping the table would silently
-- discard the frozen threshold and let the next sync bypass the guard.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM org_sync_hold) THEN
        RAISE EXCEPTION 'cannot roll back 000029: an unresolved org sync mass-deletion hold exists; dismiss or override it first';
    END IF;
END $$;

DROP TABLE IF EXISTS org_sync_hold;
