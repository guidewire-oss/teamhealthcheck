-- Fails if any stored user id is longer than the old limit; truncating ids
-- would corrupt the references, so this is left to fail loudly.
ALTER TABLE health_check_sessions ALTER COLUMN user_id TYPE VARCHAR(50);
ALTER TABLE password_reset_tokens ALTER COLUMN user_id TYPE VARCHAR(36);
