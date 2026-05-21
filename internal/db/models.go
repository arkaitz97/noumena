package db

import (
	"time"
)

type User struct {
	ID                     string     `json:"id"`
	Email                  string     `json:"email"`
	PasswordHash           *string    `json:"-"`
	GoogleSub              *string    `json:"google_sub,omitempty"`
	EmailVerifiedAt        *time.Time `json:"email_verified_at,omitempty"`
	SubscriptionPlan       string     `json:"subscription_plan"`
	StripeCustomerID       *string    `json:"stripe_customer_id,omitempty"`
	CommentQuotaMonthly    int        `json:"comment_quota_monthly"`
	CommentsUsedThisPeriod int        `json:"comments_used_this_period"`
	QuotaResetAt           *time.Time `json:"quota_reset_at,omitempty"`
	TokenVersion           int        `json:"token_version"`
	FailedLoginAttempts    int        `json:"-"`
	LockedUntil            *time.Time `json:"-"`
	DeletionRequestedAt    *time.Time `json:"-"`
	DeletionCompletedAt    *time.Time `json:"-"`
	DataExportURL          *string    `json:"-"`
	DataExportExpiresAt    *time.Time `json:"-"`
	DigestEmailOptIn       bool       `json:"digest_email_opt_in"`
	DigestEmailSentAt      *time.Time `json:"digest_email_sent_at,omitempty"`
	// billing change: subscription state machine fields.
	TrialEndsAt        *time.Time `json:"trial_ends_at,omitempty"`
	SubscriptionStatus string     `json:"subscription_status"`
	CurrentPeriodEnd   *time.Time `json:"current_period_end,omitempty"`
	BillingInterval    *string    `json:"billing_interval,omitempty"`
	// StripeSubscriptionID is the Stripe subscription id for the user's
	// currently active or most-recently-active subscription. NULL for
	// free-tier users; populated by Stripe webhook handlers and cleared on
	// customer.subscription.deleted. Used by DeleteAccount to cancel the
	// subscription synchronously when the user requests deletion.
	StripeSubscriptionID *string    `json:"stripe_subscription_id,omitempty"`
	PastDueSince         *time.Time `json:"-"`
	ActiveWorkspaceID    *string    `json:"active_workspace_id,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type PlatformChannel struct {
	ID                  string     `json:"id"`
	Platform            string     `json:"platform"`
	PlatformChannelID   string     `json:"platform_channel_id"`
	DisplayName         string     `json:"display_name"`
	LastSyncedAt        *time.Time `json:"last_synced_at,omitempty"`
	DeletionScheduledAt *time.Time `json:"deletion_scheduled_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`

	// Ingestion-pipeline state (comment-ingestion change). Optional: zero
	// values are equivalent to "no ingestion state recorded".
	IngestionErrorCount   int        `json:"ingestion_error_count,omitempty"`
	IngestionLastErrorAt  *time.Time `json:"ingestion_last_error_at,omitempty"`
	IngestionBackoffUntil *time.Time `json:"ingestion_backoff_until,omitempty"`
}

// IngestionJob is a row in the ingestion_jobs queue.
type IngestionJob struct {
	ID                string     `json:"id"`
	PlatformChannelID string     `json:"platform_channel_id"`
	JobType           string     `json:"job_type"`
	Status            string     `json:"status"`
	RetryCount        int        `json:"retry_count"`
	ScheduledAt       time.Time  `json:"scheduled_at"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	ErrorMessage      *string    `json:"error_message,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

// SyncStatus is the response payload for GET /api/channels/{id}/sync-status.
type SyncStatus struct {
	LastSyncedAt            *time.Time `json:"last_synced_at"`
	NextSyncAt              *time.Time `json:"next_sync_at"`
	BackfillProgressPercent *float64   `json:"backfill_progress_percent"`
	RecentError             *SyncError `json:"recent_error"`
}

// SyncError carries the latest ingestion error metadata for a channel.
type SyncError struct {
	At      time.Time `json:"at"`
	Message string    `json:"message"`
}

type UserChannelTracking struct {
	ID                    string     `json:"id"`
	UserID                string     `json:"user_id"`
	PlatformChannelID     string     `json:"platform_channel_id"`
	AccessTokenEnc        string     `json:"-"`
	RefreshTokenEnc       string     `json:"-"`
	TokenExpiresAt        *time.Time `json:"token_expires_at,omitempty"`
	TokenPriority         int        `json:"token_priority"`
	TrackingStatus        string     `json:"tracking_status"`
	LastSeenAt            *time.Time `json:"last_seen_at,omitempty"`
	BackfillDays          int        `json:"backfill_days"`
	TokenExpiryNotifiedAt *time.Time `json:"token_expiry_notified_at,omitempty"`
	CustomPrompt          *string    `json:"custom_prompt,omitempty"`
	WorkspaceID           *string    `json:"workspace_id,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
}

// ChannelListItem is the joined view returned by `GET /api/channels`.
type ChannelListItem struct {
	ID                    string     `json:"id"`
	Platform              string     `json:"platform"`
	PlatformChannelID     string     `json:"platform_channel_id"`
	DisplayName           string     `json:"display_name"`
	TrackingStatus        string     `json:"tracking_status"`
	TokenPriority         int        `json:"token_priority"`
	LastSyncedAt          *time.Time `json:"last_synced_at,omitempty"`
	BackfillDays          int        `json:"backfill_days"`
	TokenExpiresAt        *time.Time `json:"token_expires_at,omitempty"`
	TokenExpiryNotifiedAt *time.Time `json:"token_expiry_notified_at,omitempty"`
}

type Comment struct {
	ID                string     `json:"id"`
	PlatformChannelID string     `json:"platform_channel_id"`
	PlatformCommentID string     `json:"platform_comment_id"`
	ParentCommentID   *string    `json:"parent_comment_id,omitempty"`
	ReplyDepth        int        `json:"reply_depth"`
	AuthorHandle      *string    `json:"author_handle,omitempty"`
	Body              string     `json:"body"`
	PostedAt          time.Time  `json:"posted_at"`
	Category          *string    `json:"category,omitempty"`
	ActionableInsights []byte    `json:"actionable_insights,omitempty"`
	Topic             *string    `json:"topic,omitempty"`
	Confidence        *float64   `json:"confidence,omitempty"`
	ProcessedAt       *time.Time `json:"processed_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

type UserCommentStar struct {
	UserID    string    `json:"user_id"`
	CommentID string    `json:"comment_id"`
	StarredAt time.Time `json:"starred_at"`
}

type WeeklyDigest struct {
	ID            string     `json:"id"`
	UserID        string     `json:"user_id"`
	WeekStartDate string     `json:"week_start_date"` // Using string for DATE type, or time.Time
	Status        string     `json:"status"`
	Content       string     `json:"content"` // JSONB will be stored/marshaled to string/[]byte
	EmailedAt     *time.Time `json:"emailed_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}
