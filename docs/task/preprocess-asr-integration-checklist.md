# 预处理 ASR 接入：第 1-2 阶段任务清单

本文档只拆解预处理 ASR 接入的前两个实施阶段：

1. 服务端提供受认证的无状态 ASR 代理接口。
2. `local-agent` 从当前 I/O 选区提取临时音频，调用服务端并保存结构化转写草稿。

前端转写弹窗交互、人工校对后的确认写入、正式提交协议调整和最终成片字幕生成不在本清单范围内。

## 0. 已确认基线与边界

- [x] FunASR 已作为独立 Docker Compose 服务 `asr` 部署
- [x] FunASR 容器健康检查为 `GET /healthz`
- [x] FunASR 原始推理接口为 `POST /v1/transcriptions`，文件字段为 `file`
- [x] FunASR 当前返回 `text`、`timestamp` 和 `sentence_info`
- [x] FunASR 推理明确启用 `sentence_timestamp=True`，优先返回句子级时间段
- [x] ASR 服务仅绑定服务器 `127.0.0.1:10096`，客户端不能直接访问
- [x] 未入库素材只允许上传当前 I/O 选区的临时音频，不允许服务端读取本地原始视频
- [x] ASR 请求不创建 `asset`、任务、素材库预览、检索记录或 SRT 文件
- [x] ASR 结果先作为本地工作区草稿，只有用户确认后的转写才允许随 clean shot 正式提交
- [x] 本机真实验收样本已确认存在：`E:\ethan-work\0708-束裤带信息流拍摄\26-07-09-束裤带成片\26-07-09-束裤带成片-4\26-07-09-束裤带成片-4.mp4`
- [x] 验收样本已探测为 `19.017s`、H.264 视频、AAC 音频、`44100Hz`、双声道
- [x] 已验证该样本可导出 `19.017s`、`16000Hz`、单声道、`pcm_s16le` WAV，文件大小约 `608622` 字节

时间坐标统一规则：

- ASR 输入只包含当前 I/O 选区，ASR 返回的句段时间以选区起点为 `0ms`
- 本地 ASR 草稿和正式入库后的 clean shot 句段都保存选区相对时间
- 在原始素材时间轴上回显句段时，才使用 `source_in_ms + segment.start_ms/end_ms`
- 不把 ASR 毫秒时间戳提前转换成 SRT，也不在这一阶段转换成固定帧率时间码

## 1. 服务端无状态 ASR 代理接口

### 1.1 配置与客户端

- [x] 在服务端配置中增加 `ASR_BASE_URL`，Docker Compose 默认值为 `http://asr:10096`
- [x] 增加独立的 ASR 请求超时配置，并保证请求取消能够传递到上游 FunASR
- [x] API 服务启动不强依赖 ASR 已健康；ASR 尚未就绪时返回明确的可恢复错误
- [x] 实现 FunASR HTTP 客户端，使用 multipart 将音频字段 `file` 转发到 `/v1/transcriptions`
- [x] 为 FunASR 响应定义明确类型，不在业务代码中传递无约束的 `map[string]any`
- [x] 服务端代理不进入任务队列，也不额外增加业务并发上限
- [x] 记录请求耗时、结果状态和错误类型，但日志不得写入音频内容、完整转写正文或认证信息

### 1.2 预处理接口协议

- [x] 新增受登录认证保护的 `POST /api/preprocess/asr-transcribe`
- [x] 请求使用 multipart，至少包含 `file`、`source_in_ms`、`source_out_ms`
- [x] 不要求 `asset_id`、`product_id` 或服务端素材记录，因为此时素材尚未入库
- [x] 校验只上传一个音频文件
- [x] 校验 `source_in_ms >= 0` 且 `source_out_ms > source_in_ms`
- [x] 设置合理的请求体大小上限，并对超限请求返回稳定的业务错误码
- [x] 拒绝空文件、无法识别的音频和缺失时间范围的请求
- [x] 将 FunASR 的 `text`、`timestamp`、`sentence_info` 归一化为服务端稳定响应
- [x] 标准响应至少包含 `text`、`segments`、`source_in_ms`、`source_out_ms` 和 `time_base`
- [x] 每个 `segments` 元素至少包含 `start_ms`、`end_ms`、`text`
- [x] `time_base` 固定返回 `selection_relative_ms`
- [x] 校验归一化后的句段满足 `0 <= start_ms < end_ms <= source_out_ms - source_in_ms`
- [x] FunASR 未返回有效 `sentence_info` 时，保留完整 `text`，并采用明确的降级规则生成或省略句段

建议响应结构：

