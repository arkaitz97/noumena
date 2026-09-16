package db

import (
	"context"
	"database/sql"
	"errors"
	"math/rand"
	"time"
)

func (q *SQLiteQuery) GetChannelsNeedingSync(ctx context.Context, intervalMinutes int) ([]*PlatformChannel, error) {
	// Adaptive polling (optimize-youtube-api-quota): the configured interval
	// is a floor; channels with an ingestion_poll override are due only after
	// their own longer interval has elapsed.
	rows, err := q.db.QueryContext(ctx,
		`SELECT pc.id, pc.platform, pc.platform_channel_id, pc.display_name,
		        pc.last_synced_at, pc.deletion_scheduled_at, pc.created_at,
		        pc.ingestion_error_count,
		        pc.ingestion_last_error_at, pc.ingestion_backoff_until,
		        pc.yt_uploads_playlist_id, pc.poll_interval_minutes
		   FROM platform_channels pc
		  WHERE ((pc.last_synced_at IS NULL
		         OR pc.last_synced_at < strftime('%Y-%m-%dT%H:%M:%SZ', 'now',
		              '-' || COALESCE(pc.poll_interval_minutes, ?) || ' minutes'))
		    OR EXISTS (SELECT 1 FROM channel_sync_checkpoints csc
		                WHERE csc.platform_channel_id = pc.id
		                  AND csc.phase = 'backfill'
		                  AND csc.status <> 'complete'))
		    AND (pc.ingestion_backoff_until IS NULL
		         OR pc.ingestion_backoff_until < strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
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
		var lastSyncedAt, deletionScheduledAt, createdAt, ingestionLastErrorAt, ingestionBackoffUntil sql.NullString
		var uploadsPlaylistID sql.NullString
		var pollIntervalMinutes sql.NullInt64
		if err := rows.Scan(
			&pc.ID, &pc.Platform, &pc.PlatformChannelID, &pc.DisplayName,
			&lastSyncedAt, &deletionScheduledAt, &createdAt,
			&pc.IngestionErrorCount,
			&ingestionLastErrorAt, &ingestionBackoffUntil,
			&uploadsPlaylistID, &pollIntervalMinutes,
		); err != nil {
			return nil, err
		}
		pc.LastSyncedAt = parseTimePtr(lastSyncedAt)
		pc.DeletionScheduledAt = parseTimePtr(deletionScheduledAt)
		pc.CreatedAt = parseTime(createdAt.String)
		pc.IngestionLastErrorAt = parseTimePtr(ingestionLastErrorAt)
		pc.IngestionBackoffUntil = parseTimePtr(ingestionBackoffUntil)
		pc.UploadsPlaylistID = strPtrOrNull(uploadsPlaylistID)
		if pollIntervalMinutes.Valid {
			v := int(pollIntervalMinutes.Int64)
			pc.PollIntervalMinutes = &v
		}
		out = append(out, &pc)
	}
	return out, rows.Err()
}

func (q *SQLiteQuery) GetPlatformChannelByID(ctx context.Context, platformChannelID string) (*PlatformChannel, error) {
	pc := &PlatformChannel{}
	var lastSyncedAt, deletionScheduledAt, createdAt, ingestionLastErrorAt, ingestionBackoffUntil sql.NullString
	var uploadsPlaylistID sql.NullString
	var pollIntervalMinutes sql.NullInt64
	err := q.db.QueryRowContext(ctx,
		`SELECT id, platform, platform_channel_id, display_name,
		        last_synced_at, deletion_scheduled_at, created_at,
		        ingestion_error_count,
		        ingestion_last_error_at, ingestion_backoff_until,
		        yt_uploads_playlist_id, poll_interval_minutes
		   FROM platform_channels WHERE id = ?`,
		platformChannelID).Scan(
		&pc.ID, &pc.Platform, &pc.PlatformChannelID, &pc.DisplayName,
		&lastSyncedAt, &deletionScheduledAt, &createdAt,
		&pc.IngestionErrorCount,
		&ingestionLastErrorAt, &ingestionBackoffUntil,
		&uploadsPlaylistID, &pollIntervalMinutes,
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
	pc.UploadsPlaylistID = strPtrOrNull(uploadsPlaylistID)
	if pollIntervalMinutes.Valid {
		v := int(pollIntervalMinutes.Int64)
		pc.PollIntervalMinutes = &v
	}
	return pc, nil
}

// SetUploadsPlaylistID caches a channel's YouTube uploads-playlist id so
// subsequent jobs can discover videos via playlistItems.list (1 unit).
func (q *SQLiteQuery) SetUploadsPlaylistID(ctx context.Context, platformChannelID, playlistID string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE platform_channels SET yt_uploads_playlist_id = ? WHERE id = ?`,
		playlistID, platformChannelID)
	return err
}

