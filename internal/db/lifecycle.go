package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrNewTrackerAppeared = errors.New("lifecycle: another tracker appeared during chunked delete; aborting channel-level cleanup")

type TxBeginner interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
	ExecContext(ctx context.Context, sql string, args ...any) (sql.Result, error)
}

type LifecycleStore interface {
	ScheduleAccountDeletion(ctx context.Context, userID string, totalChannels int) (*AccountDeletionJob, error)
	CancelAccountDeletion(ctx context.Context, userID string) error
	ClaimNextDeletionJob(ctx context.Context, gracePeriod time.Duration) (*AccountDeletionJob, error)
	MarkDeletionStage(ctx context.Context, jobID, status string) error
	MarkDeletionFailed(ctx context.Context, jobID, errorMsg string) error
	MarkDeletionRevokeStatus(ctx context.Context, jobID string, attempted bool, errorMsg string) error
	IncrementChannelsProcessed(ctx context.Context, jobID string) error
	MarkDeletionCompleted(ctx context.Context, jobID, userID string) error
	MarkDeletionCompletedJobOnly(ctx context.Context, jobID string) error
	GetUserTrackingChannels(ctx context.Context, userID string) ([]*UserChannelTrackingRow, error)
	GetActiveTrackerCountForUpdate(ctx context.Context, tx *sql.Tx, platformChannelID, excludeUserID string) (int, error)
	DeleteUserChannelTracking(ctx context.Context, tx *sql.Tx, userID, platformChannelID string) error
	DeleteCommentsByChannelChunked(ctx context.Context, platformChannelID string, chunkSize int, excludeUserID string) (int, error)
	DeletePlatformChannel(ctx context.Context, tx *sql.Tx, platformChannelID string) error
	DeleteUserStarsForChannel(ctx context.Context, tx *sql.Tx, userID, platformChannelID string) error
	DeleteUserStarsAll(ctx context.Context, tx *sql.Tx, userID string) error
	DeleteUserDigests(ctx context.Context, tx *sql.Tx, userID string) error
	DeleteUserAuthTokens(ctx context.Context, tx *sql.Tx, userID string) error
	DeleteUserGDPRData(ctx context.Context, tx *sql.Tx, userID string) error
	HardDeleteUser(ctx context.Context, tx *sql.Tx, userID string) error
	ListJobsPendingRevoke(ctx context.Context) ([]*AccountDeletionJob, error)
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

type AccountDeletionJob struct {
	ID                  string
	UserID              string
	Status              string
	TotalChannels       int
	ChannelsProcessed   int
	GDPRExportGenerated bool
	RevokeAttempted     bool
	RevokeError         *string
	ErrorMessage        *string
	CreatedAt           time.Time
	StartedAt           *time.Time
	CompletedAt         *time.Time
}

type DataExportJob struct {
	ID           string
	UserID       string
	Status       string
	ExportURL    *string
	ExpiresAt    *time.Time
	ErrorMessage *string
	CreatedAt    time.Time
	StartedAt    *time.Time
	CompletedAt  *time.Time
}

type UserChannelTrackingRow struct {
	TrackingID         string
	UserID             string
	PlatformChannelID  string
	Platform           string
	PlatformChannelKey string
	DisplayName        string
	AccessTokenEnc     string
	RefreshTokenEnc    string
	TokenExpiresAt     *time.Time
}

type UserExportBundle struct {
	User     *User
	Tracking []*UserChannelTracking
	Comments []*Comment
	Digests  []*WeeklyDigest
	Stars    []*UserCommentStar
}

func (q *SQLiteQuery) ScheduleAccountDeletion(ctx context.Context, userID string, totalChannels int) (*AccountDeletionJob, error) {
	existing, err := q.findActiveDeletionJob(ctx, userID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	row := q.db.QueryRowContext(ctx, `
		INSERT INTO account_deletion_jobs (id, user_id, status, total_channels)
		VALUES (?, ?, 'pending', ?)
		RETURNING `+accountDeletionJobColumns, generateUUID(), userID, totalChannels)
	return scanAccountDeletionJob(row)
}

func (q *SQLiteQuery) CancelAccountDeletion(ctx context.Context, userID string) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx,
		`DELETE FROM account_deletion_jobs WHERE user_id = ? AND status = 'pending'`,
		userID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE users SET deletion_requested_at = NULL, updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ?`,
		userID); err != nil {
		return err
	}
	return tx.Commit()
}

func (q *SQLiteQuery) ClaimNextDeletionJob(ctx context.Context, gracePeriod time.Duration) (*AccountDeletionJob, error) {
	cutoff := time.Now().UTC().Add(-gracePeriod).Format(time.RFC3339)
	row := q.db.QueryRowContext(ctx, `
		UPDATE account_deletion_jobs
		SET status = 'exporting', started_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		WHERE id = (
			SELECT id FROM account_deletion_jobs
			WHERE status = 'pending' AND created_at <= ?
			ORDER BY created_at
			LIMIT 1
		)
		RETURNING `+accountDeletionJobColumns, cutoff)
	job, err := scanAccountDeletionJob(row)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return job, err
}

func (q *SQLiteQuery) MarkDeletionStage(ctx context.Context, jobID, status string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE account_deletion_jobs SET status = ? WHERE id = ?`, status, jobID)
	return err
}

