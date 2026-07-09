# 第一阶段任务清单

## 1. 文档目的

这份清单用于约束第一阶段的实现范围，目标不是讨论完整产品演进路线，而是明确当前系统设计下，第一批必须打通的工程闭环。

本文档只描述执行任务，不重复展开架构讨论。相关设计基线以 `docs/system-overview.md` 和 `docs/architecture.md` 为准。

第一阶段完成后，应具备下面这个最小可运行链路：

```text
Web Console 登录
-> 创建产品与卖点
-> 浏览器调用本地 Agent 生成 clean shot
-> 上传 clean shot 到服务端
-> 服务端入库 assets
-> 创建异步任务
-> worker 消费任务并更新状态
-> 前端可查看素材与任务结果
```

## 2. 第一阶段目标

- [ ] 搭建模块化单体 + worker 的基础工程骨架
- [ ] 打通共享素材库的最小入库链路
- [ ] 打通浏览器 + 本地 Agent 的 clean shot 预处理链路
- [ ] 打通 PostgreSQL / pgvector / Redis / 本地存储 的基础运行环境
- [ ] 打通用户登录、产品管理、卖点管理、素材管理、任务管理的基础控制台
- [ ] 打通异步任务创建、消费、状态回写的基础闭环
- [ ] 为后续素材分析、向量索引、文案生成、编排、渲染预留清晰扩展点

## 3. 工程与基础设施

### 3.1 工程骨架

- [ ] 初始化 Go module
- [ ] 建立 `apps/api`
- [ ] 建立 `apps/worker`
- [ ] 建立 `apps/local-agent`
- [ ] 建立 `apps/web`
- [ ] 建立 `internal/auth`
- [ ] 建立 `internal/config`
- [ ] 建立 `internal/domain`
- [ ] 建立 `internal/ffmpeg`
- [ ] 建立 `internal/modelgateway`
- [ ] 建立 `internal/queue`
- [ ] 建立 `internal/repository`
- [ ] 建立 `internal/services`
- [ ] 建立 `internal/storage`
- [ ] 补齐 `Makefile`
- [ ] 补齐 `.env.example`
- [ ] 补齐基础 `.gitignore`

### 3.2 运行环境

- [ ] 提供 `docker-compose.yml`
- [ ] 配置 PostgreSQL
- [ ] 配置 pgvector 扩展
- [ ] 配置 Redis
- [ ] 配置服务端本地存储目录
- [ ] 配置 API 环境变量
- [ ] 配置 worker 环境变量
- [ ] 配置 local-agent 环境变量
- [ ] 增加 `ffmpeg` / `ffprobe` 可用性检查
- [ ] 提供本地开发启动说明

## 4. 数据层

### 4.1 数据库迁移

- [ ] 引入 `goose`
- [ ] 建立 `migrations/`
- [ ] 建立 `sql/schema/`
- [ ] 建立 `sql/queries/`
- [ ] 增加 pgvector 扩展迁移
- [ ] 增加 `users` 迁移
- [ ] 增加 `upload_tokens` 迁移
- [ ] 增加 `system_configs` 迁移
- [ ] 增加 `products` 迁移
- [ ] 增加 `product_selling_points` 迁移
- [ ] 增加 `assets` 迁移
- [ ] 增加 `generation_tasks` 迁移
- [ ] 增加通用审计字段
- [ ] 验证迁移可重复执行

### 4.2 数据访问

- [ ] 引入 `sqlc`
- [ ] 编写 `sqlc.yaml`
- [ ] 编写 users queries
- [ ] 编写 system configs queries
- [ ] 编写 products queries
- [ ] 编写 selling points queries
- [ ] 编写 assets queries
- [ ] 编写 generation tasks queries
- [ ] 生成 Go DB 代码
- [ ] 封装 repository
- [ ] 验证数据库连接与基础读写

## 5. 后端基础能力

### 5.1 API 基础

- [ ] 搭建 Gin HTTP 服务
- [ ] 增加健康检查接口
- [ ] 增加统一错误响应
- [ ] 增加 request id 中间件
- [ ] 增加结构化日志
- [ ] 增加基础认证中间件
- [ ] 增加 `admin` / `user` 角色判断
- [ ] 增加登录接口
- [ ] 增加当前用户信息接口
- [ ] 完成 API 路由分组

### 5.2 用户与系统配置

- [ ] 实现 `UserService`
- [ ] 实现 `SystemConfigService`
- [ ] 支持初始化 admin 用户
- [ ] 增加系统配置查询接口
- [ ] 增加系统配置更新接口
- [ ] 限制只有 `admin` 可以更新系统配置
- [ ] 提供模型 provider 配置项
- [ ] 提供并发控制配置项
- [ ] 提供存储配置项
- [ ] 提供配置快照读取能力

### 5.3 产品与卖点

