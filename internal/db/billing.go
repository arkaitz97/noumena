package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ============================================================================
// billing change: Stripe webhook idempotency, subscription state mutations,
// channel-pause primitive, billing-status read helpers.
//
// All methods here are added on *PostgresQuery. The DBQuery interface
// (see db.go) exposes the corresponding contract for handlers and workers.
// ============================================================================

// IsStripeEventProcessed reports whether the given Stripe event id has
// already been processed (and an audit row inserted). Webhook handlers
// short-circuit on true to make event delivery idempotent.
func (q *PostgresQuery) IsStripeEventProcessed(ctx context.Context, eventID string) (bool, error) {
	var n int
	err := q.pool.QueryRow(ctx,
		`SELECT 1 FROM processed_stripe_events WHERE stripe_event_id = $1`,
		eventID).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// InsertProcessedStripeEvent inserts an audit row for a successfully-handled
// (or no-op) Stripe webhook delivery. Conflict on the primary key is
// reported back so callers can detect a redelivery race.
func (q *PostgresQuery) InsertProcessedStripeEvent(ctx context.Context, tx pgx.Tx, eventID, eventType string) error {
	exec := func(sql string, args ...any) error {
		if tx != nil {
			_, err := tx.Exec(ctx, sql, args...)
			return err
		}
		_, err := q.pool.Exec(ctx, sql, args...)
		return err
	}
	return exec(
		`INSERT INTO processed_stripe_events (stripe_event_id, event_type)
		 VALUES ($1, $2) ON CONFLICT (stripe_event_id) DO NOTHING`,
		eventID, eventType)
}

// SubscriptionUpdate carries the fields written by the Stripe webhook
// handlers. Pointer fields that are nil are left unchanged. The shape
// keeps the SQL straightforward while letting each event handler scope
// its mutation.
type SubscriptionUpdate struct {
	UserID                 string
	SubscriptionPlan       *string
	BillingInterval        *string // pass &"" to clear (set NULL via empty)
	StripeCustomerID       *string
	CommentQuotaMonthly    *int
	CommentsUsedThisPeriod *int
	SubscriptionStatus     *string
	CurrentPeriodEnd       *time.Time
	QuotaResetAt           *time.Time
	PastDueSince           *time.Time
	// PastDueSinceClear sets past_due_since to NULL when true.
	PastDueSinceClear bool
	// BillingIntervalClear sets billing_interval to NULL when true.
	BillingIntervalClear bool
	// StripeSubscriptionID, when non-nil, sets users.stripe_subscription_id
	// to the dereferenced value. Written by checkout.session.completed and
	// customer.subscription.{created,updated}.
	StripeSubscriptionID *string
	// StripeSubscriptionIDClear, when true, sets users.stripe_subscription_id
	// to NULL. Written by customer.subscription.deleted alongside the
	// free-plan downgrade. Takes precedence over StripeSubscriptionID.
	StripeSubscriptionIDClear bool
}

// ApplySubscriptionUpdate writes the SubscriptionUpdate inside the supplied
// transaction (or against the pool if tx is nil). Only non-nil/non-zero
// fields are emitted into the SQL.
func (q *PostgresQuery) ApplySubscriptionUpdate(ctx context.Context, tx pgx.Tx, u SubscriptionUpdate) error {
	if u.UserID == "" {
		return errors.New("UserID is required")
	}
	sets := []string{}
	args := []any{u.UserID}
	add := func(col string, val any) {
		args = append(args, val)
		sets = append(sets, fmt.Sprintf("%s = $%d", col, len(args)))
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
		add("current_period_end", *u.CurrentPeriodEnd)
	}
	if u.QuotaResetAt != nil {
		add("quota_reset_at", *u.QuotaResetAt)
	}
	if u.PastDueSinceClear {
		sets = append(sets, "past_due_since = NULL")
	} else if u.PastDueSince != nil {
		add("past_due_since", *u.PastDueSince)
	}
	if u.StripeSubscriptionIDClear {
		sets = append(sets, "stripe_subscription_id = NULL")
	} else if u.StripeSubscriptionID != nil {
		add("stripe_subscription_id", *u.StripeSubscriptionID)
	}
	if len(sets) == 0 {
		return nil
	}
	sql := "UPDATE users SET updated_at = NOW()"
	for _, s := range sets {
		sql += ", " + s
	}
	sql += " WHERE id = $1"

	if tx != nil {
		_, err := tx.Exec(ctx, sql, args...)
		return err
	}
	_, err := q.pool.Exec(ctx, sql, args...)
	return err
}

// GetUserByStripeCustomerID resolves a user from a Stripe customer id —
// the only reliable identifier on invoice.* events.
func (q *PostgresQuery) GetUserByStripeCustomerID(ctx context.Context, customerID string) (*User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE stripe_customer_id = $1`
	u, err := scanUser(q.pool.QueryRow(ctx, query, customerID))
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return u, err
}

// GetActiveChannelCount returns the number of channels the user currently
// has in tracking_status active or paused. Used by the channel-connect
// guard and the GET /api/billing/status response.
func (q *PostgresQuery) GetActiveChannelCount(ctx context.Context, userID string) (int, error) {
	var n int
	err := q.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_channel_tracking
		 WHERE user_id = $1 AND tracking_status IN ('active','paused')`,
		userID).Scan(&n)
	return n, err
}

