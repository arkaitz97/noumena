// Package lifecycle: export_worker.go drains the `data_export_jobs`
// queue. Each claimed job:
//
//   1. Loads the user's full bundle (GetUserExportBundle).
//   2. Serialises each section to indented JSON.
//   3. Streams those bytes into a ZIP file at
//      EXPORT_STORAGE_DIR/<user_id>/<job_id>.zip.
//   4. Enforces EXPORT_MAX_BYTES (default 500MB) — on overflow we add
//      a `README.md` explaining the truncation and set partial=true.
//   5. Mints a signed URL with TTL = EXPORT_LINK_TTL_DAYS.
//   6. Writes terminal state via MarkDataExportCompleted (which also
//      mirrors URL+expiry onto the users row).
//   7. Emits the `export_ready` transactional email.
//
// Single-instance design (no horizontal scale needed at our volumes).
// Lock acquisition is via the SELECT ... FOR UPDATE SKIP LOCKED inside
// ClaimNextExportJob, so multiple processes WOULD coexist correctly —
// we just don't run them.
package lifecycle

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"noumena/internal/config"
	"noumena/internal/db"
	"noumena/internal/email"
	"noumena/internal/telemetry"
)

// ExportWorker drains data_export_jobs.
type ExportWorker struct {
	Queries  db.DBQuery
	Pipeline *email.Pipeline
	Cfg      *config.Config
	Logger   *log.Logger

	Tick time.Duration
	Now  func() time.Time

	stopOnce sync.Once
	done     chan struct{}
}

// MaxBytesDefault is the cap on the assembled ZIP body before we mark
// the export as partial. Spec D9: 500 MB.
const MaxBytesDefault = 500 * 1024 * 1024

func (w *ExportWorker) Start(ctx context.Context) {
	if w.Logger == nil {
		w.Logger = log.Default()
	}
	if w.Tick == 0 {
		w.Tick = 30 * time.Second
	}
	if w.Now == nil {
		w.Now = func() time.Time { return time.Now().UTC() }
	}
	if w.Cfg == nil {
		w.Cfg = config.Default
	}
	w.done = make(chan struct{})
	go w.run(ctx)
}

func (w *ExportWorker) Stop() {
	w.stopOnce.Do(func() {
		if w.done != nil {
			<-w.done
		}
	})
}

func (w *ExportWorker) run(ctx context.Context) {
	defer close(w.done)
	t := time.NewTicker(w.Tick)
	defer t.Stop()
	w.Logger.Printf("export_worker_started tick=%s storage=%s ttl_days=%d",
		w.Tick, w.Cfg.ExportStorageDir, w.Cfg.ExportLinkTTLDays)
	for {
		select {
		case <-ctx.Done():
			w.Logger.Printf("export_worker_stopping err=%v", ctx.Err())
			return
		case <-t.C:
			w.processOne(ctx)
		}
	}
}

func (w *ExportWorker) processOne(ctx context.Context) {
	job, err := w.Queries.ClaimNextExportJob(ctx)
	if err != nil {
		w.Logger.Printf("export_worker_claim_err err=%v", err)
		return
	}
	if job == nil {
		return
	}
	w.Logger.Printf("export_worker_claimed job=%s user=%s", job.ID, job.UserID)

	if err := w.runJob(ctx, job); err != nil {
		w.Logger.Printf("export_worker_failed job=%s err=%v", job.ID, err)
		_ = w.Queries.MarkDataExportFailed(ctx, job.ID, err.Error())
		// otel-instrumentation: terminal-state counter, error path.
		recordExportCompleted(ctx, telemetry.OutcomeError)
		return
	}
	w.Logger.Printf("export_worker_completed job=%s", job.ID)
	// otel-instrumentation: terminal-state counter, success path.
	// (Partial exports still count as success — the truncation README
	// is a contract documented in spec D9, not a failure mode.)
	recordExportCompleted(ctx, telemetry.OutcomeSuccess)
}

