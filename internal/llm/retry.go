package llm

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// transientBackoff is the static retry schedule applied by retryTransient:
// 1s, 2s, 4s between attempts. A call gets len(transientBackoff)+1 total
// attempts (initial + 3 retries), matching the OpenCode client's budget.
var transientBackoff = []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}

// retryTransient runs `attempt` up to len(transientBackoff)+1 times. An
// attempt returns (result, retryAfter, retryable, err): nil err wins
// immediately; non-retryable errors abort; retryable errors wait the next
// static backoff slot (or the server's Retry-After hint when positive)
// unless the context is done. exhaustedMsg prefixes the final wrapped error,
// e.g. "openai: all retries exhausted".
func retryTransient[T any](ctx context.Context, exhaustedMsg string, attempt func() (T, time.Duration, bool, error)) (T, error) {
	var lastErr error
	for attemptNo := 0; attemptNo < len(transientBackoff)+1; attemptNo++ {
		result, retryAfter, retryable, err := attempt()
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !retryable || attemptNo == len(transientBackoff) {
			break
		}
		wait := transientBackoff[attemptNo]
		if retryAfter > 0 {
			wait = retryAfter
		}
		select {
		case <-ctx.Done():
			var zero T
			return zero, ctx.Err()
		case <-time.After(wait):
		}
	}
	var zero T
	return zero, fmt.Errorf("%s: %w", exhaustedMsg, lastErr)
}

// httpClassifyOnce performs a single HTTP POST attempt against a chat-style
// endpoint and mirrors the OpenCode client's transport semantics exactly:
//   - transport-level errors are retryable
//   - HTTP 429 and 5xx are retryable and surface the Retry-After hint
//   - other 4xx are non-retryable (auth, malformed request, ...)
//   - decode errors are non-retryable and returned verbatim
//
// provider prefixes error messages (e.g. "openai"); setAuth injects the
// provider-specific auth headers; decode maps a 2xx body to T.
func httpClassifyOnce[T any](ctx context.Context, hc *http.Client, endpoint string, body []byte, provider string, setAuth func(http.Header), decode func(io.Reader) (T, error)) (T, time.Duration, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		var zero T
		return zero, 0, false, fmt.Errorf("%s: build request: %w", provider, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if setAuth != nil {
		setAuth(req.Header)
	}

	resp, err := hc.Do(req)
	if err != nil {
		var zero T
		return zero, 0, true, fmt.Errorf("%s: transport: %w", provider, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		retryAfter := parseRetryAfterSeconds(resp.Header.Get("Retry-After"))
		_, _ = io.Copy(io.Discard, resp.Body)
		var zero T
		return zero, retryAfter, true, fmt.Errorf("%s: HTTP %d", provider, resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		var zero T
		return zero, 0, false, fmt.Errorf("%s: HTTP %d: %s", provider, resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	result, derr := decode(resp.Body)
	if derr != nil {
		var zero T
		return zero, 0, false, derr
	}
	return result, 0, false, nil
}

// parseRetryAfterSeconds returns the Retry-After value as a duration when the
// header is a positive integer count of seconds. HTTP-date values are ignored.
func parseRetryAfterSeconds(h string) time.Duration {
	h = strings.TrimSpace(h)
	if h == "" {
		return 0
	}
	if n, err := strconv.Atoi(h); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return 0
}
