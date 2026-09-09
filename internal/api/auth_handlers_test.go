package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"noumena/internal/api"
	"noumena/internal/auth"
	"noumena/internal/config"
	"noumena/internal/db"
	"noumena/internal/email"
	"noumena/internal/oauth"
)

// MockEmailClient counts each kind of message for assertion.
type mockEmailClient struct {
	verifications, resets, lockouts, googleNotices int
}

func (m *mockEmailClient) SendVerificationEmail(to, link string) error  { m.verifications++; return nil }
func (m *mockEmailClient) SendPasswordResetEmail(to, link string) error { m.resets++; return nil }
func (m *mockEmailClient) SendLoginLockoutEmail(to string) error        { m.lockouts++; return nil }
func (m *mockEmailClient) SendGoogleOnlyAccountResetNotice(to string) error {
	m.googleNotices++
	return nil
}
func (m *mockEmailClient) SendWelcomeEmail(to string) error         { return nil }
func (m *mockEmailClient) SendDigestEmail(to, content string) error { return nil }
func (m *mockEmailClient) SendLockedAccountEmail(to string) error   { return m.SendLoginLockoutEmail(to) }
func (m *mockEmailClient) SendTokenExpiryEmail(to, channelName string, expiresAt time.Time, settingsURL string) error {
	return nil
}

// Ensure mockEmailClient satisfies the interface.
var _ email.Client = (*mockEmailClient)(nil)

func setupAuthRouter(queries *MockQuery, mc email.Client) *chi.Mux {
	r := chi.NewRouter()
	h := &api.AuthHandler{Queries: queries, EmailClient: mc}

	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", h.Register)
		r.Post("/login", h.Login)
		r.Post("/refresh", h.Refresh)
		r.Post("/logout", h.Logout)
		r.Post("/verify-email", h.VerifyEmail)
		r.Post("/forgot-password", h.ForgotPassword)
		r.Post("/reset-password", h.ResetPassword)

		r.Group(func(r chi.Router) {
			r.Use(api.AuthMiddlewareWithQueries(queries))
			r.Post("/logout-all", h.LogoutAll)
			r.Post("/resend-verification", h.ResendVerification)
			r.Post("/password", h.ChangePassword)
		})
	})

	r.Route("/api", func(r chi.Router) {
		r.Use(api.AuthMiddlewareWithQueries(queries))
		r.Use(api.RequireVerifiedEmail(queries))
		r.Get("/ping", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	})

	return r
}

func registerJSON(t *testing.T, r *chi.Mux, email, password string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "password": password})
	req, _ := http.NewRequest("POST", "/auth/register", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

func TestRegisterWeakPassword(t *testing.T) {
	r := setupAuthRouter(&MockQuery{}, &mockEmailClient{})
	rr := registerJSON(t, r, "weak@example.com", "short")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (%s)", rr.Code, rr.Body.String())
	}
}

func TestRegisterDuplicate(t *testing.T) {
	q := &MockQuery{}
	r := setupAuthRouter(q, &mockEmailClient{})
	rr := registerJSON(t, r, "dup@example.com", "Password1")
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}
	rr = registerJSON(t, r, "dup@example.com", "Password1")
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d (%s)", rr.Code, rr.Body.String())
	}
}

func TestRegisterIssuesVerificationEmail(t *testing.T) {
	q := &MockQuery{}
	mc := &mockEmailClient{}
	r := setupAuthRouter(q, mc)
	rr := registerJSON(t, r, "v@example.com", "Password1")
	if rr.Code != http.StatusCreated {
		t.Fatalf("got %d: %s", rr.Code, rr.Body.String())
	}
	if mc.verifications != 1 {
		t.Errorf("expected 1 verification email, got %d", mc.verifications)
	}
	// Cookie should be set with Path=/auth.
	cookies := rr.Result().Cookies()
	var refresh *http.Cookie
	for _, c := range cookies {
		if c.Name == "refresh_token" {
			refresh = c
		}
	}
	if refresh == nil {
		t.Fatal("refresh_token cookie not set")
	}
	if refresh.Path != "/auth" {
		t.Errorf("cookie path = %q, want /auth", refresh.Path)
	}
	if !refresh.HttpOnly {
		t.Error("refresh cookie should be HttpOnly")
	}
}

func TestLoginWrongPassword(t *testing.T) {
	q := &MockQuery{}
	r := setupAuthRouter(q, &mockEmailClient{})
	registerJSON(t, r, "u@example.com", "Password1")

	body, _ := json.Marshal(map[string]string{"email": "u@example.com", "password": "wrong"})
	req, _ := http.NewRequest("POST", "/auth/login", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Invalid email or password") {
		t.Errorf("expected generic message, got %s", rr.Body.String())
	}
}

func TestLoginUnknownEmail(t *testing.T) {
	r := setupAuthRouter(&MockQuery{}, &mockEmailClient{})
	body, _ := json.Marshal(map[string]string{"email": "ghost@example.com", "password": "Password1"})
	req, _ := http.NewRequest("POST", "/auth/login", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got %d", rr.Code)
	}
}

