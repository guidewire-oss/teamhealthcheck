package v1

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agopalakrishnan/teams360/backend/application/queries"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
)

// This fixture mirrors the feature request's own worked example (a Senior
// Director with a direct pod, a Senior Manager reporting straight to them —
// skipping the Director tier entirely — and a Director who in turn has a
// Manager reporting to them), plus a second, unrelated root, an opted-out
// pod, and an unassigned pod, so one Describe block exercises: multiple
// roots, direct pods at every tier, missing intermediate levels, and
// aggregate rollup through more than two levels at once.
var _ = Describe("buildSurveyCompletionResponse", func() {
	persons := []queries.PersonNode{
		{UserID: "sd-a", Name: "Senior Director A", Email: "sda@x.com", LevelID: "senior-director", LevelName: "Senior Director", ParentID: ""},
		{UserID: "dir-b", Name: "Director B", Email: "dirb@x.com", LevelID: "director", LevelName: "Director", ParentID: "sd-a"},
		{UserID: "sm-c", Name: "Senior Manager C", Email: "smc@x.com", LevelID: "senior-manager", LevelName: "Senior Manager", ParentID: "sd-a"},
		{UserID: "mgr-d", Name: "Manager D", Email: "mgrd@x.com", LevelID: "manager", LevelName: "Manager", ParentID: "dir-b"},
		{UserID: "sd-e", Name: "Senior Director E", Email: "sde@x.com", LevelID: "senior-director", LevelName: "Senior Director", ParentID: ""},
	}

	overview := &queries.SurveyCompletionOverview{
		Persons: persons,
		Teams: []queries.SurveyCompletionTeam{
			// alpha, gamma, and epsilon are the fixture's "complete" pods --
			// per issue #143's status rule (deriveSurveyCompletionStatus),
			// that requires every eligible member's individual survey done
			// AND the team's post-workshop session completed, so each needs
			// PostWorkshopCompleted: true in addition to Completed >= Total.
			{TeamID: "alpha", TeamName: "Team Alpha", OwnerUserID: "sd-a", TeamLeadID: "tl1", TeamLeadName: "Lead Alpha", Total: 3, Completed: 3, PostWorkshopCompleted: true, HealthCheckEnabled: true},
			{TeamID: "eta", TeamName: "Team Eta", OwnerUserID: "sd-a", TeamLeadID: "tl-eta", TeamLeadName: "Lead Eta", Total: 2, Completed: 0, HealthCheckEnabled: false},
			{TeamID: "beta", TeamName: "Team Beta", OwnerUserID: "dir-b", TeamLeadID: "tl2", TeamLeadName: "Lead Beta", Total: 4, Completed: 1, HealthCheckEnabled: true},
			{TeamID: "gamma", TeamName: "Team Gamma", OwnerUserID: "mgr-d", TeamLeadID: "tl3", TeamLeadName: "Lead Gamma", Total: 2, Completed: 2, PostWorkshopCompleted: true, HealthCheckEnabled: true},
			{TeamID: "delta", TeamName: "Team Delta", OwnerUserID: "sm-c", TeamLeadID: "tl4", TeamLeadName: "Lead Delta", Total: 5, Completed: 0, HealthCheckEnabled: true},
			{TeamID: "epsilon", TeamName: "Team Epsilon", OwnerUserID: "sd-e", TeamLeadID: "tl5", TeamLeadName: "Lead Epsilon", Total: 1, Completed: 1, PostWorkshopCompleted: true, HealthCheckEnabled: true},
			{TeamID: "zeta", TeamName: "Team Zeta", OwnerUserID: "", TeamLeadID: "tl6", TeamLeadName: "Lead Zeta", Total: 1, Completed: 0, HealthCheckEnabled: true},
		},
	}

	resp := buildSurveyCompletionResponse(overview)

	It("classifies overall counts across all seven pods", func() {
		Expect(resp.TotalTeams).To(Equal(7))
		Expect(resp.OptedOut).To(Equal(1))      // eta
		Expect(resp.OptedIn).To(Equal(6))       // everything but eta
		Expect(resp.FullyComplete).To(Equal(3)) // alpha, gamma, epsilon
		Expect(resp.InProgress).To(Equal(1))    // beta
		Expect(resp.NotStarted).To(Equal(2))    // delta, zeta
	})

	It("produces exactly two root groups (both Senior Directors) and Other, in that order", func() {
		Expect(resp.Groups).To(HaveLen(3))
		Expect(resp.Groups[0].Type).To(Equal("person"))
		Expect(resp.Groups[0].Person.Name).To(Equal("Senior Director A"))
		Expect(resp.Groups[1].Type).To(Equal("person"))
		Expect(resp.Groups[1].Person.Name).To(Equal("Senior Director E"))
		Expect(resp.Groups[2].Type).To(Equal("other"))
	})

	It("never lists a Director or Senior Manager who reports to a Senior Director as a separate root", func() {
		names := []string{}
		for _, g := range resp.Groups {
			if g.Person != nil {
				names = append(names, g.Person.Name)
			}
		}
		Expect(names).To(ConsistOf("Senior Director A", "Senior Director E"))
	})

	It("nests a Senior Manager directly under a Senior Director, skipping the Director tier (missing intermediate level)", func() {
		sdA := resp.Groups[0]
		found := false
		for _, c := range sdA.Children {
			if c.Person.Name == "Senior Manager C" {
				found = true
				Expect(c.Person.LevelID).To(Equal("senior-manager"))
				Expect(c.DirectTeams).To(HaveLen(1))
				Expect(c.DirectTeams[0].TeamID).To(Equal("delta"))
				Expect(c.Children).To(BeEmpty())
			}
		}
		Expect(found).To(BeTrue(), "expected Senior Manager C to be a direct child of Senior Director A")
	})

	It("nests a Manager two levels deep under a Director under a Senior Director", func() {
		sdA := resp.Groups[0]
		found := false
		for _, c := range sdA.Children {
			if c.Person.Name == "Director B" {
				found = true
				Expect(c.Person.LevelID).To(Equal("director"))
				Expect(c.DirectTeams).To(HaveLen(1))
				Expect(c.DirectTeams[0].TeamID).To(Equal("beta"))
				Expect(c.Children).To(HaveLen(1))
				Expect(c.Children[0].Person.Name).To(Equal("Manager D"))
				Expect(c.Children[0].DirectTeams).To(HaveLen(1))
				Expect(c.Children[0].DirectTeams[0].TeamID).To(Equal("gamma"))
				Expect(c.Children[0].Children).To(BeEmpty())
			}
		}
		Expect(found).To(BeTrue(), "expected Director B to be a direct child of Senior Director A")
	})

	It("sorts a leader's children alphabetically by name", func() {
		sdA := resp.Groups[0]
		Expect(sdA.Children).To(HaveLen(2))
		Expect(sdA.Children[0].Person.Name).To(Equal("Director B")) // 'D' < 'S'
		Expect(sdA.Children[1].Person.Name).To(Equal("Senior Manager C"))
	})

	It("rolls up totals and remind counts through every level, excluding the opted-out pod from opted-in/remind math", func() {
		sdA := resp.Groups[0]
		// alpha(complete) + eta(opted_out) + beta(in_progress) + gamma(complete) + delta(not_started)
		Expect(sdA.TotalTeams).To(Equal(5))
		Expect(sdA.OptedInTeams).To(Equal(4))       // excludes eta
		Expect(sdA.RemindCount).To(Equal(2))        // beta + delta
		Expect(sdA.CompletionPercent).To(Equal(50)) // 2 complete (alpha, gamma) of 4 opted-in

		sdE := resp.Groups[1]
		Expect(sdE.TotalTeams).To(Equal(1))
		Expect(sdE.OptedInTeams).To(Equal(1))
		Expect(sdE.RemindCount).To(Equal(0))
		Expect(sdE.CompletionPercent).To(Equal(100))
	})

	It("routes the unassigned pod to Other, escalated to its team lead", func() {
		other := resp.Groups[2]
		Expect(other.Label).To(Equal("Other"))
		Expect(other.Teams).To(HaveLen(1))
		Expect(other.Teams[0].TeamID).To(Equal("zeta"))
		Expect(*other.Teams[0].TeamLeadName).To(Equal("Lead Zeta"))
	})

	It("renders every input pod exactly once across the whole tree", func() {
		seen := map[string]int{}
		for _, g := range resp.Groups {
			countGroupTeams(g, seen)
		}

		Expect(seen).To(HaveLen(7))
		for id, count := range seen {
			Expect(count).To(Equal(1), "team %s should appear exactly once", id)
		}
	})
})

