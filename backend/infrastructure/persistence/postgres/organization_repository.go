package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/agopalakrishnan/teams360/backend/domain/organization"
)

// OrganizationRepository implements the organization.Repository interface
type OrganizationRepository struct {
	db *sql.DB
}

// NewOrganizationRepository creates a new repository instance
func NewOrganizationRepository(db *sql.DB) organization.Repository {
	return &OrganizationRepository{db: db}
}

// Get retrieves the organization configuration
func (r *OrganizationRepository) Get(ctx context.Context) (*organization.OrganizationConfig, error) {
	var config organization.OrganizationConfig

	config.ID = "default"

	// Load company name from app_settings (not hardcoded)
	appSettings, err := r.GetAppSettings(ctx)
	if err == nil && appSettings.CompanyName != "" {
		config.CompanyName = appSettings.CompanyName
	} else {
		config.CompanyName = "Team360" // fallback default
	}

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

	// Derive team member level from the lowest-position hierarchy level
	if len(levels) > 0 {
		lowest := levels[0]
		for _, l := range levels[1:] {
			if l.Position > lowest.Position {
				lowest = l
			}
		}
		config.TeamMemberLevelID = lowest.ID
	} else {
		config.TeamMemberLevelID = "level-5" // fallback default
	}

	return &config, nil
}

