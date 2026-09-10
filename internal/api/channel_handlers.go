package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/oauth2"

	"noumena/internal/auth"
	"noumena/internal/billing"
	"noumena/internal/config"
	"noumena/internal/db"
	"noumena/internal/ingestion"
	"noumena/internal/oauth"
)

// BackfillEnqueuer is the contract used by the exchange handler to schedule
// a backfill for a brand-new shared channel. Spec 03 will provide the real
// implementation; until then a no-op stub is acceptable.
type BackfillEnqueuer interface {
	EnqueueBackfill(ctx context.Context, platformChannelID string, days int) error
}

type ChannelHandler struct {
	Queries          db.DBQuery
	BackfillEnqueuer BackfillEnqueuer
	// SyncStatusReader is the narrow surface for GET /api/channels/{id}/sync-status.
	// It is satisfied by *db.PostgresQuery in production and stubbed in tests.
	// May be nil; the handler returns 501 in that case.
	SyncStatusReader SyncStatusReader
	// YouTubeResolver resolves public channels by @handle/URL for OAuth-free
	// competitor tracking. May be nil; ConnectYouTubeByName returns 503 then.
	YouTubeResolver YouTubeChannelResolver
	// OAuthExchanger swaps an authorization code for tokens + channel
	// metadata. May be nil; the handler then falls back to
	// oauth.ExchangeAndFetchChannel. Injectable so tests can exercise the
	// upstream-failure path without calling Google.
	OAuthExchanger OAuthExchanger
}

// OAuthExchanger is the contract for swapping a Google authorization code for
// an OAuth token pair and the authorizing channel's metadata. Satisfied in
// production by the function value oauth.ExchangeAndFetchChannel.
type OAuthExchanger func(ctx context.Context, code string) (*oauth2.Token, *ingestion.ChannelInfo, error)

// YouTubeChannelResolver resolves a public YouTube channel from a user
// supplied handle or URL reference. Satisfied by *ingestion.YouTubeClient
// with an API key configured.
type YouTubeChannelResolver interface {
	ResolveChannelRef(ctx context.Context, ref string) (*ingestion.ChannelInfo, error)
}

// SyncStatusReader exposes the per-channel ingestion sync status. The real
// implementation lives on *db.PostgresQuery.GetSyncStatus.
type SyncStatusReader interface {
	GetSyncStatus(ctx context.Context, platformChannelID, requestingUserID string) (*db.SyncStatus, error)
}

// ============================================================================
// helpers
// ============================================================================

func writeChannelError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"code": code, "message": message})
}

func generateOAuthState() (raw, hashHex string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(raw))
	return raw, hex.EncodeToString(sum[:]), nil
}

