// Package orgsnapshot defines the provider-neutral organization snapshot contract.
package orgsnapshot

import "time"

// ContractVersion is the legacy snapshot contract version.
const ContractVersion = "1.0"

// ContractVersionV11 adds the optional user field podiqRole. Both versions are
// accepted during rollout; see SupportedContractVersion.
const ContractVersionV11 = "1.1"

// SupportedContractVersion reports whether v is a contract version THC accepts.
func SupportedContractVersion(v string) bool {
	return v == ContractVersion || v == ContractVersionV11
}

// PodIQ roles a v1.1 user may carry in podiqRole. PodIQ has already resolved
// multi-role precedence, so THC only maps the value it receives.
const (
	PodIQRoleL2        = "L2"
	PodIQRoleL1        = "L1"
	PodIQRoleDeveloper = "Developer"
)

// ValidPodIQRole reports whether role is one of the accepted podiqRole values.
func ValidPodIQRole(role string) bool {
	switch role {
	case PodIQRoleL2, PodIQRoleL1, PodIQRoleDeveloper:
		return true
	}
	return false
}

// Snapshot types.
const (
	TypeComplete = "COMPLETE"
	TypePartial  = "PARTIAL"
)

// User represents a person from an external organization data source.
type User struct {
	ID               string  `json:"id"`
	Username         string  `json:"username"`
	DisplayName      string  `json:"displayName"`
	Email            string  `json:"email"`
	HierarchyLevelID string  `json:"hierarchyLevelId"`
	ReportsToID      *string `json:"reportsToId,omitempty"` // Nil identifies a root user with no manager.
	// PodIQRole is the optional contract v1.1 role: "L2", "L1" or "Developer".
	// Nil (omitted or null) means the user keeps legacy level handling.
	PodIQRole *string `json:"podiqRole,omitempty"`
}

// Team represents a team from an external organization data source.
type Team struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	TeamLeadID         *string `json:"teamLeadId,omitempty"`         // Omitted preserves the existing THC-managed value.
	HealthCheckEnabled *bool   `json:"healthCheckEnabled,omitempty"` // tri-state: true=enabled, false=disabled, nil=not supplied.
}

// Membership associates a user with a team.
type Membership struct {
	UserID string `json:"userId"`
	TeamID string `json:"teamId"`
}

// Snapshot is a complete provider organization snapshot.
type Snapshot struct {
	ContractVersion string    `json:"contractVersion"`
	GeneratedAt     time.Time `json:"generatedAt"`
	// SnapshotType is "COMPLETE" or "PARTIAL". Omitted means COMPLETE, which is
	// what contract v1.0 documents ("the snapshot is complete for its provider").
	// Only COMPLETE lets a sync deactivate records absent from the payload.
	SnapshotType string       `json:"snapshotType,omitempty"`
	Teams        []Team       `json:"teams"`
	Users        []User       `json:"users"`
	Memberships  []Membership `json:"memberships"`
}
