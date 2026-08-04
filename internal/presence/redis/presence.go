package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const onlineTTL = 60 * time.Second

type PresenceStore struct {
	client *goredis.Client
}

func New(client *goredis.Client) *PresenceStore {
	return &PresenceStore{client: client}
}

// 在 Redis 中记录用户在线状态以及用户所在的应用节点
func (s *PresenceStore) SetOnline(ctx context.Context, userID int64, node string) error {
	return s.client.Set(ctx, onlineKey(userID), node, onlineTTL).Err()
}

// 查询指定用户是否在线, 以及该用户连接在哪个服务节点
func (s *PresenceStore) GetOnlineNode(ctx context.Context, userID int64) (string, bool, error) {
	// 根据用户 ID 生成 Redis Key，并读取在线记录
	value, err := s.client.Get(ctx, onlineKey(userID)).Result()
	if err == goredis.Nil {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

// 删除 Redis 中指定用户的在线状态记录, 将用户标记为离线
func (s *PresenceStore) SetOffline(ctx context.Context, userID int64) error {
	return s.client.Del(ctx, onlineKey(userID)).Err()
}

func onlineKey(userID int64) string {
	return fmt.Sprintf("online:user:%d", userID)
}
