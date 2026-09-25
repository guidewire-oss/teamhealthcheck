package commands

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agopalakrishnan/teams360/backend/domain/healthcheck"
)

// fakeHealthCheckRepository is an in-memory healthcheck.Repository used to unit test the
// submit command's six-month submission cooldown pre-check without a live Postgres
// database. Save() mirrors the database's exclusion-constraint behavior (see the "six month
// submission cooldown" migration): a second completed save for the same scope within six
// months of that scope's most recent completed submission fails.
type fakeHealthCheckRepository struct {
	sessions []*healthcheck.HealthCheckSession
}

func (f *fakeHealthCheckRepository) Save(_ context.Context, session *healthcheck.HealthCheckSession) error {
	if session.Completed {
		if newDate, ok := parseSubmissionDate(session.Date); ok {
			for _, existing := range f.sessions {
				if !existing.Completed || existing.ID == session.ID || existing.SurveyType != session.SurveyType {
					continue
				}
				sameScope := (session.SurveyType == healthcheck.SurveyTypePostWorkshop && existing.TeamID == session.TeamID) ||
					(session.SurveyType != healthcheck.SurveyTypePostWorkshop && existing.UserID == session.UserID)
				if !sameScope {
					continue
				}
				existingDate, existingOK := parseSubmissionDate(existing.Date)
				if existingOK && healthcheck.IsWithinCooldown(existingDate, newDate) {
					return healthcheck.NewSubmissionCooldownError(session.SurveyType, existing.AssessmentPeriod, existingDate)
				}
			}
		}
	}
	f.sessions = append(f.sessions, session)
	return nil
}

