# 通用下载库架构设计

**日期**: 2026-06-04
**状态**: 设计阶段

## 1. Context

download-manager 项目现有的下载层存在以下问题：

- **耦合度高**：Manager 通过类型断言 (`dl.(*downloader.NativeHTTPDownloader)`) 注入上下文和域名限制，违反 DIP
- **重复代码**：复合下载 `Extra["files"]` 解析在 native.go/wget.go 中完全重复
- **扩展性差**：`downloader/` 工厂通过 `config.Type` switch 选择后端，新增后端需改工厂函数
- **代理策略单一**：WgetDownloader 自建代理逻辑，与 dlcore 的 ProxySelector 重复
- **m3u8d 不可注入**：HTTP 客户端在内部硬编码创建，dlcore 的代理/Transport 无法传入
- **sproxy 集成仅 HTTP 代理**：无法利用 AES-256-GCM 加密隧道和文件 API
- **无路由规则**：下载器选择是进程级整体策略，不支持 URL 级灵活路由

## 2. 设计目标

| 目标 | 说明 |
|------|------|
| **使用简单** | 一行调用 `download.Get(ctx, url, dest)` 即可下载 |
| **完全可自定义** | 可注入自定义 Selector / Extractor / Transport，全量控制下载行为 |
| **多传输后端** | 支持 stdlib HTTP、wget、sproxy 加密隧道等多种传输方式 |
| **多下载策略** | HTTP 直下、HLS (ffmpeg/m3u8d)、复合下载、远端下载等 |
| **sproxy 集成** | 同时支持标准 HTTP 代理和加密隧道模式 |
| **可独立发布** | 放在 `pkg/download/` 但接口设计以独立项目为目标 |
| **保留向下兼容** | Manager API 响应格式不变、现有配置字段通过适配层兼容 |

## 3. 三层混合架构

```
┌──────────────────────────────────────────────────────┐
│              download.Downloader                      │
│  (持有 Selector + []Extractor + Transport 引用)       │
└──────────────────────┬───────────────────────────────┘
                       │
                  ┌────┴────┐
                  │ Selector│ ← 路由层：谁下载、走哪个代理
                  │         │
                  │ MatchExtractor(url, hint) → Extractor
                  │ SelectProxy(url, hint) → proxyURL
                  └──┬──────┘
                     │
        ┌────────────┼─────────────┬──────────────┐
        │            │             │              │
   ┌────▼───┐  ┌────▼────┐  ┌────▼────┐  ┌──────▼──────┐
   │  HTTP  │  │  HLS    │  │Composite│  │ Remote      │ ← 编排层
   │Extractor│  │Extractor│  │Extractor│  │Extractor    │   完整下载流程
   │        │  │         │  │         │  │(未来)       │
   │ 可用   │  │ ffmpeg/ │  │ 委托子  │  │ sproxy 隧道 │
   │Transport│  │ m3u8d  │  │ URL 给  │  │ 下发/回传   │
   └───┬────┘  └─────────┘  └─────────┘  └─────────────┘
       │
       │  ┌───────────┬───────────┬──────────────┐
       │  │           │           │              │
  ┌────▼┐ ┌─▼────────┐ ┌─▼────────┐ ┌─▼──────────┐
  │Stdlib│ │HTTP Proxy │ │sproxy   │ │sproxy File │ ← 传输层
  │Transp│ │Transport  │ │Tunnel   │ │Transport   │   可选
  │(默认)│ │           │ │Transport│ │(文件API)   │
  └──────┘ └──────────┘ └──────────┘ └────────────┘
```

### 3.1 分层职责

| 层次 | 角色 | 核心接口 | 谁用 |
|------|------|---------|------|
| **Selector** | 路由决策 | `MatchExtractor()` / `SelectProxy()` | Downloader 内部调用 |
| **Extractor** | 下载编排 | `Match()` / `Extract()` | 调用方直接注册 |
| **Transport** | 数据传输 | `RoundTrip()` | Extractor 内部使用（可选） |

### 3.2 核心设计原则

1. **Extractor 和 Transport 正交**：一个 Extractor（比如 HTTPExtractor）可以用任意 Transport（stdlib / wget / sproxy 隧道）；一个 Transport 可以被多个 Extractor 共用
2. **Transport 是可选的**：如果 Extractor 不依赖 HTTP 传输（如 ffmpeg 直接调命令），它不需要 Transport
3. **Extractor 也可自建传输**：m3u8d 内部可以接受注入的 `http.Client` 作为传输层，但它自己管理并发和重试

