package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

	"github.com/agopalakrishnan/teams360/backend/domain/user"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres/sqlcgen"
	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
	"golang.org/x/crypto/bcrypt"
)

// UserRepository implements the user.Repository interface
type UserRepository struct {
	db      *sql.DB
	queries *sqlcgen.Queries
}

// NewUserRepository creates a new repository instance
func NewUserRepository(db *sql.DB) user.Repository {
	return &UserRepository{db: db, queries: sqlcgen.New(db)}
}

// buildUserFromRow maps the scalar columns shared by every user-select query
// (each sqlc query produces its own distinct row type even when structurally
// identical, so this takes plain scalars rather than a shared row struct).
func buildUserFromRow(id, username, fullName, email, hierarchyLevelID string, reportsTo sql.NullString, passwordHash, authType string, createdAt, updatedAt sql.NullTime, isActive bool, deactivatedAt sql.NullTime) *user.User {
	u := &user.User{
		ID:               id,
		Username:         username,
		Name:             fullName,
		Email:            email,
		HierarchyLevelID: hierarchyLevelID,
		PasswordHash:     passwordHash,
		IsActive:         isActive,
	}

	if reportsTo.Valid {
		u.ReportsTo = &reportsTo.String
	}
	if authType != "" {
		u.AuthType = user.AuthType(authType)
	} else {
		u.AuthType = user.AuthTypeLocal
	}
	if createdAt.Valid {
		u.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		u.UpdatedAt = updatedAt.Time
	}
	applyDeactivation(u, deactivatedAt)
	u.IsAdmin = u.Username == "admin"

	return u
}

// FindByID retrieves a user by ID
func (r *UserRepository) FindByID(ctx context.Context, id string) (*user.User, error) {
	row, err := r.queries.FindUserByID(ctx, id)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found with provided ID")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find user: %w", err)
	}

	u := buildUserFromRow(row.ID, row.Username, row.FullName, row.Email, row.HierarchyLevelID, row.ReportsTo, row.PasswordHash, row.AuthType, row.CreatedAt, row.UpdatedAt, row.IsActive, row.DeactivatedAt)

	teamIDs, err := r.fetchTeamIDs(ctx, id)
	if err != nil {
		return nil, err
	}
	u.TeamIDs = teamIDs

	return u, nil
}

// FindByUsername retrieves a user by username (used for authentication)
func (r *UserRepository) FindByUsername(ctx context.Context, username string) (*user.User, error) {
	row, err := r.queries.FindUserByUsername(ctx, username)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found with provided username")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find user: %w", err)
	}

	u := buildUserFromRow(row.ID, row.Username, row.FullName, row.Email, row.HierarchyLevelID, row.ReportsTo, row.PasswordHash, row.AuthType, row.CreatedAt, row.UpdatedAt, row.IsActive, row.DeactivatedAt)

	teamIDs, err := r.fetchTeamIDs(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	u.TeamIDs = teamIDs

	return u, nil
}

// FindByEmail retrieves a user by email
func (r *UserRepository) FindByEmail(ctx context.Context, emailParam string) (*user.User, error) {
	row, err := r.queries.FindUserByEmail(ctx, emailParam)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found with provided email")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find user: %w", err)
	}

	u := buildUserFromRow(row.ID, row.Username, row.FullName, row.Email, row.HierarchyLevelID, row.ReportsTo, row.PasswordHash, row.AuthType, row.CreatedAt, row.UpdatedAt, row.IsActive, row.DeactivatedAt)

	teamIDs, err := r.fetchTeamIDs(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	u.TeamIDs = teamIDs

	return u, nil
}

// FindAll retrieves all users
func (r *UserRepository) FindAll(ctx context.Context) ([]*user.User, error) {
	rows, err := r.queries.FindAllUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query users: %w", err)
	}

	var users []*user.User
	for _, row := range rows {
		u := buildUserFromRow(row.ID, row.Username, row.FullName, row.Email, row.HierarchyLevelID, row.ReportsTo, row.PasswordHash, row.AuthType, row.CreatedAt, row.UpdatedAt, row.IsActive, row.DeactivatedAt)
		users = append(users, u)
	}

	// Batch-load every user's team memberships in one query instead of one
	// round trip per user — with thousands of users, a per-user loop here
	// turns a sub-second query into several seconds.
	if err := r.batchLoadTeamIDs(ctx, users); err != nil {
		return nil, err
	}

	return users, nil
}

