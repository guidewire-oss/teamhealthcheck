package commands

import (
	"context"
	"errors"
	"testing"

	"github.com/agopalakrishnan/teams360/backend/domain/healthcheck"
)

// fakeHealthCheckRepository is an in-memory healthcheck.Repository used to unit test the
// submit command's period-eligibility pre-check without a live Postgres database. Save()
// mirrors the database's unique-index behavior (see the "period submission uniqueness"
// migration): a second completed save for the same scope, survey type, and exact assessment
// period fails.
type fakeHealthCheckRepository struct {
	sessions []*healthcheck.HealthCheckSession
}

func scopeMatches(session *healthcheck.HealthCheckSession, surveyType, teamID, userID string) bool {
	if session.SurveyType != surveyType {
		return false
	}
	if surveyType == healthcheck.SurveyTypePostWorkshop {
		return session.TeamID == teamID
	}
	return session.UserID == userID
}

func (f *fakeHealthCheckRepository) Save(_ context.Context, session *healthcheck.HealthCheckSession) error {
	if session.Completed {
		for _, existing := range f.sessions {
			if !existing.Completed || existing.ID == session.ID {
				continue
			}
			if !scopeMatches(existing, session.SurveyType, session.TeamID, session.UserID) {
				continue
			}
			if existing.AssessmentPeriod == session.AssessmentPeriod {
				submitted, _ := f.FindSubmittedPeriods(context.Background(), healthcheck.LatestSubmissionQuery{
					SurveyType: session.SurveyType,
					TeamID:     session.TeamID,
					UserID:     session.UserID,
				})
				return healthcheck.NewSubmissionCooldownError(session.SurveyType, session.AssessmentPeriod, submitted)
			}
		}
	}
	f.sessions = append(f.sessions, session)
	return nil
}

func (f *fakeHealthCheckRepository) FindSubmittedPeriods(_ context.Context, query healthcheck.LatestSubmissionQuery) ([]string, error) {
	var periods []string
	for _, existing := range f.sessions {
		if !existing.Completed || !scopeMatches(existing, query.SurveyType, query.TeamID, query.UserID) {
			continue
		}
		periods = append(periods, existing.AssessmentPeriod)
	}
	return periods, nil
}

func (f *fakeHealthCheckRepository) FindByID(context.Context, string) (*healthcheck.HealthCheckSession, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeHealthCheckRepository) FindByTeamID(context.Context, string) ([]*healthcheck.HealthCheckSession, error) {
	return nil, nil
}
func (f *fakeHealthCheckRepository) FindByUserID(context.Context, string) ([]*healthcheck.HealthCheckSession, error) {
	return nil, nil
}
func (f *fakeHealthCheckRepository) FindByAssessmentPeriod(context.Context, string) ([]*healthcheck.HealthCheckSession, error) {
	return nil, nil
}
func (f *fakeHealthCheckRepository) Delete(context.Context, string) error { return nil }
func (f *fakeHealthCheckRepository) FindTeamHealthByManager(context.Context, string, string) ([]healthcheck.TeamHealthSummary, error) {
	return nil, nil
}
func (f *fakeHealthCheckRepository) FindAggregatedDimensionsByManager(context.Context, string, string) ([]healthcheck.DimensionSummary, error) {
	return nil, nil
}
func (f *fakeHealthCheckRepository) GetTeamSubmissionStatus(context.Context, string, string) (*healthcheck.TeamSubmissionStatus, error) {
	return nil, nil
}
func (f *fakeHealthCheckRepository) FindDistinctAssessmentPeriods(context.Context) ([]string, error) {
	return nil, nil
}

// baseCommand builds a command dated "now" (mustNow) inside the given assessment period's
// half-year, so eligibility checks (which are evaluated against the real current date) see a
// consistent, currently-open period unless a test explicitly overrides AssessmentPeriod to
// something stale or future. Tests that need a specific "now" pass it via nowOverride in
// helpers below; baseCommand itself only sets the fields common to every case.
func baseCommand(overrides func(*SubmitHealthCheckCommand)) SubmitHealthCheckCommand {
	cmd := SubmitHealthCheckCommand{
		TeamID:           "team-1",
		UserID:           "user-1",
		Date:             "2026-01-15T10:00:00Z",
		AssessmentPeriod: "2026 H1",
		SurveyType:       healthcheck.SurveyTypeIndividual,
		Completed:        true,
		Responses: []HealthCheckResponseCommand{
			{DimensionID: "mission", Score: 3, Trend: "stable"},
		},
	}
	if overrides != nil {
		overrides(&cmd)
	}
	return cmd
}

