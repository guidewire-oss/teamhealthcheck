-- Admin-configurable mass-deletion threshold for the organization sync.
-- NULL means "not configured by an administrator": the sync then falls back to
-- ORG_SYNC_MAX_DELETE_PERCENT, and to 20% when that is unset too.
ALTER TABLE app_settings
    ADD COLUMN IF NOT EXISTS org_sync_max_delete_percent DOUBLE PRECISION DEFAULT NULL
    CONSTRAINT org_sync_max_delete_percent_range
        CHECK (org_sync_max_delete_percent IS NULL
               OR (org_sync_max_delete_percent >= 1 AND org_sync_max_delete_percent <= 100));
