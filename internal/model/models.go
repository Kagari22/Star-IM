package model

import "time"

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Nickname     string    `json:"nickname"`
	UnreadCount  int64     `json:"unread_count,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type Message struct {
	ID          int64     `json:"id"`
	FromUserID  int64     `json:"from_id"`
	ToUserID    int64     `json:"to_id"`
	ContentType string    `json:"content_type"`
	Content     string    `json:"content"`
	ObjectKey   string    `json:"object_key,omitempty"`
	ObjectURL   string    `json:"object_url,omitempty"`
	FileName    string    `json:"file_name,omitempty"`
	FileSize    int64     `json:"file_size,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}
