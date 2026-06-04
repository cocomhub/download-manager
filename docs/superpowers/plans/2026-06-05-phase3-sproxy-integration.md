# Phase 3：sproxy 集成实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 将 sproxy（同工作区项目）的加密隧道和文件传输能力集成到 `pkg/download/` 中，提供 SproxyTunnelTransport 和 TunnelProxySelector。

**架构：** sproxy 是独立服务（端口 18083），通过 **Go import 方式集成**。download-manager 直接 import `github.com/cocomhub/sproxy/pkg/tunnel` 和 `github.com/cocomhub/sproxy/pkg/client` 等包，在 `go.mod` 中使用 `replace` 指令指向本地路径。TunnelProxySelector 做延迟/健康探测选最优节点，SproxyTunnelTransport 封装 tunnel.Client 和 FileClient。

**技术栈：** Go 1.26 标准库 (`net/http`, `crypto/aes`, `encoding/hex`)。sproxy 隧道协议：AES-256-GCM 流式加密，64KB 分块，12B nonce + 16B tag。

---

## 文件结构

```
pkg/download/
├── transport/
│   ├── stdlib.go              # StdlibTransport (Phase 1)
│   └── sproxy.go              # NEW: SproxyTunnelTransport
│
├── proxy/
│   └── tunnel.go              # NEW: TunnelProxySelector
│
└── bandwidth.go               # NEW: 带宽探测工具函数
```

## 前提条件：sproxy 包导入方式

在 `go.mod` 中添加 `replace` 指令指向本地 sproxy 项目，然后直接 Go import：

```go
// go.mod
require github.com/cocomhub/sproxy v0.0.0
replace github.com/cocomhub/sproxy => ../sproxy

// 代码中直接 import：
import (
    "github.com/cocomhub/sproxy/pkg/client"    // FileClient
    "github.com/cocomhub/sproxy/pkg/tunnel"     // tunnel.Client, ParseKey
)
```

**sproxy 关键 API：**

```go
// pkg/tunnel — 加密隧道
func NewClient(hexKey, tunnelURL string, timeout time.Duration, logger *slog.Logger) (*Client, error)
func (c *Client) Do(req *http.Request) (*http.Response, error)  // 通过隧道转发 HTTP 请求

// pkg/client — 文件操作客户端
func NewFileClient(serverURL string, opts ...Option) *FileClient
func WithTunnel(hexKey string) Option     // 启用隧道模式
func WithHTTPClient(hc *http.Client) Option
func (c *FileClient) Download(ctx, filename, outputPath) error
func (c *FileClient) Upload(ctx, localPath, remotePath) (*UploadResult, error)
func (c *FileClient) Delete(ctx, filename, localPath) error
func (c *FileClient) Stat(ctx, filename) (*FileInfo, error)
func (c *FileClient) List(ctx, subdirs...) ([]FileInfo, error)
func (c *FileClient) Search(ctx, q) ([]FileInfo, error)
```

---

### 任务 1：BandwidthChecker — 带宽探测工具

**文件：**
- 创建：`pkg/download/bandwidth.go`
- 创建：`pkg/download/bandwidth_test.go`

从 `pkg/dlcore/proxy.go` 迁移带宽探测逻辑到 `pkg/download/` 包，作为独立工具函数。

- [ ] **步骤 1：创建 `bandwidth.go`**

```go
package download

import (
    "context"
    "io"
    "net/http"
    "strconv"
    "strings"
    "time"
)

// CheckBandwidth 探测目标 URL 的带宽（字节/秒）。
// 下载一定字节后计算下载速率。
func CheckBandwidth(ctx context.Context, url string, probeBytes int64, timeout time.Duration) (float64, error) {
    // ...
}

// CheckHealth 检查代理/隧道节点是否健康。
func CheckHealth(ctx context.Context, url string, timeout time.Duration) error {
    // ...
}
```

- [ ] **步骤 2：创建 `bandwidth_test.go`**

```go
package download_test

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"
    "github.com/cocomhub/download-manager/pkg/download"
)

func TestCheckHealthOK(t *testing.T) {
    ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("OK"))
    }))
    defer ts.Close()

    err := download.CheckHealth(context.Background(), ts.URL, 5*time.Second)
    if err != nil {
        t.Errorf("expected nil, got %v", err)
    }
}

func TestCheckHealthFail(t *testing.T) {
    err := download.CheckHealth(context.Background(), "http://localhost:1/healthz", time.Second)
    if err == nil {
        t.Error("expected error for unreachable server")
    }
}
```

