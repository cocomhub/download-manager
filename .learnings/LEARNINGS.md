# Learnings

Corrections, insights, and knowledge gaps captured during development.

**Categories**: correction | insight | knowledge_gap | best_practice

---

## [LRN-20260613-001] best_practice — Playwright 定位器选择策略

**Logged**: 2026-06-13T15:00:00Z
**Priority**: high
**Status**: promoted
**Area**: tests

### Summary
测试定位器应遵循 "文本优先 → data-testid 备用 → CSS class 最后" 的层级，且 data-testid 必须唯一。

### Details
初始实现中使用 CSS class（如 `.grid`）、文本内容（如 `button:has-text("List")`）和 data-testid 混用导致测试脆弱：
- CSS class 在响应式适配中变更（`grid-cols-1` → `grid-cols-2`）
- 带文本的定位器在 UI 重构中改名（"List" → 纯图标无文字）
- `data-testid="search-input"` 在不同视图下重复（下载列表 + 聚合视图）

### Suggested Action
- 功能交互点必须使用 `data-testid`（P0 级）
- `data-testid` 在整个页面中必须唯一（加后缀区分视图）
- 验证性断言使用 `toContainText()` 而非 CSS class 可见性
- 详见 `CODEGEN.md`

### Metadata
- Source: conversation
- Related Files: test/playwright/specs/*.ts, web/static/index.html
- Tags: playwright, selectors, data-testid

---

## [LRN-20260613-002] best_practice — 共享 Server 下的测试隔离

**Logged**: 2026-06-13T15:30:00Z
**Priority**: high
**Status**: promoted
**Area**: tests

### Summary
多个测试共享同一个 Go 测试 Server 时，写操作（cancel/retry）与读操作（列表渲染）并行执行导致竞态。`fullyParallel: true` 加剧此问题。

### Details
T4（取消对象）、T6（API cancel）、T5（批量操作）修改 server 状态，与验证任务列表的 T1/T2 并行时，读测试看到不一致的 cancelled 状态导致间歇性失败。

### Suggested Action
- 读测试（验证 UI 渲染）和写测试（变更状态）应隔离运行
- 在 `playwright.config.ts` 中设 `fullyParallel: false`
- 或将可写测试串行化（`test.describe.serial`）
- 使用 test-tktube（稳定 all-completed）而非 test-mixed（动态状态）作为读测试的基准

### Metadata
- Source: conversation
- Related Files: test/playwright/playwright.config.ts
- Tags: testing, parallel, race-condition

---

## [LRN-20260613-003] best_practice — 视觉回归快照的稳定性

**Logged**: 2026-06-13T16:00:00Z
**Priority**: high
**Status**: promoted
**Area**: tests

### Summary
动态元素（连接状态指示器、加载动画、任务进度）导致截图基线不稳定。截图命名冲突导致基线覆盖。

### Details
- `status-indicator`（绿点/红点）每次运行可能不同 → `mask` 排除
- `animate-spin` 加载动画不同步 → `mask` 排除
- V1 和 V3 使用了相同的 `heading.png` 文件名 → 后者覆盖前者

### Suggested Action
- `toHaveScreenshot()` 的 `mask` 选项排除所有动态元素
- 每个截图使用唯一的文件名（不要复用）
- 截图区域选择稳定的 DOM 节点（如 h1 标题文字而非整个 sidebar）
- `.gitignore` 排除 `visual-regression.spec.ts-snapshots/`（CI 自动生成）

### Metadata
- Source: conversation
- Related Files: test/playwright/specs/visual-regression.spec.ts
- Tags: visual-regression, snapshots, stability

---

## [LRN-20260613-004] correction — 端口参数化必须彻底

**Logged**: 2026-06-13T16:30:00Z
**Priority**: critical
**Status**: promoted
**Area**: tests

### Summary
`fault-injection.spec.ts` 中 R3 使用了硬编码 `localhost:19199`，导致 `TEST_PORT` 环境变量被忽略。

### Details
api.ts 导出了 `TEST_PORT`，但 R3 使用了裸的 `fetch('http://localhost:19199/...')`。这是第一次 review 遗漏的。修复方案：从 api.ts 导入 `TEST_PORT` 并使用模板字符串拼接。

### Suggested Action
- 全局搜索 `localhost:19199` 确保全部替换为参数化端口
- ESLint 规则：禁止硬编码端口
- 所有 HTTP 请求必须通过 helpers/api.ts 或使用 TEST_PORT

### Metadata
- Source: conversation
- Related Files: test/playwright/specs/fault-injection.spec.ts, test/playwright/helpers/api.ts
- Tags: port, parameterization, eslint

---

## [LRN-20260613-005] knowledge_gap — SSE 事件捕获时机

**Logged**: 2026-06-13T17:00:00Z
**Priority**: high
**Status**: pending
**Area**: tests

### Summary
SSE（EventSource）在 `page.goto()` 导航时立即建立连接，`addInitScript` 的 patch 必须在 `goto()` 之前执行才能拦截到事件。

### Details
SSEWatcher 使用 `page.addInitScript()` 注入 `EventSource.prototype.addEventListener` 补丁。但：
- `addInitScript` 注册的脚本在 **新导航** 时执行（在 `window.onload` 之前）
- `page.goto('/')` 之后调用 `watcher.attach()` 不会影响已建立的 EventSource
- 正确顺序：`watcher.attach()` → `page.goto('/')`

当前 T6 测试因此无法通过 SSEWatcher 捕获事件，改为验证 API 调用的间接效果。

### Suggested Action
- SSEWatcher 的 `attach()` 必须在 `page.goto()` 之前调用
- 或使用 `page.addInitScript(() => { ... patch EventSource ... })` 在全局 setup 中注册
- 后续可改用 `page.waitForResponse()` 监听 SSE endpoint 的响应流

### Metadata
- Source: conversation
- Related Files: test/playwright/helpers/sse.ts
- Tags: SSE, EventSource, timing, addInitScript

---

## [LRN-20260613-006] correction — 断言必须真实验证

**Logged**: 2026-06-13T17:30:00Z
**Priority**: high
**Status**: promoted
**Area**: tests

### Summary
Lighthouse L2 的 `expect(altCount).toBeGreaterThanOrEqual(0)` 和 L3 的 `expect(count).toBeGreaterThanOrEqual(0)` 是永真断言，从不失败，无法检测回归。

### Details
- `altCount` 初始化为 0，loop 中只增不减，所以 `>= 0` 永远 true
- `viewportMeta.count()` 返回 `>= 0`，`>= 0` 始终满足

### Suggested Action
- 验证性断言必须能失败：统计 `withoutAlt` 并 `expect(withoutAlt).toBe(0)`
- Viewport meta 必须存在：`expect(count).toBe(1)` + 验证 content 属性
- 代码审查时检查所有 `toBeGreaterThanOrEqual(0)` 模式

### Metadata
- Source: conversation
- Related Files: test/playwright/specs/lighthouse.spec.ts
- Tags: assertions, validation, tautology

---

## [LRN-20260613-007] best_practice — 报告脚本的路径可靠性

**Logged**: 2026-06-13T18:00:00Z
**Priority**: high
**Status**: promoted
**Area**: infra

### Summary
`scripts/playwright-report-gen.sh` 初始硬编码 `test/playwright/test-results/.last-run.json`，当 CI 中 `working-directory` 改变后路径解析错误。

### Details
CI 配置 `working-directory: test/playwright` 改变了当前目录，但脚本中的相对路径假设从仓库根运行。导致：
- `.last-run.json` 找不到 → 统计静默跳过
- `test/playwright/playwright-report/` 在嵌套目录 `test/playwright/test/playwright/` 下创建

### Suggested Action
- 脚本使用 `${RESULTS_DIR}` 变量而非硬编码路径
- 在 CI 中明确传递 `test-results`（相对路径）而非硬编码
- 所有 shell 脚本考虑 `working-directory` 对路径的影响

### Metadata
- Source: conversation
- Related Files: scripts/playwright-report-gen.sh, .github/workflows/ci.yml
- Tags: CI, paths, shell-script

---

## [LRN-20260613-008] correction — fixture 加载与实际验证的偏差

**Logged**: 2026-06-13T18:30:00Z
**Priority**: medium
**Status**: pending
**Area**: tests

### Summary
`fixture-stress.spec.ts` 中的 F1-F5 测试描述上声称测试"大数据"、"压力"、"分组"等场景，但实际 `globalSetup` 始终加载 `--fixture "full"`，专用数据集从未被使用。

### Details
- F1 注释 "requires --fixture large-task" → 实际跳过
- F4 声称 "empty task edge case" → 实际验证 "full" 数据集
- F5 声称 "content_group" → 实际验证 "full" 数据集
- 只有 F2 使用了 test-mixed（"full" 中的默认任务）

### Suggested Action
- 方案 A：修改 `globalSetup` 或 fixture loader 支持按测试动态切换 fixture
- 方案 B：重写测试描述，与实际使用的 "full" 数据集匹配
- 或创建独立的 Playwright project 使用不同的 `--fixture` 参数

### Metadata
- Source: conversation
- Related Files: test/playwright/specs/fixture-stress.spec.ts, cmd/playwright-server/fixture/datasets.go
- Tags: fixture, test-data, coverage-gap

---

## [LRN-20260615-009] best_practice — sync.Map 类型断言必须安全

**Logged**: 2026-06-15T03:20:00Z
**Priority**: critical
**Status**: resolved
**Area**: backend

### Summary
从 `sync.Map.LoadOrStore` / `Load` 返回的 `any` 直接断言为具体类型（`v.(*atomic.Int64)`），若值被意外覆盖则导致 panic。

### Details
`manager/download.go` 中 `m.failedCount` 存储 `*atomic.Int64`，但原始代码直接断言：
```go
c := v.(*atomic.Int64).Add(1)
```
如果 map 中某个 key 被意外写入了其他类型（如 string），整个进程 panic。修复方案：
1. 使用 `ok` 模式检查类型，失败时重新 Store
2. 后备路径中连 `Load` 也返回错误类型时，创建全新 fallback counter

### Resolution
- **Resolved**: 2026-06-15T03:20:00Z
- **Commit**: 8d5b038
- **Notes**: download.go 两层防御，确保永不 panic

### Metadata
- Source: error
- Related Files: manager/download.go
- Tags: concurrency, type-assertion, sync.Map, panic

---

## [LRN-20260615-016] best_practice — Windows HTTP 监听必须用 127.0.0.1

**Logged**: 2026-06-15T03:30:00Z
**Priority**: high
**Status**: pending
**Area**: infra

### Summary
Windows 上 `0.0.0.0` 和 `localhost`（IPv6 `::1`）触发 Defender 防火墙弹窗。所有 Go HTTP 服务器必须用 `127.0.0.1`。

### Details
- `httptest.NewServer` 默认监听 `127.0.0.1`，无需修改
- 自定义 `http.ListenAndServe` 用 `net.Listen("tcp", "127.0.0.1:0")` 替代
- 开发配置中写 `http_port: 127.0.0.1:8080` 而非 `:8080`

### Metadata
- Source: conversation
