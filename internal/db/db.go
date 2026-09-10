package db

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// ApplyMigrations runs the versioned migration set (migration v1 is the
// embedded schema.sql baseline) against the package-global DB. It is
// idempotent: applied versions are recorded in schema_migrations and only
// pending migrations run, so it is safe to call repeatedly at startup.
func ApplyMigrations(ctx context.Context) error {
	if DB == nil {
		return fmt.Errorf("db not initialized")
	}
	return Migrate(ctx, DB)
}

var DB *sql.DB

// ExportDB is a second connection pool used only by long export reads
// (fix-export-pipeline). It keeps the export cursor off the main pool's
// single connection so a multi-minute stream cannot starve writers.
// Connections are opened with _pragma=query_only(1): SQLite rejects any
// write through them, making the handle safe for read-only sharing.
// Nil for in-memory databases (a second handle would point at a
// different private DB), in which case callers fall back to DB.
var ExportDB *sql.DB

// InitDB initializes the database connection to SQLite
func InitDB(databasePath string) error {
	// modernc.org/sqlite only honors the driver's DSN parameters
	// (_pragma, _time_format, _txlock, ...); every other key is dropped
	// silently. The old DSN (_journal_mode=/_busy_timeout=/_fk=/_loc=)
	// therefore never applied WAL, busy_timeout or foreign keys in
	// production. These pragmas are applied per connection.
	var err error
	DB, err = sql.Open("sqlite", databasePath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
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

	// Boot-time pragma assertions: a silently-ignored DSN was the root
	// cause of WAL/FK/busy_timeout never being applied. Verify the
	// connection actually runs with the required settings.
	if err := assertPragmas(DB, databasePath); err != nil {
		return fmt.Errorf("sqlite pragma check: %w", err)
	}

	if !strings.HasPrefix(databasePath, ":memory:") {
		ExportDB, err = sql.Open("sqlite", databasePath+
			"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=query_only(1)")
		if err != nil {
			return fmt.Errorf("unable to open export read database: %w", err)
		}
		// Two concurrent export streams (async worker + sync request)
		// must both make progress; 2 connections covers that without
		// letting exports monopolize the file.
		ExportDB.SetMaxOpenConns(2)
		ExportDB.SetMaxIdleConns(2)
	}

	log.Println("Successfully connected to the SQLite database")
	return nil
}

// assertPragmas verifies the pragmas required by the storage-layer spec:
// WAL journaling, foreign keys ON, and a >=5s busy timeout. In-memory
// databases cannot use WAL (journal_mode stays 'memory'), so that check
// is skipped for them.
func assertPragmas(conn *sql.DB, databasePath string) error {
	var journalMode string
	if err := conn.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		return fmt.Errorf("read journal_mode: %w", err)
	}
	if !strings.HasPrefix(databasePath, ":memory:") && journalMode != "wal" {
		return fmt.Errorf("journal_mode=%q, want %q", journalMode, "wal")
	}
	var foreignKeys, busyTimeout int
	if err := conn.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		return fmt.Errorf("read foreign_keys: %w", err)
	}
	if foreignKeys != 1 {
		return fmt.Errorf("foreign_keys=%d, want 1", foreignKeys)
	}
	if err := conn.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		return fmt.Errorf("read busy_timeout: %w", err)
	}
	if busyTimeout < 5000 {
		return fmt.Errorf("busy_timeout=%dms, want >=5000ms", busyTimeout)
	}
	return nil
}

// CloseDB closes the database connection
func CloseDB() {
	if DB != nil {
		DB.Close()
	}
	if ExportDB != nil {
		ExportDB.Close()
		ExportDB = nil
	}
}

// ErrNotFound is returned by lookup helpers when a row is missing.
var ErrNotFound = errors.New("not found")

// NowUTC returns the SQLite expression for the current UTC timestamp in
// the ISO-8601 TEXT format every table stores. Shared so the deletion
// path (handler + worker) doesn't repeat the format string.
func NowUTC() string { return "strftime('%Y-%m-%dT%H:%M:%SZ', 'now')" }

// TrialDuration is the free-trial length granted to brand-new direct-signup
// accounts (register + Google SSO). It is written as users.trial_ends_at at
// user creation; invite-accepted members join an existing workspace and do
// not get their own trial period.
const TrialDuration = 14 * 24 * time.Hour

// ErrNoPrimaryTracker is returned by GetPrimaryTracker when a channel has no
// active tracker with token_priority = 0. The channel is effectively idle.
var ErrNoPrimaryTracker = errors.New("no primary tracker")

