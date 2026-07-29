// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/agopalakrishnan/teams360/backend/domain/user"
	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

// UserRepository implements the user.Repository interface
type UserRepository struct {
	db *sql.DB
}

// NewUserRepository creates a new repository instance
func NewUserRepository(db *sql.DB) user.Repository {
	return &UserRepository{db: db}
}

// FindByID retrieves a user by ID
func (r *UserRepository) FindByID(ctx context.Context, id string) (*user.User, error) {
	var u user.User
	var email, hierarchyLevelID, reportsTo sql.NullString
	var createdAt, updatedAt sql.NullTime
	var passwordHash, authType sql.NullString

	err := r.db.QueryRowContext(ctx, `
		SELECT id, username, full_name, email, hierarchy_level_id, reports_to,
		       password_hash, auth_type, created_at, updated_at
		FROM users
		WHERE id = $1
	`, id).Scan(
		&u.ID,
		&u.Username,
		&u.Name,
		&email,
		&hierarchyLevelID,
		&reportsTo,
		&passwordHash,
		&authType,
		&createdAt,
		&updatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found with provided ID")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find user: %w", err)
	}

	// Handle NULL fields
	if email.Valid {
		u.Email = email.String
	}
	if hierarchyLevelID.Valid {
		u.HierarchyLevelID = hierarchyLevelID.String
	}
	if reportsTo.Valid {
		u.ReportsTo = &reportsTo.String
	}
	if passwordHash.Valid {
		u.PasswordHash = passwordHash.String
	}
	if authType.Valid {
		u.AuthType = user.AuthType(authType.String)
	}
	if u.AuthType == "" {
		u.AuthType = user.AuthTypeLocal
	}
	if createdAt.Valid {
		u.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		u.UpdatedAt = updatedAt.Time
	}

	// Fetch team IDs
	teamIDs, err := r.fetchTeamIDs(ctx, id)
	if err != nil {
		return nil, err
	}
	u.TeamIDs = teamIDs

	// Check if user is admin (username = 'admin')
	u.IsAdmin = u.Username == "admin"

	return &u, nil
}

// FindByUsername retrieves a user by username (used for authentication)
func (r *UserRepository) FindByUsername(ctx context.Context, username string) (*user.User, error) {
	var u user.User
	var email, hierarchyLevelID, reportsTo sql.NullString
	var createdAt, updatedAt sql.NullTime
	var passwordHash, authType sql.NullString

	err := r.db.QueryRowContext(ctx, `
		SELECT id, username, full_name, email, hierarchy_level_id, reports_to,
		       password_hash, auth_type, created_at, updated_at
		FROM users
		WHERE username = $1
	`, username).Scan(
		&u.ID,
		&u.Username,
		&u.Name,
		&email,
		&hierarchyLevelID,
		&reportsTo,
		&passwordHash,
		&authType,
		&createdAt,
		&updatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found with provided username")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find user: %w", err)
	}

	// Handle NULL fields
	if email.Valid {
		u.Email = email.String
	}
	if hierarchyLevelID.Valid {
		u.HierarchyLevelID = hierarchyLevelID.String
	}
	if reportsTo.Valid {
		u.ReportsTo = &reportsTo.String
	}
	if passwordHash.Valid {
		u.PasswordHash = passwordHash.String
	}
	if authType.Valid {
		u.AuthType = user.AuthType(authType.String)
	}
	if u.AuthType == "" {
		u.AuthType = user.AuthTypeLocal
	}
	if createdAt.Valid {
		u.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		u.UpdatedAt = updatedAt.Time
	}

	// Fetch team IDs
	teamIDs, err := r.fetchTeamIDs(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	u.TeamIDs = teamIDs

	// Check if user is admin
	u.IsAdmin = u.Username == "admin"

	return &u, nil
}

