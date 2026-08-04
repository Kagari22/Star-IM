package redis

import (
	"context"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type Store struct {
	client *goredis.Client
}

// 对某个 Redis Key 进行原子计数, 并在第一次计数时设置过期时间
var allowScript = goredis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return count
`)

func New(client *goredis.Client) *Store {
	return &Store{client: client}
}

// 判断某个请求是否通过 Redis 限流
func (s *Store) Allow(ctx context.Context, key string, limit int64, window time.Duration) (bool, error) {
	count, err := allowScript.Run(ctx, s.client, []string{key}, window.Milliseconds()).Int64()
	if err != nil {
		return false, err
	}
	return count <= limit, nil
}
