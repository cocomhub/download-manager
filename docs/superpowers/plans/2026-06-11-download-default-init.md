# pkg/download 惰性初始化 + 默认组件实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** `New()` 零参数即可用 + `Get()` 惰性初始化，无需 `SetDefault()` 前置调用。

**架构：** 将 `extractor.HTTPExtractor` 和 `transport.StdlibTransport` 移到 `download` 包根目录作为内置默认组件。`New()` 自动注册默认值，`Default()` 惰性创建实例，`downloader.New()` 用完整配置覆盖全局默认。

**技术栈：** Go 1.26 + 标准库 net/http + slog

---

## 文件变更清单

### 创建（移入根包）
- `pkg/download/http_extractor.go` — 从 `pkg/download/extractor/http.go` 移入，去掉 `"github.com/cocomhub/download-manager/pkg/download"` 导入（自引用）
- `pkg/download/transport_stdlib.go` — 从 `pkg/download/transport/stdlib.go` 移入，去掉自引用导入
- `pkg/download/http_extractor_test.go` — 从 `pkg/download/extractor/http_test.go` 移入，改为 `package download_test`

### 修改
- `pkg/download/download.go` — `New()` 添加默认组件 + `Default()` 惰性初始化
- `pkg/download/option.go` — 新增 `WithDefaults()` 辅助选项
- `downloader/downloader.go` — 引用路径更新
- `downloader/adapter.go` — 引用路径更新 + 移除 `transport` 导入
- `pkg/download/download_test.go` — 移除 `extractor` + `transport` 导入，直接使用 `download.NewHTTPExtractor()`

### 删除
- `pkg/download/extractor/http.go` — 内容已移入根包
- `pkg/download/transport/stdlib.go` — 内容已移入根包
- `pkg/download/extractor/http_test.go` — 内容已移入根包

### 测试文件（不变）
- `pkg/download/extractor/composite_test.go` — CompositeExtractor 保留在 extractor 子包，不变
- `pkg/download/extractor/hls_test.go` — HLSExtractor 保留在 extractor 子包，不变
- `pkg/download/extractor/wget_test.go` — WgetExtractor 保留在 extractor 子包，不变

---

### 任务 1：将 HTTPExtractor 移到根包

**文件：**
- 创建：`pkg/download/http_extractor.go`
- 删除：`pkg/download/extractor/http.go`

- [ ] **步骤 1：创建 `pkg/download/http_extractor.go`**

从 `pkg/download/extractor/http.go` 复制全部内容，做以下修改：
- 去掉 `package extractor` → `package download`
- 去掉 `"github.com/cocomhub/download-manager/pkg/download"` 导入（自引用）
- 所有 `download.SomeType` → `SomeType`（同包引用无需前缀）

```go
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// HTTPExtractor 是通用 HTTP 文件下载编排器。
// 它使用 Transport 做字节传输，自己管理重试、断点续传、MD5 校验。
type HTTPExtractor struct {
	transport  Transport
	selector   Selector
	maxRetries int
	rootDir    string
	logDir     string
	ua         string
}

// NewHTTPExtractor 创建并返回 HTTPExtractor 实例。
func NewHTTPExtractor() *HTTPExtractor {
	return NewHTTPExtractorWithConfig(5, "", "", "")
}

// NewHTTPExtractorWithConfig 根据配置创建 HTTPExtractor 实例。
func NewHTTPExtractorWithConfig(maxRetries int, userAgent, rootDir, logDir string) *HTTPExtractor {
	if userAgent == "" {
		userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36"
	}
	return &HTTPExtractor{
		maxRetries: maxRetries,
		rootDir:    rootDir,
		logDir:     logDir,
		ua:         userAgent,
	}
}

// SetTransport, SetSelector, Name, Match, Extract, tryDownload, buildHeaders — 内容与源文件完全一致，仅去掉 "download." 前缀
func (e *HTTPExtractor) SetTransport(t Transport) { e.transport = t }
func (e *HTTPExtractor) SetSelector(s Selector)  { e.selector = s }
func (e *HTTPExtractor) Name() string            { return "http" }
func (e *HTTPExtractor) Match(ctx context.Context, url string) bool {
	return !strings.Contains(strings.ToLower(url), ".m3u8")
}
// Extract, tryDownload, buildHeaders — 复制自 extractor/http.go，所有 download.xxx 去掉前缀
```

