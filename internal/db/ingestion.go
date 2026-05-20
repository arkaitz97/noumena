package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ============================================================================
// platform_channels: ingestion-state queries
// ============================================================================

// GetChannelsNeedingSync returns channels whose `last_synced_at` is older than
// `intervalMinutes` minutes ago (or NULL), whose `ingestion_backoff_until` is
// past or unset, and which have at least one active tracker.
func (q *PostgresQuery) GetChannelsNeedingSync(ctx context.Context, intervalMinutes int) ([]*PlatformChannel, error) {
	rows, err := q.pool.Query(ctx,
		`SELECT pc.id, pc.platform, pc.platform_channel_id, pc.display_name,
		        pc.last_synced_at, pc.deletion_scheduled_at, pc.created_at,
		        pc.ingestion_lock_id, pc.ingestion_error_count,
		        pc.ingestion_last_error_at, pc.ingestion_backoff_until
		   FROM platform_channels pc
		  WHERE (pc.last_synced_at IS NULL
		         OR pc.last_synced_at < NOW() - make_interval(mins => $1))
		    AND (pc.ingestion_backoff_until IS NULL
		         OR pc.ingestion_backoff_until < NOW())
		    AND EXISTS (
		         SELECT 1 FROM user_channel_tracking uct
		          WHERE uct.platform_channel_id = pc.id
		            AND uct.tracking_status = 'active'
		    )`,
		intervalMinutes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*PlatformChannel
	for rows.Next() {
		var pc PlatformChannel
		if err := rows.Scan(
			&pc.ID, &pc.Platform, &pc.PlatformChannelID, &pc.DisplayName,
			&pc.LastSyncedAt, &pc.DeletionScheduledAt, &pc.CreatedAt,
			&pc.IngestionLockID, &pc.IngestionErrorCount,
			&pc.IngestionLastErrorAt, &pc.IngestionBackoffUntil,
		); err != nil {
			return nil, err
		}
		out = append(out, &pc)
	}
	return out, rows.Err()
}

// GetPlatformChannelByID loads a single platform_channels row by id.
func (q *PostgresQuery) GetPlatformChannelByID(ctx context.Context, platformChannelID string) (*PlatformChannel, error) {
	pc := &PlatformChannel{}
	err := q.pool.QueryRow(ctx,
		`SELECT id, platform, platform_channel_id, display_name,
		        last_synced_at, deletion_scheduled_at, created_at,
		        ingestion_lock_id, ingestion_error_count,
		        ingestion_last_error_at, ingestion_backoff_until
		   FROM platform_channels WHERE id = $1`,
		platformChannelID).Scan(
		&pc.ID, &pc.Platform, &pc.PlatformChannelID, &pc.DisplayName,
		&pc.LastSyncedAt, &pc.DeletionScheduledAt, &pc.CreatedAt,
		&pc.IngestionLockID, &pc.IngestionErrorCount,
		&pc.IngestionLastErrorAt, &pc.IngestionBackoffUntil,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return pc, nil
}
// UpdateIngestionError records a failure on the channel: increments error
// count, sets last_error_at, and parks the channel until NOW()+retryAfter.
func (q *PostgresQuery) UpdateIngestionError(ctx context.Context, platformChannelID, errMsg string, retryAfter time.Duration) error {
	secs := int64(retryAfter.Seconds())
	if secs < 0 {
		secs = 0
	}
	_, err := q.pool.Exec(ctx,
		`UPDATE platform_channels
		    SET ingestion_error_count = ingestion_error_count + 1,
		        ingestion_last_error_at = NOW(),
		        ingestion_backoff_until = NOW() + make_interval(secs => $2)
		  WHERE id = $1`,
		platformChannelID, secs)
	_ = errMsg // kept on the per-job ingestion_jobs row, not on the channel
	return err
}

// ResetIngestionError clears all error / backoff state on a channel.
func (q *PostgresQuery) ResetIngestionError(ctx context.Context, platformChannelID string) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE platform_channels
		    SET ingestion_error_count = 0,
		        ingestion_last_error_at = NULL,
		        ingestion_backoff_until = NULL
		  WHERE id = $1`,
		platformChannelID)
	return err
}

// MarkChannelBackoff parks a channel until `until` without touching the
// error counters. Used for rate-limit responses (which are not failures).
func (q *PostgresQuery) MarkChannelBackoff(ctx context.Context, platformChannelID string, until time.Time) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE platform_channels SET ingestion_backoff_until = $2 WHERE id = $1`,
		platformChannelID, until)
	return err
}

// UpdateLastSyncedAt sets last_synced_at on the channel. Used for periodic
// checkpointing during long-running backfills.
func (q *PostgresQuery) UpdateLastSyncedAt(ctx context.Context, platformChannelID string, t time.Time) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE platform_channels SET last_synced_at = $2 WHERE id = $1`,
		platformChannelID, t)
	return err
}

// UpdateLastSyncedAtIfNull sets last_synced_at = NOW() iff it is currently
// NULL. Used by the "no token" path so a freshly-connected-but-paused channel
// stops looking like a brand new backfill candidate.
func (q *PostgresQuery) UpdateLastSyncedAtIfNull(ctx context.Context, platformChannelID string) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE platform_channels SET last_synced_at = NOW()
		  WHERE id = $1 AND last_synced_at IS NULL`,
		platformChannelID)
	return err
}

