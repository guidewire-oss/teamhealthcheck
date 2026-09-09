package v1

import (
	"net/http"
	"strings"

	"github.com/agopalakrishnan/teams360/backend/domain/organization"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/gin-gonic/gin"
)

// HierarchyAdminHandler handles hierarchy-level-related admin HTTP requests
type HierarchyAdminHandler struct {
	orgRepo organization.Repository
}

// NewHierarchyAdminHandler creates a new HierarchyAdminHandler
func NewHierarchyAdminHandler(orgRepo organization.Repository) *HierarchyAdminHandler {
	return &HierarchyAdminHandler{orgRepo: orgRepo}
}

// ListHierarchyLevels handles GET /api/v1/admin/hierarchy-levels
//
// @Summary List hierarchy levels
// @Description Returns every organizational hierarchy level (e.g. VP, Director, Manager, Team Lead, Team Member), ordered from highest to lowest scope by position. Access is restricted to authenticated users holding the Admin role with level-1 administrative permission. Each entry includes the level's identifier, display name, position, permission flags (view all teams, edit teams, manage users, take survey, view analytics), and creation/update timestamps.
// @Tags Admin Hierarchy
// @Produce json
// @Success 200 {object} dto.HierarchyLevelsResponse
// @Failure 500 {object} dto.ErrorResponse "Failed to query hierarchy levels"
// @Security BearerAuth
// @Router /admin/hierarchy-levels [get]
func (h *HierarchyAdminHandler) ListHierarchyLevels(c *gin.Context) {
	hierarchyLevels, err := h.orgRepo.FindHierarchyLevels(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to query hierarchy levels",
			Message: err.Error(),
		})
		return
	}

	// Convert domain models to DTOs
	levels := make([]dto.HierarchyLevelDTO, len(hierarchyLevels))
	for i, level := range hierarchyLevels {
		levels[i] = dto.HierarchyLevelDTO{
			ID:       level.ID,
			Name:     level.Name,
			Position: level.Position,
			Permissions: dto.HierarchyPermissionsDTO{
				CanViewAllTeams:  level.Permissions.CanViewAllTeams,
				CanEditTeams:     level.Permissions.CanEditTeams,
				CanManageUsers:   level.Permissions.CanManageUsers,
				CanTakeSurvey:    level.Permissions.CanTakeSurvey,
				CanViewAnalytics: level.Permissions.CanViewAnalytics,
			},
			CreatedAt: level.CreatedAt,
			UpdatedAt: level.UpdatedAt,
		}
	}

	c.JSON(http.StatusOK, dto.HierarchyLevelsResponse{Levels: levels})
}

// CreateHierarchyLevel handles POST /api/v1/admin/hierarchy-levels
//
// @Summary Create a hierarchy level
// @Description Creates a new organizational hierarchy level and appends it at the end of the position order, below every existing level. The level ID is auto-generated from the name (lowercased, spaces converted to hyphens) when the caller does not supply one. Permission flags for viewing teams, editing teams, managing users, taking the survey, and viewing analytics are set from the request body. Access is restricted to authenticated users holding the Admin role with level-1 administrative permission.
// @Tags Admin Hierarchy
// @Accept json
// @Produce json
// @Param body body dto.CreateHierarchyLevelRequest true "Hierarchy level"
// @Success 201 {object} dto.HierarchyLevelDTO
// @Failure 400 {object} dto.ErrorResponse "Invalid request body"
// @Failure 500 {object} dto.ErrorResponse "Failed to determine position or create hierarchy level"
// @Security BearerAuth
// @Router /admin/hierarchy-levels [post]
func (h *HierarchyAdminHandler) CreateHierarchyLevel(c *gin.Context) {
	var req dto.CreateHierarchyLevelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid request body", Message: err.Error()})
		return
	}

	// Get max position and add 1
	maxPosition, err := h.orgRepo.GetMaxHierarchyPosition(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to determine position"})
		return
	}
	newPosition := maxPosition + 1

	// Auto-generate ID from name if not provided
	levelID := req.ID
	if levelID == "" {
		levelID = generateIDFromName(req.Name)
	}

	// Create hierarchy level domain model
	level := &organization.HierarchyLevel{
		ID:       levelID,
		Name:     req.Name,
		Position: newPosition,
		Permissions: organization.Permissions{
			CanViewAllTeams:  req.Permissions.CanViewAllTeams,
			CanEditTeams:     req.Permissions.CanEditTeams,
			CanManageUsers:   req.Permissions.CanManageUsers,
			CanTakeSurvey:    req.Permissions.CanTakeSurvey,
			CanViewAnalytics: req.Permissions.CanViewAnalytics,
		},
	}

	// Save using repository
	if err := h.orgRepo.SaveHierarchyLevel(c.Request.Context(), level); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to create hierarchy level",
			Message: err.Error(),
		})
		return
	}

	// Convert to DTO and return
	responseDTO := dto.HierarchyLevelDTO{
		ID:       level.ID,
		Name:     level.Name,
		Position: level.Position,
		Permissions: dto.HierarchyPermissionsDTO{
			CanViewAllTeams:  level.Permissions.CanViewAllTeams,
			CanEditTeams:     level.Permissions.CanEditTeams,
			CanManageUsers:   level.Permissions.CanManageUsers,
			CanTakeSurvey:    level.Permissions.CanTakeSurvey,
			CanViewAnalytics: level.Permissions.CanViewAnalytics,
		},
		CreatedAt: level.CreatedAt,
		UpdatedAt: level.UpdatedAt,
	}

	c.JSON(http.StatusCreated, responseDTO)
}

