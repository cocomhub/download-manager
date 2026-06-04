# Phase 3: 运维可观测性 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 为 download-manager 增加健康检查、运行时指标、失败原因明细三个可观测维度。

**架构：** 新增 `manager/health.go`（健康检查类型 + Manager 方法）和 `manager/metrics.go`（指标聚合 + 失败记录）；在 Manager struct 中新增心跳/指标/失败记录字段；在 scheduler/worker/download/retry 路径中补充数据采集点；在 API 层暴露三个新端点。

**技术栈：** Go 1.26, atomic.Value, sync.Mutex, gorilla/mux, JSON

---

## 文件清单

| 文件 | 职责 |
|------|------|
| 新建 `manager/health.go` | HealthStatus / ComponentHealth 类型 + `GetHealthStatus()` 方法 |
| 新建 `manager/metrics.go` | `FailureRecord` 类型 + `recordFailure()` / `CollectMetrics()` / `GetFailures()` 方法 |
| 修改 `manager/manager.go` | 新增 Manager 字段（startedAt/totalDownloads/schedulerHeartbeat/workerHeartbeat + failureRecords）；扩展 taskMetrics |
| 修改 `manager/scheduler.go` | scheduler() 主循环更新 `schedulerHeartbeat` |
| 修改 `manager/download.go` | 失败时记录 FailureRecord；成功时 `totalDownloads.Add(1)`；retry 路径递增 `retried` |
| 修改 `manager/runtime_mgr.go` | worker() 更新 `workerHeartbeat` |
| 修改 `api/server.go` | 新增 `/api/healthz`、`/api/metrics`、`/api/metrics/failures` 路由和 handler |

---

### 任务 1：Manager 字段扩展 + 心跳更新

**文件：**
- 修改：`manager/manager.go` — Manager struct、NewManager、taskMetrics
- 修改：`manager/scheduler.go` — scheduler() 中更新 heartbeat
- 修改：`manager/runtime_mgr.go` — worker() 中更新 heartbeat

**目标：** 完成所有数据结构的准备工作，使后续任务能直接使用。

- [ ] **步骤 1（依赖）：确认当前文件状态**

运行 `git log --oneline -1` 确保在 `feature/phase3-observability` 分支上。

- [ ] **步骤 2：扩展 Manager struct**

在 `manager/manager.go` 的 Manager struct 中，在 `forceWg` 字段后追加：

```go
	// Heartbeat / uptime
	startedAt         time.Time     // 进程启动时间, set in NewManager
	totalDownloads    atomic.Int64  // 历史总下载次数
	schedulerHeartbeat atomic.Value // time.Time — 调度器最后心跳
	workerHeartbeat   atomic.Value  // time.Time — worker 最后心跳

	// Failure records (ring buffer)
	failureRecords  []FailureRecord
	failureMu       sync.Mutex
	failureWriteIdx int // 环形缓冲区写入索引
```

- [ ] **步骤 3：扩展 taskMetrics 结构体**

在 `manager/manager.go` 的 `taskMetrics` struct 中追加：

```go
type taskMetrics struct {
	avgLatencyMs atomic.Int64
	failures     atomic.Int64
	completed    atomic.Int64
	// 新增
	retried    atomic.Int64 // 重试次数
	lastActive atomic.Int64 // 最近活跃 unix 秒
}
```

- [ ] **步骤 4：在 NewManager 中初始化心字段**

在 `manager/manager.go` 的 `NewManager()` 函数末尾（`return mgr` 前）追加：

```go
	mgr.startedAt = time.Now()
	mgr.maxFailures = 1000
	mgr.failureRecords = make([]FailureRecord, mgr.maxFailures)
```

- [ ] **步骤 5：在 scheduler() 中添加心跳更新**

在 `manager/scheduler.go` 的 `scheduler()` 函数中，在 `for { select {` 循环的 `case <-fallbackTicker.C:` 和 `case <-m.schedulerSignal:` 中，`drainOnce()` 调用前追加：

```go
	m.schedulerHeartbeat.Store(time.Now())
```

- [ ] **步骤 6：在 worker() 中添加心跳更新**

在 `manager/runtime_mgr.go` 的 `worker()` 函数中，在 `case req, ok := <-m.downloadQueue:` 分支内（`m.download(req.task, req.obj)` 之前或之后）追加：

```go
	m.workerHeartbeat.Store(time.Now())
```

