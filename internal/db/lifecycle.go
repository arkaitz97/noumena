// Package db: lifecycle.go owns the data-lifecycle change's persistence
// surface — the `account_deletion_jobs` and `data_export_jobs` workflows
// plus the chunked, tracker-aware deletion helpers used by the deletion
// worker.
//
// Two design rules pervade this file and matter for review:
//
//   1. Deletion is OPT-IN destructive. Every helper that drops user data
//      requires an explicit user_id (or platform_channel_id we have just
//      verified is unreferenced); none of them accept a "delete all"
//      flag. The worker composes them into a sequence; the DB layer
//      refuses to be a bulk eraser.
//
//   2. The "abort if a new tracker appeared mid-delete" guarantee from
//      the spec is implemented by holding a SELECT ... FOR UPDATE on the
//      `platform_channels` row inside each chunk transaction. If the
//      tracker count climbs above zero (excluding the deleting user)
//      between chunks, DeleteCommentsByChannelChunked returns a sentinel
//      and the caller proceeds to the *user-only* deletion path
//      (orphaning the channel for someone else to keep tracking).
package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrNewTrackerAppeared is the sentinel returned by
// DeleteCommentsByChannelChunked when a concurrent tracker arrives mid-
// deletion. It is NOT a fatal error — the caller (deletion worker) is
// expected to log it and skip channel-level cleanup for this user.
var ErrNewTrackerAppeared = errors.New("lifecycle: another tracker appeared during chunked delete; aborting channel-level cleanup")

// TxBeginner is the narrow surface the lifecycle workers need from a
// connection pool: just `Begin` + `Exec` (the latter so the deletion
// worker can flip a single column on account_deletion_jobs without
// pulling in the full pgxpool.Pool API). pgxpool.Pool already satisfies
// this implicitly.
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// LifecycleStore is the data-lifecycle-specific surface of DBQuery,
// extracted so workers can depend on a narrower interface in tests.
// Embedded into DBQuery (db.go) so any production caller holding a
// DBQuery automatically has access.
type LifecycleStore interface {
	ScheduleAccountDeletion(ctx context.Context, userID string, totalChannels int) (*AccountDeletionJob, error)
	CancelAccountDeletion(ctx context.Context, userID string) error
	ClaimNextDeletionJob(ctx context.Context, gracePeriod time.Duration) (*AccountDeletionJob, error)
	MarkDeletionStage(ctx context.Context, jobID, status string) error
	MarkDeletionFailed(ctx context.Context, jobID, errorMsg string) error
	IncrementChannelsProcessed(ctx context.Context, jobID string) error
	MarkDeletionCompleted(ctx context.Context, jobID, userID string) error
	MarkDeletionCompletedJobOnly(ctx context.Context, jobID string) error
	GetUserTrackingChannels(ctx context.Context, userID string) ([]*UserChannelTrackingRow, error)
	GetActiveTrackerCountForUpdate(ctx context.Context, tx pgx.Tx, platformChannelID, excludeUserID string) (int, error)
	DeleteUserChannelTracking(ctx context.Context, tx pgx.Tx, userID, platformChannelID string) error
	DeleteCommentsByChannelChunked(ctx context.Context, platformChannelID string, chunkSize int, excludeUserID string) (int, error)
	DeletePlatformChannel(ctx context.Context, tx pgx.Tx, platformChannelID string) error
	DeleteUserStarsForChannel(ctx context.Context, tx pgx.Tx, userID, platformChannelID string) error
	DeleteUserStarsAll(ctx context.Context, tx pgx.Tx, userID string) error
	DeleteUserDigests(ctx context.Context, tx pgx.Tx, userID string) error
	DeleteUserAuthTokens(ctx context.Context, tx pgx.Tx, userID string) error
	HardDeleteUser(ctx context.Context, tx pgx.Tx, userID string) error
	EnqueueDataExport(ctx context.Context, userID string) (*DataExportJob, error)
	GetDataExportJob(ctx context.Context, jobID string) (*DataExportJob, error)
	ClaimNextExportJob(ctx context.Context) (*DataExportJob, error)
	MarkExportStarted(ctx context.Context, jobID string) error
	MarkDataExportCompleted(ctx context.Context, jobID, userID, storagePath, downloadURL string, expiresAt time.Time, partial bool) error
	MarkDataExportFailed(ctx context.Context, jobID, errorMsg string) error
	ListExpiredDataExports(ctx context.Context, now time.Time) ([]*DataExportJob, error)
	ClearExportStoragePath(ctx context.Context, jobID string) error
	GetUserExportBundle(ctx context.Context, userID string) (*UserExportBundle, error)
}

