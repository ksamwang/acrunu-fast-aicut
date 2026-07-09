# acrunu-fast-aicut

基于自研 `LLM + VLM` 的短视频批量剪辑系统。

当前仓库处于开发阶段，核心目标是围绕素材库、素材分析、检索、文案、配音、编排与渲染，逐步打通可验证的业务闭环。

相关设计与任务文档：

- [系统设计总览](./docs/system-overview.md)
- [架构设计](./docs/architecture.md)
- [任务清单](./docs/task/README.md)

## 目录结构

- `apps/api`
  - 后端 API
- `apps/worker`
  - 异步任务 worker
- `apps/local-agent`
  - 浏览器配合使用的本地预处理 Agent
- `apps/web`
  - Web Console
- `internal/`
  - 业务模块与基础设施封装
- `migrations/`
  - PostgreSQL 数据库迁移
- `docs/`
  - 设计与实施文档

## 开发依赖

建议本机安装：

- Go 1.23+
- Node.js 20+
- npm
- ffmpeg
- ffprobe

如果本地 Agent 不走系统 PATH，可通过环境变量指定：

```powershell
$env:FFMPEG_PATH="E:\tools\ffmpeg\bin\ffmpeg.exe"
$env:FFPROBE_PATH="E:\tools\ffmpeg\bin\ffprobe.exe"
```

## 环境变量

先复制示例文件：

```powershell
Copy-Item .env.example .env
```

如果开发阶段直接连接服务器上的 PostgreSQL 和 Redis，建议至少设置：

```env
APP_ENV=development

API_ADDR=:8080
API_PORT=8080
WORKER_CONCURRENCY=4
LOCAL_AGENT_ADDR=127.0.0.1:58721
VITE_API_PROXY_TARGET=http://api.example.internal:8080

DATABASE_URL=postgres://<db-user>:<db-password>@192.168.1.10:5432/aicut?sslmode=disable
REDIS_ADDR=192.168.1.10:6379
QUEUE_BACKEND=redis

STORAGE_BACKEND=local
STORAGE_LOCAL_ROOT=./storage

VLM_PROVIDER=mock
MODEL_GATEWAY_TIMEOUT_SECONDS=120

FFMPEG_PATH=ffmpeg
FFPROBE_PATH=ffprobe
```

## 本机开发启动

当前推荐开发方式是：

- 服务器运行 PostgreSQL
- 服务器运行 Redis
- 服务器运行 `api`
- 服务器运行 `worker`
- 本机运行 `web`
- 本机按需运行 `local-agent`

这种模式更适合素材分析、抽帧和后续渲染链路，因为服务端 `api`、`worker` 和 `storage` 在同一台机器上，文件路径和任务消费不会跨机器错位。

### 本机启动 Web Console

```powershell
cd .\apps\web
npm install
npm run dev
```

前端开发服务器默认通过 Vite 代理将 `/api` 请求转发到 `VITE_API_PROXY_TARGET`。

连接服务器 API 时，根目录 `.env` 使用：

```env
VITE_API_PROXY_TARGET=http://api.example.internal:8080
```

临时切回本机 API 时，可以在当前 PowerShell 设置：

```powershell
$env:VITE_API_PROXY_TARGET="http://localhost:8080"
cd .\apps\web
npm run dev
```

### 本机启动 Local Agent

调试客户机素材预处理时，再启动本地 Agent：

```powershell
go run .\apps\local-agent\main.go
```

默认监听：

```text
127.0.0.1:58721
```

### 本机临时启动 API / worker

只有在调试纯 API、数据库读写或不依赖服务端素材文件的逻辑时，才建议本机临时运行：

```powershell
go run .\apps\api\main.go
```

```powershell
go run .\apps\worker\main.go
```

如果本机 `api` 和 `worker` 连接服务器 PostgreSQL / Redis，要特别注意 `storage` 路径仍然是本机路径，不适合跑完整素材抽帧和分析闭环。

## 调试注意事项

这种“本机 Web + 服务端 API/worker”的模式适合：

- 调 Web 页面
- 调 API
- 调任务状态流转
- 调素材入库后的抽帧和分析链路
- 调后续渲染链路

但需要注意：

- 改 Go 代码后需要重新部署或重建服务端 `api` / `worker`
- 本机前端代理目标由 `VITE_API_PROXY_TARGET` 控制
- 本机 `local-agent` 仍然访问客户机本地原始素材文件

如果需要本地完整离线调试，再切换为本地全套：

- 本地 PostgreSQL
- 本地 Redis
- 本地 storage
- 本地 API
- 本地 worker
- 本地 web

## 数据库迁移

当前迁移不是 API 启动时自动执行，需要手动运行。

安装 `goose`：

```powershell
go install github.com/pressly/goose/v3/cmd/goose@latest
```

执行迁移：

```powershell
$env:DATABASE_URL="postgres://<db-user>:<db-password>@192.168.1.10:5432/aicut?sslmode=disable"
goose -dir ./migrations postgres $env:DATABASE_URL up
```

## 测试与构建

后端测试：

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./...
```

前端构建：

```powershell
cd .\apps\web
npm run build
```

前端 E2E：

```powershell
cd .\apps\web
npm run test:e2e
```
