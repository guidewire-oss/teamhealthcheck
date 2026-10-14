package services_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	"github.com/agopalakrishnan/teams360/backend/pkg/orgsnapshot"
	"github.com/google/uuid"
)

// fakeRunRecorder is a hand-rolled fake of services.SyncRunRecorder, recording
// every call it received for assertion, and letting a test script canned
// return values/errors for each read method.
type fakeRunRecorder struct {
	mu sync.Mutex

	attempts []orgprovider.OrgSyncAttempt
	skips    []orgprovider.OrgSyncSkip

	lastRun    *orgprovider.OrgSyncLastRun
	lastRunErr error

	history      []orgprovider.OrgSyncHistoryEntry
	historyLimit int

	clearHoldCalls int
}

func (f *fakeRunRecorder) RecordOrgSyncAttempt(_ context.Context, attempt orgprovider.OrgSyncAttempt) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attempts = append(f.attempts, attempt)
	return nil
}

func (f *fakeRunRecorder) RecordOrgSyncSkip(_ context.Context, skip orgprovider.OrgSyncSkip) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.skips = append(f.skips, skip)
	return nil
}

func (f *fakeRunRecorder) GetOrgSyncLastRun(_ context.Context) (*orgprovider.OrgSyncLastRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lastRunErr != nil {
		return nil, f.lastRunErr
	}
	if f.lastRun == nil {
		return &orgprovider.OrgSyncLastRun{}, nil
	}
	return f.lastRun, nil
}

func (f *fakeRunRecorder) ListOrgSyncHistory(_ context.Context, limit int) ([]orgprovider.OrgSyncHistoryEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.historyLimit = limit
	return f.history, nil
}

func (f *fakeRunRecorder) ClearOrgSyncHold(_ context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clearHoldCalls++
	return nil
}

func (f *fakeRunRecorder) attemptCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.attempts)
}

func (f *fakeRunRecorder) lastAttempt() orgprovider.OrgSyncAttempt {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.attempts[len(f.attempts)-1]
}

// fakeHeldLock records whether Release was called.
type fakeHeldLock struct {
	mu       sync.Mutex
	released bool
}

func (l *fakeHeldLock) Release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.released = true
}

// fakeLocker is a hand-rolled fake of services.SyncLocker.
type fakeLocker struct {
	// held, when true, means TryLock returns (nil, nil) -- locked elsewhere.
	held bool
	// err, when set, means TryLock fails outright (a configured locker that
	// cannot even attempt the lock).
	err error

	mu      sync.Mutex
	granted []*fakeHeldLock
}

func (l *fakeLocker) TryLock(_ context.Context) (services.HeldLock, error) {
	if l.err != nil {
		return nil, l.err
	}
	if l.held {
		return nil, nil
	}
	lock := &fakeHeldLock{}
	l.mu.Lock()
	l.granted = append(l.granted, lock)
	l.mu.Unlock()
	return lock, nil
}

// genericFailingRepo returns a plain, non-hold, non-ambiguous error from
// ApplySnapshot -- a confirmed rollback, not blocked and not ambiguous.
type genericFailingRepo struct{ err error }

func (r *genericFailingRepo) KnownHierarchyLevelIDs(_ context.Context) (map[string]bool, error) {
	return knownLevels(), nil
}

func (r *genericFailingRepo) ApplySnapshot(_ context.Context, _ orgprovider.ApplyInput) (*orgprovider.ApplyResult, error) {
	return nil, r.err
}

func TestSyncInProgressErrorWrapsTheSentinel(t *testing.T) {
	err := &services.SyncInProgressError{Trigger: orgprovider.TriggerScheduled}
	if !errors.Is(err, services.ErrSyncInProgress) {
		t.Fatal("SyncInProgressError must satisfy errors.Is(err, ErrSyncInProgress) via Unwrap")
	}
}

