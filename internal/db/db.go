package db

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

// ApplyMigrations executes the embedded schema.sql against the pool. It is
// idempotent: every CREATE/ALTER uses IF NOT EXISTS guards, so it is safe to
// run repeatedly at startup.
func ApplyMigrations(ctx context.Context) error {
	if Pool == nil {
		return fmt.Errorf("db pool not initialized")
	}
	if _, err := Pool.Exec(ctx, schemaSQL); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

var Pool *pgxpool.Pool

// InitDB initializes the database connection pool
func InitDB(databaseUrl string) error {
	var err error
	Pool, err = pgxpool.New(context.Background(), databaseUrl)
	if err != nil {
		return fmt.Errorf("unable to create database pool: %w", err)
	}

	// Ping the DB
	if err := Pool.Ping(context.Background()); err != nil {
		return fmt.Errorf("unable to ping database: %w", err)
	}

	log.Println("Successfully connected to the PostgreSQL database")
	return nil
}

// CloseDB closes the database connection pool
func CloseDB() {
	if Pool != nil {
		Pool.Close()
	}
}

// ErrNotFound is returned by lookup helpers when a row is missing.
var ErrNotFound = errors.New("not found")

// ErrNoPrimaryTracker is returned by GetPrimaryTracker when a channel has no
// active tracker with token_priority = 0. The channel is effectively idle.
var ErrNoPrimaryTracker = errors.New("no primary tracker")

// DBQuery defines the database surface used by handlers and workers.
type DBQuery interface {
	// Users (legacy)
	CreateUser(ctx context.Context, email, passwordHash string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByID(ctx context.Context, id string) (*User, error)
	UpdateUserSubscription(ctx context.Context, userID, plan, stripeCustomerID string, quota int) error

	// Users (auth)
	CreateUserWithPassword(ctx context.Context, email, passwordHash string) (*User, error)
	CreateUserGoogle(ctx context.Context, email, googleSub string) (*User, error)
	GetUserByGoogleSub(ctx context.Context, sub string) (*User, error)
	IncrementFailedLoginAttempts(ctx context.Context, userID string) (int, error)
	LockUser(ctx context.Context, userID string, until time.Time) error
	ResetLoginFailures(ctx context.Context, userID string) error
	IncrementTokenVersion(ctx context.Context, userID string) error
	GetUserTokenVersion(ctx context.Context, userID string) (int, error)
	SetEmailVerified(ctx context.Context, userID string) error
	UpdatePasswordHash(ctx context.Context, userID, hash string) error
	LinkGoogleSub(ctx context.Context, userID, sub string) error
	DeleteStaleUnverifiedUsers(ctx context.Context, olderThan time.Duration) (int64, error)

	// Email-verification tokens
	InsertEmailVerificationToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
	ConsumeEmailVerificationToken(ctx context.Context, tokenHash string) (userID string, err error)
	InvalidateEmailVerificationTokensForUser(ctx context.Context, userID string) error
	CountVerificationResendsInWindow(ctx context.Context, userID string, since time.Time) (int, error)

	// Password-reset tokens
	InsertPasswordResetToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
	ConsumePasswordResetToken(ctx context.Context, tokenHash string) (userID string, err error)
	InvalidatePasswordResetTokensForUser(ctx context.Context, userID string) error
	CountForgotPasswordEmailsInWindow(ctx context.Context, email string, since time.Time) (int, error)

	// Refresh tokens
	InsertRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
	GetRefreshTokenByHash(ctx context.Context, tokenHash string) (userID string, expiresAt time.Time, err error)
	DeleteRefreshTokenByHash(ctx context.Context, tokenHash string) error
	DeleteAllRefreshTokensForUser(ctx context.Context, userID string) error

	// Channels
	ListChannelsForUser(ctx context.Context, userID string) ([]*PlatformChannel, error)
	ConnectChannelTx(ctx context.Context, userID, platform, platformChannelID, displayName, accessTokenEnc, refreshTokenEnc string, tokenExpiresAt *time.Time) error
	PauseChannelTracking(ctx context.Context, userID, platformChannelID string) error
	RemoveChannelTracking(ctx context.Context, userID, platformChannelID string) error

	GetChannelsForSync(ctx context.Context, minutes int) ([]*PlatformChannel, error)
	UpdateSyncTime(ctx context.Context, id string) error

	// Comments
	GetCommentFeed(ctx context.Context, userID string, limit, offset int) ([]*Comment, error)
	StarComment(ctx context.Context, userID, commentID string) error
	InsertComment(ctx context.Context, c *Comment) error
	GetUnprocessedComments(ctx context.Context, limit int) ([]*Comment, error)
	GetUnprocessedCommentsByChannel(ctx context.Context, limit int) (map[string][]*Comment, error)
	UpdateCommentLLMData(ctx context.Context, id, category, topic, provider, model string, sentiment, confidence float64) error

	// LLM-processing quota / primary-tracker resolution
	IncrementCommentsUsed(ctx context.Context, userID string, count int) error
	GetPrimaryTracker(ctx context.Context, platformChannelID string) (*User, error)

	// llm_processing_jobs lifecycle
	EnqueueLLMJob(ctx context.Context, commentIDs []string, provider string) (string, error)
	MarkLLMJobProcessing(ctx context.Context, jobID string) error
	MarkLLMJobCompleted(ctx context.Context, jobID string) error
	MarkLLMJobFailed(ctx context.Context, jobID string, errorMessage string) error

	// Digests
	InsertWeeklyDigest(ctx context.Context, digest *WeeklyDigest) error
	GetUsersForDigest(ctx context.Context) ([]*User, error)
	GetRecentProcessedCommentsForUser(ctx context.Context, userID string, since time.Time) ([]*Comment, error)
	// weekly-digest change additions:
	GetEligibleCommentsForUser(ctx context.Context, userID string, since, until time.Time) ([]*Comment, error)
	InsertWeeklyDigestPending(ctx context.Context, userID string, weekStart time.Time) (string, bool, error)
	UpdateDigestContent(ctx context.Context, digestID string, contentJSON []byte, status string) error
	UpdateDigestStatus(ctx context.Context, digestID, status string) error
	GetPreviousWeekDigest(ctx context.Context, userID string, weekStart time.Time) (*WeeklyDigest, error)
	GetLatestDigestForUser(ctx context.Context, userID string) (*WeeklyDigest, error)
	ListDigests(ctx context.Context, userID string, limit, offset int) ([]*WeeklyDigest, error)
	GetStalePendingDigests(ctx context.Context, olderThan time.Time) ([]*WeeklyDigest, error)
	UpdateUserDigestOptIn(ctx context.Context, userID string, optIn bool) error
	// data-lifecycle (12.6): mark a digest as emailed; updates both
	// weekly_digests.emailed_at and users.digest_email_sent_at.
	MarkDigestEmailed(ctx context.Context, digestID, userID string, at time.Time) error

	// Tracking
	GetIngestionToken(ctx context.Context, platformChannelID string) (string, string, error)
	PromoteNextIngestionToken(ctx context.Context, platformChannelID string) error

	// --- Channel-management additions (channel-management change) ---

	// OAuth state store
	InsertOAuthState(ctx context.Context, stateHash, userID string, expiresAt time.Time) error
	ConsumeOAuthState(ctx context.Context, stateHash, userID string) (bool, error)
	CleanupExpiredOAuthStates(ctx context.Context) (int64, error)

	// platform_channels operations
	UpsertPlatformChannel(ctx context.Context, platform, platformChannelID, displayName string) (channelID string, inserted bool, err error)
	GetChannelByPlatformID(ctx context.Context, platform, platformChannelID string) (*PlatformChannel, error)
	ScheduleChannelDeletion(ctx context.Context, platformChannelID string, graceHours int) error
	ClearChannelDeletion(ctx context.Context, platformChannelID string) error
	GetActiveTrackerCount(ctx context.Context, platformChannelID string) (int, error)
	PurgeChannelData(ctx context.Context, platformChannelID string) error

	// user_channel_tracking operations
	CreateUserChannelTracking(ctx context.Context, userID, platformChannelID, encryptedAccess, encryptedRefresh string, expiresAt time.Time, backfillDays, tokenPriority int) (*UserChannelTracking, error)
	GetUserTracking(ctx context.Context, userID, platformChannelID string) (*UserChannelTracking, error)
	ListUserChannels(ctx context.Context, userID string) ([]ChannelListItem, error)
	PauseTracking(ctx context.Context, userID, platformChannelID string) error
	ResumeTracking(ctx context.Context, userID, platformChannelID string) error
	DeleteTracking(ctx context.Context, userID, platformChannelID string) error
	UpdateBackfillDays(ctx context.Context, userID, platformChannelID string, days int) error
	GetMaxTokenPriority(ctx context.Context, platformChannelID string) (int, error)
	GetIngestionTokenRow(ctx context.Context, platformChannelID string) (*UserChannelTracking, bool, error)
	UpdateTokenAndExpiry(ctx context.Context, trackingID, encryptedAccess, encryptedRefresh string, expiresAt time.Time) error
	GetExpiringTokens(ctx context.Context, withinDays int) ([]UserChannelTracking, error)
	MarkTokenExpiryNotified(ctx context.Context, trackingID string) error
	GetCommentCountForChannel(ctx context.Context, platformChannelID string) (int, error)

	// --- Billing additions (billing change) ---
	IsStripeEventProcessed(ctx context.Context, eventID string) (bool, error)
	InsertProcessedStripeEvent(ctx context.Context, tx pgx.Tx, eventID, eventType string) error
	ApplySubscriptionUpdate(ctx context.Context, tx pgx.Tx, u SubscriptionUpdate) error
	GetUserByStripeCustomerID(ctx context.Context, customerID string) (*User, error)
	GetActiveChannelCount(ctx context.Context, userID string) (int, error)
	GetOverageCostForCurrentPeriod(ctx context.Context, userID string, periodStart, periodEnd time.Time) (float64, error)
	InsertCommentUsageLog(ctx context.Context, tx pgx.Tx, userID string, count int, periodStart, periodEnd time.Time) error
	PauseChannelsAlphabetically(ctx context.Context, tx pgx.Tx, userID string, keepCount int) error
	ResetFreeTierQuotas(ctx context.Context) (int64, error)
	ExpirePastDueGrace(ctx context.Context, graceHours int) (int64, error)

	// --- Transactional email log (data-lifecycle change) ---
	// Embedded so handlers/workers that hold a DBQuery can also push to
	// the email pipeline without an extra constructor.
	EmailLogStore

	// --- Account-deletion + GDPR-export workflows (data-lifecycle change) ---
	LifecycleStore
}

// PostgresQuery implements DBQuery using pgxpool
type PostgresQuery struct {
	pool *pgxpool.Pool
}

func NewPostgresQuery(p *pgxpool.Pool) DBQuery {
	return &PostgresQuery{pool: p}
}

// Pool exposes the underlying pgx pool for callers that need direct
// connection access (e.g. the ingestion worker for advisory locks).
func (q *PostgresQuery) Pool() *pgxpool.Pool {
	return q.pool
}

const userColumns = `id, email, password_hash, google_sub, email_verified_at,
	subscription_plan, stripe_customer_id, comment_quota_monthly,
	comments_used_this_period, quota_reset_at, token_version,
	failed_login_attempts, locked_until, deletion_requested_at,
	deletion_completed_at, data_export_url, data_export_expires_at,
	digest_email_opt_in, digest_email_sent_at,
	trial_ends_at, subscription_status, current_period_end, billing_interval,
	stripe_subscription_id,
	past_due_since,
	created_at, updated_at`

func scanUser(row pgx.Row) (*User, error) {
	u := &User{}
	err := row.Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.GoogleSub, &u.EmailVerifiedAt,
		&u.SubscriptionPlan, &u.StripeCustomerID, &u.CommentQuotaMonthly,
		&u.CommentsUsedThisPeriod, &u.QuotaResetAt, &u.TokenVersion,
		&u.FailedLoginAttempts, &u.LockedUntil, &u.DeletionRequestedAt,
		&u.DeletionCompletedAt, &u.DataExportURL, &u.DataExportExpiresAt,
		&u.DigestEmailOptIn, &u.DigestEmailSentAt,
		&u.TrialEndsAt, &u.SubscriptionStatus, &u.CurrentPeriodEnd, &u.BillingInterval,
		&u.StripeSubscriptionID,
		&u.PastDueSince,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return u, nil
}

// CreateUser is the legacy helper used by older callers. New code should use
// CreateUserWithPassword.
func (q *PostgresQuery) CreateUser(ctx context.Context, email, passwordHash string) (*User, error) {
	return q.CreateUserWithPassword(ctx, email, passwordHash)
}

func (q *PostgresQuery) CreateUserWithPassword(ctx context.Context, email, passwordHash string) (*User, error) {
	query := `INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING ` + userColumns
	return scanUser(q.pool.QueryRow(ctx, query, email, passwordHash))
}

func (q *PostgresQuery) CreateUserGoogle(ctx context.Context, email, googleSub string) (*User, error) {
	query := `INSERT INTO users (email, google_sub, email_verified_at) VALUES ($1, $2, NOW()) RETURNING ` + userColumns
	return scanUser(q.pool.QueryRow(ctx, query, email, googleSub))
}

func (q *PostgresQuery) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE email = $1`
	u, err := scanUser(q.pool.QueryRow(ctx, query, email))
	if errors.Is(err, ErrNotFound) {
		// Preserve legacy behaviour: callers expect (nil, nil) on miss.
		return nil, nil
	}
	return u, err
}

func (q *PostgresQuery) GetUserByID(ctx context.Context, id string) (*User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE id = $1`
	u, err := scanUser(q.pool.QueryRow(ctx, query, id))
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return u, err
}

func (q *PostgresQuery) GetUserByGoogleSub(ctx context.Context, sub string) (*User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE google_sub = $1`
	u, err := scanUser(q.pool.QueryRow(ctx, query, sub))
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return u, err
}

func (q *PostgresQuery) UpdateUserSubscription(ctx context.Context, userID, plan, stripeCustomerID string, quota int) error {
	var stripeIDPtr *string
	if stripeCustomerID != "" {
		stripeIDPtr = &stripeCustomerID
	}
	query := `
		UPDATE users
		SET subscription_plan = $2,
		    stripe_customer_id = $3,
		    comment_quota_monthly = $4,
		    updated_at = NOW()
		WHERE id = $1
	`
	_, err := q.pool.Exec(ctx, query, userID, plan, stripeIDPtr, quota)
	return err
}

// --- Auth-state mutations ---------------------------------------------------

func (q *PostgresQuery) IncrementFailedLoginAttempts(ctx context.Context, userID string) (int, error) {
	var attempts int
	err := q.pool.QueryRow(ctx,
		`UPDATE users SET failed_login_attempts = failed_login_attempts + 1, updated_at = NOW()
		 WHERE id = $1 RETURNING failed_login_attempts`, userID).Scan(&attempts)
	return attempts, err
}

func (q *PostgresQuery) LockUser(ctx context.Context, userID string, until time.Time) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE users SET locked_until = $2, updated_at = NOW() WHERE id = $1`,
		userID, until)
	return err
}

