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

CREATE INDEX IF NOT EXISTS idx_dimensions_display_order ON health_dimensions(display_order);

COMMENT ON COLUMN health_dimensions.display_order IS 'Presentation order in the survey and admin list (lower = earlier). New dimensions default to appearing after the current last one.';
