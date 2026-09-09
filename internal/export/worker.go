package export

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"noumena/internal/db"
)

type Worker struct {
	Queries *db.SQLiteQuery
	// Pool is the query_only export read handle (db.ExportDB), NOT the
	// main write pool: streaming a long export must never occupy the
	// main pool's single connection and starve writers. Falls back to
	// the main DB when no dedicated handle exists (in-memory tests).
	Pool         *sql.DB
	ExportDir    string
	TTLHours     int
	TickInterval time.Duration

	Logger *log.Logger
}

func (w *Worker) logger() *log.Logger {
	if w.Logger != nil {
		return w.Logger
	}
	return log.Default()
}

func (w *Worker) Run(ctx context.Context) {
	if w.TickInterval <= 0 {
		w.TickInterval = 5 * time.Second
	}
	if w.TTLHours <= 0 {
		w.TTLHours = 24
	}

	if n, err := w.Queries.ResetStuckProcessingExports(ctx, 30*time.Minute); err != nil {
		w.logger().Printf("export-worker: startup reset failed: %v", err)
	} else if n > 0 {
		w.logger().Printf("export-worker: startup reset %d stuck rows to queued", n)
	}
	w.runCleanupOnce(ctx)

	t := time.NewTicker(w.TickInterval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger().Println("export-worker: shutting down")
			return
		case <-t.C:
			w.tick(ctx)
		}
	}
}

func (w *Worker) tick(ctx context.Context) {
	exp, err := w.Queries.ClaimQueuedExport(ctx)
	if errors.Is(err, db.ErrNotFound) {
		return
	}
	if err != nil {
		w.logger().Printf("export-worker: claim failed: %v", err)
		return
	}

	if err := w.process(ctx, exp); err != nil {
		w.logger().Printf("export-worker: export %s failed: %v", exp.ID, err)
		retryable := exp.RetryCount < 3
		if mErr := w.Queries.MarkExportFailed(ctx, exp.ID, err.Error(), retryable); mErr != nil {
			w.logger().Printf("export-worker: MarkExportFailed: %v", mErr)
		}
		return
	}
}

func (w *Worker) process(ctx context.Context, exp *db.Export) error {
	userDir := filepath.Join(w.ExportDir, exp.UserID)
	if err := os.MkdirAll(userDir, 0o700); err != nil {
		return fmt.Errorf("mkdir user export dir: %w", err)
	}
	ext := exp.Format
	filePath := filepath.Join(userDir, exp.ID+"."+ext)

	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open export file: %w", err)
	}

	var filters db.FeedFilters
	if err := unmarshalExportFilters(exp.Filters, &filters); err != nil {
		_ = f.Close()
		return fmt.Errorf("decode filters: %w", err)
	}

	rowCount, encErr := StreamExport(ctx, w.Pool, exp.UserID, filters, exp.Format, f)
	if encErr != nil {
		_ = f.Close()
		return fmt.Errorf("stream export: %w", encErr)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("fsync: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close export: %w", err)
	}
	return w.Queries.MarkExportCompleted(ctx, exp.ID, filePath, rowCount, w.TTLHours)
}

func (w *Worker) RunCleanup(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.runCleanupOnce(ctx)
		}
	}
}

func (w *Worker) runCleanupOnce(ctx context.Context) {
	expired, err := w.Queries.ListExpiredExports(ctx)
	if err != nil {
		w.logger().Printf("export-worker: ListExpiredExports: %v", err)
		return
	}
	for _, exp := range expired {
		if exp.FilePath != nil {
			if err := os.Remove(*exp.FilePath); err != nil && !os.IsNotExist(err) {
				w.logger().Printf("export-worker: remove %s: %v", *exp.FilePath, err)
			}
		}
		if err := w.Queries.MarkExportExpired(ctx, exp.ID); err != nil {
			w.logger().Printf("export-worker: MarkExportExpired %s: %v", exp.ID, err)
		}
	}
}

func unmarshalExportFilters(raw []byte, out *db.FeedFilters) error {
	if len(raw) == 0 {
		return nil
	}
	type wireFilters struct {
		Channels     []string `json:"channels"`
		Categories   []string `json:"categories"`
		InsightType  string   `json:"insight_type"`
		DateFrom     *string  `json:"date_from"`
		DateTo       *string  `json:"date_to"`
		Search       *string  `json:"search"`
		IncludeToxic bool     `json:"include_toxic"`
		Sort         string   `json:"sort"`
	}
	var w wireFilters
	if err := json.Unmarshal(raw, &w); err != nil {
		return err
	}
	out.Channels = w.Channels
	out.Categories = w.Categories
	out.InsightType = w.InsightType
	out.Search = w.Search
	out.IncludeToxic = w.IncludeToxic
	out.Sort = w.Sort
	if w.DateFrom != nil {
		t, err := time.Parse(time.RFC3339, *w.DateFrom)
		if err == nil {
			out.DateFrom = &t
		}
	}
	if w.DateTo != nil {
		t, err := time.Parse(time.RFC3339, *w.DateTo)
		if err == nil {
			out.DateTo = &t
		}
	}
	return nil
}