func (q *PostgresQuery) ResetLoginFailures(ctx context.Context, userID string) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE users SET failed_login_attempts = 0, locked_until = NULL, updated_at = NOW()
		 WHERE id = $1`, userID)
	return err
}

func (q *PostgresQuery) IncrementTokenVersion(ctx context.Context, userID string) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE users SET token_version = token_version + 1, updated_at = NOW() WHERE id = $1`,
		userID)
	return err
}

func (q *PostgresQuery) GetUserTokenVersion(ctx context.Context, userID string) (int, error) {
	var v int
	err := q.pool.QueryRow(ctx, `SELECT token_version FROM users WHERE id = $1`, userID).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return v, err
}

func (q *PostgresQuery) SetEmailVerified(ctx context.Context, userID string) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE users SET email_verified_at = NOW(), updated_at = NOW() WHERE id = $1`, userID)
	return err
}

func (q *PostgresQuery) UpdatePasswordHash(ctx context.Context, userID, hash string) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE users SET password_hash = $2, updated_at = NOW() WHERE id = $1`,
		userID, hash)
	return err
}

func (q *PostgresQuery) LinkGoogleSub(ctx context.Context, userID, sub string) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE users SET google_sub = $2, updated_at = NOW() WHERE id = $1`,
		userID, sub)
	return err
}

func (q *PostgresQuery) DeleteStaleUnverifiedUsers(ctx context.Context, olderThan time.Duration) (int64, error) {
	tag, err := q.pool.Exec(ctx,
		`DELETE FROM users
		 WHERE email_verified_at IS NULL
		   AND created_at < NOW() - make_interval(secs => $1)`,
		olderThan.Seconds())
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// --- Token-table helpers ----------------------------------------------------

func (q *PostgresQuery) InsertEmailVerificationToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	_, err := q.pool.Exec(ctx,
		`INSERT INTO email_verification_tokens (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		userID, tokenHash, expiresAt)
	return err
}

