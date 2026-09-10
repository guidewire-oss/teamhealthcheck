package queries

import (
	"testing"

	"github.com/agopalakrishnan/teams360/backend/domain/healthcheck"
	"github.com/agopalakrishnan/teams360/backend/domain/team"
	"github.com/agopalakrishnan/teams360/backend/domain/user"
)

func strPtr(s string) *string { return &s }

func TestIsAboveLeadershipFloor(t *testing.T) {
	positionByLevelID := map[string]int{
		"vp":              1,
		"senior-director": 2,
		"director":        2,
		"senior-manager":  3,
		"manager":         3,
	}

	tests := []struct {
		name    string
		levelID string
		want    bool
	}{
		{"VP is above the floor", "vp", true},
		{"Senior Director sits at the floor, not above it", "senior-director", false},
		{"Director shares the floor position with Senior Director", "director", false},
		{"Senior Manager is below the floor", "senior-manager", false},
		{"an unknown level id is treated as above the floor", "unknown", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAboveLeadershipFloor(tt.levelID, positionByLevelID); got != tt.want {
				t.Errorf("isAboveLeadershipFloor(%q) = %v, want %v", tt.levelID, got, tt.want)
			}
		})
	}
}

// TestResolveParentID exercises the single rule that makes the hierarchy
// recursive to arbitrary depth: a person's parent is whoever reports_to
// points at, as long as that target exists and sits at or below
// leadershipFloorPosition — applied identically whether the person is at
// the very top tier or several tiers down, and regardless of whether the
// two people happen to share the same numeric position (e.g. "Senior
// Director" and "Director" both at position 2).
func TestResolveParentID(t *testing.T) {
	positionByLevelID := map[string]int{
		"vp":              1,
		"senior-director": 2,
		"director":        2,
		"senior-manager":  3,
		"manager":         3,
	}

	vp := &user.User{ID: "vp-1", Name: "Val President", HierarchyLevelID: "vp"}
	seniorDirector := &user.User{ID: "sd-1", Name: "Sofia Director", HierarchyLevelID: "senior-director", ReportsTo: strPtr("vp-1")}
	director := &user.User{ID: "d-1", Name: "Dana Director", HierarchyLevelID: "director", ReportsTo: strPtr("sd-1")}
	seniorManager := &user.User{ID: "sm-1", Name: "Sam Manager", HierarchyLevelID: "senior-manager", ReportsTo: strPtr("sd-1")}
	manager := &user.User{ID: "m-1", Name: "Mo Manager", HierarchyLevelID: "manager", ReportsTo: strPtr("d-1")}
	orphanManager := &user.User{ID: "m-2", Name: "Ori Manager", HierarchyLevelID: "manager", ReportsTo: strPtr("nonexistent")}
	noReportsTo := &user.User{ID: "sd-2", Name: "Solo Director", HierarchyLevelID: "senior-director"}

	usersByID := map[string]*user.User{
		vp.ID: vp, seniorDirector.ID: seniorDirector, director.ID: director,
		seniorManager.ID: seniorManager, manager.ID: manager,
		orphanManager.ID: orphanManager, noReportsTo.ID: noReportsTo,
	}

	tests := []struct {
		name string
		u    *user.User
		want string
	}{
		{"a Level-2 leader reporting to a VP has no valid parent (VP sits above the floor)", seniorDirector, ""},
		{"a Director reporting to a Senior Director nests under them, despite sharing a position", director, "sd-1"},
		{"a Senior Manager reports directly to a Senior Director, skipping the Director tier entirely", seniorManager, "sd-1"},
		{"a Manager reports to a Director", manager, "d-1"},
		{"a manager whose reports_to points at an unknown user has no valid parent", orphanManager, ""},
		{"a user with no reports_to at all has no valid parent", noReportsTo, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveParentID(tt.u, usersByID, positionByLevelID); got != tt.want {
				t.Errorf("resolveParentID(%s) = %q, want %q", tt.u.ID, got, tt.want)
			}
		})
	}
}

