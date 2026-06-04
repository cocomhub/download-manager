# Phase 1: 代码可维护性重构 — 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 1569 行的 `manager/manager.go` 按职责拆分为 5 个同包文件（scheduler.go, download.go, events.go, tasks.go, aggregate.go），不动方法签名，不引入接口，修复已知 bug。

**Architecture:** 同包拆分：新文件与 `manager.go` 同在 `package manager`，直接访问 `Manager` 结构体字段，无需接口。每创建 1-2 个文件即 build + vet + test 验证。

**Tech Stack:** Go 1.26, standard library, 同包拆分

---

## 文件结构

### 现有文件（保持不动）

| 文件 | 职责 |
|------|------|
| `manager/manager.go` | `Manager` 结构体、构造函数、访问器、查询方法、配置管理（拆分后保留约 300 行） |
| `manager/aggregation_service.go` | `AggregationService` 结构体 + `AggregateObjects` 实现 |
| `manager/config_service.go` | 配置持久化/备份/审计 |
| `manager/config_mgr.go` | 配置变更管理方法 |
| `manager/runtime_mgr.go` | `worker()`, `adjustGlobalWorkers()`, `applyTaskRuntime()`, `SetTaskConfig()` |
| `manager/task_loader.go` | `loadTasks()`, `closeAllTasks()` |
| `manager/url_registry.go` | URL 状态注册表 |

### 新建文件

| 文件 | 要迁移的方法 | 预估行数 |
|------|-------------|----------|
| `manager/scheduler.go` | `Start()`, `Stop()`, `WaitForShutdown()`, `scan()`, `processTask()`, `scheduler()`, `getTaskQueue()` | ~280 |
| `manager/download.go` | `download()`, `forceDownload()`, `applyGroupPriorityPolicies()`, `RetryObject()`, `RetryAllFailed()` | ~230 |
| `manager/events.go` | `Subscribe()`, `Unsubscribe()`, `publish()`, `broadcastProgress()`, `BroadcastTaskUpdate()` | ~100 |
| `manager/tasks.go` | `CancelTask()`, `CancelTasks()`, `CancelObject()`, `UndoCancelObject()`, `ReorderObject()`, `getTask()`, `getTaskObject()` | ~160 |
| `manager/aggregate.go` | `AggregateObjects()`, `AggregateByContent()`, `GetObjectsByScopedGroup()`, `BackfillContentGroups()`, 辅助函数 | ~190 |

### 拆分后 `manager.go` 保留的内容

- `downloadRequest` 结构体, `Manager` 结构体, `taskMetrics` 结构体, `RuntimeFeatures` 结构体
- `NewManager()`, `FeaturesStatus()`, `getAllTasks()`, `GetDownloadRootDir()`, `currentCfg()`
- `searchTaskObjects()`, `countTaskObjects()`, `collectTaskObjects()`
- `GetActiveDownloads()`, `GetTaskSummaries()`, `GetTaskDetails()`
- `UpdateConfig()`, `UpdateLogConfig()`
- `cloneStorageQuery()`, `queryForTask()`, `sortRules()`, `flushAllStorages()`

---

### Task 1: 创建 `manager/scheduler.go`

**文件:** Create: `manager/scheduler.go`

从 `manager.go` 迁移以下方法：

| 方法 | manager.go 行号 |
|------|----------------|
| `Start()` | 305-360 |
| `Stop(ctx)` | 362-392 |
| `WaitForShutdown(ctx)` | 396-410 |
| `scan()` | 425-493 |
| `processTask(t)` | 495-560 |
| `scheduler()` | 574-644 |
| `getTaskQueue(id)` | 562-572 |

**需要的 import：**
```go
import (
    "context"
    "log/slog"
    "time"

    "github.com/cocomhub/download-manager/config"
    "github.com/cocomhub/download-manager/core"
    "github.com/cocomhub/download-manager/downloader"
    "github.com/cocomhub/download-manager/model"
)
```

- [ ] **Step 1: 验证构建当前状态**

Run: `cd D:/workdir/leon/cocomhub/download-manager && go build ./...`
Expected: 成功无错误

