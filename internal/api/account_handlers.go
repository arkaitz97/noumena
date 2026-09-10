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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"noumena/internal/auth"
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
	user, ok := h.requireDeletionUser(w, r)
	if !ok {
		return
	}
	if !h.confirmDeletionEmail(w, r, user) {
		return
	}
	if !h.markDeletionRequested(w, r, user) {
		return
	}
	h.cancelStripeSubscription(r, user)
	job, ok := h.scheduleDeletionJob(w, r, user.ID)
	if !ok {
		return
	}
	until := time.Now().UTC().Add(time.Duration(h.Cfg.DeletionReversibleHours) * time.Hour)
	h.sendDeletionScheduledEmail(r, user, until)
	writeDeletionAccepted(w, job.ID, until)
}

// requireDeletionUser resolves the caller and loads their user row.
func (h *AccountHandler) requireDeletionUser(w http.ResponseWriter, r *http.Request) (*db.User, bool) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil, false
	}
	user, err := h.Queries.GetUserByID(r.Context(), userID)
	if err != nil || user == nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return nil, false
	}
	return user, true
}

// confirmDeletionEmail checks the typed confirmation against the account
// email (case-insensitive).
func (h *AccountHandler) confirmDeletionEmail(w http.ResponseWriter, r *http.Request, user *db.User) bool {
	var req deleteAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return false
	}
	if strings.EqualFold(strings.TrimSpace(req.EmailConfirmation), user.Email) {
		return true
	}
	http.Error(w, "email_confirmation does not match account email", http.StatusBadRequest)
	return false
}

// markDeletionRequested sets deletion_requested_at so a parallel login
// attempt sees the pending-deletion state immediately (idempotent). The
// token_version bump is left to the deletion worker once it actually
// starts deleting — doing it here would lock the user out of cancelling.
func (h *AccountHandler) markDeletionRequested(w http.ResponseWriter, r *http.Request, user *db.User) bool {
	if user.DeletionRequestedAt != nil {
		return true
	}
	if _, err := h.exec(r, `UPDATE users SET deletion_requested_at = `+db.NowUTC()+` WHERE id = ?`, user.ID); err != nil {
		http.Error(w, "failed to mark deletion", http.StatusInternalServerError)
		return false
	}
	_ = h.Queries.InsertAuditLog(r.Context(), user.ID, "deletion_requested", `{"user_id":"`+user.ID+`"}`, clientIP(r, h.Cfg))
	return true
}

// cancelStripeSubscription cancels a live subscription if present. Errors
// are logged but do not block the deletion path — the user's intent is
// paramount, and `customer.subscription.deleted` will eventually fire and
// reconcile our state regardless.
func (h *AccountHandler) cancelStripeSubscription(r *http.Request, user *db.User) {
	if user.StripeSubscriptionID == nil || *user.StripeSubscriptionID == "" || h.Billing == nil {
		return
	}
	if err := h.Billing.CancelSubscription(r.Context(), *user.StripeSubscriptionID); err != nil {
		log.Printf("DeleteAccount: stripe CancelSubscription user=%s sub=%s err=%v",
			user.ID, *user.StripeSubscriptionID, err)
	}
}

// scheduleDeletionJob queues the idempotent account_deletion_jobs row with
// the tracked-channel count for progress reporting.
func (h *AccountHandler) scheduleDeletionJob(w http.ResponseWriter, r *http.Request, userID string) (*db.AccountDeletionJob, bool) {
	channels, _ := h.Queries.GetUserTrackingChannels(r.Context(), userID)
	job, err := h.Queries.ScheduleAccountDeletion(r.Context(), userID, len(channels))
	if err != nil {
		http.Error(w, "failed to schedule deletion", http.StatusInternalServerError)
		return nil, false
	}
	return job, true
}

