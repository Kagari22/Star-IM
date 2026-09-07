package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"IM_Chat_System/internal/model"
	"IM_Chat_System/internal/mq"

	"gorm.io/gorm"
)

type MessageRepository struct {
	db *gorm.DB
}

var ErrRecallNotAllowed = errors.New("message cannot be recalled")

func NewMessageRepository(db *gorm.DB) *MessageRepository {
	return &MessageRepository{db: db}
}

// SaveAndEnqueue 会在同一个事务中保存消息记录 (message row) 和对应的 Outbox 事件
func (r *MessageRepository) SaveAndEnqueue(ctx context.Context, message model.Message) (model.Message, error) {
	var saved model.Message

	var toUserID *int64
	if message.GroupID == nil {
		toUserID = &message.ToUserID
	}

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := messageRow{
			FromUserID:  message.FromUserID,
			ToUserID:    toUserID,
			GroupID:     message.GroupID,
			ContentType: message.ContentType,
			Content:     message.Content,
			ObjectKey:   message.ObjectKey,
			ObjectURL:   message.ObjectURL,
			FileName:    message.FileName,
			FileSize:    message.FileSize,
			ReplyToID:   message.ReplyToID,
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}

		// 读取数据库自动生成的字段, 例如 created_at
		if err := tx.First(&row, row.ID).Error; err != nil {
			return err
		}
		saved = row.model()

		payload, err := json.Marshal(mq.NewMessageCreatedEvent(saved))
		if err != nil {
			return err
		}
		if err := tx.Table("outbox_events").Create(map[string]any{
			"event_type":   "message.created",
			"message_id":   saved.ID,
			"payload":      payload,
			"available_at": gorm.Expr("NOW()"),
		}).Error; err != nil {
			return fmt.Errorf("enqueue message event: %w", err)
		}
		return nil
	})
	if err != nil {
		return model.Message{}, err
	}
	return saved, nil
}

// 查询两个用户之间的私聊消息, 并支持基于消息 ID 的增量分页
func (r *MessageRepository) ListConversation(ctx context.Context, userID, peerID, afterID int64, limit int) ([]model.Message, error) {
	var rows []messageRow
	if err := r.db.WithContext(ctx).
		Joins("LEFT JOIN user_message_hides ON user_message_hides.message_id = messages.id AND user_message_hides.user_id = ?", userID).
		Where("user_message_hides.message_id IS NULL").
		Where("id > ? AND ((from_user_id = ? AND to_user_id = ?) OR (from_user_id = ? AND to_user_id = ?))", afterID, userID, peerID, peerID, userID).
		Order("id ASC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return messageRowsToModels(rows), nil
}

// ListRecentConversation 返回会话最新的 limit 条消息（按时间升序）。
// ListConversation 的 ASC + Limit 只会取到会话最旧的前 limit 条，
// AI 上下文等需要"最近 N 条"的场景必须用本方法，否则模型永远看不到新消息。
func (r *MessageRepository) ListRecentConversation(ctx context.Context, userID, peerID int64, limit int) ([]model.Message, error) {
	var rows []messageRow
	if err := r.db.WithContext(ctx).
		Joins("LEFT JOIN user_message_hides ON user_message_hides.message_id = messages.id AND user_message_hides.user_id = ?", userID).
		Where("user_message_hides.message_id IS NULL").
		Where("((from_user_id = ? AND to_user_id = ?) OR (from_user_id = ? AND to_user_id = ?))", userID, peerID, peerID, userID).
		Order("id DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	return messageRowsToModels(rows), nil
}
func (r *MessageRepository) ListOffline(ctx context.Context, userID, afterID int64, limit int) ([]model.Message, error) {
	var rows []messageRow
	if err := r.db.WithContext(ctx).
		Where("to_user_id = ? AND id > ?", userID, afterID).
		Order("id ASC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return messageRowsToModels(rows), nil
}

// 把数据库层的 messageRow 切片转换成业务层的 model.Message 切片
func messageRowsToModels(rows []messageRow) []model.Message {
	messages := make([]model.Message, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, row.model())
	}
	return messages
}

// 查询指定群聊中的历史消息, 并支持增量分页
func (r *MessageRepository) ListGroupConversation(ctx context.Context, userID, groupID, afterID int64, limit int) ([]model.Message, error) {
	var rows []messageRow
	if err := r.db.WithContext(ctx).
		Joins("LEFT JOIN user_message_hides ON user_message_hides.message_id = messages.id AND user_message_hides.user_id = ?", userID).
		Where("user_message_hides.message_id IS NULL").
		Where("group_id = ? AND id > ?", groupID, afterID).
		Order("id ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return messageRowsToModels(rows), nil
}

// DeleteConversationMessages 删除会话中的指定消息
// 调用者必须已经是该会话的参与者;
// 首先删除相关的 outbox 记录, 以满足外键约束, 并防止过期消息被投递
func (r *MessageRepository) DeleteConversationMessages(ctx context.Context, userID int64, peerID, groupID *int64, messageIDs []int64, all bool) (int64, error) {
	if (peerID == nil) == (groupID == nil) {
		return 0, fmt.Errorf("exactly one conversation target is required")
	}
	if !all && len(messageIDs) == 0 {
		return 0, nil
	}

	var deleted int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		messages := tx.Model(&messageRow{})
		if peerID != nil {
			messages = messages.Where("group_id IS NULL AND ((from_user_id = ? AND to_user_id = ?) OR (from_user_id = ? AND to_user_id = ?))", userID, *peerID, *peerID, userID)
		} else {
			messages = messages.Where("group_id = ?", *groupID)
		}
		if !all {
			messages = messages.Where("id IN ?", messageIDs)
		}

		outcome := tx.Exec(
			"INSERT IGNORE INTO user_message_hides (user_id, message_id) SELECT ?, id FROM (?) AS selected_messages",
			userID, messages.Select("id"),
		)
		if outcome.Error != nil {
			return outcome.Error
		}
		deleted = outcome.RowsAffected
		return nil
	})
	return deleted, err
}

func (r *MessageRepository) Edit(ctx context.Context, messageID, requesterID int64, content string) (model.Message, error) {
	var row messageRow
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&messageRow{}).Where("id = ? AND from_user_id = ? AND recalled_at IS NULL AND content_type = ?", messageID, requesterID, "text").Updates(map[string]any{"content": content, "edited_at": time.Now()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("message cannot be edited")
		}
		if err := tx.First(&row, messageID).Error; err != nil {
			return err
		}

		payload, err := json.Marshal(mq.NewMessageCreatedEvent(row.model()))
		if err != nil {
			return err
		}
		if err := tx.Table("outbox_events").Create(map[string]any{
			"event_type":   "message.updated",
			"message_id":   row.ID,
			"payload":      payload,
			"available_at": gorm.Expr("NOW()"),
		}).Error; err != nil {
			return fmt.Errorf("enqueue message update event: %w", err)
		}
		return nil
	})
	if err != nil {
		return model.Message{}, err
	}
	return row.model(), nil
}

