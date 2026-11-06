package services_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	"github.com/agopalakrishnan/teams360/backend/pkg/orgsnapshot"
)

func levelOf(t *testing.T, s *orgsnapshot.Snapshot, userID string) string {
	t.Helper()
	for _, u := range s.Users {
		if u.ID == userID {
			return u.HierarchyLevelID
		}
	}
	t.Fatalf("user %q not in snapshot", userID)
	return ""
}

func TestPromotePodLeadsRaisesALevel5LeadToLevel4(t *testing.T) {
	s := baseSnapshot(
		[]orgsnapshot.User{
			user("lead", "lead", "level-5", nil),
			user("dev", "dev", "level-5", nil),
		},
		[]orgsnapshot.Team{{ID: "t1", Name: "Goa", TeamLeadID: strPtr("lead")}},
		[]orgsnapshot.Membership{{UserID: "lead", TeamID: "t1"}, {UserID: "dev", TeamID: "t1"}},
	)

	promoted := services.PromotePodLeads(s, knownLevels())

	if promoted != 1 {
		t.Fatalf("promoted = %d, want 1", promoted)
	}
	if got := levelOf(t, s, "lead"); got != "level-4" {
		t.Errorf("lead level = %q, want level-4", got)
	}
	if got := levelOf(t, s, "dev"); got != "level-5" {
		t.Errorf("non-lead level = %q, want level-5 (unchanged)", got)
	}
}

func TestPromotePodLeadsLeavesLeadsAboveLevel5Alone(t *testing.T) {
	s := baseSnapshot(
		[]orgsnapshot.User{
			user("vp", "vp", "level-1", nil),
			user("dir", "dir", "level-2", nil),
			user("mgr", "mgr", "level-3", nil),
			user("tl", "tl", "level-4", nil),
		},
		[]orgsnapshot.Team{
			{ID: "t1", Name: "A", TeamLeadID: strPtr("vp")},
			{ID: "t2", Name: "B", TeamLeadID: strPtr("dir")},
			{ID: "t3", Name: "C", TeamLeadID: strPtr("mgr")},
			{ID: "t4", Name: "D", TeamLeadID: strPtr("tl")},
		},
		nil,
	)

	promoted := services.PromotePodLeads(s, knownLevels())

	if promoted != 0 {
		t.Fatalf("promoted = %d, want 0", promoted)
	}
	for id, want := range map[string]string{"vp": "level-1", "dir": "level-2", "mgr": "level-3", "tl": "level-4"} {
		if got := levelOf(t, s, id); got != want {
			t.Errorf("%s level = %q, want %q (unchanged)", id, got, want)
		}
	}
}

func TestPromotePodLeadsCountsALeadOfSeveralTeamsOnce(t *testing.T) {
	s := baseSnapshot(
		[]orgsnapshot.User{user("lead", "lead", "level-5", nil)},
		[]orgsnapshot.Team{
			{ID: "t1", Name: "A", TeamLeadID: strPtr("lead")},
			{ID: "t2", Name: "B", TeamLeadID: strPtr("lead")},
		},
		nil,
	)

	if promoted := services.PromotePodLeads(s, knownLevels()); promoted != 1 {
		t.Fatalf("promoted = %d, want 1", promoted)
	}
	if got := levelOf(t, s, "lead"); got != "level-4" {
		t.Errorf("level = %q, want level-4", got)
	}
}

func TestPromotePodLeadsIgnoresTeamsWithoutALead(t *testing.T) {
	s := baseSnapshot(
		[]orgsnapshot.User{user("dev", "dev", "level-5", nil)},
		[]orgsnapshot.Team{{ID: "t1", Name: "A"}},
		[]orgsnapshot.Membership{{UserID: "dev", TeamID: "t1"}},
	)

	if promoted := services.PromotePodLeads(s, knownLevels()); promoted != 0 {
		t.Fatalf("promoted = %d, want 0", promoted)
	}
	if got := levelOf(t, s, "dev"); got != "level-5" {
		t.Errorf("level = %q, want level-5", got)
	}
}

