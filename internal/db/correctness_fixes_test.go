package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// TestEnqueueDataExport_NonUniqueErrorSurfaces: a real INSERT failure
// (here a foreign-key violation) must not be masked as ErrNotFound by
// the inflight fallback. Only a UNIQUE collision may take that path.
func TestEnqueueDataExport_NonUniqueErrorSurfaces(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	q := NewSQLiteQuery(database)

	_, err := q.EnqueueDataExport(ctx, "ghost-user-does-not-exist")
	if err == nil {
		t.Fatalf("EnqueueDataExport with dangling user_id: want error, got nil")
	}
	if errors.Is(err, ErrNotFound) {
		t.Fatalf("EnqueueDataExport masked real DB error as ErrNotFound: %v", err)
	}
	if !strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
		t.Fatalf("EnqueueDataExport error = %v, want the original FK failure", err)
	}
}

// TestEnqueueDataExport_InflightReturnsExisting: the partial unique index
// still resolves to the existing queued job on a genuine collision.
func TestEnqueueDataExport_InflightReturnsExisting(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	q := NewSQLiteQuery(database)

	u, err := q.CreateUserWithPassword(ctx, "inflight@example.com", "h")
	if err != nil {
		t.Fatalf("CreateUserWithPassword: %v", err)
	}
	first, err := q.EnqueueDataExport(ctx, u.ID)
	if err != nil {
		t.Fatalf("first EnqueueDataExport: %v", err)
	}
	second, err := q.EnqueueDataExport(ctx, u.ID)
	if err != nil {
		t.Fatalf("second EnqueueDataExport: %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("second enqueue ID = %s, want existing inflight %s", second.ID, first.ID)
	}
}

// TestClearExportStoragePath_ClearsUserMirror: TTL cleanup must drop both
// the job's export_url and the users.data_export_url mirror, otherwise the
// dashboard keeps advertising a URL whose file is gone.
func TestClearExportStoragePath_ClearsUserMirror(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	q := NewSQLiteQuery(database)

	u, err := q.CreateUserWithPassword(ctx, "cleanup@example.com", "h")
	if err != nil {
		t.Fatalf("CreateUserWithPassword: %v", err)
	}
	job, err := q.EnqueueDataExport(ctx, u.ID)
	if err != nil {
		t.Fatalf("EnqueueDataExport: %v", err)
	}
	expiry := time.Now().Add(time.Hour)
	if err := q.MarkDataExportCompleted(ctx, job.ID, u.ID, "/tmp/x.zip", "https://dl/one", expiry, false); err != nil {
		t.Fatalf("MarkDataExportCompleted: %v", err)
	}

	var urlBefore sql.NullString
	if err := database.QueryRowContext(ctx,
		`SELECT data_export_url FROM users WHERE id=?`, u.ID).Scan(&urlBefore); err != nil {
		t.Fatalf("read users.data_export_url before: %v", err)
	}
	if !urlBefore.Valid || urlBefore.String != "https://dl/one" {
		t.Fatalf("users.data_export_url before cleanup = %v, want https://dl/one", urlBefore)
	}

	if err := q.ClearExportStoragePath(ctx, job.ID); err != nil {
		t.Fatalf("ClearExportStoragePath: %v", err)
	}

	var urlAfter, expiresAfter sql.NullString
	var jobURL sql.NullString
	if err := database.QueryRowContext(ctx,
		`SELECT data_export_url, data_export_expires_at FROM users WHERE id=?`, u.ID).
		Scan(&urlAfter, &expiresAfter); err != nil {
		t.Fatalf("read users row after cleanup: %v", err)
	}
	if urlAfter.Valid {
		t.Errorf("users.data_export_url = %q after cleanup, want NULL", urlAfter.String)
	}
	if expiresAfter.Valid {
		t.Errorf("users.data_export_expires_at = %q after cleanup, want NULL", expiresAfter.String)
	}
	if err := database.QueryRowContext(ctx,
		`SELECT export_url FROM data_export_jobs WHERE id=?`, job.ID).Scan(&jobURL); err != nil {
		t.Fatalf("read job after cleanup: %v", err)
	}
	if jobURL.Valid {
		t.Errorf("data_export_jobs.export_url = %q after cleanup, want NULL", jobURL.String)
	}
}

// TestClearExportStoragePath_PreservesNewerExportURL: cleaning an older
// expired job must not wipe a newer export's link off the user row.
func TestClearExportStoragePath_PreservesNewerExportURL(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	q := NewSQLiteQuery(database)

	u, err := q.CreateUserWithPassword(ctx, "cleanup2@example.com", "h")
	if err != nil {
		t.Fatalf("CreateUserWithPassword: %v", err)
	}
	oldJob, err := q.EnqueueDataExport(ctx, u.ID)
	if err != nil {
		t.Fatalf("enqueue old: %v", err)
	}
	expiry := time.Now().Add(time.Hour)
	if err := q.MarkDataExportCompleted(ctx, oldJob.ID, u.ID, "/tmp/old.zip", "https://dl/old", expiry, false); err != nil {
		t.Fatalf("complete old: %v", err)
	}
	newJob, err := q.EnqueueDataExport(ctx, u.ID)
	if err != nil {
		t.Fatalf("enqueue new: %v", err)
	}
	if err := q.MarkDataExportCompleted(ctx, newJob.ID, u.ID, "/tmp/new.zip", "https://dl/new", expiry, false); err != nil {
		t.Fatalf("complete new: %v", err)
	}

	if err := q.ClearExportStoragePath(ctx, oldJob.ID); err != nil {
		t.Fatalf("ClearExportStoragePath(old): %v", err)
	}
	var url sql.NullString
	if err := database.QueryRowContext(ctx,
		`SELECT data_export_url FROM users WHERE id=?`, u.ID).Scan(&url); err != nil {
		t.Fatalf("read users after old cleanup: %v", err)
	}
	if !url.Valid || url.String != "https://dl/new" {
		t.Errorf("users.data_export_url = %v after clearing older job, want https://dl/new", url)
	}
}

// TestClearExportStoragePath_UnknownJob: a missing job is ErrNotFound.
func TestClearExportStoragePath_UnknownJob(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	q := NewSQLiteQuery(database)
	if err := q.ClearExportStoragePath(ctx, "no-such-job"); !errors.Is(err, ErrNotFound) {
		t.Errorf("ClearExportStoragePath(unknown) = %v, want ErrNotFound", err)
	}
}

// TestGetUserExportBundle_IncludesVideoContextAndSentiment proves the GDPR
// bundle carries the full comment projection, not just the legacy columns.
func TestGetUserExportBundle_IncludesVideoContextAndSentiment(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	q := NewSQLiteQuery(database)

	u, err := q.CreateUserWithPassword(ctx, "bundle@example.com", "h")
	if err != nil {
		t.Fatalf("CreateUserWithPassword: %v", err)
	}
	for _, stmt := range []string{
		`INSERT INTO platform_channels (id, platform, platform_channel_id, display_name) VALUES ('bch', 'youtube', 'yt-b', 'B')`,
		`INSERT INTO user_channel_tracking (id, user_id, platform_channel_id, access_token_enc, refresh_token_enc) VALUES ('btr', '` + u.ID + `', 'bch', 'e', 'e')`,
		`INSERT INTO comments (id, platform_channel_id, platform_comment_id, body, posted_at,
			category, actionable_insights, topic, confidence, video_id, video_title, sentiment_score, processed_at)
		 VALUES ('bcm', 'bch', 'pc-b', 'body', '2026-09-01T00:00:00Z',
			'question', '[{"text":"idea"}]', 'pricing', 0.75, 'vid-1', 'Video One', 1, '2026-09-02T00:00:00Z')`,
	} {
		if _, err := database.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed %q: %v", stmt, err)
		}
	}

	bundle, err := q.GetUserExportBundle(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUserExportBundle: %v", err)
	}
	if len(bundle.Comments) != 1 {
		t.Fatalf("bundle comments = %d, want 1", len(bundle.Comments))
	}
	c := bundle.Comments[0]
	if c.VideoID == nil || *c.VideoID != "vid-1" {
		t.Errorf("VideoID = %v, want vid-1", c.VideoID)
	}
	if c.VideoTitle == nil || *c.VideoTitle != "Video One" {
		t.Errorf("VideoTitle = %v, want Video One", c.VideoTitle)
	}
	if c.SentimentScore == nil || *c.SentimentScore != 1 {
		t.Errorf("SentimentScore = %v, want 1", c.SentimentScore)
	}
	if c.Topic == nil || *c.Topic != "pricing" {
		t.Errorf("Topic = %v, want pricing", c.Topic)
	}
	if c.Category == nil || *c.Category != "question" {
		t.Errorf("Category = %v, want question", c.Category)
	}

	// The ZIP writer marshals bundle.Comments directly; prove the new
	// fields actually serialize (embedded Comment fields stay flattened).
	body, err := json.Marshal(bundle.Comments)
	if err != nil {
		t.Fatalf("marshal bundle comments: %v", err)
	}
	for _, want := range []string{`"video_id":"vid-1"`, `"video_title":"Video One"`, `"sentiment_score":1`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("bundle comments JSON missing %s: %s", want, body)
		}
	}
}

