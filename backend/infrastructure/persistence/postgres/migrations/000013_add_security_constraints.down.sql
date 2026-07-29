-- SPDX-License-Identifier: Apache-2.0

-- Remove security constraints (in reverse order)

-- Remove indexes
DROP INDEX IF EXISTS idx_password_reset_tokens_expires;
DROP INDEX IF EXISTS idx_password_reset_tokens_user;

-- Remove unique constraint
ALTER TABLE health_check_responses DROP CONSTRAINT IF EXISTS uq_responses_session_dimension;

-- Remove foreign key constraints
ALTER TABLE health_check_responses DROP CONSTRAINT IF EXISTS fk_responses_dimension;
ALTER TABLE health_check_responses DROP CONSTRAINT IF EXISTS fk_responses_session;
ALTER TABLE health_check_sessions DROP CONSTRAINT IF EXISTS fk_sessions_user;
ALTER TABLE health_check_sessions DROP CONSTRAINT IF EXISTS fk_sessions_team;

-- Remove CHECK constraints from health_check_responses
ALTER TABLE health_check_responses DROP CONSTRAINT IF EXISTS chk_responses_comment_length;
ALTER TABLE health_check_responses DROP CONSTRAINT IF EXISTS chk_responses_trend_values;
ALTER TABLE health_check_responses DROP CONSTRAINT IF EXISTS chk_responses_score_range;

-- Remove CHECK constraints from team_supervisors
ALTER TABLE team_supervisors DROP CONSTRAINT IF EXISTS chk_supervisor_position_positive;

-- Remove CHECK constraints from hierarchy_levels
ALTER TABLE hierarchy_levels DROP CONSTRAINT IF EXISTS chk_hierarchy_position_nonnegative;

-- Remove CHECK constraints from teams
ALTER TABLE teams DROP CONSTRAINT IF EXISTS chk_teams_cadence_values;

-- Remove CHECK constraints from users
ALTER TABLE users DROP CONSTRAINT IF EXISTS chk_users_username_format;
ALTER TABLE users DROP CONSTRAINT IF EXISTS chk_users_email_format;
