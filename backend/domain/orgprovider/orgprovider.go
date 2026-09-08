// Package orgprovider models the external organization-data provider integration:
// the outcome of a sync, the fixed set of records a sync must never touch, and the
// persistence contract the infrastructure layer implements.
//
// There is deliberately no provider-ownership column anywhere in the schema (see
// docs/organization-snapshot-contract.md and the design discussion that preceded
// this package). Because "is this row provider-managed" cannot be answered by a
// database column, every user/team not explicitly protected below is treated as
// provider-managed: present in the provider's snapshot -> created/updated; absent -> a
// hard-delete candidate. The fixed allowlists in this file are what keeps that
// blanket assumption safe.
package orgprovider

import (
	"context"
	"errors"
	"fmt"

	"github.com/agopalakrishnan/teams360/backend/pkg/orgsnapshot"
)

// ProtectedHierarchyLevelID is Team Health Check's own system-administration
// role. It sits outside the 1-5 org-hierarchy scale the provider's managementLevel
// mapping produces (the provider maps into level-1..level-5 only), so a provider sync
// was never going to manage it -- this makes that explicit and enforced.
const ProtectedHierarchyLevelID = "level-admin"

// protectedAdminID is the permanent administrator account created unconditionally
// by migration 000007_seed_demo_users, in every environment.
const protectedAdminID = "admin"

// protectedUserIDs are the fixed demo/test/E2E fixture users created by
// SeedDemoData (backend/infrastructure/persistence/postgres/seed.go), gated
// behind APP_ENV=demo. The set is closed and exhaustive: it was derived by
// reading every INSERT INTO users statement in that function. These IDs never
// collide with a real provider identity (the provider's ids are Okta ids, a different
// shape entirely), so protecting them unconditionally -- not just when
// APP_ENV=demo -- is harmless in any other environment and safe everywhere.
var protectedUserIDs = map[string]bool{
	// Core demo cast
	"vp": true, "director1": true, "director2": true,
	"manager1": true, "manager2": true, "manager3": true,
	"teamlead1": true, "teamlead2": true, "teamlead3": true, "teamlead4": true, "teamlead5": true,
	"alice": true, "bob": true, "carol": true, "david": true, "eve": true, "demo": true,
	// Nova test-team cast
	"test-vp": true, "test-director": true, "test-manager": true, "test-lead": true,
	"test-member1": true, "test-member2": true,
	// E2E acceptance-test cast
	"e2e_manager1": true, "e2e_testmanager1": true, "e2e_lead1": true, "e2e_lead2": true,
	"e2e_demo": true, "e2e_member1": true, "e2e_member2": true, "e2e_member3": true,
	"e2e_fresh_member": true,
}

// protectedTeamIDs are the fixed demo teams created by SeedDemoData. Same
// closed-set reasoning as protectedUserIDs.
var protectedTeamIDs = map[string]bool{
	"team-phoenix": true, "team-dragon": true, "team-titan": true,
	"team-falcon": true, "team-eagle": true, "team-nova": true,
}

// IsProtectedUser reports whether a user must never be created, updated, or
// deleted by a provider sync, regardless of what the provider reports for this id.
func IsProtectedUser(id, hierarchyLevelID string) bool {
	return id == protectedAdminID || hierarchyLevelID == ProtectedHierarchyLevelID || protectedUserIDs[id]
}

// IsProtectedTeam reports whether a team must never be created, updated, or
// deleted by a provider sync.
func IsProtectedTeam(id string) bool {
	return protectedTeamIDs[id]
}

// SkipReasons reported when a snapshot record cannot be imported for this sync.
const (
	SkipReasonMissingLevel = "missing_hierarchy_level"
	SkipReasonUnknownLevel = "unknown_hierarchy_level"
)

// SkippedUser records a snapshot user that was not imported, and why. This is
// distinct from deletion: a skipped user's existing THC data is preserved,
// because the skip is an artefact of this sync being unable to import a
// malformed/unrecognized record, not a statement from the provider that they left.
type SkippedUser struct {
	UserID           string `json:"userId"`
	Username         string `json:"username"`
	HierarchyLevelID string `json:"hierarchyLevelId"`
	Reason           string `json:"reason"`
}

