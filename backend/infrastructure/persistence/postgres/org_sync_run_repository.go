package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	"github.com/agopalakrishnan/teams360/backend/domain/orgsyncrun"
	"github.com/agopalakrishnan/teams360/backend/domain/orgsyncschedule"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres/sqlcgen"
	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
	"github.com/lib/pq"
	"github.com/sqlc-dev/pqtype"
)

// orgSyncLockKey is the pg advisory-lock key shared by every API replica. It is
// a fixed constant ("ORGSYNC" in ASCII) so all instances contend on one lock.
const orgSyncLockKey int64 = 0x4f5247_53594e43

// maxErrorMessage matches the org_sync_runs.error_message column.
const maxErrorMessage = 500

// truncateRunes cuts s to at most max characters without splitting a UTF-8
// sequence. The error_message column is VARCHAR(500), which counts characters,
// and Postgres rejects invalid UTF-8, so a byte-based cut could make the failure
// update itself fail and leave the run active.
func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}

// OrgSyncRunRepository implements orgsyncrun.Store on PostgreSQL.
type OrgSyncRunRepository struct {
	db      *sql.DB
	queries *sqlcgen.Queries
}

// NewOrgSyncRunRepository creates the store.
func NewOrgSyncRunRepository(db *sql.DB) *OrgSyncRunRepository {
	return &OrgSyncRunRepository{db: db, queries: sqlcgen.New(db)}
}

// toRun maps a generated org_sync_runs row to the domain Run.
func toRun(row sqlcgen.OrgSyncRun) *orgsyncrun.Run {
	r := orgsyncrun.Run{
		ID:                     row.ID,
		Status:                 orgsyncrun.Status(row.Status),
		Trigger:                orgsyncrun.Trigger(row.TriggerType),
		Stage:                  orgsyncrun.Stage(row.Stage),
		InitiatedBy:            row.InitiatedBy,
		SnapshotType:           orgprovider.SnapshotType(row.SnapshotType),
		SourceGeneratedAt:      nullTime(row.SourceGeneratedAt),
		CreatedAt:              row.CreatedAt,
		StartedAt:              nullTime(row.StartedAt),
		CompletedAt:            nullTime(row.CompletedAt),
		HeartbeatAt:            nullTime(row.HeartbeatAt),
		InsertedUsers:          int(row.InsertedUsers),
		UpdatedUsers:           int(row.UpdatedUsers),
		InsertedTeams:          int(row.InsertedTeams),
		UpdatedTeams:           int(row.UpdatedTeams),
		MembershipChanges:      int(row.MembershipChanges),
		DeactivatedRecords:     int(row.DeactivatedRecords),
		RetryCount:             int(row.RetryCount),
		HealthChecksDisabled:   int(row.HealthChecksDisabled),
		HealthChecksEnabled:    int(row.HealthChecksEnabled),
		MassDeletionOverridden: row.MassDeletionOverridden,
	}
	// The columns are NOT NULL and hold 0 when nothing was recorded, so the flag,
	// not the value, says whether a count is real. Unknown stays nil, never 0.
	if row.UnchangedRecorded {
		unchangedUsers, unchangedTeams := int(row.UnchangedUsers), int(row.UnchangedTeams)
		r.UnchangedUsers, r.UnchangedTeams = &unchangedUsers, &unchangedTeams
	}
	if row.ErrorCode.Valid {
		r.ErrorCode = &row.ErrorCode.String
	}
	if row.ErrorMessage.Valid {
		r.ErrorMessage = &row.ErrorMessage.String
	}
	if row.FailedStage.Valid {
		fs := orgsyncrun.Stage(row.FailedStage.String)
		r.FailedStage = &fs
	}
	if row.WorkerID.Valid {
		r.WorkerID = &row.WorkerID.String
	}
	if row.HoldReport.Valid {
		r.HoldReport = row.HoldReport.RawMessage
	}
	return &r
}

