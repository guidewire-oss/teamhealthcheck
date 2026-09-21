package services_test

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	"github.com/agopalakrishnan/teams360/backend/pkg/orgsnapshot"
)

// thresholdStore stands in for the settings repository: it reports whatever
// an administrator is supposed to have saved, or a read failure.
type thresholdStore struct {
	saved *float64
	err   error
}

func (s *thresholdStore) GetOrgSyncMaxDeletePercent(_ context.Context) (*float64, error) {
	return s.saved, s.err
}

// recordingRepo captures the ApplyInput the sync built, which is how these
// tests observe the threshold that actually reached the guard.
type recordingRepo struct {
	lastInput orgprovider.ApplyInput
}

func (r *recordingRepo) KnownHierarchyLevelIDs(_ context.Context) (map[string]bool, error) {
	return knownLevels(), nil
}

func (r *recordingRepo) ApplySnapshot(_ context.Context, in orgprovider.ApplyInput) (*orgprovider.ApplyResult, error) {
	r.lastInput = in
	return &orgprovider.ApplyResult{UsersSynced: len(in.Snapshot.Users)}, nil
}

// thresholdFetcher returns a minimal valid snapshot: these tests are about the
// threshold, not about snapshot contents.
type thresholdFetcher struct{}

func (thresholdFetcher) FetchSnapshot(_ context.Context) (*orgsnapshot.Snapshot, error) {
	return &orgsnapshot.Snapshot{
		ContractVersion: "1.0",
		GeneratedAt:     time.Now().UTC(),
		Teams:           []orgsnapshot.Team{{ID: "t1", Name: "Team One"}},
		Users: []orgsnapshot.User{
			{ID: "u1", Username: "userone", DisplayName: "User One", Email: "u1@test.com", HierarchyLevelID: "level-3"},
		},
		Memberships: []orgsnapshot.Membership{{UserID: "u1", TeamID: "t1"}},
	}, nil
}

func floatPtr(f float64) *float64 { return &f }

func TestResolveMaxDeletePercentDefaultsTo20(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "")

	value, source, err := services.ResolveMaxDeletePercent(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != 20 {
		t.Fatalf("expected the 20%% default, got %v", value)
	}
	if source != services.ThresholdSourceDefault {
		t.Fatalf("expected source %q, got %q", services.ThresholdSourceDefault, source)
	}
}

func TestResolveMaxDeletePercentFallsBackToEnvironment(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "35")

	value, source, err := services.ResolveMaxDeletePercent(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != 35 {
		t.Fatalf("expected the environment value 35, got %v", value)
	}
	if source != services.ThresholdSourceEnvironment {
		t.Fatalf("expected source %q, got %q", services.ThresholdSourceEnvironment, source)
	}
}

func TestResolveMaxDeletePercentPrefersSavedSetting(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "35")

	value, source, err := services.ResolveMaxDeletePercent(floatPtr(5))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != 5 {
		t.Fatalf("expected the saved value 5 to beat the environment, got %v", value)
	}
	if source != services.ThresholdSourceAdmin {
		t.Fatalf("expected source %q, got %q", services.ThresholdSourceAdmin, source)
	}
}

func TestResolveMaxDeletePercentIgnoresUnusableEnvironmentValue(t *testing.T) {
	for _, raw := range []string{"", "abc", "0", "-5", "NaN", "Inf"} {
		t.Setenv(services.EnvMaxDeletePercent, raw)

		value, source, err := services.ResolveMaxDeletePercent(nil)
		if err != nil {
			t.Fatalf("%q: unexpected error: %v", raw, err)
		}
		if value != services.DefaultMaxDeletePercent || source != services.ThresholdSourceDefault {
			t.Fatalf("%q: expected the built-in default, got %v from %q", raw, value, source)
		}
	}
}

func TestResolveMaxDeletePercentRejectsUnusableSavedValue(t *testing.T) {
	for _, saved := range []float64{0, 0.5, -1, 100.5, 101, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, _, err := services.ResolveMaxDeletePercent(floatPtr(saved)); !errors.Is(err, services.ErrMaxDeletePercentNotConfigured) {
			t.Fatalf("saved %v: expected ErrMaxDeletePercentNotConfigured, got %v", saved, err)
		}
	}
}

func TestResolveMaxDeletePercentAcceptsRangeBoundaries(t *testing.T) {
	for _, saved := range []float64{1, 50, 100} {
		value, source, err := services.ResolveMaxDeletePercent(floatPtr(saved))
		if err != nil {
			t.Fatalf("saved %v: unexpected error: %v", saved, err)
		}
		if value != saved || source != services.ThresholdSourceAdmin {
			t.Fatalf("saved %v: got %v from %q", saved, value, source)
		}
	}
}