// ApplyInput carries a filtered, already-validated snapshot into persistence,
// along with the corrections the filter had to make.
type ApplyInput struct {
	// Snapshot contains only importable records.
	Snapshot *orgsnapshot.Snapshot

	// PreservedMemberUserIDs are users excluded from the snapshot whose existing
	// team_members rows must survive the per-team membership replace.
	PreservedMemberUserIDs []string

	// PreserveReportsToUserIDs are users whose manager was skipped; their
	// existing reports_to is left untouched rather than cleared.
	PreserveReportsToUserIDs map[string]bool

	// MaxDeletePercent bounds how much of the current non-protected user/team
	// population a single sync may remove. See EvaluateMassDeletionGuard.
	MaxDeletePercent float64
}

// ApplyResult reports what a sync changed.
type ApplyResult struct {
	TeamsSynced        int `json:"teamsSynced"`
	UsersSynced        int `json:"usersSynced"`
	MembershipsSynced  int `json:"membershipsSynced"`
	MembershipsRemoved int `json:"membershipsRemoved"`

	// HealthChecksDisabled/Enabled count teams whose health_check_enabled this
	// sync actually flipped, surfaced so an org-wide false doesn't disable every
	// team silently.
	HealthChecksDisabled int `json:"healthChecksDisabled"`
	HealthChecksEnabled  int `json:"healthChecksEnabled"`

	// UsersDeleted/TeamsDeleted count non-protected records absent from the
	// snapshot that were hard-deleted, per the approved "the data provider is authoritative"
	// rule -- deletion is never skipped or held back for these records.
	UsersDeleted int `json:"usersDeleted"`
	TeamsDeleted int `json:"teamsDeleted"`

	// ActionItemsDeleted counts action_items rows removed as a side effect of
	// the deletions above (action_items.created_by/team_id are NOT NULL with
	// ON DELETE CASCADE -- see migrations/000020_create_action_items -- so a
	// deleted user's/team's action items are cascade-deleted along with them).
	// This is visibility into a required cascade, not an optional retention: it
	// is reported so an admin can see what a sync actually removed, not a gate
	// that blocks the user/team deletion itself.
	ActionItemsDeleted int `json:"actionItemsDeleted"`
}

// ErrMassDeletionBlocked is returned when a sync's calculated deletions exceed
// the configured safety threshold. Nothing is written when this is returned.
var ErrMassDeletionBlocked = errors.New("sync blocked: deletions exceed the configured safety threshold, review required")

// EvaluateMassDeletionGuard aborts a sync whose calculated hard deletions would
// remove more than maxPercent of the currently-synced (non-protected) users or
// teams. This is a pure function so the threshold math has exactly one
// implementation, exercised directly by unit tests and called by the
// repository before any destructive write.
//
// The provider's documented exclusion of management levels 1-3 requires no special
// case here: those users are simply counted like any other deletion once they
// stop appearing in the snapshot. The exclusion is a validation concern (their
// absence must not be treated as an incomplete response), not a guard concern.
func EvaluateMassDeletionGuard(currentUsers, deleteUsers, currentTeams, deleteTeams int, maxPercent float64) error {
	if exceedsThreshold(currentUsers, deleteUsers, maxPercent) {
		return fmt.Errorf("%w: would delete %d of %d users", ErrMassDeletionBlocked, deleteUsers, currentUsers)
	}
	if exceedsThreshold(currentTeams, deleteTeams, maxPercent) {
		return fmt.Errorf("%w: would delete %d of %d teams", ErrMassDeletionBlocked, deleteTeams, currentTeams)
	}
	return nil
}

func exceedsThreshold(current, deletions int, maxPercent float64) bool {
	if deletions == 0 {
		return false
	}
	if current == 0 {
		// Deleting from an empty non-protected population should never happen
		// in practice (there would be nothing to delete); treat it as unsafe
		// rather than dividing by zero.
		return true
	}
	return float64(deletions)/float64(current)*100 > maxPercent
}

// Repository is the persistence contract for the provider integration. There
// is no credential storage here: the provider token is read from environment
// configuration by the transport client, never persisted.
type Repository interface {
	// KnownHierarchyLevelIDs returns the level IDs configured in this deployment.
	KnownHierarchyLevelIDs(ctx context.Context) (map[string]bool, error)

	// ApplySnapshot writes the snapshot in a single transaction. Protected users
	// and teams (IsProtectedUser/IsProtectedTeam) are never created, updated, or
	// deleted. Every other user/team present in the snapshot is upserted; every
	// other user/team absent from the snapshot is a hard-delete candidate,
	// subject to EvaluateMassDeletionGuard and the action-items retention check
	// documented on ApplyResult. On any error, nothing is committed.
	ApplySnapshot(ctx context.Context, in ApplyInput) (*ApplyResult, error)
}