## 4. 核心接口定义

### 4.1 Request / Response

```go
package download

// Request 描述一次下载请求
type Request struct {
    URL           string                            // 下载地址
    SavePath      string                            // 保存路径
    Headers       map[string]string                 // 自定义请求头
    TrackProgress bool                              // 是否跟踪进度
    OnProgress    func(progress float64, downloaded, total int64)
    Metadata      map[string]string                 // 输入/输出元数据
    Hint          *DownloadHint                     // 可选的路由提示
}

// DownloadHint 为 Selector 提供额外决策信息
type DownloadHint struct {
    FileSize    int64
    ContentType string
    Extractor   string   // 可选：指定用哪个 Extractor
    Tags        map[string]string // 自定义标签
}

// TransportRequest 传输层请求
type TransportRequest struct {
    URL       string
    Method    string
    Headers   http.Header
    Body      io.Reader
    Range     *RangeRequest
    ProxyURL  string            // Selector 选定的代理
}

type RangeRequest struct {
    Offset int64
}

// TransportResponse 传输层响应
type TransportResponse struct {
    Body          io.ReadCloser
    StatusCode    int
    ContentLength int64
    Headers       http.Header
    ProxyURL      string      // 实际使用的代理
}
```

### 4.2 Selector（路由层）

```go
// Selector 负责两项路由决策：
// 1. 哪个 Extractor 来处理此 URL
// 2. 走哪个代理（直连 / HTTP 代理 / sproxy 隧道）
type Selector interface {
    MatchExtractor(ctx context.Context, url string, hint *DownloadHint) Extractor
    SelectProxy(ctx context.Context, targetURL string, hint *DownloadHint) (proxyURL string, err error)
}

// DefaultSelector 默认实现：先注册先匹配 + 域名级代理缓存
type DefaultSelector struct {
    extractors     []Extractor
    proxySelector  ProxySelector  // 内嵌代理选择策略
}

type ProxySelector interface {
    Select(ctx context.Context, targetURL string, hint *DownloadHint) (proxyURL string, err error)
}

// StaticProxySelector 静态代理列表 + 直连探测 + 带宽评分
// 从 dlcore/proxy_selector.go 迁移而来
type StaticProxySelector struct {
    proxies    []string
    forceProxy bool
    cache      ProxyCache
}

// TunnelProxySelector sproxy 隧道代理选择
// 对多个 sproxy 实例做延迟/带宽探测，选最优
type TunnelProxySelector struct {
    instances   []TunnelInstance
    probeConfig ProbeConfig
}
```

### 4.3 Extractor（编排层）

```go
// Extractor 负责一次完整下载的编排。
// 包含：路径创建 → 代理选择 → 数据传输 → 重试循环 → 校验 → 进度回调。
type Extractor interface {
    Name() string
    Match(ctx context.Context, url string) bool
    Extract(ctx context.Context, req *Request) error
}

// Extractor 可以通过以下接口获得 Downloader 的组件引用
type ExtractorWithTransport interface {
    SetTransport(t Transport)
}

type ExtractorWithSelector interface {
    SetSelector(s Selector)
}

// ——— 内置 Extractor ———

// HTTPExtractor 通用 HTTP 文件下载
// 内部使用 Transport 做字节传输，自己管理重试、断点续传、MD5 校验
type HTTPExtractor struct {
    transport Transport
    selector  Selector
    maxRetries int
}

// HLSExtractor HLS 流下载
// 内部使用 ffmpeg 或 m3u8d 库，不依赖 Transport
type HLSExtractor struct {
    mode    string   // "ffmpeg" | "m3u8d" | "auto"
    ffmpeg  string   // ffmpeg 路径
    m3u8d   *M3U8DEngine  // m3u8d 引擎（接受注入 http.Client）
}

// CompositeExtractor 复合下载
// 处理 Request.Metadata["files"]，递归委托子 URL 给 Selector
type CompositeExtractor struct {
    selector Selector
}
```

### 4.4 Transport（传输层）

