package api_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"noumena/internal/api"
	"noumena/internal/auth"
	"noumena/internal/db"
	"noumena/internal/email"
)

// MockEmailSend emulates one row of the durable email_send_log table.
type MockEmailSend struct {
	Template  string
	ToAddress string
	At        time.Time
}

// recordEmailSend is called by mockEmailSender to mirror the production
// pipeline's email_send_log write, so rate-limit counters see the send.
func (m *MockQuery) recordEmailSend(template, toAddress string) {
	m.emailSends = append(m.emailSends, MockEmailSend{
		Template: template, ToAddress: toAddress, At: time.Now().UTC(),
	})
}

// MockQuery implements db.DBQuery for testing
type MockQuery struct {
	users         map[string]*db.User
	refreshTokens map[string]struct {
		userID  string
		expires time.Time
	}
	emailVerifTokens map[string]struct {
		userID  string
		expires time.Time
		usedAt  *time.Time
	}
	emailChangeTokens map[string]struct {
		userID  string
		email   string
		expires time.Time
		usedAt  *time.Time
	}
	passwordResetTokens map[string]struct {
		userID  string
		expires time.Time
		usedAt  *time.Time
	}
	verifResendCount       map[string]int
	emailChangeResendCount map[string]int
	// emailSends emulates the durable email_send_log rows the production
	// pipeline writes; the mock sender records into it via recordEmailSend.
	emailSends        []MockEmailSend
	lockoutEmailsSent int
	verifyEmailsSent  int
	resetEmailsSent   int
	googleNoticesSent int
	// weekly-digest fixtures (read-only handler tests)
	latestDigests map[string]*db.WeeklyDigest
	digestsByUser map[string][]*db.WeeklyDigest
	digestOptIn   map[string]bool

	// audit log tracking
	auditLogs []struct {
		UserID    string
		Action    string
		Details   string
		IPAddress string
	}

	// billing fixtures
	processedEvents       map[string]string // eventID -> event_type
	usersByStripeCustomer map[string]*db.User
	activeChannelCount    map[string]int
	overageCost           map[string]float64
	subscriptionUpdates   []db.SubscriptionUpdate
	usageLogs             []struct {
		UserID                 string
		Count                  int
		PeriodStart, PeriodEnd time.Time
	}
	pauseAlphaCalls []struct {
		UserID    string
		KeepCount int
	}
	// billing_usage history fixture (implement-missing-product-features 3.2)
	billingUsage map[string][]db.BillingUsage
}

