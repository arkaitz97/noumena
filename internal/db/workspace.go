package db

import (
	"context"
	"database/sql"
	"time"
)

func (q *SQLiteQuery) CreateWorkspace(ctx context.Context, name, createdBy string) (*Workspace, error) {
	w := &Workspace{}
	var createdAt string
	err := q.db.QueryRowContext(ctx,
		`INSERT INTO workspaces (id, name, created_by, created_at) VALUES (?, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) RETURNING id, name, created_by, created_at`,
		generateUUID(), name, createdBy).Scan(&w.ID, &w.Name, &w.CreatedBy, &createdAt)
	if err != nil {
		return nil, err
	}
	w.CreatedAt = parseTime(createdAt)
	return w, nil
}

func (q *SQLiteQuery) GetWorkspace(ctx context.Context, workspaceID string) (*Workspace, error) {
	w := &Workspace{}
	var createdAt string
	err := q.db.QueryRowContext(ctx,
		`SELECT id, name, created_by, created_at FROM workspaces WHERE id = ?`,
		workspaceID).Scan(&w.ID, &w.Name, &w.CreatedBy, &createdAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	w.CreatedAt = parseTime(createdAt)
	return w, nil
}

func (q *SQLiteQuery) UpdateWorkspace(ctx context.Context, workspaceID, name string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE workspaces SET name = ? WHERE id = ?`, name, workspaceID)
	return err
}

func (q *SQLiteQuery) DeleteWorkspace(ctx context.Context, workspaceID string) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Clear member references BEFORE removing the row so the delete
	// works with foreign_keys ON (no dangling FKs) and OFF (no dangling
	// pointers). workspace_members/workspace_invites cascade on FK but
	// are removed explicitly here for the FK-off case.
	if _, err := tx.ExecContext(ctx,
		`UPDATE users SET active_workspace_id = NULL WHERE active_workspace_id = ?`,
		workspaceID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE user_channel_tracking SET workspace_id = NULL WHERE workspace_id = ?`,
		workspaceID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM workspace_members WHERE workspace_id = ?`, workspaceID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM workspace_invites WHERE workspace_id = ?`, workspaceID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM workspaces WHERE id = ?`, workspaceID); err != nil {
		return err
	}
	return tx.Commit()
}

func (q *SQLiteQuery) ListWorkspacesForUser(ctx context.Context, userID string) ([]*WorkspaceWithMembers, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT w.id, w.name, w.created_by, w.created_at,
				COUNT(wm.user_id) as member_count,
				COALESCE(member.role, '') as user_role
		 FROM workspaces w
		 JOIN workspace_members wm ON wm.workspace_id = w.id
		 LEFT JOIN workspace_members member ON member.workspace_id = w.id AND member.user_id = ?
		 WHERE wm.user_id = ?
		 GROUP BY w.id, w.name, w.created_by, w.created_at, member.role
		 ORDER BY w.created_at DESC`,
		userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workspaces []*WorkspaceWithMembers
	for rows.Next() {
		w := &WorkspaceWithMembers{}
		var createdAt string
		err := rows.Scan(&w.ID, &w.Name, &w.CreatedBy, &createdAt, &w.MemberCount, &w.UserRole)
		if err != nil {
			return nil, err
		}
		w.CreatedAt = parseTime(createdAt)
		w.IsOwner = w.CreatedBy == userID
		workspaces = append(workspaces, w)
	}
	return workspaces, nil
}

func (q *SQLiteQuery) ListWorkspaceMembers(ctx context.Context, workspaceID string) ([]*WorkspaceMemberWithEmail, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT wm.user_id, u.email, wm.role, wm.created_at
		 FROM workspace_members wm
		 JOIN users u ON u.id = wm.user_id
		 WHERE wm.workspace_id = ?
		 ORDER BY wm.created_at ASC`,
		workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []*WorkspaceMemberWithEmail
	for rows.Next() {
		m := &WorkspaceMemberWithEmail{}
		var createdAt string
		if err := rows.Scan(&m.UserID, &m.Email, &m.Role, &createdAt); err != nil {
			return nil, err
		}
		m.CreatedAt = parseTime(createdAt)
		members = append(members, m)
	}
	return members, rows.Err()
}

func (q *SQLiteQuery) GetWorkspaceMember(ctx context.Context, workspaceID, userID string) (*WorkspaceMember, error) {
	m := &WorkspaceMember{}
	var createdAt string
	err := q.db.QueryRowContext(ctx,
		`SELECT workspace_id, user_id, role, created_at FROM workspace_members
		 WHERE workspace_id = ? AND user_id = ?`,
		workspaceID, userID).Scan(&m.WorkspaceID, &m.UserID, &m.Role, &createdAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	m.CreatedAt = parseTime(createdAt)
	return m, err
}

func (q *SQLiteQuery) AddWorkspaceMember(ctx context.Context, workspaceID, userID, role string) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO workspace_members (workspace_id, user_id, role) VALUES (?, ?, ?)`,
		workspaceID, userID, role)
	return err
}

