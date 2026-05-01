package ingestion

import (
	"context"
	"log"
	"strings"
	"time"

	"noumena/internal/db"
)

// StartWorker runs the background ingestion loop
func StartWorker(ctx context.Context, q db.DBQuery) {
	ticker := time.NewTicker(5 * time.Minute) // Check for channels needing sync every 5 minutes
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Ingestion worker stopping")
			return
		case <-ticker.C:
			processSync(ctx, q)
		}
	}
}

func processSync(ctx context.Context, q db.DBQuery) {
	// Sync channels that haven't been synced in 30 minutes
	channels, err := q.GetChannelsForSync(ctx, 30)
	if err != nil {
		log.Printf("Worker error reading channels for sync: %v", err)
		return
	}

	for _, pc := range channels {
		// Mock out DB-level distributed lock using purely channels for now 
		// (pg_try_advisory_lock implementation skipped for MVP prototype constraints)
		
		log.Printf("Syncing comments for platform channel: %s (%s)", pc.DisplayName, pc.Platform)

		var syncErr error
		if pc.Platform == "youtube" {
			syncErr = FetchAndStoreComments(ctx, q, pc)
		} else {
			log.Printf("Unsupported platform %s", pc.Platform)
			continue
		}

		if syncErr != nil {
			log.Printf("Error syncing %s: %v", pc.ID, syncErr)
			
			// Token fallback logic (token_priority increment)
			if strings.Contains(syncErr.Error(), "oauth") || strings.Contains(syncErr.Error(), "unauthorized") || strings.Contains(syncErr.Error(), "401") || strings.Contains(syncErr.Error(), "403") {
				log.Printf("Auth failure detected for %s, promoting next token.", pc.ID)
				_ = q.PromoteNextIngestionToken(ctx, pc.ID)
			}
		} else {
			_ = q.UpdateSyncTime(ctx, pc.ID)
			log.Printf("Successfully synced %s", pc.DisplayName)
		}
	}
}
