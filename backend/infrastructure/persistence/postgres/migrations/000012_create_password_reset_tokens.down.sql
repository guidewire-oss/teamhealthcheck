-- SPDX-License-Identifier: Apache-2.0

-- Rollback password reset tokens table

DROP INDEX IF EXISTS idx_password_reset_tokens_created_at;
DROP INDEX IF EXISTS idx_password_reset_tokens_expires_at;
DROP INDEX IF EXISTS idx_password_reset_tokens_user_id;
DROP TABLE IF EXISTS password_reset_tokens;
