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
	HardDeleteUser(ctx context.Context, tx *sql.Tx, userID string) error
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
	ID                   string
	UserID               string
	Status               string
	TotalChannels        int
	ChannelsProcessed    int
	GDPRExportGenerated  bool
	LastError            *string
	CreatedAt            time.Time
	StartedAt            *time.Time
	CompletedAt          *time.Time
}

type DataExportJob struct {
	ID            string
	UserID        string
	Status        string
	StoragePath   *string
	DownloadURL   *string
	ExpiresAt     *time.Time
	PartialExport bool
	LastError     *string
	CreatedAt     time.Time
	StartedAt     *time.Time
	CompletedAt   *time.Time
}

type UserChannelTrackingRow struct {
	TrackingID         string
	UserID             string
	PlatformChannelID  string
	Platform           string
	PlatformChannelKey string
	DisplayName        string
	AccessTokenEnc     string
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
		SET status = 'failed', last_error = ?, completed_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		WHERE id = ?`, errorMsg, jobID)
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

const accountDeletionJobColumns = `id, user_id, status, total_channels,
	channels_processed, gdpr_export_generated, last_error,
	created_at, started_at, completed_at`

func scanAccountDeletionJob(row *sql.Row) (*AccountDeletionJob, error) {
	j := &AccountDeletionJob{}
	var createdAt, startedAt, completedAt string
	var lastError sql.NullString
	err := row.Scan(&j.ID, &j.UserID, &j.Status, &j.TotalChannels,
		&j.ChannelsProcessed, &j.GDPRExportGenerated, &lastError,
		&createdAt, &startedAt, &completedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if lastError.Valid {
		j.LastError = &lastError.String
	}
	j.CreatedAt = parseTime(createdAt)
	j.StartedAt = parseTimePtr(parseNullString(startedAt))
	j.CompletedAt = parseTimePtr(parseNullString(completedAt))
	return j, nil
}

func (q *SQLiteQuery) GetUserTrackingChannels(ctx context.Context, userID string) ([]*UserChannelTrackingRow, error) {
	rows, err := q.db.QueryContext(ctx, `
		SELECT t.id, t.user_id, t.platform_channel_id,
		       p.platform, p.platform_channel_id, p.display_name,
		       t.access_token_enc, t.token_expires_at
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
		var tokenExpiresAt string
		if err := rows.Scan(&r.TrackingID, &r.UserID, &r.PlatformChannelID,
			&r.Platform, &r.PlatformChannelKey, &r.DisplayName,
			&r.AccessTokenEnc, &tokenExpiresAt); err != nil {
			return nil, err
		}
		r.TokenExpiresAt = parseTimePtr(parseNullString(tokenExpiresAt))
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

func (q *SQLiteQuery) MarkDataExportCompleted(ctx context.Context, jobID, userID, storagePath, downloadURL string, expiresAt time.Time, partial bool) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		UPDATE data_export_jobs
		SET status='completed', storage_path=?, download_url=?,
		    expires_at=?, partial_export=?, completed_at=strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		WHERE id=?`, storagePath, downloadURL, expiresAt.Format(time.RFC3339), partial, jobID); err != nil {
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
		UPDATE data_export_jobs SET status='failed', last_error=?, completed_at=strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		WHERE id=?`, errorMsg, jobID)
	return err
}

func (q *SQLiteQuery) ListExpiredDataExports(ctx context.Context, now time.Time) ([]*DataExportJob, error) {
	rows, err := q.db.QueryContext(ctx, `
		SELECT `+dataExportJobColumns+`
		FROM data_export_jobs
		WHERE expires_at IS NOT NULL AND expires_at < ? AND storage_path IS NOT NULL`, now.Format(time.RFC3339))
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
		SET storage_path=NULL, download_url=NULL
		WHERE id=?`, jobID)
	return err
}

const dataExportJobColumns = `id, user_id, status, storage_path,
	download_url, expires_at, partial_export, last_error,
	created_at, started_at, completed_at`

func scanDataExportJob(row *sql.Row) (*DataExportJob, error) {
	j := &DataExportJob{}
	var storagePath, downloadURL, expiresAt, lastError, createdAt, startedAt, completedAt string
	var partialExport int
	err := row.Scan(&j.ID, &j.UserID, &j.Status, &storagePath,
		&downloadURL, &expiresAt, &partialExport, &lastError,
		&createdAt, &startedAt, &completedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if storagePath != "" {
		j.StoragePath = &storagePath
	}
	if downloadURL != "" {
		j.DownloadURL = &downloadURL
	}
	if expiresAt != "" {
		t, _ := time.Parse(time.RFC3339, expiresAt)
		j.ExpiresAt = &t
	}
	if lastError != "" {
		j.LastError = &lastError
	}
	j.PartialExport = partialExport != 0
	j.CreatedAt = parseTime(createdAt)
	j.StartedAt = parseTimePtr(parseNullString(startedAt))
	j.CompletedAt = parseTimePtr(parseNullString(completedAt))
	return j, nil
}

func scanDataExportJobRows(rows *sql.Rows) (*DataExportJob, error) {
	j := &DataExportJob{}
	var storagePath, downloadURL, expiresAt, lastError, createdAt, startedAt, completedAt string
	var partialExport int
	err := rows.Scan(&j.ID, &j.UserID, &j.Status, &storagePath,
		&downloadURL, &expiresAt, &partialExport, &lastError,
		&createdAt, &startedAt, &completedAt)
	if err != nil {
		return nil, err
	}
	if storagePath != "" {
		j.StoragePath = &storagePath
	}
	if downloadURL != "" {
		j.DownloadURL = &downloadURL
	}
	if expiresAt != "" {
		t, _ := time.Parse(time.RFC3339, expiresAt)
		j.ExpiresAt = &t
	}
	if lastError != "" {
		j.LastError = &lastError
	}
	j.PartialExport = partialExport != 0
	j.CreatedAt = parseTime(createdAt)
	j.StartedAt = parseTimePtr(parseNullString(startedAt))
	j.CompletedAt = parseTimePtr(parseNullString(completedAt))
	return j, nil
}

func (q *SQLiteQuery) GetUserExportBundle(ctx context.Context, userID string) (*UserExportBundle, error) {
	user, err := q.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	bundle := &UserExportBundle{User: user}

	trackingRows, err := q.db.QueryContext(ctx, `
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

	commentRows, err := q.db.QueryContext(ctx, `
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

	digestRows, err := q.db.QueryContext(ctx, `
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

	starRows, err := q.db.QueryContext(ctx, `
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
