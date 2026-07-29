-- SPDX-License-Identifier: Apache-2.0

-- Create hierarchy_levels table for organizational structure
CREATE TABLE IF NOT EXISTS hierarchy_levels (
    id VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    position INT NOT NULL UNIQUE,
    can_view_all_teams BOOLEAN DEFAULT false,
    can_edit_teams BOOLEAN DEFAULT false,
    can_manage_users BOOLEAN DEFAULT false,
    can_take_survey BOOLEAN DEFAULT true,
    can_view_analytics BOOLEAN DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create index for position ordering
CREATE INDEX idx_hierarchy_levels_position ON hierarchy_levels(position);

-- Add comment for documentation
COMMENT ON TABLE hierarchy_levels IS 'Organizational hierarchy levels with role-based permissions';
COMMENT ON COLUMN hierarchy_levels.position IS 'Order position in hierarchy (1 = highest, e.g., VP)';

-- Seed initial hierarchy levels based on Teams360 model
INSERT INTO hierarchy_levels (id, name, position, can_view_all_teams, can_edit_teams, can_manage_users, can_take_survey, can_view_analytics) VALUES
    ('level-1', 'VP', 1, true, true, true, false, true),
    ('level-2', 'Director', 2, true, true, true, false, true),
    ('level-3', 'Manager', 3, true, false, false, false, true),
    ('level-4', 'Team Lead', 4, false, false, false, true, true),
    ('level-5', 'Team Member', 5, false, false, false, true, false),
    ('level-admin', 'Admin', 0, true, true, true, false, true);

-- Add foreign key constraint to users table (will work if users exist with these level_ids)
ALTER TABLE users ADD CONSTRAINT fk_users_hierarchy_level
    FOREIGN KEY (hierarchy_level_id) REFERENCES hierarchy_levels(id);