func (f *fakeHealthCheckRepository) FindLatestSubmission(_ context.Context, query healthcheck.LatestSubmissionQuery) (*healthcheck.HealthCheckSession, error) {
	var latest *healthcheck.HealthCheckSession
	var latestDate time.Time
	for _, existing := range f.sessions {
		if !existing.Completed || existing.SurveyType != query.SurveyType {
			continue
		}
		if query.SurveyType == healthcheck.SurveyTypePostWorkshop {
			if existing.TeamID != query.TeamID {
				continue
			}
		} else if existing.UserID != query.UserID {
			continue
		}
		d, ok := parseSubmissionDate(existing.Date)
		if !ok {
			continue
		}
		if latest == nil || d.After(latestDate) {
			latest = existing
			latestDate = d
		}
	}
	return latest, nil
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

func TestSubmitHealthCheck_BlocksSameDayDuplicateIndividualSubmission(t *testing.T) {
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
	if cooldownErr.NextEligible.Format("2006-01-02") != "2026-07-01" {
		t.Errorf("unexpected NextEligible: %+v", cooldownErr)
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
		t.Fatalf("expected SubmissionCooldownError for same team within six months, got: %v", err)
	}
}

// TestSubmitHealthCheck_EligibilityBoundary covers the combined eligibility rule: a
// submission is blocked only while it is both (a) within the six-month cooldown AND (b)
// still in the same calendar half-year as the last submission. Crossing into a new
// half-year period always opens eligibility, even well short of six months.
func TestSubmitHealthCheck_EligibilityBoundary(t *testing.T) {
	cases := []struct {
		name     string
		from     string
		to       string
		eligible bool
	}{
		{"same day resubmission is blocked", "2026-01-15T10:00:00Z", "2026-01-15T23:00:00Z", false},
		{"later the same half-year is blocked (duplicate-period prevention)", "2026-01-15T10:00:00Z", "2026-06-30T10:00:00Z", false},
		{"the moment the next half-year starts is eligible", "2026-01-15T10:00:00Z", "2026-07-01T00:00:00Z", true},
		{"exactly six months elapsed is eligible", "2026-01-15T10:00:00Z", "2026-07-15T10:00:00Z", true},
		{"six months elapsed across a year boundary is eligible", "2026-09-01T00:00:00Z", "2027-03-01T00:00:00Z", true},

		// Acceptance-criteria scenario: an H2 submission in November must not force a wait
		// until the six-month mark in May -- H1 of the following year opens January 1.
		{"a November H2 submission does not block the following January (H1)", "2026-11-15T00:00:00Z", "2027-01-01T00:00:00Z", true},
		{"the day before the following January is still blocked (still H2)", "2026-11-15T00:00:00Z", "2026-12-31T00:00:00Z", false},
		{"well under six months but crossing into H1 is eligible", "2025-11-20T00:00:00Z", "2026-05-19T00:00:00Z", true},
		{"under six months and still the same half-year (H2) is blocked", "2025-11-20T00:00:00Z", "2025-12-31T00:00:00Z", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeHealthCheckRepository{}
			handler := NewSubmitHealthCheckHandler(repo)

			if _, err := handler.Handle(baseCommand(func(c *SubmitHealthCheckCommand) { c.Date = tc.from })); err != nil {
				t.Fatalf("first submission at %q should succeed, got error: %v", tc.from, err)
			}

			_, err := handler.Handle(baseCommand(func(c *SubmitHealthCheckCommand) {
				c.ID = "session-2"
				c.Date = tc.to
			}))
			if tc.eligible && err != nil {
				t.Errorf("expected submission at %s to be eligible after %s, got error: %v", tc.to, tc.from, err)
			}
			if !tc.eligible {
				var cooldownErr *healthcheck.SubmissionCooldownError
				if !errors.As(err, &cooldownErr) {
					t.Fatalf("expected submission at %s to be blocked relative to %s, got: %v", tc.to, tc.from, err)
				}
			}
		})
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

	// Same team, same user, same day -- but a different survey type -- must be allowed.
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

	// A different team on the same day must not be blocked.
	_, err := handler.Handle(baseCommand(func(c *SubmitHealthCheckCommand) {
		postWorkshop(c)
		c.ID = "session-2"
		c.TeamID = "team-2"
	}))
	if err != nil {
		t.Errorf("a different team's post-workshop submission must not be blocked, got error: %v", err)
	}
}

func TestSubmitHealthCheck_IgnoresIncompleteDraftsForCooldownCheck(t *testing.T) {
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

func TestSubmitHealthCheck_MissingOrInvalidDateSkipsCooldownCheck(t *testing.T) {
	// A date that can't be parsed can't be compared against the cooldown window, so the
	// six-month rule must not apply -- even for what would otherwise look like an exact
	// same-day repeat of the same user/team/survey-type submission.
	for _, date := range []string{"not-a-date", "2026/01/15"} {
		t.Run("date="+date, func(t *testing.T) {
			repo := &fakeHealthCheckRepository{}
			handler := NewSubmitHealthCheckHandler(repo)

			if _, err := handler.Handle(baseCommand(func(c *SubmitHealthCheckCommand) {
				c.Date = date
			})); err != nil {
				t.Fatalf("first submission with date %q should succeed, got: %v", date, err)
			}

			_, err := handler.Handle(baseCommand(func(c *SubmitHealthCheckCommand) {
				c.ID = "session-2"
				c.Date = date
			}))
			if err != nil {
				t.Errorf("submission with unparseable date %q should not fail the cooldown check, got: %v", date, err)
			}
		})
	}
}

func TestSubmitHealthCheck_CooldownCheckUsesMostRecentSubmission(t *testing.T) {
	repo := &fakeHealthCheckRepository{}
	handler := NewSubmitHealthCheckHandler(repo)

	// User submits in January, then again in July (exactly six months later -- eligible).
	if _, err := handler.Handle(baseCommand(func(c *SubmitHealthCheckCommand) { c.Date = "2026-01-15T10:00:00Z" })); err != nil {
		t.Fatalf("January submission should succeed, got error: %v", err)
	}
	if _, err := handler.Handle(baseCommand(func(c *SubmitHealthCheckCommand) {
		c.ID = "session-2"
		c.Date = "2026-07-15T10:00:00Z"
	})); err != nil {
		t.Fatalf("July submission should succeed, got error: %v", err)
	}

	// The most recent submission is now July, so a September attempt (less than six months
	// after July) must be blocked even though it is more than six months after January.
	_, err := handler.Handle(baseCommand(func(c *SubmitHealthCheckCommand) {
		c.ID = "session-3"
		c.Date = "2026-09-01T10:00:00Z"
	}))
	var cooldownErr *healthcheck.SubmissionCooldownError
	if !errors.As(err, &cooldownErr) {
		t.Fatalf("expected September submission to be blocked by the most recent (July) submission, got: %v", err)
	}
}