// batchLoadTeamIDs fetches active team memberships for every given user in one
// query and populates each user's TeamIDs in place.
func (r *UserRepository) batchLoadTeamIDs(ctx context.Context, users []*user.User) error {
	if len(users) == 0 {
		return nil
	}
	userIDs := make([]string, len(users))
	userMap := make(map[string]*user.User, len(users))
	for i, u := range users {
		userIDs[i] = u.ID
		userMap[u.ID] = u
		u.TeamIDs = []string{}
	}

	rows, err := r.queries.FindTeamIDsForUsers(ctx, userIDs)
	if err != nil {
		return fmt.Errorf("failed to batch-load team memberships: %w", err)
	}
	for _, row := range rows {
		if u, ok := userMap[row.UserID]; ok {
			u.TeamIDs = append(u.TeamIDs, row.TeamID)
		}
	}
	return nil
}

// pageOffset returns the zero-based row offset for a 1-based page, or false
// when the product (page-1)*pageSize cannot be represented as an int. A page
// below 1 is treated as the first page; a non-positive pageSize has no valid
// offset.
func pageOffset(page, pageSize int) (int, bool) {
	if pageSize < 1 {
		return 0, false
	}
	if page < 1 {
		page = 1
	}
	if page-1 > math.MaxInt/pageSize {
		return 0, false
	}
	return (page - 1) * pageSize, true
}

