// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"

	"github.com/agopalakrishnan/teams360/backend/application/commands"
	"github.com/agopalakrishnan/teams360/backend/application/queries"
	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/healthcheck"
	"github.com/agopalakrishnan/teams360/backend/domain/organization"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
	"github.com/agopalakrishnan/teams360/backend/pkg/telemetry"
)

// Assessment period format regexes
var (
	legacyPeriodRegex     = regexp.MustCompile(`^(\d{4}) - (1st|2nd) Half$`)
	monthlyPeriodRegex    = regexp.MustCompile(`^(\d{4}) (Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)$`)
	quarterlyPeriodRegex  = regexp.MustCompile(`^(\d{4}) Q([1-4])$`)
	halfYearlyPeriodRegex = regexp.MustCompile(`^(\d{4}) H([12])$`)
	yearlyPeriodRegex     = regexp.MustCompile(`^(\d{4})$`)

	monthNameToNumber = map[string]int{
		"Jan": 1, "Feb": 2, "Mar": 3, "Apr": 4, "May": 5, "Jun": 6,
		"Jul": 7, "Aug": 8, "Sep": 9, "Oct": 10, "Nov": 11, "Dec": 12,
	}
)

// HealthCheckHandler handles health check related endpoints
type HealthCheckHandler struct {
	submitHandler       *commands.SubmitHealthCheckHandler
	dimensionsHandler   *queries.GetHealthDimensionsHandler
	teamSessionsHandler *queries.GetTeamSessionsHandler
	repository          healthcheck.Repository
	notificationService *services.NotificationService
}

// NewHealthCheckHandler creates a new handler
func NewHealthCheckHandler(repository healthcheck.Repository, orgRepo organization.Repository, notificationService *services.NotificationService) *HealthCheckHandler {
	return &HealthCheckHandler{
		submitHandler:       commands.NewSubmitHealthCheckHandler(repository),
		dimensionsHandler:   queries.NewGetHealthDimensionsHandler(orgRepo),
		teamSessionsHandler: queries.NewGetTeamSessionsHandler(repository),
		repository:          repository,
		notificationService: notificationService,
	}
}

