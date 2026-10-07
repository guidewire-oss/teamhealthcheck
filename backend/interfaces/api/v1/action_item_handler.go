package v1

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/agopalakrishnan/teams360/backend/interfaces/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ActionItemHandler handles action item CRUD endpoints
type ActionItemHandler struct {
	db *sql.DB
}

// NewActionItemHandler creates a new ActionItemHandler
func NewActionItemHandler(db *sql.DB) *ActionItemHandler {
	return &ActionItemHandler{db: db}
}

// ListActionItems handles GET /api/v1/teams/:teamId/action-items
//
// @Summary List a team's action items
// @Description Returns the action items belonging to the given team, newest first, optionally narrowed by status (open, in_progress, done) and/or assessment period via query parameters. Each item includes its dimension, creator, assignee, title, description, status, due date, assessment period, and timestamps, with names resolved for the linked dimension, creator, and assignee. Access requires membership on the team or manager-or-above privileges.
// @Tags Action Items
// @Produce json
// @Param teamId path string true "Team ID"
// @Param status query string false "Filter by status (open, in_progress, done)"
// @Param period query string false "Filter by assessment period"
// @Success 200 {object} dto.ActionItemsResponse
// @Failure 500 {object} dto.ErrorResponse "Failed to fetch/scan/read action items"
// @Security BearerAuth
// @Failure 401 {object} dto.ErrorResponse "Unauthorized"
// @Failure 403 {object} dto.ErrorResponse "Access denied: team membership or manager-or-above privileges required"
// @Router /teams/{teamId}/action-items [get]
func (h *ActionItemHandler) ListActionItems(c *gin.Context) {
	teamID := c.Param("teamId")
	status := c.Query("status")
	period := c.Query("period")

	items, err := h.fetchActionItems(c.Request.Context(), teamID, status, period)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to fetch action items", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, dto.ActionItemsResponse{ActionItems: items})
}

