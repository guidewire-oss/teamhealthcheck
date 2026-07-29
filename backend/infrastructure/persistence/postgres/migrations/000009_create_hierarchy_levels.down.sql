-- SPDX-License-Identifier: Apache-2.0

-- Remove foreign key constraint from users table
ALTER TABLE users DROP CONSTRAINT IF EXISTS fk_users_hierarchy_level;

-- Drop hierarchy_levels table
DROP TABLE IF EXISTS hierarchy_levels;
