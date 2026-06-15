# 全面重构升级设计规格

## 概述

对 download-manager 项目进行系统性重构升级，涵盖架构拆分、技术债务清理、测试覆盖补全、性能基准、CI/CD 流水线。目标是将代码质量、测试可靠性、工程自动化提升到生产级标准。

## 阶段划分

| 阶段 | 主题 | 预计工期 | 前置依赖 |
|------|------|---------|---------|
| Phase 1 | 基础修复 + 工程规范 | 1 周 | 无 |
| Phase 2 | 架构拆分 + 债务清理 | 1.5 周 | Phase 1 |
| Phase 3 | 测试补全 + Benchmark | 1.5 周 | Phase 1, Phase 2（可部分并行） |
| Phase 4 | CI 升级 + 发布流水线 | 1 周 | Phase 3 |

## Phase 1：基础修复与工程规范

### 1.1 修复当前 2 个 FAIL

#### `api/` — TestAPI_TaskNotFound

**问题**：取消不存在的任务时，`gorilla/mux` 的默认 `NotFoundHandler` 返回纯文本 `"page not found"`，而非 JSON 错误响应，导致客户端 unmarshal 失败。

**方案**：在 `api/server.go` 的 Cancel handler 中，检查任务不存在时返回明确的 JSON 错误响应，覆盖 mux 的 404 默认行为。

```go
// 在 task Cancel handler 中：
task, err := s.manager.GetTask(id)
if err != nil || task == nil {
    writeJSONError(w, http.StatusNotFound, "task_not_found", "task not found")
    return
}
```

#### `manager/` — Goroutine 泄露

**问题**：`soWorker` 和 `scheduler` goroutine 在测试结束后未退出，`testing` 框架将其报告为 package-level FAIL（尽管各 Test 函数均 PASS）。

**方案**：
1. 在 `manager/` 包中增加 `TestMain(m *testing.M)`，全局管理 Manager 生命周期
2. 每个测试中通过 `t.Cleanup(func() { mgr.Stop() })` 确保清理
3. 在 `Manager.Stop()` 中通过 `sync.WaitGroup` 等待所有后台协程退出，设超时（5s）
4. 新增 `TestManagerGoroutineLeak` 回归测试

### 1.2 激活 Pre-commit 自动化

**方案**：使用 git `core.hooksPath` 机制。

- 新建 `.githooks/pre-commit`（Shell 脚本）
- hooks 执行链：`go fix ./...` → `go fmt ./...` → `addlicense` → `go build ./...` → `go vet ./...`
- Makefile 新增 `install-hooks` 目标：`git config core.hooksPath .githooks`
- 失败时输出明确的错误提示，阻止 commit

涉及文件：
- 新建 `.githooks/pre-commit`
- 修改 `Makefile` — 新增 `install-hooks` 目标
- 删除或废弃 `scripts/pre-commit.sh`（内容整合进 `.githooks/pre-commit`）

### 1.3 零覆盖包补基础测试

为目标零覆盖包添加基础验收测试，目标首次覆盖 >40%：

| 包 | 测试内容 | 测试数 |
|----|---------|--------|
| `core/` | 接口编译期检查、常量/类型正确性、`StorageQuery` 构建器 | 3-5 |
| `downloader/` | `New()` 工厂函数、各下载器创建/Name 返回 | 5-8 |
| `pkg/logutil/` | `InitLogger` 配置、Level 解析、轮转配置 | 3-5 |
| `cmd/m3u8d/`, `cmd/tkcheck/`, `cmd/scraper_get/` | 参数解析、配置读取 | 2-3 每个 |

所有测试使用标准库 `testing` 包（不引入第三方框架）。

### 1.4 Makefile 增强

新增目标：

```makefile
test:           # go test -race -count=1 -timeout=180s ./...
test-cover:     # go test -race -count=1 -coverprofile=build/cover.out ./...
test-cover-html:# go tool cover -html=build/cover.out -o build/cover.html
test-no-mongo:  # go test -tags no_mongo -race -count=1 -timeout=180s ./...
vet:            # go vet ./...
lint:           # golangci-lint run
bench:          # go test -bench=. -benchmem -count=5 -run=^$ ./... > build/bench.txt
install-hooks:  # git config core.hooksPath .githooks
all: vet test bench  # 一站式质量检查
```

