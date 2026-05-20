package api

import (
	"context"
	"encoding/base64"
	"time"

	"noumena/internal/db"
)

// refreshStore adapts a db.DBQuery to the auth.RefreshTokenStore interface.
type refreshStore struct{ q db.DBQuery }

func (s refreshStore) InsertRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	return s.q.InsertRefreshToken(ctx, userID, tokenHash, expiresAt)
}
func (s refreshStore) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (string, time.Time, error) {
	return s.q.GetRefreshTokenByHash(ctx, tokenHash)
}
func (s refreshStore) DeleteRefreshTokenByHash(ctx context.Context, tokenHash string) error {
	return s.q.DeleteRefreshTokenByHash(ctx, tokenHash)
}
func (s refreshStore) DeleteAllRefreshTokensForUser(ctx context.Context, userID string) error {
	return s.q.DeleteAllRefreshTokensForUser(ctx, userID)
}
func (s refreshStore) GetUserTokenVersion(ctx context.Context, userID string) (int, error) {
	return s.q.GetUserTokenVersion(ctx, userID)
}

func base64URLEncode(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
func base64URLDecode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}
