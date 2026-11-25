package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	"github.com/agopalakrishnan/teams360/backend/domain/team"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres/sqlcgen"
)

// TeamRepository implements the team.Repository interface
type TeamRepository struct {
	db      *sql.DB
	queries *sqlcgen.Queries
}

// NewTeamRepository creates a new repository instance
func NewTeamRepository(db *sql.DB) team.Repository {
	return &TeamRepository{db: db, queries: sqlcgen.New(db)}
}

// buildTeamFromRow maps the scalar columns shared by FindTeamByID/FindAllTeams/
// FindTeamsByLeadID/FindTeamsBySupervisorID into a domain Team. Each caller
// still needs to hydrate Members/SupervisorChain/Tags itself.
func buildTeamFromRow(id, name string, teamLeadID sql.NullString, cadence sql.NullString, distributionListEmail sql.NullString, healthCheckEnabled bool, teamLeadName sql.NullString, createdAt, updatedAt sql.NullTime, isActive bool, deactivatedAt sql.NullTime) *team.Team {
	t := &team.Team{
		ID:                 id,
		Name:               name,
		HealthCheckEnabled: healthCheckEnabled,
		IsActive:           isActive,
	}

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
	if deactivatedAt.Valid {
		d := deactivatedAt.Time
		t.DeactivatedAt = &d
	}

	return t
}

// FindByID retrieves a team by ID
func (r *TeamRepository) FindByID(ctx context.Context, id string) (*team.Team, error) {
	row, err := r.queries.FindTeamByID(ctx, id)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("team not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find team: %w", err)
	}

	t := buildTeamFromRow(row.ID, row.Name, row.TeamLeadID, row.Cadence, row.DistributionListEmail, row.HealthCheckEnabled, row.TeamLeadName, row.CreatedAt, row.UpdatedAt, row.IsActive, row.DeactivatedAt)

	if err := r.hydrateTeam(ctx, t); err != nil {
		return nil, err
	}

	return t, nil
}

// FindAll retrieves all teams
func (r *TeamRepository) FindAll(ctx context.Context) ([]*team.Team, error) {
	return r.findByStatus(ctx, team.StatusAll)
}

// findByStatus loads teams, restricted in the query to the given status so rows
// the caller does not want are never read or scanned.
func (r *TeamRepository) findByStatus(ctx context.Context, status string) ([]*team.Team, error) {
	switch status {
	case team.StatusAll, team.StatusActive, team.StatusInactive:
	default:
		return nil, fmt.Errorf("unknown team status filter %q", status)
	}

	rows, err := r.queries.FindTeamsByStatus(ctx, status)
	if err != nil {
		return nil, fmt.Errorf("failed to query teams: %w", err)
	}

	teams := make([]*team.Team, 0, len(rows))
	for _, row := range rows {
		t := buildTeamFromRow(row.ID, row.Name, row.TeamLeadID, row.Cadence, row.DistributionListEmail, row.HealthCheckEnabled, row.TeamLeadName, row.CreatedAt, row.UpdatedAt, row.IsActive, row.DeactivatedAt)
		if err := r.hydrateTeam(ctx, t); err != nil {
			return nil, err
		}
		teams = append(teams, t)
	}

	return teams, nil
}

// FindAllWithDetails retrieves all teams with enriched details
func (r *TeamRepository) FindAllWithDetails(ctx context.Context) ([]team.Team, error) {
	return r.FindWithDetailsByStatus(ctx, team.StatusAll)
}

// FindWithDetailsByStatus retrieves teams with enriched details, filtering by
// active status in the query.
func (r *TeamRepository) FindWithDetailsByStatus(ctx context.Context, status string) ([]team.Team, error) {
	teams, err := r.findByStatus(ctx, status)
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
	rows, err := r.queries.FindTeamsByLeadID(ctx, sql.NullString{String: leadID, Valid: leadID != ""})
	if err != nil {
		return nil, fmt.Errorf("failed to query teams by lead: %w", err)
	}

	teams := make([]*team.Team, 0, len(rows))
	for _, row := range rows {
		t := buildTeamFromRow(row.ID, row.Name, row.TeamLeadID, row.Cadence, row.DistributionListEmail, row.HealthCheckEnabled, row.TeamLeadName, row.CreatedAt, row.UpdatedAt, row.IsActive, row.DeactivatedAt)
		if err := r.hydrateTeam(ctx, t); err != nil {
			return nil, err
		}
		teams = append(teams, t)
	}

	return teams, nil
}

