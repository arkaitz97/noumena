package api_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"noumena/internal/api"
	"noumena/internal/config"
	"noumena/internal/db"
)

type rlMockQuery struct {
	db.DBQuery
	count    int
	failNext bool
	lastKey  string
}

func (m *rlMockQuery) IncrementRateLimit(ctx context.Context, userKey string, windowStart time.Time, limit int) (int, error) {
	if m.failNext {
		m.failNext = false
		return 0, context.Canceled
	}
	m.count++
	m.lastKey = userKey
	return m.count, nil
}

func (m *rlMockQuery) GetRateLimitStatus(ctx context.Context, userKey string, windowStart time.Time, limit int) (*db.RateLimitStatus, error) {
	return nil, nil
}

func (m *rlMockQuery) CleanupExpiredRateLimitBuckets(ctx context.Context, before time.Time) (int64, error) {
	return 0, nil
}

func (m *rlMockQuery) CreateUser(ctx context.Context, email, passwordHash string) (*db.User, error) {
	return nil, nil
}
func (m *rlMockQuery) GetUserByEmail(ctx context.Context, email string) (*db.User, error) {
	return nil, nil
}
func (m *rlMockQuery) GetUserByID(ctx context.Context, id string) (*db.User, error) {
	return nil, nil
}
func (m *rlMockQuery) UpdateUserSubscription(ctx context.Context, userID, plan, stripeCustomerID string, quota int) error {
	return nil
}
func (m *rlMockQuery) ListChannelsForUser(ctx context.Context, userID string) ([]*db.PlatformChannel, error) {
	return nil, nil
}
func (m *rlMockQuery) ConnectChannelTx(ctx context.Context, userID, platform, platformChannelID, displayName, accessTokenEnc, refreshTokenEnc string, tokenExpiresAt *time.Time) error {
	return nil
}
func (m *rlMockQuery) PauseChannelTracking(ctx context.Context, userID, platformChannelID string) error {
	return nil
}
func (m *rlMockQuery) RemoveChannelTracking(ctx context.Context, userID, platformChannelID string) error {
	return nil
}
func (m *rlMockQuery) GetChannelsForSync(ctx context.Context, minutes int) ([]*db.PlatformChannel, error) {
	return nil, nil
}
func (m *rlMockQuery) UpdateSyncTime(ctx context.Context, id string) error {
	return nil
}
func (m *rlMockQuery) GetCommentFeed(ctx context.Context, userID string, limit, offset int) ([]*db.Comment, error) {
	return nil, nil
}
func (m *rlMockQuery) StarComment(ctx context.Context, userID, commentID string) error {
	return nil
}
func (m *rlMockQuery) GetIngestionToken(ctx context.Context, platformChannelID string) (string, string, error) {
	return "", "", nil
}
func (m *rlMockQuery) PromoteNextIngestionToken(ctx context.Context, platformChannelID string) error {
	return nil
}
func (m *rlMockQuery) InsertComment(ctx context.Context, c *db.Comment) error {
	return nil
}
func (m *rlMockQuery) GetUnprocessedComments(ctx context.Context, limit int) ([]*db.Comment, error) {
	return nil, nil
}
func (m *rlMockQuery) GetUnprocessedCommentsByChannel(ctx context.Context, limit int, maxAttempts int) (map[string][]*db.Comment, error) {
	return nil, nil
}
func (m *rlMockQuery) UpdateCommentLLMData(ctx context.Context, id, category, topic, provider, model string, actionableInsights []byte, confidence float64) error {
	return nil
}
func (m *rlMockQuery) IncrementCommentsUsed(ctx context.Context, userID string, count int) error {
	return nil
}
func (m *rlMockQuery) GetPrimaryTracker(ctx context.Context, platformChannelID string) (*db.User, error) {
	return nil, nil
}
func (m *rlMockQuery) EnqueueLLMJob(ctx context.Context, commentIDs []string, provider string) (string, error) {
	return "", nil
}
func (m *rlMockQuery) MarkLLMJobProcessing(ctx context.Context, jobID string) error {
	return nil
}
func (m *rlMockQuery) MarkLLMJobCompleted(ctx context.Context, jobID string) error {
	return nil
}
func (m *rlMockQuery) MarkLLMJobFailed(ctx context.Context, jobID string, msg string) error {
	return nil
}
func (m *rlMockQuery) InsertWeeklyDigest(ctx context.Context, digest *db.WeeklyDigest) error {
	return nil
}
func (m *rlMockQuery) GetUsersForDigest(ctx context.Context) ([]*db.User, error) {
	return nil, nil
}
func (m *rlMockQuery) GetRecentProcessedCommentsForUser(ctx context.Context, userID string, since time.Time) ([]*db.Comment, error) {
	return nil, nil
}
func (m *rlMockQuery) GetEligibleCommentsForUser(ctx context.Context, userID string, since, until time.Time) ([]*db.Comment, error) {
	return nil, nil
}
func (m *rlMockQuery) InsertWeeklyDigestPending(ctx context.Context, userID string, weekStart time.Time) (string, bool, error) {
	return "", false, nil
}
func (m *rlMockQuery) UpdateDigestContent(ctx context.Context, digestID string, contentJSON []byte, status string) error {
	return nil
}
func (m *rlMockQuery) UpdateDigestStatus(ctx context.Context, digestID, status string) error {
	return nil
}
func (m *rlMockQuery) GetPreviousWeekDigest(ctx context.Context, userID string, weekStart time.Time) (*db.WeeklyDigest, error) {
	return nil, nil
}
func (m *rlMockQuery) GetLatestDigestForUser(ctx context.Context, userID string) (*db.WeeklyDigest, error) {
	return nil, nil
}
func (m *rlMockQuery) ListDigests(ctx context.Context, userID string, limit, offset int) ([]*db.WeeklyDigest, error) {
	return nil, nil
}
func (m *rlMockQuery) GetStalePendingDigests(ctx context.Context, olderThan time.Time) ([]*db.WeeklyDigest, error) {
	return nil, nil
}
func (m *rlMockQuery) UpdateUserDigestOptIn(ctx context.Context, userID string, optIn bool) error {
	return nil
}
func (m *rlMockQuery) MarkDigestEmailed(ctx context.Context, digestID, userID string, emailedAt time.Time) error {
	return nil
}
func (m *rlMockQuery) CreateUserWithPassword(ctx context.Context, email, passwordHash string) (*db.User, error) {
	return nil, nil
}
func (m *rlMockQuery) CreateUserGoogle(ctx context.Context, email, googleSub string) (*db.User, error) {
	return nil, nil
}
func (m *rlMockQuery) GetUserByGoogleSub(ctx context.Context, sub string) (*db.User, error) {
	return nil, nil
}
func (m *rlMockQuery) IncrementFailedLoginAttempts(ctx context.Context, userID string) (int, error) {
	return 0, nil
}
func (m *rlMockQuery) LockUser(ctx context.Context, userID string, until time.Time) error {
	return nil
}
func (m *rlMockQuery) ResetLoginFailures(ctx context.Context, userID string) error {
	return nil
}
func (m *rlMockQuery) IncrementTokenVersion(ctx context.Context, userID string) error {
	return nil
}
func (m *rlMockQuery) GetUserTokenVersion(ctx context.Context, userID string) (int, error) {
	return 0, nil
}
func (m *rlMockQuery) SetEmailVerified(ctx context.Context, userID string) error {
	return nil
}
func (m *rlMockQuery) UpdatePasswordHash(ctx context.Context, userID, hash string) error {
	return nil
}
func (m *rlMockQuery) LinkGoogleSub(ctx context.Context, userID, sub string) error {
	return nil
}
func (m *rlMockQuery) DeleteStaleUnverifiedUsers(ctx context.Context, olderThan time.Duration) (int64, error) {
	return 0, nil
}
func (m *rlMockQuery) InsertEmailVerificationToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	return nil
}
func (m *rlMockQuery) ConsumeEmailVerificationToken(ctx context.Context, tokenHash string) (string, error) {
	return "", nil
}
func (m *rlMockQuery) InvalidateEmailVerificationTokensForUser(ctx context.Context, userID string) error {
	return nil
}
func (m *rlMockQuery) CountVerificationResendsInWindow(ctx context.Context, userID string, since time.Time) (int, error) {
	return 0, nil
}
func (m *rlMockQuery) InsertPasswordResetToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	return nil
}
func (m *rlMockQuery) ConsumePasswordResetToken(ctx context.Context, tokenHash string) (string, error) {
	return "", nil
}
func (m *rlMockQuery) InvalidatePasswordResetTokensForUser(ctx context.Context, userID string) error {
	return nil
}
func (m *rlMockQuery) CountForgotPasswordEmailsInWindow(ctx context.Context, email string, since time.Time) (int, error) {
	return 0, nil
}
func (m *rlMockQuery) InsertRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	return nil
}
func (m *rlMockQuery) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (string, time.Time, error) {
	return "", time.Time{}, nil
}
func (m *rlMockQuery) DeleteRefreshTokenByHash(ctx context.Context, tokenHash string) error {
	return nil
}
func (m *rlMockQuery) DeleteAllRefreshTokensForUser(ctx context.Context, userID string) error {
	return nil
}
func (m *rlMockQuery) InsertOAuthState(ctx context.Context, stateHash, userID string, expiresAt time.Time) error {
	return nil
}
func (m *rlMockQuery) ConsumeOAuthState(ctx context.Context, stateHash, userID string) (bool, error) {
	return true, nil
}
func (m *rlMockQuery) CleanupExpiredOAuthStates(ctx context.Context) (int64, error) {
	return 0, nil
}
func (m *rlMockQuery) UpsertPlatformChannel(ctx context.Context, platform, platformChannelID, displayName string) (string, bool, error) {
	return "", false, nil
}
func (m *rlMockQuery) GetChannelByPlatformID(ctx context.Context, platform, platformChannelID string) (*db.PlatformChannel, error) {
	return nil, nil
}
func (m *rlMockQuery) ScheduleChannelDeletion(ctx context.Context, platformChannelID string, graceHours int) error {
	return nil
}
func (m *rlMockQuery) ClearChannelDeletion(ctx context.Context, platformChannelID string) error {
	return nil
}
func (m *rlMockQuery) GetActiveTrackerCount(ctx context.Context, platformChannelID string) (int, error) {
	return 0, nil
}
func (m *rlMockQuery) PurgeChannelData(ctx context.Context, platformChannelID string) error {
	return nil
}
func (m *rlMockQuery) CreateUserChannelTracking(ctx context.Context, userID, platformChannelID, encryptedAccess, encryptedRefresh string, expiresAt time.Time, backfillDays, tokenPriority int) (*db.UserChannelTracking, error) {
	return nil, nil
}
func (m *rlMockQuery) GetUserTracking(ctx context.Context, userID, platformChannelID string) (*db.UserChannelTracking, error) {
	return nil, nil
}
func (m *rlMockQuery) ListUserChannels(ctx context.Context, userID string) ([]db.ChannelListItem, error) {
	return nil, nil
}
func (m *rlMockQuery) PauseTracking(ctx context.Context, userID, platformChannelID string) error {
	return nil
}
func (m *rlMockQuery) ResumeTracking(ctx context.Context, userID, platformChannelID string) error {
	return nil
}
func (m *rlMockQuery) DeleteTracking(ctx context.Context, userID, platformChannelID string) error {
	return nil
}
func (m *rlMockQuery) UpdateBackfillDays(ctx context.Context, userID, platformChannelID string, days int) error {
	return nil
}
func (m *rlMockQuery) GetMaxTokenPriority(ctx context.Context, platformChannelID string) (int, error) {
	return 0, nil
}
func (m *rlMockQuery) GetIngestionTokenRow(ctx context.Context, platformChannelID string) (*db.UserChannelTracking, bool, error) {
	return nil, false, nil
}
func (m *rlMockQuery) UpdateTokenAndExpiry(ctx context.Context, trackingID, encryptedAccess, encryptedRefresh string, expiresAt time.Time) error {
	return nil
}
func (m *rlMockQuery) GetExpiringTokens(ctx context.Context, withinDays int) ([]db.UserChannelTracking, error) {
	return nil, nil
}
func (m *rlMockQuery) MarkTokenExpiryNotified(ctx context.Context, trackingID string) error {
	return nil
}
func (m *rlMockQuery) GetCommentCountForChannel(ctx context.Context, platformChannelID string) (int, error) {
	return 0, nil
}
func (m *rlMockQuery) IsStripeEventProcessed(ctx context.Context, eventID string) (bool, error) {
	return false, nil
}
func (m *rlMockQuery) InsertProcessedStripeEvent(ctx context.Context, tx *sql.Tx, eventID, eventType string) error {
	return nil
}
func (m *rlMockQuery) ApplySubscriptionUpdate(ctx context.Context, tx *sql.Tx, u db.SubscriptionUpdate) error {
	return nil
}
func (m *rlMockQuery) GetUserByStripeCustomerID(ctx context.Context, customerID string) (*db.User, error) {
	return nil, nil
}
func (m *rlMockQuery) GetActiveChannelCount(ctx context.Context, userID string) (int, error) {
	return 0, nil
}
func (m *rlMockQuery) GetOverageCostForCurrentPeriod(ctx context.Context, userID string, periodStart, periodEnd time.Time) (float64, error) {
	return 0, nil
}
func (m *rlMockQuery) InsertCommentUsageLog(ctx context.Context, tx *sql.Tx, userID string, count int, periodStart, periodEnd time.Time) error {
	return nil
}
func (m *rlMockQuery) PauseChannelsAlphabetically(ctx context.Context, tx *sql.Tx, userID string, keepCount int) error {
	return nil
}
func (m *rlMockQuery) ResetFreeTierQuotas(ctx context.Context) (int64, error) {
	return 0, nil
}
func (m *rlMockQuery) ExpirePastDueGrace(ctx context.Context, graceHours int) (int64, error) {
	return 0, nil
}
func (m *rlMockQuery) UpdateChannelCustomPrompt(ctx context.Context, userID, platformChannelID string, customPrompt *string) error {
	return nil
}
func (m *rlMockQuery) CreateWorkspace(ctx context.Context, name, createdBy string) (*db.Workspace, error) {
	return nil, nil
}
func (m *rlMockQuery) GetWorkspace(ctx context.Context, workspaceID string) (*db.Workspace, error) {
	return nil, nil
}
func (m *rlMockQuery) UpdateWorkspace(ctx context.Context, workspaceID, name string) error {
	return nil
}
func (m *rlMockQuery) DeleteWorkspace(ctx context.Context, workspaceID string) error {
	return nil
}
func (m *rlMockQuery) ListWorkspacesForUser(ctx context.Context, userID string) ([]*db.WorkspaceWithMembers, error) {
	return nil, nil
}
func (m *rlMockQuery) GetWorkspaceMember(ctx context.Context, workspaceID, userID string) (*db.WorkspaceMember, error) {
	return nil, nil
}
func (m *rlMockQuery) AddWorkspaceMember(ctx context.Context, workspaceID, userID, role string) error {
	return nil
}
func (m *rlMockQuery) RemoveWorkspaceMember(ctx context.Context, workspaceID, userID string) error {
	return nil
}
func (m *rlMockQuery) UpdateWorkspaceMemberRole(ctx context.Context, workspaceID, userID, role string) error {
	return nil
}
func (m *rlMockQuery) CountWorkspaceMembers(ctx context.Context, workspaceID string) (int, error) {
	return 0, nil
}
func (m *rlMockQuery) CountWorkspaceAdmins(ctx context.Context, workspaceID string) (int, error) {
	return 0, nil
}
func (m *rlMockQuery) CreateWorkspaceInvite(ctx context.Context, workspaceID, email, role, tokenHash string, expiresAt time.Time) (*db.WorkspaceInvite, error) {
	return nil, nil
}
func (m *rlMockQuery) GetWorkspaceInviteByToken(ctx context.Context, tokenHash string) (*db.WorkspaceInvite, error) {
	return nil, nil
}
func (m *rlMockQuery) ConsumeWorkspaceInvite(ctx context.Context, inviteID string) error {
	return nil
}
func (m *rlMockQuery) GetWorkspaceInviteByEmail(ctx context.Context, workspaceID, email string) (*db.WorkspaceInvite, error) {
	return nil, nil
}
func (m *rlMockQuery) SetUserActiveWorkspace(ctx context.Context, userID, workspaceID string) error {
	return nil
}
func (m *rlMockQuery) GetUserActiveWorkspaceID(ctx context.Context, userID string) (string, error) {
	return "", nil
}
func (m *rlMockQuery) IsWorkspaceOwner(ctx context.Context, workspaceID, userID string) (bool, error) {
	return false, nil
}
func (m *rlMockQuery) InsertAuditLog(ctx context.Context, userID, action, details, ipAddress string) error {
	return nil
}
func (m *rlMockQuery) GetChannelCustomPrompt(ctx context.Context, platformChannelID, userID string) (string, error) {
	return "", nil
}
func (m *rlMockQuery) GetCommentBodyByID(ctx context.Context, commentID string) (string, error) {
	return "", nil
}
func (m *rlMockQuery) SearchComments(ctx context.Context, userID, query string) ([]*db.Comment, error) {
	return nil, nil
}
func (m *rlMockQuery) BulkStarComments(ctx context.Context, userID string, commentIDs []string) (int, error) {
	return 0, nil
}
func (m *rlMockQuery) BulkUnstarComments(ctx context.Context, userID string, commentIDs []string) (int, error) {
	return 0, nil
}
func (m *rlMockQuery) GetCommentThread(ctx context.Context, userID, commentID string, maxDepth int) ([]*db.Comment, error) {
	return nil, nil
}
func (m *rlMockQuery) ScheduleAccountDeletion(ctx context.Context, userID string, channelCount int) (*db.AccountDeletionJob, error) {
	return &db.AccountDeletionJob{ID: "job-1"}, nil
}
func (m *rlMockQuery) CancelAccountDeletion(ctx context.Context, userID string) error {
	return nil
}
func (m *rlMockQuery) EnqueueDataExport(ctx context.Context, userID string) (*db.DataExportJob, error) {
	return nil, nil
}
func (m *rlMockQuery) GetDataExportJob(ctx context.Context, jobID string) (*db.DataExportJob, error) {
	return nil, nil
}
func (m *rlMockQuery) GetUserPreferences(ctx context.Context, userID string) (map[string]string, error) {
	return nil, nil
}
func (m *rlMockQuery) UpdateUserPreference(ctx context.Context, userID, key, value string) error {
	return nil
}
func (m *rlMockQuery) GetCommentFeedWithFilters(ctx context.Context, userID string, filters map[string]string) ([]*db.Comment, int, error) {
	return nil, 0, nil
}
func (m *rlMockQuery) GrantChannel(ctx context.Context, workspaceID, channelID, grantedBy string) error {
	return nil
}
func (m *rlMockQuery) RevokeChannel(ctx context.Context, workspaceID, channelID string) error {
	return nil
}
func (m *rlMockQuery) ListGrantedChannels(ctx context.Context, workspaceID string) ([]string, error) {
	return nil, nil
}

