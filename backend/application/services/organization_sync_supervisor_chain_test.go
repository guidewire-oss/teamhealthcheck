package services_test

import (
	"context"
	"testing"
	"time"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/team"
	domainuser "github.com/agopalakrishnan/teams360/backend/domain/user"
	"github.com/agopalakrishnan/teams360/backend/pkg/orgsnapshot"
)

// fakeTeamRepo implements only the team.Repository methods this test needs;
// the embedded nil interface satisfies the rest at compile time and would
// panic if the sync path ever called them, which it doesn't here.
type fakeTeamRepo struct {
	team.Repository
	stored               map[string]*team.Team
	updatedChains        map[string][]*team.SupervisorLink
	updateChainCallCount map[string]int
}

func (r *fakeTeamRepo) FindByID(_ context.Context, id string) (*team.Team, error) {
	return r.stored[id], nil
}

func (r *fakeTeamRepo) UpdateSupervisorChain(_ context.Context, teamID string, chain []*team.SupervisorLink) error {
	if r.updatedChains == nil {
		r.updatedChains = map[string][]*team.SupervisorLink{}
	}
	if r.updateChainCallCount == nil {
		r.updateChainCallCount = map[string]int{}
	}
	r.updatedChains[teamID] = chain
	r.updateChainCallCount[teamID]++
	return nil
}

// fakeUserRepo implements only FindSupervisorChainUp; see fakeTeamRepo.
type fakeUserRepo struct {
	domainuser.Repository
	chains map[string][]*domainuser.User
}

func (r *fakeUserRepo) FindSupervisorChainUp(_ context.Context, userID string) ([]*domainuser.User, error) {
	return r.chains[userID], nil
}

// chainFetcher returns a snapshot with a single team so the test can drive
// rederiveSupervisorChains through Sync without depending on its contents.
type chainFetcher struct{}

func (chainFetcher) FetchSnapshot(_ context.Context) (*orgsnapshot.Snapshot, error) {
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

func TestRederiveSupervisorChainsClearsChainWhenTeamLeadRemoved(t *testing.T) {
	teamRepo := &fakeTeamRepo{
		stored: map[string]*team.Team{
			"t1": {ID: "t1", TeamLeadID: nil}, // the FK already cleared the lead
		},
	}
	userRepo := &fakeUserRepo{}

	repo := &recordingRepo{}
	service := services.NewOrganizationSyncService(repo, chainFetcher{}, userRepo, teamRepo)

	if _, err := service.Sync(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	chain, ok := teamRepo.updatedChains["t1"]
	if !ok {
		t.Fatalf("expected UpdateSupervisorChain to be called for team t1 with a removed lead")
	}
	if len(chain) != 0 {
		t.Fatalf("expected the supervisor chain to be cleared, got %v", chain)
	}
}

func TestRederiveSupervisorChainsLeavesUnchangedLeadIntact(t *testing.T) {
	leadID := "lead1"
	teamRepo := &fakeTeamRepo{
		stored: map[string]*team.Team{
			"t1": {ID: "t1", TeamLeadID: &leadID},
		},
	}
	userRepo := &fakeUserRepo{
		chains: map[string][]*domainuser.User{
			leadID: {{ID: "mgr1", HierarchyLevelID: "level-2"}},
		},
	}

	repo := &recordingRepo{}
	service := services.NewOrganizationSyncService(repo, chainFetcher{}, userRepo, teamRepo)

	if _, err := service.Sync(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	chain, ok := teamRepo.updatedChains["t1"]
	if !ok {
		t.Fatalf("expected UpdateSupervisorChain to be called for team t1")
	}
	if len(chain) != 1 || chain[0].UserID != "mgr1" {
		t.Fatalf("expected the derived chain to reflect the unchanged lead, got %v", chain)
	}
}
