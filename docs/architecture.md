# 系统架构设计稿

## 1. 架构决策

基于 `system-overview.md` 中已经确认的系统定位，当前架构采用以下设计：

- 架构形态：模块化单体 + worker
- 数据存储：PostgreSQL + pgvector + 本地/对象存储
- 模型调用：统一 Model Gateway
- 编排方式：分阶段生成，逐步校验
- 主开发语言：Go
- 前端技术栈：React + TypeScript + Vite + Ant Design

这些决策服务于一个核心目标：

> 以配音时间轴为主驱动，围绕产品素材库完成批量短视频画面编排、渲染、字幕生成和混音。

## 2. 总体架构

系统采用模块化单体作为主应用，重任务通过 worker 异步执行。

```text
Web Console
    |-----------------------------|
    |                             |
Backend API               Local Preprocess Agent
    |
Application Services
    |-- UserService
    |-- SystemConfigService
    |-- ProductAssetService
    |-- AssetAnalysisService
    |-- IndexService
    |-- ScriptVoiceoverService
    |-- PlanningService
    |-- RenderService
    |-- TaskService
    |-- ModelGateway
    |
Workers
    |-- analysis-worker
    |-- embedding-worker
    |-- planning-worker
    |-- render-worker
    |
Storage
    |-- PostgreSQL
    |-- pgvector
    |-- local/object storage
    |-- task queue
    
Local Preprocess Agent
    |-- local raw media
    |-- local ffmpeg / ffprobe
    |-- clean shot upload
```

## 3. 模块化单体 + worker

### 3.1 设计理由

模块化单体用于承载主要业务逻辑，worker 用于执行耗时任务。

该架构适合当前系统的原因：

- 领域边界可以保持清晰
- 开发和调试成本低于一开始拆分微服务
- 素材分析、向量写入、LLM 编排、视频渲染等重任务可以异步执行
- 后续如有需要，可以按模块边界拆分成独立服务
- 本地不涉及 AI 推理，Go 适合作为主业务开发语言和 worker 开发语言

### 3.2 主应用职责

主应用负责：

- 管理用户、角色和登录态
- 管理系统配置
- 提供 Web Console 所需 API
- 管理产品、卖点、素材和任务
- 编排各业务模块之间的调用关系
- 管理任务状态
- 调用 worker 执行异步任务
- 持久化业务数据

### 3.3 Worker 职责

worker 负责执行耗时、可重试、可异步化的任务。

建议的 worker 类型包括：

- `analysis-worker`
  - 执行 `ffprobe`
  - 抽帧
  - 调用 VLM
  - 调用 ASR
  - 生成素材分析结果
- `embedding-worker`
  - 生成素材、画面描述、口播句段等向量
  - 写入 pgvector
- `planning-worker`
  - 执行文案生成
  - 执行配音生成
  - 解析配音时间轴
  - 调用 LLM 分阶段生成编排结果
- `render-worker`
  - 根据 `edit_plan` 执行视频裁切和渲染
  - 基于最终音频生成或对齐字幕
  - 添加 BGM
  - 执行混音
  - 输出成片

## 4. 开发技术栈

### 4.1 后端

后端主业务系统采用 Go。

建议技术栈：

- Web 框架：Gin
- 数据访问：sqlc
- 数据库迁移：goose
- 异步任务：Asynq
- 任务队列与缓存：Redis
- 日志：slog 或 zap
- 配置：环境变量优先，必要时使用 Viper

Go 后端负责：

- API 服务
- 业务模块编排
- 任务调度
- 数据库访问
- 文件管理
- 调用 `ffmpeg` / `ffprobe`
- 调用外部模型服务

### 4.2 Worker

worker 同样采用 Go。

建议使用：

- Asynq
- Redis
- Go `os/exec` 封装 `ffmpeg` / `ffprobe`

worker 负责素材分析、向量写入、编排任务、渲染任务等异步流程。

### 4.3 前端

前端采用：

- React
- TypeScript
- Vite
- Ant Design

前端定位为内部生产控制台，主要服务于素材管理、任务管理、结果预览和人工复核。

