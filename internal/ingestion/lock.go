package ingestion

import (
	"context"
	"sync"
)

// LockReleaser releases a previously acquired channel-level lock.
// For SQLite, this is a no-op since SQLite serializes writes automatically.
type LockReleaser func()

// AcquireChannelLock for SQLite is a no-op since SQLite serializes all writes.
// Returns acquired=true with a noop releaser.
func AcquireChannelLock(ctx context.Context, db interface{}, platformChannelID string) (bool, LockReleaser, error) {
	var once sync.Once
	release := func() {
		once.Do(func() {})
	}
	return true, release, nil
}
