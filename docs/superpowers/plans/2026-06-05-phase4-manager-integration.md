# Phase 4：download-manager 接入新库 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 将现有的 `downloader/` 包（`NativeHTTPDownloader`、`WgetDownloader`）替换为使用 `pkg/download/` 新库，通过适配器桥接 `core.Downloader` 接口。

**架构：** 创建 `downloader/adapter.go` 适配器，将 `pkg/download.Downloader` 包装为 `core.Downloader` 接口。改造 `downloader/downloader.go` 工厂函数使用新库的 Option 模式构建下载器。移除 `NativeHTTPDownloader` 和 `WgetDownloader` 中的类型断言，改为通过适配器/工厂暴露配置。

**技术栈：** Go 1.26 标准库。`pkg/download/` 新库（无外部依赖）。`config.Downloader` 配置结构体保持不变。

---

## 文件结构

```
downloader/
├── adapter.go               # NEW: core.Downloader 适配器
├── composite.go             # NEW: 复合下载助手（从 native/wget 提取）
├── factory.go               # MODIFY: 改为使用 pkg/download 新库
├── native.go                # MARK DEPRECATED: 转发到新库
├── wget.go                  # MARK DEPRECATED: 转发到新库

manager/
├── manager.go               # MODIFY: 移除 NativeHTTPDownloader 类型断言
├── download.go              # MODIFY: 移除 SetContext 类型断言

config/
├── config.go                # MODIFY: 添加 Backend 字段（可选）
```

## 迁移策略

采用**适配器模式**逐步迁移，不破坏现有 Manager/Task 接口：

1. 创建适配器将 `pkg/download.Downloader` 包装为 `core.Downloader`
2. 更新工厂函数让 `config.Type == "native"` 时返回适配器
3. 迁移 `ApplyDomainLimits` 和 `SetContext` 断言到新的注入点
4. 逐步废弃旧实现，最终删除

---

### 任务 1：创建核心适配器（adapter.go）

**文件：**
- 创建：`downloader/adapter.go`

适配器是迁移的核心。它将 `pkg/download.Downloader` 包装为 `core.Downloader` 接口，处理 `*model.DownloadObject` → `*download.Request` 的映射。

- [ ] **步骤 1：查看现有 `core.Downloader` 接口和 `model.DownloadObject`**

```go
// core/interfaces.go
type Downloader interface {
    Download(obj *model.DownloadObject, headers map[string]string) error
    Name() string
}

// model/object.go
type DownloadObject struct {
    TaskID   string
    URL      string
    SavePath string
    Metadata map[string]string
    Extra    map[string]any
    Status   string
    Progress int
    mu       sync.RWMutex
}
func (o *DownloadObject) SetProgress(p int)
func (o *DownloadObject) GetProgress() int
```

- [ ] **步骤 2：创建 `downloader/adapter.go`**

```go
package downloader

import (
    "context"
    "fmt"
    "log/slog"

    "github.com/cocomhub/download-manager/core"
    "github.com/cocomhub/download-manager/model"
    "github.com/cocomhub/download-manager/pkg/download"
)

// compile-time interface check
var _ core.Downloader = (*DownloaderAdapter)(nil)

// DownloaderAdapter 将 pkg/download.Downloader 包装为 core.Downloader。
type DownloaderAdapter struct {
    dl      *download.Downloader
    dlCtx   context.Context // 由 Manager 通过 SetContext 注入
    transport download.Transport // 用于 ApplyDomainLimits
}

// NewDownloaderAdapter 创建适配器。
func NewDownloaderAdapter(dl *download.Downloader) *DownloaderAdapter {
    return &DownloaderAdapter{dl: dl}
}

func (a *DownloaderAdapter) Name() string { return "adapter" }

// SetContext 设置下载上下文（替代旧的 NativeHTTPDownloader.SetContext）。
func (a *DownloaderAdapter) SetContext(ctx context.Context) { a.dlCtx = ctx }

// ApplyDomainLimits 设置域名并发限制（通过 StdlibTransport）。
func (a *DownloaderAdapter) ApplyDomainLimits(limits map[string]int) {
    if tr, ok := a.dl.Transport().(*transport.StdlibTransport); ok {
        tr.SetDomainLimits(limits)
    }
}

// Download 实现 core.Downloader 接口。
func (a *DownloaderAdapter) Download(obj *model.DownloadObject, headers map[string]string) error {
    ctx := a.dlCtx
    if ctx == nil {
        ctx = context.Background()
    }

    // 构建 download.Request
    req := &download.Request{
        URL:      obj.URL,
        SavePath: obj.SavePath,
        Headers:  headers,
        Metadata: obj.Metadata,
        OnProgress: func(progress float64, downloaded, total int64) {
            obj.SetProgress(int(progress))
        },
    }

    return a.dl.Download(ctx, req)
}
```

- [ ] **步骤 3：验证编译**

运行：`go build ./downloader/...`
预期：PASS

- [ ] **步骤 4：Commit**

```bash
git add downloader/adapter.go
git commit -m "feat(downloader): add DownloaderAdapter bridging pkg/download to core.Downloader"
```

---

### 任务 2：改造工厂函数（factory.go）

**文件：**
- 修改：`downloader/downloader.go`

更新工厂函数，当 `config.Type == "native"` 时使用新库创建下载器。

- [ ] **步骤 1：更新 `downloader/downloader.go`**

