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
	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
	"github.com/lib/pq"
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
	db *sql.DB
}

// NewOrgSyncRunRepository creates the store.
func NewOrgSyncRunRepository(db *sql.DB) *OrgSyncRunRepository {
	return &OrgSyncRunRepository{db: db}
}

const runColumns = `id, status, stage, initiated_by, snapshot_type, source_generated_at,
	created_at, started_at, completed_at, heartbeat_at,
	inserted_users, updated_users, inserted_teams, updated_teams, membership_changes,
	unchanged_users, unchanged_teams, unchanged_recorded, deactivated_records, retry_count, error_code, error_message, failed_stage, worker_id, hold_report,
	health_checks_disabled, health_checks_enabled, mass_deletion_overridden, trigger_type`

type rowScanner interface{ Scan(dest ...any) error }

func scanRun(row rowScanner) (*orgsyncrun.Run, error) {
	var (
		r                                      orgsyncrun.Run
		status, stage, snapType                string
		srcAt, started, completed, heartbeat   sql.NullTime
		errCode, errMsg, failedStage, workerID sql.NullString
		hold                                   []byte
		unchangedUsers, unchangedTeams         int
		unchangedRecorded                      bool
		trigger                                string
	)
	if err := row.Scan(&r.ID, &status, &stage, &r.InitiatedBy, &snapType, &srcAt,
		&r.CreatedAt, &started, &completed, &heartbeat,
		&r.InsertedUsers, &r.UpdatedUsers, &r.InsertedTeams, &r.UpdatedTeams, &r.MembershipChanges,
		&unchangedUsers, &unchangedTeams, &unchangedRecorded, &r.DeactivatedRecords, &r.RetryCount, &errCode, &errMsg, &failedStage, &workerID, &hold,
		&r.HealthChecksDisabled, &r.HealthChecksEnabled, &r.MassDeletionOverridden, &trigger); err != nil {
		return nil, err
	}
	// The columns are NOT NULL and hold 0 when nothing was recorded, so the flag,
	// not the value, says whether a count is real. Unknown stays nil, never 0.
	if unchangedRecorded {
		r.UnchangedUsers, r.UnchangedTeams = &unchangedUsers, &unchangedTeams
	}
	r.Status = orgsyncrun.Status(status)
	r.Trigger = orgsyncrun.Trigger(trigger)
	r.Stage = orgsyncrun.Stage(stage)
	r.SnapshotType = orgprovider.SnapshotType(snapType)
	r.SourceGeneratedAt = nullTime(srcAt)
	r.StartedAt = nullTime(started)
	r.CompletedAt = nullTime(completed)
	r.HeartbeatAt = nullTime(heartbeat)
	if errCode.Valid {
		r.ErrorCode = &errCode.String
	}
	if errMsg.Valid {
		r.ErrorMessage = &errMsg.String
	}
	if failedStage.Valid {
		fs := orgsyncrun.Stage(failedStage.String)
		r.FailedStage = &fs
	}
	if workerID.Valid {
		r.WorkerID = &workerID.String
	}
	r.HoldReport = hold
	return &r, nil
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
		row := r.db.QueryRowContext(ctx, `SELECT `+runColumns+` FROM org_sync_runs WHERE idempotency_key = $1`, p.IdempotencyKey)
		if existing, err := scanRun(row); err == nil {
			return existing, false, nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, false, fmt.Errorf("failed to look up idempotency key: %w", err)
		}
	}

	var key any
	if p.IdempotencyKey != "" {
		key = p.IdempotencyKey
	}
	trigger := p.Trigger
	if trigger == "" {
		trigger = orgsyncrun.TriggerManual
	}
	// unchanged_recorded keeps its default FALSE: a new run has not recorded its
	// unchanged counts yet (see migration 000032).
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO org_sync_runs (id, status, stage, initiated_by, idempotency_key, snapshot_type, trigger_type)
		VALUES ($1, 'PENDING', $5, $2, $3, $4, $6)
	`, p.ID, p.InitiatedBy, key, string(p.SnapshotType), string(orgsyncrun.StageQueued), string(trigger))
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
	run, err := scanRun(r.db.QueryRowContext(ctx, `SELECT `+runColumns+` FROM org_sync_runs WHERE idempotency_key = $1`, key))
	return run, false, err
}

func (r *OrgSyncRunRepository) active(ctx context.Context) (*orgsyncrun.Run, error) {
	run, err := scanRun(r.db.QueryRowContext(ctx,
		`SELECT `+runColumns+` FROM org_sync_runs WHERE status = ANY($1) ORDER BY created_at DESC LIMIT 1`,
		pq.Array(orgsyncrun.ActiveStatuses())))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, orgsyncrun.ErrActiveRunExists
	}
	return run, err
}

// Active returns the PENDING or RUNNING run, or nil when no sync is in progress.
func (r *OrgSyncRunRepository) Active(ctx context.Context) (*orgsyncrun.Run, error) {
	run, err := scanRun(r.db.QueryRowContext(ctx,
		`SELECT `+runColumns+` FROM org_sync_runs WHERE status = ANY($1) ORDER BY created_at DESC LIMIT 1`,
		pq.Array(orgsyncrun.ActiveStatuses())))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return run, err
}

// Get returns one run.
func (r *OrgSyncRunRepository) Get(ctx context.Context, id string) (*orgsyncrun.Run, error) {
	run, err := scanRun(r.db.QueryRowContext(ctx, `SELECT `+runColumns+` FROM org_sync_runs WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, orgsyncrun.ErrNotFound
	}
	return run, err
}

