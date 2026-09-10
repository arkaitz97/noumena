package cronutil

import (
	"context"
	"time"
)

// UntilNextDailyUTC returns the duration from now until the next occurrence
// of (hour, minute) UTC. Useful for callers that want a once-a-day cron at
// a fixed wall-clock time.
func UntilNextDailyUTC(now time.Time, hour, minute int) time.Duration {
	now = now.UTC()
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.UTC)
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next.Sub(now)
}

// RunCron runs work on a ticker until ctx is cancelled.
// If runFirst is true, work is called once immediately before the ticker loop.
func RunCron(ctx context.Context, interval time.Duration, runFirst bool, work func(ctx context.Context)) {
	if runFirst {
		work(ctx)
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			work(ctx)
		}
	}
}
