// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/agopalakrishnan/teams360/backend/domain/team"
)

// TeamRepository implements the team.Repository interface
type TeamRepository struct {
	db *sql.DB
}

// NewTeamRepository creates a new repository instance
func NewTeamRepository(db *sql.DB) team.Repository {
	return &TeamRepository{db: db}
}

// FindByID retrieves a team by ID
func (r *TeamRepository) FindByID(ctx context.Context, id string) (*team.Team, error) {
	var t team.Team
	var teamLeadID sql.NullString
	var teamLeadName sql.NullString
	var cadence sql.NullString
	var distributionListEmail sql.NullString
	var createdAt, updatedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, `
		SELECT t.id, t.name, t.team_lead_id, t.cadence, t.created_at, t.updated_at, t.distribution_list_email, u.full_name
		FROM teams t
		LEFT JOIN users u ON t.team_lead_id = u.id
		WHERE t.id = $1
	`, id).Scan(
		&t.ID,
		&t.Name,
		&teamLeadID,
		&cadence,
		&createdAt,
		&updatedAt,
		&distributionListEmail,
		&teamLeadName,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("team not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find team: %w", err)
	}

	// Handle NULL fields
	if teamLeadID.Valid {
		t.TeamLeadID = &teamLeadID.String
	}
	if teamLeadName.Valid {
		t.TeamLeadName = &teamLeadName.String
	}
	if cadence.Valid {
		t.Cadence = cadence.String
	} else {
		t.Cadence = "monthly" // default
	}
	if distributionListEmail.Valid {
		t.DistributionListEmail = &distributionListEmail.String
	}
	if createdAt.Valid {
		t.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		t.UpdatedAt = updatedAt.Time
	}

	// Fetch team members
	members, err := r.FindTeamMembers(ctx, id)
	if err != nil {
		return nil, err
	}
	t.Members = members
	t.MemberCount = len(members)

	// Fetch supervisor chain
	supervisorChain, err := r.FindSupervisorChain(ctx, id)
	if err != nil {
		return nil, err
	}
	// Convert []*SupervisorLink to []SupervisorLink
	t.SupervisorChain = make([]team.SupervisorLink, len(supervisorChain))
	for i, link := range supervisorChain {
		t.SupervisorChain[i] = *link
	}

	// Fetch tags
	tags, err := r.fetchTags(ctx, id)
	if err != nil {
		return nil, err
	}
	t.Tags = tags

	return &t, nil
}

// FindAll retrieves all teams
func (r *TeamRepository) FindAll(ctx context.Context) ([]*team.Team, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT t.id, t.name, t.team_lead_id, t.cadence, t.created_at, t.updated_at, t.distribution_list_email, u.full_name
		FROM teams t
		LEFT JOIN users u ON t.team_lead_id = u.id
		ORDER BY t.name
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query teams: %w", err)
	}
	defer rows.Close()

	return r.scanTeams(ctx, rows)
}

// FindAllWithDetails retrieves all teams with enriched details
func (r *TeamRepository) FindAllWithDetails(ctx context.Context) ([]team.Team, error) {
	teams, err := r.FindAll(ctx)
	if err != nil {
		return nil, err
	}

	// Convert []*Team to []Team
	result := make([]team.Team, len(teams))
	for i, t := range teams {
		result[i] = *t
	}

	return result, nil
}

// FindByLeadID retrieves all teams led by a specific user
func (r *TeamRepository) FindByLeadID(ctx context.Context, leadID string) ([]*team.Team, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT t.id, t.name, t.team_lead_id, t.cadence, t.created_at, t.updated_at, t.distribution_list_email, u.full_name
		FROM teams t
		LEFT JOIN users u ON t.team_lead_id = u.id
		WHERE t.team_lead_id = $1
		ORDER BY t.name
	`, leadID)

	if err != nil {
		return nil, fmt.Errorf("failed to query teams by lead: %w", err)
	}
	defer rows.Close()

	return r.scanTeams(ctx, rows)
}

// FindBySupervisorID retrieves all teams where a user is in the supervisor chain
func (r *TeamRepository) FindBySupervisorID(ctx context.Context, supervisorID string) ([]*team.Team, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT t.id, t.name, t.team_lead_id, t.cadence, t.created_at, t.updated_at, t.distribution_list_email, u.full_name
		FROM teams t
		INNER JOIN team_supervisors ts ON t.id = ts.team_id
		LEFT JOIN users u ON t.team_lead_id = u.id
		WHERE ts.user_id = $1
		ORDER BY t.name
	`, supervisorID)

	if err != nil {
		return nil, fmt.Errorf("failed to query teams by supervisor: %w", err)
	}
	defer rows.Close()

	return r.scanTeams(ctx, rows)
}

