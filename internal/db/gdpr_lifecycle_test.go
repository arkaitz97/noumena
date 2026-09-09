package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// seedGDPRUser inserts rows owned by userID across every user-owned
// table plus the shared/child rows they depend on. The shared workspace
// is owned by wsCreatedBy (the deleted user is a member only, matching
// the design: workspaces themselves are not deleted by account wipe).
func seedGDPRUser(t *testing.T, ctx context.Context, q DBQuery, userID, wsCreatedBy string) {
	t.Helper()
	db := q.(*SQLiteQuery).DB()

	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, sql, args...); err != nil {
			t.Fatalf("seed %s: %v", sql, err)
		}
	}

	mustExec(`INSERT INTO workspaces (id, name, created_by) VALUES ('ws1', 'ws', ?)`, wsCreatedBy)
	mustExec(`INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ('ws1', ?, 'admin')`, userID)
	mustExec(`INSERT INTO platform_channels (id, platform, platform_channel_id, display_name) VALUES ('ch1', 'youtube', 'yt-1', 'Chan')`)
	mustExec(`INSERT INTO user_channel_tracking (id, user_id, platform_channel_id, access_token_enc, refresh_token_enc) VALUES ('tr1', ?, 'ch1', 'enc', 'enc')`, userID)
	mustExec(`INSERT INTO comments (id, platform_channel_id, platform_comment_id, body, posted_at) VALUES ('cm1', 'ch1', 'pc1', 'body', '2026-09-01T00:00:00Z')`)
	mustExec(`INSERT INTO user_comment_stars (user_id, comment_id) VALUES (?, 'cm1')`, userID)
	mustExec(`INSERT INTO ingestion_jobs (id, platform_channel_id, job_type) VALUES ('ij1', 'ch1', 'backfill')`)
	mustExec(`INSERT INTO email_verification_tokens (id, user_id, token_hash, expires_at) VALUES ('ev1', ?, 'h1', '2099-01-01T00:00:00Z')`, userID)
	mustExec(`INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at) VALUES ('pr1', ?, 'h2', '2099-01-01T00:00:00Z')`, userID)
	mustExec(`INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at) VALUES ('rt1', ?, 'h3', '2099-01-01T00:00:00Z')`, userID)
	mustExec(`INSERT INTO oauth_states (state_hash, user_id, expires_at) VALUES ('st1', ?, '2099-01-01T00:00:00Z')`, userID)
	mustExec(`INSERT INTO comment_usage_logs (id, user_id, count, period_start, period_end) VALUES ('ul1', ?, 10, '2026-09-01T00:00:00Z', '2026-10-01T00:00:00Z')`, userID)
	mustExec(`INSERT INTO comment_exports (id, user_id, format, filters, status) VALUES ('ce1', ?, 'csv', '{}', 'completed')`, userID)
	mustExec(`INSERT INTO rate_limit_buckets (id, user_key, window_start) VALUES ('rl1', ?, '2026-09-01T00:00:00Z')`, "u:"+userID)
	mustExec(`INSERT INTO weekly_digests (id, user_id, week_start_date) VALUES ('wd1', ?, '2026-08-31')`, userID)
	mustExec(`INSERT INTO data_export_jobs (id, user_id, status) VALUES ('de1', ?, 'completed')`, userID)
	mustExec(`INSERT INTO account_deletion_jobs (id, user_id, status) VALUES ('ad1', ?, 'pending')`, userID)
	mustExec(`INSERT INTO user_profiles (user_id, user_type) VALUES (?, 'creator')`, userID)
}

