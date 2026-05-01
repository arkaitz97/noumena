package db

import (
	"context"
)

// GetIngestionToken gets the highest priority active token for a channel
func (q *PostgresQuery) GetIngestionToken(ctx context.Context, pcID string) (string, string, error) {
	query := `
		SELECT access_token_enc, refresh_token_enc
		FROM user_channel_tracking
		WHERE platform_channel_id = $1 AND tracking_status = 'active'
		ORDER BY token_priority ASC, created_at ASC
		LIMIT 1
	`
	var access, refresh string
	err := q.pool.QueryRow(ctx, query, pcID).Scan(&access, &refresh)
	return access, refresh, err
}

// PromoteNextIngestionToken demotes the current primary token
func (q *PostgresQuery) PromoteNextIngestionToken(ctx context.Context, pcID string) error {
    query := `
        WITH primary_tracker AS (
            SELECT id FROM user_channel_tracking 
            WHERE platform_channel_id = $1 AND tracking_status = 'active'
            ORDER BY token_priority ASC, created_at ASC
            LIMIT 1
        )
        UPDATE user_channel_tracking
        SET token_priority = token_priority + 100
        WHERE id IN (SELECT id FROM primary_tracker)
    `
    _, err := q.pool.Exec(ctx, query, pcID)
    return err
}
