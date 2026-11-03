package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	"github.com/agopalakrishnan/teams360/backend/domain/orgsyncrun"
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
	// ErrHoldPersistFailed means a tripped mass-deletion guard could not be
	// saved. The run fails closed; the storage cause is logged, not returned.
	ErrHoldPersistFailed = errors.New("could not persist the sync hold")
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

// WithRunStore enables persisted, asynchronous runs and the cross-instance lock.
// Without it the service only offers the synchronous Sync path.
func WithRunStore(store orgsyncrun.Store) Option {
	return func(s *OrganizationSyncService) {
		s.runs = store
	}
}

// WithHoldStore persists the unresolved mass-deletion hold in the shared
// database so every API replica sees the same frozen threshold. Without it the
// hold lives in this process's memory only, which is correct for a single
// instance but invisible to other replicas and lost on restart.
func WithHoldStore(store orgsyncrun.HoldStore) Option {
	return func(s *OrganizationSyncService) {
		s.holds = store
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
	// MembershipChanges is real additions, reactivations and removals only.
	MembershipChanges int `json:"membershipChanges"`

	HealthChecksDisabled int `json:"healthChecksDisabled"`
	HealthChecksEnabled  int `json:"healthChecksEnabled"`

	UsersDeactivated int `json:"usersDeactivated"`
	TeamsDeactivated int `json:"teamsDeactivated"`

	InsertedUsers  int `json:"insertedUsers"`
	UpdatedUsers   int `json:"updatedUsers"`
	UnchangedUsers int `json:"unchangedUsers"`
	InsertedTeams  int `json:"insertedTeams"`
	UpdatedTeams   int `json:"updatedTeams"`
	UnchangedTeams int `json:"unchangedTeams"`

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

// heldSync is the in-process record of an unresolved mass-deletion hold. It is
// only used when no HoldStore is wired (single instance, tests); otherwise the
// hold lives in the database so all replicas share it.
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

	// runs persists run status and provides the cross-instance lock. Nil in
	// deployments/tests that only use the synchronous path.
	runs orgsyncrun.Store

	// holds persists the unresolved hold across replicas. Nil falls back to the
	// in-process held record below.
	holds orgsyncrun.HoldStore

	// mu is the single in-process serialization boundary for everything that
	// decides whether the mass-deletion guard may change: sync admission
	// (running), the in-process unresolved-hold record (held), threshold
	// writes, and hold dismissal. Each of those is one critical section under
	// mu, so a check ("is a sync running?") and the action it gates ("write the
	// threshold", "clear the hold", "start the sync") cannot be separated by
	// another request on this replica. Across replicas the persisted run and
	// hold stores are the authority.
	mu sync.Mutex
	// held records a sync that the mass-deletion guard stopped and that nobody
	// has resolved yet. Its presence freezes the threshold: the whole point of
	// the guard is defeated if the admin can answer a hold by raising the
	// limit and retrying.
	held *heldSync

	// running admits one sync at a time (guarded by mu). A second concurrent
	// request is rejected rather than queued: two runs applying overlapping
	// snapshots would interleave their transactions unpredictably.
	running bool
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
	if !s.admit() {
		return nil, ErrSyncInProgress
	}
	defer s.release()

	// With a run store, persisted runs on any replica serialize on the
	// cross-instance advisory lock. This synchronous path must take the same
	// lock, or it could apply concurrently with one of them. A busy lock is
	// rejected rather than waited for, like the in-process flag above.
	if s.runs != nil {
		release, ok, err := s.runs.TryLock(ctx)
		if err != nil {
			return nil, fmt.Errorf("could not take the synchronization lock: %w", err)
		}
		if !ok {
			return nil, ErrSyncInProgress
		}
		defer release()
	}

	return s.syncCore(ctx, opts, "", nil)
}

// syncCore is the one implementation of fetch -> validate -> apply. Both the
// synchronous path and persisted runs call it, so there is a single sync
// behaviour. runID and onStage are empty/nil for the synchronous path.
func (s *OrganizationSyncService) syncCore(ctx context.Context, opts SyncOptions, runID string, onStage func(orgsyncrun.Stage)) (*SyncResult, error) {
	stage := func(st orgsyncrun.Stage) {
		if onStage != nil {
			onStage(st)
		}
	}
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

	stage(orgsyncrun.StageFetching)
	snapshot, err := s.fetcher.FetchSnapshot(ctx)
	if err != nil {
		return nil, errors.Join(ErrProviderFetchFailed, err)
	}
	if snapshot == nil {
		return nil, errors.Join(ErrInvalidSnapshot, errors.New("provider returned no snapshot"))
	}

	stage(orgsyncrun.StageValidating)
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
		Snapshot:     filtered.Snapshot,
		SnapshotType: snapshotTypeOf(snapshot),
		RunID:        runID,
		OnStage: func(name string) {
			stage(orgsyncrun.Stage(name))
		},
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
			if saveErr := s.recordHold(ctx, maxDeletePercent, hold.Report); saveErr != nil {
				// Fail closed: surface the persistence failure instead of the
				// hold, so the run is not reported as a cleanly held sync.
				return nil, saveErr
			}
		}
		return nil, err
	}

	// The attempt applied, so whatever hold preceded it is resolved and the
	// threshold is editable again.
	//
	// The apply has already committed, so a failure here cannot undo it and
	// must not turn a committed sync into a reported failure. It is safe to
	// continue: a hold that stays behind keeps the threshold frozen until it
	// expires or an admin dismisses it, which errs toward the stricter guard.
	if _, err := s.DismissHold(ctx); err != nil {
		logger.Get().WithError(err).Error("sync applied but its earlier hold could not be cleared; threshold stays frozen until the hold expires or is dismissed")
	}

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
		MembershipChanges:    applied.MembershipsAdded + applied.MembershipsRemoved,
		HealthChecksDisabled: applied.HealthChecksDisabled,
		HealthChecksEnabled:  applied.HealthChecksEnabled,
		UsersDeactivated:     applied.UsersDeactivated,
		TeamsDeactivated:     applied.TeamsDeactivated,
		InsertedUsers:        applied.InsertedUsers,
		UpdatedUsers:         applied.UpdatedUsers,
		UnchangedUsers:       applied.UnchangedUsers,
		InsertedTeams:        applied.InsertedTeams,
		UpdatedTeams:         applied.UpdatedTeams,
		UnchangedTeams:       applied.UnchangedTeams,
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
		"usersDeactivated":  result.UsersDeactivated,
		"teamsDeactivated":  result.TeamsDeactivated,
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
//
// "Syncing" comes from the persisted run when a run store is configured: only a
// run whose status is PENDING or RUNNING counts. That is authoritative across
// replicas and flips the instant a run's SUCCESS commits, whereas the
// in-process flag is cleared a moment later and could be read mid-teardown,
// leaving the threshold looking locked after the sync had finished. Runs a
// crashed worker abandoned are swept first so they cannot hold the lock either.
// Without a run store (the synchronous path) the in-process flag is the only
// signal there is.
func (s *OrganizationSyncService) LockState(ctx context.Context) SyncLockState {
	state, err := s.heldState(ctx)
	if err != nil {
		// Fail closed, like syncing: if the hold cannot be read, do not let the
		// threshold change underneath a hold that may exist.
		logger.Get().WithError(err).Warn("could not read the sync hold; treating the threshold as locked")
		state = SyncLockState{Held: true}
	}
	state.Syncing = s.syncing(ctx)
	return state
}

