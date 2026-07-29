// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/agopalakrishnan/teams360/backend/application/trends"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/agopalakrishnan/teams360/backend/pkg/telemetry"
	"github.com/gin-gonic/gin"
)

// TeamDashboardHandler handles team lead dashboard endpoints
type TeamDashboardHandler struct {
	db            *sql.DB
	trendsService *trends.Service
}

// NewTeamDashboardHandler creates a new team dashboard handler
func NewTeamDashboardHandler(db *sql.DB) *TeamDashboardHandler {
	return &TeamDashboardHandler{
		db:            db,
		trendsService: trends.NewService(db),
	}
}

// GetHealthSummary handles GET /api/v1/teams/:teamId/dashboard/health-summary
// Returns radar chart data (avg score per dimension)
func (h *TeamDashboardHandler) GetHealthSummary(c *gin.Context) {
	ctx := c.Request.Context()
	teamID := c.Param("teamId")

	// Validate input
	if teamID == "" {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "Team ID is required",
			Message: "teamId parameter cannot be empty",
		})
		return
	}

	assessmentPeriod := c.Query("assessmentPeriod") // Optional filter

	// Record team lead dashboard view
	telemetry.RecordTeamLeadDashboardView(ctx, teamID, "health_summary")

	// Query to get team info, overall health, and dimension averages
	query := `
		WITH team_info AS (
			SELECT id, name
			FROM teams
			WHERE id = $1
		),
		session_stats AS (
			SELECT
				COUNT(DISTINCT hcs.id) as submission_count,
				AVG(hcr.score) as overall_health
			FROM health_check_sessions hcs
			LEFT JOIN health_check_responses hcr ON hcs.id = hcr.session_id
			WHERE hcs.team_id = $1
				AND hcs.completed = true
				AND ($2 = '' OR hcs.assessment_period = $2)
		),
		dimension_health AS (
			SELECT
				hcr.dimension_id,
				AVG(hcr.score) as avg_score,
				COUNT(hcr.id) as response_count
			FROM health_check_responses hcr
			INNER JOIN health_check_sessions hcs ON hcr.session_id = hcs.id
			WHERE hcs.team_id = $1
				AND hcs.completed = true
				AND ($2 = '' OR hcs.assessment_period = $2)
			GROUP BY hcr.dimension_id
		)
		SELECT
			ti.id,
			ti.name,
			COALESCE(ss.submission_count, 0) as submission_count,
			COALESCE(ss.overall_health, 0) as overall_health,
			COALESCE(
				json_agg(
					json_build_object(
						'dimensionId', dh.dimension_id,
						'avgScore', dh.avg_score,
						'responseCount', dh.response_count
					) ORDER BY dh.dimension_id
				) FILTER (WHERE dh.dimension_id IS NOT NULL),
				'[]'::json
			) as dimensions
		FROM team_info ti
		CROSS JOIN session_stats ss
		LEFT JOIN dimension_health dh ON true
		GROUP BY ti.id, ti.name, ss.submission_count, ss.overall_health
	`

	var teamID_result string
	var teamName string
	var submissionCount int
	var overallHealth float64
	var dimensionsJSON []byte

	err := h.db.QueryRowContext(ctx, query, teamID, assessmentPeriod).Scan(
		&teamID_result,
		&teamName,
		&submissionCount,
		&overallHealth,
		&dimensionsJSON,
	)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, dto.ErrorResponse{
			Error:   "Team not found",
			Message: "No team found with the given ID",
		})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Database query failed",
			Message: err.Error(),
		})
		return
	}

	// Parse dimensions JSON
	var dimensions []dto.DimensionSummary
	if len(dimensionsJSON) > 0 && string(dimensionsJSON) != "[]" {
		err = json.Unmarshal(dimensionsJSON, &dimensions)
		if err != nil {
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
				Error:   "Failed to parse dimension data",
				Message: err.Error(),
			})
			return
		}
	} else {
		dimensions = []dto.DimensionSummary{}
	}

	response := dto.TeamDashboardHealthSummary{
		TeamID:           teamID_result,
		TeamName:         teamName,
		AssessmentPeriod: assessmentPeriod,
		Dimensions:       dimensions,
		OverallHealth:    overallHealth,
		SubmissionCount:  submissionCount,
	}

	c.JSON(http.StatusOK, response)
}