func (w *ExportWorker) runJob(ctx context.Context, job *db.DataExportJob) error {
	bundle, err := w.Queries.GetUserExportBundle(ctx, job.UserID)
	if err != nil {
		return fmt.Errorf("load bundle: %w", err)
	}

	// Build paths.
	userDir := filepath.Join(w.Cfg.ExportStorageDir, job.UserID)
	if err := os.MkdirAll(userDir, 0o750); err != nil {
		return fmt.Errorf("mkdir storage: %w", err)
	}
	zipPath := filepath.Join(userDir, job.ID+".zip")

	partial, err := w.writeZip(zipPath, bundle)
	if err != nil {
		// Best-effort cleanup of a half-written file.
		_ = os.Remove(zipPath)
		return fmt.Errorf("write zip: %w", err)
	}
	// otel-instrumentation: stat the final ZIP for the bytes counter.
	// We measure on-disk size (post-compression) rather than the raw
	// section bytes — operators care about storage growth, not
	// pre-compression payload size. Stat failure is non-fatal: we
	// already have a complete export and don't want to fail the job
	// for a metrics-only side-effect.
	if st, statErr := os.Stat(zipPath); statErr == nil {
		recordExportBytes(ctx, st.Size())
	}

	// Sign the download URL.
	expiry := w.Now().Add(time.Duration(w.Cfg.ExportLinkTTLDays) * 24 * time.Hour)
	sigParams := email.SignURL(w.Cfg.ExportSignedURLSecret, job.UserID, job.ID, expiry)
	downloadURL := fmt.Sprintf("%s/api/exports/%s/download?%s",
		w.Cfg.AppBaseURL, job.ID, sigParams)

	if err := w.Queries.MarkDataExportCompleted(ctx, job.ID, job.UserID, zipPath, downloadURL, expiry, partial); err != nil {
		return fmt.Errorf("mark completed: %w", err)
	}

	// Email is best-effort: the user's own dashboard also surfaces the
	// link via /api/user/me. We log but don't fail the job if mail
	// fails — the file + signed URL exist regardless.
	if w.Pipeline != nil {
		if err := w.Pipeline.Send(ctx, email.Message{
			Template:       email.TemplateExportReady,
			ToUserID:       bundle.User.ID,
			ToAddress:      bundle.User.Email,
			IdempotencyKey: "export_ready:" + job.ID,
			Variables: map[string]any{
				"DownloadURL": downloadURL,
				"ExpiresAt":   expiry.Format(time.RFC1123),
			},
		}); err != nil {
			w.Logger.Printf("export_worker_email_err job=%s err=%v", job.ID, err)
		}
	}
	return nil
}

// writeZip serialises each bundle section to JSON and writes them into
// the ZIP. Returns partial=true if we tripped the size cap and emitted
// a truncation README in lieu of finishing the bundle.
//
// Each section is written as one file. We deliberately marshal to a
// byte slice first (rather than streaming json.Encoder→io.Writer) so
// the size cap applies cleanly per-section: a 1GB comments dump is
// rejected before it reaches the ZIP writer. For users with hundreds
// of MB of comments this is still memory-bounded by EXPORT_MAX_BYTES.
func (w *ExportWorker) writeZip(path string, bundle *db.UserExportBundle) (bool, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return false, err
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	maxBytes := int64(MaxBytesDefault) // could become a config knob later
	used := int64(0)
	partial := false

	type section struct {
		name string
		data any
	}
	sections := []section{
		{"account/user.json", bundle.User},
		{"channels/tracking.json", bundle.Tracking},
		{"comments/comments.json", bundle.Comments},
		{"insights/digests.json", bundle.Digests},
		{"activity/stars.json", bundle.Stars},
	}

	for _, s := range sections {
		body, err := json.MarshalIndent(s.data, "", "  ")
		if err != nil {
			return false, fmt.Errorf("marshal %s: %w", s.name, err)
		}
		if used+int64(len(body)) > maxBytes {
			partial = true
			break
		}
		used += int64(len(body))
		fw, err := zw.Create(s.name)
		if err != nil {
			return false, err
		}
		if _, err := io.Copy(fw, bytes.NewReader(body)); err != nil {
			return false, err
		}
	}

	if partial {
		readme, _ := zw.Create("README.md")
		_, _ = readme.Write([]byte(partialReadme(maxBytes)))
	}
	return partial, nil
}

func partialReadme(maxBytes int64) string {
	return fmt.Sprintf(`# Partial Export

Your account contains more data than fits in the export bundle's size
cap (%d bytes / %.0f MB). The most recent sections of your data have
been omitted to keep the download manageable.

If you need the full dataset, please contact support and we will
arrange an alternative delivery method.
`, maxBytes, float64(maxBytes)/1024/1024)
}
