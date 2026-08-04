package app

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	goredis "github.com/redis/go-redis/v9"

	"IM_Chat_System/internal/chat"
	"IM_Chat_System/internal/config"
	"IM_Chat_System/internal/handler"
	"IM_Chat_System/internal/model"
	"IM_Chat_System/internal/mq"
	"IM_Chat_System/internal/outbox"
	redisPresence "IM_Chat_System/internal/presence/redis"
	"IM_Chat_System/internal/ratelimit"
	redisRateLimit "IM_Chat_System/internal/ratelimit/redis"
	mysqlrepo "IM_Chat_System/internal/repository/mysql"
	"IM_Chat_System/internal/search"
	essearch "IM_Chat_System/internal/search/elasticsearch"
	"IM_Chat_System/internal/service"
	"IM_Chat_System/internal/storage"
	miniostorage "IM_Chat_System/internal/storage/minio"
	"IM_Chat_System/internal/tokenblacklist"
	redisBlacklist "IM_Chat_System/internal/tokenblacklist/redis"
	redisUnread "IM_Chat_System/internal/unread/redis"
)

type App struct {
	Config    config.Config
	db        *sql.DB
	redis     *goredis.Client
	auth      *handler.AuthHandler
	logout    *handler.LogoutHandler
	users     *handler.UserHandler
	messages  *handler.MessageHandler
	media     *handler.MediaHandler
	search    *handler.SearchHandler
	hub       *chat.Hub
	blacklist tokenblacklist.Store
	limiter   ratelimit.Store
	publisher mq.EventPublisher
	consumer  *mq.MessageCreatedConsumer
	cancel    context.CancelFunc
	indexer   search.Indexer
}

// 读取并校验配置
func New() (*App, error) {
	cfg := config.Load() // 从环境变量读取配置
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// 创建一个连接池对象
	db, err := sql.Open("mysql", cfg.MySQLDSN)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}

	// 创建 Redis 客户端和连接池
	rdb := goredis.NewClient(&goredis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return nil, err
	}

	userRepo := mysqlrepo.NewUserRepository(db)
	messageRepo := mysqlrepo.NewMessageRepository(db)
	presenceStore := redisPresence.New(rdb)   // 创建在线状态存储组件
	unreadStore := redisUnread.New(rdb)       //创建未读消息存储组件
	blacklistStore := redisBlacklist.New(rdb) // 创建 JWT 黑名单组件
	rateLimitStore := redisRateLimit.New(rdb) // 创建 Redis 限流组件
	// NoopUploader 未启用 MinIO 时使用
	// minio.Uploader 启动 MinIO 时使用
	var uploader storage.Uploader = storage.NoopUploader{}
	if cfg.EnableMinIO {
		// Endpoint MinIO地址, AccessKey MinIO用户, SecretKey MinIO密码, Bucket 文件桶, UseSSL 是否使用 HTTPS, MediaURLTTL 预签名下载链接的有效期
		minioUploader, err := miniostorage.New(cfg.MinIOEndpoint, cfg.MinIOAccessKey, cfg.MinIOSecretKey, cfg.MinIOBucket, cfg.MinIOUseSSL, cfg.MediaURLTTL)
		if err != nil {
			return nil, err
		}
		uploader = minioUploader
	}

	var indexer search.Indexer = search.NoopIndexer{}
	if cfg.EnableElasticsearch {
		esIndexer, err := essearch.New(cfg.ElasticsearchURL, cfg.ElasticsearchIndex)
		if err != nil {
			return nil, err
		}
		indexer = esIndexer
	}

	var publisher mq.EventPublisher = mq.NoopPublisher{}
	var consumer *mq.MessageCreatedConsumer

	if cfg.EnableRabbitMQ {
		// 创建真实 RabbitMQ 发布器
		rabbitPublisher, err := mq.NewRabbitPublisher(cfg.RabbitMQURL)
		if err != nil {
			return nil, err
		}
		publisher = rabbitPublisher
		// 创建消费者
		consumer, err = mq.NewMessageCreatedConsumer(cfg.RabbitMQURL, cfg.NodeID)
		if err != nil {
			_ = publisher.Close()
			return nil, err
		}
	}

	// 创建运行期 Context
	// runtimeCtx
	// → 传给 RabbitMQ Consumer
	// → 传给 Outbox Dispatcher
	runtimeCtx, cancel := context.WithCancel(context.Background())

	// 创建认证业务服务
	// userRepo -> 查询用户、查询用户
	// JWTSecret -> 签发和验证 JWT 的密钥
	// TokenTTL -> JWT 有效期
	authService := service.NewAuthService(userRepo, cfg.JWTSecret, int64(cfg.TokenTTL.Hours()))

	// 创建消息业务服务
	/*
		校验接收方用户是否存在
		→ 写入 MySQL messages
		→ 同事务写入 outbox_events
		→ 更新 Redis 未读消息
		→ 上传文件到 MinIO
		→ 为媒体消息生成预签名 URL
		→ 查询历史和离线消息
	*/
	messageService := service.NewMessageService(userRepo, messageRepo, unreadStore, uploader)

	// 创建 WebSocket Hub
	/*
		messageService
		→ 接收到 WebSocket 文本消息后保存消息
		JWTSecret
		→ 校验 WebSocket 身份
		NodeID
		→ 判断用户是否连接在当前应用节点
		presenceStore
		→ Redis 在线状态
		blacklistStore
		→ 判断 Token 是否已退出登录
		rateLimitStore
		→ 限制 WebSocket 发消息频率
		AllowedOrigins
		→ 限制允许连接的浏览器来源
	*/
	hub := chat.NewHub(messageService, cfg.JWTSecret, cfg.NodeID, presenceStore, blacklistStore, rateLimitStore, cfg.AllowedOrigins)

	// 创建 Outbox Dispatcher
	/*
		查询 outbox_events 中未发布事件
		→ 调用 RabbitMQ Publisher 发布
		→ 收到 Broker Confirm
		→ 标记事件 published_at
	*/
	dispatcher := outbox.NewDispatcher(mysqlrepo.NewOutboxRepository(db), publisher)

	// 启动 RabbitMQ Consumer
	if consumer != nil {
		/*
			RabbitMQ 收到 message.created
			→ 取出 event
			→ hub.DispatchMessageCreated(...)
			→ 查询 Redis 在线状态
			→ 判断用户是否在线于当前 NodeID
			→ 是：通过 WebSocket 推送给接收方
			→ 否：不推送，用户之后通过离线消息接口拉取
		*/
		if err := consumer.Start(runtimeCtx, func(ctx context.Context, event mq.MessageCreatedEvent) error { // 节点推送队列
			return hub.DispatchMessageCreated(ctx, event)
		},

			/*
				RabbitMQ 消息事件
				→ 转换为业务 Message 对象
				→ indexer.IndexMessage(...)
				→ 写入 Elasticsearch messages 索引
			*/
			func(ctx context.Context, event mq.MessageCreatedEvent) error {
				return indexer.IndexMessage(ctx, model.Message{
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
				})
			}); err != nil {
			cancel()
			_ = consumer.Close()
			_ = publisher.Close()
			return nil, err
		}
	}

	go dispatcher.Run(runtimeCtx)

	return &App{
		Config:    cfg,
		db:        db,
		redis:     rdb,
		auth:      handler.NewAuthHandler(authService),
		logout:    handler.NewLogoutHandler(blacklistStore),
		users:     handler.NewUserHandler(messageService),
		messages:  handler.NewMessageHandler(messageService),
		media:     handler.NewMediaHandler(messageService, cfg.MaxUploadBytes),
		search:    handler.NewSearchHandler(indexer, messageService),
		hub:       hub,
		blacklist: blacklistStore,
		limiter:   rateLimitStore,
		publisher: publisher,
		consumer:  consumer,
		cancel:    cancel,
		indexer:   indexer,
	}, nil
}