// SubmitHealthCheck handles POST /api/v1/health-checks
func (h *HealthCheckHandler) SubmitHealthCheck(c *gin.Context) {
	ctx := c.Request.Context()
	startTime := time.Now()

	// Start business trace span
	ctx, span := telemetry.StartHealthCheckSpan(ctx, "submit")
	defer span.End()

	log := logger.Get().WithContext(ctx)

	var req dto.SubmitHealthCheckRequest

	// Bind and validate request
	if err := c.ShouldBindJSON(&req); err != nil {
		telemetry.SetSpanError(span, err)
		log.WithError(err).Warn("invalid health check submission request")
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "Invalid request",
			Message: err.Error(),
		})
		return
	}

	// Validate date is not in the future
	if req.Date != "" {
		parsedDate, err := time.Parse(time.RFC3339Nano, req.Date)
		if err != nil {
			telemetry.SetSpanError(span, err)
			log.WithError(err).Warn("invalid date format in health check submission")
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{
				Error:   "Invalid date format",
				Message: "Date must be in RFC3339 format (e.g., 2024-01-15T10:30:00Z)",
			})
			return
		}
		if parsedDate.After(time.Now().UTC().Truncate(24 * time.Hour)) {
			log.Warn("health check submission with future date rejected")
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{
				Error:   "Invalid date: future dates are not allowed",
				Message: "Health check date cannot be in the future",
			})
			return
		}
	}

	// Validate assessment period format if provided
	if req.AssessmentPeriod != "" {
		if err := validateAssessmentPeriod(req.AssessmentPeriod); err != nil {
			log.WithError(err).Warn("invalid assessment period format")
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{
				Error:   err.Error(),
				Message: "Assessment period must be in a valid format: 'YYYY Mon', 'YYYY Q1-Q4', 'YYYY H1/H2', 'YYYY', or 'YYYY - 1st/2nd Half'",
			})
			return
		}
	}

	// Set span attributes for business context
	telemetry.SetHealthCheckAttributes(span, req.ID, req.TeamID, req.AssessmentPeriod, len(req.Responses))
	span.SetAttributes(
		attribute.String("user.id", req.UserID),
		attribute.Bool("survey.complete", req.Completed),
	)

	// Convert DTO to command
	cmd := commands.SubmitHealthCheckCommand{
		ID:               req.ID,
		TeamID:           req.TeamID,
		UserID:           req.UserID,
		Date:             req.Date,
		AssessmentPeriod: req.AssessmentPeriod,
		SurveyType:       req.SurveyType,
		Responses:        make([]commands.HealthCheckResponseCommand, len(req.Responses)),
		Completed:        req.Completed,
	}

	for i, resp := range req.Responses {
		cmd.Responses[i] = commands.HealthCheckResponseCommand{
			DimensionID: resp.DimensionID,
			Score:       resp.Score,
			Trend:       resp.Trend,
			Comment:     resp.Comment,
		}
	}

	// Execute command
	session, err := h.submitHandler.Handle(cmd)
	if err != nil {
		telemetry.SetSpanError(span, err)
		log.WithError(err).WithField("team_id", req.TeamID).Warn("failed to submit health check")
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "Failed to submit health check",
			Message: err.Error(),
		})
		return
	}

	// Record successful submission metrics
	telemetry.RecordSurveySubmission(ctx, req.TeamID, req.AssessmentPeriod, len(req.Responses), time.Since(startTime))

	// Record individual dimension scores and comments for analytics
	commentsCount := 0
	for _, resp := range req.Responses {
		telemetry.RecordDimensionScore(ctx, resp.DimensionID, float64(resp.Score), resp.Trend)
		telemetry.RecordHealthByDimension(ctx, resp.DimensionID, float64(resp.Score))
		if resp.Comment != "" {
			telemetry.RecordSurveyWithComments(ctx, req.TeamID, resp.DimensionID)
			commentsCount++
		}
	}

	// Record comment rate for this survey
	if len(req.Responses) > 0 {
		commentRate := float64(commentsCount) / float64(len(req.Responses))
		telemetry.RecordSurveyCommentRate(ctx, req.TeamID, commentRate)
	}

	telemetry.SetSpanOK(span)
	log.WithFields(map[string]interface{}{
		"session_id":        session.ID,
		"team_id":           req.TeamID,
		"assessment_period": req.AssessmentPeriod,
		"dimension_count":   len(req.Responses),
	}).Info("health check submitted successfully")

	// Fire async email notification (never blocks the response)
	if h.notificationService != nil {
		go func() {
			bgCtx := context.Background()
			if session.SurveyType == "individual" || session.SurveyType == "" {
				h.notificationService.SendIndividualSurveyEmail(bgCtx, session)
			}
			if session.SurveyType == "post_workshop" {
				h.notificationService.SendPostWorkshopEmails(bgCtx, session)
			}
		}()
	}

	// Convert to response DTO
	response := convertSessionToDTO(session)

	c.JSON(http.StatusCreated, response)
}

// GetHealthDimensions handles GET /api/v1/health-dimensions
func (h *HealthCheckHandler) GetHealthDimensions(c *gin.Context) {
	ctx := c.Request.Context()

	// Start business trace span
	ctx, span := telemetry.StartHealthCheckSpan(ctx, "get_dimensions")
	defer span.End()

	log := logger.Get().WithContext(ctx)

	query := queries.GetHealthDimensionsQuery{
		OnlyActive: true, // Default to only active dimensions
	}

	dimensions, err := h.dimensionsHandler.Handle(query)
	if err != nil {
		telemetry.SetSpanError(span, err)
		log.WithError(err).Error("failed to fetch health dimensions")
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to fetch dimensions",
			Message: err.Error(),
		})
		return
	}

	span.SetAttributes(attribute.Int("dimensions.count", len(dimensions)))
	telemetry.SetSpanOK(span)

	// Convert to response DTO
	response := dto.HealthDimensionsResponse{
		Dimensions: make([]dto.HealthDimensionResponse, len(dimensions)),
	}

	for i, dim := range dimensions {
		response.Dimensions[i] = dto.HealthDimensionResponse{
			ID:              dim.ID,
			Name:            dim.Name,
			Description:     dim.Description,
			GoodDescription: dim.GoodDescription,
			BadDescription:  dim.BadDescription,
			IsActive:        dim.IsActive,
			Weight:          dim.Weight,
		}
	}

	c.JSON(http.StatusOK, response)
}

