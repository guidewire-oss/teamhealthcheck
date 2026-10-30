package dto

import (
	"encoding/json"
	"time"

	"github.com/agopalakrishnan/teams360/backend/domain/orgsyncrun"
)

// SyncRunDTO is the persisted state of one organization sync run.
type SyncRunDTO struct {
	RunID              string          `json:"runId"`
	Status             string          `json:"status"` // PENDING | RUNNING | SUCCESS | FAILED
	Stage              string          `json:"stage"`
	StartedAt          *time.Time      `json:"startedAt"`
	CompletedAt        *time.Time      `json:"completedAt"`
	CreatedAt          time.Time       `json:"createdAt"`
	InsertedUsers      int             `json:"insertedUsers"`
	UpdatedUsers       int             `json:"updatedUsers"`
	UnchangedUsers     int             `json:"unchangedUsers"`
	InsertedTeams      int             `json:"insertedTeams"`
	UpdatedTeams       int             `json:"updatedTeams"`
	UnchangedTeams     int             `json:"unchangedTeams"`
	MembershipChanges  int             `json:"membershipChanges"`
	DeactivatedRecords int             `json:"deactivatedRecords"`
	ErrorCode          *string         `json:"errorCode"`
	ErrorMessage       *string         `json:"errorMessage"`
	FailedStage        *string         `json:"failedStage"`
	MassDeletion       json.RawMessage `json:"massDeletion,omitempty"`
	// AlreadyActive is true when a start request was answered with a run that
	// was already in flight rather than a new one.
	AlreadyActive bool `json:"alreadyActive,omitempty"`
}

// NewSyncRunDTO maps a run onto its transport shape.
func NewSyncRunDTO(r *orgsyncrun.Run) SyncRunDTO {
	d := SyncRunDTO{
		RunID: r.ID, Status: string(r.Status), Stage: string(r.Stage),
		StartedAt: r.StartedAt, CompletedAt: r.CompletedAt, CreatedAt: r.CreatedAt,
		InsertedUsers: r.InsertedUsers, UpdatedUsers: r.UpdatedUsers,
		InsertedTeams: r.InsertedTeams, UpdatedTeams: r.UpdatedTeams,
		UnchangedUsers: r.UnchangedUsers, UnchangedTeams: r.UnchangedTeams,
		MembershipChanges: r.MembershipChanges, DeactivatedRecords: r.DeactivatedRecords,
		ErrorCode: r.ErrorCode, ErrorMessage: r.ErrorMessage, MassDeletion: r.HoldReport,
	}
	if r.FailedStage != nil {
		fs := string(*r.FailedStage)
		d.FailedStage = &fs
	}
	return d
}