// LifecycleStore methods
func (m *rlMockQuery) ClaimNextDeletionJob(ctx context.Context, gracePeriod time.Duration) (*db.AccountDeletionJob, error) {
	return nil, nil
}
func (m *rlMockQuery) MarkDeletionStage(ctx context.Context, jobID, status string) error {
	return nil
}
func (m *rlMockQuery) MarkDeletionFailed(ctx context.Context, jobID, errorMsg string) error {
	return nil
}
func (m *rlMockQuery) IncrementChannelsProcessed(ctx context.Context, jobID string) error {
	return nil
}
func (m *rlMockQuery) MarkDeletionCompleted(ctx context.Context, jobID, userID string) error {
	return nil
}
func (m *rlMockQuery) MarkDeletionCompletedJobOnly(ctx context.Context, jobID string) error {
	return nil
}
func (m *rlMockQuery) GetUserTrackingChannels(ctx context.Context, userID string) ([]*db.UserChannelTrackingRow, error) {
	return nil, nil
}
func (m *rlMockQuery) GetActiveTrackerCountForUpdate(ctx context.Context, tx *sql.Tx, platformChannelID, excludeUserID string) (int, error) {
	return 0, nil
}
func (m *rlMockQuery) DeleteUserChannelTracking(ctx context.Context, tx *sql.Tx, userID, platformChannelID string) error {
	return nil
}
func (m *rlMockQuery) DeleteCommentsByChannelChunked(ctx context.Context, platformChannelID string, chunkSize int, excludeUserID string) (int, error) {
	return 0, nil
}
func (m *rlMockQuery) DeletePlatformChannel(ctx context.Context, tx *sql.Tx, platformChannelID string) error {
	return nil
}
func (m *rlMockQuery) DeleteUserStarsForChannel(ctx context.Context, tx *sql.Tx, userID, platformChannelID string) error {
	return nil
}
func (m *rlMockQuery) DeleteUserStarsAll(ctx context.Context, tx *sql.Tx, userID string) error {
	return nil
}
func (m *rlMockQuery) DeleteUserDigests(ctx context.Context, tx *sql.Tx, userID string) error {
	return nil
}
func (m *rlMockQuery) DeleteUserAuthTokens(ctx context.Context, tx *sql.Tx, userID string) error {
	return nil
}
func (m *rlMockQuery) DeleteUserGDPRData(ctx context.Context, tx *sql.Tx, userID string) error {
	return nil
}
func (m *rlMockQuery) MarkDeletionRevokeStatus(ctx context.Context, jobID string, attempted bool, errorMsg string) error {
	return nil
}
func (m *rlMockQuery) ListJobsPendingRevoke(ctx context.Context) ([]*db.AccountDeletionJob, error) {
	return nil, nil
}
func (m *rlMockQuery) HardDeleteUser(ctx context.Context, tx *sql.Tx, userID string) error {
	return nil
}
func (m *rlMockQuery) ClaimNextExportJob(ctx context.Context) (*db.DataExportJob, error) {
	return nil, nil
}
func (m *rlMockQuery) MarkExportStarted(ctx context.Context, jobID string) error {
	return nil
}
func (m *rlMockQuery) MarkDataExportCompleted(ctx context.Context, jobID, userID, storagePath, downloadURL string, expiresAt time.Time, partial bool) error {
	return nil
}
func (m *rlMockQuery) MarkDataExportFailed(ctx context.Context, jobID, errorMsg string) error {
	return nil
}
func (m *rlMockQuery) ListExpiredDataExports(ctx context.Context, now time.Time) ([]*db.DataExportJob, error) {
	return nil, nil
}
func (m *rlMockQuery) ClearExportStoragePath(ctx context.Context, jobID string) error {
	return nil
}
func (m *rlMockQuery) GetUserExportBundle(ctx context.Context, userID string) (*db.UserExportBundle, error) {
	return nil, nil
}

