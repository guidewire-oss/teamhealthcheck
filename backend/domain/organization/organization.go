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

	// OrgSyncMaxDeletePercent is the administrator-configured ceiling, as a
	// percentage from 1 to 100, on how much of the existing non-protected
	// user or team population one organization sync may delete before the
	// sync is held for review. nil means no administrator has saved a value,
	// which is what makes the environment-variable fallback reachable.
	OrgSyncMaxDeletePercent *float64 `json:"orgSyncMaxDeletePercent,omitempty"`
}

// Automatic organization-sync frequencies. Never exposed to admins as cron
// syntax -- the UI presents these three fixed choices only.
const (
	OrgSyncFrequencyDaily   = "daily"
	OrgSyncFrequencyWeekly  = "weekly"
	OrgSyncFrequencyMonthly = "monthly"
)

// OrgSyncSchedule is the admin-configured automatic organization-sync
// schedule. Disabled by default, for both new and existing deployments.
// Frequency and NextRunAt are meaningful only while Enabled -- a database
// CHECK constraint enforces the same rule, so no code path can leave the
// scheduler "on, but with no idea when".
type OrgSyncSchedule struct {
	Enabled bool `json:"enabled"`
	// Frequency is one of the OrgSyncFrequency* constants, or "" when never
	// configured.
	Frequency string `json:"frequency,omitempty"`
	// NextRunAt is a UTC instant, not a wall-clock local time -- see the
	// scheduler package's doc on why. nil while disabled.
	NextRunAt *time.Time `json:"nextRunAt,omitempty"`
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

	// Organization-sync mass-deletion threshold (percentage, 1-100).
	// A nil result means no administrator has configured one.
	GetOrgSyncMaxDeletePercent(ctx context.Context) (*float64, error)
	UpdateOrgSyncMaxDeletePercent(ctx context.Context, percent float64) error

	// GetOrgSyncSchedule reads the automatic-sync schedule for admin display.
	GetOrgSyncSchedule(ctx context.Context) (*OrgSyncSchedule, error)
	// UpdateOrgSyncSchedule is the ADMIN-facing write: enabling, disabling, or
	// changing frequency. It INITIALIZES or RECOMPUTES nextRunAt -- it never
	// ADVANCES an already-due occurrence, which is ClaimOrgSyncOccurrence's
	// job alone. The caller (the service layer) computes nextRunAt fresh from
	// "now" before calling this; frequency is "" when disabling (preserved
	// separately is a service-layer concern, not this method's).
	UpdateOrgSyncSchedule(ctx context.Context, enabled bool, frequency string, nextRunAt *time.Time) error
	// ClaimOrgSyncOccurrence is the ONLY operation that ever advances an
	// already-enabled, already-due occurrence to its next one. It succeeds
	// (true) only if the schedule is still enabled and next_run_at still
	// equals observedNextRunAt -- an atomic compare-and-swap, so of every
	// replica racing the same due occurrence, exactly one wins. A false
	// result (no error) means another replica already claimed it, or an
	// admin disabled the schedule in the interim; the caller does nothing
	// further in either case.
	ClaimOrgSyncOccurrence(ctx context.Context, observedNextRunAt, newNextRunAt time.Time) (bool, error)
}
