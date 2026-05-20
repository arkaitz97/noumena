//go:build integration

// ingestion_integration_test.go — DB-backed integration tests for the
// comment-ingestion change (§1.8 idempotent ApplyMigrations, §3.4
// advisory-lock contention, §3.5 connection-close releases lock).

package db_test

import (
	"context"
	"testing"

	"noumena/internal/db"
	"noumena/internal/dbtest"
)

// Test_ApplyMigrations_Idempotent — comment-ingestion §1.8.
//
// The harness applies migrations once to a template database. Running
// db.ApplyMigrations a second time against an already-populated DB must
// be a no-op: no errors, schema unchanged.
func Test_ApplyMigrations_Idempotent(t *testing.T) {
	pool := dbtest.SetupTestDB(t)
	if pool == nil {
		return
	}
	adopt(t, pool)
	ctx := context.Background()

	// Snapshot table list before second migration run.
	before := listTables(t, ctx)

	if err := db.ApplyMigrations(ctx); err != nil {
		t.Fatalf("second ApplyMigrations failed: %v", err)
	}

	after := listTables(t, ctx)
	if len(before) != len(after) {
		t.Fatalf("table count drifted: before=%d after=%d", len(before), len(after))
	}
	for k := range before {
		if !after[k] {
			t.Errorf("table %s missing after second migration", k)
		}
	}
}

func listTables(t *testing.T, ctx context.Context) map[string]bool {
	t.Helper()
	rows, err := db.Pool.Query(ctx,
		`SELECT table_name FROM information_schema.tables
		 WHERE table_schema='public' AND table_type='BASE TABLE'`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()
	out := make(map[string]bool)
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out[n] = true
	}
	return out
}

// Test_AdvisoryLock_ContentionAndRelease — comment-ingestion §3.4 / §3.5.
//
// Two goroutines (acting as separate workers via separate connections)
// race to acquire the same advisory lock. Exactly one wins. After the
// holder's connection is released back to the pool and closed (§3.5),
// the loser can acquire the lock.
func Test_AdvisoryLock_ContentionAndRelease(t *testing.T) {
	pool := dbtest.SetupTestDB(t)
	if pool == nil {
		return
	}
	ctx := context.Background()

	const lockID int64 = 1234567

	connA, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire A: %v", err)
	}

	if _, err := connA.Exec(ctx, "SELECT pg_advisory_lock($1)", lockID); err != nil {
		connA.Release()
		t.Fatalf("A acquire: %v", err)
	}

	connB, err := pool.Acquire(ctx)
	if err != nil {
		connA.Release()
		t.Fatalf("acquire B: %v", err)
	}
	defer connB.Release()

	var got bool
	if err := connB.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", lockID).Scan(&got); err != nil {
		connA.Release()
		t.Fatalf("B try_lock: %v", err)
	}
	if got {
		connA.Release()
		t.Fatalf("expected B to fail while A holds")
	}

	// §3.5: closing the connection holding the lock releases it. We
	// simulate this by closing connA's underlying conn explicitly via
	// Hijack so the pool drops it (Release alone keeps session locks).
	hijacked := connA.Hijack()
	_ = hijacked.Close(ctx)

	// B should now be able to acquire.
	if err := connB.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", lockID).Scan(&got); err != nil {
		t.Fatalf("B retry: %v", err)
	}
	if !got {
		t.Fatalf("expected B to acquire after A's connection closed")
	}
}