// AccountDeletionJob mirrors the `account_deletion_jobs` row. The status
// values are the strings the CHECK constraint accepts.
type AccountDeletionJob struct {
	ID                   string
	UserID               string
	Status               string // pending|exporting|deleting|completed|failed
	TotalChannels        int
	ChannelsProcessed    int
	GDPRExportGenerated  bool
	LastError            *string
	CreatedAt            time.Time
	StartedAt            *time.Time
	CompletedAt          *time.Time
}

// DataExportJob mirrors the `data_export_jobs` row.
type DataExportJob struct {
	ID            string
	UserID        string
	Status        string // queued|exporting|completed|failed
	StoragePath   *string
	DownloadURL   *string
	ExpiresAt     *time.Time
	PartialExport bool
	LastError     *string
	CreatedAt     time.Time
	StartedAt     *time.Time
	CompletedAt   *time.Time
}

// UserChannelTrackingRow is a flattened view of (user_channel_tracking
// JOIN platform_channels) optimized for the deletion worker — it carries
// just enough to reconnect to OAuth (revocation) and identify the
// channel for chunked delete.
type UserChannelTrackingRow struct {
	TrackingID         string
	UserID             string
	PlatformChannelID  string // platform_channels.id (UUID)
	Platform           string
	PlatformChannelKey string // platform-native id (e.g. UC...)
	DisplayName        string
	AccessTokenEnc     string
	TokenExpiresAt     *time.Time
}

// UserExportBundle is the in-memory shape the export worker serializes
// to per-section JSON files. We deliberately avoid streaming straight
// from rows.Scan into the ZIP — building the full bundle first lets us
// enforce the EXPORT_MAX_BYTES cap deterministically and produce a
// recognisable partial-export README when we trip it.
type UserExportBundle struct {
	User     *User
	Tracking []*UserChannelTracking
	Comments []*Comment
	Digests  []*WeeklyDigest
	Stars    []*UserCommentStar
}

// ---------------------------------------------------------------------
// Account deletion jobs (Section 6.2 - 6.5)
// ---------------------------------------------------------------------

