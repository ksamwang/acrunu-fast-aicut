# Web 前端工程化拆分任务清单

## 1. 目标与边界

- [ ] 将 `apps/web/src/main.tsx` 从页面、API、领域类型、路由和应用壳混合文件拆分为清晰的功能域模块
- [ ] 将 `apps/web/src/preprocess-page.tsx` 的工作区、导入、处理弹窗、VLM、提交等职责拆分为独立组件与 API 层
- [ ] 将 `apps/web/src/styles.css` 拆分为全局基础样式与按功能域归属的局部样式
- [ ] 保持现有 Hash 路由、登录态、服务端 API 和 Local Agent 协议不变
- [ ] 保持所有前端页面仅使用中文文案
- [ ] 保持预处理弹窗的 IDE 硬边风格，其他页面继续使用现有 Ant Design 视觉语言
- [ ] 不以重构名义改变已确认的产品交互、数据结构或服务端接口

## 2. 目录与模块边界

- [x] 建立 `src/app/`，承载应用入口、会话恢复、Hash 路由与应用壳
- [x] 建立 `src/shared/api/`，承载通用 HTTP 请求、服务端 API 和 Local Agent API
- [x] 建立 `src/shared/types/`，承载跨功能域使用的领域类型
- [x] 建立 `src/shared/lib/`，承载格式化、浏览器存储和无业务归属的工具函数
- [ ] 建立 `src/shared/components/`，仅放置跨两个及以上功能域复用的展示组件
- [x] 建立 `src/features/auth/`
- [ ] 建立 `src/features/products/`
- [ ] 建立 `src/features/assets/`
- [ ] 建立 `src/features/preprocess/`
- [ ] 建立 `src/features/settings/`
- [ ] 建立 `src/features/tasks/`
- [ ] 约束功能域之间不直接导入对方的内部组件或内部实现
- [ ] 约束跨功能域复用内容必须通过 `shared/` 暴露

## 3. 应用基础层

### 3.1 应用入口与路由

- [x] 提取 `App` 到 `src/app/App.tsx`
- [x] 提取应用壳到 `src/app/AppShell.tsx`
- [x] 提取 Hash 路由解析、路由常量和跳转逻辑到 `src/app/routes.ts`
- [x] 保持刷新后仍能恢复当前 Hash 页面
- [x] 保持刷新后仍能恢复有效登录态
- [ ] 保持登录失效时回到登录页的现有行为
- [ ] 保持左侧菜单与当前 Hash 路由同步

### 3.2 共享 API 层

- [x] 提取通用 `apiRequest`、认证头和统一错误处理到 `src/shared/api/http.ts`
- [x] 建立 `src/shared/api/server-api.ts`，集中服务端请求入口
- [x] 建立 `src/shared/api/local-agent-api.ts`，集中 `127.0.0.1:58721` 请求入口
- [x] 将 Local Agent 地址和超时策略移出页面组件
- [ ] 保持服务端 API 的请求路径、方法、请求体与错误展示行为不变
- [ ] 保持 Local Agent API 的请求路径、方法、请求体与错误展示行为不变

### 3.3 共享类型与工具

- [x] 提取认证会话、用户类型到 `src/shared/types/auth.ts`
- [x] 提取产品、卖点和产品统计类型到 `src/shared/types/product.ts`
- [x] 提取素材、抽帧、语义预览、向量对象类型到 `src/shared/types/asset.ts`
- [x] 提取预处理工作区、探测结果、VLM 标注类型到 `src/shared/types/workspace.ts`
- [x] 提取供应商、模型和运行控制类型到 `src/shared/types/settings.ts`
- [ ] 统一跨页面重复的时长、分辨率、帧率和状态文案格式化函数
- [x] 提取 Local Storage 键及读写逻辑，包含上次正式提交产品选择
- [ ] 清理重复或语义不一致的前端类型定义

## 4. 产品与卖点功能域

- [x] 提取产品列表页面到 `src/features/products/ProductsPage.tsx`
- [ ] 提取产品编辑、新建弹窗到 `ProductEditorModal.tsx`
- [ ] 提取产品白底参考图上传与预览组件
- [ ] 提取卖点编辑、新建弹窗到 `SellingPointModal.tsx`
- [ ] 提取关联素材预览弹窗到 `LinkedAssetsModal.tsx`
- [x] 将产品与卖点请求收敛到 `src/features/products/api.ts`
- [x] 将产品功能域样式迁移到 `src/features/products/styles.css`
- [ ] 保持产品编辑、删除约束、卖点编辑和关联素材预览行为不变
- [x] 确认无引用后删除 `LegacyProductsPage`

## 5. 素材库功能域

- [x] 提取素材库页面到 `src/features/assets/AssetsPage.tsx`
- [ ] 提取筛选工具栏到 `AssetFilterToolbar.tsx`
- [x] 提取素材卡片网格到 `AssetGrid.tsx`
- [x] 提取单个素材预览卡片到 `AssetCard.tsx`
- [ ] 提取素材详情弹窗到 `AssetDetailModal.tsx`
- [ ] 提取素材详情中的基础信息、抽帧预览、VLM 信息、语义预览和人工编辑面板
- [x] 将素材列表、详情、抽帧、语义预览和向量请求收敛到 `src/features/assets/api.ts`
- [ ] 将素材展示字段转换逻辑收敛到 `asset-formatters.ts`
- [x] 将素材库样式迁移到 `src/features/assets/styles.css`
- [ ] 保持素材卡片网格布局、列表卡片 body 滚动和固定分页行为不变
- [ ] 保持素材详情内部滚动，禁止 body 滚动的现有约束

## 6. 预处理功能域

