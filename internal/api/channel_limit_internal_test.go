package api

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"noumena/internal/billing"
	"noumena/internal/db"
)

// Internal tests for the channel-limit guard helper used by the
// ExchangeYouTubeCode handler (billing change, task 10.5.1).
//
// We verify the guard at the helper level rather than driving the full
// OAuth flow: the handler delegates to checkChannelLimit, so testing the
// helper is the highest-leverage way to pin the behavior described by the
// "Channel Connection Enforces Plan Channel Limit" requirement.

// guardMockQuery is a minimal db.DBQuery double for the guard tests.
// Every method that is not used by the helper panics so accidental future
// dependencies surface immediately.
type guardMockQuery struct {
	user        *db.User
	getUserErr  error
	channelCnt  int
	getCntErr   error
}

func (g *guardMockQuery) GetUserByID(ctx context.Context, id string) (*db.User, error) {
	return g.user, g.getUserErr
}

func (g *guardMockQuery) GetActiveChannelCount(ctx context.Context, userID string) (int, error) {
	return g.channelCnt, g.getCntErr
}

// ---- everything below: panic stubs for the rest of the DBQuery surface ----

func (g *guardMockQuery) CreateUser(context.Context, string, string) (*db.User, error) {
	panic("not implemented")
}
func (g *guardMockQuery) GetUserByEmail(context.Context, string) (*db.User, error) {
	panic("not implemented")
}
func (g *guardMockQuery) UpdateUserSubscription(context.Context, string, string, string, int) error {
	panic("not implemented")
}
func (g *guardMockQuery) ListChannelsForUser(context.Context, string) ([]*db.PlatformChannel, error) {
	panic("not implemented")
}
func (g *guardMockQuery) ConnectChannelTx(context.Context, string, string, string, string, string, string, *time.Time) error {
	panic("not implemented")
}
func (g *guardMockQuery) PauseChannelTracking(context.Context, string, string) error {
	panic("not implemented")
}
func (g *guardMockQuery) RemoveChannelTracking(context.Context, string, string) error {
	panic("not implemented")
}
func (g *guardMockQuery) GetChannelsForSync(context.Context, int) ([]*db.PlatformChannel, error) {
	panic("not implemented")
}
func (g *guardMockQuery) UpdateSyncTime(context.Context, string) error { panic("not implemented") }
func (g *guardMockQuery) GetCommentFeed(context.Context, string, int, int) ([]*db.Comment, error) {
	panic("not implemented")
}
func (g *guardMockQuery) StarComment(context.Context, string, string) error {
	panic("not implemented")
}
func (g *guardMockQuery) GetIngestionToken(context.Context, string) (string, string, error) {
	panic("not implemented")
}
func (g *guardMockQuery) PromoteNextIngestionToken(context.Context, string) error {
	panic("not implemented")
}
func (g *guardMockQuery) InsertComment(context.Context, *db.Comment) error { panic("not implemented") }
func (g *guardMockQuery) GetUnprocessedComments(context.Context, int) ([]*db.Comment, error) {
	panic("not implemented")
}
func (g *guardMockQuery) GetUnprocessedCommentsByChannel(context.Context, int) (map[string][]*db.Comment, error) {
	panic("not implemented")
}
func (g *guardMockQuery) UpdateCommentLLMData(context.Context, string, string, string, string, string, []byte, float64) error {
	panic("not implemented")
}
func (g *guardMockQuery) IncrementCommentsUsed(context.Context, string, int) error {
	panic("not implemented")
}
func (g *guardMockQuery) GetPrimaryTracker(context.Context, string) (*db.User, error) {
	panic("not implemented")
}
func (g *guardMockQuery) EnqueueLLMJob(context.Context, []string, string) (string, error) {
	panic("not implemented")
}
func (g *guardMockQuery) MarkLLMJobProcessing(context.Context, string) error {
	panic("not implemented")
}
func (g *guardMockQuery) MarkLLMJobCompleted(context.Context, string) error {
	panic("not implemented")
}
func (g *guardMockQuery) MarkLLMJobFailed(context.Context, string, string) error {
	panic("not implemented")
}
func (g *guardMockQuery) InsertWeeklyDigest(context.Context, *db.WeeklyDigest) error {
	panic("not implemented")
}
func (g *guardMockQuery) GetUsersForDigest(context.Context) ([]*db.User, error) {
	panic("not implemented")
}
func (g *guardMockQuery) GetRecentProcessedCommentsForUser(context.Context, string, time.Time) ([]*db.Comment, error) {
	panic("not implemented")
}
func (g *guardMockQuery) GetEligibleCommentsForUser(context.Context, string, time.Time, time.Time) ([]*db.Comment, error) {
	panic("not implemented")
}
func (g *guardMockQuery) InsertWeeklyDigestPending(context.Context, string, time.Time) (string, bool, error) {
	panic("not implemented")
}
func (g *guardMockQuery) UpdateDigestContent(context.Context, string, []byte, string) error {
	panic("not implemented")
}
func (g *guardMockQuery) UpdateDigestStatus(context.Context, string, string) error {
	panic("not implemented")
}
func (g *guardMockQuery) GetPreviousWeekDigest(context.Context, string, time.Time) (*db.WeeklyDigest, error) {
	panic("not implemented")
}
func (g *guardMockQuery) GetLatestDigestForUser(context.Context, string) (*db.WeeklyDigest, error) {
	panic("not implemented")
}
func (g *guardMockQuery) ListDigests(context.Context, string, int, int) ([]*db.WeeklyDigest, error) {
	panic("not implemented")
}
func (g *guardMockQuery) GetStalePendingDigests(context.Context, time.Time) ([]*db.WeeklyDigest, error) {
	panic("not implemented")
}
func (g *guardMockQuery) UpdateUserDigestOptIn(context.Context, string, bool) error {
	return nil
}
func (g *guardMockQuery) MarkDigestEmailed(context.Context, string, string, time.Time) error {
	return nil
}
func (g *guardMockQuery) CreateUserWithPassword(context.Context, string, string) (*db.User, error) {
	panic("not implemented")
}
func (g *guardMockQuery) CreateUserGoogle(context.Context, string, string) (*db.User, error) {
	panic("not implemented")
}
func (g *guardMockQuery) GetUserByGoogleSub(context.Context, string) (*db.User, error) {
	panic("not implemented")
}
func (g *guardMockQuery) IncrementFailedLoginAttempts(context.Context, string) (int, error) {
	panic("not implemented")
}
func (g *guardMockQuery) LockUser(context.Context, string, time.Time) error {
	panic("not implemented")
}
func (g *guardMockQuery) ResetLoginFailures(context.Context, string) error {
	panic("not implemented")
}
func (g *guardMockQuery) IncrementTokenVersion(context.Context, string) error {
	panic("not implemented")
}
func (g *guardMockQuery) GetUserTokenVersion(context.Context, string) (int, error) {
	panic("not implemented")
}
func (g *guardMockQuery) SetEmailVerified(context.Context, string) error { panic("not implemented") }
func (g *guardMockQuery) UpdatePasswordHash(context.Context, string, string) error {
	panic("not implemented")
}
func (g *guardMockQuery) LinkGoogleSub(context.Context, string, string) error {
	panic("not implemented")
}
func (g *guardMockQuery) DeleteStaleUnverifiedUsers(context.Context, time.Duration) (int64, error) {
	panic("not implemented")
}
func (g *guardMockQuery) InsertEmailVerificationToken(context.Context, string, string, time.Time) error {
	panic("not implemented")
}
func (g *guardMockQuery) ConsumeEmailVerificationToken(context.Context, string) (string, error) {
	panic("not implemented")
}
func (g *guardMockQuery) InvalidateEmailVerificationTokensForUser(context.Context, string) error {
	panic("not implemented")
}
func (g *guardMockQuery) CountVerificationResendsInWindow(context.Context, string, time.Time) (int, error) {
	panic("not implemented")
}
func (g *guardMockQuery) InsertPasswordResetToken(context.Context, string, string, time.Time) error {
	panic("not implemented")
}
func (g *guardMockQuery) ConsumePasswordResetToken(context.Context, string) (string, error) {
	panic("not implemented")
}
func (g *guardMockQuery) InvalidatePasswordResetTokensForUser(context.Context, string) error {
	panic("not implemented")
}
func (g *guardMockQuery) CountForgotPasswordEmailsInWindow(context.Context, string, time.Time) (int, error) {
	panic("not implemented")
}
func (g *guardMockQuery) InsertRefreshToken(context.Context, string, string, time.Time) error {
	panic("not implemented")
}
func (g *guardMockQuery) GetRefreshTokenByHash(context.Context, string) (string, time.Time, error) {
	panic("not implemented")
}
func (g *guardMockQuery) DeleteRefreshTokenByHash(context.Context, string) error {
	panic("not implemented")
}
func (g *guardMockQuery) DeleteAllRefreshTokensForUser(context.Context, string) error {
	panic("not implemented")
}
func (g *guardMockQuery) InsertOAuthState(context.Context, string, string, time.Time) error {
	panic("not implemented")
}
func (g *guardMockQuery) ConsumeOAuthState(context.Context, string, string) (bool, error) {
	panic("not implemented")
}
func (g *guardMockQuery) CleanupExpiredOAuthStates(context.Context) (int64, error) {
	panic("not implemented")
}
func (g *guardMockQuery) UpsertPlatformChannel(context.Context, string, string, string) (string, bool, error) {
	panic("not implemented")
}
func (g *guardMockQuery) GetChannelByPlatformID(context.Context, string, string) (*db.PlatformChannel, error) {
	panic("not implemented")
}
func (g *guardMockQuery) ScheduleChannelDeletion(context.Context, string, int) error {
	panic("not implemented")
}
func (g *guardMockQuery) ClearChannelDeletion(context.Context, string) error {
	panic("not implemented")
}
func (g *guardMockQuery) GetActiveTrackerCount(context.Context, string) (int, error) {
	panic("not implemented")
}
func (g *guardMockQuery) PurgeChannelData(context.Context, string) error { panic("not implemented") }
func (g *guardMockQuery) CreateUserChannelTracking(context.Context, string, string, string, string, time.Time, int, int) (*db.UserChannelTracking, error) {
	panic("not implemented")
}
func (g *guardMockQuery) GetUserTracking(context.Context, string, string) (*db.UserChannelTracking, error) {
	panic("not implemented")
}
func (g *guardMockQuery) ListUserChannels(context.Context, string) ([]db.ChannelListItem, error) {
	panic("not implemented")
}
func (g *guardMockQuery) PauseTracking(context.Context, string, string) error {
	panic("not implemented")
}
func (g *guardMockQuery) ResumeTracking(context.Context, string, string) error {
	panic("not implemented")
}
func (g *guardMockQuery) DeleteTracking(context.Context, string, string) error {
	panic("not implemented")
}
func (g *guardMockQuery) UpdateBackfillDays(context.Context, string, string, int) error {
	panic("not implemented")
}
func (g *guardMockQuery) GetMaxTokenPriority(context.Context, string) (int, error) {
	panic("not implemented")
}
func (g *guardMockQuery) GetIngestionTokenRow(context.Context, string) (*db.UserChannelTracking, bool, error) {
	panic("not implemented")
}
func (g *guardMockQuery) UpdateTokenAndExpiry(context.Context, string, string, string, time.Time) error {
	panic("not implemented")
}
func (g *guardMockQuery) GetExpiringTokens(context.Context, int) ([]db.UserChannelTracking, error) {
	panic("not implemented")
}
func (g *guardMockQuery) MarkTokenExpiryNotified(context.Context, string) error {
	panic("not implemented")
}
func (g *guardMockQuery) GetCommentCountForChannel(context.Context, string) (int, error) {
	panic("not implemented")
}
func (g *guardMockQuery) IsStripeEventProcessed(context.Context, string) (bool, error) {
	panic("not implemented")
}
func (g *guardMockQuery) InsertProcessedStripeEvent(context.Context, *sql.Tx, string, string) error {
	panic("not implemented")
}
func (g *guardMockQuery) ApplySubscriptionUpdate(context.Context, *sql.Tx, db.SubscriptionUpdate) error {
	panic("not implemented")
}
func (g *guardMockQuery) GetUserByStripeCustomerID(context.Context, string) (*db.User, error) {
	panic("not implemented")
}
func (g *guardMockQuery) GetOverageCostForCurrentPeriod(context.Context, string, time.Time, time.Time) (float64, error) {
	panic("not implemented")
}
func (g *guardMockQuery) InsertCommentUsageLog(context.Context, *sql.Tx, string, int, time.Time, time.Time) error {
	panic("not implemented")
}
func (g *guardMockQuery) PauseChannelsAlphabetically(context.Context, *sql.Tx, string, int) error {
	panic("not implemented")
}
func (g *guardMockQuery) ResetFreeTierQuotas(context.Context) (int64, error) {
	panic("not implemented")
}
func (g *guardMockQuery) ExpirePastDueGrace(context.Context, int) (int64, error) {
	panic("not implemented")
}

