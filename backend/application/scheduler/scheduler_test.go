package scheduler_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agopalakrishnan/teams360/backend/application/scheduler"
	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/organization"
	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
	"github.com/google/uuid"
)

// fakeClock is a controllable Clock.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// fakeTicker is a controllable TickSource: tests send to C to fire a tick,
// following this codebase's established channel-gating convention for
// deterministic concurrency tests (see gatedFetcher in
// settings_deletion_threshold_test.go) rather than a real timer.
type fakeTicker struct{ ch chan time.Time }

func newFakeTicker() *fakeTicker          { return &fakeTicker{ch: make(chan time.Time, 1)} }
func (f *fakeTicker) C() <-chan time.Time { return f.ch }
func (f *fakeTicker) Stop()               {}
func (f *fakeTicker) fire(t time.Time)    { f.ch <- t }

// fakeStore is a hand-rolled fake of scheduler.Store.
type fakeStore struct {
	mu sync.Mutex

	schedule *organization.OrgSyncSchedule
	loadErr  error

	claimWins  bool
	claimErr   error
	claimCalls []struct{ observed, next time.Time }
}

func (f *fakeStore) GetOrgSyncSchedule(_ context.Context) (*organization.OrgSyncSchedule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	return f.schedule, nil
}

func (f *fakeStore) ClaimOrgSyncOccurrence(_ context.Context, observed, next time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.claimCalls = append(f.claimCalls, struct{ observed, next time.Time }{observed, next})
	if f.claimErr != nil {
		return false, f.claimErr
	}
	return f.claimWins, nil
}

func (f *fakeStore) claimCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.claimCalls)
}

func (f *fakeStore) setSchedule(s *organization.OrgSyncSchedule) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.schedule = s
}

// fakeSyncer is a hand-rolled fake of scheduler.Syncer.
type fakeSyncer struct {
	mu sync.Mutex

	blocked       bool
	blockedReason string

	syncErr    error
	syncCalled int

	skips         []string
	skipRunIDs    []string
	syncRunIDs    []string
	recordSkipErr error
}

func (f *fakeSyncer) SyncWithOptions(_ context.Context, opts services.SyncOptions) (*services.SyncResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.syncRunIDs = append(f.syncRunIDs, opts.RunID)
	f.syncCalled++
	if f.syncErr != nil {
		return nil, f.syncErr
	}
	return &services.SyncResult{Status: "completed"}, nil
}

func (f *fakeSyncer) SchedulerBlocked() (bool, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.blocked, f.blockedReason
}

func (f *fakeSyncer) RecordSkip(_ context.Context, runID, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.recordSkipErr != nil {
		return f.recordSkipErr
	}
	f.skips = append(f.skips, reason)
	f.skipRunIDs = append(f.skipRunIDs, runID)
	return nil
}

func (f *fakeSyncer) Configured() bool { return true }

func (f *fakeSyncer) syncCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.syncCalled
}

func (f *fakeSyncer) lastSkip() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.skips) == 0 {
		return ""
	}
	return f.skips[len(f.skips)-1]
}

func (f *fakeSyncer) skipCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.skips)
}

func timePtr(t time.Time) *time.Time { return &t }

// startTestScheduler wires a Scheduler with a fake clock/ticker for
// deterministic control, starts it, and returns a function to fire one tick
// and wait briefly for it to be processed (there is no synchronous
// "tick complete" signal, so tests poll a fake's observable side effect
// after firing -- see each test's own Eventually-style wait).
func startTestScheduler(t *testing.T, syncer scheduler.Syncer, store scheduler.Store, clock *fakeClock) (*scheduler.Scheduler, *fakeTicker) {
	t.Helper()
	ticker := newFakeTicker()
	s := scheduler.New(syncer, store, time.UTC,
		scheduler.WithClock(clock),
		scheduler.WithTicker(func(time.Duration) scheduler.TickSource { return ticker }),
	)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		s.Stop(time.Second)
	})
	s.Start(ctx)
	return s, ticker
}

// waitForCondition polls fn until it returns true or the timeout elapses,
// failing the test on timeout. Used because tick() runs asynchronously on
// the scheduler's own goroutine.
func waitForCondition(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}

