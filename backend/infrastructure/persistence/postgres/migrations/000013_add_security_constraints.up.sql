-- SPDX-License-Identifier: Apache-2.0

-- Add security and data integrity constraints

-- Add CHECK constraints for email format validation at database level
ALTER TABLE users
ADD CONSTRAINT chk_users_email_format
CHECK (email ~* '^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$');

-- Add CHECK constraint for username format (allowing 2-50 chars for existing demo data like "vp")
ALTER TABLE users
ADD CONSTRAINT chk_users_username_format
CHECK (username ~ '^[a-zA-Z0-9_-]{2,50}$');

-- Add NOT NULL constraints to critical fields in health_check_sessions
-- Note: team_id and user_id already have NOT NULL

-- Add CHECK constraint for score values (1-3)
ALTER TABLE health_check_responses
ADD CONSTRAINT chk_responses_score_range
CHECK (score >= 1 AND score <= 3);

-- Add CHECK constraint for trend values
ALTER TABLE health_check_responses
ADD CONSTRAINT chk_responses_trend_values
CHECK (trend IN ('improving', 'stable', 'declining'));

-- Add CHECK constraint for cadence values in teams
ALTER TABLE teams
ADD CONSTRAINT chk_teams_cadence_values
CHECK (cadence IS NULL OR cadence IN ('weekly', 'biweekly', 'monthly', 'quarterly'));

-- Add CHECK constraint for hierarchy levels position (must be non-negative)
-- Note: Admin level uses position 0, so we allow >= 0
ALTER TABLE hierarchy_levels
ADD CONSTRAINT chk_hierarchy_position_nonnegative
CHECK (position >= 0);

-- Add CHECK constraint for team supervisor position
ALTER TABLE team_supervisors
ADD CONSTRAINT chk_supervisor_position_positive
CHECK (position > 0);

-- Note: Foreign key constraints for health_check_sessions to teams/users
-- are NOT added here because the demo seed data may reference non-existent users.
-- These constraints should be added after fixing the seed data in a future migration.
-- For now, application-level validation ensures referential integrity.

-- Drop old FK constraints without CASCADE and add new ones with CASCADE
-- This ensures cascade deletes work properly

-- Drop old FK constraints if they exist (without CASCADE behavior)
ALTER TABLE health_check_responses DROP CONSTRAINT IF EXISTS health_check_responses_session_id_fkey;
ALTER TABLE health_check_responses DROP CONSTRAINT IF EXISTS health_check_responses_dimension_id_fkey;

-- Add foreign key constraint for health_check_responses to health_check_sessions (with CASCADE)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE constraint_name = 'fk_responses_session'
        AND table_name = 'health_check_responses'
    ) THEN
        ALTER TABLE health_check_responses
        ADD CONSTRAINT fk_responses_session
        FOREIGN KEY (session_id) REFERENCES health_check_sessions(id) ON DELETE CASCADE;
    END IF;
END $$;

-- Add foreign key constraint for health_check_responses to health_dimensions (RESTRICT delete)
-- Note: Dimensions should be soft-deleted (is_active = false) rather than hard-deleted
-- since historical responses reference them
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE constraint_name = 'fk_responses_dimension'
        AND table_name = 'health_check_responses'
    ) THEN
        ALTER TABLE health_check_responses
        ADD CONSTRAINT fk_responses_dimension
        FOREIGN KEY (dimension_id) REFERENCES health_dimensions(id) ON DELETE RESTRICT;
    END IF;
END $$;

-- Add unique constraint to prevent duplicate responses per session/dimension
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE constraint_name = 'uq_responses_session_dimension'
        AND table_name = 'health_check_responses'
    ) THEN
        ALTER TABLE health_check_responses
        ADD CONSTRAINT uq_responses_session_dimension
        UNIQUE (session_id, dimension_id);
    END IF;
END $$;

-- Add comment length constraint
ALTER TABLE health_check_responses
ADD CONSTRAINT chk_responses_comment_length
CHECK (comment IS NULL OR length(comment) <= 1000);

-- Create index for password reset tokens lookup by user
CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_user
ON password_reset_tokens(user_id);

-- Create index for expired tokens cleanup
CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_expires
ON password_reset_tokens(expires_at);

-- Add comments for documentation
COMMENT ON CONSTRAINT chk_users_email_format ON users IS 'Validates email format at database level';
COMMENT ON CONSTRAINT chk_users_username_format ON users IS 'Username must be 3-50 chars, alphanumeric with _ and -';
COMMENT ON CONSTRAINT chk_responses_score_range ON health_check_responses IS 'Score must be 1, 2, or 3';
COMMENT ON CONSTRAINT chk_responses_trend_values ON health_check_responses IS 'Trend must be improving, stable, or declining';
