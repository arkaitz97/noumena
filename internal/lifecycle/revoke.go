// Package lifecycle owns the data-lifecycle change's runtime: account-
// deletion worker, GDPR-export worker, daily export-cleanup cron, plus
// the helpers they share (OAuth revocation, retry/backoff). The HTTP
// surface (DELETE /api/user/account, GET /api/user/export, etc.) lives
// in internal/api and calls into this package via the worker
// constructors.
//
// Why a separate package (vs adding to internal/api)?
//
//   - The workers are long-lived goroutines independent of the request
//     lifecycle; keeping them out of internal/api keeps that package
//     focused on HTTP framing.
//   - The deletion path touches every other table; isolating it makes
//     the destructive surface easy to audit (`grep -r "internal/lifecycle"`).
//   - Tests can exercise the worker logic without spinning up the API
//     server.
//
// File: revoke.go contains the OAuth-revocation helper used by the
// deletion worker before it drops a user's tracker. Best-effort by
// design — see RevokeOAuthToken doc comment.
package lifecycle

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"
)

// RevokeOAuthToken asks the provider to invalidate the supplied access
// token. Returns nil on success OR on a documented "skip" path (non-
// YouTube platforms) so the caller can proceed with deletion regardless.
//
// Spec contract (Section 7.1-7.3):
//   - YouTube → POST https://oauth2.googleapis.com/revoke?token=<t>
//     with a 10s timeout. 200 OR 400 (already-invalid) treated as
//     success; everything else is an error.
//   - Other platforms → log "oauth_revoke_skipped platform=<p>" and
//     return nil. We DO NOT block deletion for platforms whose
//     revocation endpoint we haven't implemented; the user's intent
//     (delete account) overrides the nice-to-have side effect (revoke
//     token). The token will expire on its own anyway.
//
// The helper deliberately uses an *http.Client constructed per call
// (rather than a package-global) so tests can inject a transport
// without mutating shared state.
func RevokeOAuthToken(ctx context.Context, platform, accessToken string, logger *log.Logger) error {
	if logger == nil {
		logger = log.Default()
	}
	switch platform {
	case "youtube", "google", "youtube_data_v3":
		return revokeGoogleToken(ctx, accessToken, logger)
	default:
		logger.Printf("oauth_revoke_skipped platform=%s reason=unsupported", platform)
		return nil
	}
}

func revokeGoogleToken(ctx context.Context, accessToken string, logger *log.Logger) error {
	endpoint := "https://oauth2.googleapis.com/revoke"
	form := url.Values{"token": {accessToken}}

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint+"?"+form.Encode(), nil)
	if err != nil {
		return fmt.Errorf("oauth revoke: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("oauth revoke: %w", err)
	}
	defer resp.Body.Close()

	// Google returns 200 on success. 400 is documented for already-
	// invalid/expired tokens, which is a no-op success from our
	// perspective. Any other status is an error worth retrying.
	switch {
	case resp.StatusCode == http.StatusOK:
		logger.Printf("oauth_revoke_ok platform=youtube")
		return nil
	case resp.StatusCode == http.StatusBadRequest:
		logger.Printf("oauth_revoke_already_invalid platform=youtube status=400")
		return nil
	default:
		return fmt.Errorf("oauth revoke: unexpected status %d", resp.StatusCode)
	}
}

// RevokeWithRetry wraps RevokeOAuthToken with the spec-mandated
// 1m / 1h / 6h backoff (three attempts total). On final failure it
// emits a structured log line and returns the last error so the caller
// can decide whether to fail the deletion job (the deletion worker
// chooses to PROCEED — see deletion_worker.go).
//
// The loop honors ctx cancellation between attempts; if the worker is
// shutting down we give up immediately rather than burning a sleep.
func RevokeWithRetry(ctx context.Context, platform, accessToken string, logger *log.Logger) error {
	if logger == nil {
		logger = log.Default()
	}
	backoffs := []time.Duration{0, time.Minute, time.Hour, 6 * time.Hour}
	var lastErr error
	for attempt, wait := range backoffs {
		if wait > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		}
		err := RevokeOAuthToken(ctx, platform, accessToken, logger)
		if err == nil {
			return nil
		}
		lastErr = err
		logger.Printf("oauth_revoke_retry platform=%s attempt=%d err=%v", platform, attempt+1, err)
	}
	logger.Printf("oauth_revoke_giving_up platform=%s err=%v", platform, lastErr)
	return lastErr
}
