package v1

import (
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
func (h *ActionItemHandler) ListActionItems(c *gin.Context) {
	teamID := c.Param("teamId")
	status := c.Query("status")
	period := c.Query("period")

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

	rows, err := h.db.QueryContext(c.Request.Context(), query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to fetch action items", Message: err.Error()})
		return
	}
	defer rows.Close()

	items := []dto.ActionItemResponse{}
	for rows.Next() {
		var item dto.ActionItemResponse
		var dimID, dimName, assignedTo, assigneeName sql.NullString
		var dueDate, assessmentPeriod sql.NullString
		var createdAt, updatedAt time.Time

		if err := rows.Scan(
			&item.ID, &item.TeamID, &dimID, &dimName,
			&item.CreatedBy, &item.CreatedByName,
			&assignedTo, &assigneeName,
			&item.Title, &item.Description, &item.Status,
			&dueDate, &assessmentPeriod,
			&createdAt, &updatedAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to scan action items", Message: err.Error()})
			return
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
			item.DueDate = &dueDate.String
		}
		if assessmentPeriod.Valid {
			item.AssessmentPeriod = &assessmentPeriod.String
		}
		item.CreatedAt = createdAt.Format(time.RFC3339)
		item.UpdatedAt = updatedAt.Format(time.RFC3339)

		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to read action items", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, dto.ActionItemsResponse{ActionItems: items})
}

// CreateActionItem handles POST /api/v1/teams/:teamId/action-items
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

	// Enforce that assignedTo (if set) is a member of this team.
	if req.AssignedTo != nil {
		var exists bool
		if err := h.db.QueryRowContext(c.Request.Context(),
			`SELECT EXISTS(SELECT 1 FROM team_members WHERE team_id=$1 AND user_id=$2)`,
			teamID, *req.AssignedTo).Scan(&exists); err != nil {
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to validate assignee", Message: err.Error()})
			return
		} else if !exists {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "assignedTo user is not a member of this team"})
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
func (h *ActionItemHandler) UpdateActionItem(c *gin.Context) {
	teamID := c.Param("teamId")
	itemID := c.Param("id")

	var req dto.UpdateActionItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid request", Message: err.Error()})
		return
	}

	// Validate status if provided
	if req.Status != nil {
		switch *req.Status {
		case "open", "in_progress", "done":
		default:
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid status", Message: "status must be open, in_progress, or done"})
			return
		}
	}
	if !dto.ValidDueDate(req.DueDate) {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid dueDate format, expected YYYY-MM-DD"})
		return
	}

	// Enforce that assignedTo (if set) is a member of this team.
	if req.AssignedTo != nil {
		var exists bool
		if err := h.db.QueryRowContext(c.Request.Context(),
			`SELECT EXISTS(SELECT 1 FROM team_members WHERE team_id=$1 AND user_id=$2)`,
			teamID, *req.AssignedTo).Scan(&exists); err != nil {
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to validate assignee", Message: err.Error()})
			return
		} else if !exists {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "assignedTo user is not a member of this team"})
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

// nullableString converts a *string to a value suitable for sql nullable param
func nullableString(s *string) interface{} {
	if s == nil {
		return nil
	}
	return *s
}