- [ ] **步骤 7：在 download() 成功/失败路径中添加 totalDownloads 和 lastActive**

在 `manager/download.go` 的 `download()` 函数中：
- 成功路径（`t.UpdateStatus(obj, dlcore.StatusCompleted, nil)` 之后）：追加 `m.totalDownloads.Add(1)`
- 设置 lastActive：在成功和失败路径中 `if v, ok := m.metrics.Load(t.ID()); ok { v.(*taskMetrics).lastActive.Store(time.Now().Unix()) }`

- [ ] **步骤 8：在 RetryObject/RetryAllFailed 中递增 retried**

在 `manager/download.go` 的 `RetryObject()` 方法末尾（return nil 前），递增 retried：
```go
	if v, ok := m.metrics.LoadOrStore(taskID, &taskMetrics{}); ok {
		v.(*taskMetrics).retried.Add(1)
	}
```

在 `RetryAllFailed()` 方法的循环中，每处理一个对象递增：
```go
	if v, ok := m.metrics.LoadOrStore(t.ID(), &taskMetrics{}); ok {
		v.(*taskMetrics).retried.Add(1)
	}
```

- [ ] **步骤 9：运行验证**

```bash
go build ./... && go vet ./... && go test ./...
```

- [ ] **步骤 10：Commit**

```bash
git add manager/manager.go manager/scheduler.go manager/runtime_mgr.go manager/download.go
git commit -m "feat(manager): Phase 3 - add heartbeat/uptime/metrics/failure fields to Manager"
```

---

### 任务 2：健康检查 (`/api/healthz`)

**文件：**
- 创建：`manager/health.go`
- 修改：`api/server.go`

- [ ] **步骤 1：创建 health.go 定义类型和 GetHealthStatus 方法**

`manager/health.go`：

```go
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"fmt"
	"strings"
	"time"
)

// ComponentHealth 描述单个组件的健康状态
type ComponentHealth struct {
	Status   string `json:"status"`             // "ok" | "error" | "stopped"
	Detail   string `json:"detail,omitempty"`
	LastBeat string `json:"last_heartbeat,omitempty"` // RFC3339
}

// HealthStatus 描述整体健康状态
type HealthStatus struct {
	Status     string                     `json:"status"` // "ok" | "degraded" | "error"
	Uptime     string                     `json:"uptime"`
	Components map[string]ComponentHealth `json:"components"`
}

// GetHealthStatus 收集各组件健康状态并返回整体评估
func (m *Manager) GetHealthStatus() HealthStatus {
	const heartbeatTimeout = 5 * time.Second
	hs := HealthStatus{
		Components: make(map[string]ComponentHealth),
		Uptime:     time.Since(m.startedAt).Round(time.Second).String(),
	}

	// Scheduler
	{
		c := ComponentHealth{}
		if m.schedulerEnabled.Load() {
			c.Status = "error"
			c.Detail = "no heartbeat"
			if v := m.schedulerHeartbeat.Load(); v != nil {
				if lastBeat, ok := v.(time.Time); ok {
					c.LastBeat = lastBeat.Format(time.RFC3339)
					if time.Since(lastBeat) < heartbeatTimeout {
						c.Status = "ok"
						c.Detail = ""
					} else {
						c.Detail = fmt.Sprintf("last heartbeat %s ago", time.Since(lastBeat).Round(time.Second))
					}
				}
			}
		} else {
			c.Status = "stopped"
		}
		hs.Components["scheduler"] = c
	}

	// Workers
	{
		c := ComponentHealth{}
		if m.workersEnabled.Load() {
			c.Status = "error"
			c.Detail = "no heartbeat"
			if v := m.workerHeartbeat.Load(); v != nil {
				if lastBeat, ok := v.(time.Time); ok {
					c.LastBeat = lastBeat.Format(time.RFC3339)
					if time.Since(lastBeat) < heartbeatTimeout {
						c.Status = "ok"
						c.Detail = fmt.Sprintf("%d workers", m.workerCount)
					} else {
						c.Detail = fmt.Sprintf("last heartbeat %s ago", time.Since(lastBeat).Round(time.Second))
					}
				}
			}
		} else {
			c.Status = "stopped"
		}
		hs.Components["workers"] = c
	}

	// EventBus
	{
		m.eventMu.RLock()
		subCount := len(m.subscribers)
		m.eventMu.RUnlock()
		hs.Components["eventbus"] = ComponentHealth{
			Status: "ok",
			Detail: fmt.Sprintf("%d subscriber(s)", subCount),
		}
	}

	// Tasks
	{
		var loaded, okCount int
		var failedTasks []string
		m.tasks.Range(func(key, value any) bool {
			loaded++
			t := value.(core.Task)
			if t.Storage() != nil {
				okCount++
			} else {
				failedTasks = append(failedTasks, t.ID())
			}
			return true
		})
		c := ComponentHealth{Status: "ok"}
		if loaded == 0 {
			c.Detail = "no tasks loaded"
		} else if okCount < loaded {
			c.Status = "degraded"
			c.Detail = fmt.Sprintf("%d/%d tasks have storage (%s)", okCount, loaded, strings.Join(failedTasks, ", "))
		} else {
			c.Detail = fmt.Sprintf("%d tasks loaded", loaded)
		}
		hs.Components["tasks"] = c
	}

	// Overall status
	overall := "ok"
	for _, c := range hs.Components {
		if c.Status == "error" {
			overall = "error"
			break
		}
		if c.Status == "degraded" {
			overall = "degraded"
		}
	}
	hs.Status = overall
	return hs
}
```

