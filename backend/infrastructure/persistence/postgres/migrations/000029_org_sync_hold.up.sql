-- The unresolved mass-deletion hold, shared by every API replica. At most one
-- row exists (id is pinned to 1): a hold freezes the deletion threshold until
-- an admin overrides or dismisses it, and that freeze must hold no matter which
-- replica serves the next request.
CREATE TABLE IF NOT EXISTS org_sync_hold (
    id         SMALLINT PRIMARY KEY CONSTRAINT org_sync_hold_singleton CHECK (id = 1),
    threshold  DOUBLE PRECISION NOT NULL,
    report     JSONB NOT NULL,
    held_at    TIMESTAMP NOT NULL
);
