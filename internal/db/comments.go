package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// CommitCommentsAndCheckpoint makes replay boundaries durable with their
// comment batch. A failed transaction leaves both at their prior position.
func (q *SQLiteQuery) CommitCommentsAndCheckpoint(ctx context.Context, comments []*Comment, checkpoint *ChannelSyncCheckpoint) (int, error) {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	inserted := 0
	for _, c := range comments {
		res, err := tx.ExecContext(ctx, `INSERT INTO comments (id, platform_channel_id, platform_comment_id, parent_comment_id, reply_depth, author_handle, body, posted_at, video_id, video_title, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) ON CONFLICT (platform_channel_id, platform_comment_id) DO UPDATE SET video_id=COALESCE(excluded.video_id, comments.video_id), video_title=COALESCE(excluded.video_title, comments.video_title)`, generateUUID(), c.PlatformChannelID, c.PlatformCommentID, c.ParentCommentID, c.ReplyDepth, c.AuthorHandle, c.Body, formatTime(c.PostedAt), c.VideoID, c.VideoTitle)
		if err != nil {
			_ = tx.Rollback()
			return inserted, err
		}
		n, _ := res.RowsAffected()
		inserted += int(n)
	}
	if checkpoint != nil {
		var token, boundary, video, msg any
		if checkpoint.PageToken != nil {
			token = *checkpoint.PageToken
		}
		if checkpoint.Boundary != nil {
			boundary = checkpoint.Boundary.UTC().Format(time.RFC3339)
		}
		if checkpoint.CurrentVideoID != nil {
			video = *checkpoint.CurrentVideoID
		}
		if checkpoint.ErrorMessage != nil {
			msg = *checkpoint.ErrorMessage
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO channel_sync_checkpoints(platform_channel_id,phase,page_token,boundary,current_video_id,status,error_message,updated_at) VALUES(?,?,?,?,?,?,?,strftime('%Y-%m-%dT%H:%M:%SZ','now')) ON CONFLICT(platform_channel_id,phase) DO UPDATE SET page_token=excluded.page_token,boundary=excluded.boundary,current_video_id=excluded.current_video_id,status=excluded.status,error_message=excluded.error_message,updated_at=excluded.updated_at`, checkpoint.PlatformChannelID, checkpoint.Phase, token, boundary, video, checkpoint.Status, msg); err != nil {
			_ = tx.Rollback()
			return inserted, err
		}
	}
	if err := tx.Commit(); err != nil {
		return inserted, err
	}
	return inserted, nil
}

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
		INSERT INTO comments (id, platform_channel_id, platform_comment_id, parent_comment_id, reply_depth, author_handle, body, posted_at, video_id, video_title, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
		ON CONFLICT (platform_channel_id, platform_comment_id) DO UPDATE SET
			video_id = COALESCE(excluded.video_id, comments.video_id),
			video_title = COALESCE(excluded.video_title, comments.video_title)
	`
	result, err := q.db.ExecContext(ctx, query,
		generateUUID(), c.PlatformChannelID, c.PlatformCommentID, c.ParentCommentID, c.ReplyDepth, c.AuthorHandle, c.Body, formatTime(c.PostedAt), c.VideoID, c.VideoTitle,
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

func (q *SQLiteQuery) GetUnprocessedCommentsByChannel(ctx context.Context, limit int, maxAttempts int) (map[string][]*Comment, error) {
	// Channels without an active primary (token_priority=0) tracker are
	// excluded at claim time: the LLM processor skips them anyway, and
	// claiming them every tick re-scans the same batch forever (log spam +
	// churn). Comments stay unprocessed and get picked up once a primary
	// exists. Retry state (llm_attempts / llm_next_retry_at) excludes
	// dead-lettered and backing-off comments.
	now := time.Now().UTC().Format(time.RFC3339)
	query := `
		SELECT c.id, c.platform_channel_id, c.platform_comment_id, c.parent_comment_id, c.reply_depth, c.author_handle, c.body, c.posted_at, c.category, c.actionable_insights, c.topic, c.confidence, c.processed_at, c.created_at
		FROM comments c
		WHERE (c.processed_at IS NULL OR c.sentiment_score IS NULL) AND c.body != ''
		  AND c.llm_attempts < ?
		  AND (c.llm_next_retry_at IS NULL OR c.llm_next_retry_at <= ?)
		  AND NOT EXISTS (
		       SELECT 1
		         FROM llm_processing_jobs j,
		              json_each(CASE WHEN json_valid(j.comment_ids) THEN j.comment_ids ELSE '[]' END) je
		        WHERE j.status IN ('pending', 'processing')
		          AND je.value = c.id
		  )
		  AND EXISTS (
		       SELECT 1 FROM user_channel_tracking uct
		        WHERE uct.platform_channel_id = c.platform_channel_id
		          AND uct.tracking_status = 'active'
		          AND uct.token_priority = 0
		  )
		ORDER BY c.posted_at ASC
		LIMIT ?
	`
	rows, err := q.db.QueryContext(ctx, query, maxAttempts, now, limit)
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
	var postedAt, processedAt, createdAt, insights sql.NullString
	if err := rows.Scan(
		&c.ID, &c.PlatformChannelID, &c.PlatformCommentID, &c.ParentCommentID, &c.ReplyDepth, &c.AuthorHandle, &c.Body, &postedAt,
		&c.Category, &insights, &c.Topic, &c.Confidence, &processedAt, &createdAt,
	); err != nil {
		return nil, err
	}
	if insights.Valid {
		c.ActionableInsights = json.RawMessage(insights.String)
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
