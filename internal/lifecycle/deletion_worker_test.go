package lifecycle

import (
	"context"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"noumena/internal/config"
	"noumena/internal/db"
	"noumena/internal/dbtest"
	"noumena/internal/email"
)

// recordingSender is a no-op SMTP transport so the real email Pipeline
// runs end-to-end (log row -> send -> mark sent) without a network.
type recordingSender struct{ calls int }

func (s *recordingSender) Send(_ context.Context, _, _, _, _, _ string, _ []string) error {
	s.calls++
	return nil
}

// newDeletionTestWorker builds a DeletionWorker over real SQLite with a
// real email Pipeline writing into a temp export storage dir.
func newDeletionTestWorker(t *testing.T, database *db.SQLiteQuery) (*DeletionWorker, *config.Config, *recordingSender) {
	t.Helper()
	cfg := *config.Default // copy; do not mutate the global default
	cfg.ExportStorageDir = t.TempDir()
	cfg.EmailEnabled = true
	cfg.EmailFromAddress = "noreply@example.test"
	cfg.EmailFromName = "Noumena"

	sender := &recordingSender{}
	pipeline := &email.Pipeline{
		Queries: database,
		Sender:  sender,
		Cfg:     &cfg,
		Logger:  log.New(io.Discard, "", 0),
		Now:     func() time.Time { return time.Now().UTC() },
	}
	w := &DeletionWorker{
		Queries:  database,
		Pool:     database.DB(),
		Pipeline: pipeline,
		Cfg:      &cfg,
		Logger:   log.New(io.Discard, "", 0),
		Now:      func() time.Time { return time.Now().UTC() },
	}
	return w, &cfg, sender
}

func countRows(t *testing.T, ctx context.Context, database *db.SQLiteQuery, query string, args ...any) int {
	t.Helper()
	var n int
	if err := database.DB().QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	return n
}

// TestRunFinalStageRemovesExportDirAndPII proves the confirmed bugs are
// fixed end-to-end on the final stage:
//   - the per-user export directory (full-PII ZIP) is gone,
//   - pre-existing email_send_log/audit_log PII rows are wiped,
//   - the deletion_completed notification's own log row is wiped,
//   - the retained deletion_completed audit event is anonymized,
//   - the user row is hard-deleted.
func TestRunFinalStageRemovesExportDirAndPII(t *testing.T) {
	ctx := context.Background()
	database := dbtest.SetupTestDB(t)
	q := db.NewSQLiteQuery(database).(*db.SQLiteQuery)
	w, cfg, sender := newDeletionTestWorker(t, q)

	user, err := q.CreateUserWithPassword(ctx, "delete-me@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUserWithPassword: %v", err)
	}
	job, err := q.ScheduleAccountDeletion(ctx, user.ID, 0)
	if err != nil {
		t.Fatalf("ScheduleAccountDeletion: %v", err)
	}

	// A completed GDPR export ZIP sitting on disk at the deterministic path.
	zipPath := filepath.Join(cfg.ExportStorageDir, user.ID, "job-1.zip")
	if err := os.MkdirAll(filepath.Dir(zipPath), 0o750); err != nil {
		t.Fatalf("mkdir export dir: %v", err)
	}
	if err := os.WriteFile(zipPath, []byte("full-pii-bundle"), 0o640); err != nil {
		t.Fatalf("write zip: %v", err)
	}

	// Pre-existing email log row (to_address + body PII) and audit row
	// whose details embed the user id.
	uid := user.ID
	if _, err := q.LogEmailAttempt(ctx, db.EmailAttemptInput{
		UserID:         &uid,
		Template:       "welcome",
		IdempotencyKey: "pre-existing:" + user.ID,
		Status:         "sent",
		AttemptCount:   1,
		ToAddress:      user.Email,
		Subject:        "hi",
		HTMLBody:       "<p>hi</p>",
		TextBody:       "hi",
	}); err != nil {
		t.Fatalf("seed email_send_log: %v", err)
	}
	if err := q.InsertAuditLog(ctx, user.ID, "login", `{"user_id":"`+user.ID+`"}`, ""); err != nil {
		t.Fatalf("seed audit_log: %v", err)
	}

	if err := w.runFinalStage(ctx, job, user); err != nil {
		t.Fatalf("runFinalStage: %v", err)
	}

	// The notification email must have actually gone through the pipeline
	// (otherwise the "just-logged row" path below would not be exercised).
	if sender.calls == 0 {
		t.Fatalf("deletion_completed email was never sent")
	}

	// ZIP and its parent directory are gone.
	if _, err := os.Stat(zipPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("export ZIP still present at %s: %v", zipPath, err)
	}
	if _, err := os.Stat(filepath.Join(cfg.ExportStorageDir, user.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("export dir still present: %v", err)
	}

	// No email_send_log PII survives, for the user nor the address.
	if n := countRows(t, ctx, q, `SELECT COUNT(*) FROM email_send_log WHERE user_id = ?`, user.ID); n != 0 {
		t.Errorf("email_send_log rows for user = %d, want 0", n)
	}
	if n := countRows(t, ctx, q, `SELECT COUNT(*) FROM email_send_log WHERE to_address = ?`, user.Email); n != 0 {
		t.Errorf("email_send_log rows for address = %d, want 0", n)
	}
	if n := countRows(t, ctx, q, `SELECT COUNT(*) FROM email_send_log WHERE idempotency_key = ?`, "deletion_completed:"+user.ID); n != 0 {
		t.Errorf("deletion_completed email row survived (count=%d)", n)
	}

	// No audit row is tied to the user id any more.
	if n := countRows(t, ctx, q, `SELECT COUNT(*) FROM audit_log WHERE user_id = ?`, user.ID); n != 0 {
		t.Errorf("audit_log rows for user = %d, want 0", n)
	}
	if n := countRows(t, ctx, q, `SELECT COUNT(*) FROM audit_log WHERE details LIKE '%' || ? || '%'`, user.ID); n != 0 {
		t.Errorf("audit_log details still reference user id (count=%d)", n)
	}

	// The deletion_completed event is retained but anonymized.
	if n := countRows(t, ctx, q, `SELECT COUNT(*) FROM audit_log WHERE action = 'deletion_completed'`); n != 1 {
		t.Fatalf("deletion_completed audit events = %d, want 1 (event must be retained)", n)
	}
	var details string
	var auditUserID *string
	if err := q.DB().QueryRowContext(ctx,
		`SELECT details, user_id FROM audit_log WHERE action = 'deletion_completed'`).
		Scan(&details, &auditUserID); err != nil {
		t.Fatalf("read deletion_completed audit row: %v", err)
	}
	if details != `{"event":"deletion_completed"}` {
		t.Errorf("audit details = %q, want anonymized event only", details)
	}
	if auditUserID != nil {
		t.Errorf("audit user_id = %q, want NULL after hard delete", *auditUserID)
	}

	// User row is gone and the job closed out.
	gone, err := q.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUserByID after delete: %v", err)
	}
	if gone != nil {
		t.Errorf("user row still present after hard delete")
	}
	if n := countRows(t, ctx, q, `SELECT COUNT(*) FROM account_deletion_jobs WHERE id = ?`, job.ID); n != 0 {
		t.Errorf("account_deletion_jobs row survived hard delete (count=%d)", n)
	}
}

