package db

import (
	"context"
	"time"
)

// RateLimitStore is the narrow data-access surface for rate limiting.
type RateLimitStore interface {
	IncrementRateLimit(ctx context.Context, userKey string, windowStart time.Time, limit int) (count int, err error)
	GetRateLimitStatus(ctx context.Context, userKey string, windowStart time.Time, limit int) (*RateLimitStatus, error)
	CleanupExpiredRateLimitBuckets(ctx context.Context, before time.Time) (int64, error)
}

var _ RateLimitStore = (DBQuery)(nil)