// lockStateLocked is LockState for a caller that already holds mu.
func (s *OrganizationSyncService) lockStateLocked(ctx context.Context) SyncLockState {
	state, err := s.heldStateLocked(ctx)
	if err != nil {
		logger.Get().WithError(err).Warn("could not read the sync hold; treating the threshold as locked")
		state = SyncLockState{Held: true}
	}
	state.Syncing = s.syncingLocked(ctx)
	return state
}

// syncing answers "is a sync in progress" as described on LockState.
func (s *OrganizationSyncService) syncing(ctx context.Context) bool {
	if s.runs == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
	}
	return s.syncingLocked(ctx)
}

// syncingLocked is syncing for a caller that already holds mu.
func (s *OrganizationSyncService) syncingLocked(ctx context.Context) bool {
	if s.runs == nil {
		return s.running
	}
	s.recoverStaleRuns(ctx)
	active, err := s.runs.Active(ctx)
	if err != nil {
		// Fail closed: if the answer cannot be read, do not let the threshold be
		// changed underneath a sync that may be running.
		logger.Get().WithError(err).Warn("could not read the active sync run; treating the threshold as locked")
		return true
	}
	return active != nil
}

// heldState reports only the unresolved-hold part of the lock. It reads the
// shared database when a HoldStore is wired, so every replica agrees; an
// expired hold is cleared on read rather than lingering.
func (s *OrganizationSyncService) heldState(ctx context.Context) (SyncLockState, error) {
	if s.holds == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
	}
	return s.heldStateLocked(ctx)
}