**关键替换清单（源文件中的 `download.` → 去掉）：**
- `download.Transport` → `Transport`
- `download.Selector` → `Selector`
- `download.Request` → `Request`
- `download.TransportRequest` → `TransportRequest`
- `download.TransportResponse` → `TransportResponse`
- `download.ResolveAction` → `ResolveAction`
- `download.ComputeFileMD5` → `ComputeFileMD5`
- `download.NewProgressReader` → `NewProgressReader`
- `download.ComposeProgress` → `ComposeProgress`
- `download.NewProgressLogCallback` → `NewProgressLogCallback`
- `download.ProgressLogOption` → `ProgressLogOption`
- `download.ErrNoTry` → `ErrNoTry`
- `download.ResolvePath` → `ResolvePath`
- `download.DownloadResult` → `DownloadResult`
- `download.RangeRequest` → `RangeRequest`
- `download.TryGetMd5` → `TryGetMd5`
- `download.DownloadHint` → `DownloadHint`
- `download.DomainLimiter` → `DomainLimiter`
- `download.NewDomainLimiter` → `NewDomainLimiter`
- `download.WithLogWriter` → `WithLogWriter`
- `download.WithMinPercentStep` → `WithMinPercentStep`
- `download.WithMaxInterval` → `WithMaxInterval`

- [ ] **步骤 2：编译验证**

运行：`go build ./pkg/download/...`
预期：编译成功，无错误

- [ ] **步骤 3：删除源文件**

```bash
git rm pkg/download/extractor/http.go
```

- [ ] **步骤 4：全量编译验证**

运行：`go build ./...`
预期：编译成功（如果失败说明被其他文件引用，继续下一步）

- [ ] **步骤 5：Commit**

```bash
git add pkg/download/http_extractor.go
git rm pkg/download/extractor/http.go
git commit -m "refactor(download): move HTTPExtractor to root package"
```

---

### 任务 2：将 StdlibTransport 移到根包

**文件：**
- 创建：`pkg/download/transport_stdlib.go`
- 删除：`pkg/download/transport/stdlib.go`

- [ ] **步骤 1：创建 `pkg/download/transport_stdlib.go`**

从 `pkg/download/transport/stdlib.go` 复制全部内容，做以下修改：
- `package transport` → `package download`
- 去掉 `"github.com/cocomhub/download-manager/pkg/download"` 导入（自引用）
- 所有 `download.` 前缀去掉

```go
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// StdlibTransport 是基于标准库 net/http 的 Transport 实现。
type StdlibTransport struct {
	client   *http.Client
	dLimiter *DomainLimiter
}

// NewStdlibTransport 创建并返回一个 StdlibTransport 实例。
func NewStdlibTransport() *StdlibTransport {
	return &StdlibTransport{
		client: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     30 * time.Second,
			},
		},
		dLimiter: NewDomainLimiter(),
	}
}

// Name, RoundTrip, SetDomainLimits — 复制自 transport/stdlib.go，去掉 "download." 前缀
func (t *StdlibTransport) Name() string { return "stdlib" }
// RoundTrip, SetDomainLimits 同上
```

- [ ] **步骤 2：编译验证**

运行：`go build ./pkg/download/...`
预期：编译成功

- [ ] **步骤 3：删除源文件**

```bash
git rm pkg/download/transport/stdlib.go
```

- [ ] **步骤 4：全量编译验证**

运行：`go build ./...`
预期：编译成功

- [ ] **步骤 5：Commit**

