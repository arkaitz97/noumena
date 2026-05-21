package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func (q *SQLiteQuery) InsertWeeklyDigest(ctx context.Context, digest *WeeklyDigest) error {
	query := `
		INSERT INTO weekly_digests (id, user_id, week_start_date, status, content, created_at)
		VALUES (?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
		RETURNING id, created_at
	`
	var createdAtStr string
	err := q.db.QueryRowContext(ctx, query, generateUUID(), digest.UserID, digest.WeekStartDate, digest.Status, digest.Content).Scan(&digest.ID, &createdAtStr)
	if err != nil {
		return err
	}
	digest.CreatedAt = parseTime(createdAtStr)
	return nil
}

func (q *SQLiteQuery) GetUsersForDigest(ctx context.Context) ([]*User, error) {
	query := `
		SELECT DISTINCT ` + qualifiedUserColumns("u") + `
		FROM users u
		JOIN user_channel_tracking uct ON uct.user_id = u.id
		WHERE uct.tracking_status = 'active'
		  AND u.email_verified_at IS NOT NULL
		  AND u.deletion_requested_at IS NULL
	`
	rows, err := q.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		u, err := scanUserRows(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (q *SQLiteQuery) GetRecentProcessedCommentsForUser(ctx context.Context, userID string, since time.Time) ([]*Comment, error) {
	// Add 1 second to until to include comments posted at the current moment
	until := time.Now().UTC().Add(time.Second)
	return q.GetEligibleCommentsForUser(ctx, userID, since, until)
}

func (q *SQLiteQuery) GetEligibleCommentsForUser(ctx context.Context, userID string, since, until time.Time) ([]*Comment, error) {
	const query = `
		SELECT c.id, c.platform_channel_id, c.platform_comment_id, c.parent_comment_id, c.reply_depth,
		       c.author_handle, c.body, c.posted_at, c.category, c.actionable_insights,
		       c.topic, c.confidence, c.processed_at, c.created_at
		FROM comments c
		JOIN user_channel_tracking uct ON c.platform_channel_id = uct.platform_channel_id
		WHERE uct.user_id = ?
		  AND uct.tracking_status = 'active'
		  AND c.processed_at IS NOT NULL
		  AND c.posted_at >= ?
		  AND c.posted_at <  ?
		  AND (c.category IS NULL OR c.category NOT IN ('toxic_criticism', 'spam'))
		ORDER BY c.posted_at DESC
	`
	rows, err := q.db.QueryContext(ctx, query, userID, since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339))
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
	return comments, rows.Err()
}

func (q *SQLiteQuery) InsertWeeklyDigestPending(ctx context.Context, userID string, weekStart time.Time) (string, bool, error) {
	weekStartStr := weekStart.UTC().Format("2006-01-02")
	const insertSQL = `
		INSERT INTO weekly_digests (id, user_id, week_start_date, status)
		VALUES (?, ?, ?, 'pending')
		ON CONFLICT (user_id, week_start_date) DO NOTHING
		RETURNING id
	`
	var id string
	err := q.db.QueryRowContext(ctx, insertSQL, generateUUID(), userID, weekStartStr).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", false, err
	}
	const selectSQL = `
		SELECT id FROM weekly_digests
		WHERE user_id = ? AND week_start_date = ?
	`
	if err := q.db.QueryRowContext(ctx, selectSQL, userID, weekStartStr).Scan(&id); err != nil {
		return "", false, err
	}
	return id, false, nil
}

func (q *SQLiteQuery) UpdateDigestContent(ctx context.Context, digestID string, contentJSON []byte, status string) error {
	if len(contentJSON) == 0 {
		_, err := q.db.ExecContext(ctx,
			`UPDATE weekly_digests SET content = NULL, status = ? WHERE id = ?`,
			status, digestID)
		return err
	}
	_, err := q.db.ExecContext(ctx,
		`UPDATE weekly_digests SET content = ?, status = ? WHERE id = ?`,
		string(contentJSON), status, digestID)
	return err
}

