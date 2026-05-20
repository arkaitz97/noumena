package ingestion

import (
	"context"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

// LockReleaser releases a previously acquired channel-level advisory lock.
// It is safe to call multiple times; subsequent calls are no-ops.
type LockReleaser func()

// DeriveLockID hashes a platform_channels.id (UUID string) into a stable
// BIGINT advisory-lock key, matching the SQL expression used by the
// ingestion-pipeline migration.
func DeriveLockID(platformChannelID string) int64 {
	sum := md5.Sum([]byte(platformChannelID))
	// First 8 bytes of the md5 digest, interpreted as int64 (network order).
	return int64(binary.BigEndian.Uint64(sum[:8]))
}

// AcquireChannelLock attempts to acquire a session-scoped pg_try_advisory_lock
// for the given channel. The lock is held on a dedicated connection that
// lives for the lifetime of the returned releaser, so the lock is never
// pulled out from underneath the caller by the pool's normal connection
// recycling. Returns acquired=false (with a noop releaser) when another
// session already holds the lock.
//
// The returned release function MUST be called when the caller is done with
// the lock (typically via defer). On context cancellation the connection is
// torn down which automatically releases the lock at the database level.
func AcquireChannelLock(ctx context.Context, pool *pgxpool.Pool, platformChannelID string) (bool, LockReleaser, error) {
	if pool == nil {
		return false, func() {}, fmt.Errorf("nil pool")
	}
	lockID := DeriveLockID(platformChannelID)

	// Acquire a dedicated connection from the pool. We MUST NOT release it
	// back to the pool until the caller is done; otherwise the session that
	// holds the advisory lock could be reused for an unrelated query and the
	// lock semantics would still hold (Postgres scopes per session) but the
	// pool may close the conn at idle and surprise-release the lock.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return false, func() {}, fmt.Errorf("acquire conn: %w", err)
	}

	var ok bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, lockID).Scan(&ok); err != nil {
		conn.Release()
		return false, func() {}, fmt.Errorf("pg_try_advisory_lock: %w", err)
	}
	if !ok {
		conn.Release()
		return false, func() {}, nil
	}

	var once sync.Once
	release := func() {
		once.Do(func() {
			// Use a background context for unlock so a cancelled job ctx
			// does not stop us from releasing the lock cleanly. If the
			// connection is dead Postgres has already released the lock
			// when the session closed.
			_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, lockID)
			conn.Release()
		})
	}
	return true, release, nil
}
