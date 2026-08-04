# IM Chat System

一个使用 Go 与 Gin 实现的即时通讯示例项目。它提供用户认证、单聊、WebSocket 实时推送、离线消息、文件上传和消息搜索，并将 MySQL、Redis、RabbitMQ、MinIO 与 Elasticsearch 组合到一套可本地运行的服务中。配套的“星讯”前端使用 Vue 3 + Vite 构建。

## 功能

- 用户注册、登录、登出与 JWT 身份认证
- 单聊文本消息与 WebSocket 实时推送
- 会话历史与离线消息拉取
- Redis 未读消息计数、在线状态、Token 黑名单和限流
- 图片、文件上传与 MinIO 临时访问链接
- RabbitMQ 异步消息事件与 Outbox 可靠投递
- Elasticsearch 消息关键词与颜文字/特殊字符搜索
- “星讯”Vue 3 + Vite 前端：欢迎页、三栏单聊工作台、搜索、上传与离线同步

## 技术栈

| 类别 | 技术 |
| --- | --- |
| 服务端 | Go 1.22、Gin、Gorilla WebSocket |
| 主数据 | MySQL 8 |
| 即时状态 | Redis 7 |
| 异步事件 | RabbitMQ 3 |
| 文件存储 | MinIO |
| 搜索 | Elasticsearch 8 |
| 本地编排 | Docker Compose |
| 前端 | Vue 3、Vite、CSS 动效 |

## 架构

~~~text
浏览器
  │ HTTP / WebSocket
  ▼
Handler / chat.Hub
  ▼
AuthService / MessageService
  ├── MySQL：用户、消息、Outbox 事件
  ├── Redis：在线状态、未读数、限流、Token 黑名单
  └── MinIO：文件对象存储

MySQL Outbox
  ▼
Outbox Dispatcher
  ▼
RabbitMQ message.created
  ├── 节点消费者 → WebSocket 实时推送
  └── 搜索消费者 → Elasticsearch 建立消息索引
~~~

消息先与 Outbox 事件在同一 MySQL 事务中写入，再由后台任务发布 RabbitMQ。这样即使 RabbitMQ 短暂不可用，已保存的消息也不会丢失，之后仍可重试发布。

## 项目结构

~~~text
cmd/server/                    程序入口
internal/app/                  依赖装配、Gin 路由、健康检查与关闭逻辑
internal/auth/                 密码哈希与 JWT
internal/chat/                 WebSocket Hub、Client、读写协程
internal/config/               环境变量配置与校验
internal/handler/              HTTP Handler、认证和限流中间件
internal/httpx/                JSON 响应辅助函数
internal/model/                User、Message 领域模型
internal/mq/                   RabbitMQ 发布者、消费者及事件类型
internal/outbox/               Outbox 调度与重试
internal/presence/             在线状态接口与 Redis 实现
internal/ratelimit/            限流接口与 Redis 实现
internal/repository/           Repository 接口与 MySQL 实现
internal/search/               搜索接口与 Elasticsearch 实现
internal/service/              认证和消息业务逻辑
internal/storage/              文件存储接口与 MinIO 实现
internal/tokenblacklist/       Token 黑名单接口与 Redis 实现
internal/unread/               未读数接口与 Redis 实现
db/schema.sql                  初始数据库结构
db/migrations/                 数据库迁移脚本
scripts/                       本地开发脚本
frontend/                      Vue 3 + Vite 前端源码
web/                           Vite 生产构建产物，由 Go 静态托管
~~~

## 快速开始

### 前置条件

- Go 1.22 或更高版本
- Docker Desktop，且已启动 Linux container engine
- Windows PowerShell

### 一键启动

在项目根目录执行：

~~~powershell
.\scripts\Start-Dev.ps1 -WithInfra
~~~

脚本会按以下顺序执行：

1. 加载开发环境变量；
2. 启动 MySQL、Redis、RabbitMQ、MinIO、Elasticsearch；
3. 等待每个基础设施就绪；
4. 执行数据库结构脚本；
5. 运行 Go 服务。

启动成功后，打开：

- 应用页面：http://127.0.0.1:8080
- 健康检查：http://127.0.0.1:8080/healthz
- RabbitMQ 管理页：http://127.0.0.1:15674
- MinIO Console：http://127.0.0.1:19001

### 只启动 Go 服务

若基础设施已经在运行：

~~~powershell
.\scripts\Start-Dev.ps1
~~~

也可以手动加载环境变量后运行：

~~~powershell
. .\scripts\Set-DevEnv.ps1
go run .\cmd\server
~~~

### 停止基础设施

~~~powershell
docker compose down
~~~

该命令不会删除 Docker volume 中的数据。若需要清空容器数据，请先确认目标后再自行执行带 volume 的 Docker 清理命令。

### 重置数据库

~~~powershell
.\scripts\Reset-Db.ps1
~~~

该脚本会重建本地 im_chat 数据库，原有数据库数据会丢失。

