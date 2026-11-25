package orgsyncrun_test

import (
	"testing"

	"github.com/agopalakrishnan/teams360/backend/domain/orgsyncrun"
)

func TestCanTransition(t *testing.T) {
	P, R, S, F := orgsyncrun.StatusPending, orgsyncrun.StatusRunning, orgsyncrun.StatusSuccess, orgsyncrun.StatusFailed
	allowed := map[[2]orgsyncrun.Status]bool{{P, R}: true, {P, F}: true, {R, S}: true, {R, F}: true}
	all := []orgsyncrun.Status{P, R, S, F}
	for _, from := range all {
		for _, to := range all {
			if got := orgsyncrun.CanTransition(from, to); got != allowed[[2]orgsyncrun.Status{from, to}] {
				t.Errorf("CanTransition(%s,%s)=%v, want %v", from, to, got, !got)
			}
		}
	}
}

func TestStatusesBeforeMatchesCanTransition(t *testing.T) {
	want := map[orgsyncrun.Status][]string{
		orgsyncrun.StatusPending: nil,
		orgsyncrun.StatusRunning: {"PENDING"},
		orgsyncrun.StatusSuccess: {"RUNNING"},
		orgsyncrun.StatusFailed:  {"PENDING", "RUNNING"},
	}
	for to, expected := range want {
		got := orgsyncrun.StatusesBefore(to)
		if len(got) != len(expected) {
			t.Fatalf("StatusesBefore(%s)=%v, want %v", to, got, expected)
		}
		for i := range got {
			if got[i] != expected[i] {
				t.Errorf("StatusesBefore(%s)=%v, want %v", to, got, expected)
			}
		}
	}
	if got := orgsyncrun.ActiveStatuses(); len(got) != 2 || got[0] != "PENDING" || got[1] != "RUNNING" {
		t.Errorf("ActiveStatuses()=%v, want [PENDING RUNNING]", got)
	}
}

func TestTerminalStatesCannotBeOverwritten(t *testing.T) {
	for _, term := range []orgsyncrun.Status{orgsyncrun.StatusSuccess, orgsyncrun.StatusFailed} {
		if !term.Terminal() {
			t.Errorf("%s should be terminal", term)
		}
		for _, to := range []orgsyncrun.Status{orgsyncrun.StatusPending, orgsyncrun.StatusRunning, orgsyncrun.StatusSuccess, orgsyncrun.StatusFailed} {
			if orgsyncrun.CanTransition(term, to) {
				t.Errorf("terminal %s must not move to %s", term, to)
			}
		}
	}
}
