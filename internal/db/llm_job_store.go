package db

import (
	"context"
	"time"
)

// LLMJobStore is the narrow data-access surface for the llm_processing_jobs
// lifecycle.
type LLMJobStore interface {
	EnqueueLLMJob(ctx context.Context, commentIDs []string, provider string) (string, error)
	EnqueueLLMJobIfNotExists(ctx context.Context, commentIDs []string, provider string) (string, bool, error)
	RecoverStaleLLMJobs(ctx context.Context, olderThan time.Duration) (int64, error)
	MarkLLMJobProcessing(ctx context.Context, jobID string) error
	MarkLLMJobCompleted(ctx context.Context, jobID string) error
	MarkLLMJobFailed(ctx context.Context, jobID string, errorMessage string) error
}

var _ LLMJobStore = (DBQuery)(nil)
