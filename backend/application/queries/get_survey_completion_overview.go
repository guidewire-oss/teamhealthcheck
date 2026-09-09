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

// leadershipFloorPosition is the shallowest hierarchy_levels.position that
// ever participates in the survey-completion hierarchy tree — that tier
// (Director-equivalent, whatever it happens to be named) and everything
// below it (Manager, Senior Manager, ... however many tiers an organization
// defines) can nest in the tree; anyone above it (VP, Admin) never becomes a
// node. This is what makes a Level-2 leader with no Level-2 (or lower)
// parent a root even when their own reports_to points at a VP, rather than
// nesting the whole tree under that VP.
const leadershipFloorPosition = 2

// GetSurveyCompletionOverviewQuery represents the query to build the
// admin survey-completion dashboard for one assessment period.
type GetSurveyCompletionOverviewQuery struct {
	AssessmentPeriod string
}

// SurveyCompletionTeam is one team's completion snapshot for the requested
// assessment period, plus the id of the single PersonNode it nests directly
// under. OwnerUserID is "" when the team has no resolvable owner (no
// supervisor recorded, or that supervisor sits above leadershipFloorPosition)
// — such a team lands in the "Other" catch-all.
//
// Completed and Total are both scoped to the SAME eligible population —
// Level 4 (Team Lead) + Level 5 (Team Member) members only, from
// team.Repository.FindEligibleMemberIDs — never a raw/unfiltered member
// count. Not Started, Opted Out, and Opted In are derived purely from these
// two fields plus HealthCheckEnabled; Complete additionally requires
// PostWorkshopCompleted (see deriveSurveyCompletionStatus) — Completed and
// Total alone can never disagree with the Individual Survey Completed value
// shown for this same team, since that value IS Completed/Total.
type SurveyCompletionTeam struct {
	TeamID                string
	TeamName              string
	OwnerUserID           string
	TeamLeadID            string
	TeamLeadName          string
	TeamLeadEmail         string
	Completed             int
	Total                 int
	PostWorkshopCompleted bool
	HealthCheckEnabled    bool
}

// PersonNode is one leadership user in the survey-completion hierarchy
// tree: the closest supervisor of at least one team, or an ancestor (via
// users.reports_to) of such a person, walked up to and including
// leadershipFloorPosition. ParentID is "" when this person is a root —
// their own reports_to is empty, unresolvable, or points to someone above
// leadershipFloorPosition (see resolveParentID). This same rule applies
// uniformly regardless of this person's own level, which is what allows a
// Level-2 leader to nest under another Level-2 leader, a Level-3 leader to
// nest under a Level-2 *or* another Level-3 leader, and so on to arbitrary
// depth — no fixed two-tier assumption anywhere.
type PersonNode struct {
	UserID    string
	Name      string
	Email     string
	LevelID   string
	LevelName string
	ParentID  string
}

// SurveyCompletionTrendPoint is one weekly bucket of cumulative individual
// survey completion, expressed as a percentage of opted-in members.
type SurveyCompletionTrendPoint struct {
	Label      string
	Completion int
}

