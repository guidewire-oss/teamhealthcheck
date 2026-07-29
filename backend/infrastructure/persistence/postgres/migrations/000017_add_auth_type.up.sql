-- SPDX-License-Identifier: Apache-2.0

-- Add auth_type column to users table
-- 'local' = username/password authentication
-- 'sso'   = authentication via external Identity Provider; password_hash unused
ALTER TABLE users ADD COLUMN IF NOT EXISTS auth_type VARCHAR(20) NOT NULL DEFAULT 'local';

ALTER TABLE users ADD CONSTRAINT chk_users_auth_type CHECK (auth_type IN ('local', 'sso'));
