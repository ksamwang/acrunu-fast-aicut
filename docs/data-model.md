# 数据模型设计稿

## 1. 目标

本文档定义系统第一版核心数据模型。

设计前提：

- 单组织多用户
- 不引入租户模型
- 所有用户共享素材库
- `admin` 负责系统配置
- 普通用户负责素材操作、任务创建和结果查看
- 原始素材在用户客户机清洗
- 服务端从 clean shot 开始管理素材
- 文案先生成配音，配音时间轴驱动画面剪辑
- 成片渲染在服务端执行

## 2. 通用字段约定

大多数业务表建议包含：

- `id`
- `created_at`
- `updated_at`
- `created_by_user_id`
- `updated_by_user_id`

说明：

- 不使用 `tenant_id`
- 用户字段用于操作审计，不用于隔离素材库
- 软删除字段按对象需要决定是否增加

建议时间字段使用：

- 数据库类型：`timestamptz`
- 业务时间轴：毫秒整数，例如 `start_ms`、`end_ms`

## 3. 用户与权限

### 3.1 `users`

用户表。

建议字段：

- `id`
- `username`
- `display_name`
- `email`
- `password_hash`
- `role`
- `status`
- `last_login_at`
- `created_at`
- `updated_at`

建议枚举：

- `role`
  - `admin`
  - `user`
- `status`
  - `active`
  - `disabled`

说明：

- `admin` 可管理系统配置、模型配置、并发控制和用户
- `user` 可使用共享素材库、上传 clean shot、创建任务和查看结果

### 3.2 `upload_tokens`

本地 Agent 短期上传 token。

建议字段：

- `id`
- `token_hash`
- `user_id`
- `product_id`
- `expires_at`
- `used_at`
- `status`
- `metadata`
- `created_at`

建议枚举：

- `status`
  - `active`
  - `used`
  - `expired`
  - `revoked`

说明：

- 本地 Agent 不保存长期账号密码
- Web Console 向服务端申请短期上传 token
- 本地 Agent 使用 token 上传 clean shot 与元数据

## 4. 系统配置

### 4.1 `system_configs`

系统配置表。

建议字段：

- `id`
- `config_key`
- `config_value`
- `config_type`
- `is_secret`
- `description`
- `updated_by_user_id`
- `created_at`
- `updated_at`

建议配置类型：

- `string`
- `number`
- `boolean`
- `json`
- `secret_ref`

说明：

- 模型 provider、默认模型参数、并发控制、渲染默认值等可放入该表
- API key 等敏感配置不建议明文散落在业务表中
- 敏感配置可用加密字段或外部 secret 引用

### 4.2 推荐配置项

建议至少支持：

- `llm.provider`
- `llm.model`
- `llm.max_concurrency`
- `vlm.provider`
- `vlm.model`
- `vlm.max_concurrency`
- `asr.provider`
- `asr.model`
- `asr.max_concurrency`
- `tts.provider`
- `tts.voice`
- `tts.max_concurrency`
- `embedding.provider`
- `embedding.model`
- `embedding.dimensions`
- `render.max_concurrency`
- `task.max_queued_per_user`
- `task.max_running_per_user`
- `storage.backend`
- `storage.local_root`

## 5. 产品与卖点

### 5.1 `products`

产品表。

建议字段：

- `id`
- `name`
- `description`
- `category`
- `status`
- `metadata`
- `created_by_user_id`
- `updated_by_user_id`
- `created_at`
- `updated_at`

建议枚举：

- `status`
  - `active`
  - `archived`

### 5.2 `product_selling_points`

产品卖点表。

建议字段：

- `id`
- `product_id`
- `title`
- `description`
- `priority`
- `status`
- `created_by_user_id`
- `updated_by_user_id`
- `created_at`
- `updated_at`

说明：

- 卖点不仅用于文案生成，也用于素材分析和镜头编排
- 素材和口播句段也应能映射到卖点

## 6. 素材库

### 6.1 `assets`

素材表。服务端只管理 clean shot，不默认保存完整原始素材。

建议字段：

- `id`
- `product_id`
- `storage_key`
- `file_name`
- `file_ext`
- `mime_type`
- `file_size`
- `checksum`
- `source_type`
- `duration_ms`
- `width`
- `height`
- `fps`
- `codec`
- `status`
- `manual_clean_status`
- `source_original_name`
- `source_in_ms`
- `source_out_ms`
- `metadata`
- `created_by_user_id`
- `updated_by_user_id`
- `created_at`
- `updated_at`

建议枚举：

- `source_type`
  - `visual_only`
  - `talking_head`
- `status`
  - `uploaded`
  - `analyzing`
  - `ready`
  - `failed`
  - `archived`