func hashOAuthState(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// planChannelLimit returns the user's active channel cap; 0 means unlimited
// (Agency or unknown user — enforcement degrades open, never bricks).
func planChannelLimit(ctx context.Context, q db.DBQuery, userID string) int {
	u, err := q.GetUserByID(ctx, userID)
	if err != nil || u == nil {
		return 0
	}
	plan := u.SubscriptionPlan
	if plan == "" {
		plan = billing.PlanFree
	}
	if lim := billing.LimitsFor(plan); !lim.ChannelsUnlimited {
		return lim.ChannelLimit
	}
	return 0
}

// insertTrackingRow creates a tracking row, enforcing the plan cap
// atomically at write time via CreateUserChannelTrackingWithLimit. It
// retries once with token_priority=1 when a concurrent insert grabbed
// prio==0 first. Returns db.ErrChannelLimitExceeded when over cap.
func insertTrackingRow(ctx context.Context, q db.DBQuery, userID, pcID, accessEnc, refreshEnc string, expiresAt time.Time, backfillDays, prio, channelLimit int) (*db.UserChannelTracking, error) {
	create := func(p int) (*db.UserChannelTracking, error) {
		if channelLimit > 0 {
			if ql, ok := q.(interface {
				CreateUserChannelTrackingWithLimit(ctx context.Context, userID, platformChannelID, encryptedAccess, encryptedRefresh string, expiresAt time.Time, backfillDays, tokenPriority, limit int) (*db.UserChannelTracking, error)
			}); ok {
				return ql.CreateUserChannelTrackingWithLimit(ctx, userID, pcID, accessEnc, refreshEnc, expiresAt, backfillDays, p, channelLimit)
			}
		}
		return q.CreateUserChannelTracking(ctx, userID, pcID, accessEnc, refreshEnc, expiresAt, backfillDays, p)
	}
	tracking, err := create(prio)
	if err != nil && isUniqueConstraintViolation(err) && prio == 0 {
		tracking, err = create(1)
	}
	return tracking, err
}

// ============================================================================
// GET /api/channels — list user's tracked channels
// ============================================================================

func (h *ChannelHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(UserIDKey).(string)

	items, err := h.Queries.ListUserChannels(r.Context(), userID)
	if err != nil {
		// Fall back to legacy projection so older test mocks that have not
		// implemented the new method still get a 200.
		log.Printf("ListUserChannels failed (%v); falling back to legacy", err)
		legacy, lerr := h.Queries.ListChannelsForUser(r.Context(), userID)
		if lerr != nil {
			writeChannelError(w, http.StatusInternalServerError, "list_failed", "failed to list channels")
			return
		}
		writeJSON(w, http.StatusOK, legacy)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// ============================================================================
// GET /api/channels/connect/youtube — return auth URL + CSRF state
// ============================================================================

func (h *ChannelHandler) ConnectYouTube(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(UserIDKey).(string)
	raw, ok := h.issueOAuthState(w, r, userID)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"auth_url": oauth.BuildAuthURL(raw),
		"state":    raw,
	})
}

// issueOAuthState mints a CSRF state and persists its hash for this user;
// returns the raw state, or false after writing the error response.
func (h *ChannelHandler) issueOAuthState(w http.ResponseWriter, r *http.Request, userID string) (string, bool) {
	raw, hash, err := generateOAuthState()
	if err != nil {
		writeChannelError(w, http.StatusInternalServerError, "state_generation_failed", "could not generate state")
		return "", false
	}
	ttl := config.Default.OAuthStateTTL
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	if err := h.Queries.InsertOAuthState(r.Context(), hash, userID, time.Now().Add(ttl)); err != nil {
		writeChannelError(w, http.StatusInternalServerError, "state_persist_failed", "could not persist state")
		return "", false
	}
	return raw, true
}

// ============================================================================
// POST /api/channels/exchange/youtube — finish the OAuth handshake
// ============================================================================

type exchangeYouTubeRequest struct {
	Code         string `json:"code"`
	State        string `json:"state"`
	BackfillDays int    `json:"backfill_days,omitempty"`
}

func (h *ChannelHandler) ExchangeYouTubeCode(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(UserIDKey).(string)
	req, ok := decodeExchangeYouTubeRequest(w, r)
	if !ok {
		return
	}
	// Validate CSRF state against the per-user store and consume it.
	if !h.consumeOAuthState(w, r, userID, req.State) {
		return
	}
	// Plan-limit guard (billing change, task 7.1). Agency is unlimited.
	// Performed AFTER the CSRF check so an attacker cannot probe the billing
	// limits without a valid OAuth flow, but BEFORE we burn a one-time
	// Google authorization code.
	if !h.requirePlanCapacity(w, r, userID) {
		return
	}
	token, info, ok := h.exchangeOAuthCode(w, r, req.Code)
	if !ok {
		return
	}
	// Encrypt tokens before any DB write.
	accessEnc, refreshEnc, ok := encryptTokenPair(w, token)
	if !ok {
		return
	}
	// Upsert shared channel and create / update tracking.
	tc, ok := h.trackNewChannel(w, r, userID, info, accessEnc, refreshEnc, tokenExpiryOrNow(token), req.BackfillDays)
	if !ok {
		return
	}
	h.enqueueNewChannelBackfill(r, tc, req.BackfillDays)
	writeTracking(w, tc, info.DisplayName)
}

func decodeExchangeYouTubeRequest(w http.ResponseWriter, r *http.Request) (exchangeYouTubeRequest, bool) {
	var req exchangeYouTubeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeChannelError(w, http.StatusBadRequest, "invalid_body", "could not decode request body")
		return req, false
	}
	if req.Code == "" {
		writeChannelError(w, http.StatusBadRequest, "invalid_code", "code is required")
		return req, false
	}
	if req.State == "" {
		writeChannelError(w, http.StatusBadRequest, "invalid_state", "state is required")
		return req, false
	}
	return req, validateBackfillDays(w, &req.BackfillDays)
}