// TestMigrate_ForeignKeyCheck_DetectsDanglingRef proves v3's post-rebuild
// integrity check actually runs: a dangling FK seeded before the rebuild
// must fail the migration.
func TestMigrate_ForeignKeyCheck_DetectsDanglingRef(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)

	// Seed the legacy shape with FK enforcement off so the dangling row
	// can be inserted.
	if _, err := database.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatalf("disable FK: %v", err)
	}
	legacy := []string{
		`CREATE TABLE workspaces (id TEXT PRIMARY KEY, name TEXT NOT NULL, created_by TEXT NOT NULL, created_at TEXT)`,
		legacyUsersDDL,
		`CREATE TABLE fk_probe (id TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id))`,
		`INSERT INTO users (id, email, subscription_status) VALUES ('u1', 'probe@example.com', 'active')`,
		`INSERT INTO fk_probe (id, user_id) VALUES ('p1', 'ghost')`,
	}
	for _, stmt := range legacy {
		if _, err := database.Exec(stmt); err != nil {
			t.Fatalf("legacy setup %q: %v", stmt, err)
		}
	}

	err := Migrate(ctx, database)
	if err == nil {
		t.Fatalf("Migrate with dangling FK: want error, got nil")
	}
	if !strings.Contains(err.Error(), "foreign_key_check") {
		t.Fatalf("Migrate error = %v, want foreign_key_check violation", err)
	}
}

