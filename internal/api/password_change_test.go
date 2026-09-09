// password_change_test.go covers POST /auth/password
// (implement-missing-product-features 2.1 + 5.1/5.5):
//   - wrong current password rejected without any session invalidation
//   - success bumps token_version, kills other sessions, keeps current one
//   - direct signup (register + Google SSO) assigns trial_ends_at = now + 14d
package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"noumena/internal/auth"
	"noumena/internal/db"
	"noumena/internal/dbtest"
)

func loginJSON(t *testing.T, r *chi.Mux, email, password string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "password": password})
	req, _ := http.NewRequest("POST", "/auth/login", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

func changePassword(t *testing.T, r *chi.Mux, access, current, newPw string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"current_password": current, "new_password": newPw})
	req, _ := http.NewRequest("POST", "/auth/password", bytes.NewReader(body))
	if access != "" {
		req.Header.Set("Authorization", "Bearer "+access)
	}
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

func refreshWithCookie(t *testing.T, r *chi.Mux, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req, _ := http.NewRequest("POST", "/auth/refresh", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

func extractAccessToken(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v (%s)", err, rr.Body.String())
	}
	return resp["access_token"]
}

func extractRefreshCookie(rr *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rr.Result().Cookies() {
		if c.Name == "refresh_token" {
			return c
		}
	}
	return nil
}

// 2.3: incorrect current password is rejected with 401 and must not bump
// token_version or touch any session.
func TestChangePassword_WrongCurrentRejected(t *testing.T) {
	q := &MockQuery{}
	r := setupAuthRouter(q, &mockEmailClient{})

	rr := registerJSON(t, r, "pw@example.com", "Password1")
	if rr.Code != http.StatusCreated {
		t.Fatalf("register: %d (%s)", rr.Code, rr.Body.String())
	}
	access := extractAccessToken(t, rr)
	if len(q.refreshTokens) != 1 {
		t.Fatalf("expected 1 refresh row after register, got %d", len(q.refreshTokens))
	}

	rr = changePassword(t, r, access, "WrongPass1", "NewPass1")
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d (%s)", rr.Code, rr.Body.String())
	}

	user := q.users["pw@example.com"]
	if user.TokenVersion != 0 {
		t.Errorf("token_version bumped on wrong current: got %d, want 0", user.TokenVersion)
	}
	if auth.VerifyPassword(*user.PasswordHash, "Password1") != true {
		t.Error("password hash changed on wrong current")
	}
	if len(q.refreshTokens) != 1 {
		t.Errorf("sessions invalidated on wrong current: got %d refresh rows, want 1", len(q.refreshTokens))
	}
}

// 2.3: policy failure returns 400 and does not invalidate anything.
func TestChangePassword_WeakNewPasswordRejected(t *testing.T) {
	q := &MockQuery{}
	r := setupAuthRouter(q, &mockEmailClient{})
	access := registerAndGetAccessToken(t, r, "weak@example.com")

	rr := changePassword(t, r, access, "Password1", "short")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (%s)", rr.Code, rr.Body.String())
	}
	if q.users["weak@example.com"].TokenVersion != 0 {
		t.Error("token_version bumped on policy failure")
	}
	if len(q.refreshTokens) != 1 {
		t.Error("sessions invalidated on policy failure")
	}
}

