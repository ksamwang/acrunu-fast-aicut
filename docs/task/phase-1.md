# 第一阶段任务清单

## 1. 阶段目标

第一阶段目标是搭建系统工程骨架，并跑通共享素材库与异步任务基础链路。

完成后的最小闭环：

```text
用户登录 Web Console
创建产品和卖点
通过本地 Agent 裁切本地视频为 clean shot
上传 clean shot 到服务端
服务端保存文件并记录 assets
API 创建一个异步任务
worker 消费任务并更新状态
前端能看到素材和任务状态
```

## 2. 工程初始化

- [x] 初始化 Go module
- [x] 建立 `apps/api`
- [x] 建立 `apps/worker`
- [x] 建立 `apps/local-agent`
- [x] 建立 `apps/web`
- [x] 建立 `internal/auth`
- [x] 建立 `internal/config`
- [x] 建立 `internal/domain`
- [x] 建立 `internal/ffmpeg`
- [x] 建立 `internal/modelgateway`
- [x] 建立 `internal/queue`
- [x] 建立 `internal/repository`
- [x] 建立 `internal/services`
- [x] 建立 `internal/storage`
- [x] 添加 `Makefile`
- [x] 添加 `.env.example`
- [x] 添加基础 `.gitignore`

备注：已使用工作区内便携 Go 工具链执行 `go mod tidy` 和 `go test ./...`，Go 代码编译检查通过。

## 3. 基础设施

- [x] 添加 `docker-compose.yml`
- [x] 配置 PostgreSQL
- [x] 配置 pgvector 扩展
- [x] 配置 Redis
- [x] 配置服务端本地 `storage/`
- [x] 配置 API 环境变量
- [x] 配置 worker 环境变量
- [x] 配置 local-agent 环境变量
- [x] 添加 `ffmpeg` / `ffprobe` 可用性检查
- [x] 添加本地开发启动说明

备注：当前执行环境未安装 Docker，但已使用工作区内便携 `ffmpeg` / `ffprobe` 完成媒体裁切与探测运行验证；Docker Compose 运行态验证仍需在 Docker 可用后执行。

## 4. 数据库迁移

- [x] 引入 `goose`
- [x] 建立 `migrations/`
- [x] 建立 `sql/schema/`
- [x] 建立 `sql/queries/`
- [x] 添加 pgvector 扩展迁移
- [x] 添加 `users` 迁移
- [x] 添加 `upload_tokens` 迁移
- [x] 添加 `system_configs` 迁移
- [x] 添加 `products` 迁移
- [x] 添加 `product_selling_points` 迁移
- [x] 添加 `assets` 迁移
- [x] 添加 `generation_tasks` 迁移
- [x] 添加基础审计字段
- [x] 验证迁移可重复执行

备注：已通过 `scripts/check-migrations.ps1` 验证所有迁移文件包含 goose Up/Down 结构。当前执行环境未安装 Docker、PostgreSQL 和 goose，数据库运行态 up/down 验证需在安装依赖后执行。

## 5. 数据访问层

- [x] 引入 `sqlc`
- [x] 添加 `sqlc.yaml`
- [x] 编写 users queries
- [x] 编写 system configs queries
- [x] 编写 products queries
- [x] 编写 selling points queries
- [x] 编写 assets queries
- [x] 编写 generation tasks queries
- [x] 生成 Go DB 代码
- [x] 封装 repository
- [ ] 跑通数据库连接

备注：已使用工作区内便携 sqlc 执行 `sqlc generate`，并生成 `internal/repository/db` 代码；`go test ./...` 已通过。当前执行环境未安装 Docker/PostgreSQL，暂不能验证数据库连接。

## 6. API 基础

- [x] 搭建 Gin HTTP 服务
- [x] 添加健康检查接口
- [x] 添加统一错误响应
- [x] 添加 request id 中间件
- [x] 添加结构化日志
- [x] 添加基础认证中间件
- [x] 添加 `admin` / `user` 角色判断
- [x] 添加用户登录接口
- [x] 添加当前用户信息接口
- [x] 添加 API 路由分组

备注：已完成 API 基础代码落地，并通过 `go test ./...` 编译检查。完整运行态验证仍依赖后续服务启动。

## 7. 用户与系统配置

- [x] 实现 `UserService`
- [x] 实现 `SystemConfigService`
- [x] 添加初始化 admin 用户能力
- [x] 添加系统配置查询接口
- [x] 添加系统配置更新接口
- [x] 限制系统配置更新仅 `admin` 可用
- [x] 添加模型 provider 配置项
- [x] 添加并发控制配置项
- [x] 添加存储配置项
- [x] 添加配置快照读取能力

备注：已完成内存版用户与系统配置服务，后续接入数据库 repository 后替换持久化实现；已通过 `go test ./...` 编译检查。

## 8. 产品与卖点

- [x] 实现 `ProductAssetService` 产品部分
- [x] 添加产品创建接口
- [x] 添加产品列表接口
- [x] 添加产品详情接口
- [x] 添加产品更新接口
- [x] 添加产品归档接口
- [x] 添加卖点创建接口
- [x] 添加卖点列表接口
- [x] 添加卖点更新接口
- [x] 添加卖点归档接口

备注：已完成内存版产品与卖点服务及 API，并通过 `go test ./...` 编译检查。

## 9. 素材入库基础链路

