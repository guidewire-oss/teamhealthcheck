-- SPDX-License-Identifier: Apache-2.0

-- Add survey_type column to distinguish individual vs post-workshop surveys
ALTER TABLE health_check_sessions
ADD COLUMN survey_type VARCHAR(20) NOT NULL DEFAULT 'individual';

ALTER TABLE health_check_sessions
ADD CONSTRAINT chk_sessions_survey_type CHECK (survey_type IN ('individual', 'post_workshop'));

CREATE INDEX idx_sessions_survey_type ON health_check_sessions(survey_type);
CREATE INDEX idx_sessions_team_period_type ON health_check_sessions(team_id, assessment_period, survey_type) WHERE completed = true;