// EmailLogStore methods
func (m *rlMockQuery) IsEmailIdempotencyKeyUsed(ctx context.Context, key string) (bool, error) {
	return false, nil
}
func (m *rlMockQuery) LogEmailAttempt(ctx context.Context, in db.EmailAttemptInput) (string, error) {
	return "", nil
}
func (m *rlMockQuery) MarkEmailSent(ctx context.Context, id string, at time.Time) error {
	return nil
}
func (m *rlMockQuery) IncrementEmailAttempt(ctx context.Context, id string, attempt int, lastErr string) error {
	return nil
}
func (m *rlMockQuery) MarkEmailFailed(ctx context.Context, id string, lastErr string) error {
	return nil
}
func (m *rlMockQuery) MarkEmailAttemptError(ctx context.Context, id string, errMsg string) error {
	return nil
}
func (m *rlMockQuery) ClaimPendingEmails(ctx context.Context, now time.Time, max int) ([]*db.EmailLogRow, error) {
	return nil, nil
}
func (m *rlMockQuery) GetEmailRowByIdempotencyKey(ctx context.Context, key string) (*db.EmailLogRow, error) {
	return nil, nil
}

func TestRateLimitMiddleware_Disabled(t *testing.T) {
	q := &rlMockQuery{}
	cfg := &config.Config{RateLimitEnabled: false}
	mw := api.RateLimitMiddleware(q, cfg)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 when rate limiting disabled, got %d", rr.Code)
	}
}

