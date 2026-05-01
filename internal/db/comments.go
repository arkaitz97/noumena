package db

import (
	"context"
)

func (q *PostgresQuery) GetCommentFeed(ctx context.Context, userID string, limit, offset int) ([]*Comment, error) {
	// Query to get all comments from channels actively tracked by the user, ignoring toxic/spam.
	query := `
		SELECT c.id, c.platform_channel_id, c.platform_comment_id, c.parent_comment_id, c.author_handle, c.body, c.posted_at, c.category, c.sentiment_score, c.topic, c.confidence, c.processed_at, c.created_at
		FROM comments c
		JOIN user_channel_tracking uct ON c.platform_channel_id = uct.platform_channel_id
		WHERE uct.user_id = $1 AND uct.tracking_status = 'active'
		  AND c.category NOT IN ('Criticism (Toxic)', 'Spam', 'Other') 
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
	query := `
		INSERT INTO comments (platform_channel_id, platform_comment_id, parent_comment_id, author_handle, body, posted_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (platform_channel_id, platform_comment_id) DO NOTHING
	`
	_, err := q.pool.Exec(ctx, query, 
		c.PlatformChannelID, c.PlatformCommentID, c.ParentCommentID, c.AuthorHandle, c.Body, c.PostedAt,
	)
	return err
}

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

func (q *PostgresQuery) UpdateCommentLLMData(ctx context.Context, id, category, topic string, sentiment, confidence float64) error {
	query := `
		UPDATE comments
		SET category = $1, topic = $2, sentiment_score = $3, confidence = $4, processed_at = NOW()
		WHERE id = $5
	`
	_, err := q.pool.Exec(ctx, query, category, topic, sentiment, confidence, id)
	return err
}
