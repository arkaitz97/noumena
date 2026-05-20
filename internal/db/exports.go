// Package db: exports.go implements the comment_exports table CRUD used by
// the insights-api export lifecycle and the background worker.
package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Export models a row in comment_exports.
type Export struct {
	ID          string          `json:"id"`
	UserID      string          `json:"user_id"`
	Format      string          `json:"format"`
	Filters     json.RawMessage `json:"filters"`
	Status      string          `json:"status"`
	RowCount    *int64          `json:"row_count,omitempty"`
	FilePath    *string         `json:"file_path,omitempty"`
	Error       *string         `json:"error,omitempty"`
	RetryCount  int             `json:"retry_count"`
	CreatedAt   time.Time       `json:"created_at"`
	StartedAt   *time.Time      `json:"started_at,omitempty"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
	ExpiresAt   *time.Time      `json:"expires_at,omitempty"`
}

// EnqueueExport inserts a comment_exports row in status='queued'. Task 4.10.
func (q *PostgresQuery) EnqueueExport(ctx context.Context, userID, format string, filters json.RawMessage) (*Export, error) {
	const sql = `
		INSERT INTO comment_exports (user_id, format, filters, status)
		VALUES ($1, $2, $3, 'queued')
		RETURNING id::text, user_id::text, format, filters::text, status, row_count, file_path, error, retry_count, created_at, started_at, completed_at, expires_at
	`
	return scanExport(q.pool.QueryRow(ctx, sql, userID, format, string(filters)))
}

// ClaimQueuedExport runs the FOR UPDATE SKIP LOCKED dance in a single Tx,
// flipping the row from queued → processing. Task 4.11.
func (q *PostgresQuery) ClaimQueuedExport(ctx context.Context) (*Export, error) {
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	const claim = `
		SELECT id
		FROM comment_exports
		WHERE status = 'queued'
		ORDER BY created_at ASC
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`
	var id string
	if err := tx.QueryRow(ctx, claim).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	const upd = `
		UPDATE comment_exports
		SET status = 'processing', started_at = NOW()
		WHERE id = $1
		RETURNING id::text, user_id::text, format, filters::text, status, row_count, file_path, error, retry_count, created_at, started_at, completed_at, expires_at
	`
	exp, err := scanExport(tx.QueryRow(ctx, upd, id))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return exp, nil
}

// MarkExportCompleted finalizes a successful export. Task 4.12.
func (q *PostgresQuery) MarkExportCompleted(ctx context.Context, exportID, filePath string, rowCount int64, ttlHours int) error {
	const sql = `
		UPDATE comment_exports
		SET status = 'completed',
		    file_path = $2,
		    row_count = $3,
		    completed_at = NOW(),
		    expires_at = NOW() + make_interval(hours => $4)
		WHERE id = $1
	`
	_, err := q.pool.Exec(ctx, sql, exportID, filePath, rowCount, ttlHours)
	return err
}

// MarkExportFailed increments retry_count and either re-queues or finalizes.
// Task 4.13. retryable=false forces a terminal 'failed' transition.
func (q *PostgresQuery) MarkExportFailed(ctx context.Context, exportID, errMsg string, retryable bool) error {
	if retryable {
		const sql = `
			UPDATE comment_exports
			SET retry_count = retry_count + 1,
			    error = $2,
			    status = CASE WHEN retry_count + 1 > 3 THEN 'failed' ELSE 'queued' END,
			    started_at = NULL
			WHERE id = $1
		`
		_, err := q.pool.Exec(ctx, sql, exportID, errMsg)
		return err
	}
	const sql = `
		UPDATE comment_exports
		SET retry_count = retry_count + 1,
		    error = $2,
		    status = 'failed'
		WHERE id = $1
	`
	_, err := q.pool.Exec(ctx, sql, exportID, errMsg)
	return err
}

// GetExport returns the export row scoped by user_id; foreign exports yield
// ErrNotFound so the handler can return 404 (NOT 403). Task 4.14.
func (q *PostgresQuery) GetExport(ctx context.Context, userID, exportID string) (*Export, error) {
	const sql = `
		SELECT id::text, user_id::text, format, filters::text, status, row_count, file_path, error, retry_count, created_at, started_at, completed_at, expires_at
		FROM comment_exports
		WHERE id = $1 AND user_id = $2
	`
	exp, err := scanExport(q.pool.QueryRow(ctx, sql, exportID, userID))
	if err != nil && errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return exp, err
}

// ListExpiredExports finds completed rows past their expires_at. Task 4.15.
func (q *PostgresQuery) ListExpiredExports(ctx context.Context) ([]*Export, error) {
	const sql = `
		SELECT id::text, user_id::text, format, filters::text, status, row_count, file_path, error, retry_count, created_at, started_at, completed_at, expires_at
		FROM comment_exports
		WHERE status = 'completed'
		  AND expires_at IS NOT NULL
		  AND expires_at < NOW()
		LIMIT 1000
	`
	rows, err := q.pool.Query(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Export
	for rows.Next() {
		exp, err := scanExport(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, exp)
	}
	return out, rows.Err()
}

// MarkExportExpired transitions a completed row → expired. Task 4.15.
func (q *PostgresQuery) MarkExportExpired(ctx context.Context, exportID string) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE comment_exports SET status = 'expired' WHERE id = $1 AND status = 'completed'`,
		exportID)
	return err
}

// ResetStuckProcessingExports re-queues processing rows older than olderThan.
// Used at worker startup to recover from prior crashes. Task 4.16.
func (q *PostgresQuery) ResetStuckProcessingExports(ctx context.Context, olderThan time.Duration) (int, error) {
	tag, err := q.pool.Exec(ctx,
		`UPDATE comment_exports
		   SET status = 'queued',
		       retry_count = retry_count + 1,
		       started_at = NULL
		 WHERE status = 'processing'
		   AND started_at IS NOT NULL
		   AND started_at < NOW() - make_interval(secs => $1)`,
		olderThan.Seconds())
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func scanExport(row pgx.Row) (*Export, error) {
	var e Export
	var filtersStr string
	if err := row.Scan(
		&e.ID, &e.UserID, &e.Format, &filtersStr, &e.Status,
		&e.RowCount, &e.FilePath, &e.Error, &e.RetryCount,
		&e.CreatedAt, &e.StartedAt, &e.CompletedAt, &e.ExpiresAt,
	); err != nil {
		return nil, err
	}
	e.Filters = json.RawMessage(filtersStr)
	return &e, nil
}

// Compile-time guard: this package depends on fmt for the (now removed)
// SET-LOCAL helper; keep the import alive for future SQL formatters.
var _ = fmt.Sprintf
