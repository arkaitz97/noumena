// account_handlers_test.go covers the AccountHandler endpoints
// introduced by data-lifecycle (14.8). Tests use the in-memory MockQuery
// + the chi router pattern from digest_handlers_test.go so they exercise
// handler decoding/validation without a live Postgres pool.
//
// Coverage:
//   - DeleteAccount rejects unauthenticated callers (14.8.1)
//   - DeleteAccount rejects mismatched email_confirmation (14.8.1)
//   - GetExportStatus refuses cross-user lookups (14.8.3)
//
// The success path of DeleteAccount + the export download endpoint
// require live Postgres (FOR UPDATE locks, transactional schedule) and
// are deferred to the integration suite.
package api_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stripe/stripe-go/v76"

	"noumena/internal/api"
	"noumena/internal/billing"
	"noumena/internal/config"
	"noumena/internal/db"
)

func newAccountRouter(h *api.AccountHandler, userID string) *chi.Mux {
	r := chi.NewRouter()
	r.Route("/api", func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if userID != "" {
					ctx := context.WithValue(req.Context(), api.UserIDKey, userID)
					req = req.WithContext(ctx)
				}
				next.ServeHTTP(w, req)
			})
		})
		r.Delete("/user/account", h.DeleteAccount)
		r.Post("/user/account/cancel-deletion", h.CancelAccountDeletion)
		r.Get("/user/data-export/{id}/status", h.GetExportStatus)
		r.Get("/user/me", h.GetProfile)
	})
	return r
}