## 默认端口

| 服务 | 地址 |
| --- | --- |
| 应用 | http://127.0.0.1:8080 |
| MySQL | 127.0.0.1:13306 |
| Redis | 127.0.0.1:16379 |
| RabbitMQ AMQP | 127.0.0.1:15673 |
| RabbitMQ 管理页 | http://127.0.0.1:15674 |
| MinIO API | 127.0.0.1:19000 |
| MinIO Console | http://127.0.0.1:19001 |
| Elasticsearch | http://127.0.0.1:19200 |

## 配置

开发环境脚本会设置以下默认值。生产环境应通过部署平台的环境变量覆盖敏感配置，尤其是 JWT、数据库和 MinIO 凭据。

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| IM_ADDR | :8080 | HTTP 服务监听地址 |
| IM_NODE_ID | node-1 | 当前应用节点标识 |
| IM_ENV | development | 运行环境 |
| IM_JWT_SECRET | 开发用长密钥 | JWT 签名密钥，至少 32 个字符 |
| IM_TOKEN_TTL_HOURS | 168 | JWT 有效期，单位小时 |
| IM_ALLOWED_ORIGINS | http://127.0.0.1:8080,http://localhost:8080,http://127.0.0.1:5173,http://localhost:5173 | WebSocket Origin 白名单，含 Vite 开发地址 |
| IM_MAX_UPLOAD_BYTES | 10485760 | 单个上传文件最大大小，默认 10 MiB |
| IM_MYSQL_DSN | root:123456@tcp(127.0.0.1:13306)/im_chat?... | MySQL DSN |
| IM_REDIS_ADDR | 127.0.0.1:16379 | Redis 地址 |
| IM_ENABLE_RABBITMQ | true | 是否启用 RabbitMQ |
| IM_RABBITMQ_URL | amqp://guest:guest@127.0.0.1:15673/ | RabbitMQ 地址 |
| IM_ENABLE_MINIO | true | 是否启用 MinIO |
| IM_MINIO_ENDPOINT | 127.0.0.1:19000 | MinIO 地址 |
| IM_MINIO_BUCKET | im-chat | MinIO Bucket 名称 |
| IM_ENABLE_ELASTICSEARCH | true | 是否启用 Elasticsearch |
| IM_ELASTICSEARCH_URL | http://127.0.0.1:19200 | Elasticsearch 地址 |
| IM_ELASTICSEARCH_INDEX | messages | 消息索引名称 |

注意：

- 启用 Elasticsearch 时必须同时启用 RabbitMQ，因为消息通过事件异步建立索引。
- Origin 不允许使用通配符。
- 生产环境启用 MinIO 时必须使用 TLS 和非默认凭据。

## HTTP API

除注册和登录外，接口都需要请求头：

~~~text
Authorization: Bearer <JWT>
~~~

所有成功响应均为 JSON；错误响应格式为：

~~~json
{"error":"错误说明"}
~~~

### 认证

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | /api/register | 注册用户，按客户端 IP 限流 |
| POST | /api/login | 登录并取得 JWT，按客户端 IP 限流 |
| POST | /api/logout | 登出并将当前 JWT 加入黑名单 |
| GET | /api/me | 获取当前用户信息 |

注册请求：

~~~json
{
  "username": "alice",
  "password": "password123",
  "nickname": "Alice"
}
~~~

登录请求：

~~~json
{
  "username": "alice",
  "password": "password123"
}
~~~

登录响应：

~~~json
{
  "token": "<JWT>",
  "user": {
    "id": 1,
    "username": "alice",
    "nickname": "Alice",
    "created_at": "2026-07-28T00:00:00Z"
  }
}
~~~

### 用户与消息

| 方法 | 路径 | 查询参数 | 说明 |
| --- | --- | --- | --- |
| GET | /api/users | 无 | 获取其他用户及对应未读数 |
| GET | /api/messages | peer_id、after_id、limit | 获取与指定用户的会话历史 |
| GET | /api/offline | after_id、limit | 获取当前用户的离线消息 |

其中 limit 默认 50，最大 100。after_id 可用于增量拉取。

### 媒体上传

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | /api/media/upload | 上传文件并创建一条媒体消息 |

请求类型为 multipart/form-data：

~~~text
to_user_id=<接收者用户 ID>
file=<文件>
~~~

允许的内容类型：

- image/jpeg
- image/png
- image/gif
- image/webp
- application/pdf
- text/plain

服务端会校验真实内容类型，并限制文件大小。媒体消息中的 object_url 是短期有效的 MinIO 预签名访问链接。

### 搜索

| 方法 | 路径 | 查询参数 | 说明 |
| --- | --- | --- | --- |
| GET | /api/search/messages | q、peer_id、limit | 搜索当前用户有权限查看的消息 |