// UpdateHierarchyLevel handles PUT /api/v1/admin/hierarchy-levels/:id
//
// @Summary Update a hierarchy level
// @Description Updates the name and/or permission flags of the hierarchy level identified by the path ID. Only the fields present in the request body are changed; the level's position is left untouched. Returns a 404 if no level exists with that ID. Access is restricted to authenticated users holding the Admin role with level-1 administrative permission.
// @Tags Admin Hierarchy
// @Accept json
// @Produce json
// @Param id path string true "Hierarchy level ID"
// @Param body body dto.UpdateHierarchyLevelRequest true "Fields to update"
// @Success 200 {object} dto.HierarchyLevelDTO
// @Failure 400 {object} dto.ErrorResponse "Invalid request body"
// @Failure 404 {object} dto.ErrorResponse "Hierarchy level not found"
// @Failure 500 {object} dto.ErrorResponse "Failed to update hierarchy level"
// @Security BearerAuth
// @Router /admin/hierarchy-levels/{id} [put]
func (h *HierarchyAdminHandler) UpdateHierarchyLevel(c *gin.Context) {
	id := c.Param("id")
	var req dto.UpdateHierarchyLevelRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid request body", Message: err.Error()})
		return
	}

	// Check if hierarchy level exists
	existingLevel, err := h.orgRepo.FindHierarchyLevelByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Hierarchy level not found"})
		return
	}

	// Update fields if provided
	if req.Name != "" {
		existingLevel.Name = req.Name
	}
	if req.Permissions != nil {
		existingLevel.Permissions.CanViewAllTeams = req.Permissions.CanViewAllTeams
		existingLevel.Permissions.CanEditTeams = req.Permissions.CanEditTeams
		existingLevel.Permissions.CanManageUsers = req.Permissions.CanManageUsers
		existingLevel.Permissions.CanTakeSurvey = req.Permissions.CanTakeSurvey
		existingLevel.Permissions.CanViewAnalytics = req.Permissions.CanViewAnalytics
	}

	// Update using repository
	if err := h.orgRepo.UpdateHierarchyLevel(c.Request.Context(), existingLevel); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to update hierarchy level",
			Message: err.Error(),
		})
		return
	}

	// Convert to DTO and return
	responseDTO := dto.HierarchyLevelDTO{
		ID:       existingLevel.ID,
		Name:     existingLevel.Name,
		Position: existingLevel.Position,
		Permissions: dto.HierarchyPermissionsDTO{
			CanViewAllTeams:  existingLevel.Permissions.CanViewAllTeams,
			CanEditTeams:     existingLevel.Permissions.CanEditTeams,
			CanManageUsers:   existingLevel.Permissions.CanManageUsers,
			CanTakeSurvey:    existingLevel.Permissions.CanTakeSurvey,
			CanViewAnalytics: existingLevel.Permissions.CanViewAnalytics,
		},
		CreatedAt: existingLevel.CreatedAt,
		UpdatedAt: existingLevel.UpdatedAt,
	}

	c.JSON(http.StatusOK, responseDTO)
}

