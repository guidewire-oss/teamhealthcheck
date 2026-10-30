package services_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	"github.com/agopalakrishnan/teams360/backend/domain/orgsyncrun"
	"github.com/agopalakrishnan/teams360/backend/pkg/orgsnapshot"
)

// memStore is an in-memory orgsyncrun.Store that enforces the same rules as the
// SQL one: a single active run, guarded transitions, and a lock with an owner.
type memStore struct {
	mu        sync.Mutex
	runs      map[string]*orgsyncrun.Run
	order     []string
	idem      map[string]string
	lockHeld  bool
	lockFail  int   // number of TryLock calls to refuse before granting
	activeErr error // when set, Active fails (to prove the lock state fails closed)
}

func newMemStore() *memStore {
	return &memStore{runs: map[string]*orgsyncrun.Run{}, idem: map[string]string{}}
}

func (m *memStore) CreatePending(_ context.Context, p orgsyncrun.NewParams) (*orgsyncrun.Run, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p.IdempotencyKey != "" {
		if id, ok := m.idem[p.IdempotencyKey]; ok {
			c := *m.runs[id]
			return &c, false, nil
		}
	}
	for _, r := range m.runs {
		if !r.Status.Terminal() {
			c := *r
			return &c, false, orgsyncrun.ErrActiveRunExists
		}
	}
	r := &orgsyncrun.Run{ID: p.ID, Status: orgsyncrun.StatusPending, Stage: orgsyncrun.StageQueued, CreatedAt: time.Now()}
	m.runs[p.ID] = r
	m.order = append(m.order, p.ID)
	if p.IdempotencyKey != "" {
		m.idem[p.IdempotencyKey] = p.ID
	}
	c := *r
	return &c, true, nil
}

func (m *memStore) Get(_ context.Context, id string) (*orgsyncrun.Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.runs[id]
	if !ok {
		return nil, orgsyncrun.ErrNotFound
	}
	c := *r
	return &c, nil
}

func (m *memStore) Latest(_ context.Context) (*orgsyncrun.Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.order) == 0 {
		return nil, nil
	}
	c := *m.runs[m.order[len(m.order)-1]]
	return &c, nil
}

func (m *memStore) Active(_ context.Context) (*orgsyncrun.Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeErr != nil {
		return nil, m.activeErr
	}
	for _, r := range m.runs {
		if !r.Status.Terminal() {
			c := *r
			return &c, nil
		}
	}
	return nil, nil
}

func (m *memStore) move(id string, to orgsyncrun.Status, apply func(*orgsyncrun.Run)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.runs[id]
	if r == nil || !orgsyncrun.CanTransition(r.Status, to) {
		return orgsyncrun.ErrNotTransitionable
	}
	r.Status = to
	apply(r)
	return nil
}

func (m *memStore) SetStage(_ context.Context, id string, st orgsyncrun.Stage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r := m.runs[id]; r != nil && !r.Status.Terminal() {
		r.Stage = st
		return nil
	}
	return orgsyncrun.ErrNotTransitionable
}

func (m *memStore) MarkRunning(_ context.Context, id, _ string) error {
	return m.move(id, orgsyncrun.StatusRunning, func(*orgsyncrun.Run) {})
}

func (m *memStore) Fail(_ context.Context, id string, f orgsyncrun.Failure) error {
	return m.move(id, orgsyncrun.StatusFailed, func(r *orgsyncrun.Run) {
		r.Stage = orgsyncrun.StageFailed
		fs, code, msg := f.Stage, f.Code, f.Message
		r.FailedStage, r.ErrorCode, r.ErrorMessage, r.HoldReport = &fs, &code, &msg, f.HoldReport
	})
}

// succeed mimics ApplySnapshot's in-transaction SUCCESS write.
func (m *memStore) succeed(id string) error {
	return m.move(id, orgsyncrun.StatusSuccess, func(r *orgsyncrun.Run) { r.Stage = orgsyncrun.StageCompleted })
}

func (m *memStore) RecoverStale(context.Context, time.Duration) (int, error) { return 0, nil }

func (m *memStore) TryLock(context.Context) (func(), bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lockFail > 0 {
		m.lockFail--
		return nil, false, nil
	}
	if m.lockHeld {
		return nil, false, nil
	}
	m.lockHeld = true
	return func() { m.mu.Lock(); m.lockHeld = false; m.mu.Unlock() }, true, nil
}