// fetchActionItems loads action items for a team, optionally filtered by
// status and assessment period. Shared by the team board and the
// manager/director/VP pod drill-down.
func (h *ActionItemHandler) fetchActionItems(ctx context.Context, teamID, status, period string) ([]dto.ActionItemResponse, error) {
	query := `
		SELECT
			ai.id, ai.team_id, ai.dimension_id,
			hd.name AS dimension_name,
			ai.created_by, cu.full_name AS created_by_name,
			ai.assigned_to, au.full_name AS assignee_name,
			ai.title, ai.description, ai.status,
			ai.due_date, ai.assessment_period,
			ai.created_at, ai.updated_at
		FROM action_items ai
		LEFT JOIN health_dimensions hd ON hd.id = ai.dimension_id
		LEFT JOIN users cu ON cu.id = ai.created_by
		LEFT JOIN users au ON au.id = ai.assigned_to
		WHERE ai.team_id = $1`

	args := []interface{}{teamID}
	idx := 2

	if status != "" {
		query += fmt.Sprintf(" AND ai.status = $%d", idx)
		args = append(args, status)
		idx++
	}
	if period != "" {
		query += fmt.Sprintf(" AND ai.assessment_period = $%d", idx)
		args = append(args, period)
	}
	query += " ORDER BY ai.created_at DESC"

	rows, err := h.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []dto.ActionItemResponse{}
	for rows.Next() {
		var item dto.ActionItemResponse
		var dimID, dimName, assignedTo, assigneeName sql.NullString
		var assessmentPeriod sql.NullString
		// due_date is a DATE column; the pq driver hands it back as a
		// time.Time, not a string. Scanning that directly into a
		// sql.NullString would let database/sql's default conversion format
		// it as RFC3339Nano (e.g. "2026-06-12T00:00:00Z"), which is why the
		// API previously emitted a timestamp instead of a date-only value.
		// Scanning into sql.NullTime and formatting explicitly with Go's
		// date-only layout keeps the API contract at YYYY-MM-DD.
		var dueDate sql.NullTime
		var createdAt, updatedAt time.Time

		if err := rows.Scan(
			&item.ID, &item.TeamID, &dimID, &dimName,
			&item.CreatedBy, &item.CreatedByName,
			&assignedTo, &assigneeName,
			&item.Title, &item.Description, &item.Status,
			&dueDate, &assessmentPeriod,
			&createdAt, &updatedAt,
		); err != nil {
			return nil, err
		}
		if dimID.Valid {
			item.DimensionID = &dimID.String
		}
		if dimName.Valid {
			item.DimensionName = &dimName.String
		}
		if assignedTo.Valid {
			item.AssignedTo = &assignedTo.String
		}
		if assigneeName.Valid {
			item.AssigneeName = &assigneeName.String
		}
		if dueDate.Valid {
			formatted := dueDate.Time.Format("2006-01-02")
			item.DueDate = &formatted
		}
		if assessmentPeriod.Valid {
			item.AssessmentPeriod = &assessmentPeriod.String
		}
		item.CreatedAt = createdAt.Format(time.RFC3339)
		item.UpdatedAt = updatedAt.Format(time.RFC3339)

		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

// CreateActionItem handles POST /api/v1/teams/:teamId/action-items
//
// @Summary Create a team action item
// @Description Creates a new action item for the given team with a title, description, and optional dimension, assignee, due date, and assessment period. New items are always created with status "open". When assignedTo is set, the target user must already be a member of the team or the request is rejected with a 400. Access requires membership on the team or manager-or-above privileges.
// @Tags Action Items
// @Accept json
// @Produce json
// @Param teamId path string true "Team ID"
// @Param body body dto.CreateActionItemRequest true "Action item"
// @Success 201 {object} map[string]interface{} "id, status, createdAt"
// @Failure 400 {object} dto.ErrorResponse "Invalid request, invalid dueDate, or assignedTo not a team member"
// @Failure 401 {object} dto.ErrorResponse "Unauthorized"
// @Failure 500 {object} dto.ErrorResponse "Failed to validate assignee or create action item"
// @Security BearerAuth
// @Failure 403 {object} dto.ErrorResponse "Access denied: team membership or manager-or-above privileges required"
// @Router /teams/{teamId}/action-items [post]
func (h *ActionItemHandler) CreateActionItem(c *gin.Context) {
	teamID := c.Param("teamId")

	claims, ok := middleware.GetClaimsFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "Unauthorized"})
		return
	}

	var req dto.CreateActionItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid request", Message: err.Error()})
		return
	}
	if !dto.ValidDueDate(req.DueDate) {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid dueDate format, expected YYYY-MM-DD"})
		return
	}

	// Enforce that assignedTo (if set) is a member of this team or the
	// creator's direct manager.
	if req.AssignedTo != nil {
		allowed, err := h.isAssigneeAllowed(c.Request.Context(), teamID, claims.UserID, *req.AssignedTo)
		if err != nil {
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to validate assignee", Message: err.Error()})
			return
		}
		if !allowed {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "assignedTo user is not a member of this team or the creator's direct manager"})
			return
		}
	}

	id := uuid.New().String()
	now := time.Now()

	_, err := h.db.ExecContext(c.Request.Context(), `
		INSERT INTO action_items
			(id, team_id, dimension_id, created_by, assigned_to, title, description, status, due_date, assessment_period, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'open',$8,$9,$10,$10)`,
		id, teamID,
		nullableString(req.DimensionID),
		claims.UserID,
		nullableString(req.AssignedTo),
		req.Title, req.Description,
		nullableString(req.DueDate),
		nullableString(req.AssessmentPeriod),
		now,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to create action item", Message: err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": id, "status": "open", "createdAt": now.Format(time.RFC3339)})
}

