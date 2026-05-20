package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// GetIngestionToken returns the encrypted access/refresh token of the
// active tracker with the lowest token_priority for a channel. It is the
// legacy compatibility shim used by the ingestion worker.
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
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return access, refresh, err
}

// PromoteNextIngestionToken demotes the current primary (token_priority = 0)
// to a sentinel high priority and elects the next-lowest-priority active
// tracker as the new primary. The election runs inside a transaction holding
// `SELECT ... FOR UPDATE` on the parent platform_channels row to serialize
// concurrent lifecycle events.
func (q *PostgresQuery) PromoteNextIngestionToken(ctx context.Context, pcID string) error {
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Serialize concurrent promotions for this channel.
	var lockedID string
	if err := tx.QueryRow(ctx,
		`SELECT id FROM platform_channels WHERE id = $1 FOR UPDATE`, pcID,
	).Scan(&lockedID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}

	// Demote the current primary, if any, to a sentinel value. We must
	// vacate priority 0 first so the partial unique index does not block
	// the subsequent promote update.
	if _, err := tx.Exec(ctx,
		`UPDATE user_channel_tracking
		 SET token_priority = 9999
		 WHERE platform_channel_id = $1
		   AND token_priority = 0
		   AND tracking_status = 'active'`,
		pcID); err != nil {
		return err
	}

	// Elect the next active tracker (if any). Choose the lowest priority
	// strictly greater than 0 so we don't re-elect a sentinel'd primary.
	var nextID string
	err = tx.QueryRow(ctx,
		`SELECT id FROM user_channel_tracking
		 WHERE platform_channel_id = $1
		   AND tracking_status = 'active'
		   AND token_priority > 0
		   AND token_priority < 9999
		 ORDER BY token_priority ASC, created_at ASC
		 LIMIT 1`,
		pcID).Scan(&nextID)
	if errors.Is(err, pgx.ErrNoRows) {
		// No replacement available; commit the demotion anyway.
		return tx.Commit(ctx)
	}
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE user_channel_tracking SET token_priority = 0 WHERE id = $1`,
		nextID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
