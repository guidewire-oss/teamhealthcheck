package v1

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agopalakrishnan/teams360/backend/application/queries"
)

var _ = Describe("buildSurveyCompletionResponse", func() {
	// One team of each kind described in the requirements:
	//   A: direct team under a director (no manager)
	//   B, C: teams under two managers who both report to the same director
	//   D: a manager-only team (no resolvable director) -> top-level manager group
	//   E: neither director nor manager -> "Other", escalated to the team lead
	//   F: opted out, but still a direct team under the director
	overview := &queries.SurveyCompletionOverview{
		Teams: []queries.SurveyCompletionTeam{
			{TeamID: "A", TeamName: "Team A", DirectorID: "dir1", DirectorName: "Dana Director", TeamLeadID: "leadA", TeamLeadName: "Ann Lead", Total: 5, Completed: 5, HealthCheckEnabled: true},
			{TeamID: "B", TeamName: "Team B", DirectorID: "dir1", DirectorName: "Dana Director", ManagerID: "mgr1", ManagerName: "Zed Manager", TeamLeadID: "leadB", TeamLeadName: "Bo Lead", Total: 4, Completed: 2, HealthCheckEnabled: true},
			{TeamID: "C", TeamName: "Team C", DirectorID: "dir1", DirectorName: "Dana Director", ManagerID: "mgr2", ManagerName: "Amy Manager", TeamLeadID: "leadC", TeamLeadName: "Cy Lead", Total: 3, Completed: 0, HealthCheckEnabled: true},
			{TeamID: "D", TeamName: "Team D", ManagerID: "mgr3", ManagerName: "Mo Manager", TeamLeadID: "leadD", TeamLeadName: "Dee Lead", Total: 2, Completed: 2, HealthCheckEnabled: true},
			{TeamID: "E", TeamName: "Team E", TeamLeadID: "lead1", TeamLeadName: "Lee Lead", Total: 6, Completed: 1, HealthCheckEnabled: true},
			{TeamID: "F", TeamName: "Team F", DirectorID: "dir1", DirectorName: "Dana Director", Total: 1, Completed: 0, HealthCheckEnabled: false},
		},
	}

	resp := buildSurveyCompletionResponse(overview)

	It("classifies overall counts across all six teams", func() {
		Expect(resp.TotalTeams).To(Equal(6))
		Expect(resp.OptedOut).To(Equal(1))  // F
		Expect(resp.OptedIn).To(Equal(5))   // A,B,C,D,E
		Expect(resp.FullyComplete).To(Equal(2)) // A,D
		Expect(resp.InProgress).To(Equal(2))    // B,E
		Expect(resp.NotStarted).To(Equal(1))    // C
	})

	It("produces exactly one director group, one top-level manager group, and Other, in that order", func() {
		Expect(resp.Groups).To(HaveLen(3))
		Expect(resp.Groups[0].Type).To(Equal("director"))
		Expect(resp.Groups[1].Type).To(Equal("manager"))
		Expect(resp.Groups[2].Type).To(Equal("other"))
	})

	It("nests the director's direct teams and its managers, sorted alphabetically by manager name", func() {
		dir := resp.Groups[0]
		Expect(dir.Director.ID).To(Equal("dir1"))
		Expect(dir.DirectTeams).To(HaveLen(2)) // A and F
		Expect(dir.Managers).To(HaveLen(2))
		Expect(dir.Managers[0].Manager.Name).To(Equal("Amy Manager")) // mgr2, alphabetically first
		Expect(dir.Managers[1].Manager.Name).To(Equal("Zed Manager")) // mgr1
		Expect(dir.Managers[0].Teams).To(HaveLen(1))
		Expect(dir.Managers[0].Teams[0].TeamID).To(Equal("C"))
		Expect(dir.Managers[1].Teams[0].TeamID).To(Equal("B"))

		// Director rollup covers direct teams + every nested manager's teams,
		// excluding the opted-out one (F).
		Expect(dir.TotalTeams).To(Equal(4))
		Expect(dir.OptedInTeams).To(Equal(3))
		Expect(dir.RemindCount).To(Equal(2)) // B (in_progress), C (not_started)
	})

	It("puts a manager with no resolvable director in its own top-level group", func() {
		mgr := resp.Groups[1]
		Expect(mgr.Manager.ID).To(Equal("mgr3"))
		Expect(mgr.Teams).To(HaveLen(1))
		Expect(mgr.Teams[0].TeamID).To(Equal("D"))
		Expect(mgr.TotalTeams).To(Equal(1))
		Expect(mgr.RemindCount).To(Equal(0))
	})

	It("routes a team with neither director nor manager to Other, escalated to its team lead", func() {
		other := resp.Groups[2]
		Expect(other.Label).To(Equal("Other"))
		Expect(other.Teams).To(HaveLen(1))
		team := other.Teams[0]
		Expect(team.TeamID).To(Equal("E"))
		Expect(team.TeamLeadID).NotTo(BeNil())
		Expect(*team.TeamLeadID).To(Equal("lead1"))
		Expect(team.TeamLeadName).NotTo(BeNil())
		Expect(*team.TeamLeadName).To(Equal("Lee Lead"))
	})

	It("sets each pod's team-lead name everywhere, not just in the Other group", func() {
		// The reminder-target tree needs a team lead label on every pod leaf,
		// so TeamLeadID/Name travels with the team regardless of which group
		// (director-direct, manager, or Other) it lands in.
		director := resp.Groups[0]
		Expect(*director.DirectTeams[0].TeamLeadName).To(Equal("Ann Lead")) // A
		Expect(*director.Managers[0].Teams[0].TeamLeadName).To(Equal("Cy Lead")) // C, under Amy Manager
		Expect(*director.Managers[1].Teams[0].TeamLeadName).To(Equal("Bo Lead")) // B, under Zed Manager

		topManager := resp.Groups[1]
		Expect(*topManager.Teams[0].TeamLeadName).To(Equal("Dee Lead")) // D
	})
})

