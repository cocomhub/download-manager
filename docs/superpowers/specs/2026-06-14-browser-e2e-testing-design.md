# Browser E2E 测试体系设计

> 2026-06-14

## 背景与目标

download-manager 项目当前有 250+ Go 单元测试，覆盖 Manager 逻辑层、HTTP API 层、下载器核心层，但**浏览器 UI 层测试完全空白**。

目标：
1. 建立 Playwright 浏览器自动化测试体系，覆盖 14 个核心 UI 场景
2. 保持 Go module 零污染（独立 go.mod）
3. AI 友好：测试代码支持 AI (Claude) 通过 Playwright MCP 交互式调试
4. 为后续移动端适配预留测试能力（标记为 TODO）

## 架构总览

```
root/
├── cmd/playwright-server/      ← Go 测试服务端（独立 go.mod）
│   ├── main.go                 ← 启动 Manager + fixture 加载
│   ├── go.mod
│   └── fixture/
│       ├── loader.go           ← LoadFixture(mgr, name)
│       └── datasets.go         ← 预定义数据集
│
├── test/playwright/            ← Playwright 测试（TypeScript）
│   ├── package.json
│   ├── playwright.config.ts    ← desktop project（mobile TODO）
│   ├── tsconfig.json
│   ├── helpers/
│   │   ├── server.ts           ← Go 子进程管理
│   │   ├── api.ts              ← REST API 封装
│   │   └── sse.ts              ← SSE 事件拦截辅助
│   └── specs/
│       ├── task-lifecycle.spec.ts
│       ├── aggregate-view.spec.ts
│       ├── dashboard.spec.ts
│       ├── config-management.spec.ts
│       ├── realtime-updates.spec.ts
│       └── error-boundary.spec.ts
│
├── Makefile                    ← 新增 playwright-* 目标
├── .gitignore                  ← 新增 Playwright 忽略规则
└── .github/workflows/ci.yml   ← 新增 playwright job（并行）
```

## 14 个测试场景

### 一、任务对象管理（5 场景）

| # | 场景 | 关键验证点 |
|---|---|---|
| 1 | 任务列表渲染 | 侧边栏展示所有任务，名称/类型/进度正确，连接状态指示 |
| 2 | 对象网格浏览 | 选择任务 → 对象卡片网格展示，含封面/状态/进度/操作按钮 |
| 3 | 对象列表视图 | 切换列表模式，列标题正确，排序/搜索/分页工作 |
| 4 | 取消/恢复对象 | 单个取消 → 状态变 cancelled → 恢复 → 状态变 pending |
| 5 | 批量操作 | 多选 → 批量取消 → 批量恢复，状态批量变更 |

### 二、实时更新（2 场景）

| # | 场景 | 关键验证点 |
|---|---|---|
| 6 | SSE 推送渲染 | 通过 API 触发对象状态变更，UI 自动更新，无需手动刷新 |
| 7 | 进度批量更新 | 批量进度推送，进度条平滑变化，重新连接后重新拉取 |

### 三、聚合视图（2 场景）

| # | 场景 | 关键验证点 |
|---|---|---|
| 8 | 聚合搜索与过滤 | 跨任务对象搜索，按状态/类型过滤，分页 |
| 9 | 内容分组 | group_by=content 模式，同组对象聚合展示，展开组查看成员 |

### 四、仪表盘（2 场景）

| # | 场景 | 关键验证点 |
|---|---|---|
| 10 | 系统健康 | 各组件健康状态显示，断路/正常切换 |
| 11 | 指标与故障记录 | 全局指标卡片，每任务指标表，故障记录列表 |

### 五、配置管理（2 场景）

| # | 场景 | 关键验证点 |
|---|---|---|
| 12 | 配置查看与 Diff | 查看当前配置，历史版本列表，Diff 对比展示 |
| 13 | 配置热加载 | 修改配置 → 应用 → 页面反映新配置 |

### 六、UI-only 模式守卫（1 场景）

| # | 场景 | 关键验证点 |
|---|---|---|
| 14 | 只读模式 | UI-only 模式启动，所有操作按钮禁用/灰化，提示写禁用 |

