// Package db: email.go owns the email_send_log table operations.
//
// The store split (EmailLogStore interface in this package, *PostgresQuery
// concrete impl below) lets the email pipeline depend on a narrow surface
// while keeping the implementation alongside the rest of the data layer.
// Tests use an in-memory fake that satisfies the same interface.
package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// EmailAttemptInput is the input shape for LogEmailAttempt — packaged so
// callers don't pass eight positional args. UserID is *string because the
// foreign key is ON DELETE SET NULL: hard-deleted users keep a row with
// user_id=NULL for audit purposes.
type EmailAttemptInput struct {
	UserID          *string
	Template        string
	IdempotencyKey  string
	Status          string
	AttemptCount    int
	LastAttemptedAt *time.Time
	LastError       *string
	SentAt          *time.Time
}

// EmailLogRow is the row shape returned by ClaimPendingEmails. The retry
// worker uses these fields to compute the next backoff and to re-render
// + re-send the message (the message body is NOT persisted; only the
// idempotency key + template + user_id + attempt metadata is, so callers
// must reconstruct rendering inputs from the original trigger source).
type EmailLogRow struct {
	ID              string
	UserID          *string
	Template        string
	IdempotencyKey  string
	Status          string
	AttemptCount    int
	LastAttemptedAt *time.Time
	LastError       *string
	SentAt          *time.Time
	CreatedAt       time.Time
}

// EmailLogStore is the narrow surface the email pipeline + retry worker
// depend on. *PostgresQuery satisfies it; tests inject an in-memory fake.
type EmailLogStore interface {
	IsEmailIdempotencyKeyUsed(ctx context.Context, key string) (bool, error)
	LogEmailAttempt(ctx context.Context, in EmailAttemptInput) (string, error)
	MarkEmailSent(ctx context.Context, id string, at time.Time) error
	IncrementEmailAttempt(ctx context.Context, id string, attempt int, lastErr string) error
	MarkEmailFailed(ctx context.Context, id string, lastErr string) error
	MarkEmailAttemptError(ctx context.Context, id string, errMsg string) error
	ClaimPendingEmails(ctx context.Context, now time.Time, max int) ([]*EmailLogRow, error)
}

// IsEmailIdempotencyKeyUsed returns true when an email_send_log row with
// the given idempotency key already exists in status='sent'. Pending and
// failed rows do NOT count as used — pending may be retried, failed has
// already been declared a permanent loss and re-sending is acceptable
// (the operator manually intervened to set the status).
func (q *PostgresQuery) IsEmailIdempotencyKeyUsed(ctx context.Context, key string) (bool, error) {
	const sql = `SELECT 1 FROM email_send_log WHERE idempotency_key = $1 AND status = 'sent' LIMIT 1`
	var one int
	err := q.pool.QueryRow(ctx, sql, key).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// LogEmailAttempt inserts a new email_send_log row and returns the new id.
// The caller handles the unique-constraint race by inspecting pgconn
// error code 23505. Defaulting attempt_count to 1 here is intentional:
// the very first call IS attempt one, even on the synchronous send path.
func (q *PostgresQuery) LogEmailAttempt(ctx context.Context, in EmailAttemptInput) (string, error) {
	const sql = `
		INSERT INTO email_send_log
			(user_id, template, idempotency_key, status, attempt_count,
			 last_attempted_at, last_error, sent_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id::text
	`
	var id string
	err := q.pool.QueryRow(ctx, sql,
		in.UserID, in.Template, in.IdempotencyKey, in.Status, in.AttemptCount,
		in.LastAttemptedAt, in.LastError, in.SentAt,
	).Scan(&id)
	return id, err
}

// MarkEmailSent transitions a row to status='sent' with sent_at set.
// Idempotent on repeated calls (the SET is unconditional).
func (q *PostgresQuery) MarkEmailSent(ctx context.Context, id string, at time.Time) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE email_send_log SET status='sent', sent_at=$2, last_error=NULL WHERE id=$1`,
		id, at)
	return err
}

// IncrementEmailAttempt advances the attempt_count and updates the
// last_attempted_at + last_error fields. Used by the retry worker when
// an attempt fails but more attempts remain.
func (q *PostgresQuery) IncrementEmailAttempt(ctx context.Context, id string, attempt int, lastErr string) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE email_send_log
		 SET attempt_count=$2, last_attempted_at=NOW(), last_error=$3
		 WHERE id=$1`,
		id, attempt, lastErr)
	return err
}

// MarkEmailFailed transitions a row to status='failed' (terminal). Called
// by the retry worker after the third attempt fails.
func (q *PostgresQuery) MarkEmailFailed(ctx context.Context, id string, lastErr string) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE email_send_log SET status='failed', last_error=$2 WHERE id=$1`,
		id, lastErr)
	return err
}

// MarkEmailAttemptError leaves status=pending and writes last_error so
// the retry worker can pick it up later. Used by the synchronous Send
// path when an SMTP attempt fails.
func (q *PostgresQuery) MarkEmailAttemptError(ctx context.Context, id string, errMsg string) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE email_send_log SET last_error=$2 WHERE id=$1`,
		id, errMsg)
	return err
}

// ClaimPendingEmails returns up to `max` rows that are ripe for retry.
// "Ripe" = status='pending' AND attempt_count < 3 AND
//   last_attempted_at <= NOW() - backoff(attempt_count).
//
// We compute the per-row deadline server-side using a CASE expression so
// every backoff-tier check happens in one round-trip. Backoffs match
// design.md D11: 1m after attempt 1, 5m after attempt 2, 30m after
// attempt 3 — but attempt 3 is the last one, so the WHERE caps at < 3.
func (q *PostgresQuery) ClaimPendingEmails(ctx context.Context, now time.Time, max int) ([]*EmailLogRow, error) {
	if max <= 0 {
		max = 50
	}
	const sql = `
		SELECT id::text, user_id::text, template, idempotency_key, status,
		       attempt_count, last_attempted_at, last_error, sent_at, created_at
		FROM email_send_log
		WHERE status = 'pending'
		  AND attempt_count < 3
		  AND COALESCE(last_attempted_at, '1970-01-01'::timestamptz)
		      <= $1 - (CASE attempt_count
		                  WHEN 1 THEN INTERVAL '1 minute'
		                  WHEN 2 THEN INTERVAL '5 minutes'
		                  ELSE INTERVAL '30 minutes'
		               END)
		ORDER BY last_attempted_at NULLS FIRST
		LIMIT $2
	`
	rows, err := q.pool.Query(ctx, sql, now, max)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*EmailLogRow
	for rows.Next() {
		var r EmailLogRow
		var userID *string
		if err := rows.Scan(&r.ID, &userID, &r.Template, &r.IdempotencyKey, &r.Status,
			&r.AttemptCount, &r.LastAttemptedAt, &r.LastError, &r.SentAt, &r.CreatedAt,
		); err != nil {
			return nil, err
		}
		r.UserID = userID
		out = append(out, &r)
	}
	return out, rows.Err()
}
