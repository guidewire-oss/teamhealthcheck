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
	"github.com/google/uuid"
)

// EnvMaxDeletePercent names the environment variable configuring the
// mass-deletion guard. It is the deployment-level fallback: an administrator's
// saved setting takes precedence over it, and DefaultMaxDeletePercent applies
// when neither is present. Its value is set in .env (see .env.example).
const EnvMaxDeletePercent = "ORG_SYNC_MAX_DELETE_PERCENT"

// MassDeletionHoldTTL bounds how long a held sync keeps the threshold frozen.
// A hold that nobody resolves must not lock the setting forever -- an admin who
// walked away from the screen would otherwise leave it unchangeable. Applied
// identically to the in-memory hold and to the durable hold_active flag, so
// a restart-surviving hold expires on the same schedule an in-process one would.
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
	// SyncWithOptions actually returns the richer *SyncInProgressError, which
	// wraps this sentinel (see its Unwrap) so every errors.Is(err,
	// ErrSyncInProgress) check -- existing or new -- keeps matching.
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

// SyncInProgressError reports that a sync is already in flight, and which
// kind -- manual or scheduled, local or on another replica. It wraps
// ErrSyncInProgress via Unwrap, not a hand-rolled Is method, so the sentinel
// stays visible to errors.Unwrap and to future %w formatting, and every
// existing errors.Is(err, ErrSyncInProgress) check keeps matching untouched.
type SyncInProgressError struct {
	// Trigger is orgprovider.TriggerManual or orgprovider.TriggerScheduled,
	// or "" when it could not be determined (e.g. a lock held by another
	// replica, which this process cannot attribute) -- never guessed. A
	// guessed trigger would be an assertion this type cannot support, in a
	// message an admin acts on.
	Trigger string
	// Remote is true when another replica holds the lock, not this process's
	// own in-process admission flag.
	Remote bool
	// Instance identifies the remote holder, when known.
	Instance string
}

func (e *SyncInProgressError) Error() string {
	desc := triggerDescription(e.Trigger)
	if e.Remote {
		if e.Instance != "" {
			return fmt.Sprintf("%s is already in progress on another instance (%s)", desc, e.Instance)
		}
		return fmt.Sprintf("%s is already in progress on another instance", desc)
	}
	return fmt.Sprintf("%s is already in progress", desc)
}

// Unwrap keeps errors.Is(err, ErrSyncInProgress) true.
func (e *SyncInProgressError) Unwrap() error { return ErrSyncInProgress }

// triggerDescription renders a trigger as a mid-sentence lowercase phrase,
// used both in SyncInProgressError.Error and in thresholdLockMessage-style
// callers. An empty or unrecognized trigger renders as the vaguest true
// statement ("a synchronization") rather than defaulting to "manual", which
// would assert something this code cannot support.
func triggerDescription(trigger string) string {
	switch trigger {
	case orgprovider.TriggerScheduled:
		return "an automatic scheduled sync"
	case orgprovider.TriggerManual:
		return "a manual sync"
	default:
		return "a synchronization"
	}
}

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

// WithInstanceID overrides the auto-generated instance identifier used for
// cross-replica attribution. Tests use this for a deterministic value;
// production takes the generated default.
func WithInstanceID(id string) Option {
	return func(s *OrganizationSyncService) {
		s.instanceID = id
	}
}

// syncRun is the identity of one sync attempt, carried through the recording
// and logging helpers so every row and log line about it agrees.
type syncRun struct {
	id         string
	trigger    string
	instanceID string
	startedAt  time.Time
}

// log returns a logger that stamps every line with the run's identity. The
// field names are what to search for in the log platform.
func (r syncRun) log() *logger.Logger {
	return logger.Get().WithFields(map[string]interface{}{
		"sync_run_id": r.id,
		"trigger":     r.trigger,
		"instance_id": r.instanceID,
	})
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
	// RunID identifies this sync across the database, the logs and the API
	// response -- search for it in the logs to see exactly what happened.
	RunID  string `json:"runId"`
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

	// RunID identifies this attempt (a UUID). Optional: the manual handler and
	// the scheduler set it so the ID is known before the sync starts; when
	// empty, SyncWithOptions generates one.
	RunID string

	// Trigger identifies what started this attempt: orgprovider.TriggerManual
	// or orgprovider.TriggerScheduled. Empty defaults to TriggerManual -- the
	// manual handler leaves this unset; the scheduler sets it explicitly.
	Trigger string
}

