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
	MessageID   int64     `json:"message_id"`
	FromUserID  int64     `json:"from_user_id"`
	ToUserID    int64     `json:"to_user_id"`
	ContentType string    `json:"content_type"`
	Content     string    `json:"content"`
	ObjectKey   string    `json:"object_key"`
	ObjectURL   string    `json:"object_url"`
	FileName    string    `json:"file_name"`
	FileSize    int64     `json:"file_size"`
	CreatedAt   time.Time `json:"created_at"`
}

func NewMessageCreatedEvent(message model.Message) MessageCreatedEvent {
	return MessageCreatedEvent{
		MessageID:   message.ID,
		FromUserID:  message.FromUserID,
		ToUserID:    message.ToUserID,
		ContentType: message.ContentType,
		Content:     message.Content,
		ObjectKey:   message.ObjectKey,
		ObjectURL:   message.ObjectURL,
		FileName:    message.FileName,
		FileSize:    message.FileSize,
		CreatedAt:   message.CreatedAt,
	}
}

type EventPublisher interface {
	PublishMessageCreated(ctx context.Context, event MessageCreatedEvent) error
	Close() error
}

type MessageCreatedHandler func(ctx context.Context, event MessageCreatedEvent) error
