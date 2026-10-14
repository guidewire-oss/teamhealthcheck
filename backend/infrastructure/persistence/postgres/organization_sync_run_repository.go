package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	"github.com/google/uuid"
)

// orgSyncRunsKeep caps org_sync_runs: each insert prunes the oldest rows
// beyond this many, so the log cannot grow without bound.
const orgSyncRunsKeep = 1000

// orgSyncRunsNewestFirst orders org_sync_runs newest first: by when the
// request finished, with created_at and run_id breaking a tie so the order is
// always deterministic. There is no id column -- run_id is the key.
const orgSyncRunsNewestFirst = `finished_at DESC, created_at DESC, run_id DESC`

// orgSyncRunColumns is the column list every read of org_sync_runs selects,
// in the order scanOrgSyncRun expects.
const orgSyncRunColumns = `
	run_id, trigger, status, started_at, finished_at, instance_id,
	threshold_percent,
	users_existing, users_deleting, users_percent,
	teams_existing, teams_deleting, teams_percent,
	writes_status, override_used, users_synced, teams_synced,
	skip_reason, message`

// sqlExecer is the part of *sql.DB and *sql.Tx the insert needs, so the same
// insert runs inside ApplySnapshot's transaction or on its own.
type sqlExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// rowScanner is the part of *sql.Row and *sql.Rows scanOrgSyncRun needs.
type rowScanner interface {
	Scan(dest ...any) error
}

// ensureRunID returns id, or a fresh UUID when the caller supplied none, so
// a direct repository caller (a test, a future job) never trips the UUID
// column on an empty string.
func ensureRunID(id string) string {
	if id == "" {
		return uuid.NewString()
	}
	return id
}

// nullableString maps "" to NULL.
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// insertOrgSyncRun appends one row to org_sync_runs and prunes the oldest
// rows beyond orgSyncRunsKeep. e.RunID must already be set. holdActive is set
// only for a blocked row: it freezes threshold editing until a successful
// sync or an admin dismissal clears it.
func insertOrgSyncRun(ctx context.Context, exec sqlExecer, e orgprovider.OrgSyncHistoryEntry, holdActive bool) error {
	var holdRecordedAt any
	if holdActive {
		holdRecordedAt = e.FinishedAt
	}

	_, err := exec.ExecContext(ctx, `
		INSERT INTO org_sync_runs (
			run_id, trigger, status, started_at, finished_at, instance_id,
			threshold_percent,
			users_existing, users_deleting, users_percent,
			teams_existing, teams_deleting, teams_percent,
			writes_status, override_used, users_synced, teams_synced,
			skip_reason, message,
			hold_active, hold_recorded_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
	`,
		e.RunID, e.Trigger, e.Status, e.StartedAt, e.FinishedAt, nullableString(e.InstanceID),
		e.ThresholdPercent,
		e.UsersExisting, e.UsersDeleting, e.UsersPercent,
		e.TeamsExisting, e.TeamsDeleting, e.TeamsPercent,
		e.WritesStatus, e.OverrideUsed, e.UsersSynced, e.TeamsSynced,
		nullableString(e.SkipReason), nullableString(e.Message),
		holdActive, holdRecordedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to record organization sync run: %w", err)
	}

	// Everything past the newest orgSyncRunsKeep rows goes. The subquery is
	// empty while fewer rows than that exist, so nothing is deleted until the
	// cap is actually exceeded.
	if _, err := exec.ExecContext(ctx, `
		DELETE FROM org_sync_runs
		WHERE run_id IN (SELECT run_id FROM org_sync_runs ORDER BY `+orgSyncRunsNewestFirst+` OFFSET $1)
	`, orgSyncRunsKeep); err != nil {
		return fmt.Errorf("failed to prune organization sync runs: %w", err)
	}
	return nil
}

// clearOrgSyncHolds resolves every durable hold. Used by a successful sync
// (inside its own transaction) and by the admin's dismiss action.
func clearOrgSyncHolds(ctx context.Context, exec sqlExecer) error {
	_, err := exec.ExecContext(ctx, `
		UPDATE org_sync_runs SET hold_active = false, hold_recorded_at = NULL
		WHERE hold_active
	`)
	return err
}