// heldSync is the server-side memory of an unresolved mass-deletion hold. It
// lives in the process rather than the database deliberately: a hold belongs to
// a sync attempt, and a restart ends every attempt in flight along with it.
// The DURABLE counterpart (hold_active in org_sync_runs) is what keeps
// threshold editing frozen across a restart or on another replica -- see
// LockState.
type heldSync struct {
	// threshold is the value the held attempt was judged against, and the value
	// every further attempt uses until the hold is resolved.
	threshold float64
	report    orgprovider.MassDeletionReport
	heldAt    time.Time
}

// SyncLockState describes why -- and at what value -- the mass-deletion
// threshold is currently frozen, and what is currently running. The zero
// value means editable and idle.
type SyncLockState struct {
	// Syncing is true while a run holds the in-process admission lock on this
	// replica.
	Syncing bool
	// Held is true while an unexpired mass-deletion hold is unresolved --
	// fail-closed OR of the in-memory hold and the durable hold_active flag.
	Held bool
	// Threshold is the value in force for the run that caused the lock.
	Threshold float64
	// HeldAt is when the hold was recorded, zero unless Held.
	HeldAt time.Time
	// Trigger names what is Syncing, when known. Empty when unknown -- never
	// guessed.
	Trigger string
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

	// locker serializes syncs across every API replica. Nil means no
	// distributed locking configured -- tolerated for unit tests and a
	// genuinely single-instance deployment. See SyncLocker's doc for the
	// fail-closed-when-configured policy.
	locker SyncLocker

	// runs persists sync history and liveness. Nil means not configured, in
	// which case LockState falls back to in-memory-only state (today's
	// behaviour) and blocked/failed attempts are not durably recorded.
	runs SyncRunRecorder

	// instanceID identifies this process for cross-replica attribution.
	// Never a credential -- a hostname-derived value is fine to log or return
	// to an admin.
	instanceID string

	// mu guards held. It is not the sync admission lock -- that is running,
	// below -- only the small record describing an unresolved hold.
	mu sync.Mutex
	// held records a sync that the mass-deletion guard stopped and that nobody
	// has resolved yet. Its presence freezes the threshold: the whole point of
	// the guard is defeated if the admin can answer a hold by raising the
	// limit and retrying.
	held *heldSync

	// running admits one sync at a time IN THIS PROCESS. A second concurrent
	// request is rejected rather than queued: two runs applying overlapping
	// snapshots would interleave their transactions unpredictably. This is no
	// longer the cross-replica guarantee -- that is locker, above -- but it
	// remains the free, same-replica fast path.
	running atomic.Bool
	// runningTrigger is set right after running is won, cleared right before
	// it is released -- read by a same-replica SyncInProgressError so the
	// conflict message can name what is actually running.
	runningTrigger atomic.Value
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
	if s.instanceID == "" {
		s.instanceID = defaultInstanceID()
	}
	return s
}

// defaultInstanceID derives a process identifier from the hostname (a pod
// name in Kubernetes) plus a random suffix, so two processes on the same host
// never collide. Never a credential -- safe to log or return to an admin.
func defaultInstanceID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	return fmt.Sprintf("%s-%06d", host, time.Now().UnixNano()%1000000)
}

// Configured reports whether a sync could run at all.
func (s *OrganizationSyncService) Configured() bool {
	return s.fetcher != nil
}

// HasSyncLocker reports whether distributed locking is configured. Not
// consulted by main.go in the current single-instance deployment (see
// WithSyncLocker's doc); kept for tests and for any future caller that
// re-introduces a cross-replica locker and wants to assert it is wired.
func (s *OrganizationSyncService) HasSyncLocker() bool {
	return s.locker != nil
}

// Sync fetches, validates and applies a snapshot with no overrides. This is
// the normal path and its behaviour is unchanged: a sync over the
// mass-deletion threshold is held.
func (s *OrganizationSyncService) Sync(ctx context.Context) (*SyncResult, error) {
	return s.SyncWithOptions(ctx, SyncOptions{})
}

