package healthcheck

import (
	"context"
	"fmt"
)

// Survey type constants
const (
	SurveyTypeIndividual   = "individual"
	SurveyTypePostWorkshop = "post_workshop"
)

// HealthCheckResponse represents a single dimension response
type HealthCheckResponse struct {
	DimensionID string `json:"dimensionId"`
	Score       int    `json:"score"` // 1 = red, 2 = yellow, 3 = green
	Trend       string `json:"trend"` // improving, stable, declining
	Comment     string `json:"comment,omitempty"`
}

// HealthCheckSession represents a completed health check
// This is an aggregate root in DDD terms
type HealthCheckSession struct {
	ID               string                `json:"id"`
	TeamID           string                `json:"teamId"`
	UserID           string                `json:"userId"`
	Date             string                `json:"date"`
	AssessmentPeriod string                `json:"assessmentPeriod,omitempty"`
	SurveyType       string                `json:"surveyType,omitempty"`
	Responses        []HealthCheckResponse `json:"responses"`
	Completed        bool                  `json:"completed"`
}

// TeamHealthSummary represents aggregated health data for a team
type TeamHealthSummary struct {
	TeamID             string             `json:"teamId"`
	TeamName           string             `json:"teamName"`
	SubmissionCount    int                `json:"submissionCount"`
	OverallHealth      float64            `json:"overallHealth"`
	Dimensions         []DimensionSummary `json:"dimensions"`
	PostWorkshopStatus string             `json:"postWorkshopStatus,omitempty"`
}

// TeamSubmissionStatus represents the submission status of a team for an assessment period
type TeamSubmissionStatus struct {
	TeamID             string `json:"teamId"`
	AssessmentPeriod   string `json:"assessmentPeriod"`
	TotalMembers       int    `json:"totalMembers"`
	SubmittedMembers   int    `json:"submittedMembers"`
	AllSubmitted       bool   `json:"allSubmitted"`
	PostWorkshopExists bool   `json:"postWorkshopExists"`
}

// LatestSubmissionQuery scopes a lookup of a caller's submitted assessment periods for a
// given survey type:
//   - individual:    scoped to UserID only (a different user's submission never blocks).
//   - post_workshop: scoped to TeamID only (one workshop consensus per team).
type LatestSubmissionQuery struct {
	SurveyType string
	TeamID     string // required when SurveyType == SurveyTypePostWorkshop
	UserID     string // required when SurveyType == SurveyTypeIndividual
}

// SubmissionCooldownError indicates a survey submission was rejected because the caller's
// scope (the user, for Individual Survey; the team, for Post-Workshop Survey) already has a
// completed submission for this exact survey type and assessment period -- one submission
// per (scope, survey type, year, half-year) is allowed. See CheckPeriodEligibility.
type SubmissionCooldownError struct {
	SurveyType          string
	LastSubmittedPeriod string // user-facing H1/H2 label of the duplicated period
	// NextEligiblePeriod is the user-facing H1/H2 label (e.g. "H2 2026") the caller next
	// becomes eligible for, computed dynamically from every period already on record for
	// this scope (see NextEligiblePeriod). The survey experience never exposes day-level
	// dates, so this label is what error messages and API responses surface.
	NextEligiblePeriod string
}

func (e *SubmissionCooldownError) Error() string {
	return fmt.Sprintf(
		"You have already submitted the %s for %s. Your next eligible survey period is %s.",
		SurveyTypeLabel(e.SurveyType), e.LastSubmittedPeriod, e.NextEligiblePeriod,
	)
}

// SurveyTypeLabel renders a survey type constant as a user-facing label.
func SurveyTypeLabel(surveyType string) string {
	if surveyType == SurveyTypePostWorkshop {
		return "Post-Workshop Survey"
	}
	return "Individual Survey"
}

// NewSubmissionCooldownError builds a SubmissionCooldownError for a duplicate submission of
// duplicatedPeriod, given every period already submitted for this scope and survey type
// (submittedPeriods -- the persisted source of truth, not submitted dates).
func NewSubmissionCooldownError(surveyType string, duplicatedPeriod string, submittedPeriods []string) *SubmissionCooldownError {
	return &SubmissionCooldownError{
		SurveyType:          surveyType,
		LastSubmittedPeriod: FormatPeriodForDisplay(duplicatedPeriod),
		NextEligiblePeriod:  NextEligiblePeriod(submittedPeriods),
	}
}

// PeriodNotOpenError indicates a survey submission was rejected because the requested
// assessment period is not currently open for a brand-new submission -- it is either a
// future half-year that has not started yet, or a stale period from an earlier year. Unlike
// SubmissionCooldownError, this is never about a duplicate: see CheckPeriodEligibility.
type PeriodNotOpenError struct {
	SurveyType string
	Period     string // user-facing H1/H2 label of the rejected period
	// Reason is either PeriodReasonFuture or PeriodReasonPast.
	Reason PeriodEligibilityReason
}

func (e *PeriodNotOpenError) Error() string {
	if e.Reason == PeriodReasonPast {
		return fmt.Sprintf("%s for %s is no longer open for new submissions.", SurveyTypeLabel(e.SurveyType), e.Period)
	}
	return fmt.Sprintf("%s for %s is not yet available.", SurveyTypeLabel(e.SurveyType), e.Period)
}

// NewPeriodNotOpenError builds a PeriodNotOpenError for period (rendered as a safe H1/H2
// label, never a quarter number).
func NewPeriodNotOpenError(surveyType string, period string, reason PeriodEligibilityReason) *PeriodNotOpenError {
	return &PeriodNotOpenError{
		SurveyType: surveyType,
		Period:     FormatPeriodForDisplay(period),
		Reason:     reason,
	}
}

// DimensionSummary represents aggregated dimension health
type DimensionSummary struct {
	DimensionID   string  `json:"dimensionId"`
	AvgScore      float64 `json:"avgScore"`
	ResponseCount int     `json:"responseCount"`
}

// Repository defines the interface for health check data access
type Repository interface {
	FindByID(ctx context.Context, id string) (*HealthCheckSession, error)
	FindByTeamID(ctx context.Context, teamID string) ([]*HealthCheckSession, error)
	FindByUserID(ctx context.Context, userID string) ([]*HealthCheckSession, error)
	FindByAssessmentPeriod(ctx context.Context, period string) ([]*HealthCheckSession, error)
	Save(ctx context.Context, session *HealthCheckSession) error
	Delete(ctx context.Context, id string) error

	// Advanced queries for manager dashboard
	FindTeamHealthByManager(ctx context.Context, managerID string, assessmentPeriod string) ([]TeamHealthSummary, error)
	FindAggregatedDimensionsByManager(ctx context.Context, managerID string, assessmentPeriod string) ([]DimensionSummary, error)

	// Team submission status for post-workshop survey
	GetTeamSubmissionStatus(ctx context.Context, teamID string, assessmentPeriod string) (*TeamSubmissionStatus, error)

	// FindDistinctAssessmentPeriods returns all unique assessment periods from submitted sessions
	FindDistinctAssessmentPeriods(ctx context.Context) ([]string, error)

	// FindSubmittedPeriods returns the assessment-period labels of every completed submission
	// matching the given survey type and scope (TeamID for post_workshop, UserID for
	// individual). This is the persisted source of truth for period-level eligibility and
	// duplicate checks -- see CheckPeriodEligibility.
	FindSubmittedPeriods(ctx context.Context, query LatestSubmissionQuery) ([]string, error)
}
