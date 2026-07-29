// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"net/http"
	"strings"

	"github.com/agopalakrishnan/teams360/backend/domain/organization"
	"github.com/agopalakrishnan/teams360/backend/domain/team"
	"github.com/agopalakrishnan/teams360/backend/domain/user"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
	"github.com/gin-gonic/gin"
)

// TeamAdminHandler handles team-related admin HTTP requests
type TeamAdminHandler struct {
	teamRepo team.Repository
	userRepo user.Repository
	orgRepo  organization.Repository
}

// NewTeamAdminHandler creates a new TeamAdminHandler
func NewTeamAdminHandler(teamRepo team.Repository, userRepo user.Repository, orgRepo organization.Repository) *TeamAdminHandler {
	return &TeamAdminHandler{teamRepo: teamRepo, userRepo: userRepo, orgRepo: orgRepo}
}

// ListTeams handles GET /api/v1/admin/teams
func (h *TeamAdminHandler) ListTeams(c *gin.Context) {
	teams, err := h.teamRepo.FindAllWithDetails(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to query teams",
			Message: err.Error(),
		})
		return
	}

	// Convert to DTOs
	teamDTOs := make([]dto.AdminTeamDTO, len(teams))
	for i, tm := range teams {
		teamDTOs[i] = dto.AdminTeamDTO{
			ID:                    tm.ID,
			Name:                  tm.Name,
			TeamLeadID:            tm.TeamLeadID,
			TeamLeadName:          tm.TeamLeadName,
			Cadence:               tm.Cadence,
			DistributionListEmail: tm.DistributionListEmail,
			MemberCount:           tm.MemberCount,
			CreatedAt:             tm.CreatedAt,
			UpdatedAt:             tm.UpdatedAt,
		}
	}

	c.JSON(http.StatusOK, dto.TeamsResponse{
		Teams: teamDTOs,
		Total: len(teamDTOs),
	})
}

// CreateTeam handles POST /api/v1/admin/teams
func (h *TeamAdminHandler) CreateTeam(c *gin.Context) {
	var req dto.CreateTeamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid request body", Message: err.Error()})
		return
	}

	// Auto-generate ID from name if not provided
	teamID := req.ID
	if teamID == "" {
		teamID = generateTeamIDFromName(req.Name)
	}

	// Create team domain model
	tm := &team.Team{
		ID:                    teamID,
		Name:                  req.Name,
		TeamLeadID:            req.TeamLeadID,
		Cadence:               req.Cadence,
		DistributionListEmail: req.DistributionListEmail,
		Members:               []team.TeamMember{},
	}

	// Save using repository
	if err := h.teamRepo.Save(c.Request.Context(), tm); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to create team",
			Message: err.Error(),
		})
		return
	}

	// Look up team lead name if set
	var teamLeadName *string
	if tm.TeamLeadID != nil && *tm.TeamLeadID != "" {
		if leadUser, err := h.userRepo.FindByID(c.Request.Context(), *tm.TeamLeadID); err == nil {
			teamLeadName = &leadUser.Name
		}
	}

	// Convert to DTO and return
	responseDTO := dto.AdminTeamDTO{
		ID:                    tm.ID,
		Name:                  tm.Name,
		TeamLeadID:            tm.TeamLeadID,
		TeamLeadName:          teamLeadName,
		Cadence:               tm.Cadence,
		DistributionListEmail: tm.DistributionListEmail,
		MemberCount:           0,
		CreatedAt:             tm.CreatedAt,
		UpdatedAt:             tm.UpdatedAt,
	}

	// Auto-derive supervisor chain from team lead's reports_to hierarchy
	if tm.TeamLeadID != nil && *tm.TeamLeadID != "" {
		h.deriveSupervisorChainForTeam(c.Request.Context(), tm.ID, *tm.TeamLeadID)
	}

	c.JSON(http.StatusCreated, responseDTO)
}