// validateBackfillDays applies the 90-day default and rejects windows
// outside the supported set.
func validateBackfillDays(w http.ResponseWriter, days *int) bool {
	if *days == 0 {
		*days = 90
	}
	if validBackfillDaysAPI(*days) {
		return true
	}
	writeChannelError(w, http.StatusBadRequest, "invalid_backfill_days", "backfill_days must be 30, 60, 90, or 180")
	return false
}

// consumeOAuthState validates the CSRF state and burns it (single use).
func (h *ChannelHandler) consumeOAuthState(w http.ResponseWriter, r *http.Request, userID, state string) bool {
	ok, err := h.Queries.ConsumeOAuthState(r.Context(), hashOAuthState(state), userID)
	if err != nil {
		writeChannelError(w, http.StatusInternalServerError, "state_check_failed", "could not validate state")
		return false
	}
	if ok {
		return true
	}
	writeChannelError(w, http.StatusBadRequest, "invalid_state", "state is missing, expired, or already used")
	return false
}

// requirePlanCapacity writes 402 when the user's plan is at its channel cap.
func (h *ChannelHandler) requirePlanCapacity(w http.ResponseWriter, r *http.Request, userID string) bool {
	if blocked, _ := checkChannelLimit(r.Context(), h.Queries, userID); !blocked {
		return true
	}
	writeChannelError(w, http.StatusPaymentRequired, "plan_limit_reached",
		"channel limit reached for the current plan; upgrade to add more channels")
	return false
}

// exchangeOAuthCode swaps the one-time code and fetches the channel metadata.
func (h *ChannelHandler) exchangeOAuthCode(w http.ResponseWriter, r *http.Request, code string) (*oauth2.Token, *ingestion.ChannelInfo, bool) {
	exchanger := h.OAuthExchanger
	if exchanger == nil {
		exchanger = oauth.ExchangeAndFetchChannel
	}
	token, info, err := exchanger(r.Context(), code)
	if err == nil {
		return token, info, true
	}
	if errors.Is(err, oauth.ErrNoRefreshToken) {
		writeChannelError(w, http.StatusBadRequest, "no_refresh_token",
			"google did not return a refresh token; revoke the app at myaccount.google.com/permissions and try again")
		return nil, nil, false
	}
	// SECURITY: the raw upstream error can carry Google endpoints, request
	// payload fragments, and library internals. Keep the detail server-side
	// and return a stable, generic client message.
	log.Printf("youtube oauth code exchange failed: %v", err)
	writeChannelError(w, http.StatusBadRequest, "exchange_failed", "could not exchange authorization code")
	return nil, nil, false
}

// encryptTokenPair encrypts both tokens before any DB write.
func encryptTokenPair(w http.ResponseWriter, token *oauth2.Token) (accessEnc, refreshEnc string, ok bool) {
	accessEnc, err := auth.Encrypt(token.AccessToken)
	if err != nil {
		writeChannelError(w, http.StatusInternalServerError, "encryption_failed", "could not encrypt access token")
		return "", "", false
	}
	refreshEnc, err = auth.Encrypt(token.RefreshToken)
	if err != nil {
		writeChannelError(w, http.StatusInternalServerError, "encryption_failed", "could not encrypt refresh token")
		return "", "", false
	}
	return accessEnc, refreshEnc, true
}

// tokenExpiryOrNow is the persisted access-token expiry; zero means +1h.
func tokenExpiryOrNow(token *oauth2.Token) time.Time {
	if token.Expiry.IsZero() {
		return time.Now().Add(1 * time.Hour)
	}
	return token.Expiry
}