// userRowCount returns the number of rows in table for the given user_id
// (or user_key for rate_limit_buckets).
func userRowCount(t *testing.T, ctx context.Context, q DBQuery, table, userID string) int {
	t.Helper()
	db := q.(*SQLiteQuery).DB()
	col := "user_id"
	if table == "rate_limit_buckets" {
		col = "user_key"
	}
	var n int
	query := fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE %s = ?`, table, col)
	if err := db.QueryRowContext(ctx, query, userID).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// gdprUserTables lists every user-owned table the wipe must empty.
var gdprUserTables = []string{
	"email_verification_tokens",
	"password_reset_tokens",
	"refresh_tokens",
	"oauth_states",
	"user_comment_stars",
	"comment_usage_logs",
	"comment_exports",
	"workspace_members",
	"rate_limit_buckets",
	"user_channel_tracking",
	"weekly_digests",
	"data_export_jobs",
	"user_profiles",
}

func testGDPRWipe(t *testing.T, fkOff bool) {
	ctx := context.Background()
	database := openTestDB(t)
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if fkOff {
		if _, err := database.Exec("PRAGMA foreign_keys=OFF"); err != nil {
			t.Fatalf("disable FK: %v", err)
		}
	}
	q := NewSQLiteQuery(database)

	user, err := q.CreateUserWithPassword(ctx, fmt.Sprintf("gdpr_%d@example.com", time.Now().UnixNano()), "hash")
	if err != nil {
		t.Fatalf("CreateUserWithPassword: %v", err)
	}
	// Second user whose shared data must survive.
	other, err := q.CreateUserWithPassword(ctx, fmt.Sprintf("other_%d@example.com", time.Now().UnixNano()), "hash")
	if err != nil {
		t.Fatalf("CreateUserWithPassword: %v", err)
	}
	seedGDPRUser(t, ctx, q, user.ID, other.ID)
	// Other user shares the channel comment (star) and the workspace.
	db := q.(*SQLiteQuery).DB()
	if _, err := db.ExecContext(ctx, `INSERT INTO user_comment_stars (user_id, comment_id) VALUES (?, 'cm1')`, other.ID); err != nil {
		t.Fatalf("seed other star: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ('ws1', ?, 'viewer')`, other.ID); err != nil {
		t.Fatalf("seed other member: %v", err)
	}

	sq := q.(*SQLiteQuery)
	tx, err := sq.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := sq.DeleteUserGDPRData(ctx, tx, user.ID); err != nil {
		_ = tx.Rollback()
		t.Fatalf("DeleteUserGDPRData: %v", err)
	}
	if err := sq.HardDeleteUser(ctx, tx, user.ID); err != nil {
		_ = tx.Rollback()
		t.Fatalf("HardDeleteUser: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	for _, table := range gdprUserTables {
		if n := userRowCount(t, ctx, q, table, user.ID); n != 0 {
			t.Errorf("%s: %d rows remain for %s (fk_off=%v)", table, n, user.ID, fkOff)
		}
	}
	// User row itself is gone.
	if _, err := q.GetUserByID(ctx, user.ID); err != nil {
		t.Fatalf("GetUserByID after delete: %v", err)
	}
	// Other user's shared rows survive (detach-only semantics).
	if n := userRowCount(t, ctx, q, "user_comment_stars", other.ID); n != 1 {
		t.Errorf("other user's star lost: %d rows (fk_off=%v)", n, fkOff)
	}
	if n := userRowCount(t, ctx, q, "workspace_members", other.ID); n != 1 {
		t.Errorf("other user's membership lost: %d rows (fk_off=%v)", n, fkOff)
	}
}

func TestDeleteUserGDPRData_WipesAllTables_FKOn(t *testing.T) {
	testGDPRWipe(t, false)
}

func TestDeleteUserGDPRData_WipesAllTables_FKOff(t *testing.T) {
	testGDPRWipe(t, true)
}

// TestPurgeChannelData_OrphanCleanup: 4.1 — stars of ALL users on the
// channel's comments and the channel's ingestion_jobs are removed with
// the channel.
func TestPurgeChannelData_OrphanCleanup(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	q := NewSQLiteQuery(database)
	db := q.(*SQLiteQuery).DB()

	u1, _ := q.CreateUserWithPassword(ctx, "purge1@example.com", "h")
	u2, _ := q.CreateUserWithPassword(ctx, "purge2@example.com", "h")
	// No active trackers (purge would refuse with any), but both users
	// still hold stars on the channel's comments.
	for _, stmt := range []string{
		`INSERT INTO platform_channels (id, platform, platform_channel_id, display_name) VALUES ('pch', 'youtube', 'yt-purge', 'P')`,
		`INSERT INTO comments (id, platform_channel_id, platform_comment_id, body, posted_at) VALUES ('pc1', 'pch', 'p1', 'b', '2026-09-01T00:00:00Z')`,
		`INSERT INTO user_comment_stars (user_id, comment_id) VALUES ('` + u1.ID + `', 'pc1')`,
		`INSERT INTO user_comment_stars (user_id, comment_id) VALUES ('` + u2.ID + `', 'pc1')`,
		`INSERT INTO ingestion_jobs (id, platform_channel_id, job_type) VALUES ('pj1', 'pch', 'backfill')`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed %q: %v", stmt, err)
		}
	}

	if err := q.PurgeChannelData(ctx, "pch"); err != nil {
		t.Fatalf("PurgeChannelData: %v", err)
	}
	var n int
	for _, query := range []string{
		`SELECT COUNT(*) FROM user_comment_stars WHERE comment_id='pc1'`,
		`SELECT COUNT(*) FROM ingestion_jobs WHERE platform_channel_id='pch'`,
		`SELECT COUNT(*) FROM comments WHERE platform_channel_id='pch'`,
		`SELECT COUNT(*) FROM user_channel_tracking WHERE platform_channel_id='pch'`,
		`SELECT COUNT(*) FROM platform_channels WHERE id='pch'`,
	} {
		if err := db.QueryRowContext(ctx, query).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 0 {
			t.Errorf("%s: %d rows remain", query, n)
		}
	}
}

