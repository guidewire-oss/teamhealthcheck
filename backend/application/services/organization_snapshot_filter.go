package services

import (
	"fmt"

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
//
// Duplicate user IDs are rejected here, on the raw input, before that
// classification runs: if one copy of a duplicated ID is skipped for its
// hierarchy level, classification would otherwise keep only the other copy,
// so the filtered snapshot handed to orgsnapshot.Validate would contain a
// single, non-duplicated ID and the contract violation would never surface.
func FilterSnapshot(in *orgsnapshot.Snapshot, knownLevels map[string]bool) (SnapshotFilterResult, error) {
	result := SnapshotFilterResult{
		PreserveReportsToUserIDs: make(map[string]bool),
	}
	if in == nil {
		return result, nil
	}

	seenUserIDs := make(map[string]bool, len(in.Users))
	for _, u := range in.Users {
		if u.ID == "" {
			continue // Validate reports every empty ID on its own; this check is about duplicate identity, not missing identity
		}
		if seenUserIDs[u.ID] {
			return result, fmt.Errorf("duplicate user id %q in snapshot", u.ID)
		}
		seenUserIDs[u.ID] = true
	}

	importable := make(map[string]bool, len(in.Users))
	users := make([]orgsnapshot.User, 0, len(in.Users))

	for _, u := range in.Users {
		switch {
		case u.ID == "":
			// A record with no id is a contract breach, not an unimportable
			// person. Dropping it here (as the level checks below would) would
			// hide the breach whenever another user survives, so it is kept as
			// is and orgsnapshot.Validate rejects the whole snapshot with a
			// field-specific error.
			users = append(users, u)
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
	//
	// An explicit empty string is different: nil means "no manager", but ""
	// is contract-invalid. It is left untouched so Validate rejects it rather
	// than being repaired here into "keep the existing manager".
	for i := range users {
		if users[i].ReportsToID != nil && *users[i].ReportsToID != "" && !importable[*users[i].ReportsToID] {
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
	// A nil in.Memberships (omitted or null) must stay nil here: orgsnapshot.Validate
	// rejects that case, and allocating a non-nil slice would mask it.
	type membershipKey struct{ userID, teamID string }
	seen := make(map[membershipKey]bool, len(in.Memberships))
	var memberships []orgsnapshot.Membership
	if in.Memberships != nil {
		memberships = make([]orgsnapshot.Membership, 0, len(in.Memberships))
	}
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
		// nil (omitted) preserves the existing lead; an explicit "" is
		// contract-invalid and must reach Validate rather than be turned into
		// nil, which would silently keep the previous lead.
		if lead == nil || *lead == "" {
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

	return result, nil
}
