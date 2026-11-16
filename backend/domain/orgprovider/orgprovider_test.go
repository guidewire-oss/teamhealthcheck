package orgprovider_test

import (
	"errors"
	"testing"

	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
)

func TestIsProtectedUser(t *testing.T) {
	cases := []struct {
		id, level string
		want      bool
	}{
		{"admin", "level-admin", true},
		{"admin", "", true},
		{"someone-else", "level-admin", true},
		{"demo", "level-5", true},
		{"vp", "level-1", true},
		{"e2e_fresh_member", "level-5", true},
		{"00ut0p2rk6RjALPhB0x7", "level-5", false},
		{"jau", "level-5", false},
	}
	for _, c := range cases {
		if got := orgprovider.IsProtectedUser(c.id, c.level); got != c.want {
			t.Errorf("IsProtectedUser(%q, %q) = %v, want %v", c.id, c.level, got, c.want)
		}
	}
}

func TestIsProtectedTeam(t *testing.T) {
	if !orgprovider.IsProtectedTeam("team-phoenix") {
		t.Error("team-phoenix must be protected")
	}
	if orgprovider.IsProtectedTeam("b35bcc94-95d7-48de-91cd-24f4fd3a1ff6") {
		t.Error("a real provider team id must not be protected")
	}
}

func TestEvaluateMassDeletionGuard(t *testing.T) {
	if err := orgprovider.EvaluateMassDeletionGuard(100, 10, 50, 5, 20); err != nil {
		t.Errorf("10%% deletion under a 20%% threshold should pass, got %v", err)
	}
	if err := orgprovider.EvaluateMassDeletionGuard(100, 21, 50, 0, 20); !errors.Is(err, orgprovider.ErrMassDeletionBlocked) {
		t.Errorf("21%% user deletion over a 20%% threshold should block, got %v", err)
	}
	if err := orgprovider.EvaluateMassDeletionGuard(100, 0, 50, 11, 20); !errors.Is(err, orgprovider.ErrMassDeletionBlocked) {
		t.Errorf("22%% team deletion over a 20%% threshold should block, got %v", err)
	}
	if err := orgprovider.EvaluateMassDeletionGuard(0, 0, 0, 0, 20); err != nil {
		t.Errorf("no current records and no deletions should never block, got %v", err)
	}
	if err := orgprovider.EvaluateMassDeletionGuard(0, 1, 10, 0, 20); !errors.Is(err, orgprovider.ErrMassDeletionBlocked) {
		t.Errorf("deleting from a zero-current population should be treated as unsafe, got %v", err)
	}
}

func TestMassDeletionHoldErrorCarriesTheGuardsOwnNumbers(t *testing.T) {
	err := orgprovider.EvaluateMassDeletionGuard(100, 25, 50, 5, 20)

	var hold *orgprovider.MassDeletionHoldError
	if !errors.As(err, &hold) {
		t.Fatalf("a tripped guard must return *MassDeletionHoldError, got %T", err)
	}
	if !errors.Is(err, orgprovider.ErrMassDeletionBlocked) {
		t.Error("the typed error must still satisfy errors.Is(ErrMassDeletionBlocked) for existing callers")
	}

	users := hold.Report.Users
	if users.Existing != 100 || users.Deleting != 25 {
		t.Errorf("expected 25 of 100 users, got %d of %d", users.Deleting, users.Existing)
	}
	if users.Percent != 25 {
		t.Errorf("expected 25%% users, got %v", users.Percent)
	}
	if !users.ExceedsThreshold || !users.ContributesToHold {
		t.Error("the user metric drove this hold and must say so")
	}

	teams := hold.Report.Teams
	if teams.Existing != 50 || teams.Deleting != 5 || teams.Percent != 10 {
		t.Errorf("expected 5 of 50 teams at 10%%, got %d of %d at %v", teams.Deleting, teams.Existing, teams.Percent)
	}
	if teams.ExceedsThreshold {
		t.Error("10%% teams is under the 20%% threshold and must not be marked as exceeding it")
	}
	if hold.Report.Threshold != 20 {
		t.Errorf("expected the configured threshold to be reported, got %v", hold.Report.Threshold)
	}
	if hold.Report.Memberships != nil {
		t.Error("the domain guard measures no memberships; persistence attaches that metric")
	}
}