func (m *MockQuery) CreateUser(ctx context.Context, email, passwordHash string) (*db.User, error) {
	if m.users == nil {
		m.users = make(map[string]*db.User)
	}
	u := &db.User{
		ID:           "test-user-id",
		Email:        email,
		PasswordHash: &passwordHash,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	m.users[email] = u
	return u, nil
}

func (m *MockQuery) GetUserByEmail(ctx context.Context, email string) (*db.User, error) {
	if u, ok := m.users[email]; ok {
		return u, nil
	}
	return nil, nil // return nil error but nil user to simulate not found
}

func (m *MockQuery) GetUserByID(ctx context.Context, id string) (*db.User, error) {
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, nil
}

func (m *MockQuery) UpdateUserSubscription(ctx context.Context, userID, plan, stripeCustomerID string, quota int) error {
	for _, u := range m.users {
		if u.ID == userID {
			u.SubscriptionPlan = plan
			if stripeCustomerID != "" {
				u.StripeCustomerID = &stripeCustomerID
			}
			u.CommentQuotaMonthly = quota
			return nil
		}
	}
	return nil
}

func (m *MockQuery) ListChannelsForUser(ctx context.Context, userID string) ([]*db.PlatformChannel, error) {
	return nil, nil
}

func (m *MockQuery) ConnectChannelTx(ctx context.Context, userID, platform, platformChannelID, displayName, accessTokenEnc, refreshTokenEnc string, tokenExpiresAt *time.Time) error {
	return nil
}

func (m *MockQuery) PauseChannelTracking(ctx context.Context, userID, platformChannelID string) error {
	return nil
}

func (m *MockQuery) RemoveChannelTracking(ctx context.Context, userID, platformChannelID string) error {
	return nil
}

func (m *MockQuery) GetChannelsForSync(ctx context.Context, minutes int) ([]*db.PlatformChannel, error) {
	return nil, nil
}

func (m *MockQuery) UpdateSyncTime(ctx context.Context, id string) error {
	return nil
}

func (m *MockQuery) GetCommentFeed(ctx context.Context, userID string, limit, offset int) ([]*db.Comment, error) {
	return []*db.Comment{
		{
			ID:                "mock-comment-id",
			PlatformChannelID: "mock-channel",
			PlatformCommentID: "mock-platform-comment",
			Body:              "Mock comment",
			PostedAt:          time.Now(),
		},
	}, nil
}

func (m *MockQuery) StarComment(ctx context.Context, userID, commentID string) error {
	return nil
}

func (m *MockQuery) GetIngestionToken(ctx context.Context, platformChannelID string) (string, string, error) {
	return "", "", nil
}

func (m *MockQuery) PromoteNextIngestionToken(ctx context.Context, platformChannelID string) error {
	return nil
}

func (m *MockQuery) InsertComment(ctx context.Context, c *db.Comment) error {
	return nil
}

func (m *MockQuery) GetUnprocessedComments(ctx context.Context, limit int) ([]*db.Comment, error) {
	return nil, nil
}

func (m *MockQuery) GetUnprocessedCommentsByChannel(ctx context.Context, limit int, maxAttempts int) (map[string][]*db.Comment, error) {
	return nil, nil
}

func (m *MockQuery) UpdateCommentLLMData(ctx context.Context, id, category, topic, provider, model string, actionableInsights []byte, confidence float64) error {
	return nil
}

func (m *MockQuery) IncrementCommentsUsed(ctx context.Context, userID string, count int) error {
	return nil
}

func (m *MockQuery) GetPrimaryTracker(ctx context.Context, platformChannelID string) (*db.User, error) {
	return nil, db.ErrNoPrimaryTracker
}

func (m *MockQuery) EnqueueLLMJob(ctx context.Context, commentIDs []string, provider string) (string, error) {
	return "", nil
}

func (m *MockQuery) EnqueueLLMJobIfNotExists(ctx context.Context, commentIDs []string, provider string) (string, bool, error) {
	return "", true, nil
}

func (m *MockQuery) RecoverStaleLLMJobs(ctx context.Context, olderThan time.Duration) (int64, error) {
	return 0, nil
}

func (m *MockQuery) MarkLLMJobProcessing(ctx context.Context, jobID string) error {
	return nil
}

func (m *MockQuery) MarkLLMJobCompleted(ctx context.Context, jobID string) error {
	return nil
}

func (m *MockQuery) MarkLLMJobFailed(ctx context.Context, jobID string, msg string) error {
	return nil
}

func (m *MockQuery) InsertWeeklyDigest(ctx context.Context, digest *db.WeeklyDigest) error {
	return nil
}

func (m *MockQuery) GetUsersForDigest(ctx context.Context) ([]*db.User, error) {
	return nil, nil
}

func (m *MockQuery) GetRecentProcessedCommentsForUser(ctx context.Context, userID string, since time.Time) ([]*db.Comment, error) {
	return nil, nil
}

// --- weekly-digest mock stubs ----------------------------------------------

func (m *MockQuery) GetEligibleCommentsForUser(ctx context.Context, userID string, since, until time.Time) ([]*db.Comment, error) {
	return nil, nil
}
func (m *MockQuery) InsertWeeklyDigestPending(ctx context.Context, userID string, weekStart time.Time) (string, bool, error) {
	return "", false, nil
}
func (m *MockQuery) UpdateDigestContent(ctx context.Context, digestID string, contentJSON []byte, status string) error {
	return nil
}
func (m *MockQuery) UpdateDigestStatus(ctx context.Context, digestID, status string) error {
	return nil
}
func (m *MockQuery) GetPreviousWeekDigest(ctx context.Context, userID string, weekStart time.Time) (*db.WeeklyDigest, error) {
	return nil, db.ErrNotFound
}
func (m *MockQuery) GetLatestDigestForUser(ctx context.Context, userID string) (*db.WeeklyDigest, error) {
	if d, ok := m.latestDigests[userID]; ok && d != nil {
		return d, nil
	}
	return nil, db.ErrNotFound
}
func (m *MockQuery) ListDigests(ctx context.Context, userID string, limit, offset int) ([]*db.WeeklyDigest, error) {
	all := m.digestsByUser[userID]
	if offset >= len(all) {
		return []*db.WeeklyDigest{}, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], nil
}
func (m *MockQuery) GetStalePendingDigests(ctx context.Context, olderThan time.Time) ([]*db.WeeklyDigest, error) {
	return nil, nil
}
func (m *MockQuery) GetDeferredDigestsDue(ctx context.Context, now time.Time, limit int) ([]*db.WeeklyDigest, error) {
	return nil, nil
}
func (m *MockQuery) GetWeeklyDigestByID(ctx context.Context, digestID string) (*db.WeeklyDigest, error) {
	return nil, db.ErrNotFound
}
func (m *MockQuery) GetMissingDigestWeeks(ctx context.Context, userID string, weeks []time.Time) ([]time.Time, error) {
	return nil, nil
}
func (m *MockQuery) UpdateUserDigestOptIn(ctx context.Context, userID string, optIn bool) error {
	if m.digestOptIn == nil {
		m.digestOptIn = map[string]bool{}
	}
	m.digestOptIn[userID] = optIn
	return nil
}

func (m *MockQuery) MarkDigestEmailed(context.Context, string, string, time.Time) error {
	return nil
}

// --- Auth-extension stubs (no-ops sufficient for tests) ---------------------

func (m *MockQuery) CreateUserWithPassword(ctx context.Context, email, passwordHash string) (*db.User, error) {
	return m.CreateUser(ctx, email, passwordHash)
}

func (m *MockQuery) CreateUserGoogle(ctx context.Context, email, googleSub string) (*db.User, error) {
	if m.users == nil {
		m.users = make(map[string]*db.User)
	}
	now := time.Now()
	u := &db.User{
		ID:              "test-user-id",
		Email:           email,
		GoogleSub:       &googleSub,
		EmailVerifiedAt: &now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	m.users[email] = u
	return u, nil
}

func (m *MockQuery) GetUserByGoogleSub(ctx context.Context, sub string) (*db.User, error) {
	for _, u := range m.users {
		if u.GoogleSub != nil && *u.GoogleSub == sub {
			return u, nil
		}
	}
	return nil, nil
}

func (m *MockQuery) IncrementFailedLoginAttempts(ctx context.Context, userID string) (int, error) {
	for _, u := range m.users {
		if u.ID == userID {
			u.FailedLoginAttempts++
			return u.FailedLoginAttempts, nil
		}
	}
	return 0, nil
}

func (m *MockQuery) LockUser(ctx context.Context, userID string, until time.Time) error {
	for _, u := range m.users {
		if u.ID == userID {
			u.LockedUntil = &until
			return nil
		}
	}
	return nil
}

func (m *MockQuery) ResetLoginFailures(ctx context.Context, userID string) error {
	for _, u := range m.users {
		if u.ID == userID {
			u.FailedLoginAttempts = 0
			u.LockedUntil = nil
			return nil
		}
	}
	return nil
}

func (m *MockQuery) IncrementTokenVersion(ctx context.Context, userID string) error {
	for _, u := range m.users {
		if u.ID == userID {
			u.TokenVersion++
			return nil
		}
	}
	return nil
}

func (m *MockQuery) GetUserTokenVersion(ctx context.Context, userID string) (int, error) {
	for _, u := range m.users {
		if u.ID == userID {
			return u.TokenVersion, nil
		}
	}
	return 0, nil
}

func (m *MockQuery) SetEmailVerified(ctx context.Context, userID string) error {
	for _, u := range m.users {
		if u.ID == userID {
			now := time.Now()
			u.EmailVerifiedAt = &now
			return nil
		}
	}
	return nil
}

func (m *MockQuery) UpdatePasswordHash(ctx context.Context, userID, hash string) error {
	for _, u := range m.users {
		if u.ID == userID {
			u.PasswordHash = &hash
			return nil
		}
	}
	return nil
}

func (m *MockQuery) LinkGoogleSub(ctx context.Context, userID, sub string) error {
	for _, u := range m.users {
		if u.ID == userID {
			u.GoogleSub = &sub
			return nil
		}
	}
	return nil
}

func (m *MockQuery) DeleteStaleUnverifiedUsers(ctx context.Context, olderThan time.Duration) (int64, error) {
	return 0, nil
}

func (m *MockQuery) InsertEmailVerificationToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	if m.emailVerifTokens == nil {
		m.emailVerifTokens = make(map[string]struct {
			userID  string
			expires time.Time
			usedAt  *time.Time
		})
	}
	m.emailVerifTokens[tokenHash] = struct {
		userID  string
		expires time.Time
		usedAt  *time.Time
	}{userID: userID, expires: expiresAt}
	if m.verifResendCount == nil {
		m.verifResendCount = make(map[string]int)
	}
	m.verifResendCount[userID]++
	return nil
}
func (m *MockQuery) ConsumeEmailVerificationToken(ctx context.Context, tokenHash string) (string, error) {
	row, ok := m.emailVerifTokens[tokenHash]
	if !ok || row.usedAt != nil || time.Now().After(row.expires) {
		return "", nil
	}
	now := time.Now()
	row.usedAt = &now
	m.emailVerifTokens[tokenHash] = row
	return row.userID, nil
}
func (m *MockQuery) InvalidateEmailVerificationTokensForUser(ctx context.Context, userID string) error {
	for h, r := range m.emailVerifTokens {
		if r.userID == userID && r.usedAt == nil {
			now := time.Now()
			r.usedAt = &now
			m.emailVerifTokens[h] = r
		}
	}
	return nil
}
func (m *MockQuery) CountVerificationResendsInWindow(ctx context.Context, userID string, since time.Time) (int, error) {
	return m.verifResendCount[userID], nil
}
func (m *MockQuery) InsertEmailChangeToken(ctx context.Context, userID, pendingEmail, tokenHash string, expiresAt time.Time) error {
	if m.emailChangeTokens == nil {
		m.emailChangeTokens = make(map[string]struct {
			userID  string
			email   string
			expires time.Time
			usedAt  *time.Time
		})
	}
	m.emailChangeTokens[tokenHash] = struct {
		userID  string
		email   string
		expires time.Time
		usedAt  *time.Time
	}{userID: userID, email: pendingEmail, expires: expiresAt}
	if m.emailChangeResendCount == nil {
		m.emailChangeResendCount = make(map[string]int)
	}
	m.emailChangeResendCount[userID]++
	return nil
}
func (m *MockQuery) CountEmailChangeResendsInWindow(ctx context.Context, userID string, since time.Time) (int, error) {
	return m.emailChangeResendCount[userID], nil
}

// seedActiveEmailChange primes an in-flight email change WITHOUT counting it
// as a resend, so tests can isolate the resend cap from the initial request.
func (m *MockQuery) seedActiveEmailChange(userID, pendingEmail string) {
	if m.emailChangeTokens == nil {
		m.emailChangeTokens = make(map[string]struct {
			userID  string
			email   string
			expires time.Time
			usedAt  *time.Time
		})
	}
	m.emailChangeTokens["seed-"+pendingEmail] = struct {
		userID  string
		email   string
		expires time.Time
		usedAt  *time.Time
	}{userID: userID, email: pendingEmail, expires: time.Now().Add(time.Hour)}
}
func (m *MockQuery) ConsumeEmailChangeToken(ctx context.Context, tokenHash string) (string, string, error) {
	row, ok := m.emailChangeTokens[tokenHash]
	if !ok || row.usedAt != nil || time.Now().After(row.expires) {
		return "", "", nil
	}
	now := time.Now()
	row.usedAt = &now
	m.emailChangeTokens[tokenHash] = row
	return row.userID, row.email, nil
}
func (m *MockQuery) GetActiveEmailChange(ctx context.Context, userID string) (*db.EmailChange, error) {
	for _, r := range m.emailChangeTokens {
		if r.userID == userID && r.usedAt == nil {
			return &db.EmailChange{UserID: userID, PendingEmail: r.email, ExpiresAt: r.expires}, nil
		}
	}
	return nil, db.ErrNotFound
}
func (m *MockQuery) InvalidateEmailChangeTokensForUser(ctx context.Context, userID string) error {
	for h, r := range m.emailChangeTokens {
		if r.userID == userID && r.usedAt == nil {
			now := time.Now()
			r.usedAt = &now
			m.emailChangeTokens[h] = r
		}
	}
	return nil
}
func (m *MockQuery) ApplyEmailChange(ctx context.Context, userID, newEmail string) error {
	u := m.users[newEmail]
	if u != nil {
		return nil // simulated uniqueness collision
	}
	for _, existing := range m.users {
		if existing.ID == userID {
			delete(m.users, existing.Email)
			existing.Email = newEmail
			now := time.Now()
			existing.EmailVerifiedAt = &now
			existing.TokenVersion++
			m.users[newEmail] = existing
			return nil
		}
	}
	return db.ErrNotFound
}
func (m *MockQuery) ListWorkspaceMembers(ctx context.Context, workspaceID string) ([]*db.WorkspaceMemberWithEmail, error) {
	return nil, nil
}
func (m *MockQuery) InsertPasswordResetToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	if m.passwordResetTokens == nil {
		m.passwordResetTokens = make(map[string]struct {
			userID  string
			expires time.Time
			usedAt  *time.Time
		})
	}
	m.passwordResetTokens[tokenHash] = struct {
		userID  string
		expires time.Time
		usedAt  *time.Time
	}{userID: userID, expires: expiresAt}
	return nil
}
func (m *MockQuery) ConsumePasswordResetToken(ctx context.Context, tokenHash string) (string, error) {
	row, ok := m.passwordResetTokens[tokenHash]
	if !ok || row.usedAt != nil || time.Now().After(row.expires) {
		return "", nil
	}
	now := time.Now()
	row.usedAt = &now
	m.passwordResetTokens[tokenHash] = row
	return row.userID, nil
}
func (m *MockQuery) InvalidatePasswordResetTokensForUser(ctx context.Context, userID string) error {
	for h, r := range m.passwordResetTokens {
		if r.userID == userID && r.usedAt == nil {
			now := time.Now()
			r.usedAt = &now
			m.passwordResetTokens[h] = r
		}
	}
	return nil
}
func (m *MockQuery) CountForgotPasswordEmailsInWindow(ctx context.Context, emailAddr string, since time.Time) (int, error) {
	n := 0
	for _, s := range m.emailSends {
		if s.Template == email.TemplatePasswordReset && s.ToAddress == emailAddr && !s.At.Before(since) {
			n++
		}
	}
	return n, nil
}
func (m *MockQuery) InsertRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	if m.refreshTokens == nil {
		m.refreshTokens = make(map[string]struct {
			userID  string
			expires time.Time
		})
	}
	m.refreshTokens[tokenHash] = struct {
		userID  string
		expires time.Time
	}{userID: userID, expires: expiresAt}
	return nil
}
func (m *MockQuery) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (string, time.Time, error) {
	row, ok := m.refreshTokens[tokenHash]
	if !ok {
		return "", time.Time{}, nil
	}
	return row.userID, row.expires, nil
}
func (m *MockQuery) DeleteRefreshTokenByHash(ctx context.Context, tokenHash string) error {
	delete(m.refreshTokens, tokenHash)
	return nil
}
func (m *MockQuery) DeleteAllRefreshTokensForUser(ctx context.Context, userID string) error {
	for h, r := range m.refreshTokens {
		if r.userID == userID {
			delete(m.refreshTokens, h)
		}
	}
	return nil
}

