package db

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// openTestDB opens an isolated in-memory SQLite with the same pragmas
// InitDB applies via the DSN.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := database.Exec(pragma); err != nil {
			t.Fatalf("%s: %v", pragma, err)
		}
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestInitDB_AppliesPragmas(t *testing.T) {
	// Task 1.4: open a temp FILE db via InitDB and assert all three
	// pragmas — the boot-time guarantee that replaced the silently
	// ignored DSN keys.
	path := filepath.Join(t.TempDir(), "pragmas.db")
	if err := InitDB(path); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer CloseDB()

	var journalMode string
	var foreignKeys, busyTimeout int
	if err := DB.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("journal_mode: %v", err)
	}
	if err := DB.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("foreign_keys: %v", err)
	}
	if err := DB.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatalf("busy_timeout: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want wal", journalMode)
	}
	if foreignKeys != 1 {
		t.Errorf("foreign_keys = %d, want 1", foreignKeys)
	}
	if busyTimeout < 5000 {
		t.Errorf("busy_timeout = %d, want >= 5000", busyTimeout)
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	// Tasks 2.1/2.5: fresh DB gets every migration in order, the ledger
	// records each version, and a second run is a no-op.
	ctx := context.Background()
	database := openTestDB(t)

	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	// 2.1: ledger table exists with version rows 1..5.
	var versions []int
	rows, err := database.Query("SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		versions = append(versions, v)
	}
	rows.Close()
	if len(versions) != 20 {
		t.Fatalf("ledger has %d versions, want 20: %v", len(versions), versions)
	}

	// Second run: no-op, ledger unchanged.
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	var n int
	if err := database.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 20 {
		t.Fatalf("ledger rows after re-run = %d, want 20", n)
	}
}

// legacyUsersDDL mirrors schema.sql's users table as it existed before
// 'trialing' joined the CHECK constraint (same column order — the v3
// rebuild copies rows positionally).
const legacyUsersDDL = `CREATE TABLE users (
    id TEXT PRIMARY KEY,
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT,
    google_sub TEXT UNIQUE,
    email_verified_at TEXT,
    subscription_plan TEXT NOT NULL DEFAULT 'free',
    stripe_customer_id TEXT,
    comment_quota_monthly INT NOT NULL DEFAULT 1000,
    comments_used_this_period INT NOT NULL DEFAULT 0,
    quota_reset_at TEXT,
    token_version INT NOT NULL DEFAULT 0,
    failed_login_attempts INT NOT NULL DEFAULT 0,
    locked_until TEXT,
    deletion_requested_at TEXT,
    deletion_completed_at TEXT,
    data_export_url TEXT,
    data_export_expires_at TEXT,
    digest_email_opt_in BOOLEAN NOT NULL DEFAULT 1,
    digest_email_sent_at TEXT,
    trial_ends_at TEXT,
    subscription_status TEXT NOT NULL DEFAULT 'active'
        CHECK (subscription_status IN ('active','past_due','paused','canceled')),
    current_period_end TEXT,
    billing_interval TEXT CHECK (billing_interval IS NULL OR billing_interval IN ('month','year')),
    stripe_subscription_id TEXT,
    past_due_since TEXT,
    active_workspace_id TEXT REFERENCES workspaces(id),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
)`