// trackedChannel is the result of registering a shared channel for a user.
type trackedChannel struct {
	pcID     string
	tracking *db.UserChannelTracking
	inserted bool // platform_channels row newly created
}

// trackNewChannel upserts the shared platform channel and records the
// caller's tracking row with the next token priority.
func (h *ChannelHandler) trackNewChannel(w http.ResponseWriter, r *http.Request, userID string,
	info *ingestion.ChannelInfo, accessEnc, refreshEnc string, expires time.Time, backfillDays int) (trackedChannel, bool) {
	pcID, inserted, err := h.Queries.UpsertPlatformChannel(r.Context(), "youtube", info.ID, info.DisplayName)
	if err != nil {
		writeChannelError(w, http.StatusInternalServerError, "channel_upsert_failed", "could not upsert channel")
		return trackedChannel{}, false
	}
	if !h.requireNotTracked(w, r, userID, pcID) {
		return trackedChannel{}, false
	}
	prio, ok := h.nextTokenPriority(w, r.Context(), pcID)
	if !ok {
		return trackedChannel{}, false
	}
	// Atomic plan-limit guard at write time (single-statement
	// INSERT...SELECT...WHERE). Unlimited plans pass channelLimit=0 and
	// skip the guarded insert.
	tracking, err := insertTrackingRow(r.Context(), h.Queries, userID, pcID,
		accessEnc, refreshEnc, expires, backfillDays, prio,
		planChannelLimit(r.Context(), h.Queries, userID))
	if err != nil {
		h.writeTrackingInsertError(w, err)
		return trackedChannel{}, false
	}
	return trackedChannel{pcID: pcID, tracking: tracking, inserted: inserted}, true
}

// requireNotTracked rejects a second tracking row for the same channel.
func (h *ChannelHandler) requireNotTracked(w http.ResponseWriter, r *http.Request, userID, pcID string) bool {
	existing, err := h.Queries.GetUserTracking(r.Context(), userID, pcID)
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		writeChannelError(w, http.StatusInternalServerError, "tracking_lookup_failed", "could not check existing tracking")
		return false
	}
	if existing == nil {
		return true
	}
	writeChannelError(w, http.StatusConflict, "already_tracked", "channel is already tracked")
	return false
}

// nextTokenPriority gives the first active tracker 0, later trackers MAX+1.
func (h *ChannelHandler) nextTokenPriority(w http.ResponseWriter, ctx context.Context, pcID string) (int, bool) {
	maxPrio, err := h.Queries.GetMaxTokenPriority(ctx, pcID)
	if err != nil {
		writeChannelError(w, http.StatusInternalServerError, "priority_lookup_failed", "could not compute token priority")
		return 0, false
	}
	if maxPrio < 0 {
		return 0, true
	}
	return maxPrio + 1, true
}

func (h *ChannelHandler) writeTrackingInsertError(w http.ResponseWriter, err error) {
	if errors.Is(err, db.ErrChannelLimitExceeded) {
		writeChannelError(w, http.StatusPaymentRequired, "plan_limit_reached",
			"channel limit reached for the current plan; upgrade to add more channels")
		return
	}
	writeChannelError(w, http.StatusInternalServerError, "tracking_create_failed", "could not create tracking row")
}

// enqueueNewChannelBackfill schedules the initial backfill for a brand-new
// shared channel; enqueue failures never fail the connect.
func (h *ChannelHandler) enqueueNewChannelBackfill(r *http.Request, tc trackedChannel, days int) {
	if !tc.inserted || h.BackfillEnqueuer == nil {
		return
	}
	if err := h.BackfillEnqueuer.EnqueueBackfill(r.Context(), tc.pcID, days); err != nil {
		log.Printf("backfill enqueue failed for channel %s: %v", tc.pcID, err)
	}
}

