package db

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// IncrementLLMAttempts bumps llm_attempts for the given comment ids and sets
// llm_next_retry_at. When park is true, llm_attempts is set to maxAttempts
// (dead-letter) and llm_next_retry_at cleared, so the claim query excludes
// the rows permanently. Not on DBQuery: test doubles opt in via type
// assertion (see processor.recordRetryState).
func (q *SQLiteQuery) IncrementLLMAttempts(ctx context.Context, ids []string, nextRetryAt *time.Time, park bool, maxAttempts int) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)+2)

	var retry any
	if nextRetryAt != nil && !park {
		retry = nextRetryAt.UTC().Format(time.RFC3339)
	} else {
		retry = nil
	}

	var query string
	if park {
		query = fmt.Sprintf(`UPDATE comments SET llm_attempts = ?, llm_next_retry_at = ? WHERE id IN (%s)`, placeholders)
		args = append(args, maxAttempts, retry)
	} else {
		query = fmt.Sprintf(`UPDATE comments SET llm_attempts = llm_attempts + 1, llm_next_retry_at = ? WHERE id IN (%s)`, placeholders)
		args = append(args, retry)
	}
	for _, id := range ids {
		args = append(args, id)
	}
	_, err := q.db.ExecContext(ctx, query, args...)
	return err
}

// MaxLLMAttempts returns the highest llm_attempts among the given ids (0 when
// none). Used to pick the next backoff slot for a batch being re-attempted.
func (q *SQLiteQuery) MaxLLMAttempts(ctx context.Context, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	var n int
	err := q.db.QueryRowContext(ctx,
		fmt.Sprintf(`SELECT COALESCE(MAX(llm_attempts), 0) FROM comments WHERE id IN (%s)`, placeholders),
		args...).Scan(&n)
	return n, err
}

// CountParkedComments counts unprocessed comments excluded from claiming
// because they reached the attempt cap.
func (q *SQLiteQuery) CountParkedComments(ctx context.Context, maxAttempts int) (int64, error) {
	var n int64
	err := q.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM comments
		  WHERE (processed_at IS NULL OR sentiment_score IS NULL)
		    AND body != ''
		    AND llm_attempts >= ?`,
		maxAttempts).Scan(&n)
	return n, err
}