```go
package downloader

import (
    "log/slog"

    "github.com/cocomhub/download-manager/config"
    "github.com/cocomhub/download-manager/core"
    "github.com/cocomhub/download-manager/pkg/download"
    "github.com/cocomhub/download-manager/pkg/download/extractor"
    "github.com/cocomhub/download-manager/pkg/download/transport"
)

// New 创建 core.Downloader 实例。
// 根据 config.Type 选择后端：
//   - "wget": 使用旧的 WgetDownloader
//   - "native" 或默认: 使用新的 pkg/download.Downloader（通过适配器）
func New(cfg config.Downloader) core.Downloader {
    switch cfg.Type {
    case "wget":
        slog.Warn("wget backend is deprecated, use native instead")
        return NewWgetDownloader(cfg)
    default:
        return newDownloaderFromConfig(cfg)
    }
}

// newDownloaderFromConfig 从配置构建新的 pkg/download 下载器。
func newDownloaderFromConfig(cfg config.Downloader) *DownloaderAdapter {
    // 创建 StdlibTransport
    tr := transport.NewStdlibTransport()
    if len(cfg.DomainLimits) > 0 {
        tr.SetDomainLimits(cfg.DomainLimits)
    }

    // 创建代理选择器
    var sel download.Selector
    if len(cfg.Proxies) > 0 {
        ps := download.NewStaticProxySelector(cfg.Proxies, cfg.ForceProxy)
        sel = download.NewDefaultSelector().WithProxySelector(ps)
    }

    // 创建下载器
    opts := []download.Option{
        download.WithTransport(tr),
        download.WithExtractor(extractor.NewHTTPExtractor()),
        download.WithExtractor(extractor.NewHLSExtractor()),
    }
    if sel != nil {
        opts = append(opts, download.WithSelector(sel))
    }

    dl := download.New(opts...)
    adapter := NewDownloaderAdapter(dl)

    // 注入传输层引用（用于 ApplyDomainLimits）
    adapter.transport = tr

    return adapter
}
```

- [ ] **步骤 2：验证编译和测试**

运行：`go build ./downloader/... && go test ./manager/...`
预期：PASS

- [ ] **步骤 3：Commit**

```bash
git add downloader/downloader.go
git commit -m "feat(downloader): update factory to use pkg/download for native backend"
```

---

### 任务 3：移除 Manager 中的类型断言

**文件：**
- 修改：`manager/manager.go:149-151`
- 修改：`manager/download.go:89-91`

Manager 目前使用 `dl.(*downloader.NativeHTTPDownloader)` 类型断言来注入上下文和域名限制。适配器提供了 `SetContext` 和 `ApplyDomainLimits` 方法，但需要接口化以便在不依赖具体类型的情况下安全调用。

- [ ] **步骤 1：在 `core/interfaces.go` 中定义接口**

```go
// core/interfaces.go — 新增接口

// DownloaderWithContext 表示支持上下文注入的下载器。
type DownloaderWithContext interface {
    SetContext(ctx context.Context)
}

// DownloaderWithDomainLimits 表示支持域名限制的下载器。
type DownloaderWithDomainLimits interface {
    ApplyDomainLimits(limits map[string]int)
}
```

- [ ] **步骤 2：修改 `manager/manager.go` 中的断言**

```go
// manager/manager.go:149-151 — 替换类型断言
if dl, ok := mgr.downloader.(core.DownloaderWithDomainLimits); ok {
    dl.ApplyDomainLimits(cfg.Downloader.DomainLimits)
}
```

- [ ] **步骤 3：修改 `manager/download.go` 中的断言**

```go
// manager/download.go:89-91 — 替换类型断言
if nd, ok := dl.(core.DownloaderWithContext); ok {
    nd.SetContext(dlCtx)
}
```

- [ ] **步骤 4：验证编译和测试**

运行：`go build ./...`
预期：PASS

- [ ] **步骤 5：Commit**

```bash
git add core/interfaces.go manager/manager.go manager/download.go
git commit -m "refactor(manager): replace NativeHTTPDownloader type assertions with core interfaces"
```

---

### 任务 4：迁移复合下载逻辑

**文件：**
- 创建：`downloader/composite.go`

复合下载目前嵌入在 `native.go` 和 `wget.go` 中。新库的 `extractor.CompositeExtractor` 已实现此功能。但 Manager 的旧流程中复合下载仍然通过旧的下载器触发。

这个任务提取公共的复合下载助手函数，让旧的 `NativeHTTPDownloader` 和 `WgetDownloader` 复用，作为过渡方案。

- [ ] **步骤 1：创建 `downloader/composite.go`**

```go
package downloader

import (
    "fmt"
    "log/slog"
    "os"
    "path/filepath"

    "github.com/cocomhub/download-manager/model"
)

// parseCompositeFiles 从 obj.Extra["files"] 解析文件列表。
// 统一处理 []map[string]string / []any 两种来源。
func parseCompositeFiles(obj *model.DownloadObject) ([]map[string]string, error) {
    // 逻辑从 native.go 和 wget.go 提取...
}

// DownloadComposite 执行复合下载，遍历 files 列表调用下载函数。
func DownloadComposite(ctx, files, headers, dlFn) error {
    // ...
}
```

- [ ] **步骤 3：Commit**

```bash
git add downloader/composite.go
git commit -m "feat(downloader): extract composite download helper from native/wget"
```

---

### 任务 5：标记废弃 + 全量回归

**文件：**
- 修改：`downloader/native.go` — 文件顶部添加 `// Deprecated` 注释
- 修改：`downloader/wget.go` — 文件顶部添加 `// Deprecated` 注释

- [ ] **步骤 1：全量回归测试**

```bash
go build ./...
go vet ./...
go test ./...
```

- [ ] **步骤 2：Commit**

```bash
git add -A && git commit -m "chore: mark native.go and wget.go as deprecated after Phase 4 migration"
```

---

## 验证

```bash
go build ./...
go vet ./...
go test ./...
```