func (q *SQLiteQuery) RemoveWorkspaceMember(ctx context.Context, workspaceID, userID string) error {
	_, err := q.db.ExecContext(ctx,
		`DELETE FROM workspace_members WHERE workspace_id = ? AND user_id = ?`,
		workspaceID, userID)
	return err
}

func (q *SQLiteQuery) UpdateWorkspaceMemberRole(ctx context.Context, workspaceID, userID, role string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE workspace_members SET role = ? WHERE workspace_id = ? AND user_id = ?`,
		role, workspaceID, userID)
	return err
}

func (q *SQLiteQuery) CountWorkspaceMembers(ctx context.Context, workspaceID string) (int, error) {
	var count int
	err := q.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM workspace_members WHERE workspace_id = ?`,
		workspaceID).Scan(&count)
	return count, err
}

func (q *SQLiteQuery) CountWorkspaceAdmins(ctx context.Context, workspaceID string) (int, error) {
	var count int
	err := q.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM workspace_members WHERE workspace_id = ? AND role = 'admin'`,
		workspaceID).Scan(&count)
	return count, err
}

func (q *SQLiteQuery) CreateWorkspaceInvite(ctx context.Context, workspaceID, email, role, tokenHash string, expiresAt time.Time) (*WorkspaceInvite, error) {
	inv := &WorkspaceInvite{}
	var expiresAtStr, consumedAtStr, createdAtStr string
	err := q.db.QueryRowContext(ctx,
		`INSERT INTO workspace_invites (id, workspace_id, email, role, token_hash, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?) RETURNING id, workspace_id, email, role, token_hash, expires_at, consumed_at, created_at`,
		generateUUID(), workspaceID, email, role, tokenHash, expiresAt.Format(time.RFC3339)).Scan(
		&inv.ID, &inv.WorkspaceID, &inv.Email, &inv.Role, &inv.TokenHash, &expiresAtStr, &consumedAtStr, &createdAtStr)
	if err != nil {
		return nil, err
	}
	inv.ExpiresAt = parseTime(expiresAtStr)
	inv.ConsumedAt = parseTimePtr(parseNullString(consumedAtStr))
	inv.CreatedAt = parseTime(createdAtStr)
	return inv, nil
}

func (q *SQLiteQuery) GetWorkspaceInviteByToken(ctx context.Context, tokenHash string) (*WorkspaceInvite, error) {
	inv := &WorkspaceInvite{}
	var expiresAtStr, consumedAtStr, createdAtStr string
	err := q.db.QueryRowContext(ctx,
		`SELECT id, workspace_id, email, role, token_hash, expires_at, consumed_at, created_at
		 FROM workspace_invites WHERE token_hash = ?`,
		tokenHash).Scan(&inv.ID, &inv.WorkspaceID, &inv.Email, &inv.Role, &inv.TokenHash, &expiresAtStr, &consumedAtStr, &createdAtStr)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	inv.ExpiresAt = parseTime(expiresAtStr)
	inv.ConsumedAt = parseTimePtr(parseNullString(consumedAtStr))
	inv.CreatedAt = parseTime(createdAtStr)
	return inv, err
}

func (q *SQLiteQuery) ConsumeWorkspaceInvite(ctx context.Context, inviteID string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE workspace_invites SET consumed_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ?`, inviteID)
	return err
}

