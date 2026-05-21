// Command enqueue_initial_backfills enqueues a `backfill` job for every
// platform_channels row whose `last_synced_at IS NULL` and which has at
// least one active tracker. Safe to run repeatedly: EnqueueIngestionJob
// is idempotent across pending/running jobs of the same type.
//
// Usage:
//
//	DATABASE_PATH=./noumena.db go run ./scripts/enqueue_initial_backfills
package main

import (
	"context"
	"log"
	"os"
	"time"

	"noumena/internal/config"
	"noumena/internal/db"
)

func main() {
	if _, err := config.Load(false); err != nil {
		log.Fatalf("config: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = "./noumena.db"
	}
	if err := db.InitDB(dbPath); err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer db.CloseDB()

	q := db.NewSQLiteQuery(db.DB)
	sq, ok := q.(*db.SQLiteQuery)
	if !ok {
		log.Fatalf("failed to create SQLiteQuery")
	}
	channels, err := sq.GetChannelsNeedingSync(ctx, 30)
	if err != nil {
		log.Fatalf("list channels: %v", err)
	}

	enq, skip := 0, 0
	for _, ch := range channels {
		if ch.LastSyncedAt != nil {
			continue // only NULL last_synced_at (pre-existing un-backfilled)
		}
		jobID, err := sq.EnqueueIngestionJob(ctx, ch.ID, "backfill")
		if err != nil {
			log.Printf("channel=%s enqueue error: %v", ch.ID, err)
			continue
		}
		if jobID == "" {
			skip++
			continue
		}
		enq++
		log.Printf("enqueued backfill for channel=%s job=%s", ch.ID, jobID)
	}
	log.Printf("done: enqueued=%d skipped=%d", enq, skip)
}
