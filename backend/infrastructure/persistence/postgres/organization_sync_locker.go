package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
)

// advisoryLockClassID/advisoryLockOrgSync form the two-integer key for
// pg_try_advisory_lock. Explicit small ints rather than hashtext(...): they
// stay greppable and legible in pg_locks. 360 = Teams360; 1 = org sync,
// leaving room for future lock classes under the same namespace.
const (
	advisoryLockClassID = 360
	advisoryLockOrgSync = 1
	lockReleaseTimeout  = 5 * time.Second
)

// OrgSyncLocker serializes organization syncs across every API replica via a
// session-scoped Postgres advisory lock, acquired on a connection pinned for
// the lock's entire lifetime.
//
// Session-scoped and non-blocking (pg_try_advisory_lock), not
// pg_try_advisory_xact_lock: ApplySnapshot's write transaction runs only
// after fetch/filter/validate (seconds to minutes against an external HTTP
// provider), so an xact-scoped lock would leave that whole phase
// unprotected. Non-blocking because a blocking pg_advisory_lock would make
// the loser wait for an entire import; the caller is expected to treat a
// held lock as "skip and try later", not "wait".
type OrgSyncLocker struct {
	db *sql.DB
}

// NewOrgSyncLocker creates a locker bound to the given database. Use the same
// *sql.DB the rest of the application uses -- the lock's dedicated connection
// is pinned FROM this pool for the lock's duration, not a separate resource.
func NewOrgSyncLocker(db *sql.DB) *OrgSyncLocker {
	return &OrgSyncLocker{db: db}
}

// TryLock attempts to acquire the lock without blocking. A nil, nil return
// means the lock is held elsewhere right now.
//
// Connection affinity is the trap this guards against, and it fails
// silently: *sql.DB is a pool, so releasing a SESSION-scoped advisory lock
// from a DIFFERENT pooled connection than the one that acquired it does
// nothing -- Postgres merely warns and returns false, while the lock stays
// held on the original connection until it is reused and closed. Every sync
// on every replica would then be refused, with no error anywhere. The fix is
// to pin one *sql.Conn for the lock's entire lifetime and release on that
// SAME connection, never through the pool.
func (l *OrgSyncLocker) TryLock(ctx context.Context) (services.HeldLock, error) {
	conn, err := l.db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to reserve a connection for the organization sync lock: %w", err)
	}

	var acquired bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1, $2)`, advisoryLockClassID, advisoryLockOrgSync).Scan(&acquired); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to attempt the organization sync lock: %w", err)
	}
	if !acquired {
		conn.Close()
		return nil, nil
	}

	return &heldOrgSyncLock{conn: conn}, nil
}

// heldOrgSyncLock holds the pinned connection for the lock's lifetime.
type heldOrgSyncLock struct {
	conn *sql.Conn
	once sync.Once
}

// Release unlocks and closes the pinned connection. Idempotent (safe to call
// more than once) and uses a context detached from the caller's -- shutdown
// may have already cancelled that one -- with its own short timeout, so
// release is attempted even during shutdown. conn.Close() is the backstop:
// even if the explicit unlock statement fails or times out, closing the
// connection releases every session-scoped lock held on it.
func (l *heldOrgSyncLock) Release() {
	l.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), lockReleaseTimeout)
		defer cancel()
		if _, err := l.conn.ExecContext(ctx, `SELECT pg_advisory_unlock($1, $2)`, advisoryLockClassID, advisoryLockOrgSync); err != nil {
			// conn.Close() below still releases the lock regardless -- this is
			// a diagnostic log, not a recovery action.
			logger.Get().WithError(err).Warn("failed to release the organization sync advisory lock; closing its connection instead")
		}
		l.conn.Close()
	})
}
