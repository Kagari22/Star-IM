package search

import (
	"context"

	"IM_Chat_System/internal/model"
)

type Indexer interface {
	IndexMessage(ctx context.Context, message model.Message) error
	SearchMessages(ctx context.Context, userID int64, query string, peerID int64, limit int) ([]model.Message, error)
	Close() error
}
