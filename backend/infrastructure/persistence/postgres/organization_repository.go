package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/agopalakrishnan/teams360/backend/domain/organization"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres/sqlcgen"
)

// OrganizationRepository implements the organization.Repository interface
type OrganizationRepository struct {
	db      *sql.DB
	queries *sqlcgen.Queries
}

// NewOrganizationRepository creates a new repository instance
func NewOrganizationRepository(db *sql.DB) organization.Repository {
	return &OrganizationRepository{db: db, queries: sqlcgen.New(db)}
}

// buildHierarchyLevelFromRow maps the scalar columns shared by every
// hierarchy-level-select query (each sqlc query produces its own distinct
// row type even when structurally identical, so this takes plain scalars).
func buildHierarchyLevelFromRow(id, name string, position int32, color sql.NullString, canViewAllTeams, canEditTeams, canManageUsers, canTakeSurvey, canViewAnalytics, canConfigureSystem, canViewReports, canExportData sql.NullBool, createdAt, updatedAt sql.NullTime) *organization.HierarchyLevel {
	level := &organization.HierarchyLevel{
		ID:       id,
		Name:     name,
		Position: int(position),
	}

	if color.Valid {
		level.Color = color.String
	}
	level.Permissions.CanViewAllTeams = canViewAllTeams.Valid && canViewAllTeams.Bool
	level.Permissions.CanEditTeams = canEditTeams.Valid && canEditTeams.Bool
	level.Permissions.CanManageUsers = canManageUsers.Valid && canManageUsers.Bool
	level.Permissions.CanTakeSurvey = canTakeSurvey.Valid && canTakeSurvey.Bool
	level.Permissions.CanViewAnalytics = canViewAnalytics.Valid && canViewAnalytics.Bool
	if canConfigureSystem.Valid {
		level.Permissions.CanConfigureSystem = canConfigureSystem.Bool
	}
	if canViewReports.Valid {
		level.Permissions.CanViewReports = canViewReports.Bool
	}
	if canExportData.Valid {
		level.Permissions.CanExportData = canExportData.Bool
	}
	if createdAt.Valid {
		level.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		level.UpdatedAt = updatedAt.Time
	}

	return level
}

// buildDimensionFromRow maps a sqlcgen.HealthDimension row to the domain struct.
func buildDimensionFromRow(row sqlcgen.HealthDimension) *organization.HealthDimension {
	dim := &organization.HealthDimension{
		ID:              row.ID,
		Name:            row.Name,
		Description:     row.Description,
		GoodDescription: row.GoodDescription,
		BadDescription:  row.BadDescription,
	}

	if row.IsActive.Valid {
		dim.IsActive = row.IsActive.Bool
	} else {
		dim.IsActive = true // default
	}
	if row.Weight.Valid {
		dim.Weight = row.Weight.Float64
	} else {
		dim.Weight = 1.0 // default
	}
	if row.CreatedAt.Valid {
		dim.CreatedAt = row.CreatedAt.Time
	}
	if row.UpdatedAt.Valid {
		dim.UpdatedAt = row.UpdatedAt.Time
	}

	return dim
}

// Get retrieves the organization configuration
func (r *OrganizationRepository) Get(ctx context.Context) (*organization.OrganizationConfig, error) {
	var config organization.OrganizationConfig

	// For now, construct config from hierarchy_levels
	config.ID = "default"
	config.CompanyName = "Team360"
	config.TeamMemberLevelID = "level-5" // Default team member level

	// Fetch hierarchy levels
	levels, err := r.FindHierarchyLevels(ctx)
	if err != nil {
		return nil, err
	}

	// Convert []*HierarchyLevel to []HierarchyLevel
	config.HierarchyLevels = make([]organization.HierarchyLevel, len(levels))
	for i, level := range levels {
		config.HierarchyLevels[i] = *level
	}

	return &config, nil
}

