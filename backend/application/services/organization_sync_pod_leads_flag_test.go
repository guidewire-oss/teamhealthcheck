package services_test

import (
	"context"
	"testing"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	"github.com/agopalakrishnan/teams360/backend/pkg/orgsnapshot"
)

// inputCapturingRepo records the ApplyInput the sync hands to persistence, so a
// test can see exactly what the repository will be told to do.
type inputCapturingRepo struct {
	levels  map[string]bool // nil means the default configured levels
	applied *orgprovider.ApplyInput
	result  orgprovider.ApplyResult // what ApplySnapshot reports back
}

func (r *inputCapturingRepo) KnownHierarchyLevelIDs(context.Context) (map[string]bool, error) {
	if r.levels != nil {
		return r.levels, nil
	}
	return knownLevels(), nil
}

func (r *inputCapturingRepo) ApplySnapshot(_ context.Context, in orgprovider.ApplyInput) (*orgprovider.ApplyResult, error) {
	r.applied = &in
	reported := r.result
	reported.UsersSynced = len(in.Snapshot.Users)
	return &reported, nil
}

func podLeadFlagSnapshot() *orgsnapshot.Snapshot {
	// A level-3 manager keeps the snapshot importable (non-empty) even in the
	// cases where the level-5 lead is skipped for an unknown level.
	return baseSnapshot(
		[]orgsnapshot.User{user("lead", "lead", "level-5", nil), user("mgr", "mgr", "level-3", nil)},
		[]orgsnapshot.Team{{ID: "t1", Name: "Goa", TeamLeadID: strPtr("lead")}},
		[]orgsnapshot.Membership{{UserID: "lead", TeamID: "t1"}, {UserID: "mgr", TeamID: "t1"}},
	)
}

func syncAndCaptureInput(t *testing.T, levels map[string]bool) orgprovider.ApplyInput {
	t.Helper()
	repo := &inputCapturingRepo{levels: levels}
	service := services.NewOrganizationSyncService(repo, fixedFetcher{snapshot: podLeadFlagSnapshot()}, nil, nil)
	if _, err := service.Sync(context.Background()); err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if repo.applied == nil {
		t.Fatal("reconciliation never ran")
	}
	return *repo.applied
}

func TestSyncTurnsOnThePodLeadMappingWhenBothLevelsAreConfigured(t *testing.T) {
	in := syncAndCaptureInput(t, nil)

	if !in.MapPodLeads {
		t.Error("MapPodLeads = false, want true when level-4 and level-5 are configured")
	}
}

func TestSyncTurnsOffThePodLeadMappingWhenALevelIsMissing(t *testing.T) {
	for _, missing := range []string{orgprovider.PodLeadLevelID, orgprovider.PodLeadSourceLevelID} {
		t.Run("without "+missing, func(t *testing.T) {
			levels := knownLevels()
			delete(levels, missing)

			in := syncAndCaptureInput(t, levels)

			if in.MapPodLeads {
				t.Errorf("MapPodLeads = true, want false when %s is not configured", missing)
			}
		})
	}
}

func TestSyncDoesNotEditTheLevelsItHandsToPersistence(t *testing.T) {
	// The mapping happens in the database transaction, so the snapshot the
	// service passes on must carry the provider's levels untouched.
	in := syncAndCaptureInput(t, nil)

	var lead *orgsnapshot.User
	for i := range in.Snapshot.Users {
		if in.Snapshot.Users[i].ID == "lead" {
			lead = &in.Snapshot.Users[i]
		}
	}
	if lead == nil {
		t.Fatal("the lead is missing from the snapshot handed to persistence, so its level cannot be checked")
	}
	if lead.HierarchyLevelID != "level-5" {
		t.Errorf("lead level handed to persistence = %q, want the provider's level-5", lead.HierarchyLevelID)
	}
}

func TestSyncReportsThePodLeadCountsPersistenceReturned(t *testing.T) {
	repo := &inputCapturingRepo{result: orgprovider.ApplyResult{PodLeadsPromoted: 3, PodLeadsReturned: 2}}
	service := services.NewOrganizationSyncService(repo, fixedFetcher{snapshot: podLeadFlagSnapshot()}, nil, nil)

	result, err := service.Sync(context.Background())
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	if result.PodLeadsPromoted != 3 || result.PodLeadsReturned != 2 {
		t.Errorf("pod lead counts = %d promoted, %d returned; want 3 and 2", result.PodLeadsPromoted, result.PodLeadsReturned)
	}
}