// ----- channel-management stubs (no-op for unit tests) -----

func (m *MockQuery) InsertOAuthState(ctx context.Context, stateHash, userID string, expiresAt time.Time) error {
	return nil
}
func (m *MockQuery) ConsumeOAuthState(ctx context.Context, stateHash, userID string) (bool, error) {
	return true, nil
}
func (m *MockQuery) CleanupExpiredOAuthStates(ctx context.Context) (int64, error) { return 0, nil }
func (m *MockQuery) UpsertPlatformChannel(ctx context.Context, platform, platformChannelID, displayName string) (string, bool, error) {
	return platformChannelID, true, nil
}
func (m *MockQuery) GetChannelByPlatformID(ctx context.Context, platform, platformChannelID string) (*db.PlatformChannel, error) {
	return nil, db.ErrNotFound
}
func (m *MockQuery) ScheduleChannelDeletion(ctx context.Context, platformChannelID string, graceHours int) error {
	return nil
}
func (m *MockQuery) ClearChannelDeletion(ctx context.Context, platformChannelID string) error {
	return nil
}
func (m *MockQuery) GetActiveTrackerCount(ctx context.Context, platformChannelID string) (int, error) {
	return 0, nil
}
func (m *MockQuery) PurgeChannelData(ctx context.Context, platformChannelID string) error {
	return nil
}
func (m *MockQuery) CreateUserChannelTracking(ctx context.Context, userID, platformChannelID, encryptedAccess, encryptedRefresh string, expiresAt time.Time, backfillDays, tokenPriority int) (*db.UserChannelTracking, error) {
	return &db.UserChannelTracking{
		UserID:            userID,
		PlatformChannelID: platformChannelID,
		BackfillDays:      backfillDays,
		TokenPriority:     tokenPriority,
		TrackingStatus:    "active",
	}, nil
}
func (m *MockQuery) GetUserTracking(ctx context.Context, userID, platformChannelID string) (*db.UserChannelTracking, error) {
	return nil, db.ErrNotFound
}
func (m *MockQuery) ListUserChannels(ctx context.Context, userID string) ([]db.ChannelListItem, error) {
	return nil, nil
}
func (m *MockQuery) PauseTracking(ctx context.Context, userID, platformChannelID string) error {
	return nil
}
func (m *MockQuery) ResumeTracking(ctx context.Context, userID, platformChannelID string) error {
	return nil
}
func (m *MockQuery) DeleteTracking(ctx context.Context, userID, platformChannelID string) error {
	return nil
}
func (m *MockQuery) UpdateBackfillDays(ctx context.Context, userID, platformChannelID string, days int) error {
	return nil
}
func (m *MockQuery) GetMaxTokenPriority(ctx context.Context, platformChannelID string) (int, error) {
	return 0, nil
}
func (m *MockQuery) GetIngestionTokenRow(ctx context.Context, platformChannelID string) (*db.UserChannelTracking, bool, error) {
	return nil, false, nil
}
func (m *MockQuery) UpdateTokenAndExpiry(ctx context.Context, trackingID, encryptedAccess, encryptedRefresh string, expiresAt time.Time) error {
	return nil
}
func (m *MockQuery) GetExpiringTokens(ctx context.Context, withinDays int) ([]db.UserChannelTracking, error) {
	return nil, nil
}
func (m *MockQuery) MarkTokenExpiryNotified(ctx context.Context, trackingID string) error {
	return nil
}
func (m *MockQuery) GetCommentCountForChannel(ctx context.Context, platformChannelID string) (int, error) {
	return 0, nil
}

