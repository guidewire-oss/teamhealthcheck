ALTER TABLE health_check_sessions
DROP CONSTRAINT IF EXISTS excl_post_workshop_six_month_gap,
DROP CONSTRAINT IF EXISTS excl_individual_six_month_gap;

ALTER TABLE health_check_sessions
ADD COLUMN quarter_number SMALLINT,
ADD COLUMN quarter_year SMALLINT;

COMMENT ON COLUMN health_check_sessions.quarter_number IS 'Calendar quarter (1-4) derived from assessment_period, used for duplicate-submission prevention';
COMMENT ON COLUMN health_check_sessions.quarter_year IS 'Calendar year matching quarter_number, used for duplicate-submission prevention';

CREATE UNIQUE INDEX idx_unique_individual_quarter_submission
ON health_check_sessions (user_id, quarter_number, quarter_year)
WHERE survey_type = 'individual' AND completed = true;

CREATE UNIQUE INDEX idx_unique_post_workshop_quarter_submission
ON health_check_sessions (team_id, quarter_number, quarter_year)
WHERE survey_type = 'post_workshop' AND completed = true;
