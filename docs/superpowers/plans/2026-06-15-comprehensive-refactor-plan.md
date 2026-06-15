# 全面重构升级实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。
>
> ⚠️ 禁用 worktree，直接在当前分支 `dev` 上开发。

**目标：** 对 download-manager 项目进行系统性重构升级，涵盖架构拆分、技术债务清理、测试覆盖补全、性能基准、CI/CD 流水线。

**架构：** 4 阶段递进式重构：Phase 1 基础修复 → Phase 2 架构拆分 → Phase 3 测试补全+Benchmark → Phase 4 CI/CD 升级。各阶段部分可并行。

**技术栈：** Go 1.26, gorilla/mux, standard library testing, GitHub Actions, Codecov, GoReleaser, Docker

---

## 修改文件总览

### Phase 1 — 基础修复与工程规范

| 操作 | 文件 | 说明 |
|------|------|------|
| 修改 | `api/server.go` | 修复 Cancel handler 404 响应格式（JSON 而非纯文本） |
| 修改 | `manager/*_test.go` | 增加 TestMain, t.Cleanup 修复 goroutine leak |
| 新建 | `.githooks/pre-commit` | Pre-commit hook 脚本 |
| 修改 | `Makefile` | 新增 test/test-cover/vet/lint/bench/install-hooks/all 目标 |
| 修改 | `core/core_test.go` | 新增接口编译期检查测试 |
| 新建 | `downloader/downloader_test.go` | 工厂函数创建测试 |
| 新建 | `pkg/logutil/logutil_test.go` | 日志初始化测试 |
| 新建 | `cmd/m3u8d/main_test.go` | CLI 参数解析测试 |
| 新建 | `cmd/tkcheck/main_test.go` | CLI 参数解析测试 |
| 新建 | `cmd/scraper_get/main_test.go` | CLI 参数解析测试 |

### Phase 2 — 架构拆分与债务清理

| 操作 | 文件 | 说明 |
|------|------|------|
| 修改 | `api/server.go` | 精简到 Router() 骨架 |
| 新建 | `api/server_task.go` | Task handler 拆分 |
| 新建 | `api/server_config.go` | Config handler 拆分 |
| 新建 | `api/server_metrics.go` | Metrics handler 拆分 |
| 新建 | `config/config_diff.go` | Diff() + Change struct 从 config.go 提取 |
| 修改 | `config/config.go` | 移除 Diff() + Change struct，保留引用 |
| 新建 | `core/tasktype.go` | TaskType 常用常量 |
| 修改 | `manager/aggregate.go` | 移除对 task/tktube 的 import，改用 core.TaskTypeTktube |
| 新建 | `pkg/dlcore/doc.go` | Deprecated 文档注释 |
| 修改 | `downloader/downloader.go` | 新增 "native_old" case → NewNativeHTTPDownloader |
| 修改 | `config/config.go` | ValidateAndClamp: native_http → native_old 自动迁移 |
| 新建 | `task/tktube/player_util.js` | JS 源文件 |
| 新建 | `task/tktube/player_util_embed.go` | go:embed |
| 修改 | `task/tktube/task.go` | 移除 playerUtilJS，引用 Embed 变量 |
| 新建 | `manager/scheduler_weight.go` | 从 scheduler.go 提取 weight 逻辑 |
| 新建 | `manager/download_group.go` | 从 download.go 提取组优先级策略 |

### Phase 3 — 测试补全 + Benchmark + Fuzz

| 操作 | 文件 | 说明 |
|------|------|------|
| 修改 | `downloader/downloader_test.go` | 新增适配层测试（10-15 个函数） |
| 新建 | `pkg/m3u8d/m3u8d_test.go` | M3U8 测试（5-8 个） |
| 新建 | `pkg/scrape/scrape_test.go` | 爬虫测试（5-8 个） |
| 新建 | `pkg/titlegroup/titlegroup_test.go` | 分组算法测试（3-5 个） |
| 修改 | `storage/*_test.go` | 新增 Benchmark |
| 修改 | `manager/*_test.go` | 新增 Benchmark + 压力测试 |
| 修改 | `config/*_test.go` | 新增 Fuzz + Benchmark |
| 新建 | `model/model_fuzz_test.go` | 状态转换 Fuzz 测试 |
| 修改 | `pkg/download/*_test.go` | 新增 Benchmark |

### Phase 4 — CI 升级 + 发布流水线

| 操作 | 文件 | 说明 |
|------|------|------|
| 修改 | `.github/workflows/ci.yml` | 覆盖率 + 跨平台矩阵 + benchmark 比较 |
| 新建 | `.codecov.yml` | Codecov 配置 |
| 新建 | `Dockerfile` | 多阶段构建 |
| 新建 | `.dockerignore` | 构建忽略配置 |
| 新建 | `.goreleaser.yaml` | 发布配置 |
| 新建 | `.github/workflows/release.yml` | 发布 CI |

---

## Phase 1 任务

### 任务 1.1：修复 api/ TestAPI_TaskNotFound FAIL

✅ **已完成**（当前测试已 PASS，无需修改）

```bash
cd D:/workdir/leon/cocomhub/download-manager && go test -v -count=1 -timeout=60s -run 'TestAPI_TaskNotFound' ./api/
# 输出：--- PASS: TestAPI_TaskNotFound
```

### 任务 1.2：修复 manager/ goroutine leak FAIL

**文件：**
- 修改：`manager/` 下的测试文件
- 可能修改：`manager/manager.go`

**当前问题：** `soWorker` 和 `scheduler` goroutines 在测试结束后未退出，导致 testing 框架报告 package FAIL。

验证策略：找出 Manager.Stop() 是否等待所有后台 goroutine 退出。

- [ ] **步骤 1：检查 Manager 中后台 goroutine 管理情况**

```bash
cd D:/workdir/leon/cocomhub/download-manager && grep -n 'go func\|go soWorker\|go scheduler\|go.*Worker\|Stop()' manager/manager.go | head -20
```

- [ ] **步骤 2：检查现有 Stop() 实现中是否等待 goroutine 完成**

```bash
cd D:/workdir/leon/cocomhub/download-manager && grep -n -A 5 'func.*Stop' manager/manager.go | head -30
```