func TestSyncInProgressErrorNeverGuessesAnUnknownTrigger(t *testing.T) {
	err := &services.SyncInProgressError{Remote: true}
	msg := err.Error()
	if msg == "" {
		t.Fatal("expected a non-empty message")
	}
	// Must not assert a specific trigger it cannot support.
	if strings.Contains(msg, "manual") || strings.Contains(msg, "scheduled") {
		t.Fatalf("an unknown trigger must not be guessed as manual or scheduled, got: %q", msg)
	}
}

// TestLocalConcurrentSyncNamesTheRunningTrigger drives two overlapping
// SyncWithOptions calls on the SAME service instance: the first is gated
// inside ApplySnapshot (channel gating, the repo's own established
// concurrency-test pattern -- see settings_deletion_threshold_test.go's
// gatedFetcher), so the second's tier-1 CAS is guaranteed to lose while the
// first is genuinely still running.
type gatedRepo struct {
	gate    chan struct{}
	started chan struct{}
}

func (r *gatedRepo) KnownHierarchyLevelIDs(_ context.Context) (map[string]bool, error) {
	return knownLevels(), nil
}

func (r *gatedRepo) ApplySnapshot(_ context.Context, in orgprovider.ApplyInput) (*orgprovider.ApplyResult, error) {
	select {
	case r.started <- struct{}{}:
	default:
	}
	<-r.gate
	return &orgprovider.ApplyResult{UsersSynced: len(in.Snapshot.Users)}, nil
}

func TestLocalConcurrentSyncNamesTheRunningTrigger(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "20")
	repo := &gatedRepo{gate: make(chan struct{}), started: make(chan struct{}, 1)}
	service := services.NewOrganizationSyncService(repo, thresholdFetcher{}, nil, nil)

	firstDone := make(chan error, 1)
	go func() {
		_, err := service.SyncWithOptions(context.Background(), services.SyncOptions{Trigger: orgprovider.TriggerScheduled})
		firstDone <- err
	}()

	select {
	case <-repo.started:
	case <-time.After(2 * time.Second):
		t.Fatal("first sync never reached ApplySnapshot")
	}

	_, err := service.SyncWithOptions(context.Background(), services.SyncOptions{Trigger: orgprovider.TriggerManual})
	var inProgress *services.SyncInProgressError
	if !errors.As(err, &inProgress) {
		t.Fatalf("expected a *SyncInProgressError, got %v", err)
	}
	if inProgress.Remote {
		t.Fatal("a same-replica collision must not be reported as remote")
	}
	if inProgress.Trigger != orgprovider.TriggerScheduled {
		t.Fatalf("expected the conflict to name the ACTUALLY running trigger (scheduled), got %q", inProgress.Trigger)
	}

	close(repo.gate)
	if err := <-firstDone; err != nil {
		t.Fatalf("the first (gated) sync should have completed cleanly, got %v", err)
	}
}

func TestConfiguredLockerErrorFailsSyncClosed(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "20")
	repo := &recordingRepo{}
	locker := &fakeLocker{err: errors.New("advisory lock connection unavailable")}
	service := services.NewOrganizationSyncService(repo, thresholdFetcher{}, nil, nil,
		services.WithSyncLocker(locker))

	_, err := service.Sync(context.Background())
	if err == nil {
		t.Fatal("a configured locker that errors must fail the sync closed, not proceed unprotected")
	}
	if repo.lastInput.Snapshot != nil {
		t.Fatal("ApplySnapshot must never be reached when the locker fails to even attempt the lock")
	}
}

func TestRemoteLockHeldIsReportedAsRemoteWithUnknownTrigger(t *testing.T) {
	repo := &recordingRepo{}
	locker := &fakeLocker{held: true}
	service := services.NewOrganizationSyncService(repo, thresholdFetcher{}, nil, nil,
		services.WithSyncLocker(locker), services.WithSyncRunRecorder(&fakeRunRecorder{}))

	_, err := service.Sync(context.Background())
	var inProgress *services.SyncInProgressError
	if !errors.As(err, &inProgress) {
		t.Fatalf("expected a *SyncInProgressError, got %v", err)
	}
	if !inProgress.Remote {
		t.Fatal("a lock held by another replica must be reported as remote")
	}
	if inProgress.Trigger != "" || inProgress.Instance != "" {
		t.Fatalf("this process cannot attribute a remote lock, so trigger and instance must stay unknown; got %+v", inProgress)
	}
}

