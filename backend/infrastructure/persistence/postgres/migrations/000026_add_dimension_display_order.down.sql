DROP TRIGGER IF EXISTS trg_health_dimensions_display_order ON health_dimensions;
DROP FUNCTION IF EXISTS health_dimensions_assign_display_order();
DROP INDEX IF EXISTS idx_dimensions_display_order;
ALTER TABLE health_dimensions DROP COLUMN IF EXISTS display_order;