// SchedulerBlocked reports whether a scheduled occurrence should be skipped
// rather than attempted, and why: a sync already running on THIS replica
// (named by trigger), or an unresolved IN-MEMORY hold. Deliberately reads
// only in-memory state, never the durable hold_active flag consulted by
// LockState -- after a restart or TTL expiry, the in-memory hold is gone and
// a re-attempt is safe (an over-threshold run writes nothing), and this
// method is how the scheduler gets to see that. This separation is
// structural: the Syncer interface the scheduler package depends on exposes
// SchedulerBlocked, never LockState, so the scheduler has no path to the
// durable flag at all.
//
// The reason returned is one of orgprovider.SkipManualRunning,
// SkipScheduledRunning, or SkipHoldUnresolved; "" when not blocked.
func (s *OrganizationSyncService) SchedulerBlocked() (bool, string) {
	if s.running.Load() {
		if trigger, _ := s.runningTrigger.Load().(string); trigger == orgprovider.TriggerScheduled {
			return true, orgprovider.SkipScheduledRunning
		}
		return true, orgprovider.SkipManualRunning
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.held == nil {
		return false, ""
	}
	if time.Since(s.held.heldAt) > MassDeletionHoldTTL {
		s.held = nil
		return false, ""
	}
	return true, orgprovider.SkipHoldUnresolved
}

// RecordSkip best-effort persists that a scheduled occurrence was skipped
// rather than attempted. A nil SyncRunRecorder makes this a no-op, matching
// every other best-effort recording method.
func (s *OrganizationSyncService) RecordSkip(ctx context.Context, runID, reason string) error {
	if s.runs == nil {
		return nil
	}
	return s.runs.RecordOrgSyncSkip(ctx, orgprovider.OrgSyncSkip{RunID: runID, Reason: reason, At: time.Now().UTC()})
}

// GetHistory returns the most recent sync history entries, newest first.
// Returns an empty list, not an error, when no SyncRunRecorder is configured.
func (s *OrganizationSyncService) GetHistory(ctx context.Context, limit int) ([]orgprovider.OrgSyncHistoryEntry, error) {
	if s.runs == nil {
		return []orgprovider.OrgSyncHistoryEntry{}, nil
	}
	return s.runs.ListOrgSyncHistory(ctx, limit)
}

// SyncWithOptions fetches, validates and applies a snapshot.
//
// Returns a *SyncInProgressError (wrapping ErrSyncInProgress) when another
// run holds the lock -- locally (this process's own admission flag) or, when
// a SyncLocker is configured, on another replica. All persistence for a
// successful or blocked outcome happens in a single transaction inside
// ApplySnapshot, so a failure at any point -- including the mass-deletion
// guard tripping -- leaves THC untouched.
//
// opts.OverrideMassDeletion waives one check and one check only. The snapshot
// is still fetched fresh and revalidated here, the protected-record
// allowlists still apply in persistence, and provider/transaction failures are
// handled exactly as they are for a normal run.
func (s *OrganizationSyncService) SyncWithOptions(ctx context.Context, opts SyncOptions) (*SyncResult, error) {
	trigger := opts.Trigger
	if trigger == "" {
		trigger = orgprovider.TriggerManual
	}
	runID := opts.RunID
	if runID == "" {
		runID = uuid.NewString()
	}
	run := syncRun{id: runID, trigger: trigger, instanceID: s.instanceID}

	// Tier 1: in-process admission. Free, and catches the common same-replica
	// case immediately. It is NO LONGER the cross-replica guarantee -- that is
	// tier 2, below -- only the fast, free, local check.
	if !s.running.CompareAndSwap(false, true) {
		localTrigger, _ := s.runningTrigger.Load().(string)
		run.log().WithField("running_trigger", localTrigger).Info("organization sync rejected: another sync is already in progress")
		return nil, &SyncInProgressError{Trigger: localTrigger}
	}
	s.runningTrigger.Store(trigger)
	defer func() {
		s.runningTrigger.Store("")
		s.running.Store(false)
	}()

	log := logger.Get()
	startedAt := time.Now().UTC()
	run.startedAt = startedAt
	run.log().Info("organization sync started")

	if s.fetcher == nil {
		return nil, ErrProviderNotConfigured
	}

	// Tier 2: the real, cross-replica mutual-exclusion primitive. A nil
	// locker means no distributed locking is configured (single-instance
	// deployments, unit tests); a CONFIGURED locker that errors while
	// acquiring fails the sync closed rather than proceeding unprotected.
	if s.locker != nil {
		lock, err := s.locker.TryLock(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to acquire the organization sync lock: %w", err)
		}
		if lock == nil {
			run.log().Info("organization sync rejected: another instance holds the sync lock")
			return nil, s.remoteInProgressError()
		}
		defer lock.Release()
	}

	// Captured once, up front, and used for this entire attempt. While a hold
	// is unresolved this returns the held attempt's own threshold (in-memory
	// or durable, whichever is fresher), so raising the setting can never be
	// the answer to a hold -- only the explicit, authorized override is.
	maxDeletePercent, err := s.thresholdForRun(ctx)
	if err != nil {
		return nil, s.failWith(ctx, run, err)
	}

	snapshot, err := s.fetcher.FetchSnapshot(ctx)
	if err != nil {
		return nil, s.failWith(ctx, run, errors.Join(ErrProviderFetchFailed, err))
	}
	if snapshot == nil {
		return nil, s.failWith(ctx, run, errors.Join(ErrInvalidSnapshot, errors.New("provider returned no snapshot")))
	}

	knownLevels, err := s.repo.KnownHierarchyLevelIDs(ctx)
	if err != nil {
		return nil, s.failWith(ctx, run, err)
	}

	// Names a record identity, not a credential, so it is safe to return to
	// an admin and is the only way to diagnose a bad snapshot -- true of both
	// failWith calls below.
	filtered, err := FilterSnapshot(snapshot, knownLevels)
	if err != nil {
		return nil, s.failWith(ctx, run, errors.Join(ErrInvalidSnapshot, err))
	}

	if err := filtered.Snapshot.ValidationError(); err != nil {
		return nil, s.failWith(ctx, run, errors.Join(ErrInvalidSnapshot, err))
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
		RunID:                    run.id,
		Trigger:                  trigger,
		InstanceID:               s.instanceID,
		StartedAt:                startedAt,
	})
	if err != nil {
		// A tripped guard freezes the threshold until the hold is resolved, so
		// the counts an admin reviews are the counts any override applies to.
		var hold *orgprovider.MassDeletionHoldError
		if errors.As(err, &hold) {
			s.recordHold(maxDeletePercent, hold.Report)
			s.recordBlockedAttempt(ctx, run, maxDeletePercent, hold.Report)
			return nil, err
		}

		// Ambiguous commit: recorded as unknown, never as a confirmed
		// rollback. Everything else here is a confirmed, deterministic
		// rollback (ApplySnapshot's own deferred Rollback already ran by the
		// time control returns here), recorded as none.
		writesStatus := orgprovider.WritesNone
		if errors.Is(err, orgprovider.ErrSyncCommitAmbiguous) {
			writesStatus = orgprovider.WritesUnknown
		}
		s.recordFailedAttempt(ctx, run, writesStatus, err)
		return nil, err
	}

	// The attempt applied, so whatever hold preceded it is resolved and the
	// threshold is editable again. The DURABLE hold flag is cleared inside
	// ApplySnapshot's own transaction (recordSuccessAttempt); this clears
	// only the in-memory side.
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
		RunID:                run.id,
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
		"sync_run_id":       run.id,
		"trigger":           run.trigger,
		"instance_id":       run.instanceID,
		"status":            result.Status,
		"usersSynced":       result.UsersSynced,
		"teamsSynced":       result.TeamsSynced,
		"membershipsSynced": result.MembershipsSynced,
		"usersDeleted":      result.UsersDeleted,
		"teamsDeleted":      result.TeamsDeleted,
		"usersSkipped":      result.UsersSkipped,
	}).Info("organization provider sync completed")

	return result, nil
}

