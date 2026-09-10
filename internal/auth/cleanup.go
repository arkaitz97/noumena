package auth

import (
	"context"
	"log"
	"time"

	"noumena/internal/db"
)

// StartUnverifiedAccountCleanup runs a daily job that deletes user rows
// whose email has not been verified for at least the given staleAfter
// duration (default 7 days). The job continues running until ctx is cancelled.
//
// Failures are logged and do not stop subsequent ticks; CASCADE foreign keys
// remove dependent token rows automatically.
func StartUnverifiedAccountCleanup(ctx context.Context, q db.AuthStore, staleAfter time.Duration, interval time.Duration) {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	if staleAfter <= 0 {
		staleAfter = 7 * 24 * time.Hour
	}

	go func() {
		// Run once at startup, then on every tick.
		runOnce := func() {
			n, err := q.DeleteStaleUnverifiedUsers(ctx, staleAfter)
			if err != nil {
				log.Printf("[cleanup] DeleteStaleUnverifiedUsers failed: %v", err)
				return
			}
			log.Printf("[cleanup] removed %d stale unverified user(s)", n)
		}
		runOnce()

		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				runOnce()
			}
		}
	}()
}