// UpdateHierarchyPosition handles PUT /api/v1/admin/hierarchy-levels/:id/position
//
// @Summary Reorder a hierarchy level
// @Description Moves the hierarchy level identified by the path ID to the given position, shifting every level between its old and new position up or down by one so the ordering stays contiguous. The reorder runs inside a single transaction that rolls back if any step fails. Access is restricted to authenticated users holding the Admin role with level-1 administrative permission.
// @Tags Admin Hierarchy
// @Accept json
// @Produce json
// @Param id path string true "Hierarchy level ID"
// @Param body body dto.UpdateHierarchyPositionRequest true "New position"
// @Success 200 {object} dto.MessageResponse
// @Failure 400 {object} dto.ErrorResponse "Invalid request body"
// @Failure 404 {object} dto.ErrorResponse "Hierarchy level not found"
// @Failure 500 {object} dto.ErrorResponse "Failed to start transaction, reorder levels, update position, or commit"
// @Security BearerAuth
// @Router /admin/hierarchy-levels/{id}/position [put]
func (h *HierarchyAdminHandler) UpdateHierarchyPosition(c *gin.Context) {
	id := c.Param("id")
	var req dto.UpdateHierarchyPositionRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid request body", Message: err.Error()})
		return
	}

	// Get current hierarchy level
	level, err := h.orgRepo.FindHierarchyLevelByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Hierarchy level not found"})
		return
	}

	currentPosition := level.Position

	// Start transaction
	tx, err := h.orgRepo.BeginTx(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to start transaction"})
		return
	}
	defer h.orgRepo.RollbackTx(tx)

	// Shift positions
	if req.NewPosition < currentPosition {
		// Moving up: shift others down
		if err := h.orgRepo.ShiftHierarchyPositions(c.Request.Context(), tx, req.NewPosition, currentPosition, 1); err != nil {
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to reorder levels"})
			return
		}
	} else if req.NewPosition > currentPosition {
		// Moving down: shift others up
		if err := h.orgRepo.ShiftHierarchyPositions(c.Request.Context(), tx, currentPosition, req.NewPosition, -1); err != nil {
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to reorder levels"})
			return
		}
	}

	// Update the target level position
	if err := h.orgRepo.UpdateHierarchyPosition(c.Request.Context(), tx, id, req.NewPosition); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to update position"})
		return
	}

	if err := h.orgRepo.CommitTx(tx); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to commit transaction"})
		return
	}

	dto.RespondMessage(c, http.StatusOK, "Position updated successfully")
}

// DeleteHierarchyLevel handles DELETE /api/v1/admin/hierarchy-levels/:id
//
// @Summary Delete a hierarchy level
// @Description Deletes the hierarchy level identified by the path ID. The delete is rejected with a 409 if any users are still assigned to that level; those users must be reassigned to a different level first. Access is restricted to authenticated users holding the Admin role with level-1 administrative permission.
// @Tags Admin Hierarchy
// @Produce json
// @Param id path string true "Hierarchy level ID"
// @Success 200 {object} dto.MessageResponse
// @Failure 409 {object} dto.ErrorResponse "Users are assigned to this level"
// @Failure 500 {object} dto.ErrorResponse "Database error or failed to delete hierarchy level"
// @Security BearerAuth
// @Router /admin/hierarchy-levels/{id} [delete]
func (h *HierarchyAdminHandler) DeleteHierarchyLevel(c *gin.Context) {
	id := c.Param("id")

	// Check if any users are using this level
	userCount, err := h.orgRepo.CountUsersAtLevel(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Database error"})
		return
	}

	if userCount > 0 {
		c.JSON(http.StatusConflict, dto.ErrorResponse{
			Error:   "Cannot delete hierarchy level",
			Message: "Users are assigned to this level. Reassign them first.",
		})
		return
	}

	// Delete using repository
	if err := h.orgRepo.DeleteHierarchyLevel(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to delete hierarchy level",
			Message: err.Error(),
		})
		return
	}

	dto.RespondMessage(c, http.StatusOK, "Hierarchy level deleted successfully")
}

// generateIDFromName creates a URL-safe ID from a name
// e.g., "Test Level 123" -> "test-level-123"
func generateIDFromName(name string) string {
	// Convert to lowercase and replace spaces with hyphens
	id := strings.ToLower(name)
	id = strings.ReplaceAll(id, " ", "-")
	// Remove any characters that aren't alphanumeric or hyphens
	var result strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	return result.String()
}