func (q *SQLiteQuery) MarkDeletionFailed(ctx context.Context, jobID, errorMsg string) error {
	_, err := q.db.ExecContext(ctx, `
		UPDATE account_deletion_jobs
		SET status = 'failed', error_message = ?, completed_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		WHERE id = ?`, errorMsg, jobID)
	return err
}

// MarkDeletionRevokeStatus records the outcome of the provider-token
// revocation attempt on the job. errorMsg "" clears any previous error
// (a later, successful out-of-band retry). The out-of-band sweeper
// picks up jobs with revoke_attempted=1 and a non-null revoke_error.
func (q *SQLiteQuery) MarkDeletionRevokeStatus(ctx context.Context, jobID string, attempted bool, errorMsg string) error {
	_, err := q.db.ExecContext(ctx, `
		UPDATE account_deletion_jobs
		SET revoke_attempted = ?,
		    revoke_error = CASE WHEN ? = '' THEN NULL ELSE ? END
		WHERE id = ?`,
		attempted, errorMsg, errorMsg, jobID)
	return err
}

func (q *SQLiteQuery) IncrementChannelsProcessed(ctx context.Context, jobID string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE account_deletion_jobs SET channels_processed = channels_processed + 1 WHERE id = ?`,
		jobID)
	return err
}

func (q *SQLiteQuery) MarkDeletionCompleted(ctx context.Context, jobID, userID string) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		UPDATE account_deletion_jobs
		SET status = 'completed', completed_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		WHERE id = ?`, jobID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE users SET deletion_completed_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		WHERE id = ?`, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func (q *SQLiteQuery) MarkDeletionCompletedJobOnly(ctx context.Context, jobID string) error {
	_, err := q.db.ExecContext(ctx, `
		UPDATE account_deletion_jobs
		SET status = 'completed', completed_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		WHERE id = ?`, jobID)
	return err
}

func (q *SQLiteQuery) findActiveDeletionJob(ctx context.Context, userID string) (*AccountDeletionJob, error) {
	row := q.db.QueryRowContext(ctx, `
		SELECT `+accountDeletionJobColumns+`
		FROM account_deletion_jobs
		WHERE user_id = ? AND status IN ('pending','exporting','deleting')
		ORDER BY created_at DESC LIMIT 1`, userID)
	job, err := scanAccountDeletionJob(row)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return job, err
}

// ListJobsPendingRevoke returns jobs still in the 'deleting' stage whose
// provider-token revocation failed, so the out-of-band sweeper can retry
// it while the user's credentials still exist in the DB.
func (q *SQLiteQuery) ListJobsPendingRevoke(ctx context.Context) ([]*AccountDeletionJob, error) {
	rows, err := q.db.QueryContext(ctx, `
		SELECT `+accountDeletionJobColumns+`
		FROM account_deletion_jobs
		WHERE status = 'deleting'
		  AND revoke_attempted = 1
		  AND revoke_error IS NOT NULL
		ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*AccountDeletionJob
	for rows.Next() {
		j := &AccountDeletionJob{}
		var createdAt string
		var revokeError, errorMessage, startedAt, completedAt sql.NullString
		if err := rows.Scan(&j.ID, &j.UserID, &j.Status, &j.TotalChannels,
			&j.ChannelsProcessed, &j.GDPRExportGenerated, &j.RevokeAttempted, &revokeError,
			&errorMessage, &createdAt, &startedAt, &completedAt); err != nil {
			return nil, err
		}
		if revokeError.Valid {
			j.RevokeError = &revokeError.String
		}
		if errorMessage.Valid {
			j.ErrorMessage = &errorMessage.String
		}
		j.CreatedAt = parseTime(createdAt)
		j.StartedAt = scanTimePtr(startedAt)
		j.CompletedAt = scanTimePtr(completedAt)
		out = append(out, j)
	}
	return out, rows.Err()
}

