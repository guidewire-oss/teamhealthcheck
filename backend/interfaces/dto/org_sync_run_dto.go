package dto

import (
	"encoding/json"
	"time"
)

// SyncRunDTO is the persisted state of one organization sync run.
type SyncRunDTO struct {
	RunID         string     `json:"runId"`
	Status        string     `json:"status"` // PENDING | RUNNING | SUCCESS | FAILED
	Stage         string     `json:"stage"`
	StartedAt     *time.Time `json:"startedAt"`
	CompletedAt   *time.Time `json:"completedAt"`
	CreatedAt     time.Time  `json:"createdAt"`
	InsertedUsers int        `json:"insertedUsers"`
	UpdatedUsers  int        `json:"updatedUsers"`
	// UnchangedUsers/UnchangedTeams are null when the run did not record them.
	UnchangedUsers     *int `json:"unchangedUsers"`
	InsertedTeams      int  `json:"insertedTeams"`
	UpdatedTeams       int  `json:"updatedTeams"`
	UnchangedTeams     *int `json:"unchangedTeams"`
	MembershipChanges  int  `json:"membershipChanges"`
	DeactivatedRecords int  `json:"deactivatedRecords"`
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

// SyncRunHistoryItemDTO is the administrator's read-only view of one sync run.
// It is deliberately narrower than the persisted row: it omits the idempotency
// key, worker id, heartbeat and the raw mass-deletion hold report.
type SyncRunHistoryItemDTO struct {
	RunID        string `json:"runId"`
	SnapshotType string `json:"snapshotType"` // COMPLETE | PARTIAL
	Trigger      string `json:"trigger"`      // MANUAL | SCHEDULED
	Status       string `json:"status"`       // PENDING | RUNNING | SUCCESS | FAILED
	Stage        string `json:"stage"`
	// InitiatedBy is the id of the administrator who started the run, or
	// "scheduler" for an automatic run.
	InitiatedBy string `json:"initiatedBy"`
	// CreatedAt is when the run was accepted (queued), not when work began.
	CreatedAt time.Time `json:"createdAt"`
	// StartedAt is when a worker began processing; null while still queued, and
	// for a run that failed before it started.
	StartedAt *time.Time `json:"startedAt"`
	// CompletedAt is when the run reached SUCCESS or FAILED; null while active.
	CompletedAt *time.Time `json:"completedAt"`
	// SourceGeneratedAt is when the external provider generated the snapshot
	// the run applied; only recorded for a SUCCESS run.
	SourceGeneratedAt *time.Time `json:"sourceGeneratedAt"`
	// DurationSeconds is StartedAt to CompletedAt; null unless both are
	// recorded and the run did not end by stale-run recovery (whose completion
	// time is the sweep time, not when work stopped).
	DurationSeconds *int64 `json:"durationSeconds"`
	// FailedStage, ErrorCode and ErrorMessage are set only for FAILED runs.
	// ErrorMessage is sanitized and capped.
	FailedStage  *string `json:"failedStage"`
	ErrorCode    *string `json:"errorCode"`
	ErrorMessage *string `json:"errorMessage"`

	// The counts below are what this run committed, with the same meaning as the
	// SyncRunDTO shown in Settings after a sync. They are non-null only for a
	// SUCCESS run, because the counters are written in the same transaction that
	// marks the run SUCCESS: a PENDING, RUNNING or FAILED run committed nothing.
	// null means "not available"; 0 is a real zero. Unchanged is also null for
	// older runs that never recorded it.
	InsertedUsers     *int `json:"insertedUsers"`
	UpdatedUsers      *int `json:"updatedUsers"`
	UnchangedUsers    *int `json:"unchangedUsers"`
	InsertedTeams     *int `json:"insertedTeams"`
	UpdatedTeams      *int `json:"updatedTeams"`
	UnchangedTeams    *int `json:"unchangedTeams"`
	MembershipChanges *int `json:"membershipChanges"`
}

// SyncRunHistoryResponse is one page of run history, newest first.
type SyncRunHistoryResponse struct {
	Runs       []SyncRunHistoryItemDTO `json:"runs"`
	Pagination PaginationDTO           `json:"pagination"`
}
