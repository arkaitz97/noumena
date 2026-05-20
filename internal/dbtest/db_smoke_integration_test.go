//go:build integration

package dbtest_test

import (
	"context"
	"testing"

	"noumena/internal/dbtest"
)

// TestHarness_Smoke is the canary verifying the harness boots, applies
// migrations, returns a working pool, and tears down via t.Cleanup.
func TestHarness_Smoke(t *testing.T) {
	pool := dbtest.SetupTestDB(t)
	if pool == nil {
		// SetupTestDB skipped because no backend is available.
		return
	}

	ctx := context.Background()
	var n int
	if err := pool.QueryRow(ctx, "SELECT 1").Scan(&n); err != nil {
		t.Fatalf("SELECT 1 failed: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1, got %d", n)
	}

	// users table exists (proves migrations ran).
	var exists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables
		                WHERE table_schema='public' AND table_name='users')`).
		Scan(&exists); err != nil {
		t.Fatalf("check users table: %v", err)
	}
	if !exists {
		t.Fatalf("users table missing — migrations did not run")
	}
}

// TestHarness_ParallelIsolation proves two parallel tests each receive
// their own database and do not see each other's rows.
func TestHarness_ParallelIsolation_A(t *testing.T) {
	t.Parallel()
	pool := dbtest.SetupTestDB(t)
	if pool == nil {
		return
	}
	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (email, password_hash) VALUES ('a@example.com', 'x')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 user, got %d (cross-test leak?)", count)
	}
}

func TestHarness_ParallelIsolation_B(t *testing.T) {
	t.Parallel()
	pool := dbtest.SetupTestDB(t)
	if pool == nil {
		return
	}
	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (email, password_hash) VALUES ('b@example.com', 'x')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 user, got %d (cross-test leak?)", count)
	}
}

// TestHarness_AdvisoryLockMultiConnection verifies the harness supports
// multi-connection semantics: pg_advisory_lock held on conn A blocks
// pg_try_advisory_lock on conn B until A's connection is released.
func TestHarness_AdvisoryLockMultiConnection(t *testing.T) {
	pool := dbtest.SetupTestDB(t)
	if pool == nil {
		return
	}
	ctx := context.Background()

	connA, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire A: %v", err)
	}
	if _, err := connA.Exec(ctx, "SELECT pg_advisory_lock(42)"); err != nil {
		connA.Release()
		t.Fatalf("lock on A: %v", err)
	}

	connB, err := pool.Acquire(ctx)
	if err != nil {
		connA.Release()
		t.Fatalf("acquire B: %v", err)
	}
	defer connB.Release()

	var got bool
	if err := connB.QueryRow(ctx, "SELECT pg_try_advisory_lock(42)").Scan(&got); err != nil {
		connA.Release()
		t.Fatalf("try_lock on B: %v", err)
	}
	if got {
		connA.Release()
		t.Fatalf("expected B to fail to acquire while A holds lock")
	}

	// Release A; B should now succeed.
	if _, err := connA.Exec(ctx, "SELECT pg_advisory_unlock(42)"); err != nil {
		connA.Release()
		t.Fatalf("unlock on A: %v", err)
	}
	connA.Release()

	if err := connB.QueryRow(ctx, "SELECT pg_try_advisory_lock(42)").Scan(&got); err != nil {
		t.Fatalf("retry try_lock on B: %v", err)
	}
	if !got {
		t.Fatalf("expected B to acquire after A released")
	}
}
