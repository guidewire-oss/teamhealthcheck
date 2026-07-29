-- SPDX-License-Identifier: Apache-2.0

-- Users table for organizational hierarchy
CREATE TABLE users (
    id VARCHAR(255) PRIMARY KEY,
    username VARCHAR(255) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    full_name VARCHAR(255) NOT NULL,
    hierarchy_level_id VARCHAR(255) NOT NULL,
    reports_to VARCHAR(255) REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Teams table
CREATE TABLE teams (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    team_lead_id VARCHAR(255) REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Team members junction table (users can be in multiple teams)
CREATE TABLE team_members (
    team_id VARCHAR(255) REFERENCES teams(id) ON DELETE CASCADE,
    user_id VARCHAR(255) REFERENCES users(id) ON DELETE CASCADE,
    joined_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (team_id, user_id)
);

-- Team supervisors for hierarchy chain (denormalized for performance)
CREATE TABLE team_supervisors (
    team_id VARCHAR(255) REFERENCES teams(id) ON DELETE CASCADE,
    user_id VARCHAR(255) REFERENCES users(id) ON DELETE CASCADE,
    hierarchy_level_id VARCHAR(255) NOT NULL,
    position INT NOT NULL, -- Order in supervisor chain (1 = closest supervisor)
    PRIMARY KEY (team_id, user_id),
    UNIQUE (team_id, position)
);

-- Indexes for performance
CREATE INDEX idx_users_reports_to ON users(reports_to);
CREATE INDEX idx_users_hierarchy_level ON users(hierarchy_level_id);
CREATE INDEX idx_team_members_user ON team_members(user_id);
CREATE INDEX idx_team_supervisors_user ON team_supervisors(user_id);
CREATE INDEX idx_team_supervisors_position ON team_supervisors(team_id, position);
