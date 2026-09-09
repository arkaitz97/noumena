package db

import (
	"context"
	"database/sql"
	"time"
)

func (q *SQLiteQuery) ListChannelsForUser(ctx context.Context, userID string) ([]*PlatformChannel, error) {
	query := `
		SELECT pc.id, pc.platform, pc.platform_channel_id, pc.display_name, pc.last_synced_at, pc.created_at
		FROM platform_channels pc
		JOIN user_channel_tracking uct ON pc.id = uct.platform_channel_id
		WHERE uct.user_id = ? AND uct.tracking_status != 'removed'
	`
	rows, err := q.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []*PlatformChannel
	for rows.Next() {
		var pc PlatformChannel
		var lastSyncedAt, createdAt sql.NullString
		if err := rows.Scan(
			&pc.ID, &pc.Platform, &pc.PlatformChannelID, &pc.DisplayName,
			&lastSyncedAt, &createdAt,
		); err != nil {
			return nil, err
		}
		pc.LastSyncedAt = parseTimePtr(lastSyncedAt)
		pc.CreatedAt = parseTime(createdAt.String)
		channels = append(channels, &pc)
	}
	return channels, nil
}

func (q *SQLiteQuery) ConnectChannelTx(ctx context.Context, userID, platform, platformChannelID, displayName, accessTokenEnc, refreshTokenEnc string, tokenExpiresAt *time.Time) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	pcID := generateUUID()
	queryPC := `
		INSERT INTO platform_channels (id, platform, platform_channel_id, display_name, created_at)
		VALUES (?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
		ON CONFLICT (platform, platform_channel_id) DO UPDATE SET display_name = excluded.display_name
		RETURNING id
	`
	err = tx.QueryRowContext(ctx, queryPC, pcID, platform, platformChannelID, displayName).Scan(&pcID)
	if err != nil {
		return err
	}

	var expiresAtStr string
	if tokenExpiresAt != nil {
		expiresAtStr = tokenExpiresAt.Format(time.RFC3339)
	}
	var activeWorkspaceID sql.NullString
	_ = tx.QueryRowContext(ctx, `SELECT active_workspace_id FROM users WHERE id = ?`, userID).Scan(&activeWorkspaceID)
	queryUCT := `
		INSERT INTO user_channel_tracking (id, user_id, platform_channel_id, workspace_id, access_token_enc, refresh_token_enc, token_expires_at, tracking_status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'active', strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
		ON CONFLICT (user_id, platform_channel_id) DO UPDATE SET
			access_token_enc = excluded.access_token_enc,
			refresh_token_enc = excluded.refresh_token_enc,
			token_expires_at = excluded.token_expires_at,
			tracking_status = 'active',
			token_expiry_notified_at = NULL,
			workspace_id = COALESCE(user_channel_tracking.workspace_id, excluded.workspace_id)
	`
	_, err = tx.ExecContext(ctx, queryUCT, generateUUID(), userID, pcID, activeWorkspaceID, accessTokenEnc, refreshTokenEnc, expiresAtStr)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (q *SQLiteQuery) PauseChannelTracking(ctx context.Context, userID, pcID string) error {
	query := `
		UPDATE user_channel_tracking
		SET tracking_status = 'paused'
		WHERE user_id = ? AND platform_channel_id = ?
	`
	_, err := q.db.ExecContext(ctx, query, userID, pcID)
	return err
}

func (q *SQLiteQuery) RemoveChannelTracking(ctx context.Context, userID, pcID string) error {
	query := `
		DELETE FROM user_channel_tracking
		WHERE user_id = ? AND platform_channel_id = ?
	`
	_, err := q.db.ExecContext(ctx, query, userID, pcID)
	return err
}

func (q *SQLiteQuery) GetChannelsForSync(ctx context.Context, minutes int) ([]*PlatformChannel, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(minutes) * time.Minute).Format(time.RFC3339)
	query := `
		SELECT id, platform, platform_channel_id, display_name, last_synced_at, created_at
		FROM platform_channels
		WHERE last_synced_at IS NULL OR last_synced_at < ?
	`
	rows, err := q.db.QueryContext(ctx, query, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []*PlatformChannel
	for rows.Next() {
		var pc PlatformChannel
		var lastSyncedAt, createdAt sql.NullString
		if err := rows.Scan(
			&pc.ID, &pc.Platform, &pc.PlatformChannelID, &pc.DisplayName,
			&lastSyncedAt, &createdAt,
		); err != nil {
			return nil, err
		}
		pc.LastSyncedAt = parseTimePtr(lastSyncedAt)
		pc.CreatedAt = parseTime(createdAt.String)
		channels = append(channels, &pc)
	}
	return channels, nil
}

func (q *SQLiteQuery) UpdateSyncTime(ctx context.Context, id string) error {
	_, err := q.db.ExecContext(ctx, "UPDATE platform_channels SET last_synced_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ?", id)
	return err
}

func parseNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