// FindByEmail retrieves a user by email
func (r *UserRepository) FindByEmail(ctx context.Context, emailParam string) (*user.User, error) {
	var u user.User
	var emailVal, hierarchyLevelID, reportsTo sql.NullString
	var createdAt, updatedAt sql.NullTime
	var passwordHash, authType sql.NullString

	err := r.db.QueryRowContext(ctx, `
		SELECT id, username, full_name, email, hierarchy_level_id, reports_to,
		       password_hash, auth_type, created_at, updated_at
		FROM users
		WHERE email = $1
	`, emailParam).Scan(
		&u.ID,
		&u.Username,
		&u.Name,
		&emailVal,
		&hierarchyLevelID,
		&reportsTo,
		&passwordHash,
		&authType,
		&createdAt,
		&updatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found with provided email")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find user: %w", err)
	}

	// Handle NULL fields
	if emailVal.Valid {
		u.Email = emailVal.String
	}
	if hierarchyLevelID.Valid {
		u.HierarchyLevelID = hierarchyLevelID.String
	}
	if reportsTo.Valid {
		u.ReportsTo = &reportsTo.String
	}
	if passwordHash.Valid {
		u.PasswordHash = passwordHash.String
	}
	if authType.Valid {
		u.AuthType = user.AuthType(authType.String)
	}
	if u.AuthType == "" {
		u.AuthType = user.AuthTypeLocal
	}
	if createdAt.Valid {
		u.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		u.UpdatedAt = updatedAt.Time
	}

	// Fetch team IDs
	teamIDs, err := r.fetchTeamIDs(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	u.TeamIDs = teamIDs

	// Check if user is admin
	u.IsAdmin = u.Username == "admin"

	return &u, nil
}

// FindAll retrieves all users
func (r *UserRepository) FindAll(ctx context.Context) ([]*user.User, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, username, full_name, email, hierarchy_level_id, reports_to,
		       password_hash, auth_type, created_at, updated_at
		FROM users
		ORDER BY username
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query users: %w", err)
	}
	defer rows.Close()

	var users []*user.User

	for rows.Next() {
		var u user.User
		var email, hierarchyLevelID, reportsTo sql.NullString
		var createdAt, updatedAt sql.NullTime
		var passwordHash, authType sql.NullString

		err := rows.Scan(
			&u.ID,
			&u.Username,
			&u.Name,
			&email,
			&hierarchyLevelID,
			&reportsTo,
			&passwordHash,
			&authType,
			&createdAt,
			&updatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}

		// Handle NULL fields
		if email.Valid {
			u.Email = email.String
		}
		if hierarchyLevelID.Valid {
			u.HierarchyLevelID = hierarchyLevelID.String
		}
		if reportsTo.Valid {
			u.ReportsTo = &reportsTo.String
		}
		if passwordHash.Valid {
			u.PasswordHash = passwordHash.String
		}
		if authType.Valid {
			u.AuthType = user.AuthType(authType.String)
		}
		if u.AuthType == "" {
			u.AuthType = user.AuthTypeLocal
		}
		if createdAt.Valid {
			u.CreatedAt = createdAt.Time
		}
		if updatedAt.Valid {
			u.UpdatedAt = updatedAt.Time
		}

		// Fetch team IDs
		teamIDs, err := r.fetchTeamIDs(ctx, u.ID)
		if err != nil {
			return nil, err
		}
		u.TeamIDs = teamIDs

		// Check if user is admin
		u.IsAdmin = u.Username == "admin"

		users = append(users, &u)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return users, nil
}