### 4.4 数据库与存储

数据库与存储采用：

- PostgreSQL
- pgvector
- Redis
- 本地文件系统
- 可选 MinIO / S3 兼容对象存储

本地文件系统可作为起步方案，但文件访问层应保留对象存储抽象。

### 4.5 视频处理

视频处理不自研底层编解码，统一调用：

- `ffmpeg`
- `ffprobe`

Go 侧通过 `os/exec` 封装命令执行、日志采集、错误处理和超时控制。

### 4.6 模型调用

本地不进行 AI 推理。

所有模型能力通过 ModelGateway 调用外部或远程模型服务，包括：

- LLM
- VLM
- ASR
- TTS
- embedding

ModelGateway 负责供应商适配、请求封装、响应解析、超时、重试、限流和日志。

### 4.7 本地预处理 Agent

人工清洗原始素材采用“浏览器 + 本地 Agent”方案。

浏览器负责：

- 素材预览交互
- 入点和出点标注
- 素材类型标注
- 产品归属选择
- 调用本地 Agent 执行预处理
- 获取服务端签发的短期上传 token
- 将 clean shot 与元数据提交到服务端

本地 Agent 负责：

- 访问客户机本地原始素材文件
- 调用本地 `ffmpeg` 执行裁切
- 生成人工清洗后的 clean shot
- 生成缩略图或预览辅助文件
- 执行基础 `ffprobe`
- 计算文件 hash / checksum
- 使用短期上传 token 上传 clean shot 与元数据到服务端

服务端不直接处理完整原始素材。服务端从 clean shot 开始进入素材分析、索引、编排和渲染流程。

成片渲染仍由服务端 `render-worker` 执行，原因是成片渲染需要统一使用素材库、配音、字幕、BGM、任务队列和产物管理。

本地 Agent 不应保存长期账号密码。用户身份由 Web Console 登录态确认，服务端为单次或短期上传签发 token，本地 Agent 只使用该 token 完成本次 clean shot 上传。

## 5. 用户与权限模型

系统采用单组织多用户模式，不引入租户模型。

核心原则：

- 所有产品、素材、文案、任务和成片属于同一个共享工作空间
- 素材库对所有登录用户开放
- 不使用 `tenant_id`
- 用户主要用于登录、权限控制和操作审计
- 系统配置权限只开放给 `admin`

### 5.1 用户角色

建议初始角色：

- `admin`
  - 用户管理
  - 模型供应商配置
  - LLM / VLM / ASR / TTS / embedding 参数配置
  - API key 或凭证配置
  - 并发控制配置
  - 队列和渲染参数配置
  - 全局任务管理
- `user`
  - 使用共享素材库
  - 上传 clean shot
  - 维护产品素材
  - 创建批量剪辑任务
  - 查看任务和成片结果

后续如需要只读权限，可以增加 `viewer`。

### 5.2 共享素材库

所有用户共享同一个素材库。

素材、产品、任务和成片不按用户隔离，但需要记录审计字段：

- `created_by_user_id`
- `updated_by_user_id`
- `created_at`
- `updated_at`

普通用户上传的 clean shot 入库后，其他用户也可以在后续编排和批量剪辑中使用。

### 5.3 系统配置

系统配置由 `admin` 管理。

建议配置项包括：

- LLM provider
- VLM provider
- ASR provider
- TTS provider
- embedding provider
- 模型默认参数
- API key 或凭证引用
- 全局 LLM 并发数
- 全局 VLM 并发数
- 全局 ASR 并发数
- 全局 TTS 并发数
- 全局渲染并发数
- 单用户最大排队任务数
- 单用户最大运行任务数
- 默认渲染参数
- 默认 BGM 和混音策略

敏感配置不应明文散落在业务表中，应由 SystemConfigService 统一管理。

### 5.4 批量任务权限

批量剪辑任务共享全局队列。

任务应记录：

- `created_by_user_id`
- `product_id`
- `status`
- `priority`
- `config_snapshot`
- `created_at`

并发控制采用全局策略，可额外限制单用户排队数和运行数，避免单个用户提交大量任务影响整体系统。

