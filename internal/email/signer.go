// Package email: signer.go is the HMAC-SHA256 URL signer used by the
// export-download endpoint. Lives in the email package only because the
// export_ready email is the primary consumer of signed URLs; the lifecycle
// package re-exports the same primitive (lifecycle/signed_url.go) so the
// dependency direction stays clean.
//
// Wire format: ?sig=<hex(hmac)>&exp=<unix>
// Canonical message: "<user_id>:<job_id>:<exp_unix>"
//
// Verify rejects when:
//   - sig is missing or not lowercase hex
//   - exp is missing, malformed, or in the past
//   - HMAC does not match (constant-time compare)
package email

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// SignURL returns "?sig=<hex>&exp=<unix>" for the given (userID, jobID, exp).
// The caller is responsible for prefixing the path; this helper only
// produces the query-string suffix so it can be appended to any base.
//
// secret MUST be non-empty: SignURL cannot report an error, so an empty
// secret would otherwise mint a signature under a publicly-known key.
// As defence-in-depth it fails closed by returning "" (no signature),
// which VerifyURL will reject, and VerifyURL independently rejects an
// empty secret before doing any HMAC work.
func SignURL(secret, userID, jobID string, exp time.Time) string {
	if secret == "" {
		return ""
	}
	expUnix := exp.UTC().Unix()
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%s:%s:%d", userID, jobID, expUnix)
	return fmt.Sprintf("sig=%s&exp=%d", hex.EncodeToString(mac.Sum(nil)), expUnix)
}

// VerifyURL returns nil when the (userID, jobID, expUnix, sigHex) tuple
// authenticates and the deadline has not passed. Errors are intentionally
// terse — callers should not surface the distinction to end users (any
// failure is a 403 to avoid leaking which leg of the check failed).
func VerifyURL(secret, userID, jobID string, expUnix int64, sigHex string) error {
	if secret == "" {
		return errors.New("invalid secret")
	}
	if expUnix <= 0 {
		return errors.New("invalid exp")
	}
	if time.Now().UTC().Unix() > expUnix {
		return errors.New("expired")
	}
	want, err := hex.DecodeString(sigHex)
	if err != nil {
		return errors.New("invalid sig")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%s:%s:%d", userID, jobID, expUnix)
	got := mac.Sum(nil)
	if !hmac.Equal(want, got) {
		return errors.New("invalid sig")
	}
	return nil
}

// ParseExp parses an exp= query value into a Unix timestamp. Helper used
// by the download handler so the parsing live next to the verify path.
func ParseExp(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}