func (q *SQLiteQuery) GetWorkspaceInviteByEmail(ctx context.Context, workspaceID, email string) (*WorkspaceInvite, error) {
	inv := &WorkspaceInvite{}
	var expiresAtStr, consumedAtStr, createdAtStr string
	err := q.db.QueryRowContext(ctx,
		`SELECT id, workspace_id, email, role, token_hash, expires_at, consumed_at, created_at
		 FROM workspace_invites WHERE workspace_id = ? AND email = ? AND consumed_at IS NULL AND expires_at > strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		 ORDER BY created_at DESC LIMIT 1`,
		workspaceID, email).Scan(&inv.ID, &inv.WorkspaceID, &inv.Email, &inv.Role, &inv.TokenHash, &expiresAtStr, &consumedAtStr, &createdAtStr)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	inv.ExpiresAt = parseTime(expiresAtStr)
	inv.ConsumedAt = parseTimePtr(parseNullString(consumedAtStr))
	inv.CreatedAt = parseTime(createdAtStr)
	return inv, err
}

func (q *SQLiteQuery) SetUserActiveWorkspace(ctx context.Context, userID, workspaceID string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE users SET active_workspace_id = ? WHERE id = ?`, workspaceID, userID)
	return err
}

func (q *SQLiteQuery) GetUserActiveWorkspaceID(ctx context.Context, userID string) (string, error) {
	var workspaceID sql.NullString
	err := q.db.QueryRowContext(ctx,
		`SELECT active_workspace_id FROM users WHERE id = ?`, userID).Scan(&workspaceID)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	if !workspaceID.Valid {
		return "", nil
	}
	return workspaceID.String, err
}

func (q *SQLiteQuery) IsWorkspaceOwner(ctx context.Context, workspaceID, userID string) (bool, error) {
	var exists bool
	err := q.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM workspaces WHERE id = ? AND created_by = ?)`,
		workspaceID, userID).Scan(&exists)
	return exists, err
}

func (q *SQLiteQuery) InsertAuditLog(ctx context.Context, userID, action string, details string, ipAddress string) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO audit_log (id, user_id, action, details, ip_address) VALUES (?, ?, ?, ?, ?)`,
		generateUUID(), userID, action, details, ipAddress)
	return err
}

func (q *SQLiteQuery) IncrementRateLimit(ctx context.Context, userKey string, windowStart time.Time, limit int) (int, error) {
	var count int
	err := q.db.QueryRowContext(ctx,
		`INSERT INTO rate_limit_buckets (id, user_key, window_start, count)
		 VALUES (?, ?, ?, 1)
		 ON CONFLICT (user_key, window_start)
		 DO UPDATE SET count = rate_limit_buckets.count + 1
		 RETURNING count`,
		generateUUID(), userKey, windowStart.Format(time.RFC3339)).Scan(&count)
	return count, err
}

func (q *SQLiteQuery) GetRateLimitStatus(ctx context.Context, userKey string, windowStart time.Time, limit int) (*RateLimitStatus, error) {
	var count int
	err := q.db.QueryRowContext(ctx,
		`SELECT count FROM rate_limit_buckets WHERE user_key = ? AND window_start = ?`,
		userKey, windowStart.Format(time.RFC3339)).Scan(&count)
	if err == sql.ErrNoRows {
		count = 0
	} else if err != nil {
		return nil, err
	}

	remaining := limit - count
	if remaining < 0 {
		remaining = 0
	}

	return &RateLimitStatus{
		UserKey:   userKey,
		Limit:     limit,
		Remaining: remaining,
		ResetAt:   windowStart.Add(time.Minute),
	}, nil
}

func (q *SQLiteQuery) CleanupExpiredRateLimitBuckets(ctx context.Context, before time.Time) (int64, error) {
	result, err := q.db.ExecContext(ctx,
		`DELETE FROM rate_limit_buckets WHERE window_start < ?`, before.Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (q *SQLiteQuery) GrantChannel(ctx context.Context, workspaceID, channelID, grantedBy string) error {
	res, err := q.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO channel_grants (id, workspace_id, channel_id, granted_by) VALUES (?, ?, ?, ?)`,
		generateUUID(), workspaceID, channelID, grantedBy)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	var one int
	err = q.db.QueryRowContext(ctx,
		`SELECT 1 FROM platform_channels WHERE id = ?`, channelID).Scan(&one)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	return err
}

func (q *SQLiteQuery) RevokeChannel(ctx context.Context, workspaceID, channelID string) error {
	_, err := q.db.ExecContext(ctx,
		`DELETE FROM channel_grants WHERE workspace_id = ? AND channel_id = ?`,
		workspaceID, channelID)
	return err
}

func (q *SQLiteQuery) ListGrantedChannels(ctx context.Context, workspaceID string) ([]string, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT channel_id FROM channel_grants WHERE workspace_id = ? ORDER BY created_at, channel_id`,
		workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
