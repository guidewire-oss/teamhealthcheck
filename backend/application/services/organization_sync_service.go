package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	"github.com/agopalakrishnan/teams360/backend/domain/team"
	"github.com/agopalakrishnan/teams360/backend/domain/user"
	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
	"github.com/agopalakrishnan/teams360/backend/pkg/orgsnapshot"
)

// EnvMaxDeletePercent names the environment variable configuring the
// mass-deletion guard. It is the deployment-level fallback: an administrator's
// saved setting takes precedence over it, and DefaultMaxDeletePercent applies
// when neither is present. Its value is set in .env (see .env.example).
const EnvMaxDeletePercent = "ORG_SYNC_MAX_DELETE_PERCENT"

// MassDeletionHoldTTL bounds how long a held sync keeps the threshold frozen.
// A hold that nobody resolves must not lock the setting forever -- an admin who
// walked away from the screen would otherwise leave it unchangeable.
const MassDeletionHoldTTL = 30 * time.Minute

// DefaultMaxDeletePercent is the mass-deletion threshold used when no
// administrator has saved one and EnvMaxDeletePercent is unset or unusable.
const DefaultMaxDeletePercent = 20.0

// MinConfigurableDeletePercent and MaxConfigurableDeletePercent bound the
// administrator-configurable threshold. The same range is enforced by the API
// handler, the database CHECK constraint, and the stored-value read below.
const (
	MinConfigurableDeletePercent = 1.0
	MaxConfigurableDeletePercent = 100.0
)

// Errors returned by OrganizationSyncService. Handlers map these to status codes.
var (
	// ErrSyncInProgress means another synchronization is already running.
	ErrSyncInProgress = errors.New("a synchronization is already in progress")
	// ErrProviderNotConfigured means the data provider client is not configured.
	ErrProviderNotConfigured = errors.New("organization provider is not configured")
	// ErrInvalidSnapshot means the provider returned data that breaches the contract.
	ErrInvalidSnapshot = errors.New("provider snapshot failed contract validation")
	// ErrProviderFetchFailed means the fetch to the external provider itself
	// failed (network error, non-200 response, oversized/malformed body). This
	// is distinct from an internal/DB failure: the handler maps it to 502
	// Bad Gateway, since it genuinely reflects an unusable upstream response.
	ErrProviderFetchFailed = errors.New("failed to fetch snapshot from provider")
	// ErrMaxDeletePercentNotConfigured means the persisted mass-deletion
	// threshold is present but unusable (outside 1-100, NaN, or infinite).
	// An unset threshold is no longer an error -- the environment variable,
	// and then DefaultMaxDeletePercent, take over -- but a stored value that
	// cannot be trusted fails the sync closed rather than silently widening
	// or narrowing the guard.
	ErrMaxDeletePercentNotConfigured = errors.New("configured mass-deletion threshold is invalid")
)

// DeleteThresholdStore reads the administrator-configured mass-deletion
// threshold. The sync service depends on this narrow interface rather than the
// whole settings repository, so it stays testable and free of a persistence
// dependency it does not otherwise need. A nil *float64 means no administrator
// has configured a threshold.
type DeleteThresholdStore interface {
	GetOrgSyncMaxDeletePercent(ctx context.Context) (*float64, error)
}

// Option customizes an OrganizationSyncService at construction.
type Option func(*OrganizationSyncService)

// WithDeleteThresholdStore wires the persisted admin setting into the
// mass-deletion guard. Without it the service falls back to
// EnvMaxDeletePercent and then DefaultMaxDeletePercent.
func WithDeleteThresholdStore(store DeleteThresholdStore) Option {
	return func(s *OrganizationSyncService) {
		s.thresholds = store
	}
}

// SnapshotFetcher fetches a complete organization snapshot from an external
// provider. The sync service depends on this interface, not a concrete
// client, so a second provider can be added without touching orchestration.
// The fetcher owns its own credentials (read from its own environment
// configuration); it is never handed a token by this service.
type SnapshotFetcher interface {
	FetchSnapshot(ctx context.Context) (*orgsnapshot.Snapshot, error)
}