func TestBlockedAttemptRecordsWritesNoneNotUnknown(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "")
	repo := &holdingRepo{}
	recorder := &fakeRunRecorder{}
	service := services.NewOrganizationSyncService(repo, thresholdFetcher{}, nil, nil,
		services.WithDeleteThresholdStore(&thresholdStore{saved: floatPtr(20)}),
		services.WithSyncRunRecorder(recorder))

	if _, err := service.Sync(context.Background()); err == nil {
		t.Fatal("expected the attempt to be held")
	}

	if recorder.attemptCount() != 1 {
		t.Fatalf("expected exactly one durable attempt record, got %d", recorder.attemptCount())
	}
	attempt := recorder.lastAttempt()
	if attempt.Status != orgprovider.StatusBlocked {
		t.Fatalf("expected status=blocked, got %q", attempt.Status)
	}
	if attempt.WritesStatus != orgprovider.WritesNone {
		t.Fatalf("a blocked attempt writes nothing -- expected writesStatus=none, got %q", attempt.WritesStatus)
	}
	if attempt.ThresholdPercent == nil || *attempt.ThresholdPercent != 20 {
		t.Fatalf("expected the threshold actually judged against to be recorded, got %v", attempt.ThresholdPercent)
	}
	if attempt.UsersDeleting == nil || attempt.UsersPercent == nil {
		t.Fatal("a blocked attempt must record its computed deletion counts/percentages")
	}
}

func TestFailedAttemptBeforeApplySnapshotRecordsWritesNoneAndNoThreshold(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "20")
	recorder := &fakeRunRecorder{}
	// A fetcher failure happens before ApplySnapshot is ever reached, so the
	// threshold, though already resolved by this point, must NOT be recorded
	// -- only blocked/success outcomes carry a threshold.
	service := services.NewOrganizationSyncService(&recordingRepo{}, failingFetcher{err: errors.New("provider unreachable")}, nil, nil,
		services.WithSyncRunRecorder(recorder))

	if _, err := service.Sync(context.Background()); err == nil {
		t.Fatal("expected a fetch failure")
	}

	attempt := recorder.lastAttempt()
	if attempt.Status != orgprovider.StatusFailed {
		t.Fatalf("expected status=failed, got %q", attempt.Status)
	}
	if attempt.WritesStatus != orgprovider.WritesNone {
		t.Fatalf("a confirmed pre-write failure must record writesStatus=none, got %q", attempt.WritesStatus)
	}
	if attempt.ThresholdPercent != nil {
		t.Fatal("a failed attempt must never carry a threshold, even if one had already been resolved")
	}
}

func TestAmbiguousCommitRecordsWritesUnknownNotNoneOrSuccess(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "20")
	recorder := &fakeRunRecorder{}
	ambiguous := errors.Join(orgprovider.ErrSyncCommitAmbiguous, errors.New("connection reset during commit"))
	service := services.NewOrganizationSyncService(&genericFailingRepo{err: ambiguous}, thresholdFetcher{}, nil, nil,
		services.WithSyncRunRecorder(recorder))

	if _, err := service.Sync(context.Background()); !errors.Is(err, orgprovider.ErrSyncCommitAmbiguous) {
		t.Fatalf("expected the ambiguous-commit error to propagate, got %v", err)
	}

	attempt := recorder.lastAttempt()
	if attempt.WritesStatus != orgprovider.WritesUnknown {
		t.Fatalf("an ambiguous commit must record writesStatus=unknown, got %q", attempt.WritesStatus)
	}
	if attempt.Status != orgprovider.StatusFailed {
		t.Fatalf("expected status=failed for an ambiguous outcome, got %q", attempt.Status)
	}
}