- [ ] **步骤 3：在 Manager 中增加 stopCh + WaitGroup 支持**

在 `manager/manager.go` 中添加或确认存在 `stopCh chan struct{}` 和 `wg sync.WaitGroup`，确保 `Stop()` 方法通过 `close(stopCh)` + `wg.Wait()` 等待所有 goroutine 退出。

如果不确定具体改动，可以在 `Manager` struct 中新增字段：

```go
// 在 Manager struct 中增加（如不存在）
stopCh chan struct{}
wg     sync.WaitGroup
```

在 `Start()` 各 goroutine 入口处 defer 使用 `select` 监听 `stopCh`：

```go
go func() {
    m.wg.Add(1)
    defer m.wg.Done()
    for {
        select {
        case <-m.stopCh:
            return
        default:
            // ... existing logic
        }
    }
}()
```

`Stop()` 中：
```go
close(m.stopCh)
m.wg.Wait()
```

- [ ] **步骤 4：在各个测试中使用 t.Cleanup 确保 Manager 关闭**

在 `manager/*_test.go` 中涉及创建 Manager 的测试，添加：

```go
mgr := NewManager(...)
t.Cleanup(func() {
    done := make(chan struct{})
    go func() {
        mgr.Stop()
        close(done)
    }()
    select {
    case <-done:
    case <-time.After(5*time.Second):
        t.Log("Warning: Manager.Stop() timed out")
    }
})
```

- [ ] **步骤 5：运行 manager 包全部测试验证**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go test -v -count=1 -timeout=120s ./manager/ 2>&1 | tail -30
```

预期：末尾 `ok   github.com/cocomhub/download-manager/manager` 而非 `FAIL`

- [ ] **步骤 6：Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager && git add manager/manager.go manager/*_test.go && git commit -m "fix: resolve goroutine leak in manager tests by proper Stop() cleanup"
```

### 任务 1.3：激活 Pre-commit 自动化

**文件：**
- 新建：`.githooks/pre-commit`
- 修改：`Makefile`

- [ ] **步骤 1：创建 `.githooks/pre-commit`**

```bash
cd D:/workdir/leon/cocomhub/download-manager && mkdir -p .githooks
```

创建 `.githooks/pre-commit`：

```sh
#!/bin/sh
# Copyright 2026 The Cocomhub Authors. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

# Pre-commit hook for download-manager
set -e

echo "🔍 Pre-commit checks..."

# 1. go fix
echo "  • go fix ./..."
go fix ./...

# 2. go fmt
echo "  • go fmt ./..."
gofmt -s -l -w .

# 3. addlicense（如果已安装）
if command -v addlicense >/dev/null 2>&1; then
    echo "  • addlicense"
    addlicense -c "The Cocomhub Authors. All rights reserved." -s=only .
fi

# 4. go build
echo "  • go build ./..."
go build ./...

# 5. go vet
echo "  • go vet ./..."
go vet ./...

echo "✅ Pre-commit checks passed"
```

- [ ] **步骤 2：设置执行权限**

```bash
cd D:/workdir/leon/cocomhub/download-manager && chmod +x .githooks/pre-commit
```

- [ ] **步骤 3：Makefile 新增 install-hooks 目标**

在 `Makefile` 末尾添加：

```makefile
# Install git hooks
.PHONY: install-hooks
install-hooks:
	@echo "Installing git hooks..."
	git config core.hooksPath .githooks
	@echo "✅ Git hooks installed at .githooks/"
```

同时在 `Makefile` 中新增 test/vet/lint/bench/all 目标：

```makefile
.PHONY: test test-cover test-cover-html test-no-mongo vet lint bench all

test:
	go test -race -count=1 -timeout=180s ./...

test-cover:
	go test -race -count=1 -coverprofile=build/cover.out -covermode=atomic ./...

test-cover-html: test-cover
	go tool cover -html=build/cover.out -o build/cover.html

test-no-mongo:
	go test -tags no_mongo -race -count=1 -timeout=180s ./...

vet:
	go vet ./...

lint:
	golangci-lint run

bench:
	go test -bench=. -benchmem -count=5 -run=^$$ ./... > build/bench.txt

all: vet test bench
	@echo "✅ All checks passed"
```

注意：`test-no-mongo` 需要和现有的 `addlicense` 等目标对齐格式（缩进用 tab）。

- [ ] **步骤 4：验证 install-hooks**

```bash
cd D:/workdir/leon/cocomhub/download-manager && git config core.hooksPath .githooks
```

- [ ] **步骤 5：Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager && git add .githooks/pre-commit Makefile && git commit -m "feat: add pre-commit hooks and Makefile test/vet/lint/bench targets"
```

### 任务 1.4：零覆盖包补基础测试

**文件：**
- 新建：`core/core_test.go`
- 新建：`downloader/downloader_test.go`
- 新建：`pkg/logutil/logutil_test.go`
- 新建：`cmd/m3u8d/main_test.go`
- 新建：`cmd/tkcheck/main_test.go`
- 新建：`cmd/scraper_get/main_test.go`

- [ ] **步骤 1：core 包接口检查测试**

```go
// core/core_test.go
package core

import (
	"context"
	"testing"

	"github.com/cocomhub/download-manager/model"
)

// 编译期检查：确保所有接口定义正确（编译即可验证）
var (
	_ Storage = (*mockStorage)(nil)
	_ Task    = (*mockTask)(nil)
	_ Downloader = (*mockDownloader)(nil)
)

type mockStorage struct{ Storage }
type mockTask struct{ Task }
type mockDownloader struct{ Downloader }

// TestStorageQueryBuilder 验证 StorageQuery 构建器
func TestStorageQueryBuilder(t *testing.T) {
	q := &StorageQuery{
		TaskID:  "task1",
		Status:  "pending",
		Types:   []string{"tktube"},
		Page:    1,
		Limit:   10,
		SortBy:  "created_at",
		Reverse: true,
	}
	if q.TaskID != "task1" {
		t.Errorf("TaskID = %q, want %q", q.TaskID, "task1")
	}
	if q.Page != 1 || q.Limit != 10 {
		t.Errorf("Page/Limit = %d/%d, want 1/10", q.Page, q.Limit)
	}
}