```go
// Transport 只做一件事：发送请求，返回响应流。
type Transport interface {
    Name() string
    RoundTrip(ctx context.Context, req *TransportRequest) (*TransportResponse, error)
}

// ——— 内置 Transport ———

// StdlibTransport 使用 Go 标准 net/http 传输
// 支持连接池、超时、Cookie、重定向等标准配置
type StdlibTransport struct {
    client   *http.Client
    dLimiter *DomainLimiter   // 域名并发限制
}

// WgetTransport 使用系统 wget 命令传输
type WgetTransport struct {
    proxySelector ProxySelector
    logDir        string
    active        sync.Map   // URL → *exec.Cmd
}

// SproxyTunnelTransport 通过 sproxy 加密隧道传输
type SproxyTunnelTransport struct {
    clients    []*tunnel.Client
    selector   TunnelInstanceSelector
}

// SproxyFileTransport 通过 sproxy 文件 API 下载
type SproxyFileTransport struct {
    client *sproxyclient.FileClient
}
```

## 5. 入口：Downloader

```go
// Downloader 是用户使用的主要入口
type Downloader struct {
    selector   Selector
    extractors []Extractor
    transport  Transport
}

// New 创建 Downloader，默认提供 HTTPExtractor
func New(opts ...Option) *Downloader { ... }

// Download 执行一次下载（完整编排）
func (d *Downloader) Download(ctx context.Context, req *Request) error {
    // 1. Selector.MatchExtractor → 获取 Extractor
    // 2. 注入 Transport 和 Selector（如果 Extractor 需要）
    // 3. Extractor.Extract → 执行下载
    // 4. 返回结果
}

// ——— 全局便捷函数 ———

// Get 一行下载（使用全局默认 Downloader）
func Get(ctx context.Context, url string, dest string) error
```

### Option 配置

```go
type Option func(*Downloader)

func WithSelector(s Selector) Option              // 自定义 Selector
func WithProxy(proxies []string, force bool) Option  // 快捷配置代理
func WithTransport(t Transport) Option             // 自定义 Transport
func WithExtractor(ex Extractor) Option            // 注册 Extractor
func WithMaxRetries(n int) Option                  // 全局重试次数
func WithDomainLimits(limits map[string]int) Option  // 域名并发限制

// HTTP 传输配置
func WithHTTPTimeout(d time.Duration) Option
func WithHTTPConnPool(maxIdle, perHost int, idleTimeout time.Duration) Option
func WithUserAgent(ua string) Option
```

## 6. 子包结构

```
pkg/download/
├── download.go          # Downloader 主结构体 + New + 全局 Get
├── request.go           # Request / TransportRequest / DownloadHint
├── option.go            # Option 函数集
├── selector.go          # Selector 接口 + DefaultSelector
├── proxy_selector.go    # ProxySelector 接口 + 静态/带宽实现
├── transport.go         # Transport 接口
│
├── extractor/            # 内置 Extractor 实现（子目录包）
│   ├── http.go          # HTTPExtractor
│   ├── hls.go           # HLSExtractor（ffmpeg + m3u8d）
│   ├── composite.go     # CompositeExtractor
│   └── remote.go        # RemoteExtractor（未来：远端下载 + sproxy 回传）
│
├── transport/            # 内置 Transport 实现（子目录包）
│   ├── stdlib.go        # StdlibTransport
│   ├── wget.go          # WgetTransport
│   └── sproxy.go        # SproxyTunnelTransport / SproxyFileTransport
│
├── proxy/               # ProxySelector 实现（子目录包）
│   ├── static.go        # StaticProxySelector
│   └── tunnel.go        # TunnelProxySelector（sproxy 隧道）
│
├── m3u8d/               # m3u8d 下载引擎（工具库，重构自 pkg/m3u8d）
│   ├── engine.go        # M3U8DEngine（接受注入 http.Client）
│   ├── grab.go          # grab 并发下载适配层
│   └── config.go        # DownloadConfig
│
├── domainlimiter.go     # 域名并发限制器
├── progress.go          # 进度跟踪器
├── md5.go               # MD5 校验工具
└── errors.go            # 哨兵错误（ErrNoTry 等）
```

## 7. 关键数据流

### 7.1 标准 HTTP 下载