// FindBySupervisorID retrieves all teams where a user is in the supervisor chain
func (r *TeamRepository) FindBySupervisorID(ctx context.Context, supervisorID string) ([]*team.Team, error) {
	rows, err := r.queries.FindTeamsBySupervisorID(ctx, supervisorID)
	if err != nil {
		return nil, fmt.Errorf("failed to query teams by supervisor: %w", err)
	}

	teams := make([]*team.Team, 0, len(rows))
	for _, row := range rows {
		t := buildTeamFromRow(row.ID, row.Name, row.TeamLeadID, row.Cadence, row.DistributionListEmail, row.HealthCheckEnabled, row.TeamLeadName, row.CreatedAt, row.UpdatedAt, row.IsActive, row.DeactivatedAt)
		if err := r.hydrateTeam(ctx, t); err != nil {
			return nil, err
		}
		teams = append(teams, t)
	}

	return teams, nil
}

// FindSurveyCompletionTeams returns a lightweight per-team projection for the
// admin survey-completion dashboard: this team's own closest supervisor, the
// Level-4 team lead, and the health_check_enabled column.
//
// The closest supervisor is whichever team_supervisors row for this team has
// the lowest `position` (that column's own definition: "1 = closest
// supervisor") — no hierarchy_levels.position filtering here at all, so this
// works regardless of how many leadership tiers a given organization's
// hierarchy_levels table defines. The caller
// (GetSurveyCompletionOverviewHandler) resolves everything above that one
// supervisor purely via users.reports_to.
//
// health_check_enabled is added by migration 000021_add_health_check_enabled
// — the migration must be applied first. Other methods on this repository
// (FindByID, FindAll, FindByLeadID, ...) also select it, so none of them
// work against a database that lacks the column.
//
// Only ACTIVE teams (teams.is_active) are returned. A deactivated team is
// not a live pod, so it must not be counted as opted in, appear in the
// dashboard's lists, or be a reminder candidate — whatever its
// health_check_enabled flag says. The filter lives here, in the one query
// every dashboard count, list and reminder plan is derived from, so none of
// them can disagree. Inactive rows are left untouched in the database.
//
// Deliberately does NOT return a member count: eligibility for the
// individual survey (Level 4 + Level 5 only) can't be answered from this
// query — see FindEligibleMemberIDs, the caller's actual source for that.
func (r *TeamRepository) FindSurveyCompletionTeams(ctx context.Context) ([]team.SurveyCompletionRow, error) {
	rows, err := r.queries.FindSurveyCompletionTeams(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query survey completion teams: %w", err)
	}

	result := make([]team.SurveyCompletionRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, team.SurveyCompletionRow{
			ID:                 row.ID,
			Name:               row.Name,
			HealthCheckEnabled: row.HealthCheckEnabled,
			SupervisorID:       row.SupervisorID,
			TeamLeadID:         row.TeamLeadID,
			TeamLeadName:       row.TeamLeadName,
			TeamLeadEmail:      row.TeamLeadEmail,
		})
	}

	return result, nil
}

// FindEligibleMemberIDs returns, for every team, the distinct ids of its
// members eligible to take the individual survey.
//
// AUTHORITATIVE SOURCE: team_members is the single source of truth for pod
// membership at every eligible level — Team Leads (Level 4) and Team
// Members (Level 5) are both inserted into team_members directly (a team's
// team_lead_id column names its lead for escalation/display purposes only;
// membership itself, for every level, lives in team_members). Eligibility
// is never inferred from a name, title, or any hard-coded level id/name: it
// is determined dynamically by joining each member's users.hierarchy_level_id
// to hierarchy_levels.position and requiring that position be exactly 4 or
// exactly 5. Managers (3), Senior Managers/Directors (2), and VPs (1) are
// excluded by this join, regardless of how their titles read.
//
// DISTINCT on (team_id, user_id) makes a user count at most once per team
// even if the caller's schema ever allowed a duplicate membership row (the
// team_members primary key already prevents that, but this query does not
// rely on that constraint alone). An inactive/missing user is excluded by
// the INNER JOIN to users: a team_members row whose user no longer exists
// never survives the join (the users FK's ON DELETE CASCADE means such a
// row cannot outlive its user in practice, but this query does not rely on
// that alone either). A soft-deactivated team is excluded too, so an Inactive
// team never receives surveys even if a member row is attached to it.
func (r *TeamRepository) FindEligibleMemberIDs(ctx context.Context) (map[string][]string, error) {
	rows, err := r.queries.FindEligibleMemberIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query eligible team members: %w", err)
	}

	result := make(map[string][]string)
	for _, row := range rows {
		result[row.TeamID] = append(result[row.TeamID], row.UserID)
	}

	return result, nil
}

