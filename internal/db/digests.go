package db

import (
	"context"
	"time"
)

func (q *PostgresQuery) InsertWeeklyDigest(ctx context.Context, digest *WeeklyDigest) error {
	query := `
		INSERT INTO weekly_digests (user_id, week_start_date, status, content, created_at)
		VALUES ($1, $2, $3, $4, NOW())
		RETURNING id, created_at
	`
	return q.pool.QueryRow(ctx, query, digest.UserID, digest.WeekStartDate, digest.Status, digest.Content).Scan(&digest.ID, &digest.CreatedAt)
}

func (q *PostgresQuery) GetUsersForDigest(ctx context.Context) ([]*User, error) {
	// A simple query to get all users who should receive digests
	query := `
		SELECT id, email, subscription_plan, comment_quota_monthly, comments_used_this_period, created_at, updated_at
		FROM users
		WHERE email_verified_at IS NOT NULL
	`
	rows, err := q.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		var u User
		if err := rows.Scan(
			&u.ID, &u.Email, &u.SubscriptionPlan, &u.CommentQuotaMonthly, &u.CommentsUsedThisPeriod, &u.CreatedAt, &u.UpdatedAt,
		); err != nil {
			return nil, err
		}
		users = append(users, &u)
	}
	return users, nil
}

func (q *PostgresQuery) GetRecentProcessedCommentsForUser(ctx context.Context, userID string, since time.Time) ([]*Comment, error) {
	query := `
		SELECT c.id, c.platform_channel_id, c.platform_comment_id, c.parent_comment_id, c.author_handle, c.body, c.posted_at, c.category, c.sentiment_score, c.topic, c.confidence, c.processed_at, c.created_at
		FROM comments c
		JOIN user_channel_tracking uct ON c.platform_channel_id = uct.platform_channel_id
		WHERE uct.user_id = $1 AND uct.tracking_status = 'active'
		  AND c.processed_at >= $2
		  AND c.category NOT IN ('Criticism (Toxic)', 'Spam', 'Other') 
		ORDER BY c.posted_at DESC
	`
	rows, err := q.pool.Query(ctx, query, userID, since)
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
