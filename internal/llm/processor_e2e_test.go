package llm

import (
	"context"
	"fmt"
	"testing"
	"time"

	"noumena/internal/db"
	"noumena/internal/dbtest"
)

// TestProcessor_RunDrainsBacklogE2E exercises the real SQLite claim exclusion
// and worker pool end to end: a 25-comment, 3-concurrency, batch-size-5
// backlog must fully drain without stalling after the first wave.
func TestProcessor_RunDrainsBacklogE2E(t *testing.T) {
	database := dbtest.SetupTestDB(t)
	q := db.NewSQLiteQuery(database).(*db.SQLiteQuery)
	ctx := context.Background()

	user, err := q.CreateUser(ctx, fmt.Sprintf("e2e-%d@example.com", time.Now().UnixNano()), "h")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	chID, _, err := q.UpsertPlatformChannel(ctx, "youtube", fmt.Sprintf("yt-e2e-%d", time.Now().UnixNano()), "C")
	if err != nil {
		t.Fatalf("UpsertPlatformChannel: %v", err)
	}
	if _, err := q.CreateUserChannelTracking(ctx, user.ID, chID, "a", "r", time.Now().UTC().Add(time.Hour), 90, 0); err != nil {
		t.Fatalf("CreateUserChannelTracking: %v", err)
	}
	const total = 25
	for i := 0; i < total; i++ {
		if err := q.InsertComment(ctx, &db.Comment{
			PlatformChannelID: chID,
			PlatformCommentID: fmt.Sprintf("pc-%d", i),
			Body:              fmt.Sprintf("comment %d", i),
			PostedAt:          time.Now(),
		}); err != nil {
			t.Fatalf("InsertComment %d: %v", i, err)
		}
	}

	cli := &fakeClient{provider: "test", model: "m"}
	p := NewProcessor(cli, q)
	p.MaxConcurrency = 3
	p.BatchSize = 5
	p.TickInterval = 10 * time.Millisecond

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { p.Run(runCtx); close(done) }()

	deadline := time.Now().Add(8 * time.Second)
	var processed int
	for time.Now().Before(deadline) {
		if err := q.DB().QueryRowContext(ctx, `SELECT count(*) FROM comments WHERE processed_at IS NOT NULL`).Scan(&processed); err != nil {
			t.Fatalf("count: %v", err)
		}
		if processed == total {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
	if processed != total {
		t.Fatalf("processed %d/%d before stall/timeout", processed, total)
	}
}