func (m *memStore) isLocked() bool { m.mu.Lock(); defer m.mu.Unlock(); return m.lockHeld }

// runRepo marks the run SUCCESS the way the SQL repo does, or fails as told.
type runRepo struct {
	store *memStore
	err   error
	gate  chan struct{} // when set, ApplySnapshot blocks until closed
	// afterSuccess, when set, makes ApplySnapshot pause AFTER the run's SUCCESS
	// has been recorded, which is the window where a lock read used to go stale.
	afterSuccess chan struct{}
	succeeded    chan struct{}
	seen         chan struct{} // closed once ApplySnapshot has started
	once         sync.Once
	last         orgprovider.ApplyInput
}

func (r *runRepo) KnownHierarchyLevelIDs(context.Context) (map[string]bool, error) {
	return knownLevels(), nil
}

func (r *runRepo) ApplySnapshot(_ context.Context, in orgprovider.ApplyInput) (*orgprovider.ApplyResult, error) {
	r.last = in
	if in.OnStage != nil {
		in.OnStage(string(orgsyncrun.StageUpsertingUsers))
	}
	if r.seen != nil {
		r.once.Do(func() { close(r.seen) })
	}
	if r.gate != nil {
		<-r.gate
	}
	if r.err != nil {
		return nil, r.err
	}
	if err := r.store.succeed(in.RunID); err != nil {
		return nil, err
	}
	if r.succeeded != nil {
		close(r.succeeded)
	}
	if r.afterSuccess != nil {
		<-r.afterSuccess
	}
	return &orgprovider.ApplyResult{UsersSynced: 1, InsertedUsers: 1}, nil
}

type typedFetcher struct{ snapType string }

func (f typedFetcher) FetchSnapshot(ctx context.Context) (*orgsnapshot.Snapshot, error) {
	s, _ := thresholdFetcher{}.FetchSnapshot(ctx)
	s.SnapshotType = f.snapType
	return s, nil
}