func nullTime(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

// CreatePending inserts a PENDING run. The partial unique index
// uq_org_sync_runs_single_active makes the "only one active run" rule a
// database guarantee, so two replicas cannot both admit a run.
func (r *OrgSyncRunRepository) CreatePending(ctx context.Context, p orgsyncrun.NewParams) (*orgsyncrun.Run, bool, error) {
	if p.IdempotencyKey != "" {
		if row, err := r.queries.FindOrgSyncRunByIdempotencyKey(ctx, sql.NullString{String: p.IdempotencyKey, Valid: true}); err == nil {
			return toRun(row), false, nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, false, fmt.Errorf("failed to look up idempotency key: %w", err)
		}
	}

	trigger := p.Trigger
	if trigger == "" {
		trigger = orgsyncrun.TriggerManual
	}
	err := r.queries.InsertPendingOrgSyncRun(ctx, sqlcgen.InsertPendingOrgSyncRunParams{
		ID:             p.ID,
		Stage:          string(orgsyncrun.StageQueued),
		InitiatedBy:    p.InitiatedBy,
		IdempotencyKey: sql.NullString{String: p.IdempotencyKey, Valid: p.IdempotencyKey != ""},
		SnapshotType:   string(p.SnapshotType),
		TriggerType:    string(trigger),
	})
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			if pqErr.Constraint == "uq_org_sync_runs_idempotency" && p.IdempotencyKey != "" {
				return r.byIdempotencyKey(ctx, p.IdempotencyKey)
			}
			active, aErr := r.active(ctx)
			if aErr != nil {
				return nil, false, aErr
			}
			return active, false, orgsyncrun.ErrActiveRunExists
		}
		return nil, false, fmt.Errorf("failed to create sync run: %w", err)
	}

	run, err := r.Get(ctx, p.ID)
	return run, true, err
}

func (r *OrgSyncRunRepository) byIdempotencyKey(ctx context.Context, key string) (*orgsyncrun.Run, bool, error) {
	row, err := r.queries.FindOrgSyncRunByIdempotencyKey(ctx, sql.NullString{String: key, Valid: true})
	if err != nil {
		return nil, false, err
	}
	return toRun(row), false, nil
}

func (r *OrgSyncRunRepository) active(ctx context.Context) (*orgsyncrun.Run, error) {
	run, err := r.Active(ctx)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, orgsyncrun.ErrActiveRunExists
	}
	return run, nil
}

// Active returns the PENDING or RUNNING run, or nil when no sync is in progress.
func (r *OrgSyncRunRepository) Active(ctx context.Context) (*orgsyncrun.Run, error) {
	row, err := r.queries.FindActiveOrgSyncRun(ctx, orgsyncrun.ActiveStatuses())
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toRun(row), nil
}

