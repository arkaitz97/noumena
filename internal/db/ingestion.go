package db

import (
	"context"
	"database/sql"
	"errors"
	"math/rand"
	"time"
)

func (q *SQLiteQuery) GetChannelsNeedingSync(ctx context.Context, intervalMinutes int) ([]*PlatformChannel, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(intervalMinutes) * time.Minute).Format(time.RFC3339)
	rows, err := q.db.QueryContext(ctx,
		`SELECT pc.id, pc.platform, pc.platform_channel_id, pc.display_name,
		        pc.last_synced_at, pc.deletion_scheduled_at, pc.created_at,
		        pc.ingestion_error_count,
		        pc.ingestion_last_error_at, pc.ingestion_backoff_until
		   FROM platform_channels pc
		  WHERE (pc.last_synced_at IS NULL
		         OR pc.last_synced_at < ?)
		    AND (pc.ingestion_backoff_until IS NULL
		         OR pc.ingestion_backoff_until < strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
		    AND EXISTS (
		         SELECT 1 FROM user_channel_tracking uct
		          WHERE uct.platform_channel_id = pc.id
		            AND uct.tracking_status = 'active'
		    )`,
		cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*PlatformChannel
	for rows.Next() {
		var pc PlatformChannel
		var lastSyncedAt, deletionScheduledAt, createdAt, ingestionLastErrorAt, ingestionBackoffUntil sql.NullString
		if err := rows.Scan(
			&pc.ID, &pc.Platform, &pc.PlatformChannelID, &pc.DisplayName,
			&lastSyncedAt, &deletionScheduledAt, &createdAt,
			&pc.IngestionErrorCount,
			&ingestionLastErrorAt, &ingestionBackoffUntil,
		); err != nil {
			return nil, err
		}
		pc.LastSyncedAt = parseTimePtr(lastSyncedAt)
		pc.DeletionScheduledAt = parseTimePtr(deletionScheduledAt)
		pc.CreatedAt = parseTime(createdAt.String)
		pc.IngestionLastErrorAt = parseTimePtr(ingestionLastErrorAt)
		pc.IngestionBackoffUntil = parseTimePtr(ingestionBackoffUntil)
		out = append(out, &pc)
	}
	return out, rows.Err()
}

func (q *SQLiteQuery) GetPlatformChannelByID(ctx context.Context, platformChannelID string) (*PlatformChannel, error) {
	pc := &PlatformChannel{}
	var lastSyncedAt, deletionScheduledAt, createdAt, ingestionLastErrorAt, ingestionBackoffUntil sql.NullString
	err := q.db.QueryRowContext(ctx,
		`SELECT id, platform, platform_channel_id, display_name,
		        last_synced_at, deletion_scheduled_at, created_at,
		        ingestion_error_count,
		        ingestion_last_error_at, ingestion_backoff_until
		   FROM platform_channels WHERE id = ?`,
		platformChannelID).Scan(
		&pc.ID, &pc.Platform, &pc.PlatformChannelID, &pc.DisplayName,
		&lastSyncedAt, &deletionScheduledAt, &createdAt,
		&pc.IngestionErrorCount,
		&ingestionLastErrorAt, &ingestionBackoffUntil,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	pc.LastSyncedAt = parseTimePtr(lastSyncedAt)
	pc.DeletionScheduledAt = parseTimePtr(deletionScheduledAt)
	pc.CreatedAt = parseTime(createdAt.String)
	pc.IngestionLastErrorAt = parseTimePtr(ingestionLastErrorAt)
	pc.IngestionBackoffUntil = parseTimePtr(ingestionBackoffUntil)
	return pc, nil
}

func (q *SQLiteQuery) UpdateIngestionError(ctx context.Context, platformChannelID, errMsg string, retryAfter time.Duration) error {
	until := time.Now().UTC().Add(retryAfter).Format(time.RFC3339)
	_, err := q.db.ExecContext(ctx,
		`UPDATE platform_channels
		    SET ingestion_error_count = ingestion_error_count + 1,
		        ingestion_last_error_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
		        ingestion_backoff_until = ?
		  WHERE id = ?`,
		until, platformChannelID)
	_ = errMsg
	return err
}

func (q *SQLiteQuery) ResetIngestionError(ctx context.Context, platformChannelID string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE platform_channels
		    SET ingestion_error_count = 0,
		        ingestion_last_error_at = NULL,
		        ingestion_backoff_until = NULL
		  WHERE id = ?`,
		platformChannelID)
	return err
}

func (q *SQLiteQuery) MarkChannelBackoff(ctx context.Context, platformChannelID string, until time.Time) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE platform_channels SET ingestion_backoff_until = ? WHERE id = ?`,
		until.Format(time.RFC3339), platformChannelID)
	return err
}

