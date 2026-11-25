-- users.id is VARCHAR(255), but these two columns that hold a user id were
-- created narrower. Re-keying an SSO user to a provider id longer than 36 (or
-- 50) characters failed when they had a password-reset token (or a health
-- check session) and aborted the whole organization sync. Widening a VARCHAR
-- is a metadata-only change in PostgreSQL: no table rewrite, indexes stay valid.
ALTER TABLE health_check_sessions ALTER COLUMN user_id TYPE VARCHAR(255);
ALTER TABLE password_reset_tokens ALTER COLUMN user_id TYPE VARCHAR(255);