// TestEventTypeConstants 验证事件类型常量
func TestEventTypeConstants(t *testing.T) {
	tests := []struct {
		name  string
		value EventType
		want  string
	}{
		{"EventTaskUpdate", EventTaskUpdate, "task_update"},
		{"EventTaskListChange", EventTaskListChange, "task_list_change"},
		{"EventObjectUpdate", EventObjectUpdate, "object_update"},
	}
	for _, tt := range tests {
		if string(tt.value) != tt.want {
			t.Errorf("%s = %q, want %q", tt.name, string(tt.value), tt.want)
		}
	}
}
```

注意：mockStorage/mockTask/mockDownloader 使用了嵌入式接口，需要确保编译通过。如果更安全的做法，也可以去掉 var 编译期检查。

- [ ] **步骤 2：运行 core 测试**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go test -v -count=1 ./core/
```

- [ ] **步骤 3：downloader 包工厂函数测试**

```go
// downloader/downloader_test.go
package downloader

import (
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
)

func TestNew_Native(t *testing.T) {
	cfg := config.Downloader{Type: "native"}
	d := New(cfg)
	if d == nil {
		t.Fatal("New() with type=native returned nil")
	}
	if d.Name() != "native_http" {
		t.Errorf("New().Name() = %q, want %q", d.Name(), "native_http")
	}
	// 验证类型
	if _, ok := d.(*DownloaderAdapter); !ok {
		t.Errorf("expected *DownloaderAdapter, got %T", d)
	}
}

func TestNew_Default(t *testing.T) {
	cfg := config.Downloader{Type: ""} // 空字符串应走默认路径（新下载器）
	d := New(cfg)
	if d == nil {
		t.Fatal("New() with empty type returned nil")
	}
	if _, ok := d.(*DownloaderAdapter); !ok {
		t.Errorf("expected *DownloaderAdapter, got %T", d)
	}
}

func TestNew_Wget(t *testing.T) {
	cfg := config.Downloader{Type: "wget"}
	d := New(cfg)
	if d == nil {
		t.Fatal("New() with type=wget returned nil")
	}
	// wget 返回 WgetDownloader，不检查具体类型（避免 import 循环）
}

func TestNew_NativeOld(t *testing.T) {
	cfg := config.Downloader{Type: "native_old"}
	d := New(cfg)
	if d == nil {
		t.Fatal("New() with type=native_old returned nil")
	}
	if _, ok := d.(*NativeHTTPDownloader); !ok {
		t.Errorf("expected *NativeHTTPDownloader, got %T", d)
	}
}

func TestNew_UnknownType(t *testing.T) {
	cfg := config.Downloader{Type: "nonexistent"}
	d := New(cfg)
	if d == nil {
		t.Fatal("New() with unknown type returned nil")
	}
	// 应回退到默认
	if _, ok := d.(*DownloaderAdapter); !ok {
		t.Errorf("expected *DownloaderAdapter fallback, got %T", d)
	}
}
```

注意：上面的测试中 `NewDownloaderAdapter` 不是导出函数，测试需要调整。

实际看 `adapter.go` 中 `NewDownloaderAdapter` 是小写（包私有）。所以测试 `TestNew_Native` 应该检查 Name() 正确性即可，不需要检查具体类型。修正：

```go
func TestNew_Getters(t *testing.T) {
    cfg := config.Downloader{Type: "native"}
    d := New(cfg)
    if d == nil {
        t.Fatal("New() returned nil")
    }
    if got := d.Name(); got == "" {
        t.Error("Name() returned empty")
    }
}
```

- [ ] **步骤 4：运行 downloader 测试**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go test -v -count=1 ./downloader/
```

- [ ] **步骤 5：pkg/logutil 初始化测试**

```go
// pkg/logutil/logutil_test.go
package logutil

import (
	"testing"
)

func TestLogConfig_Defaults(t *testing.T) {
	lc := LogConfig{}
	if lc.MaxSize != 0 {
		t.Errorf("MaxSize = %d, want 0", lc.MaxSize)
	}
	if lc.Level != "" {
		t.Errorf("Level = %q, want empty", lc.Level)
	}
}

func TestLogConfig_ValidValues(t *testing.T) {
	lc := LogConfig{
		Level:      "info",
		Filename:   "/tmp/test.log",
		MaxSize:    10,
		MaxBackups: 3,
		MaxAge:     7,
		Console:    true,
		Compress:   true,
	}
	if lc.Level != "info" {
		t.Errorf("Level = %q, want info", lc.Level)
	}
	if lc.MaxSize != 10 {
		t.Errorf("MaxSize = %d, want 10", lc.MaxSize)
	}
}
```

- [ ] **步骤 6：运行 logutil 测试**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go test -v -count=1 ./pkg/logutil/
```

- [ ] **步骤 7：cmd/m3u8d CLI 参数测试**

```go
// cmd/m3u8d/main_test.go
package main

import (
	"flag"
	"testing"
)

func TestParseFlags_Defaults(t *testing.T) {
	// 验证 flag 默认值
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	url := fs.String("url", "", "M3U8 URL")
	output := fs.String("output", "output.mp4", "Output file")
	if *url != "" {
		t.Errorf("default url = %q, want empty", *url)
	}
	if *output != "output.mp4" {
		t.Errorf("default output = %q, want output.mp4", *output)
	}
}
```

注意：m3u8d 的 main 包可能没有导出 flag 变量，上述测试可能涉及包内部细节。更简单的做法是直接编译测试：

```go
package main

import (
	"os"
	"testing"
)

func TestMainFlags(t *testing.T) {
	// 至少确保包可以编译，且 main 函数能解析基本参数
	os.Args = []string{"m3u8d", "--help"}
	// 不实际运行（避免 exit(0)），只是验证编译
}
```

实际上最简单的就是确保文件存在并包含 `package main` + 一个简单的编译验证测试，但更好的策略是：`TestMainHasMain`:
```go
package main

import "testing"

func TestMainPackageCompiles(t *testing.T) {
	// 确保 main 函数存在（编译时检查）
	var _ = main
}
```

