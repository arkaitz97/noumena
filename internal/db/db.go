package db

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// ApplyMigrations executes the embedded schema.sql against the DB. It is
// idempotent: every CREATE/ALTER uses IF NOT EXISTS guards, so it is safe to
// run repeatedly at startup.
func ApplyMigrations(ctx context.Context) error {
	if DB == nil {
		return fmt.Errorf("db not initialized")
	}
	if _, err := DB.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

var DB *sql.DB

// InitDB initializes the database connection to SQLite
func InitDB(databasePath string) error {
	var err error
	DB, err = sql.Open("sqlite", databasePath+"?_journal_mode=WAL&_busy_timeout=5000&_txlock=immediate&_fk=true&_loc=UTC")
	if err != nil {
		return fmt.Errorf("unable to open sqlite database: %w", err)
	}

	// Ping the DB
	if err := DB.Ping(); err != nil {
		return fmt.Errorf("unable to ping database: %w", err)
	}

	// Set connection pool options for SQLite
	DB.SetMaxOpenConns(1) // SQLite serializes writes
	DB.SetMaxIdleConns(1)

	log.Println("Successfully connected to the SQLite database")
	return nil
}

// CloseDB closes the database connection
func CloseDB() {
	if DB != nil {
		DB.Close()
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
	UpdateCommentLLMData(ctx context.Context, id, category, topic, provider, model string, actionableInsights []byte, confidence float64) error

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
	UpdateChannelCustomPrompt(ctx context.Context, userID, platformChannelID string, customPrompt *string) error

	// --- Billing additions (billing change) ---
	IsStripeEventProcessed(ctx context.Context, eventID string) (bool, error)
	InsertProcessedStripeEvent(ctx context.Context, tx *sql.Tx, eventID, eventType string) error
	ApplySubscriptionUpdate(ctx context.Context, tx *sql.Tx, u SubscriptionUpdate) error
	GetUserByStripeCustomerID(ctx context.Context, customerID string) (*User, error)
	GetActiveChannelCount(ctx context.Context, userID string) (int, error)
	GetOverageCostForCurrentPeriod(ctx context.Context, userID string, periodStart, periodEnd time.Time) (float64, error)
	InsertCommentUsageLog(ctx context.Context, tx *sql.Tx, userID string, count int, periodStart, periodEnd time.Time) error
	PauseChannelsAlphabetically(ctx context.Context, tx *sql.Tx, userID string, keepCount int) error
	ResetFreeTierQuotas(ctx context.Context) (int64, error)
	ExpirePastDueGrace(ctx context.Context, graceHours int) (int64, error)

	// --- Transactional email log (data-lifecycle change) ---
	// Embedded so handlers/workers that hold a DBQuery can also push to
	// the email pipeline without an extra constructor.
	EmailLogStore

	// --- Account-deletion + GDPR-export workflows (data-lifecycle change) ---
	LifecycleStore

	// --- Multi-seat workspaces (spec-gap-improvements) ---
	CreateWorkspace(ctx context.Context, name, createdBy string) (*Workspace, error)
	GetWorkspace(ctx context.Context, workspaceID string) (*Workspace, error)
	UpdateWorkspace(ctx context.Context, workspaceID, name string) error
	DeleteWorkspace(ctx context.Context, workspaceID string) error
	ListWorkspacesForUser(ctx context.Context, userID string) ([]*WorkspaceWithMembers, error)
	GetWorkspaceMember(ctx context.Context, workspaceID, userID string) (*WorkspaceMember, error)
	AddWorkspaceMember(ctx context.Context, workspaceID, userID, role string) error
	RemoveWorkspaceMember(ctx context.Context, workspaceID, userID string) error
	UpdateWorkspaceMemberRole(ctx context.Context, workspaceID, userID, role string) error
	CountWorkspaceMembers(ctx context.Context, workspaceID string) (int, error)
	CountWorkspaceAdmins(ctx context.Context, workspaceID string) (int, error)
	CreateWorkspaceInvite(ctx context.Context, workspaceID, email, role, tokenHash string, expiresAt time.Time) (*WorkspaceInvite, error)
	GetWorkspaceInviteByToken(ctx context.Context, tokenHash string) (*WorkspaceInvite, error)
	ConsumeWorkspaceInvite(ctx context.Context, inviteID string) error
	GetWorkspaceInviteByEmail(ctx context.Context, workspaceID, email string) (*WorkspaceInvite, error)
	SetUserActiveWorkspace(ctx context.Context, userID, workspaceID string) error
	GetUserActiveWorkspaceID(ctx context.Context, userID string) (string, error)
	IsWorkspaceOwner(ctx context.Context, workspaceID, userID string) (bool, error)

	// --- Audit log (spec-gap-improvements) ---
	InsertAuditLog(ctx context.Context, userID, action string, details string, ipAddress string) error

	// --- Rate limiting (spec-gap-improvements) ---
	IncrementRateLimit(ctx context.Context, userKey string, windowStart time.Time, limit int) (count int, err error)
	GetRateLimitStatus(ctx context.Context, userKey string, windowStart time.Time, limit int) (*RateLimitStatus, error)
	CleanupExpiredRateLimitBuckets(ctx context.Context, before time.Time) (int64, error)

	// LLM processing additions (spec-gap-improvements)
	GetChannelCustomPrompt(ctx context.Context, platformChannelID, userID string) (string, error)
	GetCommentBodyByID(ctx context.Context, commentID string) (string, error)
}

// SQLiteQuery implements DBQuery using database/sql + SQLite
type SQLiteQuery struct {
	db *sql.DB
}

func NewSQLiteQuery(d *sql.DB) DBQuery {
	return &SQLiteQuery{db: d}
}

// DB exposes the underlying *sql.DB for callers that need direct access.
func (q *SQLiteQuery) DB() *sql.DB {
	return q.db
}

const userColumns = `id, email, password_hash, google_sub, email_verified_at,
	subscription_plan, stripe_customer_id, comment_quota_monthly,
	comments_used_this_period, quota_reset_at, token_version,
	failed_login_attempts, locked_until, deletion_requested_at,
	deletion_completed_at, data_export_url, data_export_expires_at,
	digest_email_opt_in, digest_email_sent_at,
	trial_ends_at, subscription_status, current_period_end, billing_interval,
	stripe_subscription_id,
	past_due_since, active_workspace_id,
	created_at, updated_at`

func scanUser(row *sql.Row) (*User, error) {
	u := &User{}
	var emailVerifiedAt, lockedUntil, deletionRequestedAt, deletionCompletedAt, dataExportExpiresAt, digestEmailSentAt, trialEndsAt, currentPeriodEnd, pastDueSince, createdAt, updatedAt, quotaResetAt sql.NullString
	err := row.Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.GoogleSub, &emailVerifiedAt,
		&u.SubscriptionPlan, &u.StripeCustomerID, &u.CommentQuotaMonthly,
		&u.CommentsUsedThisPeriod, &quotaResetAt, &u.TokenVersion,
		&u.FailedLoginAttempts, &lockedUntil, &deletionRequestedAt,
		&deletionCompletedAt, &u.DataExportURL, &dataExportExpiresAt,
		&u.DigestEmailOptIn, &digestEmailSentAt,
		&trialEndsAt, &u.SubscriptionStatus, &currentPeriodEnd, &u.BillingInterval,
		&u.StripeSubscriptionID,
		&u.PastDueSince, &u.ActiveWorkspaceID,
		&createdAt, &updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	u.EmailVerifiedAt = scanTimePtr(emailVerifiedAt)
	u.LockedUntil = scanTimePtr(lockedUntil)
	u.DeletionRequestedAt = scanTimePtr(deletionRequestedAt)
	u.DeletionCompletedAt = scanTimePtr(deletionCompletedAt)
	u.DataExportExpiresAt = scanTimePtr(dataExportExpiresAt)
	u.DigestEmailSentAt = scanTimePtr(digestEmailSentAt)
	u.TrialEndsAt = scanTimePtr(trialEndsAt)
	u.CurrentPeriodEnd = scanTimePtr(currentPeriodEnd)
	u.PastDueSince = scanTimePtr(pastDueSince)
	u.CreatedAt = parseTimeNull(createdAt)
	u.UpdatedAt = parseTimeNull(updatedAt)
	u.QuotaResetAt = scanTimePtr(quotaResetAt)
	return u, nil
}

func parseTimeNull(s sql.NullString) time.Time {
	if !s.Valid || s.String == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s.String)
	if err != nil {
		return time.Time{}
	}
	return t
}

// CreateUser is the legacy helper used by older callers. New code should use
// CreateUserWithPassword.
func (q *SQLiteQuery) CreateUser(ctx context.Context, email, passwordHash string) (*User, error) {
	return q.CreateUserWithPassword(ctx, email, passwordHash)
}

func (q *SQLiteQuery) CreateUserWithPassword(ctx context.Context, email, passwordHash string) (*User, error) {
	query := `INSERT INTO users (id, email, password_hash, created_at, updated_at) VALUES (?, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) RETURNING ` + userColumns
	return scanUser(q.db.QueryRowContext(ctx, query, generateUUID(), email, passwordHash))
}

func (q *SQLiteQuery) CreateUserGoogle(ctx context.Context, email, googleSub string) (*User, error) {
	query := `INSERT INTO users (id, email, google_sub, email_verified_at, created_at, updated_at) VALUES (?, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) RETURNING ` + userColumns
	return scanUser(q.db.QueryRowContext(ctx, query, generateUUID(), email, googleSub))
}

func (q *SQLiteQuery) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE email = ?`
	u, err := scanUser(q.db.QueryRowContext(ctx, query, email))
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return u, err
}

func (q *SQLiteQuery) GetUserByID(ctx context.Context, id string) (*User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE id = ?`
	u, err := scanUser(q.db.QueryRowContext(ctx, query, id))
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return u, err
}

