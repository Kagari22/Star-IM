package chat

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"IM_Chat_System/internal/auth"
	"IM_Chat_System/internal/model"
	"IM_Chat_System/internal/mq"
	"IM_Chat_System/internal/presence"
	"IM_Chat_System/internal/ratelimit"
	"IM_Chat_System/internal/service"
	"IM_Chat_System/internal/tokenblacklist"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 70 * time.Second
	pingPeriod = 30 * time.Second
)

type Hub struct {
	mu        sync.RWMutex            // 保护 clients 这张 map。因为多个 WebSocket goroutine 会同时连接、断开、查找用户，普通 map 并发读写会崩溃。
	clients   map[int64]*Client       // 当前节点的 WebSocket 客户端连接表
	secret    string                  // JWT 密钥, WebSocket 握手时也需要验证身份
	nodeID    string                  // 当前服务节点标识
	messages  *service.MessageService // 消息业务服务。收到 WebSocket 消息后，调用它校验、持久化到 MySQL、更新未读数等。
	presence  presence.Store          // 在线状态存储，一般使用 Redis。用于记录或查询"用户在线、在哪个节点在线"。
	blacklist tokenblacklist.Store    // Token 黑名单存储。用户登出后，JWT 即使尚未过期，也能拒绝其 WebSocket 连接或后续消息。
	limiter   ratelimit.Store         // 消息发送限流，例如限制一个用户每分钟发送的消息数，避免刷屏或恶意消耗资源。
	origins   map[string]struct{}     // WebSocket 允许来源白名单
}

type Client struct {
	userID int64
	conn   *websocket.Conn // Gorilla WebSocket 的实际连接
	send   chan any        // 服务端要推送给该客户端的消息队列
	hub    *Hub            // 指回连接中心
}

// 浏览器 → 服务端
type IncomingMessage struct {
	Type      string `json:"type"`
	To        int64  `json:"to"`
	Content   string `json:"content"`
	GroupID   *int64 `json:"group_id,omitempty"`
	ReplyToID *int64 `json:"reply_to_id,omitempty"`
}

// 服务端 → 浏览器
type OutgoingMessage struct {
	Type    string        `json:"type"`
	Message model.Message `json:"message,omitempty"`
	To      int64         `json:"to,omitempty"`
	GroupID *int64        `json:"group_id,omitempty"`
	Error   string        `json:"error,omitempty"`
}

func NewHub(messages *service.MessageService, secret, nodeID string, presenceStore presence.Store, blacklist tokenblacklist.Store, limiter ratelimit.Store, allowedOrigins []string) *Hub {
	origins := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		origins[origin] = struct{}{}
	}
	return &Hub{
		clients:   make(map[int64]*Client),
		secret:    secret,
		nodeID:    nodeID,
		messages:  messages,
		presence:  presenceStore,
		blacklist: blacklist,
		limiter:   limiter,
		origins:   origins,
	}
}

func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	token := websocketToken(r)
	if token == "" {
		http.Error(w, "missing websocket token", http.StatusUnauthorized)
		return
	}
	if h.blacklist != nil {
		blocked, err := h.blacklist.Contains(r.Context(), token) // 检查 Token 是否已登出/撤销
		if err != nil {
			http.Error(w, "check token blacklist failed", http.StatusInternalServerError)
			return
		}
		if blocked { // 说明 Token 已被拉黑
			http.Error(w, "token has been revoked", http.StatusUnauthorized)
			return
		}
	}
	// 校验 JWT
	claims, err := auth.ParseToken(h.secret, token)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	// 配置升级器
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		Subprotocols:    []string{"im-chat"},
		CheckOrigin:     h.isOriginAllowed,
	}
	// 从 HTTP 升级为 WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("websocket upgrade:", err)
		return
	}
	// 创建当前用户的连接对象
	client := &Client{
		userID: claims.UserID,
		conn:   conn,
		send:   make(chan any, 16),
		hub:    h,
	}

	// 注册并启动读写协程
	h.register(client)
	go client.writePump()
	go client.readPump()
}

// 从 WebSocket 握手请求的 Sec-WebSocket-Protocol 头中提取身份验证令牌
func websocketToken(r *http.Request) string {
	protocols := websocket.Subprotocols(r)
	if len(protocols) != 2 || protocols[0] != "im-chat" {
		return ""
	}
	return protocols[1]
}

// WebSocket 握手时的来源校验函数, 用来阻止不受信任的网站建立连接
func (h *Hub) isOriginAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	parsed, err := url.Parse(origin)
	// 协议标识符和主机不为空
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	// 检查它是否存在于 NewHub 创建的白名单集合 h.origins 中
	_, ok := h.origins[parsed.Scheme+"://"+parsed.Host]
	return ok
}

