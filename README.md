# SonicHub API

基于《SonicHub 技术架构设计 V1.1》《SonicHub 需求说明书 V1.1》和 25 屏交互原型实现的 Go 后端。

- 语言：Go 1.26
- HTTP 框架：go-zero rest
- 数据库：PostgreSQL（pgx）
- 鉴权：JWT Access Token + 轮换 Refresh Token
- 异步：数据库任务队列 + Worker（生成任务、失败重试、积分释放）
- 媒体：本地文件存储，内置 WAV 生成器，未接入 Python 模型时也能跑通完整创作流程

## 已实现能力

| 模块 | 能力 |
|---|---|
| 认证 | 手机号 + 验证码登录、自动注册、Refresh Token 轮换、退出登录 |
| 用户 | 个人资料、兴趣偏好、个人租户、积分账户 |
| 音乐人 | 认证申请、审核状态、音乐人主页、代表作 |
| 社交 | 关注/取消关注、粉丝、互关朋友、拉黑 |
| 作品 | 列表、详情、可见性、发布、删除、播放计数 |
| 互动 | 点赞、收藏、评论、删除评论 |
| 推荐 | 推荐流、关注流、广场（最新/热门/风格）、搜索、负反馈 |
| 写歌 | 报价、积分冻结、异步生成、进度查询、SSE、取消、结果作品 |
| 积分 | 套餐、充值（演示）、余额、不可变流水、失败/取消释放 |
| 消息 | 通知列表、未读、全部已读、会话、私信、歌曲卡片 |
| 分享 | 短链、可见性校验、落地页数据、分享事件、失效 |
| 审核 | 机审、人工通过/驳回、举报、申诉、音乐人审核、推荐策略 |
| 运维 | 健康检查、结构化日志、数据库迁移、演示种子数据 |

## 快速启动（Docker）

```bash
docker compose -f deploy/docker-compose.yml up --build
```

服务地址：`http://localhost:8080`，健康检查 `GET /health`。

首次启动会自动建表并写入与原型一致的演示数据。

### 演示账号

| 项目 | 值 |
|---|---|
| 手机号 | `13800138000` |
| 验证码 | `1234` |
| 角色 | 沈知，管理员，可访问审核工作台 |

## 本地运行

1. 启动 PostgreSQL，创建数据库 `sonichub`。
2. 复制 `.env.example` 并按需修改环境变量。
3. 执行：

```bash
go run ./cmd/server
```

迁移文件位于 `migrations/`，启动时自动执行；`SEED_DEMO=true` 时自动写入演示数据。

## API 示例

### 登录

```bash
curl -s http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"phone":"13800138000","code":"1234"}'
```

返回 `access_token`、`refresh_token` 与用户信息。

### 写歌完整链路

```bash
TOKEN="<access_token>"

# 1. 报价
curl -s http://localhost:8080/api/v1/credit/quotes \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"styles":["电子"],"duration_seconds":30,"quality":"高清","stems":true}'

# 2. 创建生成任务（quote_id 使用上一步返回值）
curl -s http://localhost:8080/api/v1/generations \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"quote_id":"<quote_id>","prompt":"雨后的城市，路灯把水洼照亮","styles":["电子"],"duration_seconds":30,"quality":"高清","stems":true,"title":"雨停之后","visibility":"public","auto_publish":true}'

# 3. 查询任务
curl -s http://localhost:8080/api/v1/generations/<id> \
  -H "Authorization: Bearer $TOKEN"

# 4. 发布作品（若 auto_publish=false）
curl -s -X POST http://localhost:8080/api/v1/works/<work_id>/publish \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"雨停之后","visibility":"public","tags":["电子","Lo-fi"]}'
```

Worker 会生成一个可播放的 WAV，写入 `/media/generated/<generation-id>.wav`，并完成积分捕获。

### 主要接口

