package v1

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"github.com/agopalakrishnan/teams360/backend/domain/organization"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/text/unicode/norm"
)

// maxHierarchyLevelIDAttempts bounds how many "-2", "-3", ... suffixes we'll
// try before falling back to a random suffix, so a burst of identically
// named levels can never cause an infinite loop.
const maxHierarchyLevelIDAttempts = 50

// HierarchyAdminHandler handles hierarchy-level-related admin HTTP requests
type HierarchyAdminHandler struct {
	orgRepo organization.Repository
}

// NewHierarchyAdminHandler creates a new HierarchyAdminHandler
func NewHierarchyAdminHandler(orgRepo organization.Repository) *HierarchyAdminHandler {
	return &HierarchyAdminHandler{orgRepo: orgRepo}
}

// ListHierarchyLevels handles GET /api/v1/admin/hierarchy-levels
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
		levels[i] = toHierarchyLevelDTO(level)
	}

	c.JSON(http.StatusOK, dto.HierarchyLevelsResponse{Levels: levels})
}

// CreateHierarchyLevel handles POST /api/v1/admin/hierarchy-levels
func (h *HierarchyAdminHandler) CreateHierarchyLevel(c *gin.Context) {
	var req dto.CreateHierarchyLevelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid request body", Message: err.Error()})
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "Invalid request body",
			Message: "Name is required",
		})
		return
	}

	// Get max position and add 1
	maxPosition, err := h.orgRepo.GetMaxHierarchyPosition(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to determine position"})
		return
	}
	newPosition := maxPosition + 1

	// Use the caller-supplied ID verbatim if given; otherwise derive a safe,
	// unique, non-empty ID from the name.
	levelID := strings.TrimSpace(req.ID)
	if levelID == "" {
		generatedID, err := h.generateUniqueHierarchyLevelID(c.Request.Context(), req.Name)
		if err != nil {
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
				Error:   "Failed to generate hierarchy level ID",
				Message: err.Error(),
			})
			return
		}
		levelID = generatedID
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
		if errors.Is(err, organization.ErrDuplicateHierarchyLevelName) {
			c.JSON(http.StatusConflict, dto.ErrorResponse{
				Error:   "Cannot create hierarchy level",
				Message: fmt.Sprintf("A hierarchy level named %q already exists.", req.Name),
			})
			return
		}
		if errors.Is(err, organization.ErrDuplicateHierarchyLevelID) {
			c.JSON(http.StatusConflict, dto.ErrorResponse{
				Error:   "Cannot create hierarchy level",
				Message: fmt.Sprintf("A hierarchy level with id %q already exists. Provide a different id.", levelID),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to create hierarchy level",
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, toHierarchyLevelDTO(level))
}

// UpdateHierarchyLevel handles PUT /api/v1/admin/hierarchy-levels/:id
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
	if name := strings.TrimSpace(req.Name); name != "" {
		existingLevel.Name = name
	}
	if req.Permissions != nil {
		existingLevel.Permissions.CanViewAllTeams = req.Permissions.CanViewAllTeams
		existingLevel.Permissions.CanEditTeams = req.Permissions.CanEditTeams
		existingLevel.Permissions.CanManageUsers = req.Permissions.CanManageUsers
		existingLevel.Permissions.CanTakeSurvey = req.Permissions.CanTakeSurvey
		existingLevel.Permissions.CanViewAnalytics = req.Permissions.CanViewAnalytics
	}

	// Update using repository. The level's ID is never changed by this
	// endpoint, so renaming a level keeps it stable for existing users/teams
	// and JWT claims that reference it.
	if err := h.orgRepo.UpdateHierarchyLevel(c.Request.Context(), existingLevel); err != nil {
		if errors.Is(err, organization.ErrDuplicateHierarchyLevelName) {
			c.JSON(http.StatusConflict, dto.ErrorResponse{
				Error:   "Cannot update hierarchy level",
				Message: fmt.Sprintf("A hierarchy level named %q already exists.", existingLevel.Name),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to update hierarchy level",
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, toHierarchyLevelDTO(existingLevel))
}

// DeleteHierarchyLevel handles DELETE /api/v1/admin/hierarchy-levels/:id
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

	// Delete and compact remaining positions to 1..N atomically, so the list
	// never shows gaps (e.g. 1, 2, 4) after a deletion.
	tx, err := h.orgRepo.BeginTx(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to start transaction"})
		return
	}
	defer h.orgRepo.RollbackTx(tx)

	if err := h.orgRepo.DeleteHierarchyLevel(c.Request.Context(), tx, id); err != nil {
		if errors.Is(err, organization.ErrHierarchyLevelNotFound) {
			c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Hierarchy level not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to delete hierarchy level",
			Message: err.Error(),
		})
		return
	}

	if err := h.orgRepo.CompactHierarchyPositions(c.Request.Context(), tx); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to renumber hierarchy positions",
			Message: err.Error(),
		})
		return
	}

	if err := h.orgRepo.CommitTx(tx); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to commit transaction"})
		return
	}

	dto.RespondMessage(c, http.StatusOK, "Hierarchy level deleted successfully")
}