func TestConfirmedRollbackIsNeverRecordedAsUnknown(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "20")
	recorder := &fakeRunRecorder{}
	// A plain, non-ambiguous error: the transaction was deliberately rolled
	// back, a fully deterministic outcome.
	service := services.NewOrganizationSyncService(&genericFailingRepo{err: errors.New("constraint violation")}, thresholdFetcher{}, nil, nil,
		services.WithSyncRunRecorder(recorder))

	if _, err := service.Sync(context.Background()); err == nil {
		t.Fatal("expected an error")
	}

	attempt := recorder.lastAttempt()
	if attempt.WritesStatus != orgprovider.WritesNone {
		t.Fatalf("a confirmed rollback must record writesStatus=none, not %q -- conflating it with unknown defeats the tri-state design", attempt.WritesStatus)
	}
}

func TestSchedulerBlockedNeverConsultsTheDurableHold(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "20")
	recorder := &fakeRunRecorder{
		lastRun: &orgprovider.OrgSyncLastRun{
			HoldActive:     true,
			HoldRecordedAt: timePtr(time.Now().UTC()),
		},
	}
	service := services.NewOrganizationSyncService(&recordingRepo{}, thresholdFetcher{}, nil, nil,
		services.WithSyncRunRecorder(recorder))

	// SchedulerBlocked must read ONLY in-memory state: with no in-memory hold
	// set (this service has never run a sync), it must report false even
	// though the durable flag says held -- otherwise a restart-surviving hold
	// would block the scheduler, contradicting the decision that a restart
	// permits a harmless re-attempt.
	if blocked, reason := service.SchedulerBlocked(); blocked {
		t.Fatalf("SchedulerBlocked must never be true from the durable hold alone, got reason %q", reason)
	}

	// LockState, by contrast, MUST see the durable hold -- this is the
	// structural proof that the two are genuinely different code paths, not
	// the same check filtered two ways.
	state, err := service.LockState(context.Background())
	if err != nil {
		t.Fatalf("unexpected LockState error: %v", err)
	}
	if !state.Held {
		t.Fatal("LockState must be fail-closed on the durable hold_active flag")
	}
}

func TestLockStateFailsClosedOnADurableReadError(t *testing.T) {
	recorder := &fakeRunRecorder{lastRunErr: errors.New("connection reset")}
	service := services.NewOrganizationSyncService(&recordingRepo{}, thresholdFetcher{}, nil, nil,
		services.WithSyncRunRecorder(recorder))

	_, err := service.LockState(context.Background())
	if err == nil {
		t.Fatal("a durable-state read error must be returned, never silently treated as unlocked")
	}
}

func TestClearHoldClearsBothInMemoryAndDurable(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "")
	repo := &holdingRepo{}
	recorder := &fakeRunRecorder{}
	service := services.NewOrganizationSyncService(repo, thresholdFetcher{}, nil, nil,
		services.WithDeleteThresholdStore(&thresholdStore{saved: floatPtr(20)}),
		services.WithSyncRunRecorder(recorder))

	if _, err := service.Sync(context.Background()); err == nil {
		t.Fatal("expected the attempt to be held")
	}

	if err := service.ClearHold(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recorder.clearHoldCalls != 1 {
		t.Fatalf("expected ClearOrgSyncHold to be called exactly once, got %d", recorder.clearHoldCalls)
	}
	if service.DismissHold() {
		t.Fatal("the in-memory hold should already be clear after ClearHold")
	}
}

// failingFetcher always fails, for testing the pre-ApplySnapshot failure path.
type failingFetcher struct{ err error }

func (f failingFetcher) FetchSnapshot(_ context.Context) (*orgsnapshot.Snapshot, error) {
	return nil, f.err
}

func timePtr(t time.Time) *time.Time { return &t }

