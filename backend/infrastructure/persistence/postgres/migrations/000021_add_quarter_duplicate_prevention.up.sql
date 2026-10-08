-- Add computed quarter/year columns used to prevent duplicate survey submissions for the
-- same calendar quarter, regardless of the team's assessment-period cadence label.
ALTER TABLE health_check_sessions
ADD COLUMN quarter_number SMALLINT,
ADD COLUMN quarter_year SMALLINT;

COMMENT ON COLUMN health_check_sessions.quarter_number IS 'Calendar quarter (1-4) derived from assessment_period, used for duplicate-submission prevention';
COMMENT ON COLUMN health_check_sessions.quarter_year IS 'Calendar year matching quarter_number, used for duplicate-submission prevention';

-- One individual survey per user per calendar quarter. Team-agnostic by design: a
-- different user's individual submission must never block this user.
CREATE UNIQUE INDEX idx_unique_individual_quarter_submission
ON health_check_sessions (user_id, quarter_number, quarter_year)
WHERE survey_type = 'individual' AND completed = true;

-- One post-workshop survey per team per calendar quarter.
CREATE UNIQUE INDEX idx_unique_post_workshop_quarter_submission
ON health_check_sessions (team_id, quarter_number, quarter_year)
WHERE survey_type = 'post_workshop' AND completed = true;
