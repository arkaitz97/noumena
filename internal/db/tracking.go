package db

import (
	"context"
	"database/sql"
	"errors"
)

func (q *SQLiteQuery) GetIngestionToken(ctx context.Context, pcID string) (string, string, error) {
	query := `
		SELECT access_token_enc, refresh_token_enc
		FROM user_channel_tracking
		WHERE platform_channel_id = ? AND tracking_status = 'active'
		ORDER BY CASE WHEN token_expires_at IS NULL OR token_expires_at > strftime('%Y-%m-%dT%H:%M:%SZ', 'now') THEN 0 ELSE 1 END,
		         token_priority ASC, created_at ASC
		LIMIT 1
	`
	var access, refresh string
	err := q.db.QueryRowContext(ctx, query, pcID).Scan(&access, &refresh)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return access, refresh, err
}

func (q *SQLiteQuery) PromoteNextIngestionToken(ctx context.Context, pcID string) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var lockedID string
	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM platform_channels WHERE id = ?`, pcID,
	).Scan(&lockedID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE user_channel_tracking
		 SET token_priority = 9999
		 WHERE platform_channel_id = ?
		   AND token_priority = 0
		   AND tracking_status = 'active'`,
		pcID); err != nil {
		return err
	}

	var nextID string
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM user_channel_tracking
		 WHERE platform_channel_id = ?
		   AND tracking_status = 'active'
		   AND token_priority > 0
		   AND token_priority < 9999
		 ORDER BY token_priority ASC, created_at ASC
		 LIMIT 1`,
		pcID).Scan(&nextID)
	if errors.Is(err, sql.ErrNoRows) {
		return tx.Commit()
	}
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE user_channel_tracking SET token_priority = 0 WHERE id = ?`,
		nextID); err != nil {
		return err
	}
	return tx.Commit()
}

func (q *SQLiteQuery) UpdateChannelCustomPrompt(ctx context.Context, userID, platformChannelID string, customPrompt *string) error {
	var prompt interface{} = customPrompt
	if customPrompt != nil {
		prompt = *customPrompt
	}
	_, err := q.db.ExecContext(ctx,
		`UPDATE user_channel_tracking SET custom_prompt = ?
		 WHERE platform_channel_id = ? AND user_id = ?`,
		prompt, platformChannelID, userID)
	return err
}