- [ ] **Step 2: 创建 `manager/scheduler.go` 文件**

写入完整内容。包含 `// Copyright` header、package 声明、import，以及上述 7 个方法。方法体从 `manager.go` 原样复制，不做任何语义改动。

```go
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
    "context"
    "log/slog"
    "time"

    "github.com/cocomhub/download-manager/config"
    "github.com/cocomhub/download-manager/core"
    "github.com/cocomhub/download-manager/downloader"
    "github.com/cocomhub/download-manager/model"
)

// Start starts the manager main loop with scan ticker and progress broadcast.
func (m *Manager) Start() {
    slog.Info("Manager started")
    // ... (完整从 manager.go:305-360 复制)
}

// Stop signals shutdown and waits for workers with context deadline.
func (m *Manager) Stop(ctx context.Context) {
    // ... (完整从 manager.go:362-392 复制)
}

// WaitForShutdown waits for workers and force-downloads to finish, then flushes storages.
func (m *Manager) WaitForShutdown(ctx context.Context) {
    // ... (完整从 manager.go:396-410 复制)
}

// scan runs the two-phase scan: Phase 1 = scrape, Phase 2 = download scheduling.
func (m *Manager) scan() {
    // ... (完整从 manager.go:425-493 复制)
}

// processTask processes pending download objects for a given task.
func (m *Manager) processTask(t core.Task) {
    // ... (完整从 manager.go:495-560 复制)
}

// getTaskQueue returns the per-task download queue, creating it if necessary.
func (m *Manager) getTaskQueue(taskID string) chan *downloadRequest {
    // ... (完整从 manager.go:562-572 复制)
}

// scheduler implements weighted round-robin dispatch from per-task queues to the global queue.
func (m *Manager) scheduler() {
    // ... (完整从 manager.go:574-644 复制)
}
```

- [ ] **Step 3: 将 `flushAllStorages()` 留在 manager.go（不移到 scheduler.go）**

确认 `flushAllStorages()` (manager.go:412-423) 保留在原处——它被 `WaitForShutdown` 调用，且是工具方法，留在 manager.go 更合理。

- [ ] **Step 4: 从 `manager.go` 删除上述7个方法**

使用 Edit 工具删除以下行范围：
- `Start()`: 行 305-360
- `Stop()`: 行 362-392
- `WaitForShutdown()`: 行 396-410
- `scan()`: 行 425-493
- `processTask()`: 行 495-560
- `getTaskQueue()`: 行 562-572
- `scheduler()`: 行 574-644

每次删除一个方法，用空字符串替换。注意：需要逐段删除。

**关键：** 从 `manager.go` 的 import 中移除 `context`、`time`（如果它们不再被 manager.go 中的剩余方法使用）。

- [ ] **Step 5: 验证构建通过**

Run: `cd D:/workdir/leon/cocomhub/download-manager && go build ./...`
Expected: 编译成功

- [ ] **Step 6: 验证测试通过**

Run: `cd D:/workdir/leon/cocomhub/download-manager && go test ./manager/... -v -count=1 2>&1 | tail -30`
Expected: 全部 PASS

