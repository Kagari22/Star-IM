const fs = require('fs');
const {
  Document,
  Packer,
  Paragraph,
  TextRun,
  HeadingLevel,
  AlignmentType,
  PageNumber,
  Footer,
  ShadingType,
} = require('docx');

const output = process.argv[2] || 'IM-Chat-System-100-Interview-QA.docx';

const sections = [
  {
    title: '一、项目整体架构与技术选型',
    items: [
      ['请用一句话介绍这个项目。', '这是一个基于 Go、Vue 3 和 WebSocket 的即时通信系统，后端通过 MySQL 持久化消息，Redis 管理在线状态、未读数和限流，RabbitMQ 异步分发消息事件，Elasticsearch 提供消息检索，MinIO 负责媒体文件存储，Docker Compose 编排基础设施。'],
      ['项目的完整消息链路是什么？', '发送方通过 WebSocket 或媒体 HTTP 接口提交消息；Service 校验业务数据后由 MySQL 事务同时写入 messages 和 outbox_events；Dispatcher 抢占 Outbox 事件并发布到 RabbitMQ；RabbitMQ 分别驱动当前节点 WebSocket 推送和 Elasticsearch 建索引；接收方离线时从 MySQL 通过 after_id 增量拉取。'],
      ['为什么采用 Handler、Service、Repository 分层？', 'Handler 负责协议解析、鉴权和 HTTP 响应，Service 负责业务编排，Repository 负责数据库访问。这样可以隔离传输层与业务层，便于替换存储实现、编写单元测试和控制职责边界。'],
      ['项目中各个基础设施分别承担什么职责？', 'MySQL 保存用户、消息和 Outbox 事件；Redis 保存在线节点、会话未读数、Token 黑名单和限流计数；RabbitMQ 承载异步 message.created 事件；Elasticsearch 建立消息搜索索引；MinIO 保存图片和文件对象。'],
      ['为什么消息主数据放 MySQL，而不是直接放 Redis 或 Elasticsearch？', '聊天消息需要可靠持久化、事务约束和按会话增量读取，MySQL 更适合作为事实数据源。Redis 用于高频状态，Elasticsearch 用于检索，二者都不替代 MySQL 的持久化职责。'],
      ['项目如何做到服务启动时自动准备依赖？', '应用启动时读取并校验环境变量，连接 MySQL 和 Redis；启用 RabbitMQ、MinIO、Elasticsearch 时分别创建客户端和必要的交换机、Bucket、索引；Docker Compose 负责基础设施容器编排。'],
      ['项目中接口是如何解耦第三方组件的？', '通过 repository、storage.Uploader、search.Indexer、mq.EventPublisher、presence.Store 等接口隔离具体实现，生产环境注入 MySQL、MinIO、Elasticsearch、RabbitMQ 实现，测试或未启用组件时使用 Noop 实现。'],
      ['项目的核心一致性边界在哪里？', '消息表和 Outbox 事件在同一个 MySQL 事务中提交，这是强一致边界；RabbitMQ、WebSocket 和 Elasticsearch 属于事务外异步处理，整体采用至少一次投递和最终一致性。'],
      ['这个项目更接近单体还是微服务？', '它是模块化单体应用：后端进程内部按领域和接口分层，但通过 RabbitMQ 把实时推送和搜索索引解耦，具备向多节点扩展的基础。'],
      ['如果面试官要求画架构图，应该怎么画？', '从浏览器画 HTTP/WS 到 Gin Handler 和 WebSocket Hub；Handler/Hub 下接 Service；Service 下接 MySQL、Redis、MinIO；MySQL Outbox 连接 Dispatcher，再到 RabbitMQ；RabbitMQ 分叉到节点推送队列和索引队列，分别连接 Hub 和 Elasticsearch。'],
    ],
  },
  {
    title: '二、HTTP 接口、鉴权与安全',
    items: [
      ['HTTP 接口如何完成 JWT 鉴权？', 'WithAuth 中间件读取 Authorization Bearer Token，先检查 Redis 黑名单，再使用服务端密钥校验 JWT 签名和 exp，最后将 claims 和原始 token 写入 request context，交给后续 Handler。'],
      ['为什么用户 ID 要从 claims 中获取，而不是从请求参数获取？', '请求参数由客户端控制，如果直接信任 user_id 会产生越权风险。使用经过签名校验的 claims.UserID，可以保证业务身份来自服务端验证后的 Token。'],
      ['JWT 登出后为什么还能立即失效？', 'JWT 本身是无状态凭证，签发后通常不会主动变化。项目在登出时把 Token 的哈希写入 Redis 黑名单，并设置为 Token 剩余有效期；后续请求先查黑名单，命中就拒绝。'],
      ['Token 黑名单为什么存 SHA-256，而不是原始 Token？', '原始 JWT 是可直接使用的凭证，直接作为 Redis Key 会增加泄露风险。项目使用 SHA-256 作为 Key 标识，即使查看 Redis Key 也不能直接得到原 Token。'],
      ['密码是如何存储的？', '注册时使用 PBKDF2-SHA256，随机生成 16 字节盐、120000 次迭代和 32 字节派生密钥，并保存为包含算法、迭代次数、盐和摘要的编码字符串；校验时使用 hmac.Equal 做恒定时间比较。'],
      ['为什么不能把 PasswordHash 返回给前端？', '密码哈希虽然不是明文，但仍属于敏感认证材料。GetMe 和用户列表逻辑会主动清空 PasswordHash，避免通过 API 泄露。'],
      ['接口如何做限流？', 'Handler 使用 Redis 限流 Store，按客户端 IP 或用户维度生成限流 Key；Redis Lua 脚本原子执行 INCR 和首次 PEXPIRE，覆盖注册、登录、上传、搜索和 WebSocket 发言。'],
      ['WebSocket 为什么还要额外校验 Origin？', 'WebSocket 握手可能被其他站点发起，Origin 白名单可以降低跨站滥用风险。项目拒绝空 Origin，并要求来源匹配 IM_ALLOWED_ORIGINS。'],
      ['上传接口有哪些安全校验？', '服务端限制整个请求体和单文件大小，使用文件头 512 字节检测真实 MIME 类型，仅允许图片、PDF 和纯文本白名单，清理文件名，并通过私有 MinIO 的短时预签名 URL访问对象。'],
      ['项目如何防止 SQL 注入？', '所有数据库操作都通过 database/sql 的参数占位符传值，不把用户输入直接拼接进 SQL；分页、用户 ID 和搜索条件也经过边界校验。'],
    ],
  },
  {
    title: '三、WebSocket 实时通信',
    items: [
      ['WebSocket 连接建立时做了哪些校验？', '服务端从 Sec-WebSocket-Protocol 中读取 im-chat 和 Token，检查 Token 黑名单，解析 JWT，校验 Origin 白名单后才执行协议升级。'],
      ['为什么把 JWT 放在 WebSocket 子协议中？', '浏览器原生 WebSocket API 不支持自定义 Authorization Header。项目使用子协议数组 [im-chat, token] 传递认证信息，并在服务端握手阶段解析。'],
      ['Hub 的作用是什么？', 'Hub 管理当前节点的 userID 到 Client 映射，负责连接注册、注销、消息投递、在线状态同步以及 RabbitMQ 事件到 WebSocket 的转换。'],
      ['为什么每个 WebSocket Client 有独立的读写协程？', 'Gorilla WebSocket 要求连接读写并发受控。readPump 专注读取和处理客户端消息，writePump 专注发送服务端消息和 Ping，避免多个 goroutine 同时写同一连接。'],
      ['WebSocket 如何实现心跳保活？', 'writePump 每 30 秒发送 Ping，readPump 设置 Pong Handler 并刷新读超时时间；超过 pongWait 没有收到响应时，读取操作失败并注销连接。'],
      ['如何处理慢客户端？', '每个 Client 使用带缓冲的 send channel；deliver 投递时如果 channel 已满，就主动关闭连接，避免单个慢客户端阻塞整个 Hub。'],
      ['同一用户重复连接时如何处理？', 'Hub 注册时检查 clients 中是否已有旧连接，如果有则关闭旧连接，再保存新连接，保证一个节点内同一用户只保留一个活跃连接。'],
      ['收到客户端 chat 消息后的流程是什么？', 'readPump 解析 JSON，校验 type，执行 WebSocket 发言限流，调用 MessageService.SaveText 持久化并增加未读数，成功后向发送方发送 ack。'],
      ['服务端如何把消息推送给接收方？', 'RabbitMQ 节点消费者收到 message.created 后调用 Hub.DispatchMessageCreated，先查询接收方在线节点；如果节点匹配当前实例，就生成媒体 URL 并通过 deliver 写入目标 Client 的发送队列。'],
      ['WebSocket 发送成功是否代表消息已经持久化？', '项目先调用 SaveText 完成 MySQL 消息和 Outbox 事务，再发送 ack，因此 ack 代表服务端已经接受并持久化消息；真正推送给接收方由异步 RabbitMQ 链路完成。'],
    ],
  },
  {
    title: '四、MySQL、事务与消息持久化',
    items: [
      ['SaveAndEnqueue 为什么要开启事务？', '它需要保证 messages 和 outbox_events 同时成功或同时失败。如果消息写入成功但 Outbox 写入失败，系统会出现消息存在却没有异步事件的问题；事务可以避免这种不一致。'],
      ['SaveAndEnqueue 的具体步骤是什么？', '开启事务，插入 messages，获取自增 ID，在事务内查询完整消息，序列化 message.created 事件，插入 outbox_events，最后提交事务。任一步失败都会回滚。'],
      ['为什么插入消息后还要重新 SELECT 一次？', '需要拿到数据库生成的 ID 和 created_at 等最终字段，再用完整消息生成事件，确保事件内容与数据库记录一致。'],
      ['defer tx.Rollback 在 Commit 后会不会报错？', 'Commit 后事务已经结束，再调用 Rollback 通常会返回 sql.ErrTxDone；项目忽略这个兜底回滚错误，不影响已经提交的事务。'],
      ['ListConversation 如何保证只返回双方消息？', 'SQL 使用两个方向的 OR 条件：当前用户发给 peer，或 peer 发给当前用户，同时通过 userID 和 peerID 参数绑定，不能查询其他会话。'],
      ['afterID 分页有什么优势？', '它是基于递增消息 ID 的游标分页，查询条件为 id > afterID，避免深分页 OFFSET 的性能问题，也适合离线增量同步。'],
      ['ListOffline 与 ListConversation 有什么区别？', 'ListConversation 查询当前用户与指定对方的双向消息；ListOffline 查询 to_user_id 等于当前用户且 ID 大于游标的所有收到消息，用于离线补偿。'],
      ['为什么 SQL 中使用 ORDER BY id ASC？', '按消息 ID 升序返回可以保持消息时间顺序，并把本批最后一条 ID 作为下一次增量查询游标。'],
      ['数据库表中有哪些关键索引？', 'messages 有按发送方、接收方和 ID 的复合索引，以及按接收方和 ID 的索引；outbox_events 有待发布状态、可用时间、锁和 ID 的组合索引。'],
      ['项目的消息一致性属于什么级别？', 'MySQL 内的消息和 Outbox 是事务一致的；RabbitMQ 推送和 Elasticsearch 索引采用异步最终一致性，整体具备至少一次投递语义。'],
    ],
  },
  {
    title: '五、Outbox、RabbitMQ 与可靠投递',
    items: [
      ['Transactional Outbox 解决什么问题？', '它解决数据库事务与消息队列发送之间的双写一致性问题：先把业务数据和待发布事件写入同一数据库事务，再由后台 Dispatcher 异步发布到 RabbitMQ。'],
      ['Dispatcher 的 Run 方法做什么？', '启动时立即执行一次 DispatchOnce，然后使用 ticker 按 interval 周期执行；监听 ctx.Done 实现优雅停止。'],
      ['DispatchOnce 的处理流程是什么？', '调用 repository.Claim 抢占事件，逐条反序列化 payload，调用 publisher 发布并等待 Broker Confirm，成功后 MarkPublished，失败后 MarkFailed 延迟重试。'],
      ['Claim 为什么使用 FOR UPDATE SKIP LOCKED？', 'FOR UPDATE 对待处理行加排他锁，SKIP LOCKED 让其他 Dispatcher 跳过已锁定行，从而支持多实例并行抢占而不重复处理。'],
      ['locked_until 的作用是什么？', '它是租约式处理锁。Dispatcher 抢到事件后设置未来过期时间；如果进程崩溃，锁过期后其他实例可以重新抢占，避免任务永久卡住。'],
      ['available_at 的作用是什么？', '它表示事件下一次可以被处理的时间。失败后通过 MarkFailed 延迟 available_at，实现重试退避，避免故障期间高频重试。'],
      ['retryDelay 如何实现退避？', 'retryDelay 使用 2 的指数幂计算秒数，失败次数 1 到 6 对应 1、2、4、8、16、32 秒，并将次数限制在 1 到 6。'],
      ['RabbitPublisher 为什么开启 Publisher Confirm？', 'PublishWithContext 成功只代表消息写入客户端 Channel，Publisher Confirm 能确认 Broker 是否真正接收并确认消息；收到 Nack 或确认通道关闭时，发布视为失败。'],
      ['消息为什么设置 DeliveryMode Persistent？', '将消息标记为持久化，配合持久化交换机和队列，可降低 RabbitMQ 重启造成消息丢失的风险。'],
      ['消费者为什么使用手动 Ack？', 'autoAck=false，只有 JSON 解析和业务 Handler 都成功后才 Ack；解析错误 Nack 且不重新入队，业务失败 Nack 并重新入队，从而避免处理失败时消息丢失。'],
    ],
  },
  {
    title: '六、Redis 在线状态、未读数与限流',
    items: [
      ['在线状态在 Redis 中如何表示？', '使用 Key presence:user:<userID>，Value 保存应用节点 ID，并设置 onlineTTL；连接建立和心跳时续期，断开时删除。'],
      ['为什么在线状态需要 TTL？', '客户端可能异常断网或进程崩溃，无法执行 SetOffline。TTL 到期后 Redis 自动清理，避免脏在线状态长期存在。'],
      ['GetOnlineNode 返回什么？', '返回在线节点字符串、在线布尔值和错误；Redis Key 不存在时返回空节点、false、nil，把用户离线视为正常业务状态。'],
      ['未读数为什么使用 Redis Sorted Set？', 'Sorted Set 的 Member 保存消息 ID，Score 也使用消息 ID；既能用 ZCard 统计数量，又能按 ID 范围删除已读消息。'],
      ['Increment 如何增加未读数？', '对 userID-peerID 对应的 Sorted Set 执行 ZAdd，再用 ZCard 返回集合大小。消息 ID 作为 Member，重复写入相同消息不会重复增加集合元素。'],
      ['ClearConversation 如何清除已读消息？', '使用 ZRemRangeByScore 删除 Score 小于等于 throughMessageID 的元素，只清除已读范围，保留之后到达的未读消息。'],
      ['为什么批量查询未读数使用 Pipeline？', '联系人列表需要查询多个会话的 ZCard，Pipeline 可以把多个 Redis 命令批量发送，减少网络往返。'],
      ['限流 Lua 脚本做了什么？', '脚本执行 INCR，若计数为 1 则设置 PEXPIRE，最后返回计数；整个过程在 Redis 内原子执行，形成固定时间窗口限流。'],
      ['项目哪些接口使用限流？', '注册、登录、媒体上传、消息搜索和 WebSocket 发言都配置了限流；HTTP 接口常按客户端 IP，WebSocket 发言按用户 ID。'],
      ['Redis 故障时是否放行限流请求？', '项目 Allow 在 Redis 执行失败时返回错误，Handler 通常返回服务端错误，而不是无法确认限流状态时直接放行，安全上更保守。'],
    ],
  },
  {
    title: '七、Elasticsearch 搜索',
    items: [
      ['为什么要引入 Elasticsearch？', 'MySQL 适合事务和结构化查询，但消息关键词、前缀和特殊字符检索更适合 Elasticsearch 的倒排索引和查询 DSL。'],
      ['ensureIndex 的作用是什么？', '服务启动时检查索引是否存在；不存在则创建字段映射，已存在则补充 raw 搜索字段，并对历史文档做字段回填。'],
      ['content 和 content_raw 为什么同时保存？', 'content 使用 text 类型支持分词全文检索，content_raw 使用 keyword 保留原始字符，支持 ^、括号、通配符等特殊字符的精确匹配。'],
      ['Elasticsearch 查询如何做权限过滤？', 'SearchMessages 将当前 userID 放进 from_user_id 或 to_user_id 条件；指定 peerID 时再限定消息必须属于当前用户和该对方的双向会话。'],
      ['搜索使用了哪些查询方式？', '项目组合使用 multi_match、phrase_prefix 和 content_raw/file_name_raw 的 wildcard 查询，以兼顾全文、前缀和特殊字符检索。'],
      ['为什么要限制关键词最多 64 个 Unicode 字符？', '限制查询复杂度和请求资源，使用 utf8.RuneCountInString 按字符而不是 UTF-8 字节数统计，避免中文和 Emoji 被错误计数。'],
      ['搜索结果为什么还要调用 EnrichMessages？', 'Elasticsearch 里保存的是 object_key 等媒体元数据，私有 MinIO 的访问地址可能过期，因此搜索返回后重新生成最新预签名 URL。'],
      ['RabbitMQ 索引队列有什么意义？', '消息写入数据库后，通过异步事件建立 ES 索引，避免搜索索引操作阻塞实时消息主链路，同时可以失败重试。'],
      ['旧索引如何兼容新增 raw 字段？', 'ensureRawSearchFields 使用 PutMapping 增加字段，并通过 UpdateByQuery 脚本把已有文档的 content/file_name 回填到 raw 字段。'],
      ['Elasticsearch 索引失败会影响消息发送吗？', '不会直接阻塞 MySQL 消息保存和 WebSocket ack；索引事件在独立队列中失败后由消费者 Nack 重新入队，最终完成索引。'],
    ],
  },
  {
    title: '八、MinIO 媒体存储与上传安全',
    items: [
      ['MinIO 在项目中承担什么职责？', '存储图片、PDF、文本等媒体对象；MySQL 只保存对象 Key、文件名、大小和类型等元数据，避免将大文件直接存入数据库。'],
      ['New MinIO Uploader 初始化做什么？', '创建 MinIO Client，检查 Bucket 是否存在，不存在则创建，并设置私有 Bucket Policy；服务启动时完成存储基础配置。'],
      ['为什么 Bucket 要设置为私有？', '避免任何人拿到对象路径后直接匿名访问文件，所有文件访问都通过后端按需生成的短时预签名 URL。'],
      ['Upload 使用 io.Reader 有什么好处？', '上传方法接收流式 Reader，不需要把整个文件一次性读入内存，适合大文件和 HTTP multipart 数据流。'],
      ['为什么先读取文件前 512 字节？', 'HTTP DetectContentType 根据文件头特征识别真实 MIME 类型，可以减少仅依赖扩展名或客户端 Content-Type 带来的伪装风险。'],
      ['peek[:n] 与 file 为什么要拼接？', '读取前 512 字节进行检测后，file 流的位置已经向后移动；用 MultiReader 把已读前缀和剩余流拼回完整文件，避免上传文件缺少开头。'],
      ['allowedMediaType 采用什么策略？', '先去除 MIME 参数、去空格并转小写，再和 image/jpeg、image/png、image/gif、image/webp、application/pdf、text/plain 白名单比较。'],
      ['为什么要用 filepath.Base 清理文件名？', '去除客户端提交的目录路径，防止对象名携带 ../ 等路径穿越片段；项目还会替换空格并加时间戳生成对象 Key。'],
      ['预签名 URL 的有效期由什么决定？', '由 Uploader 初始化时注入的 urlTTL 决定；EnrichMessage 每次需要返回媒体时重新生成，避免把永久 URL 暴露给前端。'],
      ['上传成功但预签名失败为什么仍返回消息？', '文件和消息已经成功持久化，预签名属于访问地址补全步骤；项目记录日志并返回原消息，避免因临时 URL 服务异常否定已保存的业务结果。'],
    ],
  },
  {
    title: '九、Vue 3 前端与交互链路',
    items: [
      ['前端使用什么技术栈？', '前端使用 Vue 3、Composition API、Vite 和原生 Fetch/WebSocket API；生产构建输出到 web 目录，由 Go 静态文件服务托管。'],
      ['前端如何统一调用后端 API？', 'App.vue 中封装 api 函数，统一设置 Authorization Bearer Header、JSON Content-Type、解析 JSON 响应和转换错误信息。FormData 上传时不手动覆盖 Content-Type。'],
      ['前端如何恢复登录态？', 'Token 和用户信息保存到 localStorage，页面挂载时读取并判断 isAuthenticated；认证成功后连接 WebSocket、刷新用户列表并同步离线消息。'],
      ['WebSocket 前端如何自动选择协议？', '根据 window.location.protocol 判断当前页面是 HTTP 还是 HTTPS，分别使用 ws 或 wss，并通过当前 host 组成 WebSocket 地址。'],
      ['前端如何处理断线重连？', 'onclose 将 socketState 设置为 closed，并在 Token 仍存在时通过 setTimeout 延迟约 1.8 秒重新调用 connectSocket。'],
      ['前端如何避免实时消息重复显示？', 'appendMessage 会检查当前消息 ID 是否已经存在，只有不重复且属于当前会话的消息才加入数组，并按 ID 排序。'],
      ['前端如何实现离线增量同步？', '以 offline_cursor:<userID> 作为 localStorage Key 保存游标，循环调用 /api/offline?after_id=...，每批推进到最后一条 ID，直到返回空批次或不足一页。'],
      ['前端收到 ack 和 chat 有什么区别？', 'ack 表示发送方提交的消息已经被后端保存，chat 表示接收方通过异步推送收到消息；两者都调用 appendMessage，但 chat 还会刷新用户未读数。'],
      ['前端如何实现文件上传？', '用户选择文件后放入 FormData，附加 to_user_id 和 file 字段，调用 /api/media/upload；成功后把返回消息加入当前会话并清理文件选择状态。'],
      ['前端如何实现响应式布局？', 'CSS 使用网格和媒体查询：桌面端为成员、聊天、搜索三栏；窗口变窄时隐藏搜索栏；移动端切换为顶部横向成员列表和单列聊天区域。'],
    ],
  },
  {
    title: '十、工程化、测试与扩展问题',
    items: [
      ['项目如何进行本地环境编排？', 'Docker Compose 启动 MySQL、Redis、RabbitMQ、MinIO 和 Elasticsearch；PowerShell 脚本负责设置环境变量、等待基础设施就绪、执行数据库初始化并运行 Go 服务。'],
      ['健康检查接口检查什么？', 'healthz 使用 2 秒超时 Context，分别 Ping MySQL 和 Redis；任一依赖不可用就返回 503，全部可用才返回 status ok。'],
      ['项目如何实现优雅关闭？', 'App.Close 取消运行 Context，关闭 RabbitMQ Consumer、Publisher、Indexer、Redis 和 MySQL；Dispatcher 和消费者通过 Context 退出循环。'],
      ['项目有哪些测试方向？', '仓库包含配置校验测试、Hub 测试、Outbox Dispatcher 测试、Handler 限流测试等，重点覆盖配置、并发连接、可靠投递和接口边界。'],
      ['Noop 实现有什么作用？', '在未启用 RabbitMQ、MinIO 或 Elasticsearch 时提供空实现，使核心服务可以本地运行；同时便于单元测试时隔离外部依赖。'],
      ['如果要增加群聊，需要改哪些模块？', '需要新增群组和成员模型、权限校验、群消息路由和未读数维度；MessageCreatedEvent 增加群组信息，Hub 从单一 peer 投递扩展为成员集合投递，搜索权限也要按群成员过滤。'],
      ['如果要支持多节点，项目已有哪部分基础？', 'Redis 已记录 userID 到 nodeID 的在线映射，RabbitMQ 为每个 nodeID 建立专属队列，Hub 只向本节点连接推送，已经具备跨节点消息路由基础。'],
      ['当前项目的投递语义是 exactly-once 吗？', '不是。Transactional Outbox 和 Publisher Confirm 保障尽量不丢消息，但在 RabbitMQ 已确认、数据库 MarkPublished 失败时可能重复发布，因此是至少一次投递，消费者需要幂等。'],
      ['项目中有没有 LLM 或 Agent 功能？', '当前代码仓库没有直接实现 LLM API、Agent 工具调用或 RAG；这些可以作为后续扩展，例如通过独立服务消费消息事件，但不能把它们描述成当前项目已经实现的能力。'],
      ['你会如何继续优化这个项目？', '可以补充消息幂等键和消费去重、Outbox 最大重试次数与死信队列、RabbitMQ 连接重连、显式取消消费者、结构化日志和指标监控、WebSocket 多设备策略、数据库迁移工具以及更完整的集成测试。'],
    ],
  },
];

