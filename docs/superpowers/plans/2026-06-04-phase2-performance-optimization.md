# Phase 2: 系统性能优化 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 优化 download-manager 的核心调度/下载路径性能，减少延迟、降低锁竞争、提升吞吐量。

**架构：** 在 Phase 1 拆清代码边界后，针对 4 个具体瓶颈点做精确优化：scheduler 50ms tick → 事件驱动唤醒、per-task 队列容量可配置、进度广播批量聚合、下载队列缓冲区放大。每项修改独立验证，确保不影响正确性。

**技术栈：** Go 1.26, sync.Map, channel, atomic, slog

---

## 代码全景图（修改文件与职责）

| 文件 | 当前职责 | Phase 2 修改 |
|------|---------|-------------|
| `config/config.go` | 配置结构体 + ValidateAndClamp | 无需修改（队列缓冲改为硬编码，不引入新配置项） |
| `manager/manager.go` | Manager 结构体 + 构造函数 | 新增 `schedulerSignal` 字段；放大 `downloadQueue` 缓冲区 |
| `manager/scheduler.go` | 扫描 + 调度 + processTask | 事件驱动信号替代 50ms tick；队列容量动态计算 |
| `manager/events.go` | 事件总线 + 进度广播 | `broadcastProgress` 批量聚合变更 |
| `manager/download.go` | 下载执行 | 下载完成后通知调度器（非阻塞 signal） |

---

### 任务 1：调度器事件驱动信号（替代 50ms tick）

**文件：**
- 修改：`manager/manager.go` — 新增 `schedulerSignal` 字段
- 修改：`manager/scheduler.go` — 改造 scheduler() 与 processTask()
- 修改：`manager/download.go` — download() 完成后通知调度器

**动机：** 当前 scheduler 每 50ms 轮询一次 per-task 队列。一个对象从 processTask 入队到 scheduler 取出，最多等待 50ms。对于高频任务（如 tktube 变体解析），这引入不必要的延迟。改为信号触发：processTask 入队后立即通知 scheduler，scheduler 醒来执行一轮 drain。保留一个慢速 fallback ticker（500ms）防止信号丢失。

- [ ] **步骤 1：在 Manager 结构体中添加 `schedulerSignal` 字段**

```go
// manager/manager.go — Manager struct, 在 schedulerStop 后追加
	schedulerStop chan struct{}
	schedulerSignal chan struct{} // non-buffered signal: enqueue → wake scheduler
```

- [ ] **步骤 2：在 NewManager 中初始化 `schedulerSignal`**

```go
// manager/manager.go — NewManager, 在 mgr 初始化块中添加
		schedulerSignal: make(chan struct{}),
```

`select` 中与 `m.stopChan` / `progressTicker.C` 同级。

- [ ] **步骤 3：在 scheduler() 中将 50ms drain tick 改为信号驱动 + 500ms fallback**

```go
// manager/scheduler.go — scheduler() 函数

func (m *Manager) scheduler() {
	const maxSchedulerWeight = 8
	// 保留 ticker 作为 fallback（防止信号丢失），降低频率到 500ms
	fallbackTicker := time.NewTicker(500 * time.Millisecond)
	defer fallbackTicker.Stop()
	weights := make(map[string]int)
	lastUpdate := time.Now()

	// drainOnce 执行一轮 per-task → global 队列搬运
	drainOnce := func() {
		ids := make([]string, 0, 64)
		m.tasks.Range(func(key, value any) bool {
			ids = append(ids, key.(string))
			return true
		})
		expanded := make([]string, 0, len(ids)*maxSchedulerWeight)
		for _, id := range ids {
			w := weights[id]
			if w <= 0 {
				w = 1
			}
			for i := 0; i < w; i++ {
				expanded = append(expanded, id)
			}
		}
		for _, id := range expanded {
			q := m.getTaskQueue(id)
			select {
			case req := <-q:
				select {
				case m.downloadQueue <- req:
				default:
					select {
					case q <- req:
					default:
					}
					return // global queue full
				}
			default:
			}
		}
	}

	for {
		// 权重更新：每 2 秒一次（保留不变）
		select {
		case <-m.schedulerStop:
			return
		case <-fallbackTicker.C:
			if time.Since(lastUpdate) > 2*time.Second {
				weights = make(map[string]int)
				m.tasks.Range(func(key, value any) bool {
					id := key.(string)
					w := 1
					w += max(0, len(m.getTaskQueue(id))/8)
					if v, ok := m.metrics.Load(id); ok {
						mt := v.(*taskMetrics)
						if mt.avgLatencyMs.Load() > 5000 {
							w -= 1
						}
						if mt.failures.Load() > 0 {
							w -= int(min(mt.failures.Load(), int64(2)))
						}
						if w < 1 {
							w = 1
						}
					}
					w = min(w, maxSchedulerWeight)
					weights[id] = w
					return true
				})
				lastUpdate = time.Now()
			}
			drainOnce()
		case <-m.schedulerSignal:
			drainOnce()
		}
	}
}
```

