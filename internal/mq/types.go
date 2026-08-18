package mq

import (
	"context"
	"time"

	"IM_Chat_System/internal/model"
)

const (
	ExchangeChatEvents       = "chat.events"
	RoutingKeyMessageCreated = "chat.message.created"
	QueueMessageIndex        = "chat.message.created.index"
)

type MessageCreatedEvent struct {
	MessageID    int64      `json:"message_id"`
	FromUserID   int64      `json:"from_user_id"`
	ToUserID     int64      `json:"to_user_id"`
	GroupID      *int64     `json:"group_id,omitempty"`
	ContentType  string     `json:"content_type"`
	Content      string     `json:"content"`
	ObjectKey    string     `json:"object_key"`
	ObjectURL    string     `json:"object_url"`
	FileName     string     `json:"file_name"`
	FileSize     int64      `json:"file_size"`
	CreatedAt    time.Time  `json:"created_at"`
	RecalledAt   *time.Time `json:"recalled_at,omitempty"`
	RecalledBy   int64      `json:"recalled_by,omitempty"`
	RecallReason string     `json:"recall_reason,omitempty"`
	ReplyToID    *int64     `json:"reply_to_id,omitempty"`
	EditedAt     *time.Time `json:"edited_at,omitempty"`
}

func NewMessageRecalledEvent(message model.Message) MessageCreatedEvent {
	return MessageCreatedEvent{
		// Do not republish recalled text or media: clients only need recall state.
		MessageID: message.ID, FromUserID: message.FromUserID, ToUserID: message.ToUserID,
		GroupID: message.GroupID, CreatedAt: message.CreatedAt, RecalledAt: message.RecalledAt,
		RecalledBy: message.RecalledBy, RecallReason: message.RecallReason,
		ReplyToID: message.ReplyToID, EditedAt: message.EditedAt,
	}
}

func NewMessageCreatedEvent(message model.Message) MessageCreatedEvent {
	return MessageCreatedEvent{
		MessageID:   message.ID,
		FromUserID:  message.FromUserID,
		ToUserID:    message.ToUserID,
		GroupID:     message.GroupID,
		ContentType: message.ContentType,
		Content:     message.Content,
		ObjectKey:   message.ObjectKey,
		ObjectURL:   message.ObjectURL,
		FileName:    message.FileName,
		FileSize:    message.FileSize,
		CreatedAt:   message.CreatedAt,
		ReplyToID:   message.ReplyToID,
		EditedAt:    message.EditedAt,
	}
}

type EventPublisher interface {
	PublishMessageCreated(ctx context.Context, event MessageCreatedEvent) error
	Close() error
}

type MessageCreatedHandler func(ctx context.Context, event MessageCreatedEvent) error
