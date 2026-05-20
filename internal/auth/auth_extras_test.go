package auth_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"noumena/internal/auth"
	"noumena/internal/config"
)

func TestValidatePasswordPolicy(t *testing.T) {
	cases := []struct {
		name    string
		pw      string
		wantErr bool
	}{
		{"too short", "Ab1xy", true},
		{"no upper", "abcdefgh1", true},
		{"no digit", "Abcdefgh", true},
		{"valid", "Password1", false},
		{"valid complex", "Sup3rSecret!", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := auth.ValidatePasswordPolicy(tc.pw)
			if tc.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tc.wantErr && err != nil && !auth.IsPasswordPolicyError(err) {
				t.Errorf("expected PasswordPolicyError, got %T", err)
			}
		})
	}
}

func TestPasswordHashAndVerify(t *testing.T) {
	pw := "SecureP@ssw0rd"
	hash, err := auth.HashPassword(pw)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !auth.VerifyPassword(hash, pw) {
		t.Error("VerifyPassword should accept the correct password")
	}
	if auth.VerifyPassword(hash, "wrong") {
		t.Error("VerifyPassword should reject a wrong password")
	}
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil {
		t.Fatalf("bcrypt.Cost: %v", err)
	}
	if cost < 12 {
		t.Errorf("bcrypt cost %d below minimum 12", cost)
	}
}

func TestNeedsRehash(t *testing.T) {
	// Generate a hash at cost 10; with default config (cost 12) NeedsRehash
	// should return true.
	low, err := bcrypt.GenerateFromPassword([]byte("Password1"), 10)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	if !auth.NeedsRehash(string(low)) {
		t.Error("expected NeedsRehash=true for cost-10 hash under cost-12 config")
	}
	// Garbage hash → also true.
	if !auth.NeedsRehash("not-a-bcrypt-hash") {
		t.Error("expected NeedsRehash=true for garbage hash")
	}
	// Hash at the configured cost → false.
	good, err := auth.HashPassword("Password1")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if auth.NeedsRehash(good) {
		t.Error("expected NeedsRehash=false for fresh hash")
	}
}

// --- JWT --------------------------------------------------------------------

func TestIssueAndParseAccessToken(t *testing.T) {
	tok, exp, err := auth.IssueAccessToken("user-1", 3)
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	if tok == "" {
		t.Fatal("empty token")
	}
	if !exp.After(time.Now()) {
		t.Fatal("token already expired")
	}
	claims, err := auth.ParseAccessToken(tok)
	if err != nil {
		t.Fatalf("ParseAccessToken: %v", err)
	}
	if claims.UserID != "user-1" {
		t.Errorf("UserID = %q, want user-1", claims.UserID)
	}
	if claims.TokenVersion != 3 {
		t.Errorf("TokenVersion = %d, want 3", claims.TokenVersion)
	}
}

func TestParseAccessTokenExpired(t *testing.T) {
	// Force a tiny TTL via config override.
	orig := config.Default.JWTAccessTTL
	config.Default.JWTAccessTTL = -2 * time.Minute // already expired beyond 30s leeway
	defer func() { config.Default.JWTAccessTTL = orig }()

	tok, _, err := auth.IssueAccessToken("user-1", 0)
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	if _, err := auth.ParseAccessToken(tok); err == nil {
		t.Error("expected expired token to be rejected")
	}
}

func TestParseAccessTokenClockSkewLeeway(t *testing.T) {
	// Token expired 10 seconds ago → still accepted under 30s leeway.
	orig := config.Default.JWTAccessTTL
	config.Default.JWTAccessTTL = -10 * time.Second
	defer func() { config.Default.JWTAccessTTL = orig }()

	tok, _, err := auth.IssueAccessToken("user-1", 0)
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	if _, err := auth.ParseAccessToken(tok); err != nil {
		t.Errorf("expected leeway to admit token, got %v", err)
	}
}

func TestParseAccessTokenWrongSecret(t *testing.T) {
	tok, _, err := auth.IssueAccessToken("user-1", 0)
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	orig := config.Default.JWTSecret
	config.Default.JWTSecret = []byte("a-different-secret")
	defer func() { config.Default.JWTSecret = orig }()
	if _, err := auth.ParseAccessToken(tok); err == nil {
		t.Error("expected token signed by different secret to be rejected")
	}
}

