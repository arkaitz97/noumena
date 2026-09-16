package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// InsightsStore is the narrow data-access surface for comment processing and
// insights. Methods whose richer variants live only on *SQLiteQuery (feed
// queries, exports, aggregates) are intentionally not part of this contract.
type InsightsStore interface {
	GetCommentFeed(ctx context.Context, userID string, limit, offset int) ([]*Comment, error)
	StarComment(ctx context.Context, userID, commentID string) error
	InsertComment(ctx context.Context, c *Comment) error
	GetUnprocessedComments(ctx context.Context, limit int) ([]*Comment, error)
	GetUnprocessedCommentsByChannel(ctx context.Context, limit int, maxAttempts int) (map[string][]*Comment, error)
	UpdateCommentLLMData(ctx context.Context, id, category, topic, provider, model string, actionableInsights []byte, confidence float64) error

	IncrementCommentsUsed(ctx context.Context, userID string, count int) error
	GetPrimaryTracker(ctx context.Context, platformChannelID string) (*User, error)

	GetChannelCustomPrompt(ctx context.Context, platformChannelID, userID string) (string, error)
	GetCommentBodyByID(ctx context.Context, commentID string) (string, error)
}

var _ InsightsStore = (DBQuery)(nil)

// InsightsQuery is the full surface the insights handlers need. Unlike
// InsightsStore it carries the concrete-only feed/export/aggregate methods
// added by the insights-api change. It is deliberately NOT embedded in
// DBQuery: those methods are implemented only by *SQLiteQuery, so embedding
// them would force every DBQuery consumer and MockQuery to implement them.
type InsightsQuery interface {
	// Feed queries (BuildFeedQuery-backed, concrete-only).
	QueryCommentFeed(ctx context.Context, userID string, filters FeedFilters, timeout time.Duration) ([]*FeedRow, error)
	QuerySavedFeed(ctx context.Context, userID string, filters FeedFilters, timeout time.Duration) ([]*FeedRow, error)
	CountCommentFeed(ctx context.Context, userID string, filters FeedFilters, timeout time.Duration) (int64, error)
	StarCommentScoped(ctx context.Context, userID, commentID string) (bool, error)
	UnstarComment(ctx context.Context, userID, commentID string) (bool, error)
	UserTracksChannel(ctx context.Context, userID, channelID string) (bool, error)

	// Search / thread / bulk mutations.
	SearchComments(ctx context.Context, userID, query string) ([]*Comment, error)
	GetCommentThread(ctx context.Context, userID, commentID string, maxDepth int) ([]*Comment, error)
	BulkStarComments(ctx context.Context, userID string, commentIDs []string) (int, error)
	BulkUnstarComments(ctx context.Context, userID string, commentIDs []string) (int, error)

	// Insights aggregates.
	GetTopInsights(ctx context.Context, userID string, filters FeedFilters, limit int) ([]TopInsight, error)
	GetTopCategories(ctx context.Context, userID string, channelID *string, days int) ([]CategoryRow, error)
	GetTopTopics(ctx context.Context, userID string, channelID *string, days int, limit int) ([]TopicRow, error)
	GetInsightDistribution(ctx context.Context, userID string, filters FeedFilters) ([]InsightTypeCount, error)
	GetSentimentSeries(ctx context.Context, userID, channelID string, days int) ([]SentimentDay, error)
	GetToxicityShield(ctx context.Context, userID string, channelID *string, since time.Time) (ToxicityBucket, error)
	GetLatestDigest(ctx context.Context, userID string) (*LatestDigest, error)

	// Exports.
	EnqueueExport(ctx context.Context, userID, format string, filters json.RawMessage) (*Export, error)
	GetExport(ctx context.Context, userID, exportID string) (*Export, error)
	ListExports(ctx context.Context, userID string, limit int) ([]*Export, error)
	MarkExportCompleted(ctx context.Context, exportID, filePath string, rowCount int64, ttlHours int) error
	MarkExportFailed(ctx context.Context, exportID, errMsg string, retryable bool) error

	// Raw handles needed for feed query building and export streaming.
	DB() *sql.DB
	ExportReadDB() *sql.DB
}

var _ InsightsQuery = (*SQLiteQuery)(nil)