// SyncResult is the outcome of one synchronization run, returned to the API
// caller. It never carries a token, header, or the raw provider payload.
type SyncResult struct {
	Status string `json:"status"`

	TeamsSynced        int `json:"teamsSynced"`
	UsersSynced        int `json:"usersSynced"`
	MembershipsSynced  int `json:"membershipsSynced"`
	MembershipsRemoved int `json:"membershipsRemoved"`

	HealthChecksDisabled int `json:"healthChecksDisabled"`
	HealthChecksEnabled  int `json:"healthChecksEnabled"`

	UsersDeleted       int `json:"usersDeleted"`
	TeamsDeleted       int `json:"teamsDeleted"`
	ActionItemsDeleted int `json:"actionItemsDeleted"`

	UsersSkipped         int                       `json:"usersSkipped"`
	SkippedUsers         []orgprovider.SkippedUser `json:"skippedUsers,omitempty"`
	ManagerLinksCleared  int                       `json:"managerLinksCleared"`
	TeamLeadsCleared     int                       `json:"teamLeadsCleared"`
	MembershipsDiscarded int                       `json:"membershipsDiscarded"`

	// MassDeletionOverridden reports that this run only completed because an
	// administrator explicitly waived the mass-deletion hold, and MassDeletion
	// carries the counts that were waived. Both are absent on a normal sync.
	MassDeletionOverridden bool                            `json:"massDeletionOverridden,omitempty"`
	MassDeletion           *orgprovider.MassDeletionReport `json:"massDeletion,omitempty"`

	StartedAt   time.Time `json:"startedAt"`
	CompletedAt time.Time `json:"completedAt"`
}

// SyncOptions carries per-request choices an administrator made. It is scoped
// to one Sync call: nothing here is persisted, cached, or carried into the
// next run.
type SyncOptions struct {
	// OverrideMassDeletion waives the mass-deletion percentage guard for this
	// one run only, after an admin reviewed the held counts. The caller is
	// responsible for having established that the requester is an
	// administrator before setting it.
	OverrideMassDeletion bool

	// ConfirmedMassDeletion is the exact counts the admin reviewed before
	// requesting OverrideMassDeletion. See orgprovider.ApplyInput's field of
	// the same name: the override only takes effect when this still matches
	// the freshly recomputed report, since this sync fetches its own fresh
	// snapshot rather than replaying the one that produced the held counts.
	ConfirmedMassDeletion *orgprovider.ConfirmedMassDeletion

	// ActorUserID identifies who asked, for the audit record written when an
	// override is used.
	ActorUserID string
}

// heldSync is the server-side memory of an unresolved mass-deletion hold. It
// lives in the process rather than the database deliberately: a hold belongs to
// a sync attempt, and a restart ends every attempt in flight along with it.
type heldSync struct {
	// threshold is the value the held attempt was judged against, and the value
	// every further attempt uses until the hold is resolved.
	threshold float64
	report    orgprovider.MassDeletionReport
	heldAt    time.Time
}

// SyncLockState describes why -- and at what value -- the mass-deletion
// threshold is currently frozen. The zero value means it is editable.
type SyncLockState struct {
	// Syncing is true while a run holds the admission lock.
	Syncing bool
	// Held is true while an unexpired mass-deletion hold is unresolved.
	Held bool
	// Threshold is the value in force for the run that caused the lock.
	Threshold float64
	// HeldAt is when the hold was recorded, zero unless Held.
	HeldAt time.Time
}

// Locked reports whether the threshold may be changed right now.
func (l SyncLockState) Locked() bool { return l.Syncing || l.Held }

// OrganizationSyncService pulls an organization snapshot from the configured
// provider and applies it to THC.
type OrganizationSyncService struct {
	repo     orgprovider.Repository
	fetcher  SnapshotFetcher
	userRepo user.Repository
	teamRepo team.Repository

	// thresholds reads the admin-configured mass-deletion threshold. Nil when
	// the deployment wires no settings store, in which case the environment
	// variable and the built-in default decide.
	thresholds DeleteThresholdStore

	// mu guards held. It is not the sync admission lock -- that is running,
	// below -- only the small record describing an unresolved hold.
	mu sync.Mutex
	// held records a sync that the mass-deletion guard stopped and that nobody
	// has resolved yet. Its presence freezes the threshold: the whole point of
	// the guard is defeated if the admin can answer a hold by raising the
	// limit and retrying.
	held *heldSync

	// running admits one sync at a time. A second concurrent request is
	// rejected rather than queued: two runs applying overlapping snapshots
	// would interleave their transactions unpredictably.
	running atomic.Bool
}