const accountDeletionJobColumns = `id, user_id, status, total_channels,
	channels_processed, gdpr_export_generated, revoke_attempted, revoke_error,
	error_message, created_at, started_at, completed_at`

func scanAccountDeletionJob(row *sql.Row) (*AccountDeletionJob, error) {
	j := &AccountDeletionJob{}
	var createdAt string
	var revokeError, errorMessage, startedAt, completedAt sql.NullString
	err := row.Scan(&j.ID, &j.UserID, &j.Status, &j.TotalChannels,
		&j.ChannelsProcessed, &j.GDPRExportGenerated, &j.RevokeAttempted, &revokeError,
		&errorMessage, &createdAt, &startedAt, &completedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if revokeError.Valid {
		j.RevokeError = &revokeError.String
	}
	if errorMessage.Valid {
		j.ErrorMessage = &errorMessage.String
	}
	j.CreatedAt = parseTime(createdAt)
	j.StartedAt = scanTimePtr(startedAt)
	j.CompletedAt = scanTimePtr(completedAt)
	return j, nil
}

func (q *SQLiteQuery) GetUserTrackingChannels(ctx context.Context, userID string) ([]*UserChannelTrackingRow, error) {
	rows, err := q.db.QueryContext(ctx, `
		SELECT t.id, t.user_id, t.platform_channel_id,
		       p.platform, p.platform_channel_id, p.display_name,
		       t.access_token_enc, t.refresh_token_enc, t.token_expires_at
		FROM user_channel_tracking t
		JOIN platform_channels p ON p.id = t.platform_channel_id
		WHERE t.user_id = ?
		ORDER BY t.created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*UserChannelTrackingRow
	for rows.Next() {
		r := &UserChannelTrackingRow{}
		var tokenExpiresAt sql.NullString
		if err := rows.Scan(&r.TrackingID, &r.UserID, &r.PlatformChannelID,
			&r.Platform, &r.PlatformChannelKey, &r.DisplayName,
			&r.AccessTokenEnc, &r.RefreshTokenEnc, &tokenExpiresAt); err != nil {
			return nil, err
		}
		r.TokenExpiresAt = parseTimePtr(tokenExpiresAt)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (q *SQLiteQuery) GetActiveTrackerCountForUpdate(ctx context.Context, tx *sql.Tx, platformChannelID, excludeUserID string) (int, error) {
	var n int
	err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM user_channel_tracking
		WHERE platform_channel_id = ? AND user_id <> ?`,
		platformChannelID, excludeUserID).Scan(&n)
	return n, err
}

func (q *SQLiteQuery) DeleteUserChannelTracking(ctx context.Context, tx *sql.Tx, userID, platformChannelID string) error {
	_, err := tx.ExecContext(ctx,
		`DELETE FROM user_channel_tracking WHERE user_id = ? AND platform_channel_id = ?`,
		userID, platformChannelID)
	return err
}

func (q *SQLiteQuery) DeleteCommentsByChannelChunked(
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
		tx, err := q.db.BeginTx(ctx, nil)
		if err != nil {
			return total, err
		}
		count, err := q.GetActiveTrackerCountForUpdate(ctx, tx, platformChannelID, excludeUserID)
		if err != nil {
			_ = tx.Rollback()
			return total, err
		}
		if count > 0 {
			_ = tx.Rollback()
			return total, ErrNewTrackerAppeared
		}
		result, err := tx.ExecContext(ctx, `
			DELETE FROM comments
			WHERE id IN (
				SELECT id FROM comments
				WHERE platform_channel_id = ?
				LIMIT ?
			)`, platformChannelID, chunkSize)
		if err != nil {
			_ = tx.Rollback()
			return total, err
		}
		n, _ := result.RowsAffected()
		if err := tx.Commit(); err != nil {
			return total, err
		}
		total += int(n)
		if int(n) < chunkSize {
			return total, nil
		}
	}
}

