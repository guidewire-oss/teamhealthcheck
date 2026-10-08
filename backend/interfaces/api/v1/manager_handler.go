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
				Trend:         dim.Trend,
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

// GetManagerFinalPostWorkshopComments handles GET /api/v1/managers/:managerId/dashboard/final-post-workshop-comments
// Returns free-text comments from completed post-workshop surveys, grouped by team
func (h *ManagerHandler) GetManagerFinalPostWorkshopComments(c *gin.Context) {
	ctx := c.Request.Context()
	managerID := c.Param("managerId")

	if managerID == "" {
		dto.RespondError(c, http.StatusBadRequest, "Manager ID is required")
		return
	}

	assessmentPeriod := c.Query("assessmentPeriod")

	telemetry.RecordManagerDashboardView(ctx, managerID, "final_post_workshop_comments")

	comments, err := h.healthCheckRepo.FindFinalPostWorkshopComments(ctx, managerID, assessmentPeriod)
	if err != nil {
		dto.RespondErrorWithDetails(c, http.StatusInternalServerError, "Database query failed", err.Error())
		return
	}

	grouped := make(map[string][]dto.PostWorkshopComment)
	for _, comment := range comments {
		grouped[comment.TeamID] = append(grouped[comment.TeamID], dto.PostWorkshopComment{
			TeamID:      comment.TeamID,
			SessionID:   comment.SessionID,
			DimensionID: comment.DimensionID,
			Comment:     comment.Comment,
			Date:        comment.Date,
		})
	}

	response := dto.ManagerFinalPostWorkshopCommentsResponse{
		ManagerID:        managerID,
		Comments:         grouped,
		AssessmentPeriod: assessmentPeriod,
	}

	dto.RespondSuccess(c, http.StatusOK, response)
}

// GetManagerMemberOverview returns aggregated individual member survey data, optionally scoped by teamId
func (h *ManagerHandler) GetManagerMemberOverview(c *gin.Context) {
	ctx := c.Request.Context()
	managerID := c.Param("managerId")

	if managerID == "" {
		dto.RespondError(c, http.StatusBadRequest, "Manager ID is required")
		return
	}

	teamID := c.Query("teamId")
	assessmentPeriod := c.Query("assessmentPeriod")

	telemetry.RecordManagerDashboardView(ctx, managerID, "member_overview")

	teamSummaries, err := h.healthCheckRepo.FindMemberOverviewByManager(ctx, managerID, teamID, assessmentPeriod)
	if err != nil {
		dto.RespondErrorWithDetails(c, http.StatusInternalServerError, "Database query failed", err.Error())
		return
	}

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
			TeamID:          summary.TeamID,
			TeamName:        summary.TeamName,
			SubmissionCount: summary.SubmissionCount,
			OverallHealth:   summary.OverallHealth,
			Dimensions:      dimensions,
		}
	}

	response := dto.ManagerMemberOverviewResponse{
		ManagerID:        managerID,
		TeamID:           teamID,
		Teams:            teams,
		AssessmentPeriod: assessmentPeriod,
	}

	dto.RespondSuccess(c, http.StatusOK, response)
}

// GetSubordinates handles GET /api/v1/managers/:managerId/subordinates
// Returns the full subordinate tree for org hierarchy display
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
