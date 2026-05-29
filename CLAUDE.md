# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 构建与运行

```bash
make build                              # gofmt + 构建到 build/bin/download-manager
make run                                # build + 用 build/config.yaml 运行
./build/bin/download-manager --config config.yaml   # full 模式
./build/bin/download-manager --ui-only              # UI-only 模式
./build/bin/download-manager --run-mode=ui          # 等价 --ui-only
```

测试：`go test ./...` 或 `go test -v -run TestXxx ./path/...`

## 架构概览

单二进制，`main.go` → `manager.Manager` 编排全局流程。启动顺序：flag/env 解析 → YAML 配置加载 → 日志初始化 → flock 单实例锁 → goroutine 启动 Manager + HTTP server。

### Manager（`manager/manager.go`）

核心编排器，持有：
- `sync.Map` 的任务注册表（`tasks`）
- 全局下载队列（`downloadQueue` channel）+ 每任务队列（`taskQueues`）
- 事件总线（发布/订阅模式，SSE 使用）
- 去重缓存（`downloadingObj`）、失败计数、metrics
- 调度器 goroutine（加权轮询分發任务队列到全局队列）
- 定时扫描（`scan()` → `processTask()` 拉取 `GetDownloadObjects()`）

### 任务系统（`task/`）

通过工厂注册模式实现可插拔任务类型：

- `task/factory.go` — 全局 map `factories[string]Factory`，`Register(typ, fn)` + `NewTask(cfg, opts)`
- 每种任务类型在 `init()` 中注册自己：`hanime`、`tktube`、`urllist`、`vikacg`
- 扩展新任务：在 `task/` 下新建包，实现 `core.Task` 接口，在 `init()` 中调用 `task.Register()`，在 `config.yaml` 中添加对应配置

### 核心接口（`core/interfaces.go`）

```
Task            — ID, Type, Storage, SetDownloader, GetDownloadObjects, UpdateStatus, Concurrency, Start/Close
Storage         — Get, Update, Delete, Search, Count, Exists
Downloader      — Download(obj, headers) error
SharedRegistry  — 跨任务 URL 状态共享（Get/Update/Delete）
FailedTask      — MarkAsFailed 标记永久失败
EventBus        — Subscribe/Unsubscribe
```

### BaseTask（`task/base_task.go`）

通用基础实现，任务嵌入此 struct 并覆盖 `Type()` 和 `GetDownloadObjects()`。提供：
- 状态管理：`UpdateStatus`（落存储 + 共享注册表 + 运行时列表），`CheckAndRestoreStatus`
- 路径策略 `PathStrategy`：`first_fixed` 模式等
- 刷新器 `CommonRefresher` + 分页器 `CommonPager`
- 并发度控制（`concurrency atomic.Int64`）
- 对象管理：`GetAllObjects`, `MoveObject`, `RememberRuntimeObject`, `FlushObject`

### 存储层（`storage/`）

工厂注册模式，支持三种后端：
- `file` — JSON 文件存储，支持延迟落盘与 `ForceFlush`
- `mongo` — MongoDB 存储，按 collection + database 分
- `memory` — 进程内 map

在 `storage/factory.go` 的 `init()` 中注册。

### 下载器（`downloader/`）

- `native.go` — `NativeHTTPDownloader`，基于 `cavaliergopher/grab`，支持域名限流、进度回调、重试
- `wget.go` — 调用系统 wget
- `scraper.go` — 调用外部抓取程序
- `pkg/dlcore/` — 细粒度 HTTP 客户端、HLS/ffmpeg 处理、文件系统封装

### 配置（`config/`）

YAML 配置，结构体在 `config/config.go`，含 ValidateAndClamp 做默认值填充与旧字段迁移。新增配置段需同步：
1. `config.go` Config 结构体字段
2. `ValidateAndClamp` 中的默认值/迁移逻辑
3. `Diff()` 方法中的变更检测

### API 层（`api/server.go`）

基于 `gorilla/mux`，REST + SSE + embedded Web UI：
- `/api/tasks`, `/api/aggregate`, `/api/events`（SSE），`/files/`（文件浏览）
- 写接口在 UI mode 下被 `wrapWrite` 中间件拦截
- `api/server_write_guard_test.go` 覆盖写保护测试

### Web UI（`web/`）

- `web/static/index.html` 是主页面（内嵌 Vue 3，当前为单文件）
- `web/embed.go` 通过 `//go:embed static/*` 嵌入
- `web/static/utils/taskTypes.js` — 任务类型工具函数

### 数据模型

`model.DownloadObject`：
```
TaskID, URL, SavePath, Status(pending/downloading/completed/failed/cancelled),
Progress(int), Metadata(map[string]string), Extra(map[string]any)
```

## 关键模式

- **事件驱动**：Manager 持有订阅者 map，publish() 广播 Event（`task_update`, `task_list_change`, `object_update`），API 通过 SSE 推送
- **内容分组**：tktube 任务按 `content_group` metadata 聚合变体（分辨率/字幕标记），通过 `variantPriorityScore` 选代表，自动取消低优先级 pending 对象
- **共享状态**：`SharedRegistry` 跨任务共享 URL 状态，用于去重与状态对齐
- **运行时热更新**：`UpdateConfig()` 保存新配置 → 写备份 → 重建下载器 → 调整 worker → 热加载任务

## 工程约定

- `errors`, `os/io`, `net/http`, `context`, `sync` 优先使用标准库
- 日志统一 `pkg/logutil.InitLogger`（基于 `log/slog` + `lumberjack` 轮转）
- 单实例锁 `github.com/gofrs/flock`（跨平台）
- 无 `make lint`、无 `make test`；直接 `go test ./...`
- 代码格式：`gofmt -s`（gofumpt 已注释）