// ScheduleAccountDeletion inserts a pending job for the user. If a
// pending/exporting/deleting job already exists, returns the existing
// row (idempotent). The caller (DELETE /api/user/account handler) has
// already set users.deletion_requested_at.
func (q *PostgresQuery) ScheduleAccountDeletion(ctx context.Context, userID string, totalChannels int) (*AccountDeletionJob, error) {
	// Idempotency: check first. We accept the small race where two
	// concurrent requests both miss; the worker tolerates duplicates by
	// claiming one and observing the other later (it short-circuits if
	// the user is already gone).
	existing, err := q.findActiveDeletionJob(ctx, userID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	row := q.pool.QueryRow(ctx, `
		INSERT INTO account_deletion_jobs (user_id, status, total_channels)
		VALUES ($1, 'pending', $2)
		RETURNING `+accountDeletionJobColumns, userID, totalChannels)
	return scanAccountDeletionJob(row)
}

// CancelAccountDeletion removes the pending job and clears
// users.deletion_requested_at. Allowed only while the job is still in
// the 'pending' grace window — once the worker has claimed it
// (status='exporting' or beyond) the deletion proceeds.
//
// Returns ErrNotFound if there is no cancellable job; the handler maps
// that to HTTP 409.
func (q *PostgresQuery) CancelAccountDeletion(ctx context.Context, userID string) error {
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx,
		`DELETE FROM account_deletion_jobs WHERE user_id = $1 AND status = 'pending'`,
		userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx,
		`UPDATE users SET deletion_requested_at = NULL, updated_at = NOW() WHERE id = $1`,
		userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ClaimNextDeletionJob atomically transitions the oldest pending job
// past its grace window into 'exporting' and returns it. Returns
// (nil, nil) when there is nothing to claim (the worker treats this as
// "sleep until next tick").
//
// `gracePeriod` is the value of DELETION_REVERSIBLE_HOURS expressed as
// a Duration. We compute the cutoff in Go rather than SQL so tests can
// pump time deterministically; SQL still enforces the read by comparing
// against a parameter.
func (q *PostgresQuery) ClaimNextDeletionJob(ctx context.Context, gracePeriod time.Duration) (*AccountDeletionJob, error) {
	cutoff := time.Now().UTC().Add(-gracePeriod)
	row := q.pool.QueryRow(ctx, `
		UPDATE account_deletion_jobs
		SET status = 'exporting', started_at = NOW()
		WHERE id = (
			SELECT id FROM account_deletion_jobs
			WHERE status = 'pending' AND created_at <= $1
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		RETURNING `+accountDeletionJobColumns, cutoff)
	job, err := scanAccountDeletionJob(row)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return job, err
}

// MarkDeletionStage moves a job to the named status without other side
// effects. Used by the worker to advance from exporting → deleting.
func (q *PostgresQuery) MarkDeletionStage(ctx context.Context, jobID, status string) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE account_deletion_jobs SET status = $2 WHERE id = $1`, jobID, status)
	return err
}

// MarkDeletionFailed records a fatal error and stops the job. The
// worker's caller should fire an ops alert; the row is left in 'failed'
// so a human can re-trigger after fixing the root cause.
func (q *PostgresQuery) MarkDeletionFailed(ctx context.Context, jobID, errorMsg string) error {
	_, err := q.pool.Exec(ctx, `
		UPDATE account_deletion_jobs
		SET status = 'failed', last_error = $2, completed_at = NOW()
		WHERE id = $1`, jobID, errorMsg)
	return err
}

// IncrementChannelsProcessed bumps the progress counter exposed via
// admin views. Called once per channel the worker successfully detaches.
func (q *PostgresQuery) IncrementChannelsProcessed(ctx context.Context, jobID string) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE account_deletion_jobs SET channels_processed = channels_processed + 1 WHERE id = $1`,
		jobID)
	return err
}

