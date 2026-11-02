package commands

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agopalakrishnan/teams360/backend/domain/healthcheck"
)

// fakeHealthCheckRepository is an in-memory healthcheck.Repository used to unit test the
// submit command's period-eligibility pre-check without a live Postgres database. Save()
// mirrors the database's unique-index behavior (see the "half-year period uniqueness"
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
				return healthcheck.NewDuplicateSubmissionError(session.SurveyType, session.AssessmentPeriod, submitted)
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

// testClock is a controllable clock for the handler under test.
type testClock struct{ now time.Time }

func (c *testClock) Now() time.Time { return c.now }

// newTestHandler returns a handler wired to a test clock that starts on January 15, 2026.
func newTestHandler(repo *fakeHealthCheckRepository) (*SubmitHealthCheckHandler, *testClock) {
	clock := &testClock{now: time.Date(2026, time.January, 15, 10, 0, 0, 0, time.UTC)}
	return NewSubmitHealthCheckHandlerWithClock(repo, clock.Now), clock
}

// parseSubmissionDate parses a test command's date, in RFC3339 or plain "YYYY-MM-DD" form.
func parseSubmissionDate(date string) (time.Time, bool) {
	if t, err := time.Parse(time.RFC3339, date); err == nil {
		return t, true
	}
	if t, err := time.Parse("2006-01-02", date); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// submit moves the test clock to cmd.Date and handles cmd, so a test reads as "submit on this
// date". The handler never reads cmd.Date as the time itself.
func submit(h *SubmitHealthCheckHandler, clock *testClock, cmd SubmitHealthCheckCommand) (*healthcheck.HealthCheckSession, error) {
	if t, ok := parseSubmissionDate(cmd.Date); ok {
		clock.now = t
	}
	return h.Handle(cmd)
}

// baseCommand builds a completed Individual Survey for "2026 H1", dated January 15, 2026.
//
// The handler decides which periods are open from its own clock, never from the command's
// Date. The tests below describe a scenario as "submitting on <date>", so submit() points the
// test clock at the command's Date before each call; tests that need the clock and the Date
// to differ (to prove the Date is ignored) call handler.Handle directly with a fixed clock.
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
	handler, clock := newTestHandler(repo)

	if _, err := submit(handler, clock, baseCommand(nil)); err != nil {
		t.Fatalf("first submission should succeed, got error: %v", err)
	}

	_, err := submit(handler, clock, baseCommand(func(c *SubmitHealthCheckCommand) { c.ID = "session-2" }))
	var duplicateErr *healthcheck.DuplicateSubmissionError
	if !errors.As(err, &duplicateErr) {
		t.Fatalf("expected DuplicateSubmissionError, got: %v", err)
	}
	if duplicateErr.LastSubmittedPeriod != "H1 2026" {
		t.Errorf("unexpected LastSubmittedPeriod: %+v", duplicateErr)
	}
	if duplicateErr.NextEligiblePeriod != "H2 2026" {
		t.Errorf("unexpected NextEligiblePeriod: %+v", duplicateErr)
	}
}

func TestSubmitHealthCheck_BlocksH2DuplicateIndividualSubmission(t *testing.T) {
	repo := &fakeHealthCheckRepository{}
	handler, clock := newTestHandler(repo)

	h2 := func(c *SubmitHealthCheckCommand) {
		c.Date = "2026-11-15T10:00:00Z"
		c.AssessmentPeriod = "2026 H2"
	}

	if _, err := submit(handler, clock, baseCommand(h2)); err != nil {
		t.Fatalf("first H2 submission should succeed, got error: %v", err)
	}

	_, err := submit(handler, clock, baseCommand(func(c *SubmitHealthCheckCommand) {
		h2(c)
		c.ID = "session-2"
	}))
	var duplicateErr *healthcheck.DuplicateSubmissionError
	if !errors.As(err, &duplicateErr) {
		t.Fatalf("expected DuplicateSubmissionError, got: %v", err)
	}
	if duplicateErr.LastSubmittedPeriod != "H2 2026" {
		t.Errorf("unexpected LastSubmittedPeriod: %+v", duplicateErr)
	}
	if duplicateErr.NextEligiblePeriod != "H1 2027" {
		t.Errorf("unexpected NextEligiblePeriod: %+v", duplicateErr)
	}
}

