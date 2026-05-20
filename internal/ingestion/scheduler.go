package ingestion

import (
	"context"
	"log"
	"time"

	"noumena/internal/db"
)

// SchedulerDB is the narrow DB surface the scheduler depends on.
type SchedulerDB interface {
	GetChannelsNeedingSync(ctx context.Context, intervalMinutes int) ([]*db.PlatformChannel, error)
	EnqueueIngestionJob(ctx context.Context, platformChannelID, jobType string) (string, error)
}

// Scheduler enqueues ingestion jobs for channels that are due. It owns no
// goroutines itself — the worker loop drives it via Tick(ctx).
type Scheduler struct {
	DB              SchedulerDB
	PollIntervalMin int // minutes between consecutive polls per channel
}

// NewScheduler returns a Scheduler with the given poll interval.
func NewScheduler(db SchedulerDB, pollIntervalMinutes int) *Scheduler {
	if pollIntervalMinutes <= 0 {
		pollIntervalMinutes = 30
	}
	return &Scheduler{DB: db, PollIntervalMin: pollIntervalMinutes}
}

// Tick scans for channels needing sync and enqueues jobs idempotently.
// Returns counts for observability/testing.
type TickResult struct {
	Backfills int
	Polls     int
	Skipped   int
}

func (s *Scheduler) Tick(ctx context.Context) (TickResult, error) {
	res := TickResult{}
	channels, err := s.DB.GetChannelsNeedingSync(ctx, s.PollIntervalMin)
	if err != nil {
		return res, err
	}
	for _, ch := range channels {
		jobType := "poll"
		if ch.LastSyncedAt == nil {
			jobType = "backfill"
		}
		jobID, err := s.DB.EnqueueIngestionJob(ctx, ch.ID, jobType)
		if err != nil {
			log.Printf("scheduler: enqueue %s for channel=%s failed: %v", jobType, ch.ID, err)
			continue
		}
		if jobID == "" {
			// Idempotent: a pending/running job already exists.
			res.Skipped++
			continue
		}
		if jobType == "backfill" {
			res.Backfills++
		} else {
			res.Polls++
		}
	}
	return res, nil
}

// Run drives Tick on a fixed interval until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context, tickInterval time.Duration) {
	if tickInterval <= 0 {
		tickInterval = 30 * time.Second
	}
	t := time.NewTicker(tickInterval)
	defer t.Stop()
	// Run one tick immediately on start.
	if _, err := s.Tick(ctx); err != nil && ctx.Err() == nil {
		log.Printf("scheduler: tick error: %v", err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := s.Tick(ctx); err != nil && ctx.Err() == nil {
				log.Printf("scheduler: tick error: %v", err)
			}
		}
	}
}