func TestSchedulerDoesNothingWhenDisabled(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 3, 10, 3, 0, 0, 0, time.UTC)}
	store := &fakeStore{schedule: &organization.OrgSyncSchedule{Enabled: false}}
	syncer := &fakeSyncer{}
	_, _ = startTestScheduler(t, syncer, store, clock)

	// The immediate startup check already ran; give it a moment, then assert
	// nothing happened.
	time.Sleep(20 * time.Millisecond)
	if syncer.syncCallCount() != 0 {
		t.Fatal("a disabled schedule must never trigger a sync")
	}
	if store.claimCallCount() != 0 {
		t.Fatal("a disabled schedule must never attempt a claim")
	}
}

func TestSchedulerDoesNothingWhenNotYetDue(t *testing.T) {
	now := time.Date(2026, 3, 10, 3, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	clock := &fakeClock{now: now}
	store := &fakeStore{schedule: &organization.OrgSyncSchedule{Enabled: true, Frequency: organization.OrgSyncFrequencyDaily, NextRunAt: timePtr(future)}}
	syncer := &fakeSyncer{}
	_, _ = startTestScheduler(t, syncer, store, clock)

	time.Sleep(20 * time.Millisecond)
	if store.claimCallCount() != 0 {
		t.Fatal("must not attempt a claim before the occurrence is due")
	}
}

func TestSchedulerRunsWhenDueAndClaimWins(t *testing.T) {
	now := time.Date(2026, 3, 10, 3, 0, 0, 0, time.UTC)
	due := now.Add(-time.Minute) // due, barely -- well within the daily interval
	clock := &fakeClock{now: now}
	store := &fakeStore{
		schedule:  &organization.OrgSyncSchedule{Enabled: true, Frequency: organization.OrgSyncFrequencyDaily, NextRunAt: timePtr(due)},
		claimWins: true,
	}
	syncer := &fakeSyncer{}
	_, _ = startTestScheduler(t, syncer, store, clock)

	waitForCondition(t, time.Second, func() bool { return syncer.syncCallCount() == 1 })
	if store.claimCallCount() != 1 {
		t.Fatalf("expected exactly one claim attempt, got %d", store.claimCallCount())
	}
}

func TestSchedulerDoesNothingWhenClaimIsLost(t *testing.T) {
	now := time.Date(2026, 3, 10, 3, 0, 0, 0, time.UTC)
	due := now.Add(-time.Minute)
	clock := &fakeClock{now: now}
	store := &fakeStore{
		schedule:  &organization.OrgSyncSchedule{Enabled: true, Frequency: organization.OrgSyncFrequencyDaily, NextRunAt: timePtr(due)},
		claimWins: false, // another replica claimed it
	}
	syncer := &fakeSyncer{}
	_, _ = startTestScheduler(t, syncer, store, clock)

	waitForCondition(t, time.Second, func() bool { return store.claimCallCount() >= 1 })
	time.Sleep(20 * time.Millisecond)
	if syncer.syncCallCount() != 0 {
		t.Fatal("losing the claim must never attempt a sync")
	}
	if syncer.skipCount() != 0 {
		t.Fatal("losing the claim is not this replica's occurrence to record a skip for")
	}
}

func TestSchedulerSkipsWithOverdueCatchUpReasonPastTheBound(t *testing.T) {
	now := time.Date(2026, 3, 10, 3, 0, 0, 0, time.UTC)
	// Daily interval is 24h; overdue by exactly 24h meets ">= Interval()".
	overdueBy24h := now.Add(-24 * time.Hour)
	clock := &fakeClock{now: now}
	store := &fakeStore{
		schedule:  &organization.OrgSyncSchedule{Enabled: true, Frequency: organization.OrgSyncFrequencyDaily, NextRunAt: timePtr(overdueBy24h)},
		claimWins: true,
	}
	syncer := &fakeSyncer{}
	_, _ = startTestScheduler(t, syncer, store, clock)

	waitForCondition(t, time.Second, func() bool { return syncer.skipCount() == 1 })
	if got := syncer.lastSkip(); got != orgprovider.SkipOverdueCatchUp {
		t.Fatalf("expected skip reason %q, got %q", orgprovider.SkipOverdueCatchUp, got)
	}
	if syncer.syncCallCount() != 0 {
		t.Fatal("a catch-up-exceeded occurrence must not run")
	}
}

func TestSchedulerRunsJustUnderTheCatchUpBound(t *testing.T) {
	now := time.Date(2026, 3, 10, 3, 0, 0, 0, time.UTC)
	// Overdue by slightly less than one daily interval must still run.
	justUnder := now.Add(-24*time.Hour + time.Minute)
	clock := &fakeClock{now: now}
	store := &fakeStore{
		schedule:  &organization.OrgSyncSchedule{Enabled: true, Frequency: organization.OrgSyncFrequencyDaily, NextRunAt: timePtr(justUnder)},
		claimWins: true,
	}
	syncer := &fakeSyncer{}
	_, _ = startTestScheduler(t, syncer, store, clock)

	waitForCondition(t, time.Second, func() bool { return syncer.syncCallCount() == 1 })
	if syncer.skipCount() != 0 {
		t.Fatal("an occurrence within the catch-up bound must run, not skip")
	}
}

func TestSchedulerSkipsWhenLocallyBlockedWithoutAttemptingSync(t *testing.T) {
	now := time.Date(2026, 3, 10, 3, 0, 0, 0, time.UTC)
	due := now.Add(-time.Minute)
	clock := &fakeClock{now: now}
	store := &fakeStore{
		schedule:  &organization.OrgSyncSchedule{Enabled: true, Frequency: organization.OrgSyncFrequencyDaily, NextRunAt: timePtr(due)},
		claimWins: true,
	}
	syncer := &fakeSyncer{blocked: true, blockedReason: orgprovider.SkipHoldUnresolved}
	_, _ = startTestScheduler(t, syncer, store, clock)

	waitForCondition(t, time.Second, func() bool { return syncer.skipCount() == 1 })
	if got := syncer.lastSkip(); got != orgprovider.SkipHoldUnresolved {
		t.Fatalf("expected skip reason %q, got %q", orgprovider.SkipHoldUnresolved, got)
	}
	if syncer.syncCallCount() != 0 {
		t.Fatal("SchedulerBlocked must short-circuit before ever attempting SyncWithOptions")
	}
}

func TestSchedulerRecordsSkipWhenSyncInProgressErrorOccursDespiteLocalCheckPassing(t *testing.T) {
	now := time.Date(2026, 3, 10, 3, 0, 0, 0, time.UTC)
	due := now.Add(-time.Minute)
	clock := &fakeClock{now: now}
	store := &fakeStore{
		schedule:  &organization.OrgSyncSchedule{Enabled: true, Frequency: organization.OrgSyncFrequencyDaily, NextRunAt: timePtr(due)},
		claimWins: true,
	}
	syncer := &fakeSyncer{
		blocked: false, // local check passes
		syncErr: &services.SyncInProgressError{Remote: true, Trigger: orgprovider.TriggerManual},
	}
	_, _ = startTestScheduler(t, syncer, store, clock)

	waitForCondition(t, time.Second, func() bool { return syncer.skipCount() == 1 })
	if got := syncer.lastSkip(); got != orgprovider.SkipManualRunning {
		t.Fatalf("expected skip reason %q (from the error's Trigger), got %q", orgprovider.SkipManualRunning, got)
	}
}

func TestSchedulerDoesNotDoubleRecordAGenericSyncFailure(t *testing.T) {
	// A generic (non-SyncInProgress) error from SyncWithOptions is already
	// recorded internally by the sync service itself (blocked/failed) -- the
	// scheduler must not ALSO record a skip for it.
	now := time.Date(2026, 3, 10, 3, 0, 0, 0, time.UTC)
	due := now.Add(-time.Minute)
	clock := &fakeClock{now: now}
	store := &fakeStore{
		schedule:  &organization.OrgSyncSchedule{Enabled: true, Frequency: organization.OrgSyncFrequencyDaily, NextRunAt: timePtr(due)},
		claimWins: true,
	}
	syncer := &fakeSyncer{syncErr: errors.New("some other technical failure")}
	_, _ = startTestScheduler(t, syncer, store, clock)

	waitForCondition(t, time.Second, func() bool { return syncer.syncCallCount() == 1 })
	time.Sleep(20 * time.Millisecond)
	if syncer.skipCount() != 0 {
		t.Fatal("a generic sync failure must not be double-recorded as a skip")
	}
}

// TestSchedulerRunsOnAPeriodicTickNotJustStartup exercises the select loop's
// <-ticker.C() branch specifically, distinct from Start's own pre-loop
// startup check: the schedule is not yet due at startup, only becomes due
// once the clock advances, and a fired tick is what must notice it.
func TestSchedulerRunsOnAPeriodicTickNotJustStartup(t *testing.T) {
	start := time.Date(2026, 3, 10, 3, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: start}
	dueAtStart := start.Add(time.Minute) // not yet due when Start's own check runs
	store := &fakeStore{
		schedule:  &organization.OrgSyncSchedule{Enabled: true, Frequency: organization.OrgSyncFrequencyDaily, NextRunAt: timePtr(dueAtStart)},
		claimWins: true,
	}
	syncer := &fakeSyncer{}
	_, ticker := startTestScheduler(t, syncer, store, clock)

	time.Sleep(20 * time.Millisecond)
	if syncer.syncCallCount() != 0 {
		t.Fatal("must not have run yet -- the occurrence was not due at startup")
	}

	clock.mu.Lock()
	clock.now = dueAtStart.Add(time.Second)
	clock.mu.Unlock()
	ticker.fire(clock.Now())

	waitForCondition(t, time.Second, func() bool { return syncer.syncCallCount() == 1 })
}

func TestSchedulerNeverSetsOverrideMassDeletion(t *testing.T) {
	now := time.Date(2026, 3, 10, 3, 0, 0, 0, time.UTC)
	due := now.Add(-time.Minute)
	clock := &fakeClock{now: now}
	store := &fakeStore{
		schedule:  &organization.OrgSyncSchedule{Enabled: true, Frequency: organization.OrgSyncFrequencyDaily, NextRunAt: timePtr(due)},
		claimWins: true,
	}

	var capturedOpts services.SyncOptions
	var mu sync.Mutex
	syncer := &capturingSyncer{onSync: func(opts services.SyncOptions) {
		mu.Lock()
		defer mu.Unlock()
		capturedOpts = opts
	}}
	_, _ = startTestScheduler(t, syncer, store, clock)

	waitForCondition(t, time.Second, func() bool { return syncer.syncCallCount() == 1 })

	mu.Lock()
	defer mu.Unlock()
	if capturedOpts.OverrideMassDeletion {
		t.Fatal("a scheduled sync must never set OverrideMassDeletion")
	}
	if capturedOpts.ConfirmedMassDeletion != nil {
		t.Fatal("a scheduled sync must never set ConfirmedMassDeletion")
	}
	if capturedOpts.Trigger != orgprovider.TriggerScheduled {
		t.Fatalf("expected Trigger=%q, got %q", orgprovider.TriggerScheduled, capturedOpts.Trigger)
	}
}

// capturingSyncer records the SyncOptions it was called with.
type capturingSyncer struct {
	mu     sync.Mutex
	calls  int
	onSync func(services.SyncOptions)
}

func (c *capturingSyncer) SyncWithOptions(_ context.Context, opts services.SyncOptions) (*services.SyncResult, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	if c.onSync != nil {
		c.onSync(opts)
	}
	return &services.SyncResult{Status: "completed"}, nil
}
func (c *capturingSyncer) SchedulerBlocked() (bool, string)                { return false, "" }
func (c *capturingSyncer) RecordSkip(_ context.Context, _, _ string) error { return nil }
func (c *capturingSyncer) Configured() bool                                { return true }
func (c *capturingSyncer) syncCallCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// TestSchedulerLogsSkipOnlyAfterSuccessfulPersistence verifies the logging
// fix: a skip's Info log line must appear only once RecordSkip has actually
// succeeded -- never as a duplicate of the Warn already emitted on a
// RecordSkip failure, and never silently omitted on success.
func TestSchedulerLogsSkipOnlyAfterSuccessfulPersistence(t *testing.T) {
	var buf bytes.Buffer
	logger.Init(logger.Config{Level: "info", Output: &buf})
	t.Cleanup(func() { logger.Init(logger.Config{Level: "info"}) })

	now := time.Date(2026, 3, 10, 3, 0, 0, 0, time.UTC)
	due := now.Add(-time.Minute)
	clock := &fakeClock{now: now}
	store := &fakeStore{
		schedule:  &organization.OrgSyncSchedule{Enabled: true, Frequency: organization.OrgSyncFrequencyDaily, NextRunAt: timePtr(due)},
		claimWins: true,
	}
	syncer := &fakeSyncer{blocked: true, blockedReason: orgprovider.SkipManualRunning, recordSkipErr: errors.New("db unavailable")}
	_, ticker := startTestScheduler(t, syncer, store, clock)
	ticker.fire(now)

	waitForCondition(t, time.Second, func() bool { return store.claimCallCount() >= 1 })
	time.Sleep(20 * time.Millisecond)

	if strings.Contains(buf.String(), "scheduled organization sync skipped") {
		t.Fatalf("skip must not be logged when RecordSkip failed to persist it, got: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "failed to record organization sync skip") {
		t.Fatalf("expected the existing RecordSkip-failure warning to still be logged, got: %s", buf.String())
	}

	// Now let RecordSkip succeed on a fresh occurrence and confirm the skip IS logged.
	buf.Reset()
	syncer.mu.Lock()
	syncer.recordSkipErr = nil
	syncer.mu.Unlock()
	store.setSchedule(&organization.OrgSyncSchedule{Enabled: true, Frequency: organization.OrgSyncFrequencyDaily, NextRunAt: timePtr(due.Add(-time.Hour))})
	ticker.fire(now)

	waitForCondition(t, time.Second, func() bool { return syncer.skipCount() >= 1 })
	time.Sleep(20 * time.Millisecond)

	out := buf.String()
	if !strings.Contains(out, "scheduled organization sync skipped") {
		t.Fatalf("expected the skip to be logged once RecordSkip succeeded, got: %s", out)
	}
	if !strings.Contains(out, `"trigger":"scheduled"`) {
		t.Fatalf("expected trigger=scheduled in the skip log, got: %s", out)
	}
	if !strings.Contains(out, orgprovider.SkipManualRunning) {
		t.Fatalf("expected the skip reason in the skip log, got: %s", out)
	}
}

// TestSchedulerGivesTheSyncAValidRunID verifies a scheduled run is handed a
// UUID, so the sync's log lines, its database rows and the scheduler's own
// log lines all share one searchable ID.
func TestSchedulerGivesTheSyncAValidRunID(t *testing.T) {
	now := time.Date(2026, 3, 10, 3, 0, 0, 0, time.UTC)
	due := now.Add(-time.Minute)
	clock := &fakeClock{now: now}
	store := &fakeStore{
		schedule:  &organization.OrgSyncSchedule{Enabled: true, Frequency: organization.OrgSyncFrequencyDaily, NextRunAt: timePtr(due)},
		claimWins: true,
	}
	syncer := &fakeSyncer{}
	_, _ = startTestScheduler(t, syncer, store, clock)

	waitForCondition(t, time.Second, func() bool { return syncer.syncCallCount() == 1 })

	syncer.mu.Lock()
	defer syncer.mu.Unlock()
	if _, err := uuid.Parse(syncer.syncRunIDs[0]); err != nil {
		t.Fatalf("expected the scheduler to pass a valid UUID run ID to the sync, got %q (%v)", syncer.syncRunIDs[0], err)
	}
}

// TestSchedulerGivesASkippedOccurrenceItsOwnValidRunID verifies a skipped
// occurrence is also identifiable: the skip is recorded under a UUID.
func TestSchedulerGivesASkippedOccurrenceItsOwnValidRunID(t *testing.T) {
	now := time.Date(2026, 3, 10, 3, 0, 0, 0, time.UTC)
	due := now.Add(-time.Minute)
	clock := &fakeClock{now: now}
	store := &fakeStore{
		schedule:  &organization.OrgSyncSchedule{Enabled: true, Frequency: organization.OrgSyncFrequencyDaily, NextRunAt: timePtr(due)},
		claimWins: true,
	}
	syncer := &fakeSyncer{blocked: true, blockedReason: orgprovider.SkipManualRunning}
	_, _ = startTestScheduler(t, syncer, store, clock)

	waitForCondition(t, time.Second, func() bool { return syncer.skipCount() == 1 })

	syncer.mu.Lock()
	defer syncer.mu.Unlock()
	if _, err := uuid.Parse(syncer.skipRunIDs[0]); err != nil {
		t.Fatalf("expected the skip to be recorded under a valid UUID run ID, got %q (%v)", syncer.skipRunIDs[0], err)
	}
}