// FindByHierarchyLevel retrieves all users at a specific hierarchy level
func (r *UserRepository) FindByHierarchyLevel(ctx context.Context, levelID string) ([]*user.User, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, username, full_name, email, hierarchy_level_id, reports_to,
		       password_hash, auth_type, created_at, updated_at
		FROM users
		WHERE hierarchy_level_id = $1
		ORDER BY username
	`, levelID)

	if err != nil {
		return nil, fmt.Errorf("failed to query users by hierarchy level: %w", err)
	}
	defer rows.Close()

	return r.scanUsers(ctx, rows)
}

// FindSubordinates recursively finds all users reporting to a given user.
// Includes cycle protection (visited array) and depth limit to prevent
// infinite recursion from bad data in reports_to.
// Team memberships are batch-loaded in a single query to avoid N+1.
func (r *UserRepository) FindSubordinates(ctx context.Context, supervisorID string) ([]*user.User, error) {
	// Use recursive CTE with cycle protection and depth limit
	rows, err := r.db.QueryContext(ctx, `
		WITH RECURSIVE subordinates AS (
			-- Base case: direct reports
			SELECT id, username, full_name, email, hierarchy_level_id, reports_to,
			       password_hash, auth_type, created_at, updated_at,
			       1 AS depth, ARRAY[$1::text, id::text] AS visited
			FROM users
			WHERE reports_to = $1

			UNION ALL

			-- Recursive case: reports of reports
			-- Stops on cycle (visited array) or max depth (20 levels)
			SELECT u.id, u.username, u.full_name, u.email, u.hierarchy_level_id, u.reports_to,
			       u.password_hash, u.auth_type, u.created_at, u.updated_at,
			       s.depth + 1, s.visited || u.id::text
			FROM users u
			INNER JOIN subordinates s ON u.reports_to = s.id
			WHERE s.depth < 20
			  AND NOT (u.id::text = ANY(s.visited))
		)
		SELECT id, username, full_name, email, hierarchy_level_id, reports_to,
		       password_hash, auth_type, created_at, updated_at
		FROM subordinates
		ORDER BY username
	`, supervisorID)

	if err != nil {
		return nil, fmt.Errorf("failed to query subordinates: %w", err)
	}
	defer rows.Close()

	// Scan users without per-user team queries
	var users []*user.User
	for rows.Next() {
		var u user.User
		var email, hierarchyLevelID, reportsTo sql.NullString
		var createdAt, updatedAt sql.NullTime
		var passwordHash, authType sql.NullString

		err := rows.Scan(
			&u.ID, &u.Username, &u.Name, &email,
			&hierarchyLevelID, &reportsTo, &passwordHash,
			&authType, &createdAt, &updatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}

		if email.Valid {
			u.Email = email.String
		}
		if hierarchyLevelID.Valid {
			u.HierarchyLevelID = hierarchyLevelID.String
		}
		if reportsTo.Valid {
			u.ReportsTo = &reportsTo.String
		}
		if passwordHash.Valid {
			u.PasswordHash = passwordHash.String
		}
		if authType.Valid {
			u.AuthType = user.AuthType(authType.String)
		}
		if u.AuthType == "" {
			u.AuthType = user.AuthTypeLocal
		}
		if createdAt.Valid {
			u.CreatedAt = createdAt.Time
		}
		if updatedAt.Valid {
			u.UpdatedAt = updatedAt.Time
		}
		u.IsAdmin = u.Username == "admin"
		u.TeamIDs = []string{} // populated below in batch

		users = append(users, &u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	// Batch-load team memberships in a single query to avoid N+1
	if len(users) > 0 {
		userIDs := make([]string, len(users))
		userMap := make(map[string]*user.User, len(users))
		for i, u := range users {
			userIDs[i] = u.ID
			userMap[u.ID] = u
		}

		teamRows, err := r.db.QueryContext(ctx, `
			SELECT user_id, team_id
			FROM team_members
			WHERE user_id = ANY($1)
			ORDER BY user_id, team_id
		`, pq.Array(userIDs))
		if err != nil {
			return nil, fmt.Errorf("failed to batch-load team memberships: %w", err)
		}
		defer teamRows.Close()

		for teamRows.Next() {
			var userID, teamID string
			if err := teamRows.Scan(&userID, &teamID); err != nil {
				return nil, fmt.Errorf("failed to scan team membership: %w", err)
			}
			if u, ok := userMap[userID]; ok {
				u.TeamIDs = append(u.TeamIDs, teamID)
			}
		}
		if err := teamRows.Err(); err != nil {
			return nil, fmt.Errorf("team rows error: %w", err)
		}
	}

	return users, nil
}

// FindSupervisorChainUp walks UP the reports_to chain from a user, returning
// supervisors in order: direct manager first, then their manager, etc.
func (r *UserRepository) FindSupervisorChainUp(ctx context.Context, userID string) ([]*user.User, error) {
	rows, err := r.db.QueryContext(ctx, `
		WITH RECURSIVE supervisors AS (
			-- Base case: direct supervisor of the given user
			SELECT u.id, u.username, u.full_name, u.email, u.hierarchy_level_id, u.reports_to,
			       u.password_hash, u.auth_type, u.created_at, u.updated_at,
			       1 AS depth, ARRAY[$1::text, u.id::text] AS visited
			FROM users u
			WHERE u.id = (SELECT reports_to FROM users WHERE id = $1)

			UNION ALL

			-- Recursive case: supervisor's supervisor
			-- Stops on cycle (visited array) or max depth (20 levels)
			SELECT u.id, u.username, u.full_name, u.email, u.hierarchy_level_id, u.reports_to,
			       u.password_hash, u.auth_type, u.created_at, u.updated_at,
			       s.depth + 1, s.visited || u.id::text
			FROM users u
			INNER JOIN supervisors s ON u.id = s.reports_to
			WHERE s.depth < 20 AND NOT (u.id::text = ANY(s.visited))
		)
		SELECT id, username, full_name, email, hierarchy_level_id, reports_to,
		       password_hash, auth_type, created_at, updated_at
		FROM supervisors
		ORDER BY depth
	`, userID)

	if err != nil {
		return nil, fmt.Errorf("failed to query supervisor chain: %w", err)
	}
	defer rows.Close()

	return r.scanUsers(ctx, rows)
}

// Save persists a new user
func (r *UserRepository) Save(ctx context.Context, u *user.User) error {
	log := logger.Get()

	// Begin transaction
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		log.DB("begin_transaction").
			Table("users").
			Context("starting transaction to create new user account").
			RecordID(u.ID).
			Error(err).
			Failure()
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Convert empty strings to NULL for optional fields
	var email, hierarchyLevelID, reportsTo sql.NullString
	if u.Email != "" {
		email = sql.NullString{String: u.Email, Valid: true}
	}
	if u.HierarchyLevelID != "" {
		hierarchyLevelID = sql.NullString{String: u.HierarchyLevelID, Valid: true}
	}
	if u.ReportsTo != nil && *u.ReportsTo != "" {
		reportsTo = sql.NullString{String: *u.ReportsTo, Valid: true}
	}

	// Hash password if provided; SSO users get an empty string (password_hash is NOT NULL)
	var passwordHash sql.NullString
	if u.PasswordHash != "" {
		// If the password hash is already a bcrypt hash (starts with $2a$), use it as-is
		// Otherwise, hash it
		if len(u.PasswordHash) > 4 && u.PasswordHash[:4] == "$2a$" {
			passwordHash = sql.NullString{String: u.PasswordHash, Valid: true}
		} else {
			hashed, err := bcrypt.GenerateFromPassword([]byte(u.PasswordHash), bcrypt.DefaultCost)
			if err != nil {
				return fmt.Errorf("failed to hash password: %w", err)
			}
			passwordHash = sql.NullString{String: string(hashed), Valid: true}
		}
	} else {
		// password_hash column is NOT NULL — use empty string for SSO users
		passwordHash = sql.NullString{String: "", Valid: true}
	}

	// Set timestamps
	now := time.Now()
	if u.CreatedAt.IsZero() {
		u.CreatedAt = now
	}
	u.UpdatedAt = now

	// Default auth_type to local if not set
	if u.AuthType == "" {
		u.AuthType = user.AuthTypeLocal
	}

	// Insert user
	_, err = tx.ExecContext(ctx, `
		INSERT INTO users (id, username, full_name, email, hierarchy_level_id, reports_to, password_hash, auth_type, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, u.ID, u.Username, u.Name, email, hierarchyLevelID, reportsTo, passwordHash, string(u.AuthType), u.CreatedAt, u.UpdatedAt)

	if err != nil {
		log.DB("insert").
			Table("users").
			Context("inserting new user record into database").
			RecordID(u.ID).
			Error(err).
			Failure()
		return fmt.Errorf("failed to save user: %w", err)
	}

	// Insert team memberships
	for _, teamID := range u.TeamIDs {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO team_members (team_id, user_id)
			VALUES ($1, $2)
		`, teamID, u.ID)
		if err != nil {
			return fmt.Errorf("failed to save team membership: %w", err)
		}
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		log.DB("commit_transaction").
			Table("users").
			Context("committing transaction after creating user and team memberships").
			RecordID(u.ID).
			Error(err).
			Failure()
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Update updates an existing user
func (r *UserRepository) Update(ctx context.Context, u *user.User) error {
	log := logger.Get()

	// Begin transaction
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		log.DB("begin_transaction").
			Table("users").
			Context("starting transaction to update existing user account").
			RecordID(u.ID).
			Error(err).
			Failure()
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Convert empty strings to NULL for optional fields
	var email, hierarchyLevelID, reportsTo sql.NullString
	if u.Email != "" {
		email = sql.NullString{String: u.Email, Valid: true}
	}
	if u.HierarchyLevelID != "" {
		hierarchyLevelID = sql.NullString{String: u.HierarchyLevelID, Valid: true}
	}
	if u.ReportsTo != nil && *u.ReportsTo != "" {
		reportsTo = sql.NullString{String: *u.ReportsTo, Valid: true}
	}

	// Update timestamp
	u.UpdatedAt = time.Now()

	// Update user (don't update password_hash here - use separate method)
	result, err := tx.ExecContext(ctx, `
		UPDATE users SET
			username = $1,
			full_name = $2,
			email = $3,
			hierarchy_level_id = $4,
			reports_to = $5,
			auth_type = COALESCE(NULLIF($6, ''), auth_type),
			updated_at = $7
		WHERE id = $8
	`, u.Username, u.Name, email, hierarchyLevelID, reportsTo, string(u.AuthType), u.UpdatedAt, u.ID)

	if err != nil {
		log.DB("update").
			Table("users").
			Context("updating user profile fields (username, name, email, hierarchy, reports_to)").
			RecordID(u.ID).
			Error(err).
			Failure()
		return fmt.Errorf("failed to update user: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		log.DB("update").
			Table("users").
			Context("user record not found during update - may have been deleted").
			RecordID(u.ID).
			Failure()
		return fmt.Errorf("user not found for update")
	}

	// Update team memberships
	// Delete existing memberships
	_, err = tx.ExecContext(ctx, "DELETE FROM team_members WHERE user_id = $1", u.ID)
	if err != nil {
		return fmt.Errorf("failed to delete team memberships: %w", err)
	}

	// Insert new memberships
	for _, teamID := range u.TeamIDs {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO team_members (team_id, user_id)
			VALUES ($1, $2)
		`, teamID, u.ID)
		if err != nil {
			return fmt.Errorf("failed to save team membership: %w", err)
		}
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		log.DB("commit_transaction").
			Table("users").
			Context("committing transaction after updating user and team memberships").
			RecordID(u.ID).
			Error(err).
			Failure()
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Delete removes a user
func (r *UserRepository) Delete(ctx context.Context, id string) error {
	log := logger.Get()

	result, err := r.db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", id)
	if err != nil {
		log.DB("delete").
			Table("users").
			Context("removing user account from database").
			RecordID(id).
			Error(err).
			Failure()
		return fmt.Errorf("failed to delete user: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		log.DB("delete").
			Table("users").
			Context("user not found during delete - may have already been removed").
			RecordID(id).
			Failure()
		return fmt.Errorf("user not found for delete")
	}

	return nil
}

// FindTeamIDsForUser retrieves all team IDs for a user
func (r *UserRepository) FindTeamIDsForUser(ctx context.Context, userID string) ([]string, error) {
	return r.fetchTeamIDs(ctx, userID)
}

// FindTeamsWhereUserIsLead retrieves all team IDs where the user is a team lead
func (r *UserRepository) FindTeamsWhereUserIsLead(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id
		FROM teams
		WHERE team_lead_id = $1
		ORDER BY id
	`, userID)

	if err != nil {
		return nil, fmt.Errorf("failed to query teams where user is lead: %w", err)
	}
	defer rows.Close()

	var teamIDs []string
	for rows.Next() {
		var teamID string
		if err := rows.Scan(&teamID); err != nil {
			return nil, fmt.Errorf("failed to scan team ID: %w", err)
		}
		teamIDs = append(teamIDs, teamID)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	// Return empty slice instead of nil if no teams
	if teamIDs == nil {
		teamIDs = []string{}
	}

	return teamIDs, nil
}

// fetchTeamIDs is a helper function to get team IDs for a user
func (r *UserRepository) fetchTeamIDs(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT team_id
		FROM team_members
		WHERE user_id = $1
		ORDER BY team_id
	`, userID)

	if err != nil {
		return nil, fmt.Errorf("failed to query team memberships: %w", err)
	}
	defer rows.Close()

	var teamIDs []string
	for rows.Next() {
		var teamID string
		if err := rows.Scan(&teamID); err != nil {
			return nil, fmt.Errorf("failed to scan team ID: %w", err)
		}
		teamIDs = append(teamIDs, teamID)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	// Return empty slice instead of nil if no teams
	if teamIDs == nil {
		teamIDs = []string{}
	}

	return teamIDs, nil
}

// scanUsers is a helper function to scan query results into users
func (r *UserRepository) scanUsers(ctx context.Context, rows *sql.Rows) ([]*user.User, error) {
	var users []*user.User

	for rows.Next() {
		var u user.User
		var email, hierarchyLevelID, reportsTo sql.NullString
		var createdAt, updatedAt sql.NullTime
		var passwordHash, authType sql.NullString

		err := rows.Scan(
			&u.ID,
			&u.Username,
			&u.Name,
			&email,
			&hierarchyLevelID,
			&reportsTo,
			&passwordHash,
			&authType,
			&createdAt,
			&updatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}

		// Handle NULL fields
		if email.Valid {
			u.Email = email.String
		}
		if hierarchyLevelID.Valid {
			u.HierarchyLevelID = hierarchyLevelID.String
		}
		if reportsTo.Valid {
			u.ReportsTo = &reportsTo.String
		}
		if passwordHash.Valid {
			u.PasswordHash = passwordHash.String
		}
		if authType.Valid {
			u.AuthType = user.AuthType(authType.String)
		}
		if u.AuthType == "" {
			u.AuthType = user.AuthTypeLocal
		}
		if createdAt.Valid {
			u.CreatedAt = createdAt.Time
		}
		if updatedAt.Valid {
			u.UpdatedAt = updatedAt.Time
		}

		// Fetch team IDs
		teamIDs, err := r.fetchTeamIDs(ctx, u.ID)
		if err != nil {
			return nil, err
		}
		u.TeamIDs = teamIDs

		// Check if user is admin
		u.IsAdmin = u.Username == "admin"

		users = append(users, &u)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return users, nil
}

// UpdatePassword updates a user's password hash
func (r *UserRepository) UpdatePassword(ctx context.Context, userID string, hashedPassword string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE users
		SET password_hash = $1, updated_at = $2
		WHERE id = $3
	`, hashedPassword, time.Now(), userID)

	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("user not found for password update")
	}

	return nil
}

// VerifyPassword checks if the provided password matches the user's password hash
// This is a helper method, not part of the repository interface
func (r *UserRepository) VerifyPassword(ctx context.Context, username, password string) (*user.User, error) {
	u, err := r.FindByUsername(ctx, username)
	if err != nil {
		return nil, err
	}

	// Verify password
	err = bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password))
	if err != nil {
		return nil, fmt.Errorf("invalid password")
	}

	return u, nil
}
