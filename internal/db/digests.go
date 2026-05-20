package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// InsertWeeklyDigest is the legacy single-shot insert path. New worker code
// uses InsertWeeklyDigestPending + UpdateDigestContent for idempotency; this
// helper is retained for callers (and tests) that have not migrated yet.
func (q *PostgresQuery) InsertWeeklyDigest(ctx context.Context, digest *WeeklyDigest) error {
	query := `
		INSERT INTO weekly_digests (user_id, week_start_date, status, content, created_at)
		VALUES ($1, $2, $3, $4, NOW())
		RETURNING id, created_at
	`
	return q.pool.QueryRow(ctx, query, digest.UserID, digest.WeekStartDate, digest.Status, digest.Content).Scan(&digest.ID, &digest.CreatedAt)
}

// GetUsersForDigest returns every user with at least one user_channel_tracking
// row in tracking_status='active'. The DISTINCT guards against multi-channel
// users producing duplicate rows. Spec 06 requirement: cron eligibility scope.
func (q *PostgresQuery) GetUsersForDigest(ctx context.Context) ([]*User, error) {
	query := `
		SELECT DISTINCT ` + qualifiedUserColumns("u") + `
		FROM users u
		JOIN user_channel_tracking uct ON uct.user_id = u.id
		WHERE uct.tracking_status = 'active'
		  AND u.email_verified_at IS NOT NULL
		  AND u.deletion_requested_at IS NULL
	`
	rows, err := q.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// GetRecentProcessedCommentsForUser returns the legacy single-bound (since)
// view. It is preserved to satisfy the DBQuery interface; the digest worker
// uses GetEligibleCommentsForUser (with explicit since/until) instead.
func (q *PostgresQuery) GetRecentProcessedCommentsForUser(ctx context.Context, userID string, since time.Time) ([]*Comment, error) {
	return q.GetEligibleCommentsForUser(ctx, userID, since, time.Now().UTC())
}

// GetEligibleCommentsForUser returns processed comments owned by channels
// the user currently tracks (tracking_status='active'), within
// [since, until), excluding toxic_criticism and spam. processed_at must be
// non-NULL (Spec 06 EC: only LLM-classified comments feed the digest).
func (q *PostgresQuery) GetEligibleCommentsForUser(ctx context.Context, userID string, since, until time.Time) ([]*Comment, error) {
	const query = `
		SELECT c.id, c.platform_channel_id, c.platform_comment_id, c.parent_comment_id,
		       c.author_handle, c.body, c.posted_at, c.category, c.sentiment_score,
		       c.topic, c.confidence, c.processed_at, c.created_at
		FROM comments c
		JOIN user_channel_tracking uct ON c.platform_channel_id = uct.platform_channel_id
		WHERE uct.user_id = $1
		  AND uct.tracking_status = 'active'
		  AND c.processed_at IS NOT NULL
		  AND c.posted_at >= $2
		  AND c.posted_at <  $3
		  AND (c.category IS NULL OR c.category NOT IN ('toxic_criticism', 'spam'))
		ORDER BY c.posted_at DESC
	`
	rows, err := q.pool.Query(ctx, query, userID, since, until)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []*Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(
			&c.ID, &c.PlatformChannelID, &c.PlatformCommentID, &c.ParentCommentID,
			&c.AuthorHandle, &c.Body, &c.PostedAt, &c.Category, &c.SentimentScore,
			&c.Topic, &c.Confidence, &c.ProcessedAt, &c.CreatedAt,
		); err != nil {
			return nil, err
		}
		comments = append(comments, &c)
	}
	return comments, rows.Err()
}

// InsertWeeklyDigestPending inserts a (user_id, week_start_date) row in
// status='pending' and returns its id plus a flag indicating whether the
// insert actually wrote a new row. ON CONFLICT short-circuits to "not
// inserted" so the worker can skip already-in-progress / already-completed
// weeks idempotently. The unique index idx_weekly_digests_user_week is the
// enforcement primitive.
func (q *PostgresQuery) InsertWeeklyDigestPending(ctx context.Context, userID string, weekStart time.Time) (string, bool, error) {
	weekStartStr := weekStart.UTC().Format("2006-01-02")
	const insertSQL = `
		INSERT INTO weekly_digests (user_id, week_start_date, status)
		VALUES ($1, $2, 'pending')
		ON CONFLICT (user_id, week_start_date) DO NOTHING
		RETURNING id::text
	`
	var id string
	err := q.pool.QueryRow(ctx, insertSQL, userID, weekStartStr).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, err
	}
	// Another worker beat us to it; fetch the existing row's id.
	const selectSQL = `
		SELECT id::text FROM weekly_digests
		WHERE user_id = $1 AND week_start_date = $2
	`
	if err := q.pool.QueryRow(ctx, selectSQL, userID, weekStartStr).Scan(&id); err != nil {
		return "", false, err
	}
	return id, false, nil
}

// UpdateDigestContent overwrites content + status for a digest row. Used for
// the completed (with JSONB) and skipped/deferred (content may be NULL)
// transitions. Pass an empty contentJSON to leave content NULL.
func (q *PostgresQuery) UpdateDigestContent(ctx context.Context, digestID string, contentJSON []byte, status string) error {
	if len(contentJSON) == 0 {
		_, err := q.pool.Exec(ctx,
			`UPDATE weekly_digests SET content = NULL, status = $2 WHERE id = $1`,
			digestID, status)
		return err
	}
	_, err := q.pool.Exec(ctx,
		`UPDATE weekly_digests SET content = $2::jsonb, status = $3 WHERE id = $1`,
		digestID, string(contentJSON), status)
	return err
}

