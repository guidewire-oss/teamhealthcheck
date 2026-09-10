package v1

import (
	"net/http"
	"slices"
	"sort"

	"github.com/gin-gonic/gin"

	"github.com/agopalakrishnan/teams360/backend/application/queries"
	"github.com/agopalakrishnan/teams360/backend/domain/healthcheck"
	"github.com/agopalakrishnan/teams360/backend/domain/organization"
	"github.com/agopalakrishnan/teams360/backend/domain/team"
	"github.com/agopalakrishnan/teams360/backend/domain/user"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
)

// SurveyCompletionAdminHandler serves the org-wide survey completion
// dashboard (all teams, grouped by a recursive leadership hierarchy).
type SurveyCompletionAdminHandler struct {
	overviewHandler *queries.GetSurveyCompletionOverviewHandler
	healthCheckRepo healthcheck.Repository
}

// NewSurveyCompletionAdminHandler creates a new handler.
func NewSurveyCompletionAdminHandler(teamRepo team.Repository, healthCheckRepo healthcheck.Repository, userRepo user.Repository, orgRepo organization.Repository) *SurveyCompletionAdminHandler {
	return &SurveyCompletionAdminHandler{
		overviewHandler: queries.NewGetSurveyCompletionOverviewHandler(teamRepo, healthCheckRepo, userRepo, orgRepo),
		healthCheckRepo: healthCheckRepo,
	}
}

// GetSurveyCompletion handles GET /api/v1/admin/survey-completion.
// assessmentPeriod is optional; when omitted, the most recent period with
// any submitted data is used (FindDistinctAssessmentPeriods already returns
// periods most-recent-first — see its own doc comment). When a caller does
// supply assessmentPeriod, it must be one of those known periods; anything
// else is rejected with 400 rather than silently querying an empty/nonexistent
// period and returning a zeroed-out overview that looks like "no data yet"
// instead of "you asked for a period that doesn't exist".
func (h *SurveyCompletionAdminHandler) GetSurveyCompletion(c *gin.Context) {
	ctx := c.Request.Context()
	assessmentPeriod := c.Query("assessmentPeriod")

	periods, err := h.healthCheckRepo.FindDistinctAssessmentPeriods(ctx)
	if err != nil {
		dto.RespondErrorWithDetails(c, http.StatusInternalServerError, "Failed to resolve assessment periods", err.Error())
		return
	}

	if assessmentPeriod == "" {
		if len(periods) > 0 {
			assessmentPeriod = periods[0]
		}
	} else if !slices.Contains(periods, assessmentPeriod) {
		dto.RespondErrorWithDetails(c, http.StatusBadRequest, "Unknown assessment period", assessmentPeriod)
		return
	}

	overview, err := h.overviewHandler.Handle(ctx, queries.GetSurveyCompletionOverviewQuery{AssessmentPeriod: assessmentPeriod})
	if err != nil {
		dto.RespondErrorWithDetails(c, http.StatusInternalServerError, "Failed to build survey completion overview", err.Error())
		return
	}

	dto.RespondSuccess(c, http.StatusOK, buildSurveyCompletionResponse(overview))
}

// nodeBuilder accumulates one PersonNode's direct teams and child nodes
// while walking overview.Teams/overview.Persons; converted to a
// SurveyCompletionGroupDTO (recursively, via toDTO) once every team and
// every parent/child link has been resolved.
type nodeBuilder struct {
	person      queries.PersonNode
	directTeams []dto.SurveyCompletionTeamDTO
	children    []*nodeBuilder
}

