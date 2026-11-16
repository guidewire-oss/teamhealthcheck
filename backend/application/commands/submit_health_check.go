package commands

import (
	"context"
	"fmt"
	"time"

	"github.com/agopalakrishnan/teams360/backend/domain/healthcheck"
)

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
	// now is the clock period eligibility is decided against. It is always the server's
	// clock: the command's Date is stored as the session date but never decides which
	// periods are open.
	now func() time.Time
}

// NewSubmitHealthCheckHandler creates a new command handler that decides eligibility
// against the server's clock.
func NewSubmitHealthCheckHandler(repository healthcheck.Repository) *SubmitHealthCheckHandler {
	return NewSubmitHealthCheckHandlerWithClock(repository, time.Now)
}

// NewSubmitHealthCheckHandlerWithClock creates a command handler that reads "now" from the
// given clock, so tests can pin the current date.
func NewSubmitHealthCheckHandlerWithClock(repository healthcheck.Repository, now func() time.Time) *SubmitHealthCheckHandler {
	return &SubmitHealthCheckHandler{
		repository: repository,
		now:        now,
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

	// Store every supported period in its one canonical form ("2026 Q1" and "2026 - 2nd
	// Half"-style labels fold into "YYYY H1"/"YYYY H2"), so two labels for the same period
	// are the same string and the database's unique index sees them as duplicates.
	if canonical, ok := healthcheck.CanonicalPeriod(cmd.AssessmentPeriod); ok {
		cmd.AssessmentPeriod = canonical
	}

	// Enforce one submission per (scope, survey type, period): the caller's scope (the user,
	// for Individual Survey; the team, for Post-Workshop Survey) may not resubmit an
	// assessment period it has already completed, and may only target a period that is
	// currently open (see healthcheck.CheckPeriodEligibility). Persisted records are the
	// source of truth -- not submission dates or elapsed time -- and "now" is the server's
	// clock, never the client-supplied Date. This is a friendly pre-check; the database's
	// unique index (see the "half-year period uniqueness" migration) is the authoritative,
	// race-safe guard against two concurrent requests both passing this check.
	if cmd.Completed {
		submittedPeriods, err := h.repository.FindSubmittedPeriods(context.Background(), healthcheck.LatestSubmissionQuery{
			SurveyType: surveyType,
			TeamID:     cmd.TeamID,
			UserID:     cmd.UserID,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to check submission eligibility: %w", err)
		}
		switch reason := healthcheck.CheckPeriodEligibility(cmd.AssessmentPeriod, h.now(), submittedPeriods); reason {
		case healthcheck.PeriodEligible:
		case healthcheck.PeriodReasonDuplicate:
			return nil, healthcheck.NewDuplicateSubmissionError(surveyType, cmd.AssessmentPeriod, submittedPeriods)
		default: // future, past, or an unrecognized period (already rejected by validate)
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

	if cmd.Completed {
		if _, ok := healthcheck.CanonicalPeriod(cmd.AssessmentPeriod); !ok {
			return fmt.Errorf("assessmentPeriod is required for a completed survey and must be a half-yearly (\"2026 H1\"), monthly (\"2026 Mar\") or yearly (\"2026\") period")
		}
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
