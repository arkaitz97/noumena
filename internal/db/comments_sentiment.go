package db

import (
	"context"
	"time"
)

// SentimentDay is one per-day aggregate row for the sentiment trend
// endpoint. Date is the UTC day (YYYY-MM-DD) the comments were posted on YouTube;
// Score is the sum of sentiment_score (-1/0/1) across that day.
type SentimentDay struct {
	Date     string `json:"date"`
	Negative int64  `json:"negative"`
	Neutral  int64  `json:"neutral"`
	Positive int64  `json:"positive"`
	Score    int64  `json:"score"`
}

// UpdateCommentSentiment persists a comment's LLM sentiment score (-1/0/1).
func (q *SQLiteQuery) UpdateCommentSentiment(ctx context.Context, id string, score int) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE comments SET sentiment_score = ? WHERE id = ?`, score, id)
	return err
}

// GetSentimentSeries returns per-day sentiment aggregates over the trailing
// `days` window. When channelID is non-empty, rows are scoped to that one
// channel; when empty, the series aggregates across every channel the caller
// tracks. Access control is enforced at the SQL layer: comments are joined to
// user_channel_tracking, so rows belonging to channels the user does not
// track are invisible. Days without scored comments are omitted.
func (q *SQLiteQuery) GetSentimentSeries(ctx context.Context, userID, channelID string, days int) ([]SentimentDay, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format(time.RFC3339)
	channelClause := ""
	args := []any{userID}
	if channelID != "" {
		channelClause = " AND c.platform_channel_id = ?"
		args = append(args, channelID)
	}
	args = append(args, cutoff)
	query := `
		SELECT date(c.posted_at),
		       COALESCE(SUM(CASE WHEN c.sentiment_score = -1 THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN c.sentiment_score = 0 THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN c.sentiment_score = 1 THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(c.sentiment_score), 0)
		FROM comments c
		JOIN user_channel_tracking uct ON uct.platform_channel_id = c.platform_channel_id
		WHERE uct.user_id = ?
		  AND uct.tracking_status IN ('active','paused')
		` + channelClause + `
		  AND c.sentiment_score IS NOT NULL
		  AND c.posted_at >= ?
		GROUP BY date(c.posted_at)
		ORDER BY date(c.posted_at) ASC
	`
	rows, err := q.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SentimentDay
	for rows.Next() {
		var d SentimentDay
		if err := rows.Scan(&d.Date, &d.Negative, &d.Neutral, &d.Positive, &d.Score); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