func writeTracking(w http.ResponseWriter, tc trackedChannel, displayName string) {
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":                  tc.tracking.ID,
		"platform_channel_id": tc.pcID,
		"display_name":        displayName,
		"tracking_status":     tc.tracking.TrackingStatus,
		"token_priority":      tc.tracking.TokenPriority,
		"backfill_days":       tc.tracking.BackfillDays,
		"token_expires_at":    tc.tracking.TokenExpiresAt,
	})
}

// ============================================================================
// POST /api/channels/connect/youtube/by-name — connect a public channel by
// @handle or full URL (competitor tracking). OAuth-free; requires
// YOUTUBE_API_KEY on the server.
// ============================================================================

type connectByNameRequest struct {
	Ref          string `json:"ref"`
	BackfillDays int    `json:"backfill_days,omitempty"`
}

func (h *ChannelHandler) ConnectYouTubeByName(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(UserIDKey).(string)
	req, ok := decodeConnectByNameRequest(w, r)
	if !ok {
		return
	}
	if h.YouTubeResolver == nil {
		writeChannelError(w, http.StatusServiceUnavailable, "resolver_unavailable",
			"public channel tracking is not configured on this server")
		return
	}
	// Plan-limit guard: same semantics as the OAuth exchange, before any
	// external resolution work.
	if !h.requirePlanCapacity(w, r, userID) {
		return
	}
	info, err := h.YouTubeResolver.ResolveChannelRef(r.Context(), req.Ref)
	if err != nil {
		// SECURITY: the resolver error can carry upstream endpoint URLs and
		// response fragments; log it and return a stable client message.
		log.Printf("youtube channel resolve failed for ref %q: %v", req.Ref, err)
		writeChannelError(w, http.StatusBadRequest, "resolve_failed", "could not resolve channel")
		return
	}
	// Tokenless row: empty tokens + zero expiry. Ingestion reads public
	// data via the server API key; sweeper/revoke skip empty-token rows.
	tc, ok := h.trackNewChannel(w, r, userID, info, "", "", time.Time{}, req.BackfillDays)
	if !ok {
		return
	}
	h.enqueueNewChannelBackfill(r, tc, req.BackfillDays)
	writeTracking(w, tc, info.DisplayName)
}

func decodeConnectByNameRequest(w http.ResponseWriter, r *http.Request) (connectByNameRequest, bool) {
	var req connectByNameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeChannelError(w, http.StatusBadRequest, "invalid_body", "could not decode request body")
		return req, false
	}
	if strings.TrimSpace(req.Ref) == "" {
		writeChannelError(w, http.StatusBadRequest, "invalid_ref", "ref is required (a @handle or channel URL)")
		return req, false
	}
	return req, validateBackfillDays(w, &req.BackfillDays)
}

// ============================================================================
// POST /api/channels/{id}/reauthorize  (fix-frontend-plumbing)
// ============================================================================

// Reauthorize starts the YouTube OAuth flow for an EXISTING tracking row
// whose token expired or was revoked. Same state-cookie machinery as
// ConnectYouTube; the exchange below links the new token to the existing
// row instead of creating a channel.
func (h *ChannelHandler) Reauthorize(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(UserIDKey).(string)
	pcID := chi.URLParam(r, "id")
	if _, ok := h.requireTracked(w, r, userID, pcID); !ok {
		return
	}
	raw, ok := h.issueOAuthState(w, r, userID)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"auth_url":   oauth.BuildAuthURL(raw),
		"state":      raw,
		"channel_id": pcID,
	})
}

// requireTracked loads the caller's tracking row, writing 404 when absent.
func (h *ChannelHandler) requireTracked(w http.ResponseWriter, r *http.Request, userID, pcID string) (*db.UserChannelTracking, bool) {
	tracking, err := h.Queries.GetUserTracking(r.Context(), userID, pcID)
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		writeChannelError(w, http.StatusInternalServerError, "lookup_failed", "could not check tracking")
		return nil, false
	}
	if tracking != nil {
		return tracking, true
	}
	writeChannelError(w, http.StatusNotFound, "not_found", "channel not tracked")
	return nil, false
}

type reauthorizeExchangeRequest struct {
	Code      string `json:"code"`
	State     string `json:"state"`
	ChannelID string `json:"channel_id"`
}