func TestLoginLocksAfterFiveFailures(t *testing.T) {
	q := &MockQuery{}
	mc := &mockEmailClient{}
	r := setupAuthRouter(q, mc)
	registerJSON(t, r, "lock@example.com", "Password1")

	for i := 0; i < 5; i++ {
		body, _ := json.Marshal(map[string]string{"email": "lock@example.com", "password": "wrong"})
		req, _ := http.NewRequest("POST", "/auth/login", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
	}
	if mc.lockouts != 1 {
		t.Errorf("expected 1 lockout email, got %d", mc.lockouts)
	}
	user := q.users["lock@example.com"]
	if user == nil || user.LockedUntil == nil || !user.LockedUntil.After(time.Now()) {
		t.Errorf("expected user to be locked")
	}
}

func TestRefreshRotatesToken(t *testing.T) {
	q := &MockQuery{}
	r := setupAuthRouter(q, &mockEmailClient{})
	rr := registerJSON(t, r, "rot@example.com", "Password1")
	cookies := rr.Result().Cookies()

	req, _ := http.NewRequest("POST", "/auth/refresh", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusOK {
		t.Fatalf("refresh returned %d: %s", rr2.Code, rr2.Body.String())
	}
	// Reusing the original cookie should now fail.
	req3, _ := http.NewRequest("POST", "/auth/refresh", nil)
	for _, c := range cookies {
		req3.AddCookie(c)
	}
	rr3 := httptest.NewRecorder()
	r.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusUnauthorized {
		t.Errorf("expected old refresh token to be unusable, got %d", rr3.Code)
	}
}

func TestLogoutClearsCookie(t *testing.T) {
	q := &MockQuery{}
	r := setupAuthRouter(q, &mockEmailClient{})
	rr := registerJSON(t, r, "lo@example.com", "Password1")
	cookies := rr.Result().Cookies()

	req, _ := http.NewRequest("POST", "/auth/logout", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusNoContent {
		t.Errorf("logout: %d", rr2.Code)
	}
	if len(q.refreshTokens) != 0 {
		t.Errorf("expected no refresh rows, got %d", len(q.refreshTokens))
	}
}

func TestLogoutAllBumpsTokenVersion(t *testing.T) {
	q := &MockQuery{}
	r := setupAuthRouter(q, &mockEmailClient{})
	rr := registerJSON(t, r, "la@example.com", "Password1")
	var registerResp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &registerResp)
	access := registerResp["access_token"]

	req, _ := http.NewRequest("POST", "/auth/logout-all", nil)
	req.Header.Set("Authorization", "Bearer "+access)
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusNoContent {
		t.Fatalf("logout-all: %d (%s)", rr2.Code, rr2.Body.String())
	}
	user := q.users["la@example.com"]
	if user.TokenVersion != 1 {
		t.Errorf("expected token_version=1, got %d", user.TokenVersion)
	}
	// The same access token should now be rejected by the middleware.
	req3, _ := http.NewRequest("POST", "/auth/logout-all", nil)
	req3.Header.Set("Authorization", "Bearer "+access)
	rr3 := httptest.NewRecorder()
	r.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 after token_version bump, got %d", rr3.Code)
	}
}

func TestVerifyEmailFlow(t *testing.T) {
	q := &MockQuery{}
	mc := &mockEmailClient{}
	// Capture the link by replacing the email client.
	type captured struct{ link string }
	cap := &captured{}
	wrapped := &captureClient{inner: mc, captureFn: func(link string) { cap.link = link }}
	r := setupAuthRouter(q, wrapped)

	rr := registerJSON(t, r, "vf@example.com", "Password1")
	if rr.Code != http.StatusCreated {
		t.Fatal("register failed")
	}
	if cap.link == "" {
		t.Fatal("verification link not captured")
	}
	// Extract token from link.
	idx := strings.Index(cap.link, "token=")
	if idx == -1 {
		t.Fatalf("no token in link %s", cap.link)
	}
	tok := cap.link[idx+len("token="):]

	req, _ := http.NewRequest("POST", "/auth/verify-email?token="+tok, nil)
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusOK {
		t.Fatalf("verify-email got %d: %s", rr2.Code, rr2.Body.String())
	}
	user := q.users["vf@example.com"]
	if user == nil || user.EmailVerifiedAt == nil {
		t.Errorf("user not verified")
	}

	// Replaying the same token should now return 410.
	req2, _ := http.NewRequest("POST", "/auth/verify-email?token="+tok, nil)
	rr3 := httptest.NewRecorder()
	r.ServeHTTP(rr3, req2)
	if rr3.Code != http.StatusGone {
		t.Errorf("expected 410 on replay, got %d", rr3.Code)
	}
}

func TestVerifyEmailUnknownToken(t *testing.T) {
	r := setupAuthRouter(&MockQuery{}, &mockEmailClient{})
	req, _ := http.NewRequest("POST", "/auth/verify-email?token=bogus", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusGone {
		t.Errorf("expected 410, got %d", rr.Code)
	}
}

// TestVerifyEmailChangeFlow covers the fix-frontend-plumbing email-change
// path: an email_change_tokens token (issued by PATCH /user/me) commits the
// pending address, marks it verified, and can only be consumed once.
func TestVerifyEmailChangeFlow(t *testing.T) {
	q := &MockQuery{}
	r := setupAuthRouter(q, &mockEmailClient{})

	// Seed a user (old email) + a pending email-change token for new@.
	user := &db.User{ID: "u-emailchange", Email: "old@example.com", TokenVersion: 3}
	q.users = map[string]*db.User{user.Email: user}
	raw := "change-token-raw"
	hash := auth.HashRefreshToken(raw)
	if err := q.InsertEmailChangeToken(context.Background(), user.ID, "new@example.com", hash, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest("POST", "/auth/verify-email?token="+raw, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("verify-email-change got %d: %s", rr.Code, rr.Body.String())
	}

	updated := q.users["new@example.com"]
	if updated == nil {
		t.Fatalf("user not moved to new email")
	}
	if updated.EmailVerifiedAt == nil {
		t.Errorf("new email not marked verified")
	}
	if updated.TokenVersion != 4 {
		t.Errorf("token_version not bumped: got %d, want 4", updated.TokenVersion)
	}
	// Old email must no longer resolve (keep-old-until-verified semantics
	// end once the change commits).
	if _, ok := q.users["old@example.com"]; ok {
		t.Errorf("old email still present after commit")
	}

	// Replay → 410, and the email stays on the committed value.
	req2, _ := http.NewRequest("POST", "/auth/verify-email?token="+raw, nil)
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusGone {
		t.Errorf("expected 410 on replay, got %d", rr2.Code)
	}
	if _, ok := q.users["new@example.com"]; !ok {
		t.Errorf("email reverted after replay")
	}
}

func TestForgotPasswordAlwaysGeneric(t *testing.T) {
	q := &MockQuery{}
	mc := &mockEmailClient{}
	r := setupAuthRouter(q, mc)

	send := func(emailAddr string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"email": emailAddr})
		req, _ := http.NewRequest("POST", "/auth/forgot-password", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr
	}

	// Unknown email
	rr := send("nobody@example.com")
	if rr.Code != http.StatusOK {
		t.Errorf("unknown: %d", rr.Code)
	}
	if mc.resets != 0 {
		t.Errorf("expected no email for unknown, got %d", mc.resets)
	}

	// Known email
	registerJSON(t, r, "fp@example.com", "Password1")
	mc.resets = 0
	if rr := send("fp@example.com"); rr.Code != http.StatusOK {
		t.Errorf("known: %d", rr.Code)
	}
	if mc.resets != 1 {
		t.Errorf("expected 1 reset email, got %d", mc.resets)
	}
}

func TestForgotPasswordRateLimit(t *testing.T) {
	q := &MockQuery{}
	mc := &mockEmailClient{}
	r := setupAuthRouter(q, mc)
	registerJSON(t, r, "rl@example.com", "Password1")

	send := func() {
		body, _ := json.Marshal(map[string]string{"email": "rl@example.com"})
		req, _ := http.NewRequest("POST", "/auth/forgot-password", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d", rr.Code)
		}
	}
	for i := 0; i < 4; i++ {
		send()
	}
	if mc.resets != 3 {
		t.Errorf("expected exactly 3 reset emails (rate-limited), got %d", mc.resets)
	}
}

func TestResetPasswordWeakPassword(t *testing.T) {
	r := setupAuthRouter(&MockQuery{}, &mockEmailClient{})
	body, _ := json.Marshal(map[string]string{"password": "short"})
	req, _ := http.NewRequest("POST", "/auth/reset-password?token=abc", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestRequireVerifiedEmailMiddleware(t *testing.T) {
	q := &MockQuery{}
	r := setupAuthRouter(q, &mockEmailClient{})

	// Register (unverified).
	rr := registerJSON(t, r, "mw@example.com", "Password1")
	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)
	access := resp["access_token"]

	// /api/ping should be 403 with email_not_verified.
	req, _ := http.NewRequest("GET", "/api/ping", nil)
	req.Header.Set("Authorization", "Bearer "+access)
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr2.Code)
	}
	if !strings.Contains(rr2.Body.String(), "email_not_verified") {
		t.Errorf("expected email_not_verified body, got %s", rr2.Body.String())
	}

	// Mark verified manually, retry.
	now := time.Now()
	q.users["mw@example.com"].EmailVerifiedAt = &now
	req3, _ := http.NewRequest("GET", "/api/ping", nil)
	req3.Header.Set("Authorization", "Bearer "+access)
	rr3 := httptest.NewRecorder()
	r.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Errorf("expected 200 after verify, got %d", rr3.Code)
	}
}

