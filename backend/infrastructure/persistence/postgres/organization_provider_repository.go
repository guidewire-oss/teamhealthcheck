package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	"github.com/agopalakrishnan/teams360/backend/domain/orgsyncrun"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres/sqlcgen"
	"github.com/agopalakrishnan/teams360/backend/pkg/orgsnapshot"
)

// OrganizationProviderRepository implements orgprovider.Repository.
type OrganizationProviderRepository struct {
	db      *sql.DB
	queries *sqlcgen.Queries
}

// NewOrganizationProviderRepository creates a new repository instance.
func NewOrganizationProviderRepository(db *sql.DB) orgprovider.Repository {
	return &OrganizationProviderRepository{db: db, queries: sqlcgen.New(db)}
}

// KnownHierarchyLevelIDs returns the level IDs configured in this deployment.
func (r *OrganizationProviderRepository) KnownHierarchyLevelIDs(ctx context.Context) (map[string]bool, error) {
	ids, err := r.queries.GetHierarchyLevelIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query hierarchy levels: %w", err)
	}

	levels := make(map[string]bool, len(ids))
	for _, id := range ids {
		levels[id] = true
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
	qtx := r.queries.WithTx(tx)

	result := &orgprovider.ApplyResult{}

	// Newest-wins: the singleton settings row is locked for the rest of the
	// transaction, so two applies serialize here and an older snapshot cannot
	// overwrite a newer one that committed first.
	if err := rejectStaleSnapshot(ctx, qtx, in.Snapshot.GeneratedAt); err != nil {
		return nil, err
	}

	complete := in.SnapshotType == orgprovider.SnapshotComplete

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

	// Runs before the deletion scope is measured so a re-keyed user is not
	// counted as missing. A held sync still writes nothing: the deferred
	// Rollback discards these re-keys along with everything else. Only the
	// snapshot and preserved ids are in the set so far; the supervisor
	// exemptions below apply to deletion only.
	if err := rekeyUsersToProviderIDs(ctx, qtx, in, deletionExemptUserIDs); err != nil {
		return nil, err
	}

	// Folding in today's protected-chain supervisors keeps the mass-deletion
	// guard's count (missingUserIDs, below) in agreement with what
	// deleteMissingUsers will actually remove: that function independently,
	// atomically re-checks the same condition at the moment of each delete
	// (see its comment), so a chain edit racing the sync can only ever widen
	// the gap between "counted here" and "deleted there" in the safe
	// direction -- fewer rows removed than the guard estimated, never more.
	// Without this, the guard would count these users as deletions it will
	// never actually perform, producing false holds and override
	// confirmations that don't match the sync's real effect.
	supervisorExemptUserIDs, err := protectedTeamSupervisorUserIDs(ctx, qtx)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate protected supervisor chains: %w", err)
	}
	for id := range supervisorExemptUserIDs {
		deletionExemptUserIDs[id] = true
	}
	snapshotTeamIDs := make(map[string]bool, len(in.Snapshot.Teams))
	for _, t := range in.Snapshot.Teams {
		snapshotTeamIDs[t.ID] = true
	}

	// reconciledTeamIDs are the non-protected teams present in the snapshot --
	// exactly the teams replaceSnapshotMemberships explicitly reconciles below.
	// A missing user's row in one of these teams is removed by that team's
	// explicit DELETE (and counted in ApplyResult.MembershipsRemoved), not by
	// an FK cascade; membershipCascadeMetric needs this set to avoid counting
	// those rows twice, once correctly and once mislabeled as "cascaded".
	reconciledTeamIDs := make([]string, 0, len(in.Snapshot.Teams))
	for _, t := range in.Snapshot.Teams {
		if !orgprovider.IsProtectedTeam(t.ID) {
			reconciledTeamIDs = append(reconciledTeamIDs, t.ID)
		}
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
	missingUserIDs, currentUserCount, currentProtectedUserIDs, err := nonProtectedMissingUserIDs(ctx, qtx, deletionExemptUserIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate user deactivation scope: %w", err)
	}
	missingTeamIDs, currentTeamCount, _, err := nonProtectedMissingTeamIDs(ctx, qtx, snapshotTeamIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate team deactivation scope: %w", err)
	}
	if !complete {
		// A partial payload says nothing about who is absent from the source.
		missingUserIDs, missingTeamIDs = nil, nil
	}

	// The guard runs identically whether or not an override was requested --
	// the override only changes what happens to its verdict. Either way the
	// counts are measured, so an overridden sync can be audited with the same
	// numbers the admin was shown when it was held.
	if err := orgprovider.EvaluateMassDeletionGuard(
		currentUserCount, len(missingUserIDs), currentTeamCount, len(missingTeamIDs), in.MaxDeletePercent,
	); err != nil {
		var hold *orgprovider.MassDeletionHoldError
		if !errors.As(err, &hold) {
			return nil, err
		}

		// Attach the membership cascade for the admin's review. This is read-
		// only and informational: memberships never contribute to the hold (see
		// DeletionMetric.ContributesToHold), so measuring them cannot change the
		// verdict already reached above.
		memberships, mErr := membershipCascadeMetric(ctx, qtx, missingUserIDs, missingTeamIDs, reconciledTeamIDs, in.MaxDeletePercent)
		if mErr != nil {
			return nil, fmt.Errorf("failed to measure membership cascade: %w", mErr)
		}
		hold.Report.Memberships = memberships

		// What the provider actually sent, so an admin can tell an incomplete
		// payload ("237 teams would go, but the provider only sent 6") apart
		// from a real mass departure. Reporting only -- the verdict above is
		// already decided. These are the raw, pre-filter counts (in.IncomingUsers
		// etc.) rather than len(in.Snapshot.Users) etc. -- in.Snapshot only holds
		// what survived filtering, so using it here would undercount a payload
		// where most records were sent but skipped for a bad hierarchy level.
		hold.Report.WithIncoming(
			in.IncomingUsers, in.IncomingTeams, in.IncomingMemberships,
		)

		if !in.OverrideMassDeletion || !hold.Report.MatchesConfirmed(in.ConfirmedMassDeletion) {
			// Either no override was requested, or the provider's data changed
			// between the held response and this confirmation -- ConfirmedMassDeletion
			// no longer matches what the guard just recomputed. Either way the admin
			// has not reviewed these exact numbers, so nothing is written and a fresh
			// hold (with the current counts) is returned for them to confirm again.
			// Nothing has been written yet and the deferred Rollback discards
			// the read-only transaction, so a held sync leaves THC untouched.
			return nil, hold
		}

		waived := hold.Report
		result.MassDeletionOverride = &waived
	}

	currentProtectedUserIDSet := make(map[string]bool, len(currentProtectedUserIDs))
	for _, id := range currentProtectedUserIDs {
		currentProtectedUserIDSet[id] = true
	}

	if err := countHealthCheckTransitions(ctx, qtx, in, result); err != nil {
		return nil, err
	}
	stage := func(name string) {
		if in.OnStage != nil {
			in.OnStage(name)
		}
	}

	track := newChangeTracker()

	stage(string(orgsyncrun.StageUpsertingUsers))
	if err := upsertSnapshotUsers(ctx, qtx, in, currentProtectedUserIDSet, track); err != nil {
		return nil, err
	}
	if err := applySnapshotReportsTo(ctx, qtx, in, currentProtectedUserIDSet, track); err != nil {
		return nil, err
	}
	stage(string(orgsyncrun.StageUpsertingTeams))
	if err := upsertSnapshotTeams(ctx, qtx, in, track); err != nil {
		return nil, err
	}
	stage(string(orgsyncrun.StageSyncingMemberships))
	if err := reconcileSnapshotMemberships(ctx, qtx, in, currentProtectedUserIDSet, complete, result); err != nil {
		return nil, err
	}
	stage(string(orgsyncrun.StageReconcilingRemovals))
	if complete {
		if err := deactivateMissingTeams(ctx, qtx, missingTeamIDs, result); err != nil {
			return nil, err
		}
		if err := deactivateMissingUsers(ctx, qtx, missingUserIDs, result); err != nil {
			return nil, err
		}
	}
	if err := reconcileActiveStates(ctx, qtx, in, currentProtectedUserIDSet, track, result); err != nil {
		return nil, err
	}
	// Last write: the THC-only pod-lead mapping reads the teams as they are now
	// that everything else is in place. See applyPodLeadLevels.
	if err := applyPodLeadLevels(ctx, qtx, in, currentProtectedUserIDSet, track, result); err != nil {
		return nil, err
	}
	// Truly last: a valid podiqRole is the final level decision for its user.
	if err := applyPodIQRoleLevels(ctx, qtx, in, currentProtectedUserIDSet, track); err != nil {
		return nil, err
	}
	track.finish(result)
	if err := recordAppliedSnapshot(ctx, qtx, in.Snapshot.GeneratedAt); err != nil {
		return nil, err
	}

	// Reported BEFORE the run row is locked below. The stage update runs on a
	// different connection and needs that same row, so announcing it after
	// markRunSucceeded would wait on this very transaction while this
	// transaction waits for the announcement: a self-deadlock.
	stage(string(orgsyncrun.StageCommitting))
	if in.RunID != "" {
		if err := markRunSucceeded(ctx, qtx, in, result); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit sync transaction: %w", err)
	}

	result.UsersSynced = len(in.Snapshot.Users)
	result.TeamsSynced = len(in.Snapshot.Teams)

	return result, nil
}

