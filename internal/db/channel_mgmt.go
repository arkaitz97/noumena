package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func (q *SQLiteQuery) InsertOAuthState(ctx context.Context, stateHash, userID string, expiresAt time.Time) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO oauth_states (state_hash, user_id, expires_at) VALUES (?, ?, ?)`,
		stateHash, userID, expiresAt.Format(time.RFC3339))
	return err
}

func (q *SQLiteQuery) ConsumeOAuthState(ctx context.Context, stateHash, userID string) (bool, error) {
	var consumed string
	err := q.db.QueryRowContext(ctx,
		`DELETE FROM oauth_states
		 WHERE state_hash = ? AND user_id = ? AND expires_at > strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		 RETURNING state_hash`,
		stateHash, userID).Scan(&consumed)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (q *SQLiteQuery) CleanupExpiredOAuthStates(ctx context.Context) (int64, error) {
	result, err := q.db.ExecContext(ctx, `DELETE FROM oauth_states WHERE expires_at <= strftime('%Y-%m-%dT%H:%M:%SZ', 'now')`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (q *SQLiteQuery) UpsertPlatformChannel(ctx context.Context, platform, platformChannelID, displayName string) (string, bool, error) {
	id := generateUUID()
	var returnedID string
	err := q.db.QueryRowContext(ctx,
		`INSERT INTO platform_channels (id, platform, platform_channel_id, display_name, created_at)
		 VALUES (?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
		 ON CONFLICT(platform, platform_channel_id) DO UPDATE SET display_name = excluded.display_name, deletion_scheduled_at = NULL
		 RETURNING id`,
		id, platform, platformChannelID, displayName).Scan(&returnedID)
	if err != nil {
		return "", false, err
	}
	return returnedID, returnedID == id, nil
}

func (q *SQLiteQuery) GetChannelByPlatformID(ctx context.Context, platform, platformChannelID string) (*PlatformChannel, error) {
	var pc PlatformChannel
	var lastSyncedAt, deletionScheduledAt sql.NullString
	var createdAt string
	err := q.db.QueryRowContext(ctx,
		`SELECT id, platform, platform_channel_id, display_name, last_synced_at, deletion_scheduled_at, created_at
		 FROM platform_channels WHERE platform = ? AND platform_channel_id = ?`,
		platform, platformChannelID).Scan(
		&pc.ID, &pc.Platform, &pc.PlatformChannelID, &pc.DisplayName,
		&lastSyncedAt, &deletionScheduledAt, &createdAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	pc.LastSyncedAt = parseTimePtr(lastSyncedAt)
	pc.DeletionScheduledAt = parseTimePtr(deletionScheduledAt)
	pc.CreatedAt = parseTime(createdAt)
	return &pc, nil
}

func (q *SQLiteQuery) ScheduleChannelDeletion(ctx context.Context, platformChannelID string, graceHours int) error {
	until := time.Now().UTC().Add(time.Duration(graceHours) * time.Hour).Format(time.RFC3339)
	_, err := q.db.ExecContext(ctx,
		`UPDATE platform_channels
		 SET deletion_scheduled_at = ?
		 WHERE id = ?`,
		until, platformChannelID)
	return err
}

func (q *SQLiteQuery) ClearChannelDeletion(ctx context.Context, platformChannelID string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE platform_channels SET deletion_scheduled_at = NULL WHERE id = ?`,
		platformChannelID)
	return err
}

