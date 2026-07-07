# 项目目录结构设计稿

## 1. 目标

本项目采用 monorepo 组织方式，将服务端、worker、前端、本地预处理 Agent、数据库迁移和文档放在同一个仓库中。

目录结构设计目标：

- 服务模块边界清晰
- Go 后端与 Go worker 共用领域模型和基础设施代码
- Web Console 与 Local Preprocess Agent 独立构建
- 数据库迁移、部署配置和文档集中管理
- 后续如需拆分服务，可以按目录边界迁移

## 2. 推荐目录结构

```text
.
|-- apps/
|   |-- api/
|   |-- worker/
|   |-- web/
|   `-- local-agent/
|-- internal/
|   |-- auth/
|   |-- config/
|   |-- domain/
|   |-- ffmpeg/
|   |-- modelgateway/
|   |-- queue/
|   |-- repository/
|   |-- services/
|   `-- storage/
|-- migrations/
|-- sql/
|   |-- queries/
|   `-- schema/
|-- docs/
|-- deploy/
|-- storage/
|   |-- assets/
|   |-- frames/
|   |-- voiceovers/
|   |-- renders/
|   |-- subtitles/
|   |-- bgm/
|   `-- temp/
|-- scripts/
|-- go.mod
|-- go.sum
|-- docker-compose.yml
|-- README.md
`-- Makefile
```

## 3. `apps/`

`apps/` 存放可独立运行的应用入口。

### 3.1 `apps/api/`

Go Backend API 入口。

职责：

- 启动 HTTP 服务
- 注册路由和中间件
- 接收 Web Console 请求
- 暴露本地 Agent 上传相关 API
- 调用 `internal/services` 中的业务服务

建议入口：

```text
apps/api/main.go
```

### 3.2 `apps/worker/`

Go worker 入口。

职责：

- 启动 Asynq worker
- 注册任务 handler
- 执行素材分析、向量生成、文案配音、编排、渲染等异步任务

建议入口：

```text
apps/worker/main.go
```

### 3.3 `apps/web/`

Web Console 前端应用。

技术栈：

- React
- TypeScript
- Vite
- Ant Design

职责：

- 用户登录
- 产品和卖点管理
- 共享素材库管理
- 本地素材预处理交互
- 批量剪辑任务创建
- 任务状态查看
- 成片结果预览和复核
- admin 系统配置界面

### 3.4 `apps/local-agent/`

本地预处理 Agent。

建议同样使用 Go 开发。

职责：

- 在用户客户机运行
- 提供 localhost API 给 Web Console 调用
- 访问本地原始素材文件
- 调用本地 `ffmpeg` 和 `ffprobe`
- 根据用户标注生成 clean shot
- 生成缩略图或预览辅助文件
- 计算 hash / checksum
- 使用短期上传 token 上传 clean shot 和元数据到服务端

本地 Agent 不保存长期账号密码。

## 4. `internal/`

`internal/` 存放 Go 后端和 worker 共享的内部代码。

### 4.1 `internal/auth/`

认证和授权相关代码。

职责：

- 用户登录态
- token 校验
- 角色权限判断
- 本地 Agent 短期上传 token 校验

### 4.2 `internal/config/`

配置加载。

职责：

- 环境变量读取
- 服务端配置
- 数据库配置
- Redis 配置
- 存储配置
- 模型供应商配置引用

### 4.3 `internal/domain/`

领域模型和核心类型。

建议包含：

- `User`
- `Product`
- `SellingPoint`
- `Asset`
- `AssetAnalysis`
- `SpeechSegment`
- `ScriptVariant`
- `NarrationSegment`
- `EditPlan`
- `ClipSegment`
- `RenderJob`
- `SystemConfig`

这里定义业务概念，不直接放数据库访问逻辑。

### 4.4 `internal/ffmpeg/`

`ffmpeg` / `ffprobe` 封装。

职责：

- 媒体信息读取
- 裁切命令构建
- 抽帧命令构建
- 渲染命令构建
- 命令执行
- 超时控制
- stderr 日志采集
- 错误归一化

服务端 render-worker 和本地 Agent 都可能复用这里的命令构建逻辑。

### 4.5 `internal/modelgateway/`

统一模型服务调用。

职责：

- LLM 调用
- VLM 调用
- ASR 调用
- TTS 调用
- embedding 调用
- 供应商适配
- 请求超时、重试、限流
- 响应解析和错误归一化

本地不做 AI 推理，该模块只调用外部或远程模型服务。

### 4.6 `internal/queue/`

异步任务队列封装。

职责：

- Asynq client
- 任务类型定义
- 任务 payload 定义
- 任务入队
- 任务重试策略

### 4.7 `internal/repository/`

数据库访问层。

建议使用 `sqlc` 生成的代码，并在该目录中封装 repository。

职责：

- 用户数据访问
- 产品数据访问
- 素材数据访问
- 分析结果访问
- 编排结果访问
- 任务状态访问
- 系统配置访问

### 4.8 `internal/services/`

业务服务层。

建议按架构文档中的核心服务划分：

- `UserService`
- `SystemConfigService`
- `ProductAssetService`
- `AssetAnalysisService`
- `IndexService`
- `ScriptVoiceoverService`
- `PlanningService`
- `RenderService`
- `TaskService`

该层负责业务规则、流程编排和权限校验。

### 4.9 `internal/storage/`

文件存储抽象。

职责：

- 本地文件存储
- 后续兼容 MinIO / S3
- 文件路径生成
- 文件元数据管理
- 上传和下载
- 临时文件管理

数据库只保存文件元数据和存储 key，不直接把业务逻辑绑定到绝对路径。

## 5. `migrations/`

数据库迁移文件目录。

建议使用 `goose`。

命名示例：

```text
migrations/
  000001_create_users.sql
  000002_create_products.sql
  000003_create_assets.sql