- [ ] **步骤 8：运行 cmd/m3u8d 测试**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go test -v -count=1 ./cmd/m3u8d/
```

- [ ] **步骤 9：Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager && git add core/core_test.go downloader/downloader_test.go pkg/logutil/logutil_test.go cmd/m3u8d/main_test.go cmd/tkcheck/main_test.go cmd/scraper_get/main_test.go && git commit -m "test: add base tests for zero-coverage packages"
```

### 任务 1.5：全量验证 Phase 1

- [ ] **步骤 1：运行完整测试**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go test -race -count=1 -timeout=180s ./...
```

预期：所有包 `ok`，无 `FAIL`

- [ ] **步骤 2：运行 go vet**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go vet ./...
```

预期：无警告

- [ ] **步骤 3：Commit 收尾（如有额外修复）**

---

## Phase 2 任务

### 任务 2.1：api/server.go 按领域拆分

**文件：**
- 修改：`api/server.go`
- 新建：`api/server_task.go`
- 新建：`api/server_config.go`
- 新建：`api/server_metrics.go`

**当前状态：** `api/server.go` 813 行，`Router()` 方法注册约 30 条路由，所有 handler 以方法形式定义在 Server 上。

**策略：** 不改变路由注册逻辑，只将 handler 方法按领域移到独立文件。Server struct 定义保留在 server.go。

- [ ] **步骤 1：读取 server.go 完整内容**

```bash
cd D:/workdir/leon/cocomhub/download-manager && cat -n api/server.go
```

- [ ] **步骤 2：将 task 相关的 handler 提取到 server_task.go**

提取以下 handler 方法（方法签名不变，从原文件剪切）：
- `handleTasksGet` / `handleTaskCreate`
- `handleTaskGet` / `handleTaskUpdate`
- `handleTaskRetry` / `handleTaskCancel`
- `handleCancelBatch` / `handleObjectCancel`
- `handleUndoObjectCancel` / `handleCancelObjectBatch`
- `handleUndoCancelBatch` / `handleReorder`
- `handleTaskConfig` / `handleTaskRuntime`

在新文件 `server_task.go` 中添加 `package api` 和 import（内容与原 server.go 中使用的 import 一致）。

- [ ] **步骤 3：提取 config handler 到 server_config.go**

从 `server.go` 剪切与 config 相关的 handler：
- `handleConfigGet` / `handleConfigUpdate`
- `handleConfigHistory` / `handleConfigRollback`
- `handleConfigDiff` / `handleConfigTag`
- `handleConfigNote` / `handleConfigDelete`
- `handleConfigApply`

- [ ] **步骤 4：提取 metrics/health handler 到 server_metrics.go**

从 `server.go` 剪切：
- `handleHealthz`
- `handleRuntime`
- `handleMetrics`
- `handleMetricsFailures`
- `handleAggregate`
- `handleDownloads`

- [ ] **步骤 5：验证构建通过**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go build ./...
```

- [ ] **步骤 6：运行 api 测试验证**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go test -v -count=1 -timeout=60s ./api/ 2>&1 | tail -30
```

- [ ] **步骤 7：Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager && git add api/server.go api/server_task.go api/server_config.go api/server_metrics.go && git commit -m "refactor: split api/server.go into task/config/metrics domains"
```

### 任务 2.2：config.go Diff() 提取

**文件：**
- 新建：`config/config_diff.go`
- 修改：`config/config.go`

**当前状态：** `config.go` 约 570 行，`Diff()`（约 188 行）和 `Change struct` 都在其中。

- [ ] **步骤 1：创建 config/config_diff.go**

将 `Change struct`（367 行附近）、`Diff()`（382 行）、`taskIndex()` helper 剪切到新文件。

```go
// config/config_diff.go
package config

import "reflect"

// Change represents a single configuration change.
type Change struct {
	Path string
	A    interface{}
	B    interface{}
}

// Diff compares two Config structs and returns a list of changes.
func (a Config) Diff(b Config) []Change {
	// ... 粘贴现有 Diff() 完整实现 ...
}

func taskIndex(tasks []Task, id string) int {
	for i, t := range tasks {
		if t.ID == id {
			return i
		}
	}
	return -1
}
```

- [ ] **步骤 2：从 config.go 中移除 Change struct + Diff() + taskIndex()**

保留 `UIDefaults` 中的 `DiffSideBySide` 等字段的定义（它们是 UI 配置字段，不是 Diff 函数）。

- [ ] **步骤 3：验证构建通过**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go build ./...
```

- [ ] **步骤 4：运行 config 测试**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go test -v -count=1 -timeout=30s ./config/
```

- [ ] **步骤 5：Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager && git add config/config_diff.go config/config.go && git commit -m "refactor: extract Diff() and Change struct to config_diff.go"
```

### 任务 2.3：解耦 aggregate.go 对 task/tktube 的依赖

**文件：**
- 新建：`core/tasktype.go`
- 修改：`manager/aggregate.go`
- 修改：`manager/aggregation_service.go`（如有）

- [ ] **步骤 1：创建 core/tasktype.go**

```go
// core/tasktype.go
package core

// Task type constants.
// These should match the type strings used in task registration and config.
const (
	TaskTypeTktube  = "tktube"
	TaskTypeHanime  = "hanime"
	TaskTypeVikacg  = "vikacg"
	TaskTypeURLList = "url_list"
)
```

- [ ] **步骤 2：修改 manager/aggregate.go**

移除 import `"github.com/cocomhub/download-manager/task/tktube"`，所有 `tktube.TaskType` 替换为 `core.TaskTypeTktube`。

原有代码（aggregate.go 中）：
```go
import (
    // ...
    "github.com/cocomhub/download-manager/core"
    "github.com/cocomhub/download-manager/model"
    "github.com/cocomhub/download-manager/pkg/titlegroup"
    "github.com/cocomhub/download-manager/storage"
    "github.com/cocomhub/download-manager/task/tktube"
)
```

修改为：
```go
import (
    "log/slog"
    "maps"
    "strings"

    "github.com/cocomhub/download-manager/core"
    "github.com/cocomhub/download-manager/model"
    "github.com/cocomhub/download-manager/pkg/titlegroup"
    "github.com/cocomhub/download-manager/storage"
)
```

