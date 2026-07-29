// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
	"github.com/golang-jwt/jwt/v5"
)

// JWTService handles JWT token generation and validation
type JWTService struct {
	secretKey          []byte
	accessTokenExpiry  time.Duration
	refreshTokenExpiry time.Duration
	issuer             string
}

// TokenClaims represents the JWT claims structure
type TokenClaims struct {
	UserID         string   `json:"userId"`
	Username       string   `json:"username"`
	Email          string   `json:"email"`
	HierarchyLevel string   `json:"hierarchyLevel"`
	TeamIDs        []string `json:"teamIds"`
	jwt.RegisteredClaims
}

// RefreshTokenClaims represents refresh token claims (minimal info)
type RefreshTokenClaims struct {
	UserID    string `json:"userId"`
	TokenType string `json:"tokenType"`
	jwt.RegisteredClaims
}

// TokenPair contains both access and refresh tokens
type TokenPair struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    int64  `json:"expiresIn"` // Access token expiry in seconds
}

var (
	ErrInvalidToken     = errors.New("invalid token")
	ErrExpiredToken     = errors.New("token has expired")
	ErrInvalidTokenType = errors.New("invalid token type")
	ErrTokenRevoked     = errors.New("token has been revoked")
)

// NewJWTService creates a new JWT service
func NewJWTService() *JWTService {
	log := logger.Get()

	// Get secret from environment or generate a secure default for development
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		// Generate random secret for development (NOT for production!)
		randomBytes := make([]byte, 32)
		rand.Read(randomBytes)
		secret = base64.StdEncoding.EncodeToString(randomBytes)
		log.Security("config_warning").Details("Using randomly generated JWT_SECRET - set JWT_SECRET env var for production").Log()
	}

	// Get expiry durations from environment or use defaults
	accessExpiry := 15 * time.Minute    // Short-lived access token
	refreshExpiry := 7 * 24 * time.Hour // 7 days refresh token

	if envAccessExpiry := os.Getenv("JWT_ACCESS_EXPIRY"); envAccessExpiry != "" {
		if d, err := time.ParseDuration(envAccessExpiry); err == nil {
			accessExpiry = d
		}
	}

	if envRefreshExpiry := os.Getenv("JWT_REFRESH_EXPIRY"); envRefreshExpiry != "" {
		if d, err := time.ParseDuration(envRefreshExpiry); err == nil {
			refreshExpiry = d
		}
	}

	issuer := os.Getenv("JWT_ISSUER")
	if issuer == "" {
		issuer = "teams360"
	}

	return &JWTService{
		secretKey:          []byte(secret),
		accessTokenExpiry:  accessExpiry,
		refreshTokenExpiry: refreshExpiry,
		issuer:             issuer,
	}
}

// GenerateTokenPair creates both access and refresh tokens for a user
func (s *JWTService) GenerateTokenPair(ctx context.Context, userID, username, email, hierarchyLevel string, teamIDs []string) (*TokenPair, error) {
	now := time.Now()

	// Generate access token
	accessTokenClaims := TokenClaims{
		UserID:         userID,
		Username:       username,
		Email:          email,
		HierarchyLevel: hierarchyLevel,
		TeamIDs:        teamIDs,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    s.issuer,
			Subject:   userID,
		},
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessTokenClaims)
	accessTokenString, err := accessToken.SignedString(s.secretKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign access token: %w", err)
	}

	// Generate refresh token (minimal claims for security)
	refreshTokenClaims := RefreshTokenClaims{
		UserID:    userID,
		TokenType: "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(s.refreshTokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    s.issuer,
			Subject:   userID,
		},
	}

	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshTokenClaims)
	refreshTokenString, err := refreshToken.SignedString(s.secretKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessTokenString,
		RefreshToken: refreshTokenString,
		ExpiresIn:    int64(s.accessTokenExpiry.Seconds()),
	}, nil
}

// ValidateAccessToken validates an access token and returns the claims
func (s *JWTService) ValidateAccessToken(tokenString string) (*TokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.secretKey, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*TokenClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// ValidateRefreshToken validates a refresh token and returns the user ID
func (s *JWTService) ValidateRefreshToken(tokenString string) (string, error) {
	token, err := jwt.ParseWithClaims(tokenString, &RefreshTokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.secretKey, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return "", ErrExpiredToken
		}
		return "", ErrInvalidToken
	}

	claims, ok := token.Claims.(*RefreshTokenClaims)
	if !ok || !token.Valid {
		return "", ErrInvalidToken
	}

	// Verify token type
	if claims.TokenType != "refresh" {
		return "", ErrInvalidTokenType
	}

	return claims.UserID, nil
}

// RefreshAccessToken generates a new access token using a valid refresh token
func (s *JWTService) RefreshAccessToken(ctx context.Context, refreshTokenString string, userID, username, email, hierarchyLevel string, teamIDs []string) (string, error) {
	// Validate refresh token
	tokenUserID, err := s.ValidateRefreshToken(refreshTokenString)
	if err != nil {
		return "", err
	}

	// Verify user ID matches
	if tokenUserID != userID {
		return "", ErrInvalidToken
	}

	// Generate new access token
	now := time.Now()
	accessTokenClaims := TokenClaims{
		UserID:         userID,
		Username:       username,
		Email:          email,
		HierarchyLevel: hierarchyLevel,
		TeamIDs:        teamIDs,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    s.issuer,
			Subject:   userID,
		},
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessTokenClaims)
	accessTokenString, err := accessToken.SignedString(s.secretKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign access token: %w", err)
	}

	return accessTokenString, nil
}
