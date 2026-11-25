package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/agopalakrishnan/teams360/backend/domain/orgsyncschedule"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres/sqlcgen"
)

// OrgSyncScheduleRepository implements orgsyncschedule.Store on PostgreSQL.
type OrgSyncScheduleRepository struct {
	db      *sql.DB
	queries *sqlcgen.Queries
}

// NewOrgSyncScheduleRepository creates the store.
func NewOrgSyncScheduleRepository(db *sql.DB) *OrgSyncScheduleRepository {
	return &OrgSyncScheduleRepository{db: db, queries: sqlcgen.New(db)}
}

// Get returns the singleton schedule row (created disabled by migration 000031).
func (r *OrgSyncScheduleRepository) Get(ctx context.Context) (*orgsyncschedule.Schedule, error) {
	row, err := r.queries.GetOrgSyncSchedule(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load org sync schedule: %w", err)
	}
	s := orgsyncschedule.Schedule{
		Enabled:   row.Enabled,
		Frequency: orgsyncschedule.Frequency(row.Frequency),
		UpdatedAt: row.UpdatedAt.UTC(),
	}
	if row.NextRunAt.Valid {
		t := row.NextRunAt.Time.UTC()
		s.NextRunAt = &t
	}
	if row.UpdatedBy.Valid {
		s.UpdatedBy = &row.UpdatedBy.String
	}
	return &s, nil
}

// Save replaces the configuration.
func (r *OrgSyncScheduleRepository) Save(ctx context.Context, enabled bool, f orgsyncschedule.Frequency, nextRunAt *time.Time, updatedBy string) (*orgsyncschedule.Schedule, error) {
	var next sql.NullTime
	if enabled && nextRunAt != nil {
		next = sql.NullTime{Time: nextRunAt.UTC(), Valid: true}
	}
	err := r.queries.UpdateOrgSyncSchedule(ctx, sqlcgen.UpdateOrgSyncScheduleParams{
		Enabled:   enabled,
		Frequency: string(f),
		NextRunAt: next,
		UpdatedBy: sql.NullString{String: updatedBy, Valid: updatedBy != ""},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save org sync schedule: %w", err)
	}
	return r.Get(ctx)
}

// Claim is a compare-and-swap on next_run_at. Re-checking enabled in the WHERE
// clause means a registration that outlived a "disable" cannot launch a run.
func (r *OrgSyncScheduleRepository) Claim(ctx context.Context, due, next time.Time) (bool, error) {
	n, err := r.queries.ClaimOrgSyncSchedule(ctx, sqlcgen.ClaimOrgSyncScheduleParams{
		NextRunAt: sql.NullTime{Time: next.UTC(), Valid: true},
		Due:       sql.NullTime{Time: due.UTC(), Valid: true},
	})
	if err != nil {
		return false, fmt.Errorf("failed to claim org sync occurrence: %w", err)
	}
	return n == 1, nil
}

var _ orgsyncschedule.Store = (*OrgSyncScheduleRepository)(nil)