func TestRateLimitMiddleware_BypassPaths(t *testing.T) {
	q := &rlMockQuery{}
	cfg := &config.Config{RateLimitEnabled: true}
	mw := api.RateLimitMiddleware(q, cfg)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	bypassPaths := []string{"/health", "/ready", "/webhooks/stripe", "/metrics"}
	for _, path := range bypassPaths {
		req := httptest.NewRequest("GET", path, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("path %s: expected 200 (bypass), got %d", path, rr.Code)
		}
		if rr.Header().Get("X-RateLimit-Limit") != "" {
			t.Errorf("path %s: expected no rate limit headers, got %s", path, rr.Header().Get("X-RateLimit-Limit"))
		}
	}
}

func TestRateLimitMiddleware_CounterIncrement(t *testing.T) {
	q := &rlMockQuery{}
	cfg := &config.Config{
		RateLimitEnabled:         true,
		RateLimitAuthMaxPerMin:   100,
		RateLimitUnauthMaxPerMin: 10,
	}
	mw := api.RateLimitMiddleware(q, cfg)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 1; i <= 5; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i, rr.Code)
		}
	}
}

func TestRateLimitMiddleware_ExceedsLimit(t *testing.T) {
	q := &rlMockQuery{}
	cfg := &config.Config{
		RateLimitEnabled:         true,
		RateLimitAuthMaxPerMin:   100,
		RateLimitUnauthMaxPerMin: 3,
	}
	mw := api.RateLimitMiddleware(q, cfg)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 1; i <= 3; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i, rr.Code)
		}
	}

	req := httptest.NewRequest("GET", "/api/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 after exceeding limit, got %d", rr.Code)
	}
	if rr.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header on 429 response")
	}
}