### TODO：移动端适配（2 场景，本周期不实现）

| # | 场景 | 原因 |
|---|---|---|
| 15 | 移动端浏览 | UI 尚未做移动端适配 |
| 16 | 移动端操作 | 同上 |

## 测试数据工厂

### 预置任务

Go 端 fixture loader（`cmd/playwright-server/fixture/`）内置 4 个预置任务：

| 任务 ID | 类型 | 对象数 | 用途 |
|---|---|---|---|
| `test-tktube` | tktube | 15 | 网格/列表/搜索/排序/分页/批量操作 |
| `test-vikacg` | vikacg | 8 | 多任务聚合/内容分组 |
| `test-hanime` | hanime | 6 | 跨任务搜索 |
| `test-mixed` | urllist | 12 | 多样化状态组合（5 completed + 3 failed + 2 downloading + 2 pending） |

### 对象状态分布

```
test-tktube (15):
  - 6 completed（含 2 个同 content_group 的不同变体 → 分组测试）
  - 3 failed
  - 3 downloading（模拟进度 45%, 72%, 88%）
  - 2 pending
  - 1 cancelled
```

### 数据注入方式

双模式：

| 方式 | 用途 | 实现 |
|---|---|---|
| Go fixture loader | 测试前加载完整数据集 | `testutil.LoadFixture(mgr, "test-tktube")` 编译进 test binary |
| REST API | Playwright 侧动态调整 | `POST /api/tasks`、`PATCH /api/tasks/{id}/runtime` |

## Go Server 测试入口

### 代码位置

`cmd/playwright-server/main.go`，独立 `go.mod`：

```go
// cmd/playwright-server/go.mod
module github.com/cocomhub/download-manager/cmd/playwright-server
go 1.26
require github.com/cocomhub/download-manager v0.0.0
replace github.com/cocomhub/download-manager => ../..
```

### 启动流程

1. 解析 `--port` 和 `--fixture` 参数
2. 创建内存存储的 Manager
3. 加载 fixture 数据集
4. 启动 HTTP API Server
5. 等待 SIGTERM

### Playwright Server Helper

```typescript
// test/playwright/helpers/server.ts
export class TestServer {
  async start(fixture: string) { /* spawn + waitForHealthz */ }
  async stop() { /* SIGTERM + wait exit */ }
}
```

playwright.config.ts 使用 `globalSetup` / `globalTeardown` 管理。

## AI 与浏览器互操作

### data-testid 策略

P0（必须添加，共约 10 处修改）：

| data-testid | Vue 模板位置 | 说明 |
|---|---|---|
| `sidebar` | 侧边栏容器 `<div>` | 侧边栏根元素 |
| `task-{task.id}` | `v-for="task in filteredTasks"` 中的 `<div>` | 每任务项 |
| `object-{obj.url}` | 对象卡片/行 `<div>` | 每对象项 |
| `btn-cancel` / `btn-retry` / `btn-undo` | 操作按钮 | 关键操作 |
| `pagination` | 分页控件容器 | 分页区域 |

P1（建议添加，可后续补充）：

| data-testid | 位置 | 说明 |
|---|---|---|
| `view-mode-downloads` / `aggregate` / `dashboard` | 视图切换按钮 | 视图模式 |
| `search-input` | 搜索输入框 | 搜索框 |
| `modal-{name}` | 模态框容器 | 模态框 |

### SSE 辅助验证

```typescript
// test/playwright/helpers/sse.ts - 拦截 EventSource 消息用于验证
```

### AI 交互原则

| 原则 | 说明 |
|---|---|
| 文本优先 | `getByText` / `getByRole('button', {name: '取消'})` |
| testid 备用 | 文本不唯一或动态内容时回退 |
| 截图辅助 | 每个断言点补充截图 |
| 错误恢复 | 失败时截图 + DOM 快照 + 控制台日志 |

## .gitignore 规则

```gitignore
# Playwright test artifacts
test/playwright/node_modules/
test/playwright/playwright-report/
test/playwright/test-results/
test/playwright/artifacts/
test/playwright/.cache/

# Playwright test server binary
cmd/playwright-server/playwright-server
cmd/playwright-server/playwright-server.exe
```