// countGroupTeams recursively tallies every team id appearing anywhere in
// this group's subtree (its own directTeams/teams plus every descendant's),
// used to assert no pod is ever rendered more than once.
func countGroupTeams(g dto.SurveyCompletionGroupDTO, seen map[string]int) {
	for _, t := range g.DirectTeams {
		seen[t.TeamID]++
	}
	for _, t := range g.Teams {
		seen[t.TeamID]++
	}
	for _, c := range g.Children {
		countGroupTeams(c, seen)
	}
}

// Regression test: Not Started and Opted Out must stay distinct at every
// tier of the hierarchy, and neither is ever inferred from a 0% completion
// percentage -- opted_out comes purely from health_check_enabled, checked
// before the 0%-completion check, so an opted-out pod at 0% completion is
// never relabeled "not_started" (and a genuinely not-started, opted-IN pod
// is never swept into "opted_out"). Fixture names are generic placeholders
// only -- deriveSurveyCompletionStatus never special-cases any person or
// pod name.
var _ = Describe("buildSurveyCompletionResponse Not Started vs Opted Out", func() {
	persons := []queries.PersonNode{
		{UserID: "root-1", Name: "Root Leader", LevelID: "director", LevelName: "Director", ParentID: ""},
		{UserID: "mid-1", Name: "Middle Leader", LevelID: "manager", LevelName: "Manager", ParentID: "root-1"},
	}

	overview := &queries.SurveyCompletionOverview{
		Persons: persons,
		Teams: []queries.SurveyCompletionTeam{
			// A direct pod under the root: not started (opted in, zero progress).
			{TeamID: "pod-not-started", TeamName: "Pod Not Started", OwnerUserID: "root-1", Total: 4, Completed: 0, HealthCheckEnabled: true},
			// A direct pod under the root: opted out, ALSO at 0% completion --
			// the exact ambiguous case that must not collapse into not_started.
			{TeamID: "pod-opted-out", TeamName: "Pod Opted Out", OwnerUserID: "root-1", Total: 4, Completed: 0, HealthCheckEnabled: false},
			// A not-started pod nested one level deeper, under Middle Leader.
			{TeamID: "pod-nested-not-started", TeamName: "Pod Nested Not Started", OwnerUserID: "mid-1", Total: 3, Completed: 0, HealthCheckEnabled: true},
			// An opted-out pod nested one level deeper, under Middle Leader.
			{TeamID: "pod-nested-opted-out", TeamName: "Pod Nested Opted Out", OwnerUserID: "mid-1", Total: 3, Completed: 0, HealthCheckEnabled: false},
		},
	}

	resp := buildSurveyCompletionResponse(overview)

	It("gives the opted-out pod status opted_out, never not_started, despite 0% completion", func() {
		root := resp.Groups[0]
		byID := map[string]dto.SurveyCompletionTeamDTO{}
		for _, t := range root.DirectTeams {
			byID[t.TeamID] = t
		}
		Expect(byID["pod-not-started"].Status).To(Equal("not_started"))
		Expect(byID["pod-opted-out"].Status).To(Equal("opted_out"))
	})

	It("keeps the same distinction for a pod nested under a subordinate leader", func() {
		root := resp.Groups[0]
		Expect(root.Children).To(HaveLen(1))
		mid := root.Children[0]
		byID := map[string]dto.SurveyCompletionTeamDTO{}
		for _, t := range mid.DirectTeams {
			byID[t.TeamID] = t
		}
		Expect(byID["pod-nested-not-started"].Status).To(Equal("not_started"))
		Expect(byID["pod-nested-opted-out"].Status).To(Equal("opted_out"))
	})

	It("counts exactly two not_started and two opted_out pods overall, with no overlap", func() {
		Expect(resp.NotStarted).To(Equal(2))
		Expect(resp.OptedOut).To(Equal(2))
	})
})

