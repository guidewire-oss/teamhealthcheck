package v1

import (
	"errors"
	"net/http"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/agopalakrishnan/teams360/backend/interfaces/middleware"
	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
	"github.com/gin-gonic/gin"
)

// OrganizationSyncScheduleHandler serves the automatic-sync schedule. It is
// mounted only on the admin group (JWT + AdminOnly), like every sync route.
type OrganizationSyncScheduleHandler struct {
	scheduler *services.OrganizationSyncScheduler
}

// GetAutoSync handles GET /api/v1/admin/organization-provider/auto-sync.
func (h *OrganizationSyncScheduleHandler) GetAutoSync(c *gin.Context) {
	st, err := h.scheduler.Status(c.Request.Context())
	if err != nil {
		logger.Get().WithError(err).Error("failed to load auto sync schedule")
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to load auto sync schedule"})
		return
	}
	c.JSON(http.StatusOK, autoSyncDTO(st))
}

// UpdateAutoSync handles PUT /api/v1/admin/organization-provider/auto-sync.
// It saves configuration only; it never starts a sync.
func (h *OrganizationSyncScheduleHandler) UpdateAutoSync(c *gin.Context) {
	var req dto.UpdateAutoSyncScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Enabled == nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "Invalid request body",
			Message: `Expected {"enabled": true|false, "frequency": "DAILY"|"WEEKLY"|"MONTHLY"}`,
		})
		return
	}
	if *req.Enabled && req.Frequency == "" {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error: "Invalid frequency", Message: "frequency is required when enabling auto sync",
		})
		return
	}
	actor, _ := middleware.GetUserIDFromContext(c)
	st, err := h.scheduler.Update(c.Request.Context(), *req.Enabled, req.Frequency, actor)
	switch {
	case errors.Is(err, services.ErrInvalidSchedule):
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error: "Invalid frequency", Message: "frequency must be one of DAILY, WEEKLY, MONTHLY",
		})
	case err != nil:
		logger.Get().WithError(err).Error("failed to save auto sync schedule")
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to save auto sync schedule"})
	default:
		c.JSON(http.StatusOK, autoSyncDTO(st))
	}
}

func autoSyncDTO(st *services.AutoSyncStatus) dto.AutoSyncScheduleDTO {
	d := dto.AutoSyncScheduleDTO{
		Enabled: st.Enabled, Frequency: string(st.Frequency), Description: st.Description,
		Timezone: st.Timezone, NextRunAt: st.NextRunAt, LastSuccessAt: st.LastSuccessAt,
	}
	if a := st.LastAttempt; a != nil {
		d.LastAttempt = &dto.AutoSyncAttemptDTO{
			RunID: a.RunID, Status: string(a.Status), StartedAt: a.StartedAt,
			CompletedAt: a.CompletedAt, ErrorMessage: a.ErrorMessage,
		}
	}
	return d
}