// New methods added by spec-gap-improvements
func (g *guardMockQuery) UpdateChannelCustomPrompt(context.Context, string, string, *string) error {
	panic("not implemented")
}
func (g *guardMockQuery) CreateWorkspace(context.Context, string, string) (*db.Workspace, error) {
	panic("not implemented")
}
func (g *guardMockQuery) GetWorkspace(context.Context, string) (*db.Workspace, error) {
	panic("not implemented")
}
func (g *guardMockQuery) UpdateWorkspace(context.Context, string, string) error {
	panic("not implemented")
}
func (g *guardMockQuery) DeleteWorkspace(context.Context, string) error { panic("not implemented") }
func (g *guardMockQuery) ListWorkspacesForUser(context.Context, string) ([]*db.WorkspaceWithMembers, error) {
	panic("not implemented")
}
func (g *guardMockQuery) GetWorkspaceMember(context.Context, string, string) (*db.WorkspaceMember, error) {
	panic("not implemented")
}
func (g *guardMockQuery) AddWorkspaceMember(context.Context, string, string, string) error {
	panic("not implemented")
}
func (g *guardMockQuery) RemoveWorkspaceMember(context.Context, string, string) error {
	panic("not implemented")
}
func (g *guardMockQuery) UpdateWorkspaceMemberRole(context.Context, string, string, string) error {
	panic("not implemented")
}
func (g *guardMockQuery) CountWorkspaceMembers(context.Context, string) (int, error) {
	panic("not implemented")
}
func (g *guardMockQuery) CountWorkspaceAdmins(context.Context, string) (int, error) {
	panic("not implemented")
}
func (g *guardMockQuery) CreateWorkspaceInvite(context.Context, string, string, string, string, time.Time) (*db.WorkspaceInvite, error) {
	panic("not implemented")
}
func (g *guardMockQuery) GetWorkspaceInviteByToken(context.Context, string) (*db.WorkspaceInvite, error) {
	panic("not implemented")
}
func (g *guardMockQuery) ConsumeWorkspaceInvite(context.Context, string) error {
	panic("not implemented")
}
func (g *guardMockQuery) GetWorkspaceInviteByEmail(context.Context, string, string) (*db.WorkspaceInvite, error) {
	panic("not implemented")
}
func (g *guardMockQuery) SetUserActiveWorkspace(context.Context, string, string) error {
	panic("not implemented")
}
func (g *guardMockQuery) GetUserActiveWorkspaceID(context.Context, string) (string, error) {
	panic("not implemented")
}
func (g *guardMockQuery) IsWorkspaceOwner(context.Context, string, string) (bool, error) {
	panic("not implemented")
}
func (g *guardMockQuery) InsertAuditLog(context.Context, string, string, string, string) error {
	panic("not implemented")
}
func (g *guardMockQuery) IncrementRateLimit(context.Context, string, time.Time, int) (int, error) {
	panic("not implemented")
}
func (g *guardMockQuery) GetRateLimitStatus(context.Context, string, time.Time, int) (*db.RateLimitStatus, error) {
	panic("not implemented")
}
func (g *guardMockQuery) CleanupExpiredRateLimitBuckets(context.Context, time.Time) (int64, error) {
	panic("not implemented")
}
func (g *guardMockQuery) SearchComments(context.Context, string, string) ([]*db.Comment, error) {
	panic("not implemented")
}
func (g *guardMockQuery) BulkStarComments(context.Context, string, []string) (int, error) {
	panic("not implemented")
}
func (g *guardMockQuery) BulkUnstarComments(context.Context, string, []string) (int, error) {
	panic("not implemented")
}
func (g *guardMockQuery) GetCommentThread(context.Context, string, string, int) ([]*db.Comment, error) {
	panic("not implemented")
}
func (g *guardMockQuery) GetChannelCustomPrompt(context.Context, string, string) (string, error) {
	panic("not implemented")
}
func (g *guardMockQuery) GetCommentBodyByID(context.Context, string) (string, error) {
	panic("not implemented")
}

