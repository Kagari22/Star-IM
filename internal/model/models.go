package model

import "time"

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Nickname     string    `json:"nickname"`
	AvatarKey    string    `json:"-"`
	AvatarURL    string    `json:"avatar_url,omitempty"`
	UnreadCount  int64     `json:"unread_count,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type Message struct {
	ID           int64      `json:"id"`
	FromUserID   int64      `json:"from_id"`
	ToUserID     int64      `json:"to_id"`
	ContentType  string     `json:"content_type"`
	Content      string     `json:"content"`
	ObjectKey    string     `json:"object_key,omitempty"`
	ObjectURL    string     `json:"object_url,omitempty"`
	FileName     string     `json:"file_name,omitempty"`
	FileSize     int64      `json:"file_size,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	GroupID      *int64     `json:"group_id,omitempty"`
	RecalledAt   *time.Time `json:"recalled_at,omitempty"`
	RecalledBy   int64      `json:"recalled_by,omitempty"`
	RecallReason string     `json:"recall_reason,omitempty"`
	ReplyToID    *int64     `json:"reply_to_id,omitempty"`
	ReplyPreview string     `json:"reply_preview,omitempty"`
	EditedAt     *time.Time `json:"edited_at,omitempty"`
	IsFavorite   bool       `json:"is_favorite,omitempty"`
	IsPinned     bool       `json:"is_pinned,omitempty"`
}