```bash
git add pkg/download/transport_stdlib.go
git rm pkg/download/transport/stdlib.go
git commit -m "refactor(download): move StdlibTransport to root package"
```

---

### 任务 3：实现 New() 默认组件 + Default() 惰性初始化

**文件：**
- 修改：`pkg/download/download.go`

- [ ] **步骤 1：编写测试 `pkg/download/download_test.go` — 新增默认行为测试**

在 `download_test.go` 中添加测试（`package download_test`）：

```go
func TestNewDefault(t *testing.T) {
	// New() 零参数应产生一个可用的下载器
	d := download.New()
	if d == nil {
		t.Fatal("New() returned nil")
	}
	// 默认应包含 HTTPExtractor
	if err := d.Download(context.Background(), &download.Request{
		URL:      "http://example.com/file",
		SavePath: "test/out.bin",
	}); err == nil {
		t.Error("expected error (no network), but got nil")
	} else {
		// 报错应该是网络/IO 层面的，而不是 "no extractor found" 或 "transport not set"
		if err == download.ErrNoDefaultDownloader ||
			strings.Contains(err.Error(), "no extractor found") ||
			strings.Contains(err.Error(), "transport not set") {
			t.Fatalf("unexpected initialization error: %v", err)
		}
	}
}

func TestGetLazyInit(t *testing.T) {
	// 确保全局未初始化
	download.SetDefault(nil)

	// Get 应自动创建默认下载器
	err := download.Get(context.Background(), "http://example.com/file", "test/out.bin")
	// 因为无网络连接，预期网络层面错误而非 "not initialized"
	if err == download.ErrNoDefaultDownloader ||
		err == nil ||
		strings.Contains(err.Error(), "no extractor found") ||
		strings.Contains(err.Error(), "transport not set") {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Logf("expected network error (lazy init works): %v", err)
}
```

- [ ] **步骤 2：运行测试确认失败**

运行：`go test -v -run 'TestNewDefault|TestGetLazyInit' ./pkg/download/`
预期：编译失败 — `download.New()` 的默认行为不符合预期

- [ ] **步骤 3：修改 `New()` 添加默认组件**

```go
// New 创建 Downloader，可通过 Option 自定义配置。
// 零参数调用时自动注册 HTTPExtractor、StdlibTransport、DefaultSelector。
func New(opts ...Option) *Downloader {
	d := &Downloader{
		transport:  NewStdlibTransport(),
		selector:   NewDefaultSelector(),
		extractors: []Extractor{NewHTTPExtractor()},
	}
	for _, o := range opts {
		o(d)
	}
	return d
}
```

- [ ] **步骤 4：添加 `Default()` 惰性初始化**

```go
// defaultDl 是包级默认 Downloader 实例，通过 SetDefault 或惰性初始化。
var (
	defaultDl   *Downloader
	defaultDlMu sync.RWMutex
)

// SetDefault 替换包级默认 Downloader 实例。
func SetDefault(d *Downloader) {
	defaultDlMu.Lock()
	defaultDl = d
	defaultDlMu.Unlock()
}

// Default 返回包级默认 Downloader。
// 首次调用时若未初始化，自动创建一个零参数 New() 实例。
func Default() *Downloader {
	defaultDlMu.RLock()
	if defaultDl != nil {
		defaultDlMu.RUnlock()
		return defaultDl
	}
	defaultDlMu.RUnlock()

	// 惰性创建（double-check 风格）
	defaultDlMu.Lock()
	defer defaultDlMu.Unlock()
	if defaultDl == nil {
		defaultDl = New()
	}
	return defaultDl
}

// Get 使用默认 Downloader 执行一次简单下载。
// 若默认实例未初始化，自动创建（等效 Default().Download(...)）。
func Get(ctx context.Context, url, savePath string) error {
	return Default().Download(ctx, &Request{
		URL:      url,
		SavePath: savePath,
	})
}
```

删除旧的 `ErrNoDefaultDownloader` 变量（不再使用）。
保留 `ErrNoDefaultDownloader` 作为导出错误但标记为废弃（或直接删除，看是否被外部引用）。