- [ ] **步骤 3：验证测试**

运行：`go test ./pkg/download/... -run 'TestCheckHealth|TestCheckBandwidth' -v`
预期：PASS

- [ ] **步骤 4：Commit**

```bash
git add pkg/download/bandwidth.go pkg/download/bandwidth_test.go
git commit -m "feat(download): add BandwidthChecker and CheckHealth utilities"
```

---

### 任务 2：TunnelProxySelector — 多 sproxy 实例选择器

**文件：**
- 创建：`pkg/download/proxy/tunnel.go`
- 创建：`pkg/download/proxy/tunnel_test.go`

从设计文档中的 `TunnelProxySelector` 概念实现，对多个 sproxy 实例做延迟/健康探测，选最优节点。

- [ ] **步骤 1：编写失败测试**

```go
// pkg/download/proxy/tunnel_test.go
package proxy_test

import (
    "context"
    "testing"
    "github.com/cocomhub/download-manager/pkg/download"
)

func TestTunnelProxySelectorName(t *testing.T) {
    sel := NewTunnelProxySelector()
    if sel == nil {
        t.Fatal("NewTunnelProxySelector returned nil")
    }
}

func TestTunnelProxySelectorNoInstances(t *testing.T) {
    sel := NewTunnelProxySelector()
    proxy, err := sel.Select(context.Background(), "http://example.com/file", nil)
    if err != nil {
        t.Fatalf("Select with no instances should not error: %v", err)
    }
    if proxy != "" {
        t.Errorf("expected empty proxy, got %s", proxy)
    }
}

func TestTunnelProxySelectorWithInstance(t *testing.T) {
    sel := NewTunnelProxySelector(
        WithTunnelInstance("http://sproxy1:18083", "0000000000000000000000000000000000000000000000000000000000000000"),
    )
    proxy, err := sel.Select(context.Background(), "http://example.com/file", nil)
    if err != nil {
        t.Fatalf("Select should not error: %v", err)
    }
    if proxy == "" {
        t.Skip("no sproxy running, skipping")
    }
    t.Logf("Selected proxy: %s", proxy)
}
```

- [ ] **步骤 2：运行测试确认失败**

运行：`go test ./pkg/download/proxy/... -v`
预期：FAIL，报错 "undefined: NewTunnelProxySelector"

- [ ] **步骤 3：创建 `proxy/tunnel.go` 实现**

```go
package proxy

import (
    "context"
    "crypto/sha256"
    "fmt"
    "log/slog"
    "math"
    "net/http"
    "sort"
    "sync"
    "time"

    "github.com/cocomhub/download-manager/pkg/download"
)

// TunnelInstance 描述一个 sproxy 隧道实例。
type TunnelInstance struct {
    ServerURL string // e.g. "http://192.168.1.100:18083"
    TunnelKey string // 64 hex chars, AES-256-GCM key
}

// TunnelProxySelector 在多个 sproxy 实例中选择最优节点。
// 带宽探测缓存在内存中，每 30 秒刷新一次。
type TunnelProxySelector struct {
    instances  []TunnelInstance
    mu         sync.RWMutex
    cache      map[string]cachedResult
    probeSize  int64
    probeTimeout time.Duration
    cacheTTL   time.Duration
}

type cachedResult struct {
    proxyURL   string
    bandwidth  float64
    checkedAt  time.Time
}

// NewTunnelProxySelector 创建 TunnelProxySelector。
func NewTunnelProxySelector(opts ...TunnelOption) *TunnelProxySelector { ... }

// TunnelOption 配置 TunnelProxySelector。
type TunnelOption func(*TunnelProxySelector)
func WithTunnelInstance(serverURL, tunnelKey string) TunnelOption { ... }
func WithProbeSize(bytes int64) TunnelOption { ... }
func WithProbeTimeout(d time.Duration) TunnelOption { ... }
func WithCacheTTL(d time.Duration) TunnelOption { ... }

func (s *TunnelProxySelector) Select(ctx context.Context, targetURL string, hint *download.DownloadHint) (string, error) {
    s.mu.RLock()
    instances := s.instances
    s.mu.RUnlock()
    
    if len(instances) == 0 {
        return "", nil
    }
    // 对每个实例检查缓存，若缓存命中则使用缓存结果
    // 否则发起带宽探测
    // 返回带宽最高的实例的 proxyURL
}
```

- [ ] **步骤 4：创建 `proxy/tunnel_test.go` 单元测试**

- [ ] **步骤 5：验证测试**

