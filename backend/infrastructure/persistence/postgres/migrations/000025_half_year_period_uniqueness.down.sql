-- Reverse 000025. Every step only removes a restriction or restores a more permissive
-- rule, so this can always run, whatever data was written while the migration was applied.
DROP INDEX IF EXISTS uniq_post_workshop_period_submission;
DROP INDEX IF EXISTS uniq_individual_period_submission;

ALTER TABLE health_check_sessions DROP CONSTRAINT IF EXISTS chk_completed_session_has_period;

-- Allow 'quarterly' again and restore the previous default, in one step so the constraint
-- and the default can never disagree.
ALTER TABLE teams DROP CONSTRAINT IF EXISTS chk_teams_cadence_values;
ALTER TABLE teams
    ADD CONSTRAINT chk_teams_cadence_values
    CHECK (cadence IS NULL OR cadence IN ('monthly', 'quarterly', 'half-yearly', 'yearly'));
ALTER TABLE teams ALTER COLUMN cadence SET DEFAULT 'quarterly';

-- Note: teams moved from 'quarterly' to 'half-yearly' by the up migration are not moved back,
-- because that cannot be told apart from teams that were already 'half-yearly'.
