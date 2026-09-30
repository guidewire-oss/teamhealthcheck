DROP INDEX IF EXISTS idx_dimensions_display_order;
ALTER TABLE health_dimensions DROP COLUMN IF EXISTS display_order;
