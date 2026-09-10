package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
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

// refreshTokenVersionSep separates the random secret from the token_version
// bound into a raw refresh token. The version is public (it already travels in
// the access JWT); binding it into the refresh token lets RotateRefreshToken
// reject a token minted before a token_version bump even when the bumping code
// path forgot to sweep the corresponding row.
const refreshTokenVersionSep = "."

// generateVersionedRefreshToken mints a random refresh token bound to the
// supplied token_version. The raw form is "<64-hex-secret>.<version>".
func generateVersionedRefreshToken(tokenVersion int) (raw, hash string, err error) {
	secret, _, err := GenerateRefreshToken()
	if err != nil {
		return "", "", err
	}
	raw = secret + refreshTokenVersionSep + strconv.Itoa(tokenVersion)
	return raw, HashRefreshToken(raw), nil
}

// parseRefreshTokenVersion extracts the token_version bound into a raw refresh
// token. ok is false for legacy tokens minted before version binding existed
// (plain 64-hex, no separator).
func parseRefreshTokenVersion(raw string) (version int, ok bool) {
	i := strings.LastIndex(raw, refreshTokenVersionSep)
	if i < 0 {
		return 0, false
	}
	v, err := strconv.Atoi(raw[i+len(refreshTokenVersionSep):])
	if err != nil {
		return 0, false
	}
	return v, true
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

// IssueRefreshToken creates a new refresh token bound to the user's current
// token_version, persists its hash, and returns the raw token (to be set as a
// cookie) and its expiry.
func IssueRefreshToken(ctx context.Context, store RefreshTokenStore, userID string) (raw string, expiresAt time.Time, err error) {
	tokenVersion, err := store.GetUserTokenVersion(ctx, userID)
	if err != nil {
		return "", time.Time{}, err
	}
	raw, hash, err := generateVersionedRefreshToken(tokenVersion)
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
// and the token_version bound into the raw token must equal the user's
// current token_version. The version check closes the gap where a bumping
// code path bumps token_version but fails to delete the refresh row: a token
// minted before the bump carries the old version and is rejected (and its
// stale row swept) here, so a stolen pre-bump cookie cannot mint a fresh
// access token carrying the new version.
//
// Legacy tokens minted before version binding (plain 64-hex, no separator)
// are treated as version 0; a bumped user is still rejected by the compare.
// Residual risk: the store interface exposes no compare-and-delete primitive,
// so two concurrent rotations of the same row can both succeed before either
// delete is observed.
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
	// Enforce token-version equivalence: the version bound into the raw token
	// must match the user's current version.
	tokenVersion, ok := parseRefreshTokenVersion(oldRaw)
	if !ok {
		tokenVersion = 0
	}
	currentVersion, err := store.GetUserTokenVersion(ctx, uid)
	if err != nil {
		_ = store.DeleteRefreshTokenByHash(ctx, oldHash)
		return "", time.Time{}, "", ErrInvalidRefreshToken
	}
	if tokenVersion != currentVersion {
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
