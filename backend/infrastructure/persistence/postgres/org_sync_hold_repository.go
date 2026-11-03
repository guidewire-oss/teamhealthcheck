package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/agopalakrishnan/teams360/backend/domain/orgsyncrun"
)

// OrgSyncHoldRepository implements orgsyncrun.HoldStore on PostgreSQL.
type OrgSyncHoldRepository struct {
	db *sql.DB
}

// NewOrgSyncHoldRepository creates the store.
func NewOrgSyncHoldRepository(db *sql.DB) *OrgSyncHoldRepository {
	return &OrgSyncHoldRepository{db: db}
}

// GetHold returns the current hold, or nil, nil when there is none.
func (r *OrgSyncHoldRepository) GetHold(ctx context.Context) (*orgsyncrun.Hold, error) {
	var (
		h      orgsyncrun.Hold
		report []byte
	)
	err := r.db.QueryRowContext(ctx,
		`SELECT threshold, report, held_at FROM org_sync_hold WHERE id = 1`,
	).Scan(&h.Threshold, &report, &h.HeldAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read sync hold: %w", err)
	}
	if err := json.Unmarshal(report, &h.Report); err != nil {
		return nil, fmt.Errorf("decode sync hold report: %w", err)
	}
	h.HeldAt = h.HeldAt.UTC()
	return &h, nil
}

// SaveHold records a hold, replacing any earlier one.
func (r *OrgSyncHoldRepository) SaveHold(ctx context.Context, h orgsyncrun.Hold) error {
	report, err := json.Marshal(h.Report)
	if err != nil {
		return fmt.Errorf("encode sync hold report: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO org_sync_hold (id, threshold, report, held_at)
		VALUES (1, $1, $2, $3)
		ON CONFLICT (id) DO UPDATE
		SET threshold = EXCLUDED.threshold, report = EXCLUDED.report, held_at = EXCLUDED.held_at`,
		h.Threshold, report, h.HeldAt.UTC())
	if err != nil {
		return fmt.Errorf("save sync hold: %w", err)
	}
	return nil
}

// ClearHold removes the hold and reports whether there was one. It also drops
// the hold report of the run that raised it (the newest run carrying one), in
// the same transaction. Older runs keep their reports as history. The admin UI
// derives its "Sync Anyway" banner from the latest failed run, so without this
// a dismissed (or expired) hold would reappear after a page refresh. The run
// itself stays FAILED, with its error message, as history.
func (r *OrgSyncHoldRepository) ClearHold(ctx context.Context) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("clear sync hold: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `DELETE FROM org_sync_hold WHERE id = 1`)
	if err != nil {
		return false, fmt.Errorf("clear sync hold: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("clear sync hold: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE org_sync_runs SET hold_report = NULL
		 WHERE id = (SELECT id FROM org_sync_runs WHERE hold_report IS NOT NULL
		             ORDER BY created_at DESC, id DESC LIMIT 1)`); err != nil {
		return false, fmt.Errorf("clear sync run hold reports: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("clear sync hold: %w", err)
	}
	return n > 0, nil
}

var _ orgsyncrun.HoldStore = (*OrgSyncHoldRepository)(nil)
