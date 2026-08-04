package mq

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

type RabbitPublisher struct {
	conn          *amqp.Connection // RabbitMQ 的底层连接, 负责维护客户端与 Broker 之间的连接
	channel       *amqp.Channel // AMQP Channel, 实际执行消息发布和交换机声明等操作
	confirmations <-chan amqp.Confirmation // RabbitMQ 的底层连接, 负责维护客户端与 Broker 之间的连接
}

// 创建 RabbitMQ 消息发布器, 完成连接、信道、交换机和发布确认机制的初始化
func NewRabbitPublisher(url string) (*RabbitPublisher, error) {
	// 通过 AMQP 地址连接 RabbitMQ, 建立底层 TCP 连接
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, err
	}

	// 在 TCP 连接上创建 AMQP Channel
	// RabbitMQ 通常通过 Channel 执行发布、消费、声明交换机等操作
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	// 声明项目使用的交换机
	if err := declareExchange(ch); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, err
	}
	// 开启 RabbitMQ Publisher Confirm 发布确认模式
	if err := ch.Confirm(false); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, err
	}

	// conn: RabbitMQ 连接
	// channel: 用于发布消息的 Channel
	// confirmations: 接收 RabbitMQ 发布确认结果的 Channel
	// make(chan amqp.Confirmation, 1): 创建带缓冲的确认通道，接收 Ack 或 Nack
	return &RabbitPublisher{conn: conn, channel: ch, confirmations: ch.NotifyPublish(make(chan amqp.Confirmation, 1))}, nil
}

// 把 message.created 事件可靠地发布到 RabbitMQ, 并等待 Broker 返回确认结果
func (p *RabbitPublisher) PublishMessageCreated(ctx context.Context, event MessageCreatedEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}

	// 向 RabbitMQ 发布消息
	if err := p.channel.PublishWithContext(
		ctx,
		ExchangeChatEvents,
		RoutingKeyMessageCreated,
		false,
		false,
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			Body:         body,
		},
	); err != nil {
		return err
	}

	select {
	// 从 RabbitMQ 的确认通道读取 Broker 返回的确认结果
	case confirmation, ok := <-p.confirmations:
		if !ok {
			return fmt.Errorf("publisher confirmation channel closed")
		}
		if !confirmation.Ack {
			return fmt.Errorf("broker negatively acknowledged message")
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *RabbitPublisher) Close() error {
	if p.channel != nil {
		_ = p.channel.Close()
	}
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}

type MessageCreatedConsumer struct {
	conn       *amqp.Connection
	channel    *amqp.Channel
	nodeQueue  string
	indexQueue string
}

// 创建 message.created 事件消费者，完成 RabbitMQ 连接、Channel 和消费队列拓扑的初始化
func NewMessageCreatedConsumer(url, nodeID string) (*MessageCreatedConsumer, error) {
	conn, err := amqp.Dial(url) // 连接 RabbitMQ
	if err != nil {
		return nil, err
	}

	// 在连接上创建 AMQP Channel, 后续声明队列、绑定交换机和消费消息都通过 Channel 完成
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	// 声明消费者所需的 RabbitMQ 拓扑结构
	// node -> 当前节点的实时推送队列
	// indexQueue -> Elasticsearch 消息索引队列
	nodeQueue, indexQueue, err := declareConsumerTopology(ch, nodeID)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, err
	}

	return &MessageCreatedConsumer{conn: conn, channel: ch, nodeQueue: nodeQueue, indexQueue: indexQueue}, nil
}

func (c *MessageCreatedConsumer) Start(ctx context.Context, nodeHandler, indexHandler MessageCreatedHandler) error {
	// 监听当前节点专属的 nodeQueue 队列
	if err := c.consume(ctx, c.nodeQueue, nodeHandler); err != nil {
		return err
	}
	// 监听 Elasticsearch 使用的 indexQueue 队列
	return c.consume(ctx, c.indexQueue, indexHandler)
}

// 监听 RabbitMQ 指定队列, 解析消息, 调用业务处理器, 并根据处理结果执行 Ack 或 Nack
// 监听队列
// → 接收 Delivery
// → JSON 反序列化为事件
// → 调用业务 Handler
// → 成功 Ack
// → 解析失败丢弃
// → 业务失败重新入队
func (c *MessageCreatedConsumer) consume(ctx context.Context, queue string, handler MessageCreatedHandler) error {
	deliveries, err := c.channel.Consume( // 注册 RabbitMQ 消费者
		queue,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	// 解析失败 → Nack + 不重试
	// 业务失败 → Nack + 重新入队
	// 业务成功 → Ack + 确认消费
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case delivery, ok := <-deliveries: // 从 RabbitMQ 获取一条消息
				if !ok {
					return
				}

				var event MessageCreatedEvent
				if err := json.Unmarshal(delivery.Body, &event); err != nil {
					log.Println("rabbitmq unmarshal message created:", err)
					_ = delivery.Nack(false, false)
					continue
				}

				if handler != nil {
					// 如果配置了业务处理器, 就调用它
					if err := handler(ctx, event); err != nil {
						log.Println("rabbitmq handle message created:", err)
						_ = delivery.Nack(false, true) // // 如果业务处理失败, 不批量拒绝, requeue=true, 重新放回队列
						continue
					}
				}
				log.Printf("rabbitmq message.created consumed: message=%d from=%d to=%d\n", event.MessageID, event.FromUserID, event.ToUserID)
				_ = delivery.Ack(false) // 业务处理成功后确认消息
			}
		}
	}()

	return nil
}

func (c *MessageCreatedConsumer) Close() error {
	if c.channel != nil {
		_ = c.channel.Close()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// 声明聊天事件交换机
func declareExchange(ch *amqp.Channel) error {
	return ch.ExchangeDeclare(
		ExchangeChatEvents,
		"topic",
		true,
		false,
		false,
		false,
		nil,
	)
}

// 声明 RabbitMQ 消费端所需的完整拓扑, 包括交换机、节点实时推送队列、消息索引队列及路由绑定
func declareConsumerTopology(ch *amqp.Channel, nodeID string) (string, string, error) {
	if err := declareExchange(ch); err != nil {
		return "", "", err
	}
	if nodeID == "" {
		return "", "", fmt.Errorf("node ID is required for message consumer")
	}
	nodeQueue := "chat.message.created.node." + nodeID
	if _, err := ch.QueueDeclare(
		nodeQueue,
		false,
		true,
		false,
		false,
		nil,
	); err != nil {
		return "", "", err
	}
	if err := ch.QueueBind(nodeQueue, RoutingKeyMessageCreated, ExchangeChatEvents, false, nil); err != nil {
		return "", "", err
	}
	if _, err := ch.QueueDeclare(
		QueueMessageIndex,
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return "", "", err
	}
	if err := ch.QueueBind(
		QueueMessageIndex,
		RoutingKeyMessageCreated,
		ExchangeChatEvents,
		false,
		nil,
	); err != nil {
		return "", "", err
	}
	return nodeQueue, QueueMessageIndex, nil
}