const allItems = sections.flatMap((section) => section.items);
if (allItems.length !== 100) {
  throw new Error(`Expected 100 questions, got ${allItems.length}`);
}

const normalRun = (text, options = {}) => new TextRun({
  text,
  font: 'Microsoft YaHei',
  size: options.size || 22,
  bold: options.bold || false,
  color: options.color,
});

const children = [];
children.push(new Paragraph({
  alignment: AlignmentType.CENTER,
  spacing: { after: 180 },
  children: [normalRun('IM 实时聊天系统', { size: 36, bold: true, color: '1F4E79' })],
}));
children.push(new Paragraph({
  alignment: AlignmentType.CENTER,
  spacing: { after: 320 },
  children: [normalRun('100 个项目面试题与参考答案', { size: 28, bold: true })],
}));
children.push(new Paragraph({
  shading: { fill: 'EAF3F8', type: ShadingType.CLEAR },
  spacing: { before: 120, after: 260 },
  children: [normalRun('覆盖 Go/Gin、Vue 3、WebSocket、MySQL、Redis、RabbitMQ、Transactional Outbox、Elasticsearch、MinIO、JWT 安全与 Docker 工程化。答案均基于当前项目代码和实际架构整理，面试时可结合具体文件与调用链展开。', { size: 21 })],
}));