func TestRateLimitMiddleware_RateLimitHeaders(t *testing.T) {
	q := &rlMockQuery{}
	cfg := &config.Config{
		RateLimitEnabled:         true,
		RateLimitAuthMaxPerMin:   100,
		RateLimitUnauthMaxPerMin: 10,
	}
	mw := api.RateLimitMiddleware(q, cfg)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get("X-RateLimit-Limit") != "10" {
		t.Errorf("expected limit 10, got %s", rr.Header().Get("X-RateLimit-Limit"))
	}
	if rr.Header().Get("X-RateLimit-Remaining") == "" {
		t.Error("expected X-RateLimit-Remaining header")
	}
	if rr.Header().Get("X-RateLimit-Reset") == "" {
		t.Error("expected X-RateLimit-Reset header")
	}
}

func TestRateLimitMiddleware_DBErrorPassthrough(t *testing.T) {
	q := &rlMockQuery{failNext: true}
	cfg := &config.Config{
		RateLimitEnabled:         true,
		RateLimitAuthMaxPerMin:   100,
		RateLimitUnauthMaxPerMin: 10,
	}
	mw := api.RateLimitMiddleware(q, cfg)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 on DB error (passthrough), got %d", rr.Code)
	}
}

// TestRateLimit_SpoofedForwardedHeaderIgnored — spec "Client identity from
// a trusted source": with no TRUST_PROXY_HEADER configured, forged
// X-Forwarded-For / X-Real-IP values must NOT change the bucket key; the
// RemoteAddr-derived key is used.
func TestRateLimit_SpoofedForwardedHeaderIgnored(t *testing.T) {
	cfg := &config.Config{
		RateLimitEnabled:         true,
		RateLimitAuthMaxPerMin:   100,
		RateLimitUnauthMaxPerMin: 10,
	}
	q := &rlMockQuery{}
	mw := api.RateLimitMiddleware(q, cfg)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/auth/login", nil)
	req.RemoteAddr = "203.0.113.5:41234"
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	req.Header.Set("X-Real-IP", "9.9.9.9")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	first := q.lastKey

	req2 := httptest.NewRequest("GET", "/auth/login", nil)
	req2.RemoteAddr = "203.0.113.5:9999"
	req2.Header.Set("X-Forwarded-For", "66.66.66.66")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	if first == "" || !strings.HasPrefix(first, "ip:") {
		t.Fatalf("expected ip-hashed key, got %q", first)
	}
	if q.lastKey != first {
		t.Errorf("spoofed XFF changed the rate key: got %q want %q", q.lastKey, first)
	}
}

