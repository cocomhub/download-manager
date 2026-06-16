# Learnings

Corrections, insights, and knowledge gaps captured during development.

**Categories**: correction | insight | knowledge_gap | best_practice

---

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
# Learnings

Corrections, insights, and knowledge gaps captured during development.

**Categories**: correction | insight | knowledge_gap | best_practice

---

## [LRN-20260616-013] insight — golangci-lint v2: `//lint:ignore` 对 import 块无效，需用 `//nolint:staticcheck`

**Logged**: 2026-06-16T08:00:00Z
**Priority**: medium
**Status**: pending
**Area**: infra

### Summary
在 golangci-lint v2 中，`//lint:ignore SA1019` 放在 import 语句前的单独行无法抑制 lint 错误。需要改为 `//nolint:staticcheck` 放在 import 行行末。

### Details
```go
// 无效（v2 不识别）
//lint:ignore SA1019 deprecated dlcore
"github.com/cocomhub/pkg/dlcore"

// 有效
"github.com/cocomhub/pkg/dlcore" //nolint:staticcheck // deprecated dlcore
```

另：`//nolint:SA1019` 也不被 v2 识别（报 `Found unknown linters in directives: sa1019`），必须用 `//nolint:staticcheck`（linter 名而非规则名）。

### Metadata
- Source: error
- Related Files: .golangci.yml, downloader/native.go
- Tags: golangci-lint, lint_suppression, nolint

---

## [LRN-20260616-014] best_practice — Playwright 截图快照模板应使用 {projectName} 而非 {platform}

**Logged**: 2026-06-16T08:30:00Z
**Priority**: high
**Status**: pending
**Area**: tests

### Summary
Playwright 默认 `snapshotPathTemplate` 使用 `{platform}` 变量，在不同 OS 上生成不同后缀名（win32/linux/darwin）。这导致 CI（Linux）找不到开发机（Windows）生成的截图基线。

### Details
```ts
// 默认（跨平台不兼容）
snapshotPathTemplate: '{arg}-{platform}{ext}'
// → heading-desktop-win32.png (Windows)
// → heading-desktop-linux.png (Linux)  ❌ 找不到

// 修复（跨平台兼容）
snapshotPathTemplate: '{testFileDir}/{testFileName}-snapshots/{arg}-{projectName}{ext}'
// → heading-desktop.png (所有平台)
```

`{projectName}` 取自 `playwright.config.ts` 中 `projects[].name`（如 `desktop`、`firefox`、`webkit`），不依赖 OS 名称。

### Metadata
- Source: error
- Related Files: test/playwright/playwright.config.ts
- Tags: playwright, snapshot, cross-platform, CI

---

## [LRN-20260616-015] best_practice — Edit 工具因实际缩进差异失败时改用 cat -An + sed

**Logged**: 2026-06-16T08:45:00Z
**Priority**: medium
**Status**: pending
**Area**: config

### Summary
当 Edit 工具反复返回 `String to replace not found` 时，可能是因为文件实际缩进（tab 空格混合）与预期不符。

### Details
调试方法：
1. `sed -n 'N,Mp' target_file | cat -An` — 显示行号、tab 为 `^I`、行末为 `$`
2. 用 tab 对齐实际内容复制到 Edit 调用中
3. 如仍不匹配，改用 `sed` 直接操作文件：
   ```bash
   sed -i 'N,Md' target_file  # 删除 N-M 行
   sed -i '43i\    // insert this line' target_file  # 在第 43 行前插入
   ```

注意：`sed -i` 在 Git Bash 可用，PowerShell 中要用 `bash -c` 包裹。

### Metadata
- Source: error
- Related Files: N/A
- Tags: editing, troubleshooting, sed

---

## [LRN-20260616-016] best_practice — Playwright 测试中 axe-core 对比度类违规的调试方法

**Logged**: 2026-06-16T09:00:00Z
**Priority**: medium
**Status**: pending
**Area**: tests

### Summary
axe-core `color-contrast` 违规仅报告数量，不报告具体元素。需要手动在测试中遍历 `v.nodes` 输出 `node.html` 和 `node.target` 才能定位哪个元素失败。

### Details
```ts
colorViolations.forEach(v => {
  console.log(`  - ${v.help}: ${v.nodes.length} nodes`);
  v.nodes.forEach((node, i) => {
    console.log(`    node ${i}: ${node.html}`);
    console.log(`    target: ${node.target.join(', ')}`);
  });
});
```

定位到问题后，修复方案是改变 CSS 颜色组合提高对比度（≥4.5:1 for AA）：
- 低对比度：`bg-white` + `text-blue-500` + `border-blue-500` ≈ 2.8:1 ❌
- 修复：`bg-blue-600` + `text-white` ≈ 4.6:1+ ✅

### Metadata
- Source: error
- Related Files: test/playwright/specs/accessibility.spec.ts, web/static/index.html
- Tags: axe-core, accessibility, color_contrast, debugging

---

## [LRN-20260616-017] best_practice — CI 私有依赖认证配置模式

**Logged**: 2026-06-16T09:15:00Z
**Priority**: high
**Status**: pending
**Area**: infra

### Summary
Go module 依赖私有 GitHub 仓库时，需要在 CI 的 `go mod verify` 前配置：

1. Git URL 重写（用 PAT 鉴权）：
   ```bash
   git config --global url."https://x-access-token:${GH_PAT}@github.com/".insteadOf "https://github.com/"
   ```
2. 环境变量（绕过 sumdb/sumcheck）：
   ```
   GOPRIVATE: github.com/cocomhub/*
   GONOSUMCHECK: github.com/cocomhub/*
   GONOSUMDB: github.com/cocomhub/*
   ```

注意：`GITHUB_TOKEN` 默认为 `Contents: read` 但不对同一 org 的其他私有仓库授权。必须使用显式 PAT（`secrets.GH_PAT`）且具有目标仓库的 `Contents: read` 权限。

### Metadata
- Source: error
- Related Files: .github/workflows/ci.yml
- Tags: CI, private_repo, authentication, Go_modules

---

## [LRN-20260616-018] best_practice — go:embed 修改后必须重新构建二进制

**Logged**: 2026-06-16T09:30:00Z
**Priority**: low
**Status**: pending
**Area**: infra

### Summary
修改 `//go:embed` 文件（如 `web/static/index.html`）后，测试服务器二进制不会自动更新。必须显式重新构建（`go build`）才能生效。

### Details
Playwright 测试使用预构建的 `playwright-server.exe`（在 `SERVER_BINARY` 环境变量中指定），如果只改了 HTML/CSS 但没重新构建，测试仍然使用旧版内嵌资源。

修复：在测试前执行 `cd cmd/playwright-server && go build -o playwright-server .`。

### Metadata
- Source: error
- Related Files: cmd/playwright-server/main.go, web/static/index.html
- Tags: go:embed, build_cache, testing

---