- `manual_clean_status`
  - `cleaned`
  - `needs_review`

说明：

- `source_in_ms` 和 `source_out_ms` 记录该 clean shot 相对原始素材的裁切区间，可选
- `source_original_name` 只记录原始文件名或引用信息，不代表服务端保存原始素材

### 6.2 `asset_analysis`

素材分析结果表。

建议字段：

- `id`
- `asset_id`
- `scene_description`
- `action_description`
- `shot_size`
- `camera_motion`
- `contains_person`
- `contains_product`
- `visual_tags`
- `selling_point_tags`
- `quality_score`
- `analysis_status`
- `model_provider`
- `model_name`
- `raw_result`
- `created_at`
- `updated_at`

建议枚举：

- `shot_size`
  - `wide`
  - `full`
  - `medium`
  - `close`
  - `extreme_close`
  - `unknown`
- `camera_motion`
  - `static`
  - `push_in`
  - `pull_out`
  - `pan`
  - `tilt`
  - `tracking`
  - `handheld`
  - `unknown`
- `analysis_status`
  - `pending`
  - `processing`
  - `completed`
  - `failed`

说明：

- `visual_tags` 和 `selling_point_tags` 建议使用 JSONB
- `raw_result` 保留模型原始响应，便于排查和迭代

### 6.3 `speech_segments`

口播素材句段表。

建议字段：

- `id`
- `asset_id`
- `start_ms`
- `end_ms`
- `text`
- `speaker_style`
- `emotion`
- `keywords`
- `selling_point_id`
- `confidence`
- `asr_provider`
- `asr_model`
- `raw_result`
- `created_at`
- `updated_at`

说明：

- 一个 `talking_head` 素材可解析出多个 `speech_segment`
- `speech_segment` 是口播素材的语义检索单元
- `start_ms` 和 `end_ms` 相对该 clean shot 文件

## 7. 向量索引

### 7.1 `embeddings`

向量表，使用 pgvector。

建议字段：

- `id`
- `object_type`
- `object_id`
- `embedding_type`
- `embedding`
- `provider`
- `model`
- `dimensions`
- `metadata`
- `created_at`

建议枚举：

- `object_type`
  - `asset`
  - `speech_segment`
  - `narration_segment`
- `embedding_type`
  - `visual_description`
  - `selling_point_match`
  - `speech_text`
  - `script_text`

说明：

- `asset` 可对应画面描述向量和卖点表达向量
- `speech_segment` 对应口播文本向量
- `narration_segment` 对应文案语义向量
- metadata 应携带 `product_id`、`source_type`、`duration_ms` 等过滤信息

## 8. 文案与配音

### 8.1 `generation_tasks`

批量生成任务表。

建议字段：

- `id`
- `product_id`
- `created_by_user_id`
- `task_type`
- `status`
- `variant_count`
- `target_duration_ms`
- `style_prompt`
- `config_snapshot`
- `error_message`
- `created_at`
- `updated_at`
- `started_at`
- `finished_at`

建议枚举：

- `task_type`
  - `batch_video`
- `status`
  - `queued`
  - `running`
  - `completed`
  - `failed`
  - `cancelled`

说明：

- 所有批量任务共享全局队列
- `config_snapshot` 保存任务创建时使用的模型、并发、渲染等关键配置快照

### 8.2 `script_variants`

文案变体表。

建议字段：

- `id`
- `generation_task_id`
- `product_id`
- `variant_index`
- `script_text`
- `script_structure`
- `selling_point_coverage`
- `status`
- `llm_provider`
- `llm_model`
- `raw_result`
- `created_at`
- `updated_at`

建议枚举：

- `status`
  - `draft`
  - `voiceover_ready`
  - `planned`
  - `rendered`
  - `failed`

说明：

- 对信息流短视频，`script_text` 就是最终口播或旁白配音文本
- 文案不是字幕对象，字幕应由最终音频识别或对齐生成

### 8.3 `voiceovers`

配音音频表。

建议字段：

- `id`
- `script_variant_id`
- `storage_key`
- `voice_provider`
- `voice_model`
- `voice_name`
- `duration_ms`
- `status`
- `raw_result`
- `created_at`
- `updated_at`

建议枚举：

- `status`
  - `pending`
  - `completed`
  - `failed`

### 8.4 `narration_segments`

主叙事音频时间轴表。

建议字段：

- `id`
- `script_variant_id`
- `voiceover_id`
- `segment_index`
- `text`
- `start_ms`
- `end_ms`
- `selling_point_id`
- `intent`
- `confidence`
- `created_at`
- `updated_at`

建议枚举：

- `intent`
  - `hook`
  - `feature`
  - `benefit`
  - `usage`
  - `cta`
  - `other`

说明：

