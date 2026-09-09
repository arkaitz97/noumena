// Package lifecycle: cleanup.go runs the daily export-cleanup cron.
// Walks `data_export_jobs` for rows where expires_at < NOW() and
// export_url IS NOT NULL, removes the ZIP file, then nulls out
// export_url so the row stops appearing in the sweep.
//
// The schema stores only the signed download URL, not the local file
// path; the path derives deterministically from the storage dir +
// user id + job id (mirroring export_worker.go's layout), so cleanup
// and the download route can both locate the file without a path column.
//
// Why a separate cron rather than fold this into the export worker?
// The export worker only fires on new jobs; expired files belong to
// jobs that may have completed weeks ago. A daily sweep is the right
// cadence for a TTL of 7 days.
package lifecycle

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"noumena/internal/db"
)

// CleanupCron deletes expired export files. Schedule once per day at
// 08:00 UTC (alongside other daily jobs — see billing/quota.go's
// scheduling helper for the same convention).
type CleanupCron struct {
	Queries db.DBQuery
	Logger  *log.Logger

	// StorageDir is the GDPR export root (EXPORT_STORAGE_DIR). Files
	// live at <StorageDir>/<user_id>/<job_id>.zip. Required: without it
	// the sweep cannot locate files and logs a warning instead.
	StorageDir string

	// Hour/Minute set the daily run time in UTC. Default 08:00.
	Hour, Minute int

	stopOnce sync.Once
	done     chan struct{}
}

func (c *CleanupCron) Start(ctx context.Context) {
	if c.Logger == nil {
		c.Logger = log.Default()
	}
	if c.Hour == 0 && c.Minute == 0 {
		c.Hour = 8
	}
	c.done = make(chan struct{})
	go c.run(ctx)
}

func (c *CleanupCron) Stop() {
	c.stopOnce.Do(func() {
		if c.done != nil {
			<-c.done
		}
	})
}

func (c *CleanupCron) run(ctx context.Context) {
	defer close(c.done)
	for {
		wait := untilNextDailyUTC(time.Now().UTC(), c.Hour, c.Minute)
		c.Logger.Printf("export_cleanup_sleep next_run_in=%s", wait)
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
			c.runOnce(ctx)
		}
	}
}

// RunOnce is exported for tests + manual triggers; the periodic loop
// calls it.
func (c *CleanupCron) RunOnce(ctx context.Context) { c.runOnce(ctx) }

func (c *CleanupCron) runOnce(ctx context.Context) {
	now := time.Now().UTC()
	rows, err := c.Queries.ListExpiredDataExports(ctx, now)
	if err != nil {
		c.Logger.Printf("export_cleanup_list_err err=%v", err)
		return
	}
	if c.StorageDir == "" {
		c.Logger.Printf("export_cleanup_warn storage_dir_unset")
		return
	}
	for _, r := range rows {
		if r.ExportURL == nil {
			continue
		}
		path := filepath.Join(c.StorageDir, r.UserID, r.ID+".zip")
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			c.Logger.Printf("export_cleanup_unlink_err job=%s path=%s err=%v", r.ID, path, err)
			continue
		}
		// Try to remove the parent <user_id>/ directory if empty —
		// Remove (not RemoveAll) silently fails on non-empty dirs which
		// is exactly what we want.
		parent := filepath.Dir(path)
		_ = os.Remove(parent)

		if err := c.Queries.ClearExportStoragePath(ctx, r.ID); err != nil {
			c.Logger.Printf("export_cleanup_clear_err job=%s err=%v", r.ID, err)
			continue
		}
		c.Logger.Printf("export_cleanup_removed job=%s path=%s", r.ID, path)
	}
}

// untilNextDailyUTC duplicates the helper in billing/quota.go to avoid
// an import cycle with that package's bigger surface. The two
// implementations are kept in sync by code review.
func untilNextDailyUTC(now time.Time, hour, minute int) time.Duration {
	target := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.UTC)
	if !target.After(now) {
		target = target.Add(24 * time.Hour)
	}
	return target.Sub(now)
}