// remoteInProgressError builds the conflict error when the distributed lock
// is held by another replica. This process cannot say which trigger or
// instance holds it, so both are left unknown rather than guessed.
func (s *OrganizationSyncService) remoteInProgressError() error {
	return &SyncInProgressError{Remote: true}
}

// failWith records a confirmed-rollback failed attempt (writesStatus=none)
// and returns err unchanged, so every pre-ApplySnapshot failure site can
// record-and-return in one line instead of repeating the same wrap/record/
// return fragment. Every failure this covers happens before ApplySnapshot is
// even called, so none of them can have written anything -- WritesNone is
// always correct here, unlike the ApplySnapshot error branch below, which
// must distinguish a confirmed rollback from an ambiguous commit.
func (s *OrganizationSyncService) failWith(ctx context.Context, run syncRun, err error) error {
	s.recordFailedAttempt(ctx, run, orgprovider.WritesNone, err)
	return err
}

// recordFailedAttempt best-effort persists a failed outcome. Never changes
// the sync's own return value -- see SyncRunRecorder's doc.
func (s *OrganizationSyncService) recordFailedAttempt(ctx context.Context, run syncRun, writesStatus string, cause error) {
	// Logged before the nil check so a failure is visible in the logs even
	// where no durable recorder is wired.
	run.log().WithError(cause).WithFields(map[string]interface{}{
		"status":        orgprovider.StatusFailed,
		"writes_status": writesStatus,
	}).Error("organization sync failed")
	if s.runs == nil {
		return
	}
	attempt := orgprovider.OrgSyncAttempt{
		RunID:        run.id,
		Trigger:      run.trigger,
		Status:       orgprovider.StatusFailed,
		StartedAt:    run.startedAt,
		FinishedAt:   time.Now().UTC(),
		InstanceID:   s.instanceID,
		WritesStatus: writesStatus,
		Message:      cause.Error(),
	}
	if err := s.runs.RecordOrgSyncAttempt(ctx, attempt); err != nil {
		logger.Get().WithError(err).Warn("failed to record failed organization sync attempt")
	}
}

