// SPDX-License-Identifier: Apache-2.0

package dto

import "time"

// ============================================================================
// Hierarchy Levels DTOs
// ============================================================================

// HierarchyLevelDTO represents a hierarchy level with permissions
type HierarchyLevelDTO struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Position    int                     `json:"position"`
	Permissions HierarchyPermissionsDTO `json:"permissions"`
	CreatedAt   time.Time               `json:"createdAt"`
	UpdatedAt   time.Time               `json:"updatedAt"`
}

// HierarchyPermissionsDTO represents permissions for a hierarchy level
type HierarchyPermissionsDTO struct {
	CanViewAllTeams  bool `json:"canViewAllTeams"`
	CanEditTeams     bool `json:"canEditTeams"`
	CanManageUsers   bool `json:"canManageUsers"`
	CanTakeSurvey    bool `json:"canTakeSurvey"`
	CanViewAnalytics bool `json:"canViewAnalytics"`
}

// CreateHierarchyLevelRequest represents request to create a hierarchy level
type CreateHierarchyLevelRequest struct {
	ID          string                  `json:"id"`                      // Optional - will be auto-generated from name if not provided
	Name        string                  `json:"name" binding:"required"` // Required - used to generate ID if not provided
	Permissions HierarchyPermissionsDTO `json:"permissions"`
}

// UpdateHierarchyLevelRequest represents request to update a hierarchy level
type UpdateHierarchyLevelRequest struct {
	Name        string                   `json:"name"`
	Permissions *HierarchyPermissionsDTO `json:"permissions"`
}

// UpdateHierarchyPositionRequest represents request to reorder hierarchy levels
type UpdateHierarchyPositionRequest struct {
	NewPosition int `json:"newPosition" binding:"required,min=1"`
}

// HierarchyLevelsResponse represents response with list of hierarchy levels
type HierarchyLevelsResponse struct {
	Levels []HierarchyLevelDTO `json:"levels"`
}

// ============================================================================
// Users DTOs
// ============================================================================

