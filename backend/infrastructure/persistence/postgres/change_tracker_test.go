package postgres

import (
	"testing"

	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
)

func finish(t *changeTracker) orgprovider.ApplyResult {
	var r orgprovider.ApplyResult
	t.finish(&r)
	return r
}

func TestChangeTrackerIdenticalResyncReportsOnlyUnchanged(t *testing.T) {
	tr := newChangeTracker()
	tr.processedUsers, tr.processedTeams = 1192, 237

	r := finish(tr)
	if r.InsertedUsers != 0 || r.UpdatedUsers != 0 || r.UnchangedUsers != 1192 {
		t.Fatalf("users: want 0 added, 0 updated, 1192 unchanged; got %d/%d/%d", r.InsertedUsers, r.UpdatedUsers, r.UnchangedUsers)
	}
	if r.InsertedTeams != 0 || r.UpdatedTeams != 0 || r.UnchangedTeams != 237 {
		t.Fatalf("teams: want 0 added, 0 updated, 237 unchanged; got %d/%d/%d", r.InsertedTeams, r.UpdatedTeams, r.UnchangedTeams)
	}
}

func TestChangeTrackerSplitsAddedUpdatedUnchanged(t *testing.T) {
	tr := newChangeTracker()
	tr.processedUsers = 10
	tr.insertedUsers["new-1"], tr.insertedUsers["new-2"] = true, true
	tr.changedUsers["old-1"] = true
	tr.processedTeams = 4
	tr.insertedTeams["t-new"] = true

	r := finish(tr)
	if r.InsertedUsers != 2 || r.UpdatedUsers != 1 || r.UnchangedUsers != 7 {
		t.Fatalf("users: got %d/%d/%d, want 2/1/7", r.InsertedUsers, r.UpdatedUsers, r.UnchangedUsers)
	}
	if r.InsertedTeams != 1 || r.UpdatedTeams != 0 || r.UnchangedTeams != 3 {
		t.Fatalf("teams: got %d/%d/%d, want 1/0/3", r.InsertedTeams, r.UpdatedTeams, r.UnchangedTeams)
	}
}

// A user inserted in this run who then also gets a manager link must count as
// added only, never also as updated.
func TestMarkChangedIgnoresRecordsInsertedThisRun(t *testing.T) {
	tr := newChangeTracker()
	tr.processedUsers = 2
	tr.insertedUsers["new"] = true
	markChanged(tr.changedUsers, tr.insertedUsers, "new")
	markChanged(tr.changedUsers, tr.insertedUsers, "existing")

	r := finish(tr)
	if r.InsertedUsers != 1 || r.UpdatedUsers != 1 || r.UnchangedUsers != 0 {
		t.Fatalf("got %d/%d/%d, want 1/1/0", r.InsertedUsers, r.UpdatedUsers, r.UnchangedUsers)
	}
}

// One user changed by several mechanisms (fields, manager, reactivation) is
// still a single updated user.
func TestUserChangedByManyMechanismsCountsOnce(t *testing.T) {
	tr := newChangeTracker()
	tr.processedUsers = 3
	for i := 0; i < 3; i++ {
		markChanged(tr.changedUsers, tr.insertedUsers, "u1")
	}
	r := finish(tr)
	if r.UpdatedUsers != 1 || r.UnchangedUsers != 2 {
		t.Fatalf("got updated=%d unchanged=%d, want 1/2", r.UpdatedUsers, r.UnchangedUsers)
	}
}

func TestChangeTrackerNeverReportsNegativeUnchanged(t *testing.T) {
	tr := newChangeTracker()
	tr.processedUsers = 0
	tr.changedUsers["x"] = true
	if r := finish(tr); r.UnchangedUsers != 0 {
		t.Fatalf("unchanged must clamp at 0, got %d", r.UnchangedUsers)
	}
}