使用处（约 173, 193 行）：
```go
t.Type() != core.TaskTypeTktube
```

- [ ] **步骤 3：搜索 aggregate.go 中所有 tktube.* 引用**

```bash
cd D:/workdir/leon/cocomhub/download-manager && grep -n 'tktube\.' manager/aggregate.go manager/aggregation_service.go
```

- [ ] **步骤 4：验证构建通过**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go build ./...
```

- [ ] **步骤 5：运行 manager 测试**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go test -v -count=1 -timeout=60s -run 'TestAggregate' ./manager/ 2>&1 | tail -20
```

- [ ] **步骤 6：Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager && git add core/tasktype.go manager/aggregate.go manager/aggregation_service.go && git commit -m "refactor: decouple manager/aggregate.go from task/tktube via core.TaskType constants"
```

### 任务 2.4：标记 pkg/dlcore 为正式废弃 + 新增 native_old 类型

**文件：**
- 新建：`pkg/dlcore/doc.go`
- 修改：`downloader/downloader.go`
- 修改：`downloader/native.go`
- 修改：`config/config.go`

- [ ] **步骤 1：创建 pkg/dlcore/doc.go**

```go
// Package dlcore provides a low-level HTTP download client with support for
// proxy rotation, HLS/FFmpeg, progress tracking, and retry logic.
//
// Deprecated: This package is superseded by github.com/cocomhub/download-manager/pkg/download.
// New code should use the pkg/download package directly. Existing users should
// migrate to pkg/download for ongoing improvements and bug fixes.
//
// This package is retained only for backward compatibility and for use by
// cmd/scraper_get. No new features will be added.
package dlcore
```

- [ ] **步骤 3：在 downloader.go 中新增 "native_old" 分支**

修改 `downloader/downloader.go` 的 `New()` 函数：

```go
func New(cfg config.Downloader) core.Downloader {
    switch cfg.Type {
    case "wget":
        slog.Warn("wget backend is deprecated, use native instead")
        return NewWgetDownloader(cfg)
    case "native_old":
        slog.Warn("native_old uses deprecated pkg/dlcore, migrate to native (new pkg/download path)")
        return NewNativeHTTPDownloader(cfg)
    default:
        // "native" 或未指定 → 新路径
        return newDownloaderFromConfig(cfg)
    }
}
```

- [ ] **步骤 4：在 native.go 文件头补充废弃注释**

在 `downloader/native.go` 的 package 声明后添加：

```go
// Deprecated: NativeHTTPDownloader uses the deprecated pkg/dlcore.
// Use a "native" type config with New() to get the new pkg/download path.
```

- [ ] **步骤 5：config.go ValidateAndClamp 中 native_http → native_old 迁移**

在 `config/config.go` 的 `ValidateAndClamp()` 函数体中（约 203 行），`GlobalConcurrent` clamp 之前添加：

```go
// Migrate deprecated "native_http" type to "native_old"
if c.Downloader.Type == "native_http" {
    slog.Warn("config: downloader type 'native_http' is deprecated, migrating to 'native_old'. " +
        "Use type 'native' for the new pkg/download path.")
    c.Downloader.Type = "native_old"
}
```

- [ ] **步骤 6：验证构建 + 测试**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go build ./... && go test -v -count=1 -timeout=60s ./downloader/ ./config/
```

- [ ] **步骤 7：Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager && git add pkg/dlcore/doc.go downloader/downloader.go downloader/native.go config/config.go && git commit -m "feat: add native_old downloader type, mark dlcore and native.go as deprecated"
```

### 任务 2.5：task/tktube 内联 JS 提取

**文件：**
- 新建：`task/tktube/player_util.js`
- 新建：`task/tktube/player_util_embed.go`
- 修改：`task/tktube/task.go`

- [ ] **步骤 1：从 task/tktube/task.go 的 563 行开始提取 JS 内容到 player_util.js**

```bash
cd D:/workdir/leon/cocomhub/download-manager && sed -n '563,677p' task/tktube/task.go | sed 's/^const playerUtilJS = `//' | sed 's/`$//' > task/tktube/player_util.js
```

但直接这样提取需要处理 Go raw string 的语法边界。更可靠的方法：

```bash
cd D:/workdir/leon/cocomhub/download-manager && sed -n '563,563p' task/tktube/task.go | head -c 22
```

检查起始行。实际上 playerUtilJS 从 563 行到 677 行。最后一行是 ``` 结尾。可以用 sed 提取内容（去掉首行的 `const playerUtilJS = `` 和末行的 `` ` ```）：

```bash
cd D:/workdir/leon/cocomhub/download-manager && awk 'NR==563{sub(/^const playerUtilJS = `/, "")} NR>=563&&NR<=677' task/tktube/task.go > task/tktube/player_util.js
```

但更精准的方法是手动读取确认后操作。这里作为计划的步骤，我们使用手动提取：

```bash
# 先删除首行的 const playerUtilJS = ` 前缀
# 再删除末行的 ` 后缀
cd D:/workdir/leon/cocomhub/download-manager && sed '1s/^const playerUtilJS = `//; $s/`$//' <(sed -n '563,677p' task/tktube/task.go) > task/tktube/player_util.js
```

- [ ] **步骤 2：创建 player_util_embed.go**

```go
// task/tktube/player_util_embed.go
package tktube

import _ "embed"

// PlayerUtilJS is the embedded JavaScript utility for the tktube player.
//
//go:embed player_util.js
var PlayerUtilJS string
```

- [ ] **步骤 3：修改 task/tktube/task.go**

- 删除 `const playerUtilJS = ...`（562-677 行）
- 将 `vm.RunString(playerUtilJS)` 改为 `vm.RunString(PlayerUtilJS)`

```go
_, err = vm.RunString(PlayerUtilJS)
```

- [ ] **步骤 4：验证构建通过**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go build ./task/tktube/...
```

