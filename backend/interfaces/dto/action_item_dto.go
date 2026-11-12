package dto

import (
	"encoding/json"
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

// ValidActionItemStatuses enumerates every status an action item can hold.
var ValidActionItemStatuses = map[string]bool{
	"open":        true,
	"on_hold":     true,
	"in_progress": true,
	"done":        true,
}

// AllowedActionItemTransitions maps a current status to the set of statuses
// it may transition into. Transitions not present here are rejected.
var AllowedActionItemTransitions = map[string]map[string]bool{
	"open":        {"in_progress": true},
	"in_progress": {"on_hold": true, "done": true},
	"on_hold":     {"in_progress": true},
	"done":        {},
}

// CreateActionItemRequest is the request body for POST /api/v1/teams/:teamId/action-items
type CreateActionItemRequest struct {
	DimensionID      *string `json:"dimensionId"`
	AssignedTo       *string `json:"assignedTo"`
	Title            string  `json:"title" binding:"required,max=500"`
	Description      string  `json:"description"`
	DueDate          *string `json:"dueDate"` // ISO date string YYYY-MM-DD
	AssessmentPeriod *string `json:"assessmentPeriod"`
} //@name CreateActionItemRequest

// ClearableString distinguishes the three states a PATCH field can be in:
// absent (leave the stored value alone), JSON null (clear it), or a string
// (set it). A plain *string cannot tell absent from null.
type ClearableString struct {
	Present bool    // the key appeared in the request body
	Value   *string // nil when the key was null (or absent)
}

// UnmarshalJSON implements json.Unmarshaler. encoding/json calls it for
// an explicit null too, which is how null is told apart from a missing key.
func (c *ClearableString) UnmarshalJSON(data []byte) error {
	c.Present = true
	if string(data) == "null" {
		c.Value = nil
		return nil
	}
	var v string
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	c.Value = &v
	return nil
}

// UpdateActionItemRequest is the request body for PATCH /api/v1/teams/:teamId/action-items/:id.
// DimensionID, AssignedTo and DueDate are clearable: send null to remove the
// stored value, omit the key to leave it unchanged.
type UpdateActionItemRequest struct {
	DimensionID      ClearableString `json:"dimensionId" swaggertype:"string" extensions:"x-nullable"`
	AssignedTo       ClearableString `json:"assignedTo" swaggertype:"string" extensions:"x-nullable"`
	Title            *string         `json:"title" binding:"omitempty,max=500"`
	Description      *string         `json:"description"`
	Status           *string         `json:"status"`
	DueDate          ClearableString `json:"dueDate" swaggertype:"string" extensions:"x-nullable"`
	AssessmentPeriod *string         `json:"assessmentPeriod"`
} //@name UpdateActionItemRequest

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
} //@name ActionItemResponse

// DirectManagerResponse is returned by GET /api/v1/teams/:teamId/action-items/direct-manager
type DirectManagerResponse struct {
	ID   *string `json:"id"`
	Name *string `json:"name"`
} //@name DirectManagerResponse

// ActionItemsResponse wraps a list of action items
type ActionItemsResponse struct {
	ActionItems []ActionItemResponse `json:"actionItems"`
} //@name ActionItemsResponse

// TeamActionSummaryResponse is used by the manager endpoint
type TeamActionSummaryResponse struct {
	TeamID    string `json:"teamId"`
	TeamName  string `json:"teamName"`
	OpenCount int    `json:"openCount"`
	// Per-status counts, so clients don't have to download every card to tally them.
	StatusCounts ActionStatusCountsResponse `json:"statusCounts"`
} //@name TeamActionSummaryResponse

// ActionStatusCountsResponse holds the number of action items in each status.
type ActionStatusCountsResponse struct {
	Open       int `json:"open"`
	OnHold     int `json:"onHold"`
	InProgress int `json:"inProgress"`
	Completed  int `json:"completed"`
} //@name ActionStatusCountsResponse

// TeamsActionSummaryResponse wraps per-team action counts
type TeamsActionSummaryResponse struct {
	Teams []TeamActionSummaryResponse `json:"teams"`
} //@name TeamsActionSummaryResponse
