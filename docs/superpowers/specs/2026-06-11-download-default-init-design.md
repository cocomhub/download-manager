# pkg/download 包惰性初始化设计

## 问题

`pkg/download` 包的全局函数 `Get()` 依赖 `SetDefault()` 前置调用才能使用。当调用方（如 `manager/small_object.go` 中的小对象下载）忘记注册全局实例时，会得到 "default downloader not initialized" 错误。这不符合 Go "make the zero value useful" 的设计哲学。

同时，`New()` 虽然签名为零参数，但实际创建出来的 `Downloader` 没有注册任何 `Extractor` 和 `Transport`，无法直接用于下载，需要调用方一一注入。

## 设计目标

1. **`New()` 零参数即可用** — 不传 Option 也能得到一个能立刻下载的 `Downloader`
2. **`Get()` 惰性初始化** — 无需 `SetDefault()` 前置调用，首次调用自动创建
3. **Option 可覆盖** — 调用方可传入 `WithTransport`/`WithExtractor`/`WithSelector` 替换默认组件
4. **最通用的实现移到根包** — 将 `extractor.HTTPExtractor` 和 `transport.StdlibTransport` 作为包级默认，子包保留专用实现
5. **向后兼容** — 现有 API 签名不变，存量调用方无需修改

## 方案

**方案 A（推荐）：** 将最通用的实现从子包移到 `download` 包根目录。`New()` 自动注册默认组件。全局 `Default()` 函数惰性创建下载器实例。

### 文件结构变化

```
pkg/download/
├── download.go          # Downloader 结构体 + New() + Default() + SetDefault() + Get()
├── http_extractor.go    # ← 从 extractor/http.go 移入 HTTPExtractor
├── transport_stdlib.go  # ← 从 transport/stdlib.go 移入 StdlibTransport
├── option.go            # WithExtractor, WithTransport, WithSelector, ...
├── extractor.go         # Extractor / Canceller 接口（不变）
├── transport.go         # Transport 接口（不变）
├── selector.go          # DefaultSelector（不变）
├── proxy_selector.go    # StaticProxySelector（不变）
├── ...                  # 其余文件不变
├── extractor/           # 保留：hls.go, wget.go
└── transport/           # 保留：sproxy.go
```

### New() 零参数行为

```go
func New(opts ...Option) *Downloader {
    d := &Downloader{
        transport:  NewStdlibTransport(),
        selector:   NewDefaultSelector(),
        extractors: []Extractor{NewHTTPExtractor()},
    }
    for _, opt := range opts {
        opt(d)
    }
    return d
}
```

- 默认 Transport：`StdlibTransport`（基于 `net/http`，带域名限流）
- 默认 Selector：`DefaultSelector`（按注册顺序匹配 Extractor）
- 默认 Extractor：`HTTPExtractor`（通用 HTTP 下载，支持 ETag/304/断点续传/MD5）

### 全局惰性初始化

```go
var (
    defaultMu sync.Mutex
    defaultDl *Downloader
)

func Default() *Downloader {
    defaultMu.Lock()
    defer defaultMu.Unlock()
    if defaultDl == nil {
        defaultDl = New()
    }
    return defaultDl
}

func SetDefault(d *Downloader) {
    defaultMu.Lock()
    defaultDl = d
    defaultMu.Unlock()
}

func Get(ctx context.Context, url, savePath string) error {
    req := &Request{
        URL:      url,
        SavePath: savePath,
        Metadata: make(map[string]string, 2),
    }
    return Default().Download(ctx, req)
}
```

### 全局 SetDefault 的集成更新

`downloader/downloader.go` 的 `newDownloaderFromConfig` 中，创建 `*download.Downloader` 后调用 `download.SetDefault(dl)`，用带完整配置（代理、域名限流、自定义 UA）的实例覆盖惰性创建的默认实例。

`manager/manager.go` 的 `UpdateConfig` 路径通过 `downloader.New()` 间接调用 `SetDefault`，无需额外修改。

### 向后兼容

- `extractor.NewHTTPExtractor()` → 改为 `download.NewHTTPExtractor()` — 签名不变，只是包路径变化
- `transport.NewStdlibTransport()` → 改为 `download.NewStdlibTransport()` — 同上
- 子包 `extractor/hls.go` 和 `extractor/wget.go` 的导入路径不变
- 子包 `transport/sproxy.go` 的导入路径不变
- `option.go` 现有 Option 函数签名不变
- 新增 `WithDefault()` 选项返回内置默认 Option 列表，方便在追加自定组件时保留默认

### 存量引用点更新

| 文件 | 当前 | 改为 |
|------|------|------|
| `downloader/downloader.go` | `transport.NewStdlibTransport()` | `download.NewStdlibTransport()` |
| `downloader/adapter.go` | `extractor.NewHTTPExtractor` | `download.NewHTTPExtractor` |
| `downloader/adapter.go` | `extractor.NewCompositeExtractor` | 不变（保留在 extractor 子包） |
| `pkg/download/download_test.go` | `import extractor` + `extractor.NewHTTPExtractor()` | `download.NewHTTPExtractor()` |
| `pkg/download/download_test.go` | `import transport` + `transport.NewStdlibTransport()` | `download.NewStdlibTransport()` |

### 子包残留文件处理

`extractor/http.go` 和 `extractor/composite.go` 在移到根包后，保留空的存根文件或直接删除。考虑到不破坏 import 引用，**直接删除**更干净（存量引用已在上一节更新）。

`transport/stdlib.go` 同理。

## 验证

1. `go test ./pkg/download/...` — 所有 download 包测试通过
2. `go test ./...` — 全量测试无回归
3. `go build ./...` — 编译通过
4. `go vet ./...` — 无警告
5. 手动验证：移除 `download.SetDefault()` 调用后 `download.Get()` 正常工作
