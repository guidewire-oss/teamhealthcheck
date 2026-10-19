package queries

import (
	"context"

	"github.com/agopalakrishnan/teams360/backend/domain/healthcheck"
)

// GetDraftQuery represents the query to fetch a user's in-progress survey draft
type GetDraftQuery struct {
	TeamID     string
	UserID     string
	SurveyType string
}

// GetDraftHandler handles the get draft query
type GetDraftHandler struct {
	repository healthcheck.Repository
}

// NewGetDraftHandler creates a new query handler
func NewGetDraftHandler(repository healthcheck.Repository) *GetDraftHandler {
	return &GetDraftHandler{repository: repository}
}

// Handle executes the query. Returns healthcheck.ErrDraftNotFound if no draft exists.
func (h *GetDraftHandler) Handle(ctx context.Context, query GetDraftQuery) (*healthcheck.HealthCheckDraft, error) {
	return h.repository.GetDraft(ctx, query.UserID, query.TeamID, query.SurveyType)
}
