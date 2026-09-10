package llm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// TerminalError marks an error as non-retryable (deterministic failure).
// HTTP 4xx except 429, auth failures, JSON decode/parse errors are terminal.
type TerminalError struct{ error }

func (e *TerminalError) Unwrap() error { return e.error }

// IsTerminal reports whether err (or any error in its chain) is a TerminalError.
func IsTerminal(err error) bool {
	var term *TerminalError
	return errors.As(err, &term)
}

// terminal wraps err as a TerminalError.
func terminal(err error) error {
	if err == nil {
		return nil
	}
	if IsTerminal(err) {
		return err
	}
	return &TerminalError{err}
}

// transientBackoff is the static retry schedule applied by retryTransient:
// 1s, 2s, 4s between attempts. A call gets len(transientBackoff)+1 total
// attempts (initial + 3 retries), matching the OpenCode client's budget.
var transientBackoff = []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}

// retryTransient runs `attempt` up to len(transientBackoff)+1 times.
// Each attempt receives a context with a deadline equal to the remaining
// caller context budget divided by the number of attempts left.
// Server Retry-After is honored only when shorter than that share.
// Every HTTP attempt increments noumena_llm_provider_calls_total{provider,retryable}.
// exhaustedMsg prefixes the final wrapped error, e.g. "openai: all retries exhausted".
func retryTransient[T any](ctx context.Context, provider, exhaustedMsg string, attempt func(ctx context.Context) (T, time.Duration, bool, error)) (T, error) {
	var lastErr error
	maxAttempts := len(transientBackoff) + 1
	for attemptNo := 0; attemptNo < maxAttempts; attemptNo++ {
		remainingAttempts := maxAttempts - attemptNo
		var attemptCtx context.Context
		var cancel context.CancelFunc
		if deadline, ok := ctx.Deadline(); ok {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				var zero T
				return zero, ctx.Err()
			}
			perAttempt := remaining / time.Duration(remainingAttempts)
			attemptCtx, cancel = context.WithTimeout(ctx, perAttempt)
		} else {
			attemptCtx, cancel = context.WithCancel(ctx)
		}

		result, retryAfter, retryable, err := attempt(attemptCtx)
		recordLLMProviderCall(ctx, provider, retryable)
		cancel()

		if err == nil {
			return result, nil
		}
		lastErr = err
		if !retryable || attemptNo == maxAttempts-1 {
			break
		}
		wait := transientBackoff[attemptNo]
		if retryAfter > 0 && retryAfter < wait {
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
		return zero, 0, false, terminal(fmt.Errorf("%s: HTTP %d: %s", provider, resp.StatusCode, strings.TrimSpace(string(snippet))))
	}

	result, derr := decode(resp.Body)
	if derr != nil {
		var zero T
		return zero, 0, false, terminal(derr)
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