// UpdateTeam handles PUT /api/v1/admin/teams/:id
func (h *TeamAdminHandler) UpdateTeam(c *gin.Context) {
	id := c.Param("id")
	var req dto.UpdateTeamRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid request body", Message: err.Error()})
		return
	}

	// Check if team exists
	tm, err := h.teamRepo.FindByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Team not found"})
		return
	}

	// Track if team lead changed
	oldLeadID := ""
	if tm.TeamLeadID != nil {
		oldLeadID = *tm.TeamLeadID
	}

	// Update fields if provided
	if req.Name != nil {
		tm.Name = *req.Name
	}
	if req.TeamLeadID != nil {
		tm.TeamLeadID = req.TeamLeadID
	}
	if req.Cadence != nil {
		tm.Cadence = *req.Cadence
	}
	if req.DistributionListEmail != nil {
		tm.DistributionListEmail = req.DistributionListEmail
	}

	// Update using repository
	if err := h.teamRepo.Update(c.Request.Context(), tm); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to update team",
			Message: err.Error(),
		})
		return
	}

	// Re-derive supervisor chain if team lead changed
	newLeadID := ""
	if tm.TeamLeadID != nil {
		newLeadID = *tm.TeamLeadID
	}
	if newLeadID != oldLeadID {
		if newLeadID == "" {
			// Team lead was removed — clear stale supervisor chain
			if err := h.teamRepo.UpdateSupervisorChain(c.Request.Context(), tm.ID, []*team.SupervisorLink{}); err != nil {
				logger.Get().Warn("failed to clear supervisor chain for team " + tm.ID + ": " + err.Error())
			}
		} else {
			h.deriveSupervisorChainForTeam(c.Request.Context(), tm.ID, newLeadID)
		}
	}

	// Re-fetch team to get updated TeamLeadName from JOIN
	updatedTm, err := h.teamRepo.FindByID(c.Request.Context(), tm.ID)
	if err != nil {
		// Fallback to original tm if re-fetch fails
		updatedTm = tm
	}

	// Get member count
	memberCount, _ := h.teamRepo.CountTeamMembers(c.Request.Context(), tm.ID)

	// Convert to DTO and return
	responseDTO := dto.AdminTeamDTO{
		ID:                    updatedTm.ID,
		Name:                  updatedTm.Name,
		TeamLeadID:            updatedTm.TeamLeadID,
		TeamLeadName:          updatedTm.TeamLeadName,
		Cadence:               updatedTm.Cadence,
		DistributionListEmail: updatedTm.DistributionListEmail,
		MemberCount:           memberCount,
		CreatedAt:             updatedTm.CreatedAt,
		UpdatedAt:             updatedTm.UpdatedAt,
	}

	c.JSON(http.StatusOK, responseDTO)
}

// DeleteTeam handles DELETE /api/v1/admin/teams/:id
func (h *TeamAdminHandler) DeleteTeam(c *gin.Context) {
	id := c.Param("id")

	if err := h.teamRepo.Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to delete team",
			Message: err.Error(),
		})
		return
	}

	dto.RespondMessage(c, http.StatusOK, "Team deleted successfully")
}

// GetTeamMembers handles GET /api/v1/admin/teams/:id/members
func (h *TeamAdminHandler) GetTeamMembers(c *gin.Context) {
	teamID := c.Param("id")

	// Verify team exists
	_, err := h.teamRepo.FindByID(c.Request.Context(), teamID)
	if err != nil {
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Team not found"})
		return
	}

	members, err := h.teamRepo.FindTeamMembers(c.Request.Context(), teamID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to fetch team members",
			Message: err.Error(),
		})
		return
	}

	memberDTOs := make([]dto.TeamMemberAdminDTO, len(members))
	for i, m := range members {
		memberDTOs[i] = dto.TeamMemberAdminDTO{
			UserID:   m.ID,
			UserName: m.FullName,
			Email:    m.Email,
		}
	}

	c.JSON(http.StatusOK, dto.TeamMembersResponse{
		Members: memberDTOs,
		Total:   len(memberDTOs),
	})
}

// AddTeamMember handles POST /api/v1/admin/teams/:id/members
func (h *TeamAdminHandler) AddTeamMember(c *gin.Context) {
	teamID := c.Param("id")

	var req dto.AddTeamMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid request body", Message: err.Error()})
		return
	}

	// Verify team exists
	_, err := h.teamRepo.FindByID(c.Request.Context(), teamID)
	if err != nil {
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Team not found"})
		return
	}

	// Verify user exists
	_, err = h.userRepo.FindByID(c.Request.Context(), req.UserID)
	if err != nil {
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "User not found"})
		return
	}

	if err := h.teamRepo.AddMember(c.Request.Context(), teamID, req.UserID); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to add team member",
			Message: err.Error(),
		})
		return
	}

	dto.RespondMessage(c, http.StatusCreated, "Member added successfully")
}

