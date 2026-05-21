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
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cfg.RateLimitEnabled {
				next.ServeHTTP(w, r)
				return
			}

			if rateLimitBypassPaths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			userKey := extractRateLimitKey(r)
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

func extractRateLimitKey(r *http.Request) string {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if ok && userID != "" {
		return "u:" + userID
	}

	ip := extractIP(r)
	if ip == "" {
		return ""
	}
	hash := sha256.Sum256([]byte(ip))
	return "ip:" + hex.EncodeToString(hash[:])
}

func extractIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func isAuthenticated(r *http.Request) bool {
	userID, ok := r.Context().Value(UserIDKey).(string)
	return ok && userID != ""
}

type RateLimitCleaner struct {
	Queries    db.DBQuery
	Tick       time.Duration
	StopC      chan struct{}
	running    bool
	mu         sync.Mutex
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