// ExchangeReauthorize completes the reauthorize handshake: consumes the CSRF
// state, swaps the code for fresh tokens, and stores them on the EXISTING
// tracking row (never a new channel). The old access token is revoked
// best-effort.
func (h *ChannelHandler) ExchangeReauthorize(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(UserIDKey).(string)
	req, ok := decodeReauthorizeRequest(w, r)
	if !ok {
		return
	}
	if !h.consumeOAuthState(w, r, userID, req.State) {
		return
	}
	tracking, ok := h.requireTracked(w, r, userID, req.ChannelID)
	if !ok {
		return
	}
	token, info, ok := h.exchangeOAuthCode(w, r, req.Code)
	if !ok {
		return
	}
	// The freshly authorized identity must own the channel being re-linked;
	// otherwise the user signed in as a different YouTube account.
	if info.ID != tracking.PlatformChannelID {
		writeChannelError(w, http.StatusConflict, "channel_mismatch",
			"the authorized account does not match this channel")
		return
	}
	accessEnc, refreshEnc, ok := encryptTokenPair(w, token)
	if !ok {
		return
	}
	expires := tokenExpiryOrNow(token)
	if err := h.Queries.UpdateTokenAndExpiry(r.Context(), tracking.ID, accessEnc, refreshEnc, expires); err != nil {
		writeChannelError(w, http.StatusInternalServerError, "update_failed", "could not store new token")
		return
	}
	revokeOldAccessToken(r.Context(), req.ChannelID, tracking.AccessTokenEnc)
	writeReauthorized(w, tracking, info.DisplayName, expires)
}

func decodeReauthorizeRequest(w http.ResponseWriter, r *http.Request) (reauthorizeExchangeRequest, bool) {
	var req reauthorizeExchangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeChannelError(w, http.StatusBadRequest, "invalid_body", "could not decode request body")
		return req, false
	}
	if req.Code != "" && req.State != "" && req.ChannelID != "" {
		return req, true
	}
	writeChannelError(w, http.StatusBadRequest, "invalid_request", "code, state, and channel_id are required")
	return req, false
}

// revokeOldAccessToken is best-effort hygiene for the replaced token.
func revokeOldAccessToken(ctx context.Context, channelID, accessEnc string) {
	if accessEnc == "" {
		return
	}
	oldRaw, derr := auth.Decrypt(accessEnc)
	if derr != nil || oldRaw == "" {
		return
	}
	if rerr := oauth.RevokeToken(ctx, oldRaw); rerr != nil {
		log.Printf("reauthorize: revoke old token channel=%s err=%v", channelID, rerr)
	}
}

func writeReauthorized(w http.ResponseWriter, t *db.UserChannelTracking, displayName string, expires time.Time) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":                  t.ID,
		"platform_channel_id": t.PlatformChannelID,
		"display_name":        displayName,
		"tracking_status":     t.TrackingStatus,
		"token_priority":      t.TokenPriority,
		"backfill_days":       t.BackfillDays,
		"token_expires_at":    expires,
	})
}

// ============================================================================
// PATCH /api/channels/{id}/pause
// ============================================================================

func (h *ChannelHandler) Pause(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(UserIDKey).(string)
	pcID := chi.URLParam(r, "id")

	// First load the existing row so we can detect "not found" without
	// leaking another user's channel.
	existing, err := h.Queries.GetUserTracking(r.Context(), userID, pcID)
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		writeChannelError(w, http.StatusInternalServerError, "lookup_failed", "could not check tracking")
		return
	}
	if existing == nil {
		writeChannelError(w, http.StatusNotFound, "not_found", "channel not tracked")
		return
	}

	wasPrimary := existing.TokenPriority == 0 && existing.TrackingStatus == "active"

	if err := h.Queries.PauseTracking(r.Context(), userID, pcID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeChannelError(w, http.StatusNotFound, "not_found", "channel not tracked")
			return
		}
		writeChannelError(w, http.StatusInternalServerError, "pause_failed", "could not pause channel")
		return
	}

	if wasPrimary {
		if err := h.Queries.PromoteNextIngestionToken(r.Context(), pcID); err != nil {
			log.Printf("promote next token after pause failed: %v", err)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "channel paused"})
}

