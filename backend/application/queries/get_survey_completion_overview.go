package queries

import (
	"context"
	"fmt"
	"time"

	"github.com/agopalakrishnan/teams360/backend/domain/healthcheck"
	"github.com/agopalakrishnan/teams360/backend/domain/organization"
	"github.com/agopalakrishnan/teams360/backend/domain/team"
	"github.com/agopalakrishnan/teams360/backend/domain/user"
)

const (
	directorLevelPosition = 2
	managerLevelPosition  = 3
)

// GetSurveyCompletionOverviewQuery represents the query to build the
// admin survey-completion dashboard for one assessment period.
type GetSurveyCompletionOverviewQuery struct {
	AssessmentPeriod string
}

// SurveyCompletionTeam is one team's completion snapshot for the requested
// assessment period, along with the hierarchy owners used to group it in the
// admin dashboard.
//
// DirectorID/Name/Email is the "effective" director to nest this team's
// manager under (see resolveEffectiveDirector) — empty when the team has no
// manager or no resolvable director. ManagerID/Name/Email is empty when the
// team has no Level-3 supervisor of its own. TeamLeadID/Name/Email is the
// Level-4 team lead, always populated — the reminder-target tree shows the
// team lead alongside whichever director/manager owns the pod, and escalates
// to the team lead alone when both are empty.
type SurveyCompletionTeam struct {
	TeamID                string
	TeamName              string
	DirectorID            string
	DirectorName          string
	DirectorEmail         string
	ManagerID             string
	ManagerName           string
	ManagerEmail          string
	TeamLeadID            string
	TeamLeadName          string
	TeamLeadEmail         string
	Completed             int
	Total                 int
	PostWorkshopCompleted bool
	HealthCheckEnabled    bool
}

// SurveyCompletionTrendPoint is one weekly bucket of cumulative individual
// survey completion, expressed as a percentage of opted-in members.
type SurveyCompletionTrendPoint struct {
	Label      string
	Completion int
}

// SurveyCompletionOverview is the full aggregated result for the admin
// survey-completion dashboard.
type SurveyCompletionOverview struct {
	AssessmentPeriod string
	Teams            []SurveyCompletionTeam
	TimeSeries       []SurveyCompletionTrendPoint
}

// GetSurveyCompletionOverviewHandler aggregates per-team survey completion
// across every team in the organization.
type GetSurveyCompletionOverviewHandler struct {
	teamRepo        team.Repository
	healthCheckRepo healthcheck.Repository
	userRepo        user.Repository
	orgRepo         organization.Repository
}

// NewGetSurveyCompletionOverviewHandler creates a new query handler.
func NewGetSurveyCompletionOverviewHandler(teamRepo team.Repository, healthCheckRepo healthcheck.Repository, userRepo user.Repository, orgRepo organization.Repository) *GetSurveyCompletionOverviewHandler {
	return &GetSurveyCompletionOverviewHandler{
		teamRepo:        teamRepo,
		healthCheckRepo: healthCheckRepo,
		userRepo:        userRepo,
		orgRepo:         orgRepo,
	}
}

// resolveEffectiveDirector returns the director id to nest a team's manager
// under: the team's own chain director if it has one, otherwise the
// manager's reports_to director — but only when that reports_to user is
// actually a director (guards against a manager reporting to another
// manager, or to nobody). Returns "" when neither resolves, meaning the
// manager (if any) becomes a top-level group of its own.
func resolveEffectiveDirector(chainDirectorID, managerID string, managerByID, directorByID map[string]*user.User) string {
	if chainDirectorID != "" {
		return chainDirectorID
	}
	mgr, ok := managerByID[managerID]
	if !ok || mgr.ReportsTo == nil {
		return ""
	}
	if _, isDirector := directorByID[*mgr.ReportsTo]; !isDirector {
		return ""
	}
	return *mgr.ReportsTo
}

// hierarchyLevelIDForPosition returns the id of the hierarchy level at the
// given position (2 = Director, 3 = Manager), or "" if the organization has
// no level configured at that position.
func hierarchyLevelIDForPosition(levels []*organization.HierarchyLevel, position int) string {
	for _, l := range levels {
		if l.Position == position {
			return l.ID
		}
	}
	return ""
}

