//go:build integration

// channel_mgmt_integration_test.go — DB-backed integration tests for the
// channel-management package, exercised via the dbtest harness.
//
// These tests cover deferred tasks from the channel-management change
// (§11.2 token encryption round-trip, §12.2 oauth_states lifecycle,
// §12.3 UpsertPlatformChannel insert/conflict paths, §12.13 partial
// unique index race) that require real SQLite semantics.

package db_test

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"testing"
	"time"

	"noumena/internal/auth"
	"noumena/internal/config"
	"noumena/internal/db"
	"noumena/internal/dbtest"
)

// adopt installs the per-test DB as the package-global db.DB so that
// production helpers calling db.DB transparently target the harness DB.
// This is safe because harness tests do not run in parallel with code
// that depends on a different db.DB.
func adopt(t *testing.T, database *sql.DB) {
	t.Helper()
	prev := db.DB
	db.DB = database
	t.Cleanup(func() { db.DB = prev })
}

// seedUser inserts a minimal user row and returns its UUID.
func seedUser(t *testing.T, ctx context.Context, database *sql.DB, email string) string {
	t.Helper()
	var id string
	if err := database.QueryRowContext(ctx,
		`INSERT INTO users (email, password_hash) VALUES (?, 'x') RETURNING id`,
		email).Scan(&id); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

// Test_TokenEncryptionRoundTrip — channel-management §11.2.
//
// Verifies that the production encryption helpers produce ciphertext
// that round-trips through the user_channel_tracking columns and that
// the persisted column is in fact ciphertext (not plaintext).
func Test_TokenEncryptionRoundTrip(t *testing.T) {
	database := dbtest.SetupTestDB(t)
	if database == nil {
		return
	}
	adopt(t, database)
	q := db.NewSQLiteQuery(database)
	ctx := context.Background()

	// auth.Encrypt/Decrypt key off config.Default.EncryptionKey, which is
	// nil unless config.Load ran. Install a test key so the round-trip is
	// exercised against the real helpers (fix-crypto-http-hardening).
	prevKey := config.Default.EncryptionKey
	prevPrev := config.Default.EncryptionKeyPrevious
	config.Default.EncryptionKey = []byte("0123456789abcdef0123456789abcdef")
	config.Default.EncryptionKeyPrevious = nil
	t.Cleanup(func() {
		config.Default.EncryptionKey = prevKey
		config.Default.EncryptionKeyPrevious = prevPrev
	})

	userID := seedUser(t, ctx, database, "enc@example.com")

	// Create a platform channel via production code.
	chID, inserted, err := q.UpsertPlatformChannel(ctx, "youtube", "yt-enc-1", "Enc Channel")
	if err != nil || !inserted {
		t.Fatalf("UpsertPlatformChannel: inserted=%v err=%v", inserted, err)
	}

	const accessPlain = "ya29.access-token-secret-value"
	const refreshPlain = "1//refresh-token-secret-value"
	encA, err := auth.Encrypt(accessPlain)
	if err != nil {
		t.Fatalf("encrypt access: %v", err)
	}
	encR, err := auth.Encrypt(refreshPlain)
	if err != nil {
		t.Fatalf("encrypt ref: %v", err)
	}

	if _, err := q.CreateUserChannelTracking(ctx, userID, chID,
		encA, encR, time.Now().Add(time.Hour), 90, 0); err != nil {
		t.Fatalf("CreateUserChannelTracking: %v", err)
	}

	// Inspect the raw row: must NOT contain plaintext.
	var rawAccess, rawRefresh string
	if err := database.QueryRowContext(ctx,
		`SELECT access_token_enc, refresh_token_enc
		 FROM user_channel_tracking WHERE user_id = ?`, userID).
		Scan(&rawAccess, &rawRefresh); err != nil {
		t.Fatalf("read raw cols: %v", err)
	}
	if strings.Contains(rawAccess, accessPlain) {
		t.Fatalf("access column appears to contain plaintext: %q", rawAccess)
	}
	if strings.Contains(rawRefresh, refreshPlain) {
		t.Fatalf("refresh column appears to contain plaintext: %q", rawRefresh)
	}

	// Read back via the query helper — must match the original plaintext.
	gotAcc, gotRef, err := q.GetIngestionToken(ctx, chID)
	if err != nil {
		t.Fatalf("GetIngestionToken: %v", err)
	}
	if gotAcc != accessPlain || gotRef != refreshPlain {
		t.Fatalf("decrypt mismatch: got %q/%q, want %q/%q", gotAcc, gotRef, accessPlain, refreshPlain)
	}
}

// Test_OauthStatesLifecycle — channel-management §12.2.
//
// Verifies that InsertOAuthState / ConsumeOAuthState round-trip and that
// a second consume of the same token is rejected.
func Test_OauthStatesLifecycle(t *testing.T) {
	database := dbtest.SetupTestDB(t)
	if database == nil {
		return
	}
	adopt(t, database)
	q := db.NewSQLiteQuery(database)
	ctx := context.Background()

	userID := seedUser(t, ctx, database, "oauth@example.com")

	const stateToken = "opaque-state-token"
	expiresAt := time.Now().Add(10 * time.Minute)

	if err := q.InsertOAuthState(ctx, stateToken, userID, expiresAt); err != nil {
		t.Fatalf("InsertOAuthState: %v", err)
	}

	consumed, err := q.ConsumeOAuthState(ctx, stateToken, userID)
	if err != nil {
		t.Fatalf("ConsumeOAuthState: %v", err)
	}
	if !consumed {
		t.Fatal("expected first consume to succeed")
	}

	// Second consume must be rejected (row already deleted).
	consumed, err = q.ConsumeOAuthState(ctx, stateToken, userID)
	if err != nil {
		t.Fatalf("second ConsumeOAuthState: %v", err)
	}
	if consumed {
		t.Fatal("expected second consume to be rejected")
	}
}

// Test_UpsertPlatformChannelInsertConflict — channel-management §12.3.
//
// Verifies that the first call inserts and the second call returns
// inserted=false with the same channel ID.
func Test_UpsertPlatformChannelInsertConflict(t *testing.T) {
	database := dbtest.SetupTestDB(t)
	if database == nil {
		return
	}
	adopt(t, database)
	q := db.NewSQLiteQuery(database)
	ctx := context.Background()

	chID, inserted, err := q.UpsertPlatformChannel(ctx, "youtube", "yt-upsert-1", "First Name")
	if err != nil || !inserted {
		t.Fatalf("first upsert: inserted=%v err=%v", inserted, err)
	}

	chID2, inserted2, err := q.UpsertPlatformChannel(ctx, "youtube", "yt-upsert-1", "Second Name")
	if err != nil || inserted2 {
		t.Fatalf("second upsert: inserted=%v err=%v", inserted2, err)
	}
	if chID != chID2 {
		t.Fatalf("expected same channel id, got %s and %s", chID, chID2)
	}
}

// Test_PartialUniqueIndexRace — channel-management §12.13.
//
// Verifies that the partial unique index on (external_id, platform)
// WHERE deleted_at IS NULL prevents duplicate active channels but
// allows re-insertion after a soft-delete.
func Test_PartialUniqueIndexRace(t *testing.T) {
	database := dbtest.SetupTestDB(t)
	if database == nil {
		return
	}
	adopt(t, database)
	q := db.NewSQLiteQuery(database)
	ctx := context.Background()

	// First insert succeeds.
	chID, inserted, err := q.UpsertPlatformChannel(ctx, "youtube", "yt-race-1", "Race Channel")
	if err != nil || !inserted {
		t.Fatalf("first insert: inserted=%v err=%v", inserted, err)
	}

	// Concurrent second insert against the same external_id+platform
	// must conflict (inserted=false, no error).
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	results := make(chan bool, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, ins, err := q.UpsertPlatformChannel(ctx, "youtube", "yt-race-1", "Race Channel")
			errs <- err
			results <- ins
		}()
	}
	wg.Wait()
	close(errs)
	close(results)

	for e := range errs {
		if e != nil {
			t.Fatalf("unexpected error during concurrent upsert: %v", e)
		}
	}

	insertCount := 0
	for r := range results {
		if r {
			insertCount++
		}
	}
	if insertCount != 1 {
		t.Fatalf("expected exactly 1 insert, got %d", insertCount)
	}

	// Soft-delete the channel.
	if err := q.SoftDeleteChannel(ctx, chID); err != nil {
		t.Fatalf("SoftDeleteChannel: %v", err)
	}

	// Re-insert must now succeed with a new ID.
	chID2, inserted2, err := q.UpsertPlatformChannel(ctx, "youtube", "yt-race-1", "Race Channel (new)")
	if err != nil || !inserted2 {
		t.Fatalf("re-insert after soft-delete: inserted=%v err=%v", inserted2, err)
	}
	if chID == chID2 {
		t.Fatal("expected new channel id after re-insert")
	}
}