注意：需要将 `select` 内的 `case <-ticker.C:` 替换为上述两个 case。`drainOnce` 逻辑是从原 `case <-ticker.C:` 中提取的循环体。

> **完整替换 scheduler() 函数**（见下方代码）

```go
func (m *Manager) scheduler() {
	const maxSchedulerWeight = 8
	fallbackTicker := time.NewTicker(500 * time.Millisecond)
	defer fallbackTicker.Stop()
	weights := make(map[string]int)
	lastUpdate := time.Now()

	drainOnce := func() {
		ids := make([]string, 0, 64)
		m.tasks.Range(func(key, value any) bool {
			ids = append(ids, key.(string))
			return true
		})
		expanded := make([]string, 0, len(ids)*maxSchedulerWeight)
		for _, id := range ids {
			w := weights[id]
			if w <= 0 {
				w = 1
			}
			for i := 0; i < w; i++ {
				expanded = append(expanded, id)
			}
		}
		for _, id := range expanded {
			q := m.getTaskQueue(id)
			select {
			case req := <-q:
				select {
				case m.downloadQueue <- req:
				default:
					select {
					case q <- req:
					default:
					}
					return
				}
			default:
			}
		}
	}

	for {
		select {
		case <-m.schedulerStop:
			return
		case <-fallbackTicker.C:
			if time.Since(lastUpdate) > 2*time.Second {
				weights = make(map[string]int)
				m.tasks.Range(func(key, value any) bool {
					id := key.(string)
					w := 1
					w += max(0, len(m.getTaskQueue(id))/8)
					if v, ok := m.metrics.Load(id); ok {
						mt := v.(*taskMetrics)
						if mt.avgLatencyMs.Load() > 5000 {
							w -= 1
						}
						if mt.failures.Load() > 0 {
							w -= int(min(mt.failures.Load(), int64(2)))
						}
						if w < 1 {
							w = 1
						}
					}
					w = min(w, maxSchedulerWeight)
					weights[id] = w
					return true
				})
				lastUpdate = time.Now()
			}
			drainOnce()
		case <-m.schedulerSignal:
			drainOnce()
		}
	}
}
```

- [ ] **步骤 4：在 processTask 入队成功后发送信号**

```go
// manager/scheduler.go — processTask, 在 q <- &downloadRequest{...} 成功后追加

			m.mu.Lock()
			m.activeDownloads[t.ID()]++
			active++
			m.mu.Unlock()
			count++

			// 通知调度器：有新的待处理对象
			select {
			case m.schedulerSignal <- struct{}{}:
			default:
			}
```

- [ ] **步骤 5：在 download() 完成后发送信号（处理扫描周期之外入队的空队列场景）**

```go
// manager/download.go — download() 的 defer 块末尾追加

		// 通知调度器：可能有新槽位可用
		select {
		case m.schedulerSignal <- struct{}{}:
		default:
		}
```

- [ ] **步骤 6：在 RetryAllFailed 和 UndoCancelObject 的 processTask 调用后添加信号**

`RetryAllFailed` 末尾（`tasks.go` 的 `RetryAllFailed` — 在 `go m.processTask(t)` 后）：

```go
		select {
		case m.schedulerSignal <- struct{}{}:
		default:
		}
```

`UndoCancelObject` 末尾（`tasks.go` — 在 `go m.processTask(t)` 后）：

```go
	select {
	case m.schedulerSignal <- struct{}{}:
	default:
	}
```

- [ ] **步骤 7：在有信号的 case 后更新 `schedulerStop` 重新创建逻辑**

`UpdateConfig` 中当 scheduler 重启时需要保留新字段。`schedulerSignal` 是固定 channel 不需要重建，所以 `schedulerStop` 的重建逻辑无需修改。确认 `schedulerSignal` channel 的初始化只在 `NewManager` 中做一次即可。

- [ ] **步骤 8：运行验证**

```bash
go build ./... && go vet ./... && go test -v -run . ./manager/...
```

- [ ] **步骤 9：Commit**

```bash
git add manager/manager.go manager/scheduler.go manager/download.go manager/tasks.go
git commit -m "perf(manager): Phase 2 - replace scheduler 50ms tick with event-driven signal + 500ms fallback"
```

---

### 任务 2：Per-task 队列容量动态计算（替代硬编码 32）

**文件：**
- 修改：`manager/scheduler.go` — `getTaskQueue()` 方法

**动机：** `getTaskQueue` 硬编码 channel buffer 为 32。对于高并发任务（如 `max_concurrent=10`），32 的缓冲可能在调度器 drain 之前填满，导致对象被丢弃。改为根据任务并发度动态计算：`max(concurrency * 8, 32)`，上限 256。

- [ ] **步骤 1：修改 `getTaskQueue` 使用动态容量**

```go
// manager/scheduler.go

func (m *Manager) getTaskQueue(taskID string) chan *downloadRequest {
	if v, ok := m.taskQueues.Load(taskID); ok {
		return v.(chan *downloadRequest)
	}
	// 动态容量：根据任务并发度计算，保证充分缓冲
	cap := 64 // default
	if t, ok := m.getTask(taskID); ok {
		concurrency := t.Concurrency()
		if concurrency > 0 {
			cap = max(concurrency*8, 32)
		}
	}
	cap = min(cap, 256)
	q := make(chan *downloadRequest, cap)
	if v, loaded := m.taskQueues.LoadOrStore(taskID, q); loaded {
		return v.(chan *downloadRequest)
	}
	return q
}
```

