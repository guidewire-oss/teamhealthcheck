package services_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/orgsyncrun"
	"github.com/agopalakrishnan/teams360/backend/domain/orgsyncschedule"
)

// LatestByTrigger / LatestSuccessByTrigger make memStore an orgsyncschedule.RunHistory.
func (m *memStore) newestMatching(t orgsyncrun.Trigger, successOnly bool) *orgsyncrun.Run {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.order) - 1; i >= 0; i-- {
		r := m.runs[m.order[i]]
		if r.Trigger == t && (!successOnly || r.Status == orgsyncrun.StatusSuccess) {
			c := *r
			return &c
		}
	}
	return nil
}
func (m *memStore) LatestByTrigger(_ context.Context, t orgsyncrun.Trigger) (*orgsyncrun.Run, error) {
	return m.newestMatching(t, false), nil
}
func (m *memStore) LatestSuccessByTrigger(_ context.Context, t orgsyncrun.Trigger) (*orgsyncrun.Run, error) {
	return m.newestMatching(t, true), nil
}

// memSchedule enforces the same compare-and-swap rule as the SQL store.
type memSchedule struct {
	mu sync.Mutex
	s  orgsyncschedule.Schedule
	// beforeClaim runs just before Claim takes effect (simulates a concurrent edit).
	beforeClaim func()
}

