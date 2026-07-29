// SPDX-License-Identifier: Apache-2.0

package user

import (
	"context"
	"time"
)

// AuthType represents how a user authenticates
type AuthType string

const (
	AuthTypeLocal AuthType = "local"
	AuthTypeSSO   AuthType = "sso"
)

// User represents a user in the system
// This is an aggregate root in DDD terms
type User struct {
	ID               string    `json:"id"`
	Username         string    `json:"username"`
	Name             string    `json:"name"`
	Email            string    `json:"email,omitempty"`
	HierarchyLevelID string    `json:"hierarchyLevelId,omitempty"`
	ReportsTo        *string   `json:"reportsTo,omitempty"`
	TeamIDs          []string  `json:"teamIds"`
	IsAdmin          bool      `json:"isAdmin,omitempty"`
	PasswordHash     string    `json:"-"` // Never serialize to JSON
	AuthType         AuthType  `json:"authType,omitempty"`
	CreatedAt        time.Time `json:"createdAt,omitempty"`
	UpdatedAt        time.Time `json:"updatedAt,omitempty"`
}

// Repository defines the interface for user data access
type Repository interface {
	FindByID(ctx context.Context, id string) (*User, error)
	FindByUsername(ctx context.Context, username string) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	FindAll(ctx context.Context) ([]*User, error)
	FindByHierarchyLevel(ctx context.Context, levelID string) ([]*User, error)
	FindSubordinates(ctx context.Context, supervisorID string) ([]*User, error)
	FindSupervisorChainUp(ctx context.Context, userID string) ([]*User, error)
	Save(ctx context.Context, user *User) error
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, id string) error
	// Additional methods for team membership
	FindTeamIDsForUser(ctx context.Context, userID string) ([]string, error)
	FindTeamsWhereUserIsLead(ctx context.Context, userID string) ([]string, error)
	// Password management
	UpdatePassword(ctx context.Context, userID string, hashedPassword string) error
}
