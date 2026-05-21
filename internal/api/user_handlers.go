package api

import (
	"net/http"
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