func newMemSchedule() *memSchedule {
	return &memSchedule{s: orgsyncschedule.Schedule{Frequency: orgsyncschedule.Daily}}
}
func (m *memSchedule) Get(context.Context) (*orgsyncschedule.Schedule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.s
	return &c, nil
}
func (m *memSchedule) Save(_ context.Context, enabled bool, f orgsyncschedule.Frequency, next *time.Time, _ string) (*orgsyncschedule.Schedule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.s.Enabled, m.s.Frequency = enabled, f
	m.s.NextRunAt = nil
	if enabled {
		m.s.NextRunAt = next
	}
	c := m.s
	return &c, nil
}
func (m *memSchedule) Claim(_ context.Context, due, next time.Time) (bool, error) {
	if m.beforeClaim != nil {
		m.beforeClaim()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.s.Enabled || m.s.NextRunAt == nil || !m.s.NextRunAt.Equal(due) {
		return false, nil
	}
	m.s.NextRunAt = &next
	return true, nil
}

// fakeStarter records scheduled starts without running a sync.
type fakeStarter struct {
	calls    int32
	keys     []string
	triggers []orgsyncrun.Trigger
	err      error
	unconfig bool
	mu       sync.Mutex
}

func (f *fakeStarter) Configured() bool { return !f.unconfig }
func (f *fakeStarter) StartRun(_ context.Context, o services.SyncOptions, key string) (*orgsyncrun.Run, error) {
	atomic.AddInt32(&f.calls, 1)
	f.mu.Lock()
	f.keys, f.triggers = append(f.keys, key), append(f.triggers, o.Trigger)
	f.mu.Unlock()
	return &orgsyncrun.Run{ID: "r"}, f.err
}

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func clock(t *time.Time) func() time.Time { return func() time.Time { return *t } }

func newSched(now *time.Time, st *fakeStarter) (*services.OrganizationSyncScheduler, *memSchedule, *memStore) {
	ms, runs := newMemSchedule(), newMemStore()
	return services.NewOrganizationSyncScheduler(ms, runs, st).WithClock(clock(now)), ms, runs
}

func TestAutoSyncDefaultsToDisabledWithNoHistory(t *testing.T) {
	now := at("2026-11-04T10:00:00Z")
	sc, _, _ := newSched(&now, &fakeStarter{})
	st, err := sc.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Enabled || st.NextRunAt != nil || st.LastAttempt != nil || st.LastSuccessAt != nil || st.Timezone != "UTC" {
		t.Fatalf("unexpected default status: %+v", st)
	}
}

func TestEnablingComputesNextAndDoesNotStartASync(t *testing.T) {
	now := at("2026-11-04T10:00:00Z") // Wednesday
	starter := &fakeStarter{}
	sc, _, _ := newSched(&now, starter)
	st, err := sc.Update(context.Background(), true, "weekly", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if !st.Enabled || st.Frequency != orgsyncschedule.Weekly || st.NextRunAt == nil || !st.NextRunAt.Equal(at("2026-11-09T02:00:00Z")) {
		t.Fatalf("unexpected status: %+v", st)
	}
	sc.RunDue(context.Background())
	if starter.calls != 0 {
		t.Fatal("saving must not trigger a sync")
	}
}

func TestChangingFrequencyRecalculatesAndDisablingClearsNext(t *testing.T) {
	now := at("2026-11-04T10:00:00Z")
	sc, _, _ := newSched(&now, &fakeStarter{})
	ctx := context.Background()
	if _, err := sc.Update(ctx, true, "DAILY", "a"); err != nil {
		t.Fatal(err)
	}
	st, _ := sc.Update(ctx, true, "MONTHLY", "a")
	if !st.NextRunAt.Equal(at("2026-12-01T02:00:00Z")) {
		t.Fatalf("monthly next = %v", st.NextRunAt)
	}
	st, _ = sc.Update(ctx, false, "", "a")
	if st.Enabled || st.NextRunAt != nil || st.Frequency != orgsyncschedule.Monthly {
		t.Fatalf("disable should clear next and keep frequency: %+v", st)
	}
}

func TestInvalidFrequencyRejectedAndNothingSaved(t *testing.T) {
	now := at("2026-11-04T10:00:00Z")
	sc, ms, _ := newSched(&now, &fakeStarter{})
	for _, bad := range []string{"hourly", "every 30 days", "DAILY;DROP", "0 2 * * *"} {
		if _, err := sc.Update(context.Background(), true, bad, "a"); !errors.Is(err, services.ErrInvalidSchedule) {
			t.Errorf("%q: want ErrInvalidSchedule, got %v", bad, err)
		}
	}
	if s, _ := ms.Get(context.Background()); s.Enabled {
		t.Fatal("a rejected update must not change the schedule")
	}
}

func TestDueOccurrenceStartsScheduledRunExactlyOnceAcrossReplicas(t *testing.T) {
	now := at("2026-11-04T10:00:00Z")
	starter := &fakeStarter{}
	ms, runs := newMemSchedule(), newMemStore()
	mk := func() *services.OrganizationSyncScheduler {
		return services.NewOrganizationSyncScheduler(ms, runs, starter).WithClock(clock(&now))
	}
	if _, err := mk().Update(context.Background(), true, "DAILY", "a"); err != nil {
		t.Fatal(err)
	}
	now = at("2026-11-05T02:00:30Z") // just after the occurrence
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); mk().RunDue(context.Background()) }()
	}
	wg.Wait()
	if starter.calls != 1 {
		t.Fatalf("occurrence must fire once across replicas, fired %d times", starter.calls)
	}
	if starter.triggers[0] != orgsyncrun.TriggerScheduled || starter.keys[0] != "scheduled:2026-11-05T02:00:00Z" {
		t.Fatalf("bad start: %v %v", starter.triggers, starter.keys)
	}
	s, _ := ms.Get(context.Background())
	if !s.NextRunAt.Equal(at("2026-11-06T02:00:00Z")) {
		t.Fatalf("next must advance to the next future occurrence, got %v", s.NextRunAt)
	}
}

func TestDisabledAfterReadCannotLaunchRun(t *testing.T) {
	now := at("2026-11-04T10:00:00Z")
	starter := &fakeStarter{}
	sc, ms, _ := newSched(&now, starter)
	_, _ = sc.Update(context.Background(), true, "DAILY", "a")
	now = at("2026-11-05T02:00:10Z")
	ms.beforeClaim = func() { // admin disables between the read and the claim
		ms.mu.Lock()
		ms.s.Enabled, ms.s.NextRunAt = false, nil
		ms.mu.Unlock()
	}
	sc.RunDue(context.Background())
	if starter.calls != 0 {
		t.Fatal("a stale registration must not launch a run after disable")
	}
}

func TestNotDueAndDisabledDoNothing(t *testing.T) {
	now := at("2026-11-04T10:00:00Z")
	starter := &fakeStarter{}
	sc, _, _ := newSched(&now, starter)
	sc.RunDue(context.Background()) // disabled
	_, _ = sc.Update(context.Background(), true, "DAILY", "a")
	now = at("2026-11-05T01:59:59Z")
	sc.RunDue(context.Background()) // not yet due
	if starter.calls != 0 {
		t.Fatal("nothing should start")
	}
}

