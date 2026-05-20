// Package db: comments_feed_db.go contains the runtime DB methods that wrap
// BuildFeedQuery with statement_timeout, scanning, and error mapping. They
// constitute the read surface used by the insights-api HTTP handlers.
package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// FeedRow is the row shape projected by ModeFeed/ModeStream queries. It
// extends Comment with channel_name and is_starred (the two columns added
// by the BuildFeedQuery projection).
type FeedRow struct {
	Comment
	ChannelName string `json:"channel_name"`
	IsStarred   bool   `json:"is_starred"`
}

// ErrCountTimeout is returned by CountCommentFeed when the COUNT(*) query
// exceeds the configured statement_timeout. Handlers MUST treat this as a
// signal to fall back to total=null and "len(rows)==limit" pagination.
var ErrCountTimeout = errors.New("count query timeout")

// ErrFeedTimeout is returned by QueryCommentFeed when the feed query exceeds
// the configured statement_timeout. Handlers MUST translate this to a
// 408 Request Timeout response with code feed_query_timeout.
var ErrFeedTimeout = errors.New("feed query timeout")

// withStatementTimeout was an early helper; runtime callers use a Tx + SET
// LOCAL inline. Kept here only as a documented reminder of the technique.

// QueryCommentFeed executes BuildFeedQuery in ModeFeed and scans the rows.
// It applies cfg.FeedQueryTimeout via SET LOCAL statement_timeout. On
// timeout, returns ErrFeedTimeout.
func (q *PostgresQuery) QueryCommentFeed(ctx context.Context, userID string, filters FeedFilters, timeout time.Duration) ([]*FeedRow, error) {
	sql, args := BuildFeedQuery(userID, filters, ModeFeed)
	return q.runFeedQuery(ctx, sql, args, timeout)
}

// QuerySavedFeed is the /saved variant: builder forces ModeSaved.
func (q *PostgresQuery) QuerySavedFeed(ctx context.Context, userID string, filters FeedFilters, timeout time.Duration) ([]*FeedRow, error) {
	sql, args := BuildFeedQuery(userID, filters, ModeSaved)
	return q.runFeedQuery(ctx, sql, args, timeout)
}

func (q *PostgresQuery) runFeedQuery(ctx context.Context, sql string, args []any, timeout time.Duration) ([]*FeedRow, error) {
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL statement_timeout = %d", timeout.Milliseconds())); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		if isStatementTimeout(err) {
			return nil, ErrFeedTimeout
		}
		return nil, err
	}
	defer rows.Close()
	var out []*FeedRow
	for rows.Next() {
		r := &FeedRow{}
		if err := rows.Scan(
			&r.ID, &r.PlatformChannelID, &r.PlatformCommentID, &r.ParentCommentID,
			&r.AuthorHandle, &r.Body, &r.PostedAt, &r.Category, &r.SentimentScore,
			&r.Topic, &r.Confidence, &r.ProcessedAt, &r.CreatedAt,
			&r.ChannelName, &r.IsStarred,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		if isStatementTimeout(err) {
			return nil, ErrFeedTimeout
		}
		return nil, err
	}
	return out, tx.Commit(ctx)
}

// CountCommentFeed returns the total row count for the given filter set
// using ModeCount. On statement-timeout, returns (-1, ErrCountTimeout) so
// the handler can fall back to total=null. Task 4.2.
func (q *PostgresQuery) CountCommentFeed(ctx context.Context, userID string, filters FeedFilters, timeout time.Duration) (int64, error) {
	sql, args := BuildFeedQuery(userID, filters, ModeCount)
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL statement_timeout = %d", timeout.Milliseconds())); err != nil {
		return 0, err
	}
	var n int64
	if err := tx.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		if isStatementTimeout(err) {
			return -1, ErrCountTimeout
		}
		return 0, err
	}
	return n, tx.Commit(ctx)
}

