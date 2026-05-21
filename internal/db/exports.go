package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

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

func (q *SQLiteQuery) EnqueueExport(ctx context.Context, userID, format string, filters json.RawMessage) (*Export, error) {
	const sqlStr = `
		INSERT INTO comment_exports (id, user_id, format, filters, status)
		VALUES (?, ?, ?, ?, 'queued')
		RETURNING id, user_id, format, filters, status, row_count, file_path, error, retry_count, created_at, started_at, completed_at, expires_at
	`
	return scanExport(q.db.QueryRowContext(ctx, sqlStr, generateUUID(), userID, format, string(filters)))
}

func (q *SQLiteQuery) ClaimQueuedExport(ctx context.Context) (*Export, error) {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	const claim = `
		SELECT id
		FROM comment_exports
		WHERE status = 'queued'
		ORDER BY created_at ASC
		LIMIT 1
	`
	var id string
	if err := tx.QueryRowContext(ctx, claim).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	const upd = `
		UPDATE comment_exports
		SET status = 'processing', started_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		WHERE id = ?
		RETURNING id, user_id, format, filters, status, row_count, file_path, error, retry_count, created_at, started_at, completed_at, expires_at
	`
	exp, err := scanExport(tx.QueryRowContext(ctx, upd, id))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return exp, nil
}

func (q *SQLiteQuery) MarkExportCompleted(ctx context.Context, exportID, filePath string, rowCount int64, ttlHours int) error {
	expiresAt := time.Now().UTC().Add(time.Duration(ttlHours) * time.Hour).Format(time.RFC3339)
	const sqlStr = `
		UPDATE comment_exports
		SET status = 'completed',
		    file_path = ?,
		    row_count = ?,
		    completed_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
		    expires_at = ?
		WHERE id = ?
	`
	_, err := q.db.ExecContext(ctx, sqlStr, filePath, rowCount, expiresAt, exportID)
	return err
}

func (q *SQLiteQuery) MarkExportFailed(ctx context.Context, exportID, errMsg string, retryable bool) error {
	if retryable {
		const sqlStr = `
			UPDATE comment_exports
			SET retry_count = retry_count + 1,
			    error = ?,
			    status = CASE WHEN retry_count + 1 > 3 THEN 'failed' ELSE 'queued' END,
			    started_at = NULL
			WHERE id = ?
		`
		_, err := q.db.ExecContext(ctx, sqlStr, errMsg, exportID)
		return err
	}
	const sqlStr = `
		UPDATE comment_exports
		SET retry_count = retry_count + 1,
		    error = ?,
		    status = 'failed'
		WHERE id = ?
	`
	_, err := q.db.ExecContext(ctx, sqlStr, errMsg, exportID)
	return err
}

func (q *SQLiteQuery) GetExport(ctx context.Context, userID, exportID string) (*Export, error) {
	const sqlStr = `
		SELECT id, user_id, format, filters, status, row_count, file_path, error, retry_count, created_at, started_at, completed_at, expires_at
		FROM comment_exports
		WHERE id = ? AND user_id = ?
	`
	exp, err := scanExport(q.db.QueryRowContext(ctx, sqlStr, exportID, userID))
	if err != nil && errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return exp, err
}

func (q *SQLiteQuery) ListExpiredExports(ctx context.Context) ([]*Export, error) {
	const sqlStr = `
		SELECT id, user_id, format, filters, status, row_count, file_path, error, retry_count, created_at, started_at, completed_at, expires_at
		FROM comment_exports
		WHERE status = 'completed'
		  AND expires_at IS NOT NULL
		  AND expires_at < strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		LIMIT 1000
	`
	rows, err := q.db.QueryContext(ctx, sqlStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Export
	for rows.Next() {
		exp, err := scanExportRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, exp)
	}
	return out, rows.Err()
}

func (q *SQLiteQuery) MarkExportExpired(ctx context.Context, exportID string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE comment_exports SET status = 'expired' WHERE id = ? AND status = 'completed'`,
		exportID)
	return err
}

func (q *SQLiteQuery) ResetStuckProcessingExports(ctx context.Context, olderThan time.Duration) (int, error) {
	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339)
	result, err := q.db.ExecContext(ctx,
		`UPDATE comment_exports
		   SET status = 'queued',
		       retry_count = retry_count + 1,
		       started_at = NULL
		 WHERE status = 'processing'
		   AND started_at IS NOT NULL
		   AND started_at < ?`,
		cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

func scanExport(row *sql.Row) (*Export, error) {
	var e Export
	var filtersStr, createdAt, startedAt, completedAt, expiresAt string
	var filePath, errMsg sql.NullString
	var rowCount sql.NullInt64
	if err := row.Scan(
		&e.ID, &e.UserID, &e.Format, &filtersStr, &e.Status,
		&rowCount, &filePath, &errMsg, &e.RetryCount,
		&createdAt, &startedAt, &completedAt, &expiresAt,
	); err != nil {
		return nil, err
	}
	e.Filters = json.RawMessage(filtersStr)
	if rowCount.Valid {
		e.RowCount = &rowCount.Int64
	}
	if filePath.Valid {
		e.FilePath = &filePath.String
	}
	if errMsg.Valid {
		e.Error = &errMsg.String
	}
	e.CreatedAt = parseTime(createdAt)
	e.StartedAt = parseTimePtr(parseNullString(startedAt))
	e.CompletedAt = parseTimePtr(parseNullString(completedAt))
	e.ExpiresAt = parseTimePtr(parseNullString(expiresAt))
	return &e, nil
}

func scanExportRows(rows *sql.Rows) (*Export, error) {
	var e Export
	var filtersStr, createdAt, startedAt, completedAt, expiresAt string
	var filePath, errMsg sql.NullString
	var rowCount sql.NullInt64
	if err := rows.Scan(
		&e.ID, &e.UserID, &e.Format, &filtersStr, &e.Status,
		&rowCount, &filePath, &errMsg, &e.RetryCount,
		&createdAt, &startedAt, &completedAt, &expiresAt,
	); err != nil {
		return nil, err
	}
	e.Filters = json.RawMessage(filtersStr)
	if rowCount.Valid {
		e.RowCount = &rowCount.Int64
	}
	if filePath.Valid {
		e.FilePath = &filePath.String
	}
	if errMsg.Valid {
		e.Error = &errMsg.String
	}
	e.CreatedAt = parseTime(createdAt)
	e.StartedAt = parseTimePtr(parseNullString(startedAt))
	e.CompletedAt = parseTimePtr(parseNullString(completedAt))
	e.ExpiresAt = parseTimePtr(parseNullString(expiresAt))
	return &e, nil
}