// --- Single-instance (no distributed locker) coverage -----------------------
//
// These tests pin the current deployment shape: no SyncLocker is wired
// anywhere in this package's helpers unless a test explicitly opts in (as the
// locker-specific tests above already do), so every test below exercises the
// nil-locker path that main.go now uses. The in-process `running` flag is the
// only admission gate under test here.

// TestManualSyncSucceedsWithNoLockerConfigured pins the happy path for a
// manual sync with no distributed locker wired at all -- the deployment shape
// after removing the Postgres advisory lock from main.go. Success-attempt
// recording happens inside the real repository's own ApplySnapshot
// transaction (see recordSuccessAttempt's doc), not in this service, so the
// fake recordingRepo used here never touches the recorder -- this test's
// scope is "no locker means no error", not attempt persistence.
func TestManualSyncSucceedsWithNoLockerConfigured(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "20")
	service := services.NewOrganizationSyncService(&recordingRepo{}, thresholdFetcher{}, nil, nil)

	if _, err := service.SyncWithOptions(context.Background(), services.SyncOptions{Trigger: orgprovider.TriggerManual}); err != nil {
		t.Fatalf("a manual sync with no locker configured must succeed, got %v", err)
	}
}

// TestScheduledSyncSucceedsWithNoLockerConfigured mirrors the manual-trigger
// test above for a scheduled trigger -- both triggers funnel through the same
// SyncWithOptions, so both must behave identically with no locker wired.
func TestScheduledSyncSucceedsWithNoLockerConfigured(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "20")
	service := services.NewOrganizationSyncService(&recordingRepo{}, thresholdFetcher{}, nil, nil)

	if _, err := service.SyncWithOptions(context.Background(), services.SyncOptions{Trigger: orgprovider.TriggerScheduled}); err != nil {
		t.Fatalf("a scheduled sync with no locker configured must succeed, got %v", err)
	}
}

// TestSyncSucceedsWhenLockerExplicitlyNil is the direct counterpart to
// TestConfiguredLockerErrorFailsSyncClosed above: a locker option passed nil
// (as opposed to never called) must still be treated as "no distributed
// locking configured", never dereferenced, and HasSyncLocker must agree.
func TestSyncSucceedsWhenLockerExplicitlyNil(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "20")
	service := services.NewOrganizationSyncService(&recordingRepo{}, thresholdFetcher{}, nil, nil,
		services.WithSyncLocker(nil))

	if service.HasSyncLocker() {
		t.Fatal("HasSyncLocker must report false when the locker option was passed nil")
	}
	if _, err := service.SyncWithOptions(context.Background(), services.SyncOptions{Trigger: orgprovider.TriggerManual}); err != nil {
		t.Fatalf("a nil locker must be safely skipped, not cause an error: %v", err)
	}
}

// TestConcurrentManualSyncAttemptsExactlyOneWins is
// TestLocalConcurrentSyncNamesTheRunningTrigger's manual/manual counterpart:
// that test proves manual cannot overlap scheduled; this proves two manual
// requests cannot overlap each other either, using the same gated-repo
// pattern to guarantee the first attempt is genuinely still running.
func TestConcurrentManualSyncAttemptsExactlyOneWins(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "20")
	repo := &gatedRepo{gate: make(chan struct{}), started: make(chan struct{}, 1)}
	service := services.NewOrganizationSyncService(repo, thresholdFetcher{}, nil, nil)

	firstDone := make(chan error, 1)
	go func() {
		_, err := service.SyncWithOptions(context.Background(), services.SyncOptions{Trigger: orgprovider.TriggerManual})
		firstDone <- err
	}()

	select {
	case <-repo.started:
	case <-time.After(2 * time.Second):
		t.Fatal("first manual sync never reached ApplySnapshot")
	}

	_, err := service.SyncWithOptions(context.Background(), services.SyncOptions{Trigger: orgprovider.TriggerManual})
	var inProgress *services.SyncInProgressError
	if !errors.As(err, &inProgress) {
		t.Fatalf("expected the second concurrent manual sync to be refused with *SyncInProgressError, got %v", err)
	}
	if inProgress.Trigger != orgprovider.TriggerManual {
		t.Fatalf("expected the conflict to name manual (the actually running trigger), got %q", inProgress.Trigger)
	}

	close(repo.gate)
	if err := <-firstDone; err != nil {
		t.Fatalf("the first (gated) manual sync should have completed cleanly, got %v", err)
	}
}

