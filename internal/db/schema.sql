-- Users and auth
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT,
    google_sub TEXT UNIQUE,
    email_verified_at TIMESTAMPTZ,
    subscription_plan TEXT NOT NULL DEFAULT 'free',
    stripe_customer_id TEXT,
    comment_quota_monthly INT NOT NULL DEFAULT 1000,
    comments_used_this_period INT NOT NULL DEFAULT 0,
    quota_reset_at TIMESTAMPTZ,
    token_version INT NOT NULL DEFAULT 0,
    failed_login_attempts INT NOT NULL DEFAULT 0,
    locked_until TIMESTAMPTZ,
    deletion_requested_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Idempotent additions for previously-existing users tables
ALTER TABLE users ADD COLUMN IF NOT EXISTS token_version INT NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN IF NOT EXISTS failed_login_attempts INT NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN IF NOT EXISTS locked_until TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN IF NOT EXISTS deletion_requested_at TIMESTAMPTZ;

-- ============================================================================
-- billing change: subscription state machine, stripe webhook idempotency,
-- and per-period overage logging.
-- ============================================================================

-- Idempotent column additions on users for Stripe-backed billing state.
ALTER TABLE users ADD COLUMN IF NOT EXISTS trial_ends_at TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN IF NOT EXISTS subscription_status TEXT NOT NULL DEFAULT 'active';
ALTER TABLE users ADD COLUMN IF NOT EXISTS current_period_end TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN IF NOT EXISTS billing_interval TEXT;
-- backend-schema-and-env-fixes change: persist the Stripe subscription id so
-- DeleteAccount can cancel the subscription synchronously instead of relying
-- on Stripe's eventual cancel-on-customer-delete. NULL for free-tier and
-- never-subscribed users; populated from checkout.session.completed,
-- customer.subscription.created, and customer.subscription.updated; cleared
-- on customer.subscription.deleted.
ALTER TABLE users ADD COLUMN IF NOT EXISTS stripe_subscription_id TEXT;
-- Partial unique index: enforce one user per active stripe_subscription_id
-- while still allowing multiple NULL rows (free-tier users). Using a partial
-- index instead of a column-level UNIQUE so the NULLs don't collide.
CREATE UNIQUE INDEX IF NOT EXISTS users_stripe_subscription_id_uniq
    ON users (stripe_subscription_id)
    WHERE stripe_subscription_id IS NOT NULL;
-- Track first failure time for the 3-day grace cron. Derived from the most
-- recent invoice.payment_failed event; resets to NULL on payment_succeeded.
ALTER TABLE users ADD COLUMN IF NOT EXISTS past_due_since TIMESTAMPTZ;

-- Constrain subscription_status to known states. DROP-then-ADD pattern for
-- idempotency; we tolerate the constraint missing on first run.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_subscription_status_check;
ALTER TABLE users ADD CONSTRAINT users_subscription_status_check
    CHECK (subscription_status IN ('active','past_due','paused','canceled'));

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_billing_interval_check;
ALTER TABLE users ADD CONSTRAINT users_billing_interval_check
    CHECK (billing_interval IS NULL OR billing_interval IN ('month','year'));

-- Backfill: existing rows already pick up subscription_status='active' from
-- the column default, but be explicit for clarity.
UPDATE users SET subscription_status = 'active'
 WHERE subscription_status IS NULL OR subscription_status = '';