// FindMembers retrieves team members as domain Members
func (r *TeamRepository) FindMembers(ctx context.Context, teamID string) ([]*team.Member, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT user_id
		FROM team_members
		WHERE team_id = $1
		ORDER BY user_id
	`, teamID)

	if err != nil {
		return nil, fmt.Errorf("failed to query team members: %w", err)
	}
	defer rows.Close()

	var members []*team.Member
	for rows.Next() {
		var member team.Member
		if err := rows.Scan(&member.UserID); err != nil {
			return nil, fmt.Errorf("failed to scan member: %w", err)
		}
		member.Role = "member" // Default role
		members = append(members, &member)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	// Return empty slice instead of nil
	if members == nil {
		members = []*team.Member{}
	}

	return members, nil
}

// FindTeamMembers retrieves team members with full user details
func (r *TeamRepository) FindTeamMembers(ctx context.Context, teamID string) ([]team.TeamMember, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT u.id, u.username, u.full_name, u.email
		FROM users u
		INNER JOIN team_members tm ON u.id = tm.user_id
		WHERE tm.team_id = $1
		ORDER BY u.username
	`, teamID)

	if err != nil {
		return nil, fmt.Errorf("failed to query team members: %w", err)
	}
	defer rows.Close()

	var members []team.TeamMember
	for rows.Next() {
		var member team.TeamMember
		if err := rows.Scan(&member.ID, &member.Username, &member.FullName, &member.Email); err != nil {
			return nil, fmt.Errorf("failed to scan team member: %w", err)
		}
		members = append(members, member)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	// Return empty slice instead of nil
	if members == nil {
		members = []team.TeamMember{}
	}

	return members, nil
}

// CountTeamMembers returns the number of members in a team
func (r *TeamRepository) CountTeamMembers(ctx context.Context, teamID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM team_members
		WHERE team_id = $1
	`, teamID).Scan(&count)

	if err != nil {
		return 0, fmt.Errorf("failed to count team members: %w", err)
	}

	return count, nil
}

// FindSupervisorChain retrieves the ordered supervisor chain for a team
func (r *TeamRepository) FindSupervisorChain(ctx context.Context, teamID string) ([]*team.SupervisorLink, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT user_id, hierarchy_level_id
		FROM team_supervisors
		WHERE team_id = $1
		ORDER BY position
	`, teamID)

	if err != nil {
		return nil, fmt.Errorf("failed to query supervisor chain: %w", err)
	}
	defer rows.Close()

	var supervisorChain []*team.SupervisorLink
	for rows.Next() {
		var supervisor team.SupervisorLink
		if err := rows.Scan(&supervisor.UserID, &supervisor.LevelID); err != nil {
			return nil, fmt.Errorf("failed to scan supervisor: %w", err)
		}
		supervisorChain = append(supervisorChain, &supervisor)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	// Return empty slice instead of nil
	if supervisorChain == nil {
		supervisorChain = []*team.SupervisorLink{}
	}

	return supervisorChain, nil
}

