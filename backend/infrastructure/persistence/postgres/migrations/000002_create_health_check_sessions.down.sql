-- SPDX-License-Identifier: Apache-2.0

-- Drop health_check_sessions table
DROP INDEX IF EXISTS idx_sessions_team_date;
DROP INDEX IF EXISTS idx_sessions_user_date;
DROP INDEX IF EXISTS idx_sessions_assessment_period;
DROP INDEX IF EXISTS idx_sessions_completed;
DROP INDEX IF EXISTS idx_sessions_team_completed;
DROP TABLE IF EXISTS health_check_sessions CASCADE;
