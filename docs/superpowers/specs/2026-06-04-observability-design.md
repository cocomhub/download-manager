# Phase 3: 运维可观测性 设计文档

> 日期: 2026-06-04
> 项目: download-manager

## 综述

在 Phase 1（代码可维护性重构）和 Phase 2（系统性能优化）基础上，增加运维可观测能力。目标是让运维人员和管理员能快速了解系统健康状况、性能指标和失败详情，无需依赖日志搜索。

### 范围

| 维度 | 内容 | 状态 |
|------|------|------|
| A | 健康检查 (`/api/healthz`) | 设计确认 |
| B | 指标仪表盘 (`/api/metrics`) | 设计确认 |
| C | 失败原因明细 (`/api/metrics/failures`) | 设计确认 |

---

## A. 健康检查 (`/api/healthz`)

### 新增文件

- `manager/health.go` — 健康检查数据模型 + Manager 方法
- `manager/heartbeat.go`（或在 health.go 中）— 心跳更新逻辑

### 数据结构

```go
// manager/health.go

type ComponentHealth struct {
	Status     string `json:"status"`      // "ok" | "error" | "stopped"
	Detail     string `json:"detail,omitempty"`
	LastBeat   string `json:"last_heartbeat,omitempty"` // RFC3339
}

type HealthStatus struct {
	Status     string                     `json:"status"` // "ok" | "degraded" | "error"
	Uptime     string                     `json:"uptime"` // 进程启动至今
	Components map[string] ComponentHealth `json:"components"`
}
```

### 检查组件

| 组件 Key | 检查方式 | 状态判定 |
|----------|---------|---------|
| `scheduler` | `m.schedulerEnabled.Load()` + `schedulerHeartbeat` | enabled 且最后心跳 < 5s → ok; enabled 但无心跳或超时 → error; disabled → stopped |
| `workers` | `m.workersEnabled.Load()` + `workerCount` + `workerHeartbeats` | enabled 且至少 1 个 worker 有心跳 → ok; enabled 但 0 心跳 → error; disabled → stopped |
| `eventbus` | `m.subscribers` 长度 | 无异常情况，始终 ok（附带 subscriber 数） |
| `tasks` | 遍历 `m.tasks`，检查每个 task 的 `Storage()` 是否非 nil | 所有 task 存储可用 → ok; 部分不可用 → degraded |

### 心跳更新机制

- `manager.go` — Manager struct 新增字段：
  ```go
  startedAt         time.Time
  schedulerHeartbeat atomic.Value  // time.Time
  workerHeartbeat   atomic.Value  // time.Time（简化：只记录至少一个 worker 活跃）
  ```

- `scheduler()` 主循环中每次 tick/signal 更新 `schedulerHeartbeat`
- `worker()` 主循环中每次处理请求更新 `workerHeartbeat`
- `NewManager` 中设置 `startedAt = time.Now()`

### API 路由

`GET /api/healthz` → `api/server.go` 新增 `healthHandler`

响应示例：
```json
{
  "status": "ok",
  "uptime": "2h30m15s",
  "components": {
    "scheduler": { "status": "ok", "last_heartbeat": "2026-06-04T10:00:05+08:00" },
    "workers":   { "status": "ok", "detail": "5/5 active", "last_heartbeat": "2026-06-04T10:00:03+08:00" },
    "eventbus":  { "status": "ok", "detail": "1 subscriber" },
    "tasks":     { "status": "ok", "detail": "3 tasks loaded" }
  }
}
```

---

## B. 指标仪表盘 (`/api/metrics`)

### 新增文件

- `manager/metrics.go` — 指标聚合 + Manager 方法
- `manager/manager.go` — 扩展 `taskMetrics` + 新增 Manager 字段

### 扩展 taskMetrics

```go
// manager/manager.go

type taskMetrics struct {
	avgLatencyMs atomic.Int64
	failures     atomic.Int64
	completed    atomic.Int64
	// 新增
	retried     atomic.Int64 // 重试次数（含 forceDownload 重试）
	lastActive  atomic.Int64 // 最近活跃的 unix 秒
}
```

### Manager 新增字段

```go
// manager/manager.go — Manager struct
startedAt         time.Time       // NewManager 中设置
totalDownloads    atomic.Int64    // 历史总下载次数（download() 每次完成 +1）
schedulerHeartbeat atomic.Value   // time.Time
workerHeartbeat   atomic.Value   // time.Time
```

### 指标收集方法

`func (m *Manager) CollectMetrics() map[string]any`

从 Manager 现有字段聚合：
- `m.metrics.Range()` → 遍历 taskMetrics
- `m.activeDownloads` → 每任务活跃数
- `m.downloadingObj` → 全局活跃数（`Len()` style）
- `m.taskQueues` → 每任务队列深度（`len(queue)`）
- `m.workerCount` → 全局 worker 总数