func (q *SQLiteQuery) DeletePlatformChannel(ctx context.Context, tx *sql.Tx, platformChannelID string) error {
	var n int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_channel_tracking WHERE platform_channel_id = ?`,
		platformChannelID).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("DeletePlatformChannel: refusing to delete %s with %d active trackers", platformChannelID, n)
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM platform_channels WHERE id = ?`, platformChannelID)
	return err
}

func (q *SQLiteQuery) DeleteUserStarsForChannel(ctx context.Context, tx *sql.Tx, userID, platformChannelID string) error {
	_, err := tx.ExecContext(ctx, `
		DELETE FROM user_comment_stars
		WHERE user_id = ?
		  AND comment_id IN (SELECT id FROM comments WHERE platform_channel_id = ?)`,
		userID, platformChannelID)
	return err
}

func (q *SQLiteQuery) DeleteUserStarsAll(ctx context.Context, tx *sql.Tx, userID string) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM user_comment_stars WHERE user_id = ?`, userID)
	return err
}

func (q *SQLiteQuery) DeleteUserDigests(ctx context.Context, tx *sql.Tx, userID string) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM weekly_digests WHERE user_id = ?`, userID)
	return err
}

func (q *SQLiteQuery) DeleteUserAuthTokens(ctx context.Context, tx *sql.Tx, userID string) error {
	for _, sqlStr := range []string{
		`DELETE FROM email_verification_tokens WHERE user_id = ?`,
		`DELETE FROM password_reset_tokens WHERE user_id = ?`,
		`DELETE FROM refresh_tokens WHERE user_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, sqlStr, userID); err != nil {
			return err
		}
	}
	return nil
}

// DeleteUserGDPRData removes every row owned by the user across all
// user-owned tables. It is written to be correct whether or not foreign
// keys are enforced: it performs the DELETEs explicitly instead of
// relying on cascades. Child tables are wiped in dependency order before
// the caller removes the user row.
//
// Per-channel data (platform_channels, comments, ingestion_jobs) is NOT
// deleted here — it is handled by the detach-only per-channel stage so a
// shared channel's data survives for its other trackers.
//
// llm_processing_jobs has no user_id column and is intentionally not
// covered. workspaces/workspace_invites are shared/multi-owner and are
// handled by workspace deletion, not account deletion.
func (q *SQLiteQuery) DeleteUserGDPRData(ctx context.Context, tx *sql.Tx, userID string) error {
	// rate_limit_buckets keys on "u:"+userID (see api/rate_limit.go).
	stmts := []struct {
		sql string
		arg any
	}{
		{`DELETE FROM email_verification_tokens WHERE user_id = ?`, userID},
		{`DELETE FROM password_reset_tokens WHERE user_id = ?`, userID},
		{`DELETE FROM refresh_tokens WHERE user_id = ?`, userID},
		{`DELETE FROM oauth_states WHERE user_id = ?`, userID},
		{`DELETE FROM user_comment_stars WHERE user_id = ?`, userID},
		{`DELETE FROM comment_usage_logs WHERE user_id = ?`, userID},
		{`DELETE FROM comment_exports WHERE user_id = ?`, userID},
		{`DELETE FROM workspace_members WHERE user_id = ?`, userID},
		{`DELETE FROM rate_limit_buckets WHERE user_key = ?`, "u:" + userID},
		{`DELETE FROM user_channel_tracking WHERE user_id = ?`, userID},
		{`DELETE FROM weekly_digests WHERE user_id = ?`, userID},
		{`DELETE FROM data_export_jobs WHERE user_id = ?`, userID},
	}
	for _, s := range stmts {
		if _, err := tx.ExecContext(ctx, s.sql, s.arg); err != nil {
			return err
		}
	}
	return nil
}

func (q *SQLiteQuery) HardDeleteUser(ctx context.Context, tx *sql.Tx, userID string) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, userID)
	return err
}

func (q *SQLiteQuery) EnqueueDataExport(ctx context.Context, userID string) (*DataExportJob, error) {
	row := q.db.QueryRowContext(ctx, `
		INSERT INTO data_export_jobs (id, user_id, status)
		VALUES (?, ?, 'queued')
		RETURNING `+dataExportJobColumns, generateUUID(), userID)
	job, err := scanDataExportJob(row)
	if err == nil {
		return job, nil
	}
	// Check for unique constraint violation (in-flight job exists)
	if err != nil {
		return q.getInflightExport(ctx, userID)
	}
	return nil, err
}

func (q *SQLiteQuery) getInflightExport(ctx context.Context, userID string) (*DataExportJob, error) {
	row := q.db.QueryRowContext(ctx, `
		SELECT `+dataExportJobColumns+`
		FROM data_export_jobs
		WHERE user_id = ? AND status IN ('queued','exporting')
		ORDER BY created_at DESC LIMIT 1`, userID)
	return scanDataExportJob(row)
}

func (q *SQLiteQuery) GetDataExportJob(ctx context.Context, jobID string) (*DataExportJob, error) {
	row := q.db.QueryRowContext(ctx, `
		SELECT `+dataExportJobColumns+` FROM data_export_jobs WHERE id = ?`, jobID)
	return scanDataExportJob(row)
}

func (q *SQLiteQuery) ClaimNextExportJob(ctx context.Context) (*DataExportJob, error) {
	row := q.db.QueryRowContext(ctx, `
		UPDATE data_export_jobs
		SET status = 'exporting', started_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		WHERE id = (
			SELECT id FROM data_export_jobs
			WHERE status = 'queued'
			ORDER BY created_at
			LIMIT 1
		)
		RETURNING `+dataExportJobColumns)
	job, err := scanDataExportJob(row)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return job, err
}

func (q *SQLiteQuery) MarkExportStarted(ctx context.Context, jobID string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE data_export_jobs SET status='exporting', started_at=strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id=?`, jobID)
	return err
}

