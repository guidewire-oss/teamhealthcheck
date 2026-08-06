-- Reverse: add password_hash column to users table
DROP INDEX IF EXISTS idx_users_username;

ALTER TABLE users DROP COLUMN IF EXISTS password_hash;