// ----- billing stubs (no-op-ish; tests can prime fixtures via fields) -----

func (m *MockQuery) IsStripeEventProcessed(ctx context.Context, eventID string) (bool, error) {
	if m.processedEvents == nil {
		return false, nil
	}
	_, ok := m.processedEvents[eventID]
	return ok, nil
}

func (m *MockQuery) InsertProcessedStripeEvent(ctx context.Context, tx *sql.Tx, eventID, eventType string) error {
	if m.processedEvents == nil {
		m.processedEvents = map[string]string{}
	}
	m.processedEvents[eventID] = eventType
	return nil
}

func (m *MockQuery) ApplySubscriptionUpdate(ctx context.Context, tx *sql.Tx, u db.SubscriptionUpdate) error {
	m.subscriptionUpdates = append(m.subscriptionUpdates, u)
	if u.UserID == "" {
		return nil
	}
	for _, usr := range m.users {
		if usr.ID != u.UserID {
			continue
		}
		if u.SubscriptionPlan != nil {
			usr.SubscriptionPlan = *u.SubscriptionPlan
		}
		if u.BillingIntervalClear {
			usr.BillingInterval = nil
		} else if u.BillingInterval != nil {
			v := *u.BillingInterval
			usr.BillingInterval = &v
		}
		if u.StripeCustomerID != nil {
			v := *u.StripeCustomerID
			usr.StripeCustomerID = &v
		}
		if u.CommentQuotaMonthly != nil {
			usr.CommentQuotaMonthly = *u.CommentQuotaMonthly
		}
		if u.CommentsUsedThisPeriod != nil {
			usr.CommentsUsedThisPeriod = *u.CommentsUsedThisPeriod
		}
		if u.SubscriptionStatus != nil {
			usr.SubscriptionStatus = *u.SubscriptionStatus
		}
		if u.CurrentPeriodEnd != nil {
			t := *u.CurrentPeriodEnd
			usr.CurrentPeriodEnd = &t
		}
		if u.QuotaResetAt != nil {
			t := *u.QuotaResetAt
			usr.QuotaResetAt = &t
		}
		if u.PastDueSinceClear {
			usr.PastDueSince = nil
		} else if u.PastDueSince != nil {
			t := *u.PastDueSince
			usr.PastDueSince = &t
		}
		if u.StripeSubscriptionIDClear {
			usr.StripeSubscriptionID = nil
		} else if u.StripeSubscriptionID != nil {
			v := *u.StripeSubscriptionID
			usr.StripeSubscriptionID = &v
		}
		break
	}
	return nil
}