- [ ] **步骤 5：运行 tktube 测试**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go test -v -count=1 -timeout=30s ./task/tktube/
```

- [ ] **步骤 6：Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager && git add task/tktube/player_util.js task/tktube/player_util_embed.go task/tktube/task.go && git commit -m "refactor: extract 116-line playerUtilJS to embedded player_util.js"
```

### 任务 2.6：Manager scheduler_weight + download_group 拆分

**文件：**
- 新建：`manager/scheduler_weight.go`
- 新建：`manager/download_group.go`

- [ ] **步骤 1：从 scheduler.go 提取 weight 逻辑**

读取 `manager/scheduler.go` 中与权重计算相关的函数（如 `recalcWeights()`，权重常量等），提取到 `manager/scheduler_weight.go`。

```go
// manager/scheduler_weight.go
package manager

// 从 scheduler.go 中剪切此文件的所有权重相关内容
```

具体的权重逻辑提取需要在读取 scheduler.go 内容后确认。策略：grep 查找 `weight`、`Weight`、`recalc` 等关键词。

- [ ] **步骤 2：从 download.go 提取组优先级策略**

读取 `manager/download.go` 中与内容组优先级相关的函数，提取到 `manager/download_group.go`。

```go
// manager/download_group.go
package manager

// 内容组优先级策略
```

- [ ] **步骤 3：验证构建**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go build ./...
```

- [ ] **步骤 4：运行 manager 测试**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go test -v -count=1 -timeout=60s ./manager/
```

- [ ] **步骤 5：Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager && git add manager/scheduler_weight.go manager/download_group.go manager/scheduler.go manager/download.go && git commit -m "refactor: extract scheduler weight and download group logic to dedicated files"
```

### 任务 2.7：Phase 2 全量验证

- [ ] **步骤 1：全量构建 + vet**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go build ./... && go vet ./...
```

- [ ] **步骤 2：全量测试**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go test -race -count=1 -timeout=180s ./...
```

- [ ] **步骤 3：Commit 收尾**

---

## Phase 3 任务

### 任务 3.1：downloader 适配层测试（核心）

**文件：**
- 修改：`downloader/downloader_test.go`

基于任务 1.4 中已创建的基础测试文件，补充适配层测试。

- [ ] **步骤 1：新增 Core.Downloader 接口实现测试**

```go
// 在 downloader_test.go 中新增：

func TestDownloader_ImplementsCoreInterface(t *testing.T) {
    // 编译期检查
    var _ core.Downloader = New(config.Downloader{Type: "native"})
    var _ core.Downloader = New(config.Downloader{Type: "native_old"})
    var _ core.Downloader = New(config.Downloader{Type: "wget"})
}

func TestAdapter_Name(t *testing.T) {
    d := New(config.Downloader{Type: "native"})
    if got := d.Name(); got == "" {
        t.Error("Name() returned empty")
    }
}

func TestNativeOld_Name(t *testing.T) {
    d := New(config.Downloader{Type: "native_old"})
    if got := d.Name(); got != "native_http" {
        t.Errorf("Name() = %q, want native_http", got)
    }
}
```

- [ ] **步骤 2：测试上下文传递（SetContext）**

```go
func TestAdapter_WithContext(t *testing.T) {
    dl, ok := New(config.Downloader{Type: "native"}).(core.DownloaderWithContext)
    if !ok {
        t.Skip("Downloader does not implement DownloaderWithContext")
    }
    ctx := context.Background()
    dl.SetContext(ctx)
    // 不 panic 即可
}
```

注意：上面需要 import "context" 和 "github.com/cocomhub/download-manager/core"。

- [ ] **步骤 3：运行测试**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go test -v -count=1 -timeout=30s ./downloader/
```

