package db

import (
	"context"
	"time"
)

// WorkspaceStore is the narrow data-access surface for multi-seat workspaces
// and the audit log.
type WorkspaceStore interface {
	CreateWorkspace(ctx context.Context, name, createdBy string) (*Workspace, error)
	GetWorkspace(ctx context.Context, workspaceID string) (*Workspace, error)
	UpdateWorkspace(ctx context.Context, workspaceID, name string) error
	DeleteWorkspace(ctx context.Context, workspaceID string) error
	ListWorkspacesForUser(ctx context.Context, userID string) ([]*WorkspaceWithMembers, error)
	GetWorkspaceMember(ctx context.Context, workspaceID, userID string) (*WorkspaceMember, error)
	ListWorkspaceMembers(ctx context.Context, workspaceID string) ([]*WorkspaceMemberWithEmail, error)
	AddWorkspaceMember(ctx context.Context, workspaceID, userID, role string) error
	RemoveWorkspaceMember(ctx context.Context, workspaceID, userID string) error
	UpdateWorkspaceMemberRole(ctx context.Context, workspaceID, userID, role string) error
	CountWorkspaceMembers(ctx context.Context, workspaceID string) (int, error)
	CountWorkspaceAdmins(ctx context.Context, workspaceID string) (int, error)
	CreateWorkspaceInvite(ctx context.Context, workspaceID, email, role, tokenHash string, expiresAt time.Time) (*WorkspaceInvite, error)
	GetWorkspaceInviteByToken(ctx context.Context, tokenHash string) (*WorkspaceInvite, error)
	ConsumeWorkspaceInvite(ctx context.Context, inviteID string) error
	GetWorkspaceInviteByEmail(ctx context.Context, workspaceID, email string) (*WorkspaceInvite, error)
	SetUserActiveWorkspace(ctx context.Context, userID, workspaceID string) error
	GetUserActiveWorkspaceID(ctx context.Context, userID string) (string, error)
	IsWorkspaceOwner(ctx context.Context, workspaceID, userID string) (bool, error)
	GrantChannel(ctx context.Context, workspaceID, channelID, grantedBy string) error
	RevokeChannel(ctx context.Context, workspaceID, channelID string) error
	ListGrantedChannels(ctx context.Context, workspaceID string) ([]string, error)

	InsertAuditLog(ctx context.Context, userID, action string, details string, ipAddress string) error
}

var _ WorkspaceStore = (DBQuery)(nil)
