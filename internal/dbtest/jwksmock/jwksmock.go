// Package jwksmock provides a lightweight OIDC issuer + JWKS HTTP server
// suitable for testing OAuth/OIDC ID-token verification.
//
// Each NewServer instance generates an ephemeral RSA-2048 keypair and
// exposes:
//
//   - GET /.well-known/openid-configuration  (OIDC discovery)
//   - GET /jwks.json                         (JSON Web Key Set)
//
// Callers obtain signed RS256 ID tokens via Server.SignIDToken(claims).
// Tests inject Server.IssuerURL into code under test (e.g. by overriding
// the issuer/JWKS URL configuration of the OAuth handler) and verify that
// the handler accepts well-formed tokens and rejects tampered ones.
package jwksmock

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// Server is a running JWKS+OIDC mock issuer.
type Server struct {
	// IssuerURL is the absolute base URL of this server. Suitable for the
	// `iss` claim and as the `issuer` value in OIDC discovery.
	IssuerURL string

	// PrivateKey is the RSA key used to sign tokens.
	PrivateKey *rsa.PrivateKey

	// PublicKey is the corresponding public key (also published in JWKS).
	PublicKey *rsa.PublicKey

	// KeyID is the `kid` value used in token headers and the JWKS entry.
	KeyID string

	httpSrv *httptest.Server
}

// NewServer starts a new JWKS mock issuer and registers cleanup on t.
func NewServer(t *testing.T) *Server {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("jwksmock: generate key: %v", err)
	}
	s := &Server{
		PrivateKey: priv,
		PublicKey:  &priv.PublicKey,
		KeyID:      fmt.Sprintf("jwksmock-key-%d", nextKeyCounter()),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", s.handleDiscovery)
	mux.HandleFunc("/jwks.json", s.handleJWKS)
	s.httpSrv = httptest.NewServer(mux)
	s.IssuerURL = s.httpSrv.URL
	t.Cleanup(s.Close)
	return s
}

// Close shuts down the HTTP server. Safe to call multiple times.
func (s *Server) Close() {
	if s.httpSrv != nil {
		s.httpSrv.Close()
	}
}

// Claims is the minimum set of claims SignIDToken supports. Additional
// fields can be added via the Extra map.
type Claims struct {
	Iss           string                 `json:"iss"`
	Sub           string                 `json:"sub"`
	Aud           string                 `json:"aud"`
	Exp           int64                  `json:"exp"`
	Iat           int64                  `json:"iat"`
	Nonce         string                 `json:"nonce,omitempty"`
	Email         string                 `json:"email,omitempty"`
	EmailVerified bool                   `json:"email_verified,omitempty"`
	Extra         map[string]interface{} `json:"-"`
}

// SignIDToken signs the given claims as an RS256 JWT. The returned token
// has three base64url-encoded segments separated by `.`.
func (s *Server) SignIDToken(c Claims) (string, error) {
	if c.Iss == "" {
		c.Iss = s.IssuerURL
	}
	header := map[string]string{
		"alg": "RS256",
		"typ": "JWT",
		"kid": s.KeyID,
	}
	hb, err := json.Marshal(header)
	if err != nil {
		return "", err
	}

	// Merge Extra into the claim map so callers can add arbitrary keys.
	cMap := map[string]interface{}{
		"iss": c.Iss,
		"sub": c.Sub,
		"aud": c.Aud,
		"exp": c.Exp,
		"iat": c.Iat,
	}
	if c.Nonce != "" {
		cMap["nonce"] = c.Nonce
	}
	if c.Email != "" {
		cMap["email"] = c.Email
	}
	if c.EmailVerified {
		cMap["email_verified"] = true
	}
	for k, v := range c.Extra {
		cMap[k] = v
	}
	pb, err := json.Marshal(cMap)
	if err != nil {
		return "", err
	}

	signingInput := base64.RawURLEncoding.EncodeToString(hb) + "." +
		base64.RawURLEncoding.EncodeToString(pb)
	hash := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.PrivateKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", err
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// ----------------------------------------------------------------------------
// HTTP handlers
// ----------------------------------------------------------------------------

func (s *Server) handleDiscovery(w http.ResponseWriter, _ *http.Request) {
	doc := map[string]interface{}{
		"issuer":                                s.IssuerURL,
		"jwks_uri":                              s.IssuerURL + "/jwks.json",
		"response_types_supported":              []string{"id_token"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(doc)
}

func (s *Server) handleJWKS(w http.ResponseWriter, _ *http.Request) {
	pub := s.PublicKey
	n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	// Encode E as big-endian bytes with leading zeros stripped.
	eBig := big.NewInt(int64(pub.E))
	e := base64.RawURLEncoding.EncodeToString(eBig.Bytes())
	doc := map[string]interface{}{
		"keys": []map[string]string{{
			"kty": "RSA",
			"alg": "RS256",
			"use": "sig",
			"kid": s.KeyID,
			"n":   n,
			"e":   e,
		}},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(doc)
}

// keyCounter ensures unique `kid` values across multiple NewServer calls
// in the same test process.
var keyCounter int64

func nextKeyCounter() int64 { return atomic.AddInt64(&keyCounter, 1) }