- `narration_segment` 是画面剪辑的主时间轴锚点
- 画面 `clip_segment` 可关联到某个 `narration_segment`

## 9. 编排与时间线

### 9.1 `edit_plans`

编排方案表。

建议字段：

- `id`
- `generation_task_id`
- `script_variant_id`
- `voiceover_id`
- `product_id`
- `status`
- `target_duration_ms`
- `actual_duration_ms`
- `plan_json`
- `validation_status`
- `validation_errors`
- `created_at`
- `updated_at`

建议枚举：

- `status`
  - `draft`
  - `validated`
  - `rendering`
  - `rendered`
  - `failed`
- `validation_status`
  - `pending`
  - `passed`
  - `failed`

说明：

- `plan_json` 保存完整可复核编排方案
- 结构化字段用于快速查询状态和关联关系

### 9.2 `clip_segments`

画面时间线最小剪辑单元表。

建议字段：

- `id`
- `edit_plan_id`
- `narration_segment_id`
- `asset_id`
- `speech_segment_id`
- `sequence_index`
- `track_role`
- `source_in_ms`
- `source_out_ms`
- `timeline_in_ms`
- `timeline_duration_ms`
- `playback_rate`
- `use_original_audio`
- `mute`
- `audio_gain_db`
- `selling_point_id`
- `transition_in_hint`
- `transition_out_hint`
- `notes`
- `created_at`
- `updated_at`

建议枚举：

- `track_role`
  - `primary_visual`
  - `supporting_visual`
- `transition_in_hint`
  - `cut`
  - `fade`
  - `none`
- `transition_out_hint`
  - `cut`
  - `fade`
  - `none`

说明：

- `clip_segment` 严格单源、连续
- `clip_segment` 是画面时间线装配层的最小剪辑单元
- `narration_segment_id` 可为空，但建议尽量关联，用于表达该画面服务于哪段配音
- 字幕识别结果和 BGM 混音策略不放在 `clip_segments`

## 10. 渲染、字幕与 BGM

### 10.1 `render_jobs`

渲染任务表。

建议字段：

- `id`
- `edit_plan_id`
- `generation_task_id`
- `status`
- `priority`
- `output_storage_key`
- `render_config`
- `error_message`
- `created_by_user_id`
- `created_at`
- `updated_at`
- `started_at`
- `finished_at`

建议枚举：

- `status`
  - `queued`
  - `running`
  - `completed`
  - `failed`
  - `cancelled`

### 10.2 `subtitle_files`

字幕文件表。

建议字段：

- `id`
- `render_job_id`
- `storage_key`
- `format`
- `asr_provider`
- `asr_model`
- `status`
- `raw_result`
- `created_at`
- `updated_at`

建议枚举：

- `format`
  - `srt`
  - `vtt`
  - `ass`

说明：

- 字幕在画面剪辑完成后，基于最终时间线音频识别或对齐生成
- 字幕不作为编排阶段核心对象

### 10.3 `bgm_tracks`

BGM 资源表。

建议字段：

- `id`
- `name`
- `storage_key`
- `duration_ms`
- `mood`
- `tags`
- `status`
- `created_by_user_id`
- `updated_by_user_id`
- `created_at`
- `updated_at`

### 10.4 `render_outputs`

成片产物表。

建议字段：

- `id`
- `render_job_id`
- `generation_task_id`
- `script_variant_id`
- `storage_key`
- `file_name`
- `file_size`
- `duration_ms`
- `width`
- `height`
- `checksum`
- `status`
- `created_at`
- `updated_at`

## 11. 表关系摘要

核心关系：

- `products` 1:N `product_selling_points`
- `products` 1:N `assets`
- `assets` 1:1 `asset_analysis`
- `assets` 1:N `speech_segments`
- `generation_tasks` 1:N `script_variants`
- `script_variants` 1:1 `voiceovers`
- `script_variants` 1:N `narration_segments`
- `script_variants` 1:N `edit_plans`
- `edit_plans` 1:N `clip_segments`
- `edit_plans` 1:N `render_jobs`
- `render_jobs` 1:N `subtitle_files`
- `render_jobs` 1:N `render_outputs`

## 12. 设计结论

当前数据模型采用共享素材库设计。

核心结论：

- 不引入租户，不使用 `tenant_id`
- 用户用于登录、权限和审计
- clean shot 是服务端素材库的基础资产
- `speech_segment` 是口播素材的语义检索单元
- `script_text` 是最终配音文本
- `voiceover` 生成真实主叙事音频
- `narration_segment` 是画面剪辑的主时间轴锚点
- `clip_segment` 是画面时间线最小剪辑单元
- 字幕由最终音频识别或对齐生成
- BGM 是后置混音层
- `edit_plan` 保存可复核编排方案
