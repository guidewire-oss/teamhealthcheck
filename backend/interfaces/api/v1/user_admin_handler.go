// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"net/http"
	"strings"

	"github.com/agopalakrishnan/teams360/backend/domain/team"
	"github.com/agopalakrishnan/teams360/backend/domain/user"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// UserAdminHandler handles user-related admin HTTP requests
type UserAdminHandler struct {
	userRepo user.Repository
	teamRepo team.Repository
}

// NewUserAdminHandler creates a new UserAdminHandler
func NewUserAdminHandler(userRepo user.Repository, teamRepo team.Repository) *UserAdminHandler {
	return &UserAdminHandler{userRepo: userRepo, teamRepo: teamRepo}
}

// ListUsers handles GET /api/v1/admin/users
func (h *UserAdminHandler) ListUsers(c *gin.Context) {
	users, err := h.userRepo.FindAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to query users",
			Message: err.Error(),
		})
		return
	}

	// Convert to DTOs
	userDTOs := make([]dto.AdminUserDTO, len(users))
	for i, usr := range users {
		// Fetch team IDs
		teamIds, _ := h.userRepo.FindTeamIDsForUser(c.Request.Context(), usr.ID)
		if teamIds == nil {
			teamIds = []string{}
		}

		userDTOs[i] = dto.AdminUserDTO{
			ID:             usr.ID,
			Username:       usr.Username,
			Email:          usr.Email,
			FullName:       usr.Name,
			HierarchyLevel: usr.HierarchyLevelID,
			ReportsTo:      usr.ReportsTo,
			TeamIds:        teamIds,
			AuthType:       string(usr.AuthType),
			CreatedAt:      usr.CreatedAt,
			UpdatedAt:      usr.UpdatedAt,
		}
	}

	c.JSON(http.StatusOK, dto.UsersResponse{
		Users: userDTOs,
		Total: len(userDTOs),
	})
}

// CreateUser handles POST /api/v1/admin/users
func (h *UserAdminHandler) CreateUser(c *gin.Context) {
	var req dto.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid request body", Message: err.Error()})
		return
	}

	// Determine auth type (default to local)
	authType := user.AuthTypeLocal
	if req.AuthType == "sso" {
		authType = user.AuthTypeSSO
	}

	// For local users, password is required
	var passwordHash string
	if authType == user.AuthTypeLocal {
		if len(req.Password) < 4 {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Password is required for local users (min 4 characters)"})
			return
		}
		hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to hash password"})
			return
		}
		passwordHash = string(hashed)
	}
	// SSO users get no password hash

	// Auto-generate ID from username if not provided
	userID := req.ID
	if userID == "" {
		userID = generateUserIDFromUsername(req.Username)
	}

	// Create user domain model
	usr := &user.User{
		ID:               userID,
		Username:         req.Username,
		Email:            req.Email,
		Name:             req.FullName,
		HierarchyLevelID: req.HierarchyLevel,
		ReportsTo:        req.ReportsTo,
		PasswordHash:     passwordHash,
		AuthType:         authType,
		TeamIDs:          []string{},
	}

	// Save using repository
	if err := h.userRepo.Save(c.Request.Context(), usr); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to create user",
			Message: err.Error(),
		})
		return
	}

	// Convert to DTO and return
	responseDTO := dto.AdminUserDTO{
		ID:             usr.ID,
		Username:       usr.Username,
		Email:          usr.Email,
		FullName:       usr.Name,
		HierarchyLevel: usr.HierarchyLevelID,
		ReportsTo:      usr.ReportsTo,
		TeamIds:        usr.TeamIDs,
		AuthType:       string(usr.AuthType),
		CreatedAt:      usr.CreatedAt,
		UpdatedAt:      usr.UpdatedAt,
	}

	c.JSON(http.StatusCreated, responseDTO)
}