// GetHealthCheckByID handles GET /api/v1/health-checks/:id
func (h *HealthCheckHandler) GetHealthCheckByID(c *gin.Context) {
	ctx := c.Request.Context()

	// Start business trace span
	ctx, span := telemetry.StartHealthCheckSpan(ctx, "get_by_id")
	defer span.End()

	log := logger.Get().WithContext(ctx)
	id := c.Param("id")

	span.SetAttributes(attribute.String("healthcheck.id", id))

	session, err := h.repository.FindByID(ctx, id)
	if err != nil {
		telemetry.SetSpanError(span, err)
		log.WithField("session_id", id).Debug("health check session not found")
		c.JSON(http.StatusNotFound, dto.ErrorResponse{
			Error:   "Session not found",
			Message: err.Error(),
		})
		return
	}

	telemetry.SetSpanOK(span)
	response := convertSessionToDTO(session)
	c.JSON(http.StatusOK, response)
}

// GetTeamHealthChecks handles GET /api/v1/health-checks/team/:id
func (h *HealthCheckHandler) GetTeamHealthChecks(c *gin.Context) {
	ctx := c.Request.Context()
	startTime := time.Now()

	// Start business trace span
	ctx, span := telemetry.StartHealthCheckSpan(ctx, "get_team_sessions")
	defer span.End()

	log := logger.Get().WithContext(ctx)
	teamID := c.Param("id")
	assessmentPeriod := c.Query("assessmentPeriod")

	span.SetAttributes(
		attribute.String("team.id", teamID),
		attribute.String("healthcheck.assessment_period", assessmentPeriod),
	)

	query := queries.GetTeamSessionsQuery{
		TeamID:           teamID,
		AssessmentPeriod: assessmentPeriod,
	}

	sessions, err := h.teamSessionsHandler.Handle(query)
	if err != nil {
		telemetry.SetSpanError(span, err)
		log.WithError(err).WithField("team_id", teamID).Error("failed to fetch team health check sessions")
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to fetch sessions",
			Message: err.Error(),
		})
		return
	}

	// Record team health query metrics
	telemetry.RecordTeamHealthQuery(ctx, teamID, time.Since(startTime))

	span.SetAttributes(attribute.Int("sessions.count", len(sessions)))
	telemetry.SetSpanOK(span)

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

// GetTeamSubmissionStatus handles GET /api/v1/teams/:teamId/submission-status
func (h *HealthCheckHandler) GetTeamSubmissionStatus(c *gin.Context) {
	ctx := c.Request.Context()
	teamID := c.Param("teamId")
	assessmentPeriod := c.Query("assessmentPeriod")

	if teamID == "" {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error: "Team ID is required",
		})
		return
	}

	if assessmentPeriod == "" {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error: "assessmentPeriod query parameter is required",
		})
		return
	}

	status, err := h.repository.GetTeamSubmissionStatus(ctx, teamID, assessmentPeriod)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to get submission status",
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, dto.TeamSubmissionStatusResponse{
		TeamID:             status.TeamID,
		AssessmentPeriod:   status.AssessmentPeriod,
		TotalMembers:       status.TotalMembers,
		SubmittedMembers:   status.SubmittedMembers,
		AllSubmitted:       status.AllSubmitted,
		PostWorkshopExists: status.PostWorkshopExists,
	})
}

// GetAssessmentPeriods handles GET /api/v1/assessment-periods
func (h *HealthCheckHandler) GetAssessmentPeriods(c *gin.Context) {
	periods, err := h.repository.FindDistinctAssessmentPeriods(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to fetch assessment periods",
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"periods": periods})
}