func (q *PostgresQuery) ConsumeEmailVerificationToken(ctx context.Context, tokenHash string) (string, error) {
	var userID string
	err := q.pool.QueryRow(ctx,
		`UPDATE email_verification_tokens
		 SET used_at = NOW()
		 WHERE token_hash = $1
		   AND used_at IS NULL
		   AND expires_at > NOW()
		 RETURNING user_id`,
		tokenHash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return userID, err
}

func (q *PostgresQuery) InvalidateEmailVerificationTokensForUser(ctx context.Context, userID string) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE email_verification_tokens SET used_at = NOW()
		 WHERE user_id = $1 AND used_at IS NULL`, userID)
	return err
}

func (q *PostgresQuery) CountVerificationResendsInWindow(ctx context.Context, userID string, since time.Time) (int, error) {
	var n int
	err := q.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM email_verification_tokens
		 WHERE user_id = $1 AND created_at >= $2`, userID, since).Scan(&n)
	return n, err
}

func (q *PostgresQuery) InsertPasswordResetToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	_, err := q.pool.Exec(ctx,
		`INSERT INTO password_reset_tokens (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		userID, tokenHash, expiresAt)
	return err
}

func (q *PostgresQuery) ConsumePasswordResetToken(ctx context.Context, tokenHash string) (string, error) {
	var userID string
	err := q.pool.QueryRow(ctx,
		`UPDATE password_reset_tokens
		 SET used_at = NOW()
		 WHERE token_hash = $1
		   AND used_at IS NULL
		   AND expires_at > NOW()
		 RETURNING user_id`,
		tokenHash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return userID, err
}

func (q *PostgresQuery) InvalidatePasswordResetTokensForUser(ctx context.Context, userID string) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE password_reset_tokens SET used_at = NOW()
		 WHERE user_id = $1 AND used_at IS NULL`, userID)
	return err
}