## 6. 核心业务模块

### 6.1 UserService

负责用户和权限管理。

职责：

- 用户登录和登出
- 用户信息管理
- 角色管理
- 权限校验
- 操作审计基础信息维护

### 6.2 SystemConfigService

负责系统级配置管理。

职责：

- 管理模型供应商配置
- 管理模型参数
- 管理并发控制参数
- 管理队列和渲染默认参数
- 管理敏感配置引用
- 为 ModelGateway、TaskService 和 RenderService 提供配置

### 6.3 ProductAssetService

负责产品、卖点和素材库管理。

职责：

- 管理产品基础信息
- 管理产品卖点
- 管理人工清洗后的 `shot`
- 接收本地 Agent 上传的 clean shot 与元数据
- 记录素材创建者和更新者
- 管理素材类型，例如 `visual_only`、`talking_head`
- 管理素材文件路径和存储状态
- 维护素材与产品、卖点之间的关系

### 6.4 AssetAnalysisService

负责素材理解。

职责：

- 提取素材基础媒体信息
- 生成或读取素材时长、分辨率、帧率、编码等信息
- 对素材抽帧并调用 VLM 生成视觉分析
- 对 `talking_head` 素材调用 ASR 生成 `speech_segment`
- 合并人工标注、VLM 结果和 ASR 结果
- 生成结构化标签与开放语义描述

### 6.5 IndexService

负责结构化检索和向量检索。

职责：

- 维护结构化过滤条件
- 维护 pgvector 向量索引
- 支持 `shot` 级检索
- 支持 `speech_segment` 级检索
- 为 PlanningService 返回受控候选素材集合

检索应同时使用：

- 结构化过滤
- 语义向量召回
- 业务规则重排

### 6.6 ScriptVoiceoverService

负责文案和配音时间轴。

职责：

- 根据产品、卖点和风格约束生成文案
- 将文案切分为可管理的文本单元
- 生成或接收最终口播/旁白配音
- 解析配音真实时间轴
- 生成 `narration_segment`

该模块产出的配音时间轴是后续画面剪辑的主锚点。

### 6.7 PlanningService

负责分阶段生成和校验编排结果。

职责：

- 基于产品卖点和文案生成视频结构规划
- 基于 `narration_segment` 生成镜头需求
- 调用 IndexService 检索候选素材
- 调用 LLM 从候选素材中选择并装配画面
- 生成 `clip_segment`
- 输出 `edit_plan`
- 对生成结果进行结构校验和约束校验

PlanningService 不应直接扫描原始素材库，而应只处理 IndexService 返回的候选集合。

### 6.8 RenderService

负责将编排结果渲染为视频。

职责：

- 校验 `edit_plan`
- 根据 `clip_segment` 裁切素材
- 对齐配音时间轴
- 生成画面时间线
- 渲染临时视频
- 基于最终时间线音频生成或对齐字幕
- 添加 BGM
- 调整 BGM 与配音的混音关系
- 输出成片文件

RenderService 应执行确定的编排方案，不承担自由创作职责。

### 6.9 TaskService

负责批量任务管理。

职责：

- 创建批量生成任务
- 跟踪任务状态
- 调度 worker
- 控制多条成片的生成进度
- 记录失败原因和重试状态
- 管理成片产物
- 记录任务创建用户
- 执行全局并发控制和单用户任务限制

TaskService 还应为批量生成提供控制能力：

- 文案去重
- 首镜去重
- 素材复用控制
- 结构多样性控制

### 6.10 ModelGateway

负责统一模型调用。

职责：

- 封装外部或远程 LLM、VLM、ASR、TTS、embedding 模型服务调用
- 统一模型请求、响应和错误处理
- 统一超时、重试、限流和日志
- 屏蔽不同模型供应商之间的差异
- 为业务模块提供稳定接口
- 不在本地承载 AI 推理

业务模块不应直接依赖具体模型供应商。

## 7. 数据存储设计

### 7.1 PostgreSQL

PostgreSQL 用于存储结构化业务数据。

