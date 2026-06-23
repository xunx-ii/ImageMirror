# ImageMirror

OpenAI 图像生成 API 中转/代理服务平台。后端使用 Go、PostgreSQL、Redis/Asynq、本地共享图片目录；前端使用 React、Vite、shadcn/ui。

## 快速启动

1. 复制环境变量：

```bash
cp .env.example .env
```

2. 修改 `.env` 中的 `JWT_SECRET` 和管理员账号密码。

3. 启动服务：

```bash
docker compose up --build
```

4. 打开 `http://localhost:21580`，默认管理员为 `.env` 中的 `ADMIN_EMAIL` / `ADMIN_PASSWORD`。

5. 登录后台后，在「管理 / OpenAI」中配置 `Base URL` 和 `API Key`。

## 本地开发

后端：

```bash
cd image-mirror-api
go run ./cmd/seed
go run ./cmd/api
go run ./cmd/worker
```

前端：

```bash
cd image-mirror-web
npm install
npm run dev
```

本地前端默认使用 `http://127.0.0.1:25173`，避免占用常见端口。

默认普通用户/API Key 限流为每分钟 120 次；管理后台接口不套用普通用户限流。

## 开发者 API

创建 API Key 后可同步调用：

```bash
curl http://localhost:21580/v1/images/generations \
  -H "Authorization: Bearer imk_xxx" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-image-2","prompt":"A minimal desk lamp product photo","size":"1024x1024","quality":"medium"}'
```

图片文件通过平台接口读取，后端会校验 JWT 或 API Key 权限；文件默认 24 小时过期，由 worker 定时清理。

## 水平扩展

API 进程是无状态的，可以多副本部署；Worker 可独立扩容。多主机部署时，`STORAGE_ROOT=/data/images` 必须挂载到同一个 NFS/NAS/CSI 共享卷，否则 API 实例可能读不到 worker 写入的文件。