### 1.5 测试基础设施

- 在 `manager/` 包中增加 `TestMain` 函数
- 在 `core/` 包中增加 `coretest` 辅助包（或保持轻量），提供 `VerifyTaskInterface`、`VerifyStorageInterface` 等编译期检查辅助函数
- 增强 `manager/testutil_test.go` 中的 `mockTask`，支持可配置的返回行为

## Phase 2：架构拆分与债务清理

### 2.1 `api/server.go` 按领域拆分

当前 `server.go`（814 行）将所有 handler 集中在 `Router()` + 内联 handler 中。

**方案**：拆分为 4 个文件，通过 `s *Server` 方法共享依赖。

```
api/
  server.go              # Router() 路由注册骨架 + 中间件 + Run/Shutdown
  server_task.go         # 任务 CRUD：GET/POST /api/tasks, retry/cancel/batch
  server_config.go       # 配置管理：GET/POST /api/config/*, diff/rollback
  server_metrics.go      # 指标与健康：GET /api/metrics, /api/healthz, /api/aggregate
```

### 2.2 `config/config.go` 的 `Diff()` 提取

`Diff()` 方法（约 185 行）手写逐字段比较所有配置子结构。

**方案**：提取到 `config/config_diff.go`，子结构使用 `reflect.DeepEqual` 简化，仅对特殊字段（指针、map 顺序无关）做手写比较。

### 2.3 解耦 `manager/aggregate.go` 对 `task/tktube` 的依赖

当前 `aggregate.go` 直接 import `task/tktube` 引用 `tktube.TaskType` 常量，违反"上层不依赖具体实现"原则。

**方案**：在 `core/` 包中定义 TaskType 字符串常量：

```go
// core/tasktype.go
const (
    TaskTypeTktube  = "tktube"
    TaskTypeHanime  = "hanime"
    TaskTypeVikacg  = "vikacg"
    TaskTypeURLList = "url_list"
)
```

`aggregate.go` 中改 `obj.TaskID == tktube.TaskType` → `obj.TaskID == core.TaskTypeTktube`，移除对 `task/tktube` 的 import。

各任务包的 `.Type()` 返回值保持字符串不变。`task/factory.go` 注册 key 同样保持字符串。

### 2.4 标记 `pkg/dlcore/` 为正式废弃

**方案**：
1. `pkg/dlcore/doc.go`（新建）添加 Deprecated 注释
2. `downloader/native.go` 文件头添加 Deprecated 注释，说明使用 `downloader.New(cfg)` 指定 `"native"` 类型
3. 不删除代码（`cmd/scraper_get/` 仍有引用），但阻止新代码引用

### 2.5 新增 `"native_old"` 下载器类型（新旧对比）

当前 `downloader.New()` 工厂函数中：

- `"native"` / 默认 → 新路径（`DownloaderAdapter` → `pkg/download`）
- `"wget"` → 旧 Wget（已废弃）
- 旧 `NativeHTTPDownloader`（→ `pkg/dlcore`）无入口可达

**方案**：新增 `"native_old"` 配置类型，路由到 `NativeHTTPDownloader`，便于新旧路径可配置对比。

```go
func New(cfg config.Downloader) core.Downloader {
    switch cfg.Type {
    case "wget":
        slog.Warn("wget is deprecated, use native instead")
        return NewWgetDownloader(cfg)
    case "native_old":
        slog.Warn("native_old uses deprecated pkg/dlcore, migrate to native")
        return NewNativeHTTPDownloader(cfg)
    default: // "native" 或未指定 → 新路径
        return newDownloaderFromConfig(cfg)
    }
}
```

在 `config/config.go` 的 `ValidateAndClamp()` 中，当 `config.Type == "native_http"` 时自动迁移到 `"native_old"` 并打 warn 日志（保持旧配置向后兼容）。

涉及文件：
- `downloader/downloader.go` — switch 增加 `"native_old"` case
- `config/config.go` — `ValidateAndClamp()` 中增加 `"native_http"` → `"native_old"` 迁移
- `downloader/native.go` — 补充废弃标记注释

### 2.6 `task/tktube/task.go` 内联 JS 提取

约 680 行的 `playerUtilJS` 字符串常量。

**方案**：使用 `//go:embed` 将 `.js` 文件嵌入。