func TestBuildMassDeletionReportDoesNotDecide(t *testing.T) {
	safe := orgprovider.BuildMassDeletionReport(100, 10, 50, 5, 20)
	if safe.Held() {
		t.Error("10%% of users and teams is under a 20%% threshold and must not be held")
	}

	// A zero population with deletions is treated as 100%, matching the
	// guard's own "treat as unsafe" rule rather than dividing by zero.
	edge := orgprovider.BuildMassDeletionReport(0, 1, 10, 0, 20)
	if edge.Users.Percent != 100 || !edge.Held() {
		t.Errorf("deleting from an empty population must read as 100%% and hold, got %v", edge.Users.Percent)
	}
}

func TestMembershipMetricNeverContributesToHold(t *testing.T) {
	// A cascade metric is informational: even at 100% it must not hold a sync.
	metric := orgprovider.NewDeletionMetric(orgprovider.DeletionKindCascaded, 10, 10, 20, false)
	if metric.Kind != orgprovider.DeletionKindCascaded {
		t.Errorf("expected a cascaded metric, got %q", metric.Kind)
	}
	if !metric.ExceedsThreshold {
		t.Error("100%% is over a 20%% threshold and the raw comparison should say so")
	}
	if metric.ContributesToHold {
		t.Error("a cascade metric must never contribute to the hold")
	}

	report := orgprovider.BuildMassDeletionReport(100, 1, 50, 1, 20)
	report.Memberships = &metric
	if report.Held() {
		t.Error("an over-threshold membership cascade must not turn a safe sync into a held one")
	}
}

func TestWithIncomingReportsPayloadSizeWithoutChangingTheVerdict(t *testing.T) {
	// The shape that makes a hold confusing in practice: every user came back
	// from the provider, but almost no teams did. Users are untouched while
	// teams look catastrophic -- and the incoming counts are what tell those
	// two situations apart.
	report := orgprovider.BuildMassDeletionReport(240, 0, 243, 237, 20)
	before := report

	report.WithIncoming(240, 6, 0)

	if report.Users.Incoming != 240 || report.Teams.Incoming != 6 {
		t.Errorf("incoming counts were not recorded: users=%d teams=%d", report.Users.Incoming, report.Teams.Incoming)
	}
	// 237/243 is the arithmetic, and it must be untouched by the reporting.
	if report.Teams.Percent != before.Teams.Percent || report.Teams.Percent < 97.5 || report.Teams.Percent > 97.6 {
		t.Errorf("expected teams at ~97.5%%, got %v", report.Teams.Percent)
	}
	if report.Users.Percent != 0 || report.Users.ExceedsThreshold {
		t.Error("no user deletions means the user metric is at 0%% and cannot exceed the threshold")
	}
	if !report.Held() {
		t.Error("teams alone at 97.5%% must still hold the sync")
	}

	// A nil membership metric must not panic.
	report.Memberships = nil
	report.WithIncoming(1, 2, 3)
}

// TestMassDeletionReportMatchesConfirmed pins down the fix for the TOCTOU gap
// between a held sync's response and an admin's override: every sync
// re-fetches its own fresh snapshot, so the override must be bound to the
// exact counts reviewed, not just a bare flag.
func TestMassDeletionReportMatchesConfirmed(t *testing.T) {
	report := orgprovider.BuildMassDeletionReport(20, 6, 10, 3, 20)

	exact := &orgprovider.ConfirmedMassDeletion{UsersExisting: 20, UsersDeleting: 6, TeamsExisting: 10, TeamsDeleting: 3}
	if !report.MatchesConfirmed(exact) {
		t.Error("a confirmation with the report's own counts must match")
	}

	if report.MatchesConfirmed(nil) {
		t.Error("a nil confirmation must never match -- an override that omitted it must be refused, not silently allowed")
	}

	cases := []struct {
		name      string
		confirmed orgprovider.ConfirmedMassDeletion
	}{
		{"stale usersDeleting", orgprovider.ConfirmedMassDeletion{UsersExisting: 20, UsersDeleting: 5, TeamsExisting: 10, TeamsDeleting: 3}},
		{"stale usersExisting", orgprovider.ConfirmedMassDeletion{UsersExisting: 19, UsersDeleting: 6, TeamsExisting: 10, TeamsDeleting: 3}},
		{"stale teamsDeleting", orgprovider.ConfirmedMassDeletion{UsersExisting: 20, UsersDeleting: 6, TeamsExisting: 10, TeamsDeleting: 2}},
		{"stale teamsExisting", orgprovider.ConfirmedMassDeletion{UsersExisting: 20, UsersDeleting: 6, TeamsExisting: 11, TeamsDeleting: 3}},
	}
	for _, c := range cases {
		confirmed := c.confirmed
		if report.MatchesConfirmed(&confirmed) {
			t.Errorf("%s: a confirmation that no longer matches the fresh report must be refused", c.name)
		}
	}
}
