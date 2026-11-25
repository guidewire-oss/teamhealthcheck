-- Queries for OrganizationRepository (backend/infrastructure/persistence/postgres/organization_repository.go)
-- Moved verbatim from the hand-written SQL that lived inline in that file.
-- Do not hand-edit sqlcgen/*.go - regenerate with `sqlc generate` (see Makefile).

-- name: FindHierarchyLevels :many
SELECT id, name, position, color,
       can_view_all_teams, can_edit_teams, can_manage_users,
       can_take_survey, can_view_analytics,
       can_configure_system, can_view_reports, can_export_data,
       created_at, updated_at
FROM hierarchy_levels
ORDER BY position;

-- name: FindHierarchyLevelByID :one
SELECT id, name, position, color,
       can_view_all_teams, can_edit_teams, can_manage_users,
       can_take_survey, can_view_analytics,
       can_configure_system, can_view_reports, can_export_data,
       created_at, updated_at
FROM hierarchy_levels
WHERE id = sqlc.arg(id);

-- name: InsertHierarchyLevel :exec
INSERT INTO hierarchy_levels (
	id, name, position, color,
	can_view_all_teams, can_edit_teams, can_manage_users,
	can_take_survey, can_view_analytics,
	can_configure_system, can_view_reports, can_export_data,
	created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14);

-- name: UpsertHierarchyLevel :exec
-- Used by OrganizationRepository.Save (via saveHierarchyLevelTx) within a transaction.
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
	updated_at = EXCLUDED.updated_at;

-- name: UpdateHierarchyLevel :execrows
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
WHERE id = $13;

-- name: DeleteHierarchyLevel :execrows
DELETE FROM hierarchy_levels WHERE id = sqlc.arg(id);

-- name: GetMaxHierarchyPosition :one
-- COALESCE to 0 (matching the original hand-written behavior of returning 0
-- when the table is empty) keeps the result NOT NULL so sqlc generates a
-- plain int32 return instead of a nullable type that could panic on scan.
SELECT COALESCE(MAX(position), 0)::int AS max_position
FROM hierarchy_levels;

-- name: UpdateHierarchyPositionQuery :exec
UPDATE hierarchy_levels
SET position = sqlc.arg(new_position), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: ShiftHierarchyPositionsQuery :exec
UPDATE hierarchy_levels
SET position = position + sqlc.arg(delta), updated_at = sqlc.arg(updated_at)
WHERE position >= sqlc.arg(start_position) AND position <= sqlc.arg(end_position);

-- name: CountUsersAtLevel :one
SELECT COUNT(*) FROM users WHERE hierarchy_level_id = sqlc.arg(hierarchy_level_id);

-- name: FindDimensions :many
SELECT id, name, description, good_description, bad_description, is_active, weight,
       created_at, updated_at, display_order
FROM health_dimensions
ORDER BY display_order, id;

-- name: FindDimensionByID :one
SELECT id, name, description, good_description, bad_description, is_active, weight,
       created_at, updated_at, display_order
FROM health_dimensions
WHERE id = sqlc.arg(id);

-- name: InsertDimension :exec
INSERT INTO health_dimensions (id, name, description, good_description, bad_description, is_active, weight, display_order, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, (SELECT COALESCE(MAX(display_order), 0) + 1 FROM health_dimensions), $8, $9);

-- name: UpdateDimension :execrows
UPDATE health_dimensions SET
	name = $1,
	description = $2,
	good_description = $3,
	bad_description = $4,
	is_active = $5,
	weight = $6,
	updated_at = $7
WHERE id = $8;

-- name: DeactivateDimension :execrows
-- Soft-delete: sets is_active=false rather than removing the row, so
-- historical survey data that references the dimension stays intact.
UPDATE health_dimensions
SET is_active = false, updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id) AND is_active = true;

-- name: GetAppSettings :one
SELECT email_notifications, slack_notifications, weekly_digest, retention_months, company_name, logo_url,
       org_sync_max_delete_percent
FROM app_settings WHERE id = 1;

-- name: UpsertAppSettings :exec
INSERT INTO app_settings (id, email_notifications, slack_notifications, weekly_digest, retention_months, company_name, logo_url, updated_at)
VALUES (1, $1, $2, $3, $4, $5, $6, NOW())
ON CONFLICT (id) DO UPDATE SET
	email_notifications = EXCLUDED.email_notifications,
	slack_notifications = EXCLUDED.slack_notifications,
	weekly_digest = EXCLUDED.weekly_digest,
	retention_months = EXCLUDED.retention_months,
	company_name = EXCLUDED.company_name,
	logo_url = EXCLUDED.logo_url,
	updated_at = NOW();

-- name: UpsertBrandingSettings :exec
INSERT INTO app_settings (id, company_name, logo_url, updated_at)
VALUES (1, $1, $2, NOW())
ON CONFLICT (id) DO UPDATE SET
	company_name = EXCLUDED.company_name,
	logo_url = EXCLUDED.logo_url,
	updated_at = NOW();

-- name: UpsertNotificationSettings :exec
INSERT INTO app_settings (id, email_notifications, slack_notifications, weekly_digest, updated_at)
VALUES (1, $1, $2, $3, NOW())
ON CONFLICT (id) DO UPDATE SET
	email_notifications = EXCLUDED.email_notifications,
	slack_notifications = EXCLUDED.slack_notifications,
	weekly_digest = EXCLUDED.weekly_digest,
	updated_at = NOW();

-- name: UpsertRetentionSettings :exec
INSERT INTO app_settings (id, retention_months, updated_at)
VALUES (1, $1, NOW())
ON CONFLICT (id) DO UPDATE SET
	retention_months = EXCLUDED.retention_months,
	updated_at = NOW();

-- name: GetOrgSyncMaxDeletePercent :one
SELECT org_sync_max_delete_percent FROM app_settings WHERE id = 1;

-- name: UpsertOrgSyncMaxDeletePercent :exec
INSERT INTO app_settings (id, org_sync_max_delete_percent, updated_at)
VALUES (1, $1, NOW())
ON CONFLICT (id) DO UPDATE SET
	org_sync_max_delete_percent = EXCLUDED.org_sync_max_delete_percent,
	updated_at = NOW();
