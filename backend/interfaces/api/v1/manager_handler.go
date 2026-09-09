package v1

import (
	"net/http"

	"github.com/agopalakrishnan/teams360/backend/application/trends"
	"github.com/agopalakrishnan/teams360/backend/domain/healthcheck"
	"github.com/agopalakrishnan/teams360/backend/domain/user"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/agopalakrishnan/teams360/backend/pkg/telemetry"
	"github.com/gin-gonic/gin"
)

// ManagerHandler handles manager-related endpoints
type ManagerHandler struct {
	healthCheckRepo healthcheck.Repository
	trendsService   *trends.Service
	userRepo        user.Repository
}

// NewManagerHandler creates a new manager handler
func NewManagerHandler(healthCheckRepo healthcheck.Repository, trendsService *trends.Service, userRepo user.Repository) *ManagerHandler {
	return &ManagerHandler{
		healthCheckRepo: healthCheckRepo,
		trendsService:   trendsService,
		userRepo:        userRepo,
	}
}

// GetManagerTeamsHealth handles GET /api/v1/managers/:managerId/teams/health
//
// @Summary Get team health for manager
// @Description Returns a health summary for every team supervised by the given manager, optionally narrowed to one assessment period via a query parameter. Each team summary includes its submission count, overall health score, per-dimension averages with response counts, and post-workshop survey status. Access requires a manager-or-above role; the caller may view their own data, and callers with a director-or-above role (level-2 or level-1) may also view any other manager's data.
// @Tags Managers
// @Produce json
// @Param managerId path string true "Manager ID"
// @Param assessmentPeriod query string false "Filter by assessment period"
// @Success 200 {object} dto.ManagerTeamsHealthResponse
// @Failure 400 {object} dto.ErrorResponse "Manager ID is required"
// @Failure 500 {object} dto.ErrorResponse "Database query failed"
// @Security BearerAuth
// @Router /managers/{managerId}/teams/health [get]
func (h *ManagerHandler) GetManagerTeamsHealth(c *gin.Context) {
	ctx := c.Request.Context()
	managerID := c.Param("managerId")

	// Validate input
	if managerID == "" {
		dto.RespondError(c, http.StatusBadRequest, "Manager ID is required")
		return
	}

	assessmentPeriod := c.Query("assessmentPeriod") // Optional filter

	// Record manager dashboard view
	telemetry.RecordManagerDashboardView(ctx, managerID, "teams_health")

	// Use repository to fetch aggregated team health data
	teamSummaries, err := h.healthCheckRepo.FindTeamHealthByManager(ctx, managerID, assessmentPeriod)
	if err != nil {
		dto.RespondErrorWithDetails(c, http.StatusInternalServerError, "Database query failed", err.Error())
		return
	}

	// Convert domain models to DTOs
	teams := make([]dto.TeamHealthSummary, len(teamSummaries))
	for i, summary := range teamSummaries {
		dimensions := make([]dto.DimensionSummary, len(summary.Dimensions))
		for j, dim := range summary.Dimensions {
			dimensions[j] = dto.DimensionSummary{
				DimensionID:   dim.DimensionID,
				AvgScore:      dim.AvgScore,
				ResponseCount: dim.ResponseCount,
			}
		}

		teams[i] = dto.TeamHealthSummary{
			TeamID:             summary.TeamID,
			TeamName:           summary.TeamName,
			SubmissionCount:    summary.SubmissionCount,
			OverallHealth:      summary.OverallHealth,
			Dimensions:         dimensions,
			PostWorkshopStatus: summary.PostWorkshopStatus,
		}
	}

	response := dto.ManagerTeamsHealthResponse{
		ManagerID:        managerID,
		Teams:            teams,
		TotalTeams:       len(teams),
		AssessmentPeriod: assessmentPeriod,
	}

	dto.RespondSuccess(c, http.StatusOK, response)
}

// GetManagerAggregatedRadar handles GET /api/v1/managers/:managerId/dashboard/radar
// Returns aggregated radar chart data across all supervised teams
//
// @Summary Get radar chart data for manager
// @Description Returns the average score and response count for each health dimension, aggregated across every team supervised by the given manager, optionally narrowed to one assessment period via a query parameter. Shaped for rendering a radar chart on the manager dashboard. Access requires a manager-or-above role; the caller may view their own data, and callers with a director-or-above role (level-2 or level-1) may also view any other manager's data.
// @Tags Managers
// @Produce json
// @Param managerId path string true "Manager ID"
// @Param assessmentPeriod query string false "Filter by assessment period"
// @Success 200 {object} dto.ManagerRadarResponse
// @Failure 400 {object} dto.ErrorResponse "Manager ID is required"
// @Failure 500 {object} dto.ErrorResponse "Database query failed"
// @Security BearerAuth
// @Router /managers/{managerId}/dashboard/radar [get]
func (h *ManagerHandler) GetManagerAggregatedRadar(c *gin.Context) {
	ctx := c.Request.Context()
	managerID := c.Param("managerId")

	if managerID == "" {
		dto.RespondError(c, http.StatusBadRequest, "Manager ID is required")
		return
	}

	assessmentPeriod := c.Query("assessmentPeriod")

	// Record manager dashboard view
	telemetry.RecordManagerDashboardView(ctx, managerID, "radar")

	// Use repository to fetch aggregated dimension scores
	dimensionSummaries, err := h.healthCheckRepo.FindAggregatedDimensionsByManager(ctx, managerID, assessmentPeriod)
	if err != nil {
		dto.RespondErrorWithDetails(c, http.StatusInternalServerError, "Database query failed", err.Error())
		return
	}

	// Convert domain models to DTOs
	dimensions := make([]dto.DimensionSummary, len(dimensionSummaries))
	for i, summary := range dimensionSummaries {
		dimensions[i] = dto.DimensionSummary{
			DimensionID:   summary.DimensionID,
			AvgScore:      summary.AvgScore,
			ResponseCount: summary.ResponseCount,
		}
	}

	response := dto.ManagerRadarResponse{
		ManagerID:        managerID,
		Dimensions:       dimensions,
		AssessmentPeriod: assessmentPeriod,
	}

	dto.RespondSuccess(c, http.StatusOK, response)
}