```
Downloader.Download(ctx, req)
  │
  ├─ Selector.MatchExtractor(url) → HTTPExtractor
  ├─ 注入 transport + selector 到 extractor
  │
  └─ HTTPExtractor.Extract(ctx, req)
       ├─ ResolvePath → 创建目录
       ├─ Selector.SelectProxy(url) → proxyURL
       ├─ 检查断点续传（stat SavePath）
       │
       ├─ Loop (maxRetries):
       │   ├─ Transport.RoundTrip(ctx, &TransportRequest{
       │   │     URL, Headers, Range, ProxyURL
       │   │ })
       │   ├─ 检查 StatusCode (403/404 → ErrNoTry)
       │   ├─ 检查 ContentType (text/html → ErrNoTry)
       │   ├─ 创建 ProgressReader 包装 Body
       │   ├─ io.Copy 写入文件
       │   ├─ MD5 校验（X-Amz-Meta-Md5chksum / ETag / Content-MD5）
       │   ├─ 成功 → break
       │   └─ 失败 → sleep(backoff) → continue
       │
       └─ 填充 req.Metadata (total_size, md5, status, mod_time)
```

### 7.2 HLS 下载（HLSExtractor）

```
Downloader.Download(ctx, req)
  │
  ├─ Selector.MatchExtractor → HLSExtractor
  │
  └─ HLSExtractor.Extract(ctx, req)
       ├─ 检查 mode:
       │   ├─ "m3u8d" → 使用 M3U8DEngine
       │   │   ├─ 注入 http.Client（从 Extractor 的 Transport 获取）
       │   │   ├─ engine.DownloadAll(ctx)
       │   │   │   ├─ parseM3U8 → 下载并解析 m3u8 索引（用注入的 Client）
       │   │   │   └─ downloadFilesConcurrently → TS 分片并发（grab 用注入的 Client）
       │   │   ├─ engine.ConvertToMP4(ctx) → ffmpeg 合并
       │   │   └─ engine.Cleanup()
       │   │
       │   └─ "ffmpeg" → exec ffmpeg 命令
       │       ├─ 构建 ffmpeg args（UA, headers, extra args）
       │       └─ exec.CommandContext(ctx, ffmpeg, args...)
       │
       └─ OnProgress(100) → 填充 Metadata
```

### 7.3 复合下载

```
CompositeExtractor.Extract(ctx, req)
  ├─ 解析 req.Metadata["files"] → fileList
  ├─ 遍历 fileList:
  │   ├─ Selector.MatchExtractor(subURL) → subExtractor
  │   ├─ subExtractor.Extract(ctx, subReq)
  │   └─ 汇总进度
  └─ OnProgress(100)
```

### 7.4 sproxy 隧道下载

```
Downloader.Download(ctx, req)
  │
  ├─ Selector.MatchExtractor → HTTPExtractor
  ├─ Selector.SelectProxy(url) → "sproxy+tunnel://192.168.1.100:18083"
  │
  └─ HTTPExtractor.Extract(ctx, req)
       └─ Transport.RoundTrip(ctx, treq)  // 这里是 SproxyTunnelTransport
            └─ tunnel.Client.Do(&http.Request{
                 Method: "GET",
                 URL: "https://example.com/file.mp4",
                 // 整个请求通过 sproxy 隧道加密转发
               })
```

### 7.5 远端下载（未来扩展）

```
RemoteExtractor.Extract(ctx, req)
  ├─ 打包下载请求为任务描述
  ├─ 通过 sproxy 隧道下发到远端 sproxy 节点
  ├─ 远端节点执行下载（使用内置的 download 库）
  ├─ 下载完成后，结果文件通过 sproxy 文件 API 回传
  │   (或直接返回远端路径)
  └─ 填充结果到 req.Metadata
```

## 8. m3u8d 重构方案

### 现状问题

- `pkg/m3u8d/` 中 `http.Client` 是私有字段，无法从外部注入
- TS 分片并发下载使用 `grab.NewClient()` 硬编码创建，完全隔离
- dlcore 的代理/Transport/超时配置完全无法传入
- 无测试覆盖

### 改造目标

```go
// pkg/download/m3u8d/engine.go

type M3U8DEngine struct {
    Config     *DownloadConfig
    httpClient *http.Client    // 可注入
    grabClient *grab.Client    // 从 httpClient 创建
}

// 构造函数接受可选的 http.Client
func NewM3U8DEngine(cfg *DownloadConfig, httpClient *http.Client) *M3U8DEngine {
    if httpClient == nil {
        httpClient = &http.Client{Timeout: 30 * time.Second}
    }
    // grab 支持 WithHTTPClient？不支持的话用 httpClient.Transport 包装
    // 如果 grab 不支持注入，改用 errgroup + semaphore 实现并发
    // **用户选择保留 grab**，所以需要适配层
    grabClient := adaptToGrab(httpClient)
    return &M3U8DEngine{Config: cfg, httpClient: httpClient, grabClient: grabClient}
}

// API 保持不变
func (e *M3U8DEngine) DownloadAll(ctx context.Context) (string, error) { ... }
func (e *M3U8DEngine) ConvertToMP4(ctx context.Context, localM3U8Path string) error { ... }
func (e *M3U8DEngine) Cleanup() error { ... }
```