func TestResendVerificationRateLimit(t *testing.T) {
	q := &MockQuery{}
	mc := &mockEmailClient{}
	r := setupAuthRouter(q, mc)
	rr := registerJSON(t, r, "rs@example.com", "Password1")
	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)
	access := resp["access_token"]

	doResend := func() *httptest.ResponseRecorder {
		req, _ := http.NewRequest("POST", "/auth/resend-verification", nil)
		req.Header.Set("Authorization", "Bearer "+access)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr
	}
	// register itself counts as 1 issued token. Two resends → 3 total. Fourth should be 429.
	for i := 0; i < 2; i++ {
		if rr := doResend(); rr.Code != http.StatusOK {
			t.Fatalf("resend %d: %d (%s)", i+1, rr.Code, rr.Body.String())
		}
	}
	if rr := doResend(); rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 on 4th issuance, got %d", rr.Code)
	}
}

// captureClient wraps an email.Client and captures the verification link.
type captureClient struct {
	inner     email.Client
	captureFn func(link string)
}

func (c *captureClient) SendVerificationEmail(to, link string) error {
	c.captureFn(link)
	return c.inner.SendVerificationEmail(to, link)
}
func (c *captureClient) SendPasswordResetEmail(to, link string) error {
	c.captureFn(link)
	return c.inner.SendPasswordResetEmail(to, link)
}
func (c *captureClient) SendLoginLockoutEmail(to string) error {
	return c.inner.SendLoginLockoutEmail(to)
}
func (c *captureClient) SendGoogleOnlyAccountResetNotice(to string) error {
	return c.inner.SendGoogleOnlyAccountResetNotice(to)
}
func (c *captureClient) SendWelcomeEmail(to string) error { return c.inner.SendWelcomeEmail(to) }
func (c *captureClient) SendDigestEmail(to, content string) error {
	return c.inner.SendDigestEmail(to, content)
}
func (c *captureClient) SendLockedAccountEmail(to string) error {
	return c.inner.SendLockedAccountEmail(to)
}
func (c *captureClient) SendTokenExpiryEmail(to, channelName string, expiresAt time.Time, settingsURL string) error {
	return c.inner.SendTokenExpiryEmail(to, channelName, expiresAt, settingsURL)
}