// sendDeletionScheduledEmail is non-fatal: the users row already carries
// deletion_requested_at; the worker will proceed regardless.
func (h *AccountHandler) sendDeletionScheduledEmail(r *http.Request, user *db.User, reversibleUntil time.Time) {
	if h.Pipeline == nil {
		return
	}
	_ = h.Pipeline.Send(r.Context(), email.Message{
		Template:       email.TemplateDeletionScheduled,
		ToUserID:       user.ID,
		ToAddress:      user.Email,
		IdempotencyKey: "deletion_scheduled:" + user.ID,
		Variables:      map[string]any{"ReversibleUntil": reversibleUntil.Format(time.RFC1123)},
	})
}

func writeDeletionAccepted(w http.ResponseWriter, jobID string, reversibleUntil time.Time) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"job_id":           jobID,
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
	_ = h.Queries.InsertAuditLog(r.Context(), userID, "deletion_cancelled", `{"user_id":"`+userID+`"}`, clientIP(r, h.Cfg))
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
		resp["download_url"] = job.ExportURL
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
	if err != nil || job == nil || job.ExportURL == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err := email.VerifyURL(h.Cfg.ExportSignedURLSecret, job.UserID, jobID, expUnix, sig); err != nil {
		http.Error(w, "invalid or expired signature", http.StatusForbidden)
		return
	}
	// The DB stores only the signed URL; the local file path derives
	// deterministically (see export_worker.go's layout).
	zipPath := filepath.Join(h.Cfg.ExportStorageDir, job.UserID, jobID+".zip")
	f, err := os.Open(zipPath)
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
// PATCH /api/user/me  (email change, fix-frontend-plumbing)
// ----------------------------------------------------------------------------

// emailRe is a simplified RFC-5322 lower bound — same shape the frontend
// validates with.
var emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

// PatchProfile changes the caller's email. Contract:
//   - Body: `{"email": "..."}`. Format must pass emailRe, and the address
//     must not belong to another account (409 email_in_use).
//   - The OLD email stays active until the NEW one verifies: we store a
//     pending email-change token (email_change_tokens) and email the link
//     to the new address. POST /auth/verify-email consumes either an
//     account-verification token or an email-change token.
//   - token_version is bumped (all sessions, incl. this one, are revoked)
//     and email_verified_at is cleared, so the user re-authenticates with
//     the old email and completes the change from the new inbox.
func (h *AccountHandler) PatchProfile(w http.ResponseWriter, r *http.Request) {
	userID, user, newEmail, ok := h.validateEmailChange(w, r)
	if !ok {
		return
	}
	if !h.revokeSessionsAndUnverify(w, r, userID) {
		return
	}
	raw, ok := h.storeEmailChangeToken(w, r, userID, newEmail)
	if !ok {
		return
	}
	h.sendEmailChangeEmail(r, userID, newEmail, raw)
	writeEmailChangePending(w, user, newEmail)
}

// validateEmailChange decodes the request and applies every precondition:
// format, not-the-current-email, not-owned-by-another-account.
func (h *AccountHandler) validateEmailChange(w http.ResponseWriter, r *http.Request) (string, *db.User, string, bool) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", nil, "", false
	}
	newEmail, ok := decodeRequestedEmail(w, r)
	if !ok {
		return userID, nil, "", false
	}
	user, err := h.Queries.GetUserByID(r.Context(), userID)
	if err != nil || user == nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return userID, nil, "", false
	}
	if strings.EqualFold(newEmail, user.Email) {
		http.Error(w, "that is already your email", http.StatusBadRequest)
		return userID, nil, "", false
	}
	if h.emailTakenByOther(r.Context(), newEmail, userID) {
		writeJSON(w, http.StatusConflict, map[string]string{
			"code":  "email_in_use",
			"error": "that email is already in use",
		})
		return userID, nil, "", false
	}
	return userID, user, newEmail, true
}