主要数据包括：

- 用户
- 角色
- 系统配置
- 产品
- 卖点
- 素材
- 素材分析结果
- 口播句段
- 文案变体
- 配音时间轴
- 编排方案
- 渲染任务
- 成片产物

### 7.2 pgvector

pgvector 用于存储和检索向量数据。

建议支持以下向量类型：

- `shot` 画面描述向量
- `shot` 卖点表达向量
- `speech_segment` 文本向量
- `narration_segment` 文案语义向量

向量记录应携带 metadata，便于结构化过滤和业务重排。

### 7.3 本地/对象存储

文件类资产使用本地或对象存储。

主要包括：

- 原始导入素材
- 人工清洗后的 `shot`
- 抽帧图片
- 配音音频
- BGM 文件
- 临时渲染文件
- 字幕文件
- 最终成片

数据库中只保存文件元数据、路径、状态和关联关系。

### 7.4 Task Queue

任务队列用于调度异步任务。

适合进入队列的任务包括：

- 素材分析
- 向量生成
- 文案生成
- 配音生成
- 编排生成
- 视频渲染
- 字幕生成
- BGM 混音

## 8. 分阶段编排流程

编排不采用一次性生成完整结果的方式，而是分阶段生成、逐步校验。

建议流程：

1. 用户在浏览器中预览本地原始素材并标注入点、出点和素材类型
2. Web Console 向服务端申请短期上传 token
3. 本地 Agent 调用本地 `ffmpeg` 生成 clean shot
4. 本地 Agent 使用短期上传 token 上传 clean shot 与元数据到服务端
5. 服务端记录素材创建用户并入库
6. 服务端读取产品和卖点
7. 生成文案变体
8. 生成或录制配音
9. 解析配音时间轴，生成 `narration_segment`
10. 基于 `narration_segment` 生成镜头需求
11. 通过 IndexService 检索候选素材
12. 从候选素材中选择并装配 `clip_segment`
13. 校验 `edit_plan`
14. 服务端 `render-worker` 渲染画面时间线
15. 基于最终音频生成或对齐字幕
16. 添加 BGM 并混音
17. 输出成片

每个阶段都应产生可检查的中间结果，避免 LLM 一次性输出不可解释的完整时间线。

## 9. 校验原则

分阶段编排过程中应进行结构化校验。

建议至少包含：

- 文案是否覆盖目标卖点
- 配音时长是否满足成片约束
- `narration_segment` 时间轴是否连续且合法
- 检索候选素材是否满足镜头需求
- `clip_segment` 是否单源连续
- `clip_segment` 入出点是否落在来源素材合法范围内
- 时间线是否覆盖完整配音
- 画面是否存在不可接受的空洞
- 素材复用是否超过限制
- 成片变体之间是否过于相似
- 用户是否具备对应操作权限
- 当前队列是否超过全局并发或单用户任务限制

## 10. 设计结论

当前架构设计共识如下：

- 系统采用模块化单体 + worker
- 主开发语言采用 Go
- 前端采用 React + TypeScript + Vite + Ant Design
- worker 采用 Go + Asynq + Redis
- 人工素材清洗采用浏览器 + 本地 Agent
- 本地 Agent 负责客户机上的原始素材裁切和 clean shot 上传
- 本地 Agent 使用短期上传 token，不保存长期账号密码
- 系统采用单组织多用户模式，不引入租户模型
- 素材库对所有登录用户共享开放
- `admin` 负责模型、并发、渲染和系统配置
- 普通用户负责素材操作、任务创建和结果查看
- PostgreSQL 存储结构化业务数据
- pgvector 承载向量索引
- 本地/对象存储承载媒体文件
- 本地不进行 AI 推理
- Model Gateway 统一封装外部或远程 LLM、VLM、ASR、TTS 和 embedding 服务调用
- 编排采用分阶段生成、逐步校验
- 配音时间轴是画面剪辑的主锚点
- PlanningService 负责规划和素材装配
- RenderService 负责确定性渲染、字幕生成和 BGM 混音
- 成片渲染由服务端 `render-worker` 执行
