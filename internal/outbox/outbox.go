package outbox

import (
	"context"
	"time"
)

type Event struct {
	ID       int64
	Payload  []byte
	Attempts int
}

type Repository interface {
	Claim(ctx context.Context, limit int, lockFor time.Duration) ([]Event, error)
	MarkPublished(ctx context.Context, id int64) error
	MarkFailed(ctx context.Context, id int64, retryAfter time.Duration) error
}
