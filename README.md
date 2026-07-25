# acrunu-fast-aicut

[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](./LICENSE)

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
$env:FFMPEG_PATH="C:\Tools\ffmpeg\bin\ffmpeg.exe"
$env:FFPROBE_PATH="C:\Tools\ffmpeg\bin\ffprobe.exe"
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
API_BIND_ADDR=127.0.0.1
API_PORT=10101
WEB_BIND_ADDR=0.0.0.0
WEB_PORT=10100
WORKER_CONCURRENCY=4
LOCAL_AGENT_ADDR=127.0.0.1:58721
VITE_API_PROXY_TARGET=http://video.example.com:10100

DATABASE_URL=postgres://<db-user>:<db-password>@db.example.internal:5432/<db-name>?sslmode=disable
REDIS_ADDR=redis.example.internal:6379
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
- 服务器运行容器化 `web`
- 本机修改前端时可按需运行 Vite 开发服务器
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
VITE_API_PROXY_TARGET=http://video.example.com:10100
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

### 构建和发布 Windows Local Agent

安装 Inno Setup 6 后执行：

```powershell
.\scripts\build-local-agent-installer.ps1 -Version 0.1.0
```

脚本会构建无控制台窗口的托盘程序，完整打包 `ffmpeg.exe`、`ffprobe.exe`，并生成安装包和 `release.json`：

```text
storage/client-releases/local-agent/windows-x64/
```

发布到服务器持久化存储：

```powershell
.\scripts\publish-local-agent.ps1
```

服务端通过 `/api/client-releases/local-agent/latest` 返回当前版本，通过同源下载接口提供安装包。安装后注册 `acrunu-fastcut://launch`，Local Agent 登录自启动并仅显示托盘图标。

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

## 服务端部署

开发阶段可以用脚本将当前已提交的代码同步到服务器，并按需重建服务端容器。

先提交本地改动：

```powershell
git status
git add .
git commit -m "更新说明"
```

然后在本机 PowerShell 中显式指定部署目标：

```powershell
.\scripts\deploy-server.ps1 `
  -HostName "server.example.com" `
  -UserName "deploy" `
  -RemoteDir "/opt/acrunu-fast-aicut"
```

部署参数说明：

- `HostName`：服务器域名或 IP
- `UserName`：SSH 用户
- `RemoteDir`：服务器上的项目目录
- 重建服务：`api`、`worker`、`web`

示例 Web 入口：

```text
http://video.example.com:10100
```

未使用 `80/443` 端口时，访问地址必须显式携带 Web 服务端口。DNS 只负责域名解析，不能代替客户端指定端口。

前端使用独立 Docker 镜像构建，并由 `web` 容器内的 Nginx 提供静态页面，同时将同源 `/api` 和 `/storage` 请求转发到 Docker 网络中的 `api:8080`。服务器宿主机不需要安装或配置 Nginx。API 的宿主机端口默认仅绑定回环地址：

```text
127.0.0.1:10101 -> api:8080
```

服务器防火墙只需向业务用户开放 TCP `10100`；PostgreSQL、Redis、API、ASR、TTS 和 renderer 不需要对公网开放。部署脚本不会同步服务器 `.env`，首次切换容器化前端时必须在服务器远程目录的 `.env` 中设置：

```env
API_BIND_ADDR=127.0.0.1
API_PORT=10101
WEB_BIND_ADDR=0.0.0.0
WEB_PORT=10100
```

服务器位于 NAT 网关后时，需要在公网网关配置 TCP 端口映射。例如：

```text
公网 203.0.113.10:10100 -> 192.168.1.10:10100
```

如果网关不支持 NAT 回环，局域网设备直接访问公网域名可能失败。可以使用内外网分离 DNS，让局域网将业务域名解析到服务器私网地址，公网仍使用正常 DNS 记录。

部署后验证：

```bash
curl http://127.0.0.1:10101/api/healthz
curl http://127.0.0.1:10100/healthz
curl http://127.0.0.1:10100/api/healthz
```

首次部署 CosyVoice3 和 Remotion 时，指定媒体服务：

```powershell
.\scripts\deploy-server.ps1 -Services tts,renderer
```

CosyVoice 首次启动会下载模型并加载到 GPU，健康检查最多允许 30 分钟；Remotion 仅使用 CPU 并将 smoke render 写入 `storage/renders/remotion`。两项服务默认仅绑定服务器回环地址，后续由 Docker 内的 API / worker 调用，不对浏览器暴露。当前旁白任务已接入 CosyVoice 和 FunASR；如本次部署包含旁白表结构或 worker 代码，请同时使用 `-RunMigrations`。

运行状态和验证方式见 [媒体服务部署说明](./docs/media-services.md)。

如果本次包含数据库迁移，可以执行：

```powershell
.\scripts\deploy-server.ps1 -RunMigrations
```

`-RunMigrations` 会先在服务器上构建 `aicut-migrator:latest`，再启动一个临时 Docker 容器运行 `goose`，不要求本机或服务器系统安装 `goose`。该容器复用 `aicut-postgres` 的网络命名空间，并连接：

```text
postgres://<db-user>:<db-password>@localhost:5432/<db-name>?sslmode=disable
```

首次执行时服务器可能需要拉取 `golang:1.25-bookworm` 镜像并构建迁移镜像，耗时会稍长。

脚本使用 `git archive HEAD` 打包已提交代码，不会同步本机 `.env`、`storage/`、未跟踪文件和临时产物。服务端 `.env` 需要在服务器上单独维护。

## 数据库迁移

当前迁移不是 API 启动时自动执行，需要手动运行。

推荐在部署时使用：

```powershell
.\scripts\deploy-server.ps1 -RunMigrations
```

如果需要手动从本机执行迁移，再安装 `goose`：

安装 `goose`：

```powershell
go install github.com/pressly/goose/v3/cmd/goose@latest
```

执行迁移：

```powershell
$env:DATABASE_URL="postgres://<db-user>:<db-password>@db.example.internal:5432/<db-name>?sslmode=disable"
goose -dir ./migrations postgres $env:DATABASE_URL up
```

## 测试与构建

后端测试：

```powershell
go test ./...
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

## 开源许可

本项目的原创代码以 [GNU General Public License v3.0](./LICENSE) 发布，SPDX 标识为 `GPL-3.0-only`。

复制、修改或分发本项目及其构建产物时，需要遵守 GPL v3，并向接收者提供对应版本的完整源代码。项目依赖的 FFmpeg、Remotion、FunASR、CosyVoice、字体、模型权重及其他第三方组件继续适用各自的许可证；详见 [THIRD_PARTY_NOTICES.md](./THIRD_PARTY_NOTICES.md)。用户导入的音视频、图片、音乐及生成内容不因使用本项目而自动变更其权利归属。

`ACRUNU Fast Cut` 名称、Logo 及其他品牌标识不包含在 GPL 授权范围内。