// 2.3: success invalidates OTHER sessions while the CURRENT session stays
// valid — new access token embeds the bumped version, old refresh rows die,
// the freshly issued refresh cookie still works.
func TestChangePassword_SuccessInvalidatesOtherSessions(t *testing.T) {
	q := &MockQuery{}
	r := setupAuthRouter(q, &mockEmailClient{})

	// Session A (this browser) + session B (another device).
	rrA := registerJSON(t, r, "pw@example.com", "Password1")
	if rrA.Code != http.StatusCreated {
		t.Fatalf("register: %d (%s)", rrA.Code, rrA.Body.String())
	}
	access := extractAccessToken(t, rrA)
	rrB := loginJSON(t, r, "pw@example.com", "Password1")
	if rrB.Code != http.StatusOK {
		t.Fatalf("login B: %d", rrB.Code)
	}
	cookieB := extractRefreshCookie(rrB)
	if cookieB == nil {
		t.Fatal("no refresh cookie from login B")
	}
	if len(q.refreshTokens) != 2 {
		t.Fatalf("expected 2 refresh rows, got %d", len(q.refreshTokens))
	}

	rr := changePassword(t, r, access, "Password1", "NewPass1")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rr.Code, rr.Body.String())
	}

	// New access token embeds token_version 1 and the password really changed.
	newAccess := extractAccessToken(t, rr)
	if newAccess == "" {
		t.Fatal("no access_token in password-change response")
	}
	user := q.users["pw@example.com"]
	if user.TokenVersion != 1 {
		t.Errorf("token_version = %d, want 1", user.TokenVersion)
	}
	if !auth.VerifyPassword(*user.PasswordHash, "NewPass1") {
		t.Error("new password does not verify")
	}
	if auth.VerifyPassword(*user.PasswordHash, "Password1") {
		t.Error("old password still verifies")
	}

	// Only the freshly issued session survives.
	if len(q.refreshTokens) != 1 {
		t.Fatalf("expected 1 refresh row after change, got %d", len(q.refreshTokens))
	}

	// Session B is dead.
	if rr := refreshWithCookie(t, r, cookieB); rr.Code != http.StatusUnauthorized {
		t.Errorf("other session still valid after change: got %d", rr.Code)
	}

	// Current session stays valid: new cookie rotates successfully.
	cookieA := extractRefreshCookie(rr)
	if cookieA == nil {
		t.Fatal("no new refresh cookie in password-change response")
	}
	if rr := refreshWithCookie(t, r, cookieA); rr.Code != http.StatusOK {
		t.Errorf("current session invalidated after change: got %d (%s)", rr.Code, rr.Body.String())
	}

	// And the new access token itself passes the middleware (version matches).
	req, _ := http.NewRequest("POST", "/auth/password", bytes.NewBufferString(
		`{"current_password":"NewPass1","new_password":"Another1"}`))
	req.Header.Set("Authorization", "Bearer "+newAccess)
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusOK {
		t.Errorf("new access token rejected by middleware: got %d", rr2.Code)
	}
}

func TestChangePassword_Unauthenticated(t *testing.T) {
	r := setupAuthRouter(&MockQuery{}, &mockEmailClient{})
	rr := changePassword(t, r, "", "Password1", "NewPass1")
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// Google-only accounts have no password to verify — reject cleanly.
func TestChangePassword_GoogleOnlyNoPassword(t *testing.T) {
	q := &MockQuery{}
	r := setupAuthRouter(q, &mockEmailClient{})
	_, _ = q.CreateUserGoogle(context.Background(), "g@example.com", "g-sub")
	access, _, err := auth.IssueAccessToken("test-user-id", 0)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	rr := changePassword(t, r, access, "whatever", "NewPass1")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d (%s)", rr.Code, rr.Body.String())
	}
}

// 5.5: direct signup assigns trial_ends_at ≈ now + 14d for both the
// password-register path and the Google-SSO new-user path.
func TestSignupAssignsTrial(t *testing.T) {
	ctx := context.Background()
	database := dbtest.SetupTestDB(t)
	q := db.NewSQLiteQuery(database)

	now := time.Now().UTC()
	want := now.Add(db.TrialDuration)
	within := func(got *time.Time, label string) {
		t.Helper()
		if got == nil {
			t.Fatalf("%s: trial_ends_at not set", label)
		}
		diff := math.Abs(got.Sub(want).Seconds())
		if diff > 120 {
			t.Errorf("%s: trial_ends_at=%s, want ~%s (off by %.0fs)", label, got.UTC(), want.UTC(), diff)
		}
	}

	u1, err := q.CreateUserWithPassword(ctx, "trial_pw@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUserWithPassword: %v", err)
	}
	within(u1.TrialEndsAt, "password-register")

	u2, err := q.CreateUserGoogle(ctx, "trial_google@example.com", "g-sub")
	if err != nil {
		t.Fatalf("CreateUserGoogle: %v", err)
	}
	within(u2.TrialEndsAt, "google-sso")

	// Legacy alias path shares the same create SQL.
	u3, err := q.CreateUser(ctx, "trial_legacy@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	within(u3.TrialEndsAt, "legacy-create")
}
