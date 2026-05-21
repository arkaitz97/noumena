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

type WorkspaceInvite struct {
	ID         string     `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	Email      string     `json:"email"`
	Role       string     `json:"role"`
	TokenHash  string     `json:"-"`
	ExpiresAt  time.Time  `json:"expires_at"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type WorkspaceWithMembers struct {
	Workspace
	MemberCount int    `json:"member_count"`
	UserRole    string `json:"user_role,omitempty"`
	IsOwner     bool   `json:"is_owner"`
}

type AuditLogEntry struct {
	ID        string     `json:"id"`
	UserID    *string    `json:"user_id,omitempty"`
	Action    string     `json:"action"`
	Details   string     `json:"details,omitempty"`
	IPAddress *string    `json:"ip_address,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type RateLimitStatus struct {
	UserKey   string    `json:"user_key"`
	Limit     int       `json:"limit"`
	Remaining int       `json:"remaining"`
	ResetAt   time.Time `json:"reset_at"`
}
