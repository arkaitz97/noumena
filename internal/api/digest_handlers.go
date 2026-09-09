// Package api: digest_handlers.go owns the read-only weekly-digest API
// surface introduced by the weekly-digest change:
//
//	GET   /api/digests           — paginated list, newest first
//	GET   /api/digests/latest    — most-recent digest or 404
//	PATCH /api/user/preferences  — opt-in toggle for digest emails
//
// Authentication + email-verification middleware are applied at the router
// level (see cmd/api/main.go); these handlers can assume UserIDKey is set.
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"noumena/internal/config"
	"noumena/internal/db"
	"noumena/internal/email"
)

// DigestHandler bundles the dependencies for the digest endpoints. The
// pointer-to-DBQuery allows the same MockQuery used by other handler tests
// to drive these handlers without a live Postgres pool.
type DigestHandler struct {
	Queries db.DBQuery
	Cfg     *config.Config
}

const (
	digestListDefaultLimit = 10
	digestListMaxLimit     = 50
)

// List handles GET /api/digests. Spec 06: paginated list, limit clamped to
// [1, 50], default 10, sorted by week_start_date DESC.
func (h *DigestHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	limit := digestListDefaultLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "invalid_limit")
			return
		}
		limit = n
	}
	if limit > digestListMaxLimit {
		limit = digestListMaxLimit
	}

	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "invalid_offset")
			return
		}
		offset = n
	}

	digests, err := h.Queries.ListDigests(r.Context(), userID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_failed")
		return
	}
	if digests == nil {
		digests = []*db.WeeklyDigest{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"digests": digests,
		"limit":   limit,
		"offset":  offset,
	})
}

// Latest handles GET /api/digests/latest. Spec 06: 200 with body OR
// 404 with `{"message": "no digest yet"}`.
func (h *DigestHandler) Latest(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	d, err := h.Queries.GetLatestDigestForUser(r.Context(), userID)
	if errors.Is(err, db.ErrNotFound) {
		// Per spec, the body shape is {message: "no digest yet"} — distinct
		// from the writeError {error: ...} convention used elsewhere.
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "no digest yet"})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "latest_failed")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// updatePreferencesRequest is the documented PATCH body. Pointer-bool so
// we can distinguish "field absent" from "field set to false". Future
// preferences can be added as additional optional pointer fields without
// breaking existing clients.
type updatePreferencesRequest struct {
	DigestEmailOptIn *bool `json:"digest_email_opt_in"`
}

// UpdatePreferences handles PATCH /api/user/preferences. Spec 06: validates
// body shape, persists the new value, returns 200 on success.
func (h *DigestHandler) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req updatePreferencesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body")
		return
	}
	if req.DigestEmailOptIn == nil {
		// Empty PATCH bodies are malformed; clients must include at least
		// one supported field. Returning 400 surfaces the typo before it
		// silently no-ops in production.
		writeError(w, http.StatusBadRequest, "no_supported_fields")
		return
	}
	if err := h.Queries.UpdateUserDigestOptIn(r.Context(), userID, *req.DigestEmailOptIn); err != nil {
		writeError(w, http.StatusInternalServerError, "update_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"digest_email_opt_in": *req.DigestEmailOptIn,
	})
}

// Unsubscribe handles GET /unsubscribe?user_id=..&job=..&sig=..&exp=..
// NO AUTH MIDDLEWARE — the HMAC signature (email/signer.go, same secret
// as export download links) IS the auth. Any verification failure is a
// 403 without leaking which leg failed. On success the user's digest
// email opt-in flips off (fix-email-delivery 5.x).
func (h *DigestHandler) Unsubscribe(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	userID := q.Get("user_id")
	jobID := q.Get("job")
	sig := q.Get("sig")
	expStr := q.Get("exp")
	if userID == "" || jobID == "" || sig == "" || expStr == "" {
		http.Error(w, "missing signature", http.StatusForbidden)
		return
	}
	expUnix, err := email.ParseExp(expStr)
	if err != nil {
		http.Error(w, "invalid expiry", http.StatusForbidden)
		return
	}
	secret := ""
	if h.Cfg != nil {
		secret = h.Cfg.ExportSignedURLSecret
	}
	if err := email.VerifyURL(secret, userID, jobID, expUnix, sig); err != nil {
		http.Error(w, "invalid or expired signature", http.StatusForbidden)
		return
	}
	if err := h.Queries.UpdateUserDigestOptIn(r.Context(), userID, false); err != nil {
		writeError(w, http.StatusInternalServerError, "update_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":              "ok",
		"digest_email_opt_in": false,
	})
}