// FindPage retrieves one page of users matching filter, ordered deterministically
// by username, along with the total count of users matching filter.
// Filtering and limiting both happen at the SQL level to avoid loading the
// full user table into memory.
func (r *UserRepository) FindPage(ctx context.Context, filter user.ListFilter, page, pageSize int) ([]*user.User, int, error) {
	// page comes straight from the query string, so (page-1)*pageSize can
	// overflow int and wrap negative, which PostgreSQL rejects as a negative
	// OFFSET (a 500). pageOffset reports overflow instead of wrapping.
	offset, offsetOK := pageOffset(page, pageSize)

	total, err := r.queries.CountUsersPage(ctx, sqlcgen.CountUsersPageParams{
		HierarchyLevel: filter.HierarchyLevel,
		Search:         filter.Search,
		Status:         filter.Status,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count users: %w", err)
	}

	// A page whose offset is not representable is necessarily past the end of
	// any real result set: answer with the normal empty page (and the true
	// total) instead of issuing a query with a bogus offset.
	if !offsetOK {
		return []*user.User{}, int(total), nil
	}

	rows, err := r.queries.FindUsersPage(ctx, sqlcgen.FindUsersPageParams{
		HierarchyLevel: filter.HierarchyLevel,
		Search:         filter.Search,
		Status:         filter.Status,
		PageLimit:      int64(pageSize),
		PageOffset:     int64(offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query users page: %w", err)
	}

	users := make([]*user.User, 0, len(rows))
	for _, row := range rows {
		users = append(users, buildUserFromRow(row.ID, row.Username, row.FullName, row.Email, row.HierarchyLevelID, row.ReportsTo, row.PasswordHash, row.AuthType, row.CreatedAt, row.UpdatedAt, row.IsActive, row.DeactivatedAt))
	}

	// Batch-load team memberships for just this page's users in a single
	// query to avoid an N+1 (one fetchTeamIDs call per row).
	if err := r.batchLoadTeamIDs(ctx, users); err != nil {
		return nil, 0, err
	}

	return users, int(total), nil
}

// FindAllLite retrieves minimal fields (no team IDs, no password hash use) for
// every user. Intended for dropdowns/pickers that need the full user set
// without the cost of the team-membership joins used by the full listing.
func (r *UserRepository) FindAllLite(ctx context.Context) ([]*user.User, error) {
	rows, err := r.queries.FindAllUsersLite(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query users (lite): %w", err)
	}

	users := make([]*user.User, 0, len(rows))
	for _, row := range rows {
		u := &user.User{
			ID:               row.ID,
			Username:         row.Username,
			Name:             row.FullName,
			HierarchyLevelID: row.HierarchyLevelID,
			TeamIDs:          []string{},
		}
		u.IsAdmin = u.Username == "admin"
		users = append(users, u)
	}

	return users, nil
}

// FindByHierarchyLevel retrieves all users at a specific hierarchy level
func (r *UserRepository) FindByHierarchyLevel(ctx context.Context, levelID string) ([]*user.User, error) {
	rows, err := r.queries.FindUsersByHierarchyLevel(ctx, levelID)
	if err != nil {
		return nil, fmt.Errorf("failed to query users by hierarchy level: %w", err)
	}

	var users []*user.User
	for _, row := range rows {
		u := buildUserFromRow(row.ID, row.Username, row.FullName, row.Email, row.HierarchyLevelID, row.ReportsTo, row.PasswordHash, row.AuthType, row.CreatedAt, row.UpdatedAt, row.IsActive, row.DeactivatedAt)
		teamIDs, err := r.fetchTeamIDs(ctx, u.ID)
		if err != nil {
			return nil, err
		}
		u.TeamIDs = teamIDs
		users = append(users, u)
	}

	return users, nil
}

// FindSubordinates recursively finds all users reporting to a given user.
// Includes cycle protection (visited array) and depth limit to prevent
// infinite recursion from bad data in reports_to.
// Team memberships are batch-loaded in a single query to avoid N+1.
func (r *UserRepository) FindSubordinates(ctx context.Context, supervisorID string) ([]*user.User, error) {
	rows, err := r.queries.FindSubordinatesRecursive(ctx, supervisorID)
	if err != nil {
		return nil, fmt.Errorf("failed to query subordinates: %w", err)
	}

	var users []*user.User
	userMap := make(map[string]*user.User, len(rows))
	userIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		u := buildUserFromRow(row.ID, row.Username, row.FullName, row.Email, row.HierarchyLevelID, row.ReportsTo, row.PasswordHash, row.AuthType, row.CreatedAt, row.UpdatedAt, row.IsActive, row.DeactivatedAt)
		u.TeamIDs = []string{} // populated below in batch
		users = append(users, u)
		userMap[u.ID] = u
		userIDs = append(userIDs, u.ID)
	}

	// Batch-load team memberships in a single query to avoid N+1
	if len(userIDs) > 0 {
		teamRows, err := r.queries.FindTeamIDsForUsers(ctx, userIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to batch-load team memberships: %w", err)
		}
		for _, tr := range teamRows {
			if u, ok := userMap[tr.UserID]; ok {
				u.TeamIDs = append(u.TeamIDs, tr.TeamID)
			}
		}
	}

	return users, nil
}

// FindSupervisorChainUp walks UP the reports_to chain from a user, returning
// supervisors in order: direct manager first, then their manager, etc.
func (r *UserRepository) FindSupervisorChainUp(ctx context.Context, userID string) ([]*user.User, error) {
	rows, err := r.queries.FindSupervisorChainUpRecursive(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query supervisor chain: %w", err)
	}

	var users []*user.User
	for _, row := range rows {
		u := buildUserFromRow(row.ID, row.Username, row.FullName, row.Email, row.HierarchyLevelID, row.ReportsTo, row.PasswordHash, row.AuthType, row.CreatedAt, row.UpdatedAt, row.IsActive, row.DeactivatedAt)
		teamIDs, err := r.fetchTeamIDs(ctx, u.ID)
		if err != nil {
			return nil, err
		}
		u.TeamIDs = teamIDs
		users = append(users, u)
	}

	return users, nil
}

// Save persists a new user
func (r *UserRepository) Save(ctx context.Context, u *user.User) error {
	log := logger.Get()

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
	qtx := r.queries.WithTx(tx)

	var reportsTo sql.NullString
	if u.ReportsTo != nil && *u.ReportsTo != "" {
		reportsTo = sql.NullString{String: *u.ReportsTo, Valid: true}
	}

	// Hash password if provided; SSO users get an empty string (password_hash is NOT NULL)
	passwordHash := ""
	if u.PasswordHash != "" {
		// If the password hash is already a bcrypt hash (starts with $2a$), use it as-is
		// Otherwise, hash it
		if len(u.PasswordHash) > 4 && u.PasswordHash[:4] == "$2a$" {
			passwordHash = u.PasswordHash
		} else {
			hashed, err := bcrypt.GenerateFromPassword([]byte(u.PasswordHash), bcrypt.DefaultCost)
			if err != nil {
				return fmt.Errorf("failed to hash password: %w", err)
			}
			passwordHash = string(hashed)
		}
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

	err = qtx.InsertUser(ctx, sqlcgen.InsertUserParams{
		ID:               u.ID,
		Username:         u.Username,
		FullName:         u.Name,
		Email:            u.Email,
		HierarchyLevelID: u.HierarchyLevelID,
		ReportsTo:        reportsTo,
		PasswordHash:     passwordHash,
		AuthType:         string(u.AuthType),
		CreatedAt:        sql.NullTime{Time: u.CreatedAt, Valid: true},
		UpdatedAt:        sql.NullTime{Time: u.UpdatedAt, Valid: true},
	})
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
		if err := qtx.InsertTeamMember(ctx, sqlcgen.InsertTeamMemberParams{TeamID: teamID, UserID: u.ID}); err != nil {
			return fmt.Errorf("failed to save team membership: %w", err)
		}
	}

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
	qtx := r.queries.WithTx(tx)

	var reportsTo sql.NullString
	if u.ReportsTo != nil && *u.ReportsTo != "" {
		reportsTo = sql.NullString{String: *u.ReportsTo, Valid: true}
	}

	u.UpdatedAt = time.Now()

	// Update user (don't update password_hash here - use separate method)
	rowsAffected, err := qtx.UpdateUser(ctx, sqlcgen.UpdateUserParams{
		Username:         u.Username,
		FullName:         u.Name,
		Email:            u.Email,
		HierarchyLevelID: u.HierarchyLevelID,
		ReportsTo:        reportsTo,
		AuthType:         string(u.AuthType),
		UpdatedAt:        sql.NullTime{Time: u.UpdatedAt, Valid: true},
		ID:               u.ID,
	})
	if err != nil {
		log.DB("update").
			Table("users").
			Context("updating user profile fields (username, name, email, hierarchy, reports_to)").
			RecordID(u.ID).
			Error(err).
			Failure()
		return fmt.Errorf("failed to update user: %w", err)
	}
	if rowsAffected == 0 {
		log.DB("update").
			Table("users").
			Context("user record not found during update - may have been deleted").
			RecordID(u.ID).
			Failure()
		return fmt.Errorf("user not found for update")
	}

	// Reconcile memberships without destroying history: a team the person was
	// taken out of becomes an inactive membership (kept), and a team they are
	// put back into is reactivated. Rewriting the whole set (delete, re-insert)
	// would erase inactive history and could resurrect an old membership.
	if err := qtx.DeactivateTeamMembershipsNotIn(ctx, sqlcgen.DeactivateTeamMembershipsNotInParams{UserID: u.ID, TeamIds: u.TeamIDs}); err != nil {
		return fmt.Errorf("failed to end team memberships: %w", err)
	}
	for _, teamID := range u.TeamIDs {
		if err := qtx.AddTeamMember(ctx, sqlcgen.AddTeamMemberParams{TeamID: teamID, UserID: u.ID}); err != nil {
			return fmt.Errorf("failed to save team membership: %w", err)
		}
	}

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
	qtx := r.queries.WithTx(tx)

	rowsAffected, err := qtx.DeactivateUser(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to deactivate user: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("user not found with provided ID")
	}

	if err := qtx.DeactivateTeamMembershipsForUser(ctx, id); err != nil {
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

	rowsAffected, err := r.queries.DeleteUser(ctx, id)
	if err != nil {
		log.DB("delete").
			Table("users").
			Context("removing user account from database").
			RecordID(id).
			Error(err).
			Failure()
		return fmt.Errorf("failed to delete user: %w", err)
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

	rows, err := r.queries.FindTeamIDsForUsers(ctx, userIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to batch-load team memberships: %w", err)
	}
	for _, row := range rows {
		result[row.UserID] = append(result[row.UserID], row.TeamID)
	}

	return result, nil
}

// FindTeamsWhereUserIsLead retrieves all team IDs where the user is a team lead
func (r *UserRepository) FindTeamsWhereUserIsLead(ctx context.Context, userID string) ([]string, error) {
	teamIDs, err := r.queries.FindTeamsWhereUserIsLead(ctx, sql.NullString{String: userID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("failed to query teams where user is lead: %w", err)
	}

	// Return empty slice instead of nil if no teams
	if teamIDs == nil {
		teamIDs = []string{}
	}

	return teamIDs, nil
}

// fetchTeamIDs is a helper function to get team IDs for a user
func (r *UserRepository) fetchTeamIDs(ctx context.Context, userID string) ([]string, error) {
	teamIDs, err := r.queries.FindTeamIDsForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query team memberships: %w", err)
	}

	// Return empty slice instead of nil if no teams
	if teamIDs == nil {
		teamIDs = []string{}
	}

	return teamIDs, nil
}

// UpdatePassword updates a user's password hash
func (r *UserRepository) UpdatePassword(ctx context.Context, userID string, hashedPassword string) error {
	rowsAffected, err := r.queries.UpdateUserPassword(ctx, sqlcgen.UpdateUserPasswordParams{
		PasswordHash: hashedPassword,
		UpdatedAt:    sql.NullTime{Time: time.Now(), Valid: true},
		ID:           userID,
	})
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
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