// Save persists the organization configuration
func (r *OrganizationRepository) Save(ctx context.Context, config *organization.OrganizationConfig) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Save hierarchy levels
	for i := range config.HierarchyLevels {
		if err := r.saveHierarchyLevelTx(ctx, tx, &config.HierarchyLevels[i]); err != nil {
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// FindHierarchyLevels retrieves all hierarchy levels
func (r *OrganizationRepository) FindHierarchyLevels(ctx context.Context) ([]*organization.HierarchyLevel, error) {
	rows, err := r.queries.FindHierarchyLevels(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query hierarchy levels: %w", err)
	}

	levels := make([]*organization.HierarchyLevel, 0, len(rows))
	for _, row := range rows {
		levels = append(levels, buildHierarchyLevelFromRow(row.ID, row.Name, row.Position, row.Color,
			row.CanViewAllTeams, row.CanEditTeams, row.CanManageUsers, row.CanTakeSurvey, row.CanViewAnalytics,
			row.CanConfigureSystem, row.CanViewReports, row.CanExportData, row.CreatedAt, row.UpdatedAt))
	}

	return levels, nil
}

// FindHierarchyLevelByID retrieves a specific hierarchy level by ID
func (r *OrganizationRepository) FindHierarchyLevelByID(ctx context.Context, id string) (*organization.HierarchyLevel, error) {
	row, err := r.queries.FindHierarchyLevelByID(ctx, id)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("hierarchy level not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find hierarchy level: %w", err)
	}

	return buildHierarchyLevelFromRow(row.ID, row.Name, row.Position, row.Color,
		row.CanViewAllTeams, row.CanEditTeams, row.CanManageUsers, row.CanTakeSurvey, row.CanViewAnalytics,
		row.CanConfigureSystem, row.CanViewReports, row.CanExportData, row.CreatedAt, row.UpdatedAt), nil
}

// SaveHierarchyLevel persists a new hierarchy level
func (r *OrganizationRepository) SaveHierarchyLevel(ctx context.Context, level *organization.HierarchyLevel) error {
	var color sql.NullString
	if level.Color != "" {
		color = sql.NullString{String: level.Color, Valid: true}
	}

	// Set timestamps
	now := time.Now()
	if level.CreatedAt.IsZero() {
		level.CreatedAt = now
	}
	level.UpdatedAt = now

	err := r.queries.InsertHierarchyLevel(ctx, sqlcgen.InsertHierarchyLevelParams{
		ID:                 level.ID,
		Name:               level.Name,
		Position:           int32(level.Position),
		Color:              color,
		CanViewAllTeams:    sql.NullBool{Bool: level.Permissions.CanViewAllTeams, Valid: true},
		CanEditTeams:       sql.NullBool{Bool: level.Permissions.CanEditTeams, Valid: true},
		CanManageUsers:     sql.NullBool{Bool: level.Permissions.CanManageUsers, Valid: true},
		CanTakeSurvey:      sql.NullBool{Bool: level.Permissions.CanTakeSurvey, Valid: true},
		CanViewAnalytics:   sql.NullBool{Bool: level.Permissions.CanViewAnalytics, Valid: true},
		CanConfigureSystem: sql.NullBool{Bool: level.Permissions.CanConfigureSystem, Valid: true},
		CanViewReports:     sql.NullBool{Bool: level.Permissions.CanViewReports, Valid: true},
		CanExportData:      sql.NullBool{Bool: level.Permissions.CanExportData, Valid: true},
		CreatedAt:          sql.NullTime{Time: level.CreatedAt, Valid: true},
		UpdatedAt:          sql.NullTime{Time: level.UpdatedAt, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("failed to save hierarchy level: %w", err)
	}

	return nil
}

// UpdateHierarchyLevel updates an existing hierarchy level
func (r *OrganizationRepository) UpdateHierarchyLevel(ctx context.Context, level *organization.HierarchyLevel) error {
	var color sql.NullString
	if level.Color != "" {
		color = sql.NullString{String: level.Color, Valid: true}
	}

	level.UpdatedAt = time.Now()

	rowsAffected, err := r.queries.UpdateHierarchyLevel(ctx, sqlcgen.UpdateHierarchyLevelParams{
		Name:               level.Name,
		Position:           int32(level.Position),
		Color:              color,
		CanViewAllTeams:    sql.NullBool{Bool: level.Permissions.CanViewAllTeams, Valid: true},
		CanEditTeams:       sql.NullBool{Bool: level.Permissions.CanEditTeams, Valid: true},
		CanManageUsers:     sql.NullBool{Bool: level.Permissions.CanManageUsers, Valid: true},
		CanTakeSurvey:      sql.NullBool{Bool: level.Permissions.CanTakeSurvey, Valid: true},
		CanViewAnalytics:   sql.NullBool{Bool: level.Permissions.CanViewAnalytics, Valid: true},
		CanConfigureSystem: sql.NullBool{Bool: level.Permissions.CanConfigureSystem, Valid: true},
		CanViewReports:     sql.NullBool{Bool: level.Permissions.CanViewReports, Valid: true},
		CanExportData:      sql.NullBool{Bool: level.Permissions.CanExportData, Valid: true},
		UpdatedAt:          sql.NullTime{Time: level.UpdatedAt, Valid: true},
		ID:                 level.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to update hierarchy level: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("hierarchy level not found: %s", level.ID)
	}

	return nil
}

// DeleteHierarchyLevel removes a hierarchy level
func (r *OrganizationRepository) DeleteHierarchyLevel(ctx context.Context, id string) error {
	rowsAffected, err := r.queries.DeleteHierarchyLevel(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to delete hierarchy level: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("hierarchy level not found: %s", id)
	}

	return nil
}

// GetMaxHierarchyPosition returns the maximum position value
func (r *OrganizationRepository) GetMaxHierarchyPosition(ctx context.Context) (int, error) {
	maxPosition, err := r.queries.GetMaxHierarchyPosition(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get max position: %w", err)
	}

	return int(maxPosition), nil
}

// UpdateHierarchyPosition updates a hierarchy level's position
func (r *OrganizationRepository) UpdateHierarchyPosition(ctx context.Context, tx interface{}, id string, newPosition int) error {
	q := r.queries
	if tx != nil {
		sqlTx, ok := tx.(*sql.Tx)
		if !ok {
			return fmt.Errorf("invalid transaction type")
		}
		q = q.WithTx(sqlTx)
	}

	err := q.UpdateHierarchyPositionQuery(ctx, sqlcgen.UpdateHierarchyPositionQueryParams{
		NewPosition: int32(newPosition),
		UpdatedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		ID:          id,
	})
	if err != nil {
		return fmt.Errorf("failed to update hierarchy position: %w", err)
	}

	return nil
}

// ShiftHierarchyPositions shifts positions in a range
func (r *OrganizationRepository) ShiftHierarchyPositions(ctx context.Context, tx interface{}, start, end int, delta int) error {
	q := r.queries
	if tx != nil {
		sqlTx, ok := tx.(*sql.Tx)
		if !ok {
			return fmt.Errorf("invalid transaction type")
		}
		q = q.WithTx(sqlTx)
	}

	err := q.ShiftHierarchyPositionsQuery(ctx, sqlcgen.ShiftHierarchyPositionsQueryParams{
		Delta:         int32(delta),
		UpdatedAt:     sql.NullTime{Time: time.Now(), Valid: true},
		StartPosition: int32(start),
		EndPosition:   int32(end),
	})
	if err != nil {
		return fmt.Errorf("failed to shift hierarchy positions: %w", err)
	}

	return nil
}

// CountUsersAtLevel counts users at a specific hierarchy level
func (r *OrganizationRepository) CountUsersAtLevel(ctx context.Context, levelID string) (int, error) {
	count, err := r.queries.CountUsersAtLevel(ctx, levelID)
	if err != nil {
		return 0, fmt.Errorf("failed to count users at level: %w", err)
	}

	return int(count), nil
}

// BeginTx starts a transaction
func (r *OrganizationRepository) BeginTx(ctx context.Context) (interface{}, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	return tx, nil
}

// CommitTx commits a transaction
func (r *OrganizationRepository) CommitTx(tx interface{}) error {
	sqlTx, ok := tx.(*sql.Tx)
	if !ok {
		return fmt.Errorf("invalid transaction type")
	}
	if err := sqlTx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// RollbackTx rolls back a transaction
func (r *OrganizationRepository) RollbackTx(tx interface{}) error {
	sqlTx, ok := tx.(*sql.Tx)
	if !ok {
		return fmt.Errorf("invalid transaction type")
	}
	if err := sqlTx.Rollback(); err != nil {
		return fmt.Errorf("failed to rollback transaction: %w", err)
	}
	return nil
}

// FindDimensions retrieves all health dimensions
func (r *OrganizationRepository) FindDimensions(ctx context.Context) ([]*organization.HealthDimension, error) {
	rows, err := r.queries.FindDimensions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query health dimensions: %w", err)
	}

	dimensions := make([]*organization.HealthDimension, 0, len(rows))
	for _, row := range rows {
		dimensions = append(dimensions, buildDimensionFromRow(row))
	}

	return dimensions, nil
}

// FindDimensionByID retrieves a specific health dimension by ID
func (r *OrganizationRepository) FindDimensionByID(ctx context.Context, id string) (*organization.HealthDimension, error) {
	row, err := r.queries.FindDimensionByID(ctx, id)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("health dimension not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find health dimension: %w", err)
	}

	return buildDimensionFromRow(row), nil
}

// SaveDimension persists a new health dimension
func (r *OrganizationRepository) SaveDimension(ctx context.Context, dim *organization.HealthDimension) error {
	now := time.Now()
	if dim.CreatedAt.IsZero() {
		dim.CreatedAt = now
	}
	dim.UpdatedAt = now

	err := r.queries.InsertDimension(ctx, sqlcgen.InsertDimensionParams{
		ID:              dim.ID,
		Name:            dim.Name,
		Description:     dim.Description,
		GoodDescription: dim.GoodDescription,
		BadDescription:  dim.BadDescription,
		IsActive:        sql.NullBool{Bool: dim.IsActive, Valid: true},
		Weight:          sql.NullFloat64{Float64: dim.Weight, Valid: true},
		CreatedAt:       sql.NullTime{Time: dim.CreatedAt, Valid: true},
		UpdatedAt:       sql.NullTime{Time: dim.UpdatedAt, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("failed to save health dimension: %w", err)
	}

	return nil
}

// UpdateDimension updates an existing health dimension
func (r *OrganizationRepository) UpdateDimension(ctx context.Context, dim *organization.HealthDimension) error {
	dim.UpdatedAt = time.Now()

	rowsAffected, err := r.queries.UpdateDimension(ctx, sqlcgen.UpdateDimensionParams{
		Name:            dim.Name,
		Description:     dim.Description,
		GoodDescription: dim.GoodDescription,
		BadDescription:  dim.BadDescription,
		IsActive:        sql.NullBool{Bool: dim.IsActive, Valid: true},
		Weight:          sql.NullFloat64{Float64: dim.Weight, Valid: true},
		UpdatedAt:       sql.NullTime{Time: dim.UpdatedAt, Valid: true},
		ID:              dim.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to update health dimension: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("health dimension not found: %s", dim.ID)
	}

	return nil
}

// DeleteDimension soft-deletes a health dimension by setting is_active=false.
// This preserves historical survey data that references the dimension.
func (r *OrganizationRepository) DeleteDimension(ctx context.Context, id string) error {
	rowsAffected, err := r.queries.DeactivateDimension(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to deactivate health dimension: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("health dimension not found or already inactive: %s", id)
	}

	return nil
}

// GetAppSettings reads the singleton app_settings row
func (r *OrganizationRepository) GetAppSettings(ctx context.Context) (*organization.AppSettings, error) {
	row, err := r.queries.GetAppSettings(ctx)
	if err == sql.ErrNoRows {
		// Return defaults if row doesn't exist yet
		return &organization.AppSettings{RetentionMonths: 12, CompanyName: "My Company"}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query app settings: %w", err)
	}

	s := &organization.AppSettings{
		EmailNotifications: row.EmailNotifications,
		SlackNotifications: row.SlackNotifications,
		WeeklyDigest:       row.WeeklyDigest,
		RetentionMonths:    int(row.RetentionMonths),
		CompanyName:        row.CompanyName,
	}
	if row.LogoUrl.Valid {
		s.LogoURL = row.LogoUrl.String
	}
	if row.OrgSyncMaxDeletePercent.Valid {
		value := row.OrgSyncMaxDeletePercent.Float64
		s.OrgSyncMaxDeletePercent = &value
	}

	return s, nil
}

// UpdateAppSettings persists app settings to the singleton row
func (r *OrganizationRepository) UpdateAppSettings(ctx context.Context, s *organization.AppSettings) error {
	var logoURL sql.NullString
	if s.LogoURL != "" {
		logoURL = sql.NullString{String: s.LogoURL, Valid: true}
	}

	err := r.queries.UpsertAppSettings(ctx, sqlcgen.UpsertAppSettingsParams{
		EmailNotifications: s.EmailNotifications,
		SlackNotifications: s.SlackNotifications,
		WeeklyDigest:       s.WeeklyDigest,
		RetentionMonths:    int32(s.RetentionMonths),
		CompanyName:        s.CompanyName,
		LogoUrl:            logoURL,
	})
	if err != nil {
		return fmt.Errorf("failed to update app settings: %w", err)
	}
	return nil
}

// UpdateBrandingSettings updates only the branding columns (company_name, logo_url)
func (r *OrganizationRepository) UpdateBrandingSettings(ctx context.Context, companyName string, logoURL string) error {
	var logo sql.NullString
	if logoURL != "" {
		logo = sql.NullString{String: logoURL, Valid: true}
	}

	err := r.queries.UpsertBrandingSettings(ctx, sqlcgen.UpsertBrandingSettingsParams{
		CompanyName: companyName,
		LogoUrl:     logo,
	})
	if err != nil {
		return fmt.Errorf("failed to update branding settings: %w", err)
	}
	return nil
}

// UpdateNotificationSettings updates only the notification columns
func (r *OrganizationRepository) UpdateNotificationSettings(ctx context.Context, email, slack, digest bool) error {
	err := r.queries.UpsertNotificationSettings(ctx, sqlcgen.UpsertNotificationSettingsParams{
		EmailNotifications: email,
		SlackNotifications: slack,
		WeeklyDigest:       digest,
	})
	if err != nil {
		return fmt.Errorf("failed to update notification settings: %w", err)
	}
	return nil
}

// UpdateRetentionSettings updates only the retention_months column
func (r *OrganizationRepository) UpdateRetentionSettings(ctx context.Context, months int) error {
	err := r.queries.UpsertRetentionSettings(ctx, int32(months))
	if err != nil {
		return fmt.Errorf("failed to update retention settings: %w", err)
	}
	return nil
}

// GetOrgSyncMaxDeletePercent reads the administrator-configured mass-deletion
// threshold for the organization sync. A nil result means no administrator has
// saved one, which leaves the environment-variable fallback in charge.
func (r *OrganizationRepository) GetOrgSyncMaxDeletePercent(ctx context.Context) (*float64, error) {
	percent, err := r.queries.GetOrgSyncMaxDeletePercent(ctx)
	if err != nil {
		if err == sql.ErrNoRows {
			// No settings row yet is "not configured", not a failure.
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query organization sync delete threshold: %w", err)
	}
	if !percent.Valid {
		return nil, nil
	}
	value := percent.Float64
	return &value, nil
}

// UpdateOrgSyncMaxDeletePercent persists the mass-deletion threshold. The
// caller is responsible for validating the range; the column carries a
// matching CHECK constraint so an out-of-range write is rejected by the
// database as well.
func (r *OrganizationRepository) UpdateOrgSyncMaxDeletePercent(ctx context.Context, percent float64) error {
	if err := r.queries.UpsertOrgSyncMaxDeletePercent(ctx, sql.NullFloat64{Float64: percent, Valid: true}); err != nil {
		return fmt.Errorf("failed to update organization sync delete threshold: %w", err)
	}
	return nil
}

// Helper methods

// saveHierarchyLevelTx saves (upserts) a hierarchy level within a transaction
func (r *OrganizationRepository) saveHierarchyLevelTx(ctx context.Context, tx *sql.Tx, level *organization.HierarchyLevel) error {
	var color sql.NullString
	if level.Color != "" {
		color = sql.NullString{String: level.Color, Valid: true}
	}

	now := time.Now()
	if level.CreatedAt.IsZero() {
		level.CreatedAt = now
	}
	level.UpdatedAt = now

	err := r.queries.WithTx(tx).UpsertHierarchyLevel(ctx, sqlcgen.UpsertHierarchyLevelParams{
		ID:                 level.ID,
		Name:               level.Name,
		Position:           int32(level.Position),
		Color:              color,
		CanViewAllTeams:    sql.NullBool{Bool: level.Permissions.CanViewAllTeams, Valid: true},
		CanEditTeams:       sql.NullBool{Bool: level.Permissions.CanEditTeams, Valid: true},
		CanManageUsers:     sql.NullBool{Bool: level.Permissions.CanManageUsers, Valid: true},
		CanTakeSurvey:      sql.NullBool{Bool: level.Permissions.CanTakeSurvey, Valid: true},
		CanViewAnalytics:   sql.NullBool{Bool: level.Permissions.CanViewAnalytics, Valid: true},
		CanConfigureSystem: sql.NullBool{Bool: level.Permissions.CanConfigureSystem, Valid: true},
		CanViewReports:     sql.NullBool{Bool: level.Permissions.CanViewReports, Valid: true},
		CanExportData:      sql.NullBool{Bool: level.Permissions.CanExportData, Valid: true},
		CreatedAt:          sql.NullTime{Time: level.CreatedAt, Valid: true},
		UpdatedAt:          sql.NullTime{Time: level.UpdatedAt, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("failed to save hierarchy level: %w", err)
	}

	return nil
}
