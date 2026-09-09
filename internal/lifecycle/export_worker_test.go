package lifecycle

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"noumena/internal/config"
	"noumena/internal/db"
	"noumena/internal/dbtest"
)

// newTestExportWorker builds an ExportWorker over a real SQLite (migrated
// by the dbtest harness) writing ZIPs into a temp storage dir.
func newTestExportWorker(t *testing.T, database *db.SQLiteQuery) (*ExportWorker, *config.Config) {
	t.Helper()
	cfg := *config.Default // copy; mutate without touching the global default
	cfg.ExportStorageDir = t.TempDir()
	cfg.ExportLinkTTLDays = 7
	cfg.ExportSignedURLSecret = "test-secret"
	cfg.AppBaseURL = "https://example.test"
	w := &ExportWorker{
		Queries: database,
		Cfg:     &cfg,
		Logger:  log.New(io.Discard, "", 0),
		Now:     func() time.Time { return time.Now().UTC() },
	}
	return w, &cfg
}

func seedExportUser(t *testing.T, q *db.SQLiteQuery, email string) *db.User {
	t.Helper()
	u, err := q.CreateUserWithPassword(context.Background(), email, "hash")
	if err != nil {
		t.Fatalf("CreateUserWithPassword: %v", err)
	}
	return u
}

