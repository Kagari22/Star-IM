package outbox

import (
	"testing"
	"time"
)

func TestRetryDelayIsBounded(t *testing.T) {
	if got := retryDelay(1); got != time.Second {
		t.Fatalf("expected 1 second, got %s", got)
	}
	if got := retryDelay(100); got != 32*time.Second {
		t.Fatalf("expected capped 32 seconds, got %s", got)
	}
}
