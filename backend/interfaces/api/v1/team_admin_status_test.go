package v1_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/agopalakrishnan/teams360/backend/domain/team"
	v1 "github.com/agopalakrishnan/teams360/backend/interfaces/api/v1"
	"github.com/gin-gonic/gin"
)

// statusTeamRepo implements only what the handlers under test touch. The
// embedded nil interface means any other call -- Delete in particular -- panics,
// which fails the test: teams must never be hard-deleted through this API.
type statusTeamRepo struct {
	team.Repository
	teams         []team.Team
	deactivated   []string
	deactivateErr error
}

func (r *statusTeamRepo) FindAllWithDetails(_ context.Context) ([]team.Team, error) {
	return r.teams, nil
}

func (r *statusTeamRepo) Deactivate(_ context.Context, id string) error {
	if r.deactivateErr != nil {
		return r.deactivateErr
	}
	r.deactivated = append(r.deactivated, id)
	return nil
}

func newTeamRouter(repo *statusTeamRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := v1.NewTeamAdminHandler(repo, nil, nil)
	r := gin.New()
	r.GET("/teams", h.ListTeams)
	r.DELETE("/teams/:id", h.DeleteTeam)
	return r
}

type teamListBody struct {
	Total int `json:"total"`
	Teams []struct {
		ID            string     `json:"id"`
		IsActive      bool       `json:"isActive"`
		DeactivatedAt *time.Time `json:"deactivatedAt"`
	} `json:"teams"`
}

func listTeams(t *testing.T, repo *statusTeamRepo, path string) (int, teamListBody) {
	t.Helper()
	w := call(newTeamRouter(repo), http.MethodGet, path)
	var body teamListBody
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w.Code, body
}

func twoTeams() *statusTeamRepo {
	when := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	return &statusTeamRepo{teams: []team.Team{
		{ID: "t-active", Name: "Hopefarm", IsActive: true},
		{ID: "t-inactive", Name: "Old Squad", IsActive: false, DeactivatedAt: &when},
	}}
}

func TestListTeamsShowsActiveAndInactiveByDefault(t *testing.T) {
	code, body := listTeams(t, twoTeams(), "/teams")
	if code != http.StatusOK || body.Total != 2 {
		t.Fatalf("inactive teams must stay listed: code=%d total=%d", code, body.Total)
	}
	if !body.Teams[0].IsActive || body.Teams[1].IsActive {
		t.Fatalf("want the first team active and the second inactive, got %+v", body.Teams)
	}
	if body.Teams[0].DeactivatedAt != nil || body.Teams[1].DeactivatedAt == nil {
		t.Fatalf("only the inactive team carries a deactivation time, got %+v", body.Teams)
	}
}

func TestListTeamsStatusFilter(t *testing.T) {
	cases := map[string][]string{
		"/teams?status=all":      {"t-active", "t-inactive"},
		"/teams?status=":         {"t-active", "t-inactive"},
		"/teams?status=active":   {"t-active"},
		"/teams?status=inactive": {"t-inactive"},
		"/teams?status=INACTIVE": {"t-inactive"},
	}
	for path, want := range cases {
		code, body := listTeams(t, twoTeams(), path)
		if code != http.StatusOK {
			t.Fatalf("%s: want 200, got %d", path, code)
		}
		got := []string{}
		for _, tm := range body.Teams {
			got = append(got, tm.ID)
		}
		if len(got) != len(want) {
			t.Errorf("%s: got %v, want %v", path, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s: got %v, want %v", path, got, want)
			}
		}
		if body.Total != len(want) {
			t.Errorf("%s: total=%d, want %d", path, body.Total, len(want))
		}
	}
}

func TestListTeamsRejectsAnUnknownStatus(t *testing.T) {
	if code, _ := listTeams(t, twoTeams(), "/teams?status=deleted"); code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", code)
	}
}

func TestDeleteTeamRouteSoftDeactivatesAndNeverHardDeletes(t *testing.T) {
	repo := &statusTeamRepo{}
	w := call(newTeamRouter(repo), http.MethodDelete, "/teams/some-team") // a hard Delete would panic the fake
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
	if len(repo.deactivated) != 1 || repo.deactivated[0] != "some-team" {
		t.Fatalf("want some-team deactivated, got %v", repo.deactivated)
	}
}

func TestDeactivateUnknownTeamIs404(t *testing.T) {
	repo := &statusTeamRepo{deactivateErr: errors.New("team not found: nobody")}
	if w := call(newTeamRouter(repo), http.MethodDelete, "/teams/nobody"); w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func TestDeactivateTeamFailureIs500(t *testing.T) {
	repo := &statusTeamRepo{deactivateErr: errors.New("connection reset")}
	if w := call(newTeamRouter(repo), http.MethodDelete, "/teams/t"); w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}
