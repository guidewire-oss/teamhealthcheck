package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/agopalakrishnan/teams360/backend/domain/orgsyncschedule"
)

// OrgSyncScheduleRepository implements orgsyncschedule.Store on PostgreSQL.
type OrgSyncScheduleRepository struct {
	db *sql.DB
}

// NewOrgSyncScheduleRepository creates the store.
func NewOrgSyncScheduleRepository(db *sql.DB) *OrgSyncScheduleRepository {
	return &OrgSyncScheduleRepository{db: db}
}

// Get returns the singleton schedule row (created disabled by migration 000031).
func (r *OrgSyncScheduleRepository) Get(ctx context.Context) (*orgsyncschedule.Schedule, error) {
	var (
		s    orgsyncschedule.Schedule
		freq string
		next sql.NullTime
		by   sql.NullString
	)
	err := r.db.QueryRowContext(ctx,
		`SELECT enabled, frequency, next_run_at, updated_at, updated_by FROM org_sync_schedule WHERE id = 1`,
	).Scan(&s.Enabled, &freq, &next, &s.UpdatedAt, &by)
	if err != nil {
		return nil, fmt.Errorf("failed to load org sync schedule: %w", err)
	}
	s.Frequency = orgsyncschedule.Frequency(freq)
	s.UpdatedAt = s.UpdatedAt.UTC()
	if next.Valid {
		t := next.Time.UTC()
		s.NextRunAt = &t
	}
	if by.Valid {
		s.UpdatedBy = &by.String
	}
	return &s, nil
}

// Save replaces the configuration.
func (r *OrgSyncScheduleRepository) Save(ctx context.Context, enabled bool, f orgsyncschedule.Frequency, nextRunAt *time.Time, updatedBy string) (*orgsyncschedule.Schedule, error) {
	var next any
	if enabled && nextRunAt != nil {
		next = nextRunAt.UTC()
	}
	var by any
	if updatedBy != "" {
		by = updatedBy
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE org_sync_schedule
		SET enabled = $1, frequency = $2, next_run_at = $3, updated_at = CURRENT_TIMESTAMP, updated_by = $4
		WHERE id = 1`, enabled, string(f), next, by)
	if err != nil {
		return nil, fmt.Errorf("failed to save org sync schedule: %w", err)
	}
	return r.Get(ctx)
}

// Claim is a compare-and-swap on next_run_at. Re-checking enabled in the WHERE
// clause means a registration that outlived a "disable" cannot launch a run.
func (r *OrgSyncScheduleRepository) Claim(ctx context.Context, due, next time.Time) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE org_sync_schedule SET next_run_at = $2
		WHERE id = 1 AND enabled AND next_run_at = $1`, due.UTC(), next.UTC())
	if err != nil {
		return false, fmt.Errorf("failed to claim org sync occurrence: %w", err)
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

var _ orgsyncschedule.Store = (*OrgSyncScheduleRepository)(nil)
