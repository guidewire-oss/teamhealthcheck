ALTER TABLE action_items DROP CONSTRAINT action_items_status_check;
ALTER TABLE action_items ADD CONSTRAINT action_items_status_check
    CHECK (status IN ('open', 'on_hold', 'in_progress', 'done'));