// TestEligibleTeamCompletion is the Individual Survey Completed column's
// own calculation: completed/total, both scoped to exactly the eligible
// (Level 4 + Level 5) user ids the caller passes in -- never inferred from
// names, titles, or any other field. Fixture user ids are generic
// placeholders (u1, u2, ...), never production names.
func TestEligibleTeamCompletion(t *testing.T) {
	nineEligible := []string{"u1", "u2", "u3", "u4", "u5", "u6", "u7", "u8", "u9"}

	t.Run("7 completed out of 9 eligible returns 7/9", func(t *testing.T) {
		completedUsers := map[string]bool{
			"u1": true, "u2": true, "u3": true, "u4": true, "u5": true, "u6": true, "u7": true,
		}
		completed, total := eligibleTeamCompletion(nineEligible, completedUsers)
		if completed != 7 || total != 9 {
			t.Fatalf("got %d/%d, want 7/9", completed, total)
		}
	})

	t.Run("0 completed out of 9 eligible returns 0/9, not blank", func(t *testing.T) {
		completed, total := eligibleTeamCompletion(nineEligible, map[string]bool{})
		if completed != 0 || total != 9 {
			t.Fatalf("got %d/%d, want 0/9", completed, total)
		}
	})

	t.Run("0 completed with a nil completed-users set still returns 0/9", func(t *testing.T) {
		// A team with zero completed sessions this period has no entry in
		// progress at all -- Handle() passes a nil map in that case.
		completed, total := eligibleTeamCompletion(nineEligible, nil)
		if completed != 0 || total != 9 {
			t.Fatalf("got %d/%d, want 0/9", completed, total)
		}
	})

	t.Run("9 completed out of 9 eligible returns 9/9", func(t *testing.T) {
		completedUsers := map[string]bool{}
		for _, uid := range nineEligible {
			completedUsers[uid] = true
		}
		completed, total := eligibleTeamCompletion(nineEligible, completedUsers)
		if completed != 9 || total != 9 {
			t.Fatalf("got %d/%d, want 9/9", completed, total)
		}
	})

	t.Run("a completed session from a non-eligible (e.g. manager) user is never counted", func(t *testing.T) {
		// "manager-1" completed a session for this team, but is NOT in the
		// eligible set (a Manager, not Level 4/5) -- their completion must
		// never inflate the count.
		completedUsers := map[string]bool{"u1": true, "manager-1": true}
		completed, total := eligibleTeamCompletion([]string{"u1", "u2"}, completedUsers)
		if completed != 1 || total != 2 {
			t.Fatalf("got %d/%d, want 1/2 (manager-1 excluded from both numerator and denominator)", completed, total)
		}
	})

	t.Run("duplicate ids in the eligible list (e.g. duplicate team membership) never inflate the denominator", func(t *testing.T) {
		duplicated := []string{"u1", "u1", "u2"}
		completed, total := eligibleTeamCompletion(duplicated, map[string]bool{"u1": true})
		if completed != 1 || total != 2 {
			t.Fatalf("got %d/%d, want 1/2", completed, total)
		}
	})

	t.Run("a pod with no eligible users returns 0/0, never blank", func(t *testing.T) {
		completed, total := eligibleTeamCompletion(nil, nil)
		if completed != 0 || total != 0 {
			t.Fatalf("got %d/%d, want 0/0", completed, total)
		}
	})
}

