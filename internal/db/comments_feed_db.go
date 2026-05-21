package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type FeedRow struct {
	Comment
	ChannelName string `json:"channel_name"`
	IsStarred   bool   `json:"is_starred"`
}

var ErrCountTimeout = errors.New("count query timeout")
var ErrFeedTimeout = errors.New("feed query timeout")

func (q *SQLiteQuery) QueryCommentFeed(ctx context.Context, userID string, filters FeedFilters, timeout time.Duration) ([]*FeedRow, error) {
	sqlStr, args := BuildFeedQuery(userID, filters, ModeFeed)
	return q.runFeedQuery(ctx, sqlStr, args, timeout)
}

func (q *SQLiteQuery) QuerySavedFeed(ctx context.Context, userID string, filters FeedFilters, timeout time.Duration) ([]*FeedRow, error) {
	sqlStr, args := BuildFeedQuery(userID, filters, ModeSaved)
	return q.runFeedQuery(ctx, sqlStr, args, timeout)
}

func (q *SQLiteQuery) runFeedQuery(ctx context.Context, sqlStr string, args []any, timeout time.Duration) ([]*FeedRow, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	rows, err := q.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrFeedTimeout
		}
		return nil, err
	}
	defer rows.Close()
	var out []*FeedRow
	for rows.Next() {
		r := &FeedRow{}
		var isStarredInt int
		var actionableInsights sql.NullString
		if err := rows.Scan(
			&r.ID, &r.PlatformChannelID, &r.PlatformCommentID, &r.ParentCommentID, &r.ReplyDepth,
			&r.AuthorHandle, &r.Body, &r.PostedAt, &r.Category, &actionableInsights,
			&r.Topic, &r.Confidence, &r.ProcessedAt, &r.CreatedAt,
			&r.ChannelName, &isStarredInt,
		); err != nil {
			return nil, err
		}
		if actionableInsights.Valid {
			r.ActionableInsights = []byte(actionableInsights.String)
		}
		r.IsStarred = isStarredInt != 0
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (q *SQLiteQuery) CountCommentFeed(ctx context.Context, userID string, filters FeedFilters, timeout time.Duration) (int64, error) {
	sqlStr, args := BuildFeedQuery(userID, filters, ModeCount)

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var n int64
	if err := q.db.QueryRowContext(ctx, sqlStr, args...).Scan(&n); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return -1, ErrCountTimeout
		}
		return 0, err
	}
	return n, nil
}

func (q *SQLiteQuery) StarCommentScoped(ctx context.Context, userID, commentID string) (bool, error) {
	const sqlStr = `
		INSERT INTO user_comment_stars (user_id, comment_id, starred_at)
		SELECT ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		WHERE EXISTS (
			SELECT 1
			FROM comments c
			JOIN user_channel_tracking uct ON uct.platform_channel_id = c.platform_channel_id
			WHERE c.id = ?
			  AND uct.user_id = ?
			  AND uct.tracking_status IN ('active','paused')
		)
		ON CONFLICT (user_id, comment_id) DO NOTHING
	`
	result, err := q.db.ExecContext(ctx, sqlStr, userID, commentID, commentID, userID)
	if err != nil {
		return false, err
	}
	if n, _ := result.RowsAffected(); n > 0 {
		return true, nil
	}
	hasAccess, aerr := q.UserHasAccessToComment(ctx, userID, commentID)
	if aerr != nil {
		return false, aerr
	}
	return hasAccess, nil
}

func (q *SQLiteQuery) UnstarComment(ctx context.Context, userID, commentID string) (bool, error) {
	hasAccess, err := q.UserHasAccessToComment(ctx, userID, commentID)
	if err != nil {
		return false, err
	}
	if !hasAccess {
		return false, nil
	}
	_, err = q.db.ExecContext(ctx,
		`DELETE FROM user_comment_stars WHERE user_id = ? AND comment_id = ?`,
		userID, commentID)
	return true, err
}

func (q *SQLiteQuery) UserHasAccessToComment(ctx context.Context, userID, commentID string) (bool, error) {
	const sqlStr = `
		SELECT EXISTS (
			SELECT 1
			FROM comments c
			JOIN user_channel_tracking uct ON uct.platform_channel_id = c.platform_channel_id
			WHERE c.id = ?
			  AND uct.user_id = ?
			  AND uct.tracking_status IN ('active','paused')
		)
	`
	var ok bool
	err := q.db.QueryRowContext(ctx, sqlStr, commentID, userID).Scan(&ok)
	return ok, err
}