func (m *MockQuery) GetUserByStripeCustomerID(ctx context.Context, customerID string) (*db.User, error) {
	if u, ok := m.usersByStripeCustomer[customerID]; ok {
		return u, nil
	}
	for _, u := range m.users {
		if u.StripeCustomerID != nil && *u.StripeCustomerID == customerID {
			return u, nil
		}
	}
	return nil, nil
}

func (m *MockQuery) GetActiveChannelCount(ctx context.Context, userID string) (int, error) {
	if m.activeChannelCount == nil {
		return 0, nil
	}
	return m.activeChannelCount[userID], nil
}

func (m *MockQuery) GetOverageCostForCurrentPeriod(ctx context.Context, userID string, periodStart, periodEnd time.Time) (float64, error) {
	if m.overageCost == nil {
		return 0, nil
	}
	return m.overageCost[userID], nil
}

func (m *MockQuery) InsertCommentUsageLog(ctx context.Context, tx *sql.Tx, userID string, count int, periodStart, periodEnd time.Time) error {
	m.usageLogs = append(m.usageLogs, struct {
		UserID                 string
		Count                  int
		PeriodStart, PeriodEnd time.Time
	}{userID, count, periodStart, periodEnd})
	return nil
}

func (m *MockQuery) PauseChannelsAlphabetically(ctx context.Context, tx *sql.Tx, userID string, keepCount int) error {
	m.pauseAlphaCalls = append(m.pauseAlphaCalls, struct {
		UserID    string
		KeepCount int
	}{userID, keepCount})
	return nil
}