func (q *SQLiteQuery) GetUserByGoogleSub(ctx context.Context, sub string) (*User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE google_sub = ?`
	u, err := scanUser(q.db.QueryRowContext(ctx, query, sub))
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return u, err
}

func (q *SQLiteQuery) UpdateUserSubscription(ctx context.Context, userID, plan, stripeCustomerID string, quota int) error {
	var stripeIDPtr *string
	if stripeCustomerID != "" {
		stripeIDPtr = &stripeCustomerID
	}
	query := `
		UPDATE users
		SET subscription_plan = ?,
		    stripe_customer_id = ?,
		    comment_quota_monthly = ?,
		    updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		WHERE id = ?
	`
	_, err := q.db.ExecContext(ctx, query, plan, stripeIDPtr, quota, userID)
	return err
}

// --- Auth-state mutations ---------------------------------------------------

func (q *SQLiteQuery) IncrementFailedLoginAttempts(ctx context.Context, userID string) (int, error) {
	var attempts int
	err := q.db.QueryRowContext(ctx,
		`UPDATE users SET failed_login_attempts = failed_login_attempts + 1, updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		 WHERE id = ? RETURNING failed_login_attempts`, userID).Scan(&attempts)
	return attempts, err
}

func (q *SQLiteQuery) LockUser(ctx context.Context, userID string, until time.Time) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE users SET locked_until = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ?`,
		until.Format(time.RFC3339), userID)
	return err
}