// TestRateLimit_TrustedProxyHeader — when TRUST_PROXY_HEADER is configured,
// that header (right-most entry) is the bucket key, and client-supplied
// X-Forwarded-For is ignored.
func TestRateLimit_TrustedProxyHeader(t *testing.T) {
	cfg := &config.Config{
		RateLimitEnabled:         true,
		RateLimitAuthMaxPerMin:   100,
		RateLimitUnauthMaxPerMin: 10,
		TrustProxyHeader:         "X-Real-IP",
	}
	q := &rlMockQuery{}
	mw := api.RateLimitMiddleware(q, cfg)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/auth/login", nil)
	req.RemoteAddr = "10.0.0.1:41234"
	req.Header.Set("X-Real-IP", "198.51.100.7")
	req.Header.Set("X-Forwarded-For", "203.0.113.9") // spoofed, ignored
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	expHash := sha256.Sum256([]byte("198.51.100.7"))
	if q.lastKey != "ip:"+hex.EncodeToString(expHash[:]) {
		t.Errorf("expected key from trusted header, got %q", q.lastKey)
	}
}

// TestRateLimit_MultiValueForwardedChain — with X-Forwarded-For explicitly
// trusted, the right-most entry (the peer the proxy observed) wins; a
// spoofed client prefix does not reset the bucket.
func TestRateLimit_MultiValueForwardedChain(t *testing.T) {
	cfg := &config.Config{
		RateLimitEnabled:         true,
		RateLimitAuthMaxPerMin:   100,
		RateLimitUnauthMaxPerMin: 10,
		TrustProxyHeader:         "X-Forwarded-For",
	}
	q := &rlMockQuery{}
	mw := api.RateLimitMiddleware(q, cfg)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/auth/login", nil)
	req.RemoteAddr = "10.0.0.1:41234"
	req.Header.Set("X-Forwarded-For", "198.51.100.7, 10.0.0.1")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	first := q.lastKey

	expHash := sha256.Sum256([]byte("10.0.0.1"))
	if first != "ip:"+hex.EncodeToString(expHash[:]) {
		t.Errorf("expected right-most proxy-observed IP, got %q", first)
	}

	req2 := httptest.NewRequest("GET", "/auth/login", nil)
	req2.RemoteAddr = "10.0.0.1:41234"
	req2.Header.Set("X-Forwarded-For", "66.66.66.66, 10.0.0.1")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if q.lastKey != first {
		t.Errorf("spoofed chain prefix changed the key: got %q want %q", q.lastKey, first)
	}
}

