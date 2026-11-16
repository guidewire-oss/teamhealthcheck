// Package orgsyncschedule models the automatic organization-sync schedule:
// which frequency is configured, when the next occurrence is due, and how that
// occurrence is computed.
//
// All schedule times are UTC. Daily fires at 02:00 UTC, weekly on Monday at
// 02:00 UTC, monthly on the 1st of each calendar month at 02:00 UTC.
package orgsyncschedule

import (
	"context"
	"errors"
	"time"

	"github.com/agopalakrishnan/teams360/backend/domain/orgsyncrun"
)

// Timezone is the zone every occurrence is computed and displayed in.
const Timezone = "UTC"

// HourUTC is the hour of day (UTC) at which every frequency fires.
const HourUTC = 2

// MissedGrace is how late an occurrence may be claimed and still run. A due
// occurrence older than this (the service was down) is skipped, not replayed.
const MissedGrace = 15 * time.Minute

// Frequency is how often an automatic sync runs.
type Frequency string

const (
	Daily   Frequency = "DAILY"
	Weekly  Frequency = "WEEKLY"
	Monthly Frequency = "MONTHLY"
)

// ErrInvalidFrequency is returned for any frequency other than the three above.
var ErrInvalidFrequency = errors.New("frequency must be one of DAILY, WEEKLY, MONTHLY")

// Valid reports whether f is a supported frequency.
func (f Frequency) Valid() bool { return f == Daily || f == Weekly || f == Monthly }

// Description is the human-readable schedule, always naming the timezone.
func (f Frequency) Description() string {
	switch f {
	case Daily:
		return "Daily at 02:00 UTC"
	case Weekly:
		return "Weekly on Monday at 02:00 UTC"
	case Monthly:
		return "Monthly on the 1st at 02:00 UTC"
	}
	return ""
}

// NextAfter returns the first occurrence strictly after t, in UTC.
func NextAfter(f Frequency, t time.Time) (time.Time, error) {
	t = t.UTC()
	day := time.Date(t.Year(), t.Month(), t.Day(), HourUTC, 0, 0, 0, time.UTC)
	switch f {
	case Daily:
		if day.After(t) {
			return day, nil
		}
		return day.AddDate(0, 0, 1), nil
	case Weekly:
		// Days until Monday (0 when today is Monday).
		d := (int(time.Monday) - int(day.Weekday()) + 7) % 7
		next := day.AddDate(0, 0, d)
		if next.After(t) {
			return next, nil
		}
		return next.AddDate(0, 0, 7), nil
	case Monthly:
		first := time.Date(t.Year(), t.Month(), 1, HourUTC, 0, 0, 0, time.UTC)
		if first.After(t) {
			return first, nil
		}
		// Date normalises month 13 into January of the next year.
		return time.Date(t.Year(), t.Month()+1, 1, HourUTC, 0, 0, 0, time.UTC), nil
	}
	return time.Time{}, ErrInvalidFrequency
}

// Schedule is the persisted configuration.
type Schedule struct {
	Enabled   bool
	Frequency Frequency
	NextRunAt *time.Time // nil when disabled
	UpdatedAt time.Time
	UpdatedBy *string
}

// Store persists the single schedule row.
type Store interface {
	Get(ctx context.Context) (*Schedule, error)
	// Save replaces the configuration. nextRunAt must be nil when disabled.
	Save(ctx context.Context, enabled bool, f Frequency, nextRunAt *time.Time, updatedBy string) (*Schedule, error)
	// Claim atomically moves next_run_at from due to next, but only while the
	// schedule is still enabled and still due at exactly that occurrence. It
	// returns false when another replica claimed it first or the schedule was
	// changed or disabled since it was read.
	Claim(ctx context.Context, due, next time.Time) (bool, error)
}

// RunHistory reads automatic-run history from the shared run table.
type RunHistory interface {
	// LatestByTrigger returns the newest run with that trigger, or nil, nil.
	LatestByTrigger(ctx context.Context, t orgsyncrun.Trigger) (*orgsyncrun.Run, error)
	// LatestSuccessByTrigger returns the newest SUCCESS run with that trigger, or nil, nil.
	LatestSuccessByTrigger(ctx context.Context, t orgsyncrun.Trigger) (*orgsyncrun.Run, error)
}
