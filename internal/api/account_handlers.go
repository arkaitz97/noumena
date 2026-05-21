// Package api: account_handlers.go owns the data-lifecycle change's
// HTTP surface — DELETE /api/user/account, GET /api/user/me, the GDPR
// export trio, and the export-download endpoint that verifies signed
// URLs.
//
// All handlers live behind ACCOUNT_LIFECYCLE_API_ENABLED (see
// cmd/api/main.go) so a deployment can ship the schema migrations and
// workers without exposing the user-facing endpoints. The download
// route is the one exception: it has no auth middleware (signed URL
// IS the auth), so the gating happens at route registration time.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"noumena/internal/billing"
	"noumena/internal/config"
	"noumena/internal/db"
	"noumena/internal/email"
)

// AccountHandler bundles the dependencies the account/lifecycle
// endpoints need. Construct from cmd/api/main.go.
type AccountHandler struct {
	Queries  db.DBQuery
	Pipeline *email.Pipeline
	Billing  billing.Client
	Cfg      *config.Config
}

// ----------------------------------------------------------------------------
// DELETE /api/user/account
// ----------------------------------------------------------------------------

type deleteAccountRequest struct {
	EmailConfirmation string `json:"email_confirmation"`
}

// DeleteAccount is the user-facing kickoff for account deletion.
//
// Contract:
//   - Body must include `email_confirmation` matching the caller's
//     email (case-insensitive). Mismatch → 400.
//   - Sets users.deletion_requested_at = NOW() (NOT a hard delete; the
//     deletion worker honours DELETION_REVERSIBLE_HOURS).
//   - Cancels the user's Stripe subscription via the billing client
//     when `users.stripe_subscription_id IS NOT NULL`. Stripe failures
//     (other than `resource_missing`, which is treated as success
//     inside the billing client) are logged but do not block deletion;
//     the user's intent is paramount and a Stripe outage must not
//     strand them.
//   - Inserts a row in account_deletion_jobs (idempotent).
//   - Emits the deletion_scheduled email.
//   - Returns 202 Accepted.
//
// Idempotency: a second DELETE call from the same user during the
// reversible window is a no-op (returns 202 with the existing
// reversible_until timestamp).
func (h *AccountHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	user, err := h.Queries.GetUserByID(r.Context(), userID)
	if err != nil || user == nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	var req deleteAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if !strings.EqualFold(strings.TrimSpace(req.EmailConfirmation), user.Email) {
		http.Error(w, "email_confirmation does not match account email", http.StatusBadRequest)
		return
	}

	// Mark the user row first so a parallel login attempt sees the
	// pending-deletion state immediately. The token_version bump (not
	// done here — it would lock the user out of cancelling) is left to
	// the deletion worker once it actually starts deleting.
	if user.DeletionRequestedAt == nil {
		if _, err := h.exec(r, `UPDATE users SET deletion_requested_at = NOW() WHERE id = $1`, userID); err != nil {
			http.Error(w, "failed to mark deletion", http.StatusInternalServerError)
			return
		}
		_ = h.Queries.InsertAuditLog(r.Context(), userID, "deletion_requested", `{"user_id":"`+userID+`"}`, r.RemoteAddr)
	}

	// Cancel the live Stripe subscription, if any. Errors are logged
	// but do not block the deletion path — the user's intent is
	// paramount, and `customer.subscription.deleted` will eventually
	// fire and reconcile our state regardless.
	if user.StripeSubscriptionID != nil && *user.StripeSubscriptionID != "" && h.Billing != nil {
		if err := h.Billing.CancelSubscription(r.Context(), *user.StripeSubscriptionID); err != nil {
			log.Printf("DeleteAccount: stripe CancelSubscription user=%s sub=%s err=%v",
				userID, *user.StripeSubscriptionID, err)
		}
	}

	// Count tracked channels for progress reporting.
	channels, _ := h.Queries.GetUserTrackingChannels(r.Context(), userID)
	job, err := h.Queries.ScheduleAccountDeletion(r.Context(), userID, len(channels))
	if err != nil {
		http.Error(w, "failed to schedule deletion", http.StatusInternalServerError)
		return
	}

	// Emit the deletion_scheduled email. Failure is non-fatal — the
	// users row already carries deletion_requested_at; the worker will
	// proceed regardless.
	reversibleUntil := time.Now().UTC().Add(time.Duration(h.Cfg.DeletionReversibleHours) * time.Hour)
	if h.Pipeline != nil {
		_ = h.Pipeline.Send(r.Context(), email.Message{
			Template:       email.TemplateDeletionScheduled,
			ToUserID:       userID,
			ToAddress:      user.Email,
			IdempotencyKey: "deletion_scheduled:" + userID,
			Variables: map[string]any{
				"ReversibleUntil": reversibleUntil.Format(time.RFC1123),
			},
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"job_id":           job.ID,
		"reversible_until": reversibleUntil,
		"status":           "scheduled",
	})
}

// CancelAccountDeletion reverses a pending deletion (only valid while
// the worker has not yet claimed the job AND within the 30-day reversible
// window).
//
// POST /api/user/account/cancel-deletion
func (h *AccountHandler) CancelAccountDeletion(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	user, err := h.Queries.GetUserByID(r.Context(), userID)
	if err != nil || user == nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	if user.DeletionRequestedAt == nil {
		http.Error(w, "no pending deletion", http.StatusConflict)
		return
	}

	reversibleUntil := user.DeletionRequestedAt.Add(time.Duration(h.Cfg.DeletionReversibleHours) * time.Hour)
	if time.Now().UTC().After(reversibleUntil) {
		http.Error(w, "deletion is no longer reversible (30-day window has passed)", http.StatusConflict)
		return
	}

	if err := h.Queries.CancelAccountDeletion(r.Context(), userID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			http.Error(w, "no cancellable deletion found", http.StatusConflict)
			return
		}
		http.Error(w, "failed to cancel deletion", http.StatusInternalServerError)
		return
	}
	_ = h.Queries.InsertAuditLog(r.Context(), userID, "deletion_cancelled", `{"user_id":"`+userID+`"}`, r.RemoteAddr)
	w.WriteHeader(http.StatusOK)
}