func (q *SQLiteQuery) GetActiveTrackerCount(ctx context.Context, platformChannelID string) (int, error) {
	var n int
	err := q.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_channel_tracking
		 WHERE platform_channel_id = ? AND tracking_status = 'active'`,
		platformChannelID).Scan(&n)
	return n, err
}

func (q *SQLiteQuery) PurgeChannelData(ctx context.Context, platformChannelID string) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var n int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_channel_tracking
		 WHERE platform_channel_id = ? AND tracking_status = 'active'`,
		platformChannelID).Scan(&n); err != nil {
		return err
	}

	if n > 0 {
		if _, err := tx.ExecContext(ctx,
			`UPDATE platform_channels SET deletion_scheduled_at = NULL WHERE id = ?`,
			platformChannelID); err != nil {
			return err
		}
		return tx.Commit()
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM user_comment_stars
		 WHERE comment_id IN (SELECT id FROM comments WHERE platform_channel_id = ?)`,
		platformChannelID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM ingestion_jobs WHERE platform_channel_id = ?`, platformChannelID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM comments WHERE platform_channel_id = ?`, platformChannelID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM user_channel_tracking WHERE platform_channel_id = ?`, platformChannelID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM platform_channels WHERE id = ?`, platformChannelID); err != nil {
		return err
	}
	return tx.Commit()
}

const trackingColumns = `id, user_id, platform_channel_id, access_token_enc,
	refresh_token_enc, token_expires_at, token_priority, tracking_status,
	last_seen_at, backfill_days, token_expiry_notified_at, created_at`

func scanTracking(row *sql.Row) (*UserChannelTracking, error) {
	t := &UserChannelTracking{}
	var tokenExpiresAt, lastSeenAt, tokenExpiryNotifiedAt sql.NullString
	var createdAt string
	err := row.Scan(
		&t.ID, &t.UserID, &t.PlatformChannelID, &t.AccessTokenEnc,
		&t.RefreshTokenEnc, &tokenExpiresAt, &t.TokenPriority, &t.TrackingStatus,
		&lastSeenAt, &t.BackfillDays, &tokenExpiryNotifiedAt, &createdAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	t.TokenExpiresAt = parseTimePtr(tokenExpiresAt)
	t.LastSeenAt = parseTimePtr(lastSeenAt)
	t.TokenExpiryNotifiedAt = parseTimePtr(tokenExpiryNotifiedAt)
	t.CreatedAt = parseTime(createdAt)
	return t, nil
}

func scanTrackingRows(rows *sql.Rows) (*UserChannelTracking, error) {
	t := &UserChannelTracking{}
	var tokenExpiresAt, lastSeenAt, tokenExpiryNotifiedAt sql.NullString
	var createdAt string
	err := rows.Scan(
		&t.ID, &t.UserID, &t.PlatformChannelID, &t.AccessTokenEnc,
		&t.RefreshTokenEnc, &tokenExpiresAt, &t.TokenPriority, &t.TrackingStatus,
		&lastSeenAt, &t.BackfillDays, &tokenExpiryNotifiedAt, &createdAt,
	)
	if err != nil {
		return nil, err
	}
	t.TokenExpiresAt = parseTimePtr(tokenExpiresAt)
	t.LastSeenAt = parseTimePtr(lastSeenAt)
	t.TokenExpiryNotifiedAt = parseTimePtr(tokenExpiryNotifiedAt)
	t.CreatedAt = parseTime(createdAt)
	return t, nil
}

// ErrChannelLimitExceeded is returned by CreateUserChannelTrackingWithLimit
// when the user already tracks limit active+paused channels.
var ErrChannelLimitExceeded = errors.New("channel limit exceeded")

func (q *SQLiteQuery) CreateUserChannelTracking(ctx context.Context, userID, platformChannelID, encryptedAccess, encryptedRefresh string, expiresAt time.Time, backfillDays, tokenPriority int) (*UserChannelTracking, error) {
	return q.CreateUserChannelTrackingWithLimit(ctx, userID, platformChannelID, encryptedAccess, encryptedRefresh, expiresAt, backfillDays, tokenPriority, 1<<62)
}

func (q *SQLiteQuery) CreateUserChannelTrackingWithLimit(ctx context.Context, userID, platformChannelID, encryptedAccess, encryptedRefresh string, expiresAt time.Time, backfillDays, tokenPriority, limit int) (*UserChannelTracking, error) {
	if !validBackfillDays(backfillDays) {
		return nil, fmt.Errorf("invalid backfill_days %d", backfillDays)
	}
	var activeWorkspaceID sql.NullString
	_ = q.db.QueryRowContext(ctx, `SELECT active_workspace_id FROM users WHERE id = ?`, userID).Scan(&activeWorkspaceID)
	// Zero expiry (tokenless public-channel rows) persists as NULL so
	// expiry scans and the frontend never treat them as expired.
	tokenExpiresAt := sql.NullString{}
	if !expiresAt.IsZero() {
		tokenExpiresAt = sql.NullString{String: expiresAt.Format(time.RFC3339), Valid: true}
	}
	row := q.db.QueryRowContext(ctx,
		`INSERT INTO user_channel_tracking
		   (id, user_id, platform_channel_id, workspace_id, access_token_enc, refresh_token_enc,
		    token_expires_at, token_priority, tracking_status, backfill_days,
		    token_expiry_notified_at, created_at)
		 SELECT ?, ?, ?, ?, ?, ?, ?, ?, 'active', ?, NULL, strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		 WHERE (SELECT COUNT(*) FROM user_channel_tracking WHERE user_id = ? AND tracking_status IN ('active','paused')) < ?
		 RETURNING `+trackingColumns,
		generateUUID(), userID, platformChannelID, activeWorkspaceID, encryptedAccess, encryptedRefresh,
		tokenExpiresAt, tokenPriority, backfillDays, userID, limit)
	t, err := scanTracking(row)
	if errors.Is(err, ErrNotFound) {
		return nil, ErrChannelLimitExceeded
	}
	return t, err
}

func (q *SQLiteQuery) GetUserTracking(ctx context.Context, userID, platformChannelID string) (*UserChannelTracking, error) {
	row := q.db.QueryRowContext(ctx,
		`SELECT `+trackingColumns+` FROM user_channel_tracking
		 WHERE user_id = ? AND platform_channel_id = ?`,
		userID, platformChannelID)
	return scanTracking(row)
}

func (q *SQLiteQuery) ListUserChannels(ctx context.Context, userID string) ([]ChannelListItem, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT pc.id, pc.platform, pc.platform_channel_id, pc.display_name,
		        uct.tracking_status, uct.token_priority, pc.last_synced_at,
		        uct.backfill_days, uct.token_expires_at, uct.token_expiry_notified_at
		 FROM user_channel_tracking uct
		 JOIN platform_channels pc ON pc.id = uct.platform_channel_id
		 WHERE uct.user_id = ?
		 ORDER BY pc.created_at DESC`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ChannelListItem, 0)
	for rows.Next() {
		var it ChannelListItem
		var lastSyncedAt, tokenExpiresAt, tokenExpiryNotifiedAt sql.NullString
		if err := rows.Scan(
			&it.ID, &it.Platform, &it.PlatformChannelID, &it.DisplayName,
			&it.TrackingStatus, &it.TokenPriority, &lastSyncedAt,
			&it.BackfillDays, &tokenExpiresAt, &tokenExpiryNotifiedAt,
		); err != nil {
			return nil, err
		}
		it.LastSyncedAt = parseTimePtr(lastSyncedAt)
		it.TokenExpiresAt = parseTimePtr(tokenExpiresAt)
		it.TokenExpiryNotifiedAt = parseTimePtr(tokenExpiryNotifiedAt)
		out = append(out, it)
	}
	return out, rows.Err()
}

