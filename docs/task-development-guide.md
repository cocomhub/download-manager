# Task 开发指南

本文档说明如何在 download-manager 中开发新的 task 类型，连接新的源站数据。

## 架构概览

download-manager 使用**两层架构**：

- **Manager（通用层）**：调度、worker 池、下载执行、重试、健康检查、事件广播 — 所有 task 类型共享
- **Task（源站层）**：数据爬取、对象解析、下载对象构造 — 每种源站独立实现

```
Manager
  ├── scan() 定时循环
  │   ├── Phase 1: Scrape(ctx) → 爬取新对象，持久化到存储
  │   └── Phase 2: processTask() → GetDownloadObjects() → 调度到下载队列
  │
  ├── resolveWorker 池 (3 workers, 30s 超时)
  │   └── ResolveObject(ctx, obj) → 解析源站页面，填充 Extra["files"]
  │
  └── worker 池 → download() → Downloader.Download()
```

## 核心接口

### core.Task（必须实现）

```go
type Task interface {
    ID() string                          // 唯一标识
    Type() string                        // 任务类型名（注册用）
    Logger() *slog.Logger

    Storage() Storage                    // 存储后端
    SetDownloader(dl Downloader)
    GetDownloadHeaders() map[string]string

    // === 以下三个方法是新 task 开发的核心 ===

    GetDownloadObjects() ([]*DownloadObject, error) // 纯查询，返回 pending 对象
    ResolveObject(ctx context.Context, obj *DownloadObject) error // 解析源站详情
    UpdateStatus(obj *DownloadObject, status string, err error) error

    Concurrency() int
    SetConcurrency(n int)
    RefreshInterval() time.Duration
    SetRefreshInterval(d time.Duration)

    Start() error
    Close() error
}
```

### 可选接口

| 接口 | 用途 | 示例 |
|------|------|------|
| `core.Scraper` | 爬取新对象（分页遍历源站） | hanime, tktube, vikacg |
| `core.SmallObjectProvider` | 下载关联小对象（封面、预览） | tktube |
| `core.FailedTaskMarker` | 永久失败标记 | 所有 task（通过 BaseTask） |

## 方法职责详解

### 1. GetDownloadObjects() — 纯查询

**职责**：从存储中查询待下载对象，返回给 Manager 调度。

**规约**：
- **只做查询**，不做任何解析、爬取、状态修改
- 过滤掉 `StatusCompleted` 和 `StatusCancelled` 的终端对象
- 过滤掉 `IsMarkedFailed` 的永久失败对象
- 不限制返回数量（Manager 的 `processTask` 按 `slotsAvailable` 限制）

**实现模板**：

```go
func (t *Task) GetDownloadObjects() ([]*model.DownloadObject, error) {
    objects := t.LoadPendingFromStorage(64)
    if objects == nil {
        objects = t.SnapshotRuntimeObjects(true)
    }
    pending := make([]*model.DownloadObject, 0)
    for _, o := range objects {
        if t.IsMarkedFailed(o.URL) {
            continue
        }
        if o.GetStatus() == model.StatusCompleted || o.GetStatus() == model.StatusCancelled {
            continue
        }
        pending = append(pending, o)
    }
    return pending, nil
}
```

### 2. ResolveObject(ctx, obj) — 源站解析

**职责**：访问源站页面，提取下载链接，填充 `obj.Extra["files"]`。

**规约**：
- 由 Manager 的异步 resolve 池调用（3 workers，30s 超时）
- 解析成功 → 填充 `Extra["files"]`（`[]map[string]string`，每项含 `url`、`path`、`type`）
- 解析失败 → 返回 error，Manager 自动标记 `StatusFailed`
- 如果对象已有 `Extra["files"]`（从共享注册表恢复），可跳过解析
- 不需要 Resolve 的 task（如 urllist）使用 BaseTask 的默认空实现

**何时被调用**：
1. `processTask` 检测到 `StatusPending` 且 `!hasFiles(obj)` → 入队到 resolve 池
2. `download()` 检测到 resolve 缓存过期 → 重新 resolve
3. `handleCompositeEmptyError` 重试 → 重新 resolve
4. `RetryObject` API → 重新 resolve

### 3. Scrape(ctx) — 爬取新对象

**职责**：遍历源站页面，发现新对象，通过 `ProcessNewURLs` + `PersistTaskObject` 持久化。

**推荐实现**：使用 `PagingScanner` + `SiteAdapter` 模式。

#### SiteAdapter 接口

```go
type SiteAdapter interface {
    BuildPageURL(page int) string
    RunScraper(url string) (string, error)
    ParseTotalPages(html string) int
    ParsePage(html string) (any, error)     // 返回站点特定的 items
    ItemsToURLs(items any) []string
    BuildObject(items any, index int) (*model.DownloadObject, error)
}
```

#### 使用 PagingScanner

```go
func NewTask(cfg *config.Task, opts task.Options) (*Task, error) {
    bt, err := task.NewBaseTask(cfg, opts)
    // ...
    adapter := &myAdapter{t: t}
    scanner := task.NewPagingScanner(bt, adapter)
    bt.SetScanner(scanner)
    return t, nil
}

func (t *Task) Scrape(ctx context.Context) error {
    return t.BaseTask.Scrape(ctx) // 自动委托给 PagingScanner
}
```

## 开发新 task 的 Checklist

1. [ ] 在 `task/<name>/` 下创建包
2. [ ] 定义 `Task` 结构体，嵌入 `*task.BaseTask`
3. [ ] 实现 `Type() string` 返回唯一类型名
4. [ ] 在 `init()` 中调用 `task.Register(typeName, factory)`
5. [ ] 实现 `GetDownloadObjects()` — 纯查询（参考模板）
6. [ ] 实现 `ResolveObject(ctx, obj)` — 解析源站详情
7. [ ] 如果源站有分页列表 → 实现 `SiteAdapter` 接口 + `Scrape()`
8. [ ] 如果需要自定义 HTTP 头 → 实现 `GetDownloadHeaders()`
9. [ ] 在 `main.go` 中 `import _ "path/to/task"` 触发注册
10. [ ] 编写单元测试
11. [ ] 在 `config.yaml` 中添加对应 task 配置

## 状态流转

```
pending → (Scrape 发现) → pending
pending → (无 files) → resolving → (ResolveObject 成功) → pending
pending → (有 files) → 入队 → downloading → completed
                                          → failed → (重试) → pending
                                                   → failed_permanent
                                          → cancelled
```

## 配置示例

```yaml
tasks:
  - id: "my-source-main"
    type: "my_source"             # 与 task.Register 的 typeName 一致
    save_dir: "/downloads/my-source"
    storage:
      type: file
      config:
        path: "/data/my-source.json"
    extra:
      max_concurrent: 3           # 每任务并发限制
      refresh_interval: 3600      # 爬取间隔（秒）
      path_strategy: "first_fixed" # 路径策略
      # 源站特定配置...
      keyword: "热门"
      cookie: "session=xxx"
```

## 参考实现

| Task | 特点 | 文件 |
|------|------|------|
| **urllist** | 最简单的 task，无爬取无解析 | `task/urllist/task.go` |
| **hanime** | PagingScanner + SiteAdapter + Resolve | `task/hanime/task.go` |
| **vikacg** | PagingScanner + Resolve 重爬取 | `task/vikacg/task.go` |
| **tktube** | PagingScanner + Resolve + SmallObjectProvider | `task/tktube/task.go` |