// Task 1.3 + 5.1/5.2: full end-to-end on real SQLite — claim -> ZIP ->
// mark -> read back, with export_url persisted in the same tx as the
// 'completed' flip (no URL-less completed row).
func TestExportWorkerProcessCompletesEndToEnd(t *testing.T) {
	ctx := context.Background()
	database := dbtest.SetupTestDB(t)
	q := db.NewSQLiteQuery(database).(*db.SQLiteQuery)
	user := seedExportUser(t, q, "export@example.com")

	job, err := q.EnqueueDataExport(ctx, user.ID)
	if err != nil {
		t.Fatalf("EnqueueDataExport: %v", err)
	}
	if job.Status != "queued" {
		t.Fatalf("status: got %q want queued", job.Status)
	}

	w, cfg := newTestExportWorker(t, q)
	w.processOne(ctx)

	re, err := q.GetDataExportJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetDataExportJob: %v", err)
	}
	if re.Status != "completed" {
		t.Fatalf("status after process: got %q want completed", re.Status)
	}
	if re.ExportURL == nil || !strings.HasPrefix(*re.ExportURL, cfg.AppBaseURL) {
		t.Fatalf("completed job missing signed export_url: %v", re.ExportURL)
	}
	if re.ErrorMessage != nil {
		t.Fatalf("completed job has error_message: %v", *re.ErrorMessage)
	}

	// The ZIP must exist at the deterministic path and be non-empty.
	zipPath := filepath.Join(cfg.ExportStorageDir, user.ID, job.ID+".zip")
	st, err := os.Stat(zipPath)
	if err != nil {
		t.Fatalf("zip not written: %v", err)
	}
	if st.Size() == 0 {
		t.Fatalf("zip is empty")
	}

	// The users mirror row gets the URL too.
	u, err := q.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if u.DataExportURL == nil || *u.DataExportURL == "" {
		t.Fatalf("users.data_export_url not mirrored")
	}

	// Invariant: a completed row always carries a URL.
	var nulls int
	if err := database.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM data_export_jobs WHERE status='completed' AND export_url IS NULL`).Scan(&nulls); err != nil {
		t.Fatalf("invariant query: %v", err)
	}
	if nulls != 0 {
		t.Fatalf("found %d completed rows without export_url", nulls)
	}
}

// Task 3.3 + 4.2 + 5.2: a crash leaves the row 'exporting' with a partial
// ZIP; next boot the reaper removes the file and requeues the job.
func TestExportWorkerCrashRecoveryResetsStuckAndSweepsOrphans(t *testing.T) {
	ctx := context.Background()
	database := dbtest.SetupTestDB(t)
	q := db.NewSQLiteQuery(database).(*db.SQLiteQuery)
	// Distinct users: data_export_jobs enforces one inflight job per user.
	stuckUser := seedExportUser(t, q, "stuck@example.com")
	failedUser := seedExportUser(t, q, "failed@example.com")
	keepUser := seedExportUser(t, q, "keep@example.com")

	w, cfg := newTestExportWorker(t, q)
	writeZip := func(jobID, userID string) string {
		p := filepath.Join(cfg.ExportStorageDir, userID, jobID+".zip")
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(p, []byte("partial"), 0o640); err != nil {
			t.Fatalf("write zip: %v", err)
		}
		return p
	}

	// Crash leftover: 'exporting' for 45m (past the 30m threshold) + file.
	stuck, err := q.EnqueueDataExport(ctx, stuckUser.ID)
	if err != nil {
		t.Fatalf("EnqueueDataExport: %v", err)
	}
	old := time.Now().UTC().Add(-45 * time.Minute).Format(time.RFC3339)
	if _, err := database.ExecContext(ctx,
		`UPDATE data_export_jobs SET status='exporting', started_at=? WHERE id=?`, old, stuck.ID); err != nil {
		t.Fatalf("mark exporting: %v", err)
	}
	stuckZip := writeZip(stuck.ID, stuckUser.ID)

	// Legacy failed job with an orphan file: file must go, row stays failed.
	failed, err := q.EnqueueDataExport(ctx, failedUser.ID)
	if err != nil {
		t.Fatalf("EnqueueDataExport: %v", err)
	}
	if _, err := database.ExecContext(ctx,
		`UPDATE data_export_jobs SET status='failed' WHERE id=?`, failed.ID); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	failedZip := writeZip(failed.ID, failedUser.ID)

	// Queued job keeps any file (never had one; sweep must not touch it).
	keep, err := q.EnqueueDataExport(ctx, keepUser.ID)
	if err != nil {
		t.Fatalf("EnqueueDataExport: %v", err)
	}
	keepZip := writeZip(keep.ID, keepUser.ID)

	w.recoverAtBoot(ctx)

	re, err := q.GetDataExportJob(ctx, stuck.ID)
	if err != nil {
		t.Fatalf("GetDataExportJob: %v", err)
	}
	if re.Status != "queued" {
		t.Fatalf("stuck job status: got %q want queued", re.Status)
	}
	if _, err := os.Stat(stuckZip); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stuck job zip not swept: %v", err)
	}

	reF, err := q.GetDataExportJob(ctx, failed.ID)
	if err != nil {
		t.Fatalf("GetDataExportJob: %v", err)
	}
	if reF.Status != "failed" {
		t.Fatalf("failed job status changed: got %q", reF.Status)
	}
	if _, err := os.Stat(failedZip); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed job zip not swept: %v", err)
	}

	if _, err := os.Stat(keepZip); err != nil {
		t.Fatalf("queued job file must survive the sweep: %v", err)
	}
}

// failingMarkQueries forces MarkDataExportCompleted to fail while the
// rest of the surface is real.
type failingMarkQueries struct {
	dataExportStore
	markErr error
}

func (f *failingMarkQueries) MarkDataExportCompleted(ctx context.Context, jobID, userID, storagePath, downloadURL string, expiresAt time.Time, partial bool) error {
	return f.markErr
}

// Task 4.3: mark-completion failure removes the orphan ZIP and returns
// the job to the retryable 'queued' state (never 'failed').
func TestExportWorkerMarkFailureLeavesNoOrphan(t *testing.T) {
	ctx := context.Background()
	database := dbtest.SetupTestDB(t)
	q := db.NewSQLiteQuery(database).(*db.SQLiteQuery)
	user := seedExportUser(t, q, "export@example.com")

	job, err := q.EnqueueDataExport(ctx, user.ID)
	if err != nil {
		t.Fatalf("EnqueueDataExport: %v", err)
	}

	w, cfg := newTestExportWorker(t, q)
	w.Queries = &failingMarkQueries{dataExportStore: q, markErr: errors.New("boom")}

	w.processOne(ctx)

	re, err := q.GetDataExportJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetDataExportJob: %v", err)
	}
	if re.Status != "queued" {
		t.Fatalf("status after mark failure: got %q want queued", re.Status)
	}
	if re.StartedAt != nil {
		t.Fatalf("requeued job must have NULL started_at")
	}
	zipPath := filepath.Join(cfg.ExportStorageDir, user.ID, job.ID+".zip")
	if _, err := os.Stat(zipPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("orphan zip left behind: %v", err)
	}
}

func TestExportWorkerIncludesUserProfile(t *testing.T) {
	ctx := context.Background()
	database := dbtest.SetupTestDB(t)
	q := db.NewSQLiteQuery(database).(*db.SQLiteQuery)
	user := seedExportUser(t, q, "profile_export@example.com")

	userType := "creator"
	goal := "understand_audience"
	topics := []string{"tech", "coding"}
	if _, err := q.UpsertUserProfile(ctx, &db.UserProfile{
		UserID:        user.ID,
		UserType:      &userType,
		PrimaryGoal:   &goal,
		ContentTopics: topics,
	}); err != nil {
		t.Fatalf("UpsertUserProfile: %v", err)
	}

	job, err := q.EnqueueDataExport(ctx, user.ID)
	if err != nil {
		t.Fatalf("EnqueueDataExport: %v", err)
	}

	w, cfg := newTestExportWorker(t, q)
	w.processOne(ctx)

	zipPath := filepath.Join(cfg.ExportStorageDir, user.ID, job.ID+".zip")
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer zr.Close()

	var foundProfile bool
	for _, f := range zr.File {
		if f.Name == "profile/profile.json" {
			foundProfile = true
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open profile/profile.json: %v", err)
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatalf("read profile/profile.json: %v", err)
			}
			if !strings.Contains(string(data), `"creator"`) {
				t.Errorf("profile JSON missing 'creator': %s", string(data))
			}
			if !strings.Contains(string(data), `"understand_audience"`) {
				t.Errorf("profile JSON missing 'understand_audience': %s", string(data))
			}
		}
	}
	if !foundProfile {
		t.Errorf("profile/profile.json not found in ZIP export")
	}
}

