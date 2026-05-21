package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

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

type EmailLogStore interface {
	IsEmailIdempotencyKeyUsed(ctx context.Context, key string) (bool, error)
	LogEmailAttempt(ctx context.Context, in EmailAttemptInput) (string, error)
	MarkEmailSent(ctx context.Context, id string, at time.Time) error
	IncrementEmailAttempt(ctx context.Context, id string, attempt int, lastErr string) error
	MarkEmailFailed(ctx context.Context, id string, lastErr string) error
	MarkEmailAttemptError(ctx context.Context, id string, errMsg string) error
	ClaimPendingEmails(ctx context.Context, now time.Time, max int) ([]*EmailLogRow, error)
}

func (q *SQLiteQuery) IsEmailIdempotencyKeyUsed(ctx context.Context, key string) (bool, error) {
	const sqlStr = `SELECT 1 FROM email_send_log WHERE idempotency_key = ? AND status = 'sent' LIMIT 1`
	var one int
	err := q.db.QueryRowContext(ctx, sqlStr, key).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (q *SQLiteQuery) LogEmailAttempt(ctx context.Context, in EmailAttemptInput) (string, error) {
	const sqlStr = `
		INSERT INTO email_send_log
			(id, user_id, template, idempotency_key, status, attempt_count,
			 last_attempted_at, last_error, sent_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`
	var lastAttemptedAt, lastError, sentAt string
	if in.LastAttemptedAt != nil {
		lastAttemptedAt = in.LastAttemptedAt.Format(time.RFC3339)
	}
	if in.LastError != nil {
		lastError = *in.LastError
	}
	if in.SentAt != nil {
		sentAt = in.SentAt.Format(time.RFC3339)
	}
	var userID interface{} = in.UserID
	if in.UserID != nil {
		userID = *in.UserID
	}
	var id string
	err := q.db.QueryRowContext(ctx, sqlStr,
		generateUUID(), userID, in.Template, in.IdempotencyKey, in.Status, in.AttemptCount,
		lastAttemptedAt, lastError, sentAt,
	).Scan(&id)
	return id, err
}

func (q *SQLiteQuery) MarkEmailSent(ctx context.Context, id string, at time.Time) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE email_send_log SET status='sent', sent_at=?, last_error=NULL WHERE id=?`,
		at.Format(time.RFC3339), id)
	return err
}

func (q *SQLiteQuery) IncrementEmailAttempt(ctx context.Context, id string, attempt int, lastErr string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE email_send_log
		 SET attempt_count=?, last_attempted_at=strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), last_error=?
		 WHERE id=?`,
		id, attempt, lastErr)
	return err
}

func (q *SQLiteQuery) MarkEmailFailed(ctx context.Context, id string, lastErr string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE email_send_log SET status='failed', last_error=? WHERE id=?`,
		lastErr, id)
	return err
}

func (q *SQLiteQuery) MarkEmailAttemptError(ctx context.Context, id string, errMsg string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE email_send_log SET last_error=? WHERE id=?`,
		errMsg, id)
	return err
}

func (q *SQLiteQuery) ClaimPendingEmails(ctx context.Context, now time.Time, max int) ([]*EmailLogRow, error) {
	if max <= 0 {
		max = 50
	}
	// Compute backoff deadlines in Go
	deadline1 := now.Add(-1 * time.Minute)
	deadline2 := now.Add(-5 * time.Minute)
	deadline3 := now.Add(-30 * time.Minute)

	const sqlStr = `
		SELECT id, user_id, template, idempotency_key, status,
		       attempt_count, last_attempted_at, last_error, sent_at, created_at
		FROM email_send_log
		WHERE status = 'pending'
		  AND attempt_count < 3
		  AND (
		      (attempt_count = 1 AND COALESCE(last_attempted_at, '1970-01-01T00:00:00Z') <= ?)
		   OR (attempt_count = 2 AND COALESCE(last_attempted_at, '1970-01-01T00:00:00Z') <= ?)
		   OR (attempt_count >= 3 AND COALESCE(last_attempted_at, '1970-01-01T00:00:00Z') <= ?)
		  )
		ORDER BY last_attempted_at ASC
		LIMIT ?
	`
	rows, err := q.db.QueryContext(ctx, sqlStr,
		deadline1.Format(time.RFC3339),
		deadline2.Format(time.RFC3339),
		deadline3.Format(time.RFC3339),
		max)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*EmailLogRow
	for rows.Next() {
		var r EmailLogRow
		var userID, lastAttemptedAt, lastError, sentAt, createdAt string
		if err := rows.Scan(&r.ID, &userID, &r.Template, &r.IdempotencyKey, &r.Status,
			&r.AttemptCount, &lastAttemptedAt, &lastError, &sentAt, &createdAt,
		); err != nil {
			return nil, err
		}
		if userID != "" {
			r.UserID = &userID
		}
		if lastAttemptedAt != "" {
			t, _ := time.Parse(time.RFC3339, lastAttemptedAt)
			r.LastAttemptedAt = &t
		}
		if lastError != "" {
			r.LastError = &lastError
		}
		if sentAt != "" {
			t, _ := time.Parse(time.RFC3339, sentAt)
			r.SentAt = &t
		}
		r.CreatedAt = parseTime(createdAt)
		out = append(out, &r)
	}
	return out, rows.Err()
}
