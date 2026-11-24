UPDATE action_items SET status = 'in_progress' WHERE status = 'on_hold';

ALTER TABLE action_items DROP CONSTRAINT action_items_status_check;
ALTER TABLE action_items ADD CONSTRAINT action_items_status_check
    CHECK (status IN ('open', 'in_progress', 'done'));