func (r *MessageRepository) SetFavorite(ctx context.Context, userID, messageID int64, favorite bool) error {
	if favorite {
		return r.db.WithContext(ctx).Exec("INSERT IGNORE INTO message_favorites (user_id, message_id) VALUES (?, ?)", userID, messageID).Error
	}
	return r.db.WithContext(ctx).Exec("DELETE FROM message_favorites WHERE user_id = ? AND message_id = ?", userID, messageID).Error
}

// ListFavorite 返回当前用户在指定会话（单聊或群聊）中收藏的消息，按收藏时间倒序。
func (r *MessageRepository) ListFavorite(ctx context.Context, userID int64, peerID, groupID *int64) ([]model.Message, error) {
	var rows []messageRow
	query := r.db.WithContext(ctx).
		Joins("JOIN message_favorites ON message_favorites.message_id = messages.id AND message_favorites.user_id = ?", userID).
		Order("message_favorites.created_at DESC")
	if peerID != nil {
		query = query.Where("messages.group_id IS NULL AND ((messages.from_user_id = ? AND messages.to_user_id = ?) OR (messages.from_user_id = ? AND messages.to_user_id = ?))", userID, *peerID, *peerID, userID)
	}
	if groupID != nil {
		query = query.Where("messages.group_id = ?", *groupID)
	}
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	return messageRowsToModels(rows), nil
}

func (r *MessageRepository) MarkRead(ctx context.Context, userID, peerID, groupID, messageID int64) error {
	if groupID > 0 {
		return r.db.WithContext(ctx).Exec("UPDATE group_members SET last_read_message_id = GREATEST(last_read_message_id, ?) WHERE group_id = ? AND user_id = ?", messageID, groupID, userID).Error
	}
	return r.db.WithContext(ctx).Exec("INSERT INTO conversation_reads (user_id, peer_id, last_read_message_id) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE last_read_message_id = GREATEST(last_read_message_id, VALUES(last_read_message_id))", userID, peerID, messageID).Error
}

// 撤回一条消息, 并把"消息已撤回"事件写入 outbox, 供后续异步通知使用
func (r *MessageRepository) Recall(ctx context.Context, messageID, requesterID int64, deadline time.Time) (model.Message, error) {
	if messageID <= 0 || requesterID <= 0 {
		return model.Message{}, ErrRecallNotAllowed
	}

	var result model.Message
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		recalledAt := time.Now()
		update := tx.Model(&messageRow{}).
			Where("id = ?", messageID).
			Where("from_user_id = ?", requesterID).
			Where("recalled_at IS NULL").
			Where("created_at >= ?", deadline).
			Updates(map[string]any{
				"recalled_at": recalledAt,
				"recalled_by": requesterID,
			})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return ErrRecallNotAllowed
		}

		var row messageRow
		if err := tx.First(&row, messageID).Error; err != nil {
			return err
		}
		result = row.model()

		payload, err := json.Marshal(mq.NewMessageRecalledEvent(result))
		if err != nil {
			return err
		}
		if err := tx.Table("outbox_events").Create(map[string]any{
			"event_type":   "message.recalled",
			"message_id":   result.ID,
			"payload":      payload,
			"available_at": gorm.Expr("NOW()"),
		}).Error; err != nil {
			return fmt.Errorf("enqueue recall event: %w", err)
		}
		return nil
	})
	if err != nil {
		return model.Message{}, err
	}
	return result, nil
}