func (q *SQLiteQuery) UpdateDigestStatus(ctx context.Context, digestID, status string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE weekly_digests SET status = ? WHERE id = ?`,
		status, digestID)
	return err
}

func (q *SQLiteQuery) GetPreviousWeekDigest(ctx context.Context, userID string, weekStart time.Time) (*WeeklyDigest, error) {
	prev := weekStart.UTC().AddDate(0, 0, -7).Format("2006-01-02")
	const sqlStr = `
		SELECT id, user_id, week_start_date, status,
		       COALESCE(content, ''), emailed_at, created_at
		FROM weekly_digests
		WHERE user_id = ? AND week_start_date = ?
		LIMIT 1
	`
	return scanWeeklyDigest(q.db.QueryRowContext(ctx, sqlStr, userID, prev))
}

func (q *SQLiteQuery) GetLatestDigestForUser(ctx context.Context, userID string) (*WeeklyDigest, error) {
	const sqlStr = `
		SELECT id, user_id, week_start_date, status,
		       COALESCE(content, ''), emailed_at, created_at
		FROM weekly_digests
		WHERE user_id = ?
		ORDER BY week_start_date DESC
		LIMIT 1
	`
	return scanWeeklyDigest(q.db.QueryRowContext(ctx, sqlStr, userID))
}

func (q *SQLiteQuery) ListDigests(ctx context.Context, userID string, limit, offset int) ([]*WeeklyDigest, error) {
	if limit <= 0 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}
	const sqlStr = `
		SELECT id, user_id, week_start_date, status,
		       COALESCE(content, ''), emailed_at, created_at
		FROM weekly_digests
		WHERE user_id = ?
		ORDER BY week_start_date DESC
		LIMIT ? OFFSET ?
	`
	rows, err := q.db.QueryContext(ctx, sqlStr, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*WeeklyDigest
	for rows.Next() {
		d, err := scanWeeklyDigestRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (q *SQLiteQuery) GetStalePendingDigests(ctx context.Context, olderThan time.Time) ([]*WeeklyDigest, error) {
	const sqlStr = `
		SELECT id, user_id, week_start_date, status,
		       COALESCE(content, ''), emailed_at, created_at
		FROM weekly_digests
		WHERE status = 'pending' AND created_at < ?
	`
	rows, err := q.db.QueryContext(ctx, sqlStr, olderThan.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*WeeklyDigest
	for rows.Next() {
		d, err := scanWeeklyDigestRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (q *SQLiteQuery) UpdateUserDigestOptIn(ctx context.Context, userID string, optIn bool) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE users SET digest_email_opt_in = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ?`,
		optIn, userID)
	return err
}

func scanWeeklyDigest(row *sql.Row) (*WeeklyDigest, error) {
	var d WeeklyDigest
	var contentStr, emailedAtStr, createdAtStr string
	err := row.Scan(&d.ID, &d.UserID, &d.WeekStartDate, &d.Status, &contentStr, &emailedAtStr, &createdAtStr)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	d.Content = contentStr
	if emailedAtStr != "" {
		t, _ := time.Parse(time.RFC3339, emailedAtStr)
		d.EmailedAt = &t
	}
	d.CreatedAt = parseTime(createdAtStr)
	return &d, nil
}

func scanWeeklyDigestRows(rows *sql.Rows) (*WeeklyDigest, error) {
	var d WeeklyDigest
	var contentStr, emailedAtStr, createdAtStr string
	err := rows.Scan(&d.ID, &d.UserID, &d.WeekStartDate, &d.Status, &contentStr, &emailedAtStr, &createdAtStr)
	if err != nil {
		return nil, err
	}
	d.Content = contentStr
	if emailedAtStr != "" {
		t, _ := time.Parse(time.RFC3339, emailedAtStr)
		d.EmailedAt = &t
	}
	d.CreatedAt = parseTime(createdAtStr)
	return &d, nil
}

