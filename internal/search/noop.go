package search

import (
	"context"

	"IM_Chat_System/internal/model"
)

type NoopIndexer struct{}

func (NoopIndexer) IndexMessage(context.Context, model.Message) error { return nil }
func (NoopIndexer) SearchMessages(context.Context, int64, string, int64, int) ([]model.Message, error) {
	return []model.Message{}, nil
}
func (NoopIndexer) Close() error { return nil }