// Latest returns the most recently created run, or nil when there is none.
func (r *OrgSyncRunRepository) Latest(ctx context.Context) (*orgsyncrun.Run, error) {
	run, err := scanRun(r.db.QueryRowContext(ctx, `SELECT `+runColumns+` FROM org_sync_runs ORDER BY created_at DESC LIMIT 1`))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return run, err
}

// LatestByTrigger returns the newest run started by the given trigger, or nil.
func (r *OrgSyncRunRepository) LatestByTrigger(ctx context.Context, t orgsyncrun.Trigger) (*orgsyncrun.Run, error) {
	run, err := scanRun(r.db.QueryRowContext(ctx,
		`SELECT `+runColumns+` FROM org_sync_runs WHERE trigger_type = $1 ORDER BY created_at DESC LIMIT 1`, string(t)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return run, err
}

// LatestSuccessByTrigger returns the newest SUCCESS run for the trigger, or nil.
func (r *OrgSyncRunRepository) LatestSuccessByTrigger(ctx context.Context, t orgsyncrun.Trigger) (*orgsyncrun.Run, error) {
	run, err := scanRun(r.db.QueryRowContext(ctx,
		`SELECT `+runColumns+` FROM org_sync_runs WHERE trigger_type = $1 AND status = 'SUCCESS'
		 ORDER BY completed_at DESC NULLS LAST, created_at DESC LIMIT 1`, string(t)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return run, err
}

// List returns one page of runs newest first plus the total matching the filter.
// Every filter is a bound parameter; a NULL parameter disables its predicate, so
// one static query serves all combinations. The ORDER BY is covered by
// idx_org_sync_runs_created for the unfiltered case; the table holds one row per
// sync attempt, so the filtered scans stay small.
func (r *OrgSyncRunRepository) List(ctx context.Context, f orgsyncrun.HistoryFilter) ([]orgsyncrun.Run, int, error) {
	nullable := func(v string) any {
		if v == "" {
			return nil
		}
		return v
	}
	status, trigger, snap := nullable(string(f.Status)), nullable(string(f.Trigger)), nullable(string(f.SnapshotType))
	const where = `WHERE ($1::text IS NULL OR status = $1)
		AND ($2::text IS NULL OR trigger_type = $2)
		AND ($3::text IS NULL OR snapshot_type = $3)`

	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM org_sync_runs `+where, status, trigger, snap).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count sync runs: %w", err)
	}
	rows, err := r.db.QueryContext(ctx, `SELECT `+runColumns+` FROM org_sync_runs `+where+`
		ORDER BY created_at DESC, id DESC LIMIT $4 OFFSET $5`, status, trigger, snap, f.Limit, f.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list sync runs: %w", err)
	}
	defer rows.Close()
	runs := []orgsyncrun.Run{}
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan sync run: %w", err)
		}
		runs = append(runs, *run)
	}
	return runs, total, rows.Err()
}

// SetStage records progress and refreshes the heartbeat on a non-terminal run.
func (r *OrgSyncRunRepository) SetStage(ctx context.Context, id string, stage orgsyncrun.Stage) error {
	return r.expectOne(ctx, `
		UPDATE org_sync_runs SET stage = $2, heartbeat_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND status = ANY($3)`, id, string(stage), pq.Array(orgsyncrun.ActiveStatuses()))
}

// MarkRunning moves PENDING -> RUNNING.
func (r *OrgSyncRunRepository) MarkRunning(ctx context.Context, id, workerID string) error {
	return r.expectOne(ctx, `
		UPDATE org_sync_runs SET status = 'RUNNING', started_at = CURRENT_TIMESTAMP,
			heartbeat_at = CURRENT_TIMESTAMP, worker_id = $2
		WHERE id = $1 AND status = ANY($3)`, id, workerID, pq.Array(orgsyncrun.StatusesBefore(orgsyncrun.StatusRunning)))
}

// Fail moves a non-terminal run to FAILED, recording the stage it failed in.
func (r *OrgSyncRunRepository) Fail(ctx context.Context, id string, f orgsyncrun.Failure) error {
	msg := truncateRunes(f.Message, maxErrorMessage)
	var hold any
	if len(f.HoldReport) > 0 {
		hold = []byte(f.HoldReport)
	}
	return r.expectOne(ctx, `
		UPDATE org_sync_runs SET status = 'FAILED', stage = $7, failed_stage = $2,
			error_code = $3, error_message = $4, hold_report = $5,
			completed_at = CURRENT_TIMESTAMP, heartbeat_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND status = ANY($6)`,
		id, string(f.Stage), f.Code, msg, hold, pq.Array(orgsyncrun.StatusesBefore(orgsyncrun.StatusFailed)),
		string(orgsyncrun.StageFailed))
}

// RecoverStale fails runs abandoned by a crashed worker so the single-active
// index frees up and the UI stops showing them as in progress.
func (r *OrgSyncRunRepository) RecoverStale(ctx context.Context, ttl time.Duration) (int, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE org_sync_runs SET status = 'FAILED', failed_stage = stage, stage = $3,
			error_code = $2, error_message = 'The synchronization stopped responding and was marked failed. It is safe to run it again.',
			completed_at = CURRENT_TIMESTAMP
		WHERE (status = 'RUNNING' AND COALESCE(heartbeat_at, started_at, created_at) < NOW() - make_interval(secs => $1))
		   OR (status = 'PENDING' AND created_at < NOW() - make_interval(secs => $1))
	`, ttl.Seconds(), orgsyncrun.CodeStaleRun, string(orgsyncrun.StageFailed))
	if err != nil {
		return 0, fmt.Errorf("failed to recover stale sync runs: %w", err)
	}
	n, err := res.RowsAffected()
	return int(n), err
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
	var got bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, orgSyncLockKey).Scan(&got); err != nil {
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
		var unlocked bool
		err := conn.QueryRowContext(ctx, `SELECT pg_advisory_unlock($1)`, orgSyncLockKey).Scan(&unlocked)
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

func (r *OrgSyncRunRepository) expectOne(ctx context.Context, query string, args ...any) error {
	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update sync run: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
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
