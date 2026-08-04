package outbox

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"IM_Chat_System/internal/mq"
)

type Dispatcher struct {
	repository Repository // Outbox 仓储接口, 负责操作数据库中的 outbox_events 表
	publisher  mq.EventPublisher // 事件发布器接口, 负责将 Outbox 事件发送到消息队列
	batchSize  int // 每次最多处理多少条 Outbox 事件
	lockFor    time.Duration // 事件锁定时间
	interval   time.Duration // 调度器轮询间隔
}

func NewDispatcher(repository Repository, publisher mq.EventPublisher) *Dispatcher {
	return &Dispatcher{
		repository: repository,
		publisher:  publisher,
		batchSize:  50,
		lockFor:    30 * time.Second,
		interval:   time.Second,
	}
}

// 让 Outbox Dispatcher 持续、定时地投递待发布事件
func (d *Dispatcher) Run(ctx context.Context) {
	d.DispatchOnce(ctx)
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.DispatchOnce(ctx)
		}
	}
}

// 从数据库抢占待发布事件
// → 逐条解析事件
// → 发布到 RabbitMQ
// → 成功后标记已发布
// → 失败后记录重试时间
func (d *Dispatcher) DispatchOnce(ctx context.Context) {
	// 从 outbox_events 表中抢占一批待处理事件
	events, err := d.repository.Claim(ctx, d.batchSize, d.lockFor)
	if err != nil {
		log.Printf("outbox claim: %v", err)
		return
	}
	for _, event := range events {
		var message mq.MessageCreatedEvent
		// 将数据库中保存的 JSON payload 解析成 RabbitMQ 要发布的事件对象
		if err := json.Unmarshal(event.Payload, &message); err != nil {
			log.Printf("outbox decode event %d: %v", event.ID, err)
			// 把当前处理失败的 Outbox 事件标记为"失败待重试"
			_ = d.repository.MarkFailed(ctx, event.ID, retryDelay(event.Attempts))
			continue
		}
		// 调用 Publisher 将事件发送到 RabbitMQ
		if err := d.publisher.PublishMessageCreated(ctx, message); err != nil {
			log.Printf("outbox publish event %d: %v", event.ID, err)
			// 把当前处理失败的 Outbox 事件标记为"失败待重试"
			_ = d.repository.MarkFailed(ctx, event.ID, retryDelay(event.Attempts))
			continue
		}
		// RabbitMQ 确认成功后，将 Outbox 事件标记为已发布
		if err := d.repository.MarkPublished(ctx, event.ID); err != nil {
			log.Printf("outbox mark published event %d: %v", event.ID, err)
		}
	}
}

// 计算 Outbox 事件失败后的下一次重试间隔, 采用指数退避策略
func retryDelay(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	if attempts > 6 {
		attempts = 6
	}
	return time.Second * time.Duration(1<<uint(attempts-1))
}
