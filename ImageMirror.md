# ImageMirror 生图中转服务 — 实现计划

## Context

用户需要一个 OpenAI 图像生成 API 的中转/代理服务平台。用户在平台注册后，通过 Web UI 或开发者 API 调用 OpenAI 图像生成接口，平台按调用次数扣减预付积分。生成的图片存储在本地图片文件目录中，24h 后自动删除。系统需支持分布式水平扩展；多实例部署时，本地图片目录需使用共享挂载卷。

**技术选型**（已确认）：Go 后端 / React+Shadcn UI 前端 / PostgreSQL+sqlc / Redis+Asynq / 本地图片文件存储 / gpt-image-2 / 预付积分制

## 架构总览

```
用户/开发者 → Nginx(前端静态+API反代) → Go API(无状态,可多实例)
                                          ↓
                          PostgreSQL(持久化) + Redis(队列/缓存/限流)
                                          ↓
                      Go Worker进程(生成/清理) → 本地图片存储目录 + OpenAI API
```

**核心设计**：API 无状态、生成任务异步化(Asynq)、计费强一致(PG悲观锁)、图片文件落本地共享目录

## 项目结构

```
ImageMirror/
├── image-mirror-api/          # 后端 Go
│   ├── cmd/
│   │   ├── api/main.go        # API 入口
│   │   ├── worker/main.go     # Worker 独立入口
│   │   └── seed/main.go       # 初始数据(定价规则/管理员)
│   ├── db/
│   │   ├── migrations/        # PostgreSQL 迁移
│   │   ├── queries/           # sqlc SQL 查询
│   │   └── sqlc.yaml          # sqlc 配置
│   ├── internal/
│   │   ├── config/            # 环境变量配置
│   │   ├── http/              # router/middleware/handlers
│   │   ├── auth/              # JWT 认证(注册/登录/刷新)
│   │   ├── users/             # 用户资料
│   │   ├── api_keys/          # API Key 管理(SHA-256哈希)
│   │   ├── images/            # 图片生成核心(handler+service+task)
│   │   ├── billing/           # 积分扣减/退还/充值(事务+悲观锁)
│   │   ├── storage/           # 本地图片写入/删除/读取URL
│   │   ├── openai/            # OpenAI API 封装
│   │   ├── queue/             # Asynq 队列+清理Worker
│   │   ├── admin/             # 管理后台API
│   │   ├── pricing/           # 定价规则
│   │   └── system_config/     # 系统配置
│   ├── pkg/
│   │   ├── db/                # pgx/sqlc 连接封装
│   │   └── redis/             # Redis Client 封装
│   ├── go.mod
│   ├── Dockerfile
│   └── docker-entrypoint.sh   # 根据 APP_ROLE 启动 API 或 Worker
├── image-mirror-web/          # 前端 React+Vite+Shadcn
│   ├── src/
│   │   ├── pages/             # auth/dashboard/generate/gallery/api-keys/billing/admin
│   │   ├── components/        # ui(Shadcn)/layout/shared
│   │   ├── api/               # Axios 封装+各模块API
│   │   ├── hooks/             # use-auth/use-image-generation
│   │   └── stores/            # Zustand 状态管理
│   ├── Dockerfile
│   └── nginx.conf             # 静态文件+/api+/v1 反代
├── docker-compose.yml         # postgres+redis+api+worker+web+image-storage volume
└── .env.example
```

## 数据库 Schema (PostgreSQL)

核心模型：
- **User**: id, email, passwordHash, role(USER/ADMIN), status, balance(积分), lastLoginAt
- **ApiKey**: id, userId, name, keyPrefix(展示用), keyHash(SHA-256,唯一), status(ACTIVE/REVOKED), lastUsedAt
- **ImageGeneration**: id, userId, apiKeyId, model, prompt, size, quality, status(PENDING/PROCESSING/COMPLETED/FAILED/EXPIRED), storageKey, storageUrl, creditsCost, expiresAt(创建+24h), deletedAt
- **CreditTransaction**: id, userId, type(RECHARGE/CONSUME/REFUND/ADMIN_ADJUST), amount, balanceAfter, description, relatedId
- **PricingRule**: id, model, size, quality, credits, isActive — 唯一约束(model,size,quality)
- **SystemConfig**: id, key(唯一), value, updatedBy — 存储本地图片目录/OpenAI配置等

**默认定价**：仅启用 `gpt-image-2`。按 `size + quality` 维护可配置 PricingRule，默认积分值在 seed 中初始化，后续由管理后台按实际 OpenAI 成本调整。

