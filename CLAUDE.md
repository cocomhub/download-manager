# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 构建与运行

```bash
make build           # 本地构建（含格式化）
make build-ci        # CI 构建（跳过格式化）
make test            # 快速单元测试
make test-cover      # 测试 + 覆盖率收集
make test-no-mongo   # 不带 MongoDB 的测试
make cover-check     # 覆盖率门禁检查（默认 20%）
make vet             # go vet
make lint            # golangci-lint
make bench           # 基准测试（-count=5）
make check-loopback  # 检查测试地址是否使用不安全监听
make notest          # 检查所有包有测试文件（.notestignore 控制免检）
make gofix           # go fix ./...
make fmt             # go fix + addlicense + gofmt
make check-ci        # 全量检查入口（提交前使用）
make clean           # 清理产物
make run             # build + 用 build/config.yaml 运行
make all             # vet + test + bench

Windows 首次运行需安装 make：
  pwsh scripts/install-make.ps1

所有 CI job 通过 `make <target>` 调用，不写裸 go 命令。
```

测试：`go test ./...` 或 `go test -v -run TestXxx ./path/...`

并发测试：`go test -race -count=1 -timeout=180s ./...`

增量覆盖（2026-06-15）：
- Phase 1（并发安全）：8 个测试，覆盖 shutdown/activeDownloads race/类型断言/配置热加载等
- Phase 2（API）：5 个测试，覆盖分页边界/错误码/写保护/聚合配额
- Phase 3（Task）：14 个测试，覆盖 urllist 重复文件名/BaseTask 状态管理
- Phase 4（Storage）：10 个测试，覆盖多过滤器/并发/Flush-Recovery/损坏文件等
- Phase 5（Playwright）：10 个测试，覆盖网络节流/a11y 审计/跨浏览器布局

## 关键陷阱（2026-06-16 积累）

### sync.Map 类型断言
`LoadOrStore` / `Load` 返回的 `any` 必须用 `ok` 模式检查类型，直接 `v.(*atomic.Int64)` 会在值被覆盖时 panic。
应始终：`v, ok := m.failedCount.LoadOrStore(k, new(atomic.Int64)); counter, ok := v.(*atomic.Int64)`

### Config 指针竞争
`mgr.currentCfg()` 返回内部 `*config.Config` 指针。**绝对不能直接修改返回的指针字段**，必须先浅拷贝：`cfgCopy := *newCfg`。
`UpdateConfig` 内部也必须拷贝输入参数，避免 `ValidateAndClamp` 修改调用者的配置。

### adjustGlobalWorkers 加锁
`m.workerCount` 在 `Start()` 和 `adjustGlobalWorkers` 之间共享，后者必须在函数入口持有 `m.mu`。

### 文件编码
PowerShell 写入 Go 源码文件会默认使用 UTF-16 LE + BOM，导致 git 显示全文件变更。优先使用 bash `sed` 或 `Edit` 工具。
修改前后运行 `go fmt` 可能导致 `Edit` 的 `old_string` 不匹配——先 `go fmt` 再读文件确认内容。

### Playwright SSE 测试
- `addInitScript` 注册的 patch 在 **新导航** 时执行，必须在 `page.goto()` 前调用
- 拦截 SSE 端点使用 `page.context().route('**/api/events', route => route.abort())`

### 数据竞争保护模式
涉及 `m.downloader` 等被多个 goroutine 读写的字段，使用**专用锁 + getter/setter 封装**：
- 不要复用 `m.mu`（它保护 `activeDownloads`，在 download 热路径中频繁使用）
- 使用独立的 `downloaderMu sync.Mutex` + `getDownloader()` / `setDownloader()` 方法
- 测试代码也必须通过 setter 写入，不能直接字段赋值

### Playwright 纯文字截图快照
`h1`、`h2` 等纯文字元素在不同 OS 字体渲染下产生宽高偏差（Ubuntu CI 54px vs 本地 62px），pixel comparison 不适合。
**优先用文本断言**：`toBeVisible()` + `toHaveText('expected')`，仅在结构性元素上保留 `toHaveScreenshot`。

### Playwright `route()` 路由拦截注意
`page.context().route('**/api/**', handler)` 中如果 handler 有 `await sleep(n)` 延时：
- 第一条请求到达时路由被"占用"，第二条同路由请求到达会报 `Route is already handled!`
- 解决：加 `routeHandled` guard 只拦截首条，或跳过 health check 保心跳

### Manager worker() 空闲心跳
worker() 在 `downloadQueue` 通道无消息时处于 select 阻塞状态，不更新心跳。
health check 在 5s 超时内未收到心跳 → workers 组件 503。
**修复**：`time.NewTicker(3s)` 定时刷新心跳，不论是否有下载任务。