// toHierarchyLevelDTO converts a domain hierarchy level to its API DTO.
func toHierarchyLevelDTO(level *organization.HierarchyLevel) dto.HierarchyLevelDTO {
	return dto.HierarchyLevelDTO{
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

// generateUniqueHierarchyLevelID derives a URL-safe, non-empty, unique ID
// from a hierarchy level name.
//
// Unlike a plain ASCII-strip, this transliterates Latin accented characters
// (e.g. "Café" -> "cafe") via Unicode NFKD decomposition before dropping
// combining marks, so common non-ASCII names still produce a readable slug
// instead of collapsing to empty. Names that produce no usable ASCII
// fragment (e.g. names written entirely in a non-Latin script) fall back to
// a generic "level" base rather than an empty ID.
//
// Because two different names can still legitimately collapse to the same
// base slug (e.g. "Team Lead!" and "Team Lead?"), this also guarantees
// uniqueness against existing IDs by trying numeric suffixes, and falls back
// to a short random suffix if an unreasonable number of collisions occur.
func (h *HierarchyAdminHandler) generateUniqueHierarchyLevelID(ctx context.Context, name string) (string, error) {
	base := slugifyHierarchyLevelName(name)
	if base == "" {
		base = "level"
	}

	candidate := base
	for attempt := 0; attempt < maxHierarchyLevelIDAttempts; attempt++ {
		if attempt > 0 {
			candidate = fmt.Sprintf("%s-%d", base, attempt+1)
		}

		_, err := h.orgRepo.FindHierarchyLevelByID(ctx, candidate)
		if err == nil {
			continue // ID taken, try the next suffix
		}
		if errors.Is(err, organization.ErrHierarchyLevelNotFound) {
			return candidate, nil
		}
		return "", err
	}

	// Extremely unlikely: many collisions in a row. Guarantee termination
	// with a short random suffix instead of looping forever.
	return fmt.Sprintf("%s-%s", base, uuid.NewString()[:8]), nil
}

// slugifyHierarchyLevelName converts a display name into a lowercase,
// hyphenated, ASCII-only slug suitable for use as a primary key / URL
// segment. It never panics and may return an empty string when the name
// contains no transliterable ASCII letters or digits (callers must handle
// that case).
func slugifyHierarchyLevelName(name string) string {
	// NFKD decomposes accented letters into a base letter plus combining
	// marks (e.g. "é" -> "e" + U+0301), which we then drop below.
	decomposed := norm.NFKD.String(name)

	var b strings.Builder
	for _, r := range decomposed {
		switch {
		case unicode.Is(unicode.Mn, r):
			// Combining mark left over from decomposition - drop it.
			continue
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(unicode.ToLower(r))
		case unicode.IsSpace(r), r == '_', r == '-':
			b.WriteByte('-')
		}
		// Anything else (punctuation, non-Latin script, emoji, ...) is
		// simply dropped rather than risking a non-URL-safe ID.
	}

	slug := strings.Trim(b.String(), "-")
	// Collapse any run of hyphens produced by adjacent separators/dropped
	// characters (e.g. "Team  Lead!!" -> "team--lead" -> "team-lead").
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	return slug
}
