package api

import (
	"math"
	"net/http"
	"sort"
	"time"

	"noumena/internal/config"
	"noumena/internal/db"
)

type UserHandler struct {
	Queries db.DBQuery
	Cfg     *config.Config
}

func (h *UserHandler) GetRateLimitStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	userKey := "u:" + userID
	windowStart := time.Now().Truncate(time.Minute)
	limit := h.Cfg.RateLimitAuthMaxPerMin

	status, err := h.Queries.GetRateLimitStatus(r.Context(), userKey, windowStart, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get rate limit status")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"limit":     status.Limit,
		"remaining": status.Remaining,
		"reset_at":  status.ResetAt,
	})
}

// tokenExpiryChannel is one entry of GET /api/user/token-expiry: a channel
// owned by the current user whose OAuth token expires within the warning
// window.
type tokenExpiryChannel struct {
	ChannelID     string    `json:"channel_id"`
	Title         string    `json:"title"`
	ExpiresAt     time.Time `json:"expires_at"`
	DaysRemaining int       `json:"days_remaining"`
}

// GetTokenExpiry reports the current user's channels whose OAuth token
// expires within TOKEN_EXPIRY_WARNING_DAYS (default 7). The window matches
// the email notifier (StartTokenExpiryNotifier → GetExpiringTokens), but
// the token_expiry_notified_at guard is intentionally NOT applied: the
// in-app banner must stay visible once the email fired so the reauthorize
// action remains available until the token is refreshed.
func (h *UserHandler) GetTokenExpiry(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	warning := 7
	if h.Cfg != nil && h.Cfg.TokenExpiryWarningDays > 0 {
		warning = h.Cfg.TokenExpiryWarningDays
	}

	channels, err := h.Queries.ListUserChannels(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list channels")
		return
	}

	now := time.Now().UTC()
	until := now.AddDate(0, 0, warning)

	expiring := make([]tokenExpiryChannel, 0, len(channels))
	for _, ch := range channels {
		if ch.TrackingStatus != "active" && ch.TrackingStatus != "paused" {
			continue
		}
		if ch.TokenExpiresAt == nil {
			continue
		}
		expiresAt := ch.TokenExpiresAt.UTC()
		if !expiresAt.After(now) || expiresAt.After(until) {
			continue
		}
		expiring = append(expiring, tokenExpiryChannel{
			ChannelID:     ch.ID,
			Title:         ch.DisplayName,
			ExpiresAt:     expiresAt,
			DaysRemaining: int(math.Ceil(expiresAt.Sub(now).Hours() / 24)),
		})
	}
	sort.Slice(expiring, func(i, j int) bool {
		return expiring[i].ExpiresAt.Before(expiring[j].ExpiresAt)
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"channels":     expiring,
		"warning_days": warning,
	})
}