// --- Refresh tokens ---------------------------------------------------------

func TestGenerateRefreshTokenIsRandomAndHashed(t *testing.T) {
	raw1, hash1, err := auth.GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}
	raw2, hash2, err := auth.GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}
	if raw1 == raw2 {
		t.Error("two tokens collided")
	}
	if hash1 == hash2 {
		t.Error("hashes should differ for distinct tokens")
	}
	if len(raw1) != 64 || len(hash1) != 64 {
		t.Errorf("expected 64-char hex tokens, got raw=%d hash=%d", len(raw1), len(hash1))
	}
	if !strings.EqualFold(auth.HashRefreshToken(raw1), hash1) {
		t.Error("HashRefreshToken should be deterministic")
	}
}

// memStore implements the auth.RefreshTokenStore in memory.
type memStore struct {
	mu        sync.Mutex
	rows      map[string]memRow
	versions  map[string]int
}
type memRow struct {
	userID  string
	expires time.Time
}

func newMemStore() *memStore {
	return &memStore{rows: make(map[string]memRow), versions: make(map[string]int)}
}

func (m *memStore) InsertRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[tokenHash] = memRow{userID: userID, expires: expiresAt}
	return nil
}
func (m *memStore) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (string, time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.rows[tokenHash]
	if !ok {
		return "", time.Time{}, nil
	}
	return row.userID, row.expires, nil
}
func (m *memStore) DeleteRefreshTokenByHash(ctx context.Context, tokenHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rows, tokenHash)
	return nil
}
func (m *memStore) DeleteAllRefreshTokensForUser(ctx context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for h, r := range m.rows {
		if r.userID == userID {
			delete(m.rows, h)
		}
	}
	return nil
}
func (m *memStore) GetUserTokenVersion(ctx context.Context, userID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.versions[userID], nil
}

func TestRefreshTokenIssueAndRotate(t *testing.T) {
	store := newMemStore()
	ctx := context.Background()

	raw1, _, err := auth.IssueRefreshToken(ctx, store, "user-1")
	if err != nil {
		t.Fatalf("IssueRefreshToken: %v", err)
	}
	if len(store.rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(store.rows))
	}

	raw2, _, uid, err := auth.RotateRefreshToken(ctx, store, raw1)
	if err != nil {
		t.Fatalf("RotateRefreshToken: %v", err)
	}
	if uid != "user-1" {
		t.Errorf("uid = %q", uid)
	}
	if raw2 == raw1 {
		t.Error("rotated token must differ")
	}
	if len(store.rows) != 1 {
		t.Errorf("expected exactly one row after rotation, got %d", len(store.rows))
	}
	// Reusing the old token must fail (rotation invalidates it).
	if _, _, _, err := auth.RotateRefreshToken(ctx, store, raw1); err == nil {
		t.Error("expected reuse of rotated token to fail")
	}
}

func TestRefreshTokenExpired(t *testing.T) {
	store := newMemStore()
	ctx := context.Background()

	// Insert a row with an expiry in the past.
	raw, hash, _ := auth.GenerateRefreshToken()
	_ = store.InsertRefreshToken(ctx, "user-1", hash, time.Now().Add(-1*time.Hour))

	if _, _, _, err := auth.RotateRefreshToken(ctx, store, raw); err == nil {
		t.Error("expected expired token to fail rotation")
	}
}

func TestRevokeAllRefreshTokensForUser(t *testing.T) {
	store := newMemStore()
	ctx := context.Background()
	_, _, _ = auth.IssueRefreshToken(ctx, store, "u1")
	_, _, _ = auth.IssueRefreshToken(ctx, store, "u1")
	_, _, _ = auth.IssueRefreshToken(ctx, store, "u2")
	if len(store.rows) != 3 {
		t.Fatalf("want 3, got %d", len(store.rows))
	}
	if err := auth.RevokeAllRefreshTokensForUser(ctx, store, "u1"); err != nil {
		t.Fatal(err)
	}
	if len(store.rows) != 1 {
		t.Errorf("want 1 row remaining, got %d", len(store.rows))
	}
}