// RecordOrgSyncAttempt persists an attempt that did NOT commit any
// destructive writes: a blocked hold, or a failure at any point in
// ApplySnapshot. Both rely on that function's own deferred Rollback(), so by
// construction nothing destructive ever accompanies them -- there is no
// crash window to close here, unlike the success path, which
// ApplySnapshot records internally in its own transaction (see
// recordSuccessAttempt in organization_provider_repository.go). This method
// must never be called for a success outcome.
//
// The durable hold is raised only for a Blocked outcome. A Failed outcome
// leaves every existing hold untouched, since a later unrelated technical
// failure must never silently clear a hold an earlier attempt correctly
// raised.
func (r *OrganizationProviderRepository) RecordOrgSyncAttempt(ctx context.Context, attempt orgprovider.OrgSyncAttempt) error {
	if attempt.Status == orgprovider.StatusSuccess {
		return fmt.Errorf("RecordOrgSyncAttempt must not be called for a success outcome; ApplySnapshot records that internally")
	}
	attempt.RunID = ensureRunID(attempt.RunID)

	// The insert and its prune commit together or not at all.
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to record sync attempt: %w", err)
	}
	defer tx.Rollback()

	isBlocked := attempt.Status == orgprovider.StatusBlocked
	if err := insertOrgSyncRun(ctx, tx, orgprovider.OrgSyncHistoryEntry{OrgSyncAttempt: attempt}, isBlocked); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to record sync attempt: %w", err)
	}
	return nil
}

// RecordOrgSyncSkip persists that a scheduled occurrence was skipped rather
// than attempted, as its own row with status 'skipped'. A skip is never an
// attempt, so it cannot displace the last attempt that actually ran.
func (r *OrganizationProviderRepository) RecordOrgSyncSkip(ctx context.Context, skip orgprovider.OrgSyncSkip) error {
	skip.RunID = ensureRunID(skip.RunID)

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to record sync skip: %w", err)
	}
	defer tx.Rollback()

	// A skipped occurrence never ran, so it has no writes, counts or
	// threshold -- only the reason.
	if err := insertOrgSyncRun(ctx, tx, orgprovider.OrgSyncHistoryEntry{
		OrgSyncAttempt: orgprovider.OrgSyncAttempt{
			RunID:        skip.RunID,
			Trigger:      orgprovider.TriggerScheduled,
			Status:       orgprovider.StatusSkipped,
			StartedAt:    skip.At,
			FinishedAt:   skip.At,
			WritesStatus: orgprovider.WritesNone,
			Message:      "Scheduled sync skipped: " + skip.Reason,
		},
		SkipReason: skip.Reason,
	}, false); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to record sync skip: %w", err)
	}
	return nil
}

// scanOrgSyncRun reads one row selected with orgSyncRunColumns.
func scanOrgSyncRun(row rowScanner) (orgprovider.OrgSyncHistoryEntry, error) {
	var (
		e                                                          orgprovider.OrgSyncHistoryEntry
		instanceID, skipReason, message                            sql.NullString
		thresholdPercent, usersPercent, teamsPercent               sql.NullFloat64
		usersExisting, usersDeleting, teamsExisting, teamsDeleting sql.NullInt64
		usersSynced, teamsSynced                                   sql.NullInt64
	)
	if err := row.Scan(
		&e.RunID, &e.Trigger, &e.Status, &e.StartedAt, &e.FinishedAt, &instanceID,
		&thresholdPercent,
		&usersExisting, &usersDeleting, &usersPercent,
		&teamsExisting, &teamsDeleting, &teamsPercent,
		&e.WritesStatus, &e.OverrideUsed, &usersSynced, &teamsSynced,
		&skipReason, &message,
	); err != nil {
		return e, err
	}
	e.InstanceID = instanceID.String
	e.SkipReason = skipReason.String
	e.Message = message.String
	e.ThresholdPercent = nullFloatPtr(thresholdPercent)
	e.UsersPercent = nullFloatPtr(usersPercent)
	e.TeamsPercent = nullFloatPtr(teamsPercent)
	e.UsersExisting = nullIntPtr(usersExisting)
	e.UsersDeleting = nullIntPtr(usersDeleting)
	e.TeamsExisting = nullIntPtr(teamsExisting)
	e.TeamsDeleting = nullIntPtr(teamsDeleting)
	e.UsersSynced = nullIntPtr(usersSynced)
	e.TeamsSynced = nullIntPtr(teamsSynced)
	return e, nil
}

