package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"noumena/internal/auth"
	"noumena/internal/config"
	"noumena/internal/db"
	"noumena/internal/email"
	"noumena/internal/oauth"
)

// AuthHandler bundles the dependencies for the /auth/* routes.
type AuthHandler struct {
	Queries     db.DBQuery
	EmailClient email.Client

	// OAuthExchanger lets tests inject a fake Google token-exchange + ID-token
	// verifier in place of the real Google calls. Production code leaves this
	// nil; auth_handlers.go falls back to oauth.ExchangeAndVerifyWithRedirect.
	OAuthExchanger func(ctx context.Context, code, codeVerifier, redirectURI string) (*oauth.IDClaims, error)
}

// AuthRequest is the JSON body shape for /auth/register and /auth/login.
type AuthRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

const (
	refreshCookieName = "refresh_token"
	genericLoginError = "Invalid email or password"
)

// --- Helpers ----------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func setRefreshTokenCookie(w http.ResponseWriter, raw string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    raw,
		Path:     "/auth",
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearRefreshTokenCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     "/auth",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func emailLink(path, token string) string {
	base := strings.TrimRight(config.Default.AppBaseURL, "/")
	return fmt.Sprintf("%s%s?token=%s", base, path, token)
}

func normalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// --- Registration -----------------------------------------------------------

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = normalizeEmail(req.Email)
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}
	if err := auth.ValidatePasswordPolicy(req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Detect 409 cases up-front for a clearer message.
	if existing, _ := h.Queries.GetUserByEmail(r.Context(), req.Email); existing != nil {
		if existing.PasswordHash != nil {
			writeError(w, http.StatusConflict, "An account with this email already exists.")
			return
		}
		if existing.GoogleSub != nil {
			writeError(w, http.StatusConflict, "This email is registered with Google. Please sign in with Google.")
			return
		}
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to process password")
		return
	}

	user, err := h.Queries.CreateUserWithPassword(r.Context(), req.Email, hash)
	if err != nil {
		writeError(w, http.StatusConflict, "email already registered or database error")
		return
	}

	// Issue tokens.
	accessTok, _, err := auth.IssueAccessToken(user.ID, user.TokenVersion)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue access token")
		return
	}
	refreshRaw, refreshExp, err := auth.IssueRefreshToken(r.Context(), refreshStore{h.Queries}, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue refresh token")
		return
	}
	setRefreshTokenCookie(w, refreshRaw, refreshExp)

	// Issue verification token + email.
	if h.EmailClient != nil {
		if vRaw, vHash, err := auth.GenerateRefreshToken(); err == nil {
			expiresAt := time.Now().Add(config.Default.EmailVerificationTTL)
			if err := h.Queries.InsertEmailVerificationToken(r.Context(), user.ID, vHash, expiresAt); err == nil {
				_ = h.EmailClient.SendVerificationEmail(req.Email, emailLink("/verify-email", vRaw))
			}
		}
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"access_token": accessTok,
		"message":      "check inbox to verify account",
	})
}

// --- Login ------------------------------------------------------------------

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = normalizeEmail(req.Email)

	user, err := h.Queries.GetUserByEmail(r.Context(), req.Email)
	if err != nil || user == nil || user.PasswordHash == nil {
		writeError(w, http.StatusUnauthorized, genericLoginError)
		return
	}

	// Locked account?
	if user.LockedUntil != nil && user.LockedUntil.After(time.Now()) {
		writeError(w, http.StatusUnauthorized, genericLoginError)
		return
	}

	if !auth.VerifyPassword(*user.PasswordHash, req.Password) {
		// Increment failed counter; lock at 5 failures within 15 minutes.
		attempts, _ := h.Queries.IncrementFailedLoginAttempts(r.Context(), user.ID)
		if attempts >= 5 {
			until := time.Now().Add(15 * time.Minute)
			_ = h.Queries.LockUser(r.Context(), user.ID, until)
			if h.EmailClient != nil {
				_ = h.EmailClient.SendLoginLockoutEmail(user.Email)
			}
		}
		writeError(w, http.StatusUnauthorized, genericLoginError)
		return
	}

	// Successful auth: clear lockout state.
	_ = h.Queries.ResetLoginFailures(r.Context(), user.ID)

	// Transparent rehash if cost is below configured.
	if auth.NeedsRehash(*user.PasswordHash) {
		if newHash, err := auth.HashPassword(req.Password); err == nil {
			_ = h.Queries.UpdatePasswordHash(r.Context(), user.ID, newHash)
		}
	}

	accessTok, _, err := auth.IssueAccessToken(user.ID, user.TokenVersion)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue access token")
		return
	}
	refreshRaw, refreshExp, err := auth.IssueRefreshToken(r.Context(), refreshStore{h.Queries}, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue refresh token")
		return
	}
	setRefreshTokenCookie(w, refreshRaw, refreshExp)

	writeJSON(w, http.StatusOK, map[string]string{"access_token": accessTok})
}