```json
{
  "data": {
    "text": "识别后的完整文本",
    "segments": [
      {
        "start_ms": 320,
        "end_ms": 2480,
        "text": "第一句识别文本"
      }
    ],
    "source_in_ms": 3000,
    "source_out_ms": 12000,
    "time_base": "selection_relative_ms"
  }
}
```

### 1.3 临时文件与错误处理

- [x] 服务端 API 不持久化上传音频；必须落盘时使用独立临时目录并在成功、失败、超时后统一清理
- [x] 确认 FunASR 容器自身的临时上传文件在推理结束后删除
- [x] 将参数错误映射为 `400`
- [x] 将 ASR 尚未就绪或不可用映射为稳定的 `503` 业务错误
- [x] 将上游超时映射为 `504`
- [x] 将上游异常响应或无法解析的 JSON 映射为 `502`
- [x] 错误响应保留可供前端展示的中文信息，同时保留稳定的机器错误码
- [x] 任何失败都不得创建数据库记录、素材处理任务或遗留临时文件

### 1.4 服务端测试

- [x] 使用 `httptest.Server` 模拟 FunASR，验证 multipart 文件和字段转发正确
- [x] 覆盖正常文本与多个句段的归一化
- [x] 覆盖空 `sentence_info` 的降级行为
- [x] 覆盖非法时间范围、空文件和请求体超限
- [x] 覆盖 FunASR `503`、超时、非 JSON 响应和连接失败
- [x] 验证未登录请求不能访问预处理 ASR 接口
- [x] 验证调用前后 `asset`、任务和检索记录数量不变
- [x] 验证服务端临时目录在成功和失败后都为空

### 1.5 第 1 阶段完成标准

- [x] API 服务可以通过内部 Docker 网络调用 `http://asr:10096/v1/transcriptions`
- [x] 客户端只能通过受认证的 `/api/preprocess/asr-transcribe` 使用 FunASR
- [x] 服务端返回稳定的毫秒级结构化句段，不向调用方暴露 FunASR 原始结构差异
- [x] 服务端不保存预处理音频、转写草稿或任何未入库素材记录
- [x] 服务端单元测试和 HTTP 集成测试通过

阶段 1 验证记录（2026-07-13）：

- 本地执行 `go test ./...` 与 `go vet ./...` 通过
- 服务器执行 `docker compose config --quiet` 通过，API 容器使用 `ASR_BASE_URL=http://asr:10096`
- 修正 ASR 健康检查使用不存在的 `python` 后，容器状态恢复为 `healthy`
- 指定真实视频导出的 `19.017s` WAV 经服务端代理得到可读中文转写，句段范围为 `130ms..18655ms`
- 无效音频经真实 FunASR 调用返回 `502 / asr_invalid_response`
- API 与 ASR 容器检查均无 multipart 或推理临时文件残留

## 2. local-agent 按当前 I/O 选区提取并转写

### 2.1 音频提取能力

- [x] 在 `internal/ffmpeg` 增加独立的音频选区导出函数，不复用视频 clean shot 裁切参数
- [x] 在 `localagent.Processor` 增加可测试的 `ExtractAudio` 方法
- [x] 输入使用参数数组调用 `ffmpeg`，不得拼接 shell 命令，确保中文和空格路径可用
- [x] 导出参数固定为第一条音频流、`16000Hz`、单声道、`pcm_s16le` WAV
- [x] 导出范围严格使用当前请求的 `source_in_ms` 和 `source_out_ms`
- [x] 校验导出 WAV 的实际时长与选区时长一致，允许不超过一个音频采样周期或明确的小误差
- [x] 输入无音频流时返回明确错误，不生成空 WAV
- [x] I/O 超出当前工作源时长时直接拒绝，不自动扩展或重置 I/O
- [x] 音频临时文件使用每次请求独立目录，成功、失败和取消后都删除

预期 ffmpeg 参数语义：

```text
-ss <source_in_seconds> -t <selection_duration_seconds> -i <source>
-map 0:a:0 -vn -sn -dn -ac 1 -ar 16000 -c:a pcm_s16le <temporary.wav>
```

### 2.2 工作区转写接口

- [x] 新增 `POST /workspace/items/:itemID/transcribe`
- [x] 请求至少包含当前 `source_in_ms`、`source_out_ms`、`server_base_url` 和 `auth_token`
- [x] 校验工作区条目存在且当前素材类型为 `talking_head`
- [x] 校验当前工作源 `probe.has_audio=true`
- [x] 以请求中的当前 I/O 为准，不能使用导入时默认全片范围替代
- [x] 转写开始和结束后都不修改 `SourceInMs`、`SourceOutMs`、预览帧或时间轴状态
- [x] 将临时 WAV 以 multipart 上传到服务端 `/api/preprocess/asr-transcribe`
- [x] 透传当前登录令牌，并正确处理登录失效、服务端不可用和请求超时
- [x] 校验服务端回传的 `source_in_ms`、`source_out_ms` 与本次请求一致，防止旧请求结果覆盖新选区

