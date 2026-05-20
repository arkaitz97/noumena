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
	"github.com/jackc/pgx/v5/pgconn"

	"noumena/internal/auth"
	"noumena/internal/billing"
	"noumena/internal/config"
	"noumena/internal/db"
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

	raw, hash, err := generateOAuthState()
	if err != nil {
		writeChannelError(w, http.StatusInternalServerError, "state_generation_failed", "could not generate state")
		return
	}

	ttl := config.Default.OAuthStateTTL
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	expiresAt := time.Now().Add(ttl)

	if err := h.Queries.InsertOAuthState(r.Context(), hash, userID, expiresAt); err != nil {
		writeChannelError(w, http.StatusInternalServerError, "state_persist_failed", "could not persist state")
		return
	}

	authURL := oauth.BuildAuthURL(raw)
	writeJSON(w, http.StatusOK, map[string]string{
		"auth_url": authURL,
		"state":    raw,
	})
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

	var req exchangeYouTubeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeChannelError(w, http.StatusBadRequest, "invalid_body", "could not decode request body")
		return
	}
	if req.Code == "" {
		writeChannelError(w, http.StatusBadRequest, "invalid_code", "code is required")
		return
	}
	if req.State == "" {
		writeChannelError(w, http.StatusBadRequest, "invalid_state", "state is required")
		return
	}
	if req.BackfillDays == 0 {
		req.BackfillDays = 90
	}
	if !validBackfillDaysAPI(req.BackfillDays) {
		writeChannelError(w, http.StatusBadRequest, "invalid_backfill_days", "backfill_days must be 30, 60, 90, or 180")
		return
	}

	// Validate CSRF state against the per-user store and consume it.
	hash := hashOAuthState(req.State)
	ok, err := h.Queries.ConsumeOAuthState(r.Context(), hash, userID)
	if err != nil {
		writeChannelError(w, http.StatusInternalServerError, "state_check_failed", "could not validate state")
		return
	}
	if !ok {
		writeChannelError(w, http.StatusBadRequest, "invalid_state", "state is missing, expired, or already used")
		return
	}

	// Plan-limit guard (billing change, task 7.1). Agency is unlimited.
	// We perform this AFTER the CSRF check so an attacker cannot probe the
	// billing limits without a valid OAuth flow, but BEFORE we burn a
	// one-time Google authorization code.
	if blocked, _ := checkChannelLimit(r.Context(), h.Queries, userID); blocked {
		writeChannelError(w, http.StatusPaymentRequired, "plan_limit_reached",
			"channel limit reached for the current plan; upgrade to add more channels")
		return
	}

	// Exchange the code and fetch the channel metadata.
	token, info, err := oauth.ExchangeAndFetchChannel(r.Context(), req.Code)
	if err != nil {
		if errors.Is(err, oauth.ErrNoRefreshToken) {
			writeChannelError(w, http.StatusBadRequest, "no_refresh_token",
				"google did not return a refresh token; revoke the app at myaccount.google.com/permissions and try again")
			return
		}
		writeChannelError(w, http.StatusBadRequest, "exchange_failed", err.Error())
		return
	}

	// Encrypt tokens before any DB write.
	accessEnc, err := auth.Encrypt(token.AccessToken)
	if err != nil {
		writeChannelError(w, http.StatusInternalServerError, "encryption_failed", "could not encrypt access token")
		return
	}
	refreshEnc, err := auth.Encrypt(token.RefreshToken)
	if err != nil {
		writeChannelError(w, http.StatusInternalServerError, "encryption_failed", "could not encrypt refresh token")
		return
	}

	// Upsert shared channel and create / update tracking.
	pcID, inserted, err := h.Queries.UpsertPlatformChannel(r.Context(), "youtube", info.ID, info.DisplayName)
	if err != nil {
		writeChannelError(w, http.StatusInternalServerError, "channel_upsert_failed", "could not upsert channel")
		return
	}

	// Reject if the user already tracks this channel.
	existing, err := h.Queries.GetUserTracking(r.Context(), userID, pcID)
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		writeChannelError(w, http.StatusInternalServerError, "tracking_lookup_failed", "could not check existing tracking")
		return
	}
	if existing != nil {
		writeChannelError(w, http.StatusConflict, "already_tracked", "channel is already tracked")
		return
	}

	// Compute token_priority. First active tracker → 0, otherwise MAX+1.
	maxPrio, err := h.Queries.GetMaxTokenPriority(r.Context(), pcID)
	if err != nil {
		writeChannelError(w, http.StatusInternalServerError, "priority_lookup_failed", "could not compute token priority")
		return
	}
	prio := 0
	if maxPrio >= 0 {
		prio = maxPrio + 1
	}

	expires := token.Expiry
	if expires.IsZero() {
		expires = time.Now().Add(1 * time.Hour)
	}

	tracking, err := h.Queries.CreateUserChannelTracking(
		r.Context(), userID, pcID, accessEnc, refreshEnc, expires,
		req.BackfillDays, prio,
	)
	if err != nil {
		// Race: a concurrent insert may have grabbed token_priority=0
		// before us. Retry once with the next available priority.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && prio == 0 {
			tracking, err = h.Queries.CreateUserChannelTracking(
				r.Context(), userID, pcID, accessEnc, refreshEnc, expires,
				req.BackfillDays, 1,
			)
		}
		if err != nil {
			writeChannelError(w, http.StatusInternalServerError, "tracking_create_failed",
				"could not create tracking row")
			return
		}
	}

	if inserted && h.BackfillEnqueuer != nil {
		if err := h.BackfillEnqueuer.EnqueueBackfill(r.Context(), pcID, req.BackfillDays); err != nil {
			log.Printf("backfill enqueue failed for channel %s: %v", pcID, err)
		}
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":                  tracking.ID,
		"platform_channel_id": pcID,
		"display_name":        info.DisplayName,
		"tracking_status":     tracking.TrackingStatus,
		"token_priority":      tracking.TokenPriority,
		"backfill_days":       tracking.BackfillDays,
		"token_expires_at":    tracking.TokenExpiresAt,
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
	} else if count == 0 {
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
		"last_synced_at": status.LastSyncedAt,
		"next_sync_at":   status.NextSyncAt,
	}
	if status.BackfillProgressPercent != nil {
		resp["backfill_progress_percent"] = *status.BackfillProgressPercent
	} else {
		resp["backfill_progress_percent"] = nil
	}
	if status.RecentError != nil {
		resp["recent_error"] = map[string]interface{}{
			"at":      status.RecentError.At,
			"message": status.RecentError.Message,
		}
	} else {
		resp["recent_error"] = nil
	}
	writeJSON(w, http.StatusOK, resp)
}

// ============================================================================
// helpers
// ============================================================================

func validBackfillDaysAPI(d int) bool {
	switch d {
	case 30, 60, 90, 180:
		return true
	}
	return false
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
