# Star-IM · 星讯

即时通讯系统：Go + Vue 3，支持单聊、群聊，内置带联网搜索能力的 AI 聊天好友。

## 功能

- 单聊 / 群聊 / 好友与社交关系
- **AI 好友**：联网搜索（实时新闻）、实时天气、日期时间查询
- WebSocket 实时推送，Outbox + RabbitMQ 可靠消息投递
- Elasticsearch 消息全文搜索
- 图片/文件上传（MinIO）、离线消息、未读数、消息回执

## 技术栈

Go (Gin) · Vue 3 (Vite) · MySQL · Redis · RabbitMQ · MinIO · Elasticsearch

## 快速开始

前置条件：Go 1.22+、Docker Desktop（Linux 容器模式）、PowerShell。

**一键启动**（自动拉起 MySQL/Redis/RabbitMQ/MinIO/Elasticsearch 并运行服务）：

```powershell
.\scripts\Start-Dev.ps1 -WithInfra
```

基础设施已在运行时，只启动服务：

```powershell
.\scripts\Start-Dev.ps1
```

启动后访问 **http://127.0.0.1:8080**，注册账号即可使用。

### 启用 AI 联网搜索好友

先设置 DeepSeek API Key，再启动：

```powershell
$env:IM_AI_API_KEY = "你的 DeepSeek API Key"
.\scripts\Start-Dev.ps1
```

新注册用户会自动与「AI 好友」成为好友，可直接聊天；老用户重启服务后好友列表也会自动出现 AI 好友。

### 常用命令

| 操作 | 命令 |
| --- | --- |
| 停止基础设施 | `docker compose down` |
| 重置数据库（清空数据） | `.\scripts\Reset-Db.ps1` |
| 构建前端（输出到 web/） | `cd frontend && npm run build` |
| 运行测试 | `go test ./...` |

## 主要配置

完整配置见 `scripts/Set-DevEnv.ps1`（开发默认值）。常用 AI 相关变量：

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `IM_AI_ENABLED` | true | AI 好友开关 |
| `IM_AI_API_KEY` | 空 | DeepSeek API Key（必填） |
| `IM_AI_BASE_URL` | https://api.deepseek.com/v1 | OpenAI 兼容接口地址 |
| `IM_AI_MODEL` | deepseek-chat | 模型名称 |
| `IM_AI_ENABLE_SEARCH` | true | 联网搜索开关（仅 DeepSeek 支持） |
| `IM_AI_BOT_NICKNAME` | AI 好友 | 机器人昵称 |

## 默认端口

| 服务 | 地址 |
| --- | --- |
| 应用 | http://127.0.0.1:8080 |
| MySQL | 127.0.0.1:13306 |
| Redis | 127.0.0.1:16379 |
| RabbitMQ | 15673（AMQP）/ 15674（管理页） |
| MinIO | 19000（API）/ 19001（Console） |
| Elasticsearch | http://127.0.0.1:19200 |

## 项目结构

- `cmd/server` — Go 服务入口
- `internal` — 业务代码（app / handler / service / repository / ai / mq / outbox 等）
- `frontend` — Vue 3 前端源码
- `db` — 数据库 schema 与迁移脚本
- `scripts` — 开发脚本（启动 / 重置 / 环境变量）

## 常见问题

- **Docker Desktop 未启动**：先启动 Docker，等待引擎就绪再执行脚本。
- **8080 被占用 / 消息收不到**：同时跑多个服务进程会抢消息队列导致推送丢失。只保留一个：`Get-Process server | Stop-Process`。
- **数据库字段缺失**：重新执行 `.\scripts\Start-Dev.ps1 -WithInfra` 或 `Reset-Db.ps1`（会清数据）。

## 安全说明

开发环境使用默认凭据；生产部署务必通过环境变量覆盖 `IM_JWT_SECRET`、数据库与 MinIO 凭据，并启用 TLS。