// TestRateLimit_PerUserKeying — an authenticated request (user id in the
// request context, set by AuthMiddleware) is keyed per-user, not by IP.
func TestRateLimit_PerUserKeying(t *testing.T) {
	cfg := &config.Config{
		RateLimitEnabled:         true,
		RateLimitAuthMaxPerMin:   100,
		RateLimitUnauthMaxPerMin: 10,
		TrustProxyHeader:         "X-Forwarded-For",
	}
	q := &rlMockQuery{}
	mw := api.RateLimitMiddleware(q, cfg)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req = req.WithContext(context.WithValue(req.Context(), api.UserIDKey, "user-42"))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if q.lastKey != "u:user-42" {
		t.Errorf("expected per-user key, got %q", q.lastKey)
	}
}

// TestRateLimit_AnonymousPathKeyedByTrustedIP — anonymous requests get an
// IP bucket derived from the trusted source.
func TestRateLimit_AnonymousPathKeyedByTrustedIP(t *testing.T) {
	cfg := &config.Config{
		RateLimitEnabled:         true,
		RateLimitAuthMaxPerMin:   100,
		RateLimitUnauthMaxPerMin: 10,
	}
	q := &rlMockQuery{}
	mw := api.RateLimitMiddleware(q, cfg)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/auth/register", nil)
	req.RemoteAddr = "192.0.2.44:5555"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	expHash := sha256.Sum256([]byte("192.0.2.44"))
	if q.lastKey != "ip:"+hex.EncodeToString(expHash[:]) {
		t.Errorf("expected anonymous IP key, got %q", q.lastKey)
	}
}