-- Audit table for Stripe webhook idempotency. PRIMARY KEY on the Stripe
-- event id is the dedup mechanism.
CREATE TABLE IF NOT EXISTS processed_stripe_events (
    stripe_event_id TEXT PRIMARY KEY,
    event_type TEXT NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Append-only overage log for Creator/Studio plans. Rolled up on read by
-- GET /api/billing/status.
CREATE TABLE IF NOT EXISTS comment_usage_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    count INT NOT NULL,
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_comment_usage_logs_user_period
    ON comment_usage_logs(user_id, period_start);

-- Daily free-tier reset cron: WHERE subscription_plan='free' AND
-- quota_reset_at <= now(). The composite index makes the scan cheap.
CREATE INDEX IF NOT EXISTS idx_users_subplan_quota_reset
    ON users(subscription_plan, quota_reset_at);

-- Email verification tokens
CREATE TABLE IF NOT EXISTS email_verification_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_email_verification_tokens_token_hash ON email_verification_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_email_verification_tokens_user_id ON email_verification_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_email_verification_tokens_expires_at ON email_verification_tokens(expires_at);

-- Password reset tokens
CREATE TABLE IF NOT EXISTS password_reset_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_password_reset_tokens_token_hash ON password_reset_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_user_id ON password_reset_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_expires_at ON password_reset_tokens(expires_at);

-- Refresh tokens
CREATE TABLE IF NOT EXISTS refresh_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_refresh_tokens_token_hash ON refresh_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id ON refresh_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_expires_at ON refresh_tokens(expires_at);

-- Canonical platform channel records
CREATE TABLE IF NOT EXISTS platform_channels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    platform TEXT NOT NULL,
    platform_channel_id TEXT NOT NULL,
    display_name TEXT NOT NULL,
    last_synced_at TIMESTAMPTZ,
    deletion_scheduled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(platform, platform_channel_id)
);

-- Per-user tracking relationships
CREATE TABLE IF NOT EXISTS user_channel_tracking (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    platform_channel_id UUID NOT NULL REFERENCES platform_channels(id),
    access_token_enc TEXT NOT NULL,
    refresh_token_enc TEXT NOT NULL,
    token_expires_at TIMESTAMPTZ,
    token_priority INT NOT NULL DEFAULT 0,
    tracking_status TEXT NOT NULL DEFAULT 'active',
    last_seen_at TIMESTAMPTZ,
    backfill_days INT NOT NULL DEFAULT 90,
    token_expiry_notified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, platform_channel_id)
);

-- Idempotent additions for previously-existing user_channel_tracking tables
ALTER TABLE user_channel_tracking ADD COLUMN IF NOT EXISTS backfill_days INT NOT NULL DEFAULT 90;
ALTER TABLE user_channel_tracking ADD COLUMN IF NOT EXISTS token_expiry_notified_at TIMESTAMPTZ;

-- Enforce allowed backfill_days values; the IF NOT EXISTS guard is emulated
-- by dropping any prior incarnation of the constraint before recreating it.
ALTER TABLE user_channel_tracking DROP CONSTRAINT IF EXISTS user_channel_tracking_backfill_days_check;
ALTER TABLE user_channel_tracking ADD CONSTRAINT user_channel_tracking_backfill_days_check
    CHECK (backfill_days IN (30, 60, 90, 180));

-- Ensure the (platform, platform_channel_id) uniqueness exists for older tables
-- where the inline UNIQUE() may not have been honoured.
CREATE UNIQUE INDEX IF NOT EXISTS idx_platform_channels_platform_channel_id
    ON platform_channels (platform, platform_channel_id);

-- Exactly one active primary tracker per channel.
CREATE UNIQUE INDEX IF NOT EXISTS user_channel_tracking_one_primary_per_channel
    ON user_channel_tracking (platform_channel_id)
    WHERE token_priority = 0 AND tracking_status = 'active';

-- Daily expiry-scan helper.
CREATE INDEX IF NOT EXISTS idx_user_channel_tracking_token_expiry_scan
    ON user_channel_tracking (token_expires_at)
    WHERE tracking_status IN ('active', 'paused');

-- Short-lived OAuth state table for CSRF protection.
CREATE TABLE IF NOT EXISTS oauth_states (
    state_hash TEXT PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_oauth_states_expires_at ON oauth_states (expires_at);

-- Shared comments
CREATE TABLE IF NOT EXISTS comments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    platform_channel_id UUID NOT NULL REFERENCES platform_channels(id) ON DELETE CASCADE,
    platform_comment_id TEXT NOT NULL,
    parent_comment_id TEXT,
    author_handle TEXT,
    body TEXT NOT NULL,
    posted_at TIMESTAMPTZ NOT NULL,
    category TEXT,
    sentiment_score NUMERIC(4,2),
    topic TEXT,
    confidence NUMERIC(4,2),
    processed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(platform_channel_id, platform_comment_id)
);