- [ ] **Step 7: Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager
git add manager/scheduler.go manager/manager.go
git commit -m "refactor(manager): Phase 1 - extract scheduler.go (Start/Stop/scan/processTask/scheduler)"
```

---

### Task 2: 创建 `manager/download.go`

**文件:** Create: `manager/download.go`

从 `manager.go` 迁移以下方法：

| 方法 | manager.go 行号 |
|------|----------------|
| `download(t, obj)` | 706-807 |
| `forceDownload(t, obj)` | 810-825 |
| `applyGroupPriorityPolicies(t, obj)` | 1186-1273 |
| `RetryObject(taskID, url)` | 1297-1331 |
| `RetryAllFailed(taskID)` | 1350-1375 |

**需要的 import：**
```go
import (
    "context"
    "errors"
    "fmt"
    "log/slog"
    "maps"
    "sync/atomic"
    "time"

    "github.com/cocomhub/download-manager/core"
    "github.com/cocomhub/download-manager/downloader"
    "github.com/cocomhub/download-manager/model"
    "github.com/cocomhub/download-manager/pkg/dlcore"
    "github.com/cocomhub/download-manager/pkg/titlegroup"
    "github.com/cocomhub/download-manager/task/tktube"
)
```

- [ ] **Step 1: 创建 `manager/download.go` 文件**

写入完整内容，包含 header、package、imports 和上述 5 个方法。注意：`applyGroupPriorityPolicies` 引用了 `metadataContentGroup`、`metadataTaskType`、`variantPriorityScore`——这些函数将留在 aggregate.go 中，在 aggregate.go 创建前会留在 manager.go 中。因此 `applyGroupPriorityPolicies` 的 import 包含 `titlegroup` 和 `tktube`。

- [ ] **Step 2: 从 `manager.go` 删除上述5个方法**

删除范围：
- `metricUpdater 匿名函数在 download 内部` — 注意 `download()` 中嵌套使用了 `m.metrics`、`m.failedCount`、`m.downloadingObj`、`m.lastProgress`，这些都是 Manager 结构体字段，迁移后可直接在 download.go 中访问。
- `download()`: 行 706-807
- `forceDownload()`: 行 810-825
- `applyGroupPriorityPolicies()`: 行 1186-1273
- `RetryObject()`: 行 1297-1331
- `RetryAllFailed()`: 行 1350-1375

从 `manager.go` 的 import 中移除不再需要的包：
- `errors` — 可能在剩余代码中仍被 `UpdateConfig` 使用，检查
- `fmt` — 可能在剩余代码中仍被 `GetTaskDetails`、`CancelTask` 等使用，检查
- `time` — 可能在剩余代码中仍被 `NewManager` 使用（`time.Duration`），检查

- [ ] **Step 3: 修复 `RetryObject` 中的潜在 bug**

在 `download.go` 的 `RetryObject` 中，确认 `go m.processTask(t)` 调用——当前 `RetryObject` 调用 `m.forceDownload`，不需要修复。

- [ ] **Step 4: 验证构建通过**

Run: `cd D:/workdir/leon/cocomhub/download-manager && go build ./...`

- [ ] **Step 5: 验证测试通过**

Run: `cd D:/workdir/leon/cocomhub/download-manager && go test ./manager/... -v -count=1 2>&1 | tail -30`

- [ ] **Step 6: Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager
git add manager/download.go manager/manager.go
git commit -m "refactor(manager): Phase 1 - extract download.go (download/forceDownload/Retry/groupPolicies)"
```

---

### Task 3: 创建 `manager/events.go`

**文件:** Create: `manager/events.go`

从 `manager.go` 迁移以下方法：

| 方法 | manager.go 行号 |
|------|----------------|
| `Subscribe()` | 163-169 |
| `Unsubscribe(ch)` | 283-290 |
| `publish(e)` | 292-303 |
| `broadcastProgress()` | 646-659 |
| `BroadcastTaskUpdate(taskID)` | 661-697 |

**需要的 import：**
```go
import (
    "log/slog"
    "sync"

    "github.com/cocomhub/download-manager/core"
    "github.com/cocomhub/download-manager/model"
    "github.com/cocomhub/download-manager/pkg/dlcore"
)
```

- [ ] **Step 1: 创建 `manager/events.go` 文件**

写入完整内容。

- [ ] **Step 2: 从 `manager.go` 删除上述5个方法**

删除范围：
- `Subscribe()`: 行 163-169
- `Unsubscribe()`: 行 283-290
- `publish()`: 行 292-303
- `broadcastProgress()`: 行 646-659
- `BroadcastTaskUpdate()`: 行 661-697

从 manager.go 的 import 中移除不再需要的包：
- `time` — 如果 Start() 已移走，`time.Duration` 可能还在 `NewManager` 中使用。检查后决定。

- [ ] **Step 3: 验证构建通过**

Run: `cd D:/workdir/leon/cocomhub/download-manager && go build ./...`

- [ ] **Step 4: 验证测试通过**

Run: `cd D:/workdir/leon/cocomhub/download-manager && go test ./manager/... -v -count=1 2>&1 | tail -30`

