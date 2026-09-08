package services_test

import (
	"testing"
	"time"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	"github.com/agopalakrishnan/teams360/backend/pkg/orgsnapshot"
)

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }

// knownLevels mirrors the levels seeded by migration 000009.
func knownLevels() map[string]bool {
	return map[string]bool{
		"level-1": true, "level-2": true, "level-3": true,
		"level-4": true, "level-5": true, "level-admin": true,
	}
}

func user(id, username, level string, reportsTo *string) orgsnapshot.User {
	return orgsnapshot.User{
		ID:               id,
		Username:         username,
		DisplayName:      username,
		Email:            username + "@example.com",
		HierarchyLevelID: level,
		ReportsToID:      reportsTo,
	}
}

func baseSnapshot(users []orgsnapshot.User, teams []orgsnapshot.Team, m []orgsnapshot.Membership) *orgsnapshot.Snapshot {
	return &orgsnapshot.Snapshot{
		ContractVersion: orgsnapshot.ContractVersion,
		GeneratedAt:     time.Date(2026, 8, 31, 5, 4, 48, 0, time.UTC),
		Users:           users,
		Teams:           teams,
		Memberships:     m,
	}
}

func TestFilterSnapshotKeepsACleanSnapshotIntact(t *testing.T) {
	in := baseSnapshot(
		[]orgsnapshot.User{
			user("u1", "alice", "level-3", nil),
			user("u2", "bob", "level-5", strPtr("u1")),
		},
		[]orgsnapshot.Team{{ID: "t1", Name: "Alcatraz", TeamLeadID: strPtr("u1"), HealthCheckEnabled: boolPtr(false)}},
		[]orgsnapshot.Membership{{UserID: "u1", TeamID: "t1"}, {UserID: "u2", TeamID: "t1"}},
	)

	got := services.FilterSnapshot(in, knownLevels())

	if len(got.SkippedUsers) != 0 {
		t.Errorf("SkippedUsers = %d, want 0", len(got.SkippedUsers))
	}
	if len(got.Snapshot.Users) != 2 || len(got.Snapshot.Teams) != 1 || len(got.Snapshot.Memberships) != 2 {
		t.Errorf("got %d users, %d teams, %d memberships; want 2/1/2",
			len(got.Snapshot.Users), len(got.Snapshot.Teams), len(got.Snapshot.Memberships))
	}
	if got.Snapshot.Teams[0].TeamLeadID == nil || *got.Snapshot.Teams[0].TeamLeadID != "u1" {
		t.Error("a valid team lead was cleared")
	}
	if err := got.Snapshot.ValidationError(); err != nil {
		t.Errorf("filtered snapshot failed validation: %v", err)
	}
}

func TestFilterSnapshotSkipsUsersWithUnusableLevels(t *testing.T) {
	in := baseSnapshot(
		[]orgsnapshot.User{
			user("u1", "alice", "level-3", nil),
			user("blank", "e2euser", "", nil),   // e2e-* accounts arrive with no level
			user("exec", "ceo", "level-0", nil), // level not configured in THC
		},
		[]orgsnapshot.Team{{ID: "t1", Name: "Alcatraz"}},
		[]orgsnapshot.Membership{{UserID: "u1", TeamID: "t1"}},
	)

	got := services.FilterSnapshot(in, knownLevels())

	if len(got.Snapshot.Users) != 1 || got.Snapshot.Users[0].ID != "u1" {
		t.Fatalf("importable users = %+v, want only u1", got.Snapshot.Users)
	}
	if len(got.SkippedUsers) != 2 {
		t.Fatalf("SkippedUsers = %d, want 2", len(got.SkippedUsers))
	}

	reasons := map[string]string{}
	for _, s := range got.SkippedUsers {
		reasons[s.UserID] = s.Reason
	}
	if reasons["blank"] != orgprovider.SkipReasonMissingLevel {
		t.Errorf("blank-level reason = %q, want %q", reasons["blank"], orgprovider.SkipReasonMissingLevel)
	}
	if reasons["exec"] != orgprovider.SkipReasonUnknownLevel {
		t.Errorf("unknown-level reason = %q, want %q", reasons["exec"], orgprovider.SkipReasonUnknownLevel)
	}

	// Skipped users must be protected from the membership replace, otherwise
	// skipping them would strip their existing team rows.
	preserved := map[string]bool{}
	for _, id := range got.PreservedMemberUserIDs {
		preserved[id] = true
	}
	if !preserved["blank"] || !preserved["exec"] {
		t.Errorf("PreservedMemberUserIDs = %v, want both skipped users", got.PreservedMemberUserIDs)
	}

	if err := got.Snapshot.ValidationError(); err != nil {
		t.Errorf("filtered snapshot failed validation: %v", err)
	}
}

func TestFilterSnapshotClearsUnresolvableManagerLinks(t *testing.T) {
	in := baseSnapshot(
		[]orgsnapshot.User{
			user("u1", "alice", "level-5", strPtr("exec")), // manager is skipped
			user("u2", "bob", "level-5", strPtr("ghost")),  // manager absent from the payload
			user("exec", "ceo", "level-0", nil),
		},
		[]orgsnapshot.Team{{ID: "t1", Name: "Alcatraz"}},
		nil,
	)

	got := services.FilterSnapshot(in, knownLevels())

	if got.ClearedManagers != 2 {
		t.Errorf("ClearedManagers = %d, want 2", got.ClearedManagers)
	}
	for _, u := range got.Snapshot.Users {
		if u.ReportsToID != nil {
			t.Errorf("user %s kept an unresolvable manager %q", u.ID, *u.ReportsToID)
		}
		if !got.PreserveReportsToUserIDs[u.ID] {
			t.Errorf("user %s should be marked so its existing manager is preserved", u.ID)
		}
	}
	if err := got.Snapshot.ValidationError(); err != nil {
		t.Errorf("filtered snapshot failed validation: %v", err)
	}
}

