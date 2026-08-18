package model

import "time"

type GroupRole string

const (
	GroupRoleOwner  GroupRole = "owner"
	GroupRoleAdmin  GroupRole = "admin"
	GroupRoleMember GroupRole = "member"
)

type Group struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	OwnerID      int64     `json:"owner_id"`
	AvatarKey    string    `json:"-"`
	AvatarURL    string    `json:"avatar_url,omitempty"`
	Announcement string    `json:"announcement,omitempty"`
	AllMuted     bool      `json:"all_muted,omitempty"`
	InviteCode   string    `json:"invite_code,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type GroupMember struct {
	GroupID           int64      `json:"group_id"`
	UserID            int64      `json:"user_id"`
	Role              GroupRole  `json:"role"`
	JoinedAt          time.Time  `json:"joined_at"`
	LastReadMessageID int64      `json:"last_read_message_id"`
	MutedUntil        *time.Time `json:"muted_until,omitempty"`
}
