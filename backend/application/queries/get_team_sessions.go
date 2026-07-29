// SPDX-License-Identifier: Apache-2.0

package queries

import (
	"context"

	"github.com/agopalakrishnan/teams360/backend/domain/healthcheck"
)

// GetTeamSessionsQuery represents the query to get sessions for a team
type GetTeamSessionsQuery struct {
	TeamID           string
	AssessmentPeriod string
	Limit            int
	Offset           int
}

// GetTeamSessionsHandler handles the query
type GetTeamSessionsHandler struct {
	repository healthcheck.Repository
}

// NewGetTeamSessionsHandler creates a new query handler
func NewGetTeamSessionsHandler(repository healthcheck.Repository) *GetTeamSessionsHandler {
	return &GetTeamSessionsHandler{repository: repository}
}

// Handle executes the query
func (h *GetTeamSessionsHandler) Handle(query GetTeamSessionsQuery) ([]*healthcheck.HealthCheckSession, error) {
	// Get all sessions for the team
	sessions, err := h.repository.FindByTeamID(context.Background(), query.TeamID)
	if err != nil {
		return nil, err
	}

	// Filter by assessment period if specified
	if query.AssessmentPeriod != "" {
		filtered := []*healthcheck.HealthCheckSession{}
		for _, session := range sessions {
			if session.AssessmentPeriod == query.AssessmentPeriod {
				filtered = append(filtered, session)
			}
		}
		sessions = filtered
	}

	// Apply pagination if specified
	if query.Limit > 0 {
		start := query.Offset
		end := query.Offset + query.Limit

		if start >= len(sessions) {
			return []*healthcheck.HealthCheckSession{}, nil
		}

		if end > len(sessions) {
			end = len(sessions)
		}

		sessions = sessions[start:end]
	}

	return sessions, nil
}