func TestPromotePodLeadsDoesNothingWhenThisDeploymentHasNoLevel4(t *testing.T) {
	levels := knownLevels()
	delete(levels, "level-4")
	s := baseSnapshot(
		[]orgsnapshot.User{user("lead", "lead", "level-5", nil)},
		[]orgsnapshot.Team{{ID: "t1", Name: "A", TeamLeadID: strPtr("lead")}},
		nil,
	)

	if promoted := services.PromotePodLeads(s, levels); promoted != 0 {
		t.Fatalf("promoted = %d, want 0", promoted)
	}
	if got := levelOf(t, s, "lead"); got != "level-5" {
		t.Errorf("level = %q, want level-5", got)
	}
}

func TestPromotePodLeadsToleratesANilSnapshot(t *testing.T) {
	if promoted := services.PromotePodLeads(nil, knownLevels()); promoted != 0 {
		t.Fatalf("promoted = %d, want 0", promoted)
	}
}

// capturingRepo records the snapshot the sync hands to reconciliation, so a
// test can assert on the levels that will actually be written.
type capturingRepo struct {
	applied *orgsnapshot.Snapshot
	levels  map[string]bool // nil means the default configured levels
}

func (r *capturingRepo) KnownHierarchyLevelIDs(context.Context) (map[string]bool, error) {
	if r.levels != nil {
		return r.levels, nil
	}
	return knownLevels(), nil
}

func (r *capturingRepo) ApplySnapshot(_ context.Context, in orgprovider.ApplyInput) (*orgprovider.ApplyResult, error) {
	r.applied = in.Snapshot
	return &orgprovider.ApplyResult{UsersSynced: len(in.Snapshot.Users)}, nil
}

func TestSyncStoresALevel5PodLeadAsLevel4(t *testing.T) {
	snap := baseSnapshot(
		[]orgsnapshot.User{
			user("lead", "lead", "level-5", nil),
			user("dev", "dev", "level-5", nil),
			user("mgr", "mgr", "level-3", nil),
		},
		[]orgsnapshot.Team{
			{ID: "t1", Name: "Goa", TeamLeadID: strPtr("lead")},
			{ID: "t2", Name: "Mysuru", TeamLeadID: strPtr("mgr")},
		},
		[]orgsnapshot.Membership{
			{UserID: "lead", TeamID: "t1"}, {UserID: "dev", TeamID: "t1"}, {UserID: "mgr", TeamID: "t2"},
		},
	)
	repo := &capturingRepo{}
	service := services.NewOrganizationSyncService(repo, fixedFetcher{snapshot: snap}, nil, nil)

	if _, err := service.Sync(context.Background()); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	if repo.applied == nil {
		t.Fatal("reconciliation never ran")
	}
	for id, want := range map[string]string{"lead": "level-4", "dev": "level-5", "mgr": "level-3"} {
		if got := levelOf(t, repo.applied, id); got != want {
			t.Errorf("%s level handed to reconciliation = %q, want %q", id, got, want)
		}
	}
	// The provider's own payload must not be edited in place.
	if got := levelOf(t, snap, "lead"); got != "level-5" {
		t.Errorf("provider snapshot was mutated: lead level = %q, want level-5", got)
	}
}

func TestSyncDoesNotPromoteALeadThatIsNotAMemberOfTheirOwnTeam(t *testing.T) {
	// FilterSnapshot clears a lead who is not a member of the team. Nobody
	// should be promoted on the strength of a lead link that was discarded.
	snap := baseSnapshot(
		[]orgsnapshot.User{user("lead", "lead", "level-5", nil)},
		[]orgsnapshot.Team{{ID: "t1", Name: "Goa", TeamLeadID: strPtr("lead")}},
		[]orgsnapshot.Membership{},
	)
	repo := &capturingRepo{}
	service := services.NewOrganizationSyncService(repo, fixedFetcher{snapshot: snap}, nil, nil)

	if _, err := service.Sync(context.Background()); err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if got := levelOf(t, repo.applied, "lead"); got != "level-5" {
		t.Errorf("lead level = %q, want level-5 (lead link was discarded)", got)
	}
}

func TestSyncKeepsTheProviderLevelWhenLevel4IsNotConfigured(t *testing.T) {
	levels := knownLevels()
	delete(levels, "level-4")
	snap := baseSnapshot(
		[]orgsnapshot.User{user("lead", "lead", "level-5", nil)},
		[]orgsnapshot.Team{{ID: "t1", Name: "Goa", TeamLeadID: strPtr("lead")}},
		[]orgsnapshot.Membership{{UserID: "lead", TeamID: "t1"}},
	)
	repo := &capturingRepo{levels: levels}
	service := services.NewOrganizationSyncService(repo, fixedFetcher{snapshot: snap}, nil, nil)

	if _, err := service.Sync(context.Background()); err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if got := levelOf(t, repo.applied, "lead"); got != "level-5" {
		t.Errorf("lead level = %q, want level-5 (level-4 is not configured)", got)
	}
}