// FindMembers retrieves team members as domain Members
func (r *TeamRepository) FindMembers(ctx context.Context, teamID string) ([]*team.Member, error) {
	userIDs, err := r.queries.FindTeamMemberUserIDs(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to query team members: %w", err)
	}

	members := make([]*team.Member, 0, len(userIDs))
	for _, userID := range userIDs {
		members = append(members, &team.Member{UserID: userID, Role: "member"})
	}

	return members, nil
}

// FindTeamMembers retrieves team members with full user details
func (r *TeamRepository) FindTeamMembers(ctx context.Context, teamID string) ([]team.TeamMember, error) {
	rows, err := r.queries.FindTeamMembersDetailed(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to query team members: %w", err)
	}

	members := make([]team.TeamMember, 0, len(rows))
	for _, row := range rows {
		members = append(members, team.TeamMember{
			ID:       row.ID,
			Username: row.Username,
			FullName: row.FullName,
			Email:    row.Email,
		})
	}

	return members, nil
}

// CountTeamMembers returns the number of members in a team
func (r *TeamRepository) CountTeamMembers(ctx context.Context, teamID string) (int, error) {
	count, err := r.queries.CountTeamMembers(ctx, teamID)
	if err != nil {
		return 0, fmt.Errorf("failed to count team members: %w", err)
	}

	return int(count), nil
}

// FindSupervisorChain retrieves the ordered supervisor chain for a team
func (r *TeamRepository) FindSupervisorChain(ctx context.Context, teamID string) ([]*team.SupervisorLink, error) {
	rows, err := r.queries.FindTeamSupervisorChain(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to query supervisor chain: %w", err)
	}

	supervisorChain := make([]*team.SupervisorLink, 0, len(rows))
	for _, row := range rows {
		supervisorChain = append(supervisorChain, &team.SupervisorLink{UserID: row.UserID, LevelID: row.HierarchyLevelID})
	}

	return supervisorChain, nil
}

// Save persists a new team
func (r *TeamRepository) Save(ctx context.Context, t *team.Team) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()
	qtx := r.queries.WithTx(tx)

	teamLeadID, cadence, distributionListEmail := teamWriteFields(t)

	now := time.Now()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	t.UpdatedAt = now

	if err := qtx.InsertTeam(ctx, sqlcgen.InsertTeamParams{
		ID:                    t.ID,
		Name:                  t.Name,
		TeamLeadID:            teamLeadID,
		Cadence:               cadence,
		DistributionListEmail: distributionListEmail,
		CreatedAt:             sql.NullTime{Time: t.CreatedAt, Valid: true},
		UpdatedAt:             sql.NullTime{Time: t.UpdatedAt, Valid: true},
	}); err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return fmt.Errorf("%w: %s", team.ErrDuplicateTeam, t.ID)
		}
		return fmt.Errorf("failed to save team: %w", err)
	}

	for _, member := range t.Members {
		if err := qtx.InsertTeamMember(ctx, sqlcgen.InsertTeamMemberParams{TeamID: t.ID, UserID: member.ID}); err != nil {
			return fmt.Errorf("failed to add team member: %w", err)
		}
	}

	if err := r.updateSupervisorChainTx(ctx, qtx, t.ID, supervisorChainPtrs(t)); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Update updates an existing team
