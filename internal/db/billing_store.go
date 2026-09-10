package db

import (
	"context"
	"database/sql"
	"time"
)

// BillingStore is the narrow data-access surface consumed by the billing
// handlers and Stripe subscription maintenance.
type BillingStore interface {
	GetUserByID(ctx context.Context, id string) (*User, error)
	IsStripeEventProcessed(ctx context.Context, eventID string) (bool, error)
	InsertProcessedStripeEvent(ctx context.Context, tx *sql.Tx, eventID, eventType string) error
	ApplySubscriptionUpdate(ctx context.Context, tx *sql.Tx, u SubscriptionUpdate) error
	GetUserByStripeCustomerID(ctx context.Context, customerID string) (*User, error)
	GetActiveChannelCount(ctx context.Context, userID string) (int, error)
	GetOverageCostForCurrentPeriod(ctx context.Context, userID string, periodStart, periodEnd time.Time) (float64, error)
	InsertCommentUsageLog(ctx context.Context, tx *sql.Tx, userID string, count int, periodStart, periodEnd time.Time) error
	PauseChannelsAlphabetically(ctx context.Context, tx *sql.Tx, userID string, keepCount int) error
	ResetFreeTierQuotas(ctx context.Context) (int64, error)
	ExpirePastDueGrace(ctx context.Context, graceHours int) (int64, error)
}

var _ BillingStore = (DBQuery)(nil)
