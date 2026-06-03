# 任务处理优化设计方案

> 日期: 2026-06-03
> 项目: download-manager

## 综述

使用 4 个独立阶段依次优化 download-manager 的任务处理体系，每阶段产出完整可提交代码，包含完整性检查和潜在 bug 修复。

### 执行顺序

**Phase 1: 代码可维护性重构** → **Phase 2: 系统性能优化** → **Phase 3: 运维可观测性** → **Phase 4: Web UI 体验打磨**

选择理由：先拆分大文件以建立清晰的修改边界，再优化性能（因可维护后定位瓶颈更易），再增加可观测性，最后打磨 UI。

---

## Phase 1: 代码可维护性重构

### 目标

将 1569 行的 `manager.go` 按职责拆分为多个同包文件，使每个文件专注于一个职能域。不改变运行时行为，不引入新接口或抽象。

### 现有已拆分文件（保持不动）

| 文件 | 职责 |
|------|------|
| `aggregation_service.go` | 聚合查询逻辑 |
| `config_service.go` | 配置持久化/备份/审计 |
| `config_mgr.go` | 配置变更管理 |
| `runtime_mgr.go` | 运行时调整（worker 数量、任务热更新） |
| `task_loader.go` | 任务加载与生命周期 |
| `url_registry.go` | URL 状态注册表 |

### 新增拆分文件

#### 1. `manager/scheduler.go` — 扫描 + 调度 + 工作线程

从 `manager.go` 迁移的方法：

| 方法 | 行数 | 说明 |
|------|------|------|
| `Start()` | 55 | 主循环启动，含 ticker 循环 |
| `Stop(ctx)` | 30 | 关闭流程 |
| `WaitForShutdown(ctx)` | 15 | 等待退出 |
| `scan()` | 70 | 两阶段扫描（scrape + download） |
| `processTask(t)` | 60 | 任务级对象调度 |
| `scheduler()` | 65 | 加权轮询调度器 |
| `worker()` | - | 全局队列消费者 |
| `getTaskQueue(id)` | 10 | per-task 队列访问 |
| `closeAllTasks()` | - | 关闭所有任务 |

**完整性检查**:
- 确保 `Manager` 结构体的 `stopChan`/`workerStop`/`schedulerStop`/`workerWg`/`downloadQueue`/`taskQueues` 等字段在该文件中使用
- 检查 `Ticker` 的 Stop() 在 `Start()` defer 中正确执行
- 验证 `scan()` 的 `CompareAndSwap` 防御与 `scrapingTask` guard 没有在拆分后丢失

**潜在 bug 修复**:
- `processTask()` 中 `slotsAvailable` 计算在 `active` 并发变更后可能为负值 → 添加 `max(0, ...)` 保护
- `scheduler()` 中 `expanded` 切片初始容量 `64` 不足以容纳所有任务 × 权重(`max 8`) → 改为 `len(ids)*maxWeight`

#### 2. `manager/download.go` — 下载执行 + 重试策略

从 `manager.go` 迁移的方法：

| 方法 | 行数 | 说明 |
|------|------|------|
| `download(t, obj)` | 100 | 核心下载流程 |
| `forceDownload(t, obj)` | 15 | 绕过队列立即下载 |
| `applyGroupPriorityPolicies(t, obj)` | 90 | 内容分组优先级策略 |
| `RetryObject(taskID, url)` | 35 | 单对象重试 |
| `RetryAllFailed(taskID)` | 25 | 全任务失败重试 |

**完整性检查**:
- `download()` 中的 `stopChan` 检查、`SetContext` 传播、失败计数、metrics 更新路径
- `failedCount.LoadOrStore` → `Add(1)` → 检查 `maxRetries >= 5` 的完整链路
- metrics 的 `CompareAndSwap` 无锁更新是否线程安全

**潜在 bug 修复**:
- `failedCount` 当前硬编码 5 次重试上限，未使用 `cfg.Downloader.MaxRetries`

#### 3. `manager/events.go` — 事件总线 + 进度广播

从 `manager.go` 迁移的方法：

| 方法 | 行数 | 说明 |
|------|------|------|
| `Subscribe()` | 6 | 订阅事件 |
| `Unsubscribe(ch)` | 8 | 取消订阅 |
| `publish(e)` | 12 | 发布事件 |
| `broadcastProgress()` | 15 | 每秒进度广播 |
| `BroadcastTaskUpdate(taskID)` | 35 | 任务统计广播 |

**完整性检查**:
- `subscribers` 的 `eventMu.RWMutex` 保护进/出
- `publish` 的 `default` 分支（drop on slow consumer）未丢失关键事件
- `lastProgress` 的 `LoadOrStore`/`Store` 模式正确过滤未变更进度

#### 4. `manager/tasks.go` — 任务 CRUD + 对象操作

从 `manager.go` 迁移的方法：

| 方法 | 行数 | 说明 |
|------|------|------|
| `CancelTask(id)` | 30 | 取消整任务 |
| `CancelTasks(ids)` | 10 | 批量取消 |
| `CancelObject(taskID, url)` | 35 | 取消单对象 |
| `UndoCancelObject(taskID, url)` | 25 | 撤销取消 |
| `getTask(id)` | 5 | 获取任务 |
| `getTaskObject(t, url)` | 15 | 获取任务对象 |
| `ReorderObject(taskID, url, newIndex)` | 15 | 对象排序 |