// Ensure the interface compiles.
var _ email.Client = (*captureClient)(nil)

// Silence unused-import warning when ctx isn't otherwise used.
var _ = context.TODO

// Silence unused-import warning if needed.
var _ = auth.HashRefreshToken
var _ = db.ErrNotFound

// ---------------------------------------------------------------------------
// auth-google-popup-endpoints tests
// ---------------------------------------------------------------------------

// setupGoogleAuthRouter wires up the same routes as cmd/api/main.go for the
// Google SSO surface (redirect + JSON twins + /auth/me). It injects a
// caller-supplied OAuthExchanger so we never hit Google's real token endpoint
// or JWKS in unit tests.
func setupGoogleAuthRouter(queries *MockQuery, mc email.Client, exchanger func(ctx context.Context, code, codeVerifier, redirectURI string) (string, *oauth.IDClaims, error)) (*chi.Mux, *api.AuthHandler) {
	r := chi.NewRouter()
	h := &api.AuthHandler{
		Queries:        queries,
		EmailClient:    mc,
		OAuthExchanger: exchanger,
	}
	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", h.Register)
		r.Post("/login", h.Login)
		r.Get("/google", h.GoogleStart)
		r.Get("/google/callback", h.GoogleCallback)
		r.Get("/google/auth-url", h.GetGoogleAuthURL)
		r.Post("/google/callback", h.GoogleCallbackJSON)
		r.Group(func(r chi.Router) {
			r.Use(api.AuthMiddlewareWithQueries(queries))
			r.Post("/google/link", h.LinkGoogleConfirm)
			r.Get("/me", h.Me)
		})
	})
	return r, h
}