// RemoveTeamMember handles DELETE /api/v1/admin/teams/:id/members/:userId
func (h *TeamAdminHandler) RemoveTeamMember(c *gin.Context) {
	teamID := c.Param("id")
	userID := c.Param("userId")

	if err := h.teamRepo.RemoveMember(c.Request.Context(), teamID, userID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Team member not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to remove team member",
			Message: err.Error(),
		})
		return
	}

	dto.RespondMessage(c, http.StatusOK, "Member removed successfully")
}

// GetSupervisorChain handles GET /api/v1/admin/teams/:id/supervisors
func (h *TeamAdminHandler) GetSupervisorChain(c *gin.Context) {
	teamID := c.Param("id")

	// Verify team exists
	_, err := h.teamRepo.FindByID(c.Request.Context(), teamID)
	if err != nil {
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Team not found"})
		return
	}

	chain, err := h.teamRepo.FindSupervisorChain(c.Request.Context(), teamID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to fetch supervisor chain",
			Message: err.Error(),
		})
		return
	}

	// Enrich with user and level names
	supervisors := make([]dto.SupervisorLinkDTO, len(chain))
	for i, link := range chain {
		supervisors[i] = dto.SupervisorLinkDTO{
			UserID:  link.UserID,
			LevelID: link.LevelID,
		}

		// Look up user name
		u, err := h.userRepo.FindByID(c.Request.Context(), link.UserID)
		if err == nil {
			supervisors[i].UserName = u.Name
		}

		// Look up level name
		level, err := h.orgRepo.FindHierarchyLevelByID(c.Request.Context(), link.LevelID)
		if err == nil {
			supervisors[i].LevelName = level.Name
		}
	}

	c.JSON(http.StatusOK, dto.SupervisorChainResponse{
		TeamID:      teamID,
		Supervisors: supervisors,
	})
}

// UpdateSupervisorChain handles PUT /api/v1/admin/teams/:id/supervisors
func (h *TeamAdminHandler) UpdateSupervisorChain(c *gin.Context) {
	teamID := c.Param("id")

	var req dto.UpdateSupervisorChainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid request body", Message: err.Error()})
		return
	}

	// Verify team exists
	_, err := h.teamRepo.FindByID(c.Request.Context(), teamID)
	if err != nil {
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Team not found"})
		return
	}

	// Convert to domain model
	chain := make([]*team.SupervisorLink, len(req.Supervisors))
	for i, s := range req.Supervisors {
		chain[i] = &team.SupervisorLink{
			UserID:  s.UserID,
			LevelID: s.LevelID,
		}
	}

	if err := h.teamRepo.UpdateSupervisorChain(c.Request.Context(), teamID, chain); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to update supervisor chain",
			Message: err.Error(),
		})
		return
	}

	// Return the updated chain with enriched data
	h.GetSupervisorChain(c)
}

// generateTeamIDFromName creates a URL-safe ID from a team name
// e.g., "Test Team 123" -> "test-team-123"
func generateTeamIDFromName(name string) string {
	// Convert to lowercase and replace spaces with hyphens
	id := strings.ToLower(name)
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

// deriveSupervisorChainForTeam walks up the team lead's reports_to hierarchy
// and populates the team_supervisors table as a derived cache.
func (h *TeamAdminHandler) deriveSupervisorChainForTeam(ctx context.Context, teamID, teamLeadID string) {
	log := logger.Get()

	supervisors, err := h.userRepo.FindSupervisorChainUp(ctx, teamLeadID)
	if err != nil {
		log.Warn("failed to derive supervisor chain for team " + teamID + ": " + err.Error())
		return
	}

	chain := make([]*team.SupervisorLink, len(supervisors))
	for i, sup := range supervisors {
		chain[i] = &team.SupervisorLink{
			UserID:  sup.ID,
			LevelID: sup.HierarchyLevelID,
		}
	}

	if err := h.teamRepo.UpdateSupervisorChain(ctx, teamID, chain); err != nil {
		log.Warn("failed to save derived supervisor chain for team " + teamID + ": " + err.Error())
	}
}
