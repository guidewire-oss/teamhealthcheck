-- SPDX-License-Identifier: Apache-2.0

-- Create password reset tokens table for P0-3: Password reset flow
-- Tokens are short-lived (1 hour) and single-use

CREATE TABLE IF NOT EXISTS password_reset_tokens (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) NOT NULL,  -- bcrypt hash of the token for security
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    used_at TIMESTAMP WITH TIME ZONE,  -- NULL if not yet used
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- Ensure token_hash is unique to prevent token collision attacks
    UNIQUE(token_hash)
);

-- Index for fast token lookup
CREATE INDEX idx_password_reset_tokens_user_id ON password_reset_tokens(user_id);
CREATE INDEX idx_password_reset_tokens_expires_at ON password_reset_tokens(expires_at);

-- Cleanup old tokens after 24 hours (even expired ones)
-- This would typically be done by a scheduled job, but the index helps with manual cleanup
CREATE INDEX idx_password_reset_tokens_created_at ON password_reset_tokens(created_at);