// toDTO recursively converts this node (and its whole subtree) into a
// SurveyCompletionGroupDTO, sorting children alphabetically by name at
// every level. It also returns every team in this node's subtree (its own
// direct teams plus every descendant's), which the caller folds into its
// own aggregate stats — this is how totals/remind-counts roll all the way
// up to the root regardless of how many levels deep the subtree goes.
func (n *nodeBuilder) toDTO() (dto.SurveyCompletionGroupDTO, []dto.SurveyCompletionTeamDTO) {
	sort.Slice(n.children, func(i, j int) bool {
		return n.children[i].person.Name < n.children[j].person.Name
	})

	subtreeTeams := append([]dto.SurveyCompletionTeamDTO(nil), n.directTeams...)
	childDTOs := make([]dto.SurveyCompletionGroupDTO, 0, len(n.children))
	for _, c := range n.children {
		childDTO, childTeams := c.toDTO()
		childDTOs = append(childDTOs, childDTO)
		subtreeTeams = append(subtreeTeams, childTeams...)
	}

	total, opted, complete, remind := aggregateTeams(subtreeTeams)
	person := dto.SurveyCompletionPersonDTO{
		ID:      n.person.UserID,
		Name:    n.person.Name,
		Email:   n.person.Email,
		LevelID: n.person.LevelID,
		Level:   n.person.LevelName,
	}
	directTeams := n.directTeams
	if directTeams == nil {
		// A leader whose teams are all under their own children has no
		// direct teams of their own — serialize [] rather than the nil
		// zero value, which encoding/json would otherwise send as `null`.
		directTeams = []dto.SurveyCompletionTeamDTO{}
	}

	return dto.SurveyCompletionGroupDTO{
		Type:              "person",
		Person:            &person,
		TotalTeams:        total,
		OptedInTeams:      opted,
		CompletionPercent: percent(complete, opted),
		RemindCount:       remind,
		DirectTeams:       directTeams,
		Children:          childDTOs,
	}, subtreeTeams
}

func buildSurveyCompletionResponse(overview *queries.SurveyCompletionOverview) dto.SurveyCompletionResponse {
	nodeBuilders := make(map[string]*nodeBuilder, len(overview.Persons))
	for _, p := range overview.Persons {
		person := p
		nodeBuilders[p.UserID] = &nodeBuilder{person: person}
	}

	var rootOrder []string
	for _, p := range overview.Persons {
		if p.ParentID != "" {
			if parent, ok := nodeBuilders[p.ParentID]; ok {
				parent.children = append(parent.children, nodeBuilders[p.UserID])
				continue
			}
		}
		rootOrder = append(rootOrder, p.UserID)
	}

	var otherTeams []dto.SurveyCompletionTeamDTO
	var totalTeams, optedIn, fullyComplete, inProgress, notStarted, optedOut int

	// Every team is keyed and deduplicated by its canonical database id
	// (TeamID) here, never by name -- two different teams that happen to
	// share a display name (e.g. an imperfect sync producing "Aurora"
	// twice under two different ids) are two distinct entries and both
	// render, separately, each under its own correct owner. This guards
	// against a team id ever being assigned to more than one display
	// bucket (a leader's directTeams, or Other): the response is built
	// completely first, and seenTeamIDs is the validation that each id
	// lands in exactly one place -- see
	// TestBuildSurveyCompletionResponseNoDuplicateTeamIDs.
	seenTeamIDs := make(map[string]bool, len(overview.Teams))

	for _, t := range overview.Teams {
		if seenTeamIDs[t.TeamID] {
			continue
		}
		seenTeamIDs[t.TeamID] = true

		totalTeams++
		status := deriveSurveyCompletionStatus(t)

		switch status {
		case "opted_out":
			optedOut++
		case "complete":
			optedIn++
			fullyComplete++
		case "in_progress":
			optedIn++
			inProgress++
		default: // not_started
			optedIn++
			notStarted++
		}

		var postWorkshop *bool
		if status != "opted_out" {
			pw := t.PostWorkshopCompleted
			postWorkshop = &pw
		}

		teamDTO := dto.SurveyCompletionTeamDTO{
			TeamID:                t.TeamID,
			TeamName:              t.TeamName,
			Completed:             t.Completed,
			Total:                 t.Total,
			PostWorkshopCompleted: postWorkshop,
			Status:                status,
		}
		// TeamLeadID/Name is always populated (not just for "Other") — the
		// reminder-target tree needs every pod's team lead name to render
		// its leaf label, and the team lead is always a reminder target
		// alongside whichever leader owns the pod.
		if t.TeamLeadID != "" {
			id, name := t.TeamLeadID, t.TeamLeadName
			teamDTO.TeamLeadID = &id
			teamDTO.TeamLeadName = &name
		}

		if node, ok := nodeBuilders[t.OwnerUserID]; t.OwnerUserID != "" && ok {
			node.directTeams = append(node.directTeams, teamDTO)
		} else {
			otherTeams = append(otherTeams, teamDTO)
		}
	}

	groups := make([]dto.SurveyCompletionGroupDTO, 0, len(rootOrder)+1)
	for _, id := range rootOrder {
		groupDTO, _ := nodeBuilders[id].toDTO()
		groups = append(groups, groupDTO)
	}
	if len(otherTeams) > 0 {
		total, opted, complete, remind := aggregateTeams(otherTeams)
		groups = append(groups, dto.SurveyCompletionGroupDTO{
			Type:              "other",
			Label:             "Other",
			TotalTeams:        total,
			OptedInTeams:      opted,
			CompletionPercent: percent(complete, opted),
			RemindCount:       remind,
			Teams:             otherTeams,
		})
	}

	timeSeries := make([]dto.SurveyCompletionTrendPointDTO, len(overview.TimeSeries))
	for i, p := range overview.TimeSeries {
		timeSeries[i] = dto.SurveyCompletionTrendPointDTO{Label: p.Label, Completion: p.Completion}
	}

	return dto.SurveyCompletionResponse{
		AssessmentPeriod:  overview.AssessmentPeriod,
		OverallCompletion: percent(fullyComplete, optedIn),
		TotalTeams:        totalTeams,
		OptedIn:           optedIn,
		FullyComplete:     fullyComplete,
		InProgress:        inProgress,
		NotStarted:        notStarted,
		OptedOut:          optedOut,
		Groups:            groups,
		TimeSeries:        timeSeries,
	}
}