// Save persists a new team
func (r *TeamRepository) Save(ctx context.Context, t *team.Team) error {
	// Begin transaction
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Convert optional fields to NULL
	var teamLeadID, cadence, distributionListEmail sql.NullString
	if t.TeamLeadID != nil && *t.TeamLeadID != "" {
		teamLeadID = sql.NullString{String: *t.TeamLeadID, Valid: true}
	}
	if t.Cadence != "" {
		cadence = sql.NullString{String: t.Cadence, Valid: true}
	} else {
		cadence = sql.NullString{String: "monthly", Valid: true}
	}
	if t.DistributionListEmail != nil && *t.DistributionListEmail != "" {
		distributionListEmail = sql.NullString{String: *t.DistributionListEmail, Valid: true}
	}

	// Set timestamps
	now := time.Now()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	t.UpdatedAt = now

	// Insert team
	_, err = tx.ExecContext(ctx, `
		INSERT INTO teams (id, name, team_lead_id, cadence, distribution_list_email, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, t.ID, t.Name, teamLeadID, cadence, distributionListEmail, t.CreatedAt, t.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to save team: %w", err)
	}

	// Insert team members
	for _, member := range t.Members {
		err = r.addMemberTx(ctx, tx, t.ID, member.ID)
		if err != nil {
			return err
		}
	}

	// Insert supervisor chain
	// Convert []SupervisorLink to []*SupervisorLink
	chainPtrs := make([]*team.SupervisorLink, len(t.SupervisorChain))
	for i := range t.SupervisorChain {
		chainPtrs[i] = &t.SupervisorChain[i]
	}
	err = r.updateSupervisorChainTx(ctx, tx, t.ID, chainPtrs)
	if err != nil {
		return err
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Update updates an existing team
func (r *TeamRepository) Update(ctx context.Context, t *team.Team) error {
	// Begin transaction
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Convert optional fields to NULL
	var teamLeadID, cadence, distributionListEmail sql.NullString
	if t.TeamLeadID != nil && *t.TeamLeadID != "" {
		teamLeadID = sql.NullString{String: *t.TeamLeadID, Valid: true}
	}
	if t.Cadence != "" {
		cadence = sql.NullString{String: t.Cadence, Valid: true}
	} else {
		cadence = sql.NullString{String: "monthly", Valid: true}
	}
	if t.DistributionListEmail != nil && *t.DistributionListEmail != "" {
		distributionListEmail = sql.NullString{String: *t.DistributionListEmail, Valid: true}
	}

	// Update timestamp
	t.UpdatedAt = time.Now()

	// Update team
	result, err := tx.ExecContext(ctx, `
		UPDATE teams SET
			name = $1,
			team_lead_id = $2,
			cadence = $3,
			distribution_list_email = $4,
			updated_at = $5
		WHERE id = $6
	`, t.Name, teamLeadID, cadence, distributionListEmail, t.UpdatedAt, t.ID)

	if err != nil {
		return fmt.Errorf("failed to update team: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("team not found: %s", t.ID)
	}

	// Update team members
	// Delete existing members
	_, err = tx.ExecContext(ctx, "DELETE FROM team_members WHERE team_id = $1", t.ID)
	if err != nil {
		return fmt.Errorf("failed to delete team members: %w", err)
	}

	// Insert new members
	for _, member := range t.Members {
		err = r.addMemberTx(ctx, tx, t.ID, member.ID)
		if err != nil {
			return err
		}
	}

	// Update supervisor chain
	// Convert []SupervisorLink to []*SupervisorLink
	chainPtrs := make([]*team.SupervisorLink, len(t.SupervisorChain))
	for i := range t.SupervisorChain {
		chainPtrs[i] = &t.SupervisorChain[i]
	}
	err = r.updateSupervisorChainTx(ctx, tx, t.ID, chainPtrs)
	if err != nil {
		return err
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Delete removes a team
func (r *TeamRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM teams WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("failed to delete team: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("team not found: %s", id)
	}

	return nil
}

// AddMember adds a member to a team
func (r *TeamRepository) AddMember(ctx context.Context, teamID, userID string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO team_members (team_id, user_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, teamID, userID)

	if err != nil {
		return fmt.Errorf("failed to add team member: %w", err)
	}

	return nil
}

// RemoveMember removes a member from a team
func (r *TeamRepository) RemoveMember(ctx context.Context, teamID, userID string) error {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM team_members
		WHERE team_id = $1 AND user_id = $2
	`, teamID, userID)

	if err != nil {
		return fmt.Errorf("failed to remove team member: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("team member not found: %s in team %s", userID, teamID)
	}

	return nil
}

// UpdateSupervisorChain updates the supervisor chain for a team
func (r *TeamRepository) UpdateSupervisorChain(ctx context.Context, teamID string, chain []*team.SupervisorLink) error {
	// Begin transaction
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	err = r.updateSupervisorChainTx(ctx, tx, teamID, chain)
	if err != nil {
		return err
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Helper methods

// scanTeams is a helper function to scan query results into teams
func (r *TeamRepository) scanTeams(ctx context.Context, rows *sql.Rows) ([]*team.Team, error) {
	var teams []*team.Team

	for rows.Next() {
		var t team.Team
		var teamLeadID sql.NullString
		var teamLeadName sql.NullString
		var cadence sql.NullString
		var distributionListEmail sql.NullString
		var createdAt, updatedAt sql.NullTime

		err := rows.Scan(
			&t.ID,
			&t.Name,
			&teamLeadID,
			&cadence,
			&createdAt,
			&updatedAt,
			&distributionListEmail,
			&teamLeadName,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan team: %w", err)
		}

		// Handle NULL fields
		if teamLeadID.Valid {
			t.TeamLeadID = &teamLeadID.String
		}
		if teamLeadName.Valid {
			t.TeamLeadName = &teamLeadName.String
		}
		if cadence.Valid {
			t.Cadence = cadence.String
		} else {
			t.Cadence = "monthly"
		}
		if distributionListEmail.Valid {
			t.DistributionListEmail = &distributionListEmail.String
		}
		if createdAt.Valid {
			t.CreatedAt = createdAt.Time
		}
		if updatedAt.Valid {
			t.UpdatedAt = updatedAt.Time
		}

		// Fetch team members
		members, err := r.FindTeamMembers(ctx, t.ID)
		if err != nil {
			return nil, err
		}
		t.Members = members
		t.MemberCount = len(members)

		// Fetch supervisor chain
		supervisorChain, err := r.FindSupervisorChain(ctx, t.ID)
		if err != nil {
			return nil, err
		}
		// Convert []*SupervisorLink to []SupervisorLink
		t.SupervisorChain = make([]team.SupervisorLink, len(supervisorChain))
		for i, link := range supervisorChain {
			t.SupervisorChain[i] = *link
		}

		// Fetch tags
		tags, err := r.fetchTags(ctx, t.ID)
		if err != nil {
			return nil, err
		}
		t.Tags = tags

		teams = append(teams, &t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return teams, nil
}

// fetchTags is a helper function to get team tags
func (r *TeamRepository) fetchTags(ctx context.Context, teamID string) ([]string, error) {
	// For now, return empty slice since there's no team_tags table in the schema
	// This can be implemented later if needed
	return []string{}, nil
}

// addMemberTx adds a member within a transaction
func (r *TeamRepository) addMemberTx(ctx context.Context, tx *sql.Tx, teamID, userID string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO team_members (team_id, user_id)
		VALUES ($1, $2)
	`, teamID, userID)

	if err != nil {
		return fmt.Errorf("failed to add team member: %w", err)
	}

	return nil
}

// updateSupervisorChainTx updates supervisor chain within a transaction
func (r *TeamRepository) updateSupervisorChainTx(ctx context.Context, tx *sql.Tx, teamID string, chain []*team.SupervisorLink) error {
	// Delete existing supervisors
	_, err := tx.ExecContext(ctx, "DELETE FROM team_supervisors WHERE team_id = $1", teamID)
	if err != nil {
		return fmt.Errorf("failed to delete team supervisors: %w", err)
	}

	// Insert new supervisors (positions are 1-based per DB constraint)
	for i, supervisor := range chain {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position)
			VALUES ($1, $2, $3, $4)
		`, teamID, supervisor.UserID, supervisor.LevelID, i+1)
		if err != nil {
			return fmt.Errorf("failed to save team supervisor: %w", err)
		}
	}

	return nil
}