// TestConcurrentScheduledSyncAttemptsExactlyOneWins completes the pairwise
// coverage: manual/manual and manual/scheduled are both proven above; this
// covers scheduled/scheduled, e.g. a slow-running scheduled sync still
// in-flight when the scheduler's next tick fires.
func TestConcurrentScheduledSyncAttemptsExactlyOneWins(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "20")
	repo := &gatedRepo{gate: make(chan struct{}), started: make(chan struct{}, 1)}
	service := services.NewOrganizationSyncService(repo, thresholdFetcher{}, nil, nil)

	firstDone := make(chan error, 1)
	go func() {
		_, err := service.SyncWithOptions(context.Background(), services.SyncOptions{Trigger: orgprovider.TriggerScheduled})
		firstDone <- err
	}()

	select {
	case <-repo.started:
	case <-time.After(2 * time.Second):
		t.Fatal("first scheduled sync never reached ApplySnapshot")
	}

	_, err := service.SyncWithOptions(context.Background(), services.SyncOptions{Trigger: orgprovider.TriggerScheduled})
	var inProgress *services.SyncInProgressError
	if !errors.As(err, &inProgress) {
		t.Fatalf("expected the second concurrent scheduled sync to be refused with *SyncInProgressError, got %v", err)
	}
	if inProgress.Trigger != orgprovider.TriggerScheduled {
		t.Fatalf("expected the conflict to name scheduled (the actually running trigger), got %q", inProgress.Trigger)
	}

	close(repo.gate)
	if err := <-firstDone; err != nil {
		t.Fatalf("the first (gated) scheduled sync should have completed cleanly, got %v", err)
	}
}

// toggleFetcher fails on a chosen call number and returns a valid snapshot on
// every other call, letting a test drive "sync fails, then a later sync
// succeeds" without needing two separate service instances (which would each
// have their own, uninformative `running` flag).
type toggleFetcher struct {
	mu      sync.Mutex
	calls   int
	failOn  int // 1-indexed call number to fail; 0 means never fail
	failErr error
	ok      *orgsnapshot.Snapshot
}

func (f *toggleFetcher) FetchSnapshot(_ context.Context) (*orgsnapshot.Snapshot, error) {
	f.mu.Lock()
	f.calls++
	n := f.calls
	f.mu.Unlock()
	if f.failOn != 0 && n == f.failOn {
		return nil, f.failErr
	}
	return f.ok, nil
}

// TestSyncSucceedsAfterAPriorSyncFailed proves the running flag (and its
// paired runningTrigger) are reset on the FAILURE path, not only on success --
// a sync that fails must never permanently wedge every future sync.
func TestSyncSucceedsAfterAPriorSyncFailed(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "20")
	ok, err := thresholdFetcher{}.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("unexpected error building the baseline snapshot: %v", err)
	}
	fetcher := &toggleFetcher{failOn: 1, failErr: errors.New("provider unreachable"), ok: ok}
	service := services.NewOrganizationSyncService(&recordingRepo{}, fetcher, nil, nil)

	if _, err := service.SyncWithOptions(context.Background(), services.SyncOptions{Trigger: orgprovider.TriggerManual}); err == nil {
		t.Fatal("expected the first sync to fail")
	}

	if _, err := service.SyncWithOptions(context.Background(), services.SyncOptions{Trigger: orgprovider.TriggerScheduled}); err != nil {
		t.Fatalf("expected a later sync to run and succeed after the earlier one failed, got %v", err)
	}
}