// nonProtectedMissingUserIDs / nonProtectedMissingTeamIDs read every active id
// (and, for users, hierarchy_level_id) and return: the non-protected ids absent
// from the snapshot's id set, the total count of non-protected rows (the
// denominator for the mass-deletion guard), and the ids that ARE protected
// (so callers can shield a protected user's other references, e.g. team
// memberships, not just their own row). The protection rules are defined once,
// in the orgprovider package, and reused here.
//
// Already-inactive rows are neither candidates nor part of the denominator:
// deactivating them again is a no-op and must not skew the guard.
func nonProtectedMissingUserIDs(ctx context.Context, q *sqlcgen.Queries, snapshotIDs map[string]bool) (missing []string, nonProtectedCount int, protected []string, err error) {
	rows, err := q.FindActiveUserIDsAndLevels(ctx)
	if err != nil {
		return nil, 0, nil, err
	}
	for _, row := range rows {
		if orgprovider.IsProtectedUser(row.ID, row.HierarchyLevelID) {
			protected = append(protected, row.ID)
			continue
		}
		nonProtectedCount++
		if !snapshotIDs[row.ID] {
			missing = append(missing, row.ID)
		}
	}
	return missing, nonProtectedCount, protected, nil
}

func nonProtectedMissingTeamIDs(ctx context.Context, q *sqlcgen.Queries, snapshotIDs map[string]bool) (missing []string, nonProtectedCount int, protected []string, err error) {
	ids, err := q.FindActiveTeamIDs(ctx)
	if err != nil {
		return nil, 0, nil, err
	}
	for _, id := range ids {
		if orgprovider.IsProtectedTeam(id) {
			protected = append(protected, id)
			continue
		}
		nonProtectedCount++
		if !snapshotIDs[id] {
			missing = append(missing, id)
		}
	}
	return missing, nonProtectedCount, protected, nil
}

