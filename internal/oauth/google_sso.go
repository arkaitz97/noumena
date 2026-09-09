package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"noumena/internal/config"
)

// GooglePKCE holds a single PKCE challenge.
type GooglePKCE struct {
	State        string
	Verifier     string
	Challenge    string
	ChallengeAlg string // always "S256"
}

// GenerateGooglePKCE creates a fresh state + PKCE verifier/challenge pair.
func GenerateGooglePKCE() (*GooglePKCE, error) {
	state, err := randURLSafe(24)
	if err != nil {
		return nil, err
	}
	verifier, err := randURLSafe(48) // ~64 chars after base64url
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	return &GooglePKCE{
		State:        state,
		Verifier:     verifier,
		Challenge:    challenge,
		ChallengeAlg: "S256",
	}, nil
}

func randURLSafe(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// AuthorizeURL builds the Google authorization endpoint URL for PKCE using
// the redirect-flow `redirect_uri` from configuration.
func AuthorizeURL(p *GooglePKCE) string {
	return AuthorizeURLWithRedirect(p, config.Default.GoogleSSORedirectURL)
}

// AuthorizeURLWithRedirect builds the Google authorization endpoint URL for
// PKCE with a caller-supplied `redirect_uri`. The popup-based JSON flow
// (auth-google-popup-endpoints change) uses this with the SPA-side
// `/oauth/callback` URI; the redirect flow uses AuthorizeURL.
func AuthorizeURLWithRedirect(p *GooglePKCE, redirectURI string) string {
	v := url.Values{}
	v.Set("client_id", config.Default.GoogleSSOClientID)
	v.Set("redirect_uri", redirectURI)
	v.Set("response_type", "code")
	v.Set("scope", "openid email profile")
	v.Set("state", p.State)
	v.Set("code_challenge", p.Challenge)
	v.Set("code_challenge_method", p.ChallengeAlg)
	v.Set("access_type", "online")
	v.Set("prompt", "select_account")
	return "https://accounts.google.com/o/oauth2/v2/auth?" + v.Encode()
}

// IDClaims is the subset of an ID token we need.
type IDClaims struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Aud           string `json:"aud"`
	Iss           string `json:"iss"`
	jwt.RegisteredClaims
}

// ExchangeAndVerify exchanges the authorization code for tokens (using the
// PKCE verifier) and verifies the resulting ID token's `aud`, `iss`, `exp`,
// and signature. Returns the raw (verified) ID token and its claims.
func ExchangeAndVerify(ctx context.Context, code, codeVerifier string) (string, *IDClaims, error) {
	return ExchangeAndVerifyWithRedirect(ctx, code, codeVerifier, config.Default.GoogleSSORedirectURL)
}

// ExchangeAndVerifyWithRedirect is like ExchangeAndVerify but allows the
// caller to specify the `redirect_uri` for the token-exchange request. Google
// requires the redirect_uri sent with the code exchange to match the one used
// in the original authorization request, so the popup JSON flow must echo its
// SPA-side redirect URI here.
func ExchangeAndVerifyWithRedirect(ctx context.Context, code, codeVerifier, redirectURI string) (string, *IDClaims, error) {
	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", config.Default.GoogleSSOClientID)
	form.Set("client_secret", config.Default.GoogleSSOClientSecret)
	form.Set("redirect_uri", redirectURI)
	form.Set("grant_type", "authorization_code")
	form.Set("code_verifier", codeVerifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://oauth2.googleapis.com/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c := &http.Client{Timeout: 10 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("google token exchange: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", nil, fmt.Errorf("google token exchange failed: %d %s", resp.StatusCode, string(body))
	}
	var tok struct {
		IDToken string `json:"id_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", nil, err
	}
	if tok.IDToken == "" {
		return "", nil, errors.New("no id_token in google response")
	}
	claims, err := VerifyIDToken(ctx, tok.IDToken)
	if err != nil {
		return "", nil, err
	}
	return tok.IDToken, claims, nil
}

// VerifyIDToken verifies a Google ID token's signature against Google's
// JWKS and validates aud, iss, exp.
func VerifyIDToken(ctx context.Context, idToken string) (*IDClaims, error) {
	keyset, err := fetchGoogleJWKS(ctx)
	if err != nil {
		return nil, err
	}
	parsed, err := jwt.ParseWithClaims(idToken, &IDClaims{}, func(t *jwt.Token) (interface{}, error) {
		kid, _ := t.Header["kid"].(string)
		key, ok := keyset[kid]
		if !ok {
			return nil, fmt.Errorf("unknown kid %q", kid)
		}
		return key, nil
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithLeeway(30*time.Second))
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(*IDClaims)
	if !ok || !parsed.Valid {
		return nil, errors.New("invalid id token")
	}
	if claims.Aud != config.Default.GoogleSSOClientID {
		return nil, fmt.Errorf("id token aud mismatch")
	}
	if claims.Iss != "https://accounts.google.com" && claims.Iss != "accounts.google.com" {
		return nil, fmt.Errorf("id token iss mismatch")
	}
	if claims.Email == "" || claims.Sub == "" {
		return nil, fmt.Errorf("id token missing email or sub")
	}
	return claims, nil
}