// Save persists the organization configuration
func (r *OrganizationRepository) Save(ctx context.Context, config *organization.OrganizationConfig) error {
	// Begin transaction
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Save hierarchy levels
	for i := range config.HierarchyLevels {
		err = r.saveHierarchyLevelTx(ctx, tx, &config.HierarchyLevels[i])
		if err != nil {
			return err
		}
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// FindHierarchyLevels retrieves all hierarchy levels
func (r *OrganizationRepository) FindHierarchyLevels(ctx context.Context) ([]*organization.HierarchyLevel, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, position, color,
		       can_view_all_teams, can_edit_teams, can_manage_users,
		       can_take_survey, can_view_analytics,
		       can_configure_system, can_view_reports, can_export_data,
		       created_at, updated_at
		FROM hierarchy_levels
		ORDER BY position
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query hierarchy levels: %w", err)
	}
	defer rows.Close()

	var levels []*organization.HierarchyLevel

	for rows.Next() {
		var level organization.HierarchyLevel
		var color sql.NullString
		var canConfigureSystem, canViewReports, canExportData sql.NullBool
		var createdAt, updatedAt sql.NullTime

		err := rows.Scan(
			&level.ID,
			&level.Name,
			&level.Position,
			&color,
			&level.Permissions.CanViewAllTeams,
			&level.Permissions.CanEditTeams,
			&level.Permissions.CanManageUsers,
			&level.Permissions.CanTakeSurvey,
			&level.Permissions.CanViewAnalytics,
			&canConfigureSystem,
			&canViewReports,
			&canExportData,
			&createdAt,
			&updatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan hierarchy level: %w", err)
		}

		// Handle NULL fields
		if color.Valid {
			level.Color = color.String
		}
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

		levels = append(levels, &level)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	// Return empty slice instead of nil
	if levels == nil {
		levels = []*organization.HierarchyLevel{}
	}

	return levels, nil
}

// FindHierarchyLevelByID retrieves a specific hierarchy level by ID
func (r *OrganizationRepository) FindHierarchyLevelByID(ctx context.Context, id string) (*organization.HierarchyLevel, error) {
	var level organization.HierarchyLevel
	var color sql.NullString
	var canConfigureSystem, canViewReports, canExportData sql.NullBool
	var createdAt, updatedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, `
		SELECT id, name, position, color,
		       can_view_all_teams, can_edit_teams, can_manage_users,
		       can_take_survey, can_view_analytics,
		       can_configure_system, can_view_reports, can_export_data,
		       created_at, updated_at
		FROM hierarchy_levels
		WHERE id = $1
	`, id).Scan(
		&level.ID,
		&level.Name,
		&level.Position,
		&color,
		&level.Permissions.CanViewAllTeams,
		&level.Permissions.CanEditTeams,
		&level.Permissions.CanManageUsers,
		&level.Permissions.CanTakeSurvey,
		&level.Permissions.CanViewAnalytics,
		&canConfigureSystem,
		&canViewReports,
		&canExportData,
		&createdAt,
		&updatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("hierarchy level not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find hierarchy level: %w", err)
	}

	// Handle NULL fields
	if color.Valid {
		level.Color = color.String
	}
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

	return &level, nil
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

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO hierarchy_levels (
			id, name, position, color,
			can_view_all_teams, can_edit_teams, can_manage_users,
			can_take_survey, can_view_analytics,
			can_configure_system, can_view_reports, can_export_data,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`,
		level.ID,
		level.Name,
		level.Position,
		color,
		level.Permissions.CanViewAllTeams,
		level.Permissions.CanEditTeams,
		level.Permissions.CanManageUsers,
		level.Permissions.CanTakeSurvey,
		level.Permissions.CanViewAnalytics,
		level.Permissions.CanConfigureSystem,
		level.Permissions.CanViewReports,
		level.Permissions.CanExportData,
		level.CreatedAt,
		level.UpdatedAt,
	)

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

	// Update timestamp
	level.UpdatedAt = time.Now()

	result, err := r.db.ExecContext(ctx, `
		UPDATE hierarchy_levels SET
			name = $1,
			position = $2,
			color = $3,
			can_view_all_teams = $4,
			can_edit_teams = $5,
			can_manage_users = $6,
			can_take_survey = $7,
			can_view_analytics = $8,
			can_configure_system = $9,
			can_view_reports = $10,
			can_export_data = $11,
			updated_at = $12
		WHERE id = $13
	`,
		level.Name,
		level.Position,
		color,
		level.Permissions.CanViewAllTeams,
		level.Permissions.CanEditTeams,
		level.Permissions.CanManageUsers,
		level.Permissions.CanTakeSurvey,
		level.Permissions.CanViewAnalytics,
		level.Permissions.CanConfigureSystem,
		level.Permissions.CanViewReports,
		level.Permissions.CanExportData,
		level.UpdatedAt,
		level.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update hierarchy level: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("hierarchy level not found: %s", level.ID)
	}

	return nil
}

// DeleteHierarchyLevel removes a hierarchy level
func (r *OrganizationRepository) DeleteHierarchyLevel(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM hierarchy_levels WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("failed to delete hierarchy level: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("hierarchy level not found: %s", id)
	}

	return nil
}

// GetMaxHierarchyPosition returns the maximum position value
func (r *OrganizationRepository) GetMaxHierarchyPosition(ctx context.Context) (int, error) {
	var maxPosition sql.NullInt64
	err := r.db.QueryRowContext(ctx, `
		SELECT MAX(position)
		FROM hierarchy_levels
	`).Scan(&maxPosition)

	if err != nil {
		return 0, fmt.Errorf("failed to get max position: %w", err)
	}

	if !maxPosition.Valid {
		return 0, nil
	}

	return int(maxPosition.Int64), nil
}

// UpdateHierarchyPosition updates a hierarchy level's position
func (r *OrganizationRepository) UpdateHierarchyPosition(ctx context.Context, tx interface{}, id string, newPosition int) error {
	var err error

	if tx != nil {
		// Use provided transaction
		sqlTx, ok := tx.(*sql.Tx)
		if !ok {
			return fmt.Errorf("invalid transaction type")
		}
		_, err = sqlTx.ExecContext(ctx, `
			UPDATE hierarchy_levels
			SET position = $1, updated_at = $2
			WHERE id = $3
		`, newPosition, time.Now(), id)
	} else {
		// Use regular connection
		_, err = r.db.ExecContext(ctx, `
			UPDATE hierarchy_levels
			SET position = $1, updated_at = $2
			WHERE id = $3
		`, newPosition, time.Now(), id)
	}

	if err != nil {
		return fmt.Errorf("failed to update hierarchy position: %w", err)
	}

	return nil
}

// ShiftHierarchyPositions shifts positions in a range
func (r *OrganizationRepository) ShiftHierarchyPositions(ctx context.Context, tx interface{}, start, end int, delta int) error {
	var err error

	if tx != nil {
		// Use provided transaction
		sqlTx, ok := tx.(*sql.Tx)
		if !ok {
			return fmt.Errorf("invalid transaction type")
		}
		_, err = sqlTx.ExecContext(ctx, `
			UPDATE hierarchy_levels
			SET position = position + $1, updated_at = $2
			WHERE position >= $3 AND position <= $4
		`, delta, time.Now(), start, end)
	} else {
		// Use regular connection
		_, err = r.db.ExecContext(ctx, `
			UPDATE hierarchy_levels
			SET position = position + $1, updated_at = $2
			WHERE position >= $3 AND position <= $4
		`, delta, time.Now(), start, end)
	}

	if err != nil {
		return fmt.Errorf("failed to shift hierarchy positions: %w", err)
	}

	return nil
}

// CountUsersAtLevel counts users at a specific hierarchy level
func (r *OrganizationRepository) CountUsersAtLevel(ctx context.Context, levelID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM users
		WHERE hierarchy_level_id = $1
	`, levelID).Scan(&count)

	if err != nil {
		return 0, fmt.Errorf("failed to count users at level: %w", err)
	}

	return count, nil
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
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, description, good_description, bad_description, is_active, weight,
		       created_at, updated_at
		FROM health_dimensions
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query health dimensions: %w", err)
	}
	defer rows.Close()

	var dimensions []*organization.HealthDimension

	for rows.Next() {
		var dimension organization.HealthDimension
		var description sql.NullString
		var isActive sql.NullBool
		var weight sql.NullFloat64
		var createdAt, updatedAt sql.NullTime

		err := rows.Scan(
			&dimension.ID,
			&dimension.Name,
			&description,
			&dimension.GoodDescription,
			&dimension.BadDescription,
			&isActive,
			&weight,
			&createdAt,
			&updatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan health dimension: %w", err)
		}

		// Handle NULL fields
		if description.Valid {
			dimension.Description = description.String
		}
		if isActive.Valid {
			dimension.IsActive = isActive.Bool
		} else {
			dimension.IsActive = true // default
		}
		if weight.Valid {
			dimension.Weight = weight.Float64
		} else {
			dimension.Weight = 1.0 // default
		}
		if createdAt.Valid {
			dimension.CreatedAt = createdAt.Time
		}
		if updatedAt.Valid {
			dimension.UpdatedAt = updatedAt.Time
		}

		dimensions = append(dimensions, &dimension)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	// Return empty slice instead of nil
	if dimensions == nil {
		dimensions = []*organization.HealthDimension{}
	}

	return dimensions, nil
}

// FindDimensionByID retrieves a specific health dimension by ID
func (r *OrganizationRepository) FindDimensionByID(ctx context.Context, id string) (*organization.HealthDimension, error) {
	var dimension organization.HealthDimension
	var description sql.NullString
	var isActive sql.NullBool
	var weight sql.NullFloat64
	var createdAt, updatedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, `
		SELECT id, name, description, good_description, bad_description, is_active, weight,
		       created_at, updated_at
		FROM health_dimensions
		WHERE id = $1
	`, id).Scan(
		&dimension.ID,
		&dimension.Name,
		&description,
		&dimension.GoodDescription,
		&dimension.BadDescription,
		&isActive,
		&weight,
		&createdAt,
		&updatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("health dimension not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find health dimension: %w", err)
	}

	// Handle NULL fields
	if description.Valid {
		dimension.Description = description.String
	}
	if isActive.Valid {
		dimension.IsActive = isActive.Bool
	} else {
		dimension.IsActive = true
	}
	if weight.Valid {
		dimension.Weight = weight.Float64
	} else {
		dimension.Weight = 1.0
	}
	if createdAt.Valid {
		dimension.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		dimension.UpdatedAt = updatedAt.Time
	}

	return &dimension, nil
}

