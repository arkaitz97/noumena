package db

import (
	"context"
)

// EnqueueLLMJob inserts a new llm_processing_jobs row in 'pending' state and
// returns its UUID. The provider may be empty if not yet known at enqueue time
// (the failover wrapper picks the provider just before dispatch).
func (q *PostgresQuery) EnqueueLLMJob(ctx context.Context, commentIDs []string, provider string) (string, error) {
	var id string
	err := q.pool.QueryRow(ctx,
		`INSERT INTO llm_processing_jobs (status, comment_ids, provider)
		 VALUES ('pending', $1, NULLIF($2, ''))
		 RETURNING id`,
		commentIDs, provider).Scan(&id)
	return id, err
}

// MarkLLMJobProcessing transitions a pending row to 'processing' and stamps
// started_at. Used by Tx1 once a batch has been claimed.
func (q *PostgresQuery) MarkLLMJobProcessing(ctx context.Context, jobID string) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE llm_processing_jobs
		 SET status = 'processing', started_at = NOW()
		 WHERE id = $1`,
		jobID)
	return err
}

// MarkLLMJobCompleted finalizes a job as successfully written.
func (q *PostgresQuery) MarkLLMJobCompleted(ctx context.Context, jobID string) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE llm_processing_jobs
		 SET status = 'completed', completed_at = NOW()
		 WHERE id = $1`,
		jobID)
	return err
}

// MarkLLMJobFailed records a failure with a truncated (4KB) error message.
func (q *PostgresQuery) MarkLLMJobFailed(ctx context.Context, jobID string, errorMessage string) error {
	const maxLen = 4096
	if len(errorMessage) > maxLen {
		errorMessage = errorMessage[:maxLen]
	}
	_, err := q.pool.Exec(ctx,
		`UPDATE llm_processing_jobs
		 SET status = 'failed',
		     error_message = $2,
		     completed_at = NOW(),
		     retry_count = retry_count + 1
		 WHERE id = $1`,
		jobID, errorMessage)
	return err
}
