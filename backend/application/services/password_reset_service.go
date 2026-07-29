// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/agopalakrishnan/teams360/backend/domain/user"
	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
	"golang.org/x/crypto/bcrypt"
)

// PasswordResetToken represents a password reset token
type PasswordResetToken struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

// PasswordResetRepository defines the interface for password reset token storage
type PasswordResetRepository interface {
	// Create stores a new password reset token
	Create(ctx context.Context, token *PasswordResetToken) error
	// FindByTokenHash finds a token by its hash (for validation)
	FindValidToken(ctx context.Context, tokenHash string) (*PasswordResetToken, error)
	// MarkAsUsed marks a token as used
	MarkAsUsed(ctx context.Context, tokenID string) error
	// DeleteExpiredTokens cleans up expired tokens
	DeleteExpiredTokens(ctx context.Context) error
}

// EmailService defines the interface for sending emails
type EmailService interface {
	SendPasswordResetEmail(ctx context.Context, email, resetToken string) error
}

// PasswordResetService handles password reset operations
type PasswordResetService struct {
	resetRepo    PasswordResetRepository
	userRepo     user.Repository
	emailService EmailService
	tokenExpiry  time.Duration
}

var (
	ErrInvalidResetToken = errors.New("invalid or expired reset token")
	ErrPasswordTooShort  = errors.New("password must be at least 8 characters")
	ErrInvalidEmail      = errors.New("invalid email format")
)

// NewPasswordResetService creates a new password reset service
func NewPasswordResetService(
	resetRepo PasswordResetRepository,
	userRepo user.Repository,
	emailService EmailService,
) *PasswordResetService {
	return &PasswordResetService{
		resetRepo:    resetRepo,
		userRepo:     userRepo,
		emailService: emailService,
		tokenExpiry:  1 * time.Hour, // Tokens expire in 1 hour
	}
}

// CreateResetToken generates a password reset token for the given email
// Returns the plain token (to be sent via email) or empty string if user not found
// Note: For security, always returns success even if user doesn't exist (prevents email enumeration)
func (s *PasswordResetService) CreateResetToken(email string) (string, error) {
	ctx := context.Background()
	log := logger.Get()

	// Find user by email
	usr, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		// User not found - return empty token but no error (security)
		// Log at debug level to avoid email enumeration via logs
		log.WithField("email", logger.MaskEmail(email)).Debug("password reset requested for email not found in system - returning success to prevent enumeration")
		return "", nil
	}

	// SSO users cannot reset passwords — they authenticate via their identity provider.
	// Return silently (same as user-not-found) to avoid leaking auth type information.
	if usr.AuthType == user.AuthTypeSSO {
		log.WithField("email", logger.MaskEmail(email)).Debug("password reset requested for SSO user - returning success to prevent auth type enumeration")
		return "", nil
	}

	// Generate a secure random token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		log.WithError(err).Error("failed to generate cryptographically secure random bytes for password reset token")
		return "", fmt.Errorf("failed to generate token: %w", err)
	}
	plainToken := base64.URLEncoding.EncodeToString(tokenBytes)

	// Hash the token for storage
	tokenHash, err := bcrypt.GenerateFromPassword([]byte(plainToken), bcrypt.DefaultCost)
	if err != nil {
		log.WithError(err).Error("failed to bcrypt hash password reset token before storage")
		return "", fmt.Errorf("failed to hash token: %w", err)
	}

	// Generate unique ID
	idBytes := make([]byte, 16)
	rand.Read(idBytes)
	tokenID := base64.URLEncoding.EncodeToString(idBytes)

	// Create token record
	resetToken := &PasswordResetToken{
		ID:        tokenID,
		UserID:    usr.ID,
		TokenHash: string(tokenHash),
		ExpiresAt: time.Now().Add(s.tokenExpiry),
		CreatedAt: time.Now(),
	}

	// Save to database
	if err := s.resetRepo.Create(ctx, resetToken); err != nil {
		log.DB("insert").
			Table("password_reset_tokens").
			Context("storing new password reset token for user").
			RecordID(usr.ID).
			Error(err).
			Failure()
		return "", fmt.Errorf("failed to save token: %w", err)
	}

	// Send email (async in production, but for testing we do it sync)
	if s.emailService != nil {
		if err := s.emailService.SendPasswordResetEmail(ctx, email, plainToken); err != nil {
			// Log error but don't fail - token is created
			log.WithError(err).WithField("email", logger.MaskEmail(email)).Warn("password reset token created but email delivery failed - user will not receive reset link")
		}
	}

	log.Security("password_reset_token_created").
		UserID(usr.ID).
		Details("Password reset token generated and stored, expires in 1 hour").
		Log()
	return plainToken, nil
}

// ResetPassword validates the token and updates the user's password
func (s *PasswordResetService) ResetPassword(token, newPassword string) error {
	ctx := context.Background()
	log := logger.Get()

	// Validate password strength
	if len(newPassword) < 8 {
		log.Security("password_reset_rejected").
			Details("New password rejected: must be at least 8 characters, received " + fmt.Sprintf("%d", len(newPassword)) + " characters").
			Log()
		return ErrPasswordTooShort
	}

	// Find all tokens for comparison (we need to check against bcrypt hashes)
	// In production, you might want to limit this or use a different approach
	resetToken, err := s.findValidTokenByPlainToken(ctx, token)
	if err != nil {
		log.Security("password_reset_rejected").
			Details("Reset token not found or failed validation - token may be malformed or not exist in database").
			Log()
		return ErrInvalidResetToken
	}

	// Check if token is expired
	if time.Now().After(resetToken.ExpiresAt) {
		log.Security("password_reset_rejected").
			UserID(resetToken.UserID).
			Details("Reset token has expired - tokens are valid for 1 hour after creation").
			Log()
		return ErrInvalidResetToken
	}

	// Check if token has already been used
	if resetToken.UsedAt != nil {
		log.Security("password_reset_rejected").
			UserID(resetToken.UserID).
			Details("Reset token has already been used - each token can only be used once").
			Log()
		return ErrInvalidResetToken
	}

	// Hash the new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		log.WithError(err).Error("failed to bcrypt hash new password during password reset")
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Update user's password
	if err := s.userRepo.UpdatePassword(ctx, resetToken.UserID, string(hashedPassword)); err != nil {
		log.DB("update").
			Table("users").
			Context("updating user password hash after successful reset token validation").
			RecordID(resetToken.UserID).
			Error(err).
			Failure()
		return fmt.Errorf("failed to update password: %w", err)
	}

	// Mark token as used
	if err := s.resetRepo.MarkAsUsed(ctx, resetToken.ID); err != nil {
		// Log error but don't fail - password is updated
		log.WithError(err).Warn("password updated successfully but failed to mark reset token as used - token may be reusable until expiry")
	}

	log.Security("password_reset_completed").
		UserID(resetToken.UserID).
		Details("User password updated successfully via password reset flow").
		Log()
	return nil
}

// findValidTokenByPlainToken finds a valid token by comparing plain token against stored hashes
func (s *PasswordResetService) findValidTokenByPlainToken(ctx context.Context, plainToken string) (*PasswordResetToken, error) {
	// This is a simplified approach - in production you might want to:
	// 1. Store a non-sensitive identifier alongside the hash
	// 2. Use a database-level hash comparison if supported
	return s.resetRepo.FindValidToken(ctx, plainToken)
}

// ValidateEmail validates an email format
func ValidateEmail(email string) error {
	// Simple email regex pattern
	pattern := `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`
	matched, _ := regexp.MatchString(pattern, email)
	if !matched {
		return ErrInvalidEmail
	}
	return nil
}
