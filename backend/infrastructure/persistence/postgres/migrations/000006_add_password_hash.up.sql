-- SPDX-License-Identifier: Apache-2.0

-- Add password_hash column to users table for authentication
ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash VARCHAR(255) NOT NULL DEFAULT 'demo';

-- Update existing test users with demo password hash
-- In a real system, this would be a bcrypt hash
-- For now, we'll use plain text "demo" for development
UPDATE users SET password_hash = 'demo' WHERE password_hash IS NULL OR password_hash = '';

-- Add index for faster username lookups during authentication
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