// UpdateUser handles PUT /api/v1/admin/users/:id
func (h *UserAdminHandler) UpdateUser(c *gin.Context) {
	id := c.Param("id")
	var req dto.UpdateUserRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid request body", Message: err.Error()})
		return
	}

	// Check if user exists
	usr, err := h.userRepo.FindByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "User not found"})
		return
	}

	// Capture pre-update values for change detection
	oldReportsTo := usr.ReportsTo
	oldHierarchyLevel := usr.HierarchyLevelID

	// Update fields if provided
	if req.Username != nil {
		usr.Username = *req.Username
	}
	if req.Email != nil {
		usr.Email = *req.Email
	}
	if req.FullName != nil {
		usr.Name = *req.FullName
	}
	if req.HierarchyLevel != nil {
		usr.HierarchyLevelID = *req.HierarchyLevel
	}
	if req.ReportsTo != nil {
		usr.ReportsTo = req.ReportsTo
	}
	// Track auth type transition for password requirement
	oldAuthType := usr.AuthType
	if req.AuthType != nil {
		switch *req.AuthType {
		case "sso":
			usr.AuthType = user.AuthTypeSSO
		case "local":
			usr.AuthType = user.AuthTypeLocal
		default:
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid authType (must be 'local' or 'sso')"})
			return
		}
	}

	// Handle password update
	var newPasswordHash string
	if req.Password != nil && *req.Password != "" {
		if usr.AuthType == user.AuthTypeSSO {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Cannot set password for SSO users"})
			return
		}
		hashed, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to hash password"})
			return
		}
		newPasswordHash = string(hashed)
	}

	// Switching from SSO to local requires a password
	if oldAuthType == user.AuthTypeSSO && usr.AuthType == user.AuthTypeLocal && newPasswordHash == "" {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Password is required when switching from SSO to local authentication"})
		return
	}

	// Update using repository
	if err := h.userRepo.Update(c.Request.Context(), usr); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to update user",
			Message: err.Error(),
		})
		return
	}

	// Update password separately (Update() doesn't touch password_hash)
	if newPasswordHash != "" {
		if err := h.userRepo.UpdatePassword(c.Request.Context(), usr.ID, newPasswordHash); err != nil {
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
				Error:   "Failed to update password",
				Message: err.Error(),
			})
			return
		}
	}

	// Re-derive supervisor chains only if reports_to or hierarchy level actually changed
	reportsToChanged := !ptrStrEqual(oldReportsTo, usr.ReportsTo)
	hierarchyChanged := oldHierarchyLevel != usr.HierarchyLevelID
	if reportsToChanged || hierarchyChanged {
		h.rederiveSupervisorChains(c.Request.Context(), usr.ID)
	}

	// Fetch team IDs
	teamIds, _ := h.userRepo.FindTeamIDsForUser(c.Request.Context(), usr.ID)
	if teamIds == nil {
		teamIds = []string{}
	}

	// Convert to DTO and return
	responseDTO := dto.AdminUserDTO{
		ID:             usr.ID,
		Username:       usr.Username,
		Email:          usr.Email,
		FullName:       usr.Name,
		HierarchyLevel: usr.HierarchyLevelID,
		ReportsTo:      usr.ReportsTo,
		TeamIds:        teamIds,
		AuthType:       string(usr.AuthType),
		CreatedAt:      usr.CreatedAt,
		UpdatedAt:      usr.UpdatedAt,
	}

	c.JSON(http.StatusOK, responseDTO)
}

// DeleteUser handles DELETE /api/v1/admin/users/:id
func (h *UserAdminHandler) DeleteUser(c *gin.Context) {
	id := c.Param("id")

	if err := h.userRepo.Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to delete user",
			Message: err.Error(),
		})
		return
	}

	dto.RespondMessage(c, http.StatusOK, "User deleted successfully")
}

// generateUserIDFromUsername creates a URL-safe ID from a username
// e.g., "test_user" -> "test-user"
func generateUserIDFromUsername(username string) string {
	// Convert to lowercase, replace underscores and spaces with hyphens
	id := strings.ToLower(username)
	id = strings.ReplaceAll(id, "_", "-")
	id = strings.ReplaceAll(id, " ", "-")
	// Remove any characters that aren't alphanumeric or hyphens
	var result strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// ptrStrEqual compares two *string values for equality (nil-safe).
func ptrStrEqual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// rederiveSupervisorChains re-derives supervisor chains for all teams affected
// by a change to the given user's reports_to or hierarchy level.
func (h *UserAdminHandler) rederiveSupervisorChains(ctx context.Context, userID string) {
	log := logger.Get()

	// Find teams where this user is team lead
	leadTeams, err := h.teamRepo.FindByLeadID(ctx, userID)
	if err != nil {
		log.Warn("failed to find teams for lead " + userID + ": " + err.Error())
	}

	// Find teams where this user is in the supervisor chain
	supervisedTeams, err := h.teamRepo.FindBySupervisorID(ctx, userID)
	if err != nil {
		log.Warn("failed to find supervised teams for " + userID + ": " + err.Error())
	}

	// Collect unique team IDs that need re-derivation
	teamsToUpdate := make(map[string]*team.Team)
	for _, t := range leadTeams {
		teamsToUpdate[t.ID] = t
	}
	for _, t := range supervisedTeams {
		teamsToUpdate[t.ID] = t
	}

	// Re-derive each team's supervisor chain from its team lead
	for _, t := range teamsToUpdate {
		if t.TeamLeadID == nil || *t.TeamLeadID == "" {
			continue
		}
		supervisors, err := h.userRepo.FindSupervisorChainUp(ctx, *t.TeamLeadID)
		if err != nil {
			log.Warn("failed to derive supervisor chain for team " + t.ID + ": " + err.Error())
			continue
		}
		chain := make([]*team.SupervisorLink, len(supervisors))
		for i, sup := range supervisors {
			chain[i] = &team.SupervisorLink{
				UserID:  sup.ID,
				LevelID: sup.HierarchyLevelID,
			}
		}
		if err := h.teamRepo.UpdateSupervisorChain(ctx, t.ID, chain); err != nil {
			log.Warn("failed to update supervisor chain for team " + t.ID + ": " + err.Error())
		}
	}
}
