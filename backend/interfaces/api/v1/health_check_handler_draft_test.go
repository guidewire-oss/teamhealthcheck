package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/healthcheck"
)

// fakeHealthCheckRepository is a minimal in-memory stand-in for healthcheck.Repository, used to
// exercise handler-level behavior (like "a failed draft cleanup must not fail the submission")
// without needing a real Postgres instance. Only the methods exercised by the tests in this file
// do anything interesting; the rest return zero values, which is fine since SubmitHealthCheck
// never calls them.
type fakeHealthCheckRepository struct {
	saveErr        error
	deleteDraftErr error
	deletedDrafts  []string // "userID|teamID|surveyType" for each DeleteDraft call
}

func (f *fakeHealthCheckRepository) Save(ctx context.Context, session *healthcheck.HealthCheckSession) error {
	return f.saveErr
}
func (f *fakeHealthCheckRepository) FindByID(ctx context.Context, id string) (*healthcheck.HealthCheckSession, error) {
	return nil, healthcheck.ErrDraftNotFound
}
func (f *fakeHealthCheckRepository) FindByTeamID(ctx context.Context, teamID string) ([]*healthcheck.HealthCheckSession, error) {
	return nil, nil
}
func (f *fakeHealthCheckRepository) FindByUserID(ctx context.Context, userID string) ([]*healthcheck.HealthCheckSession, error) {
	return nil, nil
}
func (f *fakeHealthCheckRepository) FindByAssessmentPeriod(ctx context.Context, period string) ([]*healthcheck.HealthCheckSession, error) {
	return nil, nil
}
func (f *fakeHealthCheckRepository) Delete(ctx context.Context, id string) error { return nil }
func (f *fakeHealthCheckRepository) FindTeamHealthByManager(ctx context.Context, managerID string, assessmentPeriod string) ([]healthcheck.TeamHealthSummary, error) {
	return nil, nil
}
func (f *fakeHealthCheckRepository) FindAggregatedDimensionsByManager(ctx context.Context, managerID string, assessmentPeriod string) ([]healthcheck.DimensionSummary, error) {
	return nil, nil
}
func (f *fakeHealthCheckRepository) GetTeamSubmissionStatus(ctx context.Context, teamID string, assessmentPeriod string) (*healthcheck.TeamSubmissionStatus, error) {
	return nil, nil
}
func (f *fakeHealthCheckRepository) FindDistinctAssessmentPeriods(ctx context.Context) ([]string, error) {
	return nil, nil
}
func (f *fakeHealthCheckRepository) SaveDraft(ctx context.Context, draft *healthcheck.HealthCheckDraft) error {
	return nil
}
func (f *fakeHealthCheckRepository) GetDraft(ctx context.Context, userID, teamID, surveyType string) (*healthcheck.HealthCheckDraft, error) {
	return nil, healthcheck.ErrDraftNotFound
}
func (f *fakeHealthCheckRepository) DeleteDraft(ctx context.Context, userID, teamID, surveyType string) error {
	f.deletedDrafts = append(f.deletedDrafts, userID+"|"+teamID+"|"+surveyType)
	return f.deleteDraftErr
}

// setupSubmitRouter builds a minimal router exposing only POST /api/v1/health-checks, backed by
// the fake repository and a real JWT service/middleware so requests carry a genuine bearer token.
func setupSubmitRouter(t *testing.T, repo healthcheck.Repository) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	os.Setenv("JWT_SECRET", "test-secret-key-for-handler-unit-tests")
	t.Cleanup(func() { os.Unsetenv("JWT_SECRET") })

	jwtService := services.NewJWTService()
	tokenPair, err := jwtService.GenerateTokenPair(context.Background(), "unit_user1", "unit_user1", "unit1@test.com", "level-5", []string{"unit_team1"})
	require.NoError(t, err)

	router := gin.New()
	SetupHealthCheckRoutes(router, repo, nil, jwtService, nil)
	return router, tokenPair.AccessToken
}

func doSubmit(router *gin.Engine, token string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/health-checks", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func submitPayload(userID string, completed bool) map[string]any {
	return map[string]any{
		"teamId":           "unit_team1",
		"userId":           userID,
		"date":             "2024-01-15T10:00:00Z",
		"assessmentPeriod": "2023 - 2nd Half",
		"surveyType":       "individual",
		"completed":        completed,
		"responses": []map[string]any{
			{"dimensionId": "mission", "score": 3, "trend": "improving", "comment": ""},
		},
	}
}

// TestSubmitHealthCheck_DraftCleanupFailureIsNonFatal verifies that when the best-effort draft
// cleanup after a successful, completed submission fails, the submission itself still succeeds
// (the draft simply becomes stale rather than the user's real submission being lost).
func TestSubmitHealthCheck_DraftCleanupFailureIsNonFatal(t *testing.T) {
	repo := &fakeHealthCheckRepository{deleteDraftErr: assertErr("simulated delete failure")}
	router, token := setupSubmitRouter(t, repo)

	w := doSubmit(router, token, submitPayload("unit_user1", true))

	assert.Equal(t, http.StatusCreated, w.Code)
	require.Len(t, repo.deletedDrafts, 1)
	assert.Equal(t, "unit_user1|unit_team1|individual", repo.deletedDrafts[0])
}

// TestSubmitHealthCheck_IncompleteSubmissionNeverAttemptsCleanup verifies that an incomplete
// submission (Completed=false) never triggers a draft delete, so an in-progress draft is
// preserved for the user to resume.
func TestSubmitHealthCheck_IncompleteSubmissionNeverAttemptsCleanup(t *testing.T) {
	repo := &fakeHealthCheckRepository{}
	router, token := setupSubmitRouter(t, repo)

	w := doSubmit(router, token, submitPayload("unit_user1", false))

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Empty(t, repo.deletedDrafts)
}

// TestSubmitHealthCheck_RejectsUserIDMismatch verifies the authenticated-identity check: a
// caller must never be able to submit (and trigger cleanup for) another user's draft.
func TestSubmitHealthCheck_RejectsUserIDMismatch(t *testing.T) {
	repo := &fakeHealthCheckRepository{}
	router, token := setupSubmitRouter(t, repo)

	w := doSubmit(router, token, submitPayload("someone-else", true))

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Empty(t, repo.deletedDrafts)
}

type simpleError string

func (e simpleError) Error() string { return string(e) }

func assertErr(msg string) error { return simpleError(msg) }
