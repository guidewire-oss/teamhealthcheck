package dto

// TeamHealthSummary represents aggregated health data for a team
type TeamHealthSummary struct {
	TeamID             string             `json:"teamId"`
	TeamName           string             `json:"teamName"`
	OverallHealth      float64            `json:"overallHealth"`
	SubmissionCount    int                `json:"submissionCount"`
	Dimensions         []DimensionSummary `json:"dimensions"`
	PostWorkshopStatus string             `json:"postWorkshopStatus,omitempty"`
}

// DimensionSummary represents aggregated health for a single dimension
type DimensionSummary struct {
	DimensionID   string  `json:"dimensionId"`
	AvgScore      float64 `json:"avgScore"`
	ResponseCount int     `json:"responseCount"`
	// Trend is the trend value ("improving", "stable", "declining") from the
	// final completed post-workshop response for this team+dimension, if any.
	Trend string `json:"trend,omitempty"`
}

// ManagerTeamsHealthResponse represents the response for manager's teams health
type ManagerTeamsHealthResponse struct {
	ManagerID        string              `json:"managerId"`
	Teams            []TeamHealthSummary `json:"teams"`
	TotalTeams       int                 `json:"totalTeams"`
	AssessmentPeriod string              `json:"assessmentPeriod,omitempty"`
}

// ManagerRadarResponse represents aggregated radar chart data for manager
type ManagerRadarResponse struct {
	ManagerID        string             `json:"managerId"`
	Dimensions       []DimensionSummary `json:"dimensions"`
	AssessmentPeriod string             `json:"assessmentPeriod,omitempty"`
}

// ManagerTrendsResponse represents trend data for manager's teams
type ManagerTrendsResponse struct {
	ManagerID  string                  `json:"managerId"`
	Periods    []string                `json:"periods"`
	Dimensions []ManagerDimensionTrend `json:"dimensions"`
}

// ManagerDimensionTrend represents trend scores for a dimension across periods
type ManagerDimensionTrend struct {
	DimensionID string    `json:"dimensionId"`
	Scores      []float64 `json:"scores"` // matches periods array order
}

// SubordinateDTO represents a user in the subordinate tree
type SubordinateDTO struct {
	ID               string   `json:"id"`
	Username         string   `json:"username"`
	Name             string   `json:"name"`
	HierarchyLevelID string   `json:"hierarchyLevelId"`
	ReportsTo        string   `json:"reportsTo,omitempty"`
	TeamIDs          []string `json:"teamIds"`
}

// SubordinatesResponse represents the response for a manager's subordinate tree
type SubordinatesResponse struct {
	ManagerID    string           `json:"managerId"`
	Subordinates []SubordinateDTO `json:"subordinates"`
}

// PostWorkshopComment represents a single free-text comment from a final post-workshop survey
type PostWorkshopComment struct {
	TeamID      string `json:"teamId"`
	SessionID   string `json:"sessionId"`
	DimensionID string `json:"dimensionId"`
	Comment     string `json:"comment"`
	Date        string `json:"date"`
}

// ManagerFinalPostWorkshopCommentsResponse represents final post-workshop comments
// for all teams supervised by a manager, grouped by team
type ManagerFinalPostWorkshopCommentsResponse struct {
	ManagerID        string                           `json:"managerId"`
	Comments         map[string][]PostWorkshopComment `json:"comments"`
	AssessmentPeriod string                           `json:"assessmentPeriod,omitempty"`
}

// ManagerMemberOverviewResponse represents aggregated health data based only on
// individual team-member survey submissions (no post-workshop data), optionally
// scoped to a single team
type ManagerMemberOverviewResponse struct {
	ManagerID        string              `json:"managerId"`
	TeamID           string              `json:"teamId,omitempty"`
	Teams            []TeamHealthSummary `json:"teams"`
	AssessmentPeriod string              `json:"assessmentPeriod,omitempty"`
}
