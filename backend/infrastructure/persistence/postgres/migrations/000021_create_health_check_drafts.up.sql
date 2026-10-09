-- Create health_check_drafts table for server-side autosave/resume of in-progress surveys
CREATE TABLE IF NOT EXISTS health_check_drafts (
    id VARCHAR(150) PRIMARY KEY,
    team_id VARCHAR(50) NOT NULL,
    user_id VARCHAR(50) NOT NULL,
    survey_type VARCHAR(20) NOT NULL DEFAULT 'individual',
    assessment_period VARCHAR(50) NOT NULL,
    current_dimension INT NOT NULL DEFAULT 0,
    responses JSONB NOT NULL DEFAULT '[]',
    client_updated_at BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_health_check_drafts_user_team_type UNIQUE (user_id, team_id, survey_type)
);

CREATE INDEX idx_health_check_drafts_user_team_type ON health_check_drafts(user_id, team_id, survey_type);

COMMENT ON TABLE health_check_drafts IS 'In-progress (not yet submitted) survey answers, persisted server-side so a participant can resume on another browser or device';
COMMENT ON COLUMN health_check_drafts.client_updated_at IS 'Client-supplied epoch-millis timestamp used for last-write-wins ordering; an incoming save older than the stored value is ignored';