// Regression test for issue #143's status rule: a pod isn't "complete" from
// individual responses alone -- the team's post-workshop session must also
// be completed. This is the exact scenario the original implementation got
// wrong (deriving "complete" purely from Completed >= Total).
var _ = Describe("buildSurveyCompletionResponse post-workshop completion rule", func() {
	persons := []queries.PersonNode{
		{UserID: "leader-1", Name: "Leader One", LevelID: "manager", LevelName: "Manager", ParentID: ""},
	}

	overview := &queries.SurveyCompletionOverview{
		Persons: persons,
		Teams: []queries.SurveyCompletionTeam{
			// Every eligible member finished their individual survey, but the
			// team hasn't held its post-workshop session yet.
			{TeamID: "pod-individuals-only", TeamName: "Individuals Only", OwnerUserID: "leader-1", Total: 3, Completed: 3, PostWorkshopCompleted: false, HealthCheckEnabled: true},
			// Same individual completion, but the post-workshop session is
			// also done -- this is the only one of the three that should
			// read "complete".
			{TeamID: "pod-fully-done", TeamName: "Fully Done", OwnerUserID: "leader-1", Total: 3, Completed: 3, PostWorkshopCompleted: true, HealthCheckEnabled: true},
			// Post-workshop marked done, but individuals aren't -- can't be
			// "complete" either; still reads as ordinary partial progress.
			{TeamID: "pod-partial-with-workshop", TeamName: "Partial With Workshop", OwnerUserID: "leader-1", Total: 3, Completed: 1, PostWorkshopCompleted: true, HealthCheckEnabled: true},
		},
	}

	resp := buildSurveyCompletionResponse(overview)

	byID := map[string]dto.SurveyCompletionTeamDTO{}
	for _, t := range resp.Groups[0].DirectTeams {
		byID[t.TeamID] = t
	}

	It("keeps a pod in_progress when every individual response is in but the post-workshop session isn't done", func() {
		Expect(byID["pod-individuals-only"].Status).To(Equal("in_progress"))
	})

	It("only reports complete once both individual responses AND the post-workshop session are done", func() {
		Expect(byID["pod-fully-done"].Status).To(Equal("complete"))
	})

	It("stays in_progress when the post-workshop session is done but individual responses are still partial", func() {
		Expect(byID["pod-partial-with-workshop"].Status).To(Equal("in_progress"))
	})

	It("never counts an individuals-only pod as fully complete in the overall tally", func() {
		Expect(resp.FullyComplete).To(Equal(1)) // only pod-fully-done
		Expect(resp.InProgress).To(Equal(2))    // pod-individuals-only, pod-partial-with-workshop
	})
})