检查是否有外部引用：
```
Grep: ErrNoDefaultDownloader → 只在 download.go 自身定义 + download_test.go 中引用
```
可以安全删除，但保留为兼容。改为：

```go
// ErrNoDefaultDownloader 保留用于向后兼容，但 Default() 惰性初始化后不再触发此错误。
var ErrNoDefaultDownloader = errors.New("default downloader not initialized")
```

- [ ] **步骤 5：更新 `downloader/downloader.go` 中的引用**

将 `downloader/downloader.go` 中已有的 `download.SetDefault(dl)` 保留（它会在配置加载后覆盖默认实例，带上代理/域名限流等配置）。

- [ ] **步骤 6：运行测试验证通过**

运行：`go test -v -run 'TestNewDefault|TestGetLazyInit|TestDefaultNilBeforeSet|TestGetReturnsErrorBeforeSet|TestSetDefaultAndGet' ./pkg/download/`
预期：全部 PASS

- [ ] **步骤 7：Commit**

```bash
git add pkg/download/download.go pkg/download/download_test.go
git commit -m "feat(download): New() zero-arg defaults + Default() lazy init"
```

---

### 任务 4：更新存量引用路径

**文件：**
- 修改：`downloader/downloader.go`
- 修改：`downloader/adapter.go`
- 修改：`pkg/download/download_test.go`（已有导入清理）

- [ ] **步骤 1：更新 `downloader/downloader.go`**

将：
```go
import (
    "github.com/cocomhub/download-manager/pkg/download/extractor"
    "github.com/cocomhub/download-manager/pkg/download/transport"
)
```
改为：
```go
import (
    "github.com/cocomhub/download-manager/pkg/download/extractor"
    // transport 导入可以删除
)
// 注意：extractor 导入保留因为还使用 extractor.NewHLSExtractor(...)
```

将：
```go
tr := transport.NewStdlibTransport()
```
改为：
```go
tr := download.NewStdlibTransport()
```

将：
```go
httpEx := extractor.NewHTTPExtractorWithConfig(cfg.MaxRetries, userAgent, cfg.Filesystem.RootDir, cfg.Filesystem.LogDir)
```
改为：
```go
httpEx := download.NewHTTPExtractorWithConfig(cfg.MaxRetries, userAgent, cfg.Filesystem.RootDir, cfg.Filesystem.LogDir)
```

- [ ] **步骤 2：更新 `downloader/adapter.go`**

将：
```go
import (
    "github.com/cocomhub/download-manager/pkg/download/transport"
)
```
删除 `transport` 导入。

将适配器中的 StdlibTransport 类型断言：
```go
if tr, ok := a.transport.(*transport.StdlibTransport); ok {
```
改为：
```go
if tr, ok := a.transport.(*download.StdlibTransport); ok {
```

- [ ] **步骤 3：更新 `pkg/download/download_test.go`**

移除 `extractor` 和 `transport` 导入：
```go
// 删除：
"github.com/cocomhub/download-manager/pkg/download/extractor"
"github.com/cocomhub/download-manager/pkg/download/transport"
```

修改测试中的引用：
- `extractor.NewHTTPExtractor()` → `download.NewHTTPExtractor()`
- `transport.NewStdlibTransport()` → `download.NewStdlibTransport()`

更新 `TestDownloaderNoExtractor` 测试 —— `New()` 现在有默认 HTTPExtractor，所以空参数不再报 "no extractor"，需要传一个匹配不了的 URL 或使用 Hint 来触发提取器缺失：
```go
func TestDownloaderNoExtractor(t *testing.T) {
	// New() 带默认 HTTPExtractor，需要 URL 不匹配任何 extractor 才会报 no extractor
	// HTTPExtractor 只不匹配 .m3u8 结尾的 URL，所以用任意非 m3u8 URL 会匹配
	d := download.New()
	err := d.Download(context.Background(), &download.Request{
		URL:      "http://example.com/file",
		SavePath: "/tmp/file",
	})
	// 现在 New() 有默认组件，应有网络层面错误而非 "no extractor"
	if err == nil {
		t.Error("expected error")
	}
	if strings.Contains(err.Error(), "no extractor found") {
		t.Error("HTTPExtractor should match this URL")
	}
}
```

