package db

import (
	"context"
	"time"
)

// DigestStore is the narrow data-access surface for weekly digests.
type DigestStore interface {
	InsertWeeklyDigest(ctx context.Context, digest *WeeklyDigest) error
	GetUsersForDigest(ctx context.Context) ([]*User, error)
	GetRecentProcessedCommentsForUser(ctx context.Context, userID string, since time.Time) ([]*Comment, error)
	GetEligibleCommentsForUser(ctx context.Context, userID string, since, until time.Time) ([]*Comment, error)
	InsertWeeklyDigestPending(ctx context.Context, userID string, weekStart time.Time) (string, bool, error)
	UpdateDigestContent(ctx context.Context, digestID string, contentJSON []byte, status string) error
	UpdateDigestStatus(ctx context.Context, digestID, status string) error
	GetPreviousWeekDigest(ctx context.Context, userID string, weekStart time.Time) (*WeeklyDigest, error)
	GetLatestDigestForUser(ctx context.Context, userID string) (*WeeklyDigest, error)
	ListDigests(ctx context.Context, userID string, limit, offset int) ([]*WeeklyDigest, error)
	GetStalePendingDigests(ctx context.Context, olderThan time.Time) ([]*WeeklyDigest, error)
	GetDeferredDigestsDue(ctx context.Context, now time.Time, limit int) ([]*WeeklyDigest, error)
	GetWeeklyDigestByID(ctx context.Context, digestID string) (*WeeklyDigest, error)
	GetMissingDigestWeeks(ctx context.Context, userID string, weeks []time.Time) ([]time.Time, error)
	UpdateUserDigestOptIn(ctx context.Context, userID string, optIn bool) error
	MarkDigestEmailed(ctx context.Context, digestID, userID string, at time.Time) error
}

var _ DigestStore = (DBQuery)(nil)