// Regression test: a leader with no children and one with no direct teams
// must still serialize "children"/"directTeams" as [], not omit the key
// entirely. encoding/json's `omitempty` drops empty slices from the JSON
// altogether, which crashed the frontend the first time an equivalent bug
// shipped — asserting on the marshaled JSON, not just the Go struct, is
// what would have caught it.
var _ = Describe("buildSurveyCompletionResponse JSON encoding of empty arrays", func() {
	overview := &queries.SurveyCompletionOverview{
		Persons: []queries.PersonNode{
			{UserID: "leaf", Name: "Leaf Leader", LevelID: "manager", LevelName: "Manager", ParentID: ""},
			{UserID: "pass-through", Name: "Pass Through Leader", LevelID: "director", LevelName: "Director", ParentID: ""},
			{UserID: "only-child", Name: "Only Child", LevelID: "manager", LevelName: "Manager", ParentID: "pass-through"},
		},
		Teams: []queries.SurveyCompletionTeam{
			{TeamID: "G", TeamName: "Team G", OwnerUserID: "leaf", Total: 1, Completed: 1, HealthCheckEnabled: true},
			{TeamID: "H", TeamName: "Team H", OwnerUserID: "only-child", Total: 1, Completed: 1, HealthCheckEnabled: true},
		},
	}

	resp := buildSurveyCompletionResponse(overview)

	It("marshals an empty children array, not a missing or null key, for a leaf leader", func() {
		raw, err := json.Marshal(resp.Groups[0])
		Expect(err).NotTo(HaveOccurred())

		var decoded map[string]json.RawMessage
		Expect(json.Unmarshal(raw, &decoded)).To(Succeed())
		Expect(decoded).To(HaveKey("children"))
		Expect(string(decoded["children"])).To(Equal("[]"))
	})

	It("marshals an empty directTeams array, not a missing or null key, for a pass-through leader with no direct pods", func() {
		raw, err := json.Marshal(resp.Groups[1])
		Expect(err).NotTo(HaveOccurred())

		var decoded map[string]json.RawMessage
		Expect(json.Unmarshal(raw, &decoded)).To(Succeed())
		Expect(decoded).To(HaveKey("directTeams"))
		Expect(string(decoded["directTeams"])).To(Equal("[]"))
	})
})

