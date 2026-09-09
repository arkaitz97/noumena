package db

import (
	"time"
)

type Workspace struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

type WorkspaceMember struct {
	WorkspaceID string    `json:"workspace_id"`
	UserID      string    `json:"user_id"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"created_at"`
}

// WorkspaceMemberWithEmail is the members-list projection (join with users
// for the email address). Used by GET /api/workspaces/{id}/members.
type WorkspaceMemberWithEmail struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// EmailChange is a pending email-change request (fix-frontend-plumbing).
// The user stays on their old email until the token is consumed.
type EmailChange struct {
	ID           string     `json:"id"`
	UserID       string     `json:"user_id"`
	PendingEmail string     `json:"pending_email"`
	TokenHash    string     `json:"-"`
	ExpiresAt    time.Time  `json:"expires_at"`
	UsedAt       *time.Time `json:"used_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

type WorkspaceInvite struct {
	ID          string     `json:"id"`
	WorkspaceID string     `json:"workspace_id"`
	Email       string     `json:"email"`
	Role        string     `json:"role"`
	TokenHash   string     `json:"-"`
	ExpiresAt   time.Time  `json:"expires_at"`
	ConsumedAt  *time.Time `json:"consumed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type WorkspaceWithMembers struct {
	Workspace
	MemberCount int    `json:"member_count"`
	UserRole    string `json:"user_role,omitempty"`
	IsOwner     bool   `json:"is_owner"`
	// SeatsLimit is the plan seat cap for this workspace, resolved in the
	// api layer from the owner's subscription plan (billing.LimitsFor).
	SeatsLimit int `json:"seats_limit"`
}

type AuditLogEntry struct {
	ID        string    `json:"id"`
	UserID    *string   `json:"user_id,omitempty"`
	Action    string    `json:"action"`
	Details   string    `json:"details,omitempty"`
	IPAddress *string   `json:"ip_address,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type RateLimitStatus struct {
	UserKey   string    `json:"user_key"`
	Limit     int       `json:"limit"`
	Remaining int       `json:"remaining"`
	ResetAt   time.Time `json:"reset_at"`
}