- [ ] **步骤 2：在 api/server.go 中添加 healthHandler 和路由**

追加 import 中的 `"time"`（如果还没有）：

在 `Router()` 方法的 API Routes 区域追加路由：

```go
	r.HandleFunc("/api/healthz", s.healthHandler).Methods("GET")
```

在 `aggregateObjects` handler 后追加 handler 方法：

```go
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	status := s.mgr.GetHealthStatus()
	if status.Status == "error" {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else if status.Status == "degraded" {
		w.WriteHeader(http.StatusOK) // Still serve 200 for degraded, caller can check body
	}
	json.NewEncoder(w).Encode(status)
}
```

- [ ] **步骤 3：运行验证**

```bash
go build ./... && go vet ./... && go test ./...
```

- [ ] **步骤 4：Commit**

```bash
git add manager/health.go api/server.go
git commit -m "feat(manager): Phase 3 - add /api/healthz endpoint with component health checks"
```

---

### 任务 3：指标仪表盘 (`/api/metrics`)

**文件：**
- 创建：`manager/metrics.go`
- 修改：`api/server.go`

- [ ] **步骤 1：创建 metrics.go 定义类型和 CollectMetrics 方法**

`manager/metrics.go`：

```go
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"sort"
	"sync"
	"time"

	"github.com/cocomhub/download-manager/model"
)

// FailureRecord 描述单次下载失败记录
type FailureRecord struct {
	TaskID    string `json:"task_id"`
	URL       string `json:"url"`
	Error     string `json:"error"`
	Attempt   int    `json:"attempt"`
	Timestamp int64  `json:"timestamp"`
	Permanent bool   `json:"permanent"`
}

// CollectMetrics 收集当前所有任务和全局的运行时指标
func (m *Manager) CollectMetrics() map[string]any {
	taskMetricsMap := make(map[string]any)

	m.metrics.Range(func(key, value any) bool {
		id := key.(string)
		mt := value.(*taskMetrics)

		// 获取活跃下载数
		m.mu.Lock()
		active := m.activeDownloads[id]
		m.mu.Unlock()

		// 获取队列深度
		q := m.getTaskQueue(id)

		// 获取并发度
		concurrency := 2 // default
		if t, ok := m.getTask(id); ok {
			concurrency = t.Concurrency()
		}

		taskMetricsMap[id] = map[string]any{
			"avg_latency_ms": mt.avgLatencyMs.Load(),
			"completed":      mt.completed.Load(),
			"failures":       mt.failures.Load(),
			"retried":        mt.retried.Load(),
			"last_active":    mt.lastActive.Load(),
			"queue_depth":    len(q),
			"active":         active,
			"concurrency":    concurrency,
		}
		return true
	})

	// Global metrics
	activeDownloads := 0
	m.downloadingObj.Range(func(_, _ any) bool {
		activeDownloads++
		return true
	})

	schedulerStatus := "stopped"
	if m.schedulerEnabled.Load() {
		schedulerStatus = "running"
	}

	m.eventMu.RLock()
	subCount := len(m.subscribers)
	m.eventMu.RUnlock()

	global := map[string]any{
		"active_downloads":  activeDownloads,
		"worker_count":      m.workerCount,
		"scheduler":         schedulerStatus,
		"total_downloads":   m.totalDownloads.Load(),
		"subscriber_count":  subCount,
	}

	return map[string]any{
		"uptime": time.Since(m.startedAt).Round(time.Second).String(),
		"tasks":  taskMetricsMap,
		"global": global,
	}
}

// recordFailure 记录一条失败记录到环形缓冲区
func (m *Manager) recordFailure(taskID, url, errStr string, attempt int, permanent bool) {
	m.failureMu.Lock()
	defer m.failureMu.Unlock()

	idx := m.failureWriteIdx % m.maxFailures
	m.failureRecords[idx] = FailureRecord{
		TaskID:    taskID,
		URL:       url,
		Error:     errStr,
		Attempt:   attempt,
		Timestamp: time.Now().Unix(),
		Permanent: permanent,
	}
	m.failureWriteIdx++
}

// GetFailures 查询失败记录列表
func (m *Manager) GetFailures(taskID string, limit int) map[string]any {
	if limit <= 0 || limit > 200 {
		limit = 200
	}

	m.failureMu.Lock()
	total := m.failureWriteIdx
	// 从最新往旧遍历
	count := m.failureWriteIdx
	if count > m.maxFailures {
		count = m.maxFailures
	}
	records := make([]FailureRecord, 0, count)
	for i := 0; i < count; i++ {
		idx := (m.failureWriteIdx - 1 - i + m.maxFailures) % m.maxFailures
		r := m.failureRecords[idx]
		if r.TaskID == "" {
			continue
		}
		if taskID != "" && r.TaskID != taskID {
			continue
		}
		records = append(records, r)
		if len(records) >= limit {
			break
		}
	}
	m.failureMu.Unlock()

	if records == nil {
		records = make([]FailureRecord, 0)
	}
	return map[string]any{
		"failures": records,
		"total":    total,
	}
}
```

