package dto

// SubmitHealthCheckRequest represents the request payload for submitting a health check
type SubmitHealthCheckRequest struct {
	ID               string                       `json:"id,omitempty"`
	TeamID           string                       `json:"teamId" binding:"required"`
	UserID           string                       `json:"userId" binding:"required"`
	Date             string                       `json:"date" binding:"required"`
	AssessmentPeriod string                       `json:"assessmentPeriod,omitempty"`
	SurveyType       string                       `json:"surveyType,omitempty"`
	Responses        []HealthCheckResponseRequest `json:"responses" binding:"required,min=1,dive"`
	Completed        bool                         `json:"completed"`
}

// HealthCheckResponseRequest represents a single dimension response
type HealthCheckResponseRequest struct {
	DimensionID string `json:"dimensionId" binding:"required"`
	Score       int    `json:"score" binding:"required,min=1,max=3"`
	Trend       string `json:"trend" binding:"required,oneof=improving stable declining"`
	Comment     string `json:"comment,omitempty"`
}

// HealthCheckSessionResponse represents the response after creating/fetching a session
type HealthCheckSessionResponse struct {
	ID               string                        `json:"id"`
	TeamID           string                        `json:"teamId"`
	UserID           string                        `json:"userId"`
	Date             string                        `json:"date"`
	AssessmentPeriod string                        `json:"assessmentPeriod,omitempty"`
	SurveyType       string                        `json:"surveyType,omitempty"`
	Responses        []HealthCheckResponseResponse `json:"responses"`
	Completed        bool                          `json:"completed"`
	CreatedAt        string                        `json:"createdAt,omitempty"`
}

// TeamSubmissionStatusResponse represents the submission status for post-workshop surveys
type TeamSubmissionStatusResponse struct {
	TeamID             string `json:"teamId"`
	AssessmentPeriod   string `json:"assessmentPeriod"`
	TotalMembers       int    `json:"totalMembers"`
	SubmittedMembers   int    `json:"submittedMembers"`
	AllSubmitted       bool   `json:"allSubmitted"`
	PostWorkshopExists bool   `json:"postWorkshopExists"`
}

// HealthCheckResponseResponse represents a dimension response in the response
type HealthCheckResponseResponse struct {
	DimensionID string `json:"dimensionId"`
	Score       int    `json:"score"`
	Trend       string `json:"trend"`
	Comment     string `json:"comment,omitempty"`
}

// HealthDimensionResponse represents a health dimension
type HealthDimensionResponse struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	GoodDescription string  `json:"goodDescription"`
	BadDescription  string  `json:"badDescription"`
	IsActive        bool    `json:"isActive,omitempty"`
	Weight          float64 `json:"weight,omitempty"`
}

// HealthDimensionsResponse is the response containing all dimensions
type HealthDimensionsResponse struct {
	Dimensions []HealthDimensionResponse `json:"dimensions"`
}

// HealthCheckSessionsResponse is the response containing multiple sessions
type HealthCheckSessionsResponse struct {
	Sessions []HealthCheckSessionResponse `json:"sessions"`
	Total    int                          `json:"total,omitempty"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
	Code    string `json:"code,omitempty"`
	// SubmittedPeriod and NextEligibleDate are populated for submission-cooldown (409)
	// errors. SubmittedPeriod is a user-facing label, e.g. "H1 2026" -- always a half-year
	// label, never a quarter. NextEligibleDate is an ISO "YYYY-MM-DD" date, exactly
	// healthcheck.EligibilityCooldownMonths calendar months after the blocking submission,
	// so the frontend can render both without re-parsing the error message.
	SubmittedPeriod  string `json:"submittedPeriod,omitempty"`
	NextEligibleDate string `json:"nextEligibleDate,omitempty"`
}

// SurveyEligibilityResponse represents the result of a pre-submission cooldown check: the
// caller's scope (user for Individual Survey, team for Post-Workshop Survey) is ineligible
// if it has a completed submission of the same survey type within the last
// healthcheck.EligibilityCooldownMonths calendar months.
type SurveyEligibilityResponse struct {
	Eligible bool `json:"eligible"`
	// SubmittedPeriod is the user-facing H1/H2 label of the blocking submission's period,
	// populated when Eligible is false.
	SubmittedPeriod string `json:"submittedPeriod,omitempty"`
	// NextEligibleDate is an ISO "YYYY-MM-DD" date, populated when Eligible is false.
	NextEligibleDate string `json:"nextEligibleDate,omitempty"`
}