### API 路由

`GET /api/metrics` → `api/server.go` 新增 `metricsHandler`

响应示例：
```json
{
  "uptime": "2h30m15s",
  "tasks": {
    "t1": {
      "avg_latency_ms": 1200,
      "completed": 45,
      "failures": 3,
      "retried": 2,
      "last_active": 1747456800,
      "queue_depth": 5,
      "active": 3,
      "concurrency": 5
    }
  },
  "global": {
    "active_downloads": 7,
    "worker_count": 5,
    "scheduler": "running",
    "total_downloads": 128,
    "subscriber_count": 1
  }
}
```

### 数据采集点

| 指标 | 采集时机 | 数据源 |
|------|---------|--------|
| `avg_latency_ms` | download 完成 | 已有（滑动平均，无需修改） |
| `completed` | download 成功 | 已有 |
| `failures` | download 失败 | 已有 |
| `retried` | RetryObject / RetryAllFailed 时 | 新增：在 `retry()` 路径中 `m.metrics` +1 |
| `last_active` | download 完成 / 失败 | 新增：设置 `m.metrics.lastActive` |
| `queue_depth` | metrics 聚合时 | `len(m.getTaskQueue(id))` |
| `active` | metrics 聚合时 | `m.activeDownloads[id]` |
| `total_downloads` | download 完成 | 新增：`m.totalDownloads.Add(1)` |

### 无需去重或额外存储

所有指标都是聚合值，从现有运行时状态实时计算。不保留历史时间序列——这是 dashboard 层的职责。

---

## C. 失败原因明细 (`/api/metrics/failures`)

### 新增数据

Manager struct 新增：
```go
// manager/manager.go
failureRecords  []FailureRecord
failureMu       sync.Mutex
failureWriteIdx int      // 环形缓冲区写入索引
maxFailures     int      // 默认 1000
```

### FailureRecord 结构

```go
type FailureRecord struct {
	TaskID    string `json:"task_id"`
	URL       string `json:"url"`
	Error     string `json:"error"`
	Attempt   int    `json:"attempt"`    // 第几次重试失败
	Timestamp int64  `json:"timestamp"`  // unix 秒
	Permanent bool   `json:"permanent"`  // 是否永久失败（超出重试上限或 NoTry）
}
```

### 记录时机

在 `manager/download.go` 的失败路径中：
1. **每次失败**（`err != nil` 且非 cancel/shutdown）→ 记录一条 `FailureRecord`
2. `Permanent = true` 当：
   - `c >= maxRetries`（超出重试上限）
   - `dlcore.IsNoTry(err)`（不可重试错误）
   - 调用 `ft.MarkAsFailed()` 时

### 环形缓冲区

固定大小环形缓冲区，写入位置 `failureWriteIdx` 循环递增，满了后覆盖最旧记录。

### 查询 API

`GET /api/metrics/failures?task_id=xxx&limit=50`

响应：
```json
{
  "failures": [
    { "task_id": "t1", "url": "...", "error": "connection refused", "attempt": 3, "timestamp": 1747456800, "permanent": false }
  ],
  "total": 128
}
```

限制最大返回条数（`limit` 上限 200）。

---

## 文件清单

### 新增文件

| 文件 | 职责 |
|------|------|
| `manager/health.go` | HealthStatus / ComponentHealth 类型 + `GetHealthStatus()` 方法 |
| `manager/metrics.go` | `FailureRecord` 类型 + `CollectMetrics()` + `GetFailures()` 方法 |

### 修改文件

| 文件 | 修改内容 |
|------|---------|
| `manager/manager.go` | 新增 `startedAt`、`totalDownloads`、`schedulerHeartbeat`、`workerHeartbeat` 字段；扩展 `taskMetrics`（`retried`、`lastActive`）；新增 `failureRecords` 环形缓冲区 |
| `manager/scheduler.go` | scheduler() 主循环更新 `schedulerHeartbeat` |
| `manager/download.go` | download() 失败时记录 FailureRecord；成功时 `totalDownloads.Add(1)`；retry 路径递增 `retried` |
| `manager/runtime_mgr.go` | worker() 主循环更新 `workerHeartbeat` |
| `api/server.go` | 新增 `/api/healthz`、`/api/metrics`、`/api/metrics/failures` 路由和 handler |
| `model/object.go` | 新增 `ErrorRecord` 字段（可选：留到持久化失败原因时） |

---

## 工程规范

- 所有修改遵循 CLAUDE.md 约定
- 每次修改后 `go build ./...` + `go vet ./...` + `go test ./...` 全部通过
- 使用 `gofmt -s` 格式化
- commit 信息格式：`feat(manager): Phase 3 - <具体变更>`

## 变更日志

- 2026-06-04: 初版设计文档