// TestBuildTeamProgress covers how a completed individual-survey session is
// associated with a user/team/completion state, and that duplicate
// responses for the same user never inflate the completed set (it's a
// map/set, not a count).
func TestBuildTeamProgress(t *testing.T) {
	t.Run("an incomplete session is never counted", func(t *testing.T) {
		sessions := []*healthcheck.HealthCheckSession{
			{TeamID: "pod-1", UserID: "u1", SurveyType: healthcheck.SurveyTypeIndividual, Completed: false},
		}
		progress := buildTeamProgress(sessions)
		if p, ok := progress["pod-1"]; ok && p.completedUsers["u1"] {
			t.Fatalf("an incomplete session must not mark u1 as completed")
		}
	})

	t.Run("a completed post_workshop session sets postWorkshop but never counts toward completedUsers", func(t *testing.T) {
		sessions := []*healthcheck.HealthCheckSession{
			{TeamID: "pod-1", UserID: "u1", SurveyType: healthcheck.SurveyTypePostWorkshop, Completed: true},
		}
		progress := buildTeamProgress(sessions)
		p, ok := progress["pod-1"]
		if !ok || !p.postWorkshop {
			t.Fatalf("expected postWorkshop=true for pod-1")
		}
		if p.completedUsers["u1"] {
			t.Fatalf("a post_workshop session must never count toward the individual completedUsers set")
		}
	})

	t.Run("duplicate completed individual sessions for the same user only count once", func(t *testing.T) {
		sessions := []*healthcheck.HealthCheckSession{
			{ID: "s1", TeamID: "pod-1", UserID: "u1", SurveyType: healthcheck.SurveyTypeIndividual, Completed: true},
			{ID: "s2", TeamID: "pod-1", UserID: "u1", SurveyType: healthcheck.SurveyTypeIndividual, Completed: true},
		}
		progress := buildTeamProgress(sessions)
		if len(progress["pod-1"].completedUsers) != 1 {
			t.Fatalf("got %d distinct completed users, want 1", len(progress["pod-1"].completedUsers))
		}
	})

	t.Run("sessions for different teams never mix", func(t *testing.T) {
		sessions := []*healthcheck.HealthCheckSession{
			{TeamID: "pod-1", UserID: "u1", SurveyType: healthcheck.SurveyTypeIndividual, Completed: true},
			{TeamID: "pod-2", UserID: "u2", SurveyType: healthcheck.SurveyTypeIndividual, Completed: true},
		}
		progress := buildTeamProgress(sessions)
		if progress["pod-1"].completedUsers["u2"] {
			t.Fatalf("pod-2's completion must not leak into pod-1")
		}
		if !progress["pod-1"].completedUsers["u1"] || !progress["pod-2"].completedUsers["u2"] {
			t.Fatalf("expected each pod to have its own user marked complete")
		}
	})
}

// TestCountDistinctEligible covers the trend chart's opted-in-members
// denominator, which must also never double-count a duplicated id.
func TestCountDistinctEligible(t *testing.T) {
	if got := countDistinctEligible([]string{"u1", "u2", "u1", "u3"}); got != 3 {
		t.Fatalf("got %d, want 3", got)
	}
	if got := countDistinctEligible(nil); got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
}

// TestBuildSurveyCompletionTrend_UsesEarliestCompletion is the regression
// test for the trend's "earliest completion" rule: a member with more than
// one completed individual session for the period (e.g. a resubmission)
// must be counted on the EARLIEST of those dates, never on whichever one
// happens to come first in the (unsorted) sessions slice.
//
// u2 completes in week 1 (2026-01-01), establishing an early anchor. u1
// completes twice: the LATE session (2026-01-22, three weeks later) is
// listed FIRST in the slice, and the EARLY session (2026-01-01, same week
// as u2) is listed SECOND. A first-encountered-wins implementation would
// credit u1's completion to week 4, spreading the trend across four weekly
// buckets that only reach 100% at the end; the correct, earliest-wins
// implementation credits u1 to week 1 alongside u2, producing a single
// point already at 100%.
func TestBuildSurveyCompletionTrend_UsesEarliestCompletion(t *testing.T) {
	teams := []team.SurveyCompletionRow{
		{ID: "team-1", HealthCheckEnabled: true},
	}
	eligibleMemberIDs := map[string][]string{"team-1": {"u1", "u2"}}
	sessions := []*healthcheck.HealthCheckSession{
		{TeamID: "team-1", UserID: "u1", SurveyType: healthcheck.SurveyTypeIndividual, Completed: true, Date: "2026-01-22T00:00:00Z"}, // late, listed first
		{TeamID: "team-1", UserID: "u1", SurveyType: healthcheck.SurveyTypeIndividual, Completed: true, Date: "2026-01-01T00:00:00Z"}, // early, listed second
		{TeamID: "team-1", UserID: "u2", SurveyType: healthcheck.SurveyTypeIndividual, Completed: true, Date: "2026-01-01T00:00:00Z"},
	}

	points := buildSurveyCompletionTrend(teams, eligibleMemberIDs, sessions)

	if len(points) != 1 {
		t.Fatalf("got %d points, want 1 (both members' completions fall in the same week once the earliest date wins): %+v", len(points), points)
	}
	if points[0].Completion != 100 {
		t.Fatalf("got %d%% at the only point, want 100%% (u1's earliest session must count, not its later resubmission)", points[0].Completion)
	}
}