// 把新 WebSocket 客户端登记到 Hub, 并记录其在线状态
func (h *Hub) register(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// 从 clients 中查找该用户是否已有旧连接
	if old := h.clients[client.userID]; old != nil {
		// 如果存在旧连接，主动关闭它
		// 这样该用户从新设备登录或刷新页面重新建立 WebSocket 时, 旧连接会断开。
		old.conn.Close()
	}
	h.clients[client.userID] = client // 将新连接写入 Hub
	if h.presence != nil {
		// 向在线状态存储写入类似: 用户 42 在线，连接在 node-1
		if err := h.presence.SetOnline(context.Background(), client.userID, h.nodeID); err != nil {
			log.Println("presence set online:", err)
		}
	}
	log.Printf("user %d online\n", client.userID)
}

// 用于客户端断开时, 从 Hub 和在线状态存储中注销它
func (h *Hub) unregister(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.clients[client.userID] == client {
		delete(h.clients, client.userID)
		// 如果存在在线状态存储(Redis), 再执行离线标记
		if h.presence != nil {
			if err := h.presence.SetOffline(context.Background(), client.userID); err != nil {
				log.Println("presence set offline:", err)
			}
		}
		log.Printf("user %d offline\n", client.userID)
	}
}

// 向指定用户的 WebSocket 发送队列投递一条消息; 若用户不在线或客户端太慢，则返回 false
func (h *Hub) deliver(userID int64, payload any) bool {
	h.mu.RLock()
	client := h.clients[userID]
	h.mu.RUnlock()

	if client == nil {
		return false
	}

	select {
	case client.send <- payload: // 尝试将消息放入 client.send 缓冲队列
		return true
	default:
		client.conn.Close()
		return false
	}
}

// 浏览器 → 服务端
// 读取聊天消息、限流、保存消息
// 持续读取浏览器发来的 JSON 消息, 校验、限流、保存, 并向发送者回传确认或错误
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister(c)
		c.conn.Close()
	}()

	c.conn.SetReadLimit(4096)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	// 收到 Pong → 再延长 pongWait 时间
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		var input IncomingMessage
		if err := c.conn.ReadJSON(&input); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Println("websocket read:", err)
			}
			return
		}

		if input.Type == "typing" {
			c.hub.dispatchTyping(c, input)
			continue
		}
		if input.Type != "chat" {
			c.send <- OutgoingMessage{Type: "error", Error: "unsupported message type"}
			continue
		}
		// 如果配置了 Redis 限流器, 就对消息发送执行限流
		if c.hub.limiter != nil {
			// 对当前用户执行"一分钟最多 20 条消息"的限制
			allowed, err := c.hub.limiter.Allow(context.Background(), messageRateKey(c.userID), 20, time.Minute)
			if err != nil {
				c.send <- OutgoingMessage{Type: "error", Error: "rate limit check failed"} // 限流存储出错
				continue
			}
			if !allowed {
				c.send <- OutgoingMessage{Type: "error", Error: "too many messages, slow down"} // 超过每分钟上限
				continue
			}
		}

		var (
			message model.Message
			err     error
		)

		if input.GroupID != nil {
			message, err = c.hub.messages.SaveGroupTextReply(
				context.Background(),
				c.userID,
				*input.GroupID,
				input.Content,
				input.ReplyToID,
			)
		} else {
			message, err = c.hub.messages.SaveTextReply(
				context.Background(),
				c.userID,
				input.To,
				input.Content,
				input.ReplyToID,
			)
		}

		if err != nil {
			c.send <- OutgoingMessage{Type: "error", Error: err.Error()}
			continue
		}

		// 保存成功后, 给发送方回一个确认包 ack, 包含刚保存的消息和服务器生成的 ID、时间等数据。
		c.send <- OutgoingMessage{Type: "ack", Message: message}
	}
}