func TestMissedOccurrenceIsSkippedNotReplayed(t *testing.T) {
	now := at("2026-11-04T10:00:00Z")
	starter := &fakeStarter{}
	sc, ms, _ := newSched(&now, starter)
	_, _ = sc.Update(context.Background(), true, "DAILY", "a")
	now = at("2026-11-08T09:00:00Z") // service was down for days
	sc.RunDue(context.Background())
	if starter.calls != 0 {
		t.Fatal("missed occurrences must not be replayed")
	}
	s, _ := ms.Get(context.Background())
	if !s.NextRunAt.Equal(at("2026-11-09T02:00:00Z")) {
		t.Fatalf("must schedule the next future run, got %v", s.NextRunAt)
	}
	sc.RunDue(context.Background())
	if starter.calls != 0 {
		t.Fatal("no backlog on the next poll either")
	}
}

func TestRestartWithinGraceStillFiresJustMissedOccurrence(t *testing.T) {
	now := at("2026-11-04T10:00:00Z")
	starter := &fakeStarter{}
	sc, _, _ := newSched(&now, starter)
	_, _ = sc.Update(context.Background(), true, "DAILY", "a")
	now = at("2026-11-05T02:05:00Z") // restarted 5 minutes late
	sc.RunDue(context.Background())
	if starter.calls != 1 {
		t.Fatalf("want 1 start, got %d", starter.calls)
	}
}

func TestFailedOrBlockedStartDoesNotStopLaterOccurrences(t *testing.T) {
	now := at("2026-11-04T10:00:00Z")
	starter := &fakeStarter{err: orgsyncrun.ErrActiveRunExists} // a manual sync is running
	sc, ms, _ := newSched(&now, starter)
	_, _ = sc.Update(context.Background(), true, "DAILY", "a")
	now = at("2026-11-05T02:00:05Z")
	sc.RunDue(context.Background())
	s, _ := ms.Get(context.Background())
	if !s.Enabled || !s.NextRunAt.Equal(at("2026-11-06T02:00:00Z")) {
		t.Fatalf("schedule must stay enabled and advance: %+v", s)
	}
	starter.err = errors.New("boom")
	now = at("2026-11-06T02:00:05Z")
	sc.RunDue(context.Background())
	starter.err = nil
	now = at("2026-11-07T02:00:05Z")
	sc.RunDue(context.Background())
	if starter.calls != 3 {
		t.Fatalf("each occurrence should have been attempted, got %d", starter.calls)
	}
}

func TestUnconfiguredProviderDoesNotBurnTheOccurrence(t *testing.T) {
	now := at("2026-11-04T10:00:00Z")
	starter := &fakeStarter{unconfig: true}
	sc, ms, _ := newSched(&now, starter)
	_, _ = sc.Update(context.Background(), true, "DAILY", "a")
	now = at("2026-11-05T02:00:05Z")
	sc.RunDue(context.Background())
	s, _ := ms.Get(context.Background())
	if starter.calls != 0 || !s.NextRunAt.Equal(at("2026-11-05T02:00:00Z")) {
		t.Fatalf("occurrence must stay pending: calls=%d next=%v", starter.calls, s.NextRunAt)
	}
}

