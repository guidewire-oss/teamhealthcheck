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
	Error string `json:"error"`
	// Message is a human-readable description; for a rejected submission it is exactly the
	// message shown to the user (see healthcheck.SubmissionCooldownError.Error() /
	// healthcheck.PeriodNotOpenError.Error()).
	Message string `json:"message,omitempty"`
	// Code is a machine-readable reason, one of healthcheck.PeriodEligibilityReason's values
	// ("duplicate", "future_period", "past_period") when the rejection is period-related.
	Code string `json:"code,omitempty"`
	// SubmittedPeriod and NextEligiblePeriod are populated for a duplicate-submission (409)
	// error. Both are user-facing H1/H2 labels, e.g. "H1 2026" -- never a quarter and never
	// a day-level date. The survey experience never exposes exact submission or eligibility
	// dates to the client, only the half-year period they fall in.
	SubmittedPeriod    string `json:"submittedPeriod,omitempty"`
	NextEligiblePeriod string `json:"nextEligiblePeriod,omitempty"`
}

// SurveyEligibilityResponse represents the result of a pre-submission eligibility check: the
// caller's scope (user for Individual Survey, team for Post-Workshop Survey) is ineligible to
// submit the requested assessment period right now, either because it already has a
// completed submission for that exact period ("duplicate"), or because the period is not
// currently open ("future_period" / "past_period") -- see healthcheck.CheckPeriodEligibility.
type SurveyEligibilityResponse struct {
	Eligible bool `json:"eligible"`
	// Reason is one of healthcheck.PeriodEligibilityReason's values, populated when Eligible
	// is false.
	Reason string `json:"reason,omitempty"`
	// SubmittedPeriod is the user-facing H1/H2 label of the already-submitted period,
	// populated only when Reason is "duplicate".
	SubmittedPeriod string `json:"submittedPeriod,omitempty"`
	// NextEligiblePeriod is the user-facing H1/H2 label (e.g. "H2 2026") the caller next
	// becomes eligible for, populated only when Reason is "duplicate". Never a day-level
	// date.
	NextEligiblePeriod string `json:"nextEligiblePeriod,omitempty"`
}