func TestSyncUsesSavedThresholdOverEnvironment(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "80")

	repo := &recordingRepo{}
	service := services.NewOrganizationSyncService(repo, thresholdFetcher{}, nil, nil,
		services.WithDeleteThresholdStore(&thresholdStore{saved: floatPtr(7)}))

	if _, err := service.Sync(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastInput.MaxDeletePercent != 7 {
		t.Fatalf("expected the saved 7%% to reach the guard, got %v", repo.lastInput.MaxDeletePercent)
	}
}

func TestSyncUsesDefaultWhenNothingIsConfigured(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "")

	repo := &recordingRepo{}
	service := services.NewOrganizationSyncService(repo, thresholdFetcher{}, nil, nil,
		services.WithDeleteThresholdStore(&thresholdStore{}))

	if _, err := service.Sync(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastInput.MaxDeletePercent != services.DefaultMaxDeletePercent {
		t.Fatalf("expected the 20%% default to reach the guard, got %v", repo.lastInput.MaxDeletePercent)
	}
}

func TestSyncFailsClosedWhenTheStoredThresholdIsInvalid(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "20")

	repo := &recordingRepo{}
	service := services.NewOrganizationSyncService(repo, thresholdFetcher{}, nil, nil,
		services.WithDeleteThresholdStore(&thresholdStore{saved: floatPtr(150)}))

	_, err := service.Sync(context.Background())
	if !errors.Is(err, services.ErrMaxDeletePercentNotConfigured) {
		t.Fatalf("expected ErrMaxDeletePercentNotConfigured, got %v", err)
	}
	if repo.lastInput.MaxDeletePercent != 0 {
		t.Fatal("no snapshot should have been applied")
	}
}

func TestSyncFailsWhenTheThresholdCannotBeRead(t *testing.T) {
	repo := &recordingRepo{}
	readErr := errors.New("settings unavailable")
	service := services.NewOrganizationSyncService(repo, thresholdFetcher{}, nil, nil,
		services.WithDeleteThresholdStore(&thresholdStore{err: readErr}))

	_, err := service.Sync(context.Background())
	if !errors.Is(err, readErr) {
		t.Fatalf("expected the store error to surface, got %v", err)
	}
	if repo.lastInput.MaxDeletePercent != 0 {
		t.Fatal("no snapshot should have been applied")
	}
}

// holdingRepo trips the mass-deletion guard unless an attempt explicitly
// overrides it, mirroring the real repository's contract closely enough to
// exercise the service's hold bookkeeping.
type holdingRepo struct {
	lastInput orgprovider.ApplyInput
	calls     int
	// stopHolding makes the repository accept the next attempt, standing in
	// for provider data that was fixed upstream.
	stopHolding bool
}

func (r *holdingRepo) KnownHierarchyLevelIDs(_ context.Context) (map[string]bool, error) {
	return knownLevels(), nil
}

func (r *holdingRepo) ApplySnapshot(_ context.Context, in orgprovider.ApplyInput) (*orgprovider.ApplyResult, error) {
	r.calls++
	r.lastInput = in

	report := orgprovider.BuildMassDeletionReport(20, 6, 10, 3, in.MaxDeletePercent)
	if r.stopHolding {
		return &orgprovider.ApplyResult{UsersSynced: len(in.Snapshot.Users)}, nil
	}
	if in.OverrideMassDeletion && report.MatchesConfirmed(in.ConfirmedMassDeletion) {
		return &orgprovider.ApplyResult{UsersDeleted: report.Users.Deleting, MassDeletionOverride: &report}, nil
	}
	if report.Held() {
		return nil, &orgprovider.MassDeletionHoldError{Report: report}
	}
	return &orgprovider.ApplyResult{UsersSynced: len(in.Snapshot.Users)}, nil
}

func confirmedFor(report orgprovider.MassDeletionReport) *orgprovider.ConfirmedMassDeletion {
	return &orgprovider.ConfirmedMassDeletion{
		UsersExisting: report.Users.Existing,
		UsersDeleting: report.Users.Deleting,
		TeamsExisting: report.Teams.Existing,
		TeamsDeleting: report.Teams.Deleting,
	}
}