// GetOverageCostForCurrentPeriod sums the count column of comment_usage_logs
// for the user inside [periodStart, periodEnd) and returns the rolled-up
// USD overage cost: ceil(sum/1000) * $0.50. Plans free and agency are
// expected to never see rows in this table; for them the value is 0.
func (q *PostgresQuery) GetOverageCostForCurrentPeriod(ctx context.Context, userID string, periodStart, periodEnd time.Time) (float64, error) {
	var total int
	err := q.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(count), 0) FROM comment_usage_logs
		 WHERE user_id = $1 AND period_start >= $2 AND period_start < $3`,
		userID, periodStart, periodEnd).Scan(&total)
	if err != nil {
		return 0, err
	}
	if total <= 0 {
		return 0, nil
	}
	// ceil(total/1000) * 0.50
	chunks := (total + 999) / 1000
	return float64(chunks) * 0.50, nil
}

// InsertCommentUsageLog records overage rows for the LLM processor when a
// batch crosses the comment_quota_monthly boundary for a Creator/Studio
// user. Tx-aware so the row lands in the same transaction as the LLM result.
func (q *PostgresQuery) InsertCommentUsageLog(ctx context.Context, tx pgx.Tx, userID string, count int, periodStart, periodEnd time.Time) error {
	sql := `INSERT INTO comment_usage_logs (user_id, count, period_start, period_end)
	        VALUES ($1, $2, $3, $4)`
	if tx != nil {
		_, err := tx.Exec(ctx, sql, userID, count, periodStart, periodEnd)
		return err
	}
	_, err := q.pool.Exec(ctx, sql, userID, count, periodStart, periodEnd)
	return err
}

// PauseChannelsAlphabetically sets tracking_status='paused' on every active
// tracker for the user beyond keepCount, ordered by the channel's display
// name ascending (id ascending as a deterministic tie-breaker). It is the
// downgrade-on-cancel primitive (Spec D7).
func (q *PostgresQuery) PauseChannelsAlphabetically(ctx context.Context, tx pgx.Tx, userID string, keepCount int) error {
	if keepCount < 0 {
		keepCount = 0
	}
	sql := `
		UPDATE user_channel_tracking
		   SET tracking_status = 'paused'
		 WHERE id IN (
		     SELECT uct.id
		       FROM user_channel_tracking uct
		       JOIN platform_channels pc ON pc.id = uct.platform_channel_id
		      WHERE uct.user_id = $1
		        AND uct.tracking_status = 'active'
		      ORDER BY pc.display_name ASC, uct.id ASC
		      OFFSET $2
		 )`
	if tx != nil {
		_, err := tx.Exec(ctx, sql, userID, keepCount)
		return err
	}
	_, err := q.pool.Exec(ctx, sql, userID, keepCount)
	return err
}

// IncrementCommentsUsedTx is the transactional sibling of
// IncrementCommentsUsed used by the LLM processor when it persists a batch
// inside one transaction (LLM results + quota bump + optional overage log).
// Returns the new value of comments_used_this_period so callers can detect
// the moment a batch crosses the quota boundary.
func (q *PostgresQuery) IncrementCommentsUsedTx(ctx context.Context, tx pgx.Tx, userID string, count int) (int, error) {
	var used int
	err := tx.QueryRow(ctx,
		`UPDATE users
		    SET comments_used_this_period = comments_used_this_period + $1,
		        updated_at = NOW()
		  WHERE id = $2
		 RETURNING comments_used_this_period`,
		count, userID).Scan(&used)
	return used, err
}

// ResetFreeTierQuotas runs the daily cron that catches free users whose
// quota_reset_at is in the past. Returns rows affected.
func (q *PostgresQuery) ResetFreeTierQuotas(ctx context.Context) (int64, error) {
	tag, err := q.pool.Exec(ctx,
		`UPDATE users
		    SET comments_used_this_period = 0,
		        quota_reset_at = NOW() + INTERVAL '1 month',
		        updated_at = NOW()
		  WHERE subscription_plan = 'free'
		    AND quota_reset_at IS NOT NULL
		    AND quota_reset_at <= NOW()`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ExpirePastDueGrace transitions users from past_due to paused when they
// have been in that state for more than 3 days without a successful payment.
// "Time in past_due" is derived from the past_due_since column written by
// the invoice.payment_failed handler.
func (q *PostgresQuery) ExpirePastDueGrace(ctx context.Context, graceHours int) (int64, error) {
	tag, err := q.pool.Exec(ctx,
		`UPDATE users
		    SET subscription_status = 'paused',
		        updated_at = NOW()
		  WHERE subscription_status = 'past_due'
		    AND past_due_since IS NOT NULL
		    AND past_due_since <= NOW() - make_interval(hours => $1)`,
		graceHours)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ExpirePastDueGraceWithUsers is the data-lifecycle (12.4) variant of
// ExpirePastDueGrace: same transition, but RETURNINGs (id, email) for
// each user that crossed the boundary so the caller can emit the
// payment_grace_final email. Returning the rows is essential — without
// it, the cron has no way to enumerate the affected users without a
// second SELECT race window.
func (q *PostgresQuery) ExpirePastDueGraceWithUsers(ctx context.Context, graceHours int) ([]ExpiredGraceUser, error) {
	rows, err := q.pool.Query(ctx,
		`UPDATE users
		    SET subscription_status = 'paused',
		        updated_at = NOW()
		  WHERE subscription_status = 'past_due'
		    AND past_due_since IS NOT NULL
		    AND past_due_since <= NOW() - make_interval(hours => $1)
		  RETURNING id, email`,
		graceHours)
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

// ExpiredGraceUser is the (id, email) tuple ExpirePastDueGraceWithUsers
// returns per transitioned user. Used by the billing quota cron to emit
// payment_grace_final emails (12.4).
type ExpiredGraceUser struct {
	ID    string
	Email string
}

// TrialReminderUser is the (id, email, trial_ends_at) tuple the trial
// reminder cron (12.5) consumes.
type TrialReminderUser struct {
	ID          string
	Email       string
	TrialEndsAt time.Time
}

// ListUsersForTrialExpiryReminder returns users whose trial_ends_at is
// within (lo, hi]. The caller picks lo/hi to define the 1-day reminder
// window (typically NOW+2d, NOW+3d). subscription_status is constrained
// to 'trialing' so users who already paid (or were paused) are skipped.
func (q *PostgresQuery) ListUsersForTrialExpiryReminder(ctx context.Context, lo, hi time.Time) ([]TrialReminderUser, error) {
	rows, err := q.pool.Query(ctx,
		`SELECT id, email, trial_ends_at
		   FROM users
		  WHERE subscription_status = 'trialing'
		    AND trial_ends_at IS NOT NULL
		    AND trial_ends_at >  $1
		    AND trial_ends_at <= $2
		    AND deletion_requested_at IS NULL`,
		lo, hi)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TrialReminderUser
	for rows.Next() {
		var u TrialReminderUser
		if err := rows.Scan(&u.ID, &u.Email, &u.TrialEndsAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