// SetPollInterval persists the adaptive sync interval (minutes) for a
// channel; the scheduler's due-for-sync query honors it as a per-channel
// override of the configured minimum.
func (q *SQLiteQuery) SetPollInterval(ctx context.Context, platformChannelID string, minutes int) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE platform_channels SET poll_interval_minutes = ? WHERE id = ?`,
		minutes, platformChannelID)
	return err
}

// UpsertVideoSyncState durably records the incremental position for one
// video's comment walk. Empty watermark/checkpoint strings clear the column;
// status must be 'active' or 'skipped'.
func (q *SQLiteQuery) UpsertVideoSyncState(ctx context.Context, st *VideoSyncState) error {
	status := st.Status
	if status == "" {
		status = "active"
	}
	var watermark, checkpoint any
	if st.CommentWatermarkAt != nil {
		watermark = st.CommentWatermarkAt.UTC().Format(time.RFC3339)
	}
	if st.PageCheckpoint != nil && *st.PageCheckpoint != "" {
		checkpoint = *st.PageCheckpoint
	}
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO video_sync_state
		     (platform_channel_id, video_id, comment_watermark_at, page_checkpoint, status, updated_at)
		 VALUES (?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
		 ON CONFLICT(platform_channel_id, video_id) DO UPDATE SET
		     comment_watermark_at = COALESCE(excluded.comment_watermark_at, video_sync_state.comment_watermark_at),
		     page_checkpoint = excluded.page_checkpoint,
		     status = excluded.status,
		     updated_at = excluded.updated_at`,
		st.PlatformChannelID, st.VideoID, watermark, checkpoint, status)
	return err
}

