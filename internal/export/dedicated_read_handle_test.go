package export

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"noumena/internal/db"
)

// slowWriter pads the encoder output so the stream cursor stays open long
// enough to prove the writer on the other handle proceeds concurrently.
type slowWriter struct{ perRow time.Duration }

func (w *slowWriter) Write(p []byte) (int, error) {
	time.Sleep(w.perRow)
	return len(p), nil
}

// Task 2.3: a long export streamed over the dedicated query_only read
// handle must not block a concurrent write. The read handle owns its own
// connections (WAL allows one writer + many readers), so an INSERT on the
// main pool completes immediately instead of waiting for the export to
// drain. A single shared connection would hold the cursor and stall the
// write for the full stream duration — the exact failure this fixes.
func TestExportReadHandleDoesNotBlockConcurrentWrite(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"

	// Main write pool: single connection, like production (db.InitDB).
	writeDB, err := sql.Open("sqlite", dsn+"&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open write db: %v", err)
	}
	writeDB.SetMaxOpenConns(1)
	writeDB.SetMaxIdleConns(1)
	defer writeDB.Close()

	if err := db.Migrate(ctx, writeDB); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Dedicated read-only export handle (db.ExportDB equivalent).
	readDB, err := sql.Open("sqlite", dsn+"&_pragma=query_only(1)")
	if err != nil {
		t.Fatalf("open read db: %v", err)
	}
	readDB.SetMaxOpenConns(1)
	readDB.SetMaxIdleConns(1)
	defer readDB.Close()

	// query_only must reject writes — that is what makes the handle safe
	// to share with every export path.
	if _, err := readDB.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash) VALUES ('x', 'x@x.x', 'h')`); err == nil {
		t.Fatalf("query_only handle accepted a write")
	}

	q := db.NewSQLiteQuery(writeDB).(*db.SQLiteQuery)
	user, err := q.CreateUserWithPassword(ctx, "feed@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUserWithPassword: %v", err)
	}

	if _, err := writeDB.ExecContext(ctx,
		`INSERT INTO platform_channels (id, platform, platform_channel_id, display_name)
		 VALUES ('ch1', 'youtube', 'yt-1', 'Chan')`); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	if _, err := writeDB.ExecContext(ctx,
		`INSERT INTO user_channel_tracking (id, user_id, platform_channel_id, access_token_enc, refresh_token_enc)
		 VALUES ('tr1', ?, 'ch1', 'enc', 'enc')`, user.ID); err != nil {
		t.Fatalf("seed tracking: %v", err)
	}
	const n = 2000
	for i := 0; i < n; i++ {
		if _, err := writeDB.ExecContext(ctx,
			`INSERT INTO comments (id, platform_channel_id, platform_comment_id, body, posted_at, category, processed_at)
			 VALUES (?, 'ch1', ?, 'body', '2026-09-01T00:00:00Z', 'question', '2026-09-01T00:00:01Z')`,
			"c"+fmt.Sprintf("%08d", i), "pc"+fmt.Sprintf("%d", i)); err != nil {
			t.Fatalf("seed comment %d: %v", i, err)
		}
	}

	streamDone := make(chan error, 1)
	go func() {
		// Stream at a deliberately slow pace: ~2000 rows * 200µs ≈ 400ms.
		var count int64
		var serr error
		count, serr = StreamExport(ctx, readDB, user.ID, db.FeedFilters{}, "json", &slowWriter{perRow: 200 * time.Microsecond})
		// The concurrent write may or may not be captured by the cursor
		// (it inserts mid-stream); every seeded row must be present.
		if serr == nil && count < n {
			serr = &countMismatch{want: n, got: count}
		}
		streamDone <- serr
	}()

	// Give the stream a moment to open its cursor on the read handle.
	time.Sleep(20 * time.Millisecond)

	// A concurrent write must not wait for the export to finish.
	writeStart := time.Now()
	if _, err := writeDB.ExecContext(ctx,
		`INSERT INTO comments (id, platform_channel_id, platform_comment_id, body, posted_at, category, processed_at)
		 VALUES ('during', 'ch1', 'pc-during', 'body', '2026-09-01T00:00:00Z', 'question', '2026-09-01T00:00:01Z')`); err != nil {
		t.Fatalf("concurrent write failed: %v", err)
	}
	writeElapsed := time.Since(writeStart)
	if writeElapsed > 250*time.Millisecond {
		t.Fatalf("concurrent write took %v — export blocked the writer", writeElapsed)
	}

	if err := <-streamDone; err != nil {
		t.Fatalf("stream: %v", err)
	}
}

type countMismatch struct{ want, got int64 }

func (e *countMismatch) Error() string {
	return "count mismatch"
}
