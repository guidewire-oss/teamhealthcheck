package services

import "context"

// HeldLock is a held distributed lock. Release must be safe to call more than
// once (idempotent) and must be called exactly once per successful TryLock,
// via defer, covering every exit path including panic.
type HeldLock interface {
	Release()
}

// SyncLocker serializes organization syncs across every API replica. It wraps
// the in-process admission check (OrganizationSyncService.running), not the
// other way around: TryLock is called from inside SyncWithOptions, the single
// method both a manual request and a scheduled run funnel through, so manual
// and scheduled syncs on any replica are coordinated by the same mechanism.
//
// A nil SyncLocker means "no distributed locking configured" -- tolerated for
// unit tests and for a genuinely single-instance deployment. A CONFIGURED
// locker that errors while acquiring must fail the sync closed (return the
// error) rather than let the sync proceed unprotected.
type SyncLocker interface {
	// TryLock attempts to acquire the lock without blocking. A nil, nil
	// return means the lock is held elsewhere right now -- the caller must
	// treat this exactly like ErrSyncInProgress, not retry, and not proceed.
	TryLock(ctx context.Context) (HeldLock, error)
}

// WithSyncLocker wires distributed locking into the sync service. Without it,
// only the in-process running flag admits one sync at a time -- safe for a
// single replica, NOT safe for more than one.
func WithSyncLocker(locker SyncLocker) Option {
	return func(s *OrganizationSyncService) {
		s.locker = locker
	}
}