- [ ] **步骤 2：运行验证**

```bash
go build ./... && go vet ./... && go test -v -run . ./manager/...
```

- [ ] **步骤 3：Commit**

```bash
git add manager/scheduler.go
git commit -m "perf(manager): Phase 2 - dynamic per-task queue capacity based on concurrency"
```

---

### 任务 3：进度广播批量聚合

**文件：**
- 修改：`manager/events.go` — `broadcastProgress()` 方法

**动机：** `broadcastProgress()` 每秒调用一次，对每个正在下载的对象独立调用 `publish()`。每个 `publish()` 触发一次 RLock + 逐个 subscriber channel 发送。当有 N 个活跃下载时，每秒产生 N 次独立事件发送。SSE 端每次收到事件都做 JSON 解析和 DOM 更新。改为聚合单次事件包含所有进度变更。

- [ ] **步骤 1：在 `model/` 中确认 DownloadObject 的进度结构是否适合批量发送**

查看 `model.DownloadObject` 的 `GetProgress()` 方法是否返回 int。确认后设计批量事件格式。

- [ ] **步骤 2：创建聚合进度事件结构**

```go
// manager/events.go — 在 import 块后添加类型

// ProgressBatch 包含一次广播周期内所有对象的进度变更
type ProgressBatch struct {
	Updates []ProgressItem `json:"updates"`
}

type ProgressItem struct {
	TaskID   string `json:"task_id"`
	URL      string `json:"url"`
	Progress int    `json:"progress"`
	Status   string `json:"status"`
	Title    string `json:"title,omitempty"`
}
```

- [ ] **步骤 3：修改 `broadcastProgress` 批量发送**

```go
// manager/events.go

func (m *Manager) broadcastProgress() {
	batch := &ProgressBatch{
		Updates: make([]ProgressItem, 0, 64),
	}
	m.downloadingObj.Range(func(key, value any) bool {
		obj := value.(*model.DownloadObject)

		// Check if progress has changed
		last, loaded := m.lastProgress.LoadOrStore(obj.URL, -1)
		if !loaded || last.(int) != obj.GetProgress() {
			item := ProgressItem{
				TaskID:   obj.TaskID,
				URL:      obj.URL,
				Progress: obj.GetProgress(),
				Status:   obj.GetStatus(),
			}
			if obj.Metadata != nil {
				item.Title = obj.Metadata["title"]
			}
			batch.Updates = append(batch.Updates, item)
			m.lastProgress.Store(obj.URL, obj.GetProgress())
		}
		return true
	})
	if len(batch.Updates) > 0 {
		m.publish(core.Event{Type: core.EventProgressBatch, Payload: batch})
	}
}
```

- [ ] **步骤 4：确认 `core.EventType` 中是否有 `EventProgressBatch` 常量，若没有则追加**

```go
// core/interfaces.go — 在 EventType 常量区追加

	EventProgressBatch EventType = "progress_batch"
```

- [ ] **步骤 5：运行验证**

```bash
go build ./... && go vet ./... && go test -v -run . ./manager/... ./core/...
```

- [ ] **步骤 6：Commit**

```bash
git add manager/events.go core/interfaces.go
git commit -m "perf(manager): Phase 2 - batch progress broadcasts into single event per cycle"
```

---

### 任务 4：下载队列缓冲区放大

**文件：**
- 修改：`manager/manager.go` — NewManager 中的 `downloadQueue` 初始容量

**动机：** 当前 `downloadQueue` 的 buffer 为 `max(globalLimit*2, 10)`。以默认 globalLimit=5 为例，队列只能容纳 10 个请求。scheduler 一次 drain 可能填满队列然后立即返回。放大为 `max(globalLimit*8, 64)`，减少 scheduler 因队列满而退出的频率。

- [ ] **步骤 1：修改 NewManager 中的 downloadQueue 容量**

```go
// manager/manager.go — NewManager, downloadQueue 初始化
	downloadQueue: make(chan *downloadRequest, max(globalLimit*8, 64)), // Buffer size
```

- [ ] **步骤 2：运行验证**

```bash
go build ./... && go vet ./... && go test -v -run . ./manager/...
```

- [ ] **步骤 3：Commit**

```bash
git add manager/manager.go
git commit -m "perf(manager): Phase 2 - increase downloadQueue buffer from max(limit*2,10) to max(limit*8,64)"
```

---

## 验证方案

每项任务独立验证：
1. `go build ./...` — 编译通过
2. `go vet ./...` — 无静态分析警告
3. `go test -v -run . ./manager/... ./core/...` — 全部测试通过（含已有测试）

完整集成验证：
```bash
cd download-manager
go build ./... && go vet ./... && go test ./...
```

## 变更日志

- 2026-06-04: 初版计划