// TestMigrate_ForeignKeyCheck_CleanDBPasses: the strengthened check must
// not reject a healthy database.
func TestMigrate_ForeignKeyCheck_CleanDBPasses(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate clean DB: %v", err)
	}
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != len(migrations) {
		t.Errorf("ledger rows = %d, want %d", n, len(migrations))
	}
}

// openFileTestDB opens a second handle to a shared on-disk SQLite file
// with the production pragmas, one connection per handle.
func openFileTestDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	d, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open file db: %v", err)
	}
	d.SetMaxOpenConns(1)
	d.SetMaxIdleConns(1)
	t.Cleanup(func() { d.Close() })
	return d
}

// TestMigrate_Concurrent_Serializes: two processes booting together on the
// same SQLite file must not both apply migrations. Exactly one applies;
// the other waits, re-reads the ledger and no-ops. Both return nil.
func TestMigrate_Concurrent_Serializes(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "concurrent.db")
	db1 := openFileTestDB(t, path)
	db2 := openFileTestDB(t, path)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	go func() { defer wg.Done(); errs[0] = Migrate(ctx, db1) }()
	go func() { defer wg.Done(); errs[1] = Migrate(ctx, db2) }()
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("Migrate handle %d: %v", i, err)
		}
	}
	if t.Failed() {
		return
	}

	var n int
	if err := db1.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if n != len(migrations) {
		t.Errorf("ledger rows = %d, want %d (migration applied more than once?)", n, len(migrations))
	}
	// The lock is released, so the table must not be left held.
	var locked int
	if err := db1.QueryRow(`SELECT locked FROM schema_migration_lock WHERE id=1`).Scan(&locked); err != nil {
		t.Fatalf("read lock row: %v", err)
	}
	if locked != 0 {
		t.Errorf("migration lock still held (locked=%d)", locked)
	}
}