-- Per-user starred comments
CREATE TABLE IF NOT EXISTS user_comment_stars (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    comment_id UUID NOT NULL REFERENCES comments(id) ON DELETE CASCADE,
    starred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, comment_id)
);

-- ============================================================================
-- comment-ingestion: ingestion-pipeline state on platform_channels
-- ============================================================================

-- Idempotent column additions; safe to run on existing databases.
ALTER TABLE platform_channels ADD COLUMN IF NOT EXISTS ingestion_lock_id BIGINT;
ALTER TABLE platform_channels ADD COLUMN IF NOT EXISTS ingestion_error_count INT NOT NULL DEFAULT 0;
ALTER TABLE platform_channels ADD COLUMN IF NOT EXISTS ingestion_last_error_at TIMESTAMPTZ;
ALTER TABLE platform_channels ADD COLUMN IF NOT EXISTS ingestion_backoff_until TIMESTAMPTZ;

-- Backfill ingestion_lock_id deterministically for any rows that lack one.
-- ('x' || substr(md5(id::text),1,16))::bit(64)::bigint hashes the UUID into a
-- stable BIGINT used as the pg_try_advisory_lock key.
UPDATE platform_channels
   SET ingestion_lock_id = ('x' || substr(md5(id::text), 1, 16))::bit(64)::bigint
 WHERE ingestion_lock_id IS NULL;

-- Composite index used by the scheduler's "due for sync" query.
CREATE INDEX IF NOT EXISTS idx_platform_channels_sync
    ON platform_channels(last_synced_at, ingestion_backoff_until);

-- Deduplicate any pre-existing duplicate comment rows BEFORE adding the
-- unique constraint. Keeps one row per (platform_channel_id, platform_comment_id).
DELETE FROM comments a USING comments b
 WHERE a.ctid < b.ctid
   AND a.platform_channel_id = b.platform_channel_id
   AND a.platform_comment_id = b.platform_comment_id;

-- Add a named unique constraint. The inline UNIQUE(...) on the original
-- CREATE TABLE produces an autogenerated constraint name; we add a named
-- one explicitly so it appears in `\d` as comments_platform_dedup. Idempotent
-- via DO block so reruns don't error.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'comments_platform_dedup'
    ) THEN
        BEGIN
            ALTER TABLE comments
                ADD CONSTRAINT comments_platform_dedup
                UNIQUE (platform_channel_id, platform_comment_id);
        EXCEPTION WHEN duplicate_table OR duplicate_object THEN
            -- Some older databases already have an unnamed unique constraint
            -- from the inline UNIQUE(...) clause on CREATE TABLE; that is
            -- functionally equivalent so we ignore the duplicate.
            NULL;
        END;
    END IF;
END$$;

-- ============================================================================
-- comment-ingestion: ingestion_jobs queue
-- ============================================================================

CREATE TABLE IF NOT EXISTS ingestion_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    platform_channel_id UUID NOT NULL REFERENCES platform_channels(id) ON DELETE CASCADE,
    job_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    retry_count INT NOT NULL DEFAULT 0,
    scheduled_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ingestion_jobs_pending
    ON ingestion_jobs(status, scheduled_at)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_ingestion_jobs_channel
    ON ingestion_jobs(platform_channel_id, status);

-- Weekly AI-generated digests
CREATE TABLE IF NOT EXISTS weekly_digests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    week_start_date DATE NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    content JSONB,
    emailed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- weekly-digest change: per-user opt-in flag + last-email timestamp on users.
ALTER TABLE users ADD COLUMN IF NOT EXISTS digest_email_opt_in BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE users ADD COLUMN IF NOT EXISTS digest_email_sent_at TIMESTAMPTZ;

-- weekly-digest change: enforce one digest per (user, week) — also serves
-- the ON CONFLICT idempotency path used by InsertWeeklyDigestPending.
CREATE UNIQUE INDEX IF NOT EXISTS idx_weekly_digests_user_week
    ON weekly_digests(user_id, week_start_date);

-- Status enum is loose TEXT; tolerated values are
-- 'pending' | 'completed' | 'skipped_insufficient_data' | 'deferred'.
-- (Older databases may still carry rows with status='generated' from the
-- pre-weekly-digest worker; readers MUST treat unknown statuses as opaque.)

