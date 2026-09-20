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
	UserID                    string
	SubscriptionPlan          *string
	BillingInterval           *string
	StripeCustomerID          *string
	CommentQuotaMonthly       *int
	CommentsUsedThisPeriod    *int
	SubscriptionStatus        *string
	CurrentPeriodEnd          *time.Time
	QuotaResetAt              *time.Time
	PastDueSince              *time.Time
	PastDueSinceClear         bool
	BillingIntervalClear      bool
	StripeSubscriptionID      *string
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
	}
	if !u.BillingIntervalClear && u.BillingInterval != nil {
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
		add("current_period_end", u.CurrentPeriodEnd.UTC().Format(time.RFC3339))
	}
	if u.QuotaResetAt != nil {
		add("quota_reset_at", u.QuotaResetAt.UTC().Format(time.RFC3339))
	}
	if u.PastDueSinceClear {
		sets = append(sets, "past_due_since = NULL")
	}
	if !u.PastDueSinceClear && u.PastDueSince != nil {
		add("past_due_since", u.PastDueSince.UTC().Format(time.RFC3339))
	}
	if u.StripeSubscriptionIDClear {
		sets = append(sets, "stripe_subscription_id = NULL")
	}
	if !u.StripeSubscriptionIDClear && u.StripeSubscriptionID != nil {
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
		userID, periodStart.UTC().Format(time.RFC3339), periodEnd.UTC().Format(time.RFC3339)).Scan(&total)
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
		_, err := tx.ExecContext(ctx, sqlStr, generateUUID(), userID, count, periodStart.UTC().Format(time.RFC3339), periodEnd.UTC().Format(time.RFC3339))
		return err
	}
	_, err := q.db.ExecContext(ctx, sqlStr, generateUUID(), userID, count, periodStart.UTC().Format(time.RFC3339), periodEnd.UTC().Format(time.RFC3339))
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

// IncrementCommentsUsedReturning atomically increments the user's
// current-period comment usage and returns the post-increment total. It
// exists so callers can bill overage from the true post-batch value even
// when several batches for the same user run concurrently, rather than
// from a snapshot read before the increment.
func (q *SQLiteQuery) IncrementCommentsUsedReturning(ctx context.Context, userID string, n int) (int, error) {
	var used int
	err := q.db.QueryRowContext(ctx,
		`UPDATE users
		    SET comments_used_this_period = comments_used_this_period + ?
		  WHERE id = ?
		 RETURNING comments_used_this_period`,
		n, userID).Scan(&used)
	return used, err
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
	users, err := q.ExpirePastDueGraceWithUsers(ctx, graceHours)
	if err != nil {
		return 0, err
	}
	return int64(len(users)), nil
}

type ExpiredGraceUser struct {
	ID    string
	Email string
}

func (q *SQLiteQuery) ExpirePastDueGraceWithUsers(ctx context.Context, graceHours int) ([]ExpiredGraceUser, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(graceHours) * time.Hour).Format(time.RFC3339)
	rows, err := q.db.QueryContext(ctx,
		`UPDATE users
		    SET subscription_status = 'paused',
		        updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		  WHERE subscription_status = 'past_due'
		    AND past_due_since IS NOT NULL
		    AND past_due_since <= ?
		 RETURNING id, email`,
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

// BillingUsage is one row of billing_usage: a per-user per-period snapshot
// of comments_used vs quota (implement-missing-product-features 3.x).
type BillingUsage struct {
	UserID       string
	PeriodStart  time.Time
	PeriodEnd    time.Time
	CommentsUsed int
	Quota        int
	// LLMCostUSDMicro is the cumulative LLM cost for the period in
	// micro-USD (llm-cost-analytics). Converted at the API boundary.
	LLMCostUSDMicro int64
	// PriceVersion is the llm.PriceVersion snapshot of the last cost write
	// in this period; 0 means no cost recorded yet.
	PriceVersion int
}

// UpsertBillingUsage records (or refreshes) a user's current-period usage
// snapshot. Idempotent per (user_id, period_start).
func (q *SQLiteQuery) UpsertBillingUsage(ctx context.Context, u BillingUsage) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO billing_usage (id, user_id, period_start, period_end, comments_used, quota)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id, period_start) DO UPDATE SET
		     period_end = excluded.period_end,
		     comments_used = excluded.comments_used,
		     quota = excluded.quota`,
		generateUUID(), u.UserID, u.PeriodStart.UTC().Format(time.RFC3339),
		u.PeriodEnd.UTC().Format(time.RFC3339), u.CommentsUsed, u.Quota)
	return err
}

// AddLLMCost accumulates a batch's LLM cost (micro-USD) onto the user's
// billing-period rollup row, creating the row when the period is still
// open. Write-time accumulation replaces a daily rollup job (design D2);
// the row is deliberately NOT seeded with comments_used/quota — the
// reset-time UpsertBillingUsage owns those and only overwrites its own
// columns, so the two writers never clobber each other.
//
// ponytail: ceiling is (user, period) granularity — no per-channel DB
// breakdown and no daily rows. Upgrade path: add a channel_id column with
// its own unique key (or a side table) once the dashboard actually shows
// per-channel spend; the ops metric already carries the channel label.
func (q *SQLiteQuery) AddLLMCost(ctx context.Context, userID string, periodStart, periodEnd time.Time, costUSDMicro int64, priceVersion int) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO billing_usage (id, user_id, period_start, period_end, comments_used, quota, llm_cost_usd_micro, price_version)
		 VALUES (?, ?, ?, ?, 0, 0, ?, ?)
		 ON CONFLICT(user_id, period_start) DO UPDATE SET
		     period_end = excluded.period_end,
		     llm_cost_usd_micro = llm_cost_usd_micro + excluded.llm_cost_usd_micro,
		     price_version = excluded.price_version`,
		generateUUID(), userID, periodStart.UTC().Format(time.RFC3339),
		periodEnd.UTC().Format(time.RFC3339), costUSDMicro, priceVersion)
	return err
}