// End to end through the real service: scheduled and manual runs share the
// upsert path and the single-active guard, and are tracked separately.
func TestScheduledRunUsesSharedUpsertAndIsTrackedSeparately(t *testing.T) {
	fastTiming(t)
	runs := newMemStore()
	repo := &runRepo{store: runs}
	svc := services.NewOrganizationSyncService(repo, typedFetcher{}, nil, nil, services.WithRunStore(runs))
	now := at("2026-11-04T10:00:00Z")
	sc := services.NewOrganizationSyncScheduler(newMemSchedule(), runs, svc).WithClock(clock(&now))
	ctx := context.Background()
	_, _ = sc.Update(ctx, true, "DAILY", "a")

	// A manual run first: it must not show up as an automatic attempt.
	manual, err := svc.StartRun(ctx, services.SyncOptions{ActorUserID: "admin"}, "")
	if err != nil {
		t.Fatal(err)
	}
	waitTerminal(t, svc, manual.ID)
	if st, _ := sc.Status(ctx); st.LastAttempt != nil || st.LastSuccessAt != nil {
		t.Fatalf("manual sync must not touch auto-sync status: %+v", st)
	}

	now = at("2026-11-05T02:00:05Z")
	sc.RunDue(ctx)
	st, _ := sc.Status(ctx)
	if st.LastAttempt == nil {
		t.Fatal("scheduled attempt not recorded")
	}
	done := waitTerminal(t, svc, st.LastAttempt.RunID)
	if done.Status != orgsyncrun.StatusSuccess || done.Trigger != orgsyncrun.TriggerScheduled {
		t.Fatalf("want scheduled SUCCESS, got %s/%s", done.Status, done.Trigger)
	}
	if repo.last.RunID != done.ID {
		t.Fatal("scheduled run must go through ApplySnapshot like a manual run")
	}
	if st, _ = sc.Status(ctx); st.LastSuccessAt == nil || st.LastAttempt.Status != orgsyncrun.StatusSuccess {
		t.Fatalf("success not reflected: %+v", st)
	}
}

func TestScheduledRunCannotOverlapActiveManualRun(t *testing.T) {
	fastTiming(t)
	runs := newMemStore()
	repo := &runRepo{store: runs, gate: make(chan struct{}), seen: make(chan struct{})}
	svc := services.NewOrganizationSyncService(repo, typedFetcher{}, nil, nil, services.WithRunStore(runs))
	now := at("2026-11-04T10:00:00Z")
	ms := newMemSchedule()
	sc := services.NewOrganizationSyncScheduler(ms, runs, svc).WithClock(clock(&now))
	ctx := context.Background()
	_, _ = sc.Update(ctx, true, "DAILY", "a")

	if _, err := svc.StartRun(ctx, services.SyncOptions{ActorUserID: "admin"}, ""); err != nil {
		t.Fatal(err)
	}
	<-repo.seen
	now = at("2026-11-05T02:00:05Z")
	sc.RunDue(ctx)
	if r, _ := runs.LatestByTrigger(ctx, orgsyncrun.TriggerScheduled); r != nil {
		t.Fatal("a scheduled run must not start while a manual run is active")
	}
	if s, _ := ms.Get(ctx); !s.NextRunAt.Equal(at("2026-11-06T02:00:00Z")) {
		t.Fatalf("schedule must still advance, got %v", s.NextRunAt)
	}
	close(repo.gate)
}

func TestFailedAutoRunKeepsLastSuccessAndShowsFailure(t *testing.T) {
	fastTiming(t)
	runs := newMemStore()
	repo := &runRepo{store: runs}
	svc := services.NewOrganizationSyncService(repo, typedFetcher{}, nil, nil, services.WithRunStore(runs))
	now := at("2026-11-04T10:00:00Z")
	sc := services.NewOrganizationSyncScheduler(newMemSchedule(), runs, svc).WithClock(clock(&now))
	ctx := context.Background()
	_, _ = sc.Update(ctx, true, "DAILY", "a")

	now = at("2026-11-05T02:00:05Z")
	sc.RunDue(ctx)
	first, _ := sc.Status(ctx)
	waitTerminal(t, svc, first.LastAttempt.RunID)
	good, _ := sc.Status(ctx)
	if good.LastSuccessAt == nil {
		t.Fatal("first run should have succeeded")
	}

	repo.err = errors.New("pq: password=hunter2 connection refused")
	now = at("2026-11-06T02:00:05Z")
	sc.RunDue(ctx)
	bad, _ := sc.Status(ctx)
	waitTerminal(t, svc, bad.LastAttempt.RunID)
	bad, _ = sc.Status(ctx)
	if bad.LastAttempt.Status != orgsyncrun.StatusFailed {
		t.Fatalf("want FAILED, got %s", bad.LastAttempt.Status)
	}
	if bad.LastSuccessAt == nil || !bad.LastSuccessAt.Equal(*good.LastSuccessAt) {
		t.Fatal("a failed attempt must not advance last success")
	}
	if m := bad.LastAttempt.ErrorMessage; m == nil || contains(*m, "hunter2") {
		t.Fatalf("error summary must be safe, got %v", m)
	}
	if !bad.Enabled || bad.NextRunAt == nil {
		t.Fatal("failure must not stop future scheduling")
	}
}