-- ============================================================================
-- llm-processing: provider/model attribution columns + processing-job lifecycle
-- ============================================================================

-- Additive, nullable columns on comments. No backfill: rows classified before
-- this column existed simply have NULL provider/model.
ALTER TABLE comments ADD COLUMN IF NOT EXISTS llm_provider TEXT;
ALTER TABLE comments ADD COLUMN IF NOT EXISTS llm_model TEXT;

-- Per-batch lifecycle table (observability only; not the work-claim queue).
CREATE TABLE IF NOT EXISTS llm_processing_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    status TEXT NOT NULL DEFAULT 'pending',
    comment_ids UUID[] NOT NULL,
    provider TEXT,
    retry_count INT NOT NULL DEFAULT 0,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);

-- Partial index supports observability dashboards / pending-job lookups.
CREATE INDEX IF NOT EXISTS idx_llm_jobs_pending
    ON llm_processing_jobs(status, created_at)
    WHERE status = 'pending';

-- ============================================================================
-- insights-api: comment_exports table + composite indexes for read paths
-- ============================================================================
-- Tasks 1.1–1.6. Note: CONCURRENTLY is NOT used here because the embedded
-- migration runs inside an implicit transaction via Pool.Exec; CONCURRENTLY is
-- a "deploy-time" optimization that the spec calls for, but the IF NOT EXISTS
-- form is safe and idempotent on every startup. Operators running the
-- migration manually against a hot production database should instead apply
-- the equivalent statements outside a transaction with CONCURRENTLY.

CREATE TABLE IF NOT EXISTS comment_exports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    format TEXT NOT NULL CHECK (format IN ('csv','json')),
    filters JSONB NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('queued','processing','completed','failed','expired')),
    row_count INTEGER,
    file_path TEXT,
    error TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ
);

-- Worker claim path: small partial index over queued rows only.
CREATE INDEX IF NOT EXISTS idx_comment_exports_queued
    ON comment_exports(created_at)
    WHERE status = 'queued';

-- Status-poll lookup: most-recent-first per user.
CREATE INDEX IF NOT EXISTS idx_comment_exports_user_created
    ON comment_exports(user_id, created_at DESC);

-- Feed default + category filter + access-controlled scope on comments.
CREATE INDEX IF NOT EXISTS idx_comments_channel_processed
    ON comments(platform_channel_id, processed_at, category);

-- newest sort + date_from/date_to filters.
CREATE INDEX IF NOT EXISTS idx_comments_channel_posted
    ON comments(platform_channel_id, posted_at DESC);

-- /saved sort by starred_at DESC for a user.
CREATE INDEX IF NOT EXISTS idx_user_comment_stars_user_starred
    ON user_comment_stars(user_id, starred_at DESC);

-- Down-migration (applied manually if rollback is needed):
--   DROP INDEX IF EXISTS idx_user_comment_stars_user_starred;
--   DROP INDEX IF EXISTS idx_comments_channel_posted;
--   DROP INDEX IF EXISTS idx_comments_channel_processed;
--   DROP INDEX IF EXISTS idx_comment_exports_user_created;
--   DROP INDEX IF EXISTS idx_comment_exports_queued;
--   DROP TABLE  IF EXISTS comment_exports;

-- ============================================================================
-- data-lifecycle: account-deletion + GDPR-export jobs + transactional email log
-- ============================================================================
-- Tasks 1.1–1.10. All statements are idempotent; safe on every startup. The
-- new `users` columns (deletion_requested_at already exists from core-auth)
-- complete the lifecycle state needed by the deletion + export workers.

-- 1.1 — additive lifecycle/export columns on users.
ALTER TABLE users ADD COLUMN IF NOT EXISTS deletion_completed_at TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN IF NOT EXISTS data_export_url        TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS data_export_expires_at TIMESTAMPTZ;

-- 1.2 — partial index drives the deletion-worker eligibility scan; only rows
-- with a non-NULL deletion_requested_at are interesting, so a partial index
-- keeps the structure tiny on a database where deletion is the rare case.
CREATE INDEX IF NOT EXISTS idx_users_pending_deletion
    ON users(deletion_requested_at)
    WHERE deletion_requested_at IS NOT NULL;