// MarkDataExportCompleted flips the job to 'completed' and persists the
// signed download URL in export_url (mirrored onto users.data_export_url).
// The local ZIP path is not stored: it derives deterministically from the
// storage dir + user id + job id. storagePath and partial are accepted for
// interface stability but no longer persisted — the schema has no columns
// for them.
func (q *SQLiteQuery) MarkDataExportCompleted(ctx context.Context, jobID, userID, storagePath, downloadURL string, expiresAt time.Time, partial bool) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		UPDATE data_export_jobs
		SET status='completed', export_url=?,
		    expires_at=?, completed_at=strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		WHERE id=?`, downloadURL, expiresAt.Format(time.RFC3339), jobID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE users
		SET data_export_url=?, data_export_expires_at=?, updated_at=strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		WHERE id=?`, downloadURL, expiresAt.Format(time.RFC3339), userID); err != nil {
		return err
	}
	return tx.Commit()
}

func (q *SQLiteQuery) MarkDataExportFailed(ctx context.Context, jobID, errorMsg string) error {
	_, err := q.db.ExecContext(ctx, `
		UPDATE data_export_jobs SET status='failed', error_message=?, completed_at=strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		WHERE id=?`, errorMsg, jobID)
	return err
}

// RequeueDataExport returns a job to 'queued' after a failed completion
// mark. Only 'exporting' rows transition so a reaper reset or a
// concurrent claim cannot be stomped. error_message is cleared: the
// retried attempt is a fresh one and the failure was already logged.
func (q *SQLiteQuery) RequeueDataExport(ctx context.Context, jobID string) error {
	_, err := q.db.ExecContext(ctx, `
		UPDATE data_export_jobs
		SET status='queued', started_at=NULL, error_message=NULL, completed_at=NULL
		WHERE id=? AND status='exporting'`, jobID)
	return err
}

