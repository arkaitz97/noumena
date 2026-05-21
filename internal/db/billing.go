package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func (q *SQLiteQuery) IsStripeEventProcessed(ctx context.Context, eventID string) (bool, error) {
	var n int
	err := q.db.QueryRowContext(ctx,
		`SELECT 1 FROM processed_stripe_events WHERE stripe_event_id = ?`,
		eventID).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (q *SQLiteQuery) InsertProcessedStripeEvent(ctx context.Context, tx *sql.Tx, eventID, eventType string) error {
	exec := func(sqlStr string, args ...any) error {
		if tx != nil {
			_, err := tx.ExecContext(ctx, sqlStr, args...)
			return err
		}
		_, err := q.db.ExecContext(ctx, sqlStr, args...)
		return err
	}
	return exec(
		`INSERT INTO processed_stripe_events (stripe_event_id, event_type)
		 VALUES (?, ?) ON CONFLICT (stripe_event_id) DO NOTHING`,
		eventID, eventType)
}

type SubscriptionUpdate struct {
	UserID                 string
	SubscriptionPlan       *string
	BillingInterval        *string
	StripeCustomerID       *string
	CommentQuotaMonthly    *int
	CommentsUsedThisPeriod *int
	SubscriptionStatus     *string
	CurrentPeriodEnd       *time.Time
	QuotaResetAt           *time.Time
	PastDueSince           *time.Time
	PastDueSinceClear      bool
	BillingIntervalClear   bool
	StripeSubscriptionID   *string
	StripeSubscriptionIDClear bool
}

func (q *SQLiteQuery) ApplySubscriptionUpdate(ctx context.Context, tx *sql.Tx, u SubscriptionUpdate) error {
	if u.UserID == "" {
		return errors.New("UserID is required")
	}
	sets := []string{}
	args := []any{}
	add := func(col string, val any) {
		args = append(args, val)
		sets = append(sets, fmt.Sprintf("%s = ?", col))
	}
	if u.SubscriptionPlan != nil {
		add("subscription_plan", *u.SubscriptionPlan)
	}
	if u.BillingIntervalClear {
		sets = append(sets, "billing_interval = NULL")
	} else if u.BillingInterval != nil {
		add("billing_interval", *u.BillingInterval)
	}
	if u.StripeCustomerID != nil {
		add("stripe_customer_id", *u.StripeCustomerID)
	}
	if u.CommentQuotaMonthly != nil {
		add("comment_quota_monthly", *u.CommentQuotaMonthly)
	}
	if u.CommentsUsedThisPeriod != nil {
		add("comments_used_this_period", *u.CommentsUsedThisPeriod)
	}
	if u.SubscriptionStatus != nil {
		add("subscription_status", *u.SubscriptionStatus)
	}
	if u.CurrentPeriodEnd != nil {
		add("current_period_end", u.CurrentPeriodEnd.Format(time.RFC3339))
	}
	if u.QuotaResetAt != nil {
		add("quota_reset_at", u.QuotaResetAt.Format(time.RFC3339))
	}
	if u.PastDueSinceClear {
		sets = append(sets, "past_due_since = NULL")
	} else if u.PastDueSince != nil {
		add("past_due_since", u.PastDueSince.Format(time.RFC3339))
	}
	if u.StripeSubscriptionIDClear {
		sets = append(sets, "stripe_subscription_id = NULL")
	} else if u.StripeSubscriptionID != nil {
		add("stripe_subscription_id", *u.StripeSubscriptionID)
	}
	if len(sets) == 0 {
		return nil
	}
	sqlStr := "UPDATE users SET updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')"
	for _, s := range sets {
		sqlStr += ", " + s
	}
	sqlStr += " WHERE id = ?"
	args = append(args, u.UserID)

	if tx != nil {
		_, err := tx.ExecContext(ctx, sqlStr, args...)
		return err
	}
	_, err := q.db.ExecContext(ctx, sqlStr, args...)
	return err
}

func (q *SQLiteQuery) GetUserByStripeCustomerID(ctx context.Context, customerID string) (*User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE stripe_customer_id = ?`
	u, err := scanUser(q.db.QueryRowContext(ctx, query, customerID))
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return u, err
}

func (q *SQLiteQuery) GetActiveChannelCount(ctx context.Context, userID string) (int, error) {
	var n int
	err := q.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_channel_tracking
		 WHERE user_id = ? AND tracking_status IN ('active','paused')`,
		userID).Scan(&n)
	return n, err
}