func TestGetGoogleAuthURL_OK(t *testing.T) {
	prev := config.Default.GoogleSSOClientID
	prevPop := config.Default.GoogleSSOPopupRedirectURL
	config.Default.GoogleSSOClientID = "test-client"
	config.Default.GoogleSSOClientSecret = "test-secret"
	config.Default.GoogleSSOPopupRedirectURL = "https://app.example.com/oauth/callback"
	t.Cleanup(func() {
		config.Default.GoogleSSOClientID = prev
		config.Default.GoogleSSOPopupRedirectURL = prevPop
	})

	r, _ := setupGoogleAuthRouter(&MockQuery{}, &mockEmailClient{}, nil)
	req, _ := http.NewRequest("GET", "/auth/google/auth-url", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("got %d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["auth_url"] == "" || body["state"] == "" {
		t.Fatalf("missing auth_url or state: %+v", body)
	}
	if !strings.Contains(body["auth_url"], "code_challenge_method=S256") {
		t.Errorf("auth_url missing PKCE: %s", body["auth_url"])
	}
	if !strings.Contains(body["auth_url"], "state="+body["state"]) {
		t.Errorf("auth_url state does not match body state: %s vs %s", body["auth_url"], body["state"])
	}
	// state cookie must be set
	var stateCookie *http.Cookie
	for _, c := range rr.Result().Cookies() {
		if c.Name == "google_oauth_state" {
			stateCookie = c
		}
	}
	if stateCookie == nil || stateCookie.Value == "" {
		t.Fatal("google_oauth_state cookie not set")
	}
}

func TestGetGoogleAuthURL_NoRedirect(t *testing.T) {
	prev := config.Default.GoogleSSOClientID
	config.Default.GoogleSSOClientID = "test-client"
	t.Cleanup(func() { config.Default.GoogleSSOClientID = prev })

	r, _ := setupGoogleAuthRouter(&MockQuery{}, &mockEmailClient{}, nil)
	req, _ := http.NewRequest("GET", "/auth/google/auth-url", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code < 200 || rr.Code >= 300 {
		t.Fatalf("expected 2xx, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "" {
		t.Errorf("unexpected Location header: %s", loc)
	}
}

// performAuthURLAndExtractCookie hits /auth/google/auth-url and returns the
// resulting `state` value plus the `google_oauth_state` cookie so callbacks
// can replay them.
func performAuthURLAndExtractCookie(t *testing.T, r *chi.Mux) (state string, cookie *http.Cookie) {
	t.Helper()
	req, _ := http.NewRequest("GET", "/auth/google/auth-url", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("auth-url: %d %s", rr.Code, rr.Body.String())
	}
	var body map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	for _, c := range rr.Result().Cookies() {
		if c.Name == "google_oauth_state" {
			cookie = c
		}
	}
	return body["state"], cookie
}

func TestGoogleCallbackJSON_StateMismatch(t *testing.T) {
	prev := config.Default.GoogleSSOClientID
	config.Default.GoogleSSOClientID = "test-client"
	t.Cleanup(func() { config.Default.GoogleSSOClientID = prev })

	exchangerCalled := false
	exch := func(ctx context.Context, code, verifier, redirectURI string) (string, *oauth.IDClaims, error) {
		exchangerCalled = true
		return "", nil, nil
	}
	r, _ := setupGoogleAuthRouter(&MockQuery{}, &mockEmailClient{}, exch)
	_, cookie := performAuthURLAndExtractCookie(t, r)
	if cookie == nil {
		t.Fatal("no state cookie")
	}

	body, _ := json.Marshal(map[string]string{"code": "abc", "state": "WRONG-STATE"})
	req, _ := http.NewRequest("POST", "/auth/google/callback", bytes.NewReader(body))
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (%s)", rr.Code, rr.Body.String())
	}
	if exchangerCalled {
		t.Errorf("exchanger should not be called on state mismatch")
	}
	for _, c := range rr.Result().Cookies() {
		if c.Name == "refresh_token" && c.MaxAge > 0 {
			t.Errorf("refresh_token cookie should not be set on mismatch")
		}
	}
}

func TestGoogleCallbackJSON_ReturningUser(t *testing.T) {
	prev := config.Default.GoogleSSOClientID
	config.Default.GoogleSSOClientID = "test-client"
	t.Cleanup(func() { config.Default.GoogleSSOClientID = prev })

	q := &MockQuery{}
	// Pre-create a user with the matching google_sub.
	_, _ = q.CreateUserGoogle(context.Background(), "ret@example.com", "google-sub-123")

	exch := func(ctx context.Context, code, verifier, redirectURI string) (string, *oauth.IDClaims, error) {
		return "fake-id-token", &oauth.IDClaims{Sub: "google-sub-123", Email: "ret@example.com"}, nil
	}
	r, _ := setupGoogleAuthRouter(q, &mockEmailClient{}, exch)
	state, cookie := performAuthURLAndExtractCookie(t, r)

	body, _ := json.Marshal(map[string]string{"code": "code-xyz", "state": state})
	req, _ := http.NewRequest("POST", "/auth/google/callback", bytes.NewReader(body))
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["access_token"] == "" {
		t.Errorf("missing access_token in body: %s", rr.Body.String())
	}
	var refresh *http.Cookie
	for _, c := range rr.Result().Cookies() {
		if c.Name == "refresh_token" {
			refresh = c
		}
	}
	if refresh == nil || refresh.Value == "" {
		t.Errorf("refresh_token cookie not set")
	}
	if loc := rr.Header().Get("Location"); loc != "" {
		t.Errorf("must not return Location header: %s", loc)
	}
}

func TestGoogleCallbackJSON_NewUser(t *testing.T) {
	prev := config.Default.GoogleSSOClientID
	config.Default.GoogleSSOClientID = "test-client"
	t.Cleanup(func() { config.Default.GoogleSSOClientID = prev })

	q := &MockQuery{}
	exch := func(ctx context.Context, code, verifier, redirectURI string) (string, *oauth.IDClaims, error) {
		return "fake-id-token", &oauth.IDClaims{Sub: "new-sub-1", Email: "newbie@example.com"}, nil
	}
	r, _ := setupGoogleAuthRouter(q, &mockEmailClient{}, exch)
	state, cookie := performAuthURLAndExtractCookie(t, r)

	body, _ := json.Marshal(map[string]string{"code": "code-1", "state": state})
	req, _ := http.NewRequest("POST", "/auth/google/callback", bytes.NewReader(body))
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rr.Code, rr.Body.String())
	}
	user := q.users["newbie@example.com"]
	if user == nil {
		t.Fatal("expected user row to be created")
	}
	if user.EmailVerifiedAt == nil {
		t.Errorf("expected email_verified_at to be set")
	}
	if user.PasswordHash != nil {
		t.Errorf("expected password_hash NULL")
	}
	if user.TokenVersion != 0 {
		t.Errorf("expected token_version=0 got %d", user.TokenVersion)
	}
	if user.GoogleSub == nil || *user.GoogleSub != "new-sub-1" {
		t.Errorf("google_sub mismatch")
	}
	var resp map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["access_token"] == "" {
		t.Errorf("no access_token")
	}
}

