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
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

type PlatformChannel struct {
	ID                  string     `json:"id"`
	Platform            string     `json:"platform"`
	PlatformChannelID   string     `json:"platform_channel_id"`
	DisplayName         string     `json:"display_name"`
	LastSyncedAt        *time.Time `json:"last_synced_at,omitempty"`
	DeletionScheduledAt *time.Time `json:"deletion_scheduled_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
}

type UserChannelTracking struct {
	ID                string     `json:"id"`
	UserID            string     `json:"user_id"`
	PlatformChannelID string     `json:"platform_channel_id"`
	AccessTokenEnc    string     `json:"-"`
	RefreshTokenEnc   string     `json:"-"`
	TokenExpiresAt    *time.Time `json:"token_expires_at,omitempty"`
	TokenPriority     int        `json:"token_priority"`
	TrackingStatus    string     `json:"tracking_status"`
	LastSeenAt        *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

type Comment struct {
	ID                string     `json:"id"`
	PlatformChannelID string     `json:"platform_channel_id"`
	PlatformCommentID string     `json:"platform_comment_id"`
	ParentCommentID   *string    `json:"parent_comment_id,omitempty"`
	AuthorHandle      *string    `json:"author_handle,omitempty"`
	Body              string     `json:"body"`
	PostedAt          time.Time  `json:"posted_at"`
	Category          *string    `json:"category,omitempty"`
	SentimentScore    *float64   `json:"sentiment_score,omitempty"`
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
