-- Queries for PasswordResetRepository (backend/infrastructure/persistence/postgres/password_reset_repository.go)
-- Moved verbatim from the hand-written SQL that lived inline in that file.
-- Do not hand-edit sqlcgen/*.go - regenerate with `sqlc generate` (see Makefile).

-- name: CreatePasswordResetToken :exec
INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at, created_at)
VALUES ($1, $2, $3, $4, $5);

-- name: FindValidPasswordResetTokens :many
SELECT id, user_id, token_hash, expires_at, used_at, created_at
FROM password_reset_tokens
WHERE used_at IS NULL AND expires_at > sqlc.arg(now)
ORDER BY created_at DESC
LIMIT 100;

-- name: MarkPasswordResetTokenAsUsed :execrows
UPDATE password_reset_tokens
SET used_at = sqlc.arg(used_at)
WHERE id = sqlc.arg(id);

-- name: DeleteExpiredPasswordResetTokens :exec
DELETE FROM password_reset_tokens
WHERE expires_at < sqlc.arg(expires_before) OR created_at < sqlc.arg(created_before);