// isStatementTimeout matches Postgres SQLSTATE 57014 (query_canceled), which
// is what statement_timeout raises. We check both the wrapped pgx PgError
// and a substring match for portability.
func isStatementTimeout(err error) bool {
	if err == nil {
		return false
	}
	// pgx PgError code
	type coder interface{ SQLState() string }
	var cerr coder
	if errors.As(err, &cerr) && cerr.SQLState() == "57014" {
		return true
	}
	return strings.Contains(err.Error(), "canceling statement due to statement timeout")
}

// StarCommentScoped is the access-controlled star insert. The INSERT ... SELECT
// form gates the row on a successful access predicate; foreign comments yield
// 0 rows affected and the handler returns 404. Task 4.3.
//
// The legacy StarComment helper (defined in comments.go) is kept for
// pre-existing callers but is bypassed by the new /api/comments/{id}/star
// route, which uses this scoped variant.
func (q *PostgresQuery) StarCommentScoped(ctx context.Context, userID, commentID string) (bool, error) {
	const sql = `
		INSERT INTO user_comment_stars (user_id, comment_id, starred_at)
		SELECT $1, $2, NOW()
		WHERE EXISTS (
			SELECT 1
			FROM comments c
			JOIN user_channel_tracking uct ON uct.platform_channel_id = c.platform_channel_id
			WHERE c.id = $2
			  AND uct.user_id = $1
			  AND uct.tracking_status IN ('active','paused')
		)
		ON CONFLICT (user_id, comment_id) DO NOTHING
	`
	tag, err := q.pool.Exec(ctx, sql, userID, commentID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() > 0 {
		return true, nil
	}
	// 0 rows affected → either ON CONFLICT (already starred → still
	// successful, idempotent) OR access denied. Distinguish via
	// UserHasAccessToComment.
	hasAccess, aerr := q.UserHasAccessToComment(ctx, userID, commentID)
	if aerr != nil {
		return false, aerr
	}
	return hasAccess, nil
}

// UnstarComment removes a star, but only after an access-control check. It
// returns hasAccess=true even when no row existed (idempotent DELETE). 4.3.
func (q *PostgresQuery) UnstarComment(ctx context.Context, userID, commentID string) (bool, error) {
	hasAccess, err := q.UserHasAccessToComment(ctx, userID, commentID)
	if err != nil {
		return false, err
	}
	if !hasAccess {
		return false, nil
	}
	_, err = q.pool.Exec(ctx,
		`DELETE FROM user_comment_stars WHERE user_id = $1 AND comment_id = $2`,
		userID, commentID)
	return true, err
}

// UserHasAccessToComment is the access-control pre-check used by star/unstar
// and the export download endpoint. Task 4.4.
func (q *PostgresQuery) UserHasAccessToComment(ctx context.Context, userID, commentID string) (bool, error) {
	const sql = `
		SELECT EXISTS (
			SELECT 1
			FROM comments c
			JOIN user_channel_tracking uct ON uct.platform_channel_id = c.platform_channel_id
			WHERE c.id = $2
			  AND uct.user_id = $1
			  AND uct.tracking_status IN ('active','paused')
		)
	`
	var ok bool
	err := q.pool.QueryRow(ctx, sql, userID, commentID).Scan(&ok)
	return ok, err
}

// UserTracksChannel returns true when the caller has any active or paused
// tracking row for the given platform_channel_id. Task 4.9.
func (q *PostgresQuery) UserTracksChannel(ctx context.Context, userID, channelID string) (bool, error) {
	const sql = `
		SELECT EXISTS (
			SELECT 1
			FROM user_channel_tracking
			WHERE user_id = $1
			  AND platform_channel_id = $2
			  AND tracking_status IN ('active','paused')
		)
	`
	var ok bool
	err := q.pool.QueryRow(ctx, sql, userID, channelID).Scan(&ok)
	return ok, err
}

// SentimentBucket is a single rolling-window result for /api/insights/sentiment.
type SentimentBucket struct {
	Avg          float64 `json:"avg"`
	Count        int64   `json:"count"`
	PositivePct  float64 `json:"positive_pct"`
	NegativePct  float64 `json:"negative_pct"`
	NeutralPct   float64 `json:"neutral_pct"`
}

// ToxicityBucket is a single rolling-window result for /toxicity-shield.
type ToxicityBucket struct {
	TotalComments int64   `json:"total_comments"`
	FilteredCount int64   `json:"filtered_count"`
	FilteredPct   float64 `json:"filtered_pct"`
}

// TopicRow is one row in the /trends top_topics array.
type TopicRow struct {
	Topic        string  `json:"topic"`
	Count        int64   `json:"count"`
	AvgSentiment float64 `json:"avg_sentiment"`
}

// CategoryRow is one row in the /trends top_categories array.
type CategoryRow struct {
	Category string `json:"category"`
	Count    int64  `json:"count"`
}

// GetSentimentAggregates computes the ±0.3 buckets, excluding toxic/spam,
// over the caller's accessible comments. Task 4.5.
//
// channelID is optional; when nil, the aggregate spans every tracked channel.
func (q *PostgresQuery) GetSentimentAggregates(ctx context.Context, userID string, channelID *string, since time.Time) (SentimentBucket, error) {
	args := []any{userID, since}
	channelClause := ""
	if channelID != nil {
		args = append(args, *channelID)
		channelClause = " AND c.platform_channel_id = $3"
	}
	sql := `
		SELECT
		  COALESCE(AVG(c.sentiment_score)::float8, 0),
		  COUNT(*),
		  COUNT(*) FILTER (WHERE c.sentiment_score > 0.3),
		  COUNT(*) FILTER (WHERE c.sentiment_score < -0.3),
		  COUNT(*) FILTER (WHERE c.sentiment_score BETWEEN -0.3 AND 0.3)
		FROM comments c
		JOIN user_channel_tracking uct ON uct.platform_channel_id = c.platform_channel_id
		WHERE uct.user_id = $1
		  AND uct.tracking_status IN ('active','paused')
		  AND c.processed_at IS NOT NULL
		  AND c.processed_at >= $2
		  AND c.category NOT IN ('toxic_criticism','spam')` + channelClause
	var b SentimentBucket
	var pos, neg, neu int64
	if err := q.pool.QueryRow(ctx, sql, args...).Scan(&b.Avg, &b.Count, &pos, &neg, &neu); err != nil {
		return SentimentBucket{}, err
	}
	if b.Count > 0 {
		f := float64(b.Count)
		b.PositivePct = roundTo(float64(pos)/f*100, 1)
		b.NegativePct = roundTo(float64(neg)/f*100, 1)
		b.NeutralPct = roundTo(float64(neu)/f*100, 1)
	}
	b.Avg = roundTo(b.Avg, 2)
	return b, nil
}

// GetToxicityShield: denominator INCLUDES toxic+spam, numerator counts
// toxic_criticism+spam. Task 4.6.
func (q *PostgresQuery) GetToxicityShield(ctx context.Context, userID string, channelID *string, since time.Time) (ToxicityBucket, error) {
	args := []any{userID, since}
	channelClause := ""
	if channelID != nil {
		args = append(args, *channelID)
		channelClause = " AND c.platform_channel_id = $3"
	}
	sql := `
		SELECT
		  COUNT(*),
		  COUNT(*) FILTER (WHERE c.category IN ('toxic_criticism','spam'))
		FROM comments c
		JOIN user_channel_tracking uct ON uct.platform_channel_id = c.platform_channel_id
		WHERE uct.user_id = $1
		  AND uct.tracking_status IN ('active','paused')
		  AND c.processed_at IS NOT NULL
		  AND c.processed_at >= $2` + channelClause
	var b ToxicityBucket
	if err := q.pool.QueryRow(ctx, sql, args...).Scan(&b.TotalComments, &b.FilteredCount); err != nil {
		return ToxicityBucket{}, err
	}
	if b.TotalComments > 0 {
		b.FilteredPct = roundTo(float64(b.FilteredCount)/float64(b.TotalComments)*100, 1)
	}
	return b, nil
}

// GetTopTopics ranks non-empty topics by count over a posted_at window,
// excluding toxic/spam. Task 4.7.
func (q *PostgresQuery) GetTopTopics(ctx context.Context, userID string, channelID *string, days int, limit int) ([]TopicRow, error) {
	args := []any{userID, days}
	channelClause := ""
	if channelID != nil {
		args = append(args, *channelID)
		channelClause = " AND c.platform_channel_id = $" + fmt.Sprint(len(args))
	}
	args = append(args, limit)
	limitParam := fmt.Sprintf("$%d", len(args))
	sql := `
		SELECT c.topic, COUNT(*)::bigint, COALESCE(AVG(c.sentiment_score)::float8, 0)
		FROM comments c
		JOIN user_channel_tracking uct ON uct.platform_channel_id = c.platform_channel_id
		WHERE uct.user_id = $1
		  AND uct.tracking_status IN ('active','paused')
		  AND c.processed_at IS NOT NULL
		  AND c.posted_at >= NOW() - make_interval(days => $2)
		  AND c.category NOT IN ('toxic_criticism','spam')
		  AND c.topic IS NOT NULL AND c.topic <> ''` + channelClause + `
		GROUP BY c.topic
		ORDER BY COUNT(*) DESC, c.topic ASC
		LIMIT ` + limitParam
	rows, err := q.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TopicRow
	for rows.Next() {
		var r TopicRow
		if err := rows.Scan(&r.Topic, &r.Count, &r.AvgSentiment); err != nil {
			return nil, err
		}
		r.AvgSentiment = roundTo(r.AvgSentiment, 2)
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetTopCategories ranks every non-toxic/spam category by count over a
// posted_at window. Task 4.8.
func (q *PostgresQuery) GetTopCategories(ctx context.Context, userID string, channelID *string, days int) ([]CategoryRow, error) {
	args := []any{userID, days}
	channelClause := ""
	if channelID != nil {
		args = append(args, *channelID)
		channelClause = " AND c.platform_channel_id = $3"
	}
	sql := `
		SELECT c.category, COUNT(*)::bigint
		FROM comments c
		JOIN user_channel_tracking uct ON uct.platform_channel_id = c.platform_channel_id
		WHERE uct.user_id = $1
		  AND uct.tracking_status IN ('active','paused')
		  AND c.processed_at IS NOT NULL
		  AND c.posted_at >= NOW() - make_interval(days => $2)
		  AND c.category IS NOT NULL
		  AND c.category NOT IN ('toxic_criticism','spam')` + channelClause + `
		GROUP BY c.category
		ORDER BY COUNT(*) DESC, c.category ASC
	`
	rows, err := q.pool.Query(ctx, sql, args...)
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

// GetLatestDigest returns the most recent weekly_digests row for the user.
// Task 4.17. Returns ErrNotFound when none exists.
//
// NOTE: The current weekly_digests schema stores `content` as JSONB; the
// insights-api spec calls out distinct fields top_ideas/top_questions/
// sentiment_trend/health_score/week_start/week_end. We surface them by
// returning the JSONB blob plus week_start/created_at; the handler unwraps
// the blob into the documented response shape.
type LatestDigest struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	WeekStart   string    `json:"week_start"`
	WeekEnd     string    `json:"week_end"`
	Content     []byte    `json:"-"`
	CreatedAt   time.Time `json:"-"`
}

func (q *PostgresQuery) GetLatestDigest(ctx context.Context, userID string) (*LatestDigest, error) {
	const sql = `
		SELECT id::text, user_id::text, week_start_date::text, content::text, created_at
		FROM weekly_digests
		WHERE user_id = $1
		ORDER BY week_start_date DESC
		LIMIT 1
	`
	var d LatestDigest
	var contentStr string
	err := q.pool.QueryRow(ctx, sql, userID).Scan(&d.ID, &d.UserID, &d.WeekStart, &contentStr, &d.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	d.Content = []byte(contentStr)
	// week_end: derive as week_start + 6 days for surfacing in the response.
	if t, perr := time.Parse("2006-01-02", d.WeekStart); perr == nil {
		d.WeekEnd = t.AddDate(0, 0, 6).Format("2006-01-02")
	}
	return &d, nil
}

// roundTo rounds f to n decimal places using banker's truncation (good
// enough for percentage display).
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
