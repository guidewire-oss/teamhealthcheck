// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/healthcheck"
	"github.com/agopalakrishnan/teams360/backend/domain/team"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/agopalakrishnan/teams360/backend/interfaces/middleware"
)

// TeamHandler handles team-related endpoints
type TeamHandler struct {
	healthCheckRepo healthcheck.Repository
	teamRepo        team.Repository
}

// NewTeamHandler creates a new team handler
func NewTeamHandler(healthCheckRepo healthcheck.Repository, teamRepo team.Repository) *TeamHandler {
	return &TeamHandler{
		healthCheckRepo: healthCheckRepo,
		teamRepo:        teamRepo,
	}
}

// GetTeamSessions handles GET /api/v1/teams/:teamId
// Returns all health check sessions for a team with their responses
func (h *TeamHandler) GetTeamSessions(c *gin.Context) {
	teamID := c.Param("teamId")

	// Validate input
	if teamID == "" {
		dto.RespondErrorWithDetails(c, http.StatusBadRequest, "Team ID is required", "teamId parameter cannot be empty")
		return
	}

	// Get optional assessment period filter
	assessmentPeriod := c.Query("assessmentPeriod")

	var sessions []*healthcheck.HealthCheckSession
	var err error

	// Fetch sessions based on filter
	if assessmentPeriod != "" {
		// Fetch all sessions and filter in memory
		allSessions, err := h.healthCheckRepo.FindByTeamID(c.Request.Context(), teamID)
		if err != nil {
			dto.RespondErrorWithDetails(c, http.StatusInternalServerError, "Failed to fetch team sessions", err.Error())
			return
		}
		sessions = make([]*healthcheck.HealthCheckSession, 0)
		for _, session := range allSessions {
			if session.AssessmentPeriod == assessmentPeriod {
				sessions = append(sessions, session)
			}
		}
	} else {
		// Fetch all sessions for team
		sessions, err = h.healthCheckRepo.FindByTeamID(c.Request.Context(), teamID)
	}

	if err != nil {
		dto.RespondErrorWithDetails(c, http.StatusInternalServerError, "Failed to fetch team sessions", err.Error())
		return
	}

	// Convert to response DTO
	response := dto.HealthCheckSessionsResponse{
		Sessions: make([]dto.HealthCheckSessionResponse, len(sessions)),
		Total:    len(sessions),
	}

	for i, session := range sessions {
		response.Sessions[i] = convertSessionToDTO(session)
	}

	c.JSON(http.StatusOK, response)
}

// GetTeamInfo handles GET /api/v1/teams/:teamId/info
// Returns team details (id, name, cadence, members)
func (h *TeamHandler) GetTeamInfo(c *gin.Context) {
	teamID := c.Param("teamId")

	if teamID == "" {
		dto.RespondErrorWithDetails(c, http.StatusBadRequest, "Team ID is required", "teamId parameter cannot be empty")
		return
	}

	// Fetch team from repository
	tm, err := h.teamRepo.FindByID(c.Request.Context(), teamID)
	if err != nil {
		dto.RespondErrorWithDetails(c, http.StatusNotFound, "Team not found", err.Error())
		return
	}

	// Get team members
	members, err := h.teamRepo.FindTeamMembers(c.Request.Context(), teamID)
	if err != nil {
		dto.RespondErrorWithDetails(c, http.StatusInternalServerError, "Failed to fetch team members", err.Error())
		return
	}

	// Convert to DTO
	memberDTOs := make([]dto.TeamMember, len(members))
	for i, member := range members {
		memberDTOs[i] = dto.TeamMember{
			ID:       member.ID,
			Username: member.Username,
			FullName: member.FullName,
		}
	}

	response := dto.TeamInfoResponse{
		ID:      tm.ID,
		Name:    tm.Name,
		Cadence: tm.Cadence,
		Members: memberDTOs,
	}

	if tm.TeamLeadID != nil {
		response.TeamLeadID = *tm.TeamLeadID
		if tm.TeamLeadName != nil {
			response.TeamLeadName = *tm.TeamLeadName
		}
	}

	dto.RespondSuccess(c, http.StatusOK, response)
}

// ListTeams handles GET /api/v1/teams
// Returns a list of all teams
func (h *TeamHandler) ListTeams(c *gin.Context) {
	// Use repository to fetch all teams with details
	teams, err := h.teamRepo.FindAllWithDetails(c.Request.Context())
	if err != nil {
		dto.RespondErrorWithDetails(c, http.StatusInternalServerError, "Failed to fetch teams", err.Error())
		return
	}

	// Convert to DTOs
	teamSummaries := make([]dto.TeamSummary, len(teams))
	for i, tm := range teams {
		summary := dto.TeamSummary{
			ID:          tm.ID,
			Name:        tm.Name,
			Cadence:     tm.Cadence,
			MemberCount: tm.MemberCount,
		}

		if tm.TeamLeadID != nil {
			summary.TeamLeadID = *tm.TeamLeadID
			if tm.TeamLeadName != nil {
				summary.TeamLeadName = *tm.TeamLeadName
			}
		}

		teamSummaries[i] = summary
	}

	response := dto.TeamListResponse{
		Teams: teamSummaries,
		Total: len(teamSummaries),
	}

	dto.RespondSuccess(c, http.StatusOK, response)
}

// SetupTeamRoutes registers team-related routes
// All team routes require JWT authentication
// Team-specific routes require team membership or manager+ privileges
func SetupTeamRoutes(router *gin.Engine, healthCheckRepo healthcheck.Repository, teamRepo team.Repository, jwtService *services.JWTService) {
	handler := NewTeamHandler(healthCheckRepo, teamRepo)

	// Team routes - require authentication
	teams := router.Group("/api/v1/teams")
	teams.Use(middleware.JWTAuthMiddleware(jwtService))
	{
		// List all teams (authenticated users only)
		teams.GET("", handler.ListTeams)

		// Team-specific routes require team membership or manager+ privileges
		teamSpecific := teams.Group("/:teamId")
		teamSpecific.Use(middleware.TeamMembershipMiddleware("teamId"))
		{
			teamSpecific.GET("/sessions", handler.GetTeamSessions)
			teamSpecific.GET("/info", handler.GetTeamInfo)
		}
	}
}