### grab 适配策略

`grab.NewClient()` 创建一个新的默认 `http.Client`，grab v3 没有公开的 `WithHTTPClient` 选项。有两种方案：

1. **grab 适配层**：创建 grab client，将其内部的 `HTTPClient` 替换为注入的 client（通过反射或 grab 暴露的字段）
2. **备选：去掉 grab**：使用 `errgroup` + channel semaphore 实现并发下载，复用注入的 `http.Client`

根据用户选择：**保留 grab，可注入 Client**。我会先尝试让 grab 使用注入的 client，如果 grab 不支持则走适配方案。

## 9. sproxy 集成设计

### 9.1 两种模式

| 模式 | Transport | Selector | 用途 |
|------|-----------|----------|------|
| **标准 HTTP 代理** | `StdlibTransport` | `StaticProxySelector` | 兼容现有配置，sproxy 作为 HTTP 代理 |
| **加密隧道** | `SproxyTunnelTransport` | `TunnelProxySelector` | 通过 AES-256-GCM 加密隧道转发请求 |

### 9.2 配置路径

```yaml
# 方式 A: 标准 HTTP 代理（现有方式）
downloader:
  proxy:
    list:
      - "http://sproxy1:18080"
      - "http://sproxy2:18080"
    force: true

# 方式 B: 加密隧道
download:
  transport: "sproxy_tunnel"
  sproxy:
    instances:
      - server_url: "http://sproxy1:18083"
        tunnel_key: "7693db0059a3c14fd6bfec175c8e2d1d3d821a414aab77c467df06aefb70e3b7"
      - server_url: "http://sproxy2:18083"
        tunnel_key: "..."
    probe:
      interval: 30s
      timeout: 3s
```

### 9.3 sproxy 新能力需求

为了让 sproxy 更好地支持下载场景，以下新能力可以考虑：

| 能力 | 说明 | 优先级 |
|------|------|--------|
| **`/bandwidth` 端点** | sproxy 当前版本已移除 `/bandwidth`，需要重新引入或用 `/healthz` 替代 | 高 |
| **隧道任务 API** | 支持通过隧道下发下载任务描述（URL+路径+头），远端执行并回传状态 | 中（Phase 3+） |
| **隧道流式进度** | 在加密隧道帧协议中嵌入进度信息，支持远端下载进度回传 | 中 |
| **分块下载批量** | 支持同时请求多个文件分块 | 低 |

## 10. 演进计划

### Phase 1: 新包搭建 + 核心接口 + 基础实现

**目标**：创建 `pkg/download/`，实现核心接口和 HTTPExtractor + StdlibTransport

| 动作 | 文件 | 说明 |
|------|------|------|
| **新增** | `pkg/download/download.go` | Downloader 结构体、New、全局 Get |
| **新增** | `pkg/download/request.go` | Request、TransportRequest、DownloadHint |
| **新增** | `pkg/download/option.go` | Option 函数 |
| **新增** | `pkg/download/selector.go` | Selector 接口 + DefaultSelector |
| **新增** | `pkg/download/proxy_selector.go` | ProxySelector 接口 + StaticProxySelector（从 dlcore 迁移） |
| **新增** | `pkg/download/transport.go` | Transport 接口 |
| **新增** | `pkg/download/domainlimiter.go` | 域名并发限制器（从 dlcore 迁移） |
| **新增** | `pkg/download/progress.go` | ProgressReader（从 dlcore 迁移） |
| **新增** | `pkg/download/md5.go` | MD5 校验工具（从 dlcore 迁移） |
| **新增** | `pkg/download/errors.go` | 哨兵错误 |
| **新增** | `pkg/download/extractor/http.go` | HTTPExtractor（从 dlcore httpHandler 迁移） |
| **新增** | `pkg/download/transport/stdlib.go` | StdlibTransport（从 dlcore client.go 剥离） |