func (q *SQLiteQuery) ResetLoginFailures(ctx context.Context, userID string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE users SET failed_login_attempts = 0, locked_until = NULL, updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		 WHERE id = ?`, userID)
	return err
}

func (q *SQLiteQuery) IncrementTokenVersion(ctx context.Context, userID string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE users SET token_version = token_version + 1, updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ?`,
		userID)
	return err
}

func (q *SQLiteQuery) GetUserTokenVersion(ctx context.Context, userID string) (int, error) {
	var v int
	err := q.db.QueryRowContext(ctx, `SELECT token_version FROM users WHERE id = ?`, userID).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return v, err
}

func (q *SQLiteQuery) SetEmailVerified(ctx context.Context, userID string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE users SET email_verified_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ?`, userID)
	return err
}

func (q *SQLiteQuery) UpdatePasswordHash(ctx context.Context, userID, hash string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ?`,
		hash, userID)
	return err
}

func (q *SQLiteQuery) LinkGoogleSub(ctx context.Context, userID, sub string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE users SET google_sub = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ?`,
		sub, userID)
	return err
}

func (q *SQLiteQuery) DeleteStaleUnverifiedUsers(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339)
	result, err := q.db.ExecContext(ctx,
		`DELETE FROM users
		 WHERE email_verified_at IS NULL
		   AND created_at < ?`,
		cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// --- Token-table helpers ----------------------------------------------------

func (q *SQLiteQuery) InsertEmailVerificationToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO email_verification_tokens (id, user_id, token_hash, expires_at) VALUES (?, ?, ?, ?)`,
		generateUUID(), userID, tokenHash, expiresAt.Format(time.RFC3339))
	return err
}

