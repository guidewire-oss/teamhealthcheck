// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"net/http"
	"time"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/organization"
	"github.com/agopalakrishnan/teams360/backend/domain/user"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
	"github.com/agopalakrishnan/teams360/backend/pkg/telemetry"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/crypto/bcrypt"
)

// AuthHandler handles authentication-related HTTP requests
type AuthHandler struct {
	userRepo   user.Repository
	orgRepo    organization.Repository
	jwtService *services.JWTService
}

// NewAuthHandler creates a new AuthHandler
func NewAuthHandler(userRepo user.Repository, orgRepo organization.Repository, jwtService *services.JWTService) *AuthHandler {
	return &AuthHandler{
		userRepo:   userRepo,
		orgRepo:    orgRepo,
		jwtService: jwtService,
	}
}

// Login handles user authentication
func (h *AuthHandler) Login(c *gin.Context) {
	ctx := c.Request.Context()
	startTime := time.Now()

	// Start business trace span
	ctx, span := telemetry.StartAuthSpan(ctx, "login")
	defer span.End()

	log := logger.Get().WithContext(ctx)
	clientIP := c.ClientIP()
	requestID := c.GetString("request_id")
	endpoint := "/api/v1/auth/login"

	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		telemetry.RecordLogin(ctx, false, time.Since(startTime), "missing_credentials")
		telemetry.SetSpanError(span, err)
		log.Auth("login").
			IP(clientIP).
			RequestID(requestID).
			Endpoint(endpoint).
			Reason("missing_credentials").
			Details("Request body must contain username and password fields").
			Failure()
		dto.RespondError(c, http.StatusBadRequest, "Username and password are required")
		return
	}

	span.SetAttributes(attribute.String("auth.username_masked", maskUsername(req.Username)))

	// Find user by username using repository
	usr, err := h.userRepo.FindByUsername(ctx, req.Username)
	if err != nil {
		telemetry.RecordLogin(ctx, false, time.Since(startTime), "user_not_found")
		telemetry.SetSpanError(span, err)
		log.Auth("login").
			Username(req.Username).
			IP(clientIP).
			RequestID(requestID).
			Endpoint(endpoint).
			Reason("user_not_found").
			Details("No user exists with the provided username").
			Failure()
		dto.RespondError(c, http.StatusUnauthorized, "Invalid username or password")
		return
	}

	span.SetAttributes(attribute.String("user.id", usr.ID))

	// SSO users cannot log in with username/password
	if usr.AuthType == user.AuthTypeSSO {
		telemetry.RecordLogin(ctx, false, time.Since(startTime), "sso_user_local_login")
		log.Auth("login").
			Username(req.Username).
			UserID(usr.ID).
			IP(clientIP).
			RequestID(requestID).
			Endpoint(endpoint).
			Reason("sso_user_local_login").
			Details("SSO user attempted local password login").
			Failure()
		dto.RespondError(c, http.StatusUnauthorized, "Invalid username or password")
		return
	}

	// Validate password using bcrypt
	if err := bcrypt.CompareHashAndPassword([]byte(usr.PasswordHash), []byte(req.Password)); err != nil {
		telemetry.RecordLogin(ctx, false, time.Since(startTime), "incorrect_password")
		telemetry.SetSpanError(span, err)
		log.Auth("login").
			Username(req.Username).
			UserID(usr.ID).
			IP(clientIP).
			RequestID(requestID).
			Endpoint(endpoint).
			Reason("incorrect_password").
			Details("Password does not match stored hash for user").
			Failure()
		dto.RespondError(c, http.StatusUnauthorized, "Invalid username or password")
		return
	}

	teamIds := collectTeamIDs(ctx, h.userRepo, usr.ID)

	// Generate JWT tokens
	tokenPair, err := h.jwtService.GenerateTokenPair(
		c.Request.Context(),
		usr.ID,
		usr.Username,
		usr.Email,
		usr.HierarchyLevelID,
		teamIds,
	)
	if err != nil {
		log.Auth("login").
			UserID(usr.ID).
			IP(clientIP).
			RequestID(requestID).
			Endpoint(endpoint).
			Reason("jwt_generation_failed").
			Details("Failed to generate JWT access and refresh tokens: " + err.Error()).
			Failure()
		dto.RespondError(c, http.StatusInternalServerError, "Failed to generate authentication tokens")
		return
	}

	// Record successful login metrics
	telemetry.RecordLogin(ctx, true, time.Since(startTime), "")
	telemetry.IncrementActiveSessions(ctx)
	telemetry.SetSpanOK(span)

	// Log successful login
	log.Auth("login").
		UserID(usr.ID).
		IP(clientIP).
		RequestID(requestID).
		Endpoint(endpoint).
		Details("User authenticated successfully, JWT tokens issued").
		Success()

	// Fetch hierarchy level permissions to include canTakeSurvey in response
	canTakeSurvey := false
	if usr.HierarchyLevelID != "" {
		if level, err := h.orgRepo.FindHierarchyLevelByID(c.Request.Context(), usr.HierarchyLevelID); err == nil {
			canTakeSurvey = level.Permissions.CanTakeSurvey
		}
	}

	// Return user info with JWT tokens
	response := dto.LoginResponse{
		User: dto.UserDTO{
			ID:             usr.ID,
			Username:       usr.Username,
			Email:          usr.Email,
			FullName:       usr.Name,
			HierarchyLevel: usr.HierarchyLevelID,
			TeamIds:        teamIds,
			CanTakeSurvey:  canTakeSurvey,
		},
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresIn:    tokenPair.ExpiresIn,
	}

	dto.RespondSuccess(c, http.StatusOK, response)
}

