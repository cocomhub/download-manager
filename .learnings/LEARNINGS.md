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

## [LRN-20260617-001] best_practice — 数据竞争保护使用专用锁 + getter/setter

**Logged**: 2026-06-17T16:00:00Z
**Priority**: high
**Status**: promoted
**Area**: backend

### Summary
`m.downloader` 被 Manager 的 work goroutine（读）、UpdateConfig（写）、测试代码（写）三方同时访问，产生 data race。修复方式：独立 `downloaderMu sync.Mutex` + `getDownloader()`/`setDownloader()` 封装。

### Details
不同于 `m.mu`（保护 activeDownloads 热路径），使用专用锁避免锁竞争。所有读/写点（生产代码 + 测试代码）都通过 getter/setter，不再直接字段赋值。

涉及修改的 11 个文件：`manager.go`、`download.go`、`task_loader.go`、`tasks.go`、`hot_reload_test.go`、`race_test.go`、`e2e_test.go`、`mock_integration_test.go`、`scheduler_queue_test.go`。

### Metadata
- Source: error
- Related Files: manager/manager.go, manager/*.go, manager/*_test.go
- Tags: data_race, concurrency, mutex, downloader

### Promoted
- CLAUDE.md

---

## [LRN-20260617-002] best_practice — Playwright 纯文字截图不跨 OS 兼容

**Logged**: 2026-06-17T16:30:00Z
**Priority**: high
**Status**: promoted
**Area**: tests

### Summary
`h1:has-text("Tasks")` 纯文字元素在 Ubuntu CI（62×28px）与 Windows 本地（54×28px）渲染尺寸不同，连续 4 次 CI 运行 pixel diff 均超阈值。最终改用文本断言替代 snapshot。

### Details
连续修复演进：100→500→5000→10000→6000 maxDiffPixels 都无法覆盖 OS 字体差异。最终改为：
```ts
const heading = page.locator('h1:has-text("Tasks")');
await expect(heading).toBeVisible();
await expect(heading).toHaveText('Tasks');
```

**教训**：纯文字元素只验证存在性和内容，不验证像素。结构性元素（grid、sidebar）才保留 snapshot。

### Metadata
- Source: error
- Related Files: test/playwright/specs/visual-regression.spec.ts
- Tags: playwright, snapshot, cross-platform, CSS, font_rendering

### Promoted
- CLAUDE.md

---

## [LRN-20260617-003] insight — Playwright route() 延时处理竞争

**Logged**: 2026-06-17T16:45:00Z
**Priority**: medium
**Status**: promoted
**Area**: tests

### Summary
`page.context().route('**/api/**', async (route) => { await sleep(3000); route.continue(); })` 在 firefox 下，并发 API 请求到达时第一条请求阻塞 3s，第二条请求看到路由仍在"占用"中，报 `Route is already handled!`。

### Details
修复方案：
1. 加 `routeHandled` guard 只拦截首条请求，其余 pass-through
2. 跳过 health check 端点保 worker 心跳（避免 healthz 被延迟触发 503）

### Metadata
- Source: error
- Related Files: test/playwright/specs/network-resilience.spec.ts
- Tags: playwright, route, firefox, concurrency

### Promoted
- CLAUDE.md

---

## [LRN-20260617-004] best_practice — Manager worker() 空闲心跳保持

**Logged**: 2026-06-17T17:00:00Z
**Priority**: high
**Status**: promoted
**Area**: backend

### Summary
`worker()` 在 `downloadQueue` 通道无消息时 select 阻塞，不更新 `workerHeartbeat`。health check 在 5s 内无心跳 → workers 组件 503。fault-injection 测试 R2 连续失败。

### Details
```go
func (m *Manager) worker() {
    m.workerHeartbeat.Store(time.Now())
    hbTicker := time.NewTicker(3 * time.Second)
    defer hbTicker.Stop()
    for {
        select {
        case req := <-m.downloadQueue:
            m.workerHeartbeat.Store(time.Now())
            m.download(req.task, req.obj)
        case <-hbTicker.C:
            m.workerHeartbeat.Store(time.Now())  // idle heartbeat
        case <-m.stopChan:
            return
        }
    }
}
```

### Metadata
- Source: error
- Related Files: manager/runtime_mgr.go, manager/health.go
- Tags: health_check, worker, heartbeat, 503

### Promoted
- CLAUDE.md

---

## [LRN-20260617-005] best_practice — CI benchmark step 需要 continue-on-error

**Logged**: 2026-06-17T17:15:00Z
**Priority**: low
**Status**: promoted
**Area**: infra

### Summary
`benchmark-action/github-action-benchmark@v1.22.1` 在 `gh-pages` 分支不存在时 `git fetch` 失败（exit 128），直接中断整个 job。但 benchmark 报告的推送不是核心测试，不应阻塞 CI。

### Details
```yaml
- uses: benchmark-action/github-action-benchmark@...
  continue-on-error: true
```

### Metadata
- Source: error
- Related Files: .github/workflows/ci.yml
- Tags: CI, benchmark, gh-pages

### Promoted
- CLAUDE.md

---

## [LRN-20260617-006] best_practice — TestE2E_MixedResults 随机概率设计

**Logged**: 2026-06-17T17:30:00Z
**Priority**: low
**Status**: promoted
**Area**: tests

### Summary
`TestE2E_MixedResults` 使用 `fail_rate=0.5`, 10 个 objects。有 `0.5^10=0.1%` 概率全部成功导致测试失败。在 CI 多平台长期运行中，概率虽小但必然发生。

### Details
修复：`fail_rate=0.4`，全部成功概率降到 `0.4^10≈0.001%`。更低 fail_rate 确保多数对象失败的同时仍有机会部分成功满足 `1×completed` 断言。

**教训**：随机概率测试要评估最坏情况的概率。长期运行 CI 中，千分之一概率 ≈ 每 1000 次运行失败 1 次，在全量 CI（4 平台 × 日均多次）下不是足够小。

### Metadata
- Source: error
- Related Files: manager/e2e_test.go
- Tags: testing, probability, flaky_test

### Promoted
- CLAUDE.md

---

## [LRN-20260617-007] best_practice — bandwidth probe 零时长保护

**Logged**: 2026-06-17T17:45:00Z
**Priority**: medium
**Status**: pending
**Area**: backend

### Summary
`CheckBandwidth` 使用 httptest 本地服务测带宽，Windows CI 上快于 1ns 导致 `elapsed < 0` → 返回 `"elapsed time too short"` 错误。调整为 fallback 1ns 避免除零即可。

### Details
```go
elapsed := time.Since(start)
if elapsed <= 0 {
    elapsed = time.Nanosecond  // 本地 probe 极快时保护
}
```

### Metadata
- Source: error
- Related Files: pkg/download/bandwidth.go
- Tags: bandwidth, windows, division_by_zero

---

## [LRN-20260617-008] best_practice — 连续 CI 修复的调试工作流

**Logged**: 2026-06-17T18:00:00Z
**Priority**: medium
**Status**: pending
**Area**: config

### Summary
本次 CI 修复经历了 4 轮 push（commit 1→4），每轮发现新问题。高效工作流：

1. **一次性获取所有失败**：`gh run view --log | grep -E "✘|FAIL|##\[error\]"` 找出所有失败点
2. **按根因分组**：同一测试连续失败先看看趋势（是否同一个 snapshot 持续 diff）
3. **递增容差 vs 彻底修改**：snapshot 容差递增（100→500→5000→10000）纯属浪费，到第三次就应该意识到该换方案
4. **锁定版本**：github-action-benchmark pin 到 SHA 而非 tag，但 remote ref 不存在是上游问题，用 `continue-on-error` 绕过
5. **CI 代码不可测试**：CI 配置和测试修复必须 push 才能验证，应在本地用等效命令先验证

### Metadata
- Source: insight
- Tags: CI, debugging, workflow

---

## [LRN-20260617-009] best_practice — `github.com/cocomhub/sproxy` 私有依赖认证模式

**Logged**: 2026-06-17T18:15:00Z
**Priority**: high
**Status**: pending
**Area**: infra

### Summary
sproxy 是 download-manager 的私有 Go 模块依赖。CI 中需要：
1. 每个 job 都要添加 "Configure private module access" step（不能只在 test job 有）
2. lint job 的 `golangci-lint-action` 需要解析 Go module，也需要认证
3. `GOPRIVATE`/`GONOSUMCHECK`/`GONOSUMDB` 防止 sum.golang.org 验证

原代码只在 test job 有配置，lint 和 playwright job 都缺少，导致 lint 拉不到 sproxy 失败。

### Metadata
- Source: error
- Related Files: .github/workflows/ci.yml
- Tags: CI, private_module, authentication, sproxy

---

## [LRN-20260618-001] best_practice — Go 1.26 测试最佳实践迁移

**Logged**: 2026-06-18T19:00:00Z
**Priority**: high
**Status**: promoted
**Area**: tests

### Summary
全仓 84 个测试文件大规模迁移到 Go 1.26 最佳实践：t.Context() 替代 context.Background()、b.Loop() 替代 b.N、80+ 处 time.Sleep 替换为 assert.MustEventually 轮询。

### Details
**迁移内容：**
- `t.Context()` (Go 1.24+) — 替换所有测试函数中的 `context.Background()`。注意：`t.Cleanup` 内的 `context.Background()` 不能替换（测试结束后 t.Context() 已 cancel）
- `b.Loop()` (Go 1.24+, 1.26 不再阻止内联) — 7 个 benchmark 全部迁移，性能收益约 10-25%
- `t.Helper()` — HTTP 测试辅助函数（doJSONGet/doJSONPost）签名加 `t *testing.T` + `t.Helper()`
- `errors.Is` — 替代 `err.Error() == "context canceled"` 字符串匹配

### Suggested Action
新建/修改测试时默认使用 t.Context()、b.Loop()、table-driven + name 字段。

### Promoted
- download-manager/CLAUDE.md （Go 1.26 测试实践）

### Metadata
- Source: refactoring
- Related Files: 全部 *_test.go（~50 文件改动）
- Tags: go_1.26, testing, best_practice

---

## [LRN-20260618-002] best_practice — time.Sleep 同步的替换策略

**Logged**: 2026-06-18T19:00:00Z
**Priority**: critical
**Status**: promoted
**Area**: tests

### Summary
80+ 处 `time.Sleep` 用于异步同步，全部替换为 `assert.MustEventually` 轮询或 ready channel。这是 CI 稳定性的最大收益来源。

### Details
**替换模式：**
1. **API 测试等待 task seed** → `assert.MustEventually(t, fn, 3s, 50ms, "msg")` 轮询直到端点返回 200
2. **goroutine 启动同步** → ready channel: 
   ```go
   ready := make(chan struct{})
   go func() { close(ready); doWork(); done <- true }()
   <-ready // 等待 goroutine 被调度
   ```
3. **轮询等待特定状态** → `assert.MustEventually` 检查状态值
4. **TTL 过期等待** → 缩短 TTL 或改为轮询 TTL 结果
5. **共享工具** → 创建 `testutil/assert` 包，提供 `Eventually`/`MustEventually`

### 关键陷阱
- CI 上的 `time.Sleep` 比本地慢 10 倍，必须用条件轮询
- `MustEventually` 间隔用 50ms（快慢适中）
- 超时用 3s（CI 友好），单个慢测试用 10s
- `waitForObjectsFinal` 的 300ms 间隔改为 100ms（之前太保守）

### Promoted
- download-manager/CLAUDE.md （time.Sleep 替换策略）

### Metadata
- Source: refactoring
- Related Files: testutil/assert/assert.go, api/*_test.go, manager/*_test.go
- Tags: time_sleep, ci_stability, testing

---

## [LRN-20260618-003] insight — applySharedState 与 DownloaderAdapter 的 Metadata 数据竞争

**Logged**: 2026-06-18T19:00:00Z
**Priority**: critical
**Status**: promoted
**Area**: backend

### Summary
`task/base_task.go:applySharedState` 通过 `maps.Copy` 写入 `DownloadObject.Metadata`/`Extra`，但 `downloader/adapter.go` 和 `storage/query.go` 直接读取这些字段无锁保护，导致数据竞争。

### Details
**竞争链：**
- **写者**：后台 scanner goroutine → `GetAllObjects(true)` → `syncSharedToObjectLocked` → `applySharedState` → `maps.Copy(dst.Metadata, src.Metadata)`（持 `dst.Lock()`）
- **读者1**：HTTP handler → `Search()` → `matchesQuery()` / `metadataValue()` → `obj.Metadata[key]`（无锁）
- **读者2**：worker goroutine → `DownloaderAdapter.Download()` → `obj.Extra["files"]` / `copyMetadata(obj.Metadata)`（无锁）

**修复方案：**
- `storage/query.go`: `matchesQuery()` 和 `metadataValue()` 加 `obj.RLock()/RUnlock()`
- `downloader/adapter.go`: 读 `obj.Extra["files"]` 加 `RLock`，`OnMetadata` 写 `obj.Metadata[key]` 加 `Lock`，`DownloadResult` 写加 `Lock`
- `manager/aggregate.go`: `BackfillContentGroups` 写 Metadata 加 `Lock`
- `manager/scheduler.go`: `hasFiles()` 改为 `RLock`（之前是 `Lock`，降级为读锁）

### 关键教训
`MemoryStorage.Search()` 的 `RWMutex` 只保护 `objects` map 本身，不保护 `*DownloadObject` 内部的字段。
每个直接访问 `obj.Metadata`/`obj.Extra` 的地方都需要 `obj.RLock()`/`obj.Unlock()`。

### Promoted
- download-manager/CLAUDE.md （数据竞争保护模式）

### Metadata
- Source: code_review
- Related Files: downloader/adapter.go, storage/query.go, manager/aggregate.go, manager/scheduler.go, task/base_task.go
- Tags: data_race, concurrency, metadata

---

## [LRN-20260618-004] insight — CancelObject 与 resolve worker 的时序竞争

**Logged**: 2026-06-18T19:00:00Z
**Priority**: critical
**Status**: promoted
**Area**: backend

### Summary
`Manager.CancelObject` 把对象设为 `cancelled` 后，resolve worker 通过 `syncSharedToObjectLocked` 从 shared registry 读回旧状态并覆盖为 `pending`，导致 undo_cancel 失败（`object status is not cancelled`）。

### Details
**竞争时序：**
1. `CancelObject` → `t.UpdateStatus(obj, StatusCancelled)` → 状态变为 cancelled
2. resolve worker → `mockTask.Scrape()` → `GetAllObjects(true)` → `syncSharedToObjectLocked` → `applySharedState` → 从 shared registry 读到旧状态（pending/downloading）→ 覆盖为 pending
3. `UndoCancelObject` → 检查 `obj.GetStatus() != StatusCancelled` → 返回 `"object status is not cancelled"`

**根因修复：** 这本质上是 mock task 的问题（`Scrape` 在后台拉取 shared registry），但真实场景中也需要处理。最佳实践是在 `CancelObject` 后轮询确认状态。

**测试修复模式：**
```go
assert.MustEventually(t, func() bool {
    rr := doJSONPost(t, r, "/api/tasks/id/object/cancel", body)
    return rr.Code == http.StatusOK  // 重试直到 cancel 成功
}, 3*time.Second, 50*time.Millisecond, "cancel should succeed")
```

### Promoted
- download-manager/CLAUDE.md （CancelObject 竞争模式）

### Metadata
- Source: debugging
- Related Files: manager/tasks.go, manager/scheduler.go, task/base_task.go
- Tags: data_race, cancel, timing

---

## [LRN-20260618-005] best_practice — assert.MustEventually 超时消息格式

**Logged**: 2026-06-18T19:00:00Z
**Priority**: medium
**Status**: pending
**Area**: tests

### Summary
`testutil/assert/assert.go` 的 `MustEventually` 用一个 `msgAndArgs ...any` 参数实现格式化消息，但类型断言 `msgAndArgs[0].(string)` 在传入非字符串类型时静默丢弃消息。

### Details
```go
// 脆弱点：如果调用者传入非 string 第一参数，上下文丢失
if msg, ok := msgAndArgs[0].(string); ok {
    t.Fatalf("assert.MustEventually timed out after %v: "+msg, append([]any{timeout}, msgAndArgs[1:]...)...)
    return
}
t.Fatalf("assert.MustEventually timed out after %v", timeout)
```

API 应该用 `fmt.Sprintf` 或要求明确的 format string + args。

### Suggested Action
将消息格式改为：`func MustEventually(t testing.TB, fn func() bool, timeout time.Duration, msg string, args ...any)`，直接使用 `t.Fatalf("assert.MustEventually timed out after "+msg, args...)`。

### Metadata
- Source: code_review
- Related Files: testutil/assert/assert.go
- Tags: testing, api_design

---

## [LRN-20260618-006] best_practice — 新增测试时的 Manager 测试模式

**Logged**: 2026-06-18T19:00:00Z
**Priority**: medium
**Status**: pending
**Area**: tests

### Summary
编写 Manager 测试时使用 `newMockManager(t, ...)` + `startManager(t, mgr)` + `waitForTask(t, mgr, ...)` 组合。新的 `manager_coverage_test.go` 验证了 22 个场景。

### 关键模式
```go
mgr, _ := newMockManager(t, "task-id", objCount, mockdl.New(mockdl.ModeAlwaysSuccess))
_ = waitForTask(t, mgr, "task-id") // 等待 task 加载
task, _ := mgr.getTask("task-id")  // 获取 task 实例
```

### Metadata
- Source: refactoring
- Related Files: manager/manager_coverage_test.go, manager/mock_integration_test.go
- Tags: testing, manager

---

## [LRN-20260618-007] insight — 独立测试函数转表驱动的保守策略

**Logged**: 2026-06-18T19:00:00Z
**Priority**: low
**Status**: pending
**Area**: tests

### Summary
只有确实测试同一代码路径的独立函数才应合并为表驱动。测试不同接口/行为的独立函数（如 29 个 `download_test.go` 函数测试不同接口）保持原样更清晰。

### Details
**合并不当的代价：**
- 跨多行的表条目比独立函数更难读
- 不同行为路径共享 setup 导致隐式依赖
- 失败时子测试名需要人为理解

**本次合并的案例：**
- `testutil/mockdl`: AlwaysFail（3 函数） + RandomFail（2 函数） → 表驱动 ✅
- `http_extractor_test.go`: Caching（3 函数） + Resume（2 函数） → 表驱动 ✅
- 未合并：29 个 download_test.go 函数测试不同接口 ❌

### Metadata
- Source: code_review
- Related Files: testutil/mockdl/downloader_test.go, pkg/download/http_extractor_test.go
- Tags: testing, table_driven

---

## [LRN-20260619-001] correction — MemoryStorage.Search 结果不保证有序

**Logged**: 2026-06-19T07:00:00Z
**Priority**: medium
**Status**: promoted
**Area**: tests

### Summary
`MemoryStorage.Search` 依赖 map 迭代，返回顺序不固定。测试中按索引比对 `results[i].URL == wantURLs[i]` 会偶发失败。

### Details
`factory_coverage_test.go` 的 Search 测试假设返回顺序与插入顺序一致，但 `MemoryStorage.Search` 遍历 `sync.Map`（底层是无序 hash map），导致：
```
result[0].URL = "http://c.com", want "http://a.com"
```
**修复**：改用无序集合比较：将结果 URL 放入 `map[string]bool`，逐一检查期望 URL 是否存在。

### Promoted
- CLAUDE.md（新增：`MemoryStorage.Search` 不保证有序）

### Metadata
- Source: error
- Related Files: storage/factory_coverage_test.go, storage/factory.go
- Tags: testing, storage, map_iteration_order

---

## [LRN-20260619-002] correction — workerCount 应使用 atomic.Int64 而非 int + m.mu 保护

**Logged**: 2026-06-19T07:00:00Z
**Priority**: high
**Status**: resolved
**Area**: backend

### Summary
`m.workerCount` 被多个 goroutine 读写（`GetHealthStatus`、`CollectMetrics`、`adjustGlobalWorkers`、`Start`、`UpdateConfig`），最初跨越 3 个文件（health.go、metrics.go、runtime_mgr.go+manager.go+scheduler.go）。先用 `m.mu` 加锁解决 data race 后，用户进一步指出应改用 `atomic.Int64`。

### Details
**第一轮修复**（加锁）：3 处读取加了 `m.mu.Lock()/Unlock()`，仍遗漏了 `metrics.go` 的一处。

**第二轮修复**（atomic 化）：将 `workerCount` 从 `int` 改为 `atomic.Int64`（`manager.go:51`），所有 10 处读写全部改为原子操作：
- 写（4 处）：`Store(int64(limit))`
- 读（6 处）：`Load()`（3 处直接 + 3 处通过 `getWorkerCount()` 封装）

**关键教训**：加锁是**临时止血**，原子类型是**根治**。`int` 字段跨 5 个文件、3 个 goroutine 路径共享时，锁保护遗漏几乎是必然的。`atomic.Int64` 声明即安全，每个读/写点都是显式的 `Load()/Store()`。

### Resolution
- **Resolved**: 2026-06-19T08:00:00Z
- **Fix**: `int` → `atomic.Int64`，所有 10 处读写点统一改为原子操作

### Promoted
- CLAUDE.md（新增：shared int 字段优先考虑 atomic 类型）

### Metadata
- Source: correction
- Related Files: manager/manager.go, manager/runtime_mgr.go, manager/scheduler.go, manager/health.go, manager/metrics.go, manager/worker_stop_test.go
- Tags: data_race, atomic, concurrency, int_to_atomic
- See Also: LRN-20260619-007

---

## [LRN-20260619-003] correction — mockdl 测试中 AlwaysSuccess URLRouting 重复模式可归并

**Logged**: 2026-06-19T07:00:00Z
**Priority**: low
**Status**: resolved
**Area**: tests

### Summary
`testutil/mockdl/downloader_test.go` 的 `TestMockDownloader_AlwaysSuccess` 和 `TestMockDownloader_AlwaysSuccess_FiresCallbacks` 本质是同一代码路径的不同验证维度，以及 `TestMockDownloader_FailURLs` 和 `TestMockDownloader_TimeoutURLs` 是同一路由机制的不同输入。

### Details
合并为两组表驱动测试后，减少了 50% 的文件行数且保持了同样的覆盖率。

### Metadata
- Related Files: testutil/mockdl/downloader_test.go
- Tags: testing, table_driven, mockdl

---

## [LRN-20260619-004] best_practice — 多 agent 并行修改时的防冲突策略

**Logged**: 2026-06-19T07:00:00Z
**Priority**: medium
**Status**: pending
**Area**: config

### Summary
使用并行 agent 处理独立文件修改时，必须确保每个 agent 操作完全不相交的文件集。

### Details
本轮派发了 3 个并行 agent，分别操作：
- Agent 1：`testutil/mockdl/` + 4 个 manager/pkg 文件（time.Sleep）
- Agent 2：新建 `model/`/`storage/`/`configutil/`/`manager/` 的 `*_coverage_test.go`
- Agent 3：`pkg/download/http_extractor_test.go`
得益于文件无交集，3 个 agent 同时执行零冲突。**注意**：`manager/` 目录有 2 个 agent 同时修改（Agent 1 改 `manager/hot_reload_test.go` 等 3 文件，Agent 2 新建 `manager/config_mgr_coverage_test.go`）——属于不同文件，无冲突。

### Metadata
- Source: insight
- Related Files: 多个 *_test.go
- Tags: parallel_agents, orchestration, testing

---

## [LRN-20260619-005] best_practice — 第二轮覆盖率提升的关键模式

**Logged**: 2026-06-19T07:00:00Z
**Priority**: low
**Status**: pending
**Area**: tests

### Summary
第二轮覆盖率提升覆盖了 config_mgr 的 8 个方法 + config_service 的 5 个工具函数 + model 的 MarshalJSON 边界情况。

### Key Patterns
1. **MarshalJSON nil receiver**：`(*DownloadObject)(nil).MarshalJSON()` 应返回 `[]byte("null")` 而不是 panic
2. **MemoryStorage 并发路径**：多个 goroutine 同时调用 `Get/Update/Delete` 验证 `sync.RWMutex` 保护
3. **ConfigService 方法**：需要 mock FileSystem 来测试文件读写路径
4. **config_mgr 方法**：需要 Manager 完整启动 + mock task 才能调用

### Metadata
- Source: refactoring
- Tags: testing, coverage

---

## [LRN-20260619-006] best_practice — progresslog flush 测试的纯时间等待模式

**Logged**: 2026-06-19T07:00:00Z
**Priority**: low
**Status**: pending
**Area**: tests

### Summary
`pkg/download/progresslog_test.go` 的 flush 测试本质上是等待时间流逝（`maxInterval > 0` 时触发 flush），不能使用 channel 通知或轮询条件来缩短等待——必须真等 60ms。

### Details
这种测试无法用 `MustEventually` 代替，因为触发条件是定时器到期。唯一可以优化的是将 `select { case <-time.After(...): }` 显式化，而非 `time.Sleep(...)`，使意图更清晰。

### Metadata
- Related Files: pkg/download/progresslog_test.go
- Tags: testing, timing, progresslog

---

## [LRN-20260619-007] correction — 代码审查遗漏 metrics.go 的同系列 data race

**Logged**: 2026-06-19T08:00:00Z
**Priority**: high
**Status**: resolved
**Area**: backend

### Summary
修复 `health.go` 的 `workerCount` data race 后，同系列问题在 `metrics.go:63` 处漏修。代码审查发现时才补上。

### Details
`GetHealthStatus()` 的 `workerCount` data race 修复后，紧跟着检查 `manager/metrics.go` 发现同样存在 `m.workerCount` 无锁读取。`CollectMetrics()` 被 API 调用触发，`adjustGlobalWorkers()` 被配置热加载触发，两者并发时触发 data race。

**根因**：修复时只扫描了 `health.go` 的调用栈，没有 grep 全项目查 `workerCount` 的全部引用。修复后应 grep 全项目确认无其他遗漏。

### Resolution
- **Resolved**: 2026-06-19T08:00:00Z
- **Fix**: 所有 6 处读取统一用 `m.getWorkerCount()` 封装方法

### Metadata
- Source: error
- Related Files: manager/metrics.go, manager/health.go
- Tags: data_race, regression_prevention, grep_all_refs
- See Also: LRN-20260619-002

---

## [LRN-20260619-008] best_practice — 共享 int 字段的并发治理：先加锁止血，再 atomic 根治

**Logged**: 2026-06-19T08:00:00Z
**Priority**: medium
**Status**: pending
**Area**: backend

### Summary
多 goroutine 共享的 `int` 字段，锁保护容易遗漏。`atomic.Int64` 从类型声明层面消除遗漏风险。

### 决策树
```
共享 int 字段被多个 goroutine 读写？
├── 仅同一文件内 2-3 处访问 → sync.Mutex 足够
├── 跨 3+ 文件 / 5+ 访问点 → atomic.Int64（声明即安全）
└── 读多写少 + 读路径是热点（health check、metrics 等高频调用）→ atomic.Int64（无锁竞争）
```

### 操作方法
1. 声明：`workerCount atomic.Int64`
2. 写：`m.workerCount.Store(int64(val))`
3. 读：`m.workerCount.Load()`（返回 `int64`，需要 `int` 时 `int(m.workerCount.Load())`）
4. 封装读方法供外部用：`func (m *Manager) getWorkerCount() int { return int(m.workerCount.Load()) }`

### 注意事项
- `atomic.Int64.Load()` 返回 `int64`，与 `int` 比较/运算时需要显式转换
- `atomic.Int64` 是 struct 类型（含 `noCopy`），传值会编译错误，方法接收者必须是指针

### Metadata
- Source: refactoring
- Related Files: manager/manager.go
- Tags: concurrency, atomic, best_practice
- See Also: LRN-20260619-002, LRN-20260619-007

---

## [LRN-20260619-009] insight

**Logged**: 2026-06-19T05:30:00Z
**Priority**: high
**Status**: pending
**Area**: tests

### Summary
dlcore 和 pkg/download 的 maxRetries=0 语义不同 — 这是最具破坏性的行为差异

### Details
`dlcore` 中 `maxRetries=0` 表示"无限制重试"（仅受 `goto startDownload` 循环控制，cnt 从 0 到 maxRetries 不含等号 → 0 时永不退出）。
`pkg/download` 中 `maxRetries=0` 表示"不重试"（`for attempt := 1; attempt <= maxRetries; attempt++` → 0 时一次都不执行）。

这导致 Comparator 默认使用 `MaxRetries=0` 时，dlcore 能成功（无限重试直到成功），新路径则直接"max retries reached (0)"。

### Suggested Action
Comparator 默认值改为 `MaxRetries=3`，始终设明确的正值避免歧义。

### Metadata
- Source: error
- Related Files: downloader/beacon_test.go, downloader/adapter_functional_test.go
- Tags: maxRetries, behavioral_diff, compat

---

## [LRN-20260619-010] error

**Logged**: 2026-06-19T05:06:00Z
**Priority**: high
**Status**: resolved
**Area**: tests

### Summary
NativeHTTPDownloader 的 LogDir 配置导致 Windows 非法路径错误

### Error
```
ERROR Failed to create log directory dir=C:\Temp\001\C:\Temp\002
error="mkdir C:\Temp\001\C:: The filename, directory name, or volume label syntax is incorrect."
```

### Context
`NativeHTTPDownloader` 在 `NewNativeHTTPDownloader` 中处理 `LogDir` 时，如果配置了 `Filesystem.LogDir` 为绝对路径，它会通过 `filepath.Join(rootDir, logDir)` 拼接，产生 `rootDir/logDir` 的路径。当两者都是 Windows 绝对路径（`C:\...`），`filepath.Join` 保留第一个参数中的卷标，第二个参数的卷标被当作目录名，产生非法路径。

根源是 `NativeHTTPDownloader` 期望 `LogDir` 是相对于 `RootDir` 的相对路径，而 `config.ValidateAndClamp` 迁移旧字段时未对路径做相对/绝对判断。

### Suggested Fix
Comparator 构造中不默认设置 LogDir，仅当用户通过 `WithLogDir` 显式指定时才设置。

### Resolution
- **Resolved**: 2026-06-19T05:07:00Z
- **Notes**: Comparator 默认去掉 LogDir 设置，用 `WithLogDir` 选项控制

### Metadata
- Reproducible: yes
- Related Files: downloader/beacon_test.go, config/config.go
- Tags: windows, path_join, NativeHTTPDownloader

---

## [LRN-20260619-011] insight

**Logged**: 2026-06-19T05:14:00Z
**Priority**: medium
**Status**: pending
**Area**: tests

### Summary
MD5 测试中必须使用与内容实际哈希匹配的 checksum，否则触发无限重试

### Details
`dlcore` 在下载完成后会做 `computeFileMD5` 校验。如果响应头中的 `Content-MD5` / `Etag` / `X-Amz-Meta-Md5chksum` 与文件实际内容不匹配：
- 截断文件
- `goto startDownload` 重新下载
- 重试计数未超出 maxRetries 则永不停止

测试 `TestFunc_MD5_XAmzMetaHeader` 最初使用 `"test content"` + `"dUKw7TnL3Tp9KHhHX4e3MQ=="`（不匹配），导致双方都在重试循环中。

修复：使用空内容 `""` + `"1B2M2Y8AsgTpgAmY7PhCfg=="`（空内容的 base64 MD5）确保匹配。

对于 `TestFunc_MD5_ContentMD5Header` 使用 `"hello"` + `"5d41402abc4b2a76b9719d911017c592"`（hello 的 hex MD5）。

### Suggested Action
MD5 测试中先计算实际内容的 MD5 再写入 header 值，或在注释中标注使用的算法和对应值。

### Metadata
- Source: error
- Related Files: downloader/adapter_functional_test.go
- Tags: md5, retry, testing

---

## [LRN-20260619-012] insight

**Logged**: 2026-06-19T05:17:00Z
**Priority**: high
**Status**: pending
**Area**: tests

### Summary
dlcore 和 pkg/download 有 5 项已知行为差异，测试中必须适配，不能硬断言一致性

### Details

| 差异 | dlcore | pkg/download | 影响 |
|---|---|---|---|
| `Metadata["status"]` | 写入 `"completed"` | 不写入 | CheckMetadata("status") 失败 |
| Content-Type text 检测 | text Content-Type + .mp4 URL → ErrNoTry | 无此检测 | CheckAnyError 失败 |
| 路径穿越保护 | ResolvePath 拒绝 ../ | 无此限制 | CheckAnyError 失败 |
| 500 错误重试 | 部分状态下重试成功 | 重试行为不同 | CheckBothNil 失败 |
| maxRetries=0 语义 | 无限重试 | 不重试 | 基本下载失败 |

### Suggested Action
对于已知差异的测试使用松断言（无 Check 或仅记录日志），并在测试文件顶部注释说明。这些差异应在后续迭代中逐步对齐。

### Metadata
- Source: error
- Related Files: downloader/adapter_contract_test.go, downloader/adapter_functional_test.go, downloader/adapter_e2e_test.go
- Tags: behavioral_diff, compat, gapline

---

## [LRN-20260619-013] best_practice

**Logged**: 2026-06-19T05:25:00Z
**Priority**: medium
**Status**: pending
**Area**: tests

### Summary
Beacon HTTP 服务器的动态 handler 中，测试行为隔离的注意事项

### Details
1. `HandleDynamic` 中的 `bodyFunc` 闭包捕获外部变量（如 `callCount`）时，同一 Beacon 实例的多次 `cmp.Run` 调用共享这些闭包状态。每个 `t.Run("name", ...)` 子测试应使用独立的 Beacon 实例。
2. `HandleResumeBreak` 模拟断连的最可靠方式：第一次返回部分内容，后续带 Range 的请求正常返回剩余部分。
3. `HandleSlow` 使用 `time.Sleep(delay)` 实现延迟响应，与 httptest 配合良好，但延迟时间应当适中（100ms-500ms），避免测试超时。
4. Range handler 使用 `fmt.Sscanf(rangeHeader, "bytes=%d-", &start)` 精确解析 Range 头，兼容标准 HTTP Range 格式。
5. 对于 Content-Length 为 0 的文件，dlcore 能正常创建空文件，不需要特殊处理。

### Metadata
- Source: best_practice
- Related Files: downloader/beacon_test.go
- Tags: testing, beacon, httptest

---

## [LRN-20260619-014] correction

**Logged**: 2026-06-19T05:10:00Z
**Priority**: medium
**Status**: resolved
**Area**: tests

### Summary
`go vet` 要求所有 `http.Get` 的返回值必须先检查 error 再使用 resp

### Error
```
downloader\beacon_test.go:610:8: using resp before checking for errors
```

### Context
`TestBeacon_Error` 中使用了 `resp, _ := http.Get(...)` 忽略 error，然后直接 `defer resp.Body.Close()`。vet 报 "using resp before checking for errors"。

### Suggested Fix
添加 `if err != nil { t.Fatal(err) }` 检查。

### Resolution
- **Resolved**: 2026-06-19T05:10:30Z
- **Notes**: 已添加 err 检查

### Metadata
- Reproducible: yes
- Related Files: downloader/beacon_test.go

---

## [LRN-20260619-015] insight

**Logged**: 2026-06-19T05:26:00Z
**Priority**: medium
**Status**: pending
**Area**: config

### Summary
`config.Config.Downloader` 的 `Type` 字段在测试中的选择约束

### Details
- `"native"`（默认）→ `New()` → `DownloaderAdapter` + `pkg/download.Downloader`
- `"native_old"` → `NewNativeHTTPDownloader()` → 内部使用 `pkg/dlcore.Client`
- `"wget"` → `NewWgetDownloader()` → 外部 wget 进程

Comparator 构造时两种路径使用不同的 Type：
- 旧路径强制 `cfgOld.Type = "native_old"`
- 新路径强制 `cfgNew.Type = "native"`

不能混淆，否则工厂函数行为不同。测试中若需要直接访问 dlcore 的底层行为（如 `oldClient`），应通过 `NewNativeHTTPDownloader` 构造。

### Metadata
- Source: insight
- Related Files: downloader/downloader.go, config/config.go
- Tags: config, factory

---

## [LRN-20260619-016] correction

**Logged**: 2026-06-19T23:30:00Z
**Priority**: high
**Status**: pending
**Area**: tests

### Summary
golangci-lint v2 使用 staticcheck 检测 deprecated 导入，比 `go vet` 更严格 — 必须加 `//nolint:staticcheck` 而非仅依赖 `go vet` 通过

### Details
CI 中的 golangci-lint (v2.12.2) 跑 staticcheck 检查，对 `import dlcore "pkg/dlcore"` 报 `SA1019: deprecated`。
`go vet` 本地不报此警告（vet 不检查 deprecated 导入）。
修复时尝试了多种 `//nolint` 注释格式：
- `//nolint:staticcheck // comment`（同一行）✅
- 在导入块上方单独写 `//nolint:staticcheck` ❌（golangci-lint 不认）

正确格式：`dlcore "github.com/.../pkg/dlcore" //nolint:staticcheck`

### Suggested Action
测试文件中引入已废弃包时必须加 nolint 注解。如果多个文件都需要，考虑在包级加 `//nolint:staticcheck`。

### Metadata
- Source: error
- Related Files: downloader/beacon_test.go
- Tags: lint, golangci-lint, nolint, deprecated

---

## [LRN-20260619-017] insight

**Logged**: 2026-06-19T23:30:00Z
**Priority**: medium
**Status**: pending
**Area**: tests

### Summary
`TestAPI_UndoCancelObject` 和 `TestAPI_UndoCancelObjectsBatch` 是已知 flaky 测试，CI 中偶发失败与本次提交无关

### Details
两测试均因 CancelObject 与 resolve worker 的时序竞争而失败：
- `TestAPI_UndoCancelObject`: "object status is not cancelled"
- `TestAPI_UndoCancelObjectsBatch`: MustEventually 超时

本地 `-race` 连续 5 次运行均通过。说明 CI 环境压力更大，竞争窗口更长。
CLAUDE.md 已有记录（`CancelObject 与 resolve worker 时序竞争`），但目前的轮询策略（3s 超时、50ms 间隔）在 CI 上仍不够。

### Suggested Action
PR 评审时可跳过这两个测试的重运行。长期需要增加轮询超时或优化 resolve worker 的写锁策略。

### Metadata
- Source: error
- Related Files: api/server_retry_test.go
- Tags: flaky, cancel, race_condition

---

## [LRN-20260619-018] insight

**Logged**: 2026-06-19T23:30:00Z
**Priority**: medium
**Status**: pending
**Area**: tests

### Summary
golangci-lint 的 unused linter 检测到测试文件中未使用的辅助函数（`httpCodeName`），这在 `go vet` 中不会报错

### Details
`go vet` 不检查未使用的函数（top-level function），但 golangci-lint 的 `unused` linter 会。这导致 CI lint 通过但本地 vet 通过的情况。
修复方式：删除未使用的函数。如果函数是为未来扩展编写，用注释解释并保留（隐式 `var _ = fn` 可满足 unused linter）。

### Suggested Action
测试文件提交前运行 `golangci-lint run`（不仅仅是 `go vet`）以确保 CI 通过。Windows 上可安装 golangci-lint 或在 CI 失败后再修复。

### Metadata
- Source: error
- Related Files: downloader/adapter_functional_test.go
- Tags: lint, unused, golangci-lint

---

## [LRN-20260619-019] best_practice

**Logged**: 2026-06-19T23:30:00Z
**Priority**: medium
**Status**: pending
**Area**: tests

### Summary
在 `go test` 命令中使用 `-run 'TestName$'`（$ 结尾锚点）可精确匹配指定测试函数名，排除其子测试或名称相似的测试

### Details
CI 测试计划中有两个相似的测试函数 `TestAPI_UndoCancelObject` 和 `TestAPI_UndoCancelObjectsBatch`。
调试单个测试时使用 `-run 'TestAPI_UndoCancelObject$'` 避免加载 `Batch` 版本。
本地复现 flaky 测试时连续运行 5 次确认是预先存在的竞争，非本次提交引入。

### Suggested Action
调试 flaky 测试时使用 `for i in 1..N; do go test -race -count=1 -run 'TestName$' ./pkg/ 2>&1 | tail -1; done` 模式。

### Metadata
- Source: best_practice
- Tags: testing, debugging, flaky

---

## [LRN-20260619-020] correction — TOCTOU 修复：Get+检查+Update 三步非原子

**Logged**: 2026-06-19T23:50:00Z
**Priority**: critical
**Status**: resolved
**Area**: backend

### Summary
`processResolve` 中 `Storage().Get()` → 检查 cancelled → `UpdateStatus(pending)` 三个步骤存在 TOCTOU 竞争窗口，CancelObject 可在这三步之间执行，导致取消被静默覆盖。

### Details
**首次修复（commit 147ad4f）** 只加了 cancelled 检查，但 Get 和 Update 之间没有锁保护：
```
T1: processResolve: Storage().Get(url) → pending
T2: CancelObject:   UpdateStatus(cancelled)
T3: processResolve: UpdateStatus(pending) → 覆盖取消！
```

**根治方案（PR #5）**：
1. `task/base_task.go` — 新增 `SetStatusUnlessCancelled` 方法，在 `b.mu` 保护下完成读-检查-写原子流程
2. 提取 `updateStatusLocked` 内部方法供 `UpdateStatus` 和 `SetStatusUnlessCancelled` 复用
3. `core/interfaces.go` — 新增 `TaskStatusGuard` 接口，避免 Manager 对 BaseTask 的具体依赖
4. `manager/resolve.go` — 通过 `t.(core.TaskStatusGuard)` 断言调用

同时修复了 `t.Storage()` 可能为 nil 时 `Storage().Get()` 触发 nil panic 的问题。

### Resolution
- **Resolved**: 2026-06-19T23:50:00Z
- **PR**: #5
- **Notes**: SetStatusUnlessCancelled 在 b.mu 下原子操作根除 TOCTOU

### Metadata
- Source: code_review
- Related Files: manager/resolve.go, task/base_task.go, core/interfaces.go
- Tags: TOCTOU, concurrency, cancel, resolve_worker

---

## [LRN-20260619-021] correction — errcheck 必须显式 suppress

**Logged**: 2026-06-19T23:55:00Z
**Priority**: medium
**Status**: resolved
**Area**: backend

### Summary
`updateStatusLocked` 返回 error，在 `SetStatusUnlessCancelled` 中调用时必须用 `_ =` 显式丢弃，否则 golangci-lint 的 errcheck linter 报错。

### Details
```go
// 在 SetStatusUnlessCancelled 中：
_ = b.updateStatusLocked(obj, status, err)  // 必须加 _ =
```

这与项目中 `_ = b.shared.Update(obj)` 的现有模式一致。

### Metadata
- Source: error
- Related Files: task/base_task.go
- Tags: errcheck, lint

---

## [LRN-20260619-022] correction — ModeAlwaysFail + MaxRetries 最终状态是 StatusFailedPermanent

**Logged**: 2026-06-19T23:55:00Z
**Priority**: high
**Status**: resolved
**Area**: tests

### Summary
`TestFailedCount_TypeAssertion` 断言 `StatusFailed` 但 `ModeAlwaysFail` 的下载器反复失败达到 max retries 后 Manager 调用 `MarkAsFailed`，最终状态是 `StatusFailedPermanent`。

### Details
Manager 的 `download()` 方法在失败计数超过 max retries 时调用 `ft.MarkAsFailed(obj, ...)`，MockTask 的 `MarkAsFailed` 设置状态为 `StatusFailedPermanent`。所以只失败一次的期待是不正确的 — 测试用 `ModeAlwaysFail` 且 maxRetries=5，下载器会失败 5 次达到永久失败。

测试修复：将 expected status 改为 `StatusFailedPermanent`。

### Resolution
- **Resolved**: 2026-06-19T23:55:00Z

### Metadata
- Source: error
- Related Files: manager/failed_count_test.go, manager/download.go
- Tags: testing, retry, failed_permanent

---

## [LRN-20260619-023] best_practice — 新增接口方法时的分层策略

**Logged**: 2026-06-19T23:50:00Z
**Priority**: medium
**Status**: pending
**Area**: backend

### Summary
为 BaseTask 新增 `SetStatusUnlessCancelled` 时，正确的分层策略是：
1. 在 `core/interfaces.go` 定义可选的 `TaskStatusGuard` 接口
2. 在 `task/base_task.go` 实现该方法
3. 在 `manager/resolve.go` 通过类型断言 `t.(core.TaskStatusGuard)` 调用

避免 Manager 直接 import BaseTask，保持 core 接口层的抽象边界。

### Metadata
- Source: best_practice
- Related Files: core/interfaces.go, task/base_task.go, manager/resolve.go
- Tags: architecture, interface, layering

---

## [LRN-20260619-024] best_practice — 代码审查查找模式总结

**Logged**: 2026-06-19T23:50:00Z
**Priority**: medium
**Status**: pending
**Area**: docs

### Summary
本次代码审查使用了 3 个 finder angles 发现了以下问题：

| Angle | 发现的问题 | 严重度 |
|---|---|---|
| 逐行 diff 扫描 | TOCTOU 竞争（Get+检查+Update 非原子） | Critical |
| 跨文件追踪 | `Storage()` 为 nil 时 panic | Critical |
| 跨文件追踪 | UndoCancelObject vs resolve worker 竞争（无害） | Minor |
| 跨文件追踪 | MarkResolved 后 Invalidate 的顺序问题 | Minor |
| 无用代码扫描 | `t.Storage()` nil panic 确认 | Critical |
| 无用代码扫描 | 测试完全轮询化，无 time.Sleep | ✅ |

### Metadata
- Tags: code_review, methodology

---

## [LRN-20260619-025] insight — golangci-lint errcheck 与 _ = 模式

**Logged**: 2026-06-19T23:55:00Z
**Priority**: low
**Status**: pending
**Area**: config

### Summary
`errcheck` linter 要求函数返回的 error 必须被处理。显式用 `_ =` 赋值是 Go 中抑制 errcheck 的标准做法。

项目中已有的抑制模式：
- `_ = b.shared.Update(obj)` — base_task.go
- `_ = c.Cancel(obj.URL)` — tasks.go
- `_ = t.UpdateStatus(...)` — resolve.go

### Metadata
- Tags: lint, errcheck, pattern

---

## [LRN-20260619-026] correction — Edit 工具 old_string 匹配失败：Go tab 缩进（重复发作，见 ERR-20260614-002）

**Logged**: 2026-06-19T17:50:00Z
**Priority**: high
**Status**: pending
**Area**: config

### Summary
Go 源码使用 tab 缩进，Edit 工具的 `old_string` 必须使用 tab 而非空格。当 `old_string` 用空格而文件用 tab 时，Edit 返回 "String to replace not found in file"。本会话中多次出现。

### Suggested Action
- 使用 `sed -n 'line,linep' file | cat -A` 检查目标行的实际 whitespace
- 或用 Bash `sed -i` 做替换

### Metadata
- Source: error
- Related Files: pkg/download/http_extractor.go, downloader/adapter.go, manager/download.go
- Tags: tool, edit, whitespace, go
- See Also: ERR-20260614-002, ERR-20260619-003

---

## [LRN-20260619-027] correction — map[string]bool 改为 map[string]string 后查表语法变化

**Logged**: 2026-06-19T17:51:00Z
**Priority**: low
**Status**: pending
**Area**: backend

### Summary
`mediaExtensionSet` 从 `map[string]bool` 改为 `map[string]string`（记录期望 Content-Type 前缀）时，查表表达式 `mediaExtensionSet[ext]` 的语义从 `bool` 隐式真值检查变为 `string` 隐式非空检查。两者在 Go 中都合法，但语义含义不同，需用 `!= ""` 显式表达。

### Details
```go
// Before: bool map — 检查 set membership
var mediaExtensionSet = map[string]bool{".mp4": true}
if mediaExtensionSet[ext] { ... }

// After: string map — 检查 membership AND 取期望值
var mediaExtensionSet = map[string]string{".mp4": "video/"}
if mediaExtensionSet[ext] != "" { // 必须显式 != ""
    expectedPrefix := mediaExtensionSet[ext]
}
```

### Metadata
- Source: code_review
- Related Files: pkg/download/http_extractor.go
- Tags: go, refactoring, type_change

---

## [LRN-20260619-028] correction — sync.Map.Load 直接赋值 *atomic.Int64 编译错误

**Logged**: 2026-06-19T17:52:00Z
**Priority**: high
**Status**: pending
**Area**: backend

### Summary
`sync.Map.Load` 返回 `any`，不能直接赋值给 `*atomic.Int64`。即使用了 `_, _` 忽略 ok 也不行——必须用 `v, ok := m.Load(k); counter, ok := v.(*atomic.Int64)` 模式。

最安全的 fallback 模式：
```go
v, _ := m.LoadOrStore(key, new(atomic.Int64))
counter, ok := v.(*atomic.Int64)
if !ok {
    fallback := new(atomic.Int64)
    m.Store(key, fallback)
    counter = fallback
}
count := counter.Add(1)
```

### Metadata
- Source: error
- Related Files: manager/download.go
- Tags: go, sync.Map, type_assertion, atomic, concurrency
- See Also: LRN-20260619-007

---

## [LRN-20260619-029] insight — TestScheduler_ConcurrentAccess data race 是预存问题

**Logged**: 2026-06-19T17:53:00Z
**Priority**: low
**Status**: pending
**Area**: tests

### Summary
`TestScheduler_ConcurrentAccess` 存在 Start/Stop 之间的 data race（resolve workers channel 被同时读/写），通过 `git stash` + 测试确认该 race 在当前 commit 就已经存在，与本次改动无关。写测试前应先用 stash 验证预存失败，避免被"我的改动破坏了已有测试"误导。

### Details
测试方法：
1. `git stash` 恢复干净工作树
2. 运行目标测试看是否失败
3. `git stash pop` 恢复改动
4. 如果 stash 后测试同样失败，则是预存问题

### Metadata
- Source: conversation
- Related Files: manager/manager_stress_test.go, manager/resolve.go
- Tags: testing, data_race, pre_existing

---

## [LRN-20260619-030] best_practice — ErrNoTry 双 sentinel：Check 函数必须同时检查两包

**Logged**: 2026-06-19T23:30:00Z
**Priority**: high
**Status**: promoted
**Area**: tests

### Summary
`dlcore.ErrNoTry` 和 `pkgdownload.ErrNoTry` 是不同包的独立 sentinel（虽字符串相同但指针不同），`errors.Is` 按指针比较无法跨包匹配。

### Details
`CheckError()` 最初只检查 `dlcore.ErrNoTry`，导致新路径返回 `pkgdownload.ErrNoTry` 时被误判为"不是 ErrNoTry"。
`TestFunc_MetadataFailedNotWritten` 中内联 Check 也犯了同样错误——AND 短路掩盖了 bug。

修复：修改 `CheckError()` / `CheckErrNoTry()` 同时检查两个 sentinel。`TestFunc_MetadataFailedNotWritten` 的内联 Check 也同步修复。

### Suggested Action
所有涉及 ErrNoTry 断言的 Check 函数和内联检查必须使用 `||` 组合两个 sentinel。

### Metadata
- Source: code_review
- Related Files: downloader/beacon_test.go, downloader/adapter_functional_test.go
- Tags: sentinel, error_handling, comparability
- Pattern-Key: test.dual_sentinel_check
- Recurrence-Count: 2
- First-Seen: 2026-06-18
- Last-Seen: 2026-06-19
- **Promoted**: CLAUDE.md

---

## [LRN-20260619-031] correction — DlcoreOnlyRun checks 签名必须用 Check 类型

**Logged**: 2026-06-19T23:31:00Z
**Priority**: medium
**Status**: promoted
**Area**: tests

### Summary
初版 `DlcoreOnlyRun` 的 checks 参数用了 `func(t, result)` 自定义签名，与既有 `Check` 类型 `func(t, old, new)` 不兼容，导致 Check 工厂函数无法直接传入。

### Details
代码审查发现此问题后修复为 `...Check` 类型，内部调用 `check(t, &oldResult, &newResult)`。这样既有 `CheckBothNil()`、`CheckErrNoTry()` 等工厂函数可以直接传入使测试更简洁。

### Suggested Action
扩展 Comparator 基础设施时，新函数的回调签名优先复用现有类型。

### Metadata
- Source: code_review
- Related Files: downloader/beacon_test.go
- Tags: api_design, test_infra
- **Promoted**: CLAUDE.md

---

## [LRN-20260619-032] correction — CheckBothNoTry 应组合 CheckErrNoTry 而非复制

**Logged**: 2026-06-19T23:32:00Z
**Priority**: low
**Status**: resolved
**Area**: tests

### Summary
`CheckBothNoTry` 初版完整复制了 `CheckErrNoTry` 的 `errors.Is` 断言逻辑。

### Details
代码审查指出存在代码重复。修复为组合模式：
```go
base := CheckErrNoTry()
return func(t, old, new) {
    base(t, old, new)
    // 额外断言...
}
```

### Metadata
- Source: code_review
- Related Files: downloader/beacon_test.go
- Tags: dedup, code_quality

---

## [LRN-20260619-033] insight — TestFunc_ExplicitRootDir 命名与功能不匹配

**Logged**: 2026-06-19T23:33:00Z
**Priority**: low
**Status**: resolved
**Area**: tests

### Summary
初版 `TestFunc_EmptyRootDir` 名字暗示测试"空 rootDir"行为，但实际设置了 `RootDir = t.TempDir()`。

### Details
修复为 `TestFunc_ExplicitRootDir`，注释说明"验证显式设置 RootDir 后相对路径正常解析"。

### Metadata
- Source: code_review
- Related Files: downloader/adapter_functional_test.go
- Tags: naming, test_quality

---

## [LRN-20260619-034] insight — RetryBackoff 的 callCount 僵尸变量

**Logged**: 2026-06-19T23:34:00Z
**Priority**: low
**Status**: resolved
**Area**: tests

### Summary
`TestFunc_RetryBackoff` 中 `callCount` 被 `HandleDynamic` 闭包捕获，但 `cmp.Run` 先后执行两个下载器，`callCount` 反映的是两者混合计数，无断言价值。

### Details
修复：删除 `callCount` 变量（handler 无需状态）。

### Metadata
- Source: code_review
- Related Files: downloader/adapter_functional_test.go
- Tags: dead_code, test_flaky

---

## [LRN-20260619-035] insight — TestE2E_ServerErrorRecovery 不能硬断言双方成功

**Logged**: 2026-06-19T23:35:00Z
**Priority**: high
**Status**: promoted
**Area**: tests

### Summary
dlcore 对 500 直接返回错误（不自动重试），`CheckBothNil()` 会失败。
代码审查期间尝试加了 `CheckBothNil()` 后确认不可行，还原为无断言运行。

### Details
这是已知差异：dlcore 对 `HTTP error: 500` 不包装 `ErrNoTry` 也不重试（它向上返回给调用者），而 pkg/download 的重试循环会重试直到上限。两者的行为不同，不能硬断言双方都成功。

### Metadata
- Source: code_review
- Related Files: downloader/adapter_e2e_test.go
- Tags: behavioral_diff, retry
- **Promoted**: CLAUDE.md

---

## [LRN-20260619-036] insight — TestDlcoreOnly_HuaacgURL 必须避免网络依赖

**Logged**: 2026-06-19T23:36:00Z
**Priority**: high
**Status**: resolved
**Area**: tests

### Summary
初版 `TestDlcoreOnly_HuaacgURL` 访问真实 `huaacg.com` 进行测试，代码审查指出在 CI 中不可靠。

### Details
初始方案直接用 `huaacg.com` URL 触发 dlcore 的 huaacg 特殊逻辑（5s ctx 超时）。代码审查指出：
- 网络不可靠性
- 8s 硬断言会因网络延迟而失败

修复：改回到本地 beacon + `WithMaxRetries(0)` + `.jpg` URL 路径触发 `isImageURL`。但 huaacg URL 检测仅在 URL 包含 `"huaacg.com"` 时触发，本地 beacon 方案不触发此特殊逻辑。

最终选择：保留对外部 URL 的直接调用（因为 huaacg.com 逻辑需要该域名出现在 URL 中），移除 8s 硬断言，改为仅 t.Log 记录耗时。

### Metadata
- Source: code_review
- Related Files: downloader/adapter_dlcore_only_test.go
- Tags: network_dependency, ci_stability

---

## [LRN-20260619-037] correction — golangci-lint auto-fix 可能导致堆叠错误的修正

**Logged**: 2026-06-19T23:37:00Z
**Priority**: medium
**Status**: pending
**Area**: config

### Summary
在 Windows 上 `golangci-lint run --fix` 对 `downloader/adapter_functional_test.go` 进行了不必要的修正（移除了两个 `_` import），导致后续手动 `sed` 编辑时行号计算错误。

### Details
`golangci-lint run --fix` 自动移除 `"os"` 和 `"path/filepath"` import（即使在 TestFunc_ResumeContentChanged 中实际使用了这两者），因为 lint 的依赖分析在测试文件上可能不准确。这不是 lint 的推荐用法 —— `--fix` 应该谨慎使用，尤其是对测试文件。

更好的做法是手动管理 import 列表，仅使用 `goimports` 或 `gofmt -s`。

### Suggested Action
`golangci-lint run --fix` 只在明确需要时使用，对测试文件尤其谨慎。优先用 `go fix ./...` + `go fmt ./...`。

### Metadata
- Source: error
- Related Files: downloader/adapter_functional_test.go
- Tags: golangci-lint, import_management, windows

---

## [LRN-20260620-001] correction — `filepath.Ext(rawURL)` 在含查询参数 URL 上返回错误扩展名

**Logged**: 2026-06-20T08:30:00Z
**Priority**: high
**Status**: resolved
**Area**: tests

### Summary
`filepath.Ext("https://example.com/video.mp4?token=abc")` 返回 `".mp4?token=abc"` 而非 `".mp4"`，导致 Content-Type 校验在不含查询参数时工作正常、含查询参数时静默跳过。

### Details
pkg/download 在 Content-Type 校验中使用 `filepath.Ext(rawURL)`，当 URL 含查询参数时获取到错误的扩展名（包含 `?token=abc`），`mediaExtensionSet` 查表失败，校验被绕过。测试 `TestHTTPExtractorContentTypeWithQuery` 用 text/html + .mp4 URL 验证此 bug——text/html 被错误地下载成功。

修复：先 `url.Parse(rawURL)` 再取 `filepath.Ext(parsed.Path)`。

### Resolution
- **Resolved**: 2026-06-20
- **Commit**: 1d88dc3

### Metadata
- Source: error
- Related Files: pkg/download/http_extractor.go
- Tags: url_parsing, content_type, bug
- Pattern-Key: hardened.filepath_ext_query_params

---

## [LRN-20260620-002] best_practice — 浏览器标头注入用 map 预定义 + req.Headers 覆盖

**Logged**: 2026-06-20T08:45:00Z
**Priority**: medium
**Status**: resolved
**Area**: backend

### Summary
在 `buildHeaders` 中注入 Chrome 风格浏览器标头时，用 map 预定义全部标头，再被 `req.Headers` 覆盖。比 dlcore 的 `Header.Set()` 顺序更清晰。

### Details
dlcore 的 `addBrowserLikeHeaders` 先逐个 `Set` 浏览器标头，再逐个 `Set` 自定义头。pkg/download 的 `buildHeaders` 先用 `maps.Copy(h, req.Headers)` 放自定义头，再用预定义 map 放浏览器标头（仅当 key 不存在时写入）。效果相同（自定义头覆盖浏览器头），但实现更简洁。

### Resolution
- **Resolved**: 2026-06-20
- **Commit**: d5f4b9d

### Metadata
- Source: conversation
- Related Files: pkg/download/http_extractor.go
- Tags: headers, browser_emulation, design

---

## [LRN-20260620-003] knowledge_gap — CPU 热点测试不要用 `for i < b.N`，用 `b.Loop()`

**Logged**: 2026-06-20T09:00:00Z
**Priority**: low
**Status**: pending
**Area**: tests

### Summary
本次修复过程中 Cancel 测试使用 5s SlowServer，初始版本用 for 循环 `time.Sleep(delay/1024)` 导致 httptest.Server 在连接关闭后仍阻塞。改用 `select { case <-r.Context().Done(): ... }` 解决问题。

### Details
不是直接的 `b.Loop()` 问题，而是 `time.Sleep` 循环在 context 取消后不会自动退出。慢速 server 的合理实现是阻塞在 `r.Context().Done()` 上，而非用 for 循环 + time.Sleep 模拟延迟。

### Metadata
- Source: error
- Related Files: pkg/download/http_extractor_cancel_test.go
- Tags: testing, cancellation, slow_server

---

## [LRN-20260620-004] best_practice — Cancel 实现：per-URL dlCtx + LoadAndDelete

**Logged**: 2026-06-20T09:15:00Z
**Priority**: medium
**Status**: resolved
**Area**: backend

### Summary
HTTPExtractor 的 Cancel 使用 `sync.Map` 存储 `context.CancelFunc`，在 `Extract` 入口创建 `dlCtx` 并向下传递给 tryDownload 和 transport，实现按 URL 精确取消。

### Details
关键设计点：
1. `defer dlCancel()` + `defer e.cancels.Delete(req.URL)`：确保 Extract 返回时无论如何都清理
2. `dlCtx` 代替 `ctx` 向下传播到 retry loop 和 transport.RoundTrip
3. `LoadAndDelete` 原子操作，比 dlcore 的 `Load` + 手动 `Delete` 更安全
4. 不存在 URL 时返回 nil（不报错），与 dlcore 的 error 不同

测试中 SlowServer 必须阻塞在 `r.Context().Done()` 上而不是 sleep 循环，否则 Cancel 无法及时中断 io.Copy。

### Resolution
- **Resolved**: 2026-06-20
- **Commit**: b827a43

### Metadata
- Source: conversation
- Related Files: pkg/download/http_extractor.go
- Tags: cancellation, concurrency, context
- Pattern-Key: hardened.cancel_with_dlctx

---

## [LRN-20260620-005] correction — ResponseCheck 插入位置在 Content-Type 校验后、Range 回退前

**Logged**: 2026-06-20T09:30:00Z
**Priority**: medium
**Status**: resolved
**Area**: backend

### Summary
ResponseCheck 钩子的插入点选在 Content-Type 校验之后、Range 回退逻辑之前，确保所有 HTTP 响应层面的校验都能在写文件之前拦截。

### Details
dlcore 的 tk 检测位于 Content-Type 校验之前。pkg/download 的 ResponseCheck 在 Content-Type 校验之后，主要考虑：
1. Content-Type 校验是通用逻辑，应优先于领域特定检查
2. ResponseCheck 在 Content-Type 确认有效后再检查更合理
3. 日志记录顺序：Content-Type 错误先记录，ResponseCheck 拒绝后记录

### Resolution
- **Resolved**: 2026-06-20
- **Commit**: 5cb3e5a

### Metadata
- Source: conversation
- Related Files: pkg/download/http_extractor.go
- Tags: hooks, validation, design
- Pattern-Key: design.response_check_position

---

## [LRN-20260620-006] error — sed -i 插入多行代码时 \n 转义问题

**Logged**: 2026-06-20T09:45:00Z
**Priority**: high
**Status**: resolved
**Area**: config

### Summary
用 `sed -i` 插入包含 `\n` 的 Go 代码时，`\n` 在 Git Bash 的 sed 中不被正确处理，导致代码中出现 literal newline inside string 编译错误。

### Details
执行 `sed -i '... fmt.Fprintf(..., "Response check failed: %v\n", err)'` 时，`\n` 在 Git Bash sed 中被解析为 `\` + `n` 而非换行转义。最终代码中出现 `"Response check failed: %v"` 后 literal newline 再 `", err)`，Go 编译器报 "newline in string"。

修复：只能删除损坏行后用 Edit 工具重新插入，或避免在 sed 替换字符串中包含 `\n`。

### Resolution
- **Resolved**: 2026-06-20
- **Note**: 这是重复发作的 Windows + sed 问题。Edit 工具在 Go 源码上更可靠。

### Metadata
- Source: error
- Related Files: pkg/download/http_extractor.go
- Tags: sed, windows, escaping
- Recurrence-Count: 3
- First-Seen: 2026-06-14
- Last-Seen: 2026-06-20
- See Also: ERR-20260614-002
