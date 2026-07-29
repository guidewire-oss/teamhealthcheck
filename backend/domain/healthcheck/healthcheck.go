// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

package healthcheck

import "context"

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
}