// SaveDimension persists a new health dimension
func (r *OrganizationRepository) SaveDimension(ctx context.Context, dim *organization.HealthDimension) error {
	var description sql.NullString
	if dim.Description != "" {
		description = sql.NullString{String: dim.Description, Valid: true}
	}

	// Set timestamps
	now := time.Now()
	if dim.CreatedAt.IsZero() {
		dim.CreatedAt = now
	}
	dim.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO health_dimensions (id, name, description, good_description, bad_description, is_active, weight, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`,
		dim.ID,
		dim.Name,
		description,
		dim.GoodDescription,
		dim.BadDescription,
		dim.IsActive,
		dim.Weight,
		dim.CreatedAt,
		dim.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to save health dimension: %w", err)
	}

	return nil
}

// UpdateDimension updates an existing health dimension
func (r *OrganizationRepository) UpdateDimension(ctx context.Context, dim *organization.HealthDimension) error {
	var description sql.NullString
	if dim.Description != "" {
		description = sql.NullString{String: dim.Description, Valid: true}
	}

	// Update timestamp
	dim.UpdatedAt = time.Now()

	result, err := r.db.ExecContext(ctx, `
		UPDATE health_dimensions SET
			name = $1,
			description = $2,
			good_description = $3,
			bad_description = $4,
			is_active = $5,
			weight = $6,
			updated_at = $7
		WHERE id = $8
	`,
		dim.Name,
		description,
		dim.GoodDescription,
		dim.BadDescription,
		dim.IsActive,
		dim.Weight,
		dim.UpdatedAt,
		dim.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update health dimension: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("health dimension not found: %s", dim.ID)
	}

	return nil
}

// DeleteDimension soft-deletes a health dimension by setting is_active=false.
// This preserves historical survey data that references the dimension.
func (r *OrganizationRepository) DeleteDimension(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE health_dimensions
		SET is_active = false, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND is_active = true
	`, id)
	if err != nil {
		return fmt.Errorf("failed to deactivate health dimension: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("health dimension not found or already inactive: %s", id)
	}

	return nil
}

// GetAppSettings reads the singleton app_settings row
func (r *OrganizationRepository) GetAppSettings(ctx context.Context) (*organization.AppSettings, error) {
	var s organization.AppSettings
	var logoURL sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT email_notifications, slack_notifications, weekly_digest, retention_months, company_name, logo_url
		FROM app_settings WHERE id = 1
	`).Scan(&s.EmailNotifications, &s.SlackNotifications, &s.WeeklyDigest, &s.RetentionMonths, &s.CompanyName, &logoURL)
	if err != nil {
		if err == sql.ErrNoRows {
			// Return defaults if row doesn't exist yet
			return &organization.AppSettings{RetentionMonths: 12, CompanyName: "My Company"}, nil
		}
		return nil, fmt.Errorf("failed to query app settings: %w", err)
	}
	if logoURL.Valid {
		s.LogoURL = logoURL.String
	}
	return &s, nil
}

// UpdateAppSettings persists app settings to the singleton row
func (r *OrganizationRepository) UpdateAppSettings(ctx context.Context, s *organization.AppSettings) error {
	var logoURL *string
	if s.LogoURL != "" {
		logoURL = &s.LogoURL
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO app_settings (id, email_notifications, slack_notifications, weekly_digest, retention_months, company_name, logo_url, updated_at)
		VALUES (1, $1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (id) DO UPDATE SET
			email_notifications = EXCLUDED.email_notifications,
			slack_notifications = EXCLUDED.slack_notifications,
			weekly_digest = EXCLUDED.weekly_digest,
			retention_months = EXCLUDED.retention_months,
			company_name = EXCLUDED.company_name,
			logo_url = EXCLUDED.logo_url,
			updated_at = NOW()
	`, s.EmailNotifications, s.SlackNotifications, s.WeeklyDigest, s.RetentionMonths, s.CompanyName, logoURL)
	if err != nil {
		return fmt.Errorf("failed to update app settings: %w", err)
	}
	return nil
}

