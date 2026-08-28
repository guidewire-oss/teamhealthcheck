package v1

import (
	"net/http"
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
// dashboard (all teams, grouped by director -> manager -> team).
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
// any submitted data is used.
func (h *SurveyCompletionAdminHandler) GetSurveyCompletion(c *gin.Context) {
	ctx := c.Request.Context()
	assessmentPeriod := c.Query("assessmentPeriod")

	if assessmentPeriod == "" {
		periods, err := h.healthCheckRepo.FindDistinctAssessmentPeriods(ctx)
		if err != nil {
			dto.RespondErrorWithDetails(c, http.StatusInternalServerError, "Failed to resolve default assessment period", err.Error())
			return
		}
		if len(periods) > 0 {
			assessmentPeriod = periods[0]
		}
	}

	overview, err := h.overviewHandler.Handle(ctx, queries.GetSurveyCompletionOverviewQuery{AssessmentPeriod: assessmentPeriod})
	if err != nil {
		dto.RespondErrorWithDetails(c, http.StatusInternalServerError, "Failed to build survey completion overview", err.Error())
		return
	}

	dto.RespondSuccess(c, http.StatusOK, buildSurveyCompletionResponse(overview))
}

// directorBucket accumulates one director's direct teams and its nested
// manager groups while walking overview.Teams; converted to a
// SurveyCompletionGroupDTO once every team has been classified.
type directorBucket struct {
	director     dto.SurveyCompletionPersonDTO
	directTeams  []dto.SurveyCompletionTeamDTO
	managerOrder []string
	managers     map[string]*dto.SurveyCompletionManagerGroupDTO
}

// managerBucket accumulates one top-level manager's teams (a manager with no
// resolvable director).
type managerBucket struct {
	manager dto.SurveyCompletionPersonDTO
	teams   []dto.SurveyCompletionTeamDTO
}

func buildSurveyCompletionResponse(overview *queries.SurveyCompletionOverview) dto.SurveyCompletionResponse {
	directorOrder := make([]string, 0)
	directors := make(map[string]*directorBucket)
	topManagerOrder := make([]string, 0)
	topManagers := make(map[string]*managerBucket)
	var otherTeams []dto.SurveyCompletionTeamDTO

	var totalTeams, optedIn, fullyComplete, inProgress, notStarted, optedOut int

	for _, t := range overview.Teams {
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
		// reminder-target tree needs every pod's team lead name to render its
		// leaf label, and the team lead is always a reminder target alongside
		// whichever director/manager owns the pod.
		if t.TeamLeadID != "" {
			id, name := t.TeamLeadID, t.TeamLeadName
			teamDTO.TeamLeadID = &id
			teamDTO.TeamLeadName = &name
		}

		switch {
		case t.ManagerID == "" && t.DirectorID != "":
			dg := ensureDirectorBucket(directors, &directorOrder, t.DirectorID, t.DirectorName, t.DirectorEmail)
			dg.directTeams = append(dg.directTeams, teamDTO)

		case t.ManagerID != "" && t.DirectorID != "":
			dg := ensureDirectorBucket(directors, &directorOrder, t.DirectorID, t.DirectorName, t.DirectorEmail)
			mg := ensureManagerGroup(dg, t.ManagerID, t.ManagerName, t.ManagerEmail)
			mg.Teams = append(mg.Teams, teamDTO)

		case t.ManagerID != "" && t.DirectorID == "":
			mb := ensureManagerBucket(topManagers, &topManagerOrder, t.ManagerID, t.ManagerName, t.ManagerEmail)
			mb.teams = append(mb.teams, teamDTO)

		default: // no director, no manager -> Other
			otherTeams = append(otherTeams, teamDTO)
		}
	}

	groups := make([]dto.SurveyCompletionGroupDTO, 0, len(directorOrder)+len(topManagerOrder)+1)
	for _, id := range directorOrder {
		groups = append(groups, directorBucketToDTO(directors[id]))
	}
	for _, id := range topManagerOrder {
		groups = append(groups, managerBucketToDTO(topManagers[id]))
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

func ensureDirectorBucket(directors map[string]*directorBucket, order *[]string, id, name, email string) *directorBucket {
	dg, ok := directors[id]
	if !ok {
		dg = &directorBucket{
			director: dto.SurveyCompletionPersonDTO{ID: id, Name: name, Email: email},
			managers: make(map[string]*dto.SurveyCompletionManagerGroupDTO),
		}
		directors[id] = dg
		*order = append(*order, id)
	}
	return dg
}

func ensureManagerGroup(dg *directorBucket, id, name, email string) *dto.SurveyCompletionManagerGroupDTO {
	mg, ok := dg.managers[id]
	if !ok {
		mg = &dto.SurveyCompletionManagerGroupDTO{Manager: dto.SurveyCompletionPersonDTO{ID: id, Name: name, Email: email}}
		dg.managers[id] = mg
		dg.managerOrder = append(dg.managerOrder, id)
	}
	return mg
}

func ensureManagerBucket(topManagers map[string]*managerBucket, order *[]string, id, name, email string) *managerBucket {
	mb, ok := topManagers[id]
	if !ok {
		mb = &managerBucket{manager: dto.SurveyCompletionPersonDTO{ID: id, Name: name, Email: email}}
		topManagers[id] = mb
		*order = append(*order, id)
	}
	return mb
}

func directorBucketToDTO(dg *directorBucket) dto.SurveyCompletionGroupDTO {
	managerNames := append([]string(nil), dg.managerOrder...)
	sort.Slice(managerNames, func(i, j int) bool {
		return dg.managers[managerNames[i]].Manager.Name < dg.managers[managerNames[j]].Manager.Name
	})

	allTeams := append([]dto.SurveyCompletionTeamDTO(nil), dg.directTeams...)
	managerGroups := make([]dto.SurveyCompletionManagerGroupDTO, 0, len(managerNames))
	for _, id := range managerNames {
		mg := dg.managers[id]
		total, opted, complete, remind := aggregateTeams(mg.Teams)
		mg.TotalTeams = total
		mg.OptedInTeams = opted
		mg.CompletionPercent = percent(complete, opted)
		mg.RemindCount = remind
		managerGroups = append(managerGroups, *mg)
		allTeams = append(allTeams, mg.Teams...)
	}

	total, opted, complete, remind := aggregateTeams(allTeams)
	director := dg.director
	directTeams := dg.directTeams
	if directTeams == nil {
		// A director whose teams are all under managers has no direct
		// teams of its own — serialize [] rather than the nil zero value,
		// which encoding/json would otherwise send as `null`.
		directTeams = []dto.SurveyCompletionTeamDTO{}
	}
	return dto.SurveyCompletionGroupDTO{
		Type:              "director",
		Director:          &director,
		TotalTeams:        total,
		OptedInTeams:      opted,
		CompletionPercent: percent(complete, opted),
		RemindCount:       remind,
		DirectTeams:       directTeams,
		Managers:          managerGroups,
	}
}

func managerBucketToDTO(mb *managerBucket) dto.SurveyCompletionGroupDTO {
	total, opted, complete, remind := aggregateTeams(mb.teams)
	manager := mb.manager
	return dto.SurveyCompletionGroupDTO{
		Type:              "manager",
		Manager:           &manager,
		TotalTeams:        total,
		OptedInTeams:      opted,
		CompletionPercent: percent(complete, opted),
		RemindCount:       remind,
		Teams:             mb.teams,
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

// deriveSurveyCompletionStatus classifies a team purely from its counts and
// its health_check_enabled flag.
func deriveSurveyCompletionStatus(t queries.SurveyCompletionTeam) string {
	if !t.HealthCheckEnabled {
		return "opted_out"
	}
	if t.Total == 0 || t.Completed == 0 {
		return "not_started"
	}
	if t.Completed >= t.Total {
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
