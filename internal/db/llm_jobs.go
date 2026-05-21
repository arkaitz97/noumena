package db

import (
	"context"
	"encoding/json"
)

func (q *SQLiteQuery) EnqueueLLMJob(ctx context.Context, commentIDs []string, provider string) (string, error) {
	id := generateUUID()
	commentIDsJSON, _ := json.Marshal(commentIDs)
	var jobID string
	err := q.db.QueryRowContext(ctx,
		`INSERT INTO llm_processing_jobs (id, status, comment_ids, provider)
		 VALUES (?, 'pending', ?, NULLIF(?, ''))
		 RETURNING id`,
		id, string(commentIDsJSON), provider).Scan(&jobID)
	return jobID, err
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