```
task/tktube/
  task.go               # 移除 playerUtilJS 常量
  task_test.go          # 不变
  player_util.js        # 原始 JS（纯 JS 文件，无 Go wrapper）
  player_util_embed.go  # //go:embed + var PlayerUtilJS string
```

### 2.7 Manager 模块补充拆分

基于 Phase 1 manager-split 的成果，继续拆分偏大文件：

- 从 `scheduler.go`（423 行）提取 `scheduler_weight.go`：`recalcWeights()` + 权重常量
- 从 `download.go`（432 行）提取 `download_group.go`：内容组优先级策略

## Phase 3：测试覆盖补全 + Benchmark + Fuzz

### 3.1 零覆盖包补全（Phase 1 遗漏区域）

重点为以下包补充测试：

#### `downloader/` 适配层测试（核心关注点）

取代原定对 `pkg/dlcore` 的补测试计划。聚焦 `downloader/` 作为新/旧下载路径的适配层：

| 测试 | 描述 |
|------|------|
| `TestNew_Native` | `New()` 当 `cfg.Type == "native"` 时返回 `DownloaderAdapter` |
| `TestNew_NativeOld` | `cfg.Type == "native_old"` 时返回 `NativeHTTPDownloader` |
| `TestNew_Default` | `cfg.Type == ""` 时默认走 native 新路径 |
| `TestNew_Wget` | wget 下载器创建 |
| `TestAdapter_Download` | 适配器 Download 方法（mock pkg/download） |
| `TestAdapter_Cancel` | 适配器 Cancel 方法 |
| `TestAdapter_Metrics` | Metrics 注册表正确暴露 |
| `TestNativeHTTPDownloader_Interface` | 旧下载器接口实现检查 |
| `TestComposite_ParseFiles` | 复合下载文件列表解析 |
| `TestNew_InvalidType` | 未知类型处理 |

#### 其他零覆盖包

- `pkg/m3u8d/`：M3U8 playlist 解析 + 分段下载流程（5-8 测试）
- `pkg/scrape/`：爬虫驱动创建、分页器翻页、状态追踪（5-8 测试）
- `pkg/titlegroup/`：内容分组算法、变体优先级、代表选择（3-5 测试）
- `pkg/logutil/`：日志初始化、Level 解析（3-5 测试）

### 3.2 Benchmark 测试

新增 15-20 个 `Benchmark*` 函数：

| Benchmark | 关注点 | 所在包 |
|-----------|--------|--------|
| `BenchmarkStorageSearch` | 不同数据量（10/100/1000）的 Search | `storage/` |
| `BenchmarkAggregate` | 聚合查询性能 | `manager/` |
| `BenchmarkSchedulerRecalc` | 权重重算性能 | `manager/` |
| `BenchmarkConfigValidate` | 配置校验性能 | `config/` |
| `BenchmarkStatusTransition` | 状态转换验证 | `model/` |
| `BenchmarkM3U8Parse` | M3U8 playlist 解析 | `pkg/m3u8d/` |
| `BenchmarkTitleGroup` | 内容分组算法 | `pkg/titlegroup/` |
| `BenchmarkDownloadPipeline` | 下载管线吞吐（mock） | `pkg/download/` |
| `BenchmarkAdapterDownload` | 适配器下载映射 | `downloader/` |

输出 `benchstat` 兼容格式，用于 CI 中的回归比较。

### 3.3 Fuzz 测试

新增 3-5 个 `Fuzz*` 函数：

| Fuzz Test | 目标 | 包 |
|-----------|------|-----|
| `FuzzConfigParse` | 模糊 YAML 配置解析 | `config/` |
| `FuzzStatusTransition` | 模糊状态转换输入 | `model/` |
| `FuzzM3U8Parse` | 模糊 M3U8 内容 | `pkg/m3u8d/` |

### 3.4 压力/稳定性测试

| 测试 | 描述 | 位置 |
|------|------|------|
| `TestManagerGoroutineLeak` | 长时间运行后验证 goroutine 数量不增长 | `manager/` |
| `TestStorageConcurrentFlush` | 高并发读写 + ForceFlush | `storage/` |
| `TestSchedulerStability` | 调度器连续 1000 轮不 panic | `manager/` |
| `TestAPIConcurrentRequests` | API 层并发请求正确性 | `api/` |