// GetIngestionLockID returns the channel's BIGINT advisory-lock key. If the
// stored value is NULL it derives one from the UUID hash on the fly.
func (q *PostgresQuery) GetIngestionLockID(ctx context.Context, platformChannelID string) (int64, error) {
	var lockID *int64
	err := q.pool.QueryRow(ctx,
		`SELECT ingestion_lock_id FROM platform_channels WHERE id = $1`,
		platformChannelID).Scan(&lockID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	if lockID != nil {
		return *lockID, nil
	}
	// Derive on the fly.
	var derived int64
	err = q.pool.QueryRow(ctx,
		`SELECT ('x' || substr(md5($1::text), 1, 16))::bit(64)::bigint`,
		platformChannelID).Scan(&derived)
	return derived, err
}

// ============================================================================
// ingestion_jobs queue
// ============================================================================

const ingestionJobColumns = `id, platform_channel_id, job_type, status,
	retry_count, scheduled_at, started_at, completed_at, error_message, created_at`

func scanIngestionJob(row pgx.Row) (*IngestionJob, error) {
	j := &IngestionJob{}
	err := row.Scan(
		&j.ID, &j.PlatformChannelID, &j.JobType, &j.Status,
		&j.RetryCount, &j.ScheduledAt, &j.StartedAt, &j.CompletedAt,
		&j.ErrorMessage, &j.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return j, nil
}

// EnqueueIngestionJob inserts a new pending job for the channel iff there is
// not already a pending or running job of the same type. Returns the new job
// id, or empty string if a duplicate was suppressed.
func (q *PostgresQuery) EnqueueIngestionJob(ctx context.Context, platformChannelID, jobType string) (string, error) {
	var id string
	err := q.pool.QueryRow(ctx,
		`INSERT INTO ingestion_jobs (platform_channel_id, job_type, status, scheduled_at)
		 SELECT $1, $2, 'pending', NOW()
		  WHERE NOT EXISTS (
		     SELECT 1 FROM ingestion_jobs
		      WHERE platform_channel_id = $1
		        AND job_type = $2
		        AND status IN ('pending', 'running')
		 )
		 RETURNING id`,
		platformChannelID, jobType).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return id, nil
}

// ClaimNextIngestionJob atomically claims one pending job whose scheduled_at
// is past, marking it 'running'. Returns ErrNotFound when no job is eligible.
func (q *PostgresQuery) ClaimNextIngestionJob(ctx context.Context) (*IngestionJob, error) {
	row := q.pool.QueryRow(ctx,
		`UPDATE ingestion_jobs
		    SET status = 'running', started_at = NOW()
		  WHERE id = (
		     SELECT id FROM ingestion_jobs
		      WHERE status = 'pending' AND scheduled_at <= NOW()
		      ORDER BY scheduled_at
		      FOR UPDATE SKIP LOCKED
		      LIMIT 1
		  )
		 RETURNING `+ingestionJobColumns)
	return scanIngestionJob(row)
}

// MarkJobCompleted marks a running job as completed.
func (q *PostgresQuery) MarkJobCompleted(ctx context.Context, jobID string) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE ingestion_jobs
		    SET status = 'completed', completed_at = NOW(), error_message = NULL
		  WHERE id = $1`,
		jobID)
	return err
}

// MarkJobFailed reschedules the job for retry with exponential backoff
// (1s, 2s, 4s, 8s, 16s for retry_count 1..5) and increments retry_count.
// The caller is responsible for routing to MarkJobDeadLettered when
// retry_count is already at the configured max.
func (q *PostgresQuery) MarkJobFailed(ctx context.Context, jobID, errMsg string, currentRetryCount int) error {
	delaySecs := 1 << currentRetryCount // 1, 2, 4, 8, 16
	if delaySecs < 1 {
		delaySecs = 1
	}
	if delaySecs > 16 {
		delaySecs = 16
	}
	_, err := q.pool.Exec(ctx,
		`UPDATE ingestion_jobs
		    SET status = 'pending',
		        retry_count = retry_count + 1,
		        scheduled_at = NOW() + make_interval(secs => $2),
		        error_message = $3,
		        started_at = NULL
		  WHERE id = $1`,
		jobID, delaySecs, errMsg)
	return err
}

// MarkJobDeadLettered marks the job as permanently failed.
func (q *PostgresQuery) MarkJobDeadLettered(ctx context.Context, jobID, errMsg string) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE ingestion_jobs
		    SET status = 'dead_lettered',
		        completed_at = NOW(),
		        error_message = $2
		  WHERE id = $1`,
		jobID, errMsg)
	return err
}