// recordBlockedAttempt best-effort persists a blocked outcome, durably
// setting hold_active so threshold editing freezes on every replica -- see
// LockState.
func (s *OrganizationSyncService) recordBlockedAttempt(ctx context.Context, run syncRun, threshold float64, report orgprovider.MassDeletionReport) {
	usersExisting, usersDeleting, teamsExisting, teamsDeleting := report.Users.Existing, report.Users.Deleting, report.Teams.Existing, report.Teams.Deleting
	usersPercent, teamsPercent := report.Users.Percent, report.Teams.Percent
	run.log().WithFields(map[string]interface{}{
		"status":            orgprovider.StatusBlocked,
		"writes_status":     orgprovider.WritesNone,
		"threshold_percent": threshold,
		"users_deleting":    usersDeleting,
		"users_existing":    usersExisting,
		"teams_deleting":    teamsDeleting,
		"teams_existing":    teamsExisting,
	}).Warn("organization sync blocked: mass-deletion threshold exceeded")
	if s.runs == nil {
		return
	}
	attempt := orgprovider.OrgSyncAttempt{
		RunID:            run.id,
		Trigger:          run.trigger,
		Status:           orgprovider.StatusBlocked,
		StartedAt:        run.startedAt,
		FinishedAt:       time.Now().UTC(),
		InstanceID:       s.instanceID,
		ThresholdPercent: &threshold,
		UsersExisting:    &usersExisting,
		UsersDeleting:    &usersDeleting,
		UsersPercent:     &usersPercent,
		TeamsExisting:    &teamsExisting,
		TeamsDeleting:    &teamsDeleting,
		TeamsPercent:     &teamsPercent,
		WritesStatus:     orgprovider.WritesNone,
		Message: fmt.Sprintf(
			"Blocked - deletion threshold exceeded. User deletions: %d (%.1f%%). Team deletions: %d (%.1f%%). Configured threshold: %.1f%%. No changes were written.",
			usersDeleting, usersPercent, teamsDeleting, teamsPercent, threshold,
		),
	}
	if err := s.runs.RecordOrgSyncAttempt(ctx, attempt); err != nil {
		logger.Get().WithError(err).Warn("failed to record blocked organization sync attempt")
	}
}

// formatCascade renders the optional membership cascade for the audit line.
func formatCascade(m *orgprovider.DeletionMetric) string {
	if m == nil {
		return "unmeasured"
	}
	return fmt.Sprintf("%d/%d (%.1f%%)", m.Deleting, m.Existing, m.Percent)
}