func TestGoogleCallbackJSON_LinkRequired(t *testing.T) {
	prev := config.Default.GoogleSSOClientID
	config.Default.GoogleSSOClientID = "test-client"
	t.Cleanup(func() { config.Default.GoogleSSOClientID = prev })

	q := &MockQuery{}
	// Create a password-only account whose email will collide.
	_, _ = q.CreateUserWithPassword(context.Background(), "pw@example.com", "$2y$12$hashhashhashhashhashha")
	exch := func(ctx context.Context, code, verifier, redirectURI string) (string, *oauth.IDClaims, error) {
		return "fake-id-token", &oauth.IDClaims{Sub: "incoming-sub", Email: "pw@example.com"}, nil
	}
	r, _ := setupGoogleAuthRouter(q, &mockEmailClient{}, exch)
	state, cookie := performAuthURLAndExtractCookie(t, r)

	body, _ := json.Marshal(map[string]string{"code": "c", "state": state})
	req, _ := http.NewRequest("POST", "/auth/google/callback", bytes.NewReader(body))
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d (%s)", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["error"] != "link_required" {
		t.Errorf("expected error=link_required, got %s", resp["error"])
	}
	if resp["email"] != "pw@example.com" || resp["sub"] != "incoming-sub" {
		t.Errorf("unexpected payload: %+v", resp)
	}
	if resp["id_token"] != "fake-id-token" {
		t.Errorf("expected verified id_token surfaced for the link flow, got %q", resp["id_token"])
	}
	for _, c := range rr.Result().Cookies() {
		if c.Name == "refresh_token" && c.MaxAge > 0 {
			t.Errorf("must not set refresh_token on link_required")
		}
	}
}

func TestGoogleCallbackJSON_InvalidIDToken(t *testing.T) {
	prev := config.Default.GoogleSSOClientID
	config.Default.GoogleSSOClientID = "test-client"
	t.Cleanup(func() { config.Default.GoogleSSOClientID = prev })

	exch := func(ctx context.Context, code, verifier, redirectURI string) (string, *oauth.IDClaims, error) {
		return "", nil, errors.New("aud mismatch")
	}
	r, _ := setupGoogleAuthRouter(&MockQuery{}, &mockEmailClient{}, exch)
	state, cookie := performAuthURLAndExtractCookie(t, r)

	body, _ := json.Marshal(map[string]string{"code": "c", "state": state})
	req, _ := http.NewRequest("POST", "/auth/google/callback", bytes.NewReader(body))
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (%s)", rr.Code, rr.Body.String())
	}
}