- q 最多 64 个 Unicode 字符；
- peer_id 可选，用于限定某个会话；
- limit 默认 50，最大 50；
- 搜索结果来自 Elasticsearch，消息刚保存后可能需要等待异步索引完成。
- 消息内容同时保存全文索引与原始字符字段，因此可搜索颜文字和特殊符号，例如 ^、^(*￣(oo)￣)^。

首次启动更新后的服务时，会自动为已有 Elasticsearch 文档补齐原始字符字段；数据量较大时该过程可能需要少量时间。

## WebSocket 协议

连接地址：

~~~text
ws://127.0.0.1:8080/ws
~~~

浏览器需要将 JWT 放入 WebSocket 子协议列表：

~~~javascript
const ws = new WebSocket(
  "ws://127.0.0.1:8080/ws",
  ["im-chat", token]
);
~~~

服务端会验证：

1. Token 是否存在、有效且未被拉黑；
2. 浏览器 Origin 是否在 IM_ALLOWED_ORIGINS 白名单中；
3. 当前用户的消息发送频率是否超限。

客户端发送文本消息：

~~~json
{
  "type": "chat",
  "to": 2,
  "content": "你好"
}
~~~

服务端下行消息：

~~~json
{
  "type": "ack",
  "message": {
    "id": 101,
    "from_id": 1,
    "to_id": 2,
    "content_type": "text",
    "content": "你好",
    "created_at": "2026-07-28T00:00:00Z"
  }
}
~~~

type 字段含义：

| type | 含义 |
| --- | --- |
| ack | 发送者的消息已成功保存 |
| chat | 接收者收到一条实时聊天消息 |
| error | 协议、限流或保存错误 |

服务端会定期发送 Ping；浏览器会自动回复 Pong，以检测失效连接并维持 Redis 在线状态。

## 消息流转

~~~text
发送者 WebSocket
  → Client.readPump
  → Redis 用户限流
  → MessageService.SaveText
  → MySQL：messages + outbox_events（同一事务）
  → Redis：增加接收者未读数
  → 发送者收到 ack

Outbox Dispatcher
  → RabbitMQ message.created
  ├→ 接收者所在节点 Hub：WebSocket 实时推送
  └→ Elasticsearch：建立搜索索引
~~~

接收者离线时不会执行实时推送，但消息仍保存在 MySQL，可通过会话历史或离线消息接口获取。

## 前端开发

前端源码位于 frontend 目录，使用 Vue 3 和 Vite；生产构建产物输出到 web 目录，仍由 Go 服务在 8080 端口提供。“星讯”包含独立欢迎页、桌面三栏单聊工作台、昵称字母头像、文件上传、消息搜索和离线同步。

先启动后端及基础设施：

~~~powershell
.\scripts\Start-Dev.ps1 -WithInfra
~~~

再在另一个终端启动 Vite 开发服务器：

~~~powershell
cd frontend
npm install
npm run dev
~~~

打开 http://127.0.0.1:5173。Vite 会将 /api 请求和 /ws WebSocket 连接代理到 http://127.0.0.1:8080。

构建生产前端：

~~~powershell
cd frontend
npm run build
~~~

该命令会更新 web 目录中的静态资源。

## 测试

~~~powershell
. .\scripts\Set-DevEnv.ps1
go test ./...
~~~

项目将 Go 构建与模块缓存配置在项目内的 .gocache 和 .gomodcache 目录中，避免影响其他本地项目。

## 常见问题

### Docker Desktop 未启动

现象：Start-Dev.ps1 提示 Docker engine 不可用。

处理：启动 Docker Desktop，等待状态变为 Engine running 后重新执行启动脚本。

### 服务端启动时连接 MySQL 或 Redis 失败

现象：启动日志出现数据库或 Redis 连接错误。

处理：

1. 使用带 -WithInfra 的启动命令；
2. 确认 Docker 容器已启动；
3. 确认没有修改 Set-DevEnv.ps1 中的端口；
4. 访问 /healthz 验证 MySQL 与 Redis。

### 数据库字段或表不存在

对于已有旧数据库，先执行：

~~~text
db/migrations/001_add_outbox_events.sql
~~~

本地开发环境也可以使用 Reset-Db.ps1 重建数据库；该操作会删除原有数据。

### 无法建立 WebSocket

检查以下内容：

1. 是否已登录并传入有效 JWT；
2. 是否使用子协议列表 ["im-chat", token]；
3. 当前页面的 Origin 是否包含在 IM_ALLOWED_ORIGINS；
4. Token 是否已经调用登出接口而进入黑名单。

## 安全说明

- 不要在生产环境使用仓库中的开发 JWT 密钥、MySQL 密码或 MinIO 默认凭据。
- JWT 密钥至少 32 个字符，并应通过密钥管理系统配置。
- MinIO Bucket 保持私有，媒体文件通过短时预签名链接访问。
- HTTP 受保护接口使用 Bearer Token；WebSocket 使用子协议携带 Token。
- 限流、Token 黑名单、Origin 白名单仅是基础防护，生产部署还应配置 HTTPS、反向代理、日志、监控和备份。
