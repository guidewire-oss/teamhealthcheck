// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"net/http"
	"strings"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
	"github.com/gin-gonic/gin"
)

// JWTAuthMiddleware creates a middleware that validates JWT tokens
func JWTAuthMiddleware(jwtService *services.JWTService) gin.HandlerFunc {
	return func(c *gin.Context) {
		log := logger.Get()
		clientIP := c.ClientIP()
		requestID := c.GetString("request_id")
		endpoint := c.Request.URL.Path

		// Get Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			log.Auth("token_validation").
				IP(clientIP).
				RequestID(requestID).
				Endpoint(endpoint).
				Reason("missing_authorization_header").
				Details("Request to protected endpoint lacks Authorization header").
				Failure()
			dto.RespondError(c, http.StatusUnauthorized, "Authorization header is required")
			c.Abort()
			return
		}

		// Check Bearer prefix
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			log.Auth("token_validation").
				IP(clientIP).
				RequestID(requestID).
				Endpoint(endpoint).
				Reason("invalid_authorization_format").
				Details("Authorization header must use 'Bearer <token>' format").
				Failure()
			dto.RespondError(c, http.StatusUnauthorized, "Authorization header must be Bearer token")
			c.Abort()
			return
		}

		tokenString := parts[1]

		// Validate token
		claims, err := jwtService.ValidateAccessToken(tokenString)
		if err != nil {
			var reason, details string
			switch err {
			case services.ErrExpiredToken:
				reason = "access_token_expired"
				details = "JWT access token has expired, client should use refresh token to obtain new access token"
				dto.RespondError(c, http.StatusUnauthorized, "Token has expired")
			case services.ErrInvalidToken:
				reason = "access_token_invalid"
				details = "JWT access token is malformed, tampered with, or signed with wrong key"
				dto.RespondError(c, http.StatusUnauthorized, "Invalid token")
			default:
				reason = "token_validation_error"
				details = "Unexpected error during token validation: " + err.Error()
				dto.RespondError(c, http.StatusUnauthorized, "Authentication failed")
			}
			log.Auth("token_validation").
				IP(clientIP).
				RequestID(requestID).
				Endpoint(endpoint).
				Reason(reason).
				Details(details).
				Failure()
			c.Abort()
			return
		}

		// Store user info in context for downstream handlers
		c.Set("userID", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("email", claims.Email)
		c.Set("hierarchyLevel", claims.HierarchyLevel)
		c.Set("teamIDs", claims.TeamIDs)
		c.Set("claims", claims)

		c.Next()
	}
}

// OptionalJWTAuthMiddleware validates JWT if present but doesn't require it
func OptionalJWTAuthMiddleware(jwtService *services.JWTService) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.Next()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			c.Next()
			return
		}

		tokenString := parts[1]
		claims, err := jwtService.ValidateAccessToken(tokenString)
		if err != nil {
			c.Next()
			return
		}

		// Store user info in context
		c.Set("userID", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("email", claims.Email)
		c.Set("hierarchyLevel", claims.HierarchyLevel)
		c.Set("teamIDs", claims.TeamIDs)
		c.Set("claims", claims)

		c.Next()
	}
}

// GetUserIDFromContext extracts user ID from gin context
func GetUserIDFromContext(c *gin.Context) (string, bool) {
	userID, exists := c.Get("userID")
	if !exists {
		return "", false
	}
	return userID.(string), true
}

// GetClaimsFromContext extracts JWT claims from gin context
func GetClaimsFromContext(c *gin.Context) (*services.TokenClaims, bool) {
	claims, exists := c.Get("claims")
	if !exists {
		return nil, false
	}
	return claims.(*services.TokenClaims), true
}

// AdminOnlyMiddleware ensures only admin users (level-1) can access the route
// Must be used AFTER JWTAuthMiddleware
func AdminOnlyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		log := logger.Get()
		clientIP := c.ClientIP()
		requestID := c.GetString("request_id")
		endpoint := c.Request.URL.Path

		hierarchyLevel, exists := c.Get("hierarchyLevel")
		if !exists {
			log.Auth("authorization").
				IP(clientIP).
				RequestID(requestID).
				Endpoint(endpoint).
				Reason("missing_hierarchy_level").
				Details("User hierarchy level not found in context - ensure JWTAuthMiddleware runs first").
				Failure()
			dto.RespondError(c, http.StatusForbidden, "Access denied: unable to determine user role")
			c.Abort()
			return
		}

		// Admin is level-1 or level-admin (special admin account)
		level := hierarchyLevel.(string)
		if level != "level-1" && level != "level-admin" {
			userID, _ := c.Get("userID")
			log.Auth("authorization").
				UserID(userID.(string)).
				IP(clientIP).
				RequestID(requestID).
				Endpoint(endpoint).
				Reason("insufficient_privileges").
				Details("User attempted to access admin-only endpoint without admin privileges").
				Failure()
			dto.RespondError(c, http.StatusForbidden, "Access denied: admin privileges required")
			c.Abort()
			return
		}

		c.Next()
	}
}

