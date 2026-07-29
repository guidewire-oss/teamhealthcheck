-- SPDX-License-Identifier: Apache-2.0

-- Create health_check_sessions table
CREATE TABLE IF NOT EXISTS health_check_sessions (
    id VARCHAR(100) PRIMARY KEY,
    team_id VARCHAR(50) NOT NULL,
    user_id VARCHAR(50) NOT NULL,
    date DATE NOT NULL,
    assessment_period VARCHAR(50),
    completed BOOLEAN DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for common query patterns
CREATE INDEX idx_sessions_team_date ON health_check_sessions(team_id, date DESC);
CREATE INDEX idx_sessions_user_date ON health_check_sessions(user_id, date DESC);
CREATE INDEX idx_sessions_assessment_period ON health_check_sessions(assessment_period) WHERE assessment_period IS NOT NULL;
CREATE INDEX idx_sessions_completed ON health_check_sessions(completed) WHERE completed = true;
CREATE INDEX idx_sessions_team_completed ON health_check_sessions(team_id, completed, date DESC) WHERE completed = true;

-- Add comments for documentation
COMMENT ON TABLE health_check_sessions IS 'Health check sessions (aggregate root in DDD)';
COMMENT ON COLUMN health_check_sessions.assessment_period IS 'Assessment period (e.g., "2024 - 2nd Half")';
COMMENT ON COLUMN health_check_sessions.completed IS 'Whether all responses have been submitted';