## API 接口设计

### Web UI API (JWT认证, `/api` 前缀)
- `POST /api/auth/register|login|refresh` — 认证
- `GET|PATCH /api/users/me` — 用户资料
- `GET|POST|DELETE /api/api-keys` — API Key 管理
- `POST /api/images/generate` — 提交生成(异步,返回imageId)
- `GET /api/images` — 图片列表(分页)
- `GET /api/images/:id` — 图片详情(含图片访问URL)
- `GET /api/images/:id/status` — 轮询生成状态
- `GET /api/images/:id/file` — 读取本地图片文件
- `GET /api/billing/balance|transactions` — 余额/交易记录
- `POST /api/billing/recharge` — 模拟充值
- `GET /api/pricing` — 价格表

### 开发者 API (API Key认证, `/v1` 前缀, 兼容OpenAI风格)
- `POST /v1/images/generations` — 同步生成(阻塞等待,超时300s)
- `GET /v1/images/:id` — 查询图片
- `GET /v1/images/:id/file` — 读取本地图片文件
- `GET /v1/models` — 可用模型
- `GET /v1/billing/balance|usage` — 余额/用量

### 管理后台 API (JWT+Admin, `/api/admin` 前缀)
- `GET|PATCH /api/admin/users` — 用户管理(封禁/调整余额)
- `GET|POST|PUT /api/admin/pricing` — 定价配置
- `GET|PUT /api/admin/config` — 系统配置
- `GET /api/admin/stats/overview|daily` — 统计数据

## 关键流程设计

### 1. 图片生成流程
```
请求 → 认证&校验 → 查询定价(Redis缓存) → 预扣积分(PG事务+SELECT FOR UPDATE)
     → 创建记录(status=PENDING, expiresAt=now+24h) → 入队Asynq
     → [Web UI: 返回imageId,前端轮询] / [API: 阻塞等待任务完成]
     → Worker处理: 调OpenAI(gpt-image-2, b64_json) → 写入本地图片目录 → 更新COMPLETED
     → 失败时: 标记FAILED + 退还积分
```

### 2. 计费并发安全
- `BillingService.DeductCredits()`: PG事务内 `SELECT balance FROM users WHERE id=$1 FOR UPDATE` 行级悲观锁
- 余额不足 → 回滚 → 返回 402
- 扣减成功 → 记录 CreditTransaction
- 失败退还使用相同事务机制

### 3. 24h 图片清理
- `CleanupTask`: Asynq Scheduler 每小时执行
- 查询 `status=COMPLETED AND expiresAt<NOW() AND deletedAt IS NULL` LIMIT 100
- 批量删除本地图片文件 → 更新 status=EXPIRED, deletedAt=NOW()
- 生产环境可增加系统 cron 兜底扫描孤儿文件

## 本地图片存储设计

- `STORAGE_ROOT=/data/images`：图片文件根目录，API 与 Worker 都挂载同一目录
- `storageKey`：按日期和用户分片生成相对路径，例如 `2026/06/23/{userId}/{imageId}.png`
- `storageUrl`：返回平台内部图片读取地址，例如 `/api/images/{id}/file` 或 `/v1/images/{id}/file`
- 单机部署可使用 Docker named volume；多机水平扩展必须切换为 NFS/NAS/CSI 等共享挂载卷
- API 读取文件时校验用户/API Key 权限，避免直接暴露本地文件路径

## Redis 使用场景
| 场景 | Key 模式 | 说明 |
|------|---------|------|
| Asynq 队列 | `asynq:image-generation` / `asynq:cleanup` | 生成任务+定时清理 |
| Refresh Token | `refresh:{userId}:{tokenId}` TTL=7d | 登出时删除实现主动失效 |
| API Key 缓存 | `apikey:{keyHash}` TTL=5min | 避免每次查DB,吊销时清除 |
| 定价规则缓存 | `pricing:rules` (Hash) | 修改后刷新 |
| 系统配置缓存 | `config:all` (Hash) | 修改后刷新 |
| API 限流 | `ratelimit:{userId}:{window}` | 滑动窗口,每用户每分钟10次 |
| 幂等防重 | `idempotency:{userId}:{hash}` TTL=30s | 防重复提交 |

