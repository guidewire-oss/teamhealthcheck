// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/agopalakrishnan/teams360/backend/domain/organization"
	"github.com/agopalakrishnan/teams360/backend/domain/team"
	"github.com/agopalakrishnan/teams360/backend/domain/user"
	"github.com/gin-gonic/gin"
)

// AdminHandler aggregates all admin sub-handlers
type AdminHandler struct {
	HierarchyHandler *HierarchyAdminHandler
	UserHandler      *UserAdminHandler
	TeamHandler      *TeamAdminHandler
	SettingsHandler  *SettingsAdminHandler
}

// NewAdminHandler creates a new AdminHandler with all sub-handlers
func NewAdminHandler(orgRepo organization.Repository, userRepo user.Repository, teamRepo team.Repository) *AdminHandler {
	return &AdminHandler{
		HierarchyHandler: NewHierarchyAdminHandler(orgRepo),
		UserHandler:      NewUserAdminHandler(userRepo, teamRepo),
		TeamHandler:      NewTeamAdminHandler(teamRepo, userRepo, orgRepo),
		SettingsHandler:  NewSettingsAdminHandler(orgRepo),
	}
}

// ============================================================================
// Hierarchy Levels Handlers - Delegate to HierarchyAdminHandler
// ============================================================================

func (h *AdminHandler) ListHierarchyLevels(c *gin.Context) {
	h.HierarchyHandler.ListHierarchyLevels(c)
}

func (h *AdminHandler) CreateHierarchyLevel(c *gin.Context) {
	h.HierarchyHandler.CreateHierarchyLevel(c)
}

func (h *AdminHandler) UpdateHierarchyLevel(c *gin.Context) {
	h.HierarchyHandler.UpdateHierarchyLevel(c)
}

func (h *AdminHandler) UpdateHierarchyPosition(c *gin.Context) {
	h.HierarchyHandler.UpdateHierarchyPosition(c)
}

func (h *AdminHandler) DeleteHierarchyLevel(c *gin.Context) {
	h.HierarchyHandler.DeleteHierarchyLevel(c)
}

// ============================================================================
// Users Handlers - Delegate to UserAdminHandler
// ============================================================================

func (h *AdminHandler) ListUsers(c *gin.Context) {
	h.UserHandler.ListUsers(c)
}

func (h *AdminHandler) CreateUser(c *gin.Context) {
	h.UserHandler.CreateUser(c)
}

func (h *AdminHandler) UpdateUser(c *gin.Context) {
	h.UserHandler.UpdateUser(c)
}

func (h *AdminHandler) DeleteUser(c *gin.Context) {
	h.UserHandler.DeleteUser(c)
}

// ============================================================================
// Teams Handlers - Delegate to TeamAdminHandler
// ============================================================================

func (h *AdminHandler) ListTeams(c *gin.Context) {
	h.TeamHandler.ListTeams(c)
}

func (h *AdminHandler) CreateTeam(c *gin.Context) {
	h.TeamHandler.CreateTeam(c)
}

func (h *AdminHandler) UpdateTeam(c *gin.Context) {
	h.TeamHandler.UpdateTeam(c)
}

func (h *AdminHandler) DeleteTeam(c *gin.Context) {
	h.TeamHandler.DeleteTeam(c)
}

func (h *AdminHandler) GetTeamMembers(c *gin.Context) {
	h.TeamHandler.GetTeamMembers(c)
}

func (h *AdminHandler) AddTeamMember(c *gin.Context) {
	h.TeamHandler.AddTeamMember(c)
}

func (h *AdminHandler) RemoveTeamMember(c *gin.Context) {
	h.TeamHandler.RemoveTeamMember(c)
}

func (h *AdminHandler) GetTeamSupervisors(c *gin.Context) {
	h.TeamHandler.GetSupervisorChain(c)
}

func (h *AdminHandler) UpdateTeamSupervisors(c *gin.Context) {
	h.TeamHandler.UpdateSupervisorChain(c)
}

// ============================================================================
// Settings Handlers - Delegate to SettingsAdminHandler
// ============================================================================

func (h *AdminHandler) GetDimensions(c *gin.Context) {
	h.SettingsHandler.GetDimensions(c)
}

func (h *AdminHandler) CreateDimension(c *gin.Context) {
	h.SettingsHandler.CreateDimension(c)
}

func (h *AdminHandler) UpdateDimension(c *gin.Context) {
	h.SettingsHandler.UpdateDimension(c)
}

func (h *AdminHandler) DeleteDimension(c *gin.Context) {
	h.SettingsHandler.DeleteDimension(c)
}

func (h *AdminHandler) GetBrandingSettings(c *gin.Context) {
	h.SettingsHandler.GetBrandingSettings(c)
}

func (h *AdminHandler) UpdateBrandingSettings(c *gin.Context) {
	h.SettingsHandler.UpdateBrandingSettings(c)
}

func (h *AdminHandler) GetNotificationSettings(c *gin.Context) {
	h.SettingsHandler.GetNotificationSettings(c)
}

func (h *AdminHandler) UpdateNotificationSettings(c *gin.Context) {
	h.SettingsHandler.UpdateNotificationSettings(c)
}

func (h *AdminHandler) GetRetentionPolicy(c *gin.Context) {
	h.SettingsHandler.GetRetentionPolicy(c)
}

func (h *AdminHandler) UpdateRetentionPolicy(c *gin.Context) {
	h.SettingsHandler.UpdateRetentionPolicy(c)
}
