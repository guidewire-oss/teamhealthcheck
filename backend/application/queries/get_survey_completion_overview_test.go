package queries

import (
	"testing"

	"github.com/agopalakrishnan/teams360/backend/domain/organization"
	"github.com/agopalakrishnan/teams360/backend/domain/user"
)

func strPtr(s string) *string { return &s }

func TestResolveEffectiveDirector(t *testing.T) {
	director := &user.User{ID: "dir-1", Name: "Dana Director"}
	managerWithDirector := &user.User{ID: "mgr-1", Name: "Manny Manager", ReportsTo: strPtr("dir-1")}
	managerReportingToNonDirector := &user.User{ID: "mgr-2", Name: "Mo Manager", ReportsTo: strPtr("someone-else")}
	managerWithNoReportsTo := &user.User{ID: "mgr-3", Name: "May Manager"}

	directorByID := map[string]*user.User{"dir-1": director}
	managerByID := map[string]*user.User{
		"mgr-1": managerWithDirector,
		"mgr-2": managerReportingToNonDirector,
		"mgr-3": managerWithNoReportsTo,
	}

	tests := []struct {
		name             string
		chainDirectorID  string
		managerID        string
		wantDirectorID   string
	}{
		{
			name:            "chain already has a director, manager ignored",
			chainDirectorID: "dir-1",
			managerID:       "mgr-2", // reports to a non-director, but shouldn't matter
			wantDirectorID:  "dir-1",
		},
		{
			name:            "no chain director, manager resolves via reports_to",
			chainDirectorID: "",
			managerID:       "mgr-1",
			wantDirectorID:  "dir-1",
		},
		{
			name:            "no chain director, manager reports to someone who isn't a director",
			chainDirectorID: "",
			managerID:       "mgr-2",
			wantDirectorID:  "",
		},
		{
			name:            "no chain director, manager has no reports_to at all",
			chainDirectorID: "",
			managerID:       "mgr-3",
			wantDirectorID:  "",
		},
		{
			name:            "no chain director, no manager at all (Other candidate)",
			chainDirectorID: "",
			managerID:       "",
			wantDirectorID:  "",
		},
		{
			name:            "manager id set but unknown to the directory",
			chainDirectorID: "",
			managerID:       "mgr-unknown",
			wantDirectorID:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveEffectiveDirector(tt.chainDirectorID, tt.managerID, managerByID, directorByID)
			if got != tt.wantDirectorID {
				t.Errorf("resolveEffectiveDirector() = %q, want %q", got, tt.wantDirectorID)
			}
		})
	}
}

func TestHierarchyLevelIDForPosition(t *testing.T) {
	levels := []*organization.HierarchyLevel{
		{ID: "level-1", Position: 1},
		{ID: "level-2", Position: 2},
		{ID: "level-3", Position: 3},
	}

	if got := hierarchyLevelIDForPosition(levels, 2); got != "level-2" {
		t.Errorf("hierarchyLevelIDForPosition(2) = %q, want %q", got, "level-2")
	}
	if got := hierarchyLevelIDForPosition(levels, 3); got != "level-3" {
		t.Errorf("hierarchyLevelIDForPosition(3) = %q, want %q", got, "level-3")
	}
	if got := hierarchyLevelIDForPosition(levels, 99); got != "" {
		t.Errorf("hierarchyLevelIDForPosition(99) = %q, want empty", got)
	}
}