// GetResponseDistribution handles GET /api/v1/teams/:teamId/dashboard/response-distribution
// Returns score distribution per dimension (red/yellow/green counts for bar chart)
func (h *TeamDashboardHandler) GetResponseDistribution(c *gin.Context) {
	ctx := c.Request.Context()
	teamID := c.Param("teamId")

	if teamID == "" {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "Team ID is required",
			Message: "teamId parameter cannot be empty",
		})
		return
	}

	assessmentPeriod := c.Query("assessmentPeriod") // Optional filter

	// Record team lead dashboard view
	telemetry.RecordTeamLeadDashboardView(ctx, teamID, "response_distribution")

	// Query to count red/yellow/green scores per dimension
	query := `
		SELECT
			hcr.dimension_id,
			COUNT(CASE WHEN hcr.score = 1 THEN 1 END) as red,
			COUNT(CASE WHEN hcr.score = 2 THEN 1 END) as yellow,
			COUNT(CASE WHEN hcr.score = 3 THEN 1 END) as green
		FROM health_check_responses hcr
		INNER JOIN health_check_sessions hcs ON hcr.session_id = hcs.id
		WHERE hcs.team_id = $1
			AND hcs.completed = true
			AND ($2 = '' OR hcs.assessment_period = $2)
		GROUP BY hcr.dimension_id
		ORDER BY hcr.dimension_id
	`

	rows, err := h.db.QueryContext(ctx, query, teamID, assessmentPeriod)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Database query failed",
			Message: err.Error(),
		})
		return
	}
	defer rows.Close()

	distribution := []dto.DimensionDistribution{}
	for rows.Next() {
		var dim dto.DimensionDistribution
		err := rows.Scan(&dim.DimensionID, &dim.Red, &dim.Yellow, &dim.Green)
		if err != nil {
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
				Error:   "Failed to parse distribution data",
				Message: err.Error(),
			})
			return
		}
		distribution = append(distribution, dim)
	}

	response := dto.ResponseDistribution{
		TeamID:       teamID,
		Distribution: distribution,
	}

	c.JSON(http.StatusOK, response)
}

// GetIndividualResponses handles GET /api/v1/teams/:teamId/dashboard/individual-responses
// Returns individual team member responses with comments
func (h *TeamDashboardHandler) GetIndividualResponses(c *gin.Context) {
	ctx := c.Request.Context()
	teamID := c.Param("teamId")

	if teamID == "" {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "Team ID is required",
			Message: "teamId parameter cannot be empty",
		})
		return
	}

	assessmentPeriod := c.Query("assessmentPeriod") // Optional filter

	// Record team lead dashboard view
	telemetry.RecordTeamLeadDashboardView(ctx, teamID, "individual_responses")

	// Query to get individual sessions with aggregated responses
	query := `
		WITH session_responses AS (
			SELECT
				hcs.id as session_id,
				hcs.user_id,
				u.full_name as user_name,
				hcs.date,
				hcs.survey_type,
				json_agg(
					json_build_object(
						'dimensionId', hcr.dimension_id,
						'score', hcr.score,
						'trend', hcr.trend,
						'comment', COALESCE(hcr.comment, '')
					) ORDER BY hcr.dimension_id
				) as dimensions
			FROM health_check_sessions hcs
			INNER JOIN users u ON hcs.user_id = u.id
			INNER JOIN health_check_responses hcr ON hcs.id = hcr.session_id
			WHERE hcs.team_id = $1
				AND hcs.completed = true
				AND ($2 = '' OR hcs.assessment_period = $2)
			GROUP BY hcs.id, hcs.user_id, u.full_name, hcs.date, hcs.survey_type
		)
		SELECT
			session_id,
			user_id,
			user_name,
			date,
			survey_type,
			dimensions
		FROM session_responses
		ORDER BY date DESC
	`

	rows, err := h.db.QueryContext(ctx, query, teamID, assessmentPeriod)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Database query failed",
			Message: err.Error(),
		})
		return
	}
	defer rows.Close()

	responses := []dto.IndividualUserResponse{}
	for rows.Next() {
		var resp dto.IndividualUserResponse
		var dimensionsJSON []byte

		err := rows.Scan(
			&resp.SessionID,
			&resp.UserID,
			&resp.UserName,
			&resp.Date,
			&resp.SurveyType,
			&dimensionsJSON,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
				Error:   "Failed to parse response data",
				Message: err.Error(),
			})
			return
		}

		// Parse dimensions JSON
		if len(dimensionsJSON) > 0 {
			err = json.Unmarshal(dimensionsJSON, &resp.Dimensions)
			if err != nil {
				c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
					Error:   "Failed to parse dimension data",
					Message: err.Error(),
				})
				return
			}
		} else {
			resp.Dimensions = []dto.IndividualDimensionResp{}
		}

		responses = append(responses, resp)
	}

	result := dto.IndividualResponses{
		TeamID:    teamID,
		Responses: responses,
	}

	c.JSON(http.StatusOK, result)
}

// GetTrends handles GET /api/v1/teams/:teamId/dashboard/trends
// Returns trend data across assessment periods
func (h *TeamDashboardHandler) GetTrends(c *gin.Context) {
	ctx := c.Request.Context()
	teamID := c.Param("teamId")

	if teamID == "" {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "Team ID is required",
			Message: "teamId parameter cannot be empty",
		})
		return
	}

	// Record team lead dashboard view for trends
	telemetry.RecordTeamLeadDashboardView(ctx, teamID, "trends")
	telemetry.RecordTrendReportView(ctx, teamID, "team_lead")

	result, err := h.trendsService.GetTrendsForTeam(ctx, teamID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to fetch trend data",
			Message: err.Error(),
		})
		return
	}

	response := dto.TrendData{
		TeamID:     teamID,
		Periods:    result.Periods,
		Dimensions: result.Dimensions,
	}

	c.JSON(http.StatusOK, response)
}