## CI 集成

### 新增 job（与 go test 并行）

```yaml
playwright:
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-go@v5
      with:
        go-version: "1.26"
        cache-dependency-path: |
          go.sum
          cmd/playwright-server/go.sum
    - name: Build test server
      working-directory: cmd/playwright-server
      run: go build -o playwright-server .
    - uses: actions/setup-node@v4
      with:
        node-version: "22"
        cache: "npm"
        cache-dependency-path: test/playwright/package-lock.json
    - name: Install dependencies
      working-directory: test/playwright
      run: npm ci
    - name: Install Playwright browsers
      working-directory: test/playwright
      run: npx playwright install chromium
    - name: Run Playwright tests
      working-directory: test/playwright
      env:
        SERVER_BINARY: ../../cmd/playwright-server/playwright-server
        TEST_PORT: 19199
      run: npx playwright test
    - uses: actions/upload-artifact@v4
      if: failure()
      with:
        name: playwright-artifacts
        path: |
          test/playwright/test-results/
          test/playwright/artifacts/
        retention-days: 7
```

## Makefile 命令（平台兼容）

```makefile
# Playwright E2E 测试
.PHONY: playwright-server playwright-test playwright-ci

playwright-server:  ## 编译 Playwright 测试用 Go server
	cd cmd/playwright-server && go build -o playwright-server .

playwright-test: playwright-server  ## 运行 Playwright E2E 测试（开发模式）
	cd test/playwright && npx playwright test

playwright-ui: playwright-server  ## 运行 Playwright UI 交互模式（AI 辅助调试）
	cd test/playwright && npx playwright test --ui

playwright-report:  ## 查看 Playwright 测试报告
	cd test/playwright && npx playwright show-report

playwright-codegen: playwright-server  ## 启动 Playwright 代码生成器（记录 AI 操作）
	cd test/playwright && SERVER_BINARY=../../cmd/playwright-server/playwright-server \
		TEST_PORT=19199 npx playwright codegen http://localhost:19199
```

## CLAUDE.md 记录

更新 `download-manager/CLAUDE.md` 追加 Playwright 相关说明：

```markdown
## Playwright E2E 测试

测试目录 `test/playwright/`（TypeScript），测试服务端 `cmd/playwright-server/`（Go，独立 go.mod）。

```bash
make playwright-test       # 全部 E2E 测试
make playwright-ui         # Playwright UI 交互模式
make playwright-codegen    # 代码生成器，记录 AI 操作
```

关键文件：
- `test/playwright/helpers/server.ts` — Go server 子进程管理
- `test/playwright/helpers/api.ts` — REST API 封装
- `test/playwright/helpers/sse.ts` — SSE 事件拦截
- `cmd/playwright-server/fixture/` — 测试数据集

设计文档: `docs/superpowers/specs/2026-06-14-browser-e2e-testing-design.md`
```

## 平台兼容性

| 平台 | 支持 | 说明 |
|---|---|---|
| Linux (CI) | ✅ | Ubuntu latest，Chromium |
| macOS (开发) | ✅ | `playwright install chromium` |
| Windows (开发) | ✅ | `npx playwright.cmd`，PowerShell 中 `$env:SERVER_BINARY` |
| CI 并行 | ✅ | 与 `go test` job 互不依赖 |
| 端口 | 固定 19199 | 环境变量 `TEST_PORT` 可覆盖 |

### Windows 注意事项

PowerShell 环境变量语法与 bash 不同，使用 `cross-env` 或 dotenv 统一：

```bash
cd test/playwright
cross-env SERVER_BINARY=../../cmd/playwright-server/playwright-server.exe TEST_PORT=19199 npx playwright test
```

## 规格自检

- [x] 占位符扫描：无 TODO 未完成章节（移动端适配已显式标记为 TODO）
- [x] 内部一致性：架构描述与文件路径一致，CI 流程与开发流程一致
- [x] 范围检查：聚焦浏览器 UI 测试，不涉及 Manager 逻辑改造
- [x] 模糊性检查：data-testid 策略、数据工厂、CI 步骤均已明确定义