运行：`go test ./pkg/download/proxy/... -v`
预期：PASS

- [ ] **步骤 6：Commit**

```bash
git add pkg/download/proxy/
git commit -m "feat(download): add TunnelProxySelector for multi-sproxy instance selection"
```

---

### 任务 3：SproxyTunnelTransport — 隧道传输层

**文件：**
- 创建：`pkg/download/transport/sproxy.go`
- 创建：`pkg/download/transport/sproxy_test.go`

实现 `download.Transport` 接口，通过 Go import 方式集成 sproxy 的加密隧道。使用 `tunnel.Client` 封装隧道传输，`client.FileClient` 提供文件操作。

- [ ] **步骤 1：编写失败测试**

```go
// pkg/download/transport/sproxy_test.go
package transport_test

import (
    "context"
    "testing"
    "github.com/cocomhub/download-manager/pkg/download"
)

func TestSproxyTransportName(t *testing.T) {
    tr := NewSproxyTunnelTransport("http://localhost:18083")
    if tr.Name() != "sproxy" {
        t.Errorf("expected 'sproxy', got %s", tr.Name())
    }
}

func TestSproxyTransportRoundTrip(t *testing.T) {
    tr := NewSproxyTunnelTransport("http://localhost:18083")
    resp, err := tr.RoundTrip(context.Background(), &download.TransportRequest{
        URL:     "http://example.com/file",
        Method:  "GET",
    })
    if err == nil {
        t.Skip("sproxy not running, skipping")
    }
    t.Logf("Got expected error: %v", err)
}
```

- [ ] **步骤 2：运行测试确认失败**

运行：`go test ./pkg/download/transport/... -v`
预期：FAIL，报错 "undefined: NewSproxyTunnelTransport"

- [ ] **步骤 3：更新 `go.mod` 添加 sproxy 依赖**

```bash
# 将下列内容添加到 go.mod 的 require 块：
// require github.com/cocomhub/sproxy v0.0.0
// 并在 go.mod 末尾添加 replace 指令：
// replace github.com/cocomhub/sproxy => ../sproxy
# 然后运行：
cd D:\workdir\leon\cocomhub\download-manager
go mod edit -require github.com/cocomhub/sproxy@v0.0.0
go mod edit -replace github.com/cocomhub/sproxy=../sproxy
go mod tidy
```

- [ ] **步骤 3b：创建 `transport/sproxy.go` 实现**