// ----------------------------------------------------------------------------
// tests
// ----------------------------------------------------------------------------

func TestCheckChannelLimitFreeBlockedAtSecond(t *testing.T) {
	q := &guardMockQuery{
		user:       &db.User{ID: "u", SubscriptionPlan: billing.PlanFree},
		channelCnt: 1,
	}
	blocked, err := checkChannelLimit(context.Background(), q, "u")
	if err != nil {
		t.Fatal(err)
	}
	if !blocked {
		t.Error("free user with 1 channel must be blocked at second connect")
	}
}

func TestCheckChannelLimitCreatorAllowedThird(t *testing.T) {
	q := &guardMockQuery{
		user:       &db.User{ID: "u", SubscriptionPlan: billing.PlanCreator},
		channelCnt: 2,
	}
	blocked, _ := checkChannelLimit(context.Background(), q, "u")
	if blocked {
		t.Error("creator with 2 channels must be allowed a third")
	}
}

func TestCheckChannelLimitCreatorBlockedAtFourth(t *testing.T) {
	q := &guardMockQuery{
		user:       &db.User{ID: "u", SubscriptionPlan: billing.PlanCreator},
		channelCnt: 3,
	}
	blocked, _ := checkChannelLimit(context.Background(), q, "u")
	if !blocked {
		t.Error("creator with 3 channels must be blocked at fourth")
	}
}

