package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/agopalakrishnan/teams360/backend/domain/orgsyncrun"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres/sqlcgen"
)

// OrgSyncHoldRepository implements orgsyncrun.HoldStore on PostgreSQL.
type OrgSyncHoldRepository struct {
	db      *sql.DB
	queries *sqlcgen.Queries
}

// NewOrgSyncHoldRepository creates the store.
func NewOrgSyncHoldRepository(db *sql.DB) *OrgSyncHoldRepository {
	return &OrgSyncHoldRepository{db: db, queries: sqlcgen.New(db)}
}

// GetHold returns the current hold, or nil, nil when there is none.
func (r *OrgSyncHoldRepository) GetHold(ctx context.Context) (*orgsyncrun.Hold, error) {
	row, err := r.queries.GetOrgSyncHold(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read sync hold: %w", err)
	}
	h := orgsyncrun.Hold{Threshold: row.Threshold, HeldAt: row.HeldAt}
	if err := json.Unmarshal(row.Report, &h.Report); err != nil {
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
	err = r.queries.UpsertOrgSyncHold(ctx, sqlcgen.UpsertOrgSyncHoldParams{
		Threshold: h.Threshold,
		Report:    report,
		HeldAt:    h.HeldAt.UTC(),
	})
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

	qtx := r.queries.WithTx(tx)

	n, err := qtx.DeleteOrgSyncHold(ctx)
	if err != nil {
		return false, fmt.Errorf("clear sync hold: %w", err)
	}
	if err := qtx.ClearLatestOrgSyncRunHoldReport(ctx); err != nil {
		return false, fmt.Errorf("clear sync run hold reports: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("clear sync hold: %w", err)
	}
	return n > 0, nil
}

var _ orgsyncrun.HoldStore = (*OrgSyncHoldRepository)(nil)
