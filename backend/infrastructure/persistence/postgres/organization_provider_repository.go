package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	"github.com/lib/pq"
)

// OrganizationProviderRepository implements orgprovider.Repository.
type OrganizationProviderRepository struct {
	db *sql.DB
}

// NewOrganizationProviderRepository creates a new repository instance.
func NewOrganizationProviderRepository(db *sql.DB) orgprovider.Repository {
	return &OrganizationProviderRepository{db: db}
}

// KnownHierarchyLevelIDs returns the level IDs configured in this deployment.
func (r *OrganizationProviderRepository) KnownHierarchyLevelIDs(ctx context.Context) (map[string]bool, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM hierarchy_levels`)
	if err != nil {
		return nil, fmt.Errorf("failed to query hierarchy levels: %w", err)
	}
	defer rows.Close()

	levels := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan hierarchy level: %w", err)
		}
		levels[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error reading hierarchy levels: %w", err)
	}

	return levels, nil
}

// ApplySnapshot writes a filtered, validated snapshot in one transaction:
// upsert every user/team the snapshot names, reconcile membership per team,
// and hard-delete any non-protected user/team the snapshot does not mention.
// Nothing here reuses UserRepository.Update/TeamRepository.Update -- those
// rebuild team_members from opposite directions (one deletes by user_id, the
// other by team_id), so driving both across a single sync would have each
// erase the other's writes, and neither can join a caller's transaction.
func (r *OrganizationProviderRepository) ApplySnapshot(ctx context.Context, in orgprovider.ApplyInput) (*orgprovider.ApplyResult, error) {
	if in.Snapshot == nil {
		return nil, fmt.Errorf("snapshot is required")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	result := &orgprovider.ApplyResult{}

	// deletionExemptUserIDs is who counts as "present" for deletion purposes:
	// every user actually in the filtered snapshot, PLUS every user the filter
	// skipped for bad/unknown data (PreservedMemberUserIDs). A skipped user is
	// not evidence the provider removed them -- it's evidence this sync couldn't
	// validate their record -- so they must be exempt from deletion for
	// exactly the same reason their team_members/reports_to rows are already
	// preserved elsewhere. Without this, a user who is merely malformed in
	// this one sync would be wrongly hard-deleted alongside anyone genuinely
	// absent.
	deletionExemptUserIDs := make(map[string]bool, len(in.Snapshot.Users)+len(in.PreservedMemberUserIDs))
	for _, u := range in.Snapshot.Users {
		deletionExemptUserIDs[u.ID] = true
	}
	for _, id := range in.PreservedMemberUserIDs {
		deletionExemptUserIDs[id] = true
	}
	snapshotTeamIDs := make(map[string]bool, len(in.Snapshot.Teams))
	for _, t := range in.Snapshot.Teams {
		snapshotTeamIDs[t.ID] = true
	}

	// The mass-deletion guard runs before any write in this transaction: it
	// must see the database exactly as it stood before this sync, and it must
	// block every subsequent step (including the read-only health-check-
	// transition count) if it trips.
	//
	// currentProtectedUserIDs is every user currently in the database that
	// IsProtectedUser recognizes (the permanent admin, any other level-admin
	// user, and the fixed demo/test/E2E fixtures). It is used below to make
	// sure a protected user's team_members rows survive reconciliation for a
	// *non*-protected team too -- protected status must hold everywhere the
	// user appears, not only on their own users row.
	missingUserIDs, currentUserCount, currentProtectedUserIDs, err := nonProtectedMissingIDs(ctx, tx, "users", deletionExemptUserIDs, isProtectedUserRow)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate user deletion scope: %w", err)
	}
	missingTeamIDs, currentTeamCount, _, err := nonProtectedMissingIDs(ctx, tx, "teams", snapshotTeamIDs, isProtectedTeamRow)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate team deletion scope: %w", err)
	}

	if err := orgprovider.EvaluateMassDeletionGuard(
		currentUserCount, len(missingUserIDs), currentTeamCount, len(missingTeamIDs), in.MaxDeletePercent,
	); err != nil {
		return nil, err
	}

	if err := countHealthCheckTransitions(ctx, tx, in, result); err != nil {
		return nil, err
	}
	if err := upsertSnapshotUsers(ctx, tx, in); err != nil {
		return nil, err
	}
	if err := applySnapshotReportsTo(ctx, tx, in); err != nil {
		return nil, err
	}
	if err := upsertSnapshotTeams(ctx, tx, in); err != nil {
		return nil, err
	}
	if err := replaceSnapshotMemberships(ctx, tx, in, currentProtectedUserIDs, result); err != nil {
		return nil, err
	}
	// Teams are deleted before users: a team's own action_items row is keyed by
	// team_id, so counting/removing it here rather than after the user pass
	// keeps the two loops independent of ordering between them.
	if err := deleteMissingTeams(ctx, tx, missingTeamIDs, result); err != nil {
		return nil, err
	}
	if err := deleteMissingUsers(ctx, tx, missingUserIDs, result); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit sync transaction: %w", err)
	}

	result.UsersSynced = len(in.Snapshot.Users)
	result.TeamsSynced = len(in.Snapshot.Teams)
	result.MembershipsSynced = len(in.Snapshot.Memberships)

	return result, nil
}

// isProtectedUserRow/isProtectedTeamRow adapt orgprovider's protection rules
// to the row shape read back from the database, so the protection rule is
// defined exactly once (in the orgprovider package) and reused here.
func isProtectedUserRow(id string, hierarchyLevelID sql.NullString) bool {
	return orgprovider.IsProtectedUser(id, hierarchyLevelID.String)
}

func isProtectedTeamRow(id string, _ sql.NullString) bool {
	return orgprovider.IsProtectedTeam(id)
}

// nonProtectedMissingIDs reads every id (and, for users, hierarchy_level_id)
// currently in the given table, and returns: the non-protected ids absent
// from the snapshot's id set, the total count of non-protected rows (the
// denominator for the mass-deletion guard), and the ids that ARE protected
// (so callers can shield a protected user's other references, e.g. team
// memberships, not just their own row).
func nonProtectedMissingIDs(
	ctx context.Context, tx *sql.Tx, table string, snapshotIDs map[string]bool,
	isProtected func(id string, hierarchyLevelID sql.NullString) bool,
) (missing []string, nonProtectedCount int, protected []string, err error) {
	query := "SELECT id, NULL::text FROM " + table
	if table == "users" {
		query = "SELECT id, hierarchy_level_id FROM users"
	}

	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, 0, nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var levelID sql.NullString
		if err := rows.Scan(&id, &levelID); err != nil {
			return nil, 0, nil, err
		}
		if isProtected(id, levelID) {
			protected = append(protected, id)
			continue
		}
		nonProtectedCount++
		if !snapshotIDs[id] {
			missing = append(missing, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, nil, err
	}

	return missing, nonProtectedCount, protected, nil
}

// countHealthCheckTransitions measures how many teams actually change state, so
// the sync can report "N teams disabled" rather than leaving it to be discovered.
func countHealthCheckTransitions(ctx context.Context, tx *sql.Tx, in orgprovider.ApplyInput, result *orgprovider.ApplyResult) error {
	var disabling, enabling []string
	for _, t := range in.Snapshot.Teams {
		if t.HealthCheckEnabled == nil {
			continue // omitted preserves the existing value
		}
		if *t.HealthCheckEnabled {
			enabling = append(enabling, t.ID)
		} else {
			disabling = append(disabling, t.ID)
		}
	}

	if len(disabling) > 0 {
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM teams WHERE id = ANY($1) AND health_check_enabled = true
		`, pq.Array(disabling)).Scan(&result.HealthChecksDisabled); err != nil {
			return fmt.Errorf("failed to count health check transitions: %w", err)
		}
	}

	if len(enabling) > 0 {
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM teams WHERE id = ANY($1) AND health_check_enabled = false
		`, pq.Array(enabling)).Scan(&result.HealthChecksEnabled); err != nil {
			return fmt.Errorf("failed to count health check transitions: %w", err)
		}
	}

	return nil
}

// upsertSnapshotUsers writes users with reports_to left alone. The column is a
// self-referencing foreign key, so manager links can only be set once every row
// in the snapshot exists -- see applySnapshotReportsTo.
//
// password_hash and auth_type are set on insert but never overwritten on
// conflict: a person who already signs in to THC locally must keep their
// credentials when they also appear in a provider snapshot.
//
// A snapshot record whose id collides with a protected user (the permanent
// admin, or a fixed demo/test/E2E fixture) is skipped entirely -- defense in
// depth alongside the deletion-scope exclusion, since the provider's real ids never
// take this shape in practice.
func upsertSnapshotUsers(ctx context.Context, tx *sql.Tx, in orgprovider.ApplyInput) error {
	for _, u := range in.Snapshot.Users {
		if orgprovider.IsProtectedUser(u.ID, u.HierarchyLevelID) {
			continue
		}

		_, err := tx.ExecContext(ctx, `
			INSERT INTO users (id, username, email, full_name, hierarchy_level_id, reports_to, password_hash, auth_type, updated_at)
			VALUES ($1, $2, $3, $4, $5, NULL, '', 'sso', CURRENT_TIMESTAMP)
			ON CONFLICT (id) DO UPDATE SET
				username = EXCLUDED.username,
				email = EXCLUDED.email,
				full_name = EXCLUDED.full_name,
				hierarchy_level_id = EXCLUDED.hierarchy_level_id,
				updated_at = CURRENT_TIMESTAMP
		`, u.ID, u.Username, u.Email, u.DisplayName, u.HierarchyLevelID)

		if err != nil {
			// users.username and users.email are UNIQUE, so a provider record
			// colliding with a different existing THC user surfaces here rather
			// than as a conflict on id. Name the record so it can be fixed.
			return fmt.Errorf("failed to upsert user %q (%s): %w", u.Username, u.ID, err)
		}
	}
	return nil
}

// applySnapshotReportsTo writes manager links now that every snapshot user exists.
func applySnapshotReportsTo(ctx context.Context, tx *sql.Tx, in orgprovider.ApplyInput) error {
	for _, u := range in.Snapshot.Users {
		if orgprovider.IsProtectedUser(u.ID, u.HierarchyLevelID) {
			continue
		}
		if in.PreserveReportsToUserIDs[u.ID] {
			// The provider named a manager we could not import. Treat that as
			// "no statement" and keep whatever THC already had, rather than
			// asserting this person now reports to nobody.
			continue
		}

		var reportsTo interface{}
		if u.ReportsToID != nil {
			reportsTo = *u.ReportsToID
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE users SET reports_to = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $1
		`, u.ID, reportsTo); err != nil {
			return fmt.Errorf("failed to set manager for user %q: %w", u.ID, err)
		}
	}
	return nil
}

