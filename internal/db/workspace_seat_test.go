package db

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func seedWorkspaceWithMembers(t *testing.T, ctx context.Context, sq *SQLiteQuery, members int) {
	t.Helper()
	db := sq.DB()
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, sql, args...); err != nil {
			t.Fatalf("seed %s: %v", sql, err)
		}
	}
	mustExec(`INSERT INTO users (id, email) VALUES ('u-owner', 'owner@example.com')`)
	mustExec(`INSERT INTO workspaces (id, name, created_by) VALUES ('ws1', 'ws', 'u-owner')`)
	roles := []string{"admin", "analyst", "viewer", "viewer", "viewer", "viewer"}
	for i := 0; i < members; i++ {
		uid := "u-owner"
		if i > 0 {
			uid = "u-" + string(rune('0'+i))
			mustExec(`INSERT INTO users (id, email) VALUES (?, ?)`, uid, uid+"@example.com")
		}
		mustExec(`INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ('ws1', ?, ?)`, uid, roles[i])
	}
}

func TestAddWorkspaceMemberAtomic_AtLimitReturnsErrSeatLimit(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	sq := NewSQLiteQuery(database).(*SQLiteQuery)
	seedWorkspaceWithMembers(t, ctx, sq, 4)
	if _, err := sq.DB().ExecContext(ctx,
		`INSERT INTO users (id, email) VALUES ('u-new', 'new@example.com'), ('u-new-2', 'new2@example.com')`); err != nil {
		t.Fatalf("seed candidate users: %v", err)
	}

	err := sq.AddWorkspaceMemberAtomic(ctx, "ws1", "u-new", "viewer", 5)
	if err != nil {
		t.Fatalf("under limit: expected success, got %v", err)
	}
	err = sq.AddWorkspaceMemberAtomic(ctx, "ws1", "u-new-2", "viewer", 5)
	if !errors.Is(err, ErrSeatLimit) {
		t.Fatalf("at limit: expected ErrSeatLimit, got %v", err)
	}
	n, err := sq.CountWorkspaceMembers(ctx, "ws1")
	if err != nil {
		t.Fatalf("CountWorkspaceMembers: %v", err)
	}
	if n != 5 {
		t.Fatalf("expected 5 members, got %d", n)
	}
}

// TestAddWorkspaceMemberAtomic_ConcurrentRespectsLimit reproduces the
// check-then-act race: two admissions race for the last seat. The guarded
// single-statement insert must admit exactly one and reject the other.
func TestAddWorkspaceMemberAtomic_ConcurrentRespectsLimit(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	sq := NewSQLiteQuery(database).(*SQLiteQuery)
	seedWorkspaceWithMembers(t, ctx, sq, 4)
	if _, err := sq.DB().ExecContext(ctx,
		`INSERT INTO users (id, email) VALUES ('u-a', 'a@example.com'), ('u-b', 'b@example.com')`); err != nil {
		t.Fatalf("seed candidate users: %v", err)
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, uid := range []string{"u-a", "u-b"} {
		wg.Add(1)
		go func(i int, uid string) {
			defer wg.Done()
			errs[i] = sq.AddWorkspaceMemberAtomic(ctx, "ws1", uid, "viewer", 5)
		}(i, uid)
	}
	wg.Wait()

	var okCount, limitCount int
	for _, err := range errs {
		switch {
		case err == nil:
			okCount++
		case errors.Is(err, ErrSeatLimit):
			limitCount++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if okCount != 1 || limitCount != 1 {
		t.Fatalf("expected exactly one success and one ErrSeatLimit, got %d ok / %d limited", okCount, limitCount)
	}

	n, err := sq.CountWorkspaceMembers(ctx, "ws1")
	if err != nil {
		t.Fatalf("CountWorkspaceMembers: %v", err)
	}
	if n != 5 {
		t.Fatalf("seat cap exceeded: expected 5 members, got %d", n)
	}
}