func (r *TeamRepository) Update(ctx context.Context, t *team.Team) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()
	qtx := r.queries.WithTx(tx)

	teamLeadID, cadence, distributionListEmail := teamWriteFields(t)
	t.UpdatedAt = time.Now()

	rowsAffected, err := qtx.UpdateTeam(ctx, sqlcgen.UpdateTeamParams{
		Name:                  t.Name,
		TeamLeadID:            teamLeadID,
		Cadence:               cadence,
		DistributionListEmail: distributionListEmail,
		UpdatedAt:             sql.NullTime{Time: t.UpdatedAt, Valid: true},
		ID:                    t.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to update team: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("team not found: %s", t.ID)
	}

	// Reconcile members without destroying history: someone taken out of the
	// team becomes an inactive membership (kept), and someone put back is
	// reactivated. Deleting every row and re-inserting would erase the inactive
	// history and reset joined dates for everyone.
	memberIDs := make([]string, len(t.Members))
	for i, member := range t.Members {
		memberIDs[i] = member.ID
	}
	if err := qtx.DeactivateTeamMembersNotIn(ctx, sqlcgen.DeactivateTeamMembersNotInParams{TeamID: t.ID, UserIds: memberIDs}); err != nil {
		return fmt.Errorf("failed to end team memberships: %w", err)
	}

	for _, id := range memberIDs {
		if err := qtx.AddTeamMember(ctx, sqlcgen.AddTeamMemberParams{TeamID: t.ID, UserID: id}); err != nil {
			return fmt.Errorf("failed to add team member: %w", err)
		}
	}

	if err := r.updateSupervisorChainTx(ctx, qtx, t.ID, supervisorChainPtrs(t)); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Deactivate soft-deactivates a team. The teams row is kept, so the team's
// surveys, responses and action items keep resolving to it; its memberships end
// (kept as inactive history) in the same transaction. deactivated_at keeps its
// original value if the team was already inactive, so repeating the call is
// harmless. Team membership or team-lead changes never call this.
func (r *TeamRepository) Deactivate(ctx context.Context, id string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()
	qtx := r.queries.WithTx(tx)

	rowsAffected, err := qtx.DeactivateTeam(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to deactivate team: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("team not found: %s", id)
	}

	if err := qtx.DeactivateTeamMembersByTeamID(ctx, id); err != nil {
		return fmt.Errorf("failed to end memberships of deactivated team: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit team deactivation: %w", err)
	}
	return nil
}

// AddMember adds a member to a team
func (r *TeamRepository) AddMember(ctx context.Context, teamID, userID string) error {
	if err := r.queries.AddTeamMember(ctx, sqlcgen.AddTeamMemberParams{TeamID: teamID, UserID: userID}); err != nil {
		return fmt.Errorf("failed to add team member: %w", err)
	}

	return nil
}

// RemoveMember ends a member's membership of a team (soft: the row is kept as history)
func (r *TeamRepository) RemoveMember(ctx context.Context, teamID, userID string) error {
	rowsAffected, err := r.queries.RemoveTeamMember(ctx, sqlcgen.RemoveTeamMemberParams{TeamID: teamID, UserID: userID})
	if err != nil {
		return fmt.Errorf("failed to remove team member: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("team member not found: %s in team %s", userID, teamID)
	}

	return nil
}

// UpdateSupervisorChain updates the supervisor chain for a team
func (r *TeamRepository) UpdateSupervisorChain(ctx context.Context, teamID string, chain []*team.SupervisorLink) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if err := r.updateSupervisorChainTx(ctx, r.queries.WithTx(tx), teamID, chain); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Helper methods

// hydrateTeam fills in a Team's Members, SupervisorChain, and Tags.
func (r *TeamRepository) hydrateTeam(ctx context.Context, t *team.Team) error {
	members, err := r.FindTeamMembers(ctx, t.ID)
	if err != nil {
		return err
	}
	t.Members = members
	t.MemberCount = len(members)

	supervisorChain, err := r.FindSupervisorChain(ctx, t.ID)
	if err != nil {
		return err
	}
	t.SupervisorChain = make([]team.SupervisorLink, len(supervisorChain))
	for i, link := range supervisorChain {
		t.SupervisorChain[i] = *link
	}

	tags, err := r.fetchTags(ctx, t.ID)
	if err != nil {
		return err
	}
	t.Tags = tags

	return nil
}

// fetchTags is a helper function to get team tags
func (r *TeamRepository) fetchTags(ctx context.Context, teamID string) ([]string, error) {
	// For now, return empty slice since there's no team_tags table in the schema
	// This can be implemented later if needed
	return []string{}, nil
}

// teamWriteFields converts a domain Team's optional fields to NULL-able sqlc params.
func teamWriteFields(t *team.Team) (teamLeadID, cadence, distributionListEmail sql.NullString) {
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
	return
}

// supervisorChainPtrs converts a Team's []SupervisorLink to []*SupervisorLink.
func supervisorChainPtrs(t *team.Team) []*team.SupervisorLink {
	chainPtrs := make([]*team.SupervisorLink, len(t.SupervisorChain))
	for i := range t.SupervisorChain {
		chainPtrs[i] = &t.SupervisorChain[i]
	}
	return chainPtrs
}

// updateSupervisorChainTx updates supervisor chain within a transaction
func (r *TeamRepository) updateSupervisorChainTx(ctx context.Context, qtx *sqlcgen.Queries, teamID string, chain []*team.SupervisorLink) error {
	if err := qtx.DeleteTeamSupervisorsByTeamID(ctx, teamID); err != nil {
		return fmt.Errorf("failed to delete team supervisors: %w", err)
	}

	// Insert new supervisors (positions are 1-based per DB constraint)
	for i, supervisor := range chain {
		if err := qtx.InsertTeamSupervisor(ctx, sqlcgen.InsertTeamSupervisorParams{
			TeamID:           teamID,
			UserID:           supervisor.UserID,
			HierarchyLevelID: supervisor.LevelID,
			Position:         int32(i + 1),
		}); err != nil {
			return fmt.Errorf("failed to save team supervisor: %w", err)
		}
	}

	return nil
}