// Get returns one run.
func (r *OrgSyncRunRepository) Get(ctx context.Context, id string) (*orgsyncrun.Run, error) {
	row, err := r.queries.GetOrgSyncRun(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, orgsyncrun.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return toRun(row), nil
}

// Latest returns the most recently created run, or nil when there is none.
func (r *OrgSyncRunRepository) Latest(ctx context.Context) (*orgsyncrun.Run, error) {
	row, err := r.queries.FindLatestOrgSyncRun(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toRun(row), nil
}

// LatestByTrigger returns the newest run started by the given trigger, or nil.
func (r *OrgSyncRunRepository) LatestByTrigger(ctx context.Context, t orgsyncrun.Trigger) (*orgsyncrun.Run, error) {
	row, err := r.queries.FindLatestOrgSyncRunByTrigger(ctx, string(t))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toRun(row), nil
}

// LatestSuccessByTrigger returns the newest SUCCESS run for the trigger, or nil.
func (r *OrgSyncRunRepository) LatestSuccessByTrigger(ctx context.Context, t orgsyncrun.Trigger) (*orgsyncrun.Run, error) {
	row, err := r.queries.FindLatestSuccessfulOrgSyncRunByTrigger(ctx, string(t))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toRun(row), nil
}

// List returns one page of runs newest first plus the total matching the filter.
// Every filter is a bound parameter; a NULL parameter disables its predicate, so
// one static query serves all combinations.
func (r *OrgSyncRunRepository) List(ctx context.Context, f orgsyncrun.HistoryFilter) ([]orgsyncrun.Run, int, error) {
	nullable := func(v string) sql.NullString { return sql.NullString{String: v, Valid: v != ""} }
	status, trigger, snap := nullable(string(f.Status)), nullable(string(f.Trigger)), nullable(string(f.SnapshotType))

	total, err := r.queries.CountOrgSyncRuns(ctx, sqlcgen.CountOrgSyncRunsParams{
		Status: status, TriggerType: trigger, SnapshotType: snap,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count sync runs: %w", err)
	}
	rows, err := r.queries.ListOrgSyncRuns(ctx, sqlcgen.ListOrgSyncRunsParams{
		Status: status, TriggerType: trigger, SnapshotType: snap,
		PageLimit: int64(f.Limit), PageOffset: int64(f.Offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list sync runs: %w", err)
	}
	runs := make([]orgsyncrun.Run, 0, len(rows))
	for _, row := range rows {
		runs = append(runs, *toRun(row))
	}
	return runs, int(total), nil
}

// SetStage records progress and refreshes the heartbeat on a non-terminal run.
func (r *OrgSyncRunRepository) SetStage(ctx context.Context, id string, stage orgsyncrun.Stage) error {
	n, err := r.queries.SetOrgSyncRunStage(ctx, sqlcgen.SetOrgSyncRunStageParams{
		Stage: string(stage), ID: id, AllowedStatuses: orgsyncrun.ActiveStatuses(),
	})
	return expectOne(n, err)
}

// MarkRunning moves PENDING -> RUNNING.
func (r *OrgSyncRunRepository) MarkRunning(ctx context.Context, id, workerID string) error {
	n, err := r.queries.MarkOrgSyncRunRunning(ctx, sqlcgen.MarkOrgSyncRunRunningParams{
		WorkerID:        sql.NullString{String: workerID, Valid: true},
		ID:              id,
		AllowedStatuses: orgsyncrun.StatusesBefore(orgsyncrun.StatusRunning),
	})
	return expectOne(n, err)
}

// Fail moves a non-terminal run to FAILED, recording the stage it failed in.
func (r *OrgSyncRunRepository) Fail(ctx context.Context, id string, f orgsyncrun.Failure) error {
	n, err := r.queries.FailOrgSyncRun(ctx, sqlcgen.FailOrgSyncRunParams{
		Stage:           string(orgsyncrun.StageFailed),
		FailedStage:     sql.NullString{String: string(f.Stage), Valid: true},
		ErrorCode:       sql.NullString{String: f.Code, Valid: true},
		ErrorMessage:    sql.NullString{String: truncateRunes(f.Message, maxErrorMessage), Valid: true},
		HoldReport:      pqtype.NullRawMessage{RawMessage: f.HoldReport, Valid: len(f.HoldReport) > 0},
		ID:              id,
		AllowedStatuses: orgsyncrun.StatusesBefore(orgsyncrun.StatusFailed),
	})
	return expectOne(n, err)
}

// RecoverStale fails runs abandoned by a crashed worker so the single-active
// index frees up and the UI stops showing them as in progress.
func (r *OrgSyncRunRepository) RecoverStale(ctx context.Context, ttl time.Duration) (int, error) {
	n, err := r.queries.RecoverStaleOrgSyncRuns(ctx, sqlcgen.RecoverStaleOrgSyncRunsParams{
		FailedStageName: string(orgsyncrun.StageFailed),
		ErrorCode:       sql.NullString{String: orgsyncrun.CodeStaleRun, Valid: true},
		TtlSeconds:      ttl.Seconds(),
	})
	if err != nil {
		return 0, fmt.Errorf("failed to recover stale sync runs: %w", err)
	}
	return int(n), nil
}

// TryLock takes a session-level advisory lock on a dedicated connection. The
// lock belongs to that connection, so release unlocks and returns it to the
// pool; if the process dies the server drops the lock with the connection,
// which is the expiry guarantee a crashed worker needs.
func (r *OrgSyncRunRepository) TryLock(ctx context.Context) (func(), bool, error) {
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("failed to open lock connection: %w", err)
	}
	// Queries over the dedicated *sql.Conn so lock and unlock share one session.
	cq := sqlcgen.New(conn)
	got, err := cq.TryOrgSyncAdvisoryLock(ctx, orgSyncLockKey)
	if err != nil {
		conn.Close()
		return nil, false, fmt.Errorf("failed to try sync lock: %w", err)
	}
	if !got {
		conn.Close()
		return nil, false, nil
	}
	release := func() {
		// A fresh context: release must still run when the request context is gone.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		unlocked, err := cq.OrgSyncAdvisoryUnlock(ctx, orgSyncLockKey)
		if err != nil || !unlocked {
			// The lock belongs to this database session, so returning the
			// connection to the pool would keep it held and block every other
			// replica until the connection happens to close. Report the
			// connection as bad so database/sql closes it instead; ending the
			// session releases the lock.
			logger.Get().WithError(err).Error("could not release the sync advisory lock; discarding its connection")
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
		conn.Close()
	}
	return release, true, nil
}

// expectOne turns an UPDATE's affected-row count into the transition result.
func expectOne(n int64, err error) error {
	if err != nil {
		return fmt.Errorf("failed to update sync run: %w", err)
	}
	if n != 1 {
		return orgsyncrun.ErrNotTransitionable
	}
	return nil
}

var (
	_ orgsyncrun.Store           = (*OrgSyncRunRepository)(nil)
	_ orgsyncrun.HistoryReader   = (*OrgSyncRunRepository)(nil)
	_ orgsyncschedule.RunHistory = (*OrgSyncRunRepository)(nil)
)