func (m *MockQuery) ResetFreeTierQuotas(ctx context.Context) (int64, error) { return 0, nil }
func (m *MockQuery) ExpirePastDueGrace(ctx context.Context, graceHours int) (int64, error) {
	return 0, nil
}

func (m *MockQuery) GetBillingUsage(ctx context.Context, userID string, limit int) ([]db.BillingUsage, error) {
	all := m.billingUsage[userID]
	if limit > len(all) {
		limit = len(all)
	}
	return all[:limit], nil
}

// spec-gap-improvements additions
func (m *MockQuery) UpdateChannelCustomPrompt(ctx context.Context, userID, platformChannelID string, customPrompt *string) error {
	return nil
}
func (m *MockQuery) CreateWorkspace(ctx context.Context, name, createdBy string) (*db.Workspace, error) {
	return nil, nil
}
func (m *MockQuery) GetWorkspace(ctx context.Context, workspaceID string) (*db.Workspace, error) {
	return nil, nil
}
func (m *MockQuery) UpdateWorkspace(ctx context.Context, workspaceID, name string) error { return nil }
func (m *MockQuery) DeleteWorkspace(ctx context.Context, workspaceID string) error       { return nil }
func (m *MockQuery) ListWorkspacesForUser(ctx context.Context, userID string) ([]*db.WorkspaceWithMembers, error) {
	return nil, nil
}
func (m *MockQuery) GetWorkspaceMember(ctx context.Context, workspaceID, userID string) (*db.WorkspaceMember, error) {
	return nil, nil
}
func (m *MockQuery) AddWorkspaceMember(ctx context.Context, workspaceID, userID, role string) error {
	return nil
}
func (m *MockQuery) RemoveWorkspaceMember(ctx context.Context, workspaceID, userID string) error {
	return nil
}
func (m *MockQuery) UpdateWorkspaceMemberRole(ctx context.Context, workspaceID, userID, role string) error {
	return nil
}
func (m *MockQuery) CountWorkspaceMembers(ctx context.Context, workspaceID string) (int, error) {
	return 0, nil
}
func (m *MockQuery) CountWorkspaceAdmins(ctx context.Context, workspaceID string) (int, error) {
	return 0, nil
}
func (m *MockQuery) CreateWorkspaceInvite(ctx context.Context, workspaceID, email, role, tokenHash string, expiresAt time.Time) (*db.WorkspaceInvite, error) {
	return nil, nil
}
func (m *MockQuery) GetWorkspaceInviteByToken(ctx context.Context, tokenHash string) (*db.WorkspaceInvite, error) {
	return nil, nil
}
func (m *MockQuery) ConsumeWorkspaceInvite(ctx context.Context, inviteID string) error { return nil }
func (m *MockQuery) GetWorkspaceInviteByEmail(ctx context.Context, workspaceID, email string) (*db.WorkspaceInvite, error) {
	return nil, nil
}
func (m *MockQuery) SetUserActiveWorkspace(ctx context.Context, userID, workspaceID string) error {
	return nil
}
func (m *MockQuery) GetUserActiveWorkspaceID(ctx context.Context, userID string) (string, error) {
	return "", nil
}
func (m *MockQuery) IsWorkspaceOwner(ctx context.Context, workspaceID, userID string) (bool, error) {
	return false, nil
}
func (m *MockQuery) InsertAuditLog(ctx context.Context, userID, action, details, ipAddress string) error {
	m.auditLogs = append(m.auditLogs, struct {
		UserID    string
		Action    string
		Details   string
		IPAddress string
	}{userID, action, details, ipAddress})
	return nil
}
func (m *MockQuery) IncrementRateLimit(ctx context.Context, userKey string, windowStart time.Time, limit int) (int, error) {
	return 0, nil
}
func (m *MockQuery) GetRateLimitStatus(ctx context.Context, userKey string, windowStart time.Time, limit int) (*db.RateLimitStatus, error) {
	return nil, nil
}
func (m *MockQuery) CleanupExpiredRateLimitBuckets(ctx context.Context, before time.Time) (int64, error) {
	return 0, nil
}
func (m *MockQuery) GetChannelCustomPrompt(ctx context.Context, platformChannelID, userID string) (string, error) {
	return "", nil
}
func (m *MockQuery) GetCommentBodyByID(ctx context.Context, commentID string) (string, error) {
	return "", nil
}
func (m *MockQuery) SearchComments(ctx context.Context, userID, query string) ([]*db.Comment, error) {
	return nil, nil
}
func (m *MockQuery) BulkStarComments(ctx context.Context, userID string, commentIDs []string) (int, error) {
	return 0, nil
}
func (m *MockQuery) BulkUnstarComments(ctx context.Context, userID string, commentIDs []string) (int, error) {
	return 0, nil
}
func (m *MockQuery) GetCommentThread(ctx context.Context, userID, commentID string, maxDepth int) ([]*db.Comment, error) {
	return nil, nil
}
func (m *MockQuery) GrantChannel(ctx context.Context, workspaceID, channelID, grantedBy string) error {
	return nil
}
func (m *MockQuery) RevokeChannel(ctx context.Context, workspaceID, channelID string) error {
	return nil
}
func (m *MockQuery) ListGrantedChannels(ctx context.Context, workspaceID string) ([]string, error) {
	return nil, nil
}