// membershipCascadeMetric counts the team_members rows the proposed user and
// team deletions would take with them. These rows are removed by the database
// itself (team_members cascades from both users.id and teams.id -- see
// migrations/000005_create_users_and_teams), not by a delete this sync issues,
// so the metric is labelled DeletionKindCascaded and never contributes to the
// hold. It deliberately does NOT include memberships that per-team
// reconciliation would prune, which is a different operation measured after
// the fact as ApplyResult.MembershipsRemoved.
//
// The denominator is every team_members row currently in THC, which is the
// honest "of how many" for a cascade that can reach any of them.
// membershipCascadeMetric measures only rows a hard-delete removes as an FK
// cascade, per DeletionKindCascaded's own definition -- never a row
// replaceSnapshotMemberships would prune explicitly through its per-team
// reconciliation DELETE (counted separately in ApplyResult.MembershipsRemoved).
// A missing team's rows always qualify: the team itself is being hard-deleted,
// and nothing reconciles a team that isn't in the snapshot. A missing user's
// row only qualifies when its team is NOT one reconciledTeamIDs names --
// otherwise that row is removed by the explicit DELETE, not by cascade, and
// counting it here would double-label a pruned row as cascaded.
func membershipCascadeMetric(
	ctx context.Context, q *sqlcgen.Queries, missingUserIDs, missingTeamIDs, reconciledTeamIDs []string, maxPercent float64,
) (*orgprovider.DeletionMetric, error) {
	existing, err := q.CountAllTeamMemberships(ctx)
	if err != nil {
		return nil, err
	}

	var cascaded int64
	if len(missingUserIDs) > 0 || len(missingTeamIDs) > 0 {
		cascaded, err = q.CountCascadedTeamMemberships(ctx, sqlcgen.CountCascadedTeamMembershipsParams{
			MissingTeamIds:    missingTeamIDs,
			MissingUserIds:    missingUserIDs,
			ReconciledTeamIds: reconciledTeamIDs,
		})
		if err != nil {
			return nil, err
		}
	}

	metric := orgprovider.NewDeletionMetric(orgprovider.DeletionKindCascaded, int(existing), int(cascaded), maxPercent, false)
	return &metric, nil
}

// countHealthCheckTransitions measures how many teams actually change state, so
// the sync can report "N teams disabled" rather than leaving it to be discovered.
func countHealthCheckTransitions(ctx context.Context, q *sqlcgen.Queries, in orgprovider.ApplyInput, result *orgprovider.ApplyResult) error {
	var disabling, enabling []string
	for _, t := range in.Snapshot.Teams {
		if orgprovider.IsProtectedTeam(t.ID) {
			continue // upsertSnapshotTeams never writes a protected team, so it can never transition
		}
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
		n, err := q.CountTeamsDisablingHealthCheck(ctx, disabling)
		if err != nil {
			return fmt.Errorf("failed to count health check transitions: %w", err)
		}
		result.HealthChecksDisabled = int(n)
	}

	if len(enabling) > 0 {
		n, err := q.CountTeamsEnablingHealthCheck(ctx, enabling)
		if err != nil {
			return fmt.Errorf("failed to count health check transitions: %w", err)
		}
		result.HealthChecksEnabled = int(n)
	}

	return nil
}

// userIDReferences lists every column that holds a users.id. Re-keying a user
// must repoint all of them before the old row goes: the foreign keys cascade or
// null on delete, so removing the old row while anything still pointed at it
// would silently drop the user's memberships and action items.
// health_check_sessions.user_id has no foreign key but identifies the
// submitter, so it moves with the user too.
var userIDReferences = []struct {
	table, column string
	repoint       func(ctx context.Context, q *sqlcgen.Queries, oldID, newID string) error
}{
	{"team_members", "user_id", func(ctx context.Context, q *sqlcgen.Queries, oldID, newID string) error {
		return q.RepointTeamMembersUser(ctx, sqlcgen.RepointTeamMembersUserParams{OldID: oldID, NewID: newID})
	}},
	{"team_supervisors", "user_id", func(ctx context.Context, q *sqlcgen.Queries, oldID, newID string) error {
		return q.RepointTeamSupervisorsUser(ctx, sqlcgen.RepointTeamSupervisorsUserParams{OldID: oldID, NewID: newID})
	}},
	{"action_items", "created_by", func(ctx context.Context, q *sqlcgen.Queries, oldID, newID string) error {
		return q.RepointActionItemsCreatedBy(ctx, sqlcgen.RepointActionItemsCreatedByParams{OldID: oldID, NewID: newID})
	}},
	{"action_items", "assigned_to", func(ctx context.Context, q *sqlcgen.Queries, oldID, newID string) error {
		return q.RepointActionItemsAssignedTo(ctx, sqlcgen.RepointActionItemsAssignedToParams{
			OldID: sql.NullString{String: oldID, Valid: true}, NewID: sql.NullString{String: newID, Valid: true},
		})
	}},
	{"password_reset_tokens", "user_id", func(ctx context.Context, q *sqlcgen.Queries, oldID, newID string) error {
		return q.RepointPasswordResetTokensUser(ctx, sqlcgen.RepointPasswordResetTokensUserParams{OldID: oldID, NewID: newID})
	}},
	{"teams", "team_lead_id", func(ctx context.Context, q *sqlcgen.Queries, oldID, newID string) error {
		return q.RepointTeamsLead(ctx, sqlcgen.RepointTeamsLeadParams{
			OldID: sql.NullString{String: oldID, Valid: true}, NewID: sql.NullString{String: newID, Valid: true},
		})
	}},
	{"users", "reports_to", func(ctx context.Context, q *sqlcgen.Queries, oldID, newID string) error {
		return q.RepointUsersReportsTo(ctx, sqlcgen.RepointUsersReportsToParams{
			OldID: sql.NullString{String: oldID, Valid: true}, NewID: sql.NullString{String: newID, Valid: true},
		})
	}},
	{"health_check_sessions", "user_id", func(ctx context.Context, q *sqlcgen.Queries, oldID, newID string) error {
		return q.RepointHealthCheckSessionsUser(ctx, sqlcgen.RepointHealthCheckSessionsUserParams{OldID: oldID, NewID: newID})
	}},
}

// UserIDReferenceColumns lists the "table.column" pairs re-keying repoints, so a
// test can check the list against the foreign keys on users(id).
func UserIDReferenceColumns() []string {
	cols := make([]string, len(userIDReferences))
	for i, ref := range userIDReferences {
		cols[i] = ref.table + "." + ref.column
	}
	return cols
}

