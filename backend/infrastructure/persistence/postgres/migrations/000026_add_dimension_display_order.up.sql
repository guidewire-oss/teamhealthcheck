-- Add an explicit presentation-order column for health dimensions.
--
-- Without this, dimensions were only ever orderable by `id` (an arbitrary
-- short mnemonic string), which sorts alphabetically and does not match the
-- intended Spotify Squad Health Check presentation order (Mission first,
-- then Value, Speed, Fun, ...). The frontend used to mask this by rendering
-- from its own hardcoded, correctly-ordered array; now that it reads
-- dimensions live from this table, the table itself must encode the order.
ALTER TABLE health_dimensions ADD COLUMN IF NOT EXISTS display_order INTEGER NOT NULL DEFAULT 0;

UPDATE health_dimensions SET display_order = 1 WHERE id = 'mission';
UPDATE health_dimensions SET display_order = 2 WHERE id = 'value';
UPDATE health_dimensions SET display_order = 3 WHERE id = 'speed';
UPDATE health_dimensions SET display_order = 4 WHERE id = 'fun';
UPDATE health_dimensions SET display_order = 5 WHERE id = 'health';
UPDATE health_dimensions SET display_order = 6 WHERE id = 'learning';
UPDATE health_dimensions SET display_order = 7 WHERE id = 'support';
UPDATE health_dimensions SET display_order = 8 WHERE id = 'pawns';
UPDATE health_dimensions SET display_order = 9 WHERE id = 'release';
UPDATE health_dimensions SET display_order = 10 WHERE id = 'process';
UPDATE health_dimensions SET display_order = 11 WHERE id = 'teamwork';

-- ADD COLUMN ... DEFAULT 0 gave every pre-existing row 0, and only the eleven
-- seed ids above were then renumbered. A database that already has custom
-- dimensions (the admin "Add Dimension" flow predates this migration) would
-- keep those at 0 and `ORDER BY display_order, id` would sort them ahead of
-- Mission. Move every remaining row to a distinct position after the current
-- last one, oldest first, so they keep their relative creation order.
WITH base AS (
    SELECT COALESCE(MAX(display_order), 0) AS last_position
    FROM health_dimensions
    WHERE display_order > 0
),
ranked AS (
    SELECT id, ROW_NUMBER() OVER (ORDER BY created_at, id) AS rn
    FROM health_dimensions
    WHERE display_order = 0
)
UPDATE health_dimensions d
SET display_order = base.last_position + ranked.rn
FROM ranked, base
WHERE d.id = ranked.id;

-- A column default cannot contain a subquery, so inserts that do not go
-- through SaveDimension's own MAX+1 (seed data, tests, direct SQL) would
-- otherwise take the default 0 and land before Mission. This trigger gives
-- any row inserted without an explicit position (0) the next free one.
CREATE OR REPLACE FUNCTION health_dimensions_assign_display_order() RETURNS trigger AS $$
BEGIN
    IF NEW.display_order IS NULL OR NEW.display_order = 0 THEN
        SELECT COALESCE(MAX(display_order), 0) + 1 INTO NEW.display_order FROM health_dimensions;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_health_dimensions_display_order ON health_dimensions;
CREATE TRIGGER trg_health_dimensions_display_order
    BEFORE INSERT ON health_dimensions
    FOR EACH ROW EXECUTE FUNCTION health_dimensions_assign_display_order();

CREATE INDEX IF NOT EXISTS idx_dimensions_display_order ON health_dimensions(display_order);

COMMENT ON COLUMN health_dimensions.display_order IS 'Presentation order in the survey and admin list (lower = earlier). A dimension inserted without an explicit position (0) is placed after the current last one by trg_health_dimensions_display_order.';
