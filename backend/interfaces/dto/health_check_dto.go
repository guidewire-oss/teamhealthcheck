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

// SaveDraftRequest represents the request payload for autosaving an in-progress survey draft.
// Responses may be partial (score 0 = unanswered), since the survey is not yet complete. Writes
// always apply (last write wins by arrival order at the database) -- only one user is ever
// editing their own draft, so there is nothing to reconcile a conflict against. ClientUpdatedAt
// is optional display metadata only and never affects whether the save is applied.
type SaveDraftRequest struct {
	TeamID           string                 `json:"teamId" binding:"required"`
	UserID           string                 `json:"userId" binding:"required"`
	SurveyType       string                 `json:"surveyType,omitempty"`
	AssessmentPeriod string                 `json:"assessmentPeriod" binding:"required"`
	CurrentDimension int                    `json:"currentDimension"`
	Responses        []DraftResponseRequest `json:"responses" binding:"dive"`
	ClientUpdatedAt  int64                  `json:"clientUpdatedAt,omitempty"`
} //@name SaveDraftRequest

// DraftResponseRequest represents a single, possibly-incomplete dimension response within a draft
type DraftResponseRequest struct {
	DimensionID string `json:"dimensionId" binding:"required"`
	Score       int    `json:"score" binding:"min=0,max=3"`
	Trend       string `json:"trend,omitempty" binding:"omitempty,oneof=improving stable declining"`
	Comment     string `json:"comment,omitempty"`
} //@name DraftResponseRequest

// DraftResponse represents an in-progress survey draft returned to the client
type DraftResponse struct {
	ID               string                        `json:"id"`
	TeamID           string                        `json:"teamId"`
	UserID           string                        `json:"userId"`
	SurveyType       string                        `json:"surveyType"`
	AssessmentPeriod string                        `json:"assessmentPeriod"`
	CurrentDimension int                           `json:"currentDimension"`
	Responses        []HealthCheckResponseResponse `json:"responses"`
	ClientUpdatedAt  int64                         `json:"clientUpdatedAt,omitempty"`
	UpdatedAt        string                        `json:"updatedAt,omitempty"`
} //@name DraftResponse

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
}