// ResetStuckDataExports reaps crash leftovers: rows still 'exporting'
// past olderThan are retryable again. Mirrors the comment-exports reset
// (ResetStuckProcessingExports); data_export_jobs has no retry budget,
// so every stuck row is simply requeued. StartedAt NULL keeps the row
// out of future sweeps until it is claimed again.
func (q *SQLiteQuery) ResetStuckDataExports(ctx context.Context, olderThan time.Duration) (int, error) {
	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339)
	result, err := q.db.ExecContext(ctx, `
		UPDATE data_export_jobs
		SET status='queued', started_at=NULL, error_message=NULL
		WHERE status='exporting'
		  AND started_at IS NOT NULL
		  AND started_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// ListExportJobsForSweep lists rows whose ZIP file must not survive on
// disk: 'exporting' (crashed mid-write, partial file) and 'failed'
// (legacy orphans from before the cleanup path existed). Completed and
// queued rows keep their files — completed ones are downloadable, queued
// ones never wrote a ZIP.
func (q *SQLiteQuery) ListExportJobsForSweep(ctx context.Context) ([]*DataExportJob, error) {
	rows, err := q.db.QueryContext(ctx, `
		SELECT `+dataExportJobColumns+`
		FROM data_export_jobs
		WHERE status IN ('exporting','failed')
		ORDER BY created_at`)
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

func (q *SQLiteQuery) ListExpiredDataExports(ctx context.Context, now time.Time) ([]*DataExportJob, error) {
	rows, err := q.db.QueryContext(ctx, `
		SELECT `+dataExportJobColumns+`
		FROM data_export_jobs
		WHERE expires_at IS NOT NULL AND expires_at < ? AND export_url IS NOT NULL`, now.Format(time.RFC3339))
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

func (q *SQLiteQuery) ClearExportStoragePath(ctx context.Context, jobID string) error {
	_, err := q.db.ExecContext(ctx, `
		UPDATE data_export_jobs
		SET export_url=NULL
		WHERE id=?`, jobID)
	return err
}

const dataExportJobColumns = `id, user_id, status, export_url,
	expires_at, error_message,
	created_at, started_at, completed_at`

func scanDataExportJob(row *sql.Row) (*DataExportJob, error) {
	j := &DataExportJob{}
	var createdAt string
	var exportURL, expiresAt, errorMessage, startedAt, completedAt sql.NullString
	err := row.Scan(&j.ID, &j.UserID, &j.Status, &exportURL,
		&expiresAt, &errorMessage,
		&createdAt, &startedAt, &completedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if exportURL.Valid {
		j.ExportURL = &exportURL.String
	}
	if expiresAt.Valid {
		t, _ := time.Parse(time.RFC3339, expiresAt.String)
		j.ExpiresAt = &t
	}
	if errorMessage.Valid {
		j.ErrorMessage = &errorMessage.String
	}
	j.CreatedAt = parseTime(createdAt)
	j.StartedAt = scanTimePtr(startedAt)
	j.CompletedAt = scanTimePtr(completedAt)
	return j, nil
}

func scanDataExportJobRows(rows *sql.Rows) (*DataExportJob, error) {
	j := &DataExportJob{}
	var createdAt string
	var exportURL, expiresAt, errorMessage, startedAt, completedAt sql.NullString
	err := rows.Scan(&j.ID, &j.UserID, &j.Status, &exportURL,
		&expiresAt, &errorMessage,
		&createdAt, &startedAt, &completedAt)
	if err != nil {
		return nil, err
	}
	if exportURL.Valid {
		j.ExportURL = &exportURL.String
	}
	if expiresAt.Valid {
		t, _ := time.Parse(time.RFC3339, expiresAt.String)
		j.ExpiresAt = &t
	}
	if errorMessage.Valid {
		j.ErrorMessage = &errorMessage.String
	}
	j.CreatedAt = parseTime(createdAt)
	j.StartedAt = scanTimePtr(startedAt)
	j.CompletedAt = scanTimePtr(completedAt)
	return j, nil
}

func (q *SQLiteQuery) GetUserExportBundle(ctx context.Context, userID string) (*UserExportBundle, error) {
	user, err := q.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	bundle := &UserExportBundle{User: user}

	// The bundle is the long read of the GDPR export flow; run it on the
	// query_only export handle so it never occupies the main pool's
	// single connection (fix-export-pipeline 2.x).
	rd := q.ExportReadDB()

	trackingRows, err := rd.QueryContext(ctx, `
		SELECT `+trackingColumns+`
		FROM user_channel_tracking WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	for trackingRows.Next() {
		t, err := scanTrackingRows(trackingRows)
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

	commentRows, err := rd.QueryContext(ctx, `
		SELECT c.id, c.platform_channel_id, c.platform_comment_id,
		       c.parent_comment_id, c.reply_depth, c.author_handle, c.body, c.posted_at,
		       c.category, c.actionable_insights, c.topic, c.confidence,
		       c.processed_at, c.created_at
		FROM comments c
		WHERE c.platform_channel_id IN (
			SELECT platform_channel_id FROM user_channel_tracking WHERE user_id = ?
		)
		ORDER BY c.posted_at`, userID)
	if err != nil {
		return nil, err
	}
	for commentRows.Next() {
		c, err := scanCommentRow(commentRows)
		if err != nil {
			commentRows.Close()
			return nil, err
		}
		bundle.Comments = append(bundle.Comments, c)
	}
	commentRows.Close()
	if err := commentRows.Err(); err != nil {
		return nil, err
	}

	digestRows, err := rd.QueryContext(ctx, `
		SELECT id, user_id, week_start_date, status, content, emailed_at, created_at
		FROM weekly_digests WHERE user_id = ? ORDER BY week_start_date`, userID)
	if err != nil {
		return nil, err
	}
	for digestRows.Next() {
		d := &WeeklyDigest{}
		var emailedAtStr, createdAtStr string
		if err := digestRows.Scan(&d.ID, &d.UserID, &d.WeekStartDate,
			&d.Status, &d.Content, &emailedAtStr, &createdAtStr); err != nil {
			digestRows.Close()
			return nil, err
		}
		if emailedAtStr != "" {
			t, _ := time.Parse(time.RFC3339, emailedAtStr)
			d.EmailedAt = &t
		}
		d.CreatedAt = parseTime(createdAtStr)
		bundle.Digests = append(bundle.Digests, d)
	}
	digestRows.Close()
	if err := digestRows.Err(); err != nil {
		return nil, err
	}

	starRows, err := rd.QueryContext(ctx, `
		SELECT user_id, comment_id, starred_at
		FROM user_comment_stars WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	for starRows.Next() {
		s := &UserCommentStar{}
		var starredAt string
		if err := starRows.Scan(&s.UserID, &s.CommentID, &starredAt); err != nil {
			starRows.Close()
			return nil, err
		}
		s.StarredAt = parseTime(starredAt)
		bundle.Stars = append(bundle.Stars, s)
	}
	starRows.Close()
	return bundle, starRows.Err()
}