func (q *PostgresQuery) CountForgotPasswordEmailsInWindow(ctx context.Context, email string, since time.Time) (int, error) {
	var n int
	err := q.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM password_reset_tokens t
		 JOIN users u ON u.id = t.user_id
		 WHERE u.email = $1 AND t.created_at >= $2`,
		email, since).Scan(&n)
	return n, err
}

func (q *PostgresQuery) InsertRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	_, err := q.pool.Exec(ctx,
		`INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		userID, tokenHash, expiresAt)
	return err
}

func (q *PostgresQuery) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (string, time.Time, error) {
	var userID string
	var expiresAt time.Time
	err := q.pool.QueryRow(ctx,
		`SELECT user_id, expires_at FROM refresh_tokens WHERE token_hash = $1`,
		tokenHash).Scan(&userID, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", time.Time{}, ErrNotFound
	}
	return userID, expiresAt, err
}

func (q *PostgresQuery) DeleteRefreshTokenByHash(ctx context.Context, tokenHash string) error {
	_, err := q.pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE token_hash = $1`, tokenHash)
	return err
}

func (q *PostgresQuery) DeleteAllRefreshTokensForUser(ctx context.Context, userID string) error {
	_, err := q.pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE user_id = $1`, userID)
	return err
}

// --- LLM-processing quota helpers ------------------------------------------

// IncrementCommentsUsed atomically bumps comments_used_this_period for a user.
// The pre-batch quota check is advisory only; this UPDATE is the source of
// truth and is safe under concurrent processors.
func (q *PostgresQuery) IncrementCommentsUsed(ctx context.Context, userID string, count int) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE users
		 SET comments_used_this_period = comments_used_this_period + $1,
		     updated_at = NOW()
		 WHERE id = $2`,
		count, userID)
	return err
}

// GetPrimaryTracker returns the user identified by the active tracker with
// token_priority = 0 for a channel. Returns ErrNoPrimaryTracker when no such
// tracker exists (all paused, removed, or never set).
func (q *PostgresQuery) GetPrimaryTracker(ctx context.Context, platformChannelID string) (*User, error) {
	query := `
		SELECT ` + userColumns + `
		FROM users u
		JOIN user_channel_tracking uct ON uct.user_id = u.id
		WHERE uct.platform_channel_id = $1
		  AND uct.tracking_status = 'active'
		  AND uct.token_priority = 0
		LIMIT 1
	`
	u, err := scanUser(q.pool.QueryRow(ctx, query, platformChannelID))
	if errors.Is(err, ErrNotFound) {
		return nil, ErrNoPrimaryTracker
	}
	return u, err
}