### CI Benchmark step 上游失败
`benchmark-action/github-action-benchmark` 在 `gh-pages` 分支不存在时 `git fetch` 失败 → 中断 job。
**修复**：`continue-on-error: true`（不影响测试结果，仅基准报告无法推送）。

### TestE2E_MixedResults 随机概率不稳定
`fail_rate=0.5` 时 10 个 objects 全部成功的概率为 `0.5^10 ≈ 0.1%`，在 CI 多平台运行时偶发。
**修复**：`fail_rate=0.4`，将极端概率降低 10 倍（`0.4^10 ≈ 0.001%`）。

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
- golangci-lint v2 配置格式（.golangci.yml 中 `version: "2"`）

## 关键陷阱

### sync.Map 类型断言
`LoadOrStore` / `Load` 返回的 `any` 必须用 `ok` 模式检查类型，直接 `v.(*atomic.Int64)` 会在值被覆盖时 panic。
应始终：`v, ok := m.failedCount.LoadOrStore(k, new(atomic.Int64)); counter, ok := v.(*atomic.Int64)`

### Config 指针竞争
`mgr.currentCfg()` 返回内部 `*config.Config` 指针。**绝对不能直接修改返回的指针字段**，必须先浅拷贝：`cfgCopy := *newCfg`。
`UpdateConfig` 内部也必须拷贝输入参数，避免 `ValidateAndClamp` 修改调用者的配置。

### Config 深拷贝
始终使用 `cfg.Clone()` 而非手写 `make+copy`。`Clone()` 已处理所有 map/slice 字段（Tasks、Contexts、Proxies、DomainLimits、FFmpeg.ExtraArgs、Mongo）。不要用 `make+copy` 覆盖 `Clone()` 的结果——这会丢失深拷贝。

### Manager.Start 无限循环中的同步
`Start()` 末尾是 `for { select {} }`，defer 永远不会执行。`close(initializedCh)` 必须在 for 循环之前直接调用，不能使用 defer。

### 文件编码
PowerShell 写入 Go 源码文件会默认使用 UTF-16 LE + BOM，导致 git 显示全文件变更。优先使用 bash `sed` 或 `Edit` 工具。
修改前后运行 `go fmt` 可能导致 `Edit` 的 `old_string` 不匹配——先 `go fmt` 再读文件确认内容。

## 执行偏好

- **子代理开发**：多步骤实现计划优先使用 `subagent-driven-development` 技能，禁用 worktree，直接在当前分支开发。
- **worktree**：除非用户明确要求，不使用 git worktree。

## Playwright E2E 测试

浏览器 UI 自动化测试，覆盖 14 个核心场景。测试目录 `test/playwright/`（TypeScript），
测试服务端 `cmd/playwright-server/`（Go，独立 go.mod，不污染主包）。

```bash
make playwright-test       # 全部 E2E 测试（CI 模式）
make playwright-ui         # Playwright UI 交互模式（AI 辅助调试）
make playwright-codegen    # 启动代码生成器，可录制 AI 操作
```

关键文件：
- `test/playwright/helpers/server.ts` — Go server 子进程管理
- `test/playwright/helpers/api.ts` — REST API 封装
- `test/playwright/helpers/sse.ts` — SSE 事件拦截辅助
- `test/playwright/specs/` — 14 个测试场景
- `cmd/playwright-server/fixture/` — 测试数据集（4 个预置任务）

## AI 交互式测试

```bash
make playwright-codegen    # 启动 Codegen 录制（AI 操作 -> 自动生成测试）
```

Playwright Codegen 与 Chrome DevTools MCP 结合，AI 可以用自然语言描述操作步骤，
系统自动录制为测试脚本。关键 `data-testid` 锚点见 `test/playwright/CODEGEN.md`。

设计文档：`docs/superpowers/specs/2026-06-14-browser-e2e-testing-design.md`

### Playwright 测试经验规则

1. **定位器优先级**：文本属性（`getByText`）→ `data-testid`（必须唯一）→ CSS class（最后选择）
2. **断言必须可失败**：避免 `toBeGreaterThanOrEqual(0)` 等永真断言，每个断言应能真实检测回归
3. **SSE 测试**：`addInitScript` 必须在 `page.goto()` 前注册，否则无法拦截 EventSource
4. **端口参数化**：全部使用 `TEST_PORT` 环境变量，禁止硬编码 `localhost:19199`
5. **视觉回归**：动态元素加 `mask` 排除，截图文件名全局唯一不冲突
6. **fixture 与实际测试匹配**：确保描述的场景与实际加载的数据集一致
7. **报告脚本**：路径要考虑 CI 中 `working-directory` 可能改变当前目录
8. **截图快照跨平台**：`snapshotPathTemplate` 使用 `{projectName}` 而非 `{platform}`，避免 win32/linux 后缀不匹配
9. **go:embed 修改后需重新构建**：改 `web/static/` 文件后需 `cd cmd/playwright-server && go build -o playwright-server .` 才能让测试使用新内容
10. **axe-core 对比度调试**：遍历 `v.nodes[i].html` + `v.nodes[i].target` 定位颜色违规元素
11. **按钮对比度标准**：`bg-white/text-blue-500`（2.8:1）不达标，需改为 `bg-blue-600/text-white`（4.6:1+）

