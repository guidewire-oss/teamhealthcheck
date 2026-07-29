// SPDX-License-Identifier: Apache-2.0

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
}
