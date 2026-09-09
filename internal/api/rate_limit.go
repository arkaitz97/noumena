package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"noumena/internal/config"
	"noumena/internal/db"
)

type rateLimitCtxKey struct{}

type RateLimitInfo struct {
	Limit     int       `json:"limit"`
	Remaining int       `json:"remaining"`
	ResetAt   time.Time `json:"reset_at"`
}

var (
	rateLimitBypassPaths = map[string]bool{
		"/health":          true,
		"/ready":           true,
		"/webhooks/stripe": true,
		"/metrics":         true,
	}
)

func RateLimitMiddleware(queries db.DBQuery, cfg *config.Config) func(http.Handler) http.Handler {
	return rateLimitMiddleware(queries, cfg, false)
}

// AuthRateLimitMiddleware rate-limits endpoints that must always be
// protected regardless of the global RATE_LIMIT_ENABLED flag (login,
// register, verify, reset, google flows, invite accept). Unauthenticated
// requests are keyed by the trusted client IP at the unauth rate;
// authenticated requests (middleware installed after auth) get a per-user
// bucket. When queries is nil (degraded mock mode) it passes through.
func AuthRateLimitMiddleware(queries db.DBQuery, cfg *config.Config) func(http.Handler) http.Handler {
	return rateLimitMiddleware(queries, cfg, true)
}

// rateLimitMiddleware implements the shared limiter. forceEnabled makes
// the limiter run even when cfg.RateLimitEnabled is false — used for auth
// endpoints where protection must be on by default.
func rateLimitMiddleware(queries db.DBQuery, cfg *config.Config, forceEnabled bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if queries == nil {
				next.ServeHTTP(w, r)
				return
			}
			if !forceEnabled && !cfg.RateLimitEnabled {
				next.ServeHTTP(w, r)
				return
			}

			if rateLimitBypassPaths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			userKey := extractRateLimitKey(r, cfg)
			if userKey == "" {
				next.ServeHTTP(w, r)
				return
			}

			windowStart := time.Now().Truncate(time.Minute)
			limit := cfg.RateLimitAuthMaxPerMin
			if !isAuthenticated(r) {
				limit = cfg.RateLimitUnauthMaxPerMin
			}

			count, err := queries.IncrementRateLimit(r.Context(), userKey, windowStart, limit)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			remaining := limit - count
			if remaining < 0 {
				remaining = 0
			}

			resetAt := windowStart.Add(time.Minute)
			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
			w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
			w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetAt.Unix()))

			if count > limit {
				retryAfter := int(time.Until(resetAt).Seconds())
				if retryAfter < 1 {
					retryAfter = 1
				}
				w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":"rate limit exceeded","retry_after":` + fmt.Sprintf("%d", retryAfter) + `}`))
				return
			}

			ctx := context.WithValue(r.Context(), rateLimitCtxKey{}, &RateLimitInfo{
				Limit:     limit,
				Remaining: remaining,
				ResetAt:   resetAt,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractRateLimitKey(r *http.Request, cfg *config.Config) string {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if ok && userID != "" {
		return "u:" + userID
	}

	ip := trustedClientIP(r, cfg.TrustProxyHeader)
	if ip == "" {
		return ""
	}
	hash := sha256.Sum256([]byte(ip))
	return "ip:" + hex.EncodeToString(hash[:])
}

// trustedClientIP derives the client IP from a source the client cannot
// spoof. When a TRUST_PROXY_HEADER is configured the value of that single
// header (set by the reverse proxy) is used: the right-most comma-separated
// entry is the peer the proxy observed, and it must parse as a valid IP.
// Client-supplied X-Forwarded-For / X-Real-IP are ignored entirely unless
// explicitly configured. With no trusted header configured, RemoteAddr is
// used. Returns "" when no trustworthy source can be resolved (the caller
// treats that as "do not rate limit").
func trustedClientIP(r *http.Request, headerName string) string {
	if headerName != "" {
		if v := r.Header.Get(headerName); v != "" {
			parts := strings.Split(v, ",")
			candidate := strings.TrimSpace(parts[len(parts)-1])
			if net.ParseIP(candidate) != nil {
				return candidate
			}
			return ""
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if net.ParseIP(host) == nil {
		return ""
	}
	return host
}

func isAuthenticated(r *http.Request) bool {
	userID, ok := r.Context().Value(UserIDKey).(string)
	return ok && userID != ""
}

type RateLimitCleaner struct {
	Queries db.DBQuery
	Tick    time.Duration
	StopC   chan struct{}
	running bool
	mu      sync.Mutex
}

func (c *RateLimitCleaner) Start(ctx context.Context) {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return
	}
	c.running = true
	c.StopC = make(chan struct{})
	c.mu.Unlock()

	go c.run(ctx)
}

func (c *RateLimitCleaner) run(ctx context.Context) {
	ticker := time.NewTicker(c.Tick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.StopC:
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-2 * time.Minute)
			deleted, err := c.Queries.CleanupExpiredRateLimitBuckets(ctx, cutoff)
			if err != nil {
				continue
			}
			if deleted > 0 {
			}
		}
	}
}

func (c *RateLimitCleaner) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		close(c.StopC)
		c.running = false
	}
}