- [ ] **步骤 2：在 api/server.go 中添加 metricsHandler 和 failuresHandler 及路由**

在 Router() 中追加路由：

```go
	r.HandleFunc("/api/metrics", s.metricsHandler).Methods("GET")
	r.HandleFunc("/api/metrics/failures", s.failuresHandler).Methods("GET")
```

在 healthHandler 后追加 handler 方法：

```go
func (s *Server) metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	metrics := s.mgr.CollectMetrics()
	json.NewEncoder(w).Encode(metrics)
}

func (s *Server) failuresHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	taskID := r.URL.Query().Get("task_id")
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}
	result := s.mgr.GetFailures(taskID, limit)
	json.NewEncoder(w).Encode(result)
}
```

注意：`strconv` 已在 `api/server.go` 的 import 中。

- [ ] **步骤 3：在 download.go 中连接 recordFailure 调用**

在 `manager/download.go` 的 `download()` 函数中，在失败路径（`err != nil` 且非 cancel/shutdown）中调用 `recordFailure`：

在 `slog.Error("Download failed"...)` 行之后，在 `if dlcore.IsNoTry(err)` 之前插入：

```go
		recordAttempt := 0
		if v, loaded := m.failedCount.Load(obj.URL); loaded {
			recordAttempt = int(v.(*atomic.Int64).Load())
		}
		isPermanent := dlcore.IsNoTry(err) || (int64(maxRetries) > 0 && int64(recordAttempt) >= int64(maxRetries))
		m.recordFailure(t.ID(), obj.URL, err.Error(), recordAttempt+1, isPermanent)
```

注意：需要 import `"sync/atomic"`（已在 `manager/download.go` 的 import 中）。

- [ ] **步骤 4：运行验证**

```bash
go build ./... && go vet ./... && go test ./...
```

- [ ] **步骤 5：Commit**

```bash
git add manager/metrics.go api/server.go manager/download.go
git commit -m "feat(manager): Phase 3 - add /api/metrics and /api/metrics/failures endpoints"
```

---

## 验证方案

每项任务独立验证：
1. `go build ./...` — 编译通过
2. `go vet ./...` — 无静态分析警告
3. `go test ./...` — 全部测试通过

手动验证（可选）：
```bash
# 启动后检查 healthz
curl -s http://localhost:8080/api/healthz | jq .
# 检查 metrics
curl -s http://localhost:8080/api/metrics | jq .
# 检查 failures
curl -s http://localhost:8080/api/metrics/failures | jq .
```

## 变更日志

- 2026-06-04: 初版计划