| 方法 | 路径 |
|---|---|
| POST | `/api/v1/auth/login`、`/auth/refresh`、`/auth/logout` |
| GET/PUT | `/api/v1/me` |
| GET | `/api/v1/feed/recommend`、`/feed/following`、`/plaza`、`/search` |
| POST | `/api/v1/feed/feedback` |
| POST/DELETE | `/api/v1/users/{id}/follow` |
| GET | `/api/v1/me/following`、`/followers`、`/friends` |
| GET | `/api/v1/musicians/{id}` |
| POST | `/api/v1/musicians/applications` |
| GET | `/api/v1/works`、`/works/{id}`、`/works/{id}/comments` |
| POST | `/api/v1/works/{id}/like`、`/collect`、`/play`、`/comments`、`/publish` |
| PUT | `/api/v1/works/{id}/visibility` |
| POST | `/api/v1/credit/quotes`、`/credit/recharge` |
| GET | `/api/v1/me/credits`、`/me/credits/ledger`、`/credit/packages` |
| POST | `/api/v1/generations` |
| GET | `/api/v1/generations/{id}`、`/generations/{id}/events`（SSE） |
| GET | `/api/v1/notifications`、`/conversations`、`/conversations/{id}/messages` |
| POST | `/api/v1/conversations/{id}/messages` |
| POST | `/api/v1/share-links`、`/share-events` |
| GET | `/api/v1/s/{code}`（公开落地页数据） |
| POST | `/api/v1/reports`、`/appeals` |
| GET | `/api/v1/admin/stats`、`/admin/moderation`、`/admin/reports`、`/admin/applications`、`/admin/appeals`、`/admin/feed-policy` |

所有写接口返回 `{ "code": "OK", "message": "ok", "data": ... }`；错误返回对应 `code` 与 HTTP 状态码。

## 项目结构

```text
cmd/server/                  HTTP 服务入口
internal/api/                路由、中间件、Handler
internal/core/               报价、生成、发布、审核等业务逻辑
internal/model/              领域模型与 JSON DTO
internal/store/              PostgreSQL 查询与事务
internal/seed/               与原型一致的演示数据
internal/worker/             生成任务 Worker
internal/platform/auth/      JWT、Refresh Token
internal/platform/db/        连接、迁移
internal/platform/storage/   WAV 生成与媒体文件
migrations/                  建表 SQL
deploy/                      Dockerfile 与 Compose
```

## 实现边界

以下为 MVP 的明确取舍，生产化时按技术架构文档替换：

| 项 | 当前实现 | 生产目标 |
|---|---|---|
| 任务队列 | PostgreSQL `generations` + `FOR UPDATE SKIP LOCKED` | Redis Streams + Outbox + DLQ |
| 媒体存储 | 本地 `data/media` | S3 / MinIO，签名 URL |
| 模型调用 | 内置 WAV 合成器模拟模型 | Python 音乐大模型 + Model Gateway |
| 登录 | 固定演示验证码 `1234` | 短信/第三方登录 + 风控 |
| 支付 | 直接入账（演示） | 支付渠道回调、对账、退款 |
| 推荐 | SQL 热度排序 + 负反馈过滤 | 召回/排序服务、特征平台、A/B |
| 消息推送 | 仅站内通知 | APNs / FCM / 厂商推送 |

## 测试

```bash
go test ./...
```

端到端冒烟测试（需要 API 已启动）：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/smoke.ps1
```

已通过验证：

- `go build ./...`、`go vet ./...`、`go test ./...`
- 本地 PostgreSQL 18 端到端链路：登录 → 推荐/广场/搜索 → 关注 → 报价 → 生成 → 积分扣减 → 音频文件 → 点赞/评论 → 分享落地页 → 通知 → 管理台统计
- 生成任务成功：`succeeded`，生成 WAV 可访问（HTTP 200），冻结积分完成捕获，余额与流水一致

## 当前未覆盖

- 企业租户、成员 RBAC、支付渠道回调、退款对账
- APNs/FCM 等外部推送通道
- 离线特征、多路召回、A/B 实验平台
- 音乐人数据面板、歌单、话题活动、榜单、分享激励
- 真实音乐大模型调用（当前为内置 WAV 合成器）

## 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `HTTP_ADDR` | `:8080` | 监听地址 |
| `DATABASE_URL` | 本地 sonichub | PostgreSQL DSN |
| `JWT_SECRET` | 开发密钥 | 生产必须替换 |
| `DEMO_AUTH_CODE` | `1234` | 演示验证码 |
| `SEED_DEMO` | `true` | 空库时写入演示数据 |
| `ADMIN_PHONE` | `13800138000` | 管理员手机号 |
| `MEDIA_DIR` | `./data/media` | 媒体文件目录 |
| `MIGRATIONS_DIR` | `./migrations` | 迁移目录 |
| `ACCESS_TOKEN_MINUTES` | `1440` | Access Token 有效期 |
| `REFRESH_TOKEN_DAYS` | `30` | Refresh Token 有效期 |
