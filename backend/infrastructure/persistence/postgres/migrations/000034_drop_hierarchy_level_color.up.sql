-- Product simplification: hierarchy levels no longer have a configurable/
-- persisted color. Drop the column added in 000010.
ALTER TABLE hierarchy_levels DROP COLUMN IF EXISTS color;