func TestSubmitHealthCheck_BlocksDuplicatePostWorkshopSubmission(t *testing.T) {
	repo := &fakeHealthCheckRepository{}
	handler, clock := newTestHandler(repo)

	postWorkshop := func(c *SubmitHealthCheckCommand) { c.SurveyType = healthcheck.SurveyTypePostWorkshop }

	if _, err := submit(handler, clock, baseCommand(postWorkshop)); err != nil {
		t.Fatalf("first post-workshop submission should succeed, got error: %v", err)
	}

	_, err := submit(handler, clock, baseCommand(func(c *SubmitHealthCheckCommand) {
		postWorkshop(c)
		c.ID = "session-2"
		c.UserID = "different-lead" // a different Team Lead submitting for the same team
	}))
	var duplicateErr *healthcheck.DuplicateSubmissionError
	if !errors.As(err, &duplicateErr) {
		t.Fatalf("expected DuplicateSubmissionError for the same team/period, got: %v", err)
	}
}

// TestSubmitHealthCheck_H1ThenH2SameYear covers submitting H1, then later (while the
// calendar has moved into H2) submitting H2 of the same year -- both must succeed as
// distinct, independently tracked periods.
func TestSubmitHealthCheck_H1ThenH2SameYear(t *testing.T) {
	repo := &fakeHealthCheckRepository{}
	handler, clock := newTestHandler(repo)

	if _, err := submit(handler, clock, baseCommand(nil)); err != nil {
		t.Fatalf("H1 submission should succeed, got error: %v", err)
	}

	_, err := submit(handler, clock, baseCommand(func(c *SubmitHealthCheckCommand) {
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
	handler, clock := newTestHandler(repo)

	if _, err := submit(handler, clock, baseCommand(nil)); err != nil {
		t.Fatalf("H1 submission should succeed, got error: %v", err)
	}

	_, err := submit(handler, clock, baseCommand(func(c *SubmitHealthCheckCommand) {
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
	handler, clock := newTestHandler(repo)

	_, err := submit(handler, clock, baseCommand(func(c *SubmitHealthCheckCommand) {
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
	handler, clock := newTestHandler(repo)

	if _, err := submit(handler, clock, baseCommand(func(c *SubmitHealthCheckCommand) {
		c.Date = "2026-11-15T10:00:00Z"
		c.AssessmentPeriod = "2026 H2"
	})); err != nil {
		t.Fatalf("H2 submission should succeed, got error: %v", err)
	}

	_, err := submit(handler, clock, baseCommand(func(c *SubmitHealthCheckCommand) {
		c.ID = "session-2"
		c.Date = "2026-12-31T10:00:00Z" // still 2026 -- the new year has not begun
		c.AssessmentPeriod = "2027 H1"
	}))
	var notOpenErr *healthcheck.PeriodNotOpenError
	if !errors.As(err, &notOpenErr) || notOpenErr.Reason != healthcheck.PeriodReasonFuture {
		t.Fatalf("expected a future_period rejection before the new year begins, got: %v", err)
	}

	_, err = submit(handler, clock, baseCommand(func(c *SubmitHealthCheckCommand) {
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
	handler, clock := newTestHandler(repo)

	_, err := submit(handler, clock, baseCommand(func(c *SubmitHealthCheckCommand) {
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
	handler, clock := newTestHandler(repo)

	if _, err := submit(handler, clock, baseCommand(nil)); err != nil {
		t.Fatalf("first submission should succeed, got error: %v", err)
	}

	_, err := submit(handler, clock, baseCommand(func(c *SubmitHealthCheckCommand) {
		c.ID = "session-2"
		c.UserID = "user-2"
	}))
	if err != nil {
		t.Errorf("a different user's individual submission must not be blocked, got error: %v", err)
	}
}

func TestSubmitHealthCheck_SeparatesIndividualAndPostWorkshopRecords(t *testing.T) {
	repo := &fakeHealthCheckRepository{}
	handler, clock := newTestHandler(repo)

	if _, err := submit(handler, clock, baseCommand(nil)); err != nil {
		t.Fatalf("individual submission should succeed, got error: %v", err)
	}

	// Same team, same user, same period -- but a different survey type -- must be allowed.
	_, err := submit(handler, clock, baseCommand(func(c *SubmitHealthCheckCommand) {
		c.ID = "session-2"
		c.SurveyType = healthcheck.SurveyTypePostWorkshop
	}))
	if err != nil {
		t.Errorf("post-workshop submission must not be blocked by an individual submission, got error: %v", err)
	}
}

func TestSubmitHealthCheck_MatchesPostWorkshopByTeamNotUser(t *testing.T) {
	repo := &fakeHealthCheckRepository{}
	handler, clock := newTestHandler(repo)

	postWorkshop := func(c *SubmitHealthCheckCommand) { c.SurveyType = healthcheck.SurveyTypePostWorkshop }
	if _, err := submit(handler, clock, baseCommand(postWorkshop)); err != nil {
		t.Fatalf("first post-workshop submission should succeed, got error: %v", err)
	}

	// A different team for the same period must not be blocked.
	_, err := submit(handler, clock, baseCommand(func(c *SubmitHealthCheckCommand) {
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
	handler, clock := newTestHandler(repo)

	_, err := submit(handler, clock, baseCommand(func(c *SubmitHealthCheckCommand) { c.Completed = false }))
	if err != nil {
		t.Fatalf("incomplete draft save should succeed, got error: %v", err)
	}

	_, err = submit(handler, clock, baseCommand(func(c *SubmitHealthCheckCommand) { c.ID = "session-2" }))
	if err != nil {
		t.Errorf("a completed submission must not be blocked by an incomplete draft, got error: %v", err)
	}
}

// TestSubmitHealthCheck_TeamMemberAndTeamLeadBothEnforced verifies the same eligibility rule
// applies regardless of which role's user ID submits -- the backend has no separate code path
// per role, only per survey type and scope.
func TestSubmitHealthCheck_TeamMemberAndTeamLeadBothEnforced(t *testing.T) {
	repo := &fakeHealthCheckRepository{}
	handler, clock := newTestHandler(repo)

	teamMember := func(c *SubmitHealthCheckCommand) { c.UserID = "member-1" }
	if _, err := submit(handler, clock, baseCommand(teamMember)); err != nil {
		t.Fatalf("Team Member's Individual Survey submission should succeed, got error: %v", err)
	}
	_, err := submit(handler, clock, baseCommand(func(c *SubmitHealthCheckCommand) {
		teamMember(c)
		c.ID = "session-2"
	}))
	var duplicateErr *healthcheck.DuplicateSubmissionError
	if !errors.As(err, &duplicateErr) {
		t.Fatalf("Team Member's duplicate Individual Survey submission should be blocked, got: %v", err)
	}

	teamLead := func(c *SubmitHealthCheckCommand) {
		c.UserID = "lead-1"
		c.SurveyType = healthcheck.SurveyTypePostWorkshop
	}
	if _, err := submit(handler, clock, baseCommand(func(c *SubmitHealthCheckCommand) {
		teamLead(c)
		c.ID = "session-3"
	})); err != nil {
		t.Fatalf("Team Lead's Post-Workshop Survey submission should succeed, got error: %v", err)
	}
	_, err = submit(handler, clock, baseCommand(func(c *SubmitHealthCheckCommand) {
		teamLead(c)
		c.ID = "session-4"
	}))
	if !errors.As(err, &duplicateErr) {
		t.Fatalf("Team Lead's duplicate Post-Workshop Survey submission should be blocked, got: %v", err)
	}
}

// Eligibility is decided against the server's clock. A client-supplied Date is stored as the
// session date but must never decide which periods are open.
func TestSubmitHealthCheck_DecidesEligibilityFromTheServerClockNotTheClientDate(t *testing.T) {
	t.Run("a legitimate H2 submission is accepted even if the client's date predates July 1", func(t *testing.T) {
		repo := &fakeHealthCheckRepository{}
		november := time.Date(2026, time.November, 15, 10, 0, 0, 0, time.UTC)
		handler := NewSubmitHealthCheckHandlerWithClock(repo, func() time.Time { return november })

		_, err := handler.Handle(baseCommand(func(c *SubmitHealthCheckCommand) {
			c.AssessmentPeriod = "2026 H2"
			c.Date = "2026-03-01T10:00:00Z" // a draft started in H1, or a device with the wrong clock
		}))
		if err != nil {
			t.Fatalf("the server is in H2, so H2 is open whatever date the client sent, got: %v", err)
		}
	})

	t.Run("a client cannot open a future period by sending a later date", func(t *testing.T) {
		repo := &fakeHealthCheckRepository{}
		march := time.Date(2026, time.March, 1, 10, 0, 0, 0, time.UTC)
		handler := NewSubmitHealthCheckHandlerWithClock(repo, func() time.Time { return march })

		_, err := handler.Handle(baseCommand(func(c *SubmitHealthCheckCommand) {
			c.AssessmentPeriod = "2026 H2"
			c.Date = "2026-11-15T10:00:00Z"
		}))

		var notOpen *healthcheck.PeriodNotOpenError
		if !errors.As(err, &notOpen) || notOpen.Reason != healthcheck.PeriodReasonFuture {
			t.Fatalf("expected a future-period rejection, got: %v", err)
		}
	})
}

func TestSubmitHealthCheck_RejectsACompletedSurveyWithoutASupportedPeriod(t *testing.T) {
	for _, period := range []string{"", "garbage", "2026 Q5", "H1 2026", "26"} {
		repo := &fakeHealthCheckRepository{}
		handler, clock := newTestHandler(repo)

		_, err := submit(handler, clock, baseCommand(func(c *SubmitHealthCheckCommand) { c.AssessmentPeriod = period }))

		if err == nil {
			t.Errorf("period %q: a completed survey with no supported period must be rejected, not saved", period)
		}
		if len(repo.sessions) != 0 {
			t.Errorf("period %q: nothing may be saved when the period is not supported", period)
		}
	}
}

func TestSubmitHealthCheck_StillSavesAnIncompleteDraftWithoutAPeriod(t *testing.T) {
	repo := &fakeHealthCheckRepository{}
	handler, clock := newTestHandler(repo)

	_, err := submit(handler, clock, baseCommand(func(c *SubmitHealthCheckCommand) {
		c.Completed = false
		c.AssessmentPeriod = ""
	}))

	if err != nil {
		t.Fatalf("a draft is not subject to the period rules, got: %v", err)
	}
}

func TestSubmitHealthCheck_StoresThePeriodInCanonicalForm(t *testing.T) {
	repo := &fakeHealthCheckRepository{}
	handler, clock := newTestHandler(repo)

	session, err := submit(handler, clock, baseCommand(func(c *SubmitHealthCheckCommand) { c.AssessmentPeriod = "2026 Q1" }))
	if err != nil {
		t.Fatalf("an old-style label for the open half-year should still be accepted, got: %v", err)
	}
	if session.AssessmentPeriod != "2026 H1" {
		t.Errorf("stored period = %q, want the canonical \"2026 H1\"", session.AssessmentPeriod)
	}
}

func TestSubmitHealthCheck_TreatsDifferentLabelsForTheSamePeriodAsDuplicates(t *testing.T) {
	repo := &fakeHealthCheckRepository{}
	handler, clock := newTestHandler(repo)

	if _, err := submit(handler, clock, baseCommand(nil)); err != nil {
		t.Fatalf("first submission should succeed, got: %v", err)
	}
	_, err := submit(handler, clock, baseCommand(func(c *SubmitHealthCheckCommand) {
		c.ID = "second"
		c.AssessmentPeriod = "2026 Q2" // an alias of 2026 H1
	}))

	var duplicateErr *healthcheck.DuplicateSubmissionError
	if !errors.As(err, &duplicateErr) {
		t.Fatalf("expected a duplicate for the same half-year under another label, got: %v", err)
	}
}

func TestSubmitHealthCheck_SupportsMonthlyAndYearlyCadences(t *testing.T) {
	t.Run("monthly: current month open, next month not yet, same month twice is a duplicate", func(t *testing.T) {
		repo := &fakeHealthCheckRepository{}
		handler, clock := newTestHandler(repo) // January 2026

		if _, err := submit(handler, clock, baseCommand(func(c *SubmitHealthCheckCommand) { c.AssessmentPeriod = "2026 Jan" })); err != nil {
			t.Fatalf("the current month should be open, got: %v", err)
		}

		_, err := submit(handler, clock, baseCommand(func(c *SubmitHealthCheckCommand) { c.ID = "feb"; c.AssessmentPeriod = "2026 Feb" }))
		var notOpen *healthcheck.PeriodNotOpenError
		if !errors.As(err, &notOpen) || notOpen.Reason != healthcheck.PeriodReasonFuture {
			t.Fatalf("next month should not be open yet, got: %v", err)
		}

		_, err = submit(handler, clock, baseCommand(func(c *SubmitHealthCheckCommand) { c.ID = "again"; c.AssessmentPeriod = "2026 Jan" }))
		var duplicateErr *healthcheck.DuplicateSubmissionError
		if !errors.As(err, &duplicateErr) {
			t.Fatalf("the same month twice should be a duplicate, got: %v", err)
		}
		if duplicateErr.NextEligiblePeriod != "2026 Feb" {
			t.Errorf("next eligible period = %q, want 2026 Feb", duplicateErr.NextEligiblePeriod)
		}
	})

	t.Run("yearly: current year open, previous year closed, same year twice is a duplicate", func(t *testing.T) {
		repo := &fakeHealthCheckRepository{}
		handler, clock := newTestHandler(repo)

		if _, err := submit(handler, clock, baseCommand(func(c *SubmitHealthCheckCommand) { c.AssessmentPeriod = "2026" })); err != nil {
			t.Fatalf("the current year should be open, got: %v", err)
		}

		_, err := submit(handler, clock, baseCommand(func(c *SubmitHealthCheckCommand) { c.ID = "prev"; c.AssessmentPeriod = "2025" }))
		var notOpen *healthcheck.PeriodNotOpenError
		if !errors.As(err, &notOpen) || notOpen.Reason != healthcheck.PeriodReasonPast {
			t.Fatalf("the previous year should be closed, got: %v", err)
		}

		_, err = submit(handler, clock, baseCommand(func(c *SubmitHealthCheckCommand) { c.ID = "again"; c.AssessmentPeriod = "2026" }))
		var duplicateErr *healthcheck.DuplicateSubmissionError
		if !errors.As(err, &duplicateErr) {
			t.Fatalf("the same year twice should be a duplicate, got: %v", err)
		}
	})
}
