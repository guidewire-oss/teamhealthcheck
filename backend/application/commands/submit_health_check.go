// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"context"
	"fmt"
	"time"

	"github.com/agopalakrishnan/teams360/backend/domain/healthcheck"
)

// SubmitHealthCheckCommand represents the command to submit a health check
type SubmitHealthCheckCommand struct {
	ID               string
	TeamID           string
	UserID           string
	Date             string
	AssessmentPeriod string
	SurveyType       string
	Responses        []HealthCheckResponseCommand
	Completed        bool
}

// HealthCheckResponseCommand represents a response in the command
type HealthCheckResponseCommand struct {
	DimensionID string
	Score       int
	Trend       string
	Comment     string
}

// SubmitHealthCheckHandler handles the submit health check command
type SubmitHealthCheckHandler struct {
	repository healthcheck.Repository
}

// NewSubmitHealthCheckHandler creates a new command handler
func NewSubmitHealthCheckHandler(repository healthcheck.Repository) *SubmitHealthCheckHandler {
	return &SubmitHealthCheckHandler{
		repository: repository,
	}
}

// Handle executes the command
func (h *SubmitHealthCheckHandler) Handle(cmd SubmitHealthCheckCommand) (*healthcheck.HealthCheckSession, error) {
	// Generate ID if not provided
	if cmd.ID == "" {
		cmd.ID = fmt.Sprintf("session-%d", time.Now().UnixNano())
	}

	// Validate command
	if err := h.validate(cmd); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Default survey type
	surveyType := cmd.SurveyType
	if surveyType == "" {
		surveyType = healthcheck.SurveyTypeIndividual
	}

	// Convert command to domain model
	session := &healthcheck.HealthCheckSession{
		ID:               cmd.ID,
		TeamID:           cmd.TeamID,
		UserID:           cmd.UserID,
		Date:             cmd.Date,
		AssessmentPeriod: cmd.AssessmentPeriod,
		SurveyType:       surveyType,
		Responses:        make([]healthcheck.HealthCheckResponse, len(cmd.Responses)),
		Completed:        cmd.Completed,
	}

	for i, resp := range cmd.Responses {
		session.Responses[i] = healthcheck.HealthCheckResponse{
			DimensionID: resp.DimensionID,
			Score:       resp.Score,
			Trend:       resp.Trend,
			Comment:     resp.Comment,
		}
	}

	// Save to repository
	if err := h.repository.Save(context.Background(), session); err != nil {
		return nil, fmt.Errorf("failed to save session: %w", err)
	}

	return session, nil
}

// validate ensures the command is valid
func (h *SubmitHealthCheckHandler) validate(cmd SubmitHealthCheckCommand) error {
	if cmd.TeamID == "" {
		return fmt.Errorf("teamId is required")
	}

	if cmd.UserID == "" {
		return fmt.Errorf("userId is required")
	}

	if cmd.Date == "" {
		return fmt.Errorf("date is required")
	}

	if cmd.SurveyType != "" && cmd.SurveyType != healthcheck.SurveyTypeIndividual && cmd.SurveyType != healthcheck.SurveyTypePostWorkshop {
		return fmt.Errorf("surveyType must be 'individual' or 'post_workshop'")
	}

	if len(cmd.Responses) == 0 {
		return fmt.Errorf("responses cannot be empty")
	}

	// Validate each response
	for i, resp := range cmd.Responses {
		if resp.DimensionID == "" {
			return fmt.Errorf("response %d: dimensionId is required", i)
		}

		if resp.Score < 1 || resp.Score > 3 {
			return fmt.Errorf("response %d: score must be between 1 and 3", i)
		}

		if resp.Trend != "improving" && resp.Trend != "stable" && resp.Trend != "declining" {
			return fmt.Errorf("response %d: trend must be 'improving', 'stable', or 'declining'", i)
		}
	}

	return nil
}