func setupRouter(queries db.DBQuery) *chi.Mux {
	r := chi.NewRouter()

	authHandler := &api.AuthHandler{Queries: queries}
	channelHandler := &api.ChannelHandler{Queries: queries}
	commentHandler := &api.CommentHandler{Queries: queries}

	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", authHandler.Register)
		r.Post("/login", authHandler.Login)
	})

	r.Route("/api", func(r chi.Router) {
		r.Use(api.AuthMiddleware)

		r.Get("/channels", channelHandler.List)
		r.Get("/channels/connect/youtube", channelHandler.ConnectYouTube)
		r.Post("/channels/exchange/youtube", channelHandler.ExchangeYouTubeCode)
		r.Patch("/channels/{id}/pause", channelHandler.Pause)
		r.Get("/comments", commentHandler.Feed)
		r.Post("/comments/{id}/star", commentHandler.Star)
	})

	return r
}

func TestAuthRegisterLogin(t *testing.T) {
	queries := &MockQuery{}
	r := setupRouter(queries)

	// Test Registration
	body := map[string]string{
		"email":    "test@example.com",
		"password": "Password123",
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/auth/register", bytes.NewBuffer(bodyBytes))
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("Expected status 201 Created, got %v", rr.Code)
	}

	var registerResp map[string]string
	json.NewDecoder(rr.Body).Decode(&registerResp)
	if registerResp["access_token"] == "" {
		t.Errorf("Expected an access token in response")
	}

	// Test Login
	req, _ = http.NewRequest("POST", "/auth/login", bytes.NewBuffer(bodyBytes))
	rr = httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200 OK, got %v", rr.Code)
	}

	var loginResp map[string]string
	json.NewDecoder(rr.Body).Decode(&loginResp)
	if loginResp["access_token"] == "" {
		t.Errorf("Expected an access token in login response")
	}
}