// UpdateBrandingSettings updates only the branding columns (company_name, logo_url)
func (r *OrganizationRepository) UpdateBrandingSettings(ctx context.Context, companyName string, logoURL string) error {
	var logo *string
	if logoURL != "" {
		logo = &logoURL
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO app_settings (id, company_name, logo_url, updated_at)
		VALUES (1, $1, $2, NOW())
		ON CONFLICT (id) DO UPDATE SET
			company_name = EXCLUDED.company_name,
			logo_url = EXCLUDED.logo_url,
			updated_at = NOW()
	`, companyName, logo)
	if err != nil {
		return fmt.Errorf("failed to update branding settings: %w", err)
	}
	return nil
}

// UpdateNotificationSettings updates only the notification columns
func (r *OrganizationRepository) UpdateNotificationSettings(ctx context.Context, email, slack, digest bool) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO app_settings (id, email_notifications, slack_notifications, weekly_digest, updated_at)
		VALUES (1, $1, $2, $3, NOW())
		ON CONFLICT (id) DO UPDATE SET
			email_notifications = EXCLUDED.email_notifications,
			slack_notifications = EXCLUDED.slack_notifications,
			weekly_digest = EXCLUDED.weekly_digest,
			updated_at = NOW()
	`, email, slack, digest)
	if err != nil {
		return fmt.Errorf("failed to update notification settings: %w", err)
	}
	return nil
}