// Regression coverage for the reported "pods displayed twice" bug, using
// the exact pod names from that report. Spread across every placement the
// canonical ownership rules define: direct pods under a Level-2 root,
// pods under a Level-3 manager nested under that root, pods under a
// standalone top-level Level-3 manager (no resolvable Level-2 parent), and
// pods with no resolvable owner at all (Other).
var _ = Describe("buildSurveyCompletionResponse with the reported duplicate-pod names", func() {
	persons := []queries.PersonNode{
		{UserID: "l2-root", Name: "L2 Root", LevelID: "level-2", LevelName: "Director", ParentID: ""},
		{UserID: "mgr-a", Name: "Manager A", LevelID: "level-3", LevelName: "Manager", ParentID: "l2-root"},
		{UserID: "mgr-b", Name: "Manager B", LevelID: "level-3", LevelName: "Manager", ParentID: ""},
	}

	// Pods with Completed == Total also carry PostWorkshopCompleted: true so
	// they land in the "complete" status (see deriveSurveyCompletionStatus's
	// issue #143 rule) rather than "in_progress" — matching this fixture's
	// own "complete" comments and the RemindCount math below.
	reportedTeams := []queries.SurveyCompletionTeam{
		// Direct under the Level-2 root.
		{TeamID: "t-andalusia", TeamName: "Andalusia", OwnerUserID: "l2-root", Total: 2, Completed: 2, PostWorkshopCompleted: true, HealthCheckEnabled: true},
		{TeamID: "t-bobcaygeon", TeamName: "Bobcaygeon", OwnerUserID: "l2-root", Total: 2, Completed: 1, HealthCheckEnabled: true},
		{TeamID: "t-biala", TeamName: "biala", OwnerUserID: "l2-root", Total: 1, Completed: 0, HealthCheckEnabled: true},
		{TeamID: "t-bay-view", TeamName: "bay view", OwnerUserID: "l2-root", Total: 3, Completed: 3, PostWorkshopCompleted: true, HealthCheckEnabled: true},
		// Under Manager A, nested under the Level-2 root.
		{TeamID: "t-biztech", TeamName: "BizTech-Salesforce-Team", OwnerUserID: "mgr-a", Total: 4, Completed: 4, PostWorkshopCompleted: true, HealthCheckEnabled: true},
		{TeamID: "t-anekal", TeamName: "Anekal", OwnerUserID: "mgr-a", Total: 2, Completed: 0, HealthCheckEnabled: true},
		{TeamID: "t-bolinas", TeamName: "Bolinas", OwnerUserID: "mgr-a", Total: 2, Completed: 1, HealthCheckEnabled: true},
		{TeamID: "t-big-sur", TeamName: "big sur", OwnerUserID: "mgr-a", Total: 1, Completed: 1, PostWorkshopCompleted: true, HealthCheckEnabled: true},
		// Under Manager B, a standalone top-level manager (no Level-2 parent).
		{TeamID: "t-aurora", TeamName: "Aurora", OwnerUserID: "mgr-b", Total: 3, Completed: 3, PostWorkshopCompleted: true, HealthCheckEnabled: true},
		{TeamID: "t-alleppey", TeamName: "Alleppey", OwnerUserID: "mgr-b", Total: 2, Completed: 0, HealthCheckEnabled: true},
		{TeamID: "t-bombay", TeamName: "BOMBAY", OwnerUserID: "mgr-b", Total: 1, Completed: 1, PostWorkshopCompleted: true, HealthCheckEnabled: true},
		{TeamID: "t-bray", TeamName: "bray", OwnerUserID: "mgr-b", Total: 2, Completed: 1, HealthCheckEnabled: true},
		// No resolvable owner at all -> Other.
		{TeamID: "t-atlanata", TeamName: "atlanata", OwnerUserID: "", TeamLeadID: "tl-1", TeamLeadName: "Lead One", Total: 1, Completed: 0, HealthCheckEnabled: true},
		{TeamID: "t-belfast", TeamName: "belfast", OwnerUserID: "", TeamLeadID: "tl-2", TeamLeadName: "Lead Two", Total: 1, Completed: 1, PostWorkshopCompleted: true, HealthCheckEnabled: true},
		{TeamID: "t-bandipur", TeamName: "bandipur", OwnerUserID: "", TeamLeadID: "tl-3", TeamLeadName: "Lead Three", Total: 1, Completed: 0, HealthCheckEnabled: true},
	}

	overview := &queries.SurveyCompletionOverview{Persons: persons, Teams: reportedTeams}
	resp := buildSurveyCompletionResponse(overview)

	expectedOwner := map[string]string{
		"t-andalusia": "L2 Root", "t-bobcaygeon": "L2 Root", "t-biala": "L2 Root", "t-bay-view": "L2 Root",
		"t-biztech": "Manager A", "t-anekal": "Manager A", "t-bolinas": "Manager A", "t-big-sur": "Manager A",
		"t-aurora": "Manager B", "t-alleppey": "Manager B", "t-bombay": "Manager B", "t-bray": "Manager B",
		"t-atlanata": "Other", "t-belfast": "Other", "t-bandipur": "Other",
	}

	It("renders every one of the 15 reported pods exactly once, under its canonical owner", func() {
		occurrences := map[string]int{}
		owners := map[string]string{}

		var walk func(g dto.SurveyCompletionGroupDTO, ownerName string)
		walk = func(g dto.SurveyCompletionGroupDTO, ownerName string) {
			for _, t := range g.DirectTeams {
				occurrences[t.TeamID]++
				owners[t.TeamID] = ownerName
			}
			for _, t := range g.Teams {
				occurrences[t.TeamID]++
				owners[t.TeamID] = "Other"
			}
			for _, c := range g.Children {
				childOwner := ownerName
				if c.Person != nil {
					childOwner = c.Person.Name
				}
				walk(c, childOwner)
			}
		}
		for _, g := range resp.Groups {
			rootOwner := g.Label
			if g.Person != nil {
				rootOwner = g.Person.Name
			}
			walk(g, rootOwner)
		}

		Expect(occurrences).To(HaveLen(len(reportedTeams)), "every reported pod must appear -- none missing, none extra")
		for id, want := range expectedOwner {
			Expect(occurrences[id]).To(Equal(1), "pod %s must appear exactly once, appeared %d times", id, occurrences[id])
			Expect(owners[id]).To(Equal(want), "pod %s must be owned by %q, was owned by %q", id, want, owners[id])
		}
	})

	It("never lets a pod exist simultaneously in a direct list and a nested manager list", func() {
		// Every pod owned by mgr-a must appear ONLY inside Manager A's own
		// directTeams (nested under the root), never also in the root's own
		// directTeams or in Manager B's subtree.
		l2Root := resp.Groups[0]
		Expect(l2Root.Person.Name).To(Equal("L2 Root"))
		rootDirectIDs := map[string]bool{}
		for _, t := range l2Root.DirectTeams {
			rootDirectIDs[t.TeamID] = true
		}
		Expect(rootDirectIDs).To(HaveKey("t-andalusia"))
		Expect(rootDirectIDs).NotTo(HaveKey("t-biztech"), "Manager A's pod must not also appear directly under the root")

		managerA := l2Root.Children[0]
		Expect(managerA.Person.Name).To(Equal("Manager A"))
		managerADirectIDs := map[string]bool{}
		for _, t := range managerA.DirectTeams {
			managerADirectIDs[t.TeamID] = true
		}
		Expect(managerADirectIDs).To(HaveKey("t-biztech"))
		Expect(managerADirectIDs).NotTo(HaveKey("t-andalusia"), "the root's own direct pod must not leak into Manager A's list")
	})

	It("does not double-count any reported pod in group totals or remind counts", func() {
		l2Root := resp.Groups[0]
		// Root subtree = 4 direct (andalusia, bobcaygeon, biala, bay view) +
		// Manager A's 4 (biztech, anekal, bolinas, big sur) = 8 total, each
		// counted exactly once.
		Expect(l2Root.TotalTeams).To(Equal(8))
		// Remindable (in_progress/not_started, opted-in): bobcaygeon(1/2),
		// biala(0/1), anekal(0/2), bolinas(1/2) = 4.
		Expect(l2Root.RemindCount).To(Equal(4))

		managerB := resp.Groups[1]
		Expect(managerB.Person.Name).To(Equal("Manager B"))
		Expect(managerB.TotalTeams).To(Equal(4))
		Expect(managerB.RemindCount).To(Equal(2)) // alleppey(0/2), bray(1/2)

		other := resp.Groups[2]
		Expect(other.Label).To(Equal("Other"))
		Expect(other.TotalTeams).To(Equal(3))
		Expect(other.RemindCount).To(Equal(2)) // atlanata, bandipur
	})
})

