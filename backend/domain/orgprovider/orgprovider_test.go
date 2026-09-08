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