// upsertSnapshotTeams writes teams. team_lead_id and health_check_enabled use the
// bound parameters rather than EXCLUDED so that a nil (field omitted by the
// provider) preserves the existing THC value for team_lead_id, per the
// contract's tri-state rule. healthCheckEnabled is NOT tri-state in practice
// against the deployed provider (it always sends true or false), and both
// values are always applied here -- false is never treated as "missing."
func upsertSnapshotTeams(ctx context.Context, tx *sql.Tx, in orgprovider.ApplyInput) error {
	for _, t := range in.Snapshot.Teams {
		if orgprovider.IsProtectedTeam(t.ID) {
			continue
		}

		var teamLead interface{}
		if t.TeamLeadID != nil {
			teamLead = *t.TeamLeadID
		}
		var healthCheck interface{}
		if t.HealthCheckEnabled != nil {
			healthCheck = *t.HealthCheckEnabled
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO teams (id, name, team_lead_id, health_check_enabled, updated_at)
			VALUES ($1, $2, $3, COALESCE($4::boolean, true), CURRENT_TIMESTAMP)
			ON CONFLICT (id) DO UPDATE SET
				name = EXCLUDED.name,
				team_lead_id = COALESCE($3::varchar, teams.team_lead_id),
				health_check_enabled = COALESCE($4::boolean, teams.health_check_enabled),
				updated_at = CURRENT_TIMESTAMP
		`, t.ID, t.Name, teamLead, healthCheck); err != nil {
			return fmt.Errorf("failed to upsert team %q (%s): %w", t.Name, t.ID, err)
		}
	}
	return nil
}

// replaceSnapshotMemberships makes the provider authoritative for the
// membership of every non-protected team it reports, while protecting rows
// belonging to users the sync could not import AND rows belonging to any
// protected user (currentProtectedUserIDs) -- a protected user's membership
// must survive reconciliation for a team it belongs to even when that team
// itself is not protected. This is scoped per team: a team's membership is
// reconciled to exactly match that team's entries in memberships[], including
// clearing every membership when a team is returned with zero of them. A team
// absent from the snapshot entirely is never visited here -- it is handled by
// deleteMissingTeams below, whose cascade removes its team_members rows.
func replaceSnapshotMemberships(ctx context.Context, tx *sql.Tx, in orgprovider.ApplyInput, currentProtectedUserIDs []string, result *orgprovider.ApplyResult) error {
	membersByTeam := make(map[string][]string, len(in.Snapshot.Teams))
	for _, t := range in.Snapshot.Teams {
		if orgprovider.IsProtectedTeam(t.ID) {
			continue
		}
		membersByTeam[t.ID] = nil
	}
	for _, m := range in.Snapshot.Memberships {
		if _, tracked := membersByTeam[m.TeamID]; !tracked {
			continue // protected team, or a team not in the snapshot's own teams[] (already rejected by validation)
		}
		membersByTeam[m.TeamID] = append(membersByTeam[m.TeamID], m.UserID)
	}

	for teamID, members := range membersByTeam {
		keep := append([]string{}, members...)
		keep = append(keep, in.PreservedMemberUserIDs...)
		keep = append(keep, currentProtectedUserIDs...)

		res, err := tx.ExecContext(ctx, `
			DELETE FROM team_members WHERE team_id = $1 AND user_id <> ALL($2)
		`, teamID, pq.Array(keep))
		if err != nil {
			return fmt.Errorf("failed to prune members of team %q: %w", teamID, err)
		}
		if removed, err := res.RowsAffected(); err == nil {
			result.MembershipsRemoved += int(removed)
		}

		for _, userID := range members {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO team_members (team_id, user_id) VALUES ($1, $2)
				ON CONFLICT (team_id, user_id) DO NOTHING
			`, teamID, userID); err != nil {
				return fmt.Errorf("failed to add user %q to team %q: %w", userID, teamID, err)
			}
		}
	}

	return nil
}

