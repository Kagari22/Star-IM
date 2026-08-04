package presence

import "context"

type Store interface {
	SetOnline(ctx context.Context, userID int64, node string) error
	GetOnlineNode(ctx context.Context, userID int64) (string, bool, error)
	SetOffline(ctx context.Context, userID int64) error
}