// Test_GetChannelsForSyncExcludesDeleted — channel-management §12.14.
//
// Verifies that soft-deleted channels are excluded from the sync queue.
func Test_GetChannelsForSyncExcludesDeleted(t *testing.T) {
	database := dbtest.SetupTestDB(t)
	if database == nil {
		return
	}
	adopt(t, database)
	q := db.NewSQLiteQuery(database)
	ctx := context.Background()

	userID := seedUser(t, ctx, database, "sync@example.com")

	chID, _, err := q.UpsertPlatformChannel(ctx, "youtube", "yt-sync-1", "Sync Channel")
	if err != nil {
		t.Fatalf("UpsertPlatformChannel: %v", err)
	}

	// Create a tracking row so the channel has an active tracker.
	if _, err := q.CreateUserChannelTracking(ctx, userID, chID,
		"enc_acc", "enc_ref", time.Now().Add(time.Hour), 90, 0); err != nil {
		t.Fatalf("CreateUserChannelTracking: %v", err)
	}

	// Channel should appear in the sync queue.
	channels, err := q.GetChannelsForSync(ctx, 30)
	if err != nil {
		t.Fatalf("GetChannelsForSync: %v", err)
	}
	found := false
	for _, ch := range channels {
		if ch.ID == chID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected channel in sync queue before soft-delete")
	}

	// Soft-delete the channel.
	if err := q.SoftDeleteChannel(ctx, chID); err != nil {
		t.Fatalf("SoftDeleteChannel: %v", err)
	}

	// Channel should no longer appear in the sync queue.
	channels, err = q.GetChannelsForSync(ctx, 30)
	if err != nil {
		t.Fatalf("GetChannelsForSync after delete: %v", err)
	}
	for _, ch := range channels {
		if ch.ID == chID {
			t.Fatal("soft-deleted channel should not appear in sync queue")
		}
	}
}

