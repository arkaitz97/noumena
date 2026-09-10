package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
)

// Versioned SQLite migrations.
//
// Each migration runs inside its own transaction and, on success, records
// its version in schema_migrations(version, applied_at). Migrations are
// append-only: never edit or renumber a migration that has shipped —
// add a new entry instead. Migration v1 is the embedded schema.sql
// baseline (CREATE IF NOT EXISTS + legacy dedupe cleanup), so it applies
// cleanly to both empty databases and databases created by the old boot
// script.
//
// Later migrations must tolerate both shapes they can encounter:
//   - a database created by v1 (fresh install): guards make them no-ops;
//   - a database created by the pre-migration boot script: they repair it.
//
// Guarding is done in Go (column/index introspection) because SQLite has
// no portable ALTER TABLE ... IF NOT EXISTS for columns, and CHECK
// constraint changes require a full table rebuild.
//
// noFK migrations (users table rebuild) disable foreign-key enforcement
// for the duration, following SQLite's documented 12-step procedure. The
// caller's pool MUST be limited to a single connection for the pragma to
// affect the migration's transaction.

type migration struct {
	version int
	name    string
	noFK    bool
	run     func(ctx context.Context, tx *sql.Tx) error
}

var migrations = []migration{
	{
		version: 1,
		name:    "baseline schema.sql",
		run: func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, schemaSQL); err != nil {
				return fmt.Errorf("exec baseline schema: %w", err)
			}
			return nil
		},
	},
	{
		version: 2,
		name:    "align deletion/export job columns",
		run: func(ctx context.Context, tx *sql.Tx) error {
			// account_deletion_jobs.last_error -> error_message.
			if err := renameColumnIfPresent(ctx, tx, "account_deletion_jobs", "last_error", "error_message"); err != nil {
				return err
			}
			// data_export_jobs: storage_path/download_url -> export_url.
			// download_url wins when both exist (it's the URL; the local
			// path derives from storage dir + user + job ids).
			if err := renameColumnIfPresent(ctx, tx, "data_export_jobs", "download_url", "export_url"); err != nil {
				return err
			}
			if err := renameColumnIfPresent(ctx, tx, "data_export_jobs", "storage_path", "export_url"); err != nil {
				return err
			}
			if err := renameColumnIfPresent(ctx, tx, "data_export_jobs", "last_error", "error_message"); err != nil {
				return err
			}
			// partial_export no longer exists; drop if a legacy DB has it.
			has, err := columnExists(ctx, tx, "data_export_jobs", "partial_export")
			if err != nil {
				return err
			}
			if has {
				if _, err := tx.ExecContext(ctx, `ALTER TABLE data_export_jobs DROP COLUMN partial_export`); err != nil {
					return fmt.Errorf("drop partial_export: %w", err)
				}
			}
			return nil
		},
	},
	{
		version: 3,
		name:    "subscription_status CHECK includes trialing",
		noFK:    true,
		run: func(ctx context.Context, tx *sql.Tx) error {
			usersSQL := `SELECT sql FROM sqlite_master WHERE type='table' AND name='users'`
			var def string
			if err := tx.QueryRowContext(ctx, usersSQL).Scan(&def); err != nil {
				return fmt.Errorf("read users table def: %w", err)
			}
			if strings.Contains(def, "'trialing'") {
				return nil // fresh installs (v1) already allow trialing
			}
			// Legacy pre-migration CHECK. Rebuild users with the current
			// column set + trialing (must mirror schema.sql's v1 DDL).
			if _, err := tx.ExecContext(ctx, `
CREATE TABLE users_new (
    id TEXT PRIMARY KEY,
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT,
    google_sub TEXT UNIQUE,
    email_verified_at TEXT,
    subscription_plan TEXT NOT NULL DEFAULT 'free',
    stripe_customer_id TEXT,
    comment_quota_monthly INT NOT NULL DEFAULT 1000,
    comments_used_this_period INT NOT NULL DEFAULT 0,
    quota_reset_at TEXT,
    token_version INT NOT NULL DEFAULT 0,
    failed_login_attempts INT NOT NULL DEFAULT 0,
    locked_until TEXT,
    deletion_requested_at TEXT,
    deletion_completed_at TEXT,
    data_export_url TEXT,
    data_export_expires_at TEXT,
    digest_email_opt_in BOOLEAN NOT NULL DEFAULT 1,
    digest_email_sent_at TEXT,
    trial_ends_at TEXT,
    subscription_status TEXT NOT NULL DEFAULT 'active'
        CHECK (subscription_status IN ('active','past_due','paused','canceled','trialing')),
    current_period_end TEXT,
    billing_interval TEXT CHECK (billing_interval IS NULL OR billing_interval IN ('month','year')),
    stripe_subscription_id TEXT,
    past_due_since TEXT,
    active_workspace_id TEXT REFERENCES workspaces(id),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);`); err != nil {
				return fmt.Errorf("create users_new: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `
INSERT INTO users_new (id, email, password_hash, google_sub, email_verified_at,
    subscription_plan, stripe_customer_id, comment_quota_monthly,
    comments_used_this_period, quota_reset_at, token_version,
    failed_login_attempts, locked_until, deletion_requested_at,
    deletion_completed_at, data_export_url, data_export_expires_at,
    digest_email_opt_in, digest_email_sent_at, trial_ends_at,
    subscription_status, current_period_end, billing_interval,
    stripe_subscription_id, past_due_since, active_workspace_id,
    created_at, updated_at)
SELECT id, email, password_hash, google_sub, email_verified_at,
    subscription_plan, stripe_customer_id, comment_quota_monthly,
    comments_used_this_period, quota_reset_at, token_version,
    failed_login_attempts, locked_until, deletion_requested_at,
    deletion_completed_at, data_export_url, data_export_expires_at,
    digest_email_opt_in, digest_email_sent_at, trial_ends_at,
    subscription_status, current_period_end, billing_interval,
    stripe_subscription_id, past_due_since, active_workspace_id,
    created_at, updated_at
FROM users`); err != nil {
				return fmt.Errorf("copy users: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `DROP TABLE users`); err != nil {
				return fmt.Errorf("drop users: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `ALTER TABLE users_new RENAME TO users`); err != nil {
				return fmt.Errorf("rename users_new: %w", err)
			}
			// Recreate the indexes that were dropped with the old table.
			for _, idx := range []string{
				`CREATE UNIQUE INDEX IF NOT EXISTS users_stripe_subscription_id_uniq
					ON users (stripe_subscription_id)
					WHERE stripe_subscription_id IS NOT NULL`,
				`CREATE INDEX IF NOT EXISTS idx_users_subplan_quota_reset
					ON users(subscription_plan, quota_reset_at)`,
				`CREATE INDEX IF NOT EXISTS idx_users_pending_deletion
					ON users(deletion_requested_at)
					WHERE deletion_requested_at IS NOT NULL`,
			} {
				if _, err := tx.ExecContext(ctx, idx); err != nil {
					return fmt.Errorf("recreate users index: %w", err)
				}
			}
			return nil
		},
	},
	{
		version: 4,
		name:    "rebuild comments_fts",
		run: func(ctx context.Context, tx *sql.Tx) error {
			// Backfill the external-content FTS index so comments that
			// predate comments_fts become searchable. Safe on fresh DBs
			// (empty index rebuild) and idempotent.
			if _, err := tx.ExecContext(ctx, `INSERT INTO comments_fts(comments_fts) VALUES('rebuild')`); err != nil {
				return fmt.Errorf("rebuild comments_fts: %w", err)
			}
			return nil
		},
	},
	{
		version: 5,
		name:    "hot-path indexes",
		run: func(ctx context.Context, tx *sql.Tx) error {
			for _, idx := range []string{
				// LLM-processing poll: unprocessed comments.
				`CREATE INDEX IF NOT EXISTS idx_comments_processed
					ON comments(processed_at)
					WHERE processed_at IS NULL`,
				// Digest eligibility scan (status + week ordering).
				`CREATE INDEX IF NOT EXISTS idx_weekly_digests_status_created
					ON weekly_digests(status, created_at)`,
				// Deletion-sweep scan over channels.
				`CREATE INDEX IF NOT EXISTS idx_platform_channels_deletion_scheduled_at
					ON platform_channels(deletion_scheduled_at)
					WHERE deletion_scheduled_at IS NOT NULL`,
			} {
				if _, err := tx.ExecContext(ctx, idx); err != nil {
					return fmt.Errorf("create index: %w", err)
				}
			}
			return nil
		},
	},
	{
		version: 6,
		name:    "account_deletion_jobs revoke status columns",
		run: func(ctx context.Context, tx *sql.Tx) error {
			for _, col := range []struct{ name, ddl string }{
				{"revoke_attempted", "INTEGER NOT NULL DEFAULT 0"},
				{"revoke_error", "TEXT"},
			} {
				has, err := columnExists(ctx, tx, "account_deletion_jobs", col.name)
				if err != nil {
					return err
				}
				if has {
					continue
				}
				if _, err := tx.ExecContext(ctx,
					fmt.Sprintf("ALTER TABLE account_deletion_jobs ADD COLUMN %s %s", col.name, col.ddl)); err != nil {
					return fmt.Errorf("add account_deletion_jobs.%s: %w", col.name, err)
				}
			}
			return nil
		},
	},
	{
		version: 7,
		name:    "backfill quota_reset_at for existing users",
		run: func(ctx context.Context, tx *sql.Tx) error {
			// Free-tier reset cron skips users with NULL quota_reset_at
			// (they could never reset and stayed blocked at quota). Give
			// every such user a schedule; paid plans overwrite it via the
			// billing webhook.
			if _, err := tx.ExecContext(ctx,
				`UPDATE users
				 SET quota_reset_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now', '+30 days')
				 WHERE quota_reset_at IS NULL`); err != nil {
				return fmt.Errorf("backfill quota_reset_at: %w", err)
			}
			return nil
		},
	},
	{
		version: 8,
		name:    "email_send_log persist rendered body + recipient",
		run: func(ctx context.Context, tx *sql.Tx) error {
			// Retry worker re-sends the persisted body (fix-email-delivery).
			// New columns are non-null w/ empty default so legacy pending
			// rows still scan; the worker marks address-less rows failed.
			for _, col := range []struct{ name, ddl string }{
				{"to_address", "TEXT NOT NULL DEFAULT ''"},
				{"subject", "TEXT NOT NULL DEFAULT ''"},
				{"html_body", "TEXT NOT NULL DEFAULT ''"},
				{"text_body", "TEXT NOT NULL DEFAULT ''"},
			} {
				has, err := columnExists(ctx, tx, "email_send_log", col.name)
				if err != nil {
					return err
				}
				if has {
					continue
				}
				if _, err := tx.ExecContext(ctx,
					fmt.Sprintf("ALTER TABLE email_send_log ADD COLUMN %s %s", col.name, col.ddl)); err != nil {
					return fmt.Errorf("add email_send_log.%s: %w", col.name, err)
				}
			}
			return nil
		},
	},
	{
		version: 9,
		name:    "channel grants + backfill tracking workspace",
		run: func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS channel_grants (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    channel_id TEXT NOT NULL REFERENCES platform_channels(id) ON DELETE CASCADE,
    granted_by TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE(workspace_id, channel_id)
)`); err != nil {
				return fmt.Errorf("create channel_grants: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_channel_grants_workspace
    ON channel_grants(workspace_id, channel_id)`); err != nil {
				return fmt.Errorf("create idx_channel_grants_workspace: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `UPDATE user_channel_tracking SET workspace_id=(SELECT active_workspace_id FROM users WHERE users.id=user_channel_tracking.user_id) WHERE workspace_id IS NULL AND EXISTS(SELECT 1 FROM users WHERE users.id=user_channel_tracking.user_id AND active_workspace_id IS NOT NULL)`); err != nil {
				return fmt.Errorf("backfill tracking workspace_id: %w", err)
			}
			return nil
		},
	},
	{
		version: 10,
		name:    "email change tokens (fix-frontend-plumbing)",
		run: func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS email_change_tokens (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    pending_email TEXT NOT NULL,
    token_hash TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    used_at TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
)`); err != nil {
				return fmt.Errorf("create email_change_tokens: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS idx_email_change_tokens_token_hash
    ON email_change_tokens(token_hash)`); err != nil {
				return fmt.Errorf("create idx_email_change_tokens_token_hash: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS idx_email_change_tokens_user_active
    ON email_change_tokens(user_id) WHERE used_at IS NULL`); err != nil {
				return fmt.Errorf("create idx_email_change_tokens_user_active: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_email_change_tokens_expires_at
    ON email_change_tokens(expires_at)`); err != nil {
				return fmt.Errorf("create idx_email_change_tokens_expires_at: %w", err)
			}
			return nil
		},
	},
	{
		version: 11,
		name:    "comments sentiment_score column",
		run: func(ctx context.Context, tx *sql.Tx) error {
			// NULL = not yet scored; -1/0/1 after LLM classification.
			has, err := columnExists(ctx, tx, "comments", "sentiment_score")
			if err != nil {
				return err
			}
			if has {
				return nil
			}
			if _, err := tx.ExecContext(ctx,
				`ALTER TABLE comments ADD COLUMN sentiment_score INTEGER`); err != nil {
				return fmt.Errorf("add comments.sentiment_score: %w", err)
			}
			return nil
		},
	},
	{
		version: 12,
		name:    "billing_usage table",
		run: func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS billing_usage (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    period_start TEXT NOT NULL,
    period_end TEXT NOT NULL,
    comments_used INT NOT NULL,
    quota INT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE(user_id, period_start)
)`); err != nil {
				return fmt.Errorf("create billing_usage: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_billing_usage_user_period
    ON billing_usage(user_id, period_start)`); err != nil {
				return fmt.Errorf("create idx_billing_usage_user_period: %w", err)
			}
			return nil
		},
	},
	{
		version: 13,
		name:    "overage_invoices guard table",
		run: func(ctx context.Context, tx *sql.Tx) error {
			// implement-missing-product-features 8.x: exactly-once guard for
			// overage invoicing. UNIQUE(user_id, period_start) makes the
			// mark-invoiced write idempotent; the invoicing job only picks
			// up overage groups with no matching row.
			if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS overage_invoices (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    period_start TEXT NOT NULL,
    stripe_invoice_id TEXT,
    amount_cents INT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE(user_id, period_start)
)`); err != nil {
				return fmt.Errorf("create overage_invoices: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_overage_invoices_user_period
    ON overage_invoices(user_id, period_start)`); err != nil {
				return fmt.Errorf("create idx_overage_invoices_user_period: %w", err)
			}
			return nil
		},
	},
	{
		version: 14,
		name:    "comments video context columns",
		run: func(ctx context.Context, tx *sql.Tx) error {
			// NULL = comment ingested before video context was captured
			// (feed hides the video line, export emits empty cells).
			for _, col := range []struct{ name, ddl string }{
				{"video_id", "TEXT"},
				{"video_title", "TEXT"},
			} {
				has, err := columnExists(ctx, tx, "comments", col.name)
				if err != nil {
					return err
				}
				if has {
					continue
				}
				if _, err := tx.ExecContext(ctx,
					fmt.Sprintf("ALTER TABLE comments ADD COLUMN %s %s", col.name, col.ddl)); err != nil {
					return fmt.Errorf("add comments.%s: %w", col.name, err)
				}
			}
			return nil
		},
	},
	{
		version: 15,
		name:    "user_profiles table",
		run: func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS user_profiles (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    display_name TEXT CHECK (display_name IS NULL OR length(display_name) <= 100),
    user_type TEXT CHECK (user_type IS NULL OR user_type IN ('creator', 'agency', 'brand', 'other')),
    primary_goal TEXT CHECK (primary_goal IS NULL OR primary_goal IN ('understand_audience', 'generate_content_ideas', 'improve_retention', 'win_sponsorships', 'reduce_toxicity', 'report_to_clients', 'other')),
    primary_platform TEXT CHECK (primary_platform IS NULL OR primary_platform IN ('youtube', 'tiktok', 'instagram', 'other')),
    content_topics TEXT,
    audience_size_bucket TEXT CHECK (audience_size_bucket IS NULL OR audience_size_bucket IN ('under_1k', '1k_10k', '10k_50k', '50k_500k', '500k_1m', 'over_1m', 'prefer_not_to_say')),
    weekly_comment_volume_bucket TEXT CHECK (weekly_comment_volume_bucket IS NULL OR weekly_comment_volume_bucket IN ('under_100', '100_1k', '1k_10k', '10k_100k', 'over_100k', 'prefer_not_to_say')),
    success_metric TEXT CHECK (success_metric IS NULL OR success_metric IN ('views', 'watch_time', 'subscribers', 'engagement', 'sentiment', 'sponsor_deals', 'product_sales', 'client_reports', 'other')),
    role TEXT CHECK (role IS NULL OR role IN ('owner', 'strategist', 'analyst', 'moderator', 'viewer', 'other')),
    team_size_bucket TEXT CHECK (team_size_bucket IS NULL OR team_size_bucket IN ('solo', '2_5', '6_20', '21_plus', 'prefer_not_to_say')),
    publishing_frequency_bucket TEXT CHECK (publishing_frequency_bucket IS NULL OR publishing_frequency_bucket IN ('daily', 'several_weekly', 'weekly', 'biweekly', 'monthly', 'irregular', 'prefer_not_to_say')),
    insight_priorities TEXT,
    current_workflow TEXT CHECK (current_workflow IS NULL OR current_workflow IN ('native_tools', 'spreadsheets', 'moderation_tool', 'agency_dashboard', 'none', 'other')),
    biggest_pain TEXT CHECK (biggest_pain IS NULL OR biggest_pain IN ('too_many_comments', 'missed_feedback', 'toxicity', 'reporting_time', 'unclear_content_plan', 'team_visibility', 'other')),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
)`); err != nil {
				return fmt.Errorf("create user_profiles: %w", err)
			}
			hasDisplayName, err := columnExists(ctx, tx, "user_profiles", "display_name")
			if err != nil {
				return err
			}
			if !hasDisplayName {
				if _, err := tx.ExecContext(ctx, `ALTER TABLE user_profiles ADD COLUMN display_name TEXT CHECK (display_name IS NULL OR length(display_name) <= 100)`); err != nil {
					return fmt.Errorf("add display_name to user_profiles: %w", err)
				}
			}
			return nil
		},
	},
	{
		version: 16,
		name:    "user_profiles display_name column",
		run: func(ctx context.Context, tx *sql.Tx) error {
			hasDisplayName, err := columnExists(ctx, tx, "user_profiles", "display_name")
			if err != nil {
				return err
			}
			if !hasDisplayName {
				if _, err := tx.ExecContext(ctx, `ALTER TABLE user_profiles ADD COLUMN display_name TEXT CHECK (display_name IS NULL OR length(display_name) <= 100)`); err != nil {
					return fmt.Errorf("add display_name to user_profiles: %w", err)
				}
			}
			return nil
		},
	},
	{
		version: 17,
		name:    "youtube quota: uploads playlist, adaptive poll, video sync state",
		run: func(ctx context.Context, tx *sql.Tx) error {
			// optimize-youtube-api-quota: per-channel uploads-playlist cache
			// (playlistItems.list = 1 unit vs search.list = 100) and adaptive
			// poll interval (NULL = configured minimum).
			for _, col := range []struct{ name, ddl string }{
				{"yt_uploads_playlist_id", "TEXT"},
				{"poll_interval_minutes", "INTEGER"},
			} {
				has, err := columnExists(ctx, tx, "platform_channels", col.name)
				if err != nil {
					return err
				}
				if has {
					continue
				}
				if _, err := tx.ExecContext(ctx,
					fmt.Sprintf("ALTER TABLE platform_channels ADD COLUMN %s %s", col.name, col.ddl)); err != nil {
					return fmt.Errorf("add platform_channels.%s: %w", col.name, err)
				}
			}
			// Per-video incremental sync state: the newest comment posted_at
			// durably persisted (watermark) plus the commentThreads page token
			// needed to resume an interrupted walk. status='skipped' marks
			// videos whose comments are disabled (permanent, 0 further calls).
			if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS video_sync_state (
    platform_channel_id TEXT NOT NULL REFERENCES platform_channels(id) ON DELETE CASCADE,
    video_id TEXT NOT NULL,
    comment_watermark_at TEXT,
    page_checkpoint TEXT,
    status TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active','skipped')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    PRIMARY KEY (platform_channel_id, video_id)
)`); err != nil {
				return fmt.Errorf("create video_sync_state: %w", err)
			}
			return nil
		},
	},
	{
		version: 18,
		name:    "comments LLM retry state",
		run: func(ctx context.Context, tx *sql.Tx) error {
			for _, col := range []struct{ name, ddl string }{
				{"llm_attempts", "INTEGER NOT NULL DEFAULT 0"},
				{"llm_next_retry_at", "TEXT"},
			} {
				has, err := columnExists(ctx, tx, "comments", col.name)
				if err != nil {
					return err
				}
				if has {
					continue
				}
				if _, err := tx.ExecContext(ctx,
					fmt.Sprintf("ALTER TABLE comments ADD COLUMN %s %s", col.name, col.ddl)); err != nil {
					return fmt.Errorf("add comments.%s: %w", col.name, err)
				}
			}
			return nil
		},
	},
}

// Migrate applies all pending migrations to the given database inside a
// transaction per migration, recording each applied version in
// schema_migrations. The connection pool MUST be limited to a single
// connection (the noFK migrations toggle PRAGMA foreign_keys, which is a
// per-connection setting).
func Migrate(ctx context.Context, database *sql.DB) error {
	if _, err := database.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied := map[int]bool{}
	rows, err := database.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("read schema_migrations: %w", err)
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, m := range migrations {
		if applied[m.version] {
			continue
		}
		if err := runMigration(ctx, database, m); err != nil {
			return fmt.Errorf("migration v%d (%s): %w", m.version, m.name, err)
		}
	}
	return nil
}

func runMigration(ctx context.Context, database *sql.DB, m migration) error {
	// The users rebuild (v3) drops/recreates a parent table while other
	// tables reference it; foreign_keys must be off for the whole
	// migration, per SQLite's documented procedure. The pragma is
	// per-connection and must be set outside the transaction.
	restoreFK := false
	if m.noFK {
		if _, err := database.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
			return fmt.Errorf("disable foreign_keys: %w", err)
		}
		restoreFK = true
		defer func() {
			_, _ = database.ExecContext(ctx, `PRAGMA foreign_keys=ON`)
		}()
	}

	log.Printf("migrating schema to v%d (%s)", m.version, m.name)
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := m.run(ctx, tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, applied_at) VALUES (?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))`,
		m.version); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("record version %d: %w", m.version, err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	if restoreFK {
		// Prove the rebuild left referential integrity intact.
		var violations int
		if err := database.QueryRowContext(ctx, `PRAGMA foreign_key_check`).Scan(&violations); err == nil && violations > 0 {
			return fmt.Errorf("foreign_key_check: %d violations after v%d", violations, m.version)
		}
	}
	return nil
}

// renameColumnIfPresent migrates legacy column names to canonical ones:
// adds the target column when missing, copies values over, then drops the
// legacy column. No-op when the legacy column does not exist.
func renameColumnIfPresent(ctx context.Context, tx *sql.Tx, table, from, to string) error {
	hasFrom, err := columnExists(ctx, tx, table, from)
	if err != nil {
		return err
	}
	if !hasFrom {
		return nil
	}
	hasTo, err := columnExists(ctx, tx, table, to)
	if err != nil {
		return err
	}
	if !hasTo {
		if _, err := tx.ExecContext(ctx,
			fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s TEXT`, table, to)); err != nil {
			return fmt.Errorf("add column %s.%s: %w", table, to, err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf(`UPDATE %s SET %s = %s WHERE %s IS NOT NULL AND %s IS NULL`, table, to, from, from, to)); err != nil {
		return fmt.Errorf("copy %s.%s -> %s: %w", table, from, to, err)
	}
	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf(`ALTER TABLE %s DROP COLUMN %s`, table, from)); err != nil {
		return fmt.Errorf("drop column %s.%s: %w", table, from, err)
	}
	return nil
}

func columnExists(ctx context.Context, tx *sql.Tx, table, column string) (bool, error) {
	var n int
	err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("introspect %s.%s: %w", table, column, err)
	}
	return n > 0, nil
}