func (q *SQLiteQuery) PauseTracking(ctx context.Context, userID, platformChannelID string) error {
	result, err := q.db.ExecContext(ctx,
		`UPDATE user_channel_tracking
		 SET tracking_status = 'paused', token_priority = 9999
		 WHERE user_id = ? AND platform_channel_id = ?`,
		userID, platformChannelID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (q *SQLiteQuery) ResumeTracking(ctx context.Context, userID, platformChannelID string) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var status string
	err = tx.QueryRowContext(ctx,
		`SELECT tracking_status FROM user_channel_tracking
		 WHERE user_id = ? AND platform_channel_id = ?`,
		userID, platformChannelID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	if status == "active" {
		return tx.Commit()
	}

	var maxPrio sql.NullInt64
	if err := tx.QueryRowContext(ctx,
		`SELECT MAX(token_priority) FROM user_channel_tracking
		 WHERE platform_channel_id = ? AND tracking_status = 'active'`,
		platformChannelID).Scan(&maxPrio); err != nil {
		return err
	}
	newPrio := 0
	if maxPrio.Valid {
		newPrio = int(maxPrio.Int64) + 1
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE user_channel_tracking
		 SET tracking_status = 'active', token_priority = ?
		 WHERE user_id = ? AND platform_channel_id = ?`,
		newPrio, userID, platformChannelID); err != nil {
		return err
	}
	return tx.Commit()
}

func (q *SQLiteQuery) DeleteTracking(ctx context.Context, userID, platformChannelID string) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx,
		`DELETE FROM user_channel_tracking
		 WHERE user_id = ? AND platform_channel_id = ?`,
		userID, platformChannelID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrNotFound
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM user_comment_stars
		 WHERE user_id = ?
		   AND comment_id IN (
		     SELECT id FROM comments WHERE platform_channel_id = ?
		   )`,
		userID, platformChannelID); err != nil {
		return err
	}
	return tx.Commit()
}

// backfillDayOptions are the selectable initial-backfill windows (kept in
// sync with the API validation in internal/api).
var backfillDayOptions = map[int]bool{30: true, 60: true, 90: true, 180: true}

func validBackfillDays(d int) bool {
	return backfillDayOptions[d]
}

func (q *SQLiteQuery) UpdateBackfillDays(ctx context.Context, userID, platformChannelID string, days int) error {
	if !validBackfillDays(days) {
		return fmt.Errorf("invalid backfill_days %d", days)
	}
	result, err := q.db.ExecContext(ctx,
		`UPDATE user_channel_tracking SET backfill_days = ?
		 WHERE user_id = ? AND platform_channel_id = ?`,
		days, userID, platformChannelID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (q *SQLiteQuery) GetMaxTokenPriority(ctx context.Context, platformChannelID string) (int, error) {
	var maxPrio sql.NullInt64
	err := q.db.QueryRowContext(ctx,
		`SELECT MAX(token_priority) FROM user_channel_tracking
		 WHERE platform_channel_id = ?`,
		platformChannelID).Scan(&maxPrio)
	if err != nil {
		return -1, err
	}
	if !maxPrio.Valid {
		return -1, nil
	}
	return int(maxPrio.Int64), nil
}

func (q *SQLiteQuery) GetIngestionTokenRow(ctx context.Context, platformChannelID string) (*UserChannelTracking, bool, error) {
	row := q.db.QueryRowContext(ctx,
		`SELECT `+trackingColumns+` FROM user_channel_tracking
		 WHERE platform_channel_id = ? AND tracking_status = 'active'
		 ORDER BY CASE WHEN token_expires_at IS NULL OR token_expires_at > strftime('%Y-%m-%dT%H:%M:%SZ', 'now') THEN 0 ELSE 1 END,
		          token_priority ASC, created_at ASC
		 LIMIT 1`,
		platformChannelID)
	t, err := scanTracking(row)
	if errors.Is(err, ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return t, true, nil
}

func (q *SQLiteQuery) UpdateTokenAndExpiry(ctx context.Context, trackingID, encryptedAccess, encryptedRefresh string, expiresAt time.Time) error {
	result, err := q.db.ExecContext(ctx,
		`UPDATE user_channel_tracking
		 SET access_token_enc = ?,
		     refresh_token_enc = ?,
		     token_expires_at = ?,
		     token_expiry_notified_at = NULL
		 WHERE id = ?`,
		encryptedAccess, encryptedRefresh, expiresAt.Format(time.RFC3339), trackingID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (q *SQLiteQuery) GetExpiringTokens(ctx context.Context, withinDays int) ([]UserChannelTracking, error) {
	now := time.Now().UTC()
	until := now.AddDate(0, 0, withinDays).Format(time.RFC3339)
	rows, err := q.db.QueryContext(ctx,
		`SELECT `+trackingColumns+` FROM user_channel_tracking
		 WHERE tracking_status IN ('active','paused')
		   AND token_expires_at IS NOT NULL
		   AND token_expires_at BETWEEN ? AND ?
		   AND token_expiry_notified_at IS NULL`,
		now.Format(time.RFC3339), until)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []UserChannelTracking
	for rows.Next() {
		t, err := scanTrackingRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

func (q *SQLiteQuery) MarkTokenExpiryNotified(ctx context.Context, trackingID string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE user_channel_tracking SET token_expiry_notified_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ?`,
		trackingID)
	return err
}

func (q *SQLiteQuery) GetCommentCountForChannel(ctx context.Context, platformChannelID string) (int, error) {
	var n int
	err := q.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM comments WHERE platform_channel_id = ?`,
		platformChannelID).Scan(&n)
	return n, err
}

func (q *SQLiteQuery) ChannelsDueForPurge(ctx context.Context) ([]string, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT id FROM platform_channels
		 WHERE deletion_scheduled_at IS NOT NULL AND deletion_scheduled_at <= strftime('%Y-%m-%dT%H:%M:%SZ', 'now')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (q *SQLiteQuery) GetChannelCustomPrompt(ctx context.Context, platformChannelID, userID string) (string, error) {
	var customPrompt sql.NullString
	err := q.db.QueryRowContext(ctx,
		`SELECT custom_prompt FROM user_channel_tracking
		 WHERE platform_channel_id = ? AND user_id = ? AND tracking_status = 'active'
		 LIMIT 1`,
		platformChannelID, userID).Scan(&customPrompt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if !customPrompt.Valid {
		return "", nil
	}
	return customPrompt.String, err
}

func (q *SQLiteQuery) GetCommentBodyByID(ctx context.Context, commentID string) (string, error) {
	var body sql.NullString
	err := q.db.QueryRowContext(ctx,
		`SELECT body FROM comments WHERE id = ?`, commentID).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if !body.Valid {
		return "", nil
	}
	return body.String, err
}