// TestSyncSucceedsAfterAPriorSyncSucceeded is the success/success companion:
// the running flag must reset after a clean completion too, so the schedule's
// next occurrence (or another manual click) is never refused by a stale flag.
func TestSyncSucceedsAfterAPriorSyncSucceeded(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "20")
	service := services.NewOrganizationSyncService(&recordingRepo{}, thresholdFetcher{}, nil, nil)

	if _, err := service.SyncWithOptions(context.Background(), services.SyncOptions{Trigger: orgprovider.TriggerManual}); err != nil {
		t.Fatalf("unexpected error on the first sync: %v", err)
	}
	if _, err := service.SyncWithOptions(context.Background(), services.SyncOptions{Trigger: orgprovider.TriggerScheduled}); err != nil {
		t.Fatalf("expected a later sync to run after an earlier one succeeded, got %v", err)
	}
}

// panicRepo panics inside ApplySnapshot, signalling on started first so a
// test can confirm it was actually reached (as opposed to being refused by
// the running flag before ever getting there).
type panicRepo struct {
	started chan struct{}
}

func (r *panicRepo) KnownHierarchyLevelIDs(_ context.Context) (map[string]bool, error) {
	return knownLevels(), nil
}

func (r *panicRepo) ApplySnapshot(_ context.Context, _ orgprovider.ApplyInput) (*orgprovider.ApplyResult, error) {
	select {
	case r.started <- struct{}{}:
	default:
	}
	panic("simulated panic during ApplySnapshot")
}

// TestPanicDuringSyncStillReleasesRunningFlag proves the running flag's
// release is a defer set up immediately after it is won (before any of the
// fetch/validate/apply work that could panic), so a panic mid-sync still
// releases it -- otherwise a single panicking sync would permanently refuse
// every manual and scheduled sync after it, with no way to recover short of a
// process restart.
func TestPanicDuringSyncStillReleasesRunningFlag(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "20")
	repo := &panicRepo{started: make(chan struct{}, 2)}
	service := services.NewOrganizationSyncService(repo, thresholdFetcher{}, nil, nil)

	func() {
		defer func() { _ = recover() }()
		_, _ = service.SyncWithOptions(context.Background(), services.SyncOptions{Trigger: orgprovider.TriggerManual})
	}()

	select {
	case <-repo.started:
	default:
		t.Fatal("expected the first (panicking) sync to have reached ApplySnapshot")
	}

	func() {
		defer func() { _ = recover() }()
		_, _ = service.SyncWithOptions(context.Background(), services.SyncOptions{Trigger: orgprovider.TriggerScheduled})
	}()

	select {
	case <-repo.started:
	default:
		t.Fatal("expected the second sync to reach ApplySnapshot too -- the running flag must be released after a panic, not left stuck true")
	}
}

// --- Run IDs ----------------------------------------------------------------

func TestSyncStampsAFailedAttemptWithTheRunIDItWasGiven(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "20")
	recorder := &fakeRunRecorder{}
	service := services.NewOrganizationSyncService(&genericFailingRepo{err: errors.New("boom")}, thresholdFetcher{}, nil, nil,
		services.WithSyncRunRecorder(recorder))

	given := uuid.NewString()
	if _, err := service.SyncWithOptions(context.Background(), services.SyncOptions{RunID: given}); err == nil {
		t.Fatal("expected an error")
	}

	if got := recorder.lastAttempt().RunID; got != given {
		t.Fatalf("the recorded attempt must carry the run ID the caller supplied (%s), got %q", given, got)
	}
}

func TestSyncGeneratesAValidRunIDWhenNoneIsGiven(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "20")
	recorder := &fakeRunRecorder{}
	service := services.NewOrganizationSyncService(&genericFailingRepo{err: errors.New("boom")}, thresholdFetcher{}, nil, nil,
		services.WithSyncRunRecorder(recorder))

	if _, err := service.Sync(context.Background()); err == nil {
		t.Fatal("expected an error")
	}

	if _, err := uuid.Parse(recorder.lastAttempt().RunID); err != nil {
		t.Fatalf("expected a generated UUID run ID, got %q (%v)", recorder.lastAttempt().RunID, err)
	}
}