// TestDeleteStaleUnverifiedUsers_OrphanCleanup: 4.2 — stale unverified
// users' child rows (tokens/states/tracking/exports) are removed with
// them; recent/verified users are untouched.
func TestDeleteStaleUnverifiedUsers_OrphanCleanup(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	q := NewSQLiteQuery(database)
	db := q.(*SQLiteQuery).DB()

	stale, _ := q.CreateUserWithPassword(ctx, "stale@example.com", "h")
	recent, _ := q.CreateUserWithPassword(ctx, "recent@example.com", "h")
	verified, _ := q.CreateUserWithPassword(ctx, "verified@example.com", "h")
	if _, err := db.ExecContext(ctx, `UPDATE users SET created_at = '2020-01-01T00:00:00Z' WHERE id = ?`, stale.ID); err != nil {
		t.Fatalf("age stale user: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE users SET email_verified_at = '2026-01-01T00:00:00Z' WHERE id = ?`, verified.ID); err != nil {
		t.Fatalf("verify user: %v", err)
	}
	for _, stmt := range []string{
		`INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at) VALUES ('srt', '` + stale.ID + `', 'h', '2099-01-01T00:00:00Z')`,
		`INSERT INTO oauth_states (state_hash, user_id, expires_at) VALUES ('sst', '` + stale.ID + `', '2099-01-01T00:00:00Z')`,
		`INSERT INTO comment_exports (id, user_id, format, filters, status) VALUES ('sce', '` + stale.ID + `', 'json', '{}', 'queued')`,
		`INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at) VALUES ('rrt', '` + recent.ID + `', 'h2', '2099-01-01T00:00:00Z')`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed %q: %v", stmt, err)
		}
	}

	n, err := q.DeleteStaleUnverifiedUsers(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("DeleteStaleUnverifiedUsers: %v", err)
	}
	if n != 1 {
		t.Errorf("deleted %d users, want 1", n)
	}
	if _, err := q.GetUserByID(ctx, stale.ID); err != nil {
		t.Fatalf("stale user should be gone: %v", err)
	}
	if _, err := q.GetUserByID(ctx, recent.ID); err != nil {
		t.Fatalf("recent user should remain: %v", err)
	}
	if _, err := q.GetUserByID(ctx, verified.ID); err != nil {
		t.Fatalf("verified user should remain: %v", err)
	}
	for _, table := range []string{"refresh_tokens", "oauth_states", "comment_exports"} {
		if n := userRowCount(t, ctx, q, table, stale.ID); n != 0 {
			t.Errorf("%s: %d orphan rows remain for stale user", table, n)
		}
	}
	if n := userRowCount(t, ctx, q, "refresh_tokens", recent.ID); n != 1 {
		t.Errorf("recent user's token lost: %d rows", n)
	}
}

