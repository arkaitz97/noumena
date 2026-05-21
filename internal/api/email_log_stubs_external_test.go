package api_test

import (
	"context"
	"time"

	"database/sql"

	"noumena/internal/db"
)

// data-lifecycle change: extend the external test MockQuery with no-op
// EmailLogStore methods so it continues to satisfy db.DBQuery. The mock
// is shared by the auth/channel/comment/billing/digest handler tests; the
// pipeline isn't exercised through any of those paths so zero-value
// returns are fine.

func (m *MockQuery) IsEmailIdempotencyKeyUsed(context.Context, string) (bool, error) {
	return false, nil
}
func (m *MockQuery) LogEmailAttempt(context.Context, db.EmailAttemptInput) (string, error) {
	return "stub-id", nil
}
func (m *MockQuery) MarkEmailSent(context.Context, string, time.Time) error          { return nil }
func (m *MockQuery) IncrementEmailAttempt(context.Context, string, int, string) error { return nil }
func (m *MockQuery) MarkEmailFailed(context.Context, string, string) error           { return nil }
func (m *MockQuery) MarkEmailAttemptError(context.Context, string, string) error     { return nil }
func (m *MockQuery) ClaimPendingEmails(context.Context, time.Time, int) ([]*db.EmailLogRow, error) {
	return nil, nil
}

// data-lifecycle change: extend the external test MockQuery with no-op
// LifecycleStore methods.

func (m *MockQuery) ScheduleAccountDeletion(_ context.Context, userID string, _ int) (*db.AccountDeletionJob, error) {
	// Return a non-nil stub so handlers that read job.ID don't panic.
	// Tests that need to assert specific fields override this method
	// via composition or by setting fields on MockQuery directly.
	return &db.AccountDeletionJob{ID: "job-" + userID, UserID: userID}, nil
}
func (m *MockQuery) CancelAccountDeletion(context.Context, string) error { return nil }
func (m *MockQuery) ClaimNextDeletionJob(context.Context, time.Duration) (*db.AccountDeletionJob, error) {
	return nil, nil
}
func (m *MockQuery) MarkDeletionStage(context.Context, string, string) error          { return nil }
func (m *MockQuery) MarkDeletionFailed(context.Context, string, string) error         { return nil }
func (m *MockQuery) IncrementChannelsProcessed(context.Context, string) error         { return nil }
func (m *MockQuery) MarkDeletionCompleted(context.Context, string, string) error      { return nil }
func (m *MockQuery) MarkDeletionCompletedJobOnly(context.Context, string) error       { return nil }
func (m *MockQuery) GetUserTrackingChannels(context.Context, string) ([]*db.UserChannelTrackingRow, error) {
	return nil, nil
}
func (m *MockQuery) GetActiveTrackerCountForUpdate(context.Context, *sql.Tx, string, string) (int, error) {
	return 0, nil
}
func (m *MockQuery) DeleteUserChannelTracking(context.Context, *sql.Tx, string, string) error {
	return nil
}
func (m *MockQuery) DeleteCommentsByChannelChunked(context.Context, string, int, string) (int, error) {
	return 0, nil
}
func (m *MockQuery) DeletePlatformChannel(context.Context, *sql.Tx, string) error      { return nil }
func (m *MockQuery) DeleteUserStarsForChannel(context.Context, *sql.Tx, string, string) error {
	return nil
}
func (m *MockQuery) DeleteUserStarsAll(context.Context, *sql.Tx, string) error      { return nil }
func (m *MockQuery) DeleteUserDigests(context.Context, *sql.Tx, string) error       { return nil }
func (m *MockQuery) DeleteUserAuthTokens(context.Context, *sql.Tx, string) error    { return nil }
func (m *MockQuery) HardDeleteUser(context.Context, *sql.Tx, string) error          { return nil }
func (m *MockQuery) EnqueueDataExport(context.Context, string) (*db.DataExportJob, error) {
	return nil, nil
}
func (m *MockQuery) GetDataExportJob(context.Context, string) (*db.DataExportJob, error) {
	return nil, nil
}
func (m *MockQuery) ClaimNextExportJob(context.Context) (*db.DataExportJob, error) {
	return nil, nil
}
func (m *MockQuery) MarkExportStarted(context.Context, string) error { return nil }
func (m *MockQuery) MarkDataExportCompleted(context.Context, string, string, string, string, time.Time, bool) error {
	return nil
}
func (m *MockQuery) MarkDataExportFailed(context.Context, string, string) error { return nil }
func (m *MockQuery) ListExpiredDataExports(context.Context, time.Time) ([]*db.DataExportJob, error) {
	return nil, nil
}
func (m *MockQuery) ClearExportStoragePath(context.Context, string) error { return nil }
func (m *MockQuery) GetUserExportBundle(context.Context, string) (*db.UserExportBundle, error) {
	return nil, nil
}