func TestSubmitHealthCheck_BlocksH1DuplicateIndividualSubmission(t *testing.T) {
	repo := &fakeHealthCheckRepository{}
	handler := NewSubmitHealthCheckHandler(repo)

	if _, err := handler.Handle(baseCommand(nil)); err != nil {
		t.Fatalf("first submission should succeed, got error: %v", err)
	}

	_, err := handler.Handle(baseCommand(func(c *SubmitHealthCheckCommand) { c.ID = "session-2" }))
	var cooldownErr *healthcheck.SubmissionCooldownError
	if !errors.As(err, &cooldownErr) {
		t.Fatalf("expected SubmissionCooldownError, got: %v", err)
	}
	if cooldownErr.LastSubmittedPeriod != "H1 2026" {
		t.Errorf("unexpected LastSubmittedPeriod: %+v", cooldownErr)
	}
	if cooldownErr.NextEligiblePeriod != "H2 2026" {
		t.Errorf("unexpected NextEligiblePeriod: %+v", cooldownErr)
	}
}

func TestSubmitHealthCheck_BlocksH2DuplicateIndividualSubmission(t *testing.T) {
	repo := &fakeHealthCheckRepository{}
	handler := NewSubmitHealthCheckHandler(repo)

	h2 := func(c *SubmitHealthCheckCommand) {
		c.Date = "2026-11-15T10:00:00Z"
		c.AssessmentPeriod = "2026 H2"
	}

	if _, err := handler.Handle(baseCommand(h2)); err != nil {
		t.Fatalf("first H2 submission should succeed, got error: %v", err)
	}

	_, err := handler.Handle(baseCommand(func(c *SubmitHealthCheckCommand) {
		h2(c)
		c.ID = "session-2"
	}))
	var cooldownErr *healthcheck.SubmissionCooldownError
	if !errors.As(err, &cooldownErr) {
		t.Fatalf("expected SubmissionCooldownError, got: %v", err)
	}
	if cooldownErr.LastSubmittedPeriod != "H2 2026" {
		t.Errorf("unexpected LastSubmittedPeriod: %+v", cooldownErr)
	}
	if cooldownErr.NextEligiblePeriod != "H1 2027" {
		t.Errorf("unexpected NextEligiblePeriod: %+v", cooldownErr)
	}
}

func TestSubmitHealthCheck_BlocksDuplicatePostWorkshopSubmission(t *testing.T) {
	repo := &fakeHealthCheckRepository{}
	handler := NewSubmitHealthCheckHandler(repo)

	postWorkshop := func(c *SubmitHealthCheckCommand) { c.SurveyType = healthcheck.SurveyTypePostWorkshop }

	if _, err := handler.Handle(baseCommand(postWorkshop)); err != nil {
		t.Fatalf("first post-workshop submission should succeed, got error: %v", err)
	}

	_, err := handler.Handle(baseCommand(func(c *SubmitHealthCheckCommand) {
		postWorkshop(c)
		c.ID = "session-2"
		c.UserID = "different-lead" // a different Team Lead submitting for the same team
	}))
	var cooldownErr *healthcheck.SubmissionCooldownError
	if !errors.As(err, &cooldownErr) {
		t.Fatalf("expected SubmissionCooldownError for the same team/period, got: %v", err)
	}
}

// TestSubmitHealthCheck_H1ThenH2SameYear covers submitting H1, then later (while the
// calendar has moved into H2) submitting H2 of the same year -- both must succeed as
// distinct, independently tracked periods.
func TestSubmitHealthCheck_H1ThenH2SameYear(t *testing.T) {
	repo := &fakeHealthCheckRepository{}
	handler := NewSubmitHealthCheckHandler(repo)

	if _, err := handler.Handle(baseCommand(nil)); err != nil {
		t.Fatalf("H1 submission should succeed, got error: %v", err)
	}

	_, err := handler.Handle(baseCommand(func(c *SubmitHealthCheckCommand) {
		c.ID = "session-2"
		c.Date = "2026-11-15T10:00:00Z"
		c.AssessmentPeriod = "2026 H2"
	}))
	if err != nil {
		t.Errorf("H2 submission of the same year should succeed after H1, got error: %v", err)
	}
}