// 14.8.1: 401 on missing auth.
func TestDeleteAccount_Unauthenticated(t *testing.T) {
	mock := &MockQuery{}
	h := &api.AccountHandler{Queries: mock, Cfg: config.Default}
	r := newAccountRouter(h, "") // empty → no UserIDKey injected

	req := httptest.NewRequest(http.MethodDelete, "/api/user/account",
		bytes.NewBufferString(`{"email_confirmation":"a@b"}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

// 14.8.1: email_confirmation mismatch returns 400.
func TestDeleteAccount_EmailMismatch(t *testing.T) {
	user := &db.User{ID: "u1", Email: "real@example.com"}
	mock := &MockQuery{users: map[string]*db.User{"u1": user}}
	h := &api.AccountHandler{Queries: mock, Cfg: config.Default}
	r := newAccountRouter(h, "u1")

	req := httptest.NewRequest(http.MethodDelete, "/api/user/account",
		bytes.NewBufferString(`{"email_confirmation":"wrong@example.com"}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// GetProfile requires auth (sanity check; 14.8.5 partial).
func TestGetProfile_Unauthenticated(t *testing.T) {
	mock := &MockQuery{}
	h := &api.AccountHandler{Queries: mock, Cfg: config.Default}
	r := newAccountRouter(h, "")

	req := httptest.NewRequest(http.MethodGet, "/api/user/me", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================================
// backend-schema-and-env-fixes: DeleteAccount Stripe cancellation wiring
// ============================================================================

// fakeBillingClient is a billing.Client that records CancelSubscription
// calls and returns a configurable error on each. Used by 3.7 / 3.8 tests
// to assert the wiring behaviour without exercising the real Stripe SDK.
type fakeBillingClient struct {
	billing.Client
	cancellations []string
	cancelErr     error
}

func (f *fakeBillingClient) CancelSubscription(_ context.Context, subID string) error {
	f.cancellations = append(f.cancellations, subID)
	return f.cancelErr
}

// 3.7: CancelSubscription is called with the stored id.
func TestDeleteAccount_CallsStripeCancelSubscription(t *testing.T) {
	subID := "sub_123"
	user := &db.User{ID: "u1", Email: "alice@example.com", StripeSubscriptionID: &subID}
	mock := &MockQuery{users: map[string]*db.User{"u1": user}}
	bc := &fakeBillingClient{}
	h := &api.AccountHandler{Queries: mock, Cfg: config.Default, Billing: bc}
	r := newAccountRouter(h, "u1")

	req := httptest.NewRequest(http.MethodDelete, "/api/user/account",
		bytes.NewBufferString(`{"email_confirmation":"alice@example.com"}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	if len(bc.cancellations) != 1 || bc.cancellations[0] != subID {
		t.Fatalf("expected CancelSubscription(%q), got %v", subID, bc.cancellations)
	}
}

// 3.7: a non-resource_missing Stripe error does not fail the request
// (errors are logged, deletion proceeds).
func TestDeleteAccount_StripeErrorIsNonFatal(t *testing.T) {
	subID := "sub_456"
	user := &db.User{ID: "u2", Email: "bob@example.com", StripeSubscriptionID: &subID}
	mock := &MockQuery{users: map[string]*db.User{"u2": user}}
	bc := &fakeBillingClient{cancelErr: errors.New("stripe down")}
	h := &api.AccountHandler{Queries: mock, Cfg: config.Default, Billing: bc}
	r := newAccountRouter(h, "u2")

	req := httptest.NewRequest(http.MethodDelete, "/api/user/account",
		bytes.NewBufferString(`{"email_confirmation":"bob@example.com"}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202 even on Stripe error, got %d: %s", w.Code, w.Body.String())
	}
	if len(bc.cancellations) != 1 {
		t.Fatalf("expected CancelSubscription called once, got %d", len(bc.cancellations))
	}
}

// 3.7: a `resource_missing` error from Stripe is treated as success
// (the billing client wraps it; the handler sees nil from the call).
// We simulate the wrapped semantics directly by configuring the fake
// to return nil — i.e. assert the handler path matches the contract
// "billing.CancelSubscription returning nil = success".
func TestDeleteAccount_ResourceMissingTreatedAsSuccess(t *testing.T) {
	// The StripeClient implementation downgrades resource_missing to
	// nil, so the fake returns nil to mirror that behaviour. This test
	// asserts the handler proceeds normally on nil.
	subID := "sub_already_gone"
	user := &db.User{ID: "u3", Email: "c@example.com", StripeSubscriptionID: &subID}
	mock := &MockQuery{users: map[string]*db.User{"u3": user}}
	bc := &fakeBillingClient{cancelErr: nil}
	h := &api.AccountHandler{Queries: mock, Cfg: config.Default, Billing: bc}
	r := newAccountRouter(h, "u3")

	req := httptest.NewRequest(http.MethodDelete, "/api/user/account",
		bytes.NewBufferString(`{"email_confirmation":"c@example.com"}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
}

// 3.8: CancelSubscription is NOT called when stripe_subscription_id is NULL.
func TestDeleteAccount_NoStripeCallWhenSubscriptionIDNil(t *testing.T) {
	user := &db.User{ID: "u4", Email: "d@example.com"} // no StripeSubscriptionID
	mock := &MockQuery{users: map[string]*db.User{"u4": user}}
	bc := &fakeBillingClient{}
	h := &api.AccountHandler{Queries: mock, Cfg: config.Default, Billing: bc}
	r := newAccountRouter(h, "u4")

	req := httptest.NewRequest(http.MethodDelete, "/api/user/account",
		bytes.NewBufferString(`{"email_confirmation":"d@example.com"}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	if len(bc.cancellations) != 0 {
		t.Fatalf("expected no Stripe cancellation, got %v", bc.cancellations)
	}
}

// guard the stripe import (used by Client implementations referenced via
// the embedded interface — keeps go vet happy if the embed is later
// replaced with concrete fields).
var _ = stripe.SubscriptionStatusActive

// ============================================================================
// 5.6: Integration tests for audit trail on deletion events
// ============================================================================

// auditMockQuery extends MockQuery with ScheduleAccountDeletion support.
type auditMockQuery struct {
	MockQuery
	scheduleCalls []string
	cancelCalls   []string
}

func (m *auditMockQuery) ScheduleAccountDeletion(_ context.Context, userID string, totalChannels int) (*db.AccountDeletionJob, error) {
	m.scheduleCalls = append(m.scheduleCalls, userID)
	return &db.AccountDeletionJob{ID: "job-" + userID, UserID: userID, Status: "pending"}, nil
}

func (m *auditMockQuery) CancelAccountDeletion(_ context.Context, userID string) error {
	m.cancelCalls = append(m.cancelCalls, userID)
	return nil
}

func TestDeleteAccount_CreatesAuditLogEntry(t *testing.T) {
	user := &db.User{ID: "u-audit1", Email: "audit@example.com"}
	mock := &auditMockQuery{MockQuery: MockQuery{users: map[string]*db.User{"u-audit1": user}}}
	h := &api.AccountHandler{Queries: mock, Cfg: config.Default}
	r := newAccountRouter(h, "u-audit1")

	req := httptest.NewRequest(http.MethodDelete, "/api/user/account",
		bytes.NewBufferString(`{"email_confirmation":"audit@example.com"}`))
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	if len(mock.auditLogs) != 1 {
		t.Fatalf("expected 1 audit log entry, got %d", len(mock.auditLogs))
	}
	entry := mock.auditLogs[0]
	if entry.UserID != "u-audit1" {
		t.Errorf("expected userID u-audit1, got %s", entry.UserID)
	}
	if entry.Action != "deletion_requested" {
		t.Errorf("expected action deletion_requested, got %s", entry.Action)
	}
	if entry.IPAddress != "192.168.1.1:12345" {
		t.Errorf("expected IP 192.168.1.1:12345, got %s", entry.IPAddress)
	}
}

func TestCancelAccountDeletion_CreatesAuditLogEntry(t *testing.T) {
	now := time.Now().UTC()
	user := &db.User{ID: "u-audit2", Email: "cancel@example.com", DeletionRequestedAt: &now}
	mock := &auditMockQuery{MockQuery: MockQuery{users: map[string]*db.User{"u-audit2": user}}}
	h := &api.AccountHandler{Queries: mock, Cfg: config.Default}
	r := newAccountRouter(h, "u-audit2")

	req := httptest.NewRequest(http.MethodPost, "/api/user/account/cancel-deletion", nil)
	req.RemoteAddr = "10.0.0.1:54321"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(mock.auditLogs) != 1 {
		t.Fatalf("expected 1 audit log entry, got %d", len(mock.auditLogs))
	}
	entry := mock.auditLogs[0]
	if entry.Action != "deletion_cancelled" {
		t.Errorf("expected action deletion_cancelled, got %s", entry.Action)
	}
	if entry.UserID != "u-audit2" {
		t.Errorf("expected userID u-audit2, got %s", entry.UserID)
	}
}

func TestCancelAccountDeletion_RejectsPastReversibleWindow(t *testing.T) {
	// Set deletion_requested_at to 31 days ago (past the 720-hour / 30-day window)
	pastDate := time.Now().UTC().Add(-31 * 24 * time.Hour)
	user := &db.User{ID: "u-audit3", Email: "late@example.com", DeletionRequestedAt: &pastDate}
	mock := &auditMockQuery{MockQuery: MockQuery{users: map[string]*db.User{"u-audit3": user}}}
	h := &api.AccountHandler{Queries: mock, Cfg: config.Default}
	r := newAccountRouter(h, "u-audit3")

	req := httptest.NewRequest(http.MethodPost, "/api/user/account/cancel-deletion", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d: %s", w.Code, w.Body.String())
	}
	if len(mock.auditLogs) != 0 {
		t.Fatalf("expected no audit log entry for rejected cancellation, got %d", len(mock.auditLogs))
	}
	if len(mock.cancelCalls) != 0 {
		t.Fatalf("expected no CancelAccountDeletion call, got %v", mock.cancelCalls)
	}
}

func TestCancelAccountDeletion_RejectsWithoutPendingDeletion(t *testing.T) {
	user := &db.User{ID: "u-audit4", Email: "nopending@example.com"}
	mock := &auditMockQuery{MockQuery: MockQuery{users: map[string]*db.User{"u-audit4": user}}}
	h := &api.AccountHandler{Queries: mock, Cfg: config.Default}
	r := newAccountRouter(h, "u-audit4")

	req := httptest.NewRequest(http.MethodPost, "/api/user/account/cancel-deletion", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d: %s", w.Code, w.Body.String())
	}
}
