# 本地开发说明

## 1. 依赖

本地开发建议安装：

- Go 1.23+
- Docker
- Docker Compose
- Node.js 20+
- ffmpeg
- ffprobe

当前服务端容器会安装 `ffmpeg` 和 `ffprobe`，但本地 Agent 在用户客户机运行，因此客户机也需要可用的 `ffmpeg` 和 `ffprobe`。

如果不想把 `ffmpeg` 加入系统 PATH，可以通过环境变量指定可执行文件路径：

```powershell
$env:FFMPEG_PATH="E:\tools\ffmpeg\bin\ffmpeg.exe"
$env:FFPROBE_PATH="E:\tools\ffmpeg\bin\ffprobe.exe"
```

## 2. 环境变量

复制 `.env.example` 为 `.env` 后按需调整。

```powershell
Copy-Item .env.example .env
```

## 3. 启动基础设施

启动 PostgreSQL、Redis、API 和 worker：

```powershell
docker compose up --build
```

只启动 PostgreSQL 和 Redis：

```powershell
docker compose up postgres redis
```

无 Redis 的本地开发环境可临时使用文件队列验证 API 与 worker 的异步链路：

```powershell
$env:QUEUE_BACKEND="file"
```

文件队列只用于本地开发验证，生产和 Docker Compose 默认仍使用 Redis。

## 4. 检查 ffmpeg

Windows PowerShell：

```powershell
./scripts/check-ffmpeg.ps1
```

Unix shell：

```sh
./scripts/check-ffmpeg.sh
```

## 5. 本地存储目录

服务端本地存储目录位于 `storage/`。

主要子目录：

- `storage/assets`
- `storage/frames`
- `storage/voiceovers`
- `storage/renders`
- `storage/subtitles`
- `storage/bgm`
- `storage/temp`

原始素材默认不上传服务端，只上传本地 Agent 生成的 clean shot。

## 6. 数据库迁移

安装 `goose`：

```powershell
go install github.com/pressly/goose/v3/cmd/goose@latest
```

执行迁移：

```powershell
$env:DATABASE_URL="postgres://aicut:aicut@localhost:5432/aicut?sslmode=disable"
make migrate-up
```

回滚最近一次迁移：

```powershell
$env:DATABASE_URL="postgres://aicut:aicut@localhost:5432/aicut?sslmode=disable"
make migrate-down
```

检查迁移文件结构：

```powershell
make check-migrations
```