// rekeyUsersToProviderIDs moves an existing SSO user onto the id the provider
// now sends for them. upsertSnapshotUsers matches on id only, so a person
// stored under a different id (for example their username) would be inserted
// as a second row and rejected by the unique email/username constraints.
//
// A snapshot user is matched to an existing row by email (case-insensitive) or
// username. Only SSO rows are matched, never the permanent admin or fixture
// ids, and never an id in stillPresent: the provider still sends that user, so
// their history stays with them. A match on more than one row aborts the sync.
// Administrators are re-keyed too and keep their level, because the row is
// protected from the upsert afterwards.
func rekeyUsersToProviderIDs(ctx context.Context, q *sqlcgen.Queries, in orgprovider.ApplyInput, stillPresent map[string]bool) error {
	existing, err := existingUserIDs(ctx, q, in.Snapshot.Users)
	if err != nil {
		return err
	}

	for _, u := range in.Snapshot.Users {
		if existing[u.ID] || orgprovider.IsProtectedUser(u.ID, u.HierarchyLevelID) {
			continue
		}

		oldIDs, err := findRekeyCandidates(ctx, q, u.Email, u.Username, stillPresent)
		if err != nil {
			return fmt.Errorf("failed to look up existing user for %q (%s): %w", u.Username, u.ID, err)
		}
		if len(oldIDs) > 1 {
			return fmt.Errorf("provider user %q (%s) matches several existing users %v by email or username; resolve the duplicates before syncing", u.Username, u.ID, oldIDs)
		}
		if len(oldIDs) == 1 {
			if err := rekeyUser(ctx, q, oldIDs[0], u); err != nil {
				return fmt.Errorf("failed to re-key user %q from %q to %q: %w", u.Username, oldIDs[0], u.ID, err)
			}
		}
	}
	return nil
}

// existingUserIDs returns which of the snapshot users already have a row under
// the id the provider sends, in one query.
func existingUserIDs(ctx context.Context, q *sqlcgen.Queries, users []orgsnapshot.User) (map[string]bool, error) {
	ids := make([]string, len(users))
	for i, u := range users {
		ids[i] = u.ID
	}
	found, err := q.FindExistingUserIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to look up existing users: %w", err)
	}

	existing := make(map[string]bool, len(found))
	for _, id := range found {
		existing[id] = true
	}
	return existing, nil
}