// TestSubmitHealthCheck_H2WhileStillH1IsBlockedAsFuture covers the case where H1 was already
// submitted and the caller then attempts H2 of the same year while the calendar has not yet
// reached H2 -- this must be blocked as a future period, not treated as a duplicate.
func TestSubmitHealthCheck_H2WhileStillH1IsBlockedAsFuture(t *testing.T) {
	repo := &fakeHealthCheckRepository{}
	handler := NewSubmitHealthCheckHandler(repo)

	if _, err := handler.Handle(baseCommand(nil)); err != nil {
		t.Fatalf("H1 submission should succeed, got error: %v", err)
	}

	_, err := handler.Handle(baseCommand(func(c *SubmitHealthCheckCommand) {
		c.ID = "session-2"
		c.Date = "2026-03-01T10:00:00Z" // still H1 by calendar
		c.AssessmentPeriod = "2026 H2"
	}))
	var notOpenErr *healthcheck.PeriodNotOpenError
	if !errors.As(err, &notOpenErr) {
		t.Fatalf("expected PeriodNotOpenError (future), got: %v", err)
	}
	if notOpenErr.Reason != healthcheck.PeriodReasonFuture {
		t.Errorf("expected future_period reason, got: %v", notOpenErr.Reason)
	}
}

// TestSubmitHealthCheck_CurrentH2AllowsUnsubmittedH1Catchup covers the case where the
// calendar is currently in H2 and H1 of the same year was never submitted -- it must still
// be allowed as a catch-up submission.
func TestSubmitHealthCheck_CurrentH2AllowsUnsubmittedH1Catchup(t *testing.T) {
	repo := &fakeHealthCheckRepository{}
	handler := NewSubmitHealthCheckHandler(repo)

	_, err := handler.Handle(baseCommand(func(c *SubmitHealthCheckCommand) {
		c.Date = "2026-11-15T10:00:00Z" // calendar is in H2
		c.AssessmentPeriod = "2026 H1"  // catching up on a missed H1 submission
	}))
	if err != nil {
		t.Errorf("an unsubmitted H1 catch-up during H2 should be allowed, got error: %v", err)
	}
}

// TestSubmitHealthCheck_H2ThenNextYearH1BlockedUntilNewYear covers submitting H2, then
// attempting H1 of the following year before the new year has actually begun -- blocked as
// future; once the calendar reaches January, the same submission must succeed.
func TestSubmitHealthCheck_H2ThenNextYearH1BlockedUntilNewYear(t *testing.T) {
	repo := &fakeHealthCheckRepository{}
	handler := NewSubmitHealthCheckHandler(repo)

	if _, err := handler.Handle(baseCommand(func(c *SubmitHealthCheckCommand) {
		c.Date = "2026-11-15T10:00:00Z"
		c.AssessmentPeriod = "2026 H2"
	})); err != nil {
		t.Fatalf("H2 submission should succeed, got error: %v", err)
	}

	_, err := handler.Handle(baseCommand(func(c *SubmitHealthCheckCommand) {
		c.ID = "session-2"
		c.Date = "2026-12-31T10:00:00Z" // still 2026 -- the new year has not begun
		c.AssessmentPeriod = "2027 H1"
	}))
	var notOpenErr *healthcheck.PeriodNotOpenError
	if !errors.As(err, &notOpenErr) || notOpenErr.Reason != healthcheck.PeriodReasonFuture {
		t.Fatalf("expected a future_period rejection before the new year begins, got: %v", err)
	}

	_, err = handler.Handle(baseCommand(func(c *SubmitHealthCheckCommand) {
		c.ID = "session-3"
		c.Date = "2027-01-01T00:00:00Z" // the new year has begun
		c.AssessmentPeriod = "2027 H1"
	}))
	if err != nil {
		t.Errorf("H1 of the following year should be allowed once the new year begins, got error: %v", err)
	}
}

// TestSubmitHealthCheck_RejectsStalePreviousYearPeriod covers a candidate period from an
// earlier year than the current one -- blocked as past, even though it was never submitted.
func TestSubmitHealthCheck_RejectsStalePreviousYearPeriod(t *testing.T) {
	repo := &fakeHealthCheckRepository{}
	handler := NewSubmitHealthCheckHandler(repo)

	_, err := handler.Handle(baseCommand(func(c *SubmitHealthCheckCommand) {
		c.Date = "2026-03-01T10:00:00Z"
		c.AssessmentPeriod = "2025 H2"
	}))
	var notOpenErr *healthcheck.PeriodNotOpenError
	if !errors.As(err, &notOpenErr) {
		t.Fatalf("expected PeriodNotOpenError (past), got: %v", err)
	}
	if notOpenErr.Reason != healthcheck.PeriodReasonPast {
		t.Errorf("expected past_period reason, got: %v", notOpenErr.Reason)
	}
}

