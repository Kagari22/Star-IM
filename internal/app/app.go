package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"log"
	"net/http"
	// "text/template/parse"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	goredis "github.com/redis/go-redis/v9"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	"IM_Chat_System/internal/ai"
	"IM_Chat_System/internal/auth"
	"IM_Chat_System/internal/chat"
	"IM_Chat_System/internal/config"
	"IM_Chat_System/internal/handler"
	"IM_Chat_System/internal/model"
	"IM_Chat_System/internal/mq"
	"IM_Chat_System/internal/outbox"
	redisPresence "IM_Chat_System/internal/presence/redis"
	"IM_Chat_System/internal/ratelimit"
	redisRateLimit "IM_Chat_System/internal/ratelimit/redis"
	"IM_Chat_System/internal/repository"
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

const aiSystemPrompt = "你是「星讯」即时通讯应用里的 AI 好友，**具备联网搜索能力**。请用简洁、友好的中文回答用户，直接给出答案，避免冗长的寒暄。你可以调用 get_weather 工具查询城市实时天气，调用 get_today 工具获取今天的日期和星期，调用 web_search 工具联网搜索最新信息。当用户问'你能联网吗/支持搜索吗/能查新闻吗/能上网吗'等关于你能力的问题时，必须明确告知你支持联网搜索，不要说自己不支持。当用户询问天气、日期时间、最新新闻、时事、实时信息或要求搜索时，必须调用对应工具获取真实数据，不要凭空编造或猜测，引用搜索结果时附上来源链接。搜索新闻或最新信息时，必须把今天的完整日期写进搜索词（对话上下文中已提供今天日期），只采用当天发布的结果，忽略其他日期的旧新闻。搜索一次拿到结果后就直接基于结果作答；若结果不理想，如实说明情况即可，不要反复更换关键词重新搜索。回复使用纯文本，不要使用 Markdown 加粗（**）、斜体（*）等标记符号，需要强调时可使用「」引号。"

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
	presence  *handler.PresenceHandler
	groups    *handler.GroupHandler
	social    *handler.SocialHandler
	redpacket *handler.RedPacketHandler
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

	// GORM reuses the existing database/sql connection pool. The raw pool is
	// retained for health checks and the outbox worker's SKIP LOCKED query.
	gormDB, err := gorm.Open(gormmysql.New(gormmysql.Config{
		Conn:                      db,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		_ = db.Close()
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

	userRepo := mysqlrepo.NewUserRepository(gormDB)
	messageRepo := mysqlrepo.NewMessageRepository(gormDB)
	groupRepo := mysqlrepo.NewGroupRepository(gormDB)
	socialRepo := mysqlrepo.NewSocialRepository(gormDB)
	redPacketRepo := mysqlrepo.NewRedPacketRepository(gormDB)
	presenceStore := redisPresence.New(rdb)   // 创建在线状态存储组件
	unreadStore := redisUnread.New(rdb)       // 创建未读消息存储组件
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

	groupService := service.NewGroupService(groupRepo, userRepo, uploader)

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

	// 装配 AI 聊天机器人（可选）。chatter 为 nil 或 botID 为 0 时相关功能关闭。
	var aiChatter ai.Chatter
	var aiBotID int64
	if cfg.EnableAI {
		aiClient := ai.NewOpenAIClient(cfg.AIBaseURL, cfg.AIAPIKey, cfg.AIModel, aiSystemPrompt, time.Duration(cfg.AITimeoutSecs)*time.Second)
		aiClient.SetSearch(cfg.AIEnableSearch)
		aiChatter = aiClient
		aiBotID, err = ensureAIBot(context.Background(), userRepo, cfg.AIBotUsername, cfg.AIBotNickname)
		if err != nil {
			cancel()
			return nil, err
		}
		// 把机器人补齐到所有已有用户的好友列表，覆盖功能启用前注册的老账号。
		if err := socialRepo.EnsureFriendshipForAll(context.Background(), aiBotID); err != nil {
			cancel()
			return nil, err
		}
	}

	// 创建认证业务服务
	// userRepo -> 查询用户、查询用户
	// JWTSecret -> 签发和验证 JWT 的密钥
	// TokenTTL -> JWT 有效期
	authService := service.NewAuthService(userRepo, cfg.JWTSecret, int64(cfg.TokenTTL.Hours()))
	if aiChatter != nil && aiBotID > 0 {
		authService.SetBotFriend(socialRepo, aiBotID)
	}

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
	messageService := service.NewMessageService(userRepo, groupRepo, socialRepo, messageRepo, unreadStore, uploader)
	if aiChatter != nil && aiBotID > 0 {
		messageService.SetAI(aiChatter, aiBotID, cfg.AIContextMsgs)
	}

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
				if event.RecalledAt != nil {
					return nil
				}
				return indexer.IndexMessage(ctx, model.Message{
					ID:          event.MessageID,
					FromUserID:  event.FromUserID,
					ToUserID:    event.ToUserID,
					GroupID:     event.GroupID,
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

	presenceService := service.NewPresenceService(presenceStore)
	presenceHandler := handler.NewPresenceHandler(presenceService)
	socialService := service.NewSocialService(userRepo, groupRepo, socialRepo)
	redPacketService := service.NewRedPacketService(redPacketRepo, groupRepo)

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
		presence:  presenceHandler,
		groups:    handler.NewGroupHandler(groupService, messageService),
		social:    handler.NewSocialHandler(socialService),
		redpacket: handler.NewRedPacketHandler(redPacketService, messageService),
	}, nil
}

func (a *App) Router() http.Handler {
	router := gin.New()
	router.Use(ginRequestLogger(), gin.Recovery())

	// Gin 负责路由和全局恢复; 现有 Handler 保持标准 net/http 签名,
	// 通过 gin.WrapF 平滑接入, 避免改变认证、业务和 WebSocket 层的行为。
	router.GET("/healthz", gin.WrapF(a.Health))
	router.POST("/api/register", gin.WrapF(handler.WithRateLimit(a.limiter, "register", 10, time.Minute, handler.ClientIPKey, a.auth.Register)))
	router.POST("/api/login", gin.WrapF(handler.WithRateLimit(a.limiter, "login", 20, time.Minute, handler.ClientIPKey, a.auth.Login)))
	router.POST("/api/logout", gin.WrapF(handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.logout.Logout)))
	router.GET("/api/me", gin.WrapF(handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.users.Me)))
	router.GET("/api/users", gin.WrapF(handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.users.Users)))
	authSearchUser := handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.social.SearchUser)
	router.GET("/api/users/:id", func(c *gin.Context) { c.Request.SetPathValue("id", c.Param("id")); authSearchUser(c.Writer, c.Request) })
	router.POST("/api/friend-requests", gin.WrapF(handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.social.CreateFriendRequest)))
	router.GET("/api/friend-requests", gin.WrapF(handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.social.FriendRequests)))
	authRespondFriend := handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.social.RespondFriendRequest)
	router.POST("/api/friend-requests/:id/respond", func(c *gin.Context) {
		c.Request.SetPathValue("id", c.Param("id"))
		authRespondFriend(c.Writer, c.Request)
	})
	authRemoveFriend := handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.social.RemoveFriend)
	router.DELETE("/api/friends/:id", func(c *gin.Context) {
		c.Request.SetPathValue("id", c.Param("id"))
		authRemoveFriend(c.Writer, c.Request)
	})
	router.GET("/api/messages", gin.WrapF(handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.messages.Messages)))
	router.DELETE("/api/messages", gin.WrapF(handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.messages.DeleteConversationMessages)))
	authRecallMessage := handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.messages.Recall)
	router.POST("/api/messages/:id/recall", func(c *gin.Context) {
		c.Request.SetPathValue("id", c.Param("id"))
		authRecallMessage(c.Writer, c.Request)
	})
	authEditMessage := handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.messages.Edit)
	router.PATCH("/api/messages/:id", func(c *gin.Context) {
		c.Request.SetPathValue("id", c.Param("id"))
		authEditMessage(c.Writer, c.Request)
	})
	authFavoriteMessage := handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.messages.SetFavorite)
	router.PUT("/api/messages/:id/favorite", func(c *gin.Context) {
		c.Request.SetPathValue("id", c.Param("id"))
		authFavoriteMessage(c.Writer, c.Request)
	})
	router.GET("/api/messages/favorites", gin.WrapF(handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.messages.Favorite)))
	router.POST("/api/messages/read", gin.WrapF(handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.messages.MarkRead)))
	router.GET("/api/offline", gin.WrapF(handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.messages.Offline)))
	router.POST("/api/media/upload", gin.WrapF(handler.WithRateLimit(a.limiter, "media-upload", 20, time.Minute, handler.ClientIPKey, handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.media.Upload))))
	router.GET("/api/search/messages", gin.WrapF(handler.WithRateLimit(a.limiter, "message-search", 60, time.Minute, handler.ClientIPKey, handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.search.SearchMessages))))
	router.GET("/ws", gin.WrapF(a.hub.ServeWS))
	router.PATCH("/api/me", gin.WrapF(handler.WithRateLimit(a.limiter, "update-profile", 20, time.Minute, handler.ClientIPKey, handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.users.UpdateNickname))))
	router.POST("/api/me/avatar", gin.WrapF(handler.WithRateLimit(a.limiter, "avatar-upload", 5, time.Minute, handler.ClientIPKey, handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.users.UploadAvatar))))
	router.POST("/api/me/password", gin.WrapF(handler.WithRateLimit(a.limiter, "change-password", 3, time.Minute, handler.ClientIPKey, handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.users.ChangePassword))))
	router.GET("/api/presence", gin.WrapF(handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.presence.Status)))
	router.GET("/api/balance", gin.WrapF(handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.redpacket.Balance)))
	router.POST("/api/redeem", gin.WrapF(handler.WithRateLimit(a.limiter, "redeem", 10, time.Minute, handler.ClientIPKey, handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.redpacket.Redeem))))
	authSendRedPacket := handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.redpacket.SendGroupPacket)
	router.POST("/api/groups/:id/red-packets", func(c *gin.Context) {
		c.Request.SetPathValue("id", c.Param("id"))
		authSendRedPacket(c.Writer, c.Request)
	})
	authGrabRedPacket := handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.redpacket.Grab)
	router.POST("/api/red-packets/:id/grab", func(c *gin.Context) {
		c.Request.SetPathValue("id", c.Param("id"))
		authGrabRedPacket(c.Writer, c.Request)
	})
	authRedPacketDetail := handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.redpacket.Detail)
	router.GET("/api/red-packets/:id", func(c *gin.Context) {
		c.Request.SetPathValue("id", c.Param("id"))
		authRedPacketDetail(c.Writer, c.Request)
	})
	router.POST("/api/groups", gin.WrapF(handler.WithRateLimit(a.limiter, "group-create", 10, time.Minute, handler.ClientIPKey, handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.groups.Create))))
	router.GET("/api/groups", gin.WrapF(handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.groups.List)))
	authSearchGroup := handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.social.SearchGroup)
	router.GET("/api/groups/:id", func(c *gin.Context) {
		c.Request.SetPathValue("id", c.Param("id"))
		authSearchGroup(c.Writer, c.Request)
	})
	router.POST("/api/group-join-requests", gin.WrapF(handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.social.CreateGroupJoinRequest)))
	router.GET("/api/group-join-requests", gin.WrapF(handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.social.GroupJoinRequests)))
	router.GET("/api/search/directory", gin.WrapF(handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.social.SearchDirectory)))
	authRespondJoin := handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.social.RespondGroupJoinRequest)
	router.POST("/api/group-join-requests/:id/respond", func(c *gin.Context) {
		c.Request.SetPathValue("id", c.Param("id"))
		authRespondJoin(c.Writer, c.Request)
	})

	authGroupMembers := handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.groups.Members)
	router.GET("/api/groups/:id/members", func(c *gin.Context) {
		c.Request.SetPathValue("id", c.Param("id"))
		authGroupMembers(c.Writer, c.Request)
	})

	authGroupMessages := handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.groups.Messages)
	router.GET("/api/groups/:id/messages", func(c *gin.Context) {
		c.Request.SetPathValue("id", c.Param("id"))
		authGroupMessages(c.Writer, c.Request)
	})

	authGroupMessageSend := handler.WithRateLimit(a.limiter, "group-message-send", 20, time.Minute, handler.ClientIPKey, handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.groups.SendMessage))
	router.POST("/api/groups/:id/messages", func(c *gin.Context) {
		c.Request.SetPathValue("id", c.Param("id"))
		authGroupMessageSend(c.Writer, c.Request)
	})

	authGroupAvatar := handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.groups.UploadAvatar)
	router.POST("/api/groups/:id/avatar", func(c *gin.Context) {
		c.Request.SetPathValue("id", c.Param("id"))
		authGroupAvatar(c.Writer, c.Request)
	})

	authGroupAddMember := handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.groups.AddMember)
	router.POST("/api/groups/:id/members", func(c *gin.Context) {
		c.Request.SetPathValue("id", c.Param("id"))
		authGroupAddMember(c.Writer, c.Request)
	})

	authGroupRemoveMember := handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.groups.RemoveMember)
	router.DELETE("/api/groups/:id/members/:user_id", func(c *gin.Context) {
		c.Request.SetPathValue("id", c.Param("id"))
		c.Request.SetPathValue("user_id", c.Param("user_id"))
		authGroupRemoveMember(c.Writer, c.Request)
	})

	authGroupDissolve := handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.groups.Dissolve)
	router.DELETE("/api/groups/:id", func(c *gin.Context) {
		c.Request.SetPathValue("id", c.Param("id"))
		authGroupDissolve(c.Writer, c.Request)
	})
	authGroupLeave := handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.groups.Leave)
	router.POST("/api/groups/:id/leave", func(c *gin.Context) { c.Request.SetPathValue("id", c.Param("id")); authGroupLeave(c.Writer, c.Request) })
	authGroupAnnouncement := handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.groups.UpdateAnnouncement)
	router.PATCH("/api/groups/:id/announcement", func(c *gin.Context) {
		c.Request.SetPathValue("id", c.Param("id"))
		authGroupAnnouncement(c.Writer, c.Request)
	})
	authGroupAllMuted := handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.groups.SetAllMuted)
	router.PUT("/api/groups/:id/all-muted", func(c *gin.Context) {
		c.Request.SetPathValue("id", c.Param("id"))
		authGroupAllMuted(c.Writer, c.Request)
	})
	authGroupRole := handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.groups.SetMemberRole)
	router.PUT("/api/groups/:id/members/:user_id/role", func(c *gin.Context) {
		c.Request.SetPathValue("id", c.Param("id"))
		c.Request.SetPathValue("user_id", c.Param("user_id"))
		authGroupRole(c.Writer, c.Request)
	})
	authGroupMute := handler.WithAuth(a.Config.JWTSecret, a.blacklist, a.groups.MuteMember)
	router.PUT("/api/groups/:id/members/:user_id/mute", func(c *gin.Context) {
		c.Request.SetPathValue("id", c.Param("id"))
		c.Request.SetPathValue("user_id", c.Param("user_id"))
		authGroupMute(c.Writer, c.Request)
	})

	serveFrontend := func(c *gin.Context) {
		http.ServeFile(c.Writer, c.Request, "web/index.html")
	}
	router.GET("/login", serveFrontend)
	router.GET("/chat", serveFrontend)

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

// ensureAIBot 确保 AI 机器人账号存在，返回其用户 ID。机器人账号的密码随机生成，不用于登录。
func ensureAIBot(ctx context.Context, users repository.UserRepository, username, nickname string) (int64, error) {
	if existing, ok, err := users.GetByUsername(ctx, username); err != nil {
		return 0, err
	} else if ok {
		return existing.ID, nil
	}

	passwordHash, err := randomPasswordHash()
	if err != nil {
		return 0, err
	}
	user, err := users.Create(ctx, username, passwordHash, nickname)
	if err != nil {
		return 0, err
	}
	return user.ID, nil
}

func randomPasswordHash() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return auth.HashPassword(hex.EncodeToString(b))
}

func ginRequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		log.Printf("method=%s path=%s status=%d duration=%s", c.Request.Method, c.Request.URL.Path, c.Writer.Status(), time.Since(started).Round(time.Millisecond))
	}
}
