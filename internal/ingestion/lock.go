package ingestion

import "context"

// LockReleaser releases a previously acquired channel-level lock.
// For SQLite, this is a no-op since SQLite serializes writes automatically.
type LockReleaser func()

// AcquireChannelLock for SQLite is a no-op since SQLite serializes all writes.
func AcquireChannelLock(_ context.Context, _ interface{}, _ string) (bool, LockReleaser, error) {
	return true, func() {}, nil
}
