package lifecycle

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"noumena/internal/auth"
	"noumena/internal/config"
	"noumena/internal/db"
	"noumena/internal/dbtest"
)

// setTestEncryptionKey gives auth.Encrypt a valid key for the duration
// of the test and restores the previous value afterwards.
func setTestEncryptionKey(t *testing.T) {
	t.Helper()
	prev := config.Default.EncryptionKey
	config.Default.EncryptionKey = []byte("0123456789abcdef0123456789abcdef")
	t.Cleanup(func() { config.Default.EncryptionKey = prev })
}

// revokeRecorder is an httptest server that records the tokens passed
// to the Google revoke endpoint and returns a configurable status.
type revokeRecorder struct {
	mu     sync.Mutex
	tokens []string
	status int
	server *httptest.Server
}

func newRevokeRecorder(t *testing.T, status int) *revokeRecorder {
	t.Helper()
	r := &revokeRecorder{status: status}
	r.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		r.tokens = append(r.tokens, req.URL.Query().Get("token"))
		r.mu.Unlock()
		w.WriteHeader(r.status)
	}))
	t.Cleanup(r.server.Close)
	return r
}

func (r *revokeRecorder) got() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.tokens))
	copy(out, r.tokens)
	return out
}

// setRevokeServer points the revoke client and endpoint at a recorder
// for the duration of the test.
func setRevokeServer(t *testing.T, rec *revokeRecorder) {
	t.Helper()
	prevClient := revokeHTTPClient
	prevEndpoint := revokeEndpoint
	revokeHTTPClient = rec.server.Client()
	revokeEndpoint = rec.server.URL
	t.Cleanup(func() {
		revokeHTTPClient = prevClient
		revokeEndpoint = prevEndpoint
	})
}

// TestRevokeOAuthToken_AlreadyInvalidIsSuccess: 2.2 — a 400 from Google
// (already-invalid token) is treated as success, not an error.
func TestRevokeOAuthToken_AlreadyInvalidIsSuccess(t *testing.T) {
	rec := newRevokeRecorder(t, http.StatusBadRequest)
	setRevokeServer(t, rec)

	if err := RevokeOAuthToken(context.Background(), "youtube", "tok", log.New(io.Discard, "", 0)); err != nil {
		t.Fatalf("RevokeOAuthToken(400) = %v, want nil", err)
	}
	if got := rec.got(); len(got) != 1 || got[0] != "tok" {
		t.Errorf("revoked tokens = %v, want [tok]", got)
	}
}

// TestRevokeChannelTokens_RevokesRefreshThenAccess: 2.1/2.3/2.4 — both
// the refresh and access tokens are revoked, refresh first (the durable
// credential).
func TestRevokeChannelTokens_RevokesRefreshThenAccess(t *testing.T) {
	setTestEncryptionKey(t)
	rec := newRevokeRecorder(t, http.StatusOK)
	setRevokeServer(t, rec)

	encRefresh, err := auth.Encrypt("refresh-secret")
	if err != nil {
		t.Fatalf("Encrypt refresh: %v", err)
	}
	encAccess, err := auth.Encrypt("access-secret")
	if err != nil {
		t.Fatalf("Encrypt access: %v", err)
	}
	ch := &db.UserChannelTrackingRow{
		Platform:          "youtube",
		PlatformChannelID: "ch-1",
		AccessTokenEnc:    encAccess,
		RefreshTokenEnc:   encRefresh,
	}

	if err := revokeChannelTokens(context.Background(), ch, log.New(io.Discard, "", 0)); err != nil {
		t.Fatalf("revokeChannelTokens: %v", err)
	}
	got := rec.got()
	if len(got) != 2 {
		t.Fatalf("revoked %d tokens, want 2: %v", len(got), got)
	}
	if got[0] != "refresh-secret" {
		t.Errorf("first revoked token = %q, want refresh-secret (refresh first)", got[0])
	}
	if got[1] != "access-secret" {
		t.Errorf("second revoked token = %q, want access-secret", got[1])
	}
}

// TestRevokeWithRetry_BoundedGivesUp: 6.1/6.3 — a persistently failing
// endpoint is retried a bounded number of times and gives up quickly
// instead of stalling the worker tick.
func TestRevokeWithRetry_BoundedGivesUp(t *testing.T) {
	rec := newRevokeRecorder(t, http.StatusInternalServerError)
	setRevokeServer(t, rec)
	prev := revokeBackoffs
	revokeBackoffs = []time.Duration{0, time.Millisecond, time.Millisecond}
	defer func() { revokeBackoffs = prev }()

	start := time.Now()
	err := RevokeWithRetry(context.Background(), "youtube", "tok", log.New(io.Discard, "", 0))
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("RevokeWithRetry = nil, want error after exhausting retries")
	}
	if got := len(rec.got()); got != 3 {
		t.Errorf("attempts = %d, want 3", got)
	}
	if elapsed > 5*time.Second {
		t.Errorf("RevokeWithRetry took %v, want bounded (~ms with shrunk backoff)", elapsed)
	}
}