func TestMigrate_LegacyDB_Repair(t *testing.T) {
	// Tasks 3.1/3.2: a pre-migration DB with the drifted columns
	// (last_error, storage_path, download_url, partial_export) is
	// repaired in place and existing values preserved.
	ctx := context.Background()
	database := openTestDB(t)

	legacy := []string{
		`CREATE TABLE workspaces (id TEXT PRIMARY KEY, name TEXT NOT NULL, created_by TEXT NOT NULL, created_at TEXT)`,
		legacyUsersDDL,
		`CREATE TABLE account_deletion_jobs (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'pending',
			gdpr_export_generated BOOLEAN NOT NULL DEFAULT 0, channels_processed INT NOT NULL DEFAULT 0,
			total_channels INT NOT NULL DEFAULT 0, last_error TEXT,
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
			started_at TEXT, completed_at TEXT)`,
		`CREATE TABLE data_export_jobs (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'queued',
			storage_path TEXT, download_url TEXT, expires_at TEXT, partial_export BOOLEAN NOT NULL DEFAULT 0,
			last_error TEXT, created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
			started_at TEXT, completed_at TEXT)`,
		`INSERT INTO users (id, email, subscription_status) VALUES ('u1', 'repair@example.com', 'active')`,
		`INSERT INTO account_deletion_jobs (id, user_id, status, last_error) VALUES ('d1', 'u1', 'failed', 'boom')`,
		`INSERT INTO data_export_jobs (id, user_id, status, storage_path, download_url, partial_export, last_error)
			VALUES ('e1', 'u1', 'completed', '/tmp/e1.zip', 'https://dl/e1', 1, 'nope')`,
	}
	for _, stmt := range legacy {
		if _, err := database.Exec(stmt); err != nil {
			t.Fatalf("legacy setup %q: %v", stmt, err)
		}
	}

	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	var errorMessage string
	if err := database.QueryRow(
		`SELECT error_message FROM account_deletion_jobs WHERE id='d1'`).Scan(&errorMessage); err != nil {
		t.Fatalf("deletion job error_message: %v", err)
	}
	if errorMessage != "boom" {
		t.Errorf("deletion error_message = %q, want boom", errorMessage)
	}

	var exportURL, errMsg string
	var partial int
	err := database.QueryRow(
		`SELECT export_url, error_message, partial_export FROM data_export_jobs WHERE id='e1'`).Scan(&exportURL, &errMsg, &partial)
	if err == nil {
		t.Errorf("partial_export still present after migration")
	}
	if err := database.QueryRow(
		`SELECT export_url, error_message FROM data_export_jobs WHERE id='e1'`).Scan(&exportURL, &errMsg); err != nil {
		t.Fatalf("export job canonical columns: %v", err)
	}
	if exportURL != "https://dl/e1" {
		t.Errorf("export_url = %q, want https://dl/e1", exportURL)
	}
	if errMsg != "nope" {
		t.Errorf("error_message = %q, want nope", errMsg)
	}
	if !columnGone(t, database, "data_export_jobs", "storage_path") ||
		!columnGone(t, database, "data_export_jobs", "download_url") ||
		!columnGone(t, database, "account_deletion_jobs", "last_error") {
		t.Errorf("legacy columns not dropped")
	}
}

func columnGone(t *testing.T, database *sql.DB, table, column string) bool {
	t.Helper()
	var n int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&n); err != nil {
		t.Fatalf("introspect %s: %v", table, err)
	}
	return n == 0
}

