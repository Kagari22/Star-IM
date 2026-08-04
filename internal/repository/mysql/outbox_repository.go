package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"IM_Chat_System/internal/outbox"
)

type OutboxRepository struct {
	db *sql.DB
}

func NewOutboxRepository(db *sql.DB) *OutboxRepository {
	return &OutboxRepository{db: db}
}

// 用于从 outbox_events 表中抢占一批待发布事件, 避免多个 Dispatcher 实例重复处理同一条消息
// 开启事务
// → 查询符合条件的未发布事件
// → 对事件加数据库行锁
// → 设置 locked_until 和 attempts
// → 提交事务
// → 返回本实例抢到的事件
func (r *OutboxRepository) Claim(ctx context.Context, limit int, lockFor time.Duration) ([]outbox.Event, error) {
	if limit <= 0 {
		return nil, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
		SELECT id, payload, attempts
		FROM outbox_events
		WHERE published_at IS NULL
		  AND available_at <= NOW()
		  AND (locked_until IS NULL OR locked_until < NOW())
		ORDER BY id ASC
		LIMIT ? FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]outbox.Event, 0, limit)
	for rows.Next() {
		var event outbox.Event
		if err := rows.Scan(&event.ID, &event.Payload, &event.Attempts); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, event := range events {
		if _, err := tx.ExecContext(ctx, `UPDATE outbox_events SET locked_until = DATE_ADD(NOW(), INTERVAL ? MICROSECOND), attempts = attempts + 1 WHERE id = ?`, lockFor.Microseconds(), event.ID); err != nil { 
			return nil, fmt.Errorf("lock outbox event %d: %w", event.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return events, nil
}

// 事件已经成功发布到 RabbitMQ
func (r *OutboxRepository) MarkPublished(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE outbox_events SET published_at = NOW(), locked_until = NULL WHERE id = ?`, id)
	return err
}

// 表示事件本次处理失败, 需要延迟重试
func (r *OutboxRepository) MarkFailed(ctx context.Context, id int64, retryAfter time.Duration) error {
	_, err := r.db.ExecContext(ctx, `UPDATE outbox_events SET locked_until = NULL, available_at = DATE_ADD(NOW(), INTERVAL ? MICROSECOND) WHERE id = ?`, retryAfter.Microseconds(), id)
	return err
}