// ManagerOrAboveMiddleware ensures only manager level users (level-3 or above) can access the route
// Hierarchy levels: level-1 (VP/Admin), level-2 (Director), level-3 (Manager)
// Must be used AFTER JWTAuthMiddleware
func ManagerOrAboveMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		log := logger.Get()
		clientIP := c.ClientIP()
		requestID := c.GetString("request_id")
		endpoint := c.Request.URL.Path

		hierarchyLevel, exists := c.Get("hierarchyLevel")
		if !exists {
			log.Auth("authorization").
				IP(clientIP).
				RequestID(requestID).
				Endpoint(endpoint).
				Reason("missing_hierarchy_level").
				Details("User hierarchy level not found in context - ensure JWTAuthMiddleware runs first").
				Failure()
			dto.RespondError(c, http.StatusForbidden, "Access denied: unable to determine user role")
			c.Abort()
			return
		}

		level := hierarchyLevel.(string)
		// Manager or above: level-1 (VP/Admin), level-2 (Director), level-3 (Manager), level-admin (System Admin)
		allowedLevels := map[string]bool{
			"level-1":     true, // VP/Admin
			"level-2":     true, // Director
			"level-3":     true, // Manager
			"level-admin": true, // System Admin
		}

		if !allowedLevels[level] {
			userID, _ := c.Get("userID")
			log.Auth("authorization").
				UserID(userID.(string)).
				IP(clientIP).
				RequestID(requestID).
				Endpoint(endpoint).
				Reason("insufficient_privileges").
				Details("User attempted to access manager endpoint without manager or above privileges").
				Failure()
			dto.RespondError(c, http.StatusForbidden, "Access denied: manager or above privileges required")
			c.Abort()
			return
		}

		c.Next()
	}
}

// SameUserOrManagerMiddleware ensures the requesting user is accessing their own data
// OR has manager or above privileges to view subordinate data
// The managerId path parameter must match the authenticated user's ID
// Must be used AFTER JWTAuthMiddleware
func SameUserOrManagerMiddleware(paramName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		log := logger.Get()
		clientIP := c.ClientIP()
		requestID := c.GetString("request_id")
		endpoint := c.Request.URL.Path

		userID, exists := c.Get("userID")
		if !exists {
			dto.RespondError(c, http.StatusForbidden, "Access denied: user not authenticated")
			c.Abort()
			return
		}

		targetUserID := c.Param(paramName)
		hierarchyLevel, hlExists := c.Get("hierarchyLevel")
		if !hlExists {
			dto.RespondError(c, http.StatusForbidden, "Access denied: unable to determine user role")
			c.Abort()
			return
		}
		level := hierarchyLevel.(string)

		// Allow if user is accessing their own data
		if userID.(string) == targetUserID {
			c.Next()
			return
		}

		// Only directors (level-2) and above (level-1, level-admin) can access other users' data
		// Managers (level-3) should only access their own data, not other managers' data
		higherLevelRoles := map[string]bool{
			"level-1":     true, // VP
			"level-2":     true, // Director
			"level-admin": true, // System Admin
		}

		if higherLevelRoles[level] {
			// Directors and above can view any manager's data (they oversee managers)
			// TODO: In a full implementation, we'd verify the target user is actually
			// a subordinate of the requesting user in the org hierarchy
			c.Next()
			return
		}

		log.Auth("authorization").
			UserID(userID.(string)).
			IP(clientIP).
			RequestID(requestID).
			Endpoint(endpoint).
			Reason("access_denied_other_user_data").
			Details("User attempted to access another user's data without sufficient privileges").
			Failure()
		dto.RespondError(c, http.StatusForbidden, "Access denied: cannot access other user's data")
		c.Abort()
	}
}

// TeamMembershipMiddleware ensures the user is a member of the team being accessed
// OR has manager or above privileges
// The teamId path parameter is used to check membership
// Must be used AFTER JWTAuthMiddleware
func TeamMembershipMiddleware(paramName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		log := logger.Get()
		clientIP := c.ClientIP()
		requestID := c.GetString("request_id")
		endpoint := c.Request.URL.Path

		userID, exists := c.Get("userID")
		if !exists {
			dto.RespondError(c, http.StatusForbidden, "Access denied: user not authenticated")
			c.Abort()
			return
		}

		targetTeamID := c.Param(paramName)
		if targetTeamID == "" {
			c.Next() // No team specified, let handler deal with it
			return
		}

		hierarchyLevel, hlExists := c.Get("hierarchyLevel")
		if !hlExists {
			dto.RespondError(c, http.StatusForbidden, "Access denied: unable to determine user role")
			c.Abort()
			return
		}
		level := hierarchyLevel.(string)

		// Manager or above can access any team (in their hierarchy)
		allowedLevels := map[string]bool{
			"level-1":     true, // VP/Admin
			"level-2":     true, // Director
			"level-3":     true, // Manager
			"level-admin": true, // System Admin
		}

		if allowedLevels[level] {
			c.Next()
			return
		}

		// For team members and team leads, check team membership
		teamIDs, exists := c.Get("teamIDs")
		if !exists {
			dto.RespondError(c, http.StatusForbidden, "Access denied: team membership not available")
			c.Abort()
			return
		}

		teamIDList, ok := teamIDs.([]string)
		if !ok {
			dto.RespondError(c, http.StatusForbidden, "Access denied: invalid team membership data")
			c.Abort()
			return
		}

		// Check if user is a member of the target team
		isMember := false
		for _, teamID := range teamIDList {
			if teamID == targetTeamID {
				isMember = true
				break
			}
		}

		if !isMember {
			log.Auth("authorization").
				UserID(userID.(string)).
				IP(clientIP).
				RequestID(requestID).
				Endpoint(endpoint).
				Reason("team_access_denied").
				Details("User attempted to access team data they are not a member of").
				Failure()
			dto.RespondError(c, http.StatusForbidden, "Access denied: you are not a member of this team")
			c.Abort()
			return
		}

		c.Next()
	}
}