// --- Refresh ----------------------------------------------------------------

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(refreshCookieName)
	if err != nil || c.Value == "" {
		clearRefreshTokenCookie(w)
		writeError(w, http.StatusUnauthorized, "missing refresh token")
		return
	}
	newRaw, newExp, userID, err := auth.RotateRefreshToken(r.Context(), refreshStore{h.Queries}, c.Value)
	if err != nil {
		clearRefreshTokenCookie(w)
		writeError(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}
	user, err := h.Queries.GetUserByID(r.Context(), userID)
	if err != nil || user == nil {
		clearRefreshTokenCookie(w)
		writeError(w, http.StatusUnauthorized, "invalid session")
		return
	}
	accessTok, _, err := auth.IssueAccessToken(user.ID, user.TokenVersion)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue access token")
		return
	}
	setRefreshTokenCookie(w, newRaw, newExp)
	writeJSON(w, http.StatusOK, map[string]string{"access_token": accessTok})
}

// --- Logout / Logout-all ----------------------------------------------------

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(refreshCookieName); err == nil && c.Value != "" {
		_ = auth.RevokeRefreshToken(r.Context(), refreshStore{h.Queries}, c.Value)
	}
	clearRefreshTokenCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) LogoutAll(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := h.Queries.IncrementTokenVersion(r.Context(), userID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to revoke sessions")
		return
	}
	_ = h.Queries.DeleteAllRefreshTokensForUser(r.Context(), userID)
	clearRefreshTokenCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// --- Email verification -----------------------------------------------------

func (h *AuthHandler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("token")
	if raw == "" {
		writeError(w, http.StatusGone, "invalid or expired token")
		return
	}
	hash := auth.HashRefreshToken(raw)
	userID, err := h.Queries.ConsumeEmailVerificationToken(r.Context(), hash)
	if err != nil || userID == "" {
		writeError(w, http.StatusGone, "invalid or expired token")
		return
	}
	if err := h.Queries.SetEmailVerified(r.Context(), userID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to mark verified")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "email verified"})
}

func (h *AuthHandler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	user, err := h.Queries.GetUserByID(r.Context(), userID)
	if err != nil || user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if user.EmailVerifiedAt != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": "already verified"})
		return
	}

	since := time.Now().Add(-1 * time.Hour)
	count, _ := h.Queries.CountVerificationResendsInWindow(r.Context(), userID, since)
	if count >= 3 {
		writeError(w, http.StatusTooManyRequests, "too many requests; try again later")
		return
	}

	_ = h.Queries.InvalidateEmailVerificationTokensForUser(r.Context(), userID)

	vRaw, vHash, err := auth.GenerateRefreshToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}
	expiresAt := time.Now().Add(config.Default.EmailVerificationTTL)
	if err := h.Queries.InsertEmailVerificationToken(r.Context(), userID, vHash, expiresAt); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}
	if h.EmailClient != nil {
		_ = h.EmailClient.SendVerificationEmail(user.Email, emailLink("/verify-email", vRaw))
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "verification email sent"})
}

// --- Password reset ---------------------------------------------------------

const genericForgotPasswordResponse = "If an account exists with this email, a reset link has been sent."

func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = normalizeEmail(req.Email)

	// Always return the same generic message regardless of branch.
	respond := func() {
		writeJSON(w, http.StatusOK, map[string]string{"message": genericForgotPasswordResponse})
	}

	user, _ := h.Queries.GetUserByEmail(r.Context(), req.Email)
	if user == nil {
		respond()
		return
	}

	// Rate limit: at most 3 reset emails per email address per hour.
	since := time.Now().Add(-1 * time.Hour)
	count, _ := h.Queries.CountForgotPasswordEmailsInWindow(r.Context(), req.Email, since)
	if count >= 3 {
		respond()
		return
	}

	// Google-only account: send the SSO notice instead of issuing a reset token.
	if user.PasswordHash == nil && user.GoogleSub != nil {
		if h.EmailClient != nil {
			_ = h.EmailClient.SendGoogleOnlyAccountResetNotice(user.Email)
		}
		respond()
		return
	}

	// Issue a fresh reset token; invalidate old ones first.
	_ = h.Queries.InvalidatePasswordResetTokensForUser(r.Context(), user.ID)
	raw, hash, err := auth.GenerateRefreshToken()
	if err != nil {
		respond()
		return
	}
	expiresAt := time.Now().Add(config.Default.PasswordResetTTL)
	if err := h.Queries.InsertPasswordResetToken(r.Context(), user.ID, hash, expiresAt); err == nil {
		if h.EmailClient != nil {
			_ = h.EmailClient.SendPasswordResetEmail(user.Email, emailLink("/reset-password", raw))
		}
	}
	respond()
}

