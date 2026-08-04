package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"IM_Chat_System/internal/model"
	"IM_Chat_System/internal/mq"
)

type MessageRepository struct {
	db *sql.DB
}

func NewMessageRepository(db *sql.DB) *MessageRepository {
	return &MessageRepository{db: db}
}

// 保存聊天消息
// → 生成 message.created 事件
// → 写入 Outbox 表
// → 一次性提交
func (r *MessageRepository) SaveAndEnqueue(ctx context.Context, message model.Message) (model.Message, error) {
	tx, err := r.db.BeginTx(ctx, nil) // 开启数据库事务
	if err != nil {
		return model.Message{}, err
	}
	defer func() { _ = tx.Rollback() }() // 注册兜底回滚

	// 将消息写入 messages 表
	result, err := tx.ExecContext(
		ctx,
		`INSERT INTO messages (from_user_id, to_user_id, content_type, content, object_key, object_url, file_name, file_size) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		message.FromUserID,
		message.ToUserID,
		message.ContentType,
		message.Content,
		message.ObjectKey,
		message.ObjectURL,
		message.FileName,
		message.FileSize,
	)
	if err != nil {
		return model.Message{}, err
	}

	// 获取刚刚插入的消息 ID, 用于后续查询和生成事件
	id, err := result.LastInsertId()
	if err != nil {
		return model.Message{}, err
	}

	// 在同一个事务中重新查询完整消息, 获得数据库生成的字段
	saved := model.Message{}
	err = tx.QueryRowContext(
		ctx,
		`SELECT id, from_user_id, to_user_id, content_type, content, object_key, object_url, file_name, file_size, created_at FROM messages WHERE id = ?`,
		id,
	).Scan(&saved.ID, &saved.FromUserID, &saved.ToUserID, &saved.ContentType, &saved.Content, &saved.ObjectKey, &saved.ObjectURL, &saved.FileName, &saved.FileSize, &saved.CreatedAt)
	if err != nil {
		return model.Message{}, err
	}

	payload, err := json.Marshal(mq.NewMessageCreatedEvent(saved))
	if err != nil {
		return model.Message{}, err
	}

	// 向 Outbox 表写入待发布事件
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO outbox_events (event_type, message_id, payload, available_at) VALUES (?, ?, ?, NOW())`,
		"message.created", saved.ID, payload,
	); err != nil {
		return model.Message{}, fmt.Errorf("enqueue message event: %w", err)
	}

	// 提交事务
	if err := tx.Commit(); err != nil {
		return model.Message{}, err
	}
	return saved, nil
}

// 查询当前用户与指定用户之间的聊天记录, 支持基于消息 ID 的增量分页
func (r *MessageRepository) ListConversation(ctx context.Context, userID, peerID, afterID int64, limit int) ([]model.Message, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, from_user_id, to_user_id, content_type, content, object_key, object_url, file_name, file_size, created_at
		 FROM messages
		 WHERE id > ?
		   AND ((from_user_id = ? AND to_user_id = ?) OR (from_user_id = ? AND to_user_id = ?))
		 ORDER BY id ASC
		 LIMIT ?`,
		afterID, userID, peerID, peerID, userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []model.Message
	for rows.Next() {
		var message model.Message
		if err := rows.Scan(&message.ID, &message.FromUserID, &message.ToUserID, &message.ContentType, &message.Content, &message.ObjectKey, &message.ObjectURL, &message.FileName, &message.FileSize, &message.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

// 查询当前用户离线期间收到的消息, 支持基于消息 ID 的增量同步
func (r *MessageRepository) ListOffline(ctx context.Context, userID, afterID int64, limit int) ([]model.Message, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, from_user_id, to_user_id, content_type, content, object_key, object_url, file_name, file_size, created_at
		 FROM messages
		 WHERE to_user_id = ? AND id > ?
		 ORDER BY id ASC
		 LIMIT ?`,
		userID, afterID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []model.Message
	// 逐行读取数据库结果, 扫描成 model.Message, 加入返回切片
	for rows.Next() {
		var message model.Message
		if err := rows.Scan(&message.ID, &message.FromUserID, &message.ToUserID, &message.ContentType, &message.Content, &message.ObjectKey, &message.ObjectURL, &message.FileName, &message.FileSize, &message.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}