// SurveyCompletionOverview is the full aggregated result for the admin
// survey-completion dashboard: every team (flat, for the summary cards),
// every PersonNode needed to render the hierarchy those teams nest into,
// and the completion trend series.
type SurveyCompletionOverview struct {
	AssessmentPeriod string
	Teams            []SurveyCompletionTeam
	Persons          []PersonNode
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

// isAboveLeadershipFloor reports whether the given hierarchy level id sits
// strictly above leadershipFloorPosition (e.g. VP, Admin), or has no known
// position at all — such a user never becomes a hierarchy tree node.
func isAboveLeadershipFloor(levelID string, positionByLevelID map[string]int) bool {
	position, ok := positionByLevelID[levelID]
	return !ok || position < leadershipFloorPosition
}

// resolveParentID returns the user id this person's node should nest
// under: their own reports_to, but only when that target exists, has a
// hierarchy level, and that level is at or below leadershipFloorPosition.
// Returns "" (this person is a root) otherwise. Applying this one rule
// uniformly at every depth — rather than only between two fixed tiers — is
// what makes the whole hierarchy recursive to arbitrary depth.
func resolveParentID(u *user.User, usersByID map[string]*user.User, positionByLevelID map[string]int) string {
	if u == nil || u.ReportsTo == nil {
		return ""
	}
	parent, ok := usersByID[*u.ReportsTo]
	if !ok || isAboveLeadershipFloor(parent.HierarchyLevelID, positionByLevelID) {
		return ""
	}
	return parent.ID
}

// teamProgress is one team's individual-survey completion state for the
// requested assessment period: which users completed it, and whether a
// post-workshop session was also completed.
type teamProgress struct {
	completedUsers map[string]bool
	postWorkshop   bool
}

// buildTeamProgress groups completed sessions by team, keyed by survey
// type. completedUsers is a set (never a count) so a user with more than
// one completed individual-survey session row for this team — a
// resubmission, a duplicate row, etc. — is only ever counted once; that
// same set is later intersected with the eligible population by
// eligibleTeamCompletion, so a completed session from a user who isn't
// eligible for this team never inflates its completed count either.
func buildTeamProgress(sessions []*healthcheck.HealthCheckSession) map[string]*teamProgress {
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
	return progress
}

// eligibleTeamCompletion is the single calculation every Individual Survey
// Completed value, and every status/aggregate derived from it, must use:
// completed is how many of eligibleUserIDs also appear in completedUserIDs
// (the set built by buildTeamProgress); total is how many distinct users
// eligibleUserIDs names. eligibleUserIDs is deduplicated here defensively
// — a user counts at most once even if the caller's query ever returned
// the same id twice for one team — even though FindEligibleMemberIDs
// already applies its own DISTINCT. completedUserIDs may be nil (a team
// with zero completed sessions this period), in which case completed is
// always 0.
func eligibleTeamCompletion(eligibleUserIDs []string, completedUserIDs map[string]bool) (completed, total int) {
	seen := make(map[string]bool, len(eligibleUserIDs))
	for _, uid := range eligibleUserIDs {
		if seen[uid] {
			continue
		}
		seen[uid] = true
		total++
		if completedUserIDs[uid] {
			completed++
		}
	}
	return completed, total
}

// countDistinctEligible returns the number of distinct users named by ids
// — used for buildSurveyCompletionTrend's opted-in-members denominator,
// with the same defensive deduplication eligibleTeamCompletion applies.
func countDistinctEligible(ids []string) int {
	seen := make(map[string]bool, len(ids))
	count := 0
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		count++
	}
	return count
}

// Handle executes the query.
func (h *GetSurveyCompletionOverviewHandler) Handle(ctx context.Context, query GetSurveyCompletionOverviewQuery) (*SurveyCompletionOverview, error) {
	teams, err := h.teamRepo.FindSurveyCompletionTeams(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load teams: %w", err)
	}

	// The single authoritative eligible population for every status,
	// filter, and aggregate this handler produces -- see
	// team.Repository.FindEligibleMemberIDs.
	eligibleMemberIDs, err := h.teamRepo.FindEligibleMemberIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load eligible team members: %w", err)
	}

	sessions, err := h.healthCheckRepo.FindByAssessmentPeriod(ctx, query.AssessmentPeriod)
	if err != nil {
		return nil, fmt.Errorf("failed to load sessions: %w", err)
	}

	usersByID, positionByLevelID, nameByLevelID, err := h.loadDirectory(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load hierarchy directory: %w", err)
	}

	progress := buildTeamProgress(sessions)

	// Every leadership user needed for the tree: each team's own closest
	// supervisor, plus every one of that person's ancestors (via
	// reports_to), stopping the walk the moment resolveParentID returns ""
	// (a root) or an already-visited person is reached (shared ancestor —
	// no need to re-walk above them again).
	personsByID := make(map[string]*PersonNode)
	var personOrder []string

	addPersonAndAncestors := func(startUserID string) {
		userID := startUserID
		for userID != "" {
			if _, exists := personsByID[userID]; exists {
				return
			}
			u, ok := usersByID[userID]
			if !ok {
				return
			}
			parentID := resolveParentID(u, usersByID, positionByLevelID)
			personsByID[userID] = &PersonNode{
				UserID:    u.ID,
				Name:      u.Name,
				Email:     u.Email,
				LevelID:   u.HierarchyLevelID,
				LevelName: nameByLevelID[u.HierarchyLevelID],
				ParentID:  parentID,
			}
			personOrder = append(personOrder, userID)
			userID = parentID
		}
	}

	result := make([]SurveyCompletionTeam, 0, len(teams))
	for _, t := range teams {
		postWorkshop := false
		var completedUsers map[string]bool
		if p, ok := progress[t.ID]; ok {
			completedUsers = p.completedUsers
			postWorkshop = p.postWorkshop
		}
		completed, total := eligibleTeamCompletion(eligibleMemberIDs[t.ID], completedUsers)

		// The team's closest supervisor only becomes its owner when they
		// themselves are at or below leadershipFloorPosition — otherwise
		// (e.g. team_supervisors somehow names a VP directly) the team has
		// no resolvable owner and falls to "Other", consistent with the
		// same floor applied to every ancestor.
		ownerID := ""
		if t.SupervisorID != "" {
			if owner, ok := usersByID[t.SupervisorID]; ok && !isAboveLeadershipFloor(owner.HierarchyLevelID, positionByLevelID) {
				ownerID = owner.ID
				addPersonAndAncestors(ownerID)
			}
		}

		result = append(result, SurveyCompletionTeam{
			TeamID:                t.ID,
			TeamName:              t.Name,
			OwnerUserID:           ownerID,
			TeamLeadID:            t.TeamLeadID,
			TeamLeadName:          t.TeamLeadName,
			TeamLeadEmail:         t.TeamLeadEmail,
			Completed:             completed,
			Total:                 total,
			PostWorkshopCompleted: postWorkshop,
			HealthCheckEnabled:    t.HealthCheckEnabled,
		})
	}

	persons := make([]PersonNode, 0, len(personOrder))
	for _, id := range personOrder {
		persons = append(persons, *personsByID[id])
	}

	return &SurveyCompletionOverview{
		AssessmentPeriod: query.AssessmentPeriod,
		Teams:            result,
		Persons:          persons,
		TimeSeries:       buildSurveyCompletionTrend(teams, eligibleMemberIDs, sessions),
	}, nil
}

