-- Noumena SQLite schema
-- All UUIDs are TEXT (canonical hex). All timestamps are TEXT (ISO 8601).
-- UUIDs are generated in Go via github.com/google/uuid before insert.
-- Schema is idempotent: CREATE TABLE IF NOT EXISTS, safe re-runs.

-- Users and auth
CREATE TABLE IF NOT EXISTS users (
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
        CHECK (subscription_status IN ('active','past_due','paused','canceled')),
    current_period_end TEXT,
    billing_interval TEXT CHECK (billing_interval IS NULL OR billing_interval IN ('month','year')),
    stripe_subscription_id TEXT,
    past_due_since TEXT,
    active_workspace_id TEXT REFERENCES workspaces(id),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

-- Partial unique index: one user per active stripe_subscription_id
CREATE UNIQUE INDEX IF NOT EXISTS users_stripe_subscription_id_uniq
    ON users (stripe_subscription_id)
    WHERE stripe_subscription_id IS NOT NULL;

-- Index for free-tier quota reset cron
CREATE INDEX IF NOT EXISTS idx_users_subplan_quota_reset
    ON users(subscription_plan, quota_reset_at);

-- Index for deletion worker eligibility scan
CREATE INDEX IF NOT EXISTS idx_users_pending_deletion
    ON users(deletion_requested_at)
    WHERE deletion_requested_at IS NOT NULL;

-- Audit table for Stripe webhook idempotency
CREATE TABLE IF NOT EXISTS processed_stripe_events (
    stripe_event_id TEXT PRIMARY KEY,
    event_type TEXT NOT NULL,
    processed_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

-- Append-only overage log for Creator/Studio plans
CREATE TABLE IF NOT EXISTS comment_usage_logs (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    count INT NOT NULL,
    period_start TEXT NOT NULL,
    period_end TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
CREATE INDEX IF NOT EXISTS idx_comment_usage_logs_user_period
    ON comment_usage_logs(user_id, period_start);

-- Email verification tokens
CREATE TABLE IF NOT EXISTS email_verification_tokens (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    used_at TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_email_verification_tokens_token_hash
    ON email_verification_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_email_verification_tokens_user_id
    ON email_verification_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_email_verification_tokens_expires_at
    ON email_verification_tokens(expires_at);

-- Password reset tokens
CREATE TABLE IF NOT EXISTS password_reset_tokens (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    used_at TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_password_reset_tokens_token_hash
    ON password_reset_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_user_id
    ON password_reset_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_expires_at
    ON password_reset_tokens(expires_at);

-- Refresh tokens
CREATE TABLE IF NOT EXISTS refresh_tokens (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_refresh_tokens_token_hash
    ON refresh_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id
    ON refresh_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_expires_at
    ON refresh_tokens(expires_at);

-- Canonical platform channel records
CREATE TABLE IF NOT EXISTS platform_channels (
    id TEXT PRIMARY KEY,
    platform TEXT NOT NULL,
    platform_channel_id TEXT NOT NULL,
    display_name TEXT NOT NULL,
    last_synced_at TEXT,
    deletion_scheduled_at TEXT,
    ingestion_error_count INT NOT NULL DEFAULT 0,
    ingestion_last_error_at TEXT,
    ingestion_backoff_until TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE(platform, platform_channel_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_platform_channels_platform_channel_id
    ON platform_channels (platform, platform_channel_id);

-- Composite index used by the scheduler's "due for sync" query
CREATE INDEX IF NOT EXISTS idx_platform_channels_sync
    ON platform_channels(last_synced_at, ingestion_backoff_until);

-- Per-user tracking relationships
CREATE TABLE IF NOT EXISTS user_channel_tracking (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    platform_channel_id TEXT NOT NULL REFERENCES platform_channels(id),
    access_token_enc TEXT NOT NULL,
    refresh_token_enc TEXT NOT NULL,
    token_expires_at TEXT,
    token_priority INT NOT NULL DEFAULT 0,
    tracking_status TEXT NOT NULL DEFAULT 'active',
    last_seen_at TEXT,
    backfill_days INT NOT NULL DEFAULT 90
        CHECK (backfill_days IN (30, 60, 90, 180)),
    token_expiry_notified_at TEXT,
    custom_prompt TEXT,
    workspace_id TEXT REFERENCES workspaces(id),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE(user_id, platform_channel_id)
);

-- Exactly one active primary tracker per channel
CREATE UNIQUE INDEX IF NOT EXISTS user_channel_tracking_one_primary_per_channel
    ON user_channel_tracking (platform_channel_id)
    WHERE token_priority = 0 AND tracking_status = 'active';

-- Daily expiry-scan helper
CREATE INDEX IF NOT EXISTS idx_user_channel_tracking_token_expiry_scan
    ON user_channel_tracking (token_expires_at)
    WHERE tracking_status IN ('active', 'paused');

CREATE INDEX IF NOT EXISTS idx_user_channel_tracking_workspace
    ON user_channel_tracking(workspace_id);

-- Short-lived OAuth state table for CSRF protection
CREATE TABLE IF NOT EXISTS oauth_states (
    state_hash TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
CREATE INDEX IF NOT EXISTS idx_oauth_states_expires_at
    ON oauth_states (expires_at);

-- Shared comments
CREATE TABLE IF NOT EXISTS comments (
    id TEXT PRIMARY KEY,
    platform_channel_id TEXT NOT NULL REFERENCES platform_channels(id) ON DELETE CASCADE,
    platform_comment_id TEXT NOT NULL,
    parent_comment_id TEXT,
    author_handle TEXT,
    body TEXT NOT NULL,
    posted_at TEXT NOT NULL,
    category TEXT,
    topic TEXT,
    confidence REAL,
    processed_at TEXT,
    llm_provider TEXT,
    llm_model TEXT,
    reply_depth INT NOT NULL DEFAULT 0,
    actionable_insights TEXT NOT NULL DEFAULT '[]',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE(platform_channel_id, platform_comment_id)
);

-- Deduplicate any pre-existing duplicate comment rows (SQLite equivalent)
-- Run once; safe to re-run (no-op if no duplicates)
DELETE FROM comments WHERE rowid NOT IN (
    SELECT MIN(rowid) FROM comments
    GROUP BY platform_channel_id, platform_comment_id
);

-- Named unique constraint (SQLite doesn't support named constraints on UNIQUE
-- inline, so we rely on the UNIQUE(...) clause above + this index for safety)
CREATE UNIQUE INDEX IF NOT EXISTS comments_platform_dedup
    ON comments (platform_channel_id, platform_comment_id);

-- Per-user starred comments
CREATE TABLE IF NOT EXISTS user_comment_stars (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    comment_id TEXT NOT NULL REFERENCES comments(id) ON DELETE CASCADE,
    starred_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    PRIMARY KEY (user_id, comment_id)
);

CREATE INDEX IF NOT EXISTS idx_user_comment_stars_user_starred
    ON user_comment_stars(user_id, starred_at DESC);

-- Ingestion jobs queue
CREATE TABLE IF NOT EXISTS ingestion_jobs (
    id TEXT PRIMARY KEY,
    platform_channel_id TEXT NOT NULL REFERENCES platform_channels(id) ON DELETE CASCADE,
    job_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    retry_count INT NOT NULL DEFAULT 0,
    scheduled_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    started_at TEXT,
    completed_at TEXT,
    error_message TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_ingestion_jobs_pending
    ON ingestion_jobs(status, scheduled_at)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_ingestion_jobs_channel
    ON ingestion_jobs(platform_channel_id, status);

-- Weekly AI-generated digests
CREATE TABLE IF NOT EXISTS weekly_digests (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    week_start_date TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    content TEXT,
    emailed_at TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_weekly_digests_user_week
    ON weekly_digests(user_id, week_start_date);

-- LLM processing jobs
CREATE TABLE IF NOT EXISTS llm_processing_jobs (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL DEFAULT 'pending',
    comment_ids TEXT NOT NULL,
    provider TEXT,
    retry_count INT NOT NULL DEFAULT 0,
    error_message TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    started_at TEXT,
    completed_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_llm_jobs_pending
    ON llm_processing_jobs(status, created_at)
    WHERE status = 'pending';

-- Comment exports
CREATE TABLE IF NOT EXISTS comment_exports (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    format TEXT NOT NULL CHECK (format IN ('csv','json')),
    filters TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('queued','processing','completed','failed','expired')),
    row_count INTEGER,
    file_path TEXT,
    error TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    started_at TEXT,
    completed_at TEXT,
    expires_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_comment_exports_queued
    ON comment_exports(created_at)
    WHERE status = 'queued';

CREATE INDEX IF NOT EXISTS idx_comment_exports_user_created
    ON comment_exports(user_id, created_at DESC);

-- Feed and search indexes
CREATE INDEX IF NOT EXISTS idx_comments_channel_processed
    ON comments(platform_channel_id, processed_at, category);

CREATE INDEX IF NOT EXISTS idx_comments_channel_posted
    ON comments(platform_channel_id, posted_at DESC);

CREATE INDEX IF NOT EXISTS idx_comments_parent_comment_id
    ON comments(parent_comment_id);

CREATE INDEX IF NOT EXISTS idx_comments_channel_parent
    ON comments(platform_channel_id, parent_comment_id);

-- FTS5 virtual table for comment full-text search
CREATE VIRTUAL TABLE IF NOT EXISTS comments_fts USING fts5(body, content='comments', content_rowid='rowid');

-- FTS5 sync triggers
CREATE TRIGGER IF NOT EXISTS comments_fts_insert AFTER INSERT ON comments BEGIN
    INSERT INTO comments_fts(rowid, body) VALUES (new.rowid, new.body);
END;

CREATE TRIGGER IF NOT EXISTS comments_fts_delete AFTER DELETE ON comments BEGIN
    INSERT INTO comments_fts(comments_fts, rowid, body) VALUES('delete', old.rowid, old.body);
END;

CREATE TRIGGER IF NOT EXISTS comments_fts_update AFTER UPDATE ON comments BEGIN
    INSERT INTO comments_fts(comments_fts, rowid, body) VALUES('delete', old.rowid, old.body);
    INSERT INTO comments_fts(rowid, body) VALUES (new.rowid, new.body);
END;

-- Account deletion jobs
CREATE TABLE IF NOT EXISTS account_deletion_jobs (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','exporting','deleting','completed','failed')),
    gdpr_export_generated BOOLEAN NOT NULL DEFAULT 0,
    channels_processed INT NOT NULL DEFAULT 0,
    total_channels INT NOT NULL DEFAULT 0,
    error_message TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    started_at TEXT,
    completed_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_account_deletion_jobs_status_created
    ON account_deletion_jobs(status, created_at);

-- Data export jobs
CREATE TABLE IF NOT EXISTS data_export_jobs (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued','exporting','completed','failed')),
    export_url TEXT,
    expires_at TEXT,
    error_message TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    started_at TEXT,
    completed_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_data_export_jobs_status_created
    ON data_export_jobs(status, created_at);

CREATE UNIQUE INDEX IF NOT EXISTS idx_data_export_jobs_one_inflight_per_user
    ON data_export_jobs(user_id)
    WHERE status IN ('queued','exporting');

-- Email send log
CREATE TABLE IF NOT EXISTS email_send_log (
    id TEXT PRIMARY KEY,
    user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    template TEXT NOT NULL,
    idempotency_key TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','sent','failed')),
    attempt_count INT NOT NULL DEFAULT 0,
    last_attempted_at TEXT,
    last_error TEXT,
    sent_at TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_email_send_log_user_template
    ON email_send_log(user_id, template);

CREATE INDEX IF NOT EXISTS idx_email_send_log_status_attempted
    ON email_send_log(status, last_attempted_at);

-- Rate limit buckets
CREATE TABLE IF NOT EXISTS rate_limit_buckets (
    id TEXT PRIMARY KEY,
    user_key TEXT NOT NULL,
    window_start TEXT NOT NULL,
    count INT NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE(user_key, window_start)
);

CREATE INDEX IF NOT EXISTS idx_rate_limit_buckets_window_start
    ON rate_limit_buckets(window_start);

-- Workspaces
CREATE TABLE IF NOT EXISTS workspaces (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

-- Workspace members
CREATE TABLE IF NOT EXISTS workspace_members (
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'viewer'
        CHECK (role IN ('admin', 'analyst', 'viewer')),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    PRIMARY KEY (workspace_id, user_id)
);

-- Workspace invites
CREATE TABLE IF NOT EXISTS workspace_invites (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'viewer'
        CHECK (role IN ('admin', 'analyst', 'viewer')),
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TEXT NOT NULL,
    consumed_at TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_workspace_invites_workspace_email
    ON workspace_invites(workspace_id, email);

-- Audit log
CREATE TABLE IF NOT EXISTS audit_log (
    id TEXT PRIMARY KEY,
    user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    details TEXT,
    ip_address TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_audit_log_user_created
    ON audit_log(user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_log_action_created
    ON audit_log(action, created_at DESC);