// UpdateRetentionSettings updates only the retention_months column
func (r *OrganizationRepository) UpdateRetentionSettings(ctx context.Context, months int) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO app_settings (id, retention_months, updated_at)
		VALUES (1, $1, NOW())
		ON CONFLICT (id) DO UPDATE SET
			retention_months = EXCLUDED.retention_months,
			updated_at = NOW()
	`, months)
	if err != nil {
		return fmt.Errorf("failed to update retention settings: %w", err)
	}
	return nil
}

// Helper methods

// saveHierarchyLevelTx saves a hierarchy level within a transaction
func (r *OrganizationRepository) saveHierarchyLevelTx(ctx context.Context, tx *sql.Tx, level *organization.HierarchyLevel) error {
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

	_, err := tx.ExecContext(ctx, `
		INSERT INTO hierarchy_levels (
			id, name, position, color,
			can_view_all_teams, can_edit_teams, can_manage_users,
			can_take_survey, can_view_analytics,
			can_configure_system, can_view_reports, can_export_data,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			position = EXCLUDED.position,
			color = EXCLUDED.color,
			can_view_all_teams = EXCLUDED.can_view_all_teams,
			can_edit_teams = EXCLUDED.can_edit_teams,
			can_manage_users = EXCLUDED.can_manage_users,
			can_take_survey = EXCLUDED.can_take_survey,
			can_view_analytics = EXCLUDED.can_view_analytics,
			can_configure_system = EXCLUDED.can_configure_system,
			can_view_reports = EXCLUDED.can_view_reports,
			can_export_data = EXCLUDED.can_export_data,
			updated_at = EXCLUDED.updated_at
	`,
		level.ID,
		level.Name,
		level.Position,
		color,
		level.Permissions.CanViewAllTeams,
		level.Permissions.CanEditTeams,
		level.Permissions.CanManageUsers,
		level.Permissions.CanTakeSurvey,
		level.Permissions.CanViewAnalytics,
		level.Permissions.CanConfigureSystem,
		level.Permissions.CanViewReports,
		level.Permissions.CanExportData,
		level.CreatedAt,
		level.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to save hierarchy level: %w", err)
	}

	return nil
}