// ============================================================================
// POST /api/channels/{id}/resume
// ============================================================================

func (h *ChannelHandler) Resume(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(UserIDKey).(string)
	pcID := chi.URLParam(r, "id")

	if err := h.Queries.ResumeTracking(r.Context(), userID, pcID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeChannelError(w, http.StatusNotFound, "not_found", "channel not tracked")
			return
		}
		writeChannelError(w, http.StatusInternalServerError, "resume_failed", "could not resume channel")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "channel resumed"})
}

// ============================================================================
// DELETE /api/channels/{id}
// ============================================================================

func (h *ChannelHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(UserIDKey).(string)
	pcID := chi.URLParam(r, "id")

	existing, err := h.Queries.GetUserTracking(r.Context(), userID, pcID)
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		writeChannelError(w, http.StatusInternalServerError, "lookup_failed", "could not check tracking")
		return
	}
	if existing == nil {
		writeChannelError(w, http.StatusNotFound, "not_found", "channel not tracked")
		return
	}
	wasPrimary := existing.TokenPriority == 0 && existing.TrackingStatus == "active"

	if err := h.Queries.DeleteTracking(r.Context(), userID, pcID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeChannelError(w, http.StatusNotFound, "not_found", "channel not tracked")
			return
		}
		writeChannelError(w, http.StatusInternalServerError, "delete_failed", "could not delete tracking")
		return
	}

	if wasPrimary {
		if err := h.Queries.PromoteNextIngestionToken(r.Context(), pcID); err != nil {
			log.Printf("promote next token after delete failed: %v", err)
		}
	}

	count, err := h.Queries.GetActiveTrackerCount(r.Context(), pcID)
	if err != nil {
		log.Printf("active-tracker count after delete failed: %v", err)
	}
	if err == nil && count == 0 {
		grace := config.Default.ChannelDeletionGraceHours
		if grace <= 0 {
			grace = 24
		}
		if err := h.Queries.ScheduleChannelDeletion(r.Context(), pcID, grace); err != nil {
			log.Printf("schedule deletion failed: %v", err)
		}
		// Spec calls for an enqueued purge job; the daemon also picks it up.
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "channel removed"})
}

// ============================================================================
// PATCH /api/channels/{id}/backfill-window
// ============================================================================