// 服务端 → 浏览器
// 从 send channel 读取消息、写入 WebSocket、定时发送 Ping
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case payload, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			data, err := json.Marshal(payload)
			// 无法序列化成 JSON
			if err != nil {
				log.Println("json marshal:", err)
				continue
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-ticker.C:
			if c.hub.presence != nil {
				if err := c.hub.presence.SetOnline(context.Background(), c.userID, c.hub.nodeID); err != nil {
					log.Println("presence heartbeat:", err)
				}
			}
			// 刷新在线记录
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// 生成"WebSocket 发消息限流"的 Redis Key
func messageRateKey(userID int64) string {
	return "ratelimit:ws:message:user:" + strconv.FormatInt(userID, 10)
}

// 消费 RabbitMQ 的 message.created 事件，并在"接收者在线且连接在当前节点"时，将消息实时推送给接收者
func (h *Hub) DispatchMessageCreated(ctx context.Context, event mq.MessageCreatedEvent) error {
	if event.EditedAt != nil {
		return h.DispatchMessageEdited(ctx, event)
	}
	if event.RecalledAt != nil {
		return h.DispatchMessageRecalled(ctx, event)
	}
	if event.GroupID != nil {
		members, err := h.messages.GroupMembers(ctx, *event.GroupID)
		if err != nil {
			return err
		}
		message, err := h.messages.EnrichMessage(ctx, model.Message{
			ID:          event.MessageID,
			FromUserID:  event.FromUserID,
			GroupID:     event.GroupID,
			ContentType: event.ContentType,
			Content:     event.Content,
			ObjectKey:   event.ObjectKey,
			ObjectURL:   event.ObjectURL,
			FileName:    event.FileName,
			FileSize:    event.FileSize,
			CreatedAt:   event.CreatedAt,
			ReplyToID:   event.ReplyToID,
		})
		if err != nil {
			return err
		}
		payload := OutgoingMessage{
			Type:    "chat",
			Message: message,
		}
		for _, member := range members {
			// 发送者已经收到 ack，不再重复推送 chat
			if member.UserID == event.FromUserID {
				continue
			}
			if h.presence != nil {
				nodeID, online, err := h.presence.GetOnlineNode(ctx, member.UserID)
				if err != nil {
					return err
				}
				if !online || nodeID != h.nodeID {
					continue
				}
			}
			h.deliver(member.UserID, payload)
		}
		return nil
	}

	if h.presence != nil {
		nodeID, online, err := h.presence.GetOnlineNode(ctx, event.ToUserID)
		if err != nil {
			return err
		}
		if !online || nodeID != h.nodeID {
			return nil
		}
	}

	message, err := h.messages.EnrichMessage(ctx, model.Message{
		ID:          event.MessageID,
		FromUserID:  event.FromUserID,
		ToUserID:    event.ToUserID,
		ContentType: event.ContentType,
		Content:     event.Content,
		ObjectKey:   event.ObjectKey,
		ObjectURL:   event.ObjectURL,
		FileName:    event.FileName,
		FileSize:    event.FileSize,
		CreatedAt:   event.CreatedAt,
		ReplyToID:   event.ReplyToID,
	})
	if err != nil {
		return err
	}
	payload := OutgoingMessage{
		Type:    "chat",
		Message: message,
	}
	h.deliver(event.ToUserID, payload)
	return nil
}

func (h *Hub) DispatchMessageEdited(ctx context.Context, event mq.MessageCreatedEvent) error {
	message := model.Message{ID: event.MessageID, FromUserID: event.FromUserID, ToUserID: event.ToUserID, GroupID: event.GroupID, Content: event.Content, ContentType: event.ContentType, CreatedAt: event.CreatedAt, EditedAt: event.EditedAt, ReplyToID: event.ReplyToID}
	payload := OutgoingMessage{Type: "edit", Message: message}
	if event.GroupID != nil {
		members, err := h.messages.GroupMembers(ctx, *event.GroupID)
		if err != nil {
			return err
		}
		for _, member := range members {
			h.deliver(member.UserID, payload)
		}
		return nil
	}
	h.deliver(event.FromUserID, payload)
	h.deliver(event.ToUserID, payload)
	return nil
}

func (h *Hub) dispatchTyping(client *Client, input IncomingMessage) {
	payload := OutgoingMessage{Type: "typing", To: client.userID, GroupID: input.GroupID}
	if input.GroupID != nil {
		members, err := h.messages.GroupMembers(context.Background(), *input.GroupID)
		if err != nil {
			return
		}
		for _, member := range members {
			if member.UserID != client.userID {
				h.deliver(member.UserID, payload)
			}
		}
		return
	}
	if input.To > 0 {
		h.deliver(input.To, payload)
	}
}

// DispatchMessageRecalled broadcasts a recall update to every participant.
// Unlike a newly-created group message, the sender must also receive this
// event so that other tabs/devices update their local copy.
func (h *Hub) DispatchMessageRecalled(ctx context.Context, event mq.MessageCreatedEvent) error {
	message := model.Message{
		ID: event.MessageID, FromUserID: event.FromUserID, ToUserID: event.ToUserID,
		GroupID: event.GroupID, ContentType: event.ContentType, CreatedAt: event.CreatedAt,
		RecalledAt: event.RecalledAt, RecalledBy: event.RecalledBy, RecallReason: event.RecallReason,
	}
	payload := OutgoingMessage{Type: "recall", Message: message}
	if event.GroupID != nil {
		members, err := h.messages.GroupMembers(ctx, *event.GroupID)
		if err != nil {
			return err
		}
		for _, member := range members {
			if h.presence != nil {
				nodeID, online, err := h.presence.GetOnlineNode(ctx, member.UserID)
				if err != nil {
					return err
				}
				if !online || nodeID != h.nodeID {
					continue
				}
			}
			h.deliver(member.UserID, payload)
		}
		return nil
	}
	for _, userID := range []int64{event.FromUserID, event.ToUserID} {
		if userID <= 0 {
			continue
		}
		if h.presence != nil {
			nodeID, online, err := h.presence.GetOnlineNode(ctx, userID)
			if err != nil {
				return err
			}
			if !online || nodeID != h.nodeID {
				continue
			}
		}
		h.deliver(userID, payload)
	}
	return nil
}