func (q *SQLiteQuery) GetOverageCostForCurrentPeriod(ctx context.Context, userID string, periodStart, periodEnd time.Time) (float64, error) {
	var total int
	err := q.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(count), 0) FROM comment_usage_logs
		 WHERE user_id = ? AND period_start >= ? AND period_start < ?`,
		userID, periodStart.Format(time.RFC3339), periodEnd.Format(time.RFC3339)).Scan(&total)
	if err != nil {
		return 0, err
	}
	if total <= 0 {
		return 0, nil
	}
	chunks := (total + 999) / 1000
	return float64(chunks) * 0.50, nil
}

func (q *SQLiteQuery) InsertCommentUsageLog(ctx context.Context, tx *sql.Tx, userID string, count int, periodStart, periodEnd time.Time) error {
	sqlStr := `INSERT INTO comment_usage_logs (id, user_id, count, period_start, period_end)
	        VALUES (?, ?, ?, ?, ?)`
	if tx != nil {
		_, err := tx.ExecContext(ctx, sqlStr, generateUUID(), userID, count, periodStart.Format(time.RFC3339), periodEnd.Format(time.RFC3339))
		return err
	}
	_, err := q.db.ExecContext(ctx, sqlStr, generateUUID(), userID, count, periodStart.Format(time.RFC3339), periodEnd.Format(time.RFC3339))
	return err
}

func (q *SQLiteQuery) PauseChannelsAlphabetically(ctx context.Context, tx *sql.Tx, userID string, keepCount int) error {
	if keepCount < 0 {
		keepCount = 0
	}
	sqlStr := `
		UPDATE user_channel_tracking
		   SET tracking_status = 'paused'
		 WHERE id IN (
		     SELECT uct.id
		       FROM user_channel_tracking uct
		       JOIN platform_channels pc ON pc.id = uct.platform_channel_id
		      WHERE uct.user_id = ?
		        AND uct.tracking_status = 'active'
		      ORDER BY pc.display_name ASC, uct.id ASC
		      LIMIT -1 OFFSET ?
		 )`
	if tx != nil {
		_, err := tx.ExecContext(ctx, sqlStr, userID, keepCount)
		return err
	}
	_, err := q.db.ExecContext(ctx, sqlStr, userID, keepCount)
	return err
}

func (q *SQLiteQuery) IncrementCommentsUsedTx(ctx context.Context, tx *sql.Tx, userID string, count int) (int, error) {
	var used int
	err := tx.QueryRowContext(ctx,
		`UPDATE users
		    SET comments_used_this_period = comments_used_this_period + ?,
		        updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		  WHERE id = ?
		 RETURNING comments_used_this_period`,
		count, userID).Scan(&used)
	return used, err
}

func (q *SQLiteQuery) ResetFreeTierQuotas(ctx context.Context) (int64, error) {
	quotaReset := time.Now().UTC().AddDate(0, 1, 0).Format(time.RFC3339)
	result, err := q.db.ExecContext(ctx,
		`UPDATE users
		    SET comments_used_this_period = 0,
		        quota_reset_at = ?,
		        updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		  WHERE subscription_plan = 'free'
		    AND quota_reset_at IS NOT NULL
		    AND quota_reset_at <= strftime('%Y-%m-%dT%H:%M:%SZ', 'now')`, quotaReset)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (q *SQLiteQuery) ExpirePastDueGrace(ctx context.Context, graceHours int) (int64, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(graceHours) * time.Hour).Format(time.RFC3339)
	result, err := q.db.ExecContext(ctx,
		`UPDATE users
		    SET subscription_status = 'paused',
		        updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		  WHERE subscription_status = 'past_due'
		    AND past_due_since IS NOT NULL
		    AND past_due_since <= ?`,
		cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

type ExpiredGraceUser struct {
	ID    string
	Email string
}

func (q *SQLiteQuery) ExpirePastDueGraceWithUsers(ctx context.Context, graceHours int) ([]ExpiredGraceUser, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(graceHours) * time.Hour).Format(time.RFC3339)
	rows, err := q.db.QueryContext(ctx,
		`SELECT id, email
		   FROM users
		  WHERE subscription_status = 'past_due'
		    AND past_due_since IS NOT NULL
		    AND past_due_since <= ?`,
		cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ExpiredGraceUser
	for rows.Next() {
		var u ExpiredGraceUser
		if err := rows.Scan(&u.ID, &u.Email); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

type TrialReminderUser struct {
	ID          string
	Email       string
	TrialEndsAt time.Time
}

func (q *SQLiteQuery) ListUsersForTrialExpiryReminder(ctx context.Context, lo, hi time.Time) ([]TrialReminderUser, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT id, email, trial_ends_at
		   FROM users
		  WHERE subscription_status = 'trialing'
		    AND trial_ends_at IS NOT NULL
		    AND trial_ends_at >  ?
		    AND trial_ends_at <= ?
		    AND deletion_requested_at IS NULL`,
		lo.Format(time.RFC3339), hi.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TrialReminderUser
	for rows.Next() {
		var u TrialReminderUser
		var trialEndsAt string
		if err := rows.Scan(&u.ID, &u.Email, &trialEndsAt); err != nil {
			return nil, err
		}
		u.TrialEndsAt = parseTime(trialEndsAt)
		out = append(out, u)
	}
	return out, rows.Err()
}