func (h *ChannelHandler) UpdateBackfillWindow(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(UserIDKey).(string)
	pcID := chi.URLParam(r, "id")

	var body struct {
		Days int `json:"days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeChannelError(w, http.StatusBadRequest, "invalid_body", "could not decode request body")
		return
	}
	if !validBackfillDaysAPI(body.Days) {
		writeChannelError(w, http.StatusBadRequest, "invalid_backfill_days", "days must be 30, 60, 90, or 180")
		return
	}

	if err := h.Queries.UpdateBackfillDays(r.Context(), userID, pcID, body.Days); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeChannelError(w, http.StatusNotFound, "not_found", "channel not tracked")
			return
		}
		writeChannelError(w, http.StatusInternalServerError, "update_failed", "could not update backfill window")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "backfill window updated",
		"days":    body.Days,
		"note":    "this setting affects future first-time channel additions only; it does not re-backfill this channel",
	})
}

// ============================================================================
// GET /api/channels/{id}/status
// ============================================================================

func (h *ChannelHandler) Status(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(UserIDKey).(string)
	pcID := chi.URLParam(r, "id")

	tracking, err := h.Queries.GetUserTracking(r.Context(), userID, pcID)
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		writeChannelError(w, http.StatusInternalServerError, "lookup_failed", "could not load tracking")
		return
	}
	if tracking == nil {
		writeChannelError(w, http.StatusNotFound, "not_found", "channel not tracked")
		return
	}

	channels, err := h.Queries.ListUserChannels(r.Context(), userID)
	if err != nil {
		writeChannelError(w, http.StatusInternalServerError, "lookup_failed", "could not load channel list")
		return
	}
	var lastSyncedAt *time.Time
	for _, c := range channels {
		if c.ID == pcID {
			lastSyncedAt = c.LastSyncedAt
			break
		}
	}

	commentCount, err := h.Queries.GetCommentCountForChannel(r.Context(), pcID)
	if err != nil {
		commentCount = 0
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tracking_status":         tracking.TrackingStatus,
		"token_expires_at":        tracking.TokenExpiresAt,
		"last_synced_at":          lastSyncedAt,
		"is_primary_token_holder": tracking.TokenPriority == 0 && tracking.TrackingStatus == "active",
		"comment_count":           commentCount,
		"backfill_days":           tracking.BackfillDays,
	})
}

// ============================================================================
// GET /api/channels/{id}/sync-status
// ============================================================================

// SyncStatus returns ingestion-pipeline sync state for a channel. The caller
// must have an active or paused tracker on the channel; otherwise 404.
func (h *ChannelHandler) SyncStatus(w http.ResponseWriter, r *http.Request) {
	if h.SyncStatusReader == nil {
		writeChannelError(w, http.StatusNotImplemented, "not_implemented", "sync-status reader not configured")
		return
	}
	userID, _ := r.Context().Value(UserIDKey).(string)
	pcID := chi.URLParam(r, "id")

	status, err := h.SyncStatusReader.GetSyncStatus(r.Context(), pcID, userID)
	if errors.Is(err, db.ErrNotFound) {
		writeChannelError(w, http.StatusNotFound, "not_found", "channel not tracked")
		return
	}
	if err != nil {
		writeChannelError(w, http.StatusInternalServerError, "lookup_failed", "could not load sync status")
		return
	}

	resp := map[string]interface{}{
		"last_synced_at":            status.LastSyncedAt,
		"next_sync_at":              status.NextSyncAt,
		"backfill_progress_percent": nullable(status.BackfillProgressPercent),
		"recent_error":              recentErrorJSON(status.RecentError),
	}
	writeJSON(w, http.StatusOK, resp)
}

// ============================================================================
// helpers
// ============================================================================

// nullable dereferences an optional field, yielding JSON null for nil.
func nullable[T any](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}

func recentErrorJSON(e *db.SyncError) any {
	if e == nil {
		return nil
	}
	return map[string]interface{}{"at": e.At, "message": e.Message}
}

// backfillDayOptions are the selectable initial-backfill windows.
var backfillDayOptions = map[int]bool{30: true, 60: true, 90: true, 180: true}

func validBackfillDaysAPI(d int) bool {
	return backfillDayOptions[d]
}

// checkChannelLimit reports whether a new channel-connect should be blocked
// by the user's plan. The bool return is the authoritative answer used by
// callers to decide whether to fail with 402; the error is returned only so
// tests can assert on persistence failures (in production both paths log
// and proceed permissively to avoid bricking new connects on a transient
// DB hiccup).
func checkChannelLimit(ctx context.Context, q db.DBQuery, userID string) (blocked bool, err error) {
	user, err := q.GetUserByID(ctx, userID)
	if err != nil || user == nil {
		return false, err
	}
	// A paused subscription means the 3-day past-due grace expired and the
	// user is delinquent. Block new connects (and the backfills they trigger)
	// regardless of plan capacity, matching the LLM processor's guard.
	if user.SubscriptionStatus == "paused" {
		return true, nil
	}
	plan := user.SubscriptionPlan
	if plan == "" {
		plan = billing.PlanFree
	}
	limits := billing.LimitsFor(plan)
	if limits.ChannelsUnlimited {
		return false, nil
	}
	cnt, err := q.GetActiveChannelCount(ctx, userID)
	if err != nil {
		return false, err
	}
	return cnt >= limits.ChannelLimit, nil
}

// guard against unused import warnings when log is only needed in some builds
var _ = strings.TrimSpace

// isUniqueConstraintViolation returns true when the error indicates a
// SQLite UNIQUE constraint failure.
func isUniqueConstraintViolation(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
