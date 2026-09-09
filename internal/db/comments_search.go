package db

import (
	"context"
	"strings"
)

func (q *SQLiteQuery) SearchComments(ctx context.Context, userID, query string) ([]*Comment, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT c.id, c.platform_channel_id, c.platform_comment_id, c.parent_comment_id, c.reply_depth,
				c.author_handle, c.body, c.posted_at, c.category, c.actionable_insights,
				c.topic, c.confidence, c.processed_at, c.created_at
		 FROM comments c
		 JOIN user_channel_tracking uct ON uct.platform_channel_id = c.platform_channel_id
		 WHERE uct.user_id = ?
		   AND uct.tracking_status = 'active'
		   AND c.rowid IN (SELECT rowid FROM comments_fts WHERE comments_fts MATCH ?)
		 ORDER BY c.posted_at DESC
		 LIMIT 100`,
		userID, sqliteFTSQuery(query))
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

// sqliteFTSQuery converts a user search query to FTS5 syntax
func sqliteFTSQuery(q string) string {
	// Escape FTS5 special characters and wrap in quotes for phrase matching
	q = strings.ReplaceAll(q, `"`, `""`)
	return `"` + q + `"`
}

func (q *SQLiteQuery) BulkStarComments(ctx context.Context, userID string, commentIDs []string) (int, error) {
	if len(commentIDs) == 0 {
		return 0, nil
	}
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT OR IGNORE INTO user_comment_stars (user_id, comment_id) SELECT ?, ? WHERE EXISTS (SELECT 1 FROM comments c WHERE c.id = ? AND (EXISTS(SELECT 1 FROM user_channel_tracking uct WHERE uct.platform_channel_id=c.platform_channel_id AND uct.user_id=? AND uct.tracking_status IN ('active','paused')) OR EXISTS(SELECT 1 FROM channel_grants cg JOIN workspace_members wm ON wm.workspace_id=cg.workspace_id WHERE cg.channel_id=c.platform_channel_id AND wm.user_id=?)))`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	count := 0
	for _, id := range commentIDs {
		result, err := stmt.ExecContext(ctx, userID, id, id, userID, userID)
		if err != nil {
			return count, err
		}
		n, _ := result.RowsAffected()
		count += int(n)
	}
	return count, tx.Commit()
}

func (q *SQLiteQuery) BulkUnstarComments(ctx context.Context, userID string, commentIDs []string) (int, error) {
	if len(commentIDs) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(commentIDs))
	args := make([]interface{}, len(commentIDs)+3)
	args[0] = userID
	for i, id := range commentIDs {
		placeholders[i] = "?"
		args[i+1] = id
	}
	args[len(commentIDs)+1] = userID
	args[len(commentIDs)+2] = userID
	query := `DELETE FROM user_comment_stars WHERE user_id = ? AND comment_id IN (` + strings.Join(placeholders, ",") + `) AND EXISTS (SELECT 1 FROM comments c WHERE c.id = user_comment_stars.comment_id AND (EXISTS(SELECT 1 FROM user_channel_tracking uct WHERE uct.platform_channel_id=c.platform_channel_id AND uct.user_id=? AND uct.tracking_status IN ('active','paused')) OR EXISTS(SELECT 1 FROM channel_grants cg JOIN workspace_members wm ON wm.workspace_id=cg.workspace_id WHERE cg.channel_id=c.platform_channel_id AND wm.user_id=?)))`
	result, err := q.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

func (q *SQLiteQuery) GetCommentThread(ctx context.Context, userID, commentID string, maxDepth int) ([]*Comment, error) {
	rows, err := q.db.QueryContext(ctx,
		`WITH RECURSIVE thread AS (
			SELECT c.id, c.platform_channel_id, c.platform_comment_id, c.parent_comment_id, c.reply_depth,
				   c.author_handle, c.body, c.posted_at, c.category, c.actionable_insights,
				   c.topic, c.confidence, c.processed_at, c.created_at, 0 as level
			FROM comments c
			WHERE c.id = ?
			UNION ALL
			SELECT c.id, c.platform_channel_id, c.platform_comment_id, c.parent_comment_id, c.reply_depth,
				   c.author_handle, c.body, c.posted_at, c.category, c.actionable_insights,
				   c.topic, c.confidence, c.processed_at, c.created_at, t.level + 1
			FROM comments c
			JOIN thread t ON c.parent_comment_id = t.platform_comment_id
			WHERE t.level < ?
		)
		SELECT t.id, t.platform_channel_id, t.platform_comment_id, t.parent_comment_id, t.reply_depth,
			   t.author_handle, t.body, t.posted_at, t.category, t.actionable_insights,
			   t.topic, t.confidence, t.processed_at, t.created_at
		FROM thread t
		JOIN user_channel_tracking uct ON uct.platform_channel_id = t.platform_channel_id
		WHERE uct.user_id = ?
		ORDER BY t.level, t.posted_at`,
		commentID, maxDepth, userID)
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