// ListOrgSyncHistory returns up to limit rows of org_sync_runs, newest first.
func (r *OrganizationProviderRepository) ListOrgSyncHistory(ctx context.Context, limit int) ([]orgprovider.OrgSyncHistoryEntry, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+orgSyncRunColumns+` FROM org_sync_runs ORDER BY `+orgSyncRunsNewestFirst+` LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to read organization sync runs: %w", err)
	}
	defer rows.Close()

	entries := []orgprovider.OrgSyncHistoryEntry{}
	for rows.Next() {
		e, err := scanOrgSyncRun(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to read organization sync run row: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read organization sync runs: %w", err)
	}
	return entries, nil
}

// GetOrgSyncLastRun reads the assembled last-attempt / last-skip / hold state
// from the newest matching rows of org_sync_runs.
func (r *OrganizationProviderRepository) GetOrgSyncLastRun(ctx context.Context) (*orgprovider.OrgSyncLastRun, error) {
	result := &orgprovider.OrgSyncLastRun{}

	// Newest attempt that actually ran -- a skip is deliberately not one.
	attemptRow := r.db.QueryRowContext(ctx, `
		SELECT `+orgSyncRunColumns+` FROM org_sync_runs
		WHERE status IN ('success', 'blocked', 'failed')
		ORDER BY `+orgSyncRunsNewestFirst+` LIMIT 1
	`)
	if e, err := scanOrgSyncRun(attemptRow); err == nil {
		attempt := e.OrgSyncAttempt
		result.LastAttempt = &attempt
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to read organization sync last attempt: %w", err)
	}

	skipRow := r.db.QueryRowContext(ctx, `
		SELECT `+orgSyncRunColumns+` FROM org_sync_runs
		WHERE status = 'skipped'
		ORDER BY `+orgSyncRunsNewestFirst+` LIMIT 1
	`)
	if e, err := scanOrgSyncRun(skipRow); err == nil {
		result.LastSkip = &orgprovider.OrgSyncSkip{RunID: e.RunID, Reason: e.SkipReason, At: e.StartedAt}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to read organization sync last skip: %w", err)
	}

	// The newest unresolved hold. Its threshold comes from the held row
	// itself, so a later failure (which has none) cannot blank it.
	var (
		holdRecordedAt sql.NullTime
		holdThreshold  sql.NullFloat64
	)
	err := r.db.QueryRowContext(ctx, `
		SELECT hold_recorded_at, threshold_percent FROM org_sync_runs
		WHERE hold_active
		ORDER BY `+orgSyncRunsNewestFirst+` LIMIT 1
	`).Scan(&holdRecordedAt, &holdThreshold)
	switch {
	case err == nil:
		result.HoldActive = true
		if holdRecordedAt.Valid {
			t := holdRecordedAt.Time
			result.HoldRecordedAt = &t
		}
		result.HoldThresholdPercent = nullFloatPtr(holdThreshold)
	case !errors.Is(err, sql.ErrNoRows):
		return nil, fmt.Errorf("failed to read organization sync hold: %w", err)
	}

	return result, nil
}

// ClearOrgSyncHold clears the durable mass-deletion hold unconditionally.
// Called by the "dismiss without syncing" admin action ALONGSIDE (never
// instead of) the in-memory DismissHold, so the escape hatch works even on a
// replica that never itself ran the held sync.
func (r *OrganizationProviderRepository) ClearOrgSyncHold(ctx context.Context) error {
	if err := clearOrgSyncHolds(ctx, r.db); err != nil {
		return fmt.Errorf("failed to clear organization sync hold: %w", err)
	}
	return nil
}

func nullFloatPtr(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	f := v.Float64
	return &f
}

func nullIntPtr(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	i := int(v.Int64)
	return &i
}
