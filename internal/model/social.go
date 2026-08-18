package model

import "time"

type RequestStatus string

const (
	RequestPending  RequestStatus = "pending"
	RequestAccepted RequestStatus = "accepted"
	RequestRejected RequestStatus = "rejected"
)

type FriendRequest struct {
	ID          int64         `json:"id"`
	FromUserID  int64         `json:"from_user_id"`
	ToUserID    int64         `json:"to_user_id"`
	FromUsername string       `json:"from_username,omitempty"`
	FromNickname string       `json:"from_nickname,omitempty"`
	Status      RequestStatus `json:"status"`
	CreatedAt   time.Time     `json:"created_at"`
	RespondedAt *time.Time    `json:"responded_at,omitempty"`
}

type GroupJoinRequest struct {
	ID          int64         `json:"id"`
	GroupID     int64         `json:"group_id"`
	UserID      int64         `json:"user_id"`
	GroupName   string        `json:"group_name,omitempty"`
	Username    string        `json:"username,omitempty"`
	Nickname    string        `json:"nickname,omitempty"`
	Status      RequestStatus `json:"status"`
	CreatedAt   time.Time     `json:"created_at"`
	RespondedAt *time.Time    `json:"responded_at,omitempty"`
}