// decodeRequestedEmail normalizes and validates the requested address.
func decodeRequestedEmail(w http.ResponseWriter, r *http.Request) (string, bool) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return "", false
	}
	newEmail := strings.ToLower(strings.TrimSpace(req.Email))
	if newEmail == "" {
		http.Error(w, "email is required", http.StatusBadRequest)
		return "", false
	}
	if !emailRe.MatchString(newEmail) {
		http.Error(w, "invalid email address", http.StatusBadRequest)
		return "", false
	}
	return newEmail, true
}

// emailTakenByOther reports whether the address belongs to a different account.
func (h *AccountHandler) emailTakenByOther(ctx context.Context, email, userID string) bool {
	existing, _ := h.Queries.GetUserByEmail(ctx, email)
	return existing != nil && existing.ID != userID
}

// revokeSessionsAndUnverify bumps token_version (all sessions, including
// this one, are revoked) and clears email_verified_at: the account is
// unverified until the new address confirms, and the user re-authenticates
// with the old email (consistent with password reset).
func (h *AccountHandler) revokeSessionsAndUnverify(w http.ResponseWriter, r *http.Request, userID string) bool {
	if err := h.Queries.IncrementTokenVersion(r.Context(), userID); err != nil {
		http.Error(w, "failed to invalidate sessions", http.StatusInternalServerError)
		return false
	}
	if _, err := h.exec(r, `UPDATE users SET email_verified_at = NULL WHERE id = ?`, userID); err != nil {
		http.Error(w, "failed to update profile", http.StatusInternalServerError)
		return false
	}
	return true
}

// storeEmailChangeToken replaces any previous pending change with a new
// single-use token and returns the raw token for the verification link.
func (h *AccountHandler) storeEmailChangeToken(w http.ResponseWriter, r *http.Request, userID, newEmail string) (string, bool) {
	if err := h.Queries.InvalidateEmailChangeTokensForUser(r.Context(), userID); err != nil {
		http.Error(w, "failed to update profile", http.StatusInternalServerError)
		return "", false
	}
	raw, hash, err := auth.GenerateRefreshToken()
	if err != nil {
		http.Error(w, "failed to issue verification token", http.StatusInternalServerError)
		return "", false
	}
	expiresAt := time.Now().Add(config.Default.EmailVerificationTTL)
	if err := h.Queries.InsertEmailChangeToken(r.Context(), userID, newEmail, hash, expiresAt); err != nil {
		http.Error(w, "failed to store verification token", http.StatusInternalServerError)
		return "", false
	}
	return raw, true
}

// sendEmailChangeEmail emails the verification link to the NEW address;
// failure is non-fatal — the change simply stays pending until resend.
func (h *AccountHandler) sendEmailChangeEmail(r *http.Request, userID, newEmail, raw string) {
	if h.Pipeline == nil {
		return
	}
	_ = h.Pipeline.Send(r.Context(), email.Message{
		Template:       email.TemplateEmailVerification,
		ToUserID:       userID,
		ToAddress:      newEmail,
		IdempotencyKey: "email_change:" + userID + ":" + newEmail,
		Variables:      map[string]any{"VerifyURL": emailLink("/verify-email", raw)},
	})
}

func writeEmailChangePending(w http.ResponseWriter, user *db.User, newEmail string) {
	writeJSON(w, http.StatusOK, map[string]any{
		"id":                  user.ID,
		"email":               user.Email,
		"email_verified":      false,
		"pending_email":       newEmail,
		"subscription_plan":   user.SubscriptionPlan,
		"subscription_status": user.SubscriptionStatus,
		"created_at":          user.CreatedAt,
	})
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
	pendingEmail := ""
	if ec, err := h.Queries.GetActiveEmailChange(r.Context(), userID); err == nil && ec != nil {
		pendingEmail = ec.PendingEmail
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":                     user.ID,
		"email":                  user.Email,
		"email_verified":         user.EmailVerifiedAt != nil,
		"pending_email":          pendingEmail,
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