// TestRateLimit_UnauthenticatedUsesUnauthLimit — anonymous requests are
// throttled at the unauthenticated limit, not the authenticated one.
func TestRateLimit_UnauthenticatedUsesUnauthLimit(t *testing.T) {
	cfg := &config.Config{
		RateLimitEnabled:         true,
		RateLimitAuthMaxPerMin:   100,
		RateLimitUnauthMaxPerMin: 3,
	}
	q := &rlMockQuery{}
	mw := api.RateLimitMiddleware(q, cfg)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 1; i <= 3; i++ {
		req := httptest.NewRequest("GET", "/auth/login", nil)
		req.RemoteAddr = "198.51.100.20:1111"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, rr.Code)
		}
		if rr.Header().Get("X-RateLimit-Limit") != "3" {
			t.Errorf("request %d: expected unauth limit header 3, got %s", i, rr.Header().Get("X-RateLimit-Limit"))
		}
	}
	req := httptest.NewRequest("GET", "/auth/login", nil)
	req.RemoteAddr = "198.51.100.20:1111"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 for anonymous request over limit, got %d", rr.Code)
	}
}

func TestRateLimitCleaner_StartStop(t *testing.T) {
	q := &rlMockQuery{}
	cleaner := &api.RateLimitCleaner{
		Queries: q,
		Tick:    100 * time.Millisecond,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cleaner.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	cleaner.Stop()

	cleaner.Stop()
}

func TestRateLimitCleaner_DoubleStart(t *testing.T) {
	q := &rlMockQuery{}
	cleaner := &api.RateLimitCleaner{
		Queries: q,
		Tick:    100 * time.Millisecond,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cleaner.Start(ctx)
	cleaner.Start(ctx)
	cleaner.Stop()
}