// Refresh handles token refresh requests
func (h *AuthHandler) Refresh(c *gin.Context) {
	ctx := c.Request.Context()

	// Start business trace span
	ctx, span := telemetry.StartAuthSpan(ctx, "token_refresh")
	defer span.End()

	log := logger.Get().WithContext(ctx)
	clientIP := c.ClientIP()
	requestID := c.GetString("request_id")
	endpoint := "/api/v1/auth/refresh"

	var req dto.RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		telemetry.RecordTokenRefresh(ctx, false, "missing_refresh_token")
		telemetry.SetSpanError(span, err)
		log.Auth("token_refresh").
			IP(clientIP).
			RequestID(requestID).
			Endpoint(endpoint).
			Reason("missing_refresh_token").
			Details("Request body must contain refreshToken field").
			Failure()
		dto.RespondError(c, http.StatusBadRequest, "Refresh token is required")
		return
	}

	// Validate refresh token
	userID, err := h.jwtService.ValidateRefreshToken(req.RefreshToken)
	if err != nil {
		reason := "invalid_refresh_token"
		details := "Refresh token failed validation"
		if err.Error() == "token has expired" {
			reason = "refresh_token_expired"
			details = "Refresh token has expired, user must re-authenticate"
		}
		telemetry.RecordTokenRefresh(ctx, false, reason)
		telemetry.SetSpanError(span, err)
		log.Auth("token_refresh").
			IP(clientIP).
			RequestID(requestID).
			Endpoint(endpoint).
			Reason(reason).
			Details(details).
			Failure()
		dto.RespondError(c, http.StatusUnauthorized, "Invalid or expired refresh token")
		return
	}

	span.SetAttributes(attribute.String("user.id", userID))

	// Get user from repository to get current data
	usr, err := h.userRepo.FindByID(ctx, userID)
	if err != nil {
		telemetry.RecordTokenRefresh(ctx, false, "user_not_found")
		telemetry.SetSpanError(span, err)
		log.Auth("token_refresh").
			UserID(userID).
			IP(clientIP).
			RequestID(requestID).
			Endpoint(endpoint).
			Reason("user_deleted_or_not_found").
			Details("User ID from refresh token no longer exists in database").
			Failure()
		dto.RespondError(c, http.StatusUnauthorized, "User not found")
		return
	}

	teamIds := collectTeamIDs(ctx, h.userRepo, usr.ID)

	// Generate new access token
	newAccessToken, err := h.jwtService.RefreshAccessToken(
		ctx,
		req.RefreshToken,
		usr.ID,
		usr.Username,
		usr.Email,
		usr.HierarchyLevelID,
		teamIds,
	)
	if err != nil {
		telemetry.RecordTokenRefresh(ctx, false, "token_generation_failed")
		telemetry.SetSpanError(span, err)
		log.Auth("token_refresh").
			UserID(usr.ID).
			IP(clientIP).
			RequestID(requestID).
			Endpoint(endpoint).
			Reason("access_token_generation_failed").
			Details("Failed to generate new access token from valid refresh token: " + err.Error()).
			Failure()
		dto.RespondError(c, http.StatusUnauthorized, "Failed to refresh token")
		return
	}

	// Record successful token refresh
	telemetry.RecordTokenRefresh(ctx, true, "")
	telemetry.SetSpanOK(span)

	log.Auth("token_refresh").
		UserID(usr.ID).
		IP(clientIP).
		RequestID(requestID).
		Endpoint(endpoint).
		Details("New access token issued successfully").
		Success()

	response := dto.RefreshTokenResponse{
		AccessToken: newAccessToken,
		ExpiresIn:   900, // 15 minutes in seconds (default)
	}

	dto.RespondSuccess(c, http.StatusOK, response)
}

