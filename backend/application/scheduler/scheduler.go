package scheduler

import (
	"context"
	"errors"
	"time"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/organization"
	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
	"github.com/google/uuid"
)

// PollInterval is how often the scheduler checks whether the schedule is
// due. Because next_run_at is a persisted instant, the tick is only a POLL
// for due-ness, not the schedule itself -- so it can be coarse. One minute
// bounds 02:00 drift to under a minute, costs one LoadSchedule call per
// minute per replica, and is orders of magnitude below the shortest interval
// (a day), so it cannot interact badly with the catch-up bound or the
// strictly-after guard in NextOccurrence.
const PollInterval = time.Minute

// SyncRunMaxDuration bounds a scheduled sync independent of shutdown. This is
// a safety requirement, not a nicety: without it, a hung provider read holds
// the advisory lock indefinitely and every sync on every replica is refused
// -- the same silent-lockout failure mode as the lock's own connection-
// affinity trap (see organization_sync_locker.go), reached by a different
// route.
const SyncRunMaxDuration = 15 * time.Minute

// Syncer is the narrow slice of OrganizationSyncService the scheduler needs.
// Depending on an interface rather than the concrete type keeps the
// scheduler's own tests free of a fetcher, a repository, or a snapshot --
// mirroring how WithDeleteThresholdStore already narrows the sync service's
// own dependency on orgRepo.
//
// Deliberately exposes SchedulerBlocked, never LockState: LockState merges in
// the durable hold_active flag (see services.OrganizationSyncService's doc),
// and if the scheduler could reach that, a restart-surviving durable hold
// would block scheduling -- contradicting the decision that a restart
// permits a harmless re-attempt. This interface has no method that could
// return the durable flag at all, so that is a structural guarantee, not a
// convention someone can bypass.
type Syncer interface {
	SyncWithOptions(ctx context.Context, opts services.SyncOptions) (*services.SyncResult, error)
	SchedulerBlocked() (blocked bool, reason string)
	RecordSkip(ctx context.Context, runID, reason string) error
	Configured() bool
}

// Store is the narrow persistence surface the scheduler needs: reading the
// schedule for admin display is organization.Repository's job, not this
// interface's -- Store exists only for what the scheduler itself does every
// tick.
type Store interface {
	GetOrgSyncSchedule(ctx context.Context) (*organization.OrgSyncSchedule, error)
	ClaimOrgSyncOccurrence(ctx context.Context, observedNextRunAt, newNextRunAt time.Time) (bool, error)
}

// Clock abstracts "now" so tests can advance time deterministically instead
// of sleeping in real time.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

// TickSource abstracts the periodic poll trigger. Tests drive the channel
// directly to fire ticks deterministically -- the same channel-gating
// convention this codebase already uses for concurrency tests (see
// gatedFetcher in settings_deletion_threshold_test.go) rather than a
// speed-up-time fake clock, since the scheduler polls on a channel, not a
// sleep.
type TickSource interface {
	C() <-chan time.Time
	Stop()
}

type realTicker struct{ t *time.Ticker }

func (r realTicker) C() <-chan time.Time { return r.t.C }
func (r realTicker) Stop()               { r.t.Stop() }

func newRealTicker(d time.Duration) TickSource { return realTicker{time.NewTicker(d)} }

// Option customizes a Scheduler at construction.
type Option func(*Scheduler)

// WithClock overrides the clock used to determine "now". Tests use this for
// a deterministic value; production takes the real clock.
func WithClock(c Clock) Option { return func(s *Scheduler) { s.clock = c } }

// WithTicker overrides how the scheduler's poll ticker is constructed. Tests
// pass a fake TickSource so ticks fire on command instead of on a timer.
func WithTicker(newTicker func(time.Duration) TickSource) Option {
	return func(s *Scheduler) { s.newTicker = newTicker }
}

// WithPollInterval overrides PollInterval. Tests use a very short interval
// alongside a fake ticker; production takes the default.
func WithPollInterval(d time.Duration) Option {
	return func(s *Scheduler) { s.pollInterval = d }
}

