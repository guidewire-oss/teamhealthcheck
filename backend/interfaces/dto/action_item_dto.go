package dto

import (
	"time"
)

// ValidDueDate returns true if s is nil or a valid YYYY-MM-DD calendar date.
func ValidDueDate(s *string) bool {
	if s == nil {
		return true
	}
	_, err := time.Parse("2006-01-02", *s)
	return err == nil
}

// CreateActionItemRequest is the request body for POST /api/v1/teams/:teamId/action-items
type CreateActionItemRequest struct {
	DimensionID      *string `json:"dimensionId"`
	AssignedTo       *string `json:"assignedTo"`
	Title            string  `json:"title" binding:"required,max=500"`
	Description      string  `json:"description"`
	DueDate          *string `json:"dueDate"` // ISO date string YYYY-MM-DD
	AssessmentPeriod *string `json:"assessmentPeriod"`
}

// UpdateActionItemRequest is the request body for PATCH /api/v1/teams/:teamId/action-items/:id
type UpdateActionItemRequest struct {
	DimensionID      *string `json:"dimensionId"`
	AssignedTo       *string `json:"assignedTo"`
	Title            *string `json:"title" binding:"omitempty,max=500"`
	Description      *string `json:"description"`
	Status           *string `json:"status"`
	DueDate          *string `json:"dueDate"`
	AssessmentPeriod *string `json:"assessmentPeriod"`
}

// ActionItemResponse is returned in GET / POST / PATCH responses
type ActionItemResponse struct {
	ID               string  `json:"id"`
	TeamID           string  `json:"teamId"`
	DimensionID      *string `json:"dimensionId"`
	DimensionName    *string `json:"dimensionName"`
	CreatedBy        string  `json:"createdBy"`
	CreatedByName    string  `json:"createdByName"`
	AssignedTo       *string `json:"assignedTo"`
	AssigneeName     *string `json:"assigneeName"`
	Title            string  `json:"title"`
	Description      string  `json:"description"`
	Status           string  `json:"status"`
	DueDate          *string `json:"dueDate"`
	AssessmentPeriod *string `json:"assessmentPeriod"`
	CreatedAt        string  `json:"createdAt"`
	UpdatedAt        string  `json:"updatedAt"`
}

// ActionItemsResponse wraps a list of action items
type ActionItemsResponse struct {
	ActionItems []ActionItemResponse `json:"actionItems"`
}

// TeamActionSummaryResponse is used by the manager endpoint
type TeamActionSummaryResponse struct {
	TeamID    string `json:"teamId"`
	TeamName  string `json:"teamName"`
	OpenCount int    `json:"openCount"`
}

// TeamsActionSummaryResponse wraps per-team action counts
type TeamsActionSummaryResponse struct {
	Teams []TeamActionSummaryResponse `json:"teams"`
}
