package services_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	"github.com/agopalakrishnan/teams360/backend/pkg/orgsnapshot"
)

// nonApplyingRepo fails the test if ApplySnapshot is ever invoked: these
// tests exist to prove an invalid snapshot never reaches reconciliation.
type nonApplyingRepo struct {
	applyCalls int
}

func (r *nonApplyingRepo) KnownHierarchyLevelIDs(_ context.Context) (map[string]bool, error) {
	return knownLevels(), nil
}

func (r *nonApplyingRepo) ApplySnapshot(_ context.Context, in orgprovider.ApplyInput) (*orgprovider.ApplyResult, error) {
	r.applyCalls++
	return &orgprovider.ApplyResult{UsersSynced: len(in.Snapshot.Users)}, nil
}

// fixedFetcher returns whatever snapshot a test supplies, letting each case
// control exactly what the provider "sent".
type fixedFetcher struct {
	snapshot *orgsnapshot.Snapshot
}

func (f fixedFetcher) FetchSnapshot(_ context.Context) (*orgsnapshot.Snapshot, error) {
	return f.snapshot, nil
}

func membershipTestSnapshot() *orgsnapshot.Snapshot {
	return &orgsnapshot.Snapshot{
		ContractVersion: orgsnapshot.ContractVersion,
		GeneratedAt:     time.Now().UTC(),
		Teams:           []orgsnapshot.Team{{ID: "t1", Name: "Team One"}},
		Users: []orgsnapshot.User{
			{ID: "u1", Username: "userone", DisplayName: "User One", Email: "u1@test.com", HierarchyLevelID: "level-3"},
		},
	}
}

func TestSyncRejectsNilMembershipsWithoutApplying(t *testing.T) {
	snap := membershipTestSnapshot()
	snap.Memberships = nil // omitted/null from the provider

	repo := &nonApplyingRepo{}
	service := services.NewOrganizationSyncService(repo, fixedFetcher{snapshot: snap}, nil, nil)

	_, err := service.Sync(context.Background())
	if !errors.Is(err, services.ErrInvalidSnapshot) {
		t.Fatalf("expected ErrInvalidSnapshot, got %v", err)
	}
	if repo.applyCalls != 0 {
		t.Fatalf("expected reconciliation to never run for an invalid snapshot, got %d calls", repo.applyCalls)
	}
}

func TestSyncAcceptsExplicitEmptyMemberships(t *testing.T) {
	snap := membershipTestSnapshot()
	snap.Memberships = []orgsnapshot.Membership{} // explicit "memberships": []

	repo := &nonApplyingRepo{}
	service := services.NewOrganizationSyncService(repo, fixedFetcher{snapshot: snap}, nil, nil)

	if _, err := service.Sync(context.Background()); err != nil {
		t.Fatalf("expected explicit empty memberships to be accepted: %v", err)
	}
	if repo.applyCalls != 1 {
		t.Fatalf("expected reconciliation to run exactly once, got %d calls", repo.applyCalls)
	}
}

func TestSyncAcceptsNonEmptyMemberships(t *testing.T) {
	snap := membershipTestSnapshot()
	snap.Memberships = []orgsnapshot.Membership{{UserID: "u1", TeamID: "t1"}}

	repo := &nonApplyingRepo{}
	service := services.NewOrganizationSyncService(repo, fixedFetcher{snapshot: snap}, nil, nil)

	if _, err := service.Sync(context.Background()); err != nil {
		t.Fatalf("expected a valid non-empty memberships snapshot to be accepted: %v", err)
	}
	if repo.applyCalls != 1 {
		t.Fatalf("expected reconciliation to run exactly once, got %d calls", repo.applyCalls)
	}
}