func TestEachSyncGetsItsOwnRunID(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "20")
	recorder := &fakeRunRecorder{}
	service := services.NewOrganizationSyncService(&genericFailingRepo{err: errors.New("boom")}, thresholdFetcher{}, nil, nil,
		services.WithSyncRunRecorder(recorder))

	_, _ = service.Sync(context.Background())
	first := recorder.lastAttempt().RunID
	_, _ = service.Sync(context.Background())
	second := recorder.lastAttempt().RunID

	if first == second {
		t.Fatalf("two syncs must not share a run ID, both were %q", first)
	}
}

func TestSuccessfulSyncReturnsItsRunID(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "20")
	service := services.NewOrganizationSyncService(&recordingRepo{}, thresholdFetcher{}, nil, nil)

	given := uuid.NewString()
	result, err := service.SyncWithOptions(context.Background(), services.SyncOptions{RunID: given})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RunID != given {
		t.Fatalf("the result must carry the run ID (%s), got %q", given, result.RunID)
	}
}

func TestRecordSkipPassesTheRunIDToTheRecorder(t *testing.T) {
	recorder := &fakeRunRecorder{}
	service := services.NewOrganizationSyncService(&recordingRepo{}, thresholdFetcher{}, nil, nil,
		services.WithSyncRunRecorder(recorder))

	given := uuid.NewString()
	if err := service.RecordSkip(context.Background(), given, orgprovider.SkipManualRunning); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(recorder.skips) != 1 || recorder.skips[0].RunID != given || recorder.skips[0].Reason != orgprovider.SkipManualRunning {
		t.Fatalf("expected one skip recorded under run ID %s, got %+v", given, recorder.skips)
	}
}

func TestGetHistoryReadsFromTheRecorderWithTheRequestedLimit(t *testing.T) {
	entry := orgprovider.OrgSyncHistoryEntry{OrgSyncAttempt: orgprovider.OrgSyncAttempt{RunID: uuid.NewString(), Status: orgprovider.StatusFailed}}
	recorder := &fakeRunRecorder{history: []orgprovider.OrgSyncHistoryEntry{entry}}
	service := services.NewOrganizationSyncService(&recordingRepo{}, thresholdFetcher{}, nil, nil,
		services.WithSyncRunRecorder(recorder))

	got, err := service.GetHistory(context.Background(), 25)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].RunID != entry.RunID {
		t.Fatalf("expected the recorder's history to be returned, got %+v", got)
	}
	if recorder.historyLimit != 25 {
		t.Fatalf("expected the limit to be passed through, got %d", recorder.historyLimit)
	}
}

func TestGetHistoryIsEmptyNotAnErrorWithoutARecorder(t *testing.T) {
	service := services.NewOrganizationSyncService(&recordingRepo{}, thresholdFetcher{}, nil, nil)

	got, err := service.GetHistory(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("expected an empty, non-nil list, got %#v", got)
	}
}

func TestLockStateUsesTheHeldRowsThresholdNotTheLastAttempts(t *testing.T) {
	// A blocked sync at 25%, then an unrelated failure: the last attempt has
	// no threshold, but the durable hold must still report the one it was
	// judged against.
	held := 25.0
	recorder := &fakeRunRecorder{
		lastRun: &orgprovider.OrgSyncLastRun{
			HoldActive:           true,
			HoldRecordedAt:       timePtr(time.Now().UTC()),
			HoldThresholdPercent: &held,
			LastAttempt:          &orgprovider.OrgSyncAttempt{Status: orgprovider.StatusFailed},
		},
	}
	service := services.NewOrganizationSyncService(&recordingRepo{}, thresholdFetcher{}, nil, nil,
		services.WithSyncRunRecorder(recorder))

	state, err := service.LockState(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !state.Held {
		t.Fatal("expected the durable hold to freeze the threshold")
	}
	if state.Threshold != held {
		t.Fatalf("expected the held threshold %v, got %v", held, state.Threshold)
	}
}
