package api

import (
	"context"
	"time"

	"database/sql"

	"noumena/internal/db"
)

// data-lifecycle change: extend guardMockQuery (in-package mock) with
// no-op EmailLogStore methods so it continues to satisfy db.DBQuery.
// The api_test.MockQuery shares the same need; see
// email_log_stubs_external_test.go.

func (g *guardMockQuery) IsEmailIdempotencyKeyUsed(context.Context, string) (bool, error) {
	return false, nil
}
func (g *guardMockQuery) LogEmailAttempt(context.Context, db.EmailAttemptInput) (string, error) {
	return "stub-id", nil
}
func (g *guardMockQuery) MarkEmailSent(context.Context, string, time.Time) error          { return nil }
func (g *guardMockQuery) IncrementEmailAttempt(context.Context, string, int, string) error { return nil }
func (g *guardMockQuery) MarkEmailFailed(context.Context, string, string) error           { return nil }
func (g *guardMockQuery) MarkEmailAttemptError(context.Context, string, string) error     { return nil }
func (g *guardMockQuery) ClaimPendingEmails(context.Context, time.Time, int) ([]*db.EmailLogRow, error) {
	return nil, nil
}

// data-lifecycle change: extend guardMockQuery with no-op
// LifecycleStore methods (account deletion + GDPR export). These
// handler tests don't exercise the deletion/export flows directly, so
// zero-value/never-fail stubs keep the surface satisfied.

func (g *guardMockQuery) ScheduleAccountDeletion(context.Context, string, int) (*db.AccountDeletionJob, error) {
	return nil, nil
}
func (g *guardMockQuery) CancelAccountDeletion(context.Context, string) error { return nil }
func (g *guardMockQuery) ClaimNextDeletionJob(context.Context, time.Duration) (*db.AccountDeletionJob, error) {
	return nil, nil
}
func (g *guardMockQuery) MarkDeletionStage(context.Context, string, string) error          { return nil }
func (g *guardMockQuery) MarkDeletionFailed(context.Context, string, string) error         { return nil }
func (g *guardMockQuery) IncrementChannelsProcessed(context.Context, string) error         { return nil }
func (g *guardMockQuery) MarkDeletionCompleted(context.Context, string, string) error      { return nil }
func (g *guardMockQuery) MarkDeletionCompletedJobOnly(context.Context, string) error       { return nil }
func (g *guardMockQuery) GetUserTrackingChannels(context.Context, string) ([]*db.UserChannelTrackingRow, error) {
	return nil, nil
}
func (g *guardMockQuery) GetActiveTrackerCountForUpdate(context.Context, *sql.Tx, string, string) (int, error) {
	return 0, nil
}
func (g *guardMockQuery) DeleteUserChannelTracking(context.Context, *sql.Tx, string, string) error {
	return nil
}
func (g *guardMockQuery) DeleteCommentsByChannelChunked(context.Context, string, int, string) (int, error) {
	return 0, nil
}
func (g *guardMockQuery) DeletePlatformChannel(context.Context, *sql.Tx, string) error      { return nil }
func (g *guardMockQuery) DeleteUserStarsForChannel(context.Context, *sql.Tx, string, string) error {
	return nil
}
func (g *guardMockQuery) DeleteUserStarsAll(context.Context, *sql.Tx, string) error      { return nil }
func (g *guardMockQuery) DeleteUserDigests(context.Context, *sql.Tx, string) error       { return nil }
func (g *guardMockQuery) DeleteUserAuthTokens(context.Context, *sql.Tx, string) error    { return nil }
func (g *guardMockQuery) HardDeleteUser(context.Context, *sql.Tx, string) error          { return nil }
func (g *guardMockQuery) EnqueueDataExport(context.Context, string) (*db.DataExportJob, error) {
	return nil, nil
}
func (g *guardMockQuery) GetDataExportJob(context.Context, string) (*db.DataExportJob, error) {
	return nil, nil
}
func (g *guardMockQuery) ClaimNextExportJob(context.Context) (*db.DataExportJob, error) {
	return nil, nil
}
func (g *guardMockQuery) MarkExportStarted(context.Context, string) error { return nil }
func (g *guardMockQuery) MarkDataExportCompleted(context.Context, string, string, string, string, time.Time, bool) error {
	return nil
}
func (g *guardMockQuery) MarkDataExportFailed(context.Context, string, string) error { return nil }
func (g *guardMockQuery) ListExpiredDataExports(context.Context, time.Time) ([]*db.DataExportJob, error) {
	return nil, nil
}
func (g *guardMockQuery) ClearExportStoragePath(context.Context, string) error { return nil }
func (g *guardMockQuery) GetUserExportBundle(context.Context, string) (*db.UserExportBundle, error) {
	return nil, nil
}