// Handle executes the query.
func (h *GetSurveyCompletionOverviewHandler) Handle(ctx context.Context, query GetSurveyCompletionOverviewQuery) (*SurveyCompletionOverview, error) {
	teams, err := h.teamRepo.FindSurveyCompletionTeams(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load teams: %w", err)
	}

	sessions, err := h.healthCheckRepo.FindByAssessmentPeriod(ctx, query.AssessmentPeriod)
	if err != nil {
		return nil, fmt.Errorf("failed to load sessions: %w", err)
	}

	managerByID, directorByID, err := h.loadHierarchyDirectory(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load hierarchy directory: %w", err)
	}

	type teamProgress struct {
		completedUsers map[string]bool
		postWorkshop   bool
	}
	progress := make(map[string]*teamProgress)
	for _, s := range sessions {
		if !s.Completed {
			continue
		}
		p, ok := progress[s.TeamID]
		if !ok {
			p = &teamProgress{completedUsers: make(map[string]bool)}
			progress[s.TeamID] = p
		}
		if s.SurveyType == healthcheck.SurveyTypePostWorkshop {
			p.postWorkshop = true
		} else {
			p.completedUsers[s.UserID] = true
		}
	}

	result := make([]SurveyCompletionTeam, 0, len(teams))
	for _, t := range teams {
		completed := 0
		postWorkshop := false
		if p, ok := progress[t.ID]; ok {
			completed = len(p.completedUsers)
			postWorkshop = p.postWorkshop
		}

		directorID := resolveEffectiveDirector(t.ChainDirectorID, t.ManagerID, managerByID, directorByID)

		sct := SurveyCompletionTeam{
			TeamID:                t.ID,
			TeamName:              t.Name,
			ManagerID:             t.ManagerID,
			DirectorID:            directorID,
			TeamLeadID:            t.TeamLeadID,
			TeamLeadName:          t.TeamLeadName,
			TeamLeadEmail:         t.TeamLeadEmail,
			Completed:             completed,
			Total:                 t.MemberCount,
			PostWorkshopCompleted: postWorkshop,
			HealthCheckEnabled:    t.HealthCheckEnabled,
		}
		if mgr, ok := managerByID[t.ManagerID]; ok {
			sct.ManagerName = mgr.Name
			sct.ManagerEmail = mgr.Email
		}
		if dir, ok := directorByID[directorID]; ok {
			sct.DirectorName = dir.Name
			sct.DirectorEmail = dir.Email
		}
		result = append(result, sct)
	}

	return &SurveyCompletionOverview{
		AssessmentPeriod: query.AssessmentPeriod,
		Teams:            result,
		TimeSeries:       buildSurveyCompletionTrend(teams, sessions),
	}, nil
}

// loadHierarchyDirectory returns every Level-3 (manager) and Level-2
// (director) user, keyed by id, for resolving each team's effective
// director. Returns empty maps (not an error) when the organization has no
// level configured at either position — every team then simply falls back
// to the "Other" group.
func (h *GetSurveyCompletionOverviewHandler) loadHierarchyDirectory(ctx context.Context) (managerByID, directorByID map[string]*user.User, err error) {
	levels, err := h.orgRepo.FindHierarchyLevels(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load hierarchy levels: %w", err)
	}

	managerByID = make(map[string]*user.User)
	directorByID = make(map[string]*user.User)

	if managerLevelID := hierarchyLevelIDForPosition(levels, managerLevelPosition); managerLevelID != "" {
		managers, err := h.userRepo.FindByHierarchyLevel(ctx, managerLevelID)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load managers: %w", err)
		}
		for _, m := range managers {
			managerByID[m.ID] = m
		}
	}

	if directorLevelID := hierarchyLevelIDForPosition(levels, directorLevelPosition); directorLevelID != "" {
		directors, err := h.userRepo.FindByHierarchyLevel(ctx, directorLevelID)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load directors: %w", err)
		}
		for _, d := range directors {
			directorByID[d.ID] = d
		}
	}

	return managerByID, directorByID, nil
}

// buildSurveyCompletionTrend computes cumulative individual-survey
// completion, as a percentage of opted-in members, bucketed by calendar
// week of the session date. Weeks are numbered as days-since-epoch/7 rather
// than ISO week-of-year so a period spanning a year boundary still buckets
// and orders correctly.
func buildSurveyCompletionTrend(teams []team.SurveyCompletionRow, sessions []*healthcheck.HealthCheckSession) []SurveyCompletionTrendPoint {
	optedInMembers := 0
	optedIn := make(map[string]bool)
	for _, t := range teams {
		if t.HealthCheckEnabled {
			optedInMembers += t.MemberCount
			optedIn[t.ID] = true
		}
	}
	if optedInMembers == 0 {
		return []SurveyCompletionTrendPoint{}
	}

	type completionKey struct{ team, user string }
	seen := make(map[completionKey]bool)
	weekCounts := make(map[int]int)
	minWeek, maxWeek := 0, 0
	haveWeek := false

	for _, s := range sessions {
		if !s.Completed || s.SurveyType == healthcheck.SurveyTypePostWorkshop || !optedIn[s.TeamID] {
			continue
		}
		k := completionKey{s.TeamID, s.UserID}
		if seen[k] {
			continue
		}
		d, err := time.Parse(time.RFC3339, s.Date)
		if err != nil {
			// Fall back to a bare date in case the source data ever stores
			// it without a time component.
			d, err = time.Parse("2006-01-02", s.Date)
			if err != nil {
				continue
			}
		}
		seen[k] = true

		week := int(d.Unix() / (7 * 24 * 3600))
		weekCounts[week]++
		if !haveWeek {
			minWeek, maxWeek, haveWeek = week, week, true
			continue
		}
		if week < minWeek {
			minWeek = week
		}
		if week > maxWeek {
			maxWeek = week
		}
	}

	if !haveWeek {
		return []SurveyCompletionTrendPoint{}
	}

	points := make([]SurveyCompletionTrendPoint, 0, maxWeek-minWeek+1)
	cumulative := 0
	label := 0
	for w := minWeek; w <= maxWeek; w++ {
		cumulative += weekCounts[w]
		label++
		points = append(points, SurveyCompletionTrendPoint{
			Label:      fmt.Sprintf("Wk %d", label),
			Completion: percent(cumulative, optedInMembers),
		})
	}
	return points
}

func percent(part, total int) int {
	if total == 0 {
		return 0
	}
	return int((float64(part)/float64(total))*100 + 0.5)
}
