package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

func (q *SQLiteQuery) EnqueueLLMJob(ctx context.Context, commentIDs []string, provider string) (string, error) {
	id, _, err := q.EnqueueLLMJobIfNotExists(ctx, commentIDs, provider)
	return id, err
}

// llmOrphanAge bounds how long a pending/processing job may block its comment
// set via dedupe. Must exceed the largest configured BatchTimeout so a live
// (slow) LLM call is never mistaken for an orphan; BatchTimeout caps in-flight
// at 300s, so 10m is safe. The reaper's 15m sweep is too slow — every tick
// would batch_deduped against the corpse until then.
const llmOrphanAge = 10 * time.Minute

// EnqueueLLMJobIfNotExists dedupes: pending/processing job with identical comment set → return existing id, created=false.
// Stale orphans (pending, or processing older than llmOrphanAge) are expired
// to failed first so they cannot wedge their comment set indefinitely.
func (q *SQLiteQuery) EnqueueLLMJobIfNotExists(ctx context.Context, commentIDs []string, provider string) (string, bool, error) {
	commentIDsJSON, _ := json.Marshal(commentIDs)
	payload := string(commentIDsJSON)
	cutoff := time.Now().UTC().Add(-llmOrphanAge).Format(time.RFC3339)

	// Expire any stale rows that would otherwise shadow this comment set.
	if _, err := q.db.ExecContext(ctx,
		`UPDATE llm_processing_jobs
		   SET status = 'failed',
		       error_message = 'stale orphan; expired for re-enqueue',
		       completed_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		 WHERE status IN ('pending','processing')
		   AND comment_ids = ?
		   AND COALESCE(started_at, created_at) < ?`,
		payload, cutoff); err != nil {
		return "", false, err
	}

	var existing string
	err := q.db.QueryRowContext(ctx,
		`SELECT id FROM llm_processing_jobs
		  WHERE status IN ('pending','processing') AND comment_ids = ?
		  LIMIT 1`,
		payload).Scan(&existing)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", false, err
	}
	id := generateUUID()
	var jobID string
	err = q.db.QueryRowContext(ctx,
		`INSERT INTO llm_processing_jobs (id, status, comment_ids, provider)
		 VALUES (?, 'pending', ?, NULLIF(?, ''))
		 RETURNING id`,
		id, payload, provider).Scan(&jobID)
	if err != nil {
		return "", false, err
	}
	return jobID, true, nil
}

func (q *SQLiteQuery) MarkLLMJobProcessing(ctx context.Context, jobID string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE llm_processing_jobs
		 SET status = 'processing', started_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		 WHERE id = ?`,
		jobID)
	return err
}

func (q *SQLiteQuery) MarkLLMJobCompleted(ctx context.Context, jobID string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE llm_processing_jobs
		 SET status = 'completed', completed_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		 WHERE id = ?`,
		jobID)
	return err
}

func (q *SQLiteQuery) MarkLLMJobFailed(ctx context.Context, jobID string, errorMessage string) error {
	const maxLen = 4096
	if len(errorMessage) > maxLen {
		errorMessage = errorMessage[:maxLen]
	}
	_, err := q.db.ExecContext(ctx,
		`UPDATE llm_processing_jobs
		 SET status = 'failed',
		     error_message = ?,
		     completed_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
		     retry_count = retry_count + 1
		 WHERE id = ?`,
		errorMessage, jobID)
	return err
}

// RecoverStaleLLMJobs marks processing jobs older than olderThan as failed.
// Failed (never pending) is deliberate: a reset-to-pending row still matches
// the dedupe in EnqueueLLMJobIfNotExists (SELECT matches IN
// ('pending','processing')) yet is never re-claimed, so its comment set
// would batch_deduped forever. Failure records the orphan as terminal and
// unblocks re-enqueue.
func (q *SQLiteQuery) RecoverStaleLLMJobs(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339)
	res, err := q.db.ExecContext(ctx,
		`UPDATE llm_processing_jobs
		    SET status = 'failed',
		        error_message = 'orphaned by restart; expired by reaper',
		        completed_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		  WHERE status = 'processing'
		    AND started_at IS NOT NULL
		    AND started_at < ?`,
		cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
