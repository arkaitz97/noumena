package email

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestVerifyURLRejectsEmptySecret proves an empty HMAC key can never
// authenticate, even when the signature is computed with that same empty
// key: the check happens before the exp/decode work.
func TestVerifyURLRejectsEmptySecret(t *testing.T) {
	exp := time.Now().Add(time.Hour).UTC().Unix()
	q := "sig=&exp=0"
	// Compute a signature that would match under an empty secret.
	forged := SignURL("", "user-1", "job-1", time.Unix(exp, 0))
	if forged != "" {
		t.Fatalf("SignURL with empty secret should fail closed, got %q", forged)
	}
	if err := VerifyURL("", "user-1", "job-1", exp, q); err == nil {
		t.Fatal("VerifyURL with empty secret must error")
	}
	// A correctly shaped tuple signed with the empty secret must still fail.
	sig := computeHMACHex("", "user-1", "job-1", exp)
	if err := VerifyURL("", "user-1", "job-1", exp, sig); err == nil {
		t.Fatal("VerifyURL must reject a real signature under an empty secret")
	}
}

// TestSignURLVerifyRealSecretRoundTrip proves a URL signed with a real
// secret verifies successfully.
func TestSignURLVerifyRealSecretRoundTrip(t *testing.T) {
	const secret = "test-secret"
	exp := time.Unix(4102444800, 0) // 2100-01-01, deterministic future.
	q := SignURL(secret, "user-1", "job-1", exp)
	sig, expUnix := splitSigExp(t, q)
	if err := VerifyURL(secret, "user-1", "job-1", expUnix, sig); err != nil {
		t.Fatalf("verify good sig: %v", err)
	}
}

// TestVerifyURLRejectsTamperedSig proves a mutated signature fails.
func TestVerifyURLRejectsTamperedSig(t *testing.T) {
	const secret = "test-secret"
	exp := time.Unix(4102444800, 0)
	q := SignURL(secret, "user-1", "job-1", exp)
	sig, expUnix := splitSigExp(t, q)

	bad := flipHex(sig)
	if bad == sig {
		t.Fatal("flipHex produced identical sig")
	}
	if err := VerifyURL(secret, "user-1", "job-1", expUnix, bad); err == nil {
		t.Fatal("tampered sig must fail")
	}
}

// TestVerifyURLRejectsExpired proves a past exp fails.
func TestVerifyURLRejectsExpired(t *testing.T) {
	const secret = "test-secret"
	expUnix := int64(1) // 1970-01-01, unambiguously in the past.
	sig := computeHMACHex(secret, "user-1", "job-1", expUnix)
	if err := VerifyURL(secret, "user-1", "job-1", expUnix, sig); err == nil {
		t.Fatal("expired exp must fail")
	}
}

func computeHMACHex(secret, userID, jobID string, expUnix int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%s:%s:%d", userID, jobID, expUnix)
	return hex.EncodeToString(mac.Sum(nil))
}

func splitSigExp(t *testing.T, q string) (string, int64) {
	t.Helper()
	parts := strings.Split(q, "&")
	if len(parts) != 2 {
		t.Fatalf("expected sig=...&exp=..., got %q", q)
	}
	sig := strings.TrimPrefix(parts[0], "sig=")
	expStr := strings.TrimPrefix(parts[1], "exp=")
	expUnix, err := ParseExp(expStr)
	if err != nil {
		t.Fatalf("ParseExp(%q): %v", expStr, err)
	}
	return sig, expUnix
}

func flipHex(s string) string {
	if s == "" {
		return "0"
	}
	b := []byte(s)
	if b[0] == '0' {
		b[0] = '1'
	} else {
		b[0] = '0'
	}
	return string(b)
}
