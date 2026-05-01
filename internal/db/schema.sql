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
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

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
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, platform_channel_id)
);

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
