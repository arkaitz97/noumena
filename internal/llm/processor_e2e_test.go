package llm

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
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

// syncWriter captures log output from concurrent goroutines.
type syncWriter struct {
	mu    sync.Mutex
	lines []string
}

func (w *syncWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lines = append(w.lines, string(p))
	return len(p), nil
}

func (f *fakeClient) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (w *syncWriter) count(substr string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := 0
	for _, l := range w.lines {
		if strings.Contains(l, substr) {
			n++
		}
	}
	return n
}

// 5.1 — Default concurrency (4) on a multi-channel backlog must drain fully
// with zero `event=batch_deduped` lines: the durable claim excludes in-flight
// comments, so concurrent workers never reserve overlapping sets.
func TestProcessor_DefaultConcurrencyNoDedupeSpike(t *testing.T) {
	database := dbtest.SetupTestDB(t)
	q := db.NewSQLiteQuery(database).(*db.SQLiteQuery)
	ctx := context.Background()

	user, err := q.CreateUser(ctx, fmt.Sprintf("dedupe-%d@example.com", time.Now().UnixNano()), "h")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	const channels, perChannel = 5, 4
	total := channels * perChannel
	for c := 0; c < channels; c++ {
		suffix := fmt.Sprintf("%d-%d", time.Now().UnixNano(), c)
		chID, _, err := q.UpsertPlatformChannel(ctx, "youtube", "yt-"+suffix, "C")
		if err != nil {
			t.Fatalf("UpsertPlatformChannel: %v", err)
		}
		if _, err := q.CreateUserChannelTracking(ctx, user.ID, chID, "a", "r", time.Now().UTC().Add(time.Hour), 90, 0); err != nil {
			t.Fatalf("CreateUserChannelTracking: %v", err)
		}
		for i := 0; i < perChannel; i++ {
			if err := q.InsertComment(ctx, &db.Comment{
				PlatformChannelID: chID,
				PlatformCommentID: fmt.Sprintf("pc-%s-%d", suffix, i),
				Body:              fmt.Sprintf("comment %d", i),
				PostedAt:          time.Now(),
			}); err != nil {
				t.Fatalf("InsertComment: %v", err)
			}
		}
	}

	cli := &fakeClient{provider: "test", model: "m"}
	p := NewProcessor(cli, q) // default MaxConcurrency = 4
	p.BatchSize = 2           // force multiple reclaim waves per channel
	p.TickInterval = 10 * time.Millisecond

	w := &syncWriter{}
	oldOut := log.Default().Writer()
	log.SetOutput(w)
	defer log.SetOutput(oldOut)

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
		t.Fatalf("processed %d/%d before timeout", processed, total)
	}
	if n := w.count("event=batch_deduped"); n != 0 {
		t.Fatalf("batch_deduped spike at default concurrency: %d events", n)
	}
	var starts int
	if err := q.DB().QueryRowContext(ctx, `SELECT count(*) FROM llm_processing_jobs`).Scan(&starts); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if got := w.count("event=batch_started"); got != starts {
		t.Fatalf("batch_started log count %d != job rows %d", got, starts)
	}
	if cli.callCount() != starts {
		t.Fatalf("LLM calls %d != job rows %d (duplicate classification)", cli.callCount(), starts)
	}
}