func waitTerminal(t *testing.T, svc *services.OrganizationSyncService, id string) *orgsyncrun.Run {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r, err := svc.GetRun(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if r.Status.Terminal() {
			return r
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run %s never reached a terminal state", id)
	return nil
}

func fastTiming(t *testing.T) {
	oldWait, oldRetry := services.LockWaitTimeout, services.LockRetryInterval
	services.LockWaitTimeout, services.LockRetryInterval = 300*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { services.LockWaitTimeout, services.LockRetryInterval = oldWait, oldRetry })
}

func TestStartRunReturnsPendingBeforeWorkFinishes(t *testing.T) {
	fastTiming(t)
	store := newMemStore()
	repo := &runRepo{store: store, gate: make(chan struct{}), seen: make(chan struct{})}
	svc := services.NewOrganizationSyncService(repo, typedFetcher{}, nil, nil, services.WithRunStore(store))

	run, err := svc.StartRun(context.Background(), services.SyncOptions{ActorUserID: "admin"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if run.ID == "" || run.Status != orgsyncrun.StatusPending {
		t.Fatalf("want a PENDING run with an id, got %+v", run)
	}

	<-repo.seen
	mid, _ := svc.GetRun(context.Background(), run.ID)
	if mid.Status != orgsyncrun.StatusRunning || mid.Stage != orgsyncrun.StageUpsertingUsers {
		t.Fatalf("want RUNNING/UPSERTING_USERS while applying, got %s/%s", mid.Status, mid.Stage)
	}
	if !store.isLocked() {
		t.Fatal("the cross-instance lock must be held while running")
	}

	close(repo.gate)
	done := waitTerminal(t, svc, run.ID)
	if done.Status != orgsyncrun.StatusSuccess {
		t.Fatalf("want SUCCESS, got %s (%v)", done.Status, done.ErrorMessage)
	}
	if repo.last.RunID != run.ID || repo.last.SnapshotType != orgprovider.SnapshotComplete {
		t.Fatalf("apply must receive the run id and a COMPLETE type, got %+v", repo.last)
	}
	time.Sleep(20 * time.Millisecond)
	if store.isLocked() {
		t.Fatal("lock must be released after the run")
	}
}

func TestFailedRunRecordsStageAndSafeMessage(t *testing.T) {
	fastTiming(t)
	store := newMemStore()
	repo := &runRepo{store: store, err: errors.New(`pq: duplicate key value violates "users_email_key" (secret@corp.com)`)}
	svc := services.NewOrganizationSyncService(repo, typedFetcher{}, nil, nil, services.WithRunStore(store))

	run, _ := svc.StartRun(context.Background(), services.SyncOptions{}, "")
	done := waitTerminal(t, svc, run.ID)

	if done.Status != orgsyncrun.StatusFailed {
		t.Fatalf("want FAILED, got %s", done.Status)
	}
	if done.FailedStage == nil || *done.FailedStage != orgsyncrun.StageUpsertingUsers {
		t.Fatalf("want failed stage UPSERTING_USERS, got %v", done.FailedStage)
	}
	if *done.ErrorCode != orgsyncrun.CodeDatabase {
		t.Fatalf("want %s, got %s", orgsyncrun.CodeDatabase, *done.ErrorCode)
	}
	if got := *done.ErrorMessage; got == "" || contains(got, "secret@corp.com") || contains(got, "pq:") {
		t.Fatalf("error message must be safe, got %q", got)
	}
}

func TestInvalidPayloadFailsAtValidatingWithoutApplying(t *testing.T) {
	fastTiming(t)
	store := newMemStore()
	repo := &runRepo{store: store}
	svc := services.NewOrganizationSyncService(repo, typedFetcher{snapType: "BOGUS"}, nil, nil, services.WithRunStore(store))

	run, _ := svc.StartRun(context.Background(), services.SyncOptions{}, "")
	done := waitTerminal(t, svc, run.ID)

	if done.Status != orgsyncrun.StatusFailed || *done.ErrorCode != orgsyncrun.CodeInvalidSnapshot {
		t.Fatalf("want FAILED/INVALID_SNAPSHOT, got %s/%v", done.Status, done.ErrorCode)
	}
	if *done.FailedStage != orgsyncrun.StageValidating {
		t.Fatalf("want stage VALIDATING, got %s", *done.FailedStage)
	}
	if repo.last.Snapshot != nil {
		t.Fatal("an invalid payload must never reach the database layer")
	}
}

func TestPartialSnapshotIsNotAppliedAsComplete(t *testing.T) {
	fastTiming(t)
	store := newMemStore()
	repo := &runRepo{store: store}
	svc := services.NewOrganizationSyncService(repo, typedFetcher{snapType: "PARTIAL"}, nil, nil, services.WithRunStore(store))

	run, _ := svc.StartRun(context.Background(), services.SyncOptions{}, "")
	waitTerminal(t, svc, run.ID)
	if repo.last.SnapshotType != orgprovider.SnapshotPartial {
		t.Fatalf("PARTIAL payload must reach apply as PARTIAL, got %q", repo.last.SnapshotType)
	}
}

func TestSecondStartWhileActiveFollowsExistingRun(t *testing.T) {
	fastTiming(t)
	store := newMemStore()
	repo := &runRepo{store: store, gate: make(chan struct{}), seen: make(chan struct{})}
	svc := services.NewOrganizationSyncService(repo, typedFetcher{}, nil, nil, services.WithRunStore(store))

	first, _ := svc.StartRun(context.Background(), services.SyncOptions{}, "")
	<-repo.seen
	second, err := svc.StartRun(context.Background(), services.SyncOptions{}, "")
	if !errors.Is(err, orgsyncrun.ErrActiveRunExists) || second == nil || second.ID != first.ID {
		t.Fatalf("a duplicate click must return the active run, got %v / %+v", err, second)
	}
	close(repo.gate)
	waitTerminal(t, svc, first.ID)
}

func TestIdempotencyKeyReturnsOriginalRun(t *testing.T) {
	fastTiming(t)
	store := newMemStore()
	repo := &runRepo{store: store}
	svc := services.NewOrganizationSyncService(repo, typedFetcher{}, nil, nil, services.WithRunStore(store))

	a, _ := svc.StartRun(context.Background(), services.SyncOptions{}, "key-1")
	waitTerminal(t, svc, a.ID)
	b, err := svc.StartRun(context.Background(), services.SyncOptions{}, "key-1")
	if err != nil || b.ID != a.ID {
		t.Fatalf("same key must replay the first run, got %v / %+v", err, b)
	}
}

func TestLockContentionRetriesThenSucceeds(t *testing.T) {
	fastTiming(t)
	store := newMemStore()
	store.lockFail = 3
	repo := &runRepo{store: store}
	svc := services.NewOrganizationSyncService(repo, typedFetcher{}, nil, nil, services.WithRunStore(store))

	run, _ := svc.StartRun(context.Background(), services.SyncOptions{}, "")
	if done := waitTerminal(t, svc, run.ID); done.Status != orgsyncrun.StatusSuccess {
		t.Fatalf("a briefly busy lock must be retried, not failed; got %s", done.Status)
	}
}

func TestLockContentionTimeoutFailsAtAcquiringLock(t *testing.T) {
	fastTiming(t)
	store := newMemStore()
	store.lockFail = 1 << 30
	repo := &runRepo{store: store}
	svc := services.NewOrganizationSyncService(repo, typedFetcher{}, nil, nil, services.WithRunStore(store))

	run, _ := svc.StartRun(context.Background(), services.SyncOptions{}, "")
	done := waitTerminal(t, svc, run.ID)
	if done.Status != orgsyncrun.StatusFailed || *done.FailedStage != orgsyncrun.StageAcquiringLock || *done.ErrorCode != orgsyncrun.CodeLockUnavailable {
		t.Fatalf("want FAILED at ACQUIRING_LOCK, got %s/%v/%v", done.Status, done.FailedStage, done.ErrorCode)
	}
	if repo.last.Snapshot != nil {
		t.Fatal("nothing may be applied without the lock")
	}
}

func TestStaleWorkerCannotOverwriteTerminalRun(t *testing.T) {
	store := newMemStore()
	run, _, _ := store.CreatePending(context.Background(), orgsyncrun.NewParams{ID: "r1"})
	_ = store.MarkRunning(context.Background(), run.ID, "w")
	if err := store.Fail(context.Background(), run.ID, orgsyncrun.Failure{Stage: orgsyncrun.StageFetching, Code: orgsyncrun.CodeStaleRun}); err != nil {
		t.Fatal(err)
	}
	if err := store.succeed(run.ID); !errors.Is(err, orgsyncrun.ErrNotTransitionable) {
		t.Fatalf("a late SUCCESS must be refused, got %v", err)
	}
	if err := store.Fail(context.Background(), run.ID, orgsyncrun.Failure{Code: "X"}); !errors.Is(err, orgsyncrun.ErrNotTransitionable) {
		t.Fatalf("a second FAILED must be refused, got %v", err)
	}
}

func TestMassDeactivationHoldIsStoredForOverride(t *testing.T) {
	fastTiming(t)
	store := newMemStore()
	hold := &orgprovider.MassDeletionHoldError{Report: orgprovider.BuildMassDeletionReport(10, 8, 5, 0, 20)}
	repo := &runRepo{store: store, err: hold}
	svc := services.NewOrganizationSyncService(repo, typedFetcher{}, nil, nil, services.WithRunStore(store))

	run, _ := svc.StartRun(context.Background(), services.SyncOptions{}, "")
	done := waitTerminal(t, svc, run.ID)
	if *done.ErrorCode != orgsyncrun.CodeMassDeactivationHold || len(done.HoldReport) == 0 {
		t.Fatalf("hold must persist its counts, got %v / %s", done.ErrorCode, done.HoldReport)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ---- lock state: only an active (PENDING/RUNNING) run may hold the guard ----

func TestLockStateIsHeldOnlyWhileRunIsActive(t *testing.T) {
	fastTiming(t)
	ctx := context.Background()
	store := newMemStore()
	repo := &runRepo{store: store, gate: make(chan struct{}), seen: make(chan struct{})}
	svc := services.NewOrganizationSyncService(repo, typedFetcher{}, nil, nil, services.WithRunStore(store))

	if svc.LockState(ctx).Locked() {
		t.Fatal("nothing has run: the threshold must be editable")
	}
	run, _ := svc.StartRun(ctx, services.SyncOptions{}, "")
	<-repo.seen
	if st := svc.LockState(ctx); !st.Syncing || !st.Locked() {
		t.Fatalf("a RUNNING run must lock the threshold, got %+v", st)
	}

	close(repo.gate)
	waitTerminal(t, svc, run.ID)
	if st := svc.LockState(ctx); st.Syncing || st.Locked() {
		t.Fatalf("a SUCCESS run must release the threshold, got %+v", st)
	}
}

// The race behind the stuck orange banner: SUCCESS commits, the UI re-reads the
// lock, but the worker has not finished tearing down (its in-process flag is
// still set). The lock must already read as released.
func TestLockStateReleasedAtCommitEvenWhileWorkerIsStillTearingDown(t *testing.T) {
	fastTiming(t)
	ctx := context.Background()
	store := newMemStore()
	repo := &runRepo{store: store, afterSuccess: make(chan struct{}), succeeded: make(chan struct{})}
	svc := services.NewOrganizationSyncService(repo, typedFetcher{}, nil, nil, services.WithRunStore(store))

	run, _ := svc.StartRun(ctx, services.SyncOptions{}, "")
	<-repo.succeeded // SUCCESS recorded; worker still blocked inside ApplySnapshot

	if r, _ := svc.GetRun(ctx, run.ID); r.Status != orgsyncrun.StatusSuccess {
		t.Fatalf("want SUCCESS, got %s", r.Status)
	}
	if st := svc.LockState(ctx); st.Locked() {
		t.Fatalf("the guard must not stay locked after SUCCESS, got %+v", st)
	}

	close(repo.afterSuccess)
	waitTerminal(t, svc, run.ID)
}

func TestLockStateReleasedAfterFailedRun(t *testing.T) {
	fastTiming(t)
	ctx := context.Background()
	store := newMemStore()
	repo := &runRepo{store: store, err: errors.New("boom")}
	svc := services.NewOrganizationSyncService(repo, typedFetcher{}, nil, nil, services.WithRunStore(store))

	run, _ := svc.StartRun(ctx, services.SyncOptions{}, "")
	waitTerminal(t, svc, run.ID)
	if st := svc.LockState(ctx); st.Locked() {
		t.Fatalf("a FAILED run must release the threshold, got %+v", st)
	}
}

func TestCrossInstanceLockIsReleasedAfterRun(t *testing.T) {
	fastTiming(t)
	store := newMemStore()
	repo := &runRepo{store: store}
	svc := services.NewOrganizationSyncService(repo, typedFetcher{}, nil, nil, services.WithRunStore(store))

	run, _ := svc.StartRun(context.Background(), services.SyncOptions{}, "")
	waitTerminal(t, svc, run.ID)
	deadline := time.Now().Add(time.Second)
	for store.isLocked() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if store.isLocked() {
		t.Fatal("the advisory lock must be released once the run ends")
	}
}

func TestFailedRunReleasesLockToo(t *testing.T) {
	fastTiming(t)
	store := newMemStore()
	repo := &runRepo{store: store, err: errors.New("boom")}
	svc := services.NewOrganizationSyncService(repo, typedFetcher{}, nil, nil, services.WithRunStore(store))

	run, _ := svc.StartRun(context.Background(), services.SyncOptions{}, "")
	waitTerminal(t, svc, run.ID)
	deadline := time.Now().Add(time.Second)
	for store.isLocked() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if store.isLocked() {
		t.Fatal("a failed run must still release the advisory lock")
	}
}

func TestLockStateFailsClosedWhenActiveRunCannotBeRead(t *testing.T) {
	store := newMemStore()
	store.activeErr = errors.New("db down")
	svc := services.NewOrganizationSyncService(&runRepo{store: store}, typedFetcher{}, nil, nil, services.WithRunStore(store))
	if !svc.LockState(context.Background()).Syncing {
		t.Fatal("an unreadable run state must not unlock the guard")
	}
}

func TestOldCompletedRunsNeverLockTheGuard(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	// One run ends SUCCESS, the next ends FAILED; created one after the other
	// because only a single run may be active at a time.
	if _, created, err := store.CreatePending(ctx, orgsyncrun.NewParams{ID: "a"}); err != nil || !created {
		t.Fatalf("create a: %v %v", created, err)
	}
	_ = store.MarkRunning(ctx, "a", "w")
	if err := store.succeed("a"); err != nil {
		t.Fatal(err)
	}
	if _, created, err := store.CreatePending(ctx, orgsyncrun.NewParams{ID: "b"}); err != nil || !created {
		t.Fatalf("create b: %v %v", created, err)
	}
	_ = store.MarkRunning(ctx, "b", "w")
	if err := store.Fail(ctx, "b", orgsyncrun.Failure{Code: "X"}); err != nil {
		t.Fatal(err)
	}
	svc := services.NewOrganizationSyncService(&runRepo{store: store}, typedFetcher{}, nil, nil, services.WithRunStore(store))
	if svc.LockState(context.Background()).Locked() {
		t.Fatal("terminal runs must never keep the guard locked")
	}
}