// Regression test for the specific root cause this bug most likely traces
// to: two DATABASE rows that are genuinely different teams (different
// canonical TeamID) but happen to share the exact same display name
// ("Aurora" -- one of the reported pods). Per the canonical ownership
// rules, these must never be merged or collapsed into one row -- both
// render, separately, each under its own correct owner.
var _ = Describe("buildSurveyCompletionResponse with two different team IDs sharing the same name", func() {
	persons := []queries.PersonNode{
		{UserID: "mgr-a", Name: "Manager A", LevelID: "level-3", LevelName: "Manager", ParentID: ""},
		{UserID: "mgr-b", Name: "Manager B", LevelID: "level-3", LevelName: "Manager", ParentID: ""},
	}
	overview := &queries.SurveyCompletionOverview{
		Persons: persons,
		Teams: []queries.SurveyCompletionTeam{
			{TeamID: "aurora-1", TeamName: "Aurora", OwnerUserID: "mgr-a", Total: 2, Completed: 2, PostWorkshopCompleted: true, HealthCheckEnabled: true},
			{TeamID: "aurora-2", TeamName: "Aurora", OwnerUserID: "mgr-b", Total: 3, Completed: 0, HealthCheckEnabled: true},
		},
	}

	resp := buildSurveyCompletionResponse(overview)

	It("renders both same-named teams separately, each exactly once under its own owner, never merged", func() {
		Expect(resp.Groups).To(HaveLen(2))

		managerA := resp.Groups[0]
		Expect(managerA.Person.Name).To(Equal("Manager A"))
		Expect(managerA.DirectTeams).To(HaveLen(1))
		Expect(managerA.DirectTeams[0].TeamID).To(Equal("aurora-1"))
		Expect(managerA.DirectTeams[0].TeamName).To(Equal("Aurora"))
		Expect(managerA.DirectTeams[0].Status).To(Equal("complete"))

		managerB := resp.Groups[1]
		Expect(managerB.Person.Name).To(Equal("Manager B"))
		Expect(managerB.DirectTeams).To(HaveLen(1))
		Expect(managerB.DirectTeams[0].TeamID).To(Equal("aurora-2"))
		Expect(managerB.DirectTeams[0].TeamName).To(Equal("Aurora"))
		Expect(managerB.DirectTeams[0].Status).To(Equal("not_started"))
	})
})

