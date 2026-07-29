// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"golang.org/x/crypto/bcrypt"
)

// PasswordResetRepository implements services.PasswordResetRepository
type PasswordResetRepository struct {
	db *sql.DB
}

// NewPasswordResetRepository creates a new password reset repository
func NewPasswordResetRepository(db *sql.DB) *PasswordResetRepository {
	return &PasswordResetRepository{db: db}
}

// Create stores a new password reset token
func (r *PasswordResetRepository) Create(ctx context.Context, token *services.PasswordResetToken) error {
	query := `
		INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`

	_, err := r.db.ExecContext(ctx, query,
		token.ID,
		token.UserID,
		token.TokenHash,
		token.ExpiresAt,
		token.CreatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create password reset token: %w", err)
	}

	return nil
}

// FindValidToken finds a valid (not used, not expired) token by comparing plain token against stored hashes
func (r *PasswordResetRepository) FindValidToken(ctx context.Context, plainToken string) (*services.PasswordResetToken, error) {
	// Query all valid (not used, not expired) tokens
	// Note: In production with many users, you might want to limit this or use a different approach
	query := `
		SELECT id, user_id, token_hash, expires_at, used_at, created_at
		FROM password_reset_tokens
		WHERE used_at IS NULL AND expires_at > $1
		ORDER BY created_at DESC
		LIMIT 100
	`

	rows, err := r.db.QueryContext(ctx, query, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to query tokens: %w", err)
	}
	defer rows.Close()

	// Compare plain token against each hash
	for rows.Next() {
		var token services.PasswordResetToken
		var usedAt sql.NullTime

		err := rows.Scan(
			&token.ID,
			&token.UserID,
			&token.TokenHash,
			&token.ExpiresAt,
			&usedAt,
			&token.CreatedAt,
		)
		if err != nil {
			continue
		}

		if usedAt.Valid {
			token.UsedAt = &usedAt.Time
		}

		// Compare plain token with stored hash
		if err := bcrypt.CompareHashAndPassword([]byte(token.TokenHash), []byte(plainToken)); err == nil {
			return &token, nil
		}
	}

	return nil, fmt.Errorf("token not found")
}

// MarkAsUsed marks a token as used
func (r *PasswordResetRepository) MarkAsUsed(ctx context.Context, tokenID string) error {
	query := `
		UPDATE password_reset_tokens
		SET used_at = $1
		WHERE id = $2
	`

	result, err := r.db.ExecContext(ctx, query, time.Now(), tokenID)
	if err != nil {
		return fmt.Errorf("failed to mark token as used: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("token not found")
	}

	return nil
}

// DeleteExpiredTokens cleans up expired tokens (call periodically)
func (r *PasswordResetRepository) DeleteExpiredTokens(ctx context.Context) error {
	query := `
		DELETE FROM password_reset_tokens
		WHERE expires_at < $1 OR created_at < $2
	`

	// Delete tokens that are expired or older than 24 hours
	_, err := r.db.ExecContext(ctx, query, time.Now(), time.Now().Add(-24*time.Hour))
	if err != nil {
		return fmt.Errorf("failed to delete expired tokens: %w", err)
	}

	return nil
}