-- 1.3 — account_deletion_jobs: one row per scheduled deletion. The 48-hour
-- grace window is enforced by the worker's claim query, not at the schema
-- level; this row exists from the moment the user submits DELETE
-- /api/user/account.
CREATE TABLE IF NOT EXISTS account_deletion_jobs (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id               UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status                TEXT NOT NULL DEFAULT 'pending',
    gdpr_export_generated BOOLEAN NOT NULL DEFAULT false,
    channels_processed    INT NOT NULL DEFAULT 0,
    total_channels        INT NOT NULL DEFAULT 0,
    error_message         TEXT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at            TIMESTAMPTZ,
    completed_at          TIMESTAMPTZ
);

-- DROP-then-ADD pattern keeps the CHECK constraint idempotent across reruns.
ALTER TABLE account_deletion_jobs DROP CONSTRAINT IF EXISTS account_deletion_jobs_status_check;
ALTER TABLE account_deletion_jobs ADD CONSTRAINT account_deletion_jobs_status_check
    CHECK (status IN ('pending','exporting','deleting','completed','failed'));

-- 1.4 — worker claim path: status + age ordering.
CREATE INDEX IF NOT EXISTS idx_account_deletion_jobs_status_created
    ON account_deletion_jobs(status, created_at);

-- 1.5 — data_export_jobs: GDPR export pipeline. Separate from the deletion
-- job so users can request an export without scheduling deletion.
CREATE TABLE IF NOT EXISTS data_export_jobs (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status        TEXT NOT NULL DEFAULT 'queued',
    export_url    TEXT,
    expires_at    TIMESTAMPTZ,
    error_message TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at    TIMESTAMPTZ,
    completed_at  TIMESTAMPTZ
);

ALTER TABLE data_export_jobs DROP CONSTRAINT IF EXISTS data_export_jobs_status_check;
ALTER TABLE data_export_jobs ADD CONSTRAINT data_export_jobs_status_check
    CHECK (status IN ('queued','exporting','completed','failed'));

-- 1.6 — worker claim path.
CREATE INDEX IF NOT EXISTS idx_data_export_jobs_status_created
    ON data_export_jobs(status, created_at);

-- 1.7 — at most one in-flight export per user (queued OR exporting). The
-- partial unique index is the durable enforcement; the API short-circuits
-- to the existing job when this constraint would fire, so callers see
-- HTTP 202 with the existing job_id rather than an error.
CREATE UNIQUE INDEX IF NOT EXISTS idx_data_export_jobs_one_inflight_per_user
    ON data_export_jobs(user_id)
    WHERE status IN ('queued','exporting');

-- 1.8 — email_send_log: durable idempotency + retry state for every
-- transactional email. ON DELETE SET NULL on user_id retains the audit
-- trail after a hard-delete (Spec: deletion of users does not cascade-
-- destroy email history; the row is anonymised instead).
CREATE TABLE IF NOT EXISTS email_send_log (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            UUID REFERENCES users(id) ON DELETE SET NULL,
    template           TEXT NOT NULL,
    idempotency_key    TEXT NOT NULL UNIQUE,
    status             TEXT NOT NULL DEFAULT 'pending',
    attempt_count      INT  NOT NULL DEFAULT 0,
    last_attempted_at  TIMESTAMPTZ,
    last_error         TEXT,
    sent_at            TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE email_send_log DROP CONSTRAINT IF EXISTS email_send_log_status_check;
ALTER TABLE email_send_log ADD CONSTRAINT email_send_log_status_check
    CHECK (status IN ('pending','sent','failed'));

-- 1.9 — per-user, per-template lookup. Used by ops dashboards and by the
-- "did this user receive X email yet?" diagnostic path.
CREATE INDEX IF NOT EXISTS idx_email_send_log_user_template
    ON email_send_log(user_id, template);

-- 1.10 — retry-worker claim path: status + age ordering on last_attempted_at.
CREATE INDEX IF NOT EXISTS idx_email_send_log_status_attempted
    ON email_send_log(status, last_attempted_at);