func TestMigrate_LegacyDB_TrialingAndFTS(t *testing.T) {
	// Tasks 4.1/4.2/4.5: a DB that predates the trialing CHECK and the
	// FTS table is rebuilt (users, preserving rows) and backfilled
	// (comments_fts), so trialing inserts succeed and pre-migration
	// comments are searchable.
	ctx := context.Background()
	database := openTestDB(t)

	legacy := []string{
		`CREATE TABLE workspaces (id TEXT PRIMARY KEY, name TEXT NOT NULL, created_by TEXT NOT NULL, created_at TEXT)`,
		legacyUsersDDL,
		`CREATE TABLE platform_channels (
			id TEXT PRIMARY KEY, platform TEXT NOT NULL, platform_channel_id TEXT NOT NULL,
			display_name TEXT NOT NULL, last_synced_at TEXT, deletion_scheduled_at TEXT,
			ingestion_error_count INT NOT NULL DEFAULT 0, ingestion_last_error_at TEXT,
			ingestion_backoff_until TEXT, created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
			UNIQUE(platform, platform_channel_id))`,
		`CREATE TABLE comments (
			id TEXT PRIMARY KEY, platform_channel_id TEXT NOT NULL REFERENCES platform_channels(id) ON DELETE CASCADE,
			platform_comment_id TEXT NOT NULL, parent_comment_id TEXT, author_handle TEXT, body TEXT NOT NULL,
			posted_at TEXT NOT NULL, category TEXT, topic TEXT, confidence REAL, processed_at TEXT,
			llm_provider TEXT, llm_model TEXT, reply_depth INT NOT NULL DEFAULT 0,
			actionable_insights TEXT NOT NULL DEFAULT '[]',
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
			UNIQUE(platform_channel_id, platform_comment_id))`,
		`CREATE TABLE user_channel_tracking (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			platform_channel_id TEXT NOT NULL REFERENCES platform_channels(id),
			access_token_enc TEXT NOT NULL, refresh_token_enc TEXT NOT NULL, token_expires_at TEXT,
			token_priority INT NOT NULL DEFAULT 0, tracking_status TEXT NOT NULL DEFAULT 'active',
			last_seen_at TEXT, backfill_days INT NOT NULL DEFAULT 90,
			token_expiry_notified_at TEXT, custom_prompt TEXT, workspace_id TEXT,
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
			UNIQUE(user_id, platform_channel_id))`,
		`INSERT INTO users (id, email, subscription_status) VALUES ('u1', 'fts@example.com', 'active')`,
		`INSERT INTO platform_channels (id, platform, platform_channel_id, display_name) VALUES ('ch1', 'youtube', 'yt1', 'Chan')`,
		`INSERT INTO user_channel_tracking (id, user_id, platform_channel_id, access_token_enc, refresh_token_enc, token_priority)
			VALUES ('t1', 'u1', 'ch1', 'enc', 'enc', 0)`,
		`INSERT INTO comments (id, platform_channel_id, platform_comment_id, body, posted_at)
			VALUES ('c1', 'ch1', 'pc1', 'needle in the haystack', '2026-09-01T00:00:00Z')`,
	}
	for _, stmt := range legacy {
		if _, err := database.Exec(stmt); err != nil {
			t.Fatalf("legacy setup %q: %v", stmt, err)
		}
	}

	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// 4.1: trialing now accepted by the rebuilt CHECK.
	if _, err := database.Exec(
		`INSERT INTO users (id, email, subscription_status) VALUES ('u2', 'trial@example.com', 'trialing')`); err != nil {
		t.Fatalf("insert trialing status: %v", err)
	}
	// User row survived the rebuild.
	var email string
	if err := database.QueryRow(`SELECT email FROM users WHERE id='u1'`).Scan(&email); err != nil {
		t.Fatalf("users row lost in rebuild: %v", err)
	}
	if email != "fts@example.com" {
		t.Errorf("users.email = %q after rebuild", email)
	}

	// 4.2/4.5: pre-migration comment is searchable after the FTS rebuild.
	q := NewSQLiteQuery(database).(*SQLiteQuery)
	hits, err := q.SearchComments(ctx, "u1", "needle")
	if err != nil {
		t.Fatalf("SearchComments: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "c1" {
		t.Errorf("SearchComments hits = %d (%v), want 1 (c1)", len(hits), hits)
	}

	// 4.3/4.4: new indexes exist.
	for _, idx := range []string{
		"idx_comments_processed",
		"idx_weekly_digests_status_created",
		"idx_platform_channels_deletion_scheduled_at",
	} {
		var n int
		if err := database.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, idx).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("index %s missing after migration", idx)
		}
	}
}