// Regression test: a director with no managers (all teams direct) and a
// director with no direct teams (all teams under a manager) must still
// serialize "managers"/"directTeams" as [], not omit the key entirely.
// encoding/json's `omitempty` drops empty slices from the JSON altogether,
// which crashed the frontend (`group.managers.flatMap` on undefined) the
// first time this shipped — asserting on the marshaled JSON, not just the
// Go struct, is what would have caught it.
var _ = Describe("buildSurveyCompletionResponse JSON encoding of empty child arrays", func() {
	overview := &queries.SurveyCompletionOverview{
		Teams: []queries.SurveyCompletionTeam{
			// dir-only: a director with only a direct team, no managers.
			{TeamID: "G", TeamName: "Team G", DirectorID: "dir2", DirectorName: "Gail Director", Total: 1, Completed: 1, HealthCheckEnabled: true},
			// mgr-only-under-director: a director with only a manager, no direct teams.
			{TeamID: "H", TeamName: "Team H", DirectorID: "dir3", DirectorName: "Hana Director", ManagerID: "mgr4", ManagerName: "Nia Manager", Total: 1, Completed: 1, HealthCheckEnabled: true},
		},
	}

	resp := buildSurveyCompletionResponse(overview)

	It("marshals an empty managers array, not a missing or null key, for a director with no managers", func() {
		raw, err := json.Marshal(resp.Groups[0])
		Expect(err).NotTo(HaveOccurred())

		var decoded map[string]json.RawMessage
		Expect(json.Unmarshal(raw, &decoded)).To(Succeed())
		Expect(decoded).To(HaveKey("managers"))
		Expect(string(decoded["managers"])).To(Equal("[]"))
	})

	It("marshals an empty directTeams array, not a missing or null key, for a director with no direct teams", func() {
		raw, err := json.Marshal(resp.Groups[1])
		Expect(err).NotTo(HaveOccurred())

		var decoded map[string]json.RawMessage
		Expect(json.Unmarshal(raw, &decoded)).To(Succeed())
		Expect(decoded).To(HaveKey("directTeams"))
		Expect(string(decoded["directTeams"])).To(Equal("[]"))
	})
})
