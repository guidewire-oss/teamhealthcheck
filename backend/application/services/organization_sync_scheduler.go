package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agopalakrishnan/teams360/backend/domain/orgsyncrun"
	"github.com/agopalakrishnan/teams360/backend/domain/orgsyncschedule"
	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
)

// SchedulerPollInterval is how often each replica checks for a due occurrence.
const SchedulerPollInterval = time.Minute

// scheduledActor labels runs started by the scheduler.
const scheduledActor = "scheduler"

// ErrInvalidSchedule is returned for a rejected schedule update.
var ErrInvalidSchedule = errors.New("invalid sync schedule")

// RunStarter is the part of OrganizationSyncService the scheduler needs. The
// scheduler starts runs through the same StartRun entry point as the manual
// endpoint, so validation, mapping, the transaction, the mass-deletion guard,
// the single-active-run index and the advisory lock all apply unchanged.
type RunStarter interface {
	Configured() bool
	StartRun(ctx context.Context, opts SyncOptions, idempotencyKey string) (*orgsyncrun.Run, error)
}

// AutoSyncAttempt summarises one automatic run for display.
type AutoSyncAttempt struct {
	RunID        string
	Status       orgsyncrun.Status
	StartedAt    time.Time // when the attempt was recorded
	CompletedAt  *time.Time
	ErrorMessage *string // already safe to show an administrator
}

// AutoSyncStatus is the schedule plus automatic-run history.
type AutoSyncStatus struct {
	Enabled       bool
	Frequency     orgsyncschedule.Frequency
	Description   string
	Timezone      string
	NextRunAt     *time.Time
	LastAttempt   *AutoSyncAttempt
	LastSuccessAt *time.Time
	UpdatedAt     time.Time
}

// OrganizationSyncScheduler owns the automatic-sync schedule and fires due
// occurrences. State lives in the database, so every replica can run it: an
// occurrence is claimed with a compare-and-swap and fires exactly once.
type OrganizationSyncScheduler struct {
	store   orgsyncschedule.Store
	history orgsyncschedule.RunHistory
	starter RunStarter
	now     func() time.Time
}

// NewOrganizationSyncScheduler wires the scheduler.
func NewOrganizationSyncScheduler(store orgsyncschedule.Store, history orgsyncschedule.RunHistory, starter RunStarter) *OrganizationSyncScheduler {
	return &OrganizationSyncScheduler{store: store, history: history, starter: starter, now: time.Now}
}

// WithClock replaces the clock (tests).
func (s *OrganizationSyncScheduler) WithClock(now func() time.Time) *OrganizationSyncScheduler {
	s.now = now
	return s
}

// ParseFrequency normalises and validates a client-supplied frequency.
func ParseFrequency(raw string) (orgsyncschedule.Frequency, error) {
	f := orgsyncschedule.Frequency(strings.ToUpper(strings.TrimSpace(raw)))
	if !f.Valid() {
		return "", fmt.Errorf("%w: %v", ErrInvalidSchedule, orgsyncschedule.ErrInvalidFrequency)
	}
	return f, nil
}

// Status returns the saved schedule and automatic-run history.
func (s *OrganizationSyncScheduler) Status(ctx context.Context) (*AutoSyncStatus, error) {
	sched, err := s.store.Get(ctx)
	if err != nil {
		return nil, err
	}
	st := &AutoSyncStatus{
		Enabled: sched.Enabled, Frequency: sched.Frequency, Description: sched.Frequency.Description(),
		Timezone: orgsyncschedule.Timezone, UpdatedAt: sched.UpdatedAt,
	}
	if sched.Enabled {
		st.NextRunAt = sched.NextRunAt
	}
	last, err := s.history.LatestByTrigger(ctx, orgsyncrun.TriggerScheduled)
	if err != nil {
		return nil, err
	}
	if last != nil {
		st.LastAttempt = &AutoSyncAttempt{
			RunID: last.ID, Status: last.Status, StartedAt: last.CreatedAt.UTC(),
			CompletedAt: last.CompletedAt, ErrorMessage: last.ErrorMessage,
		}
	}
	ok, err := s.history.LatestSuccessByTrigger(ctx, orgsyncrun.TriggerScheduled)
	if err != nil {
		return nil, err
	}
	if ok != nil {
		st.LastSuccessAt = ok.CompletedAt
	}
	return st, nil
}