// GetManagerTrends handles GET /api/v1/managers/:managerId/dashboard/trends
// Returns trend data across assessment periods for all supervised teams
//
// @Summary Get trend data for supervised teams
// @Description Returns the list of assessment periods and, for each health dimension, the average score in each of those periods, aggregated across every team the given manager supervises. Used to render trend lines on the manager dashboard. Access requires a manager-or-above role; the caller may view their own data, and callers with a director-or-above role (level-2 or level-1) may also view any other manager's data.
// @Tags Managers
// @Produce json
// @Param managerId path string true "Manager ID"
// @Success 200 {object} dto.ManagerTrendsResponse
// @Failure 400 {object} dto.ErrorResponse "Manager ID is required"
// @Failure 500 {object} dto.ErrorResponse "Failed to fetch trend data"
// @Security BearerAuth
// @Router /managers/{managerId}/dashboard/trends [get]
func (h *ManagerHandler) GetManagerTrends(c *gin.Context) {
	ctx := c.Request.Context()
	managerID := c.Param("managerId")

	if managerID == "" {
		dto.RespondError(c, http.StatusBadRequest, "Manager ID is required")
		return
	}

	// Record manager dashboard view for trends
	telemetry.RecordManagerDashboardView(ctx, managerID, "trends")
	telemetry.RecordTrendReportView(ctx, managerID, "manager")

	result, err := h.trendsService.GetTrendsForManager(ctx, managerID)
	if err != nil {
		dto.RespondErrorWithDetails(c, http.StatusInternalServerError, "Failed to fetch trend data", err.Error())
		return
	}

	// Convert trend dimensions to DTO format
	dimensions := make([]dto.ManagerDimensionTrend, len(result.Dimensions))
	for i, dim := range result.Dimensions {
		dimensions[i] = dto.ManagerDimensionTrend(dim)
	}

	response := dto.ManagerTrendsResponse{
		ManagerID:  managerID,
		Periods:    result.Periods,
		Dimensions: dimensions,
	}

	dto.RespondSuccess(c, http.StatusOK, response)
}

// GetSubordinates handles GET /api/v1/managers/:managerId/subordinates
// Returns the full subordinate tree for org hierarchy display
//
// @Summary Get a manager's subordinates
// @Description Returns every direct and indirect report of the given manager as a flat list, each entry carrying the subordinate's ID, username, name, hierarchy level, immediate supervisor, and team memberships, for rendering the org hierarchy view. Access requires a manager-or-above role; the caller may view their own subordinates, and callers with a director-or-above role (level-2 or level-1) may also view another manager's subordinates.
// @Tags Managers
// @Produce json
// @Param managerId path string true "Manager ID"
// @Success 200 {object} dto.SubordinatesResponse
// @Failure 400 {object} dto.ErrorResponse "Manager ID is required"
// @Failure 500 {object} dto.ErrorResponse "Failed to fetch subordinates"
// @Security BearerAuth
// @Router /managers/{managerId}/subordinates [get]
func (h *ManagerHandler) GetSubordinates(c *gin.Context) {
	ctx := c.Request.Context()
	managerID := c.Param("managerId")

	if managerID == "" {
		dto.RespondError(c, http.StatusBadRequest, "Manager ID is required")
		return
	}

	subordinates, err := h.userRepo.FindSubordinates(ctx, managerID)
	if err != nil {
		dto.RespondErrorWithDetails(c, http.StatusInternalServerError, "Failed to fetch subordinates", err.Error())
		return
	}

	subs := make([]dto.SubordinateDTO, len(subordinates))
	for i, u := range subordinates {
		reportsTo := ""
		if u.ReportsTo != nil {
			reportsTo = *u.ReportsTo
		}
		subs[i] = dto.SubordinateDTO{
			ID:               u.ID,
			Username:         u.Username,
			Name:             u.Name,
			HierarchyLevelID: u.HierarchyLevelID,
			ReportsTo:        reportsTo,
			TeamIDs:          u.TeamIDs,
		}
	}

	response := dto.SubordinatesResponse{
		ManagerID:    managerID,
		Subordinates: subs,
	}

	dto.RespondSuccess(c, http.StatusOK, response)
}