// NewOrganizationSyncService creates the service. A nil fetcher is tolerated
// at construction (mirrors the "disabled until configured" convention used
// elsewhere); Sync reports the misconfiguration when actually invoked.
func NewOrganizationSyncService(
	repo orgprovider.Repository,
	fetcher SnapshotFetcher,
	userRepo user.Repository,
	teamRepo team.Repository,
	opts ...Option,
) *OrganizationSyncService {
	s := &OrganizationSyncService{
		repo:     repo,
		fetcher:  fetcher,
		userRepo: userRepo,
		teamRepo: teamRepo,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Configured reports whether a sync could run at all.
func (s *OrganizationSyncService) Configured() bool {
	return s.fetcher != nil
}

// Sync fetches, validates and applies a snapshot with no overrides. This is
// the normal path and its behaviour is unchanged: a sync over the
// mass-deletion threshold is held.
func (s *OrganizationSyncService) Sync(ctx context.Context) (*SyncResult, error) {
	return s.SyncWithOptions(ctx, SyncOptions{})
}

// SyncWithOptions fetches, validates and applies a snapshot.
//
// Returns ErrSyncInProgress when another run holds the lock. All persistence
// happens in a single transaction, so a failure at any point -- including the
// mass-deletion guard tripping -- leaves THC untouched.
//
// opts.OverrideMassDeletion waives one check and one check only. The snapshot
// is still fetched fresh and revalidated here, the protected-record
// allowlists still apply in persistence, and provider/transaction failures are
// handled exactly as they are for a normal run.
func (s *OrganizationSyncService) SyncWithOptions(ctx context.Context, opts SyncOptions) (*SyncResult, error) {
	if !s.running.CompareAndSwap(false, true) {
		return nil, ErrSyncInProgress
	}
	defer s.running.Store(false)

	log := logger.Get()
	startedAt := time.Now().UTC()

	if s.fetcher == nil {
		return nil, ErrProviderNotConfigured
	}

	// Captured once, up front, and used for this entire attempt. While a hold
	// is unresolved this returns the held attempt's own threshold, so raising
	// the setting can never be the answer to a hold -- only the explicit,
	// authorized override is.
	maxDeletePercent, err := s.thresholdForRun(ctx)
	if err != nil {
		return nil, err
	}

	snapshot, err := s.fetcher.FetchSnapshot(ctx)
	if err != nil {
		return nil, errors.Join(ErrProviderFetchFailed, err)
	}
	if snapshot == nil {
		return nil, errors.Join(ErrInvalidSnapshot, errors.New("provider returned no snapshot"))
	}

	knownLevels, err := s.repo.KnownHierarchyLevelIDs(ctx)
	if err != nil {
		return nil, err
	}

	filtered, err := FilterSnapshot(snapshot, knownLevels)
	if err != nil {
		// Names a record identity, not a credential, so it is safe to return
		// to an admin and is the only way to diagnose a bad snapshot.
		return nil, errors.Join(ErrInvalidSnapshot, err)
	}

	if err := filtered.Snapshot.ValidationError(); err != nil {
		// Validation errors name records, not credentials, so they are safe to
		// return to an admin and are the only way to diagnose a bad snapshot.
		return nil, errors.Join(ErrInvalidSnapshot, err)
	}

	applied, err := s.repo.ApplySnapshot(ctx, orgprovider.ApplyInput{
		Snapshot: filtered.Snapshot,
		// The raw, pre-filter payload's own counts -- not len(filtered.Snapshot.*),
		// which only reflects what survived filtering. See ApplyInput's doc.
		IncomingUsers:            len(snapshot.Users),
		IncomingTeams:            len(snapshot.Teams),
		IncomingMemberships:      len(snapshot.Memberships),
		PreservedMemberUserIDs:   filtered.PreservedMemberUserIDs,
		PreserveReportsToUserIDs: filtered.PreserveReportsToUserIDs,
		MaxDeletePercent:         maxDeletePercent,
		OverrideMassDeletion:     opts.OverrideMassDeletion,
		ConfirmedMassDeletion:    opts.ConfirmedMassDeletion,
	})
	if err != nil {
		// A tripped guard freezes the threshold until the hold is resolved, so
		// the counts an admin reviews are the counts any override applies to.
		var hold *orgprovider.MassDeletionHoldError
		if errors.As(err, &hold) {
			s.recordHold(maxDeletePercent, hold.Report)
		}
		return nil, err
	}

	// The attempt applied, so whatever hold preceded it is resolved and the
	// threshold is editable again.
	s.DismissHold()

	// The supervisor chain is a denormalized cache derived from reports_to.
	// Rebuilding it is best-effort and deliberately outside the transaction:
	// the sync has already committed, and a stale cache is recoverable whereas
	// a rolled-back org import is not. A failure here is logged but does not
	// change the sync's reported status -- it is a real (if narrow) gap
	// between "the sync completed" and "every derived structure is
	// consistent," called out explicitly rather than silently claimed away.
	supervisorChainErr := s.rederiveSupervisorChains(ctx, filtered.Snapshot.Teams)

	result := &SyncResult{
		Status:               "completed",
		TeamsSynced:          applied.TeamsSynced,
		UsersSynced:          applied.UsersSynced,
		MembershipsSynced:    applied.MembershipsSynced,
		MembershipsRemoved:   applied.MembershipsRemoved,
		HealthChecksDisabled: applied.HealthChecksDisabled,
		HealthChecksEnabled:  applied.HealthChecksEnabled,
		UsersDeleted:         applied.UsersDeleted,
		TeamsDeleted:         applied.TeamsDeleted,
		ActionItemsDeleted:   applied.ActionItemsDeleted,
		UsersSkipped:         len(filtered.SkippedUsers),
		SkippedUsers:         filtered.SkippedUsers,
		ManagerLinksCleared:  filtered.ClearedManagers,
		TeamLeadsCleared:     filtered.ClearedTeamLeads,
		MembershipsDiscarded: filtered.DroppedMemberships + filtered.DuplicateMemberships,
		StartedAt:            startedAt,
		CompletedAt:          time.Now().UTC(),
	}
	if supervisorChainErr != nil {
		result.Status = "completed_with_warnings"
	}
	if applied.MassDeletionOverride != nil {
		result.MassDeletionOverridden = true
		result.MassDeletion = applied.MassDeletionOverride

		// An override is a deliberate, destructive, human decision, so it gets
		// its own audit record naming who made it and exactly what it waived --
		// not just a line in the completion log below.
		report := applied.MassDeletionOverride
		actor := opts.ActorUserID
		if actor == "" {
			actor = "unknown"
		}
		log.Security("org_sync_mass_deletion_override").
			UserID(actor).
			Details(fmt.Sprintf(
				"administrator overrode the mass-deletion hold: users %d/%d (%.1f%%), teams %d/%d (%.1f%%), memberships cascaded %s, threshold %.1f%%",
				report.Users.Deleting, report.Users.Existing, report.Users.Percent,
				report.Teams.Deleting, report.Teams.Existing, report.Teams.Percent,
				formatCascade(report.Memberships),
				report.Threshold,
			)).
			Log()
	}

	log.WithFields(map[string]interface{}{
		"usersSynced":       result.UsersSynced,
		"teamsSynced":       result.TeamsSynced,
		"membershipsSynced": result.MembershipsSynced,
		"usersDeleted":      result.UsersDeleted,
		"teamsDeleted":      result.TeamsDeleted,
		"usersSkipped":      result.UsersSkipped,
	}).Info("organization provider sync completed")

	return result, nil
}

// formatCascade renders the optional membership cascade for the audit line.
func formatCascade(m *orgprovider.DeletionMetric) string {
	if m == nil {
		return "unmeasured"
	}
	return fmt.Sprintf("%d/%d (%.1f%%)", m.Deleting, m.Existing, m.Percent)
}

// LockState reports whether the mass-deletion threshold is currently frozen,
// and at what value. An expired hold is dropped here rather than lingering, so
// callers never have to reason about staleness themselves.
func (s *OrganizationSyncService) LockState() SyncLockState {
	state := SyncLockState{Syncing: s.running.Load()}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.held != nil {
		if time.Since(s.held.heldAt) > MassDeletionHoldTTL {
			s.held = nil
		} else {
			state.Held = true
			state.Threshold = s.held.threshold
			state.HeldAt = s.held.heldAt
		}
	}
	return state
}

// DismissHold clears an unresolved hold, which re-enables threshold editing.
// It is what an administrator does after deciding to fix the provider data
// instead of overriding, and what a completed sync does implicitly. It reports
// whether there was anything to clear.
func (s *OrganizationSyncService) DismissHold() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	existed := s.held != nil
	s.held = nil
	return existed
}

// recordHold remembers a tripped guard, together with the threshold it was
// judged against.
func (s *OrganizationSyncService) recordHold(threshold float64, report orgprovider.MassDeletionReport) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.held = &heldSync{threshold: threshold, report: report, heldAt: time.Now().UTC()}
}

