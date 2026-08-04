package ratelimit

import (
	"context"
	"time"
)

type Store interface {
	Allow(ctx context.Context, key string, limit int64, window time.Duration) (bool, error)
}