// ReleaseJobToPending resets a claimed job back to pending with a delayed
// scheduled_at. Used when the worker cannot acquire the channel's advisory
// lock (lock-busy yields without consuming retry budget).
func (q *PostgresQuery) ReleaseJobToPending(ctx context.Context, jobID string, delay time.Duration) error {
	secs := int64(delay.Seconds())
	if secs < 0 {
		secs = 0
	}
	_, err := q.pool.Exec(ctx,
		`UPDATE ingestion_jobs
		    SET status = 'pending',
		        started_at = NULL,
		        scheduled_at = NOW() + make_interval(secs => $2)
		  WHERE id = $1`,
		jobID, secs)
	return err
}

// ============================================================================
// sync-status read
// ============================================================================

// GetSyncStatus returns the user-facing sync status for a channel. The caller
// must enforce that requestingUserID is a tracker of the channel.
func (q *PostgresQuery) GetSyncStatus(ctx context.Context, platformChannelID, requestingUserID string) (*SyncStatus, error) {
	// Authorize: requestingUser must have any tracking row (active OR paused).
	var trackingExists bool
	if err := q.pool.QueryRow(ctx,
		`SELECT EXISTS (
		     SELECT 1 FROM user_channel_tracking
		      WHERE user_id = $1
		        AND platform_channel_id = $2
		        AND tracking_status IN ('active','paused')
		 )`,
		requestingUserID, platformChannelID).Scan(&trackingExists); err != nil {
		return nil, err
	}
	if !trackingExists {
		return nil, ErrNotFound
	}

	var (
		lastSyncedAt    *time.Time
		backoffUntil    *time.Time
		lastErrorAt     *time.Time
		errorCount      int
		backfillDays    int
	)
	err := q.pool.QueryRow(ctx,
		`SELECT pc.last_synced_at, pc.ingestion_backoff_until,
		        pc.ingestion_last_error_at, pc.ingestion_error_count,
		        COALESCE(MIN(uct.backfill_days), 90)
		   FROM platform_channels pc
		   LEFT JOIN user_channel_tracking uct
		          ON uct.platform_channel_id = pc.id
		         AND uct.tracking_status IN ('active','paused')
		  WHERE pc.id = $1
		  GROUP BY pc.id, pc.last_synced_at, pc.ingestion_backoff_until,
		           pc.ingestion_last_error_at, pc.ingestion_error_count`,
		platformChannelID).Scan(&lastSyncedAt, &backoffUntil, &lastErrorAt, &errorCount, &backfillDays)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	// Look up the latest error message via ingestion_jobs (best-effort).
	var lastErrMsg *string
	_ = q.pool.QueryRow(ctx,
		`SELECT error_message FROM ingestion_jobs
		  WHERE platform_channel_id = $1 AND error_message IS NOT NULL
		  ORDER BY COALESCE(completed_at, started_at, created_at) DESC
		  LIMIT 1`,
		platformChannelID).Scan(&lastErrMsg)

	// Detect "backfill in progress" via a pending or running backfill job.
	var backfillInProgress bool
	if err := q.pool.QueryRow(ctx,
		`SELECT EXISTS (
		     SELECT 1 FROM ingestion_jobs
		      WHERE platform_channel_id = $1
		        AND job_type = 'backfill'
		        AND status IN ('pending','running')
		 )`,
		platformChannelID).Scan(&backfillInProgress); err != nil {
		return nil, err
	}

	out := &SyncStatus{LastSyncedAt: lastSyncedAt}

	// next_sync_at = max(last_synced_at + 30min, backoff_until)
	if lastSyncedAt != nil {
		nxt := lastSyncedAt.Add(30 * time.Minute)
		if backoffUntil != nil && backoffUntil.After(nxt) {
			nxt = *backoffUntil
		}
		out.NextSyncAt = &nxt
	} else if backoffUntil != nil {
		out.NextSyncAt = backoffUntil
	}

	if backfillInProgress && lastSyncedAt != nil && backfillDays > 0 {
		horizon := time.Now().Add(-time.Duration(backfillDays) * 24 * time.Hour)
		denom := time.Since(horizon).Seconds()
		if denom > 0 {
			pct := (lastSyncedAt.Sub(horizon).Seconds() / denom) * 100.0
			if pct < 0 {
				pct = 0
			}
			if pct > 100 {
				pct = 100
			}
			out.BackfillProgressPercent = &pct
		}
	}

	if lastErrorAt != nil && lastErrMsg != nil {
		out.RecentError = &SyncError{At: *lastErrorAt, Message: *lastErrMsg}
	}

	return out, nil
}

// GetMaxBackfillDays returns the maximum backfill_days requested across all
// active+paused trackers of the given channel. Returns 90 if there are no
// trackers (defensive fallback). The "max" semantics ensures the backfill
// reaches as far back as the most demanding subscriber.
func (q *PostgresQuery) GetMaxBackfillDays(ctx context.Context, platformChannelID string) (int, error) {
	var days int
	err := q.pool.QueryRow(ctx,
		`SELECT COALESCE(MAX(backfill_days), 90)
		   FROM user_channel_tracking
		  WHERE platform_channel_id = $1
		    AND tracking_status IN ('active','paused')`,
		platformChannelID).Scan(&days)
	if err != nil {
		return 0, err
	}
	if days <= 0 {
		days = 90
	}
	return days, nil
}
