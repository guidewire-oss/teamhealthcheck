-- SPDX-License-Identifier: Apache-2.0

-- Create health_check_responses table
CREATE TABLE IF NOT EXISTS health_check_responses (
    id SERIAL PRIMARY KEY,
    session_id VARCHAR(100) NOT NULL REFERENCES health_check_sessions(id) ON DELETE CASCADE,
    dimension_id VARCHAR(50) NOT NULL REFERENCES health_dimensions(id),
    score SMALLINT NOT NULL CHECK (score >= 1 AND score <= 3),
    trend VARCHAR(20) NOT NULL CHECK (trend IN ('improving', 'stable', 'declining')),
    comment TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Ensure one response per dimension per session
CREATE UNIQUE INDEX idx_responses_session_dimension ON health_check_responses(session_id, dimension_id);

-- Index for aggregation queries
CREATE INDEX idx_responses_dimension_score ON health_check_responses(dimension_id, score);
CREATE INDEX idx_responses_session ON health_check_responses(session_id);

-- Add comments for documentation
COMMENT ON TABLE health_check_responses IS 'Individual dimension responses (value object, part of session aggregate)';
COMMENT ON COLUMN health_check_responses.score IS '1 = red (poor), 2 = yellow (medium), 3 = green (good)';
COMMENT ON COLUMN health_check_responses.trend IS 'Trend direction: improving, stable, declining';