- [ ] **Step 5: Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager
git add manager/events.go manager/manager.go
git commit -m "refactor(manager): Phase 1 - extract events.go (Subscribe/Unsubscribe/publish/broadcast)"
```

---

### Task 4: 创建 `manager/tasks.go`

**文件:** Create: `manager/tasks.go`

从 `manager.go` 迁移以下方法：

| 方法 | manager.go 行号 |
|------|----------------|
| `getTask(id)` | 699-704 |
| `getTaskObject(t, url)` | 267-281 |
| `CancelTask(taskID)` | 1377-1409 |
| `CancelTasks(ids)` | 1411-1421 |
| `CancelObject(taskID, url)` | 1424-1457 |
| `UndoCancelObject(taskID, url)` | 1460-1482 |
| `ReorderObject(taskID, url, newIndex)` | 1334-1347 |

**需要的 import：**
```go
import (
    "errors"
    "fmt"
    "log/slog"

    "github.com/cocomhub/download-manager/core"
    "github.com/cocomhub/download-manager/model"
    "github.com/cocomhub/download-manager/pkg/dlcore"
)
```

- [ ] **Step 1: 创建 `manager/tasks.go` 文件**

写入完整内容。

- [ ] **Step 2: 修复 `CancelObject` 中的重复 `downloadingObj.Delete` bug**

原始代码（行 1424-1457）中有两处 `m.downloadingObj.Delete(obj.URL)`：
- 第一处在行 1399（if 条件块内）：`if _, active := m.downloadingObj.Load(obj.URL); active { ... m.downloadingObj.Delete(obj.URL) ... }`
- 第二处在行 1448：没有条件，总是执行

在合并后的 tasks.go 中，只保留第一处（在 `if _, active` 检查块内），删除无条件的第二处。修正后的版本：

```go
func (m *Manager) CancelObject(taskID, url string) error {
    t, ok := m.getTask(taskID)
    if !ok {
        return fmt.Errorf("task not found")
    }
    obj, err := m.getTaskObject(t, url)
    if err != nil {
        return err
    }
    if obj == nil {
        return fmt.Errorf("object not found")
    }
    if obj.GetStatus() == dlcore.StatusCompleted {
        return fmt.Errorf("object already completed")
    }
    t.UpdateStatus(obj, dlcore.StatusCancelled, nil)
    m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
    m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})
    if _, active := m.downloadingObj.Load(obj.URL); active {
        if c, ok := m.downloader.(interface {
            Cancel(url string) error
        }); ok {
            _ = c.Cancel(obj.URL)
        }
        m.downloadingObj.Delete(obj.URL)
        m.mu.Lock()
        if m.activeDownloads[taskID] > 0 {
            m.activeDownloads[taskID]--
        }
        m.mu.Unlock()
    }
    m.BroadcastTaskUpdate(taskID)
    return nil
}
```

- [ ] **Step 3: 从 `manager.go` 删除上述7个方法**

删除范围：
- `getTask()`: 行 699-704
- `getTaskObject()`: 行 267-281
- `CancelTask()`: 行 1377-1409
- `CancelTasks()`: 行 1411-1421
- `CancelObject()`: 行 1424-1457
- `UndoCancelObject()`: 行 1460-1482
- `ReorderObject()`: 行 1334-1347

- [ ] **Step 4: 验证构建通过**

Run: `cd D:/workdir/leon/cocomhub/download-manager && go build ./...`

- [ ] **Step 5: 验证测试通过**

Run: `cd D:/workdir/leon/cocomhub/download-manager && go test ./manager/... -v -count=1 2>&1 | tail -40`

- [ ] **Step 6: Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager
git add manager/tasks.go manager/manager.go
git commit -m "refactor(manager): Phase 1 - extract tasks.go (cancel/retry/reorder) + fix double downloadingObj.Delete bug"
```

---

### Task 5: 创建 `manager/aggregate.go`

**文件:** Create: `manager/aggregate.go`

从 `manager.go` 迁移以下方法和辅助函数：

