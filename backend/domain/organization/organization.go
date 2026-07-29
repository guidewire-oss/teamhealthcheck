// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

package organization

import (
	"context"
	"time"
)

// HierarchyLevel defines a level in the organizational hierarchy
type HierarchyLevel struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Position    int         `json:"position"` // Renamed from Level to Position for clarity
	Color       string      `json:"color,omitempty"`
	Permissions Permissions `json:"permissions"`
	CreatedAt   time.Time   `json:"createdAt,omitempty"`
	UpdatedAt   time.Time   `json:"updatedAt,omitempty"`
}

// Permissions defines what a hierarchy level can do
type Permissions struct {
	CanViewAllTeams    bool `json:"canViewAllTeams"`
	CanEditTeams       bool `json:"canEditTeams"`
	CanManageUsers     bool `json:"canManageUsers"`
	CanTakeSurvey      bool `json:"canTakeSurvey"`
	CanViewAnalytics   bool `json:"canViewAnalytics"`
	CanConfigureSystem bool `json:"canConfigureSystem,omitempty"`
	CanViewReports     bool `json:"canViewReports,omitempty"`
	CanExportData      bool `json:"canExportData,omitempty"`
}

// HealthDimension represents a health check dimension
type HealthDimension struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	GoodDescription string    `json:"goodDescription"`
	BadDescription  string    `json:"badDescription"`
	IsActive        bool      `json:"isActive"`
	Weight          float64   `json:"weight"`
	CreatedAt       time.Time `json:"createdAt,omitempty"`
	UpdatedAt       time.Time `json:"updatedAt,omitempty"`
}

// AppSettings represents application-wide settings (singleton row)
type AppSettings struct {
	EmailNotifications bool   `json:"emailNotifications"`
	SlackNotifications bool   `json:"slackNotifications"`
	WeeklyDigest       bool   `json:"weeklyDigest"`
	RetentionMonths    int    `json:"retentionMonths"`
	CompanyName        string `json:"companyName"`
	LogoURL            string `json:"logoURL"`
}

// OrganizationConfig represents the organization configuration
// This is an aggregate root in DDD terms
type OrganizationConfig struct {
	ID                string           `json:"id"`
	CompanyName       string           `json:"companyName"`
	HierarchyLevels   []HierarchyLevel `json:"hierarchyLevels"`
	TeamMemberLevelID string           `json:"teamMemberLevelId"`
	CreatedAt         string           `json:"createdAt"`
	UpdatedAt         string           `json:"updatedAt"`
}

// Repository defines the interface for organization data access
type Repository interface {
	// Organization config
	Get(ctx context.Context) (*OrganizationConfig, error)
	Save(ctx context.Context, config *OrganizationConfig) error

	// Hierarchy levels
	FindHierarchyLevels(ctx context.Context) ([]*HierarchyLevel, error)
	FindHierarchyLevelByID(ctx context.Context, id string) (*HierarchyLevel, error)
	SaveHierarchyLevel(ctx context.Context, level *HierarchyLevel) error
	UpdateHierarchyLevel(ctx context.Context, level *HierarchyLevel) error
	DeleteHierarchyLevel(ctx context.Context, id string) error
	GetMaxHierarchyPosition(ctx context.Context) (int, error)
	UpdateHierarchyPosition(ctx context.Context, tx interface{}, id string, newPosition int) error
	ShiftHierarchyPositions(ctx context.Context, tx interface{}, start, end int, delta int) error
	CountUsersAtLevel(ctx context.Context, levelID string) (int, error)
	BeginTx(ctx context.Context) (interface{}, error)
	CommitTx(tx interface{}) error
	RollbackTx(tx interface{}) error

	// Health dimensions
	FindDimensions(ctx context.Context) ([]*HealthDimension, error)
	FindDimensionByID(ctx context.Context, id string) (*HealthDimension, error)
	SaveDimension(ctx context.Context, dim *HealthDimension) error
	UpdateDimension(ctx context.Context, dim *HealthDimension) error
	DeleteDimension(ctx context.Context, id string) error

	// App settings (singleton)
	GetAppSettings(ctx context.Context) (*AppSettings, error)
	UpdateAppSettings(ctx context.Context, settings *AppSettings) error
	UpdateBrandingSettings(ctx context.Context, companyName string, logoURL string) error
	UpdateNotificationSettings(ctx context.Context, email, slack, digest bool) error
	UpdateRetentionSettings(ctx context.Context, months int) error
}