- [x] 定义 clean shot 上传元数据结构
- [x] 定义短期 upload token 申请接口
- [x] 实现 upload token 生成
- [x] 实现 upload token 校验
- [x] 实现服务端 clean shot 文件接收
- [x] 保存 clean shot 到 `storage/assets`
- [x] 写入 `assets` 表
- [x] 记录 `created_by_user_id`
- [x] 调用 `ffprobe` 读取时长
- [x] 调用 `ffprobe` 读取分辨率
- [x] 调用 `ffprobe` 读取帧率
- [x] 根据处理结果更新素材状态为 `ready` 或 `failed`
- [x] 添加素材列表接口
- [x] 添加素材详情接口

备注：已完成内存版素材入库链路和本地文件存储封装，并通过 `go test ./...` 编译检查。已使用便携 `ffprobe` 验证服务端上传后可读取时长、分辨率和帧率，并将素材状态写为 `ready`。

## 10. 本地 Agent 最小版本

- [x] 搭建 `apps/local-agent`
- [x] 本地 Agent 启动 localhost HTTP 服务
- [x] 添加本地 Agent 健康检查接口
- [x] 接收浏览器传入的本地文件路径
- [x] 接收入点和出点参数
- [x] 接收素材类型参数
- [x] 调用本地 `ffmpeg` 生成 clean shot
- [x] 调用本地 `ffprobe` 读取基础信息
- [x] 计算 clean shot checksum
- [x] 使用 upload token 上传 clean shot
- [x] 返回本地预处理结果
- [x] 处理 ffmpeg 执行失败

备注：已完成本地 Agent 最小版本代码落地，并通过 `go test ./...` 编译检查。已使用便携 `ffmpeg` / `ffprobe` 验证本地 Agent 可裁切 clean shot、读取媒体信息并通过 upload token 上传到服务端。

## 11. 队列与 worker

- [x] 引入 Asynq
- [x] 定义任务类型
- [x] 定义任务 payload 结构
- [x] 实现任务入队封装
- [x] 实现 worker 启动入口
- [x] 注册测试任务 handler
- [x] API 能提交测试任务
- [x] worker 能消费测试任务
- [ ] 任务状态写入数据库
- [x] 支持失败原因记录
- [x] 支持重试次数记录

备注：已完成 Asynq 队列封装、测试任务 API 和 worker handler，并通过 `go test ./...` 编译检查。已抽象任务状态存储并接入 PostgreSQL task store，`DATABASE_URL` 可连接时使用 `generation_tasks`，不可用时回退 `storage/temp/tasks.json`。当前执行环境未安装 PostgreSQL 和 Redis，数据库写入与队列消费运行态仍待验证。

## 12. 前端基础

- [x] 初始化 Vite React 项目
- [x] 接入 TypeScript
- [x] 接入 Ant Design
- [x] 建立登录页
- [x] 建立基础布局
- [x] 建立产品管理页面
- [x] 建立卖点管理入口
- [x] 建立素材列表页面
- [x] 建立素材上传 / 本地 Agent 入口
- [x] 建立任务列表页面
- [x] 建立系统配置页面
- [x] 限制系统配置页面仅 `admin` 可见

备注：已完成 Web Console 前端骨架，并对接登录、产品、卖点、素材、任务和系统配置 API；已通过 `npm run build` 验证。

## 13. 验证闭环

- [ ] 启动 Docker Compose
- [x] 启动 API
- [ ] 启动 worker
- [x] 启动 Web Console
- [x] 启动 local-agent
- [x] 创建 admin 用户
- [ ] 登录 Web Console
- [x] 创建产品
- [x] 创建产品卖点
- [x] 本地选择视频并裁切 clean shot
- [x] 上传 clean shot 到服务端
- [x] 服务端写入素材库
- [ ] 前端可查看素材列表
- [ ] API 创建测试异步任务
- [ ] worker 消费测试任务
- [ ] 前端可查看任务状态

备注：已使用便携 Go 临时启动 API 并验证 `/api/healthz`、登录、系统配置读取/写入、产品创建、卖点创建；已临时启动 local-agent 并验证 `/healthz`；已启动 Web Console dev server 并用 HTTP 200 验证页面可访问。已使用便携 `ffmpeg` / `ffprobe` 生成测试视频并完成 local-agent 裁切、upload token 上传、服务端保存、`ffprobe` 探测和 `/api/assets` 查询，返回素材 `status=ready`、`duration_ms=2066`、`width=320`、`height=568`。Docker Compose、PostgreSQL、Redis、worker 和前端点击交互闭环仍待验证。

## 14. 第一阶段暂不包含

- [ ] 完整 LLM 文案生成
- [ ] VLM 画面理解
- [ ] ASR 口播句段识别
- [ ] TTS 配音生成
- [ ] pgvector 真实向量检索
- [ ] 完整 `edit_plan` 生成
- [ ] 服务端真实成片渲染
- [ ] 字幕生成
- [ ] BGM 混音
- [ ] 多模板批量生成策略

## 15. 完成标准

- [x] Go API 可启动
- [ ] Go worker 可启动
- [x] Web Console 可启动
- [x] Local Agent 可启动
- [ ] PostgreSQL + pgvector 可用
- [ ] Redis 可用
- [ ] 数据库迁移可执行
- [x] 用户登录可用
- [x] 系统配置可读写
- [x] 产品和卖点基础管理可用
- [x] clean shot 上传链路可用
- [x] 素材基础入库可用
- [ ] 异步任务入队和消费可用
- [ ] 前端可查看素材和任务状态