func TestHoldFreezesTheThresholdForLaterAttempts(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "")

	// The store reports 20% first and 100% afterwards: the sort of change the
	// lock exists to make irrelevant.
	store := &shiftingStore{values: []float64{20, 100, 100}}
	repo := &holdingRepo{}
	service := services.NewOrganizationSyncService(repo, thresholdFetcher{}, nil, nil,
		services.WithDeleteThresholdStore(store))

	if _, err := service.Sync(context.Background()); !errors.Is(err, orgprovider.ErrMassDeletionBlocked) {
		t.Fatalf("expected the first attempt to be held, got %v", err)
	}

	state := service.LockState()
	if !state.Held || state.Threshold != 20 {
		t.Fatalf("expected a hold frozen at 20%%, got %+v", state)
	}
	if !state.Locked() {
		t.Fatal("a held sync must lock the threshold")
	}

	if _, err := service.Sync(context.Background()); !errors.Is(err, orgprovider.ErrMassDeletionBlocked) {
		t.Fatalf("expected the retry to be held too, got %v", err)
	}
	if repo.lastInput.MaxDeletePercent != 20 {
		t.Fatalf("the retry must reuse the held 20%%, got %v", repo.lastInput.MaxDeletePercent)
	}
}

func TestOverrideResolvesTheHoldAndUnlocks(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "")

	repo := &holdingRepo{}
	service := services.NewOrganizationSyncService(repo, thresholdFetcher{}, nil, nil,
		services.WithDeleteThresholdStore(&thresholdStore{saved: floatPtr(20)}))

	_, err := service.Sync(context.Background())
	var hold *orgprovider.MassDeletionHoldError
	if !errors.As(err, &hold) {
		t.Fatalf("expected a hold, got %v", err)
	}
	if !service.LockState().Locked() {
		t.Fatal("expected the threshold to be locked")
	}

	result, err := service.SyncWithOptions(context.Background(), services.SyncOptions{
		OverrideMassDeletion:  true,
		ConfirmedMassDeletion: confirmedFor(hold.Report),
		ActorUserID:           "admin",
	})
	if err != nil {
		t.Fatalf("the explicit override should apply: %v", err)
	}
	if !result.MassDeletionOverridden {
		t.Fatal("expected the result to record the override")
	}
	if service.LockState().Locked() {
		t.Fatal("a resolved hold must unlock the threshold")
	}
}

func TestCleanSyncResolvesAPriorHold(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "")

	repo := &holdingRepo{}
	service := services.NewOrganizationSyncService(repo, thresholdFetcher{}, nil, nil,
		services.WithDeleteThresholdStore(&thresholdStore{saved: floatPtr(20)}))

	if _, err := service.Sync(context.Background()); err == nil {
		t.Fatal("expected the first attempt to be held")
	}

	// The provider data is fixed upstream and the next sync applies normally.
	repo.stopHolding = true
	if _, err := service.Sync(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if service.LockState().Locked() {
		t.Fatal("a completed sync must unlock the threshold")
	}
}

func TestDismissHoldUnlocksWithoutApplying(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "")

	repo := &holdingRepo{}
	service := services.NewOrganizationSyncService(repo, thresholdFetcher{}, nil, nil,
		services.WithDeleteThresholdStore(&thresholdStore{saved: floatPtr(20)}))

	if _, err := service.Sync(context.Background()); err == nil {
		t.Fatal("expected the first attempt to be held")
	}
	callsWhenHeld := repo.calls

	if !service.DismissHold() {
		t.Fatal("expected a hold to dismiss")
	}
	if service.LockState().Locked() {
		t.Fatal("dismissing must unlock the threshold")
	}
	if repo.calls != callsWhenHeld {
		t.Fatal("dismissing must not apply anything")
	}
	if service.DismissHold() {
		t.Fatal("a second dismissal has nothing to clear")
	}
}

func TestThresholdIsResolvedFreshOnceNoHoldRemains(t *testing.T) {
	t.Setenv(services.EnvMaxDeletePercent, "")

	store := &shiftingStore{values: []float64{20, 55}}
	repo := &holdingRepo{}
	service := services.NewOrganizationSyncService(repo, thresholdFetcher{}, nil, nil,
		services.WithDeleteThresholdStore(store))

	if _, err := service.Sync(context.Background()); err == nil {
		t.Fatal("expected the first attempt to be held")
	}
	service.DismissHold()

	repo.stopHolding = true
	if _, err := service.Sync(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastInput.MaxDeletePercent != 55 {
		t.Fatalf("once unlocked, the current setting applies again; got %v", repo.lastInput.MaxDeletePercent)
	}
}

// shiftingStore returns the next value in a list on each read, repeating the
// last one, so a test can model a setting that changes between attempts.
type shiftingStore struct {
	values []float64
	reads  int
}

func (s *shiftingStore) GetOrgSyncMaxDeletePercent(_ context.Context) (*float64, error) {
	index := s.reads
	if index >= len(s.values) {
		index = len(s.values) - 1
	}
	s.reads++
	value := s.values[index]
	return &value, nil
}
