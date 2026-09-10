package db_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	_ "modernc.org/sqlite"

	"noumena/internal/db"
	"noumena/internal/dbtest"
)

// openFileLLMDB opens a temporary on-disk SQLite database migrated to the
// current schema. Unlike the in-memory harness it allows multiple connections
// to the same file, which is required to reproduce the cross-process enqueue
// race that the partial unique index guards against.
func openFileLLMDB(t *testing.T, path string, maxConns int) *sql.DB {
	t.Helper()
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open file db: %v", err)
	}
	database.SetMaxOpenConns(maxConns)
	database.SetMaxIdleConns(maxConns)
	if err := db.Migrate(context.Background(), database); err != nil {
		t.Fatalf("migrate file db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

// TestEnqueueLLMJobIfNotExists_ConcurrentSameCommentIDs reproduces the
// SELECT-then-INSERT race: two processor instances (separate pools to the same
// file) try to enqueue the identical comment set. The partial unique index
// (migration v19) plus the UNIQUE-conflict recovery in the function must make
// exactly one call report created=true, the other created=false with the
// winner's id, and leave exactly one active row.
func TestEnqueueLLMJobIfNotExists_ConcurrentSameCommentIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "llm_jobs.db")
	// Two independent pools instance the two racing processors; each pool
	// owns its own physical connection in the common case.
	database := openFileLLMDB(t, path, 2)
	other := openFileLLMDB(t, path, 1)

	q1 := db.NewSQLiteQuery(database).(*db.SQLiteQuery)
	q2 := db.NewSQLiteQuery(other).(*db.SQLiteQuery)

	const iterations = 100
	for i := 0; i < iterations; i++ {
		ids := []string{fmt.Sprintf("c-%d-a", i), fmt.Sprintf("c-%d-b", i)}
		payload, _ := json.Marshal(ids)

		start := make(chan struct{})
		type result struct {
			id      string
			created bool
			err     error
		}
		results := make([]result, 2)
		var wg sync.WaitGroup
		for idx, q := range []*db.SQLiteQuery{q1, q2} {
			wg.Add(1)
			go func(idx int, q *db.SQLiteQuery) {
				defer wg.Done()
				<-start
				id, created, err := q.EnqueueLLMJobIfNotExists(context.Background(), ids, "p")
				results[idx] = result{id: id, created: created, err: err}
			}(idx, q)
		}
		close(start)
		wg.Wait()

		var createdID string
		createdCount := 0
		for _, r := range results {
			if r.err != nil {
				t.Fatalf("iteration %d: enqueue error: %v", i, r.err)
			}
			if r.created {
				createdCount++
				createdID = r.id
			}
		}
		if createdCount != 1 {
			t.Fatalf("iteration %d: created count = %d, want exactly 1 (results=%+v)", i, createdCount, results)
		}
		for _, r := range results {
			if r.id != createdID {
				t.Fatalf("iteration %d: id %q != winner %q", i, r.id, createdID)
			}
		}

		var rows int
		if err := database.QueryRow(
			`SELECT COUNT(*) FROM llm_processing_jobs
			  WHERE comment_ids = ? AND status IN ('pending','processing')`,
			string(payload)).Scan(&rows); err != nil {
			t.Fatalf("iteration %d: count rows: %v", i, err)
		}
		if rows != 1 {
			t.Fatalf("iteration %d: active rows for payload = %d, want 1", i, rows)
		}
	}
}

// TestEnqueueLLMJobIfNotExists_ReenqueueAfterTerminal proves the index
// predicate is partial: a failed/completed row with identical comment_ids does
// not block a brand-new active job.
func TestEnqueueLLMJobIfNotExists_ReenqueueAfterTerminal(t *testing.T) {
	ctx := context.Background()
	q := db.NewSQLiteQuery(dbtest.SetupTestDB(t)).(*db.SQLiteQuery)

	ids := []string{"reterminal-a", "reterminal-b"}

	firstID, created, err := q.EnqueueLLMJobIfNotExists(ctx, ids, "p")
	if err != nil || !created {
		t.Fatalf("first enqueue: created=%v err=%v", created, err)
	}
	if err := q.MarkLLMJobFailed(ctx, firstID, "boom"); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	secondID, created2, err := q.EnqueueLLMJobIfNotExists(ctx, ids, "p")
	if err != nil {
		t.Fatalf("re-enqueue after failed: %v", err)
	}
	if !created2 {
		t.Fatalf("re-enqueue after failed must create a new job, got created=false id=%q", secondID)
	}
	if secondID == firstID {
		t.Fatalf("re-enqueue returned the terminal job id %q", secondID)
	}

	// Now complete the second and prove completion also frees the set.
	if err := q.MarkLLMJobCompleted(ctx, secondID); err != nil {
		t.Fatalf("mark completed: %v", err)
	}
	thirdID, created3, err := q.EnqueueLLMJobIfNotExists(ctx, ids, "p")
	if err != nil || !created3 {
		t.Fatalf("re-enqueue after completed: created=%v err=%v", created3, err)
	}
	if thirdID == firstID || thirdID == secondID {
		t.Fatalf("re-enqueue after completed reused a terminal id %q", thirdID)
	}
}

// TestLLMJobs_ActiveCommentIDsUniqueIndex asserts the durable DB-level
// invariant directly: two active rows with the same comment_ids are rejected,
// while a terminal row followed by a new active row is allowed.
func TestLLMJobs_ActiveCommentIDsUniqueIndex(t *testing.T) {
	database := dbtest.SetupTestDB(t)
	var n int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_llm_jobs_active_comment_ids'`).Scan(&n); err != nil {
		t.Fatalf("introspect index: %v", err)
	}
	if n != 1 {
		t.Fatalf("idx_llm_jobs_active_comment_ids missing after migration")
	}

	insert := func(id, status, commentIDs string) error {
		_, err := database.Exec(
			`INSERT INTO llm_processing_jobs (id, status, comment_ids) VALUES (?, ?, ?)`,
			id, status, commentIDs)
		return err
	}

	if err := insert("job-1", "pending", `["x"]`); err != nil {
		t.Fatalf("insert first active: %v", err)
	}
	err := insert("job-2", "processing", `["x"]`)
	if err == nil || !strings.Contains(err.Error(), "UNIQUE constraint failed") {
		t.Fatalf("second active duplicate must fail with UNIQUE constraint, got %v", err)
	}

	// A terminal row is not covered by the partial predicate, so a new active
	// row for the same set is permitted.
	if _, err := database.Exec(`UPDATE llm_processing_jobs SET status='failed' WHERE id='job-1'`); err != nil {
		t.Fatalf("mark terminal: %v", err)
	}
	if err := insert("job-3", "pending", `["x"]`); err != nil {
		t.Fatalf("insert after terminal: %v", err)
	}
}