func TestPromotePodLeadsIsIdempotent(t *testing.T) {
	s := baseSnapshot(
		[]orgsnapshot.User{user("lead", "lead", "level-5", nil)},
		[]orgsnapshot.Team{{ID: "t1", Name: "Goa", TeamLeadID: strPtr("lead")}},
		nil,
	)

	if first := services.PromotePodLeads(s, knownLevels()); first != 1 {
		t.Fatalf("first run promoted %d, want 1", first)
	}
	if second := services.PromotePodLeads(s, knownLevels()); second != 0 {
		t.Errorf("second run promoted %d, want 0 (already level-4)", second)
	}
	if got := levelOf(t, s, "lead"); got != "level-4" {
		t.Errorf("level = %q, want level-4", got)
	}
}

func TestSyncRecomputesEachRunSoAFormerLeadReturnsToLevel5(t *testing.T) {
	withLead := func() *orgsnapshot.Snapshot {
		return baseSnapshot(
			[]orgsnapshot.User{user("lead", "lead", "level-5", nil), user("dev", "dev", "level-5", nil)},
			[]orgsnapshot.Team{{ID: "t1", Name: "Goa", TeamLeadID: strPtr("lead")}},
			[]orgsnapshot.Membership{{UserID: "lead", TeamID: "t1"}, {UserID: "dev", TeamID: "t1"}},
		)
	}
	// The same person, but the provider now names someone else as lead.
	withNewLead := func() *orgsnapshot.Snapshot {
		s := withLead()
		s.Teams[0].TeamLeadID = strPtr("dev")
		return s
	}

	runSync := func(snap *orgsnapshot.Snapshot) *orgsnapshot.Snapshot {
		repo := &capturingRepo{}
		service := services.NewOrganizationSyncService(repo, fixedFetcher{snapshot: snap}, nil, nil)
		if _, err := service.Sync(context.Background()); err != nil {
			t.Fatalf("sync failed: %v", err)
		}
		return repo.applied
	}

	// Two syncs of the same snapshot give the same result.
	for run := 1; run <= 2; run++ {
		applied := runSync(withLead())
		if got := levelOf(t, applied, "lead"); got != "level-4" {
			t.Fatalf("run %d: lead level = %q, want level-4", run, got)
		}
	}

	// When the lead changes, the former lead is stored at the provider's level-5
	// again and the new lead is promoted.
	applied := runSync(withNewLead())
	if got := levelOf(t, applied, "lead"); got != "level-5" {
		t.Errorf("former lead level = %q, want level-5", got)
	}
	if got := levelOf(t, applied, "dev"); got != "level-4" {
		t.Errorf("new lead level = %q, want level-4", got)
	}
}

// podiqLevelForGrade is the provider's rule (mapping 1), reproduced here so the
// tests can start from a seniority grade exactly as the provider does. THC
// itself only ever sees the resulting level.
func podiqLevelForGrade(grade int) string {
	return fmt.Sprintf("level-%d", grade-3)
}

func hasUser(s *orgsnapshot.Snapshot, userID string) bool {
	for _, u := range s.Users {
		if u.ID == userID {
			return true
		}
	}
	return false
}

func syncOnce(t *testing.T, snap *orgsnapshot.Snapshot) *orgsnapshot.Snapshot {
	t.Helper()
	repo := &capturingRepo{}
	service := services.NewOrganizationSyncService(repo, fixedFetcher{snapshot: snap}, nil, nil)
	if _, err := service.Sync(context.Background()); err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if repo.applied == nil {
		t.Fatal("reconciliation never ran")
	}
	return repo.applied
}

// The documented table: seniority grade -> level the provider sends -> level
// THC stores, for a pod lead and for a non-lead.
var gradeTable = []struct {
	grade        int
	providerSent string
	leadStored   string
	memberStored string
}{
	{4, "level-1", "level-1", "level-1"},
	{5, "level-2", "level-2", "level-2"},
	{6, "level-3", "level-3", "level-3"},
	{7, "level-4", "level-4", "level-4"},
	{8, "level-5", "level-4", "level-5"}, // the only grade THC promotes
}

