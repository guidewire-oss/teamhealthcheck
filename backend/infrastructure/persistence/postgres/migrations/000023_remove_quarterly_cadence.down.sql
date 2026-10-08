ALTER TABLE teams DROP CONSTRAINT IF EXISTS chk_teams_cadence_values;

ALTER TABLE teams
ADD CONSTRAINT chk_teams_cadence_values
CHECK (cadence IS NULL OR cadence IN ('monthly', 'quarterly', 'half-yearly', 'yearly'));

-- Note: teams migrated from 'quarterly' to 'half-yearly' by the up migration are not
-- restored to 'quarterly' here, since that mapping cannot be distinguished from teams that
-- were already 'half-yearly' before the up migration ran.
