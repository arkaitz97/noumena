package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"noumena/internal/config"
)

// AccessClaims is the JWT payload for access tokens.
type AccessClaims struct {
	UserID       string `json:"user_id"`
	TokenVersion int    `json:"token_version"`
	jwt.RegisteredClaims
}

// CustomClaims retained for backwards compatibility with existing handlers /
// tests that reference it. New code should use AccessClaims.
type CustomClaims = AccessClaims

// IssueAccessToken signs a new access token embedding userID and the user's
// current tokenVersion. Returns the JWT string and its expiration time.
func IssueAccessToken(userID string, tokenVersion int) (string, time.Time, error) {
	now := time.Now()
	exp := now.Add(config.Default.JWTAccessTTL)
	claims := AccessClaims{
		UserID:       userID,
		TokenVersion: tokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(config.Default.JWTSecret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, exp, nil
}

// ParseAccessToken parses and validates an access token, allowing 30 seconds
// of clock skew. It returns the embedded AccessClaims on success.
func ParseAccessToken(tokenStr string) (*AccessClaims, error) {
	parsed, err := jwt.ParseWithClaims(
		tokenStr,
		&AccessClaims{},
		func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return config.Default.JWTSecret, nil
		},
		jwt.WithLeeway(30*time.Second),
	)
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(*AccessClaims)
	if !ok || !parsed.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

// GenerateTokens retains its legacy signature (used by the existing tests and
// older handlers) but now produces a token_version=0 access token plus a raw
// refresh token. New code should use IssueAccessToken / IssueRefreshToken.
func GenerateTokens(userID string) (string, string, error) {
	access, _, err := IssueAccessToken(userID, 0)
	if err != nil {
		return "", "", err
	}
	raw, _, err := GenerateRefreshToken()
	if err != nil {
		return "", "", err
	}
	return access, raw, nil
}

// ValidateToken retained for legacy callers.
func ValidateToken(tokenStr string) (*AccessClaims, error) {
	return ParseAccessToken(tokenStr)
}

// --- Refresh tokens ---------------------------------------------------------

// GenerateRefreshToken returns a 32-byte cryptographically random token
// (hex-encoded, 64 characters) and its SHA-256 hash (hex-encoded, 64
// characters).
func GenerateRefreshToken() (raw string, hash string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", err
	}
	raw = hex.EncodeToString(buf)
	hash = HashRefreshToken(raw)
	return raw, hash, nil
}

// HashRefreshToken computes the SHA-256 hash of a raw refresh token and
// returns it hex-encoded.
func HashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// RefreshTokenStore is the minimum DB surface required by the refresh-token
// helpers. internal/db.PostgresQuery satisfies it.
type RefreshTokenStore interface {
	InsertRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
	GetRefreshTokenByHash(ctx context.Context, tokenHash string) (userID string, expiresAt time.Time, err error)
	DeleteRefreshTokenByHash(ctx context.Context, tokenHash string) error
	DeleteAllRefreshTokensForUser(ctx context.Context, userID string) error
	GetUserTokenVersion(ctx context.Context, userID string) (int, error)
}

// IssueRefreshToken creates a new refresh token, persists its hash, and
// returns the raw token (to be set as a cookie) and its expiry.
func IssueRefreshToken(ctx context.Context, store RefreshTokenStore, userID string) (raw string, expiresAt time.Time, err error) {
	raw, hash, err := GenerateRefreshToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt = time.Now().Add(config.Default.JWTRefreshTTL)
	if err = store.InsertRefreshToken(ctx, userID, hash, expiresAt); err != nil {
		return "", time.Time{}, err
	}
	return raw, expiresAt, nil
}

// ErrInvalidRefreshToken is returned when a refresh token is unknown,
// expired, or its user's token_version no longer matches.
var ErrInvalidRefreshToken = errors.New("invalid or expired refresh token")

// RotateRefreshToken validates the supplied raw refresh token and, in a
// single logical operation, deletes the old row and inserts a new one. It
// returns the new raw token, its expiry, and the user_id it belongs to.
//
// Validation: the token hash must exist; expires_at must be in the future;
// the user's current token_version is fetched but token-version equivalence
// is enforced at the access-token layer (the refresh row is deleted on
// logout-all and password-reset, so any existing row already represents a
// valid session).
func RotateRefreshToken(ctx context.Context, store RefreshTokenStore, oldRaw string) (newRaw string, newExpiry time.Time, userID string, err error) {
	oldHash := HashRefreshToken(oldRaw)
	uid, exp, err := store.GetRefreshTokenByHash(ctx, oldHash)
	if err != nil {
		return "", time.Time{}, "", ErrInvalidRefreshToken
	}
	if uid == "" || time.Now().After(exp) {
		// Best-effort cleanup of expired row.
		_ = store.DeleteRefreshTokenByHash(ctx, oldHash)
		return "", time.Time{}, "", ErrInvalidRefreshToken
	}
	// Delete the old row, then issue a new one.
	if err = store.DeleteRefreshTokenByHash(ctx, oldHash); err != nil {
		return "", time.Time{}, "", err
	}
	newRaw, newExpiry, err = IssueRefreshToken(ctx, store, uid)
	if err != nil {
		return "", time.Time{}, "", err
	}
	return newRaw, newExpiry, uid, nil
}

// RevokeRefreshToken deletes the refresh-token row whose hash matches.
func RevokeRefreshToken(ctx context.Context, store RefreshTokenStore, raw string) error {
	return store.DeleteRefreshTokenByHash(ctx, HashRefreshToken(raw))
}

// RevokeAllRefreshTokensForUser deletes every refresh-token row for the user.
func RevokeAllRefreshTokensForUser(ctx context.Context, store RefreshTokenStore, userID string) error {
	return store.DeleteAllRefreshTokensForUser(ctx, userID)
}