### 2.3 本地结构化草稿

- [x] 为工作区条目增加结构化 ASR 草稿，至少保存完整文本、句段、对应 I/O 和生成时间
- [x] 句段在本地保持毫秒整数，不转换成 SRT 文本
- [x] ASR 草稿时间保持选区相对时间，范围必须位于 `0` 到选区时长之间
- [x] ASR 结果只更新草稿，不静默覆盖用户已经确认或手工编辑的 `Transcript`
- [x] 只有本次 I/O 仍与发起请求时一致时才写入草稿
- [x] 用户之后修改 I/O 时，将旧草稿标记为过期或清除，不能继续当作当前选区结果
- [x] 工作区状态持久化后重启 local-agent 仍能恢复 ASR 草稿
- [x] ASR 失败时保留原有人工转写和上一次有效草稿，并返回本次错误
- [x] 不生成 `.srt` 文件，不创建 clean shot，不触发正式提交

### 2.4 local-agent 自动化测试

- [x] 为 ffmpeg 音频导出参数增加单元测试，覆盖中文路径和非零 I/O
- [x] 使用 Processor stub 验证提取范围严格等于请求 I/O
- [x] 使用 HTTP 测试服务验证上传文件格式、认证头和时间范围字段
- [x] 覆盖无音频、非法 I/O、ffmpeg 失败、服务端失败、超时和返回范围不匹配
- [x] 覆盖请求期间用户改变 I/O 时旧结果不得覆盖当前工作区
- [x] 覆盖已有人工 `Transcript` 不被 ASR 草稿覆盖
- [x] 覆盖成功、失败和取消后的本地临时文件清理
- [x] 覆盖 local-agent 重启后结构化草稿恢复

### 2.5 真实视频验收

真实验收不使用 mock 音频，直接从以下文件提取：

```text
E:\ethan-work\0708-束裤带信息流拍摄\26-07-09-束裤带成片\26-07-09-束裤带成片-4\26-07-09-束裤带成片-4.mp4
```

- [x] 直接把上述绝对路径传给 `ExtractAudio`，证明中文路径可用
- [x] 使用全范围 `I=0ms`、`O=19017ms` 导出 WAV
- [x] 使用 `ffprobe` 确认 WAV 为 `pcm_s16le`、`16000Hz`、单声道、约 `19.017s`
- [x] 将该 WAV 经服务端代理发送给 FunASR，返回非空且可读的中文转写
- [x] 确认所有句段时间均位于 `0..19017ms`
- [x] 再选择一个非零入点的子区间，确认实际导出时长等于 `O-I`
- [x] 确认子区间 ASR 句段仍从选区相对 `0ms` 开始，不错误叠加源视频入点
- [x] 确认在原始视频时间轴回显时使用 `source_in_ms + segment time`
- [x] 转写前后核对时间轴 I/O 完全不变
- [x] 核对 local-agent 与服务端均无遗留临时音频
- [x] 核对服务端没有新增素材、任务、向量或素材级日志记录

### 2.6 第 2 阶段完成标准

- [x] local-agent 能从任意有效 `talking_head` 当前 I/O 选区导出标准 WAV
- [x] local-agent 能通过受认证服务端接口取得结构化 FunASR 结果
- [x] 真实样本能够得到可读转写和有效毫秒级句段
- [x] ASR 草稿与发起请求时的 I/O 严格绑定，不重置时间轴，也不覆盖人工确认文本
- [x] 全链路不生成 SRT、不提前提交素材、不遗留临时音频
- [x] 自动化测试与真实视频验收全部通过后，才进入前端转写弹窗接入阶段

阶段 2 验证记录（2026-07-13）：

- `go test ./internal/ffmpeg ./internal/localagent` 与 `go vet ./internal/ffmpeg ./internal/localagent` 通过
- 指定真实视频全范围 `0..19017ms` 导出为 `pcm_s16le`、`16000Hz`、单声道 WAV，并得到非空中文草稿
- 同一真实视频在 `3000..12000ms` 子区间再次转写成功；返回句段保持选区相对时间，映射回源时间轴后未越界
- 本机 HTTP 代理环境下，local-agent 对服务器地址强制直连，避免流式 multipart 被代理中断
- 调用前后服务器素材数量保持 `6 -> 6`，任务数量保持 `16 -> 16`
- API 与 FunASR 容器无残留 multipart 或临时音频，FunASR 状态为 `healthy`