func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	rawTok := r.URL.Query().Get("token")
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if rawTok == "" {
		writeError(w, http.StatusGone, "invalid or expired token")
		return
	}
	if err := auth.ValidatePasswordPolicy(req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	hash := auth.HashRefreshToken(rawTok)
	userID, err := h.Queries.ConsumePasswordResetToken(r.Context(), hash)
	if err != nil || userID == "" {
		writeError(w, http.StatusGone, "invalid or expired token")
		return
	}

	pwHash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to process password")
		return
	}
	if err := h.Queries.UpdatePasswordHash(r.Context(), userID, pwHash); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update password")
		return
	}
	// Invalidate every other session.
	_ = h.Queries.IncrementTokenVersion(r.Context(), userID)
	_ = h.Queries.DeleteAllRefreshTokensForUser(r.Context(), userID)
	_ = h.Queries.ResetLoginFailures(r.Context(), userID)

	writeJSON(w, http.StatusOK, map[string]string{"message": "password updated"})
}

// --- Google SSO -------------------------------------------------------------

const googleStateCookieName = "google_oauth_state"

type googleStateCookieValue struct {
	State    string `json:"s"`
	Verifier string `json:"v"`
}

func setGoogleStateCookie(w http.ResponseWriter, p *oauth.GooglePKCE) error {
	v := googleStateCookieValue{State: p.State, Verifier: p.Verifier}
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     googleStateCookieName,
		Value:    base64URLEncode(body),
		Path:     "/auth",
		MaxAge:   600, // 10 minutes
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func readGoogleStateCookie(r *http.Request) (*googleStateCookieValue, error) {
	c, err := r.Cookie(googleStateCookieName)
	if err != nil {
		return nil, err
	}
	body, err := base64URLDecode(c.Value)
	if err != nil {
		return nil, err
	}
	var v googleStateCookieValue
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func clearGoogleStateCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     googleStateCookieName,
		Value:    "",
		Path:     "/auth",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *AuthHandler) GoogleStart(w http.ResponseWriter, r *http.Request) {
	if config.Default.GoogleSSOClientID == "" {
		writeError(w, http.StatusServiceUnavailable, "Google SSO is not configured")
		return
	}
	pkce, err := oauth.GenerateGooglePKCE()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start oauth")
		return
	}
	if err := setGoogleStateCookie(w, pkce); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start oauth")
		return
	}
	http.Redirect(w, r, oauth.AuthorizeURL(pkce), http.StatusFound)
}

// --- Google callback shared pipeline ---------------------------------------

// googleCallbackOutcome enumerates the four terminal states of the shared
// callback pipeline. The redirect handler maps each to a 302 destination; the
// JSON handler maps each to a status code + body shape.
type googleCallbackOutcome int

const (
	googleOutcomeReturningUser     googleCallbackOutcome = iota // sub matched OR same-email-google-only re-link
	googleOutcomeNewUser                                        // brand-new sub + brand-new email
	googleOutcomeLinkRequired                                   // email collides with a password-only account
	googleOutcomeLinkExistingGoogle                             // email matched, sub differed, sub re-bound
)

// googleCallbackResult is the discriminated union returned by
// processGoogleCallback. Only the fields relevant to the chosen Outcome are
// populated.
type googleCallbackResult struct {
	Outcome googleCallbackOutcome
	User    *db.User // returning_user, new_user, link_existing_google
	Email   string   // link_required (also populated for the others, for logging/parity)
	Sub     string   // link_required, link_existing_google
}

