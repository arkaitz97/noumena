package db

import (
	"context"
	"database/sql"
	"time"
)

func (q *SQLiteQuery) GetCommentFeed(ctx context.Context, userID string, limit, offset int) ([]*Comment, error) {
	query := `
		SELECT c.id, c.platform_channel_id, c.platform_comment_id, c.parent_comment_id, c.reply_depth, c.author_handle, c.body, c.posted_at, c.category, c.actionable_insights, c.topic, c.confidence, c.processed_at, c.created_at
		FROM comments c
		JOIN user_channel_tracking uct ON c.platform_channel_id = uct.platform_channel_id
		WHERE uct.user_id = ? AND uct.tracking_status = 'active'
		  AND c.category NOT IN ('toxic_criticism', 'spam', 'other')
		  AND c.processed_at IS NOT NULL
		ORDER BY c.posted_at DESC
		LIMIT ? OFFSET ?
	`
	rows, err := q.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []*Comment
	for rows.Next() {
		c, err := scanCommentRow(rows)
		if err != nil {
			return nil, err
		}
		comments = append(comments, c)
	}
	return comments, nil
}

func (q *SQLiteQuery) StarComment(ctx context.Context, userID, commentID string) error {
	query := `
		INSERT INTO user_comment_stars (user_id, comment_id)
		VALUES (?, ?)
		ON CONFLICT DO NOTHING
	`
	_, err := q.db.ExecContext(ctx, query, userID, commentID)
	return err
}

func (q *SQLiteQuery) InsertComment(ctx context.Context, c *Comment) error {
	_, err := q.InsertCommentRowsAffected(ctx, c)
	return err
}

func (q *SQLiteQuery) InsertCommentRowsAffected(ctx context.Context, c *Comment) (int64, error) {
	query := `
		INSERT INTO comments (id, platform_channel_id, platform_comment_id, parent_comment_id, reply_depth, author_handle, body, posted_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
		ON CONFLICT (platform_channel_id, platform_comment_id) DO NOTHING
	`
	result, err := q.db.ExecContext(ctx, query,
		generateUUID(), c.PlatformChannelID, c.PlatformCommentID, c.ParentCommentID, c.ReplyDepth, c.AuthorHandle, c.Body, formatTime(c.PostedAt),
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (q *SQLiteQuery) InsertComments(ctx context.Context, comments []*Comment) (int, error) {
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

func (q *SQLiteQuery) GetUnprocessedComments(ctx context.Context, limit int) ([]*Comment, error) {
	query := `
		SELECT id, platform_channel_id, platform_comment_id, parent_comment_id, reply_depth, author_handle, body, posted_at, category, actionable_insights, topic, confidence, processed_at, created_at
		FROM comments
		WHERE processed_at IS NULL AND body != ''
		ORDER BY posted_at ASC
		LIMIT ?
	`
	rows, err := q.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []*Comment
	for rows.Next() {
		c, err := scanCommentRow(rows)
		if err != nil {
			return nil, err
		}
		comments = append(comments, c)
	}
	return comments, nil
}

func (q *SQLiteQuery) GetUnprocessedCommentsByChannel(ctx context.Context, limit int) (map[string][]*Comment, error) {
	query := `
		SELECT id, platform_channel_id, platform_comment_id, parent_comment_id, reply_depth, author_handle, body, posted_at, category, actionable_insights, topic, confidence, processed_at, created_at
		FROM comments
		WHERE processed_at IS NULL AND body != ''
		ORDER BY posted_at ASC
		LIMIT ?
	`
	rows, err := q.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	grouped := make(map[string][]*Comment)
	for rows.Next() {
		c, err := scanCommentRow(rows)
		if err != nil {
			return nil, err
		}
		grouped[c.PlatformChannelID] = append(grouped[c.PlatformChannelID], c)
	}
	return grouped, nil
}

func scanCommentRow(rows *sql.Rows) (*Comment, error) {
	var c Comment
	var postedAt, processedAt, createdAt sql.NullString
	if err := rows.Scan(
		&c.ID, &c.PlatformChannelID, &c.PlatformCommentID, &c.ParentCommentID, &c.ReplyDepth, &c.AuthorHandle, &c.Body, &postedAt,
		&c.Category, &c.ActionableInsights, &c.Topic, &c.Confidence, &processedAt, &createdAt,
	); err != nil {
		return nil, err
	}
	c.PostedAt = parseTime(postedAt.String)
	c.ProcessedAt = parseTimePtr(processedAt)
	c.CreatedAt = parseTime(createdAt.String)
	return &c, nil
}

func (q *SQLiteQuery) UpdateCommentLLMData(ctx context.Context, id, category, topic, provider, model string, actionableInsights []byte, confidence float64) error {
	query := `
		UPDATE comments
		SET category = ?,
		    topic = ?,
		    actionable_insights = ?,
		    confidence = ?,
		    llm_provider = ?,
		    llm_model = ?,
		    processed_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		WHERE id = ?
	`
	_, err := q.db.ExecContext(ctx, query, category, topic, string(actionableInsights), confidence, provider, model, id)
	return err
}

// formatTime formats a time.Time to RFC3339 for SQLite storage, or returns empty string if zero
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// parseTime parses an RFC3339 string from SQLite, returning zero time on failure
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

// scanTimePtr scans a nullable time string from SQLite
func scanTimePtr(s sql.NullString) *time.Time {
	if !s.Valid || s.String == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s.String)
	if err != nil {
		return nil
	}
	return &t
}