func (a *App) Router() http.Handler {
	router := gin.New()
	router.Use(ginRequestLogger(), gin.Recovery())

	// Gin 负责路由和全局恢复；现有 Handler 保持标准 net/http 签名，
	// 通过 gin.WrapF 平滑接入，避免改变认证、业务和 WebSocket 层的行为。
	router.GET("/healthz", gin.WrapF(a.Health))
	router.POST("/api/register", gin.WrapF(handler.WithRateLimit(a.limiter, "register", 10, time.Minute, handler.ClientIPKey, a.auth.Register)))
	router.POST("/api/login", gin.WrapF(handler.WithRateLimit(a.limiter, "login", 20, time.Minute, handler.ClientIPKey, a.auth.Login)))
	router.POST("/api/logout", gin.WrapF(handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.logout.Logout)))
	router.GET("/api/me", gin.WrapF(handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.users.Me)))
	router.GET("/api/users", gin.WrapF(handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.users.Users)))
	router.GET("/api/messages", gin.WrapF(handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.messages.Messages)))
	router.GET("/api/offline", gin.WrapF(handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.messages.Offline)))
	router.POST("/api/media/upload", gin.WrapF(handler.WithRateLimit(a.limiter, "media-upload", 20, time.Minute, handler.ClientIPKey, handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.media.Upload))))
	router.GET("/api/search/messages", gin.WrapF(handler.WithRateLimit(a.limiter, "message-search", 60, time.Minute, handler.ClientIPKey, handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.search.SearchMessages))))
	router.GET("/ws", gin.WrapF(a.hub.ServeWS))

	// 未匹配到 API 或 WebSocket 路由时，交给原生文件服务返回 web 目录内容。
	router.NoRoute(gin.WrapH(http.FileServer(http.Dir("web"))))
	return router
}

func (a *App) Health(w http.ResponseWriter, r *http.Request) {
	// 健康检查最多执行 2 秒
	// r.Context() 是当前 HTTP 请求的 Context
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.db.PingContext(ctx); err != nil {
		http.Error(w, "mysql unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := a.redis.Ping(ctx).Err(); err != nil {
		http.Error(w, "redis unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (a *App) Close() {
	if a.cancel != nil {
		a.cancel()
	}
	if a.consumer != nil {
		_ = a.consumer.Close()
	}
	if a.publisher != nil {
		_ = a.publisher.Close()
	}
	if a.indexer != nil {
		_ = a.indexer.Close()
	}
	if a.redis != nil {
		_ = a.redis.Close()
	}
	if a.db != nil {
		_ = a.db.Close()
	}
}

func ginRequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		log.Printf("method=%s path=%s status=%d duration=%s", c.Request.Method, c.Request.URL.Path, c.Writer.Status(), time.Since(started).Round(time.Millisecond))
	}
}
