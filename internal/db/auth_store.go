package db

import (
	"context"
	"time"
)

// AuthStore is the narrow data-access surface consumed by the auth handlers.
type AuthStore interface {
	// Users
	CreateUser(ctx context.Context, email, passwordHash string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByID(ctx context.Context, id string) (*User, error)
	UpdateUserSubscription(ctx context.Context, userID, plan, stripeCustomerID string, quota int) error

	// User profiles
	GetUserProfile(ctx context.Context, userID string) (*UserProfile, error)
	UpsertUserProfile(ctx context.Context, profile *UserProfile) (*UserProfile, error)

	// Auth user creation / lookup
	CreateUserWithPassword(ctx context.Context, email, passwordHash string) (*User, error)
	CreateUserGoogle(ctx context.Context, email, googleSub string) (*User, error)
	GetUserByGoogleSub(ctx context.Context, sub string) (*User, error)
	IncrementFailedLoginAttempts(ctx context.Context, userID string) (int, error)
	LockUser(ctx context.Context, userID string, until time.Time) error
	ResetLoginFailures(ctx context.Context, userID string) error
	IncrementTokenVersion(ctx context.Context, userID string) error
	GetUserTokenVersion(ctx context.Context, userID string) (int, error)
	SetEmailVerified(ctx context.Context, userID string) error
	UpdatePasswordHash(ctx context.Context, userID, hash string) error
	LinkGoogleSub(ctx context.Context, userID, sub string) error
	DeleteStaleUnverifiedUsers(ctx context.Context, olderThan time.Duration) (int64, error)

	// Email-verification tokens
	InsertEmailVerificationToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
	ConsumeEmailVerificationToken(ctx context.Context, tokenHash string) (string, error)
	InvalidateEmailVerificationTokensForUser(ctx context.Context, userID string) error
	CountVerificationResendsInWindow(ctx context.Context, userID string, since time.Time) (int, error)

	// Email-change tokens
	InsertEmailChangeToken(ctx context.Context, userID, pendingEmail, tokenHash string, expiresAt time.Time) error
	ConsumeEmailChangeToken(ctx context.Context, tokenHash string) (userID, pendingEmail string, err error)
	GetActiveEmailChange(ctx context.Context, userID string) (*EmailChange, error)
	InvalidateEmailChangeTokensForUser(ctx context.Context, userID string) error
	ApplyEmailChange(ctx context.Context, userID, newEmail string) error

	// Password-reset tokens
	InsertPasswordResetToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
	ConsumePasswordResetToken(ctx context.Context, tokenHash string) (string, error)
	InvalidatePasswordResetTokensForUser(ctx context.Context, userID string) error
	CountForgotPasswordEmailsInWindow(ctx context.Context, email string, since time.Time) (int, error)

	// Refresh tokens
	InsertRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
	GetRefreshTokenByHash(ctx context.Context, tokenHash string) (userID string, expiresAt time.Time, err error)
	DeleteRefreshTokenByHash(ctx context.Context, tokenHash string) error
	DeleteAllRefreshTokensForUser(ctx context.Context, userID string) error
}

var _ AuthStore = (DBQuery)(nil)
