package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ============================================================================
// OAuth state store
// ============================================================================

func (q *PostgresQuery) InsertOAuthState(ctx context.Context, stateHash, userID string, expiresAt time.Time) error {
	_, err := q.pool.Exec(ctx,
		`INSERT INTO oauth_states (state_hash, user_id, expires_at) VALUES ($1, $2, $3)`,
		stateHash, userID, expiresAt)
	return err
}

// ConsumeOAuthState atomically deletes a state row, returning true iff a
// matching unexpired row existed for the (state_hash, user_id) pair.
func (q *PostgresQuery) ConsumeOAuthState(ctx context.Context, stateHash, userID string) (bool, error) {
	var consumed string
	err := q.pool.QueryRow(ctx,
		`DELETE FROM oauth_states
		 WHERE state_hash = $1 AND user_id = $2 AND expires_at > NOW()
		 RETURNING state_hash`,
		stateHash, userID).Scan(&consumed)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (q *PostgresQuery) CleanupExpiredOAuthStates(ctx context.Context) (int64, error) {
	tag, err := q.pool.Exec(ctx, `DELETE FROM oauth_states WHERE expires_at <= NOW()`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ============================================================================
// platform_channels
// ============================================================================

// UpsertPlatformChannel inserts a new shared channel row, or refreshes the
// display name on conflict. The returned `inserted` flag tells callers
// whether a brand-new row was created so they can enqueue a backfill.
func (q *PostgresQuery) UpsertPlatformChannel(ctx context.Context, platform, platformChannelID, displayName string) (string, bool, error) {
	var id string
	var inserted bool
	err := q.pool.QueryRow(ctx,
		`INSERT INTO platform_channels (platform, platform_channel_id, display_name)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (platform, platform_channel_id)
		 DO UPDATE SET display_name = EXCLUDED.display_name,
		               deletion_scheduled_at = NULL
		 RETURNING id, (xmax = 0) AS inserted`,
		platform, platformChannelID, displayName).Scan(&id, &inserted)
	if err != nil {
		return "", false, err
	}
	return id, inserted, nil
}

func (q *PostgresQuery) GetChannelByPlatformID(ctx context.Context, platform, platformChannelID string) (*PlatformChannel, error) {
	var pc PlatformChannel
	err := q.pool.QueryRow(ctx,
		`SELECT id, platform, platform_channel_id, display_name, last_synced_at, deletion_scheduled_at, created_at
		 FROM platform_channels WHERE platform = $1 AND platform_channel_id = $2`,
		platform, platformChannelID).Scan(
		&pc.ID, &pc.Platform, &pc.PlatformChannelID, &pc.DisplayName,
		&pc.LastSyncedAt, &pc.DeletionScheduledAt, &pc.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &pc, nil
}

func (q *PostgresQuery) ScheduleChannelDeletion(ctx context.Context, platformChannelID string, graceHours int) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE platform_channels
		 SET deletion_scheduled_at = NOW() + make_interval(hours => $2)
		 WHERE id = $1`,
		platformChannelID, graceHours)
	return err
}

func (q *PostgresQuery) ClearChannelDeletion(ctx context.Context, platformChannelID string) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE platform_channels SET deletion_scheduled_at = NULL WHERE id = $1`,
		platformChannelID)
	return err
}

func (q *PostgresQuery) GetActiveTrackerCount(ctx context.Context, platformChannelID string) (int, error) {
	var n int
	err := q.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_channel_tracking
		 WHERE platform_channel_id = $1 AND tracking_status = 'active'`,
		platformChannelID).Scan(&n)
	return n, err
}

// PurgeChannelData re-checks the active-tracker count inside a transaction
// and, if zero, deletes the shared comments and the platform_channels row.
// If a tracker has reappeared, it clears `deletion_scheduled_at` and exits.
func (q *PostgresQuery) PurgeChannelData(ctx context.Context, platformChannelID string) error {
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var n int
	if err := tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_channel_tracking
		 WHERE platform_channel_id = $1 AND tracking_status = 'active'`,
		platformChannelID).Scan(&n); err != nil {
		return err
	}

	if n > 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE platform_channels SET deletion_scheduled_at = NULL WHERE id = $1`,
			platformChannelID); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM comments WHERE platform_channel_id = $1`, platformChannelID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM user_channel_tracking WHERE platform_channel_id = $1`, platformChannelID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM platform_channels WHERE id = $1`, platformChannelID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ============================================================================
// user_channel_tracking
// ============================================================================

const trackingColumns = `id, user_id, platform_channel_id, access_token_enc,
	refresh_token_enc, token_expires_at, token_priority, tracking_status,
	last_seen_at, backfill_days, token_expiry_notified_at, created_at`

func scanTracking(row pgx.Row) (*UserChannelTracking, error) {
	t := &UserChannelTracking{}
	err := row.Scan(
		&t.ID, &t.UserID, &t.PlatformChannelID, &t.AccessTokenEnc,
		&t.RefreshTokenEnc, &t.TokenExpiresAt, &t.TokenPriority, &t.TrackingStatus,
		&t.LastSeenAt, &t.BackfillDays, &t.TokenExpiryNotifiedAt, &t.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return t, nil
}

func (q *PostgresQuery) CreateUserChannelTracking(ctx context.Context, userID, platformChannelID, encryptedAccess, encryptedRefresh string, expiresAt time.Time, backfillDays, tokenPriority int) (*UserChannelTracking, error) {
	if !validBackfillDays(backfillDays) {
		return nil, fmt.Errorf("invalid backfill_days %d", backfillDays)
	}
	row := q.pool.QueryRow(ctx,
		`INSERT INTO user_channel_tracking
		   (user_id, platform_channel_id, access_token_enc, refresh_token_enc,
		    token_expires_at, token_priority, tracking_status, backfill_days,
		    token_expiry_notified_at)
		 VALUES ($1, $2, $3, $4, $5, $6, 'active', $7, NULL)
		 RETURNING `+trackingColumns,
		userID, platformChannelID, encryptedAccess, encryptedRefresh,
		expiresAt, tokenPriority, backfillDays)
	return scanTracking(row)
}

func (q *PostgresQuery) GetUserTracking(ctx context.Context, userID, platformChannelID string) (*UserChannelTracking, error) {
	row := q.pool.QueryRow(ctx,
		`SELECT `+trackingColumns+` FROM user_channel_tracking
		 WHERE user_id = $1 AND platform_channel_id = $2`,
		userID, platformChannelID)
	return scanTracking(row)
}

func (q *PostgresQuery) ListUserChannels(ctx context.Context, userID string) ([]ChannelListItem, error) {
	rows, err := q.pool.Query(ctx,
		`SELECT pc.id, pc.platform, pc.platform_channel_id, pc.display_name,
		        uct.tracking_status, uct.token_priority, pc.last_synced_at,
		        uct.backfill_days, uct.token_expires_at, uct.token_expiry_notified_at
		 FROM user_channel_tracking uct
		 JOIN platform_channels pc ON pc.id = uct.platform_channel_id
		 WHERE uct.user_id = $1
		 ORDER BY pc.created_at DESC`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ChannelListItem, 0)
	for rows.Next() {
		var it ChannelListItem
		if err := rows.Scan(
			&it.ID, &it.Platform, &it.PlatformChannelID, &it.DisplayName,
			&it.TrackingStatus, &it.TokenPriority, &it.LastSyncedAt,
			&it.BackfillDays, &it.TokenExpiresAt, &it.TokenExpiryNotifiedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (q *PostgresQuery) PauseTracking(ctx context.Context, userID, platformChannelID string) error {
	tag, err := q.pool.Exec(ctx,
		`UPDATE user_channel_tracking
		 SET tracking_status = 'paused', token_priority = 9999
		 WHERE user_id = $1 AND platform_channel_id = $2`,
		userID, platformChannelID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (q *PostgresQuery) ResumeTracking(ctx context.Context, userID, platformChannelID string) error {
	// Compute the next available priority for the channel inside a single
	// transaction so two concurrent resumes don't both pick 0.
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx,
		`SELECT tracking_status FROM user_channel_tracking
		 WHERE user_id = $1 AND platform_channel_id = $2`,
		userID, platformChannelID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	if status == "active" {
		// Idempotent no-op; commit to release lock.
		return tx.Commit(ctx)
	}

	// Find the lowest unused priority (0 if no active primary, else MAX+1).
	var maxPrio *int
	if err := tx.QueryRow(ctx,
		`SELECT MAX(token_priority) FROM user_channel_tracking
		 WHERE platform_channel_id = $1 AND tracking_status = 'active'`,
		platformChannelID).Scan(&maxPrio); err != nil {
		return err
	}
	newPrio := 0
	if maxPrio != nil {
		newPrio = *maxPrio + 1
	}

	if _, err := tx.Exec(ctx,
		`UPDATE user_channel_tracking
		 SET tracking_status = 'active', token_priority = $3
		 WHERE user_id = $1 AND platform_channel_id = $2`,
		userID, platformChannelID, newPrio); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// DeleteTracking deletes the user's tracking row and their stars for that
// channel inside a single transaction.
func (q *PostgresQuery) DeleteTracking(ctx context.Context, userID, platformChannelID string) error {
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx,
		`DELETE FROM user_channel_tracking
		 WHERE user_id = $1 AND platform_channel_id = $2`,
		userID, platformChannelID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM user_comment_stars
		 WHERE user_id = $1
		   AND comment_id IN (
		     SELECT id FROM comments WHERE platform_channel_id = $2
		   )`,
		userID, platformChannelID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func validBackfillDays(d int) bool {
	switch d {
	case 30, 60, 90, 180:
		return true
	}
	return false
}

func (q *PostgresQuery) UpdateBackfillDays(ctx context.Context, userID, platformChannelID string, days int) error {
	if !validBackfillDays(days) {
		return fmt.Errorf("invalid backfill_days %d", days)
	}
	tag, err := q.pool.Exec(ctx,
		`UPDATE user_channel_tracking SET backfill_days = $3
		 WHERE user_id = $1 AND platform_channel_id = $2`,
		userID, platformChannelID, days)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetMaxTokenPriority returns the highest token_priority currently in use
// (any status) for a channel, or -1 if no tracker exists.
func (q *PostgresQuery) GetMaxTokenPriority(ctx context.Context, platformChannelID string) (int, error) {
	var maxPrio *int
	err := q.pool.QueryRow(ctx,
		`SELECT MAX(token_priority) FROM user_channel_tracking
		 WHERE platform_channel_id = $1`,
		platformChannelID).Scan(&maxPrio)
	if err != nil {
		return -1, err
	}
	if maxPrio == nil {
		return -1, nil
	}
	return *maxPrio, nil
}

// GetIngestionTokenRow returns the lowest-priority active tracker for the
// channel, or (nil, false, nil) when none exists.
func (q *PostgresQuery) GetIngestionTokenRow(ctx context.Context, platformChannelID string) (*UserChannelTracking, bool, error) {
	row := q.pool.QueryRow(ctx,
		`SELECT `+trackingColumns+` FROM user_channel_tracking
		 WHERE platform_channel_id = $1 AND tracking_status = 'active'
		 ORDER BY token_priority ASC, created_at ASC
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

func (q *PostgresQuery) UpdateTokenAndExpiry(ctx context.Context, trackingID, encryptedAccess, encryptedRefresh string, expiresAt time.Time) error {
	tag, err := q.pool.Exec(ctx,
		`UPDATE user_channel_tracking
		 SET access_token_enc = $2,
		     refresh_token_enc = $3,
		     token_expires_at = $4,
		     token_expiry_notified_at = NULL
		 WHERE id = $1`,
		trackingID, encryptedAccess, encryptedRefresh, expiresAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (q *PostgresQuery) GetExpiringTokens(ctx context.Context, withinDays int) ([]UserChannelTracking, error) {
	rows, err := q.pool.Query(ctx,
		`SELECT `+trackingColumns+` FROM user_channel_tracking
		 WHERE tracking_status IN ('active','paused')
		   AND token_expires_at IS NOT NULL
		   AND token_expires_at BETWEEN NOW() AND NOW() + make_interval(days => $1)
		   AND token_expiry_notified_at IS NULL`,
		withinDays)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []UserChannelTracking
	for rows.Next() {
		var t UserChannelTracking
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.PlatformChannelID, &t.AccessTokenEnc,
			&t.RefreshTokenEnc, &t.TokenExpiresAt, &t.TokenPriority, &t.TrackingStatus,
			&t.LastSeenAt, &t.BackfillDays, &t.TokenExpiryNotifiedAt, &t.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (q *PostgresQuery) MarkTokenExpiryNotified(ctx context.Context, trackingID string) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE user_channel_tracking SET token_expiry_notified_at = NOW() WHERE id = $1`,
		trackingID)
	return err
}

func (q *PostgresQuery) GetCommentCountForChannel(ctx context.Context, platformChannelID string) (int, error) {
	var n int
	err := q.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM comments WHERE platform_channel_id = $1`,
		platformChannelID).Scan(&n)
	return n, err
}

// ChannelsDueForPurge returns the platform_channels that have a
// deletion_scheduled_at in the past. The purge job uses this to drive its
// scan; PurgeChannelData re-checks the tracker count atomically.
func (q *PostgresQuery) ChannelsDueForPurge(ctx context.Context) ([]string, error) {
	rows, err := q.pool.Query(ctx,
		`SELECT id FROM platform_channels
		 WHERE deletion_scheduled_at IS NOT NULL AND deletion_scheduled_at <= NOW()`)
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
