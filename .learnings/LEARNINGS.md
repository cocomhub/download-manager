
## [LRN-20260616-001] best_practice — Config 深拷贝必须用 Clone() 而非浅拷贝

**Logged**: 2026-06-16T06:00:00Z
**Priority**: high
**Status**: promoted
**Area**: backend

### Summary
共享 `*config.Config` 指针直接修改 map/slice 字段会导致 data race。必须使用 `Clone()` 深拷贝，且不要用 `make+copy` 覆盖 `Clone()` 的 Tasks 结果。

### Details
原代码在 `api/server.go` 中：
```go
cc := cur.Clone()
cc.Tasks = make([]config.Task, len(cur.Tasks))
copy(cc.Tasks, cur.Tasks)  // 用 cur 的浅拷贝覆盖了 Clone() 的深拷贝！
```
这导致 `cc.Tasks[i].Extra` 和 `.Storage.Config` map 仍指向原始数据，失去深拷贝意义。

### Suggested Action
始终使用 `cfg.Clone()` 并直接使用返回的 Tasks 字段。`Clone()` 已正确处理所有 map/slice 的深拷贝（Tasks、Contexts、Proxies、DomainLimits、FFmpeg.ExtraArgs、Mongo）。

### Promoted
- CLAUDE.md (Config 深拷贝陷阱)

### Metadata
- Source: code_review
- Related Files: config/config.go, api/server.go
- Tags: data_race, config, cloning

---

## [LRN-20260616-002] correction — initializedCh 不能在 defer 中 close（无限循环）

**Logged**: 2026-06-16T06:30:00Z
**Priority**: high
**Status**: promoted
**Area**: backend

### Summary
`Manager.Start()` 中的 `close(m.initializedCh)` 如果在 `for{}` 循环前用 `defer`，将永远不会执行。必须放在循环前直接 close。

### Details
`Start()` 末尾是 `for { select { case ... } }` 无限循环。defer 在该函数退出时执行，但无限循环永不退出。所以需要直接调用 `close(m.initializedCh)` 而非 defer。

### Resolution
- **Resolved**: 2026-06-16T06:30:00Z
- **Notes**: 位置在 scheduler.go:58

### Promoted
- CLAUDE.md (Manager.Start 无限循环中的同步)

### Metadata
- Related Files: manager/scheduler.go
- Tags: goroutine, synchronization, test

---

## [LRN-20260616-003] best_practice — golangci-lint v7 配置格式变化

**Logged**: 2026-06-16T07:00:00Z
**Priority**: medium
**Status**: promoted
**Area**: infra

### Summary
golangci-lint v7 (version 2.x.x) 使用新的配置格式：`version: "2"` + `linters/settings/exclusions/rules` + `linters/exclusions/presets`。旧的 `issues/exclude-rules` 不再适用。

### Details
旧格式（v1）：
```yaml
issues:
  exclude-rules:
    - path: _test\.go
      linters: [errcheck]
```

新格式（v2）：
```yaml
linters:
  exclusions:
    rules:
      - path: _test\.go
        linters: [errcheck]
    presets:
      - comments
      - common-false-positives
```

### Suggested Action
使用 `golangci-lint config verify` 验证配置。使用 `//lint:ignore` 注释对特定行抑制 lint。

### Metadata
- Source: error
- Related Files: .golangci.yml
- Tags: golangci-lint, config, migration

---

## [LRN-20260616-004] insight — sync.Map 类型断言必须用 ok 模式

**Logged**: 2026-06-16T07:30:00Z
**Priority**: high
**Status**: promoted
**Area**: backend

### Summary
`sync.Map` 的 `LoadOrStore`/`Load` 返回 `(any, bool)`，对 value 做类型断言时必须使用 ok 模式，直接断言会在值被覆盖时 panic。

### Details
```go
// 错误
counter := v.(*atomic.Int64)

// 正确
v, ok := m.failedCount.LoadOrStore(k, new(atomic.Int64))
counter, ok := v.(*atomic.Int64)
```

### Suggested Action
代码审查时重点检查 `sync.Map` 的类型断言模式。

### Metadata
- Source: code_review
- Related Files: manager/manager.go
- Tags: sync.Map, type_assertion, concurrency

---

## [LRN-20260616-005] insight — ValidateAndClamp 修改 config 的 map 字段引发 data race

**Logged**: 2026-06-16T08:00:00Z
**Priority**: high
**Status**: promoted
**Area**: backend

### Summary
`ValidateAndClamp()` 直接修改传入的 `*Config` 的 `Extra` map（`t.Extra["refresh_interval"] = ...`），如果该 Config 被多个 goroutine 共享，就产生 data race。

### Details
修复方案：在调用 `ValidateAndClamp()` 前先对 Config 做 `Clone()` 深拷贝，或在调用方确保独占访问。`UpdateLogConfig()` 和 `UpdateConfig()` 中已改为先 Clone 再 ValidateAndClamp。