// heldStateLocked is heldState for a caller that already holds mu.
func (s *OrganizationSyncService) heldStateLocked(ctx context.Context) (SyncLockState, error) {
	var state SyncLockState

	if s.holds != nil {
		h, err := s.holds.GetHold(ctx)
		if err != nil {
			return state, err
		}
		if h == nil {
			return state, nil
		}
		if time.Since(h.HeldAt) > MassDeletionHoldTTL {
			if _, err := s.holds.ClearHold(ctx); err != nil {
				return state, err
			}
			return state, nil
		}
		state.Held = true
		state.Threshold = h.Threshold
		state.HeldAt = h.HeldAt
		return state, nil
	}

	if s.held != nil {
		if time.Since(s.held.heldAt) > MassDeletionHoldTTL {
			s.held = nil
		} else {
			state.Held = true
			state.Threshold = s.held.threshold
			state.HeldAt = s.held.heldAt
		}
	}
	return state, nil
}

// admit claims the single in-process sync slot, reporting false when one is
// running.
func (s *OrganizationSyncService) admit() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return false
	}
	s.running = true
	return true
}

func (s *OrganizationSyncService) release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
}

// ThresholdLockedError is returned by UpdateMaxDeletePercent when a sync is
// running or held, carrying the state that caused the refusal.
type ThresholdLockedError struct{ State SyncLockState }

func (e *ThresholdLockedError) Error() string {
	if e.State.Syncing {
		return "the deletion threshold cannot be changed while a synchronization is running"
	}
	return "the deletion threshold cannot be changed while a synchronization is held for review"
}

// UpdateMaxDeletePercent persists a new threshold via write, but only if no
// sync is running or held, and does so while holding the same mutex sync
// admission takes. A sync therefore cannot be admitted on this replica between
// the check and the write, which a handler-level LockState() read followed by
// an unprotected repository write could not guarantee. It returns
// *ThresholdLockedError when refused. write must not call back into this
// service.
func (s *OrganizationSyncService) UpdateMaxDeletePercent(ctx context.Context, percent float64, write func(context.Context, float64) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state := s.lockStateLocked(ctx); state.Locked() {
		return &ThresholdLockedError{State: state}
	}
	return write(ctx, percent)
}

// DismissHoldIfIdle clears an unresolved hold on behalf of an administrator,
// refusing with ErrSyncInProgress while a sync is running. The check and the
// clear are one critical section under the admission mutex: a sync that is
// admitted afterwards can create a fresh hold, but it can never have that
// hold cleared by a dismissal that was decided before it started. It reports
// whether there was a hold to clear.
func (s *OrganizationSyncService) DismissHoldIfIdle(ctx context.Context) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.syncingLocked(ctx) {
		return false, ErrSyncInProgress
	}
	return s.clearHoldLocked(ctx)
}

// DismissHold clears an unresolved hold, which re-enables threshold editing.
// It is what an administrator does after deciding to fix the provider data
// instead of overriding, and what a completed sync does implicitly. It reports
// whether there was anything to clear. A storage failure is returned, not
// folded into "nothing to clear": the hold may still be active.
func (s *OrganizationSyncService) DismissHold(ctx context.Context) (bool, error) {
	if s.holds == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
	}
	return s.clearHoldLocked(ctx)
}

// clearHoldLocked clears the hold from the shared store, or from the
// in-process record when none is wired. The caller holds mu.
func (s *OrganizationSyncService) clearHoldLocked(ctx context.Context) (bool, error) {
	if s.holds != nil {
		existed, err := s.holds.ClearHold(ctx)
		if err != nil {
			logger.Get().WithError(err).Error("could not clear the sync hold")
			return false, fmt.Errorf("could not clear the sync hold: %w", err)
		}
		return existed, nil
	}
	existed := s.held != nil
	s.held = nil
	return existed, nil
}

