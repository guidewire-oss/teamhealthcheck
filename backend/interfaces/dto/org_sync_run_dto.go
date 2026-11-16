package dto

import (
	"encoding/json"
	"time"
)

// SyncRunDTO is the persisted state of one organization sync run.
type SyncRunDTO struct {
	RunID              string     `json:"runId"`
	Status             string     `json:"status"` // PENDING | RUNNING | SUCCESS | FAILED
	Stage              string     `json:"stage"`
	StartedAt          *time.Time `json:"startedAt"`
	CompletedAt        *time.Time `json:"completedAt"`
	CreatedAt          time.Time  `json:"createdAt"`
	InsertedUsers      int        `json:"insertedUsers"`
	UpdatedUsers       int        `json:"updatedUsers"`
	UnchangedUsers     int        `json:"unchangedUsers"`
	InsertedTeams      int        `json:"insertedTeams"`
	UpdatedTeams       int        `json:"updatedTeams"`
	UnchangedTeams     int        `json:"unchangedTeams"`
	MembershipChanges  int        `json:"membershipChanges"`
	DeactivatedRecords int        `json:"deactivatedRecords"`
	// HealthChecksDisabled and HealthChecksEnabled count teams whose health
	// checks the sync switched off or on.
	HealthChecksDisabled int `json:"healthChecksDisabled"`
	HealthChecksEnabled  int `json:"healthChecksEnabled"`
	// MassDeletionOverridden is true when an administrator waived the
	// mass-deletion hold for this run.
	MassDeletionOverridden bool            `json:"massDeletionOverridden"`
	ErrorCode              *string         `json:"errorCode"`
	ErrorMessage           *string         `json:"errorMessage"`
	FailedStage            *string         `json:"failedStage"`
	MassDeletion           json.RawMessage `json:"massDeletion,omitempty"`
	// AlreadyActive is true when a start request was answered with a run that
	// was already in flight rather than a new one.
	AlreadyActive bool `json:"alreadyActive,omitempty"`
}

// AutoSyncAttemptDTO is one automatic run, as shown in Settings.
type AutoSyncAttemptDTO struct {
	RunID        string     `json:"runId"`
	Status       string     `json:"status"` // PENDING | RUNNING | SUCCESS | FAILED
	StartedAt    time.Time  `json:"startedAt"`
	CompletedAt  *time.Time `json:"completedAt"`
	ErrorMessage *string    `json:"errorMessage"`
}

// AutoSyncScheduleDTO is the saved automatic-sync schedule and its history.
// All timestamps are UTC (RFC 3339); Timezone names the schedule's zone.
type AutoSyncScheduleDTO struct {
	Enabled       bool                `json:"enabled"`
	Frequency     string              `json:"frequency"` // DAILY | WEEKLY | MONTHLY
	Description   string              `json:"description"`
	Timezone      string              `json:"timezone"`
	NextRunAt     *time.Time          `json:"nextRunAt"`
	LastAttempt   *AutoSyncAttemptDTO `json:"lastAttempt"`
	LastSuccessAt *time.Time          `json:"lastSuccessAt"`
}

// UpdateAutoSyncScheduleRequest is the body of PUT .../auto-sync.
// Frequency may be omitted when only disabling.
type UpdateAutoSyncScheduleRequest struct {
	Enabled   *bool  `json:"enabled" binding:"required"`
	Frequency string `json:"frequency"`
}