let number = 1;
for (const section of sections) {
  children.push(new Paragraph({
    heading: HeadingLevel.HEADING_1,
    children: [normalRun(section.title, { size: 28, bold: true, color: '1F4E79' })],
  }));
  for (const [question, answer] of section.items) {
    children.push(new Paragraph({
      heading: HeadingLevel.HEADING_2,
      children: [normalRun(`${number}. 题目：${question}`, { size: 24, bold: true, color: '2F5597' })],
    }));
    children.push(new Paragraph({
      spacing: { after: 180 },
      children: [normalRun(`答案：${answer}`, { size: 21 })],
    }));
    number += 1;
  }
}

const doc = new Document({
  styles: {
    default: {
      document: {
        run: { font: 'Microsoft YaHei', size: 22 },
        paragraph: { spacing: { line: 300 } },
      },
    },
    paragraphStyles: [
      {
        id: 'Heading1', name: 'Heading 1', basedOn: 'Normal', next: 'Normal', quickFormat: true,
        run: { font: 'Microsoft YaHei', size: 28, bold: true, color: '1F4E79' },
        paragraph: { spacing: { before: 300, after: 180 }, outlineLevel: 0 },
      },
      {
        id: 'Heading2', name: 'Heading 2', basedOn: 'Normal', next: 'Normal', quickFormat: true,
        run: { font: 'Microsoft YaHei', size: 24, bold: true, color: '2F5597' },
        paragraph: { spacing: { before: 180, after: 100 }, outlineLevel: 1 },
      },
    ],
  },
  sections: [{
    properties: {
      page: {
        size: { width: 11906, height: 16838 },
        margin: { top: 1200, right: 1200, bottom: 1200, left: 1200 },
      },
    },
    footers: {
      default: new Footer({
        children: [new Paragraph({
          alignment: AlignmentType.CENTER,
          children: [normalRun('IM 实时聊天系统面试题　|　第 ', { size: 18, color: '808080' }), new TextRun({ children: [PageNumber.CURRENT], font: 'Microsoft YaHei', size: 18, color: '808080' })],
        })],
      }),
    },
    children,
  }],
});

Packer.toBuffer(doc).then((buffer) => {
  fs.writeFileSync(output, buffer);
  console.log(`Wrote ${output} with ${allItems.length} questions.`);
});

