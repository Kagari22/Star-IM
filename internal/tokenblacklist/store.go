package tokenblacklist

import (
	"context"
	"time"
)

type Store interface {
	Blacklist(ctx context.Context, token string, ttl time.Duration) error
	Contains(ctx context.Context, token string) (bool, error)
}