// LockState reports whether the mass-deletion threshold is currently frozen,
// at what value, and what is currently running -- for the threshold-edit
// endpoints and the sync-conflict message.
//
// Fail-closed on EITHER signal: locked if the local in-memory hold is set, OR
// if the durable hold_active flag is set and unexpired -- not durable-only,
// so a same-replica hold that has just tripped, but whose durable write
// hasn't landed yet, still freezes editing on this replica without waiting on
// a round trip.
//
// An error here MUST be treated as "locked" by the caller, never as
// "editable" -- a durable-hold read error must never produce a false
// "editable" response. When no SyncRunRecorder is configured, this returns exactly
// today's in-memory-only behaviour and never errors, preserving existing
// no-database handler tests.
func (s *OrganizationSyncService) LockState(ctx context.Context) (SyncLockState, error) {
	state := SyncLockState{Syncing: s.running.Load()}
	if v, ok := s.runningTrigger.Load().(string); ok {
		state.Trigger = v
	}

	s.mu.Lock()
	if s.held != nil {
		if time.Since(s.held.heldAt) > MassDeletionHoldTTL {
			s.held = nil
		} else {
			state.Held = true
			state.Threshold = s.held.threshold
			state.HeldAt = s.held.heldAt
		}
	}
	s.mu.Unlock()

	if s.runs == nil {
		return state, nil
	}

	lastRun, err := s.runs.GetOrgSyncLastRun(ctx)
	if err != nil {
		return SyncLockState{}, fmt.Errorf("failed to read organization sync hold state: %w", err)
	}
	if lastRun.HoldActive && lastRun.HoldRecordedAt != nil && time.Since(*lastRun.HoldRecordedAt) <= MassDeletionHoldTTL {
		state.Held = true
		switch {
		case lastRun.HoldThresholdPercent != nil:
			state.Threshold = *lastRun.HoldThresholdPercent
		case lastRun.LastAttempt != nil && lastRun.LastAttempt.ThresholdPercent != nil:
			state.Threshold = *lastRun.LastAttempt.ThresholdPercent
		}
		state.HeldAt = *lastRun.HoldRecordedAt
	}

	return state, nil
}

// DismissHold clears an unresolved IN-MEMORY hold, which re-enables threshold
// editing on THIS replica. It is what a completed sync does implicitly (the
// durable side is cleared separately, inside ApplySnapshot's own transaction
// on success). It reports whether there was anything to clear.
//
// For the admin-facing "dismiss without syncing" action, use ClearHold, which
// clears both the in-memory and the durable hold -- DismissHold alone would
// leave the durable flag (and therefore every OTHER replica's threshold
// lock) untouched.
func (s *OrganizationSyncService) DismissHold() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	existed := s.held != nil
	s.held = nil
	return existed
}

// ClearHold resolves an unresolved hold WITHOUT running a sync -- the
// "dismiss without syncing" admin action. It clears the in-memory hold and,
// when a SyncRunRecorder is configured, the durable hold_active flag
// unconditionally, so the escape hatch works even on a replica whose
// in-memory hold was never set (because another replica ran the held sync).
func (s *OrganizationSyncService) ClearHold(ctx context.Context) error {
	s.DismissHold()
	if s.runs == nil {
		return nil
	}
	if err := s.runs.ClearOrgSyncHold(ctx); err != nil {
		return fmt.Errorf("failed to clear organization sync hold: %w", err)
	}
	return nil
}

// GetLastRun reads the persisted last-attempt/last-skip state for API/UI
// display. Returns an empty (never-run) result, not an error, when no
// SyncRunRecorder is configured.
func (s *OrganizationSyncService) GetLastRun(ctx context.Context) (*orgprovider.OrgSyncLastRun, error) {
	if s.runs == nil {
		return &orgprovider.OrgSyncLastRun{}, nil
	}
	return s.runs.GetOrgSyncLastRun(ctx)
}

// recordHold remembers a tripped guard, together with the threshold it was
// judged against.
func (s *OrganizationSyncService) recordHold(threshold float64, report orgprovider.MassDeletionReport) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.held = &heldSync{threshold: threshold, report: report, heldAt: time.Now().UTC()}
}

// thresholdForRun returns the threshold this attempt is judged against: the
// frozen value while a hold is unresolved (in-memory or durable), otherwise
// the configured one.
func (s *OrganizationSyncService) thresholdForRun(ctx context.Context) (float64, error) {
	state, err := s.LockState(ctx)
	if err != nil {
		return 0, err
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