// UpdateActionItem handles PATCH /api/v1/teams/:teamId/action-items/:id
//
// @Summary Update a team action item
// @Description Partially updates the action item identified by the path ID within the given team; only the fields present in the request body are changed. Status, when provided, must be one of open, in_progress, or done. When assignedTo is set, the target user must already be a member of the team or the request is rejected with a 400. Returns a 404 if no matching item exists for that team. Access requires membership on the team or manager-or-above privileges.
// @Tags Action Items
// @Accept json
// @Produce json
// @Param teamId path string true "Team ID"
// @Param id path string true "Action item ID"
// @Param body body dto.UpdateActionItemRequest true "Fields to update"
// @Success 200 {object} map[string]bool "updated"
// @Failure 400 {object} dto.ErrorResponse "Invalid request, invalid status/dueDate, or assignedTo not a team member"
// @Failure 404 {object} dto.ErrorResponse "Action item not found"
// @Failure 500 {object} dto.ErrorResponse "Failed to validate assignee or update action item"
// @Security BearerAuth
// @Failure 401 {object} dto.ErrorResponse "Unauthorized"
// @Failure 403 {object} dto.ErrorResponse "Access denied: team membership or manager-or-above privileges required"
// @Router /teams/{teamId}/action-items/{id} [patch]
func (h *ActionItemHandler) UpdateActionItem(c *gin.Context) {
	teamID := c.Param("teamId")
	itemID := c.Param("id")

	claims, ok := middleware.GetClaimsFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "Unauthorized"})
		return
	}

	var req dto.UpdateActionItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid request", Message: err.Error()})
		return
	}

	// Validate status if provided
	if req.Status != nil && !dto.ValidActionItemStatuses[*req.Status] {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid status", Message: "status must be open, on_hold, in_progress, or done"})
		return
	}
	if !dto.ValidDueDate(req.DueDate) {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid dueDate format, expected YYYY-MM-DD"})
		return
	}

	// Fetch the current row to enforce status-transition rules and
	// creator-only edit authorization.
	var currentStatus, createdBy string
	if err := h.db.QueryRowContext(c.Request.Context(),
		`SELECT status, created_by FROM action_items WHERE id = $1 AND team_id = $2`,
		itemID, teamID).Scan(&currentStatus, &createdBy); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Action item not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to load action item", Message: err.Error()})
		return
	}

	// A status change is a workflow transition, permitted for anyone who can
	// reach this endpoint (team members with access to the team's board).
	// Only the allowed transitions may be applied.
	if req.Status != nil && *req.Status != currentStatus {
		if !dto.AllowedActionItemTransitions[currentStatus][*req.Status] {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid status transition", Message: fmt.Sprintf("cannot move from %s to %s", currentStatus, *req.Status)})
			return
		}
	}

	// Any edit to the content fields (as opposed to a pure status
	// transition) is restricted to the creator, and only while the item is
	// not Done.
	editsContent := req.DimensionID != nil || req.AssignedTo != nil || req.Title != nil ||
		req.Description != nil || req.DueDate != nil || req.AssessmentPeriod != nil
	if editsContent {
		if claims.UserID != createdBy {
			c.JSON(http.StatusForbidden, dto.ErrorResponse{Error: "Only the creator of this action item may edit it"})
			return
		}
		if currentStatus == "done" {
			c.JSON(http.StatusForbidden, dto.ErrorResponse{Error: "Done action items cannot be edited"})
			return
		}
	}

	// Enforce that assignedTo (if set) is a member of this team or the
	// creator's direct manager.
	if req.AssignedTo != nil {
		allowed, err := h.isAssigneeAllowed(c.Request.Context(), teamID, createdBy, *req.AssignedTo)
		if err != nil {
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to validate assignee", Message: err.Error()})
			return
		}
		if !allowed {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "assignedTo user is not a member of this team or the creator's direct manager"})
			return
		}
	}

	res, err := h.db.ExecContext(c.Request.Context(), `
		UPDATE action_items SET
			dimension_id      = COALESCE($3, dimension_id),
			assigned_to       = COALESCE($4, assigned_to),
			title             = COALESCE($5, title),
			description       = COALESCE($6, description),
			status            = COALESCE($7, status),
			due_date          = COALESCE($8, due_date),
			assessment_period = COALESCE($9, assessment_period),
			updated_at        = NOW()
		WHERE id = $1 AND team_id = $2`,
		itemID, teamID,
		nullableString(req.DimensionID),
		nullableString(req.AssignedTo),
		req.Title, req.Description, req.Status,
		nullableString(req.DueDate),
		nullableString(req.AssessmentPeriod),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to update action item", Message: err.Error()})
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Action item not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"updated": true})
}