// Scheduler polls the persisted organization-sync schedule and triggers a
// scheduled run through the same path a manual sync uses, when due. It owns
// a goroutine and must be Stopped during shutdown.
type Scheduler struct {
	syncer Syncer
	store  Store
	loc    *time.Location

	clock        Clock
	newTicker    func(time.Duration) TickSource
	pollInterval time.Duration

	stop chan struct{}
	done chan struct{}
}

// New creates a Scheduler. loc is the IANA location every occurrence is
// computed in (see ORG_SYNC_SCHEDULE_TZ in main.go).
func New(syncer Syncer, store Store, loc *time.Location, opts ...Option) *Scheduler {
	s := &Scheduler{
		syncer:       syncer,
		store:        store,
		loc:          loc,
		clock:        realClock{},
		newTicker:    newRealTicker,
		pollInterval: PollInterval,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Start begins polling in a background goroutine. It checks once immediately
// (the bounded startup catch-up: a schedule that came due while the process
// was down is evaluated right away, not after waiting up to pollInterval)
// and then on every tick until Stop is called or ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	s.stop = make(chan struct{})
	s.done = make(chan struct{})
	ticker := s.newTicker(s.pollInterval)

	go func() {
		defer close(s.done)
		defer ticker.Stop()

		s.tick(ctx)

		for {
			select {
			case <-ctx.Done():
				return
			case <-s.stop:
				return
			case <-ticker.C():
				s.tick(ctx)
			}
		}
	}()
}

// Stop signals the polling goroutine to exit and waits up to grace for it to
// do so. Never blocks longer than grace, so a slow or wedged tick cannot hang
// shutdown indefinitely.
func (s *Scheduler) Stop(grace time.Duration) {
	if s.stop == nil {
		return
	}
	close(s.stop)
	select {
	case <-s.done:
	case <-time.After(grace):
	}
}

// tick is one poll: load the schedule, decide due-ness, claim the occurrence
// if due, and either skip or run it. Every error is logged and swallowed --
// a scheduler tick failing must never crash the process, and the next tick
// tries again.
func (s *Scheduler) tick(ctx context.Context) {
	log := logger.Get()

	schedule, err := s.store.GetOrgSyncSchedule(ctx)
	if err != nil {
		log.WithError(err).Warn("failed to load organization sync schedule")
		return
	}
	if !schedule.Enabled || schedule.NextRunAt == nil {
		return
	}

	now := s.clock.Now()
	if schedule.NextRunAt.After(now) {
		return // not due yet
	}

	// Bounded catch-up: overdue < Interval() runs; overdue >= Interval()
	// skips forward. At exactly one interval overdue, the FOLLOWING
	// occurrence is already due or imminent, so running the stale one buys
	// nothing and doubles the destructive work.
	overdue := now.Sub(*schedule.NextRunAt)
	catchUpExceeded := overdue >= Interval(schedule.Frequency)

	// The claim is the ONLY operation that ever advances an already-due
	// occurrence, and it doubles as the concurrency control for the
	// skip-recording race: if every replica's scheduler observes the same
	// due occurrence while none of them can even attempt the sync (an
	// unrelated manual sync holds the lock elsewhere), only the replica
	// whose claim actually matches a row is responsible for anything
	// further. Losing the claim -- another replica already claimed it, or
	// an admin disabled the schedule in the interim -- means doing nothing.
	newNextRunAt := NextOccurrence(schedule.Frequency, now, s.loc)
	won, err := s.store.ClaimOrgSyncOccurrence(ctx, *schedule.NextRunAt, newNextRunAt)
	if err != nil {
		log.WithError(err).Warn("failed to claim organization sync occurrence")
		return
	}
	if !won {
		return
	}

	// One ID per claimed occurrence: it names the sync if one runs, or the
	// skip record if the occurrence is skipped, and is stamped on every log
	// line below so the whole outcome can be found by searching for it.
	runID := uuid.NewString()

	if catchUpExceeded {
		if err := s.syncer.RecordSkip(ctx, runID, orgprovider.SkipOverdueCatchUp); err != nil {
			log.WithError(err).Warn("failed to record organization sync catch-up skip")
			return
		}
		log.WithFields(map[string]any{
			"sync_run_id": runID,
			"trigger":     orgprovider.TriggerScheduled,
			"reason":      orgprovider.SkipOverdueCatchUp,
			"at":          now,
		}).Info("scheduled organization sync skipped")
		return
	}

	// Cheap, in-memory, same-replica check first: SchedulerBlocked reads only
	// local state and touches neither the lock nor the provider, so a
	// collision this replica already knows about costs one predicate, not a
	// wasted sync attempt.
	if blocked, reason := s.syncer.SchedulerBlocked(); blocked {
		if err := s.syncer.RecordSkip(ctx, runID, reason); err != nil {
			log.WithError(err).Warn("failed to record organization sync skip")
			return
		}
		log.WithFields(map[string]any{
			"sync_run_id": runID,
			"trigger":     orgprovider.TriggerScheduled,
			"reason":      reason,
			"at":          now,
		}).Info("scheduled organization sync skipped")
		return
	}

	// Detached from shutdown, and independently bounded: a run in progress
	// when SIGTERM arrives must not be aborted by our own cancel signal for
	// no benefit (context.WithoutCancel) -- but it must not be able to hold
	// the advisory lock forever either (SyncRunMaxDuration). Detaching is
	// strictly better, not merely nicer: ApplySnapshot is a single
	// transaction, so a process ending mid-run leaves no partial writes
	// regardless -- the worst case (rollback) is unchanged, and the best
	// case (a nearly-finished run gets to commit) is only possible if the
	// run's own context outlives the root one.
	runCtx, cancelRun := context.WithTimeout(context.WithoutCancel(ctx), SyncRunMaxDuration)
	defer cancelRun()

	result, syncErr := s.syncer.SyncWithOptions(runCtx, services.SyncOptions{Trigger: orgprovider.TriggerScheduled, RunID: runID})
	if syncErr == nil {
		log.WithFields(map[string]any{"sync_run_id": runID, "usersDeleted": result.UsersDeleted, "teamsDeleted": result.TeamsDeleted}).
			Info("scheduled organization sync completed")
		return
	}

	// A *SyncInProgressError here means the local pre-check above passed but
	// a genuine cross-replica collision (or a last-instant local race) still
	// occurred at the actual lock. This is a skip, not a technical failure --
	// SyncWithOptions never records it as a failed attempt (see its own
	// admission-check ordering), so the scheduler is the only place this
	// outcome is ever durably recorded.
	var inProgress *services.SyncInProgressError
	if errors.As(syncErr, &inProgress) {
		reason := orgprovider.SkipManualRunning
		if inProgress.Trigger == orgprovider.TriggerScheduled {
			reason = orgprovider.SkipScheduledRunning
		}
		// An unknown trigger (an unreadable or absent remote claim) has no
		// dedicated skip reason in the schema -- deliberately not adding a
		// fifth CHECK-constraint value for the rare intersection of two
		// already-rare events (a genuine cross-replica collision AND an
		// unreadable claim at the same instant). Recorded as the more common
		// manual-collision bucket instead.
		if err := s.syncer.RecordSkip(ctx, runID, reason); err != nil {
			log.WithError(err).Warn("failed to record organization sync skip")
			return
		}
		log.WithFields(map[string]any{
			"sync_run_id":       runID,
			"trigger":           orgprovider.TriggerScheduled,
			"reason":            reason,
			"at":                now,
			"activeRunTrigger":  inProgress.Trigger,
			"activeRunInstance": inProgress.Instance,
		}).Info("scheduled organization sync skipped")
		return
	}

	// Any other error (blocked, technical failure) was already recorded by
	// SyncWithOptions itself -- see recordBlockedAttempt/failWith. Nothing
	// further to do here; log for operational visibility only.
	log.WithError(syncErr).WithField("sync_run_id", runID).Info("scheduled organization sync did not complete")
}
