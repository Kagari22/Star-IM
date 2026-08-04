package unread

import "context"

type Store interface {
	Increment(ctx context.Context, userID, peerID, messageID int64) (int64, error)
	ClearConversation(ctx context.Context, userID, peerID, throughMessageID int64) error
	GetConversationCounts(ctx context.Context, userID int64, peerIDs []int64) (map[int64]int64, error)
}