// MarkDeletionCompleted sets terminal state on the job AND mirrors the
// completion timestamp onto users.deletion_completed_at. Both writes
// happen in one transaction so the user row never disagrees with the
// job row.
//
// IMPORTANT: this method does NOT delete the users row — that's
// HardDeleteUser's job (called separately so the worker can keep the
// users row alive long enough to send the deletion_completed email
// referencing it). The order in the worker is:
//   1. send deletion_completed email
//   2. HardDeleteUser
//   3. MarkDeletionCompleted (the users row is gone; we only touch the
//      job row in this case — see `markDeletionCompletedJobOnly`)
//
// We expose `MarkDeletionCompleted` for the optional pre-hard-delete
// path; the more common path is `MarkDeletionCompletedJobOnly`.
func (q *PostgresQuery) MarkDeletionCompleted(ctx context.Context, jobID, userID string) error {
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		UPDATE account_deletion_jobs
		SET status = 'completed', completed_at = NOW()
		WHERE id = $1`, jobID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE users SET deletion_completed_at = NOW(), updated_at = NOW()
		WHERE id = $1`, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// MarkDeletionCompletedJobOnly closes out the job row after the user
// has already been hard-deleted. Equivalent to MarkDeletionCompleted
// minus the users-table touch.
func (q *PostgresQuery) MarkDeletionCompletedJobOnly(ctx context.Context, jobID string) error {
	_, err := q.pool.Exec(ctx, `
		UPDATE account_deletion_jobs
		SET status = 'completed', completed_at = NOW()
		WHERE id = $1`, jobID)
	return err
}

func (q *PostgresQuery) findActiveDeletionJob(ctx context.Context, userID string) (*AccountDeletionJob, error) {
	row := q.pool.QueryRow(ctx, `
		SELECT `+accountDeletionJobColumns+`
		FROM account_deletion_jobs
		WHERE user_id = $1 AND status IN ('pending','exporting','deleting')
		ORDER BY created_at DESC LIMIT 1`, userID)
	job, err := scanAccountDeletionJob(row)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return job, err
}

const accountDeletionJobColumns = `id, user_id, status, total_channels,
	channels_processed, gdpr_export_generated, last_error,
	created_at, started_at, completed_at`

func scanAccountDeletionJob(row pgx.Row) (*AccountDeletionJob, error) {
	j := &AccountDeletionJob{}
	err := row.Scan(&j.ID, &j.UserID, &j.Status, &j.TotalChannels,
		&j.ChannelsProcessed, &j.GDPRExportGenerated, &j.LastError,
		&j.CreatedAt, &j.StartedAt, &j.CompletedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return j, nil
}

// ---------------------------------------------------------------------
// Tracking introspection + chunked delete (Section 6.6 - 6.10)
// ---------------------------------------------------------------------

// GetUserTrackingChannels returns one row per channel the user tracks,
// joined with the platform_channels metadata the deletion worker needs.
// Rows are stable-ordered by tracking.created_at so progress is
// deterministic across worker restarts.
func (q *PostgresQuery) GetUserTrackingChannels(ctx context.Context, userID string) ([]*UserChannelTrackingRow, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT t.id, t.user_id, t.platform_channel_id,
		       p.platform, p.platform_channel_id, p.display_name,
		       t.access_token_enc, t.token_expires_at
		FROM user_channel_tracking t
		JOIN platform_channels p ON p.id = t.platform_channel_id
		WHERE t.user_id = $1
		ORDER BY t.created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*UserChannelTrackingRow
	for rows.Next() {
		r := &UserChannelTrackingRow{}
		if err := rows.Scan(&r.TrackingID, &r.UserID, &r.PlatformChannelID,
			&r.Platform, &r.PlatformChannelKey, &r.DisplayName,
			&r.AccessTokenEnc, &r.TokenExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetActiveTrackerCountForUpdate returns the count of OTHER users still
// tracking the given platform channel, holding a SELECT ... FOR UPDATE
// lock on the platform_channels row. The lock prevents a concurrent
// CreateUserChannelTracking from sneaking a new tracker in between our
// count and our chunk delete.
//
// `tx` MUST be a live pgx.Tx; calling this outside a transaction
// defeats the FOR UPDATE.
func (q *PostgresQuery) GetActiveTrackerCountForUpdate(ctx context.Context, tx pgx.Tx, platformChannelID, excludeUserID string) (int, error) {
	// First: lock the platform_channels row.
	if _, err := tx.Exec(ctx,
		`SELECT 1 FROM platform_channels WHERE id = $1 FOR UPDATE`,
		platformChannelID); err != nil {
		return 0, err
	}
	var n int
	err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM user_channel_tracking
		WHERE platform_channel_id = $1 AND user_id <> $2`,
		platformChannelID, excludeUserID).Scan(&n)
	return n, err
}

// DeleteUserChannelTracking removes the tracker join row inside a
// caller-supplied transaction. Promotion of the next ingestion-token
// owner (when the deleted user was primary) lives in
// PromoteNextIngestionToken (tracking.go) and is called by the worker
// after a successful delete.
func (q *PostgresQuery) DeleteUserChannelTracking(ctx context.Context, tx pgx.Tx, userID, platformChannelID string) error {
	_, err := tx.Exec(ctx,
		`DELETE FROM user_channel_tracking WHERE user_id = $1 AND platform_channel_id = $2`,
		userID, platformChannelID)
	return err
}

// DeleteCommentsByChannelChunked deletes all comments for a channel in
// short chunked transactions of `chunkSize` rows each. Between chunks,
// the function re-acquires a SELECT ... FOR UPDATE lock on the
// platform_channels row and re-reads the active tracker count: if any
// other user has started tracking, the function returns
// ErrNewTrackerAppeared (along with the count of rows deleted so far)
// and the caller MUST NOT proceed to delete the platform_channels row.
//
// Chunked deletes serve two ops goals:
//   - bound the lock duration on the comments table (no 6-hour DELETE)
//   - allow the deletion to abort cleanly if a new tracker appears
//
// Returns the total number of comment rows deleted.
func (q *PostgresQuery) DeleteCommentsByChannelChunked(
	ctx context.Context,
	platformChannelID string,
	chunkSize int,
	excludeUserID string,
) (int, error) {
	if chunkSize <= 0 {
		chunkSize = 10000
	}
	total := 0
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		tx, err := q.pool.Begin(ctx)
		if err != nil {
			return total, err
		}
		// Re-check tracker count under lock. If someone else started
		// tracking since we began, abort cleanly.
		count, err := q.GetActiveTrackerCountForUpdate(ctx, tx, platformChannelID, excludeUserID)
		if err != nil {
			_ = tx.Rollback(ctx)
			return total, err
		}
		if count > 0 {
			_ = tx.Rollback(ctx)
			return total, ErrNewTrackerAppeared
		}
		// Delete a chunk. Postgres doesn't support LIMIT directly on
		// DELETE; we use a CTE selecting the next chunk by primary key.
		tag, err := tx.Exec(ctx, `
			DELETE FROM comments
			WHERE id IN (
				SELECT id FROM comments
				WHERE platform_channel_id = $1
				LIMIT $2
			)`, platformChannelID, chunkSize)
		if err != nil {
			_ = tx.Rollback(ctx)
			return total, err
		}
		n := int(tag.RowsAffected())
		if err := tx.Commit(ctx); err != nil {
			return total, err
		}
		total += n
		if n < chunkSize {
			// drained
			return total, nil
		}
	}
}

// DeletePlatformChannel removes the platform_channels row. The caller
// MUST have already verified (under the same tx) that no trackers
// remain — otherwise the FK ON DELETE CASCADE will silently destroy a
// live user's data.
//
// Defensive guard: we re-check the count one last time INSIDE the same
// transaction and refuse if non-zero. Belt + suspenders.
func (q *PostgresQuery) DeletePlatformChannel(ctx context.Context, tx pgx.Tx, platformChannelID string) error {
	var n int
	if err := tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_channel_tracking WHERE platform_channel_id = $1`,
		platformChannelID).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("DeletePlatformChannel: refusing to delete %s with %d active trackers", platformChannelID, n)
	}
	_, err := tx.Exec(ctx, `DELETE FROM platform_channels WHERE id = $1`, platformChannelID)
	return err
}

// DeleteUserStarsForChannel removes the per-user star rows tied to
// comments on a specific channel. Used during the per-channel cleanup
// loop so stars vanish in lockstep with the comments they reference.
func (q *PostgresQuery) DeleteUserStarsForChannel(ctx context.Context, tx pgx.Tx, userID, platformChannelID string) error {
	_, err := tx.Exec(ctx, `
		DELETE FROM user_comment_stars
		WHERE user_id = $1
		  AND comment_id IN (SELECT id FROM comments WHERE platform_channel_id = $2)`,
		userID, platformChannelID)
	return err
}

// DeleteUserStarsAll wipes the user's stars regardless of channel —
// used in the final stage when we drop everything still hanging off the
// user_id.
func (q *PostgresQuery) DeleteUserStarsAll(ctx context.Context, tx pgx.Tx, userID string) error {
	_, err := tx.Exec(ctx, `DELETE FROM user_comment_stars WHERE user_id = $1`, userID)
	return err
}

// DeleteUserDigests drops the user's weekly_digests rows. Cheap; no
// chunking needed (one row per week).
func (q *PostgresQuery) DeleteUserDigests(ctx context.Context, tx pgx.Tx, userID string) error {
	_, err := tx.Exec(ctx, `DELETE FROM weekly_digests WHERE user_id = $1`, userID)
	return err
}

// DeleteUserAuthTokens clears the three auth-related tables. We delete
// each separately rather than using a single CTE for clarity in the
// query log; volumes are tiny.
func (q *PostgresQuery) DeleteUserAuthTokens(ctx context.Context, tx pgx.Tx, userID string) error {
	for _, sql := range []string{
		`DELETE FROM email_verification_tokens WHERE user_id = $1`,
		`DELETE FROM password_reset_tokens WHERE user_id = $1`,
		`DELETE FROM refresh_tokens WHERE user_id = $1`,
	} {
		if _, err := tx.Exec(ctx, sql, userID); err != nil {
			return err
		}
	}
	return nil
}

// HardDeleteUser removes the users row. Should be called LAST, after
// every dependent table has been cleared (otherwise FK constraints will
// reject). The caller is the deletion worker's final stage; it runs
// after the deletion_completed email has been queued.
func (q *PostgresQuery) HardDeleteUser(ctx context.Context, tx pgx.Tx, userID string) error {
	_, err := tx.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	return err
}

// ---------------------------------------------------------------------
// Data export jobs (Section 6.15 - 6.17)
// ---------------------------------------------------------------------

// EnqueueDataExport inserts a queued export job. The partial unique
// index `idx_data_export_jobs_one_inflight_per_user` makes a second
// in-flight job impossible at the DB level; we catch the 23505 race and
// return the existing row instead so the API call is idempotent.
func (q *PostgresQuery) EnqueueDataExport(ctx context.Context, userID string) (*DataExportJob, error) {
	row := q.pool.QueryRow(ctx, `
		INSERT INTO data_export_jobs (user_id, status)
		VALUES ($1, 'queued')
		RETURNING `+dataExportJobColumns, userID)
	job, err := scanDataExportJob(row)
	if err == nil {
		return job, nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		// In-flight job already exists; return it.
		return q.getInflightExport(ctx, userID)
	}
	return nil, err
}

func (q *PostgresQuery) getInflightExport(ctx context.Context, userID string) (*DataExportJob, error) {
	row := q.pool.QueryRow(ctx, `
		SELECT `+dataExportJobColumns+`
		FROM data_export_jobs
		WHERE user_id = $1 AND status IN ('queued','exporting')
		ORDER BY created_at DESC LIMIT 1`, userID)
	return scanDataExportJob(row)
}

// GetDataExportJob fetches a job by id. The handler uses it to render
// status responses and to verify ownership before serving downloads.
func (q *PostgresQuery) GetDataExportJob(ctx context.Context, jobID string) (*DataExportJob, error) {
	row := q.pool.QueryRow(ctx, `
		SELECT `+dataExportJobColumns+` FROM data_export_jobs WHERE id = $1`, jobID)
	return scanDataExportJob(row)
}

// ClaimNextExportJob atomically transitions the oldest queued job into
// 'exporting' and returns it. Returns (nil, nil) on idle.
func (q *PostgresQuery) ClaimNextExportJob(ctx context.Context) (*DataExportJob, error) {
	row := q.pool.QueryRow(ctx, `
		UPDATE data_export_jobs
		SET status = 'exporting', started_at = NOW()
		WHERE id = (
			SELECT id FROM data_export_jobs
			WHERE status = 'queued'
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		RETURNING `+dataExportJobColumns)
	job, err := scanDataExportJob(row)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return job, err
}

// MarkExportStarted is a separate hook from ClaimNextExportJob in case
// the worker wants to mark the started_at later (currently they happen
// together in ClaimNextExportJob; MarkExportStarted is exposed for
// completeness / future-proofing).
func (q *PostgresQuery) MarkExportStarted(ctx context.Context, jobID string) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE data_export_jobs SET status='exporting', started_at=NOW() WHERE id=$1`, jobID)
	return err
}

// MarkDataExportCompleted records terminal success: storage path, signed
// URL, expiry. We mirror the URL + expiry onto the users row so the
// `GET /api/user/me` profile endpoint can surface them without a join.
// Both writes happen in one transaction.
//
// Naming: the comment-exports feature already owns MarkExportCompleted;
// the data-lifecycle export is a distinct workflow (full-account JSON
// bundle, not a comment CSV) so we use a Data-prefixed name.
func (q *PostgresQuery) MarkDataExportCompleted(ctx context.Context, jobID, userID, storagePath, downloadURL string, expiresAt time.Time, partial bool) error {
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		UPDATE data_export_jobs
		SET status='completed', storage_path=$2, download_url=$3,
		    expires_at=$4, partial_export=$5, completed_at=NOW()
		WHERE id=$1`, jobID, storagePath, downloadURL, expiresAt, partial); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET data_export_url=$2, data_export_expires_at=$3, updated_at=NOW()
		WHERE id=$1`, userID, downloadURL, expiresAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// MarkDataExportFailed records an unrecoverable error. The job stays
// in 'failed' for human review; the cleanup cron will not touch its
// (non-existent) storage file.
func (q *PostgresQuery) MarkDataExportFailed(ctx context.Context, jobID, errorMsg string) error {
	_, err := q.pool.Exec(ctx, `
		UPDATE data_export_jobs SET status='failed', last_error=$2, completed_at=NOW()
		WHERE id=$1`, jobID, errorMsg)
	return err
}

// ListExpiredDataExports returns export jobs whose expires_at is in
// the past AND whose storage_path is non-NULL (i.e. the file is still
// on disk waiting to be cleaned up). Used by the daily cleanup cron
// (Section 10).
func (q *PostgresQuery) ListExpiredDataExports(ctx context.Context, now time.Time) ([]*DataExportJob, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT `+dataExportJobColumns+`
		FROM data_export_jobs
		WHERE expires_at IS NOT NULL AND expires_at < $1 AND storage_path IS NOT NULL`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*DataExportJob
	for rows.Next() {
		j, err := scanDataExportJobRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// ClearExportStoragePath nulls out the storage_path/download_url after
// the file is gone. Keeps the job row around as historical record but
// stops the cleanup cron from re-considering it.
func (q *PostgresQuery) ClearExportStoragePath(ctx context.Context, jobID string) error {
	_, err := q.pool.Exec(ctx, `
		UPDATE data_export_jobs
		SET storage_path=NULL, download_url=NULL
		WHERE id=$1`, jobID)
	return err
}

const dataExportJobColumns = `id, user_id, status, storage_path,
	download_url, expires_at, partial_export, last_error,
	created_at, started_at, completed_at`

func scanDataExportJob(row pgx.Row) (*DataExportJob, error) {
	j := &DataExportJob{}
	err := row.Scan(&j.ID, &j.UserID, &j.Status, &j.StoragePath,
		&j.DownloadURL, &j.ExpiresAt, &j.PartialExport, &j.LastError,
		&j.CreatedAt, &j.StartedAt, &j.CompletedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return j, nil
}

func scanDataExportJobRows(rows pgx.Rows) (*DataExportJob, error) {
	j := &DataExportJob{}
	err := rows.Scan(&j.ID, &j.UserID, &j.Status, &j.StoragePath,
		&j.DownloadURL, &j.ExpiresAt, &j.PartialExport, &j.LastError,
		&j.CreatedAt, &j.StartedAt, &j.CompletedAt)
	return j, err
}

// ---------------------------------------------------------------------
// Export bundle assembly (Section 6.17)
// ---------------------------------------------------------------------

// GetUserExportBundle assembles the complete user-data snapshot the
// export worker writes to ZIP. Each section is a distinct query so a
// failure in one (e.g. comments) doesn't block the others — the worker
// can decide whether a partial bundle is acceptable.
//
// IMPORTANT: this aggregates potentially large result sets into Go
// memory. The export worker enforces EXPORT_MAX_BYTES *after*
// serialization; it relies on the bundle fitting in RAM during build.
// For users with millions of comments this is the choke point — we
// will need to switch to streaming if it becomes a real problem.
func (q *PostgresQuery) GetUserExportBundle(ctx context.Context, userID string) (*UserExportBundle, error) {
	user, err := q.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	bundle := &UserExportBundle{User: user}

	// Tracking
	trackingRows, err := q.pool.Query(ctx, `
		SELECT `+trackingColumns+`
		FROM user_channel_tracking WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	for trackingRows.Next() {
		t, err := scanTracking(trackingRows)
		if err != nil {
			trackingRows.Close()
			return nil, err
		}
		bundle.Tracking = append(bundle.Tracking, t)
	}
	trackingRows.Close()
	if err := trackingRows.Err(); err != nil {
		return nil, err
	}

	// Comments tied to the user's tracked channels.
	commentRows, err := q.pool.Query(ctx, `
		SELECT c.id, c.platform_channel_id, c.platform_comment_id,
		       c.parent_comment_id, c.author_handle, c.body, c.posted_at,
		       c.category, c.sentiment_score, c.topic, c.confidence,
		       c.processed_at, c.created_at
		FROM comments c
		WHERE c.platform_channel_id IN (
			SELECT platform_channel_id FROM user_channel_tracking WHERE user_id = $1
		)
		ORDER BY c.posted_at`, userID)
	if err != nil {
		return nil, err
	}
	for commentRows.Next() {
		c := &Comment{}
		if err := commentRows.Scan(&c.ID, &c.PlatformChannelID, &c.PlatformCommentID,
			&c.ParentCommentID, &c.AuthorHandle, &c.Body, &c.PostedAt,
			&c.Category, &c.SentimentScore, &c.Topic, &c.Confidence,
			&c.ProcessedAt, &c.CreatedAt); err != nil {
			commentRows.Close()
			return nil, err
		}
		bundle.Comments = append(bundle.Comments, c)
	}
	commentRows.Close()
	if err := commentRows.Err(); err != nil {
		return nil, err
	}

	// Digests
	digestRows, err := q.pool.Query(ctx, `
		SELECT id, user_id, week_start_date, status, content, emailed_at, created_at
		FROM weekly_digests WHERE user_id = $1 ORDER BY week_start_date`, userID)
	if err != nil {
		return nil, err
	}
	for digestRows.Next() {
		d := &WeeklyDigest{}
		if err := digestRows.Scan(&d.ID, &d.UserID, &d.WeekStartDate,
			&d.Status, &d.Content, &d.EmailedAt, &d.CreatedAt); err != nil {
			digestRows.Close()
			return nil, err
		}
		bundle.Digests = append(bundle.Digests, d)
	}
	digestRows.Close()
	if err := digestRows.Err(); err != nil {
		return nil, err
	}

	// Stars
	starRows, err := q.pool.Query(ctx, `
		SELECT user_id, comment_id, starred_at
		FROM user_comment_stars WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	for starRows.Next() {
		s := &UserCommentStar{}
		if err := starRows.Scan(&s.UserID, &s.CommentID, &s.StarredAt); err != nil {
			starRows.Close()
			return nil, err
		}
		bundle.Stars = append(bundle.Stars, s)
	}
	starRows.Close()
	return bundle, starRows.Err()
}