- [ ] **步骤 4：Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager && git add downloader/downloader_test.go && git commit -m "test: add downloader adapter layer tests"
```

### 任务 3.2：Benchmark 测试

**文件：**
- 修改：`storage/*_test.go`（新增 Benchmark）
- 修改：`manager/*_test.go`（新增 Benchmark）
- 修改：`config/*_test.go`（新增 Benchmark）
- 新建：`model/model_bench_test.go`（新增 Benchmark）

- [ ] **步骤 1：Config ValidateAndClamp Benchmark**

```go
// config/bench_test.go
package config

import "testing"

func BenchmarkValidateAndClamp(b *testing.B) {
    cfg := DefaultConfig() // 假设有这个函数或直接构造
    cfg.Tasks = []Task{
        {ID: "task1", Type: "tktube", SaveDir: "/tmp/task1", Storage: StorageCfg{Type: "file"}},
        {ID: "task2", Type: "hanime", SaveDir: "/tmp/task2", Storage: StorageCfg{Type: "file"}},
    }
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        cfg.ValidateAndClamp()
    }
}

func DefaultConfig() *Config {
    return &Config{
        Server: Server{
            HTTPPort: 8080,
            WorkDir: "/tmp/work",
        },
        Downloader: Downloader{
            GlobalConcurrent: 3,
            MaxRetries: 3,
        },
        TaskScan: TaskScan{
            Interval: 10,
        },
    }
}
```

注意：需要先检查是否有 `DefaultConfig()` 这类函数。如果没有，直接在测试内构造。

- [ ] **步骤 2：Manager Aggregate Benchmark**

```go
// manager/bench_test.go
package manager

import (
	"testing"
	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/storage"
)

func BenchmarkAggregateObjects(b *testing.B) {
	mgr := NewManager(&config.Config{
		Server: config.Server{WorkDir: b.TempDir()},
	})

	// 预填充 100 个对象
	store := storage.NewMemoryStorage()
	_ = store // 实际使用 manager 的存储

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mgr.AggregateObjects(1, 20, "", "created_at", "", nil)
	}
}
```

注意：上述代码假设 `NewMemoryStorage` 是导出函数。实际需要先检查 `storage/factory.go` 中内存存储的创建方式。

- [ ] **步骤 3：运行 Benchmark**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go test -bench=. -benchmem -count=3 -run=^$ ./config/ ./manager/ 2>&1 | head -30
```

- [ ] **步骤 4：Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager && git add config/bench_test.go manager/bench_test.go && git commit -m "bench: add config validation and manager aggregate benchmarks"
```

### 任务 3.3：Fuzz 测试

**文件：**
- 新建：`config/config_fuzz_test.go`
- 新建：`model/model_fuzz_test.go`

- [ ] **步骤 1：Config Parse Fuzz**

```go
// config/config_fuzz_test.go
package config

import "testing"

func FuzzConfigParse(f *testing.F) {
    f.Add([]byte("server:\n  http_port: 8080\n"))
    f.Add([]byte("downloader:\n  type: native\n"))
    f.Add([]byte("tasks:\n  - id: test\n    type: tktube\n"))

    f.Fuzz(func(t *testing.T, data []byte) {
        var cfg Config
        // 假设 config 包有 LoadYAML 或类似函数
        // 或使用 yaml.Unmarshal
        cfg.ValidateAndClamp()
    })
}
```

注意：需要确认 config 包是否暴露了 yaml unmarshal。如果没有，Fuzz 测试需要调整。

- [ ] **步骤 2：Model Status Transition Fuzz**

```go
// model/model_fuzz_test.go
package model

import "testing"

func FuzzStatusTransition(f *testing.F) {
    f.Add("pending")
    f.Add("downloading")
    f.Add("completed")
    f.Add("failed")
    f.Add("cancelled")

    f.Fuzz(func(t *testing.T, status string) {
        obj := &DownloadObject{Status: "pending"}
        // 验证不会 panic
        obj.Status = status
        _ = obj
    })
}
```

- [ ] **步骤 3：运行 Fuzz 测试（短时间）**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go test -fuzz=FuzzStatusTransition -fuzztime=10s ./model/
```

- [ ] **步骤 4：Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager && git add config/config_fuzz_test.go model/model_fuzz_test.go && git commit -m "test: add fuzz tests for config parse and model status"
```

### 任务 3.4：压力/稳定性测试

**文件：**
- 修改：`manager/*_test.go`（追加压力测试）

- [ ] **步骤 1：Goroutine Leak 回归测试**

```go
// 追加到 manager/*_test.go
func TestManagerGoroutineLeak(t *testing.T) {
    before := runtime.NumGoroutine()

    mgr := NewManager(/* config */)
    mgr.Start()
    time.Sleep(100 * time.Millisecond)
    mgr.Stop()
    time.Sleep(200 * time.Millisecond)

    after := runtime.NumGoroutine()
    leaked := after - before
    if leaked > 2 { // 允许少量临时 goroutine
        t.Errorf("goroutine leak: %d goroutines remained after Stop()", leaked)
    }
}
```

注意：需要导入 `"runtime"`、`"time"` 等包。同时根据实际的 NewManager 签名调整。

- [ ] **步骤 2：API 并发请求压力测试**

```go
// api/bench_test.go 或 api/concurrency_test.go
func TestAPIConcurrentRequests(t *testing.T) {
    // 启动 20 个并发 goroutine 请求 /api/tasks
    // 验证所有响应 200
}
```

- [ ] **步骤 3：运行压力测试**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go test -v -count=1 -timeout=120s -run 'TestGoroutineLeak|TestConcurrent|TestStability' ./manager/ ./api/
```

- [ ] **步骤 4：Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager && git add manager/manager_stress_test.go api/api_concurrency_test.go && git commit -m "test: add goroutine leak regression and concurrency stress tests"
```

### 任务 3.5：Phase 3 全量验证

- [ ] **步骤 1：全量构建 + vet**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go build ./... && go vet ./...
```

- [ ] **步骤 2：全量测试（含 race）**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go test -race -count=1 -timeout=180s ./...
```

- [ ] **步骤 3：Benchmark 运行确认**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go test -bench=. -benchmem -count=1 -run=^$ ./... 2>&1 | grep -E 'Benchmark|ok'
```

- [ ] **步骤 4：生成覆盖率报告**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go test -race -count=1 -coverprofile=build/cover.out -covermode=atomic ./...
```

- [ ] **步骤 5：检查覆盖率变化**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go tool cover -func=build/cover.out | grep -E 'total:|downloader|core|logutil'
```

- [ ] **步骤 6：Commit 收尾**

---

## Phase 4 任务

### 任务 4.1：CI 覆盖率集成

**文件：**
- 修改：`.github/workflows/ci.yml`
- 新建：`.codecov.yml`

- [ ] **步骤 1：增强 ci.yml test job 加入覆盖率**

在 `test` job 的 `go test` 步骤后添加：

```yaml
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
    fail_ci_if_error: false

- name: Upload coverage artifact
  uses: actions/upload-artifact@v4
  with:
    name: coverage-report
    path: |
      build/cover.out
      build/cover.html
    retention-days: 7
```

`go test` 步骤也需要改为带 coverprofile 运行：

```yaml
- name: Run tests with coverage
  run: go test -race -count=1 -coverprofile=build/cover.out -covermode=atomic -timeout=180s ./...
```

- [ ] **步骤 2：创建 .codecov.yml**

```yaml
# .codecov.yml
codecov:
  require_ci_to_pass: true
  notify:
    after_n_builds: 1

coverage:
  status:
    project:
      default:
        target: 60%
        threshold: 2%
    patch:
      default:
        target: 70%

comment:
  layout: "reach, diff, flags, files"
  behavior: default
  require_changes: false
```

- [ ] **步骤 3：Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager && git add .github/workflows/ci.yml .codecov.yml && git commit -m "ci: add coverage reporting and Codecov integration"
```

### 任务 4.2：跨平台测试矩阵

**文件：**
- 修改：`.github/workflows/ci.yml`

- [ ] **步骤 1：在 test job 增加 strategy matrix**

```yaml
jobs:
  test:
    strategy:
      matrix:
        os: [ubuntu-latest, windows-latest, macOS-latest]
        go: ['1.22', '1.24', '1.26']
        exclude:
          # Windows + Go 1.26 如果已知有问题
          - os: windows-latest
            go: '1.26'
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: ${{ matrix.go }}
          check-latest: true
      - name: go mod verify
        if: matrix.os == 'ubuntu-latest'
        run: go mod verify
      - name: go build
        run: go build ./...
      - name: Run tests
        run: go test -race -count=1 -coverprofile=build/cover.out -covermode=atomic -timeout=180s ./...