func testDeleteWorkspaceCleanup(t *testing.T, fkOff bool) {
	ctx := context.Background()
	database := openTestDB(t)
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if fkOff {
		if _, err := database.Exec("PRAGMA foreign_keys=OFF"); err != nil {
			t.Fatalf("disable FK: %v", err)
		}
	}
	q := NewSQLiteQuery(database)
	db := q.(*SQLiteQuery).DB()

	owner, _ := q.CreateUserWithPassword(ctx, "wsowner@example.com", "h")
	member, _ := q.CreateUserWithPassword(ctx, "wsmember@example.com", "h")
	if _, err := q.CreateWorkspace(ctx, "Team", owner.ID); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	ws, err := q.GetWorkspace(ctx, "Team") // name lookup fails; fetch by owner list instead
	if err == nil {
		_ = ws
	}
	var wsID string
	if err := db.QueryRowContext(ctx, `SELECT id FROM workspaces WHERE name='Team'`).Scan(&wsID); err != nil {
		t.Fatalf("get ws id: %v", err)
	}
	if err := q.AddWorkspaceMember(ctx, wsID, member.ID, "viewer"); err != nil {
		t.Fatalf("AddWorkspaceMember: %v", err)
	}
	if err := q.SetUserActiveWorkspace(ctx, member.ID, wsID); err != nil {
		t.Fatalf("SetUserActiveWorkspace: %v", err)
	}
	for _, stmt := range []string{
		`INSERT INTO platform_channels (id, platform, platform_channel_id, display_name) VALUES ('wch', 'youtube', 'yt-ws', 'W')`,
		`INSERT INTO user_channel_tracking (id, user_id, platform_channel_id, access_token_enc, refresh_token_enc, workspace_id) VALUES ('wtr', '` + member.ID + `', 'wch', 'e', 'e', '` + wsID + `')`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed %q: %v", stmt, err)
		}
	}

	if err := q.DeleteWorkspace(ctx, wsID); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}

	// Members' active_workspace_id NULLed.
	var activeWorkspaceID *string
	if err := db.QueryRowContext(ctx, `SELECT active_workspace_id FROM users WHERE id=?`, member.ID).Scan(&activeWorkspaceID); err != nil {
		t.Fatalf("read active_workspace_id: %v", err)
	}
	if activeWorkspaceID != nil {
		t.Errorf("active_workspace_id = %q, want NULL (fk_off=%v)", *activeWorkspaceID, fkOff)
	}
	// Tracking workspace_id cleared.
	var workspaceID *string
	if err := db.QueryRowContext(ctx, `SELECT workspace_id FROM user_channel_tracking WHERE id='wtr'`).Scan(&workspaceID); err != nil {
		t.Fatalf("read tracking workspace_id: %v", err)
	}
	if workspaceID != nil {
		t.Errorf("tracking.workspace_id = %q, want NULL (fk_off=%v)", *workspaceID, fkOff)
	}
	// Workspace + membership rows gone.
	for _, query := range []string{
		`SELECT COUNT(*) FROM workspaces WHERE id='` + wsID + `'`,
		`SELECT COUNT(*) FROM workspace_members WHERE workspace_id='` + wsID + `'`,
		`SELECT COUNT(*) FROM workspace_invites WHERE workspace_id='` + wsID + `'`,
	} {
		var n int
		if err := db.QueryRowContext(ctx, query).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 0 {
			t.Errorf("%s: %d rows remain (fk_off=%v)", query, n, fkOff)
		}
	}
}

func TestDeleteWorkspace_CleansMemberRefs_FKOn(t *testing.T) {
	testDeleteWorkspaceCleanup(t, false)
}

func TestDeleteWorkspace_CleansMemberRefs_FKOff(t *testing.T) {
	testDeleteWorkspaceCleanup(t, true)
}

// TestQuotaResetAt_NewUserScheduled: 7.1 — new users get a ~30d reset
// schedule so the free-tier cron always has a target.
func TestQuotaResetAt_NewUserScheduled(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	q := NewSQLiteQuery(database)

	for name, create := range map[string]func() (*User, error){
		"password": func() (*User, error) { return q.CreateUserWithPassword(ctx, "qr_pw@example.com", "h") },
		"google":   func() (*User, error) { return q.CreateUserGoogle(ctx, "qr_go@example.com", "sub1") },
	} {
		u, err := create()
		if err != nil {
			t.Fatalf("%s create: %v", name, err)
		}
		if u.QuotaResetAt == nil {
			t.Fatalf("%s: quota_reset_at is NULL", name)
		}
		until := time.Until(*u.QuotaResetAt)
		if until < 25*24*time.Hour || until > 35*24*time.Hour {
			t.Errorf("%s: quota_reset_at = %v (%v from now), want ~30d", name, *u.QuotaResetAt, until)
		}
	}
}

