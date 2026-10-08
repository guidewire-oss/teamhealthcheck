-- Remove 'quarterly' as a valid team cadence. Quarter-based assessment periods have been
-- replaced entirely by the six-month submission cooldown (see the "six month submission
-- cooldown" migration), so surveying every quarter is no longer a supported cadence.
-- Existing quarterly-cadence teams migrate to half-yearly, the closest supported cadence.
UPDATE teams SET cadence = 'half-yearly' WHERE cadence = 'quarterly';

ALTER TABLE teams DROP CONSTRAINT IF EXISTS chk_teams_cadence_values;

ALTER TABLE teams
ADD CONSTRAINT chk_teams_cadence_values
CHECK (cadence IS NULL OR cadence IN ('monthly', 'half-yearly', 'yearly'));