// AdminUserDTO represents detailed user information for admin
type AdminUserDTO struct {
	ID             string    `json:"id"`
	Username       string    `json:"username"`
	Email          string    `json:"email"`
	FullName       string    `json:"fullName"`
	HierarchyLevel string    `json:"hierarchyLevel"`
	ReportsTo      *string   `json:"reportsTo"`
	TeamIds        []string  `json:"teamIds"`
	AuthType       string    `json:"authType"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// CreateUserRequest represents request to create a user
type CreateUserRequest struct {
	ID             string  `json:"id"`                          // Optional - will be auto-generated from username if not provided
	Username       string  `json:"username" binding:"required"` // Required - used to generate ID if not provided
	Email          string  `json:"email" binding:"required,email"`
	FullName       string  `json:"fullName" binding:"required"`
	Password       string  `json:"password"`                                     // Required for local users, omit for SSO users
	AuthType       string  `json:"authType" binding:"omitempty,oneof=local sso"` // "local" (default) or "sso"
	HierarchyLevel string  `json:"hierarchyLevel" binding:"required"`
	ReportsTo      *string `json:"reportsTo"`
}

// UpdateUserRequest represents request to update a user
type UpdateUserRequest struct {
	Username       *string `json:"username"`
	Email          *string `json:"email"`
	FullName       *string `json:"fullName"`
	Password       *string `json:"password"`
	AuthType       *string `json:"authType" binding:"omitempty,oneof=local sso"`
	HierarchyLevel *string `json:"hierarchyLevel"`
	ReportsTo      *string `json:"reportsTo"`
}

// UsersResponse represents response with list of users
type UsersResponse struct {
	Users []AdminUserDTO `json:"users"`
	Total int            `json:"total"`
}

// ============================================================================
// Teams DTOs
// ============================================================================

// AdminTeamDTO represents detailed team information for admin
type AdminTeamDTO struct {
	ID                    string    `json:"id"`
	Name                  string    `json:"name"`
	TeamLeadID            *string   `json:"teamLeadId"`
	TeamLeadName          *string   `json:"teamLeadName"`
	Cadence               string    `json:"cadence"`
	DistributionListEmail *string   `json:"distributionListEmail,omitempty"`
	MemberCount           int       `json:"memberCount"`
	CreatedAt             time.Time `json:"createdAt"`
	UpdatedAt             time.Time `json:"updatedAt"`
}

// CreateTeamRequest represents request to create a team
type CreateTeamRequest struct {
	ID                    string  `json:"id"`                      // Optional - will be auto-generated from name if not provided
	Name                  string  `json:"name" binding:"required"` // Required - used to generate ID if not provided
	TeamLeadID            *string `json:"teamLeadId"`
	Cadence               string  `json:"cadence" binding:"required,oneof=monthly quarterly half-yearly yearly"`
	DistributionListEmail *string `json:"distributionListEmail" binding:"omitempty,email"`
}

// UpdateTeamRequest represents request to update a team
type UpdateTeamRequest struct {
	Name                  *string `json:"name"`
	TeamLeadID            *string `json:"teamLeadId"`
	Cadence               *string `json:"cadence" binding:"omitempty,oneof=monthly quarterly half-yearly yearly"`
	DistributionListEmail *string `json:"distributionListEmail" binding:"omitempty,email"`
}

// TeamsResponse represents response with list of teams
type TeamsResponse struct {
	Teams []AdminTeamDTO `json:"teams"`
	Total int            `json:"total"`
}

// TeamMemberAdminDTO represents a team member in admin context
type TeamMemberAdminDTO struct {
	UserID   string `json:"userId"`
	UserName string `json:"userName"`
	Email    string `json:"email"`
}

// TeamMembersResponse represents response with list of team members
type TeamMembersResponse struct {
	Members []TeamMemberAdminDTO `json:"members"`
	Total   int                  `json:"total"`
}

// AddTeamMemberRequest represents request to add a member to a team
type AddTeamMemberRequest struct {
	UserID string `json:"userId" binding:"required"`
}

// SupervisorLinkDTO represents a supervisor in the chain with display names
type SupervisorLinkDTO struct {
	UserID    string `json:"userId"`
	UserName  string `json:"userName"`
	LevelID   string `json:"levelId"`
	LevelName string `json:"levelName"`
}

// SupervisorChainResponse represents the full supervisor chain for a team
type SupervisorChainResponse struct {
	TeamID      string              `json:"teamId"`
	Supervisors []SupervisorLinkDTO `json:"supervisors"`
}

// UpdateSupervisorChainRequest represents request to update a team's supervisor chain
type UpdateSupervisorChainRequest struct {
	Supervisors []SupervisorLinkInput `json:"supervisors" binding:"required"`
}

// SupervisorLinkInput represents a supervisor link in update requests
type SupervisorLinkInput struct {
	UserID  string `json:"userId" binding:"required"`
	LevelID string `json:"levelId" binding:"required"`
}

// ============================================================================
// Settings DTOs
// ============================================================================

// HealthDimensionDTO represents a health check dimension
type HealthDimensionDTO struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	GoodDescription string    `json:"goodDescription"`
	BadDescription  string    `json:"badDescription"`
	IsActive        bool      `json:"isActive"`
	Weight          float64   `json:"weight"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// CreateDimensionRequest represents request to create a dimension
type CreateDimensionRequest struct {
	ID              string  `json:"id" binding:"required"`
	Name            string  `json:"name" binding:"required"`
	Description     string  `json:"description"`
	GoodDescription string  `json:"goodDescription" binding:"required"`
	BadDescription  string  `json:"badDescription" binding:"required"`
	IsActive        *bool   `json:"isActive"`
	Weight          float64 `json:"weight"`
}

// UpdateDimensionRequest represents request to update a dimension
type UpdateDimensionRequest struct {
	Name            *string  `json:"name"`
	Description     *string  `json:"description"`
	GoodDescription *string  `json:"goodDescription"`
	BadDescription  *string  `json:"badDescription"`
	IsActive        *bool    `json:"isActive"`
	Weight          *float64 `json:"weight"`
}

// DimensionsResponse represents response with list of dimensions
type DimensionsResponse struct {
	Dimensions []HealthDimensionDTO `json:"dimensions"`
}

// BrandingSettings represents company branding configuration
type BrandingSettings struct {
	CompanyName string `json:"companyName" binding:"required,max=100"`
	LogoURL     string `json:"logoURL"`
}

// NotificationSettings represents notification configuration
type NotificationSettings struct {
	EmailEnabled       bool     `json:"emailEnabled"`
	SlackEnabled       bool     `json:"slackEnabled"`
	NotifyOnSubmission bool     `json:"notifyOnSubmission"`
	NotifyManagers     bool     `json:"notifyManagers"`
	ReminderDaysBefore int      `json:"reminderDaysBefore"`
	ReminderRecipients []string `json:"reminderRecipients"`
	SmtpConfigured     bool     `json:"smtpConfigured"`
}

// RetentionPolicy represents data retention configuration
type RetentionPolicy struct {
	KeepSessionsMonths int  `json:"keepSessionsMonths"`
	ArchiveEnabled     bool `json:"archiveEnabled"`
	AnonymizeAfterDays int  `json:"anonymizeAfterDays"`
}
