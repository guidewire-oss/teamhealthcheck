-- SPDX-License-Identifier: Apache-2.0

-- Administrator (always created — needed for login in all environments)
-- Demo users are now seeded at app startup via SeedDemoData() when APP_ENV=demo
INSERT INTO users (id, username, email, full_name, hierarchy_level_id, reports_to, password_hash)
VALUES
    ('admin', 'admin', 'admin@teams360.demo', 'System Administrator', 'level-admin', NULL, '$2a$10$OIc/j2lHs3sUYkWSEr8VW.HFva8imAr5l4tHIIx0bLwqKiCwdicve')
ON CONFLICT (id) DO UPDATE SET
    hierarchy_level_id = 'level-admin',
    password_hash = '$2a$10$OIc/j2lHs3sUYkWSEr8VW.HFva8imAr5l4tHIIx0bLwqKiCwdicve';