func (q *SQLiteQuery) UserTracksChannel(ctx context.Context, userID, channelID string) (bool, error) {
	const sqlStr = `
		SELECT EXISTS (
			SELECT 1
			FROM user_channel_tracking
			WHERE user_id = ?
			  AND platform_channel_id = ?
			  AND tracking_status IN ('active','paused')
		)
	`
	var ok bool
	err := q.db.QueryRowContext(ctx, sqlStr, userID, channelID).Scan(&ok)
	return ok, err
}

type InsightTypeCount struct {
	Type  string `json:"type"`
	Count int64  `json:"count"`
}

type TopInsight struct {
	Type        string  `json:"type"`
	Description string  `json:"description"`
	Count       int64   `json:"count"`
	AvgConfidence float64 `json:"avg_confidence"`
}

type ToxicityBucket struct {
	TotalComments int64   `json:"total_comments"`
	FilteredCount int64   `json:"filtered_count"`
	FilteredPct   float64 `json:"filtered_pct"`
}

type TopicRow struct {
	Topic string `json:"topic"`
	Count int64  `json:"count"`
}

type CategoryRow struct {
	Category string `json:"category"`
	Count    int64  `json:"count"`
}

func (q *SQLiteQuery) GetInsightDistribution(ctx context.Context, userID string, channelID *string, since time.Time) ([]InsightTypeCount, error) {
	args := []any{userID, since.Format(time.RFC3339)}
	channelClause := ""
	if channelID != nil {
		args = append(args, *channelID)
		channelClause = " AND c.platform_channel_id = ?"
	}
	sqlStr := `
		SELECT json_extract(elem.value, '$.type'), COUNT(*)
		FROM comments c
		JOIN user_channel_tracking uct ON uct.platform_channel_id = c.platform_channel_id,
		json_each(c.actionable_insights) elem
		WHERE uct.user_id = ?
		  AND uct.tracking_status IN ('active','paused')
		  AND c.processed_at IS NOT NULL
		  AND c.processed_at >= ?
		  AND c.category NOT IN ('toxic_criticism','spam')
		  AND c.actionable_insights != '[]'` + channelClause + `
		GROUP BY json_extract(elem.value, '$.type')
		ORDER BY COUNT(*) DESC
	`
	rows, err := q.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InsightTypeCount
	for rows.Next() {
		var r InsightTypeCount
		if err := rows.Scan(&r.Type, &r.Count); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (q *SQLiteQuery) GetTopInsights(ctx context.Context, userID string, channelID *string, since time.Time, limit int) ([]TopInsight, error) {
	args := []any{userID, since.Format(time.RFC3339)}
	channelClause := ""
	if channelID != nil {
		args = append(args, *channelID)
		channelClause = " AND c.platform_channel_id = ?"
	}
	args = append(args, limit)
	limitParam := "?"
	sqlStr := `
		SELECT json_extract(elem.value, '$.type'), json_extract(elem.value, '$.description'), COUNT(*), AVG(json_extract(elem.value, '$.confidence'))
		FROM comments c
		JOIN user_channel_tracking uct ON uct.platform_channel_id = c.platform_channel_id,
		json_each(c.actionable_insights) elem
		WHERE uct.user_id = ?
		  AND uct.tracking_status IN ('active','paused')
		  AND c.processed_at IS NOT NULL
		  AND c.processed_at >= ?
		  AND c.category NOT IN ('toxic_criticism','spam')
		  AND c.actionable_insights != '[]'` + channelClause + `
		GROUP BY json_extract(elem.value, '$.type'), json_extract(elem.value, '$.description')
		ORDER BY COUNT(*) DESC
		LIMIT ` + limitParam
	rows, err := q.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TopInsight
	for rows.Next() {
		var r TopInsight
		if err := rows.Scan(&r.Type, &r.Description, &r.Count, &r.AvgConfidence); err != nil {
			return nil, err
		}
		r.AvgConfidence = roundTo(r.AvgConfidence, 2)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (q *SQLiteQuery) GetToxicityShield(ctx context.Context, userID string, channelID *string, since time.Time) (ToxicityBucket, error) {
	args := []any{userID, since.Format(time.RFC3339)}
	channelClause := ""
	if channelID != nil {
		args = append(args, *channelID)
		channelClause = " AND c.platform_channel_id = ?"
	}
	sqlStr := `
		SELECT
		  COUNT(*),
		  SUM(CASE WHEN c.category IN ('toxic_criticism','spam') THEN 1 ELSE 0 END)
		FROM comments c
		JOIN user_channel_tracking uct ON uct.platform_channel_id = c.platform_channel_id
		WHERE uct.user_id = ?
		  AND uct.tracking_status IN ('active','paused')
		  AND c.processed_at IS NOT NULL
		  AND c.processed_at >= ?` + channelClause
	var b ToxicityBucket
	if err := q.db.QueryRowContext(ctx, sqlStr, args...).Scan(&b.TotalComments, &b.FilteredCount); err != nil {
		return ToxicityBucket{}, err
	}
	if b.TotalComments > 0 {
		b.FilteredPct = roundTo(float64(b.FilteredCount)/float64(b.TotalComments)*100, 1)
	}
	return b, nil
}

func (q *SQLiteQuery) GetTopTopics(ctx context.Context, userID string, channelID *string, days int, limit int) ([]TopicRow, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format(time.RFC3339)
	args := []any{userID, cutoff}
	channelClause := ""
	if channelID != nil {
		args = append(args, *channelID)
		channelClause = " AND c.platform_channel_id = ?"
	}
	args = append(args, limit)
	sqlStr := `
		SELECT c.topic, COUNT(*)
		FROM comments c
		JOIN user_channel_tracking uct ON uct.platform_channel_id = c.platform_channel_id
		WHERE uct.user_id = ?
		  AND uct.tracking_status IN ('active','paused')
		  AND c.processed_at IS NOT NULL
		  AND c.posted_at >= ?
		  AND c.category NOT IN ('toxic_criticism','spam')
		  AND c.topic IS NOT NULL AND c.topic <> ''` + channelClause + `
		GROUP BY c.topic
		ORDER BY COUNT(*) DESC, c.topic ASC
		LIMIT ?
	`
	rows, err := q.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TopicRow
	for rows.Next() {
		var r TopicRow
		if err := rows.Scan(&r.Topic, &r.Count); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (q *SQLiteQuery) GetTopCategories(ctx context.Context, userID string, channelID *string, days int) ([]CategoryRow, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format(time.RFC3339)
	args := []any{userID, cutoff}
	channelClause := ""
	if channelID != nil {
		args = append(args, *channelID)
		channelClause = " AND c.platform_channel_id = ?"
	}
	sqlStr := `
		SELECT c.category, COUNT(*)
		FROM comments c
		JOIN user_channel_tracking uct ON uct.platform_channel_id = c.platform_channel_id
		WHERE uct.user_id = ?
		  AND uct.tracking_status IN ('active','paused')
		  AND c.processed_at IS NOT NULL
		  AND c.posted_at >= ?
		  AND c.category IS NOT NULL
		  AND c.category NOT IN ('toxic_criticism','spam')` + channelClause + `
		GROUP BY c.category
		ORDER BY COUNT(*) DESC, c.category ASC
	`
	rows, err := q.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CategoryRow
	for rows.Next() {
		var r CategoryRow
		if err := rows.Scan(&r.Category, &r.Count); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type LatestDigest struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	WeekStart string    `json:"week_start"`
	WeekEnd   string    `json:"week_end"`
	Content   []byte    `json:"-"`
	CreatedAt time.Time `json:"-"`
}

func (q *SQLiteQuery) GetLatestDigest(ctx context.Context, userID string) (*LatestDigest, error) {
	const sqlStr = `
		SELECT id, user_id, week_start_date, content, created_at
		FROM weekly_digests
		WHERE user_id = ?
		ORDER BY week_start_date DESC
		LIMIT 1
	`
	var d LatestDigest
	var contentStr, createdAtStr sql.NullString
	err := q.db.QueryRowContext(ctx, sqlStr, userID).Scan(&d.ID, &d.UserID, &d.WeekStart, &contentStr, &createdAtStr)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if contentStr.Valid {
		d.Content = []byte(contentStr.String)
	}
	if createdAtStr.Valid {
		d.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr.String)
	}
	if t, perr := time.Parse("2006-01-02", d.WeekStart); perr == nil {
		d.WeekEnd = t.AddDate(0, 0, 6).Format("2006-01-02")
	}
	return &d, nil
}

func roundTo(f float64, n int) float64 {
	mul := 1.0
	for i := 0; i < n; i++ {
		mul *= 10
	}
	if f >= 0 {
		return float64(int64(f*mul+0.5)) / mul
	}
	return float64(int64(f*mul-0.5)) / mul
}
