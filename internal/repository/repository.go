package repository

import (
	"context"

	"IM_Chat_System/internal/model"
)

type UserRepository interface {
	Create(ctx context.Context, username, passwordHash, nickname string) (model.User, error)
	GetByID(ctx context.Context, id int64) (model.User, bool, error)
	GetByUsername(ctx context.Context, username string) (model.User, bool, error)
	List(ctx context.Context, excludeUserID int64) ([]model.User, error)
}

type MessageRepository interface {
	SaveAndEnqueue(ctx context.Context, message model.Message) (model.Message, error)
	ListConversation(ctx context.Context, userID, peerID, afterID int64, limit int) ([]model.Message, error)
	ListOffline(ctx context.Context, userID, afterID int64, limit int) ([]model.Message, error)
}
