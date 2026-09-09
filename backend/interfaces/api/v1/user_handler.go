package v1

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/agopalakrishnan/teams360/backend/interfaces/middleware"
	"github.com/gin-gonic/gin"
)

// UserHandler handles user-related endpoints
type UserHandler struct {
	db *sql.DB
}

// NewUserHandler creates a new user handler
func NewUserHandler(db *sql.DB) *UserHandler {
	return &UserHandler{
		db: db,
	}
}

// GetCurrentUser returns the currently authenticated user's info
// GET /api/v1/users/me (requires JWT auth)
//
// @Summary Get the current authenticated user
// @Description Returns the profile of the caller identified by the bearer token: ID, username, email, hierarchy level, and team IDs taken directly from the JWT claims, with the full name filled in by a database lookup. Returns a 401 if the request carries no valid authenticated session.
// @Tags Users
// @Produce json
// @Success 200 {object} dto.UserDTO
// @Failure 401 {object} dto.ErrorResponse "User not authenticated"
// @Security BearerAuth
// @Router /users/me [get]
func (h *UserHandler) GetCurrentUser(c *gin.Context) {
	ctx := c.Request.Context()

	// Get user claims from JWT middleware
	claims, ok := middleware.GetClaimsFromContext(c)
	if !ok {
		dto.RespondError(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	// Return user info from JWT claims
	response := dto.UserDTO{
		ID:             claims.UserID,
		Username:       claims.Username,
		Email:          claims.Email,
		HierarchyLevel: claims.HierarchyLevel,
		TeamIds:        claims.TeamIDs,
	}

	// Get full name from database
	var fullName string
	err := h.db.QueryRowContext(ctx, "SELECT full_name FROM users WHERE id = $1", claims.UserID).Scan(&fullName)
	if err == nil {
		response.FullName = fullName
	}

	dto.RespondSuccess(c, http.StatusOK, response)
}

// SetupProtectedUserRoutes sets up user routes that require JWT authentication
func SetupProtectedUserRoutes(router *gin.Engine, db *sql.DB, jwtService *services.JWTService) {
	handler := NewUserHandler(db)

	// Protected user routes (require JWT)
	protected := router.Group("/api/v1/users")
	protected.Use(middleware.JWTAuthMiddleware(jwtService))
	{
		protected.GET("/me", handler.GetCurrentUser)
	}
}

// GetUserSurveyHistory handles GET /api/v1/users/:userId/survey-history
//
// @Summary Get a user's survey history
// @Description Returns the given user's past health check sessions, newest first, each with its team, date, assessment period, completion status, average score, response count, and the full per-dimension responses (dimension name, score, trend, comment). Results can be narrowed to one assessment period and are capped at 10 sessions by default, or by the limit query parameter. Also reports the user's total session count, unaffected by the limit. Access requires the caller to be the same user or their manager.
// @Tags Users
// @Produce json
// @Param userId path string true "User ID"
// @Param assessmentPeriod query string false "Filter by assessment period"
// @Param limit query int false "Max number of sessions to return (default 10)"
// @Success 200 {object} dto.SurveyHistoryResponse
// @Failure 400 {object} dto.ErrorResponse "User ID is required"
// @Failure 500 {object} dto.ErrorResponse "Database query failed or failed to parse survey history data"
// @Security BearerAuth
// @Router /users/{userId}/survey-history [get]
func (h *UserHandler) GetUserSurveyHistory(c *gin.Context) {
	ctx := c.Request.Context()
	userID := c.Param("userId")

	// Validate input
	if userID == "" {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "User ID is required"})
		return
	}

	// Get optional query parameters
	assessmentPeriod := c.Query("assessmentPeriod")
	limitStr := c.Query("limit")

	// Default limit to 10 if not specified
	limit := 10
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	// Build query to get user's survey history
	query := `
		SELECT
			hcs.id as session_id,
			hcs.team_id,
			t.name as team_name,
			hcs.date,
			COALESCE(hcs.assessment_period, '') as assessment_period,
			hcs.completed,
			COALESCE(AVG(hcr.score), 0) as avg_score,
			COUNT(hcr.id) as response_count
		FROM health_check_sessions hcs
		JOIN teams t ON hcs.team_id = t.id
		LEFT JOIN health_check_responses hcr ON hcs.id = hcr.session_id
		WHERE hcs.user_id = $1
			AND ($2 = '' OR hcs.assessment_period = $2)
		GROUP BY hcs.id, hcs.team_id, t.name, hcs.date, hcs.assessment_period, hcs.completed
		ORDER BY hcs.date DESC
		LIMIT $3
	`

	rows, err := h.db.QueryContext(ctx, query, userID, assessmentPeriod, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Database query failed",
			Message: err.Error(),
		})
		return
	}
	defer rows.Close()

	surveyHistory := []dto.SurveyHistoryEntry{}
	for rows.Next() {
		var entry dto.SurveyHistoryEntry

		err := rows.Scan(
			&entry.SessionID,
			&entry.TeamID,
			&entry.TeamName,
			&entry.Date,
			&entry.AssessmentPeriod,
			&entry.Completed,
			&entry.AvgScore,
			&entry.ResponseCount,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
				Error:   "Failed to parse survey history data",
				Message: err.Error(),
			})
			return
		}

		// Initialize empty responses slice
		entry.Responses = []dto.SurveyHistoryResponseItem{}
		surveyHistory = append(surveyHistory, entry)
	}
	rows.Close()

	// Fetch responses for each session
	responsesQuery := `
		SELECT
			hcr.dimension_id,
			hd.name as dimension_name,
			hcr.score,
			COALESCE(hcr.trend, '') as trend,
			COALESCE(hcr.comment, '') as comment
		FROM health_check_responses hcr
		JOIN health_dimensions hd ON hcr.dimension_id = hd.id
		WHERE hcr.session_id = $1
		ORDER BY hd.id
	`

	for i := range surveyHistory {
		responseRows, err := h.db.QueryContext(ctx, responsesQuery, surveyHistory[i].SessionID)
		if err != nil {
			// Log error but continue with empty responses
			continue
		}

		for responseRows.Next() {
			var response dto.SurveyHistoryResponseItem
			err := responseRows.Scan(
				&response.DimensionID,
				&response.DimensionName,
				&response.Score,
				&response.Trend,
				&response.Comment,
			)
			if err != nil {
				continue
			}
			surveyHistory[i].Responses = append(surveyHistory[i].Responses, response)
		}
		responseRows.Close()
	}

	// Get total count of sessions (without limit)
	countQuery := `
		SELECT COUNT(DISTINCT hcs.id)
		FROM health_check_sessions hcs
		WHERE hcs.user_id = $1
			AND ($2 = '' OR hcs.assessment_period = $2)
	`

	var totalSessions int
	err = h.db.QueryRowContext(ctx, countQuery, userID, assessmentPeriod).Scan(&totalSessions)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to get total session count",
			Message: err.Error(),
		})
		return
	}

	response := dto.SurveyHistoryResponse{
		UserID:        userID,
		SurveyHistory: surveyHistory,
		TotalSessions: totalSessions,
	}

	c.JSON(http.StatusOK, response)
}