// processGoogleCallback runs the full server-side OAuth callback pipeline:
// state validation, code+PKCE token exchange, ID-token verification, then
// classification of the resulting (sub, email) into one of the four outcomes
// in googleCallbackOutcome. It performs DB writes (user creation / sub
// re-link) for the outcomes that require them, but it never touches the
// HTTP response — the calling handler is responsible for shaping the wire
// reply (redirect-flow vs JSON-flow).
//
// Errors map to HTTP 400 in both calling handlers (invalid state, missing
// code, invalid id token, internal create-user failure).
func (h *AuthHandler) processGoogleCallback(ctx context.Context, code, state string, cookieValue *googleStateCookieValue, redirectURI string) (*googleCallbackResult, error) {
	if cookieValue == nil {
		return nil, errors.New("missing oauth state cookie")
	}
	if state == "" || state != cookieValue.State {
		return nil, errors.New("state mismatch")
	}
	if code == "" {
		return nil, errors.New("missing code")
	}

	exchanger := h.OAuthExchanger
	if exchanger == nil {
		exchanger = oauth.ExchangeAndVerifyWithRedirect
	}
	claims, err := exchanger(ctx, code, cookieValue.Verifier, redirectURI)
	if err != nil {
		return nil, fmt.Errorf("invalid id token: %w", err)
	}

	emailAddr := normalizeEmail(claims.Email)

	// Returning user: matched by google_sub.
	if user, _ := h.Queries.GetUserByGoogleSub(ctx, claims.Sub); user != nil {
		return &googleCallbackResult{
			Outcome: googleOutcomeReturningUser,
			User:    user,
			Email:   emailAddr,
			Sub:     claims.Sub,
		}, nil
	}

	// Possibly an existing account with the same email.
	if existing, _ := h.Queries.GetUserByEmail(ctx, emailAddr); existing != nil {
		// Password-only account: requires user-confirmed link.
		if existing.GoogleSub == nil && existing.PasswordHash != nil {
			return &googleCallbackResult{
				Outcome: googleOutcomeLinkRequired,
				Email:   emailAddr,
				Sub:     claims.Sub,
			}, nil
		}
		// Existing google-only user with same email but the sub didn't match —
		// treat as returning user (sub may have been re-linked); update sub.
		if existing.GoogleSub != nil && *existing.GoogleSub != claims.Sub {
			_ = h.Queries.LinkGoogleSub(ctx, existing.ID, claims.Sub)
		}
		return &googleCallbackResult{
			Outcome: googleOutcomeLinkExistingGoogle,
			User:    existing,
			Email:   emailAddr,
			Sub:     claims.Sub,
		}, nil
	}

	// New user.
	user, err := h.Queries.CreateUserGoogle(ctx, emailAddr, claims.Sub)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}
	return &googleCallbackResult{
		Outcome: googleOutcomeNewUser,
		User:    user,
		Email:   emailAddr,
		Sub:     claims.Sub,
	}, nil
}

func (h *AuthHandler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := readGoogleStateCookie(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid oauth state")
		return
	}
	clearGoogleStateCookie(w)

	result, err := h.processGoogleCallback(
		r.Context(),
		r.URL.Query().Get("code"),
		r.URL.Query().Get("state"),
		stateCookie,
		config.Default.GoogleSSORedirectURL,
	)
	if err != nil {
		// Preserve historical error messages of the redirect handler so any
		// existing observability stays meaningful.
		msg := err.Error()
		switch {
		case strings.Contains(msg, "state mismatch"):
			writeError(w, http.StatusBadRequest, "state mismatch")
		case strings.Contains(msg, "missing code"):
			writeError(w, http.StatusBadRequest, "missing code")
		case strings.Contains(msg, "invalid id token"):
			writeError(w, http.StatusBadRequest, "invalid id token")
		case strings.Contains(msg, "failed to create user"):
			writeError(w, http.StatusInternalServerError, "failed to create user")
		default:
			writeError(w, http.StatusBadRequest, "invalid oauth state")
		}
		return
	}

	base := strings.TrimRight(config.Default.AppBaseURL, "/")

	switch result.Outcome {
	case googleOutcomeLinkRequired:
		http.Redirect(w, r,
			fmt.Sprintf("%s/auth/link-google?email=%s&sub=%s", base, result.Email, result.Sub),
			http.StatusFound)
		return
	case googleOutcomeReturningUser, googleOutcomeLinkExistingGoogle:
		if err := h.signInAndIssue(w, r, result.User); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to sign in")
			return
		}
		http.Redirect(w, r, base+"/dashboard", http.StatusFound)
		return
	case googleOutcomeNewUser:
		if err := h.signInAndIssue(w, r, result.User); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to sign in")
			return
		}
		http.Redirect(w, r, base+"/onboarding", http.StatusFound)
		return
	}
}

// --- Google JSON popup-flow endpoints (auth-google-popup-endpoints) --------