func (q *SQLiteQuery) UpdateLastSyncedAt(ctx context.Context, platformChannelID string, t time.Time) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE platform_channels SET last_synced_at = ? WHERE id = ?`,
		t.Format(time.RFC3339), platformChannelID)
	return err
}

func (q *SQLiteQuery) UpdateLastSyncedAtIfNull(ctx context.Context, platformChannelID string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE platform_channels SET last_synced_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		  WHERE id = ? AND last_synced_at IS NULL`,
		platformChannelID)
	return err
}

const ingestionJobColumns = `id, platform_channel_id, job_type, status,
	retry_count, scheduled_at, started_at, completed_at, error_message, created_at`

func scanIngestionJob(row *sql.Row) (*IngestionJob, error) {
	j := &IngestionJob{}
	var scheduledAt, createdAt string
	var startedAt, completedAt sql.NullString
	err := row.Scan(
		&j.ID, &j.PlatformChannelID, &j.JobType, &j.Status,
		&j.RetryCount, &scheduledAt, &startedAt, &completedAt,
		&j.ErrorMessage, &createdAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	j.ScheduledAt = parseTime(scheduledAt)
	j.StartedAt = parseTimePtr(startedAt)
	j.CompletedAt = parseTimePtr(completedAt)
	j.CreatedAt = parseTime(createdAt)
	return j, nil
}

func (q *SQLiteQuery) EnqueueIngestionJob(ctx context.Context, platformChannelID, jobType string) (string, error) {
	var id string
	err := q.db.QueryRowContext(ctx,
		`INSERT INTO ingestion_jobs (id, platform_channel_id, job_type, status, scheduled_at)
		 SELECT ?, ?, ?, 'pending', strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		  WHERE NOT EXISTS (
		     SELECT 1 FROM ingestion_jobs
		      WHERE platform_channel_id = ?
		        AND job_type = ?
		        AND status IN ('pending', 'running')
		 )
		 RETURNING id`,
		generateUUID(), platformChannelID, jobType, platformChannelID, jobType).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return id, nil
}

// ClaimNextIngestionJob uses atomic UPDATE-based claiming (no FOR UPDATE SKIP LOCKED)
func (q *SQLiteQuery) ClaimNextIngestionJob(ctx context.Context) (*IngestionJob, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	row := q.db.QueryRowContext(ctx,
		`UPDATE ingestion_jobs
		    SET status = 'running', started_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		  WHERE id = (
		     SELECT id FROM ingestion_jobs
		      WHERE status = 'pending' AND scheduled_at <= ?
		      ORDER BY scheduled_at
		      LIMIT 1
		  )
		 RETURNING `+ingestionJobColumns, now)
	return scanIngestionJob(row)
}

func (q *SQLiteQuery) MarkJobCompleted(ctx context.Context, jobID string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE ingestion_jobs
		    SET status = 'completed', completed_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), error_message = NULL
		  WHERE id = ?`,
		jobID)
	return err
}

func (q *SQLiteQuery) MarkJobFailed(ctx context.Context, jobID, errMsg string, currentRetryCount int) error {
	delaySecs := 1 << currentRetryCount
	if delaySecs < 1 {
		delaySecs = 1
	}
	if delaySecs > 16 {
		delaySecs = 16
	}
	// Jitter: +[0,delaySecs]s so bursty channels don't retry in lockstep.
	delaySecs += rand.Intn(delaySecs + 1)
	scheduledAt := time.Now().UTC().Add(time.Duration(delaySecs) * time.Second).Format(time.RFC3339)
	_, err := q.db.ExecContext(ctx,
		`UPDATE ingestion_jobs
		    SET status = 'pending',
		        retry_count = retry_count + 1,
		        scheduled_at = ?,
		        error_message = ?,
		        started_at = NULL
		  WHERE id = ?`,
		scheduledAt, errMsg, jobID)
	return err
}

func (q *SQLiteQuery) MarkJobDeadLettered(ctx context.Context, jobID, errMsg string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE ingestion_jobs
		    SET status = 'dead_lettered',
		        completed_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
		        error_message = ?
		  WHERE id = ?`,
		errMsg, jobID)
	return err
}

func (q *SQLiteQuery) ReleaseJobToPending(ctx context.Context, jobID string, delay time.Duration) error {
	secs := int64(delay.Seconds())
	if secs < 0 {
		secs = 0
	}
	scheduledAt := time.Now().UTC().Add(time.Duration(secs) * time.Second).Format(time.RFC3339)
	_, err := q.db.ExecContext(ctx,
		`UPDATE ingestion_jobs
		    SET status = 'pending',
		        started_at = NULL,
		        scheduled_at = ?
		  WHERE id = ?`,
		scheduledAt, jobID)
	return err
}

