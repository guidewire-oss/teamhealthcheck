package v1_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agopalakrishnan/teams360/backend/domain/team"
	v1 "github.com/agopalakrishnan/teams360/backend/interfaces/api/v1"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/gin-gonic/gin"
)

// participationRepo counts FindByID calls. The participation guard must reject before the handler
// touches the repository, so a rejected request leaves the count at zero. Any other repository call
// hits the embedded nil interface and panics, failing the test.
type participationRepo struct {
	team.Repository
	findByIDCalls int
}

func (r *participationRepo) FindByID(context.Context, string) (*team.Team, error) {
	r.findByIDCalls++
	return nil, errors.New("not found") // lets an accepted request stop with a clean 404
}

func putTeam(repo *participationRepo, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	h := v1.NewTeamAdminHandler(repo, nil, nil)
	r := gin.New()
	r.PUT("/teams/:id", h.UpdateTeam)

	req := httptest.NewRequest(http.MethodPut, "/teams/t1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// Presence is what is rejected, not the value: true, false and an explicit null all count as an
// attempt to edit PodIQ-owned participation.
func TestUpdateTeamRejectsAnyHealthCheckEnabledValue(t *testing.T) {
	for _, value := range []string{"true", "false", "null"} {
		t.Run(value, func(t *testing.T) {
			repo := &participationRepo{}
			w := putTeam(repo, `{"name":"Renamed","healthCheckEnabled":`+value+`}`)

			if w.Code != http.StatusConflict {
				t.Fatalf("healthCheckEnabled=%s: got %d, want 409: %s", value, w.Code, w.Body.String())
			}
			var resp dto.ErrorResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("response is not an ErrorResponse: %v", err)
			}
			if resp.Code != dto.CodeParticipationManagedByPodIQ {
				t.Fatalf("code = %q, want %q", resp.Code, dto.CodeParticipationManagedByPodIQ)
			}
			if repo.findByIDCalls != 0 {
				t.Fatalf("a rejected request must not reach the repository, got %d call(s)", repo.findByIDCalls)
			}
		})
	}
}

func TestUpdateTeamAcceptsARequestThatOmitsHealthCheckEnabled(t *testing.T) {
	repo := &participationRepo{}
	w := putTeam(repo, `{"name":"Renamed"}`)

	if w.Code == http.StatusConflict {
		t.Fatalf("an ordinary edit must not be rejected as a participation edit: %s", w.Body.String())
	}
	if repo.findByIDCalls != 1 {
		t.Fatalf("the edit should have proceeded to the repository once, got %d call(s)", repo.findByIDCalls)
	}
}
