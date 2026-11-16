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

// Reminder scopes accepted by POST /api/v1/admin/survey-completion/reminders.
// They match the three Remind buttons on the dashboard.
const (
	SurveyReminderScopeTeam   = "team"   // one pod: its owner and its team lead
	SurveyReminderScopePerson = "person" // one leader: their whole lagging subtree
	SurveyReminderScopeOrg    = "org"    // every lagging pod in the organization
)

// SurveyReminderRequest names who to remind. The client sends only a target;
// the server decides the recipients from the survey-completion data, so a
// caller can never address an arbitrary email.
type SurveyReminderRequest struct {
	Scope            string `json:"scope"`
	TeamID           string `json:"teamId,omitempty"`   // scope=team
	PersonID         string `json:"personId,omitempty"` // scope=person
	AssessmentPeriod string `json:"assessmentPeriod,omitempty"`
}

// SurveyReminderResponse reports what a reminder run did.
type SurveyReminderResponse struct {
	AssessmentPeriod string `json:"assessmentPeriod"`
	// Mode is "send" for normal delivery, or "log" when email delivery is
	// switched off (EMAIL_MODE=log): nothing was sent, and each reminder was
	// written to the server log instead.
	Mode string `json:"mode"`
	// RedirectedTo lists the test addresses every email was delivered to
	// instead of its real recipient (EMAIL_REDIRECT_TO). Empty means no
	// redirect is in effect; use Mode to tell whether anything was delivered
	// (nothing is when Mode is "log").
	RedirectedTo []string `json:"redirectedTo"`
	// TestPod is set when EMAIL_TEST_POD limited the recipients to one pod.
	TestPod string   `json:"testPod,omitempty"`
	Sent    int      `json:"sent"`
	SentTo  []string `json:"sentTo"`
	// Logged and LoggedFor are the reminders written to the log in log mode.
	Logged    int      `json:"logged"`
	LoggedFor []string `json:"loggedFor"`
	// SkippedNoEmail are recipients with no email address on file.
	SkippedNoEmail []string `json:"skippedNoEmail"`
	// Failed are recipients the mail transport rejected.
	Failed []string `json:"failed"`
	// PodsWithoutTeamLead are lagging pods with no team lead to remind.
	PodsWithoutTeamLead []string `json:"podsWithoutTeamLead"`
}