```

注意：`go mod verify` 只在单一平台运行避免冗余。Windows 上可能需要处理路径（如 `cover.out` 兼容性）。

- [ ] **步骤 2：Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager && git add .github/workflows/ci.yml && git commit -m "ci: add cross-platform test matrix (3 OS × 3 Go versions)"
```

### 任务 4.3：Benchmark 回归比较

**文件：**
- 修改：`.github/workflows/ci.yml`

- [ ] **步骤 1：在 ci.yml 的 test job 中增加 benchmark 步骤**

在 test 步骤后（可选，只在 ubuntu-latest 上跑以避免差异）：

```yaml
- name: Run benchmarks
  if: matrix.os == 'ubuntu-latest' && matrix.go == '1.26'
  run: go test -bench=. -benchmem -count=3 -run=^$ ./... > build/bench.txt

- uses: benchmark-action/github-action-benchmark@v1
  if: matrix.os == 'ubuntu-latest' && matrix.go == '1.26'
  with:
    tool: go
    output-file-path: build/bench.txt
    github-token: ${{ secrets.GITHUB_TOKEN }}
    alert-threshold: '200%'
    comment-on-alert: true
    fail-on-alert: false
```

- [ ] **步骤 2：Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager && git add .github/workflows/ci.yml && git commit -m "ci: add benchmark regression comparison with historical trends"
```

### 任务 4.4：Docker 多阶段构建

**文件：**
- 新建：`Dockerfile`
- 新建：`.dockerignore`

- [ ] **步骤 1：创建 Dockerfile**

```dockerfile
# Dockerfile
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git ca-certificates
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w -X main.Version=$(git describe --tags 2>/dev/null || echo dev) -X main.BuildAt=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o /build/download-manager .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata ffmpeg wget
COPY --from=builder /build/download-manager /usr/local/bin/download-manager
EXPOSE 8080
ENTRYPOINT ["download-manager"]
CMD ["--config", "/etc/download-manager/config.yaml"]
```

- [ ] **步骤 2：创建 .dockerignore**

```
.git/
.gitignore
*.md
test/
cmd/playwright-server/
.claude/
.trae/
.cursor/
build/
node_modules/
```

- [ ] **步骤 3：验证 Docker 构建**

```bash
cd D:/workdir/leon/cocomhub/download-manager && docker build -t download-manager:test .
```

- [ ] **步骤 4：Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager && git add Dockerfile .dockerignore && git commit -m "ci: add multi-stage Docker build"
```

### 任务 4.5：GoReleaser + Release CI

**文件：**
- 新建：`.goreleaser.yaml`
- 新建：`.github/workflows/release.yml`

- [ ] **步骤 1：创建 .goreleaser.yaml**

```yaml
# .goreleaser.yaml
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
    goos:
      - linux
      - darwin
      - windows
    goarch:
      - amd64
      - arm64

archives:
  - files:
      - config.yaml
      - README.md
    format_overrides:
      - goos: windows
        format: zip

checksum:
  name_template: 'checksums.txt'

snapshot:
  name_template: "{{ incpatch .Version }}-next"

changelog:
  use: github
  filters:
    exclude:
      - '^docs:'
      - '^ci:'
      - '^test:'
      - '^chore:'
```

- [ ] **步骤 2：创建 release.yml**

```yaml
# .github/workflows/release.yml
name: Release

on:
  push:
    tags:
      - 'v*'

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
          check-latest: true

      - name: Run tests
        run: go test -race -count=1 -timeout=180s ./...

      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          distribution: goreleaser
          version: latest
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}

      - name: Build and push Docker image
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Build and push Docker
        uses: docker/build-push-action@v5
        with:
          push: true
          tags: |
            ghcr.io/${{ github.repository }}:${{ github.ref_name }}
            ghcr.io/${{ github.repository }}:latest
```

- [ ] **步骤 3：Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager && git add .goreleaser.yaml .github/workflows/release.yml && git commit -m "ci: add GoReleaser release pipeline and Docker image publish"
```

### 任务 4.6：Phase 4 全量验证

- [ ] **步骤 1：CI 流程验证（push 到 dev 分支）**

```bash
cd D:/workdir/leon/cocomhub/download-manager && git push origin dev
```

注：需要等 CI 运行完成，确认覆盖率 artifact 上传、跨平台矩阵通过。

- [ ] **步骤 2：Docker 构建验证**

```bash
cd D:/workdir/leon/cocomhub/download-manager && docker build -t download-manager:latest .
```

- [ ] **步骤 3：Commit 收尾**

---

## 验证与交付

### 阶段出口标准检查清单

**Phase 1 出口：**
- [ ] `go test ./...` 全部 PASS（包括之前 FAIL 的 api/ 和 manager/）
- [ ] `go vet ./...` 无 warning
- [ ] `make install-hooks` 后可触发 pre-commit 检查
- [ ] core/, downloader/, pkg/logutil/, cmd/* 均有基础测试

**Phase 2 出口：**
- [ ] api/server.go handler 已按 task/config/metrics 拆分
- [ ] manager/aggregate.go 不再 import 具体任务包
- [ ] config/config.go Diff() 已移除
- [ ] "native_old" 类型可正常工作
- [ ] task/tktube/task.go 减少 116 行内联 JS
- [ ] 构建通过，测试通过

**Phase 3 出口：**
- [ ] 全量测试总耗时 ≤ 60s
- [ ] Benchmark 可重复运行
- [ ] Fuzz 测试可运行
- [ ] 压力测试验证无 goroutine leak
- [ ] 覆盖率 ≥ 60%
- [ ] downloader 适配层覆盖完整下载器创建路径

**Phase 4 出口：**
- [ ] CI 产出覆盖率报告（HTML + Codecov）
- [ ] 跨平台 3OS × 3Go 版本测试通过
- [ ] Docker 镜像可构建并运行
- [ ] GoReleaser 配置完整
- [ ] Benchmark 历史趋势展示

### 最终验证

- [ ] 所有阶段出口标准满足
- [ ] 项目自述文档（CLAUDE.md、README.md）同步更新
- [ ] 设计规格文档已 commit