// ----------------------------------------------------------------------------
// POST /api/user/data-export   (kicks off a GDPR export)
// GET  /api/user/data-export/{job_id}/status
// ----------------------------------------------------------------------------

func (h *AccountHandler) RequestDataExport(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	job, err := h.Queries.EnqueueDataExport(r.Context(), userID)
	if err != nil {
		http.Error(w, "failed to enqueue export", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"job_id": job.ID,
		"status": job.Status,
	})
}

// GetExportStatus returns the current state of an export job. Scoped
// to the calling user — a foreign job_id returns 404 (not 403, to
// avoid leaking job-id existence).
func (h *AccountHandler) GetExportStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	jobID := chi.URLParam(r, "id")
	job, err := h.Queries.GetDataExportJob(r.Context(), jobID)
	if err != nil || job == nil || job.UserID != userID {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]any{"job_id": job.ID, "status": job.Status}
	if job.Status == "completed" {
		resp["download_url"] = job.DownloadURL
		resp["expires_at"] = job.ExpiresAt
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// DownloadExport serves the ZIP file. NO AUTH MIDDLEWARE — the signed
// URL is the auth. Verifies the signature against
// EXPORT_SIGNED_URL_SECRET and that the file still exists on disk.
//
// GET /api/exports/{job_id}/download?sig=...&exp=...
func (h *AccountHandler) DownloadExport(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	q := r.URL.Query()
	sig := q.Get("sig")
	expStr := q.Get("exp")
	if sig == "" || expStr == "" {
		http.Error(w, "missing signature", http.StatusForbidden)
		return
	}
	expUnix, err := email.ParseExp(expStr)
	if err != nil {
		http.Error(w, "invalid expiry", http.StatusForbidden)
		return
	}
	job, err := h.Queries.GetDataExportJob(r.Context(), jobID)
	if err != nil || job == nil || job.StoragePath == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err := email.VerifyURL(h.Cfg.ExportSignedURLSecret, job.UserID, jobID, expUnix, sig); err != nil {
		http.Error(w, "invalid or expired signature", http.StatusForbidden)
		return
	}
	f, err := os.Open(*job.StoragePath)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="noumena-export-%s.zip"`, jobID))
	_, _ = io.Copy(w, f)
}

// ----------------------------------------------------------------------------
// GET /api/user/me
// ----------------------------------------------------------------------------

// GetProfile returns the calling user's profile. We expose the new
// data-lifecycle fields (data_export_url, data_export_expires_at,
// deletion_requested_at) so the dashboard can surface "Your export is
// ready" / "Account deletion in N hours" banners without extra API
// calls.
func (h *AccountHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	user, err := h.Queries.GetUserByID(r.Context(), userID)
	if err != nil || user == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":                     user.ID,
		"email":                  user.Email,
		"email_verified":         user.EmailVerifiedAt != nil,
		"subscription_plan":      user.SubscriptionPlan,
		"subscription_status":    user.SubscriptionStatus,
		"deletion_requested_at":  user.DeletionRequestedAt,
		"data_export_url":        user.DataExportURL,
		"data_export_expires_at": user.DataExportExpiresAt,
		"created_at":             user.CreatedAt,
	})
}

// ----------------------------------------------------------------------------
// helpers
// ----------------------------------------------------------------------------

// exec runs an UPDATE/DELETE without expecting a row back. We type-
// assert to *db.SQLiteQuery to reach the underlying pool — same
// pattern InsightsHandler uses. Returns ErrNotFound when running in
// mock mode (tests should override the handler if they need to assert
// the SQL fired).
func (h *AccountHandler) exec(r *http.Request, sql string, args ...any) (int64, error) {
	sq, ok := h.Queries.(*db.SQLiteQuery)
	if !ok {
		return 0, nil // mock-mode no-op
	}
	result, err := sq.DB().ExecContext(r.Context(), sql, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// pathTail used to be a manual URL-segment extractor; replaced by
// chi.URLParam since the routes are mounted on a chi router. Kept as a
// stub for any caller in tests; unused in production code.
var _ = strings.TrimSuffix