// loadDirectory returns every user keyed by id, every hierarchy level's
// position keyed by level id, and every hierarchy level's display name
// keyed by level id — everything resolveParentID and the PersonNode builder
// need, with no assumption baked in about how many levels exist or what
// they're named.
func (h *GetSurveyCompletionOverviewHandler) loadDirectory(ctx context.Context) (usersByID map[string]*user.User, positionByLevelID map[string]int, nameByLevelID map[string]string, err error) {
	levels, err := h.orgRepo.FindHierarchyLevels(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load hierarchy levels: %w", err)
	}
	positionByLevelID = make(map[string]int, len(levels))
	nameByLevelID = make(map[string]string, len(levels))
	for _, l := range levels {
		positionByLevelID[l.ID] = l.Position
		nameByLevelID[l.ID] = l.Name
	}

	users, err := h.userRepo.FindAll(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load users: %w", err)
	}
	usersByID = make(map[string]*user.User, len(users))
	for _, u := range users {
		usersByID[u.ID] = u
	}

	return usersByID, positionByLevelID, nameByLevelID, nil
}

// parseSessionDate parses a health-check session's stored date, tried first
// as a full RFC3339 timestamp and falling back to a bare date in case the
// source data ever stores it without a time component.
func parseSessionDate(raw string) (time.Time, error) {
	if d, err := time.Parse(time.RFC3339, raw); err == nil {
		return d, nil
	}
	return time.Parse("2006-01-02", raw)
}

// buildSurveyCompletionTrend computes cumulative individual-survey
// completion, as a percentage of opted-in ELIGIBLE members (Level 4 + Level
// 5 only, from eligibleMemberIDs — the same population every other status
// and aggregate uses), bucketed by calendar week of the session date. Weeks
// are numbered as days-since-epoch/7 rather than ISO week-of-year so a
// period spanning a year boundary still buckets and orders correctly.
func buildSurveyCompletionTrend(teams []team.SurveyCompletionRow, eligibleMemberIDs map[string][]string, sessions []*healthcheck.HealthCheckSession) []SurveyCompletionTrendPoint {
	optedInMembers := 0
	optedIn := make(map[string]bool)
	for _, t := range teams {
		if t.HealthCheckEnabled {
			optedInMembers += countDistinctEligible(eligibleMemberIDs[t.ID])
			optedIn[t.ID] = true
		}
	}
	if optedInMembers == 0 {
		return []SurveyCompletionTrendPoint{}
	}

	// A member can have more than one completed individual session for the
	// same period (e.g. a resubmission) — the trend must count their
	// completion on the EARLIEST of those dates, not whichever one this
	// unsorted `sessions` slice happens to return first. Pass 1 finds that
	// earliest date per (team, user); pass 2 (below) buckets by it.
	type completionKey struct{ team, user string }
	earliest := make(map[completionKey]time.Time)

	for _, s := range sessions {
		if !s.Completed || s.SurveyType == healthcheck.SurveyTypePostWorkshop || !optedIn[s.TeamID] {
			continue
		}
		d, err := parseSessionDate(s.Date)
		if err != nil {
			continue
		}
		k := completionKey{s.TeamID, s.UserID}
		if existing, ok := earliest[k]; !ok || d.Before(existing) {
			earliest[k] = d
		}
	}

	if len(earliest) == 0 {
		return []SurveyCompletionTrendPoint{}
	}

	weekCounts := make(map[int]int)
	minWeek, maxWeek := 0, 0
	haveWeek := false
	for _, d := range earliest {
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