// TestDeleteOneChannel_RevokeFailureIsNonFatal: 6.3 — when provider
// revocation fails, the channel deletion still completes and the job
// records revoke_error for the out-of-band sweeper.
func TestDeleteOneChannel_RevokeFailureIsNonFatal(t *testing.T) {
	setTestEncryptionKey(t)
	rec := newRevokeRecorder(t, http.StatusInternalServerError)
	setRevokeServer(t, rec)
	prev := revokeBackoffs
	revokeBackoffs = []time.Duration{0, time.Millisecond, time.Millisecond}
	defer func() { revokeBackoffs = prev }()

	ctx := context.Background()
	database := dbtest.SetupTestDB(t)
	q := db.NewSQLiteQuery(database)

	user, err := q.CreateUserWithPassword(ctx, "revokefail@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUserWithPassword: %v", err)
	}
	encAccess, _ := auth.Encrypt("access")
	encRefresh, _ := auth.Encrypt("refresh")
	if _, err := database.ExecContext(ctx,
		`INSERT INTO platform_channels (id, platform, platform_channel_id, display_name) VALUES ('ch1', 'youtube', 'yt-revoke', 'R')`); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	if _, err := database.ExecContext(ctx,
		`INSERT INTO user_channel_tracking (id, user_id, platform_channel_id, access_token_enc, refresh_token_enc)
		 VALUES ('tr1', ?, 'ch1', ?, ?)`, user.ID, encAccess, encRefresh); err != nil {
		t.Fatalf("seed tracking: %v", err)
	}

	job, err := q.ScheduleAccountDeletion(ctx, user.ID, 1)
	if err != nil {
		t.Fatalf("ScheduleAccountDeletion: %v", err)
	}
	if err := q.MarkDeletionStage(ctx, job.ID, "deleting"); err != nil {
		t.Fatalf("MarkDeletionStage: %v", err)
	}

	w := &DeletionWorker{
		Queries: q,
		Pool:    database,
		Cfg:     config.Default,
		Logger:  log.New(io.Discard, "", 0),
		Now:     func() time.Time { return time.Now().UTC() },
	}
	channels, err := q.GetUserTrackingChannels(ctx, user.ID)
	if err != nil || len(channels) != 1 {
		t.Fatalf("GetUserTrackingChannels: %d channels, err %v", len(channels), err)
	}

	if err := w.deleteOneChannel(ctx, job, channels[0]); err != nil {
		t.Fatalf("deleteOneChannel with failing revoke: %v (revoke must be non-fatal)", err)
	}

	// Tracking row deleted despite revoke failure.
	var trackingCount int
	if err := database.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_channel_tracking WHERE id='tr1'`).Scan(&trackingCount); err != nil {
		t.Fatalf("count tracking: %v", err)
	}
	if trackingCount != 0 {
		t.Errorf("tracking row remains after deleteOneChannel")
	}
	// Revoke failure recorded on the job.
	var revokeAttempted int
	var revokeError string
	if err := database.QueryRowContext(ctx,
		`SELECT revoke_attempted, COALESCE(revoke_error, '') FROM account_deletion_jobs WHERE id=?`, job.ID).
		Scan(&revokeAttempted, &revokeError); err != nil {
		t.Fatalf("read job revoke status: %v", err)
	}
	if revokeAttempted != 1 {
		t.Errorf("revoke_attempted = %d, want 1", revokeAttempted)
	}
	if !strings.Contains(revokeError, "revoke refresh token") {
		t.Errorf("revoke_error = %q, want refresh-token failure recorded", revokeError)
	}
}

// TestRevokeSweeper_RetriesAndClears: 6.2 — the sweeper retries failed
// revocations and clears the job's revoke_error on success.
func TestRevokeSweeper_RetriesAndClears(t *testing.T) {
	setTestEncryptionKey(t)
	rec := newRevokeRecorder(t, http.StatusOK)
	setRevokeServer(t, rec)

	ctx := context.Background()
	database := dbtest.SetupTestDB(t)
	q := db.NewSQLiteQuery(database)

	user, err := q.CreateUserWithPassword(ctx, "sweep@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUserWithPassword: %v", err)
	}
	encAccess, _ := auth.Encrypt("access")
	encRefresh, _ := auth.Encrypt("refresh")
	if _, err := database.ExecContext(ctx,
		`INSERT INTO platform_channels (id, platform, platform_channel_id, display_name) VALUES ('ch1', 'youtube', 'yt-sweep', 'S')`); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	if _, err := database.ExecContext(ctx,
		`INSERT INTO user_channel_tracking (id, user_id, platform_channel_id, access_token_enc, refresh_token_enc)
		 VALUES ('tr1', ?, 'ch1', ?, ?)`, user.ID, encAccess, encRefresh); err != nil {
		t.Fatalf("seed tracking: %v", err)
	}
	job, err := q.ScheduleAccountDeletion(ctx, user.ID, 1)
	if err != nil {
		t.Fatalf("ScheduleAccountDeletion: %v", err)
	}
	if err := q.MarkDeletionStage(ctx, job.ID, "deleting"); err != nil {
		t.Fatalf("MarkDeletionStage: %v", err)
	}
	if err := q.MarkDeletionRevokeStatus(ctx, job.ID, true, "previous failure"); err != nil {
		t.Fatalf("MarkDeletionRevokeStatus: %v", err)
	}

	s := &RevokeSweeper{Queries: q, Logger: log.New(io.Discard, "", 0)}
	s.processOne(ctx)

	// Both tokens revoked by the sweeper.
	if got := rec.got(); len(got) != 2 {
		t.Errorf("sweeper revoked %d tokens, want 2: %v", len(got), got)
	}
	// Job no longer pending revoke.
	pending, err := q.ListJobsPendingRevoke(ctx)
	if err != nil {
		t.Fatalf("ListJobsPendingRevoke: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("pending revoke jobs after sweep = %+v, want none", pending)
	}
}
