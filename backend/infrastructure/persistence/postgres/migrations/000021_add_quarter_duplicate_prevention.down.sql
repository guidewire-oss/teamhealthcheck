DROP INDEX IF EXISTS idx_unique_post_workshop_quarter_submission;
DROP INDEX IF EXISTS idx_unique_individual_quarter_submission;

ALTER TABLE health_check_sessions
DROP COLUMN IF EXISTS quarter_year,
DROP COLUMN IF EXISTS quarter_number;