// GetVideoSyncState returns the durable incremental position for one video.
// ErrNotFound when no checkpoint has been recorded yet.
func (q *SQLiteQuery) GetVideoSyncState(ctx context.Context, platformChannelID, videoID string) (*VideoSyncState, error) {
	st := &VideoSyncState{PlatformChannelID: platformChannelID, VideoID: videoID}
	var watermark, checkpoint, replyCheckpoint, replyChecked sql.NullString
	err := q.db.QueryRowContext(ctx,
		`SELECT comment_watermark_at, page_checkpoint, status, reply_page_checkpoint, remote_reply_count, last_reply_checked_at
		   FROM video_sync_state
		  WHERE platform_channel_id = ? AND video_id = ?`,
		platformChannelID, videoID).Scan(&watermark, &checkpoint, &st.Status, &replyCheckpoint, &st.RemoteReplyCount, &replyChecked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	st.CommentWatermarkAt = parseTimePtr(watermark)
	st.PageCheckpoint = strPtrOrNull(checkpoint)
	st.ReplyPageCheckpoint = strPtrOrNull(replyCheckpoint)
	st.LastReplyCheckedAt = parseTimePtr(replyChecked)
	return st, nil
}

func (q *SQLiteQuery) GetChannelSyncCheckpoint(ctx context.Context, channelID, phase string) (*ChannelSyncCheckpoint, error) {
	c := &ChannelSyncCheckpoint{PlatformChannelID: channelID, Phase: phase}
	var token, boundary, video, msg, updated string
	err := q.db.QueryRowContext(ctx, `SELECT page_token,boundary,current_video_id,status,error_message,updated_at FROM channel_sync_checkpoints WHERE platform_channel_id=? AND phase=?`, channelID, phase).Scan(&token, &boundary, &video, &c.Status, &msg, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c.PageToken, c.CurrentVideoID, c.ErrorMessage = strPtr(token), strPtr(video), strPtr(msg)
	if boundary != "" {
		t := parseTime(boundary)
		c.Boundary = &t
	}
	c.UpdatedAt = parseTime(updated)
	return c, nil
}

func (q *SQLiteQuery) AdvanceChannelSyncCheckpoint(ctx context.Context, c *ChannelSyncCheckpoint) error {
	var token, boundary, video, msg any
	if c.PageToken != nil {
		token = *c.PageToken
	}
	if c.Boundary != nil {
		boundary = c.Boundary.UTC().Format(time.RFC3339)
	}
	if c.CurrentVideoID != nil {
		video = *c.CurrentVideoID
	}
	if c.ErrorMessage != nil {
		msg = *c.ErrorMessage
	}
	_, err := q.db.ExecContext(ctx, `INSERT INTO channel_sync_checkpoints(platform_channel_id,phase,page_token,boundary,current_video_id,status,error_message,updated_at) VALUES(?,?,?,?,?,?,?,strftime('%Y-%m-%dT%H:%M:%SZ','now')) ON CONFLICT(platform_channel_id,phase) DO UPDATE SET page_token=excluded.page_token,boundary=excluded.boundary,current_video_id=excluded.current_video_id,status=excluded.status,error_message=excluded.error_message,updated_at=excluded.updated_at`, c.PlatformChannelID, c.Phase, token, boundary, video, c.Status, msg)
	return err
}

func (q *SQLiteQuery) CompleteChannelSyncCheckpoint(ctx context.Context, channelID, phase string) error {
	return q.updateChannelCheckpointStatus(ctx, channelID, phase, "complete", nil)
}
func (q *SQLiteQuery) PauseChannelSyncCheckpoint(ctx context.Context, channelID, phase string, errMsg string) error {
	return q.updateChannelCheckpointStatus(ctx, channelID, phase, "paused", &errMsg)
}
func (q *SQLiteQuery) updateChannelCheckpointStatus(ctx context.Context, channelID, phase, status string, msg *string) error {
	_, err := q.db.ExecContext(ctx, `UPDATE channel_sync_checkpoints SET status=?,error_message=?,updated_at=strftime('%Y-%m-%dT%H:%M:%SZ','now') WHERE platform_channel_id=? AND phase=?`, status, msg, channelID, phase)
	return err
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
		until.UTC().Format(time.RFC3339), platformChannelID)
	return err
}

func (q *SQLiteQuery) UpdateLastSyncedAt(ctx context.Context, platformChannelID string, t time.Time) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE platform_channels SET last_synced_at = ? WHERE id = ?`,
		t.UTC().Format(time.RFC3339), platformChannelID)
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

// EnqueueIngestionJob inserts a pending job unless ANY job for the channel is
// already pending or running, regardless of job_type. Deduping across types is
// the SQLite substitute for AcquireChannelLock (a no-op under SQLite): without
// it a backfill and a poll for the same channel could both be in flight,
// double-spending YouTube quota and racing UpsertVideoSyncState.
func (q *SQLiteQuery) EnqueueIngestionJob(ctx context.Context, platformChannelID, jobType string) (string, error) {
	var id string
	err := q.db.QueryRowContext(ctx,
		`INSERT INTO ingestion_jobs (id, platform_channel_id, job_type, status, scheduled_at)
		 SELECT ?, ?, ?, 'pending', strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		  WHERE NOT EXISTS (
		     SELECT 1 FROM ingestion_jobs
		      WHERE platform_channel_id = ?
		        AND status IN ('pending', 'running')
		 )
		 RETURNING id`,
		generateUUID(), platformChannelID, jobType, platformChannelID).Scan(&id)
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
	out.NextSyncAt = nextSyncAt(lastSyncedAt, backoffUntil)

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
