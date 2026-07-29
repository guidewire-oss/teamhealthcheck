-- SPDX-License-Identifier: Apache-2.0

DROP INDEX IF EXISTS idx_sessions_team_period_type;
DROP INDEX IF EXISTS idx_sessions_survey_type;
ALTER TABLE health_check_sessions DROP CONSTRAINT IF EXISTS chk_sessions_survey_type;
ALTER TABLE health_check_sessions DROP COLUMN IF EXISTS survey_type;