// TestRunFinalStageExportRemovalFailureAbortsDeletion proves the failure
// branch: if the on-disk PII cannot be removed, runFinalStage returns an
// error and does NOT touch the database (no wipe, no hard delete). The
// removeAll seam makes the failure deterministic regardless of test uid.
func TestRunFinalStageExportRemovalFailureAbortsDeletion(t *testing.T) {
	ctx := context.Background()
	database := dbtest.SetupTestDB(t)
	q := db.NewSQLiteQuery(database).(*db.SQLiteQuery)
	w, cfg, _ := newDeletionTestWorker(t, q)

	user, err := q.CreateUserWithPassword(ctx, "keep-me@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUserWithPassword: %v", err)
	}
	job, err := q.ScheduleAccountDeletion(ctx, user.ID, 0)
	if err != nil {
		t.Fatalf("ScheduleAccountDeletion: %v", err)
	}
	zipPath := filepath.Join(cfg.ExportStorageDir, user.ID, "job-1.zip")
	if err := os.MkdirAll(filepath.Dir(zipPath), 0o750); err != nil {
		t.Fatalf("mkdir export dir: %v", err)
	}
	if err := os.WriteFile(zipPath, []byte("full-pii-bundle"), 0o640); err != nil {
		t.Fatalf("write zip: %v", err)
	}
	uid := user.ID
	if _, err := q.LogEmailAttempt(ctx, db.EmailAttemptInput{
		UserID:         &uid,
		Template:       "welcome",
		IdempotencyKey: "keep:" + user.ID,
		Status:         "sent",
		AttemptCount:   1,
		ToAddress:      user.Email,
	}); err != nil {
		t.Fatalf("seed email_send_log: %v", err)
	}
	if err := q.InsertAuditLog(ctx, user.ID, "login", "{}", ""); err != nil {
		t.Fatalf("seed audit_log: %v", err)
	}

	prev := removeAll
	removeAll = func(string) error { return errors.New("simulated unlink failure") }
	defer func() { removeAll = prev }()

	if err := w.runFinalStage(ctx, job, user); err == nil {
		t.Fatal("runFinalStage = nil, want error when export dir cannot be removed")
	}

	// Nothing on disk was deleted, and the user + PII rows survive.
	if _, err := os.Stat(zipPath); err != nil {
		t.Errorf("export ZIP removed despite failure: %v", err)
	}
	alive, err := q.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if alive == nil {
		t.Errorf("user was hard-deleted despite export-removal failure")
	}
	if n := countRows(t, ctx, q, `SELECT COUNT(*) FROM email_send_log WHERE user_id = ?`, user.ID); n != 1 {
		t.Errorf("email_send_log rows = %d, want 1 (no wipe on failure)", n)
	}
	if n := countRows(t, ctx, q, `SELECT COUNT(*) FROM audit_log WHERE user_id = ?`, user.ID); n != 1 {
		t.Errorf("audit_log rows = %d, want 1 (no wipe on failure)", n)
	}
	if n := countRows(t, ctx, q, `SELECT COUNT(*) FROM account_deletion_jobs WHERE id = ? AND status = 'completed'`, job.ID); n != 0 {
		t.Errorf("account_deletion_jobs marked completed despite failure")
	}
	if n := countRows(t, ctx, q, `SELECT COUNT(*) FROM account_deletion_jobs WHERE id = ?`, job.ID); n != 1 {
		t.Errorf("account_deletion_jobs row = %d, want 1 (job retained on failure)", n)
	}
}