func TestAuthMe_OK(t *testing.T) {
	q := &MockQuery{}
	r, _ := setupGoogleAuthRouter(q, &mockEmailClient{}, nil)
	// Register a user to get an access token.
	rr := registerJSON(t, r, "me@example.com", "Password1")
	if rr.Code != http.StatusCreated {
		t.Fatalf("register: %d", rr.Code)
	}
	var resp map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	access := resp["access_token"]

	req, _ := http.NewRequest("GET", "/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+access)
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rr2.Code, rr2.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(rr2.Body.Bytes(), &body)
	if body["email"] != "me@example.com" {
		t.Errorf("email = %v", body["email"])
	}
	if body["id"] == nil || body["id"] == "" {
		t.Errorf("missing id: %+v", body)
	}
	if _, ok := body["email_verified"].(bool); !ok {
		t.Errorf("email_verified missing or not bool")
	}
}

func TestAuthMe_NoAuthHeader(t *testing.T) {
	r, _ := setupGoogleAuthRouter(&MockQuery{}, &mockEmailClient{}, nil)
	req, _ := http.NewRequest("GET", "/auth/me", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAuthMe_ExpiredToken(t *testing.T) {
	q := &MockQuery{}
	r, _ := setupGoogleAuthRouter(q, &mockEmailClient{}, nil)
	registerJSON(t, r, "exp@example.com", "Password1")

	// Force JWT expiry by reducing the access TTL to a negative duration and
	// minting a fresh token under that config.
	prev := config.Default.JWTAccessTTL
	config.Default.JWTAccessTTL = -1 * time.Hour
	t.Cleanup(func() { config.Default.JWTAccessTTL = prev })

	access, _, err := auth.IssueAccessToken("test-user-id", 0)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	req, _ := http.NewRequest("GET", "/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+access)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAuthMe_IndependentOfLifecycleFlag(t *testing.T) {
	prev := config.Default.AccountLifecycleAPIEnabled
	config.Default.AccountLifecycleAPIEnabled = false
	t.Cleanup(func() { config.Default.AccountLifecycleAPIEnabled = prev })

	q := &MockQuery{}
	r, _ := setupGoogleAuthRouter(q, &mockEmailClient{}, nil)
	rr := registerJSON(t, r, "lf@example.com", "Password1")
	var resp map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)

	req, _ := http.NewRequest("GET", "/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+resp["access_token"])
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusOK {
		t.Errorf("expected 200 with lifecycle flag off, got %d", rr2.Code)
	}
}

func TestAuthMe_EmailUnverifiedReturnsFalse(t *testing.T) {
	q := &MockQuery{}
	r, _ := setupGoogleAuthRouter(q, &mockEmailClient{}, nil)
	rr := registerJSON(t, r, "uv@example.com", "Password1")
	var resp map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)

	// New users from /auth/register are NOT verified by default in the mock.
	if u := q.users["uv@example.com"]; u != nil {
		u.EmailVerifiedAt = nil
	}

	req, _ := http.NewRequest("GET", "/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+resp["access_token"])
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rr2.Code, rr2.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(rr2.Body.Bytes(), &body)
	if v, _ := body["email_verified"].(bool); v {
		t.Errorf("expected email_verified=false, got true")
	}
}

func TestGoogleCallbackRedirect_RegressionReturningUser(t *testing.T) {
	prev := config.Default.GoogleSSOClientID
	prevBase := config.Default.AppBaseURL
	config.Default.GoogleSSOClientID = "test-client"
	config.Default.AppBaseURL = "https://app.example.com"
	t.Cleanup(func() {
		config.Default.GoogleSSOClientID = prev
		config.Default.AppBaseURL = prevBase
	})

	q := &MockQuery{}
	_, _ = q.CreateUserGoogle(context.Background(), "rg@example.com", "rg-sub")
	exch := func(ctx context.Context, code, verifier, redirectURI string) (string, *oauth.IDClaims, error) {
		return "fake-id-token", &oauth.IDClaims{Sub: "rg-sub", Email: "rg@example.com"}, nil
	}
	r, _ := setupGoogleAuthRouter(q, &mockEmailClient{}, exch)

	// Drive GoogleStart to populate the cookie + state.
	startReq, _ := http.NewRequest("GET", "/auth/google", nil)
	startRR := httptest.NewRecorder()
	r.ServeHTTP(startRR, startReq)
	if startRR.Code != http.StatusFound {
		t.Fatalf("GoogleStart expected 302, got %d", startRR.Code)
	}
	var stateCookie *http.Cookie
	for _, c := range startRR.Result().Cookies() {
		if c.Name == "google_oauth_state" {
			stateCookie = c
		}
	}
	if stateCookie == nil {
		t.Fatal("no state cookie from GoogleStart")
	}
	// Extract the state from the redirect URL.
	loc := startRR.Header().Get("Location")
	idx := strings.Index(loc, "state=")
	if idx == -1 {
		t.Fatal("no state in Location")
	}
	tail := loc[idx+len("state="):]
	if amp := strings.Index(tail, "&"); amp != -1 {
		tail = tail[:amp]
	}

	cbReq, _ := http.NewRequest("GET", "/auth/google/callback?code=abc&state="+tail, nil)
	cbReq.AddCookie(stateCookie)
	cbRR := httptest.NewRecorder()
	r.ServeHTTP(cbRR, cbReq)
	if cbRR.Code != http.StatusFound {
		t.Fatalf("redirect callback expected 302 returning_user, got %d (%s)", cbRR.Code, cbRR.Body.String())
	}
	if cbRR.Header().Get("Location") == "" {
		t.Errorf("expected Location header on returning_user 302")
	}
}

// ---------------------------------------------------------------------------
// fix-auth-critical-security: LinkGoogleConfirm hardening
// ---------------------------------------------------------------------------

// registerAndGetAccessToken registers a user and returns the access token.
func registerAndGetAccessToken(t *testing.T, r *chi.Mux, email string) string {
	t.Helper()
	rr := registerJSON(t, r, email, "Password1")
	if rr.Code != http.StatusCreated {
		t.Fatalf("register: %d (%s)", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	return resp["access_token"]
}

// doLinkConfirm performs a (possibly unauthenticated) POST /auth/google/link.
func doLinkConfirm(t *testing.T, r *chi.Mux, access, body string) *httptest.ResponseRecorder {
	t.Helper()
	req, _ := http.NewRequest("POST", "/auth/google/link", bytes.NewBufferString(body))
	if access != "" {
		req.Header.Set("Authorization", "Bearer "+access)
	}
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

// setupLinkRouter creates a password account + session and wires a fake
// ID-token verifier into the handler. Tests may override verifier behavior
// by mutating h.IDTokenVerifier after this call.
func setupLinkRouter(t *testing.T) (*chi.Mux, *api.AuthHandler, *MockQuery, string) {
	t.Helper()
	q := &MockQuery{}
	r, h := setupGoogleAuthRouter(q, &mockEmailClient{}, nil)
	h.IDTokenVerifier = func(ctx context.Context, idToken string) (*oauth.IDClaims, error) {
		return &oauth.IDClaims{Sub: "google-sub-9", Email: "link@example.com"}, nil
	}
	access := registerAndGetAccessToken(t, r, "link@example.com")
	return r, h, q, access
}

func TestLinkGoogleConfirm_UnauthenticatedRejected(t *testing.T) {
	r, _, q, _ := setupLinkRouter(t)
	rr := doLinkConfirm(t, r, "", `{"email":"link@example.com","sub":"google-sub-9","id_token":"any"}`)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d (%s)", rr.Code, rr.Body.String())
	}
	if u := q.users["link@example.com"]; u == nil || u.GoogleSub != nil {
		t.Errorf("no account mutation allowed on unauthenticated link attempt")
	}
}

func TestLinkGoogleConfirm_ExpiredOrInvalidTokenRejected(t *testing.T) {
	r, h, q, access := setupLinkRouter(t)
	h.IDTokenVerifier = func(ctx context.Context, idToken string) (*oauth.IDClaims, error) {
		return nil, errors.New("token expired")
	}
	rr := doLinkConfirm(t, r, access, `{"email":"link@example.com","sub":"google-sub-9","id_token":"expired"}`)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d (%s)", rr.Code, rr.Body.String())
	}
	if u := q.users["link@example.com"]; u == nil || u.GoogleSub != nil {
		t.Errorf("no account mutation allowed on invalid token")
	}
}

func TestLinkGoogleConfirm_MissingIDTokenRejected(t *testing.T) {
	r, _, q, access := setupLinkRouter(t)
	rr := doLinkConfirm(t, r, access, `{"email":"link@example.com","sub":"google-sub-9"}`)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d (%s)", rr.Code, rr.Body.String())
	}
	if u := q.users["link@example.com"]; u == nil || u.GoogleSub != nil {
		t.Errorf("no account mutation allowed when id_token missing")
	}
}

func TestLinkGoogleConfirm_WrongSubRejected(t *testing.T) {
	r, _, q, access := setupLinkRouter(t)
	// Verified token's sub differs from the requested sub.
	rr := doLinkConfirm(t, r, access, `{"email":"link@example.com","sub":"attacker-sub","id_token":"any"}`)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d (%s)", rr.Code, rr.Body.String())
	}
	if u := q.users["link@example.com"]; u == nil || u.GoogleSub != nil {
		t.Errorf("no account mutation allowed on sub mismatch")
	}
}

func TestLinkGoogleConfirm_WrongEmailRejected(t *testing.T) {
	r, _, q, access := setupLinkRouter(t)
	// Session user (link@example.com) attempts to link onto another account.
	rr := doLinkConfirm(t, r, access, `{"email":"victim@example.com","sub":"google-sub-9","id_token":"any"}`)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d (%s)", rr.Code, rr.Body.String())
	}
	if u := q.users["link@example.com"]; u == nil || u.GoogleSub != nil {
		t.Errorf("no account mutation allowed on email mismatch")
	}
}

func TestLinkGoogleConfirm_LegitFlowIssuesTokens(t *testing.T) {
	r, _, q, access := setupLinkRouter(t)
	rr := doLinkConfirm(t, r, access, `{"email":"link@example.com","sub":"google-sub-9","id_token":"any"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rr.Code, rr.Body.String())
	}
	if u := q.users["link@example.com"]; u == nil || u.GoogleSub == nil || *u.GoogleSub != "google-sub-9" {
		t.Errorf("google_sub not set on legit flow: %+v", u)
	}
	if rr.Header().Get("X-Access-Token") == "" {
		t.Errorf("expected X-Access-Token header on successful link")
	}
	var refresh *http.Cookie
	for _, c := range rr.Result().Cookies() {
		if c.Name == "refresh_token" {
			refresh = c
		}
	}
	if refresh == nil || refresh.Value == "" {
		t.Errorf("expected refresh_token cookie on successful link")
	}
}
