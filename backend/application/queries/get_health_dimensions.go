// SPDX-License-Identifier: Apache-2.0

package queries

import (
	"context"
	"fmt"

	"github.com/agopalakrishnan/teams360/backend/domain/organization"
)

// GetHealthDimensionsQuery represents the query to get all health dimensions
type GetHealthDimensionsQuery struct {
	OnlyActive bool
}

// GetHealthDimensionsHandler handles the query
type GetHealthDimensionsHandler struct {
	repository organization.Repository
}

// NewGetHealthDimensionsHandler creates a new query handler
func NewGetHealthDimensionsHandler(repository organization.Repository) *GetHealthDimensionsHandler {
	return &GetHealthDimensionsHandler{repository: repository}
}

// Handle executes the query
func (h *GetHealthDimensionsHandler) Handle(query GetHealthDimensionsQuery) ([]*organization.HealthDimension, error) {
	dimensions, err := h.repository.FindDimensions(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get dimensions: %w", err)
	}

	// Filter by active if requested
	if query.OnlyActive {
		filtered := []*organization.HealthDimension{}
		for _, dim := range dimensions {
			if dim.IsActive {
				filtered = append(filtered, dim)
			}
		}
		return filtered, nil
	}

	return dimensions, nil
}