// thresholdForRun returns the threshold this attempt is judged against: the
// frozen value while a hold is unresolved, otherwise the configured one.
func (s *OrganizationSyncService) thresholdForRun(ctx context.Context) (float64, error) {
	if state := s.LockState(); state.Held {
		return state.Threshold, nil
	}
	return s.maxDeletePercent(ctx)
}

// maxDeletePercent resolves the mass-deletion threshold for this run, in
// precedence order:
//
//  1. the threshold an administrator saved through admin settings,
//  2. ORG_SYNC_MAX_DELETE_PERCENT, for deployments that configured the guard
//     before the admin setting existed,
//  3. DefaultMaxDeletePercent (20%).
//
// A stored value outside 1-100, NaN, or infinite is an error rather than a
// reason to fall through: a guard nobody can trust must not be quietly
// replaced by a different one. A store read failure is likewise an error --
// the sync does not proceed on an unknown threshold.
func (s *OrganizationSyncService) maxDeletePercent(ctx context.Context) (float64, error) {
	var saved *float64
	if s.thresholds != nil {
		var err error
		saved, err = s.thresholds.GetOrgSyncMaxDeletePercent(ctx)
		if err != nil {
			return 0, fmt.Errorf("failed to read the configured mass-deletion threshold: %w", err)
		}
	}

	value, _, err := ResolveMaxDeletePercent(saved)
	return value, err
}

