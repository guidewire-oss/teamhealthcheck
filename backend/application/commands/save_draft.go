package commands

import (
	"context"
	"fmt"

	"github.com/agopalakrishnan/teams360/backend/domain/healthcheck"
	"github.com/agopalakrishnan/teams360/backend/domain/organization"
)

// validTrends are the only trend values a (possibly partial) draft response may carry. An empty
// string is also allowed separately, since a dimension may not have a trend selection yet.
var validTrends = map[string]bool{
	"improving": true,
	"stable":    true,
	"declining": true,
}

// SaveDraftCommand represents the command to upsert an in-progress survey draft.
// Unlike SubmitHealthCheckCommand, responses may be partial (a dimension may have
// no trend, or no score yet), since this is autosaved before the survey is complete.
type SaveDraftCommand struct {
	TeamID           string
	UserID           string
	SurveyType       string
	AssessmentPeriod string
	CurrentDimension int
	Responses        []HealthCheckResponseCommand
	// ClientUpdatedAt is optional display-only metadata (epoch-millis); it never affects
	// ordering or whether the save is applied. Saves always overwrite (last write wins by
	// arrival order) since only one user is ever editing their own draft.
	ClientUpdatedAt int64
}

// SaveDraftHandler handles the save draft command
type SaveDraftHandler struct {
	repository    healthcheck.Repository
	orgRepository organization.Repository
}

// NewSaveDraftHandler creates a new command handler. orgRepository is used to validate
// CurrentDimension against the current set of active survey dimensions.
func NewSaveDraftHandler(repository healthcheck.Repository, orgRepository organization.Repository) *SaveDraftHandler {
	return &SaveDraftHandler{repository: repository, orgRepository: orgRepository}
}

// Handle executes the command
func (h *SaveDraftHandler) Handle(ctx context.Context, cmd SaveDraftCommand) (*healthcheck.HealthCheckDraft, error) {
	dimensionCount, err := h.activeDimensionCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load dimensions: %w", err)
	}

	if err := h.validate(cmd, dimensionCount); err != nil {
		return nil, err
	}

	surveyType := cmd.SurveyType
	if surveyType == "" {
		surveyType = healthcheck.SurveyTypeIndividual
	}

	draft := &healthcheck.HealthCheckDraft{
		TeamID:           cmd.TeamID,
		UserID:           cmd.UserID,
		SurveyType:       surveyType,
		AssessmentPeriod: cmd.AssessmentPeriod,
		CurrentDimension: cmd.CurrentDimension,
		Responses:        make([]healthcheck.HealthCheckResponse, len(cmd.Responses)),
		ClientUpdatedAt:  cmd.ClientUpdatedAt,
	}

	for i, resp := range cmd.Responses {
		draft.Responses[i] = healthcheck.HealthCheckResponse{
			DimensionID: resp.DimensionID,
			Score:       resp.Score,
			Trend:       resp.Trend,
			Comment:     resp.Comment,
		}
	}

	if err := h.repository.SaveDraft(ctx, draft); err != nil {
		return nil, fmt.Errorf("failed to save draft: %w", err)
	}

	return draft, nil
}

// activeDimensionCount returns the number of currently active health dimensions, used to bound
// CurrentDimension. A count of 0 (e.g. dimensions not yet seeded) disables the upper-bound check
// rather than rejecting every save.
func (h *SaveDraftHandler) activeDimensionCount(ctx context.Context) (int, error) {
	dimensions, err := h.orgRepository.FindDimensions(ctx)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, dim := range dimensions {
		if dim.IsActive {
			count++
		}
	}
	return count, nil
}

// validate ensures the command is valid. Score is intentionally not range-checked against a
// non-zero minimum here (unlike SubmitHealthCheckCommand) because a draft may hold partial,
// in-progress answers (score 0 = unanswered yet).
func (h *SaveDraftHandler) validate(cmd SaveDraftCommand, dimensionCount int) error {
	if cmd.TeamID == "" {
		return NewValidationError("teamId is required")
	}

	if cmd.UserID == "" {
		return NewValidationError("userId is required")
	}

	if cmd.AssessmentPeriod == "" {
		return NewValidationError("assessmentPeriod is required")
	}

	if cmd.SurveyType != "" && cmd.SurveyType != healthcheck.SurveyTypeIndividual && cmd.SurveyType != healthcheck.SurveyTypePostWorkshop {
		return NewValidationError("surveyType must be 'individual' or 'post_workshop'")
	}

	if cmd.CurrentDimension < 0 {
		return NewValidationError("currentDimension must not be negative")
	}
	if dimensionCount > 0 && cmd.CurrentDimension >= dimensionCount {
		return NewValidationError("currentDimension must be less than the number of active survey dimensions (%d)", dimensionCount)
	}

	for i, resp := range cmd.Responses {
		if resp.DimensionID == "" {
			return NewValidationError("response %d: dimensionId is required", i)
		}
		if resp.Score < 0 || resp.Score > 3 {
			return NewValidationError("response %d: score must be between 0 and 3", i)
		}
		if resp.Trend != "" && !validTrends[resp.Trend] {
			return NewValidationError("response %d: trend must be 'improving', 'stable', 'declining', or empty", i)
		}
	}

	return nil
}
