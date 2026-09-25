package healthcheck

import (
	"context"
	"fmt"
	"time"
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

// LatestSubmissionQuery looks up the most recent completed submission of a survey type,
// scoped by survey type:
//   - individual:    scoped to UserID only (a different user's submission never blocks).
//   - post_workshop: scoped to TeamID only (one workshop consensus per team).
type LatestSubmissionQuery struct {
	SurveyType string
	TeamID     string // required when SurveyType == SurveyTypePostWorkshop
	UserID     string // required when SurveyType == SurveyTypeIndividual
}

// SubmissionCooldownError indicates a survey submission was rejected because the caller's
// scope (the user, for Individual Survey; the team, for Post-Workshop Survey) already has a
// completed submission of the same survey type, and neither the EligibilityCooldownMonths
// cooldown has elapsed nor has the calendar moved into a new half-year period since that
// submission (see NextEligibleDate).
type SubmissionCooldownError struct {
	SurveyType          string
	LastSubmittedPeriod string // user-facing H1/H2 label of the blocking submission's period
	LastSubmissionDate  time.Time
	NextEligible        time.Time
}

func (e *SubmissionCooldownError) Error() string {
	return fmt.Sprintf(
		"You have already submitted the %s for %s. Your next submission will be available on %s.",
		SurveyTypeLabel(e.SurveyType), e.LastSubmittedPeriod, e.NextEligible.Format("Jan 2, 2006"),
	)
}

// SurveyTypeLabel renders a survey type constant as a user-facing label.
func SurveyTypeLabel(surveyType string) string {
	if surveyType == SurveyTypePostWorkshop {
		return "Post-Workshop Survey"
	}
	return "Individual Survey"
}

// NewSubmissionCooldownError builds a SubmissionCooldownError for the given survey type from
// the blocking prior submission's assessment period (rendered as a safe H1/H2 label, never a
// quarter number) and its actual submission date.
func NewSubmissionCooldownError(surveyType string, lastSubmittedPeriod string, lastSubmissionDate time.Time) *SubmissionCooldownError {
	return &SubmissionCooldownError{
		SurveyType:          surveyType,
		LastSubmittedPeriod: FormatPeriodForDisplay(lastSubmittedPeriod),
		LastSubmissionDate:  lastSubmissionDate,
		NextEligible:        NextEligibleDate(lastSubmissionDate),
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

	// FindLatestSubmission returns the most recent completed submission (by date) matching
	// the given survey type and scope, or nil if none exists. Used to enforce the
	// EligibilityCooldownMonths rolling-window submission restriction.
	FindLatestSubmission(ctx context.Context, query LatestSubmissionQuery) (*HealthCheckSession, error)
}
