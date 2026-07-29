// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

package dto

// TeamDashboardHealthSummary represents radar chart data (avg score per dimension)
type TeamDashboardHealthSummary struct {
	TeamID           string             `json:"teamId"`
	TeamName         string             `json:"teamName"`
	AssessmentPeriod string             `json:"assessmentPeriod,omitempty"`
	Dimensions       []DimensionSummary `json:"dimensions"`
	OverallHealth    float64            `json:"overallHealth"`
	SubmissionCount  int                `json:"submissionCount"`
}

// ResponseDistribution represents score distribution per dimension (for bar chart)
type ResponseDistribution struct {
	TeamID       string                  `json:"teamId"`
	Distribution []DimensionDistribution `json:"distribution"`
}

// DimensionDistribution represents red/yellow/green counts for a dimension
type DimensionDistribution struct {
	DimensionID string `json:"dimensionId"`
	Red         int    `json:"red"`    // score = 1
	Yellow      int    `json:"yellow"` // score = 2
	Green       int    `json:"green"`  // score = 3
}

// IndividualResponses represents individual team member responses
type IndividualResponses struct {
	TeamID    string                   `json:"teamId"`
	Responses []IndividualUserResponse `json:"responses"`
}

// IndividualUserResponse represents one user's full health check session
type IndividualUserResponse struct {
	SessionID  string                    `json:"sessionId"`
	UserID     string                    `json:"userId"`
	UserName   string                    `json:"userName"`
	Date       string                    `json:"date"`
	SurveyType string                    `json:"surveyType"`
	Dimensions []IndividualDimensionResp `json:"dimensions"`
}

// IndividualDimensionResp represents a user's response for one dimension
type IndividualDimensionResp struct {
	DimensionID string `json:"dimensionId"`
	Score       int    `json:"score"`
	Trend       string `json:"trend"`
	Comment     string `json:"comment,omitempty"`
}

// TrendData represents trend data across assessment periods
type TrendData struct {
	TeamID     string           `json:"teamId"`
	Periods    []string         `json:"periods"`
	Dimensions []DimensionTrend `json:"dimensions"`
}

// DimensionTrend represents trend scores for a dimension across periods
type DimensionTrend struct {
	DimensionID string    `json:"dimensionId"`
	Scores      []float64 `json:"scores"` // matches periods array order
}

// TeamInfoResponse represents detailed team information
type TeamInfoResponse struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Cadence      string       `json:"cadence"`
	Members      []TeamMember `json:"members"`
	TeamLeadID   string       `json:"teamLeadId,omitempty"`
	TeamLeadName string       `json:"teamLeadName,omitempty"`
}

// TeamMember represents a member of a team
type TeamMember struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	FullName string `json:"fullName"`
}

// TeamListResponse represents a list of teams
type TeamListResponse struct {
	Teams []TeamSummary `json:"teams"`
	Total int           `json:"total"`
}

// TeamSummary represents a summary of a team for list views
type TeamSummary struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Cadence      string `json:"cadence"`
	MemberCount  int    `json:"memberCount"`
	TeamLeadID   string `json:"teamLeadId,omitempty"`
	TeamLeadName string `json:"teamLeadName,omitempty"`
}