更新 `TestDownloaderWithExtractor` — 不再需要显式添加 extractor（默认已有），改为测试自定义 extractor 追加：
```go
func TestDownloaderWithExtractor(t *testing.T) {
	// New() 默认已有 HTTPExtractor，追加自定义 extractor
	mock := &mockExtractor{name: "mock"}
	d := download.New(
		download.WithExtractor(mock),
	)
	ext := download.NewHTTPExtractor()
	_ = ext // just verify the constructor works from root package
	// ...
}
```

- [ ] **步骤 4：编译验证**

运行：`go build ./...`
预期：编译成功，无错误

- [ ] **步骤 5：运行全量测试**

运行：`go test ./...`
预期：全部 PASS

- [ ] **步骤 6：Commit**

```bash
git add downloader/downloader.go downloader/adapter.go pkg/download/download_test.go
git commit -m "fix(download): update import paths after moving HTTPExtractor and StdlibTransport"
```

---

### 任务 5：迁移 HTTPExtractor 测试文件

**文件：**
- 创建：`pkg/download/http_extractor_test.go`
- 删除：`pkg/download/extractor/http_test.go`

- [ ] **步骤 1：创建 `pkg/download/http_extractor_test.go`**

从 `pkg/download/extractor/http_test.go` 复制全部内容，做以下修改：
- `package extractor_test` → `package download_test`
- 导入 `"github.com/cocomhub/download-manager/pkg/download/extractor"` → 改为 `"github.com/cocomhub/download-manager/pkg/download"`
- 导入 `"github.com/cocomhub/download-manager/pkg/download/transport"` → 删除（StdlibTransport 在根包）
- 所有 `extractor.NewHTTPExtractor()` → `download.NewHTTPExtractor()`
- 所有 `transport.NewStdlibTransport()` → `download.NewStdlibTransport()`
- 所有 `ext.NewTransport(...)` → `download.NewStdlibTransport()`

- [ ] **步骤 2：编译验证**

运行：`go build ./...`
预期：编译成功

- [ ] **步骤 3：运行测试验证通过**

运行：`go test -v -run TestHTTPExtractor ./pkg/download/`
预期：全部 PASS

- [ ] **步骤 4：删除原测试文件**

```bash
git rm pkg/download/extractor/http_test.go
```

- [ ] **步骤 5：全量测试**

运行：`go test ./...`
预期：全部 PASS

- [ ] **步骤 6：Commit**

```bash
git add pkg/download/http_extractor_test.go
git rm pkg/download/extractor/http_test.go
git commit -m "test(download): move HTTPExtractor test to root package"
```

---

### 任务 6：验证 + go vet

- [ ] **步骤 1：全量编译构建**

运行：`go build ./...`
预期：成功

- [ ] **步骤 2：运行全量测试**

运行：`go test ./...`
预期：全部 PASS

- [ ] **步骤 3：go vet**

运行：`go vet ./...`
预期：无警告

- [ ] **步骤 4：最终验证隔离性**

确认 `pkg/download/extractor/http.go` 和 `pkg/download/transport/stdlib.go` 已删除（`git ls-files --deleted` 显示它们被删除）。

确认 `git status` 结果干净（只剩期望的变更）。

- [ ] **步骤 5：Commit（若还有未提交的修改）**

```bash
git commit -m "chore: final cleanup after download package default init refactor"
```

## 验证方式

```bash
# 编译
go build ./...

# 全量测试
go test ./...

# vet
go vet ./...

# 独立验证：SetDefault(nil) 后 Get() 自动创建默认下载器
go test -v -run TestGetLazyInit ./pkg/download/
```