func TestCheckChannelLimitStudioBoundary(t *testing.T) {
	q := &guardMockQuery{
		user:       &db.User{ID: "u", SubscriptionPlan: billing.PlanStudio},
		channelCnt: 14,
	}
	if blocked, _ := checkChannelLimit(context.Background(), q, "u"); blocked {
		t.Error("studio with 14 channels must be allowed a 15th")
	}
	q.channelCnt = 15
	if blocked, _ := checkChannelLimit(context.Background(), q, "u"); !blocked {
		t.Error("studio with 15 channels must be blocked at 16th")
	}
}

func TestCheckChannelLimitAgencyUnlimited(t *testing.T) {
	q := &guardMockQuery{
		user:       &db.User{ID: "u", SubscriptionPlan: billing.PlanAgency},
		channelCnt: 50,
	}
	blocked, _ := checkChannelLimit(context.Background(), q, "u")
	if blocked {
		t.Error("agency with 50 channels must never be blocked")
	}
}

func TestCheckChannelLimitMissingUserPermitsConnect(t *testing.T) {
	// We never want a transient lookup hiccup to brick all new connects.
	q := &guardMockQuery{getUserErr: errors.New("db down")}
	blocked, err := checkChannelLimit(context.Background(), q, "u")
	if err == nil {
		t.Error("expected error to surface for caller logging")
	}
	if blocked {
		t.Error("guard must permissively allow when user lookup fails")
	}
}