// findRekeyCandidates returns the ids of SSO users that share the given email
// or username. Administrators are included, since they are real people whose
// row should follow their provider id. The permanent admin account, the fixed
// demo/test fixtures and any row whose id is still in the snapshot
// (stillPresent) are excluded. Passing an empty level to IsProtectedUser checks
// the id only.
func findRekeyCandidates(ctx context.Context, q *sqlcgen.Queries, email, username string, stillPresent map[string]bool) ([]string, error) {
	found, err := q.FindSSOUserIDsByEmailOrUsername(ctx, sqlcgen.FindSSOUserIDsByEmailOrUsernameParams{Email: email, Username: username})
	if err != nil {
		return nil, err
	}

	var ids []string
	for _, id := range found {
		if stillPresent[id] || orgprovider.IsProtectedUser(id, "") {
			continue
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// rekeyUser gives the user stored under oldID the provider's id. users.id is
// referenced by foreign keys with no ON UPDATE CASCADE, so this takes four
// steps: free the old row's unique email and username, insert the replacement
// (keeping the stored hierarchy level, password_hash, auth_type and
// created_at), repoint every reference, then delete the old row, which nothing
// points at by then.
func rekeyUser(ctx context.Context, q *sqlcgen.Queries, oldID string, u orgsnapshot.User) error {
	if err := q.FreeUserCredentialsForRekey(ctx, oldID); err != nil {
		return err
	}
	if err := q.CopyUserUnderNewID(ctx, sqlcgen.CopyUserUnderNewIDParams{
		OldID: oldID, NewID: u.ID, Username: u.Username, Email: u.Email, FullName: u.DisplayName,
	}); err != nil {
		return err
	}
	for _, ref := range userIDReferences {
		if err := ref.repoint(ctx, q, oldID, u.ID); err != nil {
			return fmt.Errorf("failed to repoint %s.%s: %w", ref.table, ref.column, err)
		}
	}
	_, err := q.DeleteUser(ctx, oldID)
	return err
}

// changeTracker records which records a sync really changed, so the counts it
// reports are committed changes and not merely records it looked at. A user
// counts at most once: as inserted, or as updated (changed fields, a manager
// change, or a reactivation) -- never both.
type changeTracker struct {
	processedUsers, processedTeams int
	insertedUsers, changedUsers    map[string]bool
	insertedTeams, changedTeams    map[string]bool
}

func newChangeTracker() *changeTracker {
	return &changeTracker{
		insertedUsers: map[string]bool{}, changedUsers: map[string]bool{},
		insertedTeams: map[string]bool{}, changedTeams: map[string]bool{},
	}
}

// markChanged records a change to an existing record unless the record was
// inserted by this same run.
func markChanged(changed, inserted map[string]bool, id string) {
	if !inserted[id] {
		changed[id] = true
	}
}

// finish copies the tracked counts onto the result. Unchanged is what is left
// of the records the sync processed after inserts and updates are removed.
func (t *changeTracker) finish(r *orgprovider.ApplyResult) {
	r.InsertedUsers, r.UpdatedUsers = len(t.insertedUsers), len(t.changedUsers)
	r.InsertedTeams, r.UpdatedTeams = len(t.insertedTeams), len(t.changedTeams)
	r.UnchangedUsers = t.processedUsers - r.InsertedUsers - r.UpdatedUsers
	r.UnchangedTeams = t.processedTeams - r.InsertedTeams - r.UpdatedTeams
	if r.UnchangedUsers < 0 {
		r.UnchangedUsers = 0
	}
	if r.UnchangedTeams < 0 {
		r.UnchangedTeams = 0
	}
}

// upsertSnapshotUsers writes users with reports_to left alone. The column is a
// self-referencing foreign key, so manager links can only be set once every row
// in the snapshot exists -- see applySnapshotReportsTo.
//
// An existing row is only written when a normalized business field differs
// (username, email, display name, hierarchy level), so re-syncing identical
// data touches nothing -- not even updated_at -- and counts as unchanged.
// Comparison trims whitespace and ignores email case. RETURNING yields a row
// only when the statement inserted or updated, so "no row" means unchanged.
//
// is_active is deliberately not written here: reactivation and deactivation
// are decided once, after memberships are reconciled (reconcileActiveStates),
// so a record is never flipped on and back off inside one run.
//
// password_hash and auth_type are set on insert but never overwritten on
// conflict: a person who already signs in to THC locally must keep their
// credentials when they also appear in a provider snapshot.
//
// A snapshot record whose id collides with a protected user (the permanent
// admin, or a fixed demo/test/E2E fixture) is skipped entirely, and
// currentProtectedUserIDs additionally skips any user this deployment's own
// database already recognizes as protected, regardless of what the snapshot
// claims their hierarchy level is now.
//
// While the THC-only pod-lead mapping is on (in.MapPodLeads), a stored
// PodLeadLevelID against a provider PodLeadSourceLevelID is not treated as a
// provider change: applyPodLeadLevels decides that pair at the end of the run,
// when the final leads are known. Without this, every stable pod lead would be
// written to level-5 here and back to level-4 there, and show up as an updated
// user on every sync. With the mapping off both ids are empty strings, which
// match no real level, so the statement behaves exactly as before.
func upsertSnapshotUsers(ctx context.Context, q *sqlcgen.Queries, in orgprovider.ApplyInput, currentProtectedUserIDs map[string]bool, track *changeTracker) error {
	sourceLevel, leadLevel := "", ""
	if in.MapPodLeads {
		sourceLevel, leadLevel = orgprovider.PodLeadSourceLevelID, orgprovider.PodLeadLevelID
	}

	for _, u := range in.Snapshot.Users {
		if orgprovider.IsProtectedUser(u.ID, u.HierarchyLevelID) || currentProtectedUserIDs[u.ID] {
			continue
		}
		track.processedUsers++

		// A role-mapped user is written at the level their role names, so a
		// provider level THC lacks cannot break the foreign key and a stored
		// level-4 is never held back by the pod-lead exemption. applyPodIQRoleLevels
		// still decides the final level.
		level, userSourceLevel, userLeadLevel := u.HierarchyLevelID, sourceLevel, leadLevel
		if roleLevel, ok := podIQRoleLevel(u); ok {
			level, userSourceLevel, userLeadLevel = roleLevel, "", ""
		}

		inserted, err := q.UpsertSnapshotUser(ctx, sqlcgen.UpsertSnapshotUserParams{
			ID:             u.ID,
			Username:       u.Username,
			Email:          u.Email,
			FullName:       u.DisplayName,
			Level:          level,
			PodLeadLevel:   userLeadLevel,
			PodSourceLevel: userSourceLevel,
		})
		if errors.Is(err, sql.ErrNoRows) {
			continue // existing row, identical business fields: unchanged
		}
		if err != nil {
			// users.username and users.email are UNIQUE, so a provider record
			// colliding with a different existing THC user surfaces here rather
			// than as a conflict on id. Name the record so it can be fixed.
			return fmt.Errorf("failed to upsert user %q (%s): %w", u.Username, u.ID, err)
		}
		if inserted {
			track.insertedUsers[u.ID] = true
		} else {
			track.changedUsers[u.ID] = true
		}
	}
	return nil
}

// applyPodLeadLevels is the THC-only pod-lead mapping (orgprovider.PodLeadLevelID),
// the last write of the apply. Among the snapshot users the provider sent as
// PodLeadSourceLevelID it
//
//   - promotes every user who leads an active team they belong to, and
//   - returns a stored PodLeadLevelID to the provider's level for every user who
//     no longer does.
//
// It runs after users, teams, memberships and reactivation are all written, so
// it reads the final teams and leads straight from the database. That is why it
// needs no copy of the rules that decide who ends up leading what: a lead kept
// because the snapshot omitted teamLeadId, a team reactivated by this snapshot,
// a user re-keyed to a new provider id, a protected team, a partial snapshot and
// a lead replaced in this run are all just rows by now.
//
// Users are matched by the provider's id, which rekeyUsersToProviderIDs has
// already made the stored id. Protected users are never touched. Only rows that
// actually change are written, and each is recorded as updated, so a sync with
// nothing to change still reports nothing changed.
func applyPodLeadLevels(ctx context.Context, q *sqlcgen.Queries, in orgprovider.ApplyInput, currentProtectedUserIDs map[string]bool, track *changeTracker, result *orgprovider.ApplyResult) error {
	if !in.MapPodLeads {
		return nil
	}

	var sourceIDs []string
	for _, u := range in.Snapshot.Users {
		if u.HierarchyLevelID != orgprovider.PodLeadSourceLevelID {
			continue
		}
		if orgprovider.IsProtectedUser(u.ID, u.HierarchyLevelID) || currentProtectedUserIDs[u.ID] {
			continue
		}
		if _, ok := podIQRoleLevel(u); ok {
			continue // the role decides this user's level; see applyPodIQRoleLevels
		}
		sourceIDs = append(sourceIDs, u.ID)
	}
	if len(sourceIDs) == 0 {
		return nil
	}

	// Promotes users who lead an active team they are an active member of (an
	// inactive membership row is history, so it does not make someone a lead).
	promotedIDs, err := q.PromotePodLeads(ctx, sqlcgen.PromotePodLeadsParams{
		SourceIds:   sourceIDs,
		LeadLevel:   orgprovider.PodLeadLevelID,
		SourceLevel: orgprovider.PodLeadSourceLevelID,
	})
	if err != nil {
		return fmt.Errorf("failed to promote pod leads: %w", err)
	}
	promoted := markUsersChanged(track, promotedIDs)

	returnedIDs, err := q.ReturnFormerPodLeads(ctx, sqlcgen.ReturnFormerPodLeadsParams{
		SourceIds:   sourceIDs,
		LeadLevel:   orgprovider.PodLeadLevelID,
		SourceLevel: orgprovider.PodLeadSourceLevelID,
	})
	if err != nil {
		return fmt.Errorf("failed to return former pod leads to their provider level: %w", err)
	}
	returned := markUsersChanged(track, returnedIDs)

	result.PodLeadsPromoted = promoted
	result.PodLeadsReturned = returned
	return nil
}

// podIQRoleLevel returns the level a user's podiqRole maps to. ok is false when
// the user has no role or an invalid one (validation rejects those before apply).
func podIQRoleLevel(u orgsnapshot.User) (string, bool) {
	if u.PodIQRole == nil {
		return "", false
	}
	return orgprovider.PodIQRoleLevelID(*u.PodIQRole)
}

// applyPodIQRoleLevels is the final level decision (contract v1.1 podiqRole):
// L2 -> level-3, L1 -> level-4, Developer -> level-5. It overrides the
// provider's hierarchyLevelId, teamLeadId and membership-based lead logic, and
// applyPodLeadLevels skips these users entirely. Users without a role are not
// touched. Protected users are never touched. Only rows whose level differs are
// written, so an identical second sync reports no change.
func applyPodIQRoleLevels(ctx context.Context, q *sqlcgen.Queries, in orgprovider.ApplyInput, currentProtectedUserIDs map[string]bool, track *changeTracker) error {
	idsByLevel := make(map[string][]string)
	for _, u := range in.Snapshot.Users {
		level, ok := podIQRoleLevel(u)
		if !ok {
			continue
		}
		if orgprovider.IsProtectedUser(u.ID, u.HierarchyLevelID) || currentProtectedUserIDs[u.ID] {
			continue
		}
		idsByLevel[level] = append(idsByLevel[level], u.ID)
	}
	for _, level := range orgprovider.PodIQRoleLevelIDs() {
		ids := idsByLevel[level]
		if len(ids) == 0 {
			continue
		}
		changed, err := q.SetUserLevelForIDs(ctx, sqlcgen.SetUserLevelForIDsParams{Level: level, Ids: ids})
		if err != nil {
			return fmt.Errorf("failed to apply podiqRole level %s: %w", level, err)
		}
		markUsersChanged(track, changed)
	}
	return nil
}

// markUsersChanged records every id returned by a level UPDATE ... RETURNING id
// as a changed user, and returns how many there were.
func markUsersChanged(track *changeTracker, ids []string) int {
	for _, id := range ids {
		markChanged(track.changedUsers, track.insertedUsers, id)
	}
	return len(ids)
}

// applySnapshotReportsTo writes manager links now that every snapshot user exists.
// See upsertSnapshotUsers for why currentProtectedUserIDs is checked alongside
// IsProtectedUser: protection must survive a snapshot that relabels a
// currently-protected user's hierarchy level.
func applySnapshotReportsTo(ctx context.Context, q *sqlcgen.Queries, in orgprovider.ApplyInput, currentProtectedUserIDs map[string]bool, track *changeTracker) error {
	for _, u := range in.Snapshot.Users {
		if orgprovider.IsProtectedUser(u.ID, u.HierarchyLevelID) || currentProtectedUserIDs[u.ID] {
			continue
		}
		if in.PreserveReportsToUserIDs[u.ID] {
			// The provider named a manager we could not import. Treat that as
			// "no statement" and keep whatever THC already had, rather than
			// asserting this person now reports to nobody.
			continue
		}

		var reportsTo sql.NullString
		if u.ReportsToID != nil {
			reportsTo = sql.NullString{String: *u.ReportsToID, Valid: true}
		}

		// Only a real manager change writes (and bumps updated_at).
		n, err := q.SetUserReportsToIfChanged(ctx, sqlcgen.SetUserReportsToIfChangedParams{ID: u.ID, ReportsTo: reportsTo})
		if err != nil {
			return fmt.Errorf("failed to set manager for user %q: %w", u.ID, err)
		}
		if n > 0 {
			markChanged(track.changedUsers, track.insertedUsers, u.ID)
		}
	}
	return nil
}

// upsertSnapshotTeams writes teams. team_lead_id and health_check_enabled use the
// bound parameters rather than EXCLUDED so that a nil (field omitted by the
// provider) preserves the existing THC value, per the contract's tri-state
// rule. As with users, an existing team is only written when a business field
// really differs, so an identical re-sync counts the team as unchanged.
// is_active is decided afterwards in reconcileActiveStates.
func upsertSnapshotTeams(ctx context.Context, q *sqlcgen.Queries, in orgprovider.ApplyInput, track *changeTracker) error {
	for _, t := range in.Snapshot.Teams {
		if orgprovider.IsProtectedTeam(t.ID) {
			continue
		}
		track.processedTeams++

		var teamLead sql.NullString
		if t.TeamLeadID != nil {
			teamLead = sql.NullString{String: *t.TeamLeadID, Valid: true}
		}
		var healthCheck sql.NullBool
		if t.HealthCheckEnabled != nil {
			healthCheck = sql.NullBool{Bool: *t.HealthCheckEnabled, Valid: true}
		}

		inserted, err := q.UpsertSnapshotTeam(ctx, sqlcgen.UpsertSnapshotTeamParams{
			ID:                 t.ID,
			Name:               t.Name,
			TeamLeadID:         teamLead,
			HealthCheckEnabled: healthCheck,
		})
		if errors.Is(err, sql.ErrNoRows) {
			continue // unchanged
		}
		if err != nil {
			return fmt.Errorf("failed to upsert team %q (%s): %w", t.Name, t.ID, err)
		}
		if inserted {
			track.insertedTeams[t.ID] = true
		} else {
			track.changedTeams[t.ID] = true
		}
	}
	return nil
}

// reconcileSnapshotMemberships synchronizes team_members for every
// non-protected team the snapshot reports. Membership rows are never deleted:
//
//   - a pair in the snapshot is inserted, or reactivated if it was inactive;
//   - on a COMPLETE snapshot, an active row for a reported team that the
//     snapshot no longer lists becomes inactive (deactivated_at set), which keeps
//     it as history. This is how Hopefarm -> SRE leaves an inactive Hopefarm row
//     and an active SRE row;
//   - on a PARTIAL snapshot nothing is deactivated.
//
// Rows of users the filter could not import (PreservedMemberUserIDs) and rows of
// protected users are never touched, exactly as before.
//
// result.MembershipsSynced counts pairs reconciled for tracked teams;
// MembershipsAdded counts inserts/reactivations and MembershipsRemoved counts
// deactivations.
//
// A membership entry for a user upsertSnapshotUsers skipped as protected
// (see that function) is dropped here too, not just left unsynced: inserting
// it would reference a users.id row that was never written for a protected
// incoming user absent from THC, violating the team_members foreign key.
// Protected users keep whatever membership they already have -- the keep
// list below still carries currentProtectedUserIDs -- this only stops the
// sync from trying to insert a *new* row for one.
func reconcileSnapshotMemberships(ctx context.Context, q *sqlcgen.Queries, in orgprovider.ApplyInput, currentProtectedUserIDs map[string]bool, complete bool, result *orgprovider.ApplyResult) error {
	skipMembershipUserIDs := make(map[string]bool, len(currentProtectedUserIDs))
	for id := range currentProtectedUserIDs {
		skipMembershipUserIDs[id] = true
	}
	for _, u := range in.Snapshot.Users {
		if orgprovider.IsProtectedUser(u.ID, u.HierarchyLevelID) {
			skipMembershipUserIDs[u.ID] = true
		}
	}

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
		if skipMembershipUserIDs[m.UserID] {
			continue // protected user: upsertSnapshotUsers never wrote this id, so inserting it here would violate the FK
		}
		membersByTeam[m.TeamID] = append(membersByTeam[m.TeamID], m.UserID)
	}

	for teamID, members := range membersByTeam {
		result.MembershipsSynced += len(members)

		if complete {
			keep := append([]string{}, members...)
			keep = append(keep, in.PreservedMemberUserIDs...)
			for id := range currentProtectedUserIDs {
				keep = append(keep, id)
			}

			n, err := q.DeactivateTeamMembersNotKept(ctx, sqlcgen.DeactivateTeamMembersNotKeptParams{TeamID: teamID, KeepUserIds: keep})
			if err != nil {
				return fmt.Errorf("failed to deactivate members of team %q: %w", teamID, err)
			}
			result.MembershipsRemoved += int(n)
		}

		for _, userID := range members {
			// Inserts a new row or flips an inactive one back on; an already-active
			// row is left alone, so the count is 1 only for a real change.
			n, err := q.ActivateSnapshotTeamMember(ctx, sqlcgen.ActivateSnapshotTeamMemberParams{TeamID: teamID, UserID: userID})
			if err != nil {
				return fmt.Errorf("failed to add user %q to team %q: %w", userID, teamID, err)
			}
			result.MembershipsAdded += int(n)
		}
	}

	return nil
}

// deactivateMissingTeams marks every non-protected team absent from a COMPLETE
// snapshot inactive, along with its memberships. The team row, its action items
// and its survey history are all retained.
func deactivateMissingTeams(ctx context.Context, q *sqlcgen.Queries, missingTeamIDs []string, result *orgprovider.ApplyResult) error {
	if len(missingTeamIDs) == 0 {
		return nil
	}
	n, err := q.DeactivateTeamsByIDs(ctx, missingTeamIDs)
	if err != nil {
		return fmt.Errorf("failed to deactivate teams: %w", err)
	}
	result.TeamsDeactivated += int(n)

	removed, err := q.DeactivateMembershipsOfTeams(ctx, missingTeamIDs)
	if err != nil {
		return fmt.Errorf("failed to deactivate memberships of removed teams: %w", err)
	}
	result.MembershipsRemoved += int(removed)
	return nil
}

// deactivateMissingUsers marks every non-protected user absent from a COMPLETE
// snapshot inactive and ends their memberships. The users row is never deleted,
// so health_check_sessions, responses and action_items still resolve to a person.
//
// A user named in a protected team's supervisor chain is left active, the same
// exemption the old delete honored, re-checked in the statement itself so a
// concurrent chain edit cannot slip past the earlier read.
func deactivateMissingUsers(ctx context.Context, q *sqlcgen.Queries, missingUserIDs []string, result *orgprovider.ApplyResult) error {
	if len(missingUserIDs) == 0 {
		return nil
	}
	protectedTeamIDs := orgprovider.ProtectedTeamIDs()

	// hierarchy_level_id is re-checked here, not just in the earlier candidate
	// list: a user promoted to level-admin between that read and this statement
	// must not be deactivated just because the stale candidate list still names
	// them.
	n, err := q.DeactivateMissingUsers(ctx, sqlcgen.DeactivateMissingUsersParams{
		Ids:              missingUserIDs,
		ProtectedLevel:   orgprovider.ProtectedHierarchyLevelID,
		ProtectedTeamIds: protectedTeamIDs,
	})
	if err != nil {
		return fmt.Errorf("failed to deactivate users: %w", err)
	}
	result.UsersDeactivated += int(n)

	removed, err := q.DeactivateMembershipsOfInactiveUsers(ctx, missingUserIDs)
	if err != nil {
		return fmt.Errorf("failed to deactivate memberships of removed users: %w", err)
	}
	result.MembershipsRemoved += int(removed)
	return nil
}

// reconcileActiveStates decides, once and after memberships are reconciled,
// whether each person and team in the snapshot is active. Doing it in one pass
// means a record is never switched on and back off within a run, so the counts
// are the net committed change.
//
//   - A person with at least one membership row but no active one has no active
//     organization left, so they become inactive (counted as deactivated).
//   - An inactive person who now has an active membership, or who never had any
//     membership (a VP, say), is reactivated (counted as updated).
//   - A team the snapshot reports is reactivated if it was inactive.
func reconcileActiveStates(ctx context.Context, q *sqlcgen.Queries, in orgprovider.ApplyInput, currentProtectedUserIDs map[string]bool, track *changeTracker, result *orgprovider.ApplyResult) error {
	userIDs := make([]string, 0, len(in.Snapshot.Users))
	for _, u := range in.Snapshot.Users {
		if orgprovider.IsProtectedUser(u.ID, u.HierarchyLevelID) || currentProtectedUserIDs[u.ID] {
			continue
		}
		userIDs = append(userIDs, u.ID)
	}
	if len(userIDs) > 0 {
		deactivated, err := q.DeactivateUsersWithoutActiveMembership(ctx, userIDs)
		if err != nil {
			return fmt.Errorf("failed to deactivate users without an active membership: %w", err)
		}
		result.UsersDeactivated += len(deactivated)

		reactivated, err := q.ReactivateUsers(ctx, userIDs)
		if err != nil {
			return fmt.Errorf("failed to reactivate users: %w", err)
		}
		markUsersChanged(track, reactivated)
	}

	teamIDs := make([]string, 0, len(in.Snapshot.Teams))
	for _, t := range in.Snapshot.Teams {
		if !orgprovider.IsProtectedTeam(t.ID) {
			teamIDs = append(teamIDs, t.ID)
		}
	}
	if len(teamIDs) > 0 {
		reactivated, err := q.ReactivateTeams(ctx, teamIDs)
		if err != nil {
			return fmt.Errorf("failed to reactivate teams: %w", err)
		}
		for _, id := range reactivated {
			markChanged(track.changedTeams, track.insertedTeams, id)
		}
	}
	return nil
}

// rejectStaleSnapshot locks the singleton settings row and refuses a snapshot
// generated before the last one applied. An equal timestamp is allowed so a
// manual re-run of the same snapshot is idempotent.
func rejectStaleSnapshot(ctx context.Context, q *sqlcgen.Queries, generatedAt time.Time) error {
	last, err := q.LockLastAppliedSnapshotTime(ctx)
	if err != nil {
		return fmt.Errorf("failed to read last applied snapshot time: %w", err)
	}
	if last.Valid && generatedAt.Before(last.Time) {
		return orgprovider.ErrStaleSnapshot
	}
	return nil
}

// recordAppliedSnapshot stores the provider timestamp inside the same
// transaction, so it only advances when the whole sync commits.
func recordAppliedSnapshot(ctx context.Context, q *sqlcgen.Queries, generatedAt time.Time) error {
	if err := q.RecordAppliedSnapshotTime(ctx, sql.NullTime{Time: generatedAt.UTC(), Valid: true}); err != nil {
		return fmt.Errorf("failed to record applied snapshot time: %w", err)
	}
	return nil
}

// protectedTeamSupervisorUserIDs returns every user currently named in a
// protected team's stored supervisor chain (team_supervisors), as of this
// call. It exists solely to keep the mass-deletion guard's eligibility
// count in agreement with deleteMissingUsers' own, independently
// re-checked, atomic exclusion -- see that function's comment for why this
// snapshot is never treated as the actual safeguard.
func protectedTeamSupervisorUserIDs(ctx context.Context, q *sqlcgen.Queries) (map[string]bool, error) {
	rows, err := q.FindDistinctTeamSupervisors(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read team supervisors: %w", err)
	}

	exempt := make(map[string]bool)
	for _, row := range rows {
		if orgprovider.IsProtectedTeam(row.TeamID) {
			exempt[row.UserID] = true
		}
	}
	return exempt, nil
}

// markRunSucceeded writes the run's terminal SUCCESS state in the apply
// transaction, including the snapshot type actually applied: the run row is
// created before the snapshot is fetched, so it starts out as COMPLETE.
//
// The WHERE clause is the stale-worker guard: only a RUNNING run can succeed, so if a recovery sweep already failed it the update matches no
// row, this returns an error, and the whole apply rolls back.
func markRunSucceeded(ctx context.Context, q *sqlcgen.Queries, in orgprovider.ApplyInput, r *orgprovider.ApplyResult) error {
	// The zero value means PARTIAL (see ApplyInput), and the column's CHECK
	// constraint rejects an empty string.
	snapshotType := in.SnapshotType
	if snapshotType == "" {
		snapshotType = orgprovider.SnapshotPartial
	}
	n, err := q.MarkOrgSyncRunSucceeded(ctx, sqlcgen.MarkOrgSyncRunSucceededParams{
		Stage:                  string(orgsyncrun.StageCompleted),
		SourceGeneratedAt:      sql.NullTime{Time: in.Snapshot.GeneratedAt.UTC(), Valid: true},
		InsertedUsers:          int32(r.InsertedUsers),
		UpdatedUsers:           int32(r.UpdatedUsers),
		InsertedTeams:          int32(r.InsertedTeams),
		UpdatedTeams:           int32(r.UpdatedTeams),
		MembershipChanges:      int32(r.MembershipsAdded + r.MembershipsRemoved),
		DeactivatedRecords:     int32(r.UsersDeactivated + r.TeamsDeactivated),
		UnchangedUsers:         int32(r.UnchangedUsers),
		UnchangedTeams:         int32(r.UnchangedTeams),
		HealthChecksDisabled:   int32(r.HealthChecksDisabled),
		HealthChecksEnabled:    int32(r.HealthChecksEnabled),
		MassDeletionOverridden: r.MassDeletionOverride != nil,
		SnapshotType:           string(snapshotType),
		ID:                     in.RunID,
		AllowedStatuses:        orgsyncrun.StatusesBefore(orgsyncrun.StatusSuccess),
	})
	if err != nil {
		return fmt.Errorf("failed to record run success: %w", err)
	}
	if n != 1 {
		return fmt.Errorf("sync run %s is no longer running; discarding apply", in.RunID)
	}
	return nil
}
