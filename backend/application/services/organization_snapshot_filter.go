package services

import (
	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	"github.com/agopalakrishnan/teams360/backend/pkg/orgsnapshot"
)

// SnapshotFilterResult is a snapshot reduced to records this deployment can
// actually import, plus a record of every correction that was required.
type SnapshotFilterResult struct {
	// Snapshot contains only importable records and is internally consistent,
	// so it is safe to hand to orgsnapshot.Validate.
	Snapshot *orgsnapshot.Snapshot

	// SkippedUsers are users excluded from the import, with the reason.
	SkippedUsers []orgprovider.SkippedUser

	// PreservedMemberUserIDs are the skipped users' IDs. Membership rows they
	// already hold in THC must survive the sync.
	PreservedMemberUserIDs []string

	// PreserveReportsToUserIDs marks users whose manager could not be imported.
	PreserveReportsToUserIDs map[string]bool

	DroppedMemberships   int
	DuplicateMemberships int
	ClearedManagers      int
	ClearedTeamLeads     int
}

// FilterSnapshot removes records this deployment cannot import and repairs the
// references that removal leaves dangling.
//
// The organization snapshot contract deliberately leaves hierarchy-level
// existence to synchronization: a snapshot names levels but does not define
// them, so only THC knows which are configured. orgsnapshot.Validate treats a
// missing or dangling reference as a hard error, which would let a single
// unimportable person block an entire org sync. Filtering first, then
// validating, keeps that safety net for genuine contract breaches while letting
// a good snapshot through.
//
// Nothing here invents data. Every repair either drops a record or clears a
// reference, and each is counted so the sync can report exactly what happened.
func FilterSnapshot(in *orgsnapshot.Snapshot, knownLevels map[string]bool) SnapshotFilterResult {
	result := SnapshotFilterResult{
		PreserveReportsToUserIDs: make(map[string]bool),
	}
	if in == nil {
		return result
	}

	importable := make(map[string]bool, len(in.Users))
	users := make([]orgsnapshot.User, 0, len(in.Users))

	for _, u := range in.Users {
		switch {
		case u.HierarchyLevelID == "":
			result.SkippedUsers = append(result.SkippedUsers, orgprovider.SkippedUser{
				UserID:   u.ID,
				Username: u.Username,
				Reason:   orgprovider.SkipReasonMissingLevel,
			})
			result.PreservedMemberUserIDs = append(result.PreservedMemberUserIDs, u.ID)
		case !knownLevels[u.HierarchyLevelID]:
			result.SkippedUsers = append(result.SkippedUsers, orgprovider.SkippedUser{
				UserID:           u.ID,
				Username:         u.Username,
				HierarchyLevelID: u.HierarchyLevelID,
				Reason:           orgprovider.SkipReasonUnknownLevel,
			})
			result.PreservedMemberUserIDs = append(result.PreservedMemberUserIDs, u.ID)
		default:
			importable[u.ID] = true
			users = append(users, u)
		}
	}

	// Clear manager links that point outside the importable set — whether the
	// manager was skipped above or was never in the snapshot at all. Both mean
	// "the provider named someone we cannot resolve", and in both cases keeping
	// THC's existing value beats asserting the person reports to nobody.
	for i := range users {
		if users[i].ReportsToID != nil && !importable[*users[i].ReportsToID] {
			users[i].ReportsToID = nil
			result.PreserveReportsToUserIDs[users[i].ID] = true
			result.ClearedManagers++
		}
	}

	teamIDs := make(map[string]bool, len(in.Teams))
	for _, t := range in.Teams {
		teamIDs[t.ID] = true
	}

	// Keep memberships whose user and team both survived, dropping duplicates.
	type membershipKey struct{ userID, teamID string }
	seen := make(map[membershipKey]bool, len(in.Memberships))
	memberships := make([]orgsnapshot.Membership, 0, len(in.Memberships))
	membersOfTeam := make(map[string]map[string]bool, len(in.Teams))

	for _, m := range in.Memberships {
		if !importable[m.UserID] || !teamIDs[m.TeamID] {
			result.DroppedMemberships++
			continue
		}
		key := membershipKey{userID: m.UserID, teamID: m.TeamID}
		if seen[key] {
			result.DuplicateMemberships++
			continue
		}
		seen[key] = true
		memberships = append(memberships, m)

		if membersOfTeam[m.TeamID] == nil {
			membersOfTeam[m.TeamID] = make(map[string]bool)
		}
		membersOfTeam[m.TeamID][m.UserID] = true
	}

	// A team lead must be importable and a member of their own team. When either
	// fails, clear the lead so THC keeps whoever it already had — the contract
	// treats an omitted teamLeadId as "preserve the existing value".
	teams := make([]orgsnapshot.Team, len(in.Teams))
	copy(teams, in.Teams)
	for i := range teams {
		lead := teams[i].TeamLeadID
		if lead == nil {
			continue
		}
		if !importable[*lead] || !membersOfTeam[teams[i].ID][*lead] {
			teams[i].TeamLeadID = nil
			result.ClearedTeamLeads++
		}
	}

	result.Snapshot = &orgsnapshot.Snapshot{
		ContractVersion: in.ContractVersion,
		GeneratedAt:     in.GeneratedAt,
		Teams:           teams,
		Users:           users,
		Memberships:     memberships,
	}

	return result
}