func TestProviderRuleMatchesTheDocumentedTable(t *testing.T) {
	for _, c := range gradeTable {
		if got := podiqLevelForGrade(c.grade); got != c.providerSent {
			t.Errorf("grade %d: provider level = %q, table says %q", c.grade, got, c.providerSent)
		}
	}
}

func TestEveryGradeIsStoredAtTheDocumentedLevel(t *testing.T) {
	for _, c := range gradeTable {
		t.Run(fmt.Sprintf("grade %d", c.grade), func(t *testing.T) {
			level := podiqLevelForGrade(c.grade)
			snap := baseSnapshot(
				[]orgsnapshot.User{user("lead", "lead", level, nil), user("member", "member", level, nil)},
				[]orgsnapshot.Team{{ID: "t1", Name: "Goa", TeamLeadID: strPtr("lead")}},
				[]orgsnapshot.Membership{{UserID: "lead", TeamID: "t1"}, {UserID: "member", TeamID: "t1"}},
			)

			applied := syncOnce(t, snap)

			if got := levelOf(t, applied, "lead"); got != c.leadStored {
				t.Errorf("pod lead stored as %q, want %q", got, c.leadStored)
			}
			if got := levelOf(t, applied, "member"); got != c.memberStored {
				t.Errorf("non-lead stored as %q, want %q", got, c.memberStored)
			}
		})
	}
}

func TestAllGradesInOneSnapshotAreMappedIndependently(t *testing.T) {
	var users []orgsnapshot.User
	var teams []orgsnapshot.Team
	var memberships []orgsnapshot.Membership
	for _, c := range gradeTable {
		level := podiqLevelForGrade(c.grade)
		lead := fmt.Sprintf("lead%d", c.grade)
		member := fmt.Sprintf("member%d", c.grade)
		team := fmt.Sprintf("team%d", c.grade)
		users = append(users, user(lead, lead, level, nil), user(member, member, level, nil))
		teams = append(teams, orgsnapshot.Team{ID: team, Name: team, TeamLeadID: strPtr(lead)})
		memberships = append(memberships,
			orgsnapshot.Membership{UserID: lead, TeamID: team},
			orgsnapshot.Membership{UserID: member, TeamID: team})
	}

	applied := syncOnce(t, baseSnapshot(users, teams, memberships))

	for _, c := range gradeTable {
		if got := levelOf(t, applied, fmt.Sprintf("lead%d", c.grade)); got != c.leadStored {
			t.Errorf("grade %d lead stored as %q, want %q", c.grade, got, c.leadStored)
		}
		if got := levelOf(t, applied, fmt.Sprintf("member%d", c.grade)); got != c.memberStored {
			t.Errorf("grade %d non-lead stored as %q, want %q", c.grade, got, c.memberStored)
		}
	}
}

func TestLevelsTHCDoesNotHaveAreSkippedAndNeverPromoted(t *testing.T) {
	// Grade 9 arrives as level-6, which THC has no such level for, and a blank
	// grade arrives as "". Neither is imported, neither is promoted, and a
	// grade-8 lead in the same snapshot is still promoted.
	snap := baseSnapshot(
		[]orgsnapshot.User{
			user("grade9", "grade9", podiqLevelForGrade(9), nil),
			user("blank", "blank", "", nil),
			user("grade8", "grade8", podiqLevelForGrade(8), nil),
		},
		[]orgsnapshot.Team{
			{ID: "t9", Name: "Nine", TeamLeadID: strPtr("grade9")},
			{ID: "tb", Name: "Blank", TeamLeadID: strPtr("blank")},
			{ID: "t8", Name: "Eight", TeamLeadID: strPtr("grade8")},
		},
		[]orgsnapshot.Membership{
			{UserID: "grade9", TeamID: "t9"}, {UserID: "blank", TeamID: "tb"}, {UserID: "grade8", TeamID: "t8"},
		},
	)

	applied := syncOnce(t, snap)

	for _, id := range []string{"grade9", "blank"} {
		if hasUser(applied, id) {
			t.Errorf("%s has a level THC does not know and must be skipped, but it was imported", id)
		}
	}
	if got := levelOf(t, applied, "grade8"); got != "level-4" {
		t.Errorf("grade-8 lead stored as %q, want level-4", got)
	}
}
