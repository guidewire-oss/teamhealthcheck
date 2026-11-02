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
	var deactivatedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, `
		SELECT id, username, full_name, email, hierarchy_level_id, reports_to,
		       password_hash, auth_type, created_at, updated_at, is_active, deactivated_at
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
		&u.IsActive,
		&deactivatedAt,
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

	if deactivatedAt.Valid {
		t := deactivatedAt.Time
		u.DeactivatedAt = &t
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
	var deactivatedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, `
		SELECT id, username, full_name, email, hierarchy_level_id, reports_to,
		       password_hash, auth_type, created_at, updated_at, is_active, deactivated_at
		FROM users
		WHERE username = $1 AND is_active
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
		&u.IsActive,
		&deactivatedAt,
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

	applyDeactivation(&u, deactivatedAt)

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
	var deactivatedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, `
		SELECT id, username, full_name, email, hierarchy_level_id, reports_to,
		       password_hash, auth_type, created_at, updated_at, is_active, deactivated_at
		FROM users
		WHERE email = $1 AND is_active
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
		&u.IsActive,
		&deactivatedAt,
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

	applyDeactivation(&u, deactivatedAt)

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
		       password_hash, auth_type, created_at, updated_at, is_active, deactivated_at
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
		var deactivatedAt sql.NullTime

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
			&u.IsActive,
			&deactivatedAt,
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

		applyDeactivation(&u, deactivatedAt)

		// Check if user is admin
		u.IsAdmin = u.Username == "admin"

		users = append(users, &u)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	// Batch-load every user's team memberships in one query instead of one
	// round trip per user — with thousands of users, a per-user loop here
	// turns a sub-second query into several seconds.
	if len(users) > 0 {
		userIDs := make([]string, len(users))
		userMap := make(map[string]*user.User, len(users))
		for i, u := range users {
			userIDs[i] = u.ID
			userMap[u.ID] = u
			u.TeamIDs = []string{}
		}

		teamRows, err := r.db.QueryContext(ctx, `
			SELECT user_id, team_id
			FROM team_members
			WHERE user_id = ANY($1) AND is_active
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

// FindPage retrieves one page of users matching filter, ordered deterministically
// by username, along with the total count of users matching filter.
// Filtering and limiting both happen at the SQL level to avoid loading the
// full user table into memory.
func (r *UserRepository) FindPage(ctx context.Context, filter user.ListFilter, page, pageSize int) ([]*user.User, int, error) {
	offset := (page - 1) * pageSize

	var total int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM users
		WHERE ($1 = '' OR hierarchy_level_id = $1)
		  AND ($2 = '' OR username ILIKE '%' || $2 || '%'
		            OR full_name ILIKE '%' || $2 || '%'
		            OR email ILIKE '%' || $2 || '%')
		  AND ($3 = '' OR ($3 = 'active' AND is_active) OR ($3 = 'inactive' AND NOT is_active))
	`, filter.HierarchyLevel, filter.Search, filter.Status).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count users: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, username, full_name, email, hierarchy_level_id, reports_to,
		       password_hash, auth_type, created_at, updated_at, is_active, deactivated_at
		FROM users
		WHERE ($1 = '' OR hierarchy_level_id = $1)
		  AND ($2 = '' OR username ILIKE '%' || $2 || '%'
		            OR full_name ILIKE '%' || $2 || '%'
		            OR email ILIKE '%' || $2 || '%')
		  AND ($5 = '' OR ($5 = 'active' AND is_active) OR ($5 = 'inactive' AND NOT is_active))
		ORDER BY username
		LIMIT $3 OFFSET $4
	`, filter.HierarchyLevel, filter.Search, pageSize, offset, filter.Status)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query users page: %w", err)
	}
	defer rows.Close()

	users, err := r.scanUsersWithoutTeamIDs(rows)
	if err != nil {
		return nil, 0, err
	}

	// Batch-load team memberships for just this page's users in a single
	// query to avoid an N+1 (one fetchTeamIDs call per row).
	if len(users) > 0 {
		userIDs := make([]string, len(users))
		userMap := make(map[string]*user.User, len(users))
		for i, u := range users {
			userIDs[i] = u.ID
			userMap[u.ID] = u
			u.TeamIDs = []string{}
		}

		teamRows, err := r.db.QueryContext(ctx, `
			SELECT user_id, team_id
			FROM team_members
			WHERE user_id = ANY($1) AND is_active
			ORDER BY user_id, team_id
		`, pq.Array(userIDs))
		if err != nil {
			return nil, 0, fmt.Errorf("failed to batch-load team memberships: %w", err)
		}
		defer teamRows.Close()

		for teamRows.Next() {
			var userID, teamID string
			if err := teamRows.Scan(&userID, &teamID); err != nil {
				return nil, 0, fmt.Errorf("failed to scan team membership: %w", err)
			}
			if u, ok := userMap[userID]; ok {
				u.TeamIDs = append(u.TeamIDs, teamID)
			}
		}
		if err := teamRows.Err(); err != nil {
			return nil, 0, fmt.Errorf("team rows error: %w", err)
		}
	}

	return users, total, nil
}

