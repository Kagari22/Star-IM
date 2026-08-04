package redis

import (
	"context"
	"fmt"
	"strconv"

	goredis "github.com/redis/go-redis/v9"
)

type Store struct {
	client *goredis.Client
}

func New(client *goredis.Client) *Store {
	return &Store{client: client}
}

// 记录一条未读消息，并返回当前会话未读总数
func (s *Store) Increment(ctx context.Context, userID, peerID, messageID int64) (int64, error) {
	key := conversationKey(userID, peerID)
	if err := s.client.ZAdd(ctx, key, goredis.Z{Score: float64(messageID), Member: strconv.FormatInt(messageID, 10)}).Err(); err != nil {
		return 0, err
	}
	return s.client.ZCard(ctx, key).Result()
}

// 清除当前用户与指定用户之间, 消息 ID 小于等于 throughMessageID 的未读消息
func (s *Store) ClearConversation(ctx context.Context, userID, peerID, throughMessageID int64) error {
	if throughMessageID <= 0 {
		return nil
	}
	return s.client.ZRemRangeByScore(ctx, conversationKey(userID, peerID), "-inf", strconv.FormatInt(throughMessageID, 10)).Err()
}

// 批量获取当前用户与多个用户会话的未读数, 用于联系人列表显示未读角标
func (s *Store) GetConversationCounts(ctx context.Context, userID int64, peerIDs []int64) (map[int64]int64, error) {
	result := make(map[int64]int64, len(peerIDs))
	if len(peerIDs) == 0 {
		return result, nil
	}

	pipe := s.client.Pipeline()
	cmds := make(map[int64]*goredis.IntCmd, len(peerIDs))
	for _, peerID := range peerIDs {
		cmds[peerID] = pipe.ZCard(ctx, conversationKey(userID, peerID))
	}
	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}

	for peerID, cmd := range cmds {
		value, cmdErr := cmd.Result()
		if cmdErr != nil {
			return nil, cmdErr
		}
		result[peerID] = value
	}
	return result, nil
}

func conversationKey(userID, peerID int64) string {
	return fmt.Sprintf("unread:v2:user:%d:peer:%d", userID, peerID)
}
