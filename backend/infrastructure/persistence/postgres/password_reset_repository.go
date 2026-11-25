package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres/sqlcgen"
	"golang.org/x/crypto/bcrypt"
)

// PasswordResetRepository implements services.PasswordResetRepository
type PasswordResetRepository struct {
	db      *sql.DB
	queries *sqlcgen.Queries
}

// NewPasswordResetRepository creates a new password reset repository
func NewPasswordResetRepository(db *sql.DB) *PasswordResetRepository {
	return &PasswordResetRepository{db: db, queries: sqlcgen.New(db)}
}

// Create stores a new password reset token
func (r *PasswordResetRepository) Create(ctx context.Context, token *services.PasswordResetToken) error {
	err := r.queries.CreatePasswordResetToken(ctx, sqlcgen.CreatePasswordResetTokenParams{
		ID:        token.ID,
		UserID:    token.UserID,
		TokenHash: token.TokenHash,
		ExpiresAt: token.ExpiresAt,
		CreatedAt: sql.NullTime{Time: token.CreatedAt, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("failed to create password reset token: %w", err)
	}

	return nil
}

// FindValidToken finds a valid (not used, not expired) token by comparing plain token against stored hashes
func (r *PasswordResetRepository) FindValidToken(ctx context.Context, plainToken string) (*services.PasswordResetToken, error) {
	// Query all valid (not used, not expired) tokens
	// Note: In production with many users, you might want to limit this or use a different approach
	rows, err := r.queries.FindValidPasswordResetTokens(ctx, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to query tokens: %w", err)
	}

	// Compare plain token against each hash
	for _, row := range rows {
		token := services.PasswordResetToken{
			ID:        row.ID,
			UserID:    row.UserID,
			TokenHash: row.TokenHash,
			ExpiresAt: row.ExpiresAt,
			CreatedAt: row.CreatedAt.Time,
		}

		if row.UsedAt.Valid {
			token.UsedAt = &row.UsedAt.Time
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
	rowsAffected, err := r.queries.MarkPasswordResetTokenAsUsed(ctx, sqlcgen.MarkPasswordResetTokenAsUsedParams{
		UsedAt: sql.NullTime{Time: time.Now(), Valid: true},
		ID:     tokenID,
	})
	if err != nil {
		return fmt.Errorf("failed to mark token as used: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("token not found")
	}

	return nil
}

// DeleteExpiredTokens cleans up expired tokens (call periodically)
func (r *PasswordResetRepository) DeleteExpiredTokens(ctx context.Context) error {
	// Delete tokens that are expired or older than 24 hours
	err := r.queries.DeleteExpiredPasswordResetTokens(ctx, sqlcgen.DeleteExpiredPasswordResetTokensParams{
		ExpiresBefore: time.Now(),
		CreatedBefore: sql.NullTime{Time: time.Now().Add(-24 * time.Hour), Valid: true},
	})
	if err != nil {
		return fmt.Errorf("failed to delete expired tokens: %w", err)
	}

	return nil
}
