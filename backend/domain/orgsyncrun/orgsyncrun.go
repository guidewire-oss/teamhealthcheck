// Package orgsyncrun models one persisted organization-sync attempt: its
// status, current stage, counters and failure details. Status lives in the
// database (not process memory) so the admin UI can recover it after a
// refresh, and so a crashed worker's run can be recovered by another replica.
package orgsyncrun

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
)

// Status is the lifecycle state of a run.
type Status string

const (
	StatusPending Status = "PENDING" // accepted; waiting to start or for the sync lock
	StatusRunning Status = "RUNNING" // lock held, actively processing
	StatusSuccess Status = "SUCCESS" // the apply transaction committed
	StatusFailed  Status = "FAILED"
)

// Terminal reports whether no further transition is allowed.
func (s Status) Terminal() bool { return s == StatusSuccess || s == StatusFailed }

// CanTransition encodes the only legal moves:
//
//	PENDING -> RUNNING -> SUCCESS
//	PENDING -> RUNNING -> FAILED
//	PENDING -> FAILED
//	RUNNING -> FAILED
//
// Terminal states have no outgoing edge, which is what stops a stale worker
// from overwriting a result another worker (or the recovery sweep) recorded.
func CanTransition(from, to Status) bool {
	switch from {
	case StatusPending:
		return to == StatusRunning || to == StatusFailed
	case StatusRunning:
		return to == StatusSuccess || to == StatusFailed
	}
	return false
}

// Stage is the step a run is on, reported to the UI and recorded on failure.
type Stage string

const (
	StageQueued              Stage = "QUEUED"
	StageAcquiringLock       Stage = "ACQUIRING_LOCK"
	StageFetching            Stage = "FETCHING"
	StageValidating          Stage = "VALIDATING"
	StageUpsertingUsers      Stage = "UPSERTING_USERS"
	StageUpsertingTeams      Stage = "UPSERTING_TEAMS"
	StageSyncingMemberships  Stage = "SYNCING_MEMBERSHIPS"
	StageReconcilingRemovals Stage = "RECONCILING_REMOVALS"
	StageCommitting          Stage = "COMMITTING"
	StageCompleted           Stage = "COMPLETED"
	StageFailed              Stage = "FAILED"
)

// Error codes stored with a failed run. They are stable identifiers; the
// message beside them is human text and never contains a credential.
const (
	CodeProviderNotConfigured = "PROVIDER_NOT_CONFIGURED"
	CodeLockUnavailable       = "LOCK_UNAVAILABLE"
	CodeProviderFetchFailed   = "PROVIDER_FETCH_FAILED"
	CodeInvalidSnapshot       = "INVALID_SNAPSHOT"
	CodeStaleSnapshot         = "STALE_SNAPSHOT"
	CodeMassDeactivationHold  = "MASS_DEACTIVATION_HOLD"
	CodeDatabase              = "DATABASE_ERROR"
	CodeStaleRun              = "STALE_RUN"
	CodeInternal              = "INTERNAL_ERROR"
)

// Run is the persisted record of one sync attempt.
type Run struct {
	ID                string
	Status            Status
	Stage             Stage
	InitiatedBy       string
	SnapshotType      orgprovider.SnapshotType
	SourceGeneratedAt *time.Time
	CreatedAt         time.Time
	StartedAt         *time.Time
	CompletedAt       *time.Time
	HeartbeatAt       *time.Time

	InsertedUsers      int
	UpdatedUsers       int
	UnchangedUsers     int
	InsertedTeams      int
	UpdatedTeams       int
	UnchangedTeams     int
	MembershipChanges  int
	DeactivatedRecords int
	RetryCount         int

	ErrorCode    *string
	ErrorMessage *string
	FailedStage  *Stage
	WorkerID     *string

	// HoldReport is set when the failure was the mass-deactivation guard, so the
	// UI can show the counts and offer the explicit override.
	HoldReport json.RawMessage
}

// Failure describes why a run ended in FAILED.
type Failure struct {
	Stage      Stage
	Code       string
	Message    string // already safe to show an administrator
	HoldReport json.RawMessage
}

// Outcome carries the counters committed with a successful run.
type Outcome struct {
	InsertedUsers, UpdatedUsers, InsertedTeams, UpdatedTeams int
	MembershipChanges, DeactivatedRecords                    int
	SourceGeneratedAt                                        time.Time
}

// Errors returned by a Store.
var (
	// ErrActiveRunExists means a PENDING or RUNNING run is already recorded; the
	// existing run is returned alongside it so the caller can follow it instead.
	ErrActiveRunExists = errors.New("a synchronization run is already active")
	// ErrNotFound means no run has that id.
	ErrNotFound = errors.New("sync run not found")
	// ErrNotTransitionable means the run is no longer in a state this move is
	// legal from -- typically because a recovery sweep or another worker already
	// finished it. Callers must stop writing for that run.
	ErrNotTransitionable = errors.New("sync run is not in a state that allows this transition")
)

// NewParams are the inputs to create a PENDING run.
type NewParams struct {
	ID             string
	InitiatedBy    string
	IdempotencyKey string
	SnapshotType   orgprovider.SnapshotType
}

// Store persists runs and provides the cross-instance lock.
type Store interface {
	// CreatePending inserts a PENDING run. If an active run already exists it
	// returns that run with ErrActiveRunExists. If IdempotencyKey was used
	// before, the earlier run is returned with a nil error and created == false.
	CreatePending(ctx context.Context, p NewParams) (run *Run, created bool, err error)

	Get(ctx context.Context, id string) (*Run, error)
	Latest(ctx context.Context) (*Run, error) // nil, nil when no run exists

	// Active returns the run whose status is PENDING or RUNNING, or nil, nil when
	// none. It is the one authoritative answer to "is a sync in progress": a
	// run that has reached SUCCESS or FAILED is never returned, so a finished
	// sync can never keep the mass-deactivation threshold locked.
	Active(ctx context.Context) (*Run, error)

	// SetStage records progress and refreshes the heartbeat. It only affects a
	// non-terminal run and reports ErrNotTransitionable otherwise.
	SetStage(ctx context.Context, id string, stage Stage) error
	// MarkRunning moves PENDING -> RUNNING.
	MarkRunning(ctx context.Context, id, workerID string) error
	// Fail moves a PENDING or RUNNING run to FAILED.
	Fail(ctx context.Context, id string, f Failure) error

	// RecoverStale fails runs whose worker stopped heartbeating: RUNNING with a
	// heartbeat older than ttl, or PENDING older than ttl. Returns how many.
	RecoverStale(ctx context.Context, ttl time.Duration) (int, error)

	// TryLock takes the cross-instance sync lock without blocking. ok is false
	// when another holder has it. release must be called exactly once when ok.
	TryLock(ctx context.Context) (release func(), ok bool, err error)
}