### 6.1 工作区与导入

- [x] 提取预处理页面入口到 `src/features/preprocess/PreprocessPage.tsx`
- [ ] 提取工作区工具栏到 `WorkspaceToolbar.tsx`
- [ ] 提取预处理素材卡片到 `WorkspaceCard.tsx`
- [ ] 提取导入视频弹窗到 `ImportVideosModal.tsx`
- [ ] 提取导入视频的文件预览、缩略图和确认导入逻辑
- [ ] 将工作区查询、导入、清空和删除请求收敛到 `src/features/preprocess/api.ts`
- [ ] 保持待处理、本地已保存、待提交、已入库的状态展示与清空能力不变

### 6.2 处理弹窗与分析

- [ ] 提取预处理大弹窗到 `PreprocessModal.tsx`
- [x] 将 `VideoTrimEditor` 移入 `src/features/preprocess/`，保持其对外 Props 和帧级 I/O 语义不变
- [ ] 提取本地分析结果展示组件
- [ ] 提取三帧抽样弹窗到 `FramePreviewModal.tsx`
- [ ] 提取口播转写弹窗到 `TranscriptModal.tsx`
- [ ] 提取备注弹窗
- [ ] 提取 VLM 标注触发与异步状态展示组件
- [ ] 提取产品参考图开关与预览逻辑
- [ ] 保持空格播放/暂停、`I`/`O` 出入点、帧级时间轴和时间标尺行为不变
- [ ] 保持三帧基于选区入点、中点、出点的抽样规则不变
- [ ] 保持解释帧率/慢放后源视频、时间轴、I/O 与提交元数据一致
- [ ] 保持正式提交必须选择产品及记住上次产品选择的约束
- [x] 将预处理样式迁移到 `src/features/preprocess/styles.css`

## 7. 设置与任务功能域

### 7.1 系统设置

- [x] 提取设置页到 `src/features/settings/SettingsPage.tsx`
- [ ] 提取模型供应商 Tab 到 `ProviderSettingsTab.tsx`
- [ ] 提取默认模型 Tab 到 `ModelSettingsTab.tsx`
- [ ] 提取运行控制 Tab 到 `RuntimeSettingsTab.tsx`
- [ ] 保持供应商、LLM、VLM、向量模型的选择与模型拉取行为不变
- [ ] 保持向量模型支持手动输入的行为不变
- [x] 将设置请求收敛到 `src/features/settings/api.ts`
- [ ] 将设置页滚动限制在页面内容区域，禁止 body 滚动
- [x] 将设置样式迁移到 `src/features/settings/styles.css`
- [x] 确认无引用后删除 `LegacySettingsPage`

### 7.2 任务

- [x] 提取任务页面到 `src/features/tasks/TasksPage.tsx`
- [x] 将任务查询与操作请求收敛到 `src/features/tasks/api.ts`
- [ ] 将任务功能域样式迁移到 `src/features/tasks/styles.css`
- [ ] 保持现有任务列表、筛选和状态展示行为不变

## 8. 样式与依赖治理

- [x] 将 `src/styles.css` 缩减为 reset、应用壳、全局高度与基础 Ant Design 覆盖
- [ ] 保持 `html`、`body`、`#root` 禁止滚动的全局约束
- [x] 将功能域样式按 `assets`、`preprocess`、`products`、`settings`、`tasks` 迁移
- [ ] 保留现有 CSS 类名前缀，避免迁移期间发生选择器冲突
- [ ] 每迁移一个功能域，清除原全局文件中对应的重复规则
- [ ] 检查不同页面的滚动容器只有一个预期滚动源
- [ ] 检查预处理弹窗外页面不会继承 IDE 风格控件样式
- [ ] 不新增与现有设计系统冲突的第三方 UI 库

## 9. 迁移顺序与回归

- [ ] 第一批：完成 `app/`、`shared/api/`、`shared/types/`，不改变任何页面 UI
- [ ] 第一批：执行 `npm.cmd run build`
- [ ] 第一批：验证登录、刷新 Hash 路由和退出登录
- [ ] 第二批：完成产品与素材库功能域拆分
- [ ] 第二批：执行 `npm.cmd run build`
- [ ] 第二批：验证产品、卖点、素材筛选、素材详情、分页和列表滚动
- [ ] 第三批：完成预处理功能域拆分
- [ ] 第三批：执行 `npm.cmd run build`
- [ ] 第三批：验证导入、I/O 裁切、三帧、VLM、备注、转写、保存和正式提交
- [ ] 第四批：完成设置、任务功能域拆分和遗留页面清理
- [ ] 第四批：执行 `npm.cmd run build`
- [ ] 第四批：验证供应商、模型、向量模型手动输入、设置页滚动和任务页
- [ ] 每批次补充或更新对应 Playwright 用例
- [ ] 每批次通过 Playwright 核心路径回归后才进入下一批次

## 10. 完成标准

- [x] `main.tsx` 仅保留 React 挂载，或被替换为最小入口文件
- [x] 不存在承载多个业务页面的单一 TSX 文件
- [x] 不存在混合所有功能域页面样式的单一业务样式文件
- [ ] 服务端 API 与 Local Agent API 均有独立、可复用的调用层
- [ ] 前端共享领域类型只有一个权威定义位置
- [ ] 已确认无引用的 `LegacyProductsPage` 与 `LegacySettingsPage` 已删除
- [x] 现有 `npm.cmd run build` 通过
- [x] 核心用户路径的 Playwright 回归通过
- [ ] 手工验证桌面视口下无 body 滚动、无内容溢出、无卡片布局回归
- [ ] 所有任务仅在有代码与验证证据后勾选