func qualifiedUserColumns(alias string) string {
	return alias + ".id, " + alias + ".email, " + alias + ".password_hash, " +
		alias + ".google_sub, " + alias + ".email_verified_at, " +
		alias + ".subscription_plan, " + alias + ".stripe_customer_id, " +
		alias + ".comment_quota_monthly, " + alias + ".comments_used_this_period, " +
		alias + ".quota_reset_at, " + alias + ".token_version, " +
		alias + ".failed_login_attempts, " + alias + ".locked_until, " +
		alias + ".deletion_requested_at, " +
		alias + ".deletion_completed_at, " + alias + ".data_export_url, " +
		alias + ".data_export_expires_at, " +
		alias + ".digest_email_opt_in, " + alias + ".digest_email_sent_at, " +
		alias + ".trial_ends_at, " + alias + ".subscription_status, " +
		alias + ".current_period_end, " + alias + ".billing_interval, " +
		alias + ".stripe_subscription_id, " +
		alias + ".past_due_since, " + alias + ".active_workspace_id, " +
		alias + ".created_at, " + alias + ".updated_at"
}

func scanUserRows(rows *sql.Rows) (*User, error) {
	u := &User{}
	var emailVerifiedAt, quotaResetAt, lockedUntil, deletionRequestedAt, deletionCompletedAt sql.NullString
	var dataExportURL, dataExportExpiresAt, digestEmailSentAt sql.NullString
	var trialEndsAt, currentPeriodEnd, pastDueSince sql.NullString
	var stripeCustomerID, billingInterval, stripeSubscriptionID, activeWorkspaceID sql.NullString
	var createdAt, updatedAt sql.NullString
	err := rows.Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.GoogleSub, &emailVerifiedAt,
		&u.SubscriptionPlan, &stripeCustomerID, &u.CommentQuotaMonthly,
		&u.CommentsUsedThisPeriod, &quotaResetAt, &u.TokenVersion,
		&u.FailedLoginAttempts, &lockedUntil, &deletionRequestedAt,
		&deletionCompletedAt, &dataExportURL, &dataExportExpiresAt,
		&u.DigestEmailOptIn, &digestEmailSentAt,
		&trialEndsAt, &u.SubscriptionStatus, &currentPeriodEnd, &billingInterval,
		&stripeSubscriptionID,
		&pastDueSince, &activeWorkspaceID,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	u.EmailVerifiedAt = parseTimePtr(emailVerifiedAt)
	u.StripeCustomerID = strPtrOrNull(stripeCustomerID)
	u.QuotaResetAt = parseTimePtr(quotaResetAt)
	u.LockedUntil = parseTimePtr(lockedUntil)
	u.DeletionRequestedAt = parseTimePtr(deletionRequestedAt)
	u.DeletionCompletedAt = parseTimePtr(deletionCompletedAt)
	u.DataExportURL = strPtrOrNull(dataExportURL)
	u.DataExportExpiresAt = parseTimePtr(dataExportExpiresAt)
	u.DigestEmailSentAt = parseTimePtr(digestEmailSentAt)
	u.TrialEndsAt = parseTimePtr(trialEndsAt)
	u.CurrentPeriodEnd = parseTimePtr(currentPeriodEnd)
	u.BillingInterval = strPtrOrNull(billingInterval)
	u.StripeSubscriptionID = strPtrOrNull(stripeSubscriptionID)
	u.PastDueSince = parseTimePtr(pastDueSince)
	u.ActiveWorkspaceID = strPtrOrNull(activeWorkspaceID)
	u.CreatedAt = parseTime(createdAt.String)
	u.UpdatedAt = parseTime(updatedAt.String)
	return u, nil
}

func strPtrOrNull(s sql.NullString) *string {
	if !s.Valid || s.String == "" {
		return nil
	}
	return &s.String
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func parseTimePtr(s sql.NullString) *time.Time {
	if !s.Valid || s.String == "" {
		return nil
	}
	t, _ := time.Parse(time.RFC3339, s.String)
	return &t
}

func (q *SQLiteQuery) MarkDigestEmailed(ctx context.Context, digestID, userID string, at time.Time) error {
	if _, err := q.db.ExecContext(ctx,
		`UPDATE weekly_digests SET emailed_at = ? WHERE id = ?`,
		at.Format(time.RFC3339), digestID); err != nil {
		return err
	}
	_, err := q.db.ExecContext(ctx,
		`UPDATE users SET digest_email_sent_at = ? WHERE id = ?`,
		at.Format(time.RFC3339), userID)
	return err
}