<!-- superpowers-zh:begin (do not edit between these markers) -->
# Superpowers-ZH 中文增强版

本项目已安装 superpowers-zh 技能框架（20 个 skills）。

## 核心规则

1. **收到任务时，先检查是否有匹配的 skill** — 哪怕只有 1% 的可能性也要检查
2. **设计先于编码** — 收到功能需求时，先用 brainstorming skill 做需求分析
3. **测试先于实现** — 写代码前先写测试（TDD）
4. **验证先于完成** — 声称完成前必须运行验证命令

## 可用 Skills

Skills 位于 `.claude/skills/` 目录，每个 skill 有独立的 `SKILL.md` 文件。

- **brainstorming**: 在任何创造性工作之前必须使用此技能——创建功能、构建组件、添加功能或修改行为。在实现之前先探索用户意图、需求和设计。
- **chinese-code-review**: 中文 review 沟通参考——话术模板、分级标注（必须修复/建议修改/仅供参考）、国内团队常见反模式应对。仅在用户显式 /chinese-code-review 时调用，不要根据上下文自动触发。
- **chinese-commit-conventions**: 中文 commit 与 changelog 配置参考——Conventional Commits 中文适配、commitlint/husky/commitizen 中文模板、conventional-changelog 中文配置。仅在用户显式 /chinese-commit-conventions 时调用，不要根据上下文自动触发。
- **chinese-documentation**: 中文文档排版参考——中英文空格、全半角标点、术语保留、链接格式、中文文案排版指北约定。仅在用户显式 /chinese-documentation 时调用，不要根据上下文自动触发。
- **chinese-git-workflow**: 国内 Git 平台配置参考——Gitee、Coding.net、极狐 GitLab、CNB 的 SSH/HTTPS/凭据/CI 接入差异与镜像同步配置。仅在用户显式 /chinese-git-workflow 时调用，不要根据上下文自动触发。
- **dispatching-parallel-agents**: 当面对 2 个以上可以独立进行、无共享状态或顺序依赖的任务时使用
- **executing-plans**: 当你有一份书面实现计划需要在单独的会话中执行，并设有审查检查点时使用
- **finishing-a-development-branch**: 当实现完成、所有测试通过、需要决定如何集成工作时使用——通过提供合并、PR 或清理等结构化选项来引导开发工作的收尾
- **mcp-builder**: MCP 服务器构建方法论 — 系统化构建生产级 MCP 工具，让 AI 助手连接外部能力
- **receiving-code-review**: 收到代码审查反馈后、实施建议之前使用，尤其当反馈不明确或技术上有疑问时——需要技术严谨性和验证，而非敷衍附和或盲目执行
- **requesting-code-review**: 完成任务、实现重要功能或合并前使用，用于验证工作成果是否符合要求
- **subagent-driven-development**: 当在当前会话中执行包含独立任务的实现计划时使用
- **systematic-debugging**: 遇到任何 bug、测试失败或异常行为时使用，在提出修复方案之前执行
- **test-driven-development**: 在实现任何功能或修复 bug 时使用，在编写实现代码之前
- **using-git-worktrees**: 当需要开始与当前工作区隔离的功能开发，或在执行实现计划之前使用——通过原生工具或 git worktree 回退机制确保隔离工作区存在
- **using-superpowers**: 在开始任何对话时使用——确立如何查找和使用技能，要求在任何响应（包括澄清性问题）之前调用 Skill 工具
- **verification-before-completion**: 在宣称工作完成、已修复或测试通过之前使用，在提交或创建 PR 之前——必须运行验证命令并确认输出后才能声称成功；始终用证据支撑断言
- **workflow-runner**: 在 Claude Code / OpenClaw / Cursor 中直接运行 agency-orchestrator YAML 工作流——无需 API key，使用当前会话的 LLM 作为执行引擎。当用户提供 .yaml 工作流文件或要求多角色协作完成任务时触发。
- **writing-plans**: 当你有规格说明或需求用于多步骤任务时使用，在动手写代码之前
- **writing-skills**: 当创建新技能、编辑现有技能或在部署前验证技能是否有效时使用

## 如何使用

当任务匹配某个 skill 时，使用 `Skill` 工具加载对应 skill 并严格遵循其流程。绝不要用 Read 工具读取 SKILL.md 文件。

如果你认为哪怕只有 1% 的可能性某个 skill 适用于你正在做的事情，你必须调用该 skill 检查。
<!-- superpowers-zh:end -->