// Threshold sources, reported to admins so the settings UI can say where the
// value in force actually came from.
const (
	ThresholdSourceAdmin       = "admin"
	ThresholdSourceEnvironment = "environment"
	ThresholdSourceDefault     = "default"
)

// ResolveMaxDeletePercent applies the precedence rule in one place, so the
// guard and the settings endpoint can never disagree about which threshold is
// in force. saved is the administrator's stored value, or nil when none.
func ResolveMaxDeletePercent(saved *float64) (float64, string, error) {
	if saved != nil {
		if !validDeletePercent(*saved) {
			return 0, "", ErrMaxDeletePercentNotConfigured
		}
		return *saved, ThresholdSourceAdmin, nil
	}
	if value, ok := envMaxDeletePercent(); ok {
		return value, ThresholdSourceEnvironment, nil
	}
	return DefaultMaxDeletePercent, ThresholdSourceDefault, nil
}

// validDeletePercent reports whether a persisted threshold is usable. NaN and
// Inf are rejected explicitly: both slip past a naive range comparison.
func validDeletePercent(value float64) bool {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return false
	}
	return value >= MinConfigurableDeletePercent && value <= MaxConfigurableDeletePercent
}

// envMaxDeletePercent reads the deployment-level fallback. An unset, empty, or
// unparseable value simply means "no fallback configured" -- the built-in
// default then applies -- so this reports usability rather than erroring.
func envMaxDeletePercent() (float64, bool) {
	raw := os.Getenv(EnvMaxDeletePercent)
	if raw == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(raw, 64)
	// Reject NaN/Inf since they can bypass the percentage validation.
	if err != nil || value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return value, true
}

// rederiveSupervisorChains refreshes team_supervisors for the synced teams,
// mirroring the derivation the admin team handler performs after a lead
// changes. Returns a non-nil error if any team's chain failed to rebuild, so
// the caller can surface that the post-commit step was incomplete.
func (s *OrganizationSyncService) rederiveSupervisorChains(ctx context.Context, teams []orgsnapshot.Team) error {
	if s.userRepo == nil || s.teamRepo == nil {
		return nil
	}

	log := logger.Get()
	var firstErr error

	for _, t := range teams {
		if orgprovider.IsProtectedTeam(t.ID) {
			continue // persistence never updates a protected team, so its supervisor chain must not move either
		}

		stored, err := s.teamRepo.FindByID(ctx, t.ID)
		if err != nil {
			log.Warn("failed to look up team " + t.ID + " for supervisor chain rederivation: " + err.Error())
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if stored == nil || stored.TeamLeadID == nil {
			continue
		}

		supervisors, err := s.userRepo.FindSupervisorChainUp(ctx, *stored.TeamLeadID)
		if err != nil {
			log.Warn("failed to derive supervisor chain for team " + t.ID + ": " + err.Error())
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		chain := make([]*team.SupervisorLink, len(supervisors))
		for i, sup := range supervisors {
			chain[i] = &team.SupervisorLink{UserID: sup.ID, LevelID: sup.HierarchyLevelID}
		}

		if err := s.teamRepo.UpdateSupervisorChain(ctx, t.ID, chain); err != nil {
			log.Warn("failed to save derived supervisor chain for team " + t.ID + ": " + err.Error())
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	return firstErr
}