// recordHold remembers a tripped guard, together with the threshold it was
// judged against.
//
// It returns an error when the shared hold cannot be saved. The caller must
// fail the run on that error rather than report a normal hold: an unsaved
// hold leaves the threshold editable, so a retry could bypass the guard.
func (s *OrganizationSyncService) recordHold(ctx context.Context, threshold float64, report orgprovider.MassDeletionReport) error {
	heldAt := time.Now().UTC()
	if s.holds != nil {
		if err := s.holds.SaveHold(ctx, orgsyncrun.Hold{Threshold: threshold, Report: report, HeldAt: heldAt}); err != nil {
			// Log the storage cause here; the returned error carries only the
			// sentinel so raw store errors never reach API clients.
			logger.Get().WithError(err).Error("could not persist the sync hold")
			return ErrHoldPersistFailed
		}
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.held = &heldSync{threshold: threshold, report: report, heldAt: heldAt}
	return nil
}

// thresholdForRun returns the threshold this attempt is judged against: the
// frozen value while a hold is unresolved, otherwise the configured one.
func (s *OrganizationSyncService) thresholdForRun(ctx context.Context) (float64, error) {
	state, err := s.heldState(ctx)
	if err != nil {
		return 0, fmt.Errorf("read sync hold: %w", err)
	}
	if state.Held {
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
	if err != nil || !validDeletePercent(value) {
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
		if stored == nil {
			continue
		}
		if stored.TeamLeadID == nil {
			// The sync removed this team's lead and the FK cleared
			// teams.team_lead_id; the supervisor chain must not keep
			// pointing at a lead that no longer applies.
			if err := s.teamRepo.UpdateSupervisorChain(ctx, t.ID, nil); err != nil {
				log.Warn("failed to clear supervisor chain for team " + t.ID + ": " + err.Error())
				if firstErr == nil {
					firstErr = err
				}
			}
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

// snapshotTypeOf decides whether absent records may be deactivated. A payload
// only counts as COMPLETE when the contract says so: either snapshotType is
// explicitly "COMPLETE", or it is omitted, which contract v1.0 defines as a
// complete snapshot. Non-empty is never taken as evidence of completeness --
// ValidationError has already rejected empty users/teams -- and PARTIAL (or any
// value validation did not accept) keeps every missing record untouched.
func snapshotTypeOf(snap *orgsnapshot.Snapshot) orgprovider.SnapshotType {
	switch snap.SnapshotType {
	case "", orgsnapshot.TypeComplete:
		return orgprovider.SnapshotComplete
	default:
		return orgprovider.SnapshotPartial
	}
}

// Timing for persisted runs. Variables, not constants, so tests can shrink them.
var (
	// LockWaitTimeout bounds how long a run stays PENDING waiting for the lock.
	LockWaitTimeout = 2 * time.Minute
	// LockRetryInterval is the pause between lock attempts.
	LockRetryInterval = 2 * time.Second
	// RunTimeout bounds one whole run's execution.
	RunTimeout = 10 * time.Minute
	// stageUpdateTimeout bounds one progress write.
	stageUpdateTimeout = 5 * time.Second
	// StaleRunTTL is how long a run may go without a heartbeat before the
	// recovery sweep fails it. It must exceed the longest gap between stage
	// updates; a healthy run heartbeats at every stage change.
	StaleRunTTL = 15 * time.Minute
)

// RunsEnabled reports whether persisted asynchronous runs are available.
func (s *OrganizationSyncService) RunsEnabled() bool { return s.runs != nil }

// StartRun records a PENDING run and executes it in the background. It returns
// as soon as the PENDING row is persisted, never after the sync itself. If a run
// is already active it returns that run with ErrActiveRunExists instead of
// starting a second one. A repeated idempotencyKey returns the original run.
func (s *OrganizationSyncService) StartRun(ctx context.Context, opts SyncOptions, idempotencyKey string) (*orgsyncrun.Run, error) {
	if s.runs == nil {
		return nil, errors.New("persisted sync runs are not configured")
	}
	if s.fetcher == nil {
		return nil, ErrProviderNotConfigured
	}

	s.recoverStaleRuns(ctx)

	actor := opts.ActorUserID
	if actor == "" {
		actor = "scheduler"
	}
	id, err := newRunID()
	if err != nil {
		return nil, err
	}
	run, created, err := s.runs.CreatePending(ctx, orgsyncrun.NewParams{
		ID: id, InitiatedBy: actor, IdempotencyKey: idempotencyKey, SnapshotType: orgprovider.SnapshotComplete,
	})
	if err != nil {
		return run, err
	}
	if !created {
		return run, nil // idempotent replay: already scheduled by the first request
	}

	// Detached from the request: the admin may close the tab, and the work must
	// still finish or fail visibly.
	go s.executeRun(context.WithoutCancel(ctx), run.ID, opts)
	return run, nil
}

func newRunID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate run id: %w", err)
	}
	return "sync_" + hex.EncodeToString(b), nil
}

// executeRun drives one run PENDING -> RUNNING -> SUCCESS|FAILED. SUCCESS is
// written by ApplySnapshot inside the data transaction, so it can only exist
// after a commit; this function only ever records FAILED.
func (s *OrganizationSyncService) executeRun(ctx context.Context, runID string, opts SyncOptions) {
	log := logger.Get().WithFields(map[string]interface{}{"runId": runID})
	started := time.Now()
	ctx, cancel := context.WithTimeout(ctx, RunTimeout)
	defer cancel()

	currentStage := orgsyncrun.StageAcquiringLock
	fail := func(stage orgsyncrun.Stage, code, message string, hold json.RawMessage) {
		err := s.runs.Fail(context.WithoutCancel(ctx), runID, orgsyncrun.Failure{
			Stage: stage, Code: code, Message: message, HoldReport: hold,
		})
		if errors.Is(err, orgsyncrun.ErrNotTransitionable) {
			log.Warn("run already finished elsewhere; not overwriting its result")
			return
		}
		if err != nil {
			log.WithError(err).Error("failed to record sync run failure")
		}
		log.WithFields(map[string]interface{}{
			"status": orgsyncrun.StatusFailed, "stage": stage, "code": code,
			"durationMs": time.Since(started).Milliseconds(),
		}).Warn("organization sync run failed")
	}
	setStage := func(st orgsyncrun.Stage) {
		currentStage = st
		// Progress is best-effort: a bounded wait means a blocked progress write
		// can never hang the sync itself.
		stageCtx, stageCancel := context.WithTimeout(ctx, stageUpdateTimeout)
		defer stageCancel()
		if err := s.runs.SetStage(stageCtx, runID, st); err != nil && !errors.Is(err, orgsyncrun.ErrNotTransitionable) {
			log.WithError(err).Warn("failed to record sync stage")
		}
	}

	// In-process admission still guards this replica's threshold-lock state.
	if !s.admit() {
		fail(orgsyncrun.StageAcquiringLock, orgsyncrun.CodeLockUnavailable, "Another synchronization is running on this server.", nil)
		return
	}
	defer s.release()

	// PENDING while waiting for the lock: a busy lock is retried, not failed.
	release, err := s.acquireLock(ctx, setStage)
	if err != nil {
		fail(orgsyncrun.StageAcquiringLock, orgsyncrun.CodeLockUnavailable, err.Error(), nil)
		return
	}
	defer release()

	if err := s.runs.MarkRunning(ctx, runID, workerID()); err != nil {
		if errors.Is(err, orgsyncrun.ErrNotTransitionable) {
			// Someone (the recovery sweep) already failed this run: do not run it.
			log.WithError(err).Warn("run could not be moved to RUNNING; abandoning")
			return
		}
		// Any other error (a database problem, say) must not leave the run
		// PENDING: that would block new runs and the threshold lock until the
		// recovery sweep fails it 15 minutes later. Fail it now.
		code, message, hold := classifyRunError(err)
		fail(currentStage, code, message, hold)
		return
	}
	log.Info("organization sync run started")

	_, err = s.syncCore(ctx, opts, runID, func(st orgsyncrun.Stage) { setStage(st) })
	if err == nil {
		log.WithFields(map[string]interface{}{
			"status": orgsyncrun.StatusSuccess, "durationMs": time.Since(started).Milliseconds(),
		}).Info("organization sync run succeeded")
		return
	}
	code, message, hold := classifyRunError(err)
	fail(currentStage, code, message, hold)
}

// acquireLock retries the cross-instance lock until LockWaitTimeout.
func (s *OrganizationSyncService) acquireLock(ctx context.Context, setStage func(orgsyncrun.Stage)) (func(), error) {
	setStage(orgsyncrun.StageAcquiringLock)
	deadline := time.Now().Add(LockWaitTimeout)
	for {
		release, ok, err := s.runs.TryLock(ctx)
		if err != nil {
			return nil, errors.New("could not reach the database to take the synchronization lock")
		}
		if ok {
			return release, nil
		}
		if time.Now().After(deadline) {
			return nil, errors.New("another synchronization is holding the lock; try again shortly")
		}
		select {
		case <-ctx.Done():
			return nil, errors.New("synchronization was cancelled while waiting for the lock")
		case <-time.After(LockRetryInterval):
		}
	}
}

func workerID() string {
	host, _ := os.Hostname()
	return fmt.Sprintf("%s:%d", host, os.Getpid())
}

// classifyRunError maps an internal error onto a stable code and a message that
// is safe to show an administrator. Raw database errors are replaced by a
// generic sentence: they can carry identifiers and are logged, not stored.
func classifyRunError(err error) (code, message string, hold json.RawMessage) {
	var holdErr *orgprovider.MassDeletionHoldError
	switch {
	case errors.As(err, &holdErr):
		report, _ := json.Marshal(holdErr.Report)
		return orgsyncrun.CodeMassDeactivationHold,
			"This sync would deactivate an unusually large share of users or teams. No changes were made. Review the counts, then fix the provider data or re-run with an explicit override.",
			report
	case errors.Is(err, ErrProviderNotConfigured):
		return orgsyncrun.CodeProviderNotConfigured, "The organization provider is not configured.", nil
	case errors.Is(err, ErrInvalidSnapshot):
		// Validation messages name records, not credentials; they are the only
		// way to diagnose a bad payload.
		return orgsyncrun.CodeInvalidSnapshot, err.Error(), nil
	case errors.Is(err, ErrProviderFetchFailed):
		return orgsyncrun.CodeProviderFetchFailed, "The organization provider could not be reached or returned an unusable response.", nil
	case errors.Is(err, orgprovider.ErrStaleSnapshot):
		return orgsyncrun.CodeStaleSnapshot, "The provider snapshot is older than the one already applied, so it was not applied.", nil
	case errors.Is(err, ErrMaxDeletePercentNotConfigured):
		return orgsyncrun.CodeInternal, "The saved deactivation threshold is outside the allowed 1-100% range.", nil
	default:
		logger.Get().WithError(err).Error("organization sync run failed")
		return orgsyncrun.CodeDatabase, "The synchronization failed while saving changes. Nothing was changed. Check the server logs and try again.", nil
	}
}

// recoverStaleRuns fails runs a crashed worker left PENDING/RUNNING, so they
// cannot block admission forever or be reported as in progress indefinitely.
// Best-effort: a failed sweep must not block the request that triggered it.
func (s *OrganizationSyncService) recoverStaleRuns(ctx context.Context) {
	if n, err := s.runs.RecoverStale(ctx, StaleRunTTL); err != nil {
		logger.Get().WithError(err).Warn("stale sync run recovery failed")
	} else if n > 0 {
		logger.Get().WithFields(map[string]interface{}{"recovered": n}).Warn("recovered stale sync runs")
	}
}

// LatestRun returns the most recent run, after failing any that a crashed
// worker abandoned, so the status the UI polls can always reach a final state.
// It and GetRun back the status endpoints.
func (s *OrganizationSyncService) LatestRun(ctx context.Context) (*orgsyncrun.Run, error) {
	if s.runs == nil {
		return nil, nil
	}
	s.recoverStaleRuns(ctx)
	return s.runs.Latest(ctx)
}

// GetRun returns one run by id, after failing any that a crashed worker
// abandoned (see LatestRun).
func (s *OrganizationSyncService) GetRun(ctx context.Context, id string) (*orgsyncrun.Run, error) {
	if s.runs == nil {
		return nil, orgsyncrun.ErrNotFound
	}
	s.recoverStaleRuns(ctx)
	return s.runs.Get(ctx, id)
}