### Suggested Action
任何调用 `ValidateAndClamp()` 的路径都必须确保 config 是独占的（通过 Clone）或外部有锁保护。

### Metadata
- Source: error
- Tags: data_race, config, validation

---

## [LRN-20260616-006] best_practice — `Manager.Start()` 与测试之间的同步信号

**Logged**: 2026-06-16T08:30:00Z
**Priority**: high
**Status**: promoted
**Area**: tests

### Summary
Manager.Start() 异步启动 goroutine，测试中用 `time.Sleep` 等待初始化完成不稳定且浪费。应使用 channel 信号：

```go
// Manager 中添加
initializedCh chan struct{}

// Start() 末尾
close(m.initializedCh)

// 测试中
<-mgr.Initialized()
```

### Details
`startAPIManager` 在 `go mgr.Start()` 后立即 `<-mgr.Initialized()` 等待，确保 `loadTasks()` 等初始化完成后再执行测试操作。移除了 `time.Sleep(200ms)` 的需要。

### Metadata
- Source: code_review
- Tags: testing, synchronization, goroutine

---

## [LRN-20260616-007] best_practice — 架构解耦：上层不应依赖具体任务包

**Logged**: 2026-06-16T09:00:00Z
**Priority**: medium
**Status**: promoted
**Area**: backend

### Summary
`manager/aggregate.go` 直接 import `task/tktube` 导致编排层依赖具体实现。解决方案：在 `core/` 包中定义 `TaskType` 常量字符串。

### Details
```go
// core/tasktype.go
const (
    TaskTypeTktube  = "tktube"
    TaskTypeHanime  = "hanime"
    TaskTypeVikacg  = "vikacg"
    TaskTypeURLList = "url_list"
)
```
所有引用 `tktube.TaskType` 的地方改为 `core.TaskTypeTktube`。对比现有 `task/factory.go` 中的注册 key 保持一致。

### Metadata
- Source: architecture_review
- Related Files: core/tasktype.go, manager/aggregate.go
- Tags: architecture, decoupling, dependency

---

## [LRN-20260616-008] best_practice — go:embed 内联常量提取

**Logged**: 2026-06-16T09:30:00Z
**Priority**: medium
**Status**: resolved
**Area**: backend

### Summary
大型内联 JS 字符串（116 行 `playerUtilJS`）应使用 `//go:embed` 提取到独立文件，减少 Go 源文件体积，支持语法高亮和编辑器支持。

### Details
```go
// task/tktube/player_util_embed.go
package tktube
import _ "embed"
//go:embed player_util.js
var PlayerUtilJS string
```

注意：`//go:embed` 指令必须紧贴变量声明上方，不能有空行。embed 的变量必须是包级别（不能是局部变量）。

### Metadata
- Related Files: task/tktube/player_util_embed.go, task/tktube/task.go
- Tags: go:embed, refactoring

---

## [LRN-20260616-009] best_practice — pre-commit hook 安装方式

**Logged**: 2026-06-16T10:00:00Z
**Priority**: medium
**Status**: promoted
**Area**: infra

### Summary
使用 `git config core.hooksPath .githooks` 而非直接修改 `.git/hooks/`。Makefile 提供 `install-hooks` 目标。

### Details
```makefile
install-hooks:
	git config core.hooksPath .githooks
```

Hook 链：`go fix` → `gofmt -s` → `addlicense` → `go build` → `go vet`

### Metadata
- Tags: git, hooks, automation

---

## [LRN-20260616-010] correction — `scripts/pre-commit.sh` vs `.githooks/pre-commit` 冲突

**Logged**: 2026-06-16T10:30:00Z
**Priority**: low
**Status**: resolved
**Area**: config

### Summary
同时存在 `scripts/pre-commit.sh`（手动）和 `.githooks/pre-commit`（自动 hook），内容有分歧，造成维护负担。删除前者。

### Metadata
- Tags: cleanup, dead_code

---

## [LRN-20260616-011] insight — 内容组优先级选择器依赖 core.TaskType

**Logged**: 2026-06-16T11:00:00Z
**Priority**: low
**Status**: resolved
**Area**: backend

### Summary
`variantPriorityScore()` 通过 `t.Type() != core.TaskTypeTktube` 来判断是否应用 tktube 特有逻辑，而不是依赖具体的 `*tktube.Task` 类型断言。这使得代码可在不导入具体任务包的情况下正常工作。

### Metadata
- Tags: design_pattern, interface

---

## [LRN-20260616-012] best_practice — `maps.Copy` 替代手动 for-range map 拷贝 (Go 1.21+)

**Logged**: 2026-06-16T11:30:00Z
**Priority**: low
**Status**: resolved
**Area**: backend

### Summary
Go 1.21+ 的 `maps.Copy(dst, src)` 可以替代手写 `for k, v := range src { dst[k] = v }`。

### Metadata
- Tags: Go1.21, maps, simplification