// RecordCurrentPeriodUsage snapshots the current period for every user who
// has a quota schedule (quota_reset_at NOT NULL). The period is anchored at
// quota_reset_at — the next reset — so period_end = quota_reset_at and
// period_start = one month earlier, mirroring the overage-log convention
// (periodStart = periodEnd − 1 month). Returns the number of rows written.
//
// Callers run this BEFORE ResetFreeTierQuotas so the just-finished period's
// usage is captured before the reset zeroes comments_used_this_period.
func (q *SQLiteQuery) RecordCurrentPeriodUsage(ctx context.Context) (int64, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT id, comments_used_this_period, comment_quota_monthly, quota_reset_at
		   FROM users
		  WHERE quota_reset_at IS NOT NULL`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type snap struct {
		userID    string
		used      int
		quota     int
		periodEnd time.Time
	}
	var snaps []snap
	for rows.Next() {
		var s snap
		var resetAt string
		if err := rows.Scan(&s.userID, &s.used, &s.quota, &resetAt); err != nil {
			return 0, err
		}
		s.periodEnd = parseTime(resetAt)
		snaps = append(snaps, s)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	var written int64
	for _, s := range snaps {
		if s.periodEnd.IsZero() {
			continue
		}
		if err := q.UpsertBillingUsage(ctx, BillingUsage{
			UserID:       s.userID,
			PeriodStart:  s.periodEnd.AddDate(0, -1, 0),
			PeriodEnd:    s.periodEnd,
			CommentsUsed: s.used,
			Quota:        s.quota,
		}); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}

// GetBillingUsage returns the user's most recent usage periods, newest
// first. limit must be >= 1.
func (q *SQLiteQuery) GetBillingUsage(ctx context.Context, userID string, limit int) ([]BillingUsage, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT user_id, period_start, period_end, comments_used, quota, llm_cost_usd_micro, price_version
		   FROM billing_usage
		  WHERE user_id = ?
		  ORDER BY period_start DESC
		  LIMIT ?`,
		userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BillingUsage
	for rows.Next() {
		var u BillingUsage
		var ps, pe string
		if err := rows.Scan(&u.UserID, &ps, &pe, &u.CommentsUsed, &u.Quota, &u.LLMCostUSDMicro, &u.PriceVersion); err != nil {
			return nil, err
		}
		u.PeriodStart = parseTime(ps)
		u.PeriodEnd = parseTime(pe)
		out = append(out, u)
	}
	return out, rows.Err()
}

// OverageGroup is one uninvoiced (user, period) overage bucket, aggregated
// from comment_usage_logs (implement-missing-product-features 8.x).
type OverageGroup struct {
	UserID      string
	Plan        string
	PeriodStart time.Time
	PeriodEnd   time.Time
	Overage     int
}

// ListOverageGroups returns overage totals grouped by (user, period) for
// plans that track overage (creator/studio), excluding periods already
// recorded in overage_invoices. Exactly-once invoicing relies on this
// exclusion plus the UNIQUE(user_id, period_start) guard.
func (q *SQLiteQuery) ListOverageGroups(ctx context.Context) ([]OverageGroup, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT u.id, u.subscription_plan, l.period_start, l.period_end, SUM(l.count) AS overage
		   FROM comment_usage_logs l
		   JOIN users u ON u.id = l.user_id
		   LEFT JOIN overage_invoices oi
		     ON oi.user_id = l.user_id AND oi.period_start = l.period_start
		  WHERE u.subscription_plan IN ('creator', 'studio')
		    AND oi.id IS NULL
		  GROUP BY u.id, u.subscription_plan, l.period_start, l.period_end`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OverageGroup
	for rows.Next() {
		var g OverageGroup
		var ps, pe string
		if err := rows.Scan(&g.UserID, &g.Plan, &ps, &pe, &g.Overage); err != nil {
			return nil, err
		}
		g.PeriodStart = parseTime(ps)
		g.PeriodEnd = parseTime(pe)
		out = append(out, g)
	}
	return out, rows.Err()
}

// MarkOverageInvoiced records the exactly-once guard row for a charged
// (user, period). Idempotent: a second call for the same key is a no-op.
func (q *SQLiteQuery) MarkOverageInvoiced(ctx context.Context, userID, periodStart, stripeInvoiceID string, amountCents int) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO overage_invoices (id, user_id, period_start, stripe_invoice_id, amount_cents)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(user_id, period_start) DO NOTHING`,
		generateUUID(), userID, periodStart, stripeInvoiceID, amountCents)
	return err
}

// IsOverageInvoiced reports whether a (user, period) already has a guard row.
func (q *SQLiteQuery) IsOverageInvoiced(ctx context.Context, userID, periodStart string) (bool, error) {
	var n int
	err := q.db.QueryRowContext(ctx,
		`SELECT 1 FROM overage_invoices WHERE user_id = ? AND period_start = ?`,
		userID, periodStart).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
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
		lo.UTC().Format(time.RFC3339), hi.UTC().Format(time.RFC3339))
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