## 前端页面
| 路由 | 页面 | 说明 |
|------|------|------|
| /login, /register | 认证页 | 登录注册 |
| /dashboard | 仪表盘 | 余额/用量概览 |
| /generate | 生成工作台 | 左参数面板+右结果区,轮询状态 |
| /gallery | 我的图片 | 网格卡片+24h倒计时+下载 |
| /api-keys | API Key管理 | 创建后弹窗展示明文(仅一次) |
| /billing/recharge | 充值 | 模拟充值 |
| /billing/transactions | 交易记录 | 表格+筛选+分页 |
| /admin/* | 管理后台 | 用户/定价/配置/统计 |

## Docker 部署

**docker-compose.yml** 服务编排：
- `postgres:16-alpine` — 数据库
- `redis:7-alpine` — 缓存/队列
- `api` — Go API(可 `replicas: N` 水平扩展)
- `worker` — 独立 Go Worker 进程(根据 `APP_ROLE=worker` 启动)
- `web` — Nginx(前端静态 + /api + /v1 反代, `/v1` 超时300s)
- `image-storage` — 本地图片存储卷，挂载到 API 与 Worker 的 `/data/images`

**分布式扩展**：API/Worker 业务状态在 PG/Redis 中，可多副本；PG 用 PgBouncer 管理连接；Redis 用 Sentinel 高可用；图片目录在多主机部署时使用共享挂载卷

## 实现步骤

### 阶段一：基础框架 (P0)
1. 初始化 Go 项目 + go.mod + Gin/Chi 路由 + golangci-lint 配置
2. docker-compose.yml (PostgreSQL + Redis + image-storage volume)
3. PostgreSQL migrations + sqlc + seed (定价规则/管理员)
4. pgx/sqlc DB 封装 + Redis Client 封装
5. 全局错误处理中间件 + 统一响应结构 + 请求校验

### 阶段二：用户认证 (P0)
6. Auth 模块: 注册/登录(bcrypt+JWT) + JWT中间件
7. Refresh Token (Redis白名单)
8. Users 模块: 资料 CRUD
9. 前端初始化 (Vite+React+Shadcn UI) + 登录注册页 + 路由守卫

### 阶段三：计费系统 (P0)
10. Billing 模块: 积分扣减(事务+悲观锁)/退还/充值
11. Pricing 模块: 定价查询 + Redis缓存
12. 前端仪表盘 + 充值 + 交易记录页

### 阶段四：图片生成核心 (P0)
13. Storage 模块: 本地图片写入/删除/读取URL
14. OpenAI 模块: 封装 images.generate (`gpt-image-2`)
15. Queue 模块: Asynq + ImagesProcessor
16. Images 模块: Web UI 生成API (入队+状态轮询)
17. 前端生成工作台 + 我的图片页

### 阶段五：API Key 与开发者API (P0)
18. ApiKeys 模块: 创建/吊销/列表 (SHA-256哈希)
19. ApiKey 中间件 + Redis缓存
20. 开发者 API `/v1/images/generations` (同步等待模式)
21. 前端 API Key 管理页

### 阶段六：24h清理与限流 (P0-P1)
22. CleanupTask: 定时扫描+批量删除本地图片文件
23. Asynq Scheduler 注册定时任务
24. API 限流中间件 (Redis滑动窗口)

### 阶段七：管理后台 (P1)
25. Admin 中间件 + Admin 模块
26. 用户管理 + 定价配置 + 系统配置
27. 统计数据 + 前端管理后台页面

### 阶段八：生产化 (P1-P2)
28. 健康检查 `/health`
29. 结构化日志 (zap/slog)
30. Dockerfile 优化 + docker-compose 生产配置
31. .env.example 文档

## 验证方案

1. **本地启动**: `docker compose up -d` 启动全部服务
2. **数据库**: `migrate -path db/migrations -database "$DATABASE_URL" up && go run ./cmd/seed` 初始化
3. **认证流程**: 注册用户 → 登录获取JWT → 调用 /api/users/me
4. **生成流程**: Web UI 输入prompt → 选择 `gpt-image-2` → 生成 → 查看图片 → 24h后确认删除
5. **计费流程**: 充值 → 生成扣费 → 查看交易记录 → 余额不足时被拒绝
6. **开发者API**: 创建API Key → curl 调用 /v1/images/generations → 确认返回图片URL
7. **清理任务**: 生成图片 → 手动修改 expiresAt 为过去时间 → 等待清理任务执行 → 确认本地图片文件已删
8. **管理后台**: 管理员登录 → 调整用户余额 → 修改定价 → 修改存储配置