// Regression test proving the seenTeamIDs guard in
// buildSurveyCompletionResponse actually holds: even if the SAME TeamID
// were somehow handed to it twice (e.g. a future upstream regression), the
// response must still show it exactly once rather than doubling its count
// or rendering it in two places.
var _ = Describe("buildSurveyCompletionResponse never lets one team ID render twice", func() {
	persons := []queries.PersonNode{
		{UserID: "mgr-a", Name: "Manager A", LevelID: "level-3", LevelName: "Manager", ParentID: ""},
	}
	overview := &queries.SurveyCompletionOverview{
		Persons: persons,
		Teams: []queries.SurveyCompletionTeam{
			{TeamID: "dup-1", TeamName: "Duplicate Pod", OwnerUserID: "mgr-a", Total: 4, Completed: 2, HealthCheckEnabled: true},
			// Same TeamID handed to the builder a second time -- must not
			// double-render or double-count, no matter how it got here.
			{TeamID: "dup-1", TeamName: "Duplicate Pod", OwnerUserID: "mgr-a", Total: 4, Completed: 2, HealthCheckEnabled: true},
		},
	}

	resp := buildSurveyCompletionResponse(overview)

	It("counts the repeated team ID only once in TotalTeams and in the owner's DirectTeams", func() {
		Expect(resp.TotalTeams).To(Equal(1))
		Expect(resp.Groups).To(HaveLen(1))
		Expect(resp.Groups[0].DirectTeams).To(HaveLen(1))
		Expect(resp.Groups[0].TotalTeams).To(Equal(1))
	})
})