**完整性检查**:
- `CancelObject` 中 `downloadingObj.Delete` 是否在合适位置调用
- `CancelTask` 中调用 `downloader.Cancel(url)` 前检查是否实现了 Cancel 接口
- `UndoCancelObject` 中调用 `go m.processTask(t)` 确保并发安全

**潜在 bug 修复**:
- `CancelObject()` 中有两处可能重复删除 `downloadingObj`（第 1399 行和第 1448 行），确认是否为 bug 并修正

#### 5. `manager/aggregate.go` — 聚合查询方法

从 `manager.go` 迁移的方法：

| 方法 | 行数 | 说明 |
|------|------|------|
| `AggregateObjects(page, limit, ...)` | 1 | 委托给 aggSvc |
| `AggregateByContent(page, limit, ...)` | 130 | 按内容分组聚合 |
| `GetObjectsByScopedGroup(taskID, taskType, group)` | 20 | 获取分组对象 |
| `BackfillContentGroups()` | 50 | 回填 content_group 元数据 |
| 辅助函数: `metadataContentGroup()`、`metadataTaskType()`、`scopedContentGroupKey()`、`variantPriorityScore()` | 各 5-15 | |

**完整性检查**:
- `AggregateByContent` 的 `typeMatches()` 与 `Sort` 逻辑
- `variantPriorityScore` 的 `titlegroup.TKTVariantFlags` 解析逻辑
- 分页 `ApplyQueryToObjects` 在聚合后重复排序是否一致

### 拆分后 `manager.go` 保留内容

| 内容 | 预估行数 |
|------|----------|
| `Manager` 结构体定义（文档注释 + 字段） | 50 |
| `NewManager()` 构造函数 | 45 |
| `RuntimeFeatures` 类型 + `FeaturesStatus()` | 10 |
| `getAllTasks()` / `currentCfg()` / `GetConfig()` 等访问器 | 20 |
| `searchTaskObjects()` / `countTaskObjects()` / `collectTaskObjects()` | 50 |
| `GetTaskSummaries()` / `GetTaskDetails()` / `GetActiveDownloads()` | 80 |
| `GetDownloadRootDir()` / `flushAllStorages()` | 25 |
| `cloneStorageQuery()` / `queryForTask()` / `sortRules()` 等工具 | 20 |

总计保留约 **300 行**。

### 迁移策略

1. 逐个创建新文件，每次 build + vet + test 验证
2. 确保 `gofmt -s` 格式化一致
3. 检查 `go test -run . ./manager/...` 全部通过
4. 提交流程（后续由 writing-plans 细化）：每拆 1-2 个文件一个 commit

---

## Phase 2: 系统性能优化

### 范围

在 Phase 1 拆清代码边界后，优化任务处理的性能瓶颈。

### 已识别潜在问题

1. **两层队列延迟**：per-task 队列（容量 32）→ scheduler（50ms tick）→ 全局队列，一条等待对象经过至少 50ms 才到达 worker
2. **scheduler 每 2 秒全量重算**：对所有任务的 metrics 遍历加锁，可能影响高频吞吐
3. **scan 串行 processTask**：当前所有任务在同一周期内 `go m.processTask(t)` 并发执行，但 `GetDownloadObjects()` 可能重复拉取同一批对象
4. **per-task queue buffer 固定 32**：积压多时丢失请求（scheduler 中 `default` 分支丢弃）
5. **Event 广播全部走 JSON 序列化**：高频进度事件可能成为瓶颈

### 优化方向

- **调度器降频或事件驱动**：将 50ms tick 改为信号触发（有对象入队即调度）
- **per-task 队列容量可调**：根据任务并发数动态计算
- **去重缓存优化**：减少 `downloadingObj` 的 `LoadOrStore` 竞争
- **进度广播节流**：聚合 1s 窗口内的进度变更，批量发送

*（Phase 2 的详细设计方案将在 Phase 1 完成并提交后展开）*

---

## Phase 3: 运维可观测性

### 范围

为系统增加诊断和观测能力，便于排查问题。

### 规划方向

- 增加 Manager / Scheduler / Worker 各级状态指标 API
- 失败原因明细导出（完整 error chain）
- 任务处理延迟分布（P50/P95/P99）
- 配置变更审计日志增强
- 健康检查端点丰富

*（Phase 3 的详细设计方案将在 Phase 2 完成并提交后展开）*

---

## Phase 4: Web UI 体验打磨

### 范围

改善前端交互体验。

### 规划方向

- 任务列表实时状态刷新优化
- 批量操作（多选 → 取消/重试）交互
- 进度展示优化（大文件下载进度粒度）
- 错误信息展示与交互
- 配置编辑界面对比/回滚可视化增强

*（Phase 4 的详细设计方案将在 Phase 3 完成并提交后展开）*

---

## 工程规范

- 所有修改遵循 `CLAUDE.md` 中的 AGENTS.md 约定
- 使用 `gofmt -s` 格式化
- 提交前 `go build ./...` + `go vet ./...` + `go test ./...` 全部通过
- 每次 commit 信息包含：`refactor(manager): Phase 1 - <具体变更>`
- 全部使用 UTF-8 without BOM

## 变更日志

- 2026-06-03: 初版设计文档