// Test_ConnectChannelTxRollback — channel-management §12.4.
//
// Verifies that when the transaction is rolled back, no tracking row is
// created.
func Test_ConnectChannelTxRollback(t *testing.T) {
	database := dbtest.SetupTestDB(t)
	if database == nil {
		return
	}
	adopt(t, database)
	q := db.NewSQLiteQuery(database)
	ctx := context.Background()

	userID := seedUser(t, ctx, database, "rollback@example.com")

	chID, _, err := q.UpsertPlatformChannel(ctx, "youtube", "yt-rollback-1", "Rollback Channel")
	if err != nil {
		t.Fatalf("UpsertPlatformChannel: %v", err)
	}

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}

	if err := q.ConnectChannelTxWithTx(ctx, tx, userID, "youtube", chID, "Test Channel",
		"enc_acc", "enc_ref", time.Now()); err != nil {
		t.Fatalf("ConnectChannelTxWithTx: %v", err)
	}

	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	// Verify no tracking row was created.
	tracks, err := q.ListChannelsForUser(ctx, userID)
	if err != nil {
		t.Fatalf("ListChannelsForUser: %v", err)
	}
	if len(tracks) != 0 {
		t.Fatalf("expected 0 tracking rows after rollback, got %d", len(tracks))
	}
}