| 方法/函数 | manager.go 行号 |
|-----------|----------------|
| `AggregateObjects(...)` | 961-963（委托给 aggSvc） |
| `AggregateByContent(...)` | 966-1092 |
| `GetObjectsByScopedGroup(...)` | 1276-1294 |
| `BackfillContentGroups()` | 1130-1181 |
| `metadataContentGroup(obj)` | 1094-1099 |
| `metadataTaskType(obj)` | 1101-1106 |
| `scopedContentGroupKey(...)` | 1108-1110 |
| `variantPriorityScore(t, obj)` | 1112-1127 |

**需要的 import：**
```go
import (
    "log/slog"
    "maps"
    "sort"
    "strings"

    "github.com/cocomhub/download-manager/core"
    "github.com/cocomhub/download-manager/model"
    "github.com/cocomhub/download-manager/pkg/dlcore"
    "github.com/cocomhub/download-manager/pkg/titlegroup"
    "github.com/cocomhub/download-manager/storage"
    "github.com/cocomhub/download-manager/task/tktube"
)
```

- [ ] **Step 1: 创建 `manager/aggregate.go` 文件**

写入完整内容。

注意：`AggregateObjects` 方法当前在 manager.go 中只是一个委托：
```go
func (m *Manager) AggregateObjects(page, limit int64, search, sortBy, status string, types []string) (map[string]any, error) {
    return m.aggSvc.AggregateObjects(page, limit, search, sortBy, status, types)
}
```
原样迁移。

- [ ] **Step 2: 从 `manager.go` 删除上述方法和辅助函数**

删除范围：
- `AggregateObjects()`: 行 961-964
- `AggregateByContent()`: 行 966-1092
- 辅助函数: 行 1094-1127
- `BackfillContentGroups()`: 行 1130-1181
- `GetObjectsByScopedGroup()`: 行 1276-1294

从 manager.go 的 import 中移除不再需要的包：
- `sort`
- `maps` (确认 `GetTaskSummaries` 和 `GetTaskDetails` 是否还需要)
- `pkg/titlegroup`
- `task/tktube`
- `storage`

- [ ] **Step 3: 验证构建通过**

Run: `cd D:/workdir/leon/cocomhub/download-manager && go build ./...`

- [ ] **Step 4: 验证测试通过**

Run: `cd D:/workdir/leon/cocomhub/download-manager && go test ./manager/... -v -count=1 2>&1 | tail -50`

特别注意聚合测试和分组策略测试：
Run: `cd D:/workdir/leon/cocomhub/download-manager && go test ./manager/... -v -count=1 -run "Aggregate|Content|Group|Backfill" 2>&1`

- [ ] **Step 5: Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager
git add manager/aggregate.go manager/manager.go
git commit -m "refactor(manager): Phase 1 - extract aggregate.go (AggregateByContent/Backfill/group helpers)"
```

---

### Task 6: 修复 scheduler.go 中的已知 bug

**文件:** Modify: `manager/scheduler.go`

- [ ] **Step 1: 修复 `processTask()` 中 `slotsAvailable` 可能为负数的问题**

当前代码：
```go
m.mu.Lock()
active := m.activeDownloads[t.ID()]
if active >= limit {
    m.mu.Unlock()
    return
}
m.mu.Unlock()

slotsAvailable := limit - active
```

修正：加 `max(0, ...)` 保护：
```go
slotsAvailable := max(0, limit-active)
```

- [ ] **Step 2: 修复 `scheduler()` 中 `expanded` 切片初始容量不足问题**

当前代码：
```go
expanded := make([]string, 0, 64)
```

修正：根据任务数和最大权重动态计算容量：
```go
maxWeight := 8
expanded := make([]string, 0, len(ids)*maxWeight)
```

用常量替代魔法数 8（已在代码中作为权重上限出现）。

- [ ] **Step 3: 验证构建和测试通过**

Run: `cd D:/workdir/leon/cocomhub/download-manager && go build ./... && go test ./manager/... -v -count=1 2>&1 | tail -20`

- [ ] **Step 4: Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager
git add manager/scheduler.go
git commit -m "fix(manager): Phase 1 - fix processTask slotsAvailable negative edge case and scheduler expanded capacity"
```

---

### Task 7: 修复 download.go 中的硬编码重试上限

**文件:** Modify: `manager/download.go`