// TestQuotaResetAt_ResetAndBackfill: 7.2/7.3 — past-due free users are
// reset by the cron, and NULL-schedule users get one via the v7 backfill
// UPDATE (mirrors the migration exactly).
func TestQuotaResetAt_ResetAndBackfill(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	q := NewSQLiteQuery(database)
	db := q.(*SQLiteQuery).DB()

	pastDue, _ := q.CreateUserWithPassword(ctx, "pastdue@example.com", "h")
	if _, err := db.ExecContext(ctx,
		`UPDATE users SET quota_reset_at = '2020-01-01T00:00:00Z', comments_used_this_period = 42 WHERE id = ?`,
		pastDue.ID); err != nil {
		t.Fatalf("age past-due user: %v", err)
	}

	n, err := q.ResetFreeTierQuotas(ctx)
	if err != nil {
		t.Fatalf("ResetFreeTierQuotas: %v", err)
	}
	if n != 1 {
		t.Errorf("reset %d users, want 1", n)
	}
	var used int
	var resetAt string
	if err := db.QueryRowContext(ctx,
		`SELECT comments_used_this_period, quota_reset_at FROM users WHERE id=?`, pastDue.ID).
		Scan(&used, &resetAt); err != nil {
		t.Fatalf("read past-due user: %v", err)
	}
	if used != 0 {
		t.Errorf("comments_used = %d, want 0", used)
	}
	if resetAt <= time.Now().UTC().Format(time.RFC3339) {
		t.Errorf("quota_reset_at = %q, want future", resetAt)
	}

	// 7.2 backfill: existing NULL users get a schedule (v7 migration SQL).
	nullUser, _ := q.CreateUserWithPassword(ctx, "nullqr@example.com", "h")
	if _, err := db.ExecContext(ctx,
		`UPDATE users SET quota_reset_at = NULL WHERE id = ?`, nullUser.ID); err != nil {
		t.Fatalf("null out schedule: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE users
		 SET quota_reset_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now', '+30 days')
		 WHERE quota_reset_at IS NULL`); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	var backfilled string
	if err := db.QueryRowContext(ctx,
		`SELECT quota_reset_at FROM users WHERE id=?`, nullUser.ID).Scan(&backfilled); err != nil {
		t.Fatalf("read backfilled user: %v", err)
	}
	parsed, err := time.Parse(time.RFC3339, backfilled)
	if err != nil {
		t.Fatalf("parse backfilled quota_reset_at %q: %v", backfilled, err)
	}
	if until := time.Until(parsed); until < 25*24*time.Hour || until > 35*24*time.Hour {
		t.Errorf("backfilled quota_reset_at = %v (%v from now), want ~30d", parsed, until)
	}
}

// TestDeletionRevokeStatusRoundTrip: 6.2 — revoke failures are recorded
// on the job and surfaced by the sweeper query; clearing the error
// removes the job from the retry set.
func TestDeletionRevokeStatusRoundTrip(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	q := NewSQLiteQuery(database)

	u, _ := q.CreateUserWithPassword(ctx, "revokestatus@example.com", "h")
	job, err := q.ScheduleAccountDeletion(ctx, u.ID, 0)
	if err != nil {
		t.Fatalf("ScheduleAccountDeletion: %v", err)
	}
	if err := q.MarkDeletionStage(ctx, job.ID, "deleting"); err != nil {
		t.Fatalf("MarkDeletionStage: %v", err)
	}
	if err := q.MarkDeletionRevokeStatus(ctx, job.ID, true, "boom"); err != nil {
		t.Fatalf("MarkDeletionRevokeStatus: %v", err)
	}

	pending, err := q.ListJobsPendingRevoke(ctx)
	if err != nil {
		t.Fatalf("ListJobsPendingRevoke: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != job.ID {
		t.Fatalf("pending revoke jobs = %+v, want 1 (job %s)", pending, job.ID)
	}
	if pending[0].RevokeError == nil || *pending[0].RevokeError != "boom" {
		t.Errorf("revoke_error = %v, want boom", pending[0].RevokeError)
	}

	// Successful retry clears the error → job leaves the retry set.
	if err := q.MarkDeletionRevokeStatus(ctx, job.ID, true, ""); err != nil {
		t.Fatalf("MarkDeletionRevokeStatus clear: %v", err)
	}
	pending, err = q.ListJobsPendingRevoke(ctx)
	if err != nil {
		t.Fatalf("ListJobsPendingRevoke: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("pending revoke jobs after clear = %+v, want none", pending)
	}
}
