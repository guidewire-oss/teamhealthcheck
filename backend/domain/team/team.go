package team

import (
	"context"
	"time"
)

// Team represents a team in the organization
// This is an aggregate root in DDD terms
type Team struct {
	ID                    string           `json:"id"`
	Name                  string           `json:"name"`
	Cadence               string           `json:"cadence"` // monthly, quarterly, half-yearly, yearly
	NextCheckDate         string           `json:"nextCheckDate"`
	TeamLeadID            *string          `json:"teamLeadId,omitempty"`
	TeamLeadName          *string          `json:"teamLeadName,omitempty"`
	Members               []TeamMember     `json:"members"`
	MemberCount           int              `json:"memberCount"`
	SupervisorChain       []SupervisorLink `json:"supervisorChain"`
	DistributionListEmail *string          `json:"distributionListEmail,omitempty"`
	Department            string           `json:"department,omitempty"`
	Division              string           `json:"division,omitempty"`
	Tags                  []string         `json:"tags,omitempty"`
	CreatedAt             time.Time        `json:"createdAt,omitempty"`
	UpdatedAt             time.Time        `json:"updatedAt,omitempty"`
}

// TeamMember represents a member of a team
type TeamMember struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	FullName string `json:"fullName"`
	Email    string `json:"email,omitempty"`
}

// SupervisorLink represents a link in the supervisor chain
type SupervisorLink struct {
	UserID  string `json:"userId"`
	LevelID string `json:"levelId"`
}

// Member represents a team member with their role
type Member struct {
	UserID string `json:"userId"`
	Role   string `json:"role,omitempty"` // lead, member
}

// SurveyCompletionRow is a lightweight per-team projection for the admin
// survey-completion dashboard (hierarchy owner and the opt-in flag). Kept
// separate from Team, rather than adding fields to it, because it depends
// on a column (health_check_enabled) that is not present on every database
// yet — see FindSurveyCompletionTeams.
//
// There is deliberately no member-count field here: "how many members"
// depends on WHICH members are eligible for the individual survey (Level 4
// Team Lead + Level 5 Team Member only — see FindEligibleMemberIDs), so
// that count is derived by the caller from FindEligibleMemberIDs' result,
// never from a raw, unfiltered team_members count.
//
// SupervisorID is this team's own closest supervisor as recorded in
// team_supervisors (the row with the lowest `position`, i.e. "1 = closest
// supervisor" per that column's own definition) — "" when the team has no
// supervisor recorded at all. The caller
// (GetSurveyCompletionOverviewHandler) resolves this person's own
// ancestors, arbitrarily deep, purely via users.reports_to — team_supervisors
// is only ever consulted for the single closest supervisor, never for the
// rest of the chain. TeamLead* is the Level-4 escalation target, always
// populated when the team has a team lead.
type SurveyCompletionRow struct {
	ID                 string
	Name               string
	SupervisorID       string
	TeamLeadID         string
	TeamLeadName       string
	TeamLeadEmail      string
	HealthCheckEnabled bool
}

// Repository defines the interface for team data access
type Repository interface {
	FindByID(ctx context.Context, id string) (*Team, error)
	FindAll(ctx context.Context) ([]*Team, error)
	FindByLeadID(ctx context.Context, leadID string) ([]*Team, error)
	FindBySupervisorID(ctx context.Context, supervisorID string) ([]*Team, error)
	FindMembers(ctx context.Context, teamID string) ([]*Member, error)
	FindSupervisorChain(ctx context.Context, teamID string) ([]*SupervisorLink, error)
	Save(ctx context.Context, team *Team) error
	Update(ctx context.Context, team *Team) error
	Delete(ctx context.Context, id string) error
	AddMember(ctx context.Context, teamID, userID string) error
	RemoveMember(ctx context.Context, teamID, userID string) error
	UpdateSupervisorChain(ctx context.Context, teamID string, chain []*SupervisorLink) error
	// Additional methods for team details
	FindTeamMembers(ctx context.Context, teamID string) ([]TeamMember, error)
	CountTeamMembers(ctx context.Context, teamID string) (int, error)
	FindAllWithDetails(ctx context.Context) ([]Team, error)
	// FindSurveyCompletionTeams returns the admin survey-completion projection
	// for every team. Requires the health_check_enabled column on teams —
	// only this method selects it, so every other team query keeps working
	// unchanged against databases that haven't added it yet.
	FindSurveyCompletionTeams(ctx context.Context) ([]SurveyCompletionRow, error)
	// FindEligibleMemberIDs returns, for every team, the ids of its members
	// who are eligible to take the individual survey -- authoritatively
	// defined as hierarchy_levels.position exactly 4 (Team Lead) or exactly
	// 5 (Team Member), joined dynamically through users.hierarchy_level_id
	// and hierarchy_levels.position -- never a hard-coded level id/name.
	// Managers, Senior Managers, Directors, VPs, and any other position are
	// excluded. This is the single authoritative eligible population every
	// survey-completion status, filter, and aggregate (Individual Survey
	// Completed, Not Started, Fully Completed, Opted Out, Opted In, Total
	// Teams) must be calculated from -- see
	// GetSurveyCompletionOverviewHandler.Handle.
	FindEligibleMemberIDs(ctx context.Context) (map[string][]string, error)
}