**验证**：
```bash
go build ./pkg/download/...
go test ./pkg/download/...
```

### Phase 2: 补齐 Extractor 和 Transport

**目标**：HLSExtractor、CompositeExtractor、WgetTransport + m3u8d 重构

| 动作 | 文件 | 说明 |
|------|------|------|
| **新增** | `pkg/download/extractor/hls.go` | HLSExtractor（整合 ffmpeg + m3u8d） |
| **新增** | `pkg/download/extractor/composite.go` | CompositeExtractor |
| **新增** | `pkg/download/transport/wget.go` | WgetTransport（从 downloader/wget.go 迁移） |
| **重构** | `pkg/download/m3u8d/engine.go` | M3U8DEngine 重构 |
| **新增** | `pkg/download/m3u8d/grab.go` | grab 适配层 |
| **新增** | `pkg/download/m3u8d/config.go` | DownloadConfig |

**验证**：
```bash
go test -run 'HLS|Composite|M3U8D' ./pkg/download/...
```

### Phase 3: sproxy 集成

**目标**：sproxy Transport + TunnelProxySelector

| 动作 | 文件 | 说明 |
|------|------|------|
| **新增** | `pkg/download/transport/sproxy.go` | SproxyTunnelTransport |
| **新增** | `pkg/download/proxy/tunnel.go` | TunnelProxySelector |
| **修改** | `downloader/scraper.go` | 移除硬编码隧道密钥 |
| **删除** | `cmd/scraper_get/tunnel/tunnel.go` | 替换为 sproxy 官方 tunnel |

**验证**：
```bash
go test -tags=sproxy_integration ./pkg/download/...
```

### Phase 4: download-manager 接入

**目标**：替换现有下载层，Manager 使用新库

| 动作 | 文件 | 说明 |
|------|------|------|
| **修改** | `core/interfaces.go` | Downloader 接口增强 |
| **修改** | `manager/download.go` | 移除类型断言 |
| **修改** | `manager/manager.go` | 使用新库 |
| **修改** | `downloader/downloader.go` | 适配器：将旧接口映射到新库 |
| **标记废弃** | `pkg/dlcore/` | 标记 deprecated，内部调用新库 |
| **测试** | 全量回归 | |

**验证**：
```bash
go build ./...
go test ./...
```

### Phase 5: 路由规则 + 可观测性

**目标**：URL 模式路由规则、Per-handler 指标

| 动作 | 文件 | 说明 |
|------|------|------|
| **新增** | `pkg/download/rules.go` | 规则引擎 |
| **新增** | `pkg/download/metrics.go` | 下载指标 |
| **新增** | `pkg/download/middleware.go` | 下载中间件链 |
| **新增** | `api/metrics_handler.go` | 暴露指标 API |

**验证**：
```bash
go test -run 'Rules|Metrics' ./pkg/download/...
```

## 11. 测试策略

| 层级 | 测试内容 | 方式 |
|------|---------|------|
| **单元测试** | 每个接口的实现单独测试 | `go test ./pkg/download/...` |
| **集成测试** | Transport + Extractor 组合 | `go test -tags=integration` |
| **sproxy 测试** | 加密隧道端到端 | `go test -tags=sproxy_integration` |
| **回归测试** | 确保下载行为不变 | 启动 manager 跑实际下载 |

## 12. 向后兼容性

| 层面 | 兼容策略 |
|------|---------|
| **配置** | `downloader.type` / `proxies` / `force_proxy` 等旧字段在适配层转为新配置 |
| **API 响应** | `GET /api/objects` 的 JSON 字段保持不变 |
| **存储数据** | MongoDB / 文件存储的对象 Extra/Metadata 结构不变 |
| **Task 实现** | `core.Downloader` 接口保持，新增方法可选实现 |

## 13. 风险和应对

| 风险 | 概率 | 影响 | 应对 |
|------|------|------|------|
| Transport 剥离时 HTTP 行为改变 | 中 | 高 | 严格回归测试，逐函数提取 |
| grab 注入 Client 不可行 | 低 | 中 | 备用方案：errgroup + semaphore |
| sproxy 的 `/bandwidth` 端点缺失 | 高 | 中 | 临时用 `/healthz` 替代 |
| Manager 接口变更破坏 Task | 中 | 高 | 分步迁移，同时支持新旧接口 |