func TestProtectedEndpoints(t *testing.T) {
	queries := &MockQuery{}
	r := setupRouter(queries)

	// 1. Unauthenticated request to /api/channels
	req, _ := http.NewRequest("GET", "/api/channels", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 Unauthorized, got %v", rr.Code)
	}

	// 2. Generate a valid token
	accessToken, _, _ := auth.GenerateTokens("test-user-id")

	// 3. Authenticated request to /api/channels
	req, _ = http.NewRequest("GET", "/api/channels", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %v", rr.Code)
	}
}

func TestChannelHandlers(t *testing.T) {
	queries := &MockQuery{}
	r := setupRouter(queries)
	accessToken, _, _ := auth.GenerateTokens("test-user-id")

	// Helper to make requests
	doRequest := func(method, path string, body []byte) *httptest.ResponseRecorder {
		var req *http.Request
		if body != nil {
			req, _ = http.NewRequest(method, path, bytes.NewBuffer(body))
		} else {
			req, _ = http.NewRequest(method, path, nil)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr
	}

	// 1. List
	rr := doRequest("GET", "/api/channels", nil)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200 OK for List, got %v", rr.Code)
	}

	// 2. Connect YouTube — new design returns 200 JSON with auth_url
	rr = doRequest("GET", "/api/channels/connect/youtube", nil)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200 OK with auth_url JSON, got %v", rr.Code)
	}

	// 3. Exchange YouTube Code (Mocked to fail oauth due to no real code, but let's test bad request)
	rr = doRequest("POST", "/api/channels/exchange/youtube", []byte(`{"code":""}`))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 Bad Request, got %v", rr.Code)
	}

	// 4. Pause — MockQuery's GetUserTracking returns ErrNotFound, so handler returns 404
	rr = doRequest("PATCH", "/api/channels/yt123/pause", nil)
	if rr.Code != http.StatusNotFound {
		t.Errorf("Expected 404 Not Found (mock has no tracking row), got %v", rr.Code)
	}
}

func TestCommentHandlers(t *testing.T) {
	queries := &MockQuery{}
	r := setupRouter(queries)
	accessToken, _, _ := auth.GenerateTokens("test-user-id")

	// Helper
	doRequest := func(method, path string) *httptest.ResponseRecorder {
		req, _ := http.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr
	}

	// 1. Feed
	rr := doRequest("GET", "/api/comments")
	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200 OK for Feed, got %v", rr.Code)
	}

	// 2. Star
	rr = doRequest("POST", "/api/comments/c123/star")
	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200 OK for Star, got %v", rr.Code)
	}
}