// Helper function to convert domain model to DTO
func convertSessionToDTO(session *healthcheck.HealthCheckSession) dto.HealthCheckSessionResponse {
	response := dto.HealthCheckSessionResponse{
		ID:               session.ID,
		TeamID:           session.TeamID,
		UserID:           session.UserID,
		Date:             session.Date,
		AssessmentPeriod: session.AssessmentPeriod,
		SurveyType:       session.SurveyType,
		Responses:        make([]dto.HealthCheckResponseResponse, len(session.Responses)),
		Completed:        session.Completed,
	}

	for i, resp := range session.Responses {
		response.Responses[i] = dto.HealthCheckResponseResponse{
			DimensionID: resp.DimensionID,
			Score:       resp.Score,
			Trend:       resp.Trend,
			Comment:     resp.Comment,
		}
	}

	return response
}

// validateAssessmentPeriod validates the assessment period format and ensures it's not in the future.
// Valid formats:
//   - Legacy:      "YYYY - 1st Half" or "YYYY - 2nd Half"
//   - Monthly:     "YYYY Jan" through "YYYY Dec"
//   - Quarterly:   "YYYY Q1" through "YYYY Q4"
//   - Half-yearly: "YYYY H1" or "YYYY H2"
//   - Yearly:      "YYYY"
func validateAssessmentPeriod(period string) error {
	now := time.Now()
	currentYear := now.Year()
	currentMonth := int(now.Month())

	// Try legacy format: "YYYY - 1st Half" or "YYYY - 2nd Half"
	// "YYYY - 1st Half" covers Jul-Dec of YYYY; "YYYY - 2nd Half" covers Jan-Jun of YYYY+1
	if m := legacyPeriodRegex.FindStringSubmatch(period); m != nil {
		year, _ := strconv.Atoi(m[1])
		half := m[2]
		if half == "1st" {
			// Covers Jul-Dec of YYYY — future if we haven't reached Jul of YYYY
			if year > currentYear || (year == currentYear && currentMonth < 7) {
				return fmt.Errorf("invalid assessment period: future assessment periods are not allowed")
			}
		} else {
			// Covers Jan-Jun of YYYY+1 — future if we haven't reached Jan of YYYY+1
			if year+1 > currentYear {
				return fmt.Errorf("invalid assessment period: future assessment periods are not allowed")
			}
		}
		return nil
	}

	// Try monthly format: "YYYY Jan"
	if m := monthlyPeriodRegex.FindStringSubmatch(period); m != nil {
		year, _ := strconv.Atoi(m[1])
		monthNum := monthNameToNumber[m[2]]
		if year > currentYear || (year == currentYear && monthNum > currentMonth) {
			return fmt.Errorf("invalid assessment period: future assessment periods are not allowed")
		}
		return nil
	}

	// Try quarterly format: "YYYY Q1"
	if m := quarterlyPeriodRegex.FindStringSubmatch(period); m != nil {
		year, _ := strconv.Atoi(m[1])
		quarter, _ := strconv.Atoi(m[2])
		currentQuarter := (currentMonth-1)/3 + 1
		if year > currentYear || (year == currentYear && quarter > currentQuarter) {
			return fmt.Errorf("invalid assessment period: future assessment periods are not allowed")
		}
		return nil
	}

	// Try half-yearly format: "YYYY H1"
	if m := halfYearlyPeriodRegex.FindStringSubmatch(period); m != nil {
		year, _ := strconv.Atoi(m[1])
		half, _ := strconv.Atoi(m[2])
		currentHalf := 1
		if currentMonth > 6 {
			currentHalf = 2
		}
		if year > currentYear || (year == currentYear && half > currentHalf) {
			return fmt.Errorf("invalid assessment period: future assessment periods are not allowed")
		}
		return nil
	}

	// Try yearly format: "YYYY"
	if m := yearlyPeriodRegex.FindStringSubmatch(period); m != nil {
		year, _ := strconv.Atoi(m[1])
		if year > currentYear {
			return fmt.Errorf("invalid assessment period: future assessment periods are not allowed")
		}
		return nil
	}

	return fmt.Errorf("invalid assessment period format: must be 'YYYY Mon', 'YYYY Q1-Q4', 'YYYY H1/H2', 'YYYY', or 'YYYY - 1st/2nd Half'")
}
