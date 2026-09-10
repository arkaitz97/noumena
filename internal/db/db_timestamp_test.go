package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
)

// cestZone is a fixed +02:00 zone used to model any process east of UTC,
// where a caller-supplied local time formatted with time.RFC3339 yields a
// "+02:00" offset that sorts lexically after a UTC "...Z" boundary.
var cestZone = time.FixedZone("CEST", 2*3600)

func seedTimestampUserAndWorkspace(t *testing.T, ctx context.Context, database *sql.DB) string {
	t.Helper()
	if _, err := database.ExecContext(ctx,
		`INSERT INTO users (id, email) VALUES ('u-ts', 'ts@example.com')`); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := database.ExecContext(ctx,
		`INSERT INTO workspaces (id, name, created_by) VALUES ('ws-ts', 'ws', 'u-ts')`); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	return "u-ts"
}

func assertStoredTimestampUTC(t *testing.T, database *sql.DB, query, arg string, want time.Time) {
	t.Helper()
	var got string
	if err := database.QueryRowContext(context.Background(), query, arg).Scan(&got); err != nil {
		t.Fatalf("read stored timestamp: %v", err)
	}
	if !strings.HasSuffix(got, "Z") {
		t.Fatalf("stored timestamp %q does not end in Z (not UTC)", got)
	}
	parsed, err := time.Parse(time.RFC3339, got)
	if err != nil {
		t.Fatalf("parse stored timestamp %q: %v", got, err)
	}
	if !parsed.Equal(want) {
		t.Fatalf("stored timestamp %q represents %s, want %s", got, parsed, want)
	}
}

// TestTimestampWritersStoreUTC proves that a caller-supplied expiry in a
// non-UTC zone is persisted as the same instant with a trailing "Z", so it
// compares correctly against strftime('%Y-%m-%dT%H:%M:%SZ','now') readers.
func TestTimestampWritersStoreUTC(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	sq := NewSQLiteQuery(database).(*SQLiteQuery)
	userID := seedTimestampUserAndWorkspace(t, ctx, database)

	// 14:00 in +02:00 is 12:00 UTC.
	localExpiry := time.Date(2026, 9, 10, 14, 0, 0, 0, cestZone)
	wantUTC := localExpiry.UTC()

	if err := sq.InsertEmailVerificationToken(ctx, userID, "vh-utc", localExpiry); err != nil {
		t.Fatalf("InsertEmailVerificationToken: %v", err)
	}
	assertStoredTimestampUTC(t, database,
		`SELECT expires_at FROM email_verification_tokens WHERE token_hash = ?`, "vh-utc", wantUTC)

	if err := sq.InsertEmailChangeToken(ctx, userID, "new@example.com", "ch-utc", localExpiry); err != nil {
		t.Fatalf("InsertEmailChangeToken: %v", err)
	}
	assertStoredTimestampUTC(t, database,
		`SELECT expires_at FROM email_change_tokens WHERE token_hash = ?`, "ch-utc", wantUTC)

	if _, err := sq.CreateWorkspaceInvite(ctx, "ws-ts", "invitee@example.com", "viewer", "iv-utc", localExpiry); err != nil {
		t.Fatalf("CreateWorkspaceInvite: %v", err)
	}
	assertStoredTimestampUTC(t, database,
		`SELECT expires_at FROM workspace_invites WHERE token_hash = ?`, "iv-utc", wantUTC)
}

// TestConsumeEmailVerificationTokenRejectsLocalZoneExpired is the behavioural
// regression: before the fix a token already expired (now-1h) but expressed in
// a +02:00 zone serialised to a wall-clock time still ahead of UTC "now", so
// the lexical `expires_at > strftime(...Z)` guard wrongly accepted it.
func TestConsumeEmailVerificationTokenRejectsLocalZoneExpired(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	sq := NewSQLiteQuery(database).(*SQLiteQuery)
	userID := seedTimestampUserAndWorkspace(t, ctx, database)

	expired := time.Now().In(cestZone).Add(-time.Hour)
	if err := sq.InsertEmailVerificationToken(ctx, userID, "expired-hash", expired); err != nil {
		t.Fatalf("insert expired token: %v", err)
	}
	if _, err := sq.ConsumeEmailVerificationToken(ctx, "expired-hash"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired token: want ErrNotFound, got %v", err)
	}

	valid := time.Now().In(cestZone).Add(time.Hour)
	if err := sq.InsertEmailVerificationToken(ctx, userID, "valid-hash", valid); err != nil {
		t.Fatalf("insert valid token: %v", err)
	}
	got, err := sq.ConsumeEmailVerificationToken(ctx, "valid-hash")
	if err != nil {
		t.Fatalf("valid token: want success, got %v", err)
	}
	if got != userID {
		t.Fatalf("valid token returned user %q, want %q", got, userID)
	}
}

// TestRateLimitWindowKeysStoredUTC proves window_start is normalised to UTC so
// the write (IncrementRateLimit), read (GetRateLimitStatus) and cleanup
// (CleanupExpiredRateLimitBuckets) paths all agree on the same bucket key even
// when callers pass a non-UTC window boundary.
func TestRateLimitWindowKeysStoredUTC(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	sq := NewSQLiteQuery(database).(*SQLiteQuery)

	const userKey = "rl-user"
	windowStart := time.Now().In(cestZone).Truncate(time.Minute)

	n, err := sq.IncrementRateLimit(ctx, userKey, windowStart, 5)
	if err != nil {
		t.Fatalf("IncrementRateLimit: %v", err)
	}
	if n != 1 {
		t.Fatalf("first increment: want 1 got %d", n)
	}

	var stored string
	if err := database.QueryRowContext(ctx,
		`SELECT window_start FROM rate_limit_buckets WHERE user_key = ?`, userKey).Scan(&stored); err != nil {
		t.Fatalf("read window_start: %v", err)
	}
	if !strings.HasSuffix(stored, "Z") {
		t.Fatalf("stored window_start %q does not end in Z (not UTC)", stored)
	}
	parsed, err := time.Parse(time.RFC3339, stored)
	if err != nil {
		t.Fatalf("parse window_start %q: %v", stored, err)
	}
	if !parsed.Equal(windowStart) {
		t.Fatalf("window_start %q represents %s, want %s", stored, parsed, windowStart)
	}

	status, err := sq.GetRateLimitStatus(ctx, userKey, windowStart, 5)
	if err != nil {
		t.Fatalf("GetRateLimitStatus: %v", err)
	}
	if status.Remaining != 4 {
		t.Fatalf("read path did not match bucket: want remaining 4, got %d", status.Remaining)
	}

	deleted, err := sq.CleanupExpiredRateLimitBuckets(ctx, windowStart.Add(time.Minute).In(cestZone))
	if err != nil {
		t.Fatalf("CleanupExpiredRateLimitBuckets: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("cleanup did not match bucket: want 1 deleted, got %d", deleted)
	}
}
