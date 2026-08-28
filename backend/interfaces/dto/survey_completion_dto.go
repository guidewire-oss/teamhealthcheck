package dto

// SurveyCompletionPersonDTO identifies a director or manager in the admin
// survey-completion dashboard's hierarchy.
type SurveyCompletionPersonDTO struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

// SurveyCompletionTeamDTO is one team's row in the admin survey-completion
// dashboard.
type SurveyCompletionTeamDTO struct {
	TeamID                string `json:"teamId"`
	TeamName              string `json:"teamName"`
	Completed             int    `json:"completed"`
	Total                 int    `json:"total"`
	PostWorkshopCompleted *bool  `json:"postWorkshopCompleted"`
	Status                string `json:"status"` // complete | in_progress | not_started | opted_out
	// TeamLeadID/Name identify the pod's Level-4 team lead, set whenever the
	// team has one. Used both for the "Other" group's escalation target and
	// for labeling this pod's leaf in every reminder-target tree.
	TeamLeadID   *string `json:"teamLeadId,omitempty"`
	TeamLeadName *string `json:"teamLeadName,omitempty"`
}

// SurveyCompletionManagerGroupDTO is a Level-3 manager's row, nested under
// its director (or standing alone as a top-level group — see
// SurveyCompletionGroupDTO.Type).
type SurveyCompletionManagerGroupDTO struct {
	Manager           SurveyCompletionPersonDTO `json:"manager"`
	TotalTeams        int                       `json:"totalTeams"`
	OptedInTeams      int                       `json:"optedInTeams"`
	CompletionPercent int                       `json:"completionPercent"`
	RemindCount       int                       `json:"remindCount"`
	Teams             []SurveyCompletionTeamDTO `json:"teams"`
}

// SurveyCompletionGroupDTO is one top-level row in the admin
// survey-completion table: a director, a manager standing in for a missing
// director, or the catch-all "Other" group.
type SurveyCompletionGroupDTO struct {
	Type              string                     `json:"type"` // director | manager | other
	Director          *SurveyCompletionPersonDTO `json:"director,omitempty"`
	Manager           *SurveyCompletionPersonDTO `json:"manager,omitempty"` // type=manager only
	Label             string                     `json:"label,omitempty"`  // type=other only, e.g. "Other"
	TotalTeams        int                        `json:"totalTeams"`
	OptedInTeams      int                        `json:"optedInTeams"`
	CompletionPercent int                        `json:"completionPercent"`
	RemindCount       int                        `json:"remindCount"`
	// DirectTeams/Managers are for type=director only — always present as
	// [] rather than omitted, even when empty (e.g. a director with no
	// managers, or a director whose only teams are under managers), since
	// the frontend always spreads/maps over them without a null check.
	DirectTeams []SurveyCompletionTeamDTO         `json:"directTeams"`
	Managers    []SurveyCompletionManagerGroupDTO `json:"managers"`
	// Teams is for type=manager and type=other only — same "always an
	// array" contract as above.
	Teams []SurveyCompletionTeamDTO `json:"teams"`
}

// SurveyCompletionTrendPointDTO is one weekly bucket of cumulative
// completion for the trend chart.
type SurveyCompletionTrendPointDTO struct {
	Label      string `json:"label"`
	Completion int    `json:"completion"`
}

// SurveyCompletionResponse is the full response for
// GET /api/v1/admin/survey-completion.
type SurveyCompletionResponse struct {
	AssessmentPeriod  string                          `json:"assessmentPeriod"`
	OverallCompletion int                             `json:"overallCompletion"`
	TotalTeams        int                             `json:"totalTeams"`
	OptedIn           int                             `json:"optedIn"`
	FullyComplete     int                             `json:"fullyComplete"`
	InProgress        int                             `json:"inProgress"`
	NotStarted        int                             `json:"notStarted"`
	OptedOut          int                             `json:"optedOut"`
	Groups            []SurveyCompletionGroupDTO      `json:"groups"`
	TimeSeries        []SurveyCompletionTrendPointDTO `json:"timeSeries"`
}
