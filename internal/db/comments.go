package db

import (
	"context"
)

func (q *PostgresQuery) GetCommentFeed(ctx context.Context, userID string, limit, offset int) ([]*Comment, error) {
	// Filter the feed by the llm-processing taxonomy. Toxic and spam are
	// excluded by spec; "other" is a low-confidence catch-all that should
	// not surface in positive insight feeds either.
	query := `
		SELECT c.id, c.platform_channel_id, c.platform_comment_id, c.parent_comment_id, c.author_handle, c.body, c.posted_at, c.category, c.sentiment_score, c.topic, c.confidence, c.processed_at, c.created_at
		FROM comments c
		JOIN user_channel_tracking uct ON c.platform_channel_id = uct.platform_channel_id
		WHERE uct.user_id = $1 AND uct.tracking_status = 'active'
		  AND c.category NOT IN ('toxic_criticism', 'spam', 'other')
		  AND c.processed_at IS NOT NULL
		ORDER BY c.posted_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := q.pool.Query(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []*Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(
			&c.ID, &c.PlatformChannelID, &c.PlatformCommentID, &c.ParentCommentID, &c.AuthorHandle, &c.Body, &c.PostedAt,
			&c.Category, &c.SentimentScore, &c.Topic, &c.Confidence, &c.ProcessedAt, &c.CreatedAt,
		); err != nil {
			return nil, err
		}
		comments = append(comments, &c)
	}
	return comments, nil
}

func (q *PostgresQuery) StarComment(ctx context.Context, userID, commentID string) error {
	query := `
		INSERT INTO user_comment_stars (user_id, comment_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`
	_, err := q.pool.Exec(ctx, query, userID, commentID)
	return err
}

func (q *PostgresQuery) InsertComment(ctx context.Context, c *Comment) error {
	_, err := q.InsertCommentRowsAffected(ctx, c)
	return err
}

// InsertCommentRowsAffected is the upsert variant exposed for ingestion code
// that needs to know whether a duplicate was suppressed by ON CONFLICT.
func (q *PostgresQuery) InsertCommentRowsAffected(ctx context.Context, c *Comment) (int64, error) {
	query := `
		INSERT INTO comments (platform_channel_id, platform_comment_id, parent_comment_id, author_handle, body, posted_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (platform_channel_id, platform_comment_id) DO NOTHING
	`
	tag, err := q.pool.Exec(ctx, query,
		c.PlatformChannelID, c.PlatformCommentID, c.ParentCommentID, c.AuthorHandle, c.Body, c.PostedAt,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// InsertComments inserts a batch of comments using ON CONFLICT DO NOTHING and
// returns the number of rows actually inserted (i.e. excluding silently
// skipped duplicates). Empty input is a no-op.
func (q *PostgresQuery) InsertComments(ctx context.Context, comments []*Comment) (int, error) {
	inserted := 0
	for _, c := range comments {
		n, err := q.InsertCommentRowsAffected(ctx, c)
		if err != nil {
			return inserted, err
		}
		inserted += int(n)
	}
	return inserted, nil
}

// GetUnprocessedComments is the legacy, non-grouped helper kept for backward
// compatibility with older callers. New LLM batch processing should use
// GetUnprocessedCommentsByChannel.
func (q *PostgresQuery) GetUnprocessedComments(ctx context.Context, limit int) ([]*Comment, error) {
	query := `
		SELECT id, platform_channel_id, platform_comment_id, parent_comment_id, author_handle, body, posted_at, category, sentiment_score, topic, confidence, processed_at, created_at
		FROM comments
		WHERE processed_at IS NULL AND body != ''
		ORDER BY posted_at ASC
		LIMIT $1
	`
	rows, err := q.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []*Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(
			&c.ID, &c.PlatformChannelID, &c.PlatformCommentID, &c.ParentCommentID, &c.AuthorHandle, &c.Body, &c.PostedAt,
			&c.Category, &c.SentimentScore, &c.Topic, &c.Confidence, &c.ProcessedAt, &c.CreatedAt,
		); err != nil {
			return nil, err
		}
		comments = append(comments, &c)
	}
	return comments, nil
}

// GetUnprocessedCommentsByChannel claims up to `limit` unprocessed comments
// using FOR UPDATE SKIP LOCKED and groups them by platform_channel_id so the
// LLM batch processor can dispatch one prompt per channel without mixing
// tenants. The lock is held only until the surrounding transaction commits;
// callers MUST run this inside a short Tx and release before invoking the LLM.
func (q *PostgresQuery) GetUnprocessedCommentsByChannel(ctx context.Context, limit int) (map[string][]*Comment, error) {
	query := `
		SELECT id, platform_channel_id, platform_comment_id, parent_comment_id, author_handle, body, posted_at, category, sentiment_score, topic, confidence, processed_at, created_at
		FROM comments
		WHERE processed_at IS NULL AND body != ''
		ORDER BY posted_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`
	rows, err := q.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	grouped := make(map[string][]*Comment)
	for rows.Next() {
		var c Comment
		if err := rows.Scan(
			&c.ID, &c.PlatformChannelID, &c.PlatformCommentID, &c.ParentCommentID, &c.AuthorHandle, &c.Body, &c.PostedAt,
			&c.Category, &c.SentimentScore, &c.Topic, &c.Confidence, &c.ProcessedAt, &c.CreatedAt,
		); err != nil {
			return nil, err
		}
		grouped[c.PlatformChannelID] = append(grouped[c.PlatformChannelID], &c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return grouped, nil
}

// UpdateCommentLLMData persists the final classification for a single comment
// including the provider/model attribution introduced by the llm-processing
// change. Sets processed_at = NOW() so the comment is no longer claimable.
func (q *PostgresQuery) UpdateCommentLLMData(ctx context.Context, id, category, topic, provider, model string, sentiment, confidence float64) error {
	query := `
		UPDATE comments
		SET category = $1,
		    topic = $2,
		    sentiment_score = $3,
		    confidence = $4,
		    llm_provider = $5,
		    llm_model = $6,
		    processed_at = NOW()
		WHERE id = $7
	`
	_, err := q.pool.Exec(ctx, query, category, topic, sentiment, confidence, provider, model, id)
	return err
}
