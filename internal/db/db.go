package db

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var Pool *pgxpool.Pool

// InitDB initializes the database connection pool
func InitDB(databaseUrl string) error {
	var err error
	Pool, err = pgxpool.New(context.Background(), databaseUrl)
	if err != nil {
		return fmt.Errorf("unable to create database pool: %w", err)
	}

	// Ping the DB
	if err := Pool.Ping(context.Background()); err != nil {
		return fmt.Errorf("unable to ping database: %w", err)
	}

	log.Println("Successfully connected to the PostgreSQL database")
	return nil
}

// CloseDB closes the database connection pool
func CloseDB() {
	if Pool != nil {
		Pool.Close()
	}
}

// Ensure interface for test mockability
type DBQuery interface {
	CreateUser(ctx context.Context, email, passwordHash string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByID(ctx context.Context, id string) (*User, error)
	UpdateUserSubscription(ctx context.Context, userID, plan, stripeCustomerID string, quota int) error
	
	// Channels
	ListChannelsForUser(ctx context.Context, userID string) ([]*PlatformChannel, error)
	ConnectChannelTx(ctx context.Context, userID, platform, platformChannelID, displayName, accessTokenEnc, refreshTokenEnc string, tokenExpiresAt *time.Time) error
	PauseChannelTracking(ctx context.Context, userID, platformChannelID string) error
	RemoveChannelTracking(ctx context.Context, userID, platformChannelID string) error
	
	GetChannelsForSync(ctx context.Context, minutes int) ([]*PlatformChannel, error)
	UpdateSyncTime(ctx context.Context, id string) error

	// Comments
	GetCommentFeed(ctx context.Context, userID string, limit, offset int) ([]*Comment, error)
	StarComment(ctx context.Context, userID, commentID string) error
	InsertComment(ctx context.Context, c *Comment) error
	GetUnprocessedComments(ctx context.Context, limit int) ([]*Comment, error)
	UpdateCommentLLMData(ctx context.Context, id, category, topic string, sentiment, confidence float64) error

	// Digests
	InsertWeeklyDigest(ctx context.Context, digest *WeeklyDigest) error
	GetUsersForDigest(ctx context.Context) ([]*User, error)
	GetRecentProcessedCommentsForUser(ctx context.Context, userID string, since time.Time) ([]*Comment, error)

	// Tracking
	GetIngestionToken(ctx context.Context, platformChannelID string) (string, string, error)
	PromoteNextIngestionToken(ctx context.Context, platformChannelID string) error
}

// PostgresQuery implements DBQuery using pgxpool
type PostgresQuery struct {
	pool *pgxpool.Pool
}

func NewPostgresQuery(p *pgxpool.Pool) DBQuery {
	return &PostgresQuery{pool: p}
}

func (q *PostgresQuery) CreateUser(ctx context.Context, email, passwordHash string) (*User, error) {
	query := `
		INSERT INTO users (email, password_hash)
		VALUES ($1, $2)
		RETURNING id, email, password_hash, subscription_plan, comment_quota_monthly, comments_used_this_period, created_at, updated_at
	`
	user := &User{}
	err := q.pool.QueryRow(ctx, query, email, passwordHash).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.SubscriptionPlan,
		&user.CommentQuotaMonthly,
		&user.CommentsUsedThisPeriod,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (q *PostgresQuery) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	query := `
		SELECT id, email, password_hash, subscription_plan, comment_quota_monthly, comments_used_this_period, created_at, updated_at
		FROM users
		WHERE email = $1
	`
	user := &User{}
	err := q.pool.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.SubscriptionPlan,
		&user.CommentQuotaMonthly,
		&user.CommentsUsedThisPeriod,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (q *PostgresQuery) GetUserByID(ctx context.Context, id string) (*User, error) {
	query := `
		SELECT id, email, password_hash, subscription_plan, stripe_customer_id, comment_quota_monthly, comments_used_this_period, created_at, updated_at
		FROM users
		WHERE id = $1
	`
	user := &User{}
	err := q.pool.QueryRow(ctx, query, id).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.SubscriptionPlan,
		&user.StripeCustomerID,
		&user.CommentQuotaMonthly,
		&user.CommentsUsedThisPeriod,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (q *PostgresQuery) UpdateUserSubscription(ctx context.Context, userID, plan, stripeCustomerID string, quota int) error {
	var stripeIDPtr *string
	if stripeCustomerID != "" {
		stripeIDPtr = &stripeCustomerID
	}
	query := `
		UPDATE users
		SET subscription_plan = $2,
		    stripe_customer_id = $3,
		    comment_quota_monthly = $4,
		    updated_at = NOW()
		WHERE id = $1
	`
	_, err := q.pool.Exec(ctx, query, userID, plan, stripeIDPtr, quota)
	return err
}