// aggregateTeams computes the same rollup numbers the dashboard's group
// header rows have always shown: total teams, opted-in teams, fully-complete
// teams, and how many are remindable (in progress or not started).
func aggregateTeams(teams []dto.SurveyCompletionTeamDTO) (total, optedIn, complete, remind int) {
	total = len(teams)
	for _, t := range teams {
		if t.Status == "opted_out" {
			continue
		}
		optedIn++
		switch t.Status {
		case "complete":
			complete++
		case "in_progress", "not_started":
			remind++
		}
	}
	return total, optedIn, complete, remind
}

// deriveSurveyCompletionStatus classifies a team from its counts, its
// post-workshop session, and its health_check_enabled flag.
//
// "complete" requires BOTH every eligible member's individual survey done
// (Completed >= Total) AND the team's post-workshop session completed --
// per the original survey-completion-dashboard spec (issue #143), the
// individual surveys alone only capture each member's private input; the
// post-workshop session is where the team reviews those results together,
// and a team isn't done until that has happened too. A team whose members
// have all finished their individual surveys but hasn't yet held its
// post-workshop session is "in_progress", never "complete" -- exactly the
// same bucket a team with only some members done falls into, since from a
// completion standpoint neither team is finished.
func deriveSurveyCompletionStatus(t queries.SurveyCompletionTeam) string {
	if !t.HealthCheckEnabled {
		return "opted_out"
	}
	if t.Total == 0 || t.Completed == 0 {
		return "not_started"
	}
	if t.Completed >= t.Total && t.PostWorkshopCompleted {
		return "complete"
	}
	return "in_progress"
}

func percent(part, total int) int {
	if total == 0 {
		return 0
	}
	return int((float64(part)/float64(total))*100 + 0.5)
}
