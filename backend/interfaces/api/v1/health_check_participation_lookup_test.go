package v1_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agopalakrishnan/teams360/backend/domain/healthcheck"
	v1 "github.com/agopalakrishnan/teams360/backend/interfaces/api/v1"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/gin-gonic/gin"
)

// lookupFailingRepo fails the enrollment lookup. Any other repository call hits the embedded nil
// interface and panics, which fails the test: the lookup must stop the submission first.
type lookupFailingRepo struct {
	healthcheck.Repository
	cause error
}

func (r *lookupFailingRepo) IsTeamHealthCheckEnabled(context.Context, string) (bool, bool, error) {
	return false, false, r.cause
}

// A failed enrollment lookup is the server's fault, so it is a 500 -- not the 400 reserved for a bad
// request -- and the database detail stays out of the response.
func TestSubmitHealthCheckReportsAFailedEnrollmentLookupAsAServerError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := v1.NewHealthCheckHandler(&lookupFailingRepo{cause: errors.New("pq: connection reset by peer")}, nil, nil)
	r := gin.New()
	r.POST("/health-checks", h.SubmitHealthCheck)

	body := `{"teamId":"t1","userId":"u1","date":"2026-01-15T10:00:00Z","assessmentPeriod":"2026 H1","surveyType":"individual","completed":true,
		"responses":[{"dimensionId":"mission","score":3,"trend":"stable"}]}`
	req := httptest.NewRequest(http.MethodPost, "/health-checks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("got %d, want 500: %s", w.Code, w.Body.String())
	}
	var resp dto.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is not an ErrorResponse: %v", err)
	}
	if strings.Contains(w.Body.String(), "connection reset") || strings.Contains(w.Body.String(), "pq:") {
		t.Fatalf("database detail leaked to the client: %s", w.Body.String())
	}
}
