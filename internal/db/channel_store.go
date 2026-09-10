package db

import (
	"context"
	"time"
)

// ChannelStore is the narrow data-access surface for channel management,
// tracker tokens and OAuth state.
type ChannelStore interface {
	GetUserByID(ctx context.Context, id string) (*User, error)

	ListChannelsForUser(ctx context.Context, userID string) ([]*PlatformChannel, error)
	ConnectChannelTx(ctx context.Context, userID, platform, platformChannelID, displayName, accessTokenEnc, refreshTokenEnc string, tokenExpiresAt *time.Time) error
	PauseChannelTracking(ctx context.Context, userID, platformChannelID string) error
	RemoveChannelTracking(ctx context.Context, userID, platformChannelID string) error
	GetChannelsForSync(ctx context.Context, minutes int) ([]*PlatformChannel, error)
	UpdateSyncTime(ctx context.Context, id string) error

	GetIngestionToken(ctx context.Context, platformChannelID string) (string, string, error)
	PromoteNextIngestionToken(ctx context.Context, platformChannelID string) error

	InsertOAuthState(ctx context.Context, stateHash, userID string, expiresAt time.Time) error
	ConsumeOAuthState(ctx context.Context, stateHash, userID string) (bool, error)
	CleanupExpiredOAuthStates(ctx context.Context) (int64, error)

	UpsertPlatformChannel(ctx context.Context, platform, platformChannelID, displayName string) (channelID string, inserted bool, err error)
	GetChannelByPlatformID(ctx context.Context, platform, platformChannelID string) (*PlatformChannel, error)
	ScheduleChannelDeletion(ctx context.Context, platformChannelID string, graceHours int) error
	ClearChannelDeletion(ctx context.Context, platformChannelID string) error
	GetActiveTrackerCount(ctx context.Context, platformChannelID string) (int, error)
	PurgeChannelData(ctx context.Context, platformChannelID string) error

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
}

var _ ChannelStore = (DBQuery)(nil)