// DBQuery defines the database surface used by handlers and workers. It is
// the composition of the narrow per-consumer interfaces; consumers should
// depend on the narrow interface they actually use rather than DBQuery.
type DBQuery interface {
	AuthStore
	BillingStore
	InsightsStore
	ChannelStore
	DigestStore
	LLMJobStore
	WorkspaceStore
	LifecycleStore
	EmailLogStore
	RateLimitStore
}

// SQLiteQuery implements DBQuery using database/sql + SQLite
type SQLiteQuery struct {
	db *sql.DB
	// ro is the optional query_only export handle (db.ExportDB). Long
	// export reads run through it so they never hold the main pool's
	// single connection; nil falls back to db (in-memory test DBs).
	ro *sql.DB
}

func NewSQLiteQuery(d *sql.DB) DBQuery {
	return &SQLiteQuery{db: d}
}

// SetExportReadDB attaches the dedicated query_only export handle (see
// db.ExportDB). Call once after construction when the handle exists.
func (q *SQLiteQuery) SetExportReadDB(d *sql.DB) {
	q.ro = d
}

// ExportReadDB returns the query_only export handle when attached, else
// the main DB. Read-only export streams should use this instead of DB()
// so the export cursor never occupies the main pool's connection.
func (q *SQLiteQuery) ExportReadDB() *sql.DB {
	if q.ro != nil {
		return q.ro
	}
	return q.db
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
	return parseTime(s.String)
}

// CreateUser is the legacy helper used by older callers. New code should use
// CreateUserWithPassword.
func (q *SQLiteQuery) CreateUser(ctx context.Context, email, passwordHash string) (*User, error) {
	return q.CreateUserWithPassword(ctx, email, passwordHash)
}

func (q *SQLiteQuery) CreateUserWithPassword(ctx context.Context, email, passwordHash string) (*User, error) {
	trialEndsAt := time.Now().UTC().Add(TrialDuration).Format(time.RFC3339)
	query := `INSERT INTO users (id, email, password_hash, trial_ends_at, quota_reset_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now', '+30 days'), strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) RETURNING ` + userColumns
	return scanUser(q.db.QueryRowContext(ctx, query, generateUUID(), email, passwordHash, trialEndsAt))
}

func (q *SQLiteQuery) CreateUserGoogle(ctx context.Context, email, googleSub string) (*User, error) {
	trialEndsAt := time.Now().UTC().Add(TrialDuration).Format(time.RFC3339)
	query := `INSERT INTO users (id, email, google_sub, email_verified_at, trial_ends_at, quota_reset_at, created_at, updated_at)
		VALUES (?, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now', '+30 days'), strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) RETURNING ` + userColumns
	return scanUser(q.db.QueryRowContext(ctx, query, generateUUID(), email, googleSub, trialEndsAt))
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
		until.UTC().Format(time.RFC3339), userID)
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
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx,
		`SELECT id FROM users
		 WHERE email_verified_at IS NULL
		   AND created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, tx.Commit()
	}

	// FK cascades are inert when foreign_keys=OFF, so orphan cleanup is
	// explicit and identical under both enforcement states.
	for _, sqlStr := range []string{
		`DELETE FROM email_verification_tokens WHERE user_id = ?`,
		`DELETE FROM password_reset_tokens WHERE user_id = ?`,
		`DELETE FROM refresh_tokens WHERE user_id = ?`,
		`DELETE FROM oauth_states WHERE user_id = ?`,
		`DELETE FROM user_channel_tracking WHERE user_id = ?`,
		`DELETE FROM comment_exports WHERE user_id = ?`,
		`DELETE FROM rate_limit_buckets WHERE user_key = ?`,
	} {
		for _, id := range ids {
			arg := any(id)
			if sqlStr == `DELETE FROM rate_limit_buckets WHERE user_key = ?` {
				arg = "u:" + id
			}
			if _, err := tx.ExecContext(ctx, sqlStr, arg); err != nil {
				return 0, err
			}
		}
	}

	result, err := tx.ExecContext(ctx,
		`DELETE FROM users
		 WHERE email_verified_at IS NULL
		   AND created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// --- Token-table helpers ----------------------------------------------------

func (q *SQLiteQuery) InsertEmailVerificationToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO email_verification_tokens (id, user_id, token_hash, expires_at) VALUES (?, ?, ?, ?)`,
		generateUUID(), userID, tokenHash, expiresAt.UTC().Format(time.RFC3339))
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
		 WHERE user_id = ? AND created_at >= ?`, userID, since.UTC().Format(time.RFC3339)).Scan(&n)
	return n, err
}