// Logout handles user logout (token invalidation)
func (h *AuthHandler) Logout(c *gin.Context) {
	ctx := c.Request.Context()

	// Start business trace span
	ctx, span := telemetry.StartAuthSpan(ctx, "logout")
	defer span.End()

	log := logger.Get().WithContext(ctx)
	clientIP := c.ClientIP()
	requestID := c.GetString("request_id")
	endpoint := "/api/v1/auth/logout"

	// Try to get user ID from context if authenticated
	userID, exists := c.Get("user_id")
	if exists {
		span.SetAttributes(attribute.String("user.id", userID.(string)))
		log.Auth("logout").
			UserID(userID.(string)).
			IP(clientIP).
			RequestID(requestID).
			Endpoint(endpoint).
			Details("User session ended, client should discard tokens").
			Success()
	} else {
		// Log logout attempt without user context (unauthenticated logout request)
		log.Auth("logout").
			IP(clientIP).
			RequestID(requestID).
			Endpoint(endpoint).
			Details("Logout requested without authenticated session").
			Success()
	}

	// Record logout metric and decrement active sessions
	telemetry.RecordLogout(ctx)
	telemetry.DecrementActiveSessions(ctx)
	telemetry.SetSpanOK(span)

	// For stateless JWT, logout is handled client-side by removing tokens
	// In a production system, you would add the token to a blacklist
	dto.RespondSuccess(c, http.StatusOK, gin.H{"message": "Logged out successfully"})
}

// collectTeamIDs returns a deduplicated list of all team IDs for a user —
// combining direct memberships and teams where the user is the lead.
// It never returns nil; an error fetching either list is treated as empty.
func collectTeamIDs(ctx context.Context, repo user.Repository, userID string) []string {
	teamIds, err := repo.FindTeamIDsForUser(ctx, userID)
	if err != nil || teamIds == nil {
		teamIds = []string{}
	}
	leadTeamIds, err := repo.FindTeamsWhereUserIsLead(ctx, userID)
	if err != nil {
		return teamIds
	}
	seen := make(map[string]bool, len(teamIds))
	for _, id := range teamIds {
		seen[id] = true
	}
	for _, id := range leadTeamIds {
		if !seen[id] {
			seen[id] = true
			teamIds = append(teamIds, id)
		}
	}
	return teamIds
}

// maskUsername masks username for privacy (shows first 2 and last char)
// Example: "johndoe" -> "jo***e"
func maskUsername(username string) string {
	if len(username) <= 3 {
		return "***"
	}
	return username[:2] + "***" + string(username[len(username)-1])
}
