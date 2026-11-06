package services

import "github.com/agopalakrishnan/teams360/backend/pkg/orgsnapshot"

// How a person's level reaches THC takes two mappings, applied in this order.
//
// Mapping 1 (provider, not in this repo): the provider derives hierarchyLevelId
// from the seniority grade (Okta managementLevel) as level-(grade - 3). Grades
// 1-3 are never sent. THC receives only the finished level.
//
// Mapping 2 (THC only, this file): a pod lead sent as level-5 ("Team Member")
// is stored as level-4 ("Team Lead"). THC grants the Dashboard and Export on
// level-4, and a senior engineer who leads a pod would otherwise never get it.
//
//	grade   provider level   pod lead stored as   non-lead stored as
//	  4     level-1          level-1              level-1
//	  5     level-2          level-2              level-2
//	  6     level-3          level-3              level-3
//	  7     level-4          level-4              level-4
//	  8     level-5          level-4  (promoted)  level-5
//	  9+    level-6 and up   skipped: THC has no such level (see FilterSnapshot)
//
// Both ids live here so mapping 2 is defined in exactly one place.
const (
	podLeadSourceLevelID = "level-5"
	podLeadLevelID       = "level-4"
)

// PromotePodLeads rewrites, in place, the level of every user who leads a team
// in the snapshot and whom the provider sent as podLeadSourceLevelID, to
// podLeadLevelID. It returns how many users were promoted.
//
// It must run on the snapshot FilterSnapshot returned, not on the raw provider
// payload: FilterSnapshot clears a teamLeadId that points at a user THC cannot
// import or who is not a member of their own team, and PromotePodLeads trusts
// the teamLeadId values it finds. Because FilterSnapshot builds its own user
// slice, editing that copy leaves the provider's payload untouched.
//
// Only an exact podLeadSourceLevelID is promoted. Leads already at level-4, and
// leads above it (levels 1-3), keep the level the provider sent, as do
// non-leads. The result is recomputed from the snapshot on every sync, so a
// former lead is stored at the provider's level-5 again on the next run.
//
// If this deployment has no podLeadLevelID configured the mapping is skipped
// and every level is left as the provider sent it.
func PromotePodLeads(s *orgsnapshot.Snapshot, knownLevels map[string]bool) int {
	if s == nil || !knownLevels[podLeadLevelID] {
		return 0
	}

	leads := make(map[string]bool, len(s.Teams))
	for _, t := range s.Teams {
		if t.TeamLeadID != nil && *t.TeamLeadID != "" {
			leads[*t.TeamLeadID] = true
		}
	}
	if len(leads) == 0 {
		return 0
	}

	promoted := 0
	for i := range s.Users {
		u := &s.Users[i]
		if leads[u.ID] && u.HierarchyLevelID == podLeadSourceLevelID {
			u.HierarchyLevelID = podLeadLevelID
			promoted++
		}
	}
	return promoted
}