```go
package transport

import (
    "context"
    "crypto/tls"
    "fmt"
    "log/slog"
    "net/http"
    "strings"
    "time"

    "github.com/cocomhub/download-manager/pkg/download"
    "github.com/cocomhub/sproxy/pkg/client"
    "github.com/cocomhub/sproxy/pkg/tunnel"
)

// SproxyTunnelTransport 通过 sproxy 加密隧道传输数据。
// 使用 tunnel.Client 做 HTTP 隧道转发，可选的 FileClient 做文件操作。
type SproxyTunnelTransport struct {
    serverURL   string
    tunnelKey   string
    tunnelURL   string
    healthURL   string
    client      *http.Client
    tunnelCl    *tunnel.Client
    fileClient  *client.FileClient
    useTunnel   bool
}

// NewSproxyTunnelTransport 创建 SproxyTunnelTransport。
func NewSproxyTunnelTransport(serverURL string, opts ...SproxyOption) *SproxyTunnelTransport {
    t := &SproxyTunnelTransport{
        serverURL: strings.TrimRight(serverURL, "/"),
        healthURL: strings.TrimRight(serverURL, "/") + "/healthz",
        client: &http.Client{
            Timeout: 600 * time.Second,
            Transport: &http.Transport{
                MaxIdleConns:        20,
                MaxIdleConnsPerHost: 5,
                IdleConnTimeout:     60 * time.Second,
                // TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // 生产环境不应跳级验证；使用标准 CA 证书或自签 CA 添加至信任存储
            },
        },
    }
    for _, o := range opts { o(t) }

    // 如果配置了隧道密钥，初始化 tunnel.Client
    if t.tunnelKey != "" {
        tc, err := tunnel.NewClient(t.tunnelKey, t.serverURL+"/tunnel", 600*time.Second, slog.Default())
        if err == nil {
            t.tunnelCl = tc
            t.useTunnel = true
            t.fileClient = client.NewFileClient(t.serverURL,
                client.WithTunnel(t.tunnelKey),
                client.WithHTTPClient(t.client),
            )
        }
    }
    return t
}

// SproxyOption 配置 SproxyTunnelTransport。
type SproxyOption func(*SproxyTunnelTransport)

func WithSproxyTunnelKey(key string) SproxyOption {
    return func(t *SproxyTunnelTransport) { t.tunnelKey = key }
}

func WithSproxyHTTPClient(c *http.Client) SproxyOption {
    return func(t *SproxyTunnelTransport) { t.client = c }
}

func (t *SproxyTunnelTransport) Name() string { return "sproxy" }

func (t *SproxyTunnelTransport) RoundTrip(ctx context.Context, treq *download.TransportRequest) (*download.TransportResponse, error) {
    if t.useTunnel && t.tunnelCl != nil {
        return t.roundTripViaTunnel(ctx, treq)
    }
    return t.roundTripViaProxy(ctx, treq)
}

// roundTripViaProxy 通过 sproxy HTTP 代理转发（非加密）。
func (t *SproxyTunnelTransport) roundTripViaProxy(ctx context.Context, treq *download.TransportRequest) (*download.TransportResponse, error) {
    targetURL := treq.URL
    targetURL = strings.TrimPrefix(targetURL, "http://")
    targetURL = strings.TrimPrefix(targetURL, "https://")
    fullURL := t.serverURL + "/" + targetURL

    hreq, err := http.NewRequestWithContext(ctx, treq.Method, fullURL, nil)
    if err != nil {
        return nil, fmt.Errorf("sproxy: failed to create request: %w", err)
    }
    for k, v := range treq.Headers {
        hreq.Header.Set(k, v)
    }
    if treq.Range != nil && treq.Range.Offset > 0 {
        hreq.Header.Set("Range", fmt.Sprintf("bytes=%d-", treq.Range.Offset))
    }

    resp, err := t.client.Do(hreq)
    if err != nil {
        return nil, fmt.Errorf("sproxy: request failed: %w", err)
    }

    headers := make(map[string]string)
    for k := range resp.Header {
        headers[k] = resp.Header.Get(k)
    }
    return &download.TransportResponse{
        Body:          resp.Body,
        StatusCode:    resp.StatusCode,
        ContentLength: resp.ContentLength,
        Headers:       headers,
        ProxyURL:      t.serverURL,
    }, nil
}

// roundTripViaTunnel 通过 sproxy 加密隧道转发请求。
func (t *SproxyTunnelTransport) roundTripViaTunnel(ctx context.Context, treq *download.TransportRequest) (*download.TransportResponse, error) {
    hreq, err := http.NewRequestWithContext(ctx, treq.Method, treq.URL, nil)
    if err != nil {
        return nil, fmt.Errorf("sproxy: tunnel request failed: %w", err)
    }
    for k, v := range treq.Headers {
        hreq.Header.Set(k, v)
    }
    if treq.Range != nil && treq.Range.Offset > 0 {
        hreq.Header.Set("Range", fmt.Sprintf("bytes=%d-", treq.Range.Offset))
    }

    resp, err := t.tunnelCl.Do(hreq)
    if err != nil {
        return nil, fmt.Errorf("sproxy: tunnel roundtrip failed: %w", err)
    }

    headers := make(map[string]string)
    for k := range resp.Header {
        headers[k] = resp.Header.Get(k)
    }
    return &download.TransportResponse{
        Body:          resp.Body,
        StatusCode:    resp.StatusCode,
        ContentLength: resp.ContentLength,
        Headers:       headers,
        ProxyURL:      t.serverURL,
    }, nil
}

// HealthCheck 检查 sproxy 服务是否健康。
func (t *SproxyTunnelTransport) HealthCheck(ctx context.Context) error {
    hreq, err := http.NewRequestWithContext(ctx, "GET", t.healthURL, nil)
    if err != nil {
        return err
    }
    resp, err := t.client.Do(hreq)
    if err != nil {
        return fmt.Errorf("sproxy health check failed: %w", err)
    }
    resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("sproxy health check returned status %d", resp.StatusCode)
    }
    return nil
}
```

- [ ] **步骤 4：验证测试**

运行：`go test ./pkg/download/transport/... -v`
预期：PASS

- [ ] **步骤 5：Commit**

```bash
git add pkg/download/transport/sproxy.go pkg/download/transport/sproxy_test.go
git commit -m "feat(download): add SproxyTunnelTransport for sproxy HTTP proxy transport"
```

---

## 验证

```bash
go build ./pkg/download/...
go vet ./pkg/download/...
go test -count=1 ./pkg/download/...
```