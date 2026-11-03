package services_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	"github.com/agopalakrishnan/teams360/backend/pkg/orgsnapshot"
)

// gatedRepo lets a test hold a sync inside ApplySnapshot (admitted, running)
// until it chooses to release it, then reports a mass-deletion hold.
type gatedRepo struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func newGatedRepo() *gatedRepo {
	return &gatedRepo{entered: make(chan struct{}), release: make(chan struct{})}
}

func (r *gatedRepo) KnownHierarchyLevelIDs(_ context.Context) (map[string]bool, error) {
	return knownLevels(), nil
}

func (r *gatedRepo) ApplySnapshot(_ context.Context, in orgprovider.ApplyInput) (*orgprovider.ApplyResult, error) {
	r.once.Do(func() { close(r.entered) })
	<-r.release
	return nil, &orgprovider.MassDeletionHoldError{Report: orgprovider.BuildMassDeletionReport(20, 6, 10, 3, in.MaxDeletePercent)}
}

// signalingFetcher reports when a sync has been admitted and reached the
// fetch, which happens strictly after admission.
type signalingFetcher struct {
	reached chan struct{}
	once    sync.Once
}

func (f *signalingFetcher) FetchSnapshot(ctx context.Context) (*orgsnapshot.Snapshot, error) {
	f.once.Do(func() { close(f.reached) })
	return thresholdFetcher{}.FetchSnapshot(ctx)
}

func waitFor(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func notYet(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
		t.Fatalf("%s happened too early", what)
	case <-time.After(150 * time.Millisecond):
	}
}

// A threshold write that has passed its lock check must finish before any sync
// can be admitted: otherwise a sync could start between the check and the
// write and have its threshold changed mid-run.
func TestUpdateMaxDeletePercent_SyncCannotBeAdmittedBetweenCheckAndWrite(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "")
	fetcher := &signalingFetcher{reached: make(chan struct{})}
	service := services.NewOrganizationSyncService(&recordingRepo{}, fetcher, nil, nil)

	writeEntered := make(chan struct{})
	releaseWrite := make(chan struct{})
	var written float64
	updateDone := make(chan error, 1)
	go func() {
		updateDone <- service.UpdateMaxDeletePercent(context.Background(), 35, func(_ context.Context, p float64) error {
			close(writeEntered) // the lock check has passed
			<-releaseWrite
			written = p
			return nil
		})
	}()
	waitFor(t, writeEntered, "the threshold write to start")

	syncDone := make(chan error, 1)
	go func() {
		_, err := service.Sync(context.Background())
		syncDone <- err
	}()

	// The sync must be unable to start while the write is mid-flight.
	notYet(t, fetcher.reached, "sync admission")

	close(releaseWrite)
	if err := <-updateDone; err != nil {
		t.Fatalf("update failed: %v", err)
	}
	waitFor(t, fetcher.reached, "the sync to be admitted after the write")
	if err := <-syncDone; err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if written != 35 {
		t.Fatalf("write saw %v, want 35", written)
	}
}

func TestUpdateMaxDeletePercent_RefusedWhileSyncRunningAndNeverWrites(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "")
	repo := newGatedRepo()
	service := services.NewOrganizationSyncService(repo, thresholdFetcher{}, nil, nil)

	syncDone := make(chan struct{})
	go func() {
		_, _ = service.Sync(context.Background())
		close(syncDone)
	}()
	waitFor(t, repo.entered, "the sync to be running")

	wrote := false
	err := service.UpdateMaxDeletePercent(context.Background(), 50, func(context.Context, float64) error {
		wrote = true
		return nil
	})

	var locked *services.ThresholdLockedError
	if !errors.As(err, &locked) || !locked.State.Syncing {
		t.Fatalf("want ThresholdLockedError{Syncing}, got %v", err)
	}
	if wrote {
		t.Fatal("a refused update must not write")
	}
	close(repo.release)
	<-syncDone
}

func TestUpdateMaxDeletePercent_RefusedWhileHeld(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "")
	service := services.NewOrganizationSyncService(&holdingRepo{}, thresholdFetcher{}, nil, nil)
	if _, err := service.Sync(context.Background()); err == nil {
		t.Fatal("expected the sync to be held")
	}

	err := service.UpdateMaxDeletePercent(context.Background(), 90, func(context.Context, float64) error {
		t.Fatal("must not write while held")
		return nil
	})

	var locked *services.ThresholdLockedError
	if !errors.As(err, &locked) || !locked.State.Held {
		t.Fatalf("want ThresholdLockedError{Held}, got %v", err)
	}
}

func TestUpdateMaxDeletePercent_WritesWhenIdleAndPropagatesWriteErrors(t *testing.T) {
	service := services.NewOrganizationSyncService(&recordingRepo{}, thresholdFetcher{}, nil, nil)

	var got float64
	if err := service.UpdateMaxDeletePercent(context.Background(), 12.5, func(_ context.Context, p float64) error {
		got = p
		return nil
	}); err != nil || got != 12.5 {
		t.Fatalf("err=%v got=%v, want nil/12.5", err, got)
	}

	boom := errors.New("db down")
	if err := service.UpdateMaxDeletePercent(context.Background(), 20, func(context.Context, float64) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("want the write error back, got %v", err)
	}
	// A failed write must not leave the service locked.
	if service.LockState(context.Background()).Locked() {
		t.Fatal("a failed write must not lock the service")
	}
}

// A dismissal decided while a sync is running must not clear the hold that
// sync is about to create.
func TestDismissHoldIfIdle_RefusedWhileSyncRunning_AndDoesNotClearTheNewHold(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "")
	repo := newGatedRepo()
	service := services.NewOrganizationSyncService(repo, thresholdFetcher{}, nil, nil)

	syncDone := make(chan struct{})
	go func() {
		_, _ = service.Sync(context.Background())
		close(syncDone)
	}()
	waitFor(t, repo.entered, "the sync to be running")

	cleared, err := service.DismissHoldIfIdle(context.Background())
	if !errors.Is(err, services.ErrSyncInProgress) || cleared {
		t.Fatalf("want ErrSyncInProgress and nothing cleared, got cleared=%v err=%v", cleared, err)
	}

	close(repo.release)
	<-syncDone

	if state := service.LockState(context.Background()); !state.Held {
		t.Fatalf("the hold the sync created must survive the refused dismissal, state=%+v", state)
	}
}

func TestDismissHoldIfIdle_ClearsAnExistingHoldAndReportsIt(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "")
	service := services.NewOrganizationSyncService(&holdingRepo{}, thresholdFetcher{}, nil, nil)
	if _, err := service.Sync(context.Background()); err == nil {
		t.Fatal("expected the sync to be held")
	}

	cleared, err := service.DismissHoldIfIdle(context.Background())
	if err != nil || !cleared {
		t.Fatalf("want cleared=true err=nil, got cleared=%v err=%v", cleared, err)
	}
	if service.LockState(context.Background()).Locked() {
		t.Fatal("dismissal must unlock")
	}
	if cleared, err = service.DismissHoldIfIdle(context.Background()); err != nil || cleared {
		t.Fatalf("a second dismissal has nothing to clear, got cleared=%v err=%v", cleared, err)
	}
}

// Hammers admission, threshold writes and dismissals together; with -race this
// fails if any of them touches shared state outside the one mutex.
func TestSyncAdmissionAndAdminOperations_AreRaceFree(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "")
	service := services.NewOrganizationSyncService(&holdingRepo{}, thresholdFetcher{}, nil, nil)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(3)
		go func() { defer wg.Done(); _, _ = service.Sync(context.Background()) }()
		go func() {
			defer wg.Done()
			_ = service.UpdateMaxDeletePercent(context.Background(), 25, func(context.Context, float64) error { return nil })
		}()
		go func() { defer wg.Done(); _, _ = service.DismissHoldIfIdle(context.Background()); _ = service.LockState(context.Background()) }()
	}
	wg.Wait()
}