### 3.5 测试耗时优化

当前全量测试预估 ~120s，目标降至 **~60s**。

| 问题 | 方案 | 预计加速 |
|------|------|---------|
| 真实 HTTP 下载 | 使用 mock + `httptest.NewServer` | 30s → 2s |
| 每测试建新 Manager | Manager 复用 + `t.Cleanup` | 15s → 5s |
| 串行测试 | `t.Parallel()` | 30% |
| goroutine 退出等待 | 明确 shutdown order | 10s → 1s |
| 重复创建存储 | `TestMain` 级共享 | 5s → 1s |

## Phase 4：CI 完整升级 + 发布流水线

### 4.1 CI 覆盖率集成

在 `.github/workflows/ci.yml` 中增强 test job：

```yaml
- name: Test with coverage
  run: go test -race -count=1 -coverprofile=build/cover.out -covermode=atomic ./...

- name: Generate coverage report
  run: |
    go tool cover -html=build/cover.out -o build/cover.html
    echo "## Coverage Report" >> $GITHUB_STEP_SUMMARY
    go tool cover -func=build/cover.out >> $GITHUB_STEP_SUMMARY

- name: Upload coverage to Codecov
  uses: codecov/codecov-action@v4
  with:
    files: build/cover.out
    flags: unittests
```

新建 `.codecov.yml`，设置覆盖率目标（`target: 60%`）、PR 注释配置。

### 4.2 跨平台测试矩阵

```yaml
strategy:
  matrix:
    os: [ubuntu-latest, windows-latest, macOS-latest]
    go: ['1.22', '1.24', '1.26']
```

- Playwright 测试排除在跨平台矩阵之外（仅 Ubuntu）
- 路径分隔符差异在测试中通过 `filepath.Join` 而非硬编码处理
- `go mod verify` 只在单一平台运行（避免网络重复）

### 4.3 Benchmark 回归比较

```yaml
- name: Run benchmarks
  run: go test -bench=. -benchmem -count=5 -run=^$ ./... > build/bench.txt

- uses: benchmark-action/github-action-benchmark@v1
  with:
    tool: go
    output-file-path: build/bench.txt
    alert-threshold: '200%'
    comment-on-alert: true
```

基准历史数据存储于 `gh-pages` 分支，通过 GitHub Pages 展示趋势。

### 4.4 Docker 多阶段构建

新建 `Dockerfile`：

```dockerfile
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /build/download-manager .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata ffmpeg wget
COPY --from=builder /build/download-manager /usr/local/bin/
EXPOSE 8080
ENTRYPOINT ["download-manager"]
```

新建 `.dockerignore`，排除 `test/`, `cmd/playwright-server/`, `.git/` 等构建无关目录。

### 4.5 GoReleaser 发布自动化

新建 `.goreleaser.yaml`：

```yaml
project_name: download-manager
before:
  hooks:
    - go mod tidy
builds:
  - main: .
    ldflags:
      - -s -w
      - -X main.Version={{.Version}}
      - -X main.BuildAt={{.Date}}
    env:
      - CGO_ENABLED=0
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
archives:
  - files: [config.yaml, README.md]
checksum:
  name_template: 'checksums.txt'
changelog:
  use: github
```

新建 `.github/workflows/release.yml`，触发于 tag push `v*`：

1. 运行完整测试
2. GoReleaser 构建 + 发布 GitHub Release
3. 构建并推送 Docker 镜像到 ghcr.io

### 4.6 CI 性能回归告警

- Benchmark threshold: 200%（单次变化超 2 倍）
- 测试超时告警：单 job > 10min 标记 warning
- PR 评论中输出 benchmark diff > 10% 的项

## 文件变更清单

### Phase 1

| 操作 | 文件 | 说明 |
|------|------|------|
| 修改 | `api/server.go` | 修复 Cancel handler 404 响应格式 |
| 修改 | `manager/*_test.go` | 增加 TestMain, t.Cleanup 修复 goroutine leak |
| 新建 | `.githooks/pre-commit` | Pre-commit hook 脚本 |
| 修改 | `Makefile` | 新增 test/test-cover/vet/lint/bench/install-hooks 目标 |
| 新建 | `core/core_test.go` | 接口契约测试 |
| 新建 | `downloader/downloader_test.go` | 工厂函数测试 |
| 新建 | `pkg/logutil/logutil_test.go` | 日志初始化测试 |
| 新建 | `cmd/m3u8d/main_test.go` | CLI 参数解析测试 |
| 新建 | `cmd/tkcheck/main_test.go` | CLI 参数解析测试 |
| 新建 | `cmd/scraper_get/main_test.go` | CLI 参数解析测试 |