func (q *SQLiteQuery) GetSyncStatus(ctx context.Context, platformChannelID, requestingUserID string) (*SyncStatus, error) {
	var trackingExists bool
	if err := q.db.QueryRowContext(ctx,
		`SELECT EXISTS (
		     SELECT 1 FROM user_channel_tracking
		      WHERE user_id = ?
		        AND platform_channel_id = ?
		        AND tracking_status IN ('active','paused')
		 )`,
		requestingUserID, platformChannelID).Scan(&trackingExists); err != nil {
		return nil, err
	}
	if !trackingExists {
		return nil, ErrNotFound
	}

	var lastSyncedAt, backoffUntil, lastErrorAt sql.NullString
	var errorCount, backfillDays int
	err := q.db.QueryRowContext(ctx,
		`SELECT pc.last_synced_at, pc.ingestion_backoff_until,
		        pc.ingestion_last_error_at, pc.ingestion_error_count,
		        COALESCE(MIN(uct.backfill_days), 90)
		   FROM platform_channels pc
		   LEFT JOIN user_channel_tracking uct
		          ON uct.platform_channel_id = pc.id
		         AND uct.tracking_status IN ('active','paused')
		  WHERE pc.id = ?
		  GROUP BY pc.id`,
		platformChannelID).Scan(&lastSyncedAt, &backoffUntil, &lastErrorAt, &errorCount, &backfillDays)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	var lastErrMsg *string
	_ = q.db.QueryRowContext(ctx,
		`SELECT error_message FROM ingestion_jobs
		  WHERE platform_channel_id = ? AND error_message IS NOT NULL
		  ORDER BY COALESCE(completed_at, started_at, created_at) DESC
		  LIMIT 1`,
		platformChannelID).Scan(&lastErrMsg)

	var backfillInProgress bool
	if err := q.db.QueryRowContext(ctx,
		`SELECT EXISTS (
		     SELECT 1 FROM ingestion_jobs
		      WHERE platform_channel_id = ?
		        AND job_type = 'backfill'
		        AND status IN ('pending','running')
		 )`,
		platformChannelID).Scan(&backfillInProgress); err != nil {
		return nil, err
	}

	out := &SyncStatus{LastSyncedAt: parseTimePtr(lastSyncedAt)}

	if lastSyncedAt.Valid {
		ls := parseTime(lastSyncedAt.String)
		nxt := ls.Add(30 * time.Minute)
		if bu := parseTime(backoffUntil.String); backoffUntil.Valid && bu.After(nxt) {
			nxt = bu
		}
		out.NextSyncAt = &nxt
	} else if backoffUntil.Valid {
		bu := parseTime(backoffUntil.String)
		out.NextSyncAt = &bu
	}

	if backfillInProgress && lastSyncedAt.Valid && backfillDays > 0 {
		horizon := time.Now().Add(-time.Duration(backfillDays) * 24 * time.Hour)
		denom := time.Since(horizon).Seconds()
		if denom > 0 {
			pct := (parseTime(lastSyncedAt.String).Sub(horizon).Seconds() / denom) * 100.0
			if pct < 0 {
				pct = 0
			}
			if pct > 100 {
				pct = 100
			}
			out.BackfillProgressPercent = &pct
		}
	}

	if lastErrorAt.Valid && lastErrMsg != nil {
		out.RecentError = &SyncError{At: parseTime(lastErrorAt.String), Message: *lastErrMsg}
	}

	return out, nil
}

func (q *SQLiteQuery) GetMaxBackfillDays(ctx context.Context, platformChannelID string) (int, error) {
	var days sql.NullInt64
	err := q.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(backfill_days), 90)
		   FROM user_channel_tracking
		  WHERE platform_channel_id = ?
		    AND tracking_status IN ('active','paused')`,
		platformChannelID).Scan(&days)
	if err != nil {
		return 0, err
	}
	if !days.Valid || days.Int64 <= 0 {
		return 90, nil
	}
	return int(days.Int64), nil
}

// RecoverStaleIngestionJobs resets running jobs older than olderThan to pending.
func (q *SQLiteQuery) RecoverStaleIngestionJobs(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339)
	res, err := q.db.ExecContext(ctx,
		`UPDATE ingestion_jobs
		    SET status = 'pending', started_at = NULL,
		        scheduled_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		  WHERE status = 'running'
		    AND started_at IS NOT NULL
		    AND started_at < ?`,
		cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
