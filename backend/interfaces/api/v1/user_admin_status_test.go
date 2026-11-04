package v1_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agopalakrishnan/teams360/backend/domain/user"
	v1 "github.com/agopalakrishnan/teams360/backend/interfaces/api/v1"
	"github.com/gin-gonic/gin"
)

// statusUserRepo implements only what the handlers under test touch. The
// embedded nil interface means any other call -- Delete in particular -- panics,
// which fails the test: people must never be hard-deleted through this API.
type statusUserRepo struct {
	user.Repository
	users         []*user.User
	lastFilter    user.ListFilter
	deactivated   []string
	deactivateErr error
}

func (r *statusUserRepo) FindPage(_ context.Context, f user.ListFilter, _, _ int) ([]*user.User, int, error) {
	r.lastFilter = f
	return r.users, len(r.users), nil
}

func (r *statusUserRepo) Deactivate(_ context.Context, id string) error {
	if r.deactivateErr != nil {
		return r.deactivateErr
	}
	r.deactivated = append(r.deactivated, id)
	return nil
}

func newUserRouter(repo *statusUserRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := v1.NewUserAdminHandler(repo, nil)
	r := gin.New()
	r.GET("/users", h.ListUsers)
	r.DELETE("/users/:id", h.DeleteUser)
	return r
}

func call(r *gin.Engine, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestListUsersExposesActiveAndInactiveStatus(t *testing.T) {
	when := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	repo := &statusUserRepo{users: []*user.User{
		{ID: "a", Username: "alice", Name: "Alice", IsActive: true},
		{ID: "b", Username: "bob", Name: "Bob", IsActive: false, DeactivatedAt: &when},
	}}
	w := call(newUserRouter(repo), http.MethodGet, "/users")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
	var body struct {
		Users []struct {
			ID            string     `json:"id"`
			IsActive      bool       `json:"isActive"`
			DeactivatedAt *time.Time `json:"deactivatedAt"`
		} `json:"users"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Users) != 2 || !body.Users[0].IsActive || body.Users[1].IsActive {
		t.Fatalf("want alice active and bob inactive, got %+v", body.Users)
	}
	if body.Users[0].DeactivatedAt != nil || body.Users[1].DeactivatedAt == nil {
		t.Fatalf("only the inactive person carries a deactivation time, got %+v", body.Users)
	}
}

func TestListUsersStatusFilter(t *testing.T) {
	cases := map[string]string{
		"/users":                 "", // everyone: inactive people stay listed by default
		"/users?status=all":      "",
		"/users?status=":         "",
		"/users?status=active":   user.StatusActive,
		"/users?status=inactive": user.StatusInactive,
		"/users?status=INACTIVE": user.StatusInactive,
	}
	for path, want := range cases {
		repo := &statusUserRepo{}
		if w := call(newUserRouter(repo), http.MethodGet, path); w.Code != http.StatusOK {
			t.Fatalf("%s: want 200, got %d", path, w.Code)
		}
		if repo.lastFilter.Status != want {
			t.Errorf("%s: filter status = %q, want %q", path, repo.lastFilter.Status, want)
		}
	}
}

func TestListUsersKeepsSearchTogetherWithStatus(t *testing.T) {
	repo := &statusUserRepo{}
	call(newUserRouter(repo), http.MethodGet, "/users?search=bob&status=inactive&role=level-5")
	if repo.lastFilter.Search != "bob" || repo.lastFilter.Status != user.StatusInactive || repo.lastFilter.HierarchyLevel != "level-5" {
		t.Fatalf("filters must combine, got %+v", repo.lastFilter)
	}
}

func TestListUsersRejectsAnUnknownStatus(t *testing.T) {
	repo := &statusUserRepo{}
	if w := call(newUserRouter(repo), http.MethodGet, "/users?status=deleted"); w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestDeleteRouteSoftDeactivatesAndNeverHardDeletes(t *testing.T) {
	repo := &statusUserRepo{}
	w := call(newUserRouter(repo), http.MethodDelete, "/users/some-user") // a hard Delete would panic the fake
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
	if len(repo.deactivated) != 1 || repo.deactivated[0] != "some-user" {
		t.Fatalf("want some-user deactivated, got %v", repo.deactivated)
	}
}

func TestDeactivateUnknownUserIs404(t *testing.T) {
	repo := &statusUserRepo{deactivateErr: errors.New("user not found with provided ID")}
	if w := call(newUserRouter(repo), http.MethodDelete, "/users/nobody"); w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func TestDeactivateFailureIs500(t *testing.T) {
	repo := &statusUserRepo{deactivateErr: errors.New("connection reset")}
	if w := call(newUserRouter(repo), http.MethodDelete, "/users/u"); w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}