### Phase 2

| 操作 | 文件 | 说明 |
|------|------|------|
| 修改 | `api/server.go` | 精简，保留 Router() 骨架 |
| 新建 | `api/server_task.go` | Task handler 拆分 |
| 新建 | `api/server_config.go` | Config handler 拆分 |
| 新建 | `api/server_metrics.go` | Metrics handler 拆分 |
| 新建 | `config/config_diff.go` | Diff() 方法提取 |
| 新建 | `core/tasktype.go` | TaskType 常量定义 |
| 修改 | `manager/aggregate.go` | 移除对 task/tktube 的 import |
| 新建 | `pkg/dlcore/doc.go` | Deprecated 文档注释 |
| 修改 | `downloader/downloader.go` | 新增 native_old case |
| 修改 | `config/config.go` | ValidateAndClamp 中 native_http 自动迁移 |
| 新建 | `task/tktube/player_util.js` | JS 文件提取 |
| 新建 | `task/tktube/player_util_embed.go` | go:embed |
| 修改 | `task/tktube/task.go` | 移除内联 JS 常量 |
| 新建 | `manager/scheduler_weight.go` | 权重逻辑提取 |
| 新建 | `manager/download_group.go` | 组策略提取 |

### Phase 3

| 操作 | 文件 | 说明 |
|------|------|------|
| 修改 | `downloader/downloader_test.go` | 补适配层测试（10-15 个） |
| 新建 | `pkg/m3u8d/m3u8d_test.go` | 5-8 测试 |
| 新建 | `pkg/scrape/scrape_test.go` | 5-8 测试 |
| 新建 | `pkg/titlegroup/titlegroup_test.go` | 3-5 测试 |
| 修改 | `storage/*_test.go` | 新增 Benchmark |
| 修改 | `manager/*_test.go` | 新增 Benchmark + 稳定性测试 |
| 修改 | `config/*_test.go` | 新增 Fuzz + Benchmark |
| 新建 | `model/model_fuzz_test.go` | Fuzz 测试 |

### Phase 4

| 操作 | 文件 | 说明 |
|------|------|------|
| 修改 | `.github/workflows/ci.yml` | 覆盖率 + 跨平台矩阵 + benchmark |
| 新建 | `.codecov.yml` | Codecov 配置 |
| 新建 | `Dockerfile` | 多阶段构建 |
| 新建 | `.dockerignore` | Docker 构建忽略 |
| 新建 | `.goreleaser.yaml` | 发布配置 |
| 新建 | `.github/workflows/release.yml` | 发布 CI |

## 验证标准

### 每阶段出口标准

**Phase 1**：
- [ ] `go test ./...` 全部 PASS（包括之前 FAIL 的 2 包）
- [ ] `go vet ./...` 无 warning
- [ ] `make install-hooks` 后可触发 pre-commit 检查
- [ ] 4 个零覆盖包均有基础测试，覆盖率 >40%

**Phase 2**：
- [ ] `api/server.go` 缩减 60%+（handler 已拆分）
- [ ] `manager/aggregate.go` 不再 import 具体任务包
- [ ] `config/config.go` Diff() 移除后总行数下降
- [ ] `"native_old"` 类型可正常工作
- [ ] `task/tktube/task.go` 减少 680 行
- [ ] 构建通过，测试通过

**Phase 3**：
- [ ] 全量测试耗时 ≤ 60s（当前 ~120s）
- [ ] Benchmark 可重复运行，输出 `benchstat` 兼容格式
- [ ] Fuzz 测试可运行
- [ ] 总体覆盖率 ≥ 65%（当前 ~56.5%）
- [ ] 压力测试无 goroutine leak

**Phase 4**：
- [ ] CI 产出覆盖率报告
- [ ] 跨平台 3OS × 3Go 版本测试通过
- [ ] Docker 镜像可构建并运行
- [ ] GoReleaser 可构建多平台二进制
- [ ] Benchmark 历史趋势展示