- [ ] 实现 `ProductAssetService` 的产品与卖点管理部分
- [ ] 增加产品创建接口
- [ ] 增加产品列表接口
- [ ] 增加产品详情接口
- [ ] 增加产品更新接口
- [ ] 增加产品归档接口
- [ ] 增加卖点创建接口
- [ ] 增加卖点列表接口
- [ ] 增加卖点更新接口
- [ ] 增加卖点归档接口

## 6. 素材入库最小闭环

### 6.1 服务端素材入库

- [ ] 定义 clean shot 上传元数据结构
- [ ] 提供短期 upload token 申请接口
- [ ] 实现 upload token 生成
- [ ] 实现 upload token 校验
- [ ] 实现服务端 clean shot 文件接收
- [ ] 将文件保存到 `storage/assets`
- [ ] 写入 `assets` 表
- [ ] 记录 `created_by_user_id`
- [ ] 调用 `ffprobe` 读取时长
- [ ] 调用 `ffprobe` 读取分辨率
- [ ] 调用 `ffprobe` 读取帧率
- [ ] 根据处理结果更新素材状态
- [ ] 提供素材列表接口
- [ ] 提供素材详情接口

### 6.2 本地 Agent 最小能力

- [ ] 启动 `apps/local-agent`
- [ ] 提供 localhost HTTP 服务
- [ ] 提供本地健康检查接口
- [ ] 接收浏览器传入的本地文件路径
- [ ] 接收入点和出点参数
- [ ] 接收素材类型参数
- [ ] 调用本地 `ffmpeg` 生成 clean shot
- [ ] 调用本地 `ffprobe` 读取基础信息
- [ ] 计算 clean shot checksum
- [ ] 使用 upload token 上传 clean shot
- [ ] 返回本地预处理结果
- [ ] 处理 `ffmpeg` 执行失败

## 7. 队列与任务基础闭环

- [ ] 引入 Asynq
- [ ] 定义任务类型
- [ ] 定义任务 payload 结构
- [ ] 实现任务入队封装
- [ ] 实现 worker 启动入口
- [ ] 注册测试任务 handler
- [ ] API 支持提交测试任务
- [ ] worker 支持消费测试任务
- [ ] 任务状态写入数据库
- [ ] 支持失败原因记录
- [ ] 支持重试次数记录

## 8. Web Console 基础能力

- [ ] 初始化 Vite React 项目
- [ ] 接入 TypeScript
- [ ] 接入 Ant Design
- [ ] 建立登录页
- [ ] 建立基础布局
- [ ] 建立产品管理页
- [ ] 建立卖点管理入口
- [ ] 建立素材列表页
- [ ] 建立素材上传 / 本地 Agent 入口
- [ ] 建立任务列表页
- [ ] 建立系统配置页
- [ ] 限制系统配置页仅 `admin` 可见

## 9. 第一阶段验收闭环

- [ ] 启动 PostgreSQL / pgvector / Redis
- [ ] 启动 API
- [ ] 启动 worker
- [ ] 启动 Web Console
- [ ] 启动 local-agent
- [ ] 创建 admin 用户
- [ ] 登录 Web Console
- [ ] 创建产品
- [ ] 创建产品卖点
- [ ] 本地选择视频并裁出 clean shot
- [ ] 上传 clean shot 到服务端
- [ ] 服务端完成素材入库
- [ ] 前端可查看素材列表
- [ ] API 创建测试异步任务
- [ ] worker 消费任务并更新状态
- [ ] 前端可查看任务状态

## 10. 第一阶段暂不包含

- [ ] 完整 VLM 画面理解
- [ ] 完整 ASR 口播句段识别
- [ ] embedding 向量写入与检索
- [ ] 自动文案生成
- [ ] TTS 配音生成
- [ ] `narration_segment` 时间轴生成
- [ ] `clip_segment` 编排
- [ ] `edit_plan` 生成
- [ ] 服务端成片渲染
- [ ] 字幕识别与生成
- [ ] BGM 混音
- [ ] 批量多变体生成控制

## 11. 完成标准

- [ ] API 可启动并可登录
- [ ] worker 可启动并消费基础任务
- [ ] Web Console 可完成基础业务操作
- [ ] local-agent 可在客户端完成 clean shot 预处理
- [ ] PostgreSQL + pgvector 可用
- [ ] Redis 可用
- [ ] 数据库迁移可执行
- [ ] 产品与卖点基础管理可用
- [ ] clean shot 上传链路可用
- [ ] 素材基础入库可用
- [ ] 异步任务创建与状态回写可用
- [ ] 前端可查看素材与任务结果

## 12. 使用规则

- 只有在有代码、测试或真实运行验证支撑时，才勾选对应任务
- 如果某项能力只是接口存在，但没有闭环验证，不应标记完成
- 设计讨论以 [system-overview.md](../system-overview.md) 和 [architecture.md](../architecture.md) 为准
- 第一阶段任务清单服务于当前系统设计，不额外引入未确认的新产品假设
