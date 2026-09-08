package dto

// SurveyCompletionPersonDTO identifies one leadership user in the admin
// survey-completion dashboard's hierarchy tree.
type SurveyCompletionPersonDTO struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Email   string `json:"email,omitempty"`
	LevelID string `json:"levelId"` // hierarchy_level_id, schema-driven
	Level   string `json:"level"`   // hierarchy_levels.name — the display/role label, never invented
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

// SurveyCompletionGroupDTO is one row in the admin survey-completion
// table's recursive hierarchy: either a leadership person (with their own
// direct pods and, recursively, their own children), or the catch-all
// "Other" group for pods with no resolvable leadership owner. There is no
// separate type per hierarchy tier — the same shape nests to whatever depth
// the organization's own reports_to chains actually go.
type SurveyCompletionGroupDTO struct {
	Type              string                     `json:"type"`             // person | other
	Person            *SurveyCompletionPersonDTO `json:"person,omitempty"` // type=person only
	Label             string                     `json:"label,omitempty"`  // type=other only, e.g. "Other"
	TotalTeams        int                        `json:"totalTeams"`
	OptedInTeams      int                        `json:"optedInTeams"`
	CompletionPercent int                        `json:"completionPercent"`
	RemindCount       int                        `json:"remindCount"`
	// DirectTeams/Children are for type=person only — always present as []
	// rather than omitted, even when empty, since the frontend always
	// spreads/maps over them without a null check.
	DirectTeams []SurveyCompletionTeamDTO  `json:"directTeams"`
	Children    []SurveyCompletionGroupDTO `json:"children"`
	// Teams is for type=other only — same "always an array" contract.
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
