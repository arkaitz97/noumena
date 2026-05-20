package jwksmock_test

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"noumena/internal/dbtest/jwksmock"
)

func TestSignAndVerifyRoundTrip(t *testing.T) {
	srv := jwksmock.NewServer(t)
	now := time.Now().Unix()
	tok, err := srv.SignIDToken(jwksmock.Claims{
		Sub:           "user-123",
		Aud:           "test-audience",
		Iat:           now,
		Exp:           now + 300,
		Email:         "u@example.com",
		EmailVerified: true,
		Nonce:         "n-once",
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 token segments, got %d", len(parts))
	}

	// Verify signature using the server's public key.
	signingInput := parts[0] + "." + parts[1]
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	hash := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(srv.PublicKey, crypto.SHA256, hash[:], sig); err != nil {
		t.Fatalf("signature verification failed: %v", err)
	}

	// Decode payload claims.
	pbytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(pbytes, &got); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if got["sub"] != "user-123" {
		t.Errorf("sub: got %v", got["sub"])
	}
	if got["aud"] != "test-audience" {
		t.Errorf("aud: got %v", got["aud"])
	}
	if got["iss"] != srv.IssuerURL {
		t.Errorf("iss: got %v want %s", got["iss"], srv.IssuerURL)
	}
	if got["nonce"] != "n-once" {
		t.Errorf("nonce: got %v", got["nonce"])
	}
	if got["email_verified"] != true {
		t.Errorf("email_verified: got %v", got["email_verified"])
	}
}

func TestTamperedTokenFailsVerification(t *testing.T) {
	srv := jwksmock.NewServer(t)
	now := time.Now().Unix()
	tok, err := srv.SignIDToken(jwksmock.Claims{
		Sub: "user-123",
		Aud: "aud",
		Iat: now,
		Exp: now + 60,
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("malformed token")
	}

	// Mutate one byte of the payload; signature should no longer verify.
	pbytes, _ := base64.RawURLEncoding.DecodeString(parts[1])
	pbytes[0] ^= 0x01
	parts[1] = base64.RawURLEncoding.EncodeToString(pbytes)
	tampered := strings.Join(parts, ".")

	tParts := strings.Split(tampered, ".")
	signingInput := tParts[0] + "." + tParts[1]
	sig, _ := base64.RawURLEncoding.DecodeString(tParts[2])
	hash := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(srv.PublicKey, crypto.SHA256, hash[:], sig); err == nil {
		t.Fatalf("expected signature verification to fail on tampered token")
	}
}

func TestDiscoveryAndJWKSEndpoints(t *testing.T) {
	srv := jwksmock.NewServer(t)

	// Discovery doc.
	resp, err := httpGet(srv.IssuerURL + "/.well-known/openid-configuration")
	if err != nil {
		t.Fatalf("get discovery: %v", err)
	}
	var disc map[string]interface{}
	if err := json.Unmarshal(resp, &disc); err != nil {
		t.Fatalf("decode discovery: %v", err)
	}
	if disc["issuer"] != srv.IssuerURL {
		t.Errorf("discovery issuer = %v", disc["issuer"])
	}
	if disc["jwks_uri"] != srv.IssuerURL+"/jwks.json" {
		t.Errorf("discovery jwks_uri = %v", disc["jwks_uri"])
	}

	// JWKS doc.
	resp, err = httpGet(srv.IssuerURL + "/jwks.json")
	if err != nil {
		t.Fatalf("get jwks: %v", err)
	}
	var jwks struct {
		Keys []map[string]string `json:"keys"`
	}
	if err := json.Unmarshal(resp, &jwks); err != nil {
		t.Fatalf("decode jwks: %v", err)
	}
	if len(jwks.Keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(jwks.Keys))
	}
	k := jwks.Keys[0]
	if k["kid"] != srv.KeyID {
		t.Errorf("kid = %s", k["kid"])
	}
	if k["alg"] != "RS256" || k["kty"] != "RSA" {
		t.Errorf("alg/kty = %s/%s", k["alg"], k["kty"])
	}
}