func TestFilterSnapshotDropsUnusableMemberships(t *testing.T) {
	in := baseSnapshot(
		[]orgsnapshot.User{
			user("u1", "alice", "level-5", nil),
			user("exec", "ceo", "level-0", nil),
		},
		[]orgsnapshot.Team{{ID: "t1", Name: "Alcatraz"}},
		[]orgsnapshot.Membership{
			{UserID: "u1", TeamID: "t1"},
			{UserID: "exec", TeamID: "t1"},  // user skipped
			{UserID: "u1", TeamID: "ghost"}, // team not in the snapshot
			{UserID: "u1", TeamID: "t1"},    // duplicate
		},
	)

	got := services.FilterSnapshot(in, knownLevels())

	if len(got.Snapshot.Memberships) != 1 {
		t.Fatalf("memberships = %+v, want exactly one", got.Snapshot.Memberships)
	}
	if got.DroppedMemberships != 2 {
		t.Errorf("DroppedMemberships = %d, want 2", got.DroppedMemberships)
	}
	if got.DuplicateMemberships != 1 {
		t.Errorf("DuplicateMemberships = %d, want 1", got.DuplicateMemberships)
	}
	if err := got.Snapshot.ValidationError(); err != nil {
		t.Errorf("filtered snapshot failed validation: %v", err)
	}
}

func TestFilterSnapshotClearsUnusableTeamLeads(t *testing.T) {
	in := baseSnapshot(
		[]orgsnapshot.User{
			user("u1", "alice", "level-5", nil),
			user("exec", "ceo", "level-0", nil),
		},
		[]orgsnapshot.Team{
			{ID: "t1", Name: "Skipped lead", TeamLeadID: strPtr("exec")},
			{ID: "t2", Name: "Lead is not a member", TeamLeadID: strPtr("u1")},
			{ID: "t3", Name: "Lead is a member", TeamLeadID: strPtr("u1")},
		},
		[]orgsnapshot.Membership{{UserID: "u1", TeamID: "t3"}},
	)

	got := services.FilterSnapshot(in, knownLevels())

	leads := map[string]*string{}
	for _, tm := range got.Snapshot.Teams {
		leads[tm.ID] = tm.TeamLeadID
	}
	if leads["t1"] != nil {
		t.Error("t1 kept a lead that could not be imported")
	}
	if leads["t2"] != nil {
		t.Error("t2 kept a lead who is not a member of the team")
	}
	if leads["t3"] == nil || *leads["t3"] != "u1" {
		t.Error("t3 lost a valid lead")
	}
	if got.ClearedTeamLeads != 2 {
		t.Errorf("ClearedTeamLeads = %d, want 2", got.ClearedTeamLeads)
	}
	if err := got.Snapshot.ValidationError(); err != nil {
		t.Errorf("filtered snapshot failed validation: %v", err)
	}
}

func TestFilterSnapshotPreservesTriStateHealthCheckFlag(t *testing.T) {
	in := baseSnapshot(
		[]orgsnapshot.User{user("u1", "alice", "level-5", nil)},
		[]orgsnapshot.Team{
			{ID: "t1", Name: "Off", HealthCheckEnabled: boolPtr(false)},
			{ID: "t2", Name: "On", HealthCheckEnabled: boolPtr(true)},
			{ID: "t3", Name: "Unspecified"},
		},
		nil,
	)

	got := services.FilterSnapshot(in, knownLevels())

	flags := map[string]*bool{}
	for _, tm := range got.Snapshot.Teams {
		flags[tm.ID] = tm.HealthCheckEnabled
	}
	if flags["t1"] == nil || *flags["t1"] {
		t.Error("explicit false was not preserved")
	}
	if flags["t2"] == nil || !*flags["t2"] {
		t.Error("explicit true was not preserved")
	}
	if flags["t3"] != nil {
		t.Error("omitted flag must stay nil so persistence preserves the existing value")
	}
}

func TestFilterSnapshotDoesNotMutateItsInput(t *testing.T) {
	in := baseSnapshot(
		[]orgsnapshot.User{user("u1", "alice", "level-5", strPtr("ghost"))},
		[]orgsnapshot.Team{{ID: "t1", Name: "Alcatraz", TeamLeadID: strPtr("ghost")}},
		nil,
	)

	services.FilterSnapshot(in, knownLevels())

	if in.Users[0].ReportsToID == nil {
		t.Error("the caller's user slice was mutated")
	}
	if in.Teams[0].TeamLeadID == nil {
		t.Error("the caller's team slice was mutated")
	}
}

func TestFilterSnapshotHandlesNil(t *testing.T) {
	got := services.FilterSnapshot(nil, knownLevels())

	if got.Snapshot != nil {
		t.Error("nil input should not produce a snapshot")
	}
	if got.PreserveReportsToUserIDs == nil {
		t.Error("PreserveReportsToUserIDs should always be usable")
	}
}