// FindAllLite retrieves minimal fields (no team IDs, no password hash use) for
// every user. Intended for dropdowns/pickers that need the full user set
// without the cost of the team-membership joins used by the full listing.
func (r *UserRepository) FindAllLite(ctx context.Context) ([]*user.User, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, username, full_name, hierarchy_level_id
		FROM users
		ORDER BY username
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query users (lite): %w", err)
	}
	defer rows.Close()

	var users []*user.User
	for rows.Next() {
		var u user.User
		var hierarchyLevelID sql.NullString
		if err := rows.Scan(&u.ID, &u.Username, &u.Name, &hierarchyLevelID); err != nil {
			return nil, fmt.Errorf("failed to scan user (lite): %w", err)
		}
		if hierarchyLevelID.Valid {
			u.HierarchyLevelID = hierarchyLevelID.String
		}
		u.IsAdmin = u.Username == "admin"
		u.TeamIDs = []string{}
		users = append(users, &u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return users, nil
}

// scanUsersWithoutTeamIDs scans query results into users without fetching
// team IDs per row (caller is responsible for batch-loading TeamIDs).
func (r *UserRepository) scanUsersWithoutTeamIDs(rows *sql.Rows) ([]*user.User, error) {
	var users []*user.User

	for rows.Next() {
		var u user.User
		var email, hierarchyLevelID, reportsTo sql.NullString
		var createdAt, updatedAt sql.NullTime
		var passwordHash, authType sql.NullString
		var deactivatedAt sql.NullTime

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
			&u.IsActive,
			&deactivatedAt,
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

		if deactivatedAt.Valid {
			t := deactivatedAt.Time
			u.DeactivatedAt = &t
		}

		u.IsAdmin = u.Username == "admin"

		users = append(users, &u)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return users, nil
}

// FindByHierarchyLevel retrieves all users at a specific hierarchy level
func (r *UserRepository) FindByHierarchyLevel(ctx context.Context, levelID string) ([]*user.User, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, username, full_name, email, hierarchy_level_id, reports_to,
		       password_hash, auth_type, created_at, updated_at, is_active, deactivated_at
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
			       password_hash, auth_type, created_at, updated_at, is_active, deactivated_at,
			       1 AS depth, ARRAY[$1::text, id::text] AS visited
			FROM users
			WHERE reports_to = $1

			UNION ALL

			-- Recursive case: reports of reports
			-- Stops on cycle (visited array) or max depth (20 levels)
			SELECT u.id, u.username, u.full_name, u.email, u.hierarchy_level_id, u.reports_to,
			       u.password_hash, u.auth_type, u.created_at, u.updated_at, u.is_active, u.deactivated_at,
			       s.depth + 1, s.visited || u.id::text
			FROM users u
			INNER JOIN subordinates s ON u.reports_to = s.id
			WHERE s.depth < 20
			  AND NOT (u.id::text = ANY(s.visited))
		)
		SELECT id, username, full_name, email, hierarchy_level_id, reports_to,
		       password_hash, auth_type, created_at, updated_at, is_active, deactivated_at
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
		var deactivatedAt sql.NullTime

		err := rows.Scan(
			&u.ID, &u.Username, &u.Name, &email,
			&hierarchyLevelID, &reportsTo, &passwordHash,
			&authType, &createdAt, &updatedAt,
			&u.IsActive,
			&deactivatedAt,
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

		applyDeactivation(&u, deactivatedAt)
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
			WHERE user_id = ANY($1) AND is_active
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
			       u.password_hash, u.auth_type, u.created_at, u.updated_at, u.is_active, u.deactivated_at,
			       1 AS depth, ARRAY[$1::text, u.id::text] AS visited
			FROM users u
			WHERE u.id = (SELECT reports_to FROM users WHERE id = $1)

			UNION ALL

			-- Recursive case: supervisor's supervisor
			-- Stops on cycle (visited array) or max depth (20 levels)
			SELECT u.id, u.username, u.full_name, u.email, u.hierarchy_level_id, u.reports_to,
			       u.password_hash, u.auth_type, u.created_at, u.updated_at, u.is_active, u.deactivated_at,
			       s.depth + 1, s.visited || u.id::text
			FROM users u
			INNER JOIN supervisors s ON u.id = s.reports_to
			WHERE s.depth < 20 AND NOT (u.id::text = ANY(s.visited))
		)
		SELECT id, username, full_name, email, hierarchy_level_id, reports_to,
		       password_hash, auth_type, created_at, updated_at, is_active, deactivated_at
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
	// Reconcile memberships without destroying history: a team the person was
	// taken out of becomes an inactive membership (kept), and a team they are
	// put back into is reactivated. Rewriting the whole set (delete, re-insert)
	// would erase inactive history and could resurrect an old membership.
	if _, err = tx.ExecContext(ctx, `
		UPDATE team_members
		SET is_active = false, deactivated_at = COALESCE(deactivated_at, CURRENT_TIMESTAMP)
		WHERE user_id = $1 AND is_active AND team_id <> ALL($2)
	`, u.ID, pq.Array(u.TeamIDs)); err != nil {
		return fmt.Errorf("failed to end team memberships: %w", err)
	}

	for _, teamID := range u.TeamIDs {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO team_members (team_id, user_id)
			VALUES ($1, $2)
			ON CONFLICT (team_id, user_id) DO UPDATE
				SET is_active = true, deactivated_at = NULL
				WHERE team_members.is_active = false
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

// Deactivate soft-deactivates a person. The users row is kept, so surveys,
// responses and action items keep resolving to them; their memberships are ended
// (kept as inactive history) so they stop receiving surveys. Both writes happen
// in one transaction. deactivated_at keeps its original value if the person was
// already inactive, which makes repeating the call harmless.
func (r *UserRepository) Deactivate(ctx context.Context, id string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
		UPDATE users
		SET is_active = false, deactivated_at = COALESCE(deactivated_at, CURRENT_TIMESTAMP), updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("failed to deactivate user: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("failed to confirm user deactivation: %w", err)
	} else if n == 0 {
		return fmt.Errorf("user not found with provided ID")
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE team_members
		SET is_active = false, deactivated_at = COALESCE(deactivated_at, CURRENT_TIMESTAMP)
		WHERE user_id = $1 AND is_active
	`, id); err != nil {
		return fmt.Errorf("failed to end memberships of deactivated user: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit user deactivation: %w", err)
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

// FindTeamIDsForUsers batch-loads team memberships for every given user in a
// single query — the same "avoid N+1" pattern already used by
// FindSubordinates, applied here for callers (like the admin users list)
// that otherwise called FindTeamIDsForUser once per user. Users with no
// memberships are simply absent from the returned map.
func (r *UserRepository) FindTeamIDsForUsers(ctx context.Context, userIDs []string) (map[string][]string, error) {
	result := make(map[string][]string, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT user_id, team_id
		FROM team_members
		WHERE user_id = ANY($1) AND is_active
		ORDER BY user_id, team_id
	`, pq.Array(userIDs))
	if err != nil {
		return nil, fmt.Errorf("failed to batch-load team memberships: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var userID, teamID string
		if err := rows.Scan(&userID, &teamID); err != nil {
			return nil, fmt.Errorf("failed to scan team membership: %w", err)
		}
		result[userID] = append(result[userID], teamID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return result, nil
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
		WHERE user_id = $1 AND is_active
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
		var deactivatedAt sql.NullTime

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
			&u.IsActive,
			&deactivatedAt,
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

		applyDeactivation(&u, deactivatedAt)

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

// applyDeactivation records when a soft-deactivated user was deactivated.
func applyDeactivation(u *user.User, deactivatedAt sql.NullTime) {
	if deactivatedAt.Valid {
		t := deactivatedAt.Time
		u.DeactivatedAt = &t
	}
}