// Update saves the schedule. Enabling or changing frequency computes the next
// future occurrence and never triggers an immediate sync; disabling clears it.
// When frequency is empty the saved one is kept.
func (s *OrganizationSyncScheduler) Update(ctx context.Context, enabled bool, rawFrequency, actor string) (*AutoSyncStatus, error) {
	var freq orgsyncschedule.Frequency
	if strings.TrimSpace(rawFrequency) == "" {
		cur, err := s.store.Get(ctx)
		if err != nil {
			return nil, err
		}
		freq = cur.Frequency
	} else {
		var err error
		if freq, err = ParseFrequency(rawFrequency); err != nil {
			return nil, err
		}
	}
	var next *time.Time
	if enabled {
		n, err := orgsyncschedule.NextAfter(freq, s.now())
		if err != nil {
			return nil, err
		}
		next = &n
	}
	if _, err := s.store.Save(ctx, enabled, freq, next, actor); err != nil {
		return nil, err
	}
	return s.Status(ctx)
}

// RunDue fires the occurrence that is due, if any. It is safe to call from many
// replicas and many times: persisted enablement and the due time are rechecked
// atomically when the occurrence is claimed.
func (s *OrganizationSyncScheduler) RunDue(ctx context.Context) {
	log := logger.Get()
	sched, err := s.store.Get(ctx)
	if err != nil {
		log.WithError(err).Warn("auto sync: could not load schedule")
		return
	}
	if !sched.Enabled {
		return
	}
	now := s.now().UTC()
	if sched.NextRunAt == nil {
		// Enabled without a next occurrence (e.g. edited by hand): repair it.
		if next, err := orgsyncschedule.NextAfter(sched.Frequency, now); err == nil {
			if _, err := s.store.Save(ctx, true, sched.Frequency, &next, scheduledActor); err != nil {
				log.WithError(err).Warn("auto sync: could not repair next run time")
			}
		}
		return
	}
	due := sched.NextRunAt.UTC()
	if due.After(now) {
		return
	}
	// Without a configured provider nothing can run; leave the occurrence in
	// place rather than burning it.
	if !s.starter.Configured() {
		return
	}
	next, err := orgsyncschedule.NextAfter(sched.Frequency, now)
	if err != nil {
		log.WithError(err).Warn("auto sync: bad frequency")
		return
	}
	claimed, err := s.store.Claim(ctx, due, next)
	if err != nil {
		log.WithError(err).Warn("auto sync: claim failed")
		return
	}
	if !claimed {
		return // another replica won, or the schedule changed or was disabled
	}
	if now.Sub(due) > orgsyncschedule.MissedGrace {
		// Missed while the service was down: skip, never replay a backlog.
		log.WithFields(map[string]interface{}{"occurrence": due.Format(time.RFC3339), "next": next.Format(time.RFC3339)}).
			Warn("auto sync: missed occurrence skipped")
		return
	}
	key := "scheduled:" + due.Format(time.RFC3339)
	_, err = s.starter.StartRun(ctx, SyncOptions{Trigger: orgsyncrun.TriggerScheduled, ActorUserID: scheduledActor}, key)
	switch {
	case err == nil:
		log.WithFields(map[string]interface{}{"occurrence": due.Format(time.RFC3339)}).Info("auto sync: run started")
	case errors.Is(err, orgsyncrun.ErrActiveRunExists):
		log.WithFields(map[string]interface{}{"occurrence": due.Format(time.RFC3339)}).
			Warn("auto sync: another sync is active; occurrence skipped")
	default:
		log.WithError(err).Warn("auto sync: could not start run; will retry at the next occurrence")
	}
}

// Start polls until ctx is cancelled. The first check happens immediately, so
// a restart inside the grace window still fires a just-missed occurrence and
// anything older is skipped to the next future one.
func (s *OrganizationSyncScheduler) Start(ctx context.Context) {
	s.RunDue(ctx)
	t := time.NewTicker(SchedulerPollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.RunDue(ctx)
		}
	}
}