// CountEmailChangeResendsInWindow counts the verification emails sent to the
// pending (new) address for an in-flight email change. The ResendVerification
// email-change branch writes email_change_tokens, so it must consult this
// counter rather than CountVerificationResendsInWindow.
func (q *SQLiteQuery) CountEmailChangeResendsInWindow(ctx context.Context, userID string, since time.Time) (int, error) {
	var n int
	err := q.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM email_change_tokens
		 WHERE user_id = ? AND created_at >= ?`, userID, since.UTC().Format(time.RFC3339)).Scan(&n)
	return n, err
}

// --- Email-change tokens (fix-frontend-plumbing) ---------------------------

func (q *SQLiteQuery) InsertEmailChangeToken(ctx context.Context, userID, pendingEmail, tokenHash string, expiresAt time.Time) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO email_change_tokens (id, user_id, pending_email, token_hash, expires_at) VALUES (?, ?, ?, ?, ?)`,
		generateUUID(), userID, pendingEmail, tokenHash, expiresAt.UTC().Format(time.RFC3339))
	return err
}

func (q *SQLiteQuery) ConsumeEmailChangeToken(ctx context.Context, tokenHash string) (string, string, error) {
	var userID, pendingEmail string
	err := q.db.QueryRowContext(ctx,
		`UPDATE email_change_tokens
		 SET used_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		 WHERE token_hash = ?
		   AND used_at IS NULL
		   AND expires_at > strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		 RETURNING user_id, pending_email`,
		tokenHash).Scan(&userID, &pendingEmail)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return userID, pendingEmail, err
}

func (q *SQLiteQuery) GetActiveEmailChange(ctx context.Context, userID string) (*EmailChange, error) {
	e := &EmailChange{}
	var expiresAt, createdAt string
	var usedAt *string
	err := q.db.QueryRowContext(ctx,
		`SELECT id, user_id, pending_email, token_hash, expires_at, used_at, created_at
		 FROM email_change_tokens
		 WHERE user_id = ? AND used_at IS NULL
		 ORDER BY created_at DESC LIMIT 1`,
		userID).Scan(&e.ID, &e.UserID, &e.PendingEmail, &e.TokenHash, &expiresAt, &usedAt, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	e.ExpiresAt = parseTime(expiresAt)
	e.CreatedAt = parseTime(createdAt)
	if usedAt != nil {
		t := parseTime(*usedAt)
		e.UsedAt = &t
	}
	return e, nil
}

func (q *SQLiteQuery) InvalidateEmailChangeTokensForUser(ctx context.Context, userID string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE email_change_tokens SET used_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		 WHERE user_id = ? AND used_at IS NULL`, userID)
	return err
}

func (q *SQLiteQuery) ApplyEmailChange(ctx context.Context, userID, newEmail string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE users
		 SET email = ?, email_verified_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
		     token_version = token_version + 1
		 WHERE id = ?`, newEmail, userID)
	return err
}

func (q *SQLiteQuery) InsertPasswordResetToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at) VALUES (?, ?, ?, ?)`,
		generateUUID(), userID, tokenHash, expiresAt.UTC().Format(time.RFC3339))
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
	// Count actual send attempts for this address, not issued tokens: the
	// Google-only branch (no token) writes an email_send_log row but no
	// password_reset_tokens row, so token-counting let it bypass the cap.
	// The template literal must match email.TemplatePasswordReset.
	var n int
	err := q.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM email_send_log
		 WHERE to_address = ? AND template = 'password_reset' AND created_at >= ?`,
		email, since.UTC().Format(time.RFC3339)).Scan(&n)
	return n, err
}

func (q *SQLiteQuery) InsertRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at) VALUES (?, ?, ?, ?)`,
		generateUUID(), userID, tokenHash, expiresAt.UTC().Format(time.RFC3339))
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
	// Columns are explicitly u.-qualified: the JOIN with user_channel_tracking
	// also carries an `id` column, and an unqualified list makes SQLite raise
	// "ambiguous column name: id".
	query := `
		SELECT u.id, u.email, u.password_hash, u.google_sub, u.email_verified_at,
		       u.subscription_plan, u.stripe_customer_id, u.comment_quota_monthly,
		       u.comments_used_this_period, u.quota_reset_at, u.token_version,
		       u.failed_login_attempts, u.locked_until, u.deletion_requested_at,
		       u.deletion_completed_at, u.data_export_url, u.data_export_expires_at,
		       u.digest_email_opt_in, u.digest_email_sent_at,
		       u.trial_ends_at, u.subscription_status, u.current_period_end, u.billing_interval,
		       u.stripe_subscription_id,
		       u.past_due_since, u.active_workspace_id,
		       u.created_at, u.updated_at
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