// UpdateDigestStatus updates status only — used for the deferred terminal
// transition where content remains whatever it was (typically NULL).
func (q *PostgresQuery) UpdateDigestStatus(ctx context.Context, digestID, status string) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE weekly_digests SET status = $2 WHERE id = $1`,
		digestID, status)
	return err
}

// GetPreviousWeekDigest returns the digest at weekStart - 7 days for the
// user, used as a baseline for the sentiment-trend computation. ErrNotFound
// when no baseline exists (first-week creators).
func (q *PostgresQuery) GetPreviousWeekDigest(ctx context.Context, userID string, weekStart time.Time) (*WeeklyDigest, error) {
	prev := weekStart.UTC().AddDate(0, 0, -7).Format("2006-01-02")
	const sql = `
		SELECT id::text, user_id::text, week_start_date::text, status,
		       COALESCE(content::text, ''), emailed_at, created_at
		FROM weekly_digests
		WHERE user_id = $1 AND week_start_date = $2
		LIMIT 1
	`
	return scanWeeklyDigest(q.pool.QueryRow(ctx, sql, userID, prev))
}

// GetLatestDigestForUser returns the newest weekly_digests row for the user
// by week_start_date, or ErrNotFound. Used by GET /api/digests/latest.
//
// Note: this is a separate method from the insights-api `GetLatestDigest`
// (which returns a *LatestDigest with a different shape); the weekly-digest
// API returns the full WeeklyDigest model.
func (q *PostgresQuery) GetLatestDigestForUser(ctx context.Context, userID string) (*WeeklyDigest, error) {
	const sql = `
		SELECT id::text, user_id::text, week_start_date::text, status,
		       COALESCE(content::text, ''), emailed_at, created_at
		FROM weekly_digests
		WHERE user_id = $1
		ORDER BY week_start_date DESC
		LIMIT 1
	`
	return scanWeeklyDigest(q.pool.QueryRow(ctx, sql, userID))
}

// ListDigests returns up to `limit` digests for the user ordered by
// week_start_date DESC, paginated by `offset`.
func (q *PostgresQuery) ListDigests(ctx context.Context, userID string, limit, offset int) ([]*WeeklyDigest, error) {
	if limit <= 0 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}
	const sql = `
		SELECT id::text, user_id::text, week_start_date::text, status,
		       COALESCE(content::text, ''), emailed_at, created_at
		FROM weekly_digests
		WHERE user_id = $1
		ORDER BY week_start_date DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := q.pool.Query(ctx, sql, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*WeeklyDigest
	for rows.Next() {
		d, err := scanWeeklyDigest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetStalePendingDigests returns digest rows still in status='pending' whose
// created_at is older than the cutoff. The worker calls this on startup to
// promote stale rows (likely orphaned by a process restart mid-retry) to
// 'deferred'.
func (q *PostgresQuery) GetStalePendingDigests(ctx context.Context, olderThan time.Time) ([]*WeeklyDigest, error) {
	const sql = `
		SELECT id::text, user_id::text, week_start_date::text, status,
		       COALESCE(content::text, ''), emailed_at, created_at
		FROM weekly_digests
		WHERE status = 'pending' AND created_at < $1
	`
	rows, err := q.pool.Query(ctx, sql, olderThan)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*WeeklyDigest
	for rows.Next() {
		d, err := scanWeeklyDigest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// UpdateUserDigestOptIn flips users.digest_email_opt_in for a user. Used by
// PATCH /api/user/preferences.
func (q *PostgresQuery) UpdateUserDigestOptIn(ctx context.Context, userID string, optIn bool) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE users SET digest_email_opt_in = $2, updated_at = NOW() WHERE id = $1`,
		userID, optIn)
	return err
}

// scanWeeklyDigest scans one weekly_digests row into a *WeeklyDigest. Maps
// pgx.ErrNoRows to ErrNotFound for caller convenience.
func scanWeeklyDigest(row pgx.Row) (*WeeklyDigest, error) {
	var d WeeklyDigest
	err := row.Scan(&d.ID, &d.UserID, &d.WeekStartDate, &d.Status, &d.Content, &d.EmailedAt, &d.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// qualifiedUserColumns prefixes every userColumns identifier with the given
// table alias, so a JOIN-style query can SELECT the full user row without
// ambiguity. The list is kept in lockstep with the unqualified userColumns
// constant in db.go — when adding a column there, mirror it here.
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
		alias + ".past_due_since, " +
		alias + ".created_at, " + alias + ".updated_at"
}

// MarkDigestEmailed sets weekly_digests.emailed_at and the user's
// users.digest_email_sent_at in a single transactionless update pair.
// data-lifecycle (12.6): called from the digest worker after the
// TemplateWeeklyDigest send returns nil. Failures are logged by the
// caller; the email is already enqueued, so the worst case is a stale
// emailed_at value (acceptable — the email_send_log row is the source of
// truth for actual delivery).
func (q *PostgresQuery) MarkDigestEmailed(ctx context.Context, digestID, userID string, at time.Time) error {
	if _, err := q.pool.Exec(ctx,
		`UPDATE weekly_digests SET emailed_at = $2 WHERE id = $1`,
		digestID, at); err != nil {
		return err
	}
	_, err := q.pool.Exec(ctx,
		`UPDATE users SET digest_email_sent_at = $2 WHERE id = $1`,
		userID, at)
	return err
}
