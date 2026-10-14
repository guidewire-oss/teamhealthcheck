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
	"time"

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

// protectedTeamIDs are the fixed demo teams created by SeedDemoData, plus the
// E2E acceptance-suite fixture teams. Same closed-set reasoning as
// protectedUserIDs -- and the E2E entries are the counterpart to the E2E user
// cast protected above: those users' memberships live in these teams, so
// protecting the people without protecting their teams would still let a sync
// delete the teams and cascade the memberships away.
var protectedTeamIDs = map[string]bool{
	// Core demo cast
	"team-phoenix": true, "team-dragon": true, "team-titan": true,
	"team-falcon": true, "team-eagle": true, "team-nova": true,
	// E2E acceptance-test cast (tests/acceptance/suite_test.go)
	"e2e_team1": true, "e2e_team2": true, "e2e_team3": true,
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

// ProtectedTeamIDs returns every protected team id, for callers that need to
// pass the set into a SQL query (e.g. to guard a cascading delete) rather
// than check membership one id at a time.
func ProtectedTeamIDs() []string {
	ids := make([]string, 0, len(protectedTeamIDs))
	for id := range protectedTeamIDs {
		ids = append(ids, id)
	}
	return ids
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

	// IncomingUsers/IncomingTeams/IncomingMemberships are the RAW provider
	// payload's own record counts, before FilterSnapshot drops anything --
	// unlike Snapshot, which only ever holds what survived filtering. A held
	// sync's MassDeletionReport.WithIncoming needs the raw counts: comparing
	// a big deletion against the filtered (importable) count would hide a
	// payload where most records were sent but skipped for a bad hierarchy
	// level, understating how much the provider actually sent and making an
	// unsafe override look more justified than it is.
	IncomingUsers       int
	IncomingTeams       int
	IncomingMemberships int

	// PreservedMemberUserIDs are users excluded from the snapshot whose existing
	// team_members rows must survive the per-team membership replace.
	PreservedMemberUserIDs []string

	// PreserveReportsToUserIDs are users whose manager was skipped; their
	// existing reports_to is left untouched rather than cleared.
	PreserveReportsToUserIDs map[string]bool

	// MaxDeletePercent bounds how much of the current non-protected user/team
	// population a single sync may remove. See EvaluateMassDeletionGuard.
	MaxDeletePercent float64

	// OverrideMassDeletion waives the mass-deletion percentage guard for this
	// one apply, after an administrator reviewed the held sync's counts and
	// explicitly asked for it. It waives nothing else: snapshot validation,
	// the protected-record allowlists, and the single-transaction guarantee
	// all still apply, and MaxDeletePercent itself is left untouched.
	//
	// Every sync re-fetches the provider snapshot fresh, including an override
	// request -- so OverrideMassDeletion alone would let an admin approve one
	// set of counts and have a completely different, unreviewed deletion
	// applied if the provider's data changed in between. ConfirmedMassDeletion
	// closes that gap: the override only takes effect when it still matches
	// the freshly recomputed report (see MassDeletionReport.MatchesConfirmed).
	OverrideMassDeletion bool

	// ConfirmedMassDeletion is the exact counts an administrator reviewed
	// before requesting OverrideMassDeletion. Required for the override to
	// take effect; nil or stale (no longer matching the freshly recomputed
	// report) is treated exactly like no override at all -- the sync is held
	// again, with the current counts, for a fresh confirmation.
	ConfirmedMassDeletion *ConfirmedMassDeletion

	// RunID, Trigger, InstanceID and StartedAt identify this attempt for the durable
	// success record ApplySnapshot writes internally, in the same transaction
	// as the destructive writes it describes, immediately before COMMIT. They
	// are used only on the success path -- a blocked or failed outcome is
	// recorded separately by the caller, since this transaction rolls back on
	// every other path and cannot durably record anything itself. An empty
	// RunID is replaced with a fresh UUID so a direct caller never fails on it.
	RunID      string
	Trigger    string
	InstanceID string
	StartedAt  time.Time
}

// ConfirmedMassDeletion is the counts an administrator actually reviewed
// before overriding a held sync -- an echo of the MassDeletionReport the
// held response carried, not a second calculation.
type ConfirmedMassDeletion struct {
	UsersExisting int
	UsersDeleting int
	TeamsExisting int
	TeamsDeleting int
}

// ApplyResult reports what a sync changed.
type ApplyResult struct {
	TeamsSynced int `json:"teamsSynced"`
	UsersSynced int `json:"usersSynced"`

	// MembershipsSynced counts memberships actually reconciled against a
	// tracked (non-protected) team -- not the raw count of entries the
	// snapshot sent. A membership for a protected team, or for a team absent
	// from the snapshot's own teams[], is dropped before reaching this count.
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

	// MassDeletionOverride is set only when the guard tripped and an
	// administrator's explicit override let the sync proceed anyway. It carries
	// the counts that were waived, so the service can audit-log them.
	MassDeletionOverride *MassDeletionReport `json:"massDeletionOverride,omitempty"`
}

// ErrMassDeletionBlocked is returned when a sync's calculated deletions exceed
// the configured safety threshold. Nothing is written when this is returned.
var ErrMassDeletionBlocked = errors.New("sync blocked: deletions exceed the configured safety threshold, review required")

// Kinds a DeletionMetric can report. The distinction matters to an admin
// reviewing a held sync: a "deleted" count is a row this sync would remove
// directly and is what the guard measures; a "cascaded" count is a row the
// database removes as a consequence of those deletions (a foreign key with
// ON DELETE CASCADE) and is reported for visibility only.
const (
	DeletionKindDeleted  = "deleted"
	DeletionKindCascaded = "cascaded"
)

// DeletionMetric is one entity type's share of this sync's deletions, using
// exactly the numbers the guard itself works from: Deleting/Existing*100
// compared against Threshold. Existing is always the count of eligible
// (non-protected) rows currently in THC, never the whole table.
type DeletionMetric struct {
	// Kind is DeletionKindDeleted or DeletionKindCascaded -- see the constants.
	Kind string `json:"kind"`
	// Existing is the denominator: eligible rows currently in THC.
	Existing int `json:"existing"`
	// Incoming is how many records of this type the provider's snapshot
	// actually contained. It is not part of the guard's arithmetic, but it is
	// the single most useful number for diagnosing a surprising hold: a large
	// Deleting alongside a small Incoming means the provider under-reported
	// this entity type, not that the organization really shrank. Zero when the
	// count was not supplied.
	Incoming int `json:"incoming"`
	// Deleting is the numerator: rows this sync proposes to remove.
	Deleting int `json:"deleting"`
	// Percent is Deleting/Existing*100, or 100 when Existing is 0 and
	// Deleting is not (the same "treat as unsafe" rule exceedsThreshold uses).
	Percent float64 `json:"percent"`
	// Threshold is the configured maximum percentage, for display alongside Percent.
	Threshold float64 `json:"threshold"`
	// ExceedsThreshold is the raw comparison result for this metric.
	ExceedsThreshold bool `json:"exceedsThreshold"`
	// ContributesToHold is false for metrics reported for information only.
	// Only user and team deletions can hold a sync; membership cascade is
	// displayed but never blocks, which is exactly the pre-existing guard
	// semantics and is stated here rather than left to be inferred.
	ContributesToHold bool `json:"contributesToHold"`
}

// MassDeletionReport is the full picture of what a sync proposes to remove,
// built from the same counts EvaluateMassDeletionGuard compares. It carries no
// record identities, provider payload, or credentials -- only counts.
type MassDeletionReport struct {
	Threshold float64        `json:"threshold"`
	Users     DeletionMetric `json:"users"`
	Teams     DeletionMetric `json:"teams"`
	// Memberships is the informational cascade metric, attached by the
	// persistence layer when it can be measured. It is nil when unavailable.
	Memberships *DeletionMetric `json:"memberships,omitempty"`
}

// WithIncoming records how many users, teams and memberships the provider's
// snapshot actually contained. This is reporting only -- it never changes a
// percentage, a verdict, or which metrics contribute to the hold.
func (r *MassDeletionReport) WithIncoming(users, teams, memberships int) {
	r.Users.Incoming = users
	r.Teams.Incoming = teams
	if r.Memberships != nil {
		r.Memberships.Incoming = memberships
	}
}

// Held reports whether any hold-contributing metric is over the threshold.
func (r MassDeletionReport) Held() bool {
	return (r.Users.ContributesToHold && r.Users.ExceedsThreshold) ||
		(r.Teams.ContributesToHold && r.Teams.ExceedsThreshold)
}

// MatchesConfirmed reports whether confirmed still describes r exactly. A nil
// confirmed never matches -- an override request that omitted it is treated
// the same as a stale one, since neither proves the admin reviewed r.
func (r MassDeletionReport) MatchesConfirmed(confirmed *ConfirmedMassDeletion) bool {
	if confirmed == nil {
		return false
	}
	return confirmed.UsersExisting == r.Users.Existing &&
		confirmed.UsersDeleting == r.Users.Deleting &&
		confirmed.TeamsExisting == r.Teams.Existing &&
		confirmed.TeamsDeleting == r.Teams.Deleting
}

// MassDeletionHoldError is the typed error a tripped guard returns. It wraps
// ErrMassDeletionBlocked, so every existing errors.Is check keeps working,
// and carries the counts so the API can show an admin what would have been
// deleted instead of only saying that something would have been.
type MassDeletionHoldError struct {
	Report MassDeletionReport
}

func (e *MassDeletionHoldError) Error() string {
	// Same wording the guard used before this type existed, so log lines and
	// message assertions read identically.
	switch {
	case e.Report.Users.ExceedsThreshold:
		return fmt.Sprintf("%v: would delete %d of %d users", ErrMassDeletionBlocked, e.Report.Users.Deleting, e.Report.Users.Existing)
	case e.Report.Teams.ExceedsThreshold:
		return fmt.Sprintf("%v: would delete %d of %d teams", ErrMassDeletionBlocked, e.Report.Teams.Deleting, e.Report.Teams.Existing)
	default:
		return ErrMassDeletionBlocked.Error()
	}
}

// Unwrap keeps errors.Is(err, ErrMassDeletionBlocked) true.
func (e *MassDeletionHoldError) Unwrap() error { return ErrMassDeletionBlocked }

// BuildMassDeletionReport computes every metric the guard decides on, without
// deciding anything itself. EvaluateMassDeletionGuard is the only caller that
// turns it into an error; the override path uses it to log what was waived.
func BuildMassDeletionReport(currentUsers, deleteUsers, currentTeams, deleteTeams int, maxPercent float64) MassDeletionReport {
	return MassDeletionReport{
		Threshold: maxPercent,
		Users:     NewDeletionMetric(DeletionKindDeleted, currentUsers, deleteUsers, maxPercent, true),
		Teams:     NewDeletionMetric(DeletionKindDeleted, currentTeams, deleteTeams, maxPercent, true),
	}
}

// NewDeletionMetric builds one metric from the same percentage math the guard
// applies, so a displayed percentage can never disagree with the decision.
func NewDeletionMetric(kind string, current, deletions int, maxPercent float64, contributesToHold bool) DeletionMetric {
	return DeletionMetric{
		Kind:              kind,
		Existing:          current,
		Deleting:          deletions,
		Percent:           deletionPercent(current, deletions),
		Threshold:         maxPercent,
		ExceedsThreshold:  exceedsThreshold(current, deletions, maxPercent),
		ContributesToHold: contributesToHold,
	}
}

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
//
// On a trip it returns a *MassDeletionHoldError carrying the counts. The
// numbers are the guard's own, not a second calculation.
func EvaluateMassDeletionGuard(currentUsers, deleteUsers, currentTeams, deleteTeams int, maxPercent float64) error {
	report := BuildMassDeletionReport(currentUsers, deleteUsers, currentTeams, deleteTeams, maxPercent)
	if report.Held() {
		return &MassDeletionHoldError{Report: report}
	}
	return nil
}

// deletionPercent is the single percentage formula: deletions as a share of
// the eligible existing population. A deletion from an empty population is
// reported as 100% to match exceedsThreshold treating it as unsafe.
func deletionPercent(current, deletions int) float64 {
	if deletions == 0 {
		return 0
	}
	if current == 0 {
		return 100
	}
	return float64(deletions) / float64(current) * 100
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
	return deletionPercent(current, deletions) > maxPercent
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
	// subject to EvaluateMassDeletionGuard (unless ApplyInput.OverrideMassDeletion
	// waives that one check) and the action-items retention check documented on
	// ApplyResult. On any error, nothing is committed.
	ApplySnapshot(ctx context.Context, in ApplyInput) (*ApplyResult, error)
}
