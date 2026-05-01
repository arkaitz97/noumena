package db

import (
	"context"
	"time"
)

func (q *PostgresQuery) ListChannelsForUser(ctx context.Context, userID string) ([]*PlatformChannel, error) {
	query := `
		SELECT pc.id, pc.platform, pc.platform_channel_id, pc.display_name, pc.last_synced_at, pc.created_at
		FROM platform_channels pc
		JOIN user_channel_tracking uct ON pc.id = uct.platform_channel_id
		WHERE uct.user_id = $1 AND uct.tracking_status != 'removed'
	`
	rows, err := q.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []*PlatformChannel
	for rows.Next() {
		var pc PlatformChannel
		if err := rows.Scan(
			&pc.ID, &pc.Platform, &pc.PlatformChannelID, &pc.DisplayName,
			&pc.LastSyncedAt, &pc.CreatedAt,
		); err != nil {
			return nil, err
		}
		channels = append(channels, &pc)
	}
	return channels, nil
}

func (q *PostgresQuery) ConnectChannelTx(ctx context.Context, userID, platform, platformChannelID, displayName, accessTokenEnc, refreshTokenEnc string, tokenExpiresAt *time.Time) error {
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Upsert platform_channels (ignore if exists, but we want the ID regardless)
	var pcID string
	queryPC := `
		INSERT INTO platform_channels (platform, platform_channel_id, display_name)
		VALUES ($1, $2, $3)
		ON CONFLICT (platform, platform_channel_id) DO UPDATE SET display_name = EXCLUDED.display_name
		RETURNING id
	`
	err = tx.QueryRow(ctx, queryPC, platform, platformChannelID, displayName).Scan(&pcID)
	if err != nil {
		return err
	}

	// Upsert user_channel_tracking
	queryUCT := `
		INSERT INTO user_channel_tracking (user_id, platform_channel_id, access_token_enc, refresh_token_enc, token_expires_at, tracking_status)
		VALUES ($1, $2, $3, $4, $5, 'active')
		ON CONFLICT (user_id, platform_channel_id) DO UPDATE SET 
			access_token_enc = EXCLUDED.access_token_enc,
			refresh_token_enc = EXCLUDED.refresh_token_enc,
			token_expires_at = EXCLUDED.token_expires_at,
			tracking_status = 'active'
	`
	_, err = tx.Exec(ctx, queryUCT, userID, pcID, accessTokenEnc, refreshTokenEnc, tokenExpiresAt)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (q *PostgresQuery) PauseChannelTracking(ctx context.Context, userID, pcID string) error {
	query := `
		UPDATE user_channel_tracking
		SET tracking_status = 'paused'
		WHERE user_id = $1 AND platform_channel_id = $2
	`
	_, err := q.pool.Exec(ctx, query, userID, pcID)
	return err
}

func (q *PostgresQuery) RemoveChannelTracking(ctx context.Context, userID, pcID string) error {
	// PRD says to delete the user_channel_tracking row completely.
	query := `
		DELETE FROM user_channel_tracking
		WHERE user_id = $1 AND platform_channel_id = $2
	`
	_, err := q.pool.Exec(ctx, query, userID, pcID)
	return err
}

func (q *PostgresQuery) GetChannelsForSync(ctx context.Context, minutes int) ([]*PlatformChannel, error) {
	query := `
		SELECT id, platform, platform_channel_id, display_name, last_synced_at, created_at
		FROM platform_channels
		WHERE last_synced_at IS NULL OR last_synced_at < NOW() - make_interval(mins => $1)
	`
	rows, err := q.pool.Query(ctx, query, minutes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []*PlatformChannel
	for rows.Next() {
		var pc PlatformChannel
		if err := rows.Scan(
			&pc.ID, &pc.Platform, &pc.PlatformChannelID, &pc.DisplayName,
			&pc.LastSyncedAt, &pc.CreatedAt,
		); err != nil {
			return nil, err
		}
		channels = append(channels, &pc)
	}
	return channels, nil
}

func (q *PostgresQuery) UpdateSyncTime(ctx context.Context, id string) error {
	_, err := q.pool.Exec(ctx, "UPDATE platform_channels SET last_synced_at = NOW() WHERE id = $1", id)
	return err
}