// deleteMissingTeams hard-deletes every non-protected team absent from the
// snapshot -- unconditionally, per the approved "the data provider is authoritative"
// rule. Nothing here holds a team back.
//
// team_members and team_supervisors cascade from teams.id (see
// migrations/000005_create_users_and_teams), so their rows for a deleted team
// are removed automatically and intentionally -- verified foreign-key
// behavior, not an assumption. action_items.team_id also cascades (see
// migrations/000020_create_action_items); those rows are REQUIRED to survive
// nothing -- they are deleted along with the team, and counted into
// ActionItemsDeleted purely so an admin can see it happened, not to gate it.
// health_check_sessions/responses have no foreign key to teams at all
// (migrations/000013_add_security_constraints defers it deliberately), so
// they are never touched by this delete -- that preservation is required and
// unconditional, unlike action_items.
func deleteMissingTeams(ctx context.Context, tx *sql.Tx, missingTeamIDs []string, result *orgprovider.ApplyResult) error {
	for _, teamID := range missingTeamIDs {
		var actionItemCount int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM action_items WHERE team_id = $1
		`, teamID).Scan(&actionItemCount); err != nil {
			return fmt.Errorf("failed to check action items for team %q: %w", teamID, err)
		}

		if _, err := tx.ExecContext(ctx, `DELETE FROM teams WHERE id = $1`, teamID); err != nil {
			return fmt.Errorf("failed to delete team %q: %w", teamID, err)
		}
		result.TeamsDeleted++
		result.ActionItemsDeleted += actionItemCount
	}
	return nil
}

// deleteMissingUsers hard-deletes every non-protected user absent from the
// snapshot -- unconditionally, per the approved "the data provider is authoritative"
// rule. Nothing here holds a user back.
//
// users.reports_to and teams.team_lead_id both use ON DELETE SET NULL
// (migrations/000005), so deleting a manager or team lead safely clears those
// references rather than leaving them dangling; team_members and
// password_reset_tokens cascade (migrations/000005, 000012).
// action_items.created_by also cascades (migrations/000020); those rows are
// deleted along with the user and counted into ActionItemsDeleted for
// visibility only, not held back.
//
// health_check_sessions.user_id has no foreign key at all -- deliberately
// deferred in migrations/000013_add_security_constraints ("application-level
// validation ensures referential integrity" for now). Deleting a user
// therefore REQUIRES no cascade into health-check history -- there is none to
// cascade -- but a session row referencing a deleted user becomes an orphaned
// reference (a user_id with no matching users row), exactly as it already
// could before this feature existed. That gap is pre-existing and out of
// scope for this change.
func deleteMissingUsers(ctx context.Context, tx *sql.Tx, missingUserIDs []string, result *orgprovider.ApplyResult) error {
	for _, userID := range missingUserIDs {
		var actionItemCount int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM action_items WHERE created_by = $1
		`, userID).Scan(&actionItemCount); err != nil {
			return fmt.Errorf("failed to check action items for user %q: %w", userID, err)
		}

		if _, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID); err != nil {
			return fmt.Errorf("failed to delete user %q: %w", userID, err)
		}
		result.UsersDeleted++
		result.ActionItemsDeleted += actionItemCount
	}
	return nil
}
