CREATE TABLE action_items (
    id                VARCHAR(100)  PRIMARY KEY,
    team_id           VARCHAR(255)  NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    dimension_id      VARCHAR(50)   REFERENCES health_dimensions(id) ON DELETE SET NULL,
    created_by        VARCHAR(255)  NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    assigned_to       VARCHAR(255)  REFERENCES users(id) ON DELETE SET NULL,
    title             VARCHAR(500)  NOT NULL,
    description       TEXT,
    status            VARCHAR(20)   NOT NULL DEFAULT 'open'
                      CHECK (status IN ('open', 'in_progress', 'done')),
    due_date          DATE,
    assessment_period VARCHAR(50),
    created_at        TIMESTAMPTZ   DEFAULT NOW(),
    updated_at        TIMESTAMPTZ   DEFAULT NOW()
);

CREATE INDEX idx_action_items_team_id ON action_items(team_id);
CREATE INDEX idx_action_items_team_status ON action_items(team_id, status);