func (q *SQLiteQuery) ConsumeEmailVerificationToken(ctx context.Context, tokenHash string) (string, error) {
	var userID string
	err := q.db.QueryRowContext(ctx,
		`UPDATE email_verification_tokens
		 SET used_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		 WHERE token_hash = ?
		   AND used_at IS NULL
		   AND expires_at > strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		 RETURNING user_id`,
		tokenHash).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return userID, err
}

func (q *SQLiteQuery) InvalidateEmailVerificationTokensForUser(ctx context.Context, userID string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE email_verification_tokens SET used_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		 WHERE user_id = ? AND used_at IS NULL`, userID)
	return err
}

func (q *SQLiteQuery) CountVerificationResendsInWindow(ctx context.Context, userID string, since time.Time) (int, error) {
	var n int
	err := q.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM email_verification_tokens
		 WHERE user_id = ? AND created_at >= ?`, userID, since.Format(time.RFC3339)).Scan(&n)
	return n, err
}

func (q *SQLiteQuery) InsertPasswordResetToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at) VALUES (?, ?, ?, ?)`,
		generateUUID(), userID, tokenHash, expiresAt.Format(time.RFC3339))
	return err
}

func (q *SQLiteQuery) ConsumePasswordResetToken(ctx context.Context, tokenHash string) (string, error) {
	var userID string
	err := q.db.QueryRowContext(ctx,
		`UPDATE password_reset_tokens
		 SET used_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		 WHERE token_hash = ?
		   AND used_at IS NULL
		   AND expires_at > strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		 RETURNING user_id`,
		tokenHash).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return userID, err
}

func (q *SQLiteQuery) InvalidatePasswordResetTokensForUser(ctx context.Context, userID string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE password_reset_tokens SET used_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		 WHERE user_id = ? AND used_at IS NULL`, userID)
	return err
}

func (q *SQLiteQuery) CountForgotPasswordEmailsInWindow(ctx context.Context, email string, since time.Time) (int, error) {
	var n int
	err := q.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM password_reset_tokens t
		 JOIN users u ON u.id = t.user_id
		 WHERE u.email = ? AND t.created_at >= ?`,
		email, since.Format(time.RFC3339)).Scan(&n)
	return n, err
}

func (q *SQLiteQuery) InsertRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at) VALUES (?, ?, ?, ?)`,
		generateUUID(), userID, tokenHash, expiresAt.Format(time.RFC3339))
	return err
}

func (q *SQLiteQuery) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (string, time.Time, error) {
	var userID string
	var expiresAtStr string
	err := q.db.QueryRowContext(ctx,
		`SELECT user_id, expires_at FROM refresh_tokens WHERE token_hash = ?`,
		tokenHash).Scan(&userID, &expiresAtStr)
	if errors.Is(err, sql.ErrNoRows) {
		return "", time.Time{}, ErrNotFound
	}
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt, _ := time.Parse(time.RFC3339, expiresAtStr)
	return userID, expiresAt, err
}

func (q *SQLiteQuery) DeleteRefreshTokenByHash(ctx context.Context, tokenHash string) error {
	_, err := q.db.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE token_hash = ?`, tokenHash)
	return err
}

func (q *SQLiteQuery) DeleteAllRefreshTokensForUser(ctx context.Context, userID string) error {
	_, err := q.db.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE user_id = ?`, userID)
	return err
}

// --- LLM-processing quota helpers ------------------------------------------

func (q *SQLiteQuery) IncrementCommentsUsed(ctx context.Context, userID string, count int) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE users
		 SET comments_used_this_period = comments_used_this_period + ?,
		     updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		 WHERE id = ?`,
		count, userID)
	return err
}

func (q *SQLiteQuery) GetPrimaryTracker(ctx context.Context, platformChannelID string) (*User, error) {
	query := `
		SELECT ` + userColumns + `
		FROM users u
		JOIN user_channel_tracking uct ON uct.user_id = u.id
		WHERE uct.platform_channel_id = ?
		  AND uct.tracking_status = 'active'
		  AND uct.token_priority = 0
		LIMIT 1
	`
	u, err := scanUser(q.db.QueryRowContext(ctx, query, platformChannelID))
	if errors.Is(err, ErrNotFound) {
		return nil, ErrNoPrimaryTracker
	}
	return u, err
}

// generateUUID creates a new UUID string using github.com/google/uuid
func generateUUID() string {
	return uuid.New().String()
}