func TestLifecycleJobs_RealSQLite(t *testing.T) {
	// Task 3.5: deletion job claim and export job completion succeed on
	// a real (migrated) SQLite database — the drift that made them
	// error at runtime is gone.
	ctx := context.Background()
	database := openTestDB(t)
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	q := NewSQLiteQuery(database)

	user, err := q.CreateUserWithPassword(ctx, fmt.Sprintf("lifecycle_%d@example.com", time.Now().UnixNano()), "hash")
	if err != nil {
		t.Fatalf("CreateUserWithPassword: %v", err)
	}

	// Deletion claim.
	if _, err := q.ScheduleAccountDeletion(ctx, user.ID, 0); err != nil {
		t.Fatalf("ScheduleAccountDeletion: %v", err)
	}
	job, err := q.ClaimNextDeletionJob(ctx, 0)
	if err != nil {
		t.Fatalf("ClaimNextDeletionJob: %v", err)
	}
	if job == nil || job.Status != "exporting" {
		t.Fatalf("claimed deletion job = %+v, want status exporting", job)
	}
	if err := q.MarkDeletionFailed(ctx, job.ID, "simulated failure"); err != nil {
		t.Fatalf("MarkDeletionFailed: %v", err)
	}
	var errorMessage string
	if err := database.QueryRow(
		`SELECT error_message FROM account_deletion_jobs WHERE id=?`, job.ID).Scan(&errorMessage); err != nil {
		t.Fatalf("read error_message: %v", err)
	}
	if errorMessage != "simulated failure" {
		t.Errorf("error_message = %q, want simulated failure", errorMessage)
	}

	// Export completion.
	export, err := q.EnqueueDataExport(ctx, user.ID)
	if err != nil {
		t.Fatalf("EnqueueDataExport: %v", err)
	}
	claimed, err := q.ClaimNextExportJob(ctx)
	if err != nil {
		t.Fatalf("ClaimNextExportJob: %v", err)
	}
	if claimed == nil || claimed.ID != export.ID {
		t.Fatalf("claimed export = %+v, want %s", claimed, export.ID)
	}
	expiry := time.Now().Add(24 * time.Hour)
	if err := q.MarkDataExportCompleted(ctx, export.ID, user.ID, "/tmp/unused.zip", "https://dl.example/export", expiry, false); err != nil {
		t.Fatalf("MarkDataExportCompleted: %v", err)
	}
	done, err := q.GetDataExportJob(ctx, export.ID)
	if err != nil {
		t.Fatalf("GetDataExportJob: %v", err)
	}
	if done.Status != "completed" || done.ExportURL == nil || *done.ExportURL != "https://dl.example/export" {
		t.Errorf("completed export = %+v, want status completed + export_url set", done)
	}
	if done.ErrorMessage != nil {
		t.Errorf("ErrorMessage = %v, want nil", *done.ErrorMessage)
	}

	// Failure path round-trip.
	if err := q.MarkDataExportFailed(ctx, export.ID, "zip failed"); err != nil {
		t.Fatalf("MarkDataExportFailed: %v", err)
	}
	failed, err := q.GetDataExportJob(ctx, export.ID)
	if err != nil {
		t.Fatal(err)
	}
	if failed.ErrorMessage == nil || *failed.ErrorMessage != "zip failed" {
		t.Errorf("ErrorMessage = %v, want zip failed", failed.ErrorMessage)
	}

	// users.data_export_url mirror.
	var mirror string
	if err := database.QueryRow(
		`SELECT data_export_url FROM users WHERE id=?`, user.ID).Scan(&mirror); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(mirror, "https://dl.example/export") {
		t.Errorf("users.data_export_url = %q", mirror)
	}
}

// Ensure the file-based InitDB path also migrates cleanly end-to-end.
func TestInitDB_AndApplyMigrations_FileDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "boot.db")
	if err := InitDB(path); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer CloseDB()
	if err := ApplyMigrations(context.Background()); err != nil {
		t.Fatalf("ApplyMigrations: %v", err)
	}
	var n int
	if err := DB.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 20 {
		t.Fatalf("ledger rows = %d, want 20", n)
	}
}