func TestSubmitHealthCheck_DoesNotBlockDifferentUserIndividualSubmission(t *testing.T) {
	repo := &fakeHealthCheckRepository{}
	handler := NewSubmitHealthCheckHandler(repo)

	if _, err := handler.Handle(baseCommand(nil)); err != nil {
		t.Fatalf("first submission should succeed, got error: %v", err)
	}

	_, err := handler.Handle(baseCommand(func(c *SubmitHealthCheckCommand) {
		c.ID = "session-2"
		c.UserID = "user-2"
	}))
	if err != nil {
		t.Errorf("a different user's individual submission must not be blocked, got error: %v", err)
	}
}

func TestSubmitHealthCheck_SeparatesIndividualAndPostWorkshopRecords(t *testing.T) {
	repo := &fakeHealthCheckRepository{}
	handler := NewSubmitHealthCheckHandler(repo)

	if _, err := handler.Handle(baseCommand(nil)); err != nil {
		t.Fatalf("individual submission should succeed, got error: %v", err)
	}

	// Same team, same user, same period -- but a different survey type -- must be allowed.
	_, err := handler.Handle(baseCommand(func(c *SubmitHealthCheckCommand) {
		c.ID = "session-2"
		c.SurveyType = healthcheck.SurveyTypePostWorkshop
	}))
	if err != nil {
		t.Errorf("post-workshop submission must not be blocked by an individual submission, got error: %v", err)
	}
}

func TestSubmitHealthCheck_MatchesPostWorkshopByTeamNotUser(t *testing.T) {
	repo := &fakeHealthCheckRepository{}
	handler := NewSubmitHealthCheckHandler(repo)

	postWorkshop := func(c *SubmitHealthCheckCommand) { c.SurveyType = healthcheck.SurveyTypePostWorkshop }
	if _, err := handler.Handle(baseCommand(postWorkshop)); err != nil {
		t.Fatalf("first post-workshop submission should succeed, got error: %v", err)
	}

	// A different team for the same period must not be blocked.
	_, err := handler.Handle(baseCommand(func(c *SubmitHealthCheckCommand) {
		postWorkshop(c)
		c.ID = "session-2"
		c.TeamID = "team-2"
	}))
	if err != nil {
		t.Errorf("a different team's post-workshop submission must not be blocked, got error: %v", err)
	}
}

func TestSubmitHealthCheck_IgnoresIncompleteDraftsForDuplicateCheck(t *testing.T) {
	repo := &fakeHealthCheckRepository{}
	handler := NewSubmitHealthCheckHandler(repo)

	_, err := handler.Handle(baseCommand(func(c *SubmitHealthCheckCommand) { c.Completed = false }))
	if err != nil {
		t.Fatalf("incomplete draft save should succeed, got error: %v", err)
	}

	_, err = handler.Handle(baseCommand(func(c *SubmitHealthCheckCommand) { c.ID = "session-2" }))
	if err != nil {
		t.Errorf("a completed submission must not be blocked by an incomplete draft, got error: %v", err)
	}
}

// TestSubmitHealthCheck_TeamMemberAndTeamLeadBothEnforced verifies the same eligibility rule
// applies regardless of which role's user ID submits -- the backend has no separate code path
// per role, only per survey type and scope.
func TestSubmitHealthCheck_TeamMemberAndTeamLeadBothEnforced(t *testing.T) {
	repo := &fakeHealthCheckRepository{}
	handler := NewSubmitHealthCheckHandler(repo)

	teamMember := func(c *SubmitHealthCheckCommand) { c.UserID = "member-1" }
	if _, err := handler.Handle(baseCommand(teamMember)); err != nil {
		t.Fatalf("Team Member's Individual Survey submission should succeed, got error: %v", err)
	}
	_, err := handler.Handle(baseCommand(func(c *SubmitHealthCheckCommand) {
		teamMember(c)
		c.ID = "session-2"
	}))
	var cooldownErr *healthcheck.SubmissionCooldownError
	if !errors.As(err, &cooldownErr) {
		t.Fatalf("Team Member's duplicate Individual Survey submission should be blocked, got: %v", err)
	}

	teamLead := func(c *SubmitHealthCheckCommand) {
		c.UserID = "lead-1"
		c.SurveyType = healthcheck.SurveyTypePostWorkshop
	}
	if _, err := handler.Handle(baseCommand(func(c *SubmitHealthCheckCommand) {
		teamLead(c)
		c.ID = "session-3"
	})); err != nil {
		t.Fatalf("Team Lead's Post-Workshop Survey submission should succeed, got error: %v", err)
	}
	_, err = handler.Handle(baseCommand(func(c *SubmitHealthCheckCommand) {
		teamLead(c)
		c.ID = "session-4"
	}))
	if !errors.As(err, &cooldownErr) {
		t.Fatalf("Team Lead's duplicate Post-Workshop Survey submission should be blocked, got: %v", err)
	}
}
