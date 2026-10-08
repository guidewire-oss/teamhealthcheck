package commands

import (
	"context"
	"fmt"
	"time"

	"github.com/agopalakrishnan/teams360/backend/domain/healthcheck"
)

// parseSubmissionDate parses a session date in either RFC3339 (as sent by the frontend on
// submission) or plain "YYYY-MM-DD" (as returned by Postgres DATE columns) form.
func parseSubmissionDate(date string) (time.Time, bool) {
	if t, err := time.Parse(time.RFC3339, date); err == nil {
		return t, true
	}
	if t, err := time.Parse("2006-01-02", date); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// SubmitHealthCheckCommand represents the command to submit a health check
type SubmitHealthCheckCommand struct {
	ID               string
	TeamID           string
	UserID           string
	Date             string
	AssessmentPeriod string
	SurveyType       string
	Responses        []HealthCheckResponseCommand
	Completed        bool
}

// HealthCheckResponseCommand represents a response in the command
type HealthCheckResponseCommand struct {
	DimensionID string
	Score       int
	Trend       string
	Comment     string
}

// SubmitHealthCheckHandler handles the submit health check command
type SubmitHealthCheckHandler struct {
	repository healthcheck.Repository
}

// NewSubmitHealthCheckHandler creates a new command handler
func NewSubmitHealthCheckHandler(repository healthcheck.Repository) *SubmitHealthCheckHandler {
	return &SubmitHealthCheckHandler{
		repository: repository,
	}
}

// Handle executes the command
func (h *SubmitHealthCheckHandler) Handle(cmd SubmitHealthCheckCommand) (*healthcheck.HealthCheckSession, error) {
	// Generate ID if not provided
	if cmd.ID == "" {
		cmd.ID = fmt.Sprintf("session-%d", time.Now().UnixNano())
	}

	// Validate command
	if err := h.validate(cmd); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Default survey type
	surveyType := cmd.SurveyType
	if surveyType == "" {
		surveyType = healthcheck.SurveyTypeIndividual
	}

	// Enforce one submission per (scope, survey type, year, half-year): the caller's scope
	// (the user, for Individual Survey; the team, for Post-Workshop Survey) may not resubmit
	// an assessment period it has already completed, and may only target a period that is
	// currently open (see healthcheck.CheckPeriodEligibility). Persisted records are the
	// source of truth -- not submission dates or elapsed time. This is a friendly pre-check;
	// the database's unique index (see the "period submission uniqueness" migration) is the
	// authoritative, race-safe guard against two concurrent requests both passing this check.
	if cmd.Completed {
		submittedPeriods, err := h.repository.FindSubmittedPeriods(context.Background(), healthcheck.LatestSubmissionQuery{
			SurveyType: surveyType,
			TeamID:     cmd.TeamID,
			UserID:     cmd.UserID,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to check submission eligibility: %w", err)
		}
		now := time.Now()
		if parsed, ok := parseSubmissionDate(cmd.Date); ok {
			now = parsed
		}
		switch reason := healthcheck.CheckPeriodEligibility(cmd.AssessmentPeriod, now, submittedPeriods); reason {
		case healthcheck.PeriodEligible, healthcheck.PeriodReasonInvalidFormat:
			// An unrecognized period format is not this check's concern -- it is rejected
			// by the HTTP layer's format validation before reaching this command.
		case healthcheck.PeriodReasonDuplicate:
			return nil, healthcheck.NewSubmissionCooldownError(surveyType, cmd.AssessmentPeriod, submittedPeriods)
		default: // PeriodReasonFuture or PeriodReasonPast
			return nil, healthcheck.NewPeriodNotOpenError(surveyType, cmd.AssessmentPeriod, reason)
		}
	}

	// Convert command to domain model
	session := &healthcheck.HealthCheckSession{
		ID:               cmd.ID,
		TeamID:           cmd.TeamID,
		UserID:           cmd.UserID,
		Date:             cmd.Date,
		AssessmentPeriod: cmd.AssessmentPeriod,
		SurveyType:       surveyType,
		Responses:        make([]healthcheck.HealthCheckResponse, len(cmd.Responses)),
		Completed:        cmd.Completed,
	}

	for i, resp := range cmd.Responses {
		session.Responses[i] = healthcheck.HealthCheckResponse{
			DimensionID: resp.DimensionID,
			Score:       resp.Score,
			Trend:       resp.Trend,
			Comment:     resp.Comment,
		}
	}

	// Save to repository
	if err := h.repository.Save(context.Background(), session); err != nil {
		return nil, fmt.Errorf("failed to save session: %w", err)
	}

	return session, nil
}

// validate ensures the command is valid
func (h *SubmitHealthCheckHandler) validate(cmd SubmitHealthCheckCommand) error {
	if cmd.TeamID == "" {
		return fmt.Errorf("teamId is required")
	}

	if cmd.UserID == "" {
		return fmt.Errorf("userId is required")
	}

	if cmd.Date == "" {
		return fmt.Errorf("date is required")
	}

	if cmd.SurveyType != "" && cmd.SurveyType != healthcheck.SurveyTypeIndividual && cmd.SurveyType != healthcheck.SurveyTypePostWorkshop {
		return fmt.Errorf("surveyType must be 'individual' or 'post_workshop'")
	}

	if len(cmd.Responses) == 0 {
		return fmt.Errorf("responses cannot be empty")
	}

	// Validate each response
	for i, resp := range cmd.Responses {
		if resp.DimensionID == "" {
			return fmt.Errorf("response %d: dimensionId is required", i)
		}

		if resp.Score < 1 || resp.Score > 3 {
			return fmt.Errorf("response %d: score must be between 1 and 3", i)
		}

		if resp.Trend != "improving" && resp.Trend != "stable" && resp.Trend != "declining" {
			return fmt.Errorf("response %d: trend must be 'improving', 'stable', or 'declining'", i)
		}
	}

	return nil
}