// GetGoogleAuthURL returns the Google consent URL as a JSON body so the SPA
// can open it in a popup without a 302. State + PKCE verifier are persisted
// using the same signed `google_oauth_state` cookie as the redirect flow.
func (h *AuthHandler) GetGoogleAuthURL(w http.ResponseWriter, r *http.Request) {
	if config.Default.GoogleSSOClientID == "" {
		writeError(w, http.StatusServiceUnavailable, "Google SSO is not configured")
		return
	}
	pkce, err := oauth.GenerateGooglePKCE()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start oauth")
		return
	}
	if err := setGoogleStateCookie(w, pkce); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start oauth")
		return
	}
	redirectURI := config.Default.GoogleSSOPopupRedirectURL
	if redirectURI == "" {
		redirectURI = config.Default.GoogleSSORedirectURL
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"auth_url": oauth.AuthorizeURLWithRedirect(pkce, redirectURI),
		"state":    pkce.State,
	})
}

// GoogleCallbackJSON is the popup-flow callback. It accepts `{ code, state }`
// in the request body, runs the same pipeline as the redirect handler, and
// returns the same JSON shape as POST /auth/login on success — never a 302.
func (h *AuthHandler) GoogleCallbackJSON(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code  string `json:"code"`
		State string `json:"state"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Code == "" || req.State == "" {
		writeError(w, http.StatusBadRequest, "code and state are required")
		return
	}

	stateCookie, err := readGoogleStateCookie(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid oauth state")
		return
	}
	clearGoogleStateCookie(w)

	redirectURI := config.Default.GoogleSSOPopupRedirectURL
	if redirectURI == "" {
		redirectURI = config.Default.GoogleSSORedirectURL
	}

	result, err := h.processGoogleCallback(r.Context(), req.Code, req.State, stateCookie, redirectURI)
	if err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "state mismatch"):
			writeError(w, http.StatusBadRequest, "invalid_state")
		case strings.Contains(msg, "missing code"):
			writeError(w, http.StatusBadRequest, "invalid_code")
		case strings.Contains(msg, "invalid id token"):
			writeError(w, http.StatusBadRequest, "invalid_id_token")
		case strings.Contains(msg, "failed to create user"):
			writeError(w, http.StatusInternalServerError, "failed to create user")
		default:
			writeError(w, http.StatusBadRequest, "invalid_state")
		}
		return
	}

	switch result.Outcome {
	case googleOutcomeLinkRequired:
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "link_required",
			"email": result.Email,
			"sub":   result.Sub,
		})
		return
	case googleOutcomeReturningUser, googleOutcomeNewUser, googleOutcomeLinkExistingGoogle:
		access, _, err := auth.IssueAccessToken(result.User.ID, result.User.TokenVersion)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to issue access token")
			return
		}
		refreshRaw, refreshExp, err := auth.IssueRefreshToken(r.Context(), refreshStore{h.Queries}, result.User.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to issue refresh token")
			return
		}
		setRefreshTokenCookie(w, refreshRaw, refreshExp)
		writeJSON(w, http.StatusOK, map[string]string{"access_token": access})
		return
	}
}

// Me returns the authenticated user's profile in the same shape as
// /api/user/me's minimal subset. It runs under the access-token JWT
// middleware only — no email-verification gate, no ACCOUNT_LIFECYCLE flag —
// so it is always available for SPA auth-store hydration.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	user, err := h.Queries.GetUserByID(r.Context(), userID)
	if err != nil || user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":             user.ID,
		"email":          user.Email,
		"email_verified": user.EmailVerifiedAt != nil,
	})
}


// LinkGoogleConfirm sets google_sub on the existing password account once the
// user confirms the link from the link-confirmation view.
func (h *AuthHandler) LinkGoogleConfirm(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
		Sub   string `json:"sub"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = normalizeEmail(req.Email)

	user, err := h.Queries.GetUserByEmail(r.Context(), req.Email)
	if err != nil || user == nil {
		writeError(w, http.StatusBadRequest, "no matching account")
		return
	}
	if err := h.Queries.LinkGoogleSub(r.Context(), user.ID, req.Sub); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to link account")
		return
	}
	if err := h.signInAndIssue(w, r, user); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to sign in")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "linked"})
}

// signInAndIssue issues access + refresh tokens for an authenticated user.
func (h *AuthHandler) signInAndIssue(w http.ResponseWriter, r *http.Request, user *db.User) error {
	if user == nil {
		return errors.New("nil user")
	}
	access, _, err := auth.IssueAccessToken(user.ID, user.TokenVersion)
	if err != nil {
		return err
	}
	refreshRaw, refreshExp, err := auth.IssueRefreshToken(r.Context(), refreshStore{h.Queries}, user.ID)
	if err != nil {
		return err
	}
	setRefreshTokenCookie(w, refreshRaw, refreshExp)
	w.Header().Set("X-Access-Token", access) // for SPAs that want to read it from a redirect target
	return nil
}
