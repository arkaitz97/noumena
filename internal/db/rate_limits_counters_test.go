package db

import (
	"context"
	"testing"
	"time"
)

// insertEmailLog seeds a durable email_send_log row with an explicit
// created_at so window boundaries can be exercised deterministically.
func insertEmailLog(t *testing.T, ctx context.Context, q *SQLiteQuery, id, template, toAddress string, createdAt time.Time) {
	t.Helper()
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO email_send_log (id, user_id, template, idempotency_key, status, to_address, created_at)
		 VALUES (?, NULL, ?, ?, 'sent', ?, ?)`,
		id, template, "ik-"+id, toAddress, createdAt.UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("seed email_send_log %s: %v", id, err)
	}
}

// TestCountForgotPasswordEmailsInWindow proves the counter measures actual
// sends (email_send_log rows) per address + password-reset template, so the
// token-less Google-only branch now counts toward the same cap.
func TestCountForgotPasswordEmailsInWindow(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	q := NewSQLiteQuery(database).(*SQLiteQuery)

	now := time.Now().UTC().Truncate(time.Second)
	since := now.Add(-1 * time.Hour)

	// Google-only style send: a password_reset send with NO
	// password_reset_tokens row. Must be counted.
	insertEmailLog(t, ctx, q, "fp-google", "password_reset", "target@example.com", now.Add(-30*time.Minute))
	// Normal-branch sends inside the window.
	insertEmailLog(t, ctx, q, "fp-normal-1", "password_reset", "target@example.com", now.Add(-10*time.Minute))
	insertEmailLog(t, ctx, q, "fp-normal-2", "password_reset", "target@example.com", now.Add(-50*time.Minute))
	// Boundary: exactly at `since` is inside (>=).
	insertEmailLog(t, ctx, q, "fp-boundary", "password_reset", "target@example.com", since)
	// Noise: wrong template, wrong address, and outside the window.
	insertEmailLog(t, ctx, q, "fp-wrong-template", "email_verification", "target@example.com", now.Add(-5*time.Minute))
	insertEmailLog(t, ctx, q, "fp-wrong-address", "password_reset", "other@example.com", now.Add(-5*time.Minute))
	insertEmailLog(t, ctx, q, "fp-old", "password_reset", "target@example.com", now.Add(-90*time.Minute))

	n, err := q.CountForgotPasswordEmailsInWindow(ctx, "target@example.com", since)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 4 {
		t.Fatalf("count = %d, want 4 (google-only + 2 normal + boundary)", n)
	}
}

// TestCountForgotPasswordEmailsInWindow_UTCBoundary proves a non-UTC `since`
// is normalised before comparison. A +02:00 local boundary would lexically
// sort after the stored "...Z" timestamps and wrongly exclude in-window rows.
func TestCountForgotPasswordEmailsInWindow_UTCBoundary(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	q := NewSQLiteQuery(database).(*SQLiteQuery)

	now := time.Now().UTC().Truncate(time.Second)
	insertEmailLog(t, ctx, q, "fp-utc", "password_reset", "utc@example.com", now.Add(-30*time.Minute))

	sinceLocal := now.Add(-1 * time.Hour).In(cestZone)
	n, err := q.CountForgotPasswordEmailsInWindow(ctx, "utc@example.com", sinceLocal)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("count = %d, want 1 (local-zone since must normalise to UTC)", n)
	}
}

func insertEmailChangeToken(t *testing.T, ctx context.Context, q *SQLiteQuery, id, userID, tokenHash string, createdAt time.Time, used bool) {
	t.Helper()
	var usedAt any
	if used {
		usedAt = createdAt.UTC().Format(time.RFC3339)
	}
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO email_change_tokens (id, user_id, pending_email, token_hash, expires_at, used_at, created_at)
		 VALUES (?, ?, 'pending@example.com', ?, ?, ?, ?)`,
		id, userID, tokenHash,
		createdAt.UTC().Add(time.Hour).Format(time.RFC3339), usedAt,
		createdAt.UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("seed email_change_tokens %s: %v", id, err)
	}
}

// TestCountEmailChangeResendsInWindow pins the window semantics of the new
// counter: only rows for the user, at or after `since`, count.
func TestCountEmailChangeResendsInWindow(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	q := NewSQLiteQuery(database).(*SQLiteQuery)
	if _, err := database.ExecContext(ctx,
		`INSERT INTO users (id, email) VALUES ('u-ec', 'ec@example.com')`); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := database.ExecContext(ctx,
		`INSERT INTO users (id, email) VALUES ('u-other', 'other@example.com')`); err != nil {
		t.Fatalf("seed other: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	since := now.Add(-1 * time.Hour)

	// Only one row per user may be active (used_at IS NULL) due to the
	// partial unique index, so all but one are marked used — the counter
	// counts issued tokens regardless of consumption.
	insertEmailChangeToken(t, ctx, q, "ec-1", "u-ec", "h1", now.Add(-10*time.Minute), true)
	insertEmailChangeToken(t, ctx, q, "ec-2", "u-ec", "h2", now.Add(-40*time.Minute), true)
	insertEmailChangeToken(t, ctx, q, "ec-boundary", "u-ec", "h3", since, false)
	insertEmailChangeToken(t, ctx, q, "ec-old", "u-ec", "h4", now.Add(-2*time.Hour), true)
	insertEmailChangeToken(t, ctx, q, "ec-other-user", "u-other", "h5", now.Add(-5*time.Minute), false)

	n, err := q.CountEmailChangeResendsInWindow(ctx, "u-ec", since)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 3 {
		t.Fatalf("count = %d, want 3 (2 recent + boundary; old + other user excluded)", n)
	}
}

// TestCountEmailChangeResendsInWindow_UTCBoundary proves a non-UTC `since` is
// normalised, matching the other window counters.
func TestCountEmailChangeResendsInWindow_UTCBoundary(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	q := NewSQLiteQuery(database).(*SQLiteQuery)
	if _, err := database.ExecContext(ctx,
		`INSERT INTO users (id, email) VALUES ('u-ec-utc', 'ecutc@example.com')`); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	insertEmailChangeToken(t, ctx, q, "ec-utc", "u-ec-utc", "h-utc", now.Add(-30*time.Minute), false)

	sinceLocal := now.Add(-1 * time.Hour).In(cestZone)
	n, err := q.CountEmailChangeResendsInWindow(ctx, "u-ec-utc", sinceLocal)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("count = %d, want 1 (local-zone since must normalise to UTC)", n)
	}
}