```

迁移文件只负责数据库结构变更。

## 6. `sql/`

`sql/` 存放 sqlc 相关文件。

建议结构：

```text
sql/
  schema/
  queries/
```

### 6.1 `sql/schema/`

存放数据库 schema 定义副本，供 sqlc 生成代码使用。

### 6.2 `sql/queries/`

存放 sqlc query 文件。

建议按业务对象拆分：

- `users.sql`
- `products.sql`
- `assets.sql`
- `tasks.sql`
- `edit_plans.sql`
- `system_configs.sql`

## 7. `docs/`

文档目录。

当前已包含：

- `system-overview.md`
- `architecture.md`
- `project-structure.md`

后续建议继续增加：

- `data-model.md`
- `edit-plan-schema.md`
- `model-gateway.md`
- `local-agent.md`
- `deployment.md`

## 8. `deploy/`

部署配置目录。

建议包含：

- Dockerfile
- Nginx 配置
- systemd 配置，可选
- 环境变量示例
- 部署脚本

## 9. `storage/`

本地开发和单机部署时的媒体文件目录。

建议结构：

```text
storage/
  assets/
  frames/
  voiceovers/
  renders/
  subtitles/
  bgm/
  temp/
```

说明：

- `assets/`：clean shot 文件
- `frames/`：抽帧图片
- `voiceovers/`：配音音频
- `renders/`：渲染产物
- `subtitles/`：字幕文件
- `bgm/`：BGM 文件
- `temp/`：临时文件

原始素材默认不上传服务端，不作为服务端存储目录的一部分。

## 10. `scripts/`

项目辅助脚本目录。

建议包含：

- 开发环境初始化脚本
- 数据库迁移脚本
- ffmpeg 检查脚本
- 测试数据生成脚本
- 本地清理脚本

## 11. 根目录文件

建议根目录包含：

- `go.mod`
- `go.sum`
- `docker-compose.yml`
- `Makefile`
- `README.md`

### 11.1 `docker-compose.yml`

用于本地开发和单机部署。

建议包含：

- api
- worker
- web
- PostgreSQL + pgvector
- Redis
- 可选 MinIO

### 11.2 `Makefile`

统一常用命令。

建议包含：

- `make dev`
- `make test`
- `make migrate-up`
- `make migrate-down`
- `make sqlc`
- `make lint`

## 12. 设计结论

当前项目目录结构采用 monorepo。

核心原则：

- `apps/` 放应用入口
- `internal/` 放 Go 共享业务和基础设施代码
- `migrations/` 放数据库迁移
- `sql/` 放 sqlc schema 和 query
- `docs/` 放设计文档
- `storage/` 放本地开发和单机部署的媒体文件
- 服务端只存 clean shot，不默认存原始素材
- 本地 Agent 与服务端共用部分 Go 能力，但作为独立应用构建