// DeleteActionItem handles DELETE /api/v1/teams/:teamId/action-items/:id
//
// @Summary Delete a team action item
// @Description Permanently deletes the action item identified by the path ID from the given team. Returns a 404 if no matching item exists for that team. Access requires membership on the team or manager-or-above privileges.
// @Tags Action Items
// @Produce json
// @Param teamId path string true "Team ID"
// @Param id path string true "Action item ID"
// @Success 200 {object} map[string]bool "deleted"
// @Failure 404 {object} dto.ErrorResponse "Action item not found"
// @Failure 500 {object} dto.ErrorResponse "Failed to delete action item"
// @Security BearerAuth
// @Failure 401 {object} dto.ErrorResponse "Unauthorized"
// @Failure 403 {object} dto.ErrorResponse "Access denied: team membership or manager-or-above privileges required"
// @Router /teams/{teamId}/action-items/{id} [delete]
func (h *ActionItemHandler) DeleteActionItem(c *gin.Context) {
	teamID := c.Param("teamId")
	itemID := c.Param("id")

	res, err := h.db.ExecContext(c.Request.Context(), `
		DELETE FROM action_items WHERE id = $1 AND team_id = $2`, itemID, teamID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to delete action item", Message: err.Error()})
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Action item not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// GetTeamsActionSummary handles GET /api/v1/managers/:managerId/teams/action-items
//
// @Summary Get action item counts by team
// @Description Returns, for every team the given manager supervises, the team's ID, name, and count of action items whose status is not "done", ordered by team name. Access requires a manager-or-above role, and the authenticated caller may only request their own summary; requesting another manager's summary is rejected with a 403.
// @Tags Action Items
// @Produce json
// @Param managerId path string true "Manager ID"
// @Success 200 {object} dto.TeamsActionSummaryResponse
// @Failure 403 {object} dto.ErrorResponse "Forbidden — can only view your own summary"
// @Failure 500 {object} dto.ErrorResponse "Failed to fetch/read action summaries"
// @Security BearerAuth
// @Router /managers/{managerId}/teams/action-items [get]
func (h *ActionItemHandler) GetTeamsActionSummary(c *gin.Context) {
	managerID := c.Param("managerId")

	// Enforce that the authenticated user can only read their own summary.
	claims, ok := middleware.GetClaimsFromContext(c)
	if !ok || claims.UserID != managerID {
		c.JSON(http.StatusForbidden, dto.ErrorResponse{Error: "Forbidden"})
		return
	}

	rows, err := h.db.QueryContext(c.Request.Context(), `
		SELECT t.id, t.name, COUNT(ai.id) AS open_count
		FROM teams t
		INNER JOIN team_supervisors ts ON ts.team_id = t.id AND ts.user_id = $1
		LEFT JOIN action_items ai ON ai.team_id = t.id AND ai.status != 'done'
		GROUP BY t.id, t.name
		ORDER BY t.name`, managerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to fetch action summaries", Message: err.Error()})
		return
	}
	defer rows.Close()

	summaries := []dto.TeamActionSummaryResponse{}
	for rows.Next() {
		var s dto.TeamActionSummaryResponse
		if err := rows.Scan(&s.TeamID, &s.TeamName, &s.OpenCount); err != nil {
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to read action summaries", Message: err.Error()})
			return
		}
		summaries = append(summaries, s)
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to read action summaries", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, dto.TeamsActionSummaryResponse{Teams: summaries})
}

// isAssigneeAllowed returns true if candidateID is either a member of teamID
// or the direct manager (reports_to) of the action item's creator.
func (h *ActionItemHandler) isAssigneeAllowed(ctx context.Context, teamID, creatorID, candidateID string) (bool, error) {
	var allowed bool
	err := h.db.QueryRowContext(ctx, `
		SELECT
			EXISTS(SELECT 1 FROM team_members WHERE team_id = $1 AND user_id = $2)
			OR EXISTS(SELECT 1 FROM users WHERE id = $3 AND reports_to = $2)
	`, teamID, candidateID, creatorID).Scan(&allowed)
	if err != nil {
		return false, err
	}
	return allowed, nil
}

// GetDirectManager handles GET /api/v1/teams/:teamId/action-items/direct-manager
// Returns the authenticated user's direct manager (reports_to), if any.
func (h *ActionItemHandler) GetDirectManager(c *gin.Context) {
	claims, ok := middleware.GetClaimsFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "Unauthorized"})
		return
	}

	var id, name sql.NullString
	err := h.db.QueryRowContext(c.Request.Context(), `
		SELECT m.id, m.full_name
		FROM users u
		JOIN users m ON m.id = u.reports_to
		WHERE u.id = $1`, claims.UserID).Scan(&id, &name)
	if err != nil && err != sql.ErrNoRows {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to resolve direct manager", Message: err.Error()})
		return
	}

	resp := dto.DirectManagerResponse{}
	if id.Valid {
		resp.ID = &id.String
		resp.Name = &name.String
	}
	c.JSON(http.StatusOK, resp)
}

// GetPodActionItems handles GET /api/v1/managers/:managerId/teams/:teamId/action-items
// Returns the full, read-only action board for a pod in the manager's hierarchy.
func (h *ActionItemHandler) GetPodActionItems(c *gin.Context) {
	managerID := c.Param("managerId")
	teamID := c.Param("teamId")

	claims, ok := middleware.GetClaimsFromContext(c)
	if !ok || claims.UserID != managerID {
		c.JSON(http.StatusForbidden, dto.ErrorResponse{Error: "Forbidden"})
		return
	}

	var inScope bool
	if err := h.db.QueryRowContext(c.Request.Context(),
		`SELECT EXISTS(SELECT 1 FROM team_supervisors WHERE team_id = $1 AND user_id = $2)`,
		teamID, managerID).Scan(&inScope); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to validate pod access", Message: err.Error()})
		return
	}
	if !inScope {
		c.JSON(http.StatusForbidden, dto.ErrorResponse{Error: "Forbidden: team is not in your hierarchy"})
		return
	}

	items, err := h.fetchActionItems(c.Request.Context(), teamID, "", "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to fetch action items", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, dto.ActionItemsResponse{ActionItems: items})
}

// nullableString converts a *string to a value suitable for sql nullable param
func nullableString(s *string) interface{} {
	if s == nil {
		return nil
	}
	return *s
}