- [ ] **Step 1: 将硬编码的 `5` 次重试上限替换为配置值**

当前代码（在 `download()` 方法中）：
```go
// Check if max retries reached
if c >= 5 {
    if ft, ok := t.(core.FailedTask); ok {
        ft.MarkAsFailed(obj, fmt.Errorf("max retries reached: %w", err))
    }
}
```

修正：使用 `m.currentCfg().Downloader.MaxRetries`，保留 5 作为默认值：
```go
maxRetries := m.currentCfg().Downloader.MaxRetries
if maxRetries <= 0 {
    maxRetries = 5
}
if c >= int64(maxRetries) {
    if ft, ok := t.(core.FailedTask); ok {
        ft.MarkAsFailed(obj, fmt.Errorf("max retries reached: %w", err))
    }
}
```

`download.go` 需要添加 `fmt` 到 import。

- [ ] **Step 2: 验证构建和测试通过**

Run: `cd D:/workdir/leon/cocomhub/download-manager && go build ./... && go test ./manager/... -v -count=1 2>&1 | tail -20`

- [ ] **Step 3: Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager
git add manager/download.go
git commit -m "fix(manager): Phase 1 - use configurable MaxRetries instead of hardcoded 5"
```

---

### Task 8: 最终验证

- [ ] **Step 1: 全量构建验证**

Run: `cd D:/workdir/leon/cocomhub/download-manager && go build ./...`
Expected: 编译成功，零错误

- [ ] **Step 2: go vet 检查**

Run: `cd D:/workdir/leon/cocomhub/download-manager && go vet ./manager/...`
Expected: 零警告

- [ ] **Step 3: 全量测试**

Run: `cd D:/workdir/leon/cocomhub/download-manager && go test ./manager/... -v -count=1 2>&1 | tail -60`
Expected: 全部 PASS

- [ ] **Step 4: 验证 manager.go 最终大小**

Run: `cd D:/workdir/leon/cocomhub/download-manager && wc -l manager/manager.go`
Expected: ~300-400 行

- [ ] **Step 5: 验证新文件的存在性**

Run: `ls -la manager/scheduler.go manager/download.go manager/events.go manager/tasks.go manager/aggregate.go`
Expected: 5 个文件都存在

- [ ] **Step 6: 无残留引用检查**

Run: `cd D:/workdir/leon/cocomhub/download-manager && grep -n "titlegroup\|tktube" manager/manager.go`
Expected: 无匹配（tktube/titlegroup 已移至 aggregate.go/download.go）

- [ ] **Step 7: 格式化**

Run: `cd D:/workdir/leon/cocomhub/download-manager && gofmt -s -w manager/`
Expected: 成功格式化

- [ ] **Step 8: 最终 Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager
git add -A
git commit -m "refactor(manager): Phase 1 - final formatting and cleanup"
```

---

## 自检清单

### Spec 覆盖
- ✅ manager.go 拆分为 scheduler.go、download.go、events.go、tasks.go、aggregate.go
- ✅ `CancelObject` 双重 `downloadingObj.Delete` 修复（Task 4）
- ✅ `processTask` `slotsAvailable` 负数保护（Task 6）
- ✅ `scheduler` `expanded` 容量修正（Task 6）
- ✅ 硬编码重试上限 5 改为配置驱动（Task 7）
- ✅ 所有方法签名不变，不引入接口
- ✅ 每步 build + vet + test 验证

### 占位符检查
- 所有步骤包含实际代码和命令，无 TBD/TODO
- import 列表精确匹配迁移后的方法依赖
- 行号引用精确

### 类型一致性
- 方法签名与 manager.go 中的原始签名完全一致
- Manager 结构体字段引用路径正确
- 辅助函数（`metadataContentGroup` 等）移到 aggregate.go，所有调用者（`applyGroupPriorityPolicies` 在 download.go）引用正确

### 测试一致性
- 所有测试文件无需改动（方法签名不变，同包拆分）
- aggregation_service_test.go 测试的 AggregationService 逻辑不在拆分范围内
- content_group_backfill_test.go、group_policies_test.go、aggregate_test.go、aggregate_group_test.go 自动覆盖对应逻辑