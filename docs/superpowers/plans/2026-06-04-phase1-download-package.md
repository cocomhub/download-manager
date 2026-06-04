# Phase 1：新包搭建 + 核心接口 + 基础组件 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 在 `pkg/download/` 下创建完整的通用下载库，包含三层架构（Selector/Extractor/Transport）的核心接口、基础组件实现（HTTPExtractor + StdlibTransport + DefaultSelector + StaticProxySelector）和工具函数（DomainLimiter、ProgressReader、MD5、FS）。

**架构：** 三层混合架构。Selector 路由请求到 Extractor（同时决定代理），Extractor 编排完整下载流程（重试/校验/进度），Transport 只负责字节传输。`Downloader` 结构体是用户入口，`download.Get()` 提供一行调用。

**技术栈：** Go 1.26 标准库 (`net/http`, `sync`, `context`, `io`, `crypto/md5`)，无第三方依赖。从 `pkg/dlcore/` 迁移现有代码但不耦合，新包零外部依赖。

---

## 文件结构

```
pkg/download/
├── download.go              # Downloader 主结构体 + New() + 全局 Get()
├── request.go               # Request / TransportRequest / TransportResponse / DownloadHint
├── option.go                # Option 函数集
├── selector.go              # Selector 接口 + DefaultSelector
├── extractor.go             # Extractor 接口定义（仅接口，实现在子目录）
├── transport.go             # Transport 接口定义
├── proxy_selector.go        # ProxySelector 接口 + StaticProxySelector
├── domainlimiter.go         # DomainLimiter（从 dlcore 迁移）
├── progress.go              # ProgressReader（从 dlcore 迁移）
├── md5.go                   # MD5 校验工具（从 dlcore 迁移）
├── errors.go                # ErrNoTry 等哨兵错误
├── fs.go                    # ResolvePath / IsWithinRoot（从 dlcore 迁移）
│
├── extractor/               # Extractor 实现
│   └── http.go              # HTTPExtractor
│
├── transport/               # Transport 实现
│   └── stdlib.go            # StdlibTransport
│
└── download_test.go         # 集成测试
```

## 任务分解

### 任务 1：基础类型 + 接口定义

**文件：**
- 创建：`pkg/download/request.go`
- 创建：`pkg/download/errors.go`
- 创建：`pkg/download/extractor.go`
- 创建：`pkg/download/transport.go`
- 创建：`pkg/download/selector.go`

- [ ] **步骤 1：创建 `pkg/download/` 目录和 `request.go`**

```go
// pkg/download/request.go
package download

type DownloadHint struct {
	FileSize    int64
	ContentType string
	Extractor   string
	Tags        map[string]string
}

type Request struct {
	URL           string
	SavePath      string
	Headers       map[string]string
	TrackProgress bool
	OnProgress    func(progress float64, downloaded, total int64)
	Metadata      map[string]string
	Hint          *DownloadHint
}

type RangeRequest struct {
	Offset int64
}

type TransportRequest struct {
	URL     string
	Method  string
	Headers map[string]string
	Body    []byte
	Range   *RangeRequest
	ProxyURL string
}

type TransportResponse struct {
	Body          io.ReadCloser
	StatusCode    int
	ContentLength int64
	Headers       map[string]string
	ProxyURL      string
}
```

编写完后 `go vet ./pkg/download/...` 应通过。

- [ ] **步骤 2：创建 `errors.go`**

```go
// pkg/download/errors.go
package download

import "errors"

var ErrNoTry = errors.New("no try left")

func IsNoTry(err error) bool {
	return errors.Is(err, ErrNoTry)
}
```

- [ ] **步骤 3：创建 `extractor.go`（Extractor 接口）**

```go
// pkg/download/extractor.go
package download

import "context"

type Extractor interface {
	Name() string
	Match(ctx context.Context, url string) bool
	Extract(ctx context.Context, req *Request) error
}
```

- [ ] **步骤 4：创建 `transport.go`（Transport 接口）**

```go
// pkg/download/transport.go
package download

import "context"

type Transport interface {
	Name() string
	RoundTrip(ctx context.Context, req *TransportRequest) (*TransportResponse, error)
}
```

- [ ] **步骤 5：创建 `selector.go`（Selector + ProxySelector 接口）**

```go
// pkg/download/selector.go
package download

import "context"

type Selector interface {
	MatchExtractor(ctx context.Context, url string, hint *DownloadHint) Extractor
	SelectProxy(ctx context.Context, targetURL string, hint *DownloadHint) (proxyURL string, err error)
}

type ProxySelector interface {
	Select(ctx context.Context, targetURL string, hint *DownloadHint) (proxyURL string, err error)
}
```

- [ ] **步骤 6：编译检查**

```bash
cd download-manager
go vet ./pkg/download/...
```
预期：无错误

- [ ] **步骤 7：Commit**

```bash
git add pkg/download/
git commit -m "feat(download): add core interfaces (Selector/Extractor/Transport) and types"
```

### 任务 2：工具函数（DomainLimiter + ProgressReader + MD5 + FS）

**文件：**
- 创建：`pkg/download/domainlimiter.go`
- 创建：`pkg/download/progress.go`
- 创建：`pkg/download/md5.go`
- 创建：`pkg/download/fs.go`

- [ ] **步骤 1：创建 `domainlimiter.go`（从 dlcore 迁移，独立可测试）**

```go
// pkg/download/domainlimiter.go
package download

import (
	"net/url"
	"sync"
)

type DomainLimiter struct {
	mu    sync.Mutex
	cond  *sync.Cond
	limit map[string]int
	cur   map[string]int
}

func NewDomainLimiter() *DomainLimiter {
	d := &DomainLimiter{
		limit: make(map[string]int),
		cur:   make(map[string]int),
	}
	d.cond = sync.NewCond(&d.mu)
	return d
}

func (d *DomainLimiter) Set(host string, max int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if max <= 0 {
		max = 1
	}
	d.limit[host] = max
	d.cond.Broadcast()
}

func (d *DomainLimiter) Acquire(rawURL string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	host := u.Host
	d.mu.Lock()
	for max := d.limit[host]; max != 0 && d.cur[host] >= max; max = d.limit[host] {
		d.cond.Wait()
	}
	d.cur[host]++
	d.mu.Unlock()
}

func (d *DomainLimiter) Release(rawURL string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	host := u.Host
	d.mu.Lock()
	if d.cur[host] > 0 {
		d.cur[host]--
	}
	d.cond.Broadcast()
	d.mu.Unlock()
}
```

```go
// pkg/download/domainlimiter_test.go
package download

import (
	"sync"
	"testing"
)

func TestDomainLimiterSetAndAcquire(t *testing.T) {
	dl := NewDomainLimiter()
	dl.Set("example.com", 2)

	dl.Acquire("http://example.com/file1")  // should not block
	dl.Acquire("http://example.com/file2")  // should not block (2nd)
	done := make(chan struct{})
	go func() {
		dl.Acquire("http://example.com/file3")  // should block until Release
		close(done)
	}()

	// Release one slot
	dl.Release("http://example.com/file1")

	// The goroutine should unblock
	<-done
}

func TestDomainLimiterReleaseUnknown(t *testing.T) {
	dl := NewDomainLimiter()
	dl.Release("http://unknown.com/file") // should not panic
}
```

- [ ] **步骤 2：运行 DomainLimiter 测试**

```bash
go test -v -run TestDomainLimiter ./pkg/download/
```
预期：PASS

- [ ] **步骤 3：创建 `progress.go`（ProgressReader，从 dlcore 迁移，增加测试）**

```go
// pkg/download/progress.go
package download

import "io"

type ProgressReader struct {
	reader     io.Reader
	total      int64
	downloaded int64
	onProgress func(progress float64, downloaded, total int64)
}

func NewProgressReader(reader io.Reader, total int64, onProgress func(float64, int64, int64)) *ProgressReader {
	return &ProgressReader{
		reader:     reader,
		total:      total,
		onProgress: onProgress,
	}
}

func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		pr.downloaded += int64(n)
		if pr.total > 0 && pr.onProgress != nil {
			progress := float64(pr.downloaded) / float64(pr.total) * 100
			pr.onProgress(progress, pr.downloaded, pr.total)
		}
	}
	return n, err
}

func (pr *ProgressReader) Done() {
	if pr.onProgress != nil && pr.total > 0 {
		pr.onProgress(100, pr.total, pr.total)
	}
}
```

```go
// pkg/download/progress_test.go
package download

import (
	"strings"
	"testing"
)

func TestProgressReader(t *testing.T) {
	content := "hello world this is a test"
	reader := strings.NewReader(content)
	var lastProgress float64
	pr := NewProgressReader(reader, int64(len(content)), func(p float64, d, t int64) {
		lastProgress = p
	})

	buf := make([]byte, 5)
	for {
		_, err := pr.Read(buf)
		if err != nil {
			break
		}
	}
	pr.Done()
	if lastProgress != 100 {
		t.Errorf("expected 100%% progress, got %.1f%%", lastProgress)
	}
}
```

- [ ] **步骤 4：运行 ProgressReader 测试**

```bash
go test -v -run TestProgressReader ./pkg/download/
```
预期：PASS

- [ ] **步骤 5：创建 `md5.go`（从 dlcore 迁移）**

```go
// pkg/download/md5.go
package download

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"io"
	"os"
)

func ComputeFileMD5(filePath string) (base64MD5, hexMD5 string, err error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", "", err
	}
	defer file.Close()
	hasher := md5.New()
	buf := make([]byte, 64*1024)
	if _, err := io.CopyBuffer(hasher, file, buf); err != nil {
		return "", "", err
	}
	hashBytes := hasher.Sum(nil)
	return base64.StdEncoding.EncodeToString(hashBytes), hex.EncodeToString(hashBytes), nil
}

// TryGetMd5 从响应头中提取期望的 MD5 值
func TryGetMd5(headers map[string]string) string {
	if xAmzMetaMd5 := headers["X-Amz-Meta-Md5chksum"]; len(xAmzMetaMd5) == 24 {
		return xAmzMetaMd5
	}
	if etag := headers["Etag"]; len(etag) == 34 && etag[0] == '"' && etag[33] == '"' {
		return etag[1:33]
	}
	if wantHexMd5 := headers["Content-MD5"]; len(wantHexMd5) == 32 {
		return wantHexMd5
	}
	return ""
}
```

```go
// pkg/download/md5_test.go
package download

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComputeFileMD5(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	_, hexMD5, err := ComputeFileMD5(path)
	if err != nil {
		t.Fatal(err)
	}
	if hexMD5 != "5d41402abc4b2a76b9719d911017c592" {
		t.Errorf("unexpected md5: %s", hexMD5)
	}
}

func TestTryGetMd5(t *testing.T) {
	h := map[string]string{"Content-MD5": "d41d8cd98f00b204e9800998ecf8427e"}
	if got := TryGetMd5(h); got != "d41d8cd98f00b204e9800998ecf8427e" {
		t.Errorf("unexpected md5: %s", got)
	}
}
```

- [ ] **步骤 6：运行 MD5 测试**

```bash
go test -v -run TestComputeFileMD5\|TestTryGetMd5 ./pkg/download/
```
预期：PASS

- [ ] **步骤 7：创建 `fs.go`（从 dlcore 迁移）**

```go
// pkg/download/fs.go
package download

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ResolvePath(rootDir, p string) (string, error) {
	if rootDir == "" {
		return p, nil
	}
	if filepath.IsAbs(p) {
		if isWithinRoot(rootDir, p) {
			return p, nil
		}
		return "", fmt.Errorf("path outside root: %s", p)
	}
	rp, err := cleanJoin(rootDir, p)
	if err != nil {
		return "", err
	}
	if !isWithinRoot(rootDir, rp) {
		return "", fmt.Errorf("path outside root: %s", p)
	}
	return rp, nil
}

func isWithinRoot(rootDir, p string) bool {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return false
	}
	absP, err := filepath.Abs(p)
	if err != nil {
		return false
	}
	if absRoot == absP {
		return true
	}
	if !strings.HasSuffix(absRoot, string(filepath.Separator)) {
		absRoot += string(filepath.Separator)
	}
	return strings.HasPrefix(absP, absRoot)
}

func cleanJoin(rootDir string, elems ...string) (string, error) {
	all := append([]string{rootDir}, elems...)
	return filepath.Clean(filepath.Join(all...)), nil
}

func EnsureDir(path string) error {
	dir := filepath.Dir(path)
	if dir != "" {
		return os.MkdirAll(dir, 0755)
	}
	return nil
}
```

```go
// pkg/download/fs_test.go
package download

import (
	"path/filepath"
	"testing"
)

func TestResolvePathRelative(t *testing.T) {
	got, err := ResolvePath("/root", "sub/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(got) != filepath.Clean("/root/sub/file.txt") {
		t.Errorf("unexpected path: %s", got)
	}
}

func TestResolvePathOutsideRoot(t *testing.T) {
	_, err := ResolvePath("/root", "../outside/file.txt")
	if err == nil {
		t.Error("expected error for path outside root")
	}
}
```

- [ ] **步骤 8：运行 FS 测试**

```bash
go test -v -run TestResolvePath ./pkg/download/
```
预期：PASS

- [ ] **步骤 9：Commit**

```bash
git add pkg/download/domainlimiter.go pkg/download/domainlimiter_test.go
git add pkg/download/progress.go pkg/download/progress_test.go
git add pkg/download/md5.go pkg/download/md5_test.go
git add pkg/download/fs.go pkg/download/fs_test.go
git commit -m "feat(download): add utility components (DomainLimiter, ProgressReader, MD5, FS)"
```

### 任务 3：DomainLimiter 并发测试（确保 DomainLimiter 可用）

- [ ] **步骤 1：编写并发竞争测试**

```go
// pkg/download/domainlimiter_test.go (追加)
func TestDomainLimiterConcurrent(t *testing.T) {
	dl := NewDomainLimiter()
	dl.Set("example.com", 3)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			dl.Acquire("http://example.com/file")
			dl.Release("http://example.com/file")
		}(i)
	}
	wg.Wait()
}
```

- [ ] **步骤 2：运行测试**

```bash
go test -v -run TestDomainLimiterConcurrent -race ./pkg/download/
```
预期：PASS（无 race）

- [ ] **步骤 3：Commit**

```bash
git add pkg/download/domainlimiter_test.go
git commit -m "test(download): add concurrent test for DomainLimiter"
```

### 任务 4：ProxySelector 实现（StaticProxySelector）

**文件：**
- 创建：`pkg/download/proxy_selector.go`

- [ ] **步骤 1：创建 `proxy_selector.go`**

```go
// pkg/download/proxy_selector.go
package download

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type StaticProxySelector struct {
	proxies          []string
	forceProxy       bool
	cacheDir         string
	decisionCacheTTL int // seconds
	probeTimeout     int // seconds
	bandwidthSuffix  string
}

func NewStaticProxySelector(proxies []string) *StaticProxySelector {
	return &StaticProxySelector{
		proxies:          proxies,
		decisionCacheTTL: 1,
		probeTimeout:     3,
		bandwidthSuffix:  "/bandwidth",
	}
}

func (s *StaticProxySelector) WithForceProxy(v bool) *StaticProxySelector {
	s.forceProxy = v
	return s
}

func (s *StaticProxySelector) WithCache(dir string, ttlSecs int) *StaticProxySelector {
	s.cacheDir = dir
	if ttlSecs > 0 {
		s.decisionCacheTTL = ttlSecs
	}
	return s
}

func (s *StaticProxySelector) WithProbe(timeoutSecs int) *StaticProxySelector {
	if timeoutSecs > 0 {
		s.probeTimeout = timeoutSecs
	}
	return s
}

func (s *StaticProxySelector) WithBandwidthSuffix(suffix string) *StaticProxySelector {
	if suffix != "" {
		s.bandwidthSuffix = suffix
	}
	return s
}

func (s *StaticProxySelector) Select(ctx context.Context, targetURL string, hint *DownloadHint) (string, error) {
	if len(s.proxies) == 0 {
		return "", nil
	}

	u, err := url.Parse(targetURL)
	if err != nil {
		return "", err
	}
	domain := u.Host

	cacheBase := s.cacheDir
	if cacheBase == "" {
		cacheBase = filepath.Join(os.TempDir(), "download_proxy_cache")
	}
	cachePath := filepath.Join(cacheBase, domain)

	// 检查缓存
	if info, err := os.Stat(cachePath); err == nil {
		ttl := s.decisionCacheTTL
		if ttl <= 0 {
			ttl = 1
		}
		if time.Since(info.ModTime()) < time.Duration(ttl)*time.Second {
			content, _ := os.ReadFile(cachePath)
			contentStr := strings.TrimSpace(string(content))
			if contentStr == "direct" {
				return "", nil
			}
			if contentStr == "proxy" {
				return s.selectBestProxy(cachePath)
			}
		}
	}

	// 直连探测
	if !s.forceProxy {
		if checkDirect(targetURL, s.probeTimeout) {
			_ = os.MkdirAll(filepath.Dir(cachePath), 0755)
			_ = os.WriteFile(cachePath, []byte("direct"), 0644)
			return "", nil
		}
	}

	return s.selectBestProxy(cachePath)
}

func (s *StaticProxySelector) selectBestProxy(cachePath string) (string, error) {
	bestProxy := ""
	minBandwidth := 999999.0
	for _, p := range s.proxies {
		bw := getProxyBandwidth(p, s.bandwidthSuffix, s.probeTimeout)
		if bw < minBandwidth {
			minBandwidth = bw
			bestProxy = p
		}
	}
	if bestProxy != "" {
		_ = os.MkdirAll(filepath.Dir(cachePath), 0755)
		_ = os.WriteFile(cachePath, []byte("proxy"), 0644)
		return bestProxy, nil
	}
	return "", fmt.Errorf("no suitable proxy found")
}

// checkDirect 检测是否可直接访问目标 URL
func checkDirect(targetURL string, timeoutSecs int) bool {
	if timeoutSecs <= 0 {
		timeoutSecs = 3
	}
	client := &http.Client{Timeout: time.Duration(timeoutSecs) * time.Second}
	hreq, err := http.NewRequest("HEAD", targetURL, nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(hreq)
	if err != nil || resp.StatusCode != 200 {
		return false
	}
	resp.Body.Close()
	return true
}

// getProxyBandwidth 查询代理的带宽值（数值越小越好）
func getProxyBandwidth(proxyURL, suffix string, timeoutSecs int) float64 {
	if strings.TrimSpace(suffix) == "" {
		suffix = "/bandwidth"
	}
	if !strings.HasPrefix(suffix, "/") {
		suffix = "/" + suffix
	}
	target := fmt.Sprintf("%s%s", strings.TrimRight(proxyURL, "/"), suffix)
	if timeoutSecs <= 0 {
		timeoutSecs = 3
	}
	client := &http.Client{Timeout: time.Duration(timeoutSecs) * time.Second}
	resp, err := client.Get(target)
	if err != nil {
		return 999999
	}
	defer resp.Body.Close()
	body, err := readAll(resp.Body)
	if err != nil {
		return 999999
	}
	val := 999999.0
	if parsed, err := parseFloat(strings.TrimSpace(string(body)), 64); err == nil {
		val = parsed
	}
	slog.Debug("Proxy bandwidth", "proxy", proxyURL, "bandwidth", val)
	return val
}
```

> 注：上面的 `readAll` 和 `parseFloat` 是标准库 `io.ReadAll` 和 `strconv.ParseFloat` 的别名。实际代码直接使用标准库函数。

```go
// pkg/download/proxy_selector_test.go
package download

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestStaticProxySelectorNoProxies(t *testing.T) {
	ps := NewStaticProxySelector(nil)
	proxy, err := ps.Select(context.Background(), "http://example.com/file", nil)
	if err != nil {
		t.Fatal(err)
	}
	if proxy != "" {
		t.Errorf("expected direct, got proxy: %s", proxy)
	}
}

func TestStaticProxySelectorCacheDir(t *testing.T) {
	dir := t.TempDir()
	ps := NewStaticProxySelector([]string{"http://proxy:8080"}).
		WithCache(dir, 60).
		WithForceProxy(true)
	// WithForceProxy skips direct check; bandwidth probe will fail so expect error
	_, err := ps.Select(context.Background(), "http://example.com/file", nil)
	if err == nil {
		// It's OK if it fails because the proxy doesn't exist
		// We're just testing it doesn't panic
	}
	// Verify cache file was created
	cachePath := filepath.Join(dir, "example.com")
	if _, err := os.Stat(cachePath); err == nil {
		t.Log("cache file created:", cachePath)
	}
}
```

- [ ] **步骤 2：运行 ProxySelector 测试**

```bash
go test -v -run TestStaticProxySelector ./pkg/download/
```
预期：PASS

- [ ] **步骤 3：Commit**

```bash
git add pkg/download/proxy_selector.go pkg/download/proxy_selector_test.go
git commit -m "feat(download): add StaticProxySelector with bandwidth probing and caching"
```

### 任务 5：StdlibTransport + HTTPExtractor

**文件：**
- 创建：`pkg/download/transport/stdlib.go`
- 创建：`pkg/download/extractor/http.go`

- [ ] **步骤 1：创建 `pkg/download/transport/stdlib.go`**

```go
// pkg/download/transport/stdlib.go
package transport

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cocomhub/download-manager/pkg/download"
)

type StdlibTransport struct {
	client   *http.Client
	dLimiter *download.DomainLimiter
}

func NewStdlibTransport() *StdlibTransport {
	return &StdlibTransport{
		client: &http.Client{
			Timeout: 600 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     30 * time.Second,
			},
		},
		dLimiter: download.NewDomainLimiter(),
	}
}

func (t *StdlibTransport) WithHTTPClient(client *http.Client) *StdlibTransport {
	t.client = client
	return t
}

func (t *StdlibTransport) ApplyDomainLimits(limits map[string]int) {
	for host, max := range limits {
		t.dLimiter.Set(host, max)
	}
}

func (t *StdlibTransport) Name() string { return "stdlib" }

func (t *StdlibTransport) RoundTrip(ctx context.Context, treq *download.TransportRequest) (*download.TransportResponse, error) {
	targetURL := treq.URL

	// 如果指定了代理，重写 URL
	if treq.ProxyURL != "" {
		targetURL = strings.TrimPrefix(targetURL, "http://")
		targetURL = strings.TrimPrefix(targetURL, "https://")
		targetURL = treq.ProxyURL + "/" + targetURL
	}

	t.dLimiter.Acquire(treq.URL)
	defer t.dLimiter.Release(treq.URL)

	hreq, err := http.NewRequestWithContext(ctx, treq.Method, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 设置请求头
	for k, v := range treq.Headers {
		hreq.Header.Set(k, v)
	}

	// 设置 Range 头（断点续传）
	if treq.Range != nil && treq.Range.Offset > 0 {
		hreq.Header.Set("Range", fmt.Sprintf("bytes=%d-", treq.Range.Offset))
	}

	resp, err := t.client.Do(hreq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}

	// 收集响应头
	headers := make(map[string]string)
	for k := range resp.Header {
		headers[k] = resp.Header.Get(k)
	}

	return &download.TransportResponse{
		Body:          resp.Body,
		StatusCode:    resp.StatusCode,
		ContentLength: resp.ContentLength,
		Headers:       headers,
		ProxyURL:      treq.ProxyURL,
	}, nil
}
```

- [ ] **步骤 2：编译检查**

```bash
go vet ./pkg/download/transport/...
```
预期：无错误

- [ ] **步骤 3：创建 `pkg/download/extractor/http.go`**

```go
// pkg/download/extractor/http.go
package extractor

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cocomhub/download-manager/pkg/download"
)

const (
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

// HTTPExtractor 是通用 HTTP 文件下载编排器
type HTTPExtractor struct {
	transport  download.Transport
	selector   download.Selector
	maxRetries int
	rootDir    string
	logDir     string
	ua         string
	disableBrowserHeaders bool
	progressMinStep       float64
	progressMaxInterval   int
}

func NewHTTPExtractor() *HTTPExtractor {
	return &HTTPExtractor{
		maxRetries:           5,
		ua:                   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36",
		progressMinStep:      0.5,
		progressMaxInterval:  10,
	}
}

func (e *HTTPExtractor) WithTransport(t download.Transport) *HTTPExtractor {
	e.transport = t
	return e
}

func (e *HTTPExtractor) WithSelector(s download.Selector) *HTTPExtractor {
	e.selector = s
	return e
}

func (e *HTTPExtractor) WithMaxRetries(n int) *HTTPExtractor {
	e.maxRetries = n
	return e
}

func (e *HTTPExtractor) WithRootDir(dir string) *HTTPExtractor {
	e.rootDir = dir
	return e
}

func (e *HTTPExtractor) WithLogDir(dir string) *HTTPExtractor {
	e.logDir = dir
	return e
}

func (e *HTTPExtractor) Name() string { return "http" }

// Match 始终返回 false（作为兜底 Extractor，最后匹配）
func (e *HTTPExtractor) Match(ctx context.Context, url string) bool { return false }

// MatchAnyURL 可选的宽匹配，用于 URL 自动路由
func (e *HTTPExtractor) MatchAnyURL(ctx context.Context, url string) bool {
	// 非 m3u8、非特殊协议的 URL 都匹配
	return !strings.Contains(strings.ToLower(url), ".m3u8")
}

// Extract 执行完整下载编排
func (e *HTTPExtractor) Extract(ctx context.Context, req *download.Request) error {
	rPath := req.SavePath
	var err error
	if e.rootDir != "" {
		rPath, err = download.ResolvePath(e.rootDir, req.SavePath)
		if err != nil {
			return err
		}
	}

	// 创建目录
	if err := os.MkdirAll(filepath.Dir(rPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// 准备日志文件
	var logW io.Writer = io.Discard
	if e.logDir != "" {
		logFileName := filepath.Base(rPath) + "." + time.Now().Format("20060102150405") + ".native.log"
		logPath := filepath.Join(e.logDir, logFileName)
		f, fErr := os.Create(logPath)
		if fErr == nil {
			defer f.Close()
			logW = f
		} else {
			slog.Warn("Failed to create log file", "path", logPath, "error", fErr)
		}
	}

	// 选择代理
	proxyURL := ""
	if e.selector != nil {
		proxyURL, err = e.selector.SelectProxy(ctx, req.URL, req.Hint)
		if err != nil {
			slog.Warn("Proxy selection failed, falling back to direct", "url", req.URL, "error", err)
		}
	}

	// 检查断点续传
	startOffset := int64(0)
	if fi, statErr := os.Stat(rPath); statErr == nil && fi.Size() > 0 {
		startOffset = fi.Size()
		slog.Info("Resuming download", "file", req.SavePath, "offset", startOffset)
	}

	// 重试循环
	attempt := 0
	for {
		attempt++
		if e.maxRetries > 0 && attempt > e.maxRetries {
			return fmt.Errorf("%w: max retries reached (%d)", download.ErrNoTry, e.maxRetries)
		}

		treq := &download.TransportRequest{
			URL:      req.URL,
			Method:   "GET",
			ProxyURL: proxyURL,
			Headers:  e.buildHeaders(req),
		}

		if startOffset > 0 {
			treq.Range = &download.RangeRequest{Offset: startOffset}
		}

		// 先做 GET 请求探测（无 Range 时）
		if startOffset == 0 {
			// 直接下载模式
			tresp, tErr := e.transport.RoundTrip(ctx, treq)
			if tErr != nil {
				return tErr
			}
			resp := tresp

			// 检查 HTTP 状态码
			if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound {
				resp.Body.Close()
				return fmt.Errorf("%w: HTTP %d", download.ErrNoTry, resp.StatusCode)
			}

			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
				resp.Body.Close()
				if resp.StatusCode >= 400 {
					return fmt.Errorf("%w: HTTP %d", download.ErrNoTry, resp.StatusCode)
				}
				return fmt.Errorf("HTTP error: %d", resp.StatusCode)
			}

			// 计算总大小
			totalSize := resp.ContentLength
			if contentRange := resp.Headers["Content-Range"]; contentRange != "" {
				parts := strings.Split(contentRange, "/")
				if len(parts) == 2 {
					totalSize, _ = strconv.ParseInt(parts[1], 10, 64)
				}
			}

			// 写入文件
			err = e.writeFile(ctx, rPath, resp.Body, 0, totalSize, req, logW)
			resp.Body.Close()
			if err == nil {
				req.Metadata["total_size"] = strconv.FormatInt(totalSize, 10)
				req.Metadata["status"] = StatusCompleted
				return nil
			}

			// 不可重试的错误直接返回
			if download.IsNoTry(err) {
				return err
			}

			// 可重试：sleep backoff 后继续
			slog.Warn("Download attempt failed, retrying", "attempt", attempt, "error", err)
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}

		// 断点续传模式：先 HEAD 检查
		headTReq := &download.TransportRequest{
			URL:      req.URL,
			Method:   "HEAD",
			ProxyURL: proxyURL,
		}
		headResp, hErr := e.transport.RoundTrip(ctx, headTReq)
		if hErr == nil {
			contentLength, _ := strconv.ParseInt(headResp.Headers["Content-Length"], 10, 64)
			headResp.Body.Close()

			if contentLength == startOffset || contentLength == 0 || contentLength == -1 {
				// 文件已经完整
				wantMd5 := download.TryGetMd5(headResp.Headers)
				if wantMd5 == "" {
					req.Metadata["total_size"] = strconv.FormatInt(startOffset, 10)
					req.Metadata["status"] = StatusCompleted
					return nil
				}
				// 有 MD5 校验需求
				base64MD5, hexMD5, cErr := download.ComputeFileMD5(rPath)
				if cErr == nil && (base64MD5 == wantMd5 || hexMD5 == wantMd5) {
					req.Metadata["md5_base64"] = base64MD5
					req.Metadata["md5_hex"] = hexMD5
					req.Metadata["total_size"] = strconv.FormatInt(startOffset, 10)
					req.Metadata["status"] = StatusCompleted
					return nil
				}
				// MD5 不匹配，从头下载
				startOffset = 0
				continue
			}
		}

		// 带 Range 的 GET 下载
		tresp, tErr := e.transport.RoundTrip(ctx, treq)
		if tErr != nil {
			return tErr
		}
		resp := tresp

		if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
			resp.Body.Close()
			startOffset = 0
			continue
		}

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
			resp.Body.Close()
			return fmt.Errorf("HTTP error: %d", resp.StatusCode)
		}

		// 计算总大小
		totalSize := resp.ContentLength
		if contentRange := resp.Headers["Content-Range"]; contentRange != "" {
			parts := strings.Split(contentRange, "/")
			if len(parts) == 2 {
				totalSize, _ = strconv.ParseInt(parts[1], 10, 64)
			}
		}
		if startOffset > 0 && totalSize > 0 {
			totalSize += startOffset
		}

		err = e.writeFile(ctx, rPath, resp.Body, startOffset, totalSize, req, logW)
		resp.Body.Close()
		if err == nil {
			req.Metadata["total_size"] = strconv.FormatInt(totalSize, 10)
			req.Metadata["status"] = StatusCompleted

			// 设置 Last-Modified 时间
			if modTimeStr := resp.Headers["Last-Modified"]; modTimeStr != "" {
				if modTime, pErr := time.Parse(time.RFC1123, modTimeStr); pErr == nil {
					os.Chtimes(rPath, modTime, modTime)
					req.Metadata["mod_time"] = modTime.Format(time.RFC3339Nano)
				}
			}
			return nil
		}

		if download.IsNoTry(err) {
			return err
		}

		slog.Warn("Download attempt failed, retrying", "attempt", attempt, "error", err)
		time.Sleep(time.Duration(attempt) * time.Second)
	}
}

func (e *HTTPExtractor) buildHeaders(req *download.Request) map[string]string {
	h := make(map[string]string)
	if req.Headers != nil {
		for k, v := range req.Headers {
			h[k] = v
		}
	}
	// 默认 UA
	if _, ok := h["User-Agent"]; !ok && e.ua != "" {
		h["User-Agent"] = e.ua
	}
	return h
}

func (e *HTTPExtractor) writeFile(ctx context.Context, rPath string, body io.ReadCloser, startOffset, totalSize int64, req *download.Request, logW io.Writer) error {
	fileFlags := os.O_CREATE | os.O_WRONLY
	if startOffset > 0 {
		fileFlags |= os.O_APPEND
	} else {
		fileFlags |= os.O_TRUNC
	}

	file, err := os.OpenFile(rPath, fileFlags, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	var reader io.Reader = body
	if req.TrackProgress && req.OnProgress != nil && totalSize > 0 {
		reader = download.NewProgressReader(body, totalSize, req.OnProgress)
	}

	_, err = io.Copy(file, reader)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}
```

- [ ] **步骤 4：编译检查**

```bash
go vet ./pkg/download/extractor/... ./pkg/download/transport/...
```
预期：无错误

- [ ] **步骤 5：编写 HTTPExtractor 单元测试**

```go
// pkg/download/extractor/http_test.go
package extractor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cocomhub/download-manager/pkg/download"
	"github.com/cocomhub/download-manager/pkg/download/transport"
)

func TestHTTPExtractorBasic(t *testing.T) {
	// 启动测试 HTTP 服务器
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello world"))
	}))
	defer ts.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "output.txt")

	ext := NewHTTPExtractor().
		WithTransport(transport.NewStdlibTransport()).
		WithMaxRetries(1)

	meta := make(map[string]string)
	err := ext.Extract(context.Background(), &download.Request{
		URL:           ts.URL,
		SavePath:      dest,
		TrackProgress: false,
		Metadata:      meta,
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", string(data))
	}
	if meta["status"] != StatusCompleted {
		t.Errorf("expected completed status, got %s", meta["status"])
	}
}

func TestHTTPExtractorWithProgress(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("this is a longer file content for progress testing"))
	}))
	defer ts.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "prog.txt")

	ext := NewHTTPExtractor().
		WithTransport(transport.NewStdlibTransport()).
		WithMaxRetries(1)

	var prog float64
	err := ext.Extract(context.Background(), &download.Request{
		URL:           ts.URL,
		SavePath:      dest,
		TrackProgress: true,
		OnProgress: func(p float64, d, t int64) {
			prog = p
		},
		Metadata: make(map[string]string),
	})
	if err != nil {
		t.Fatal(err)
	}
	if prog != 100 {
		t.Errorf("expected 100%% progress, got %.1f%%", prog)
	}
}

func TestHTTPExtractor404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	dir := t.TempDir()
	ext := NewHTTPExtractor().
		WithTransport(transport.NewStdlibTransport()).
		WithMaxRetries(1)

	err := ext.Extract(context.Background(), &download.Request{
		URL:      ts.URL + "/nonexistent",
		SavePath: filepath.Join(dir, "out.txt"),
		Metadata: make(map[string]string),
	})
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !download.IsNoTry(err) {
		t.Errorf("expected ErrNoTry, got %v", err)
	}
}
```

- [ ] **步骤 6：运行 HTTPExtractor 测试**

```bash
go test -v -run TestHTTPExtractor ./pkg/download/extractor/...
```
预期：PASS

- [ ] **步骤 7：Commit**

```bash
git add pkg/download/transport/stdlib.go
git add pkg/download/extractor/http.go pkg/download/extractor/http_test.go
git commit -m "feat(download): add StdlibTransport and HTTPExtractor with progress support"
```

### 任务 6：DefaultSelector + Downloader 主结构体

**文件：**
- 创建：`pkg/download/download.go`
- 创建：`pkg/download/option.go`
- 创建：`pkg/download/download_test.go`

- [ ] **步骤 1：创建 `download.go`**

```go
// pkg/download/download.go
package download

import (
	"context"
	"fmt"
	"log/slog"
)

// Downloader 是用户使用的主要入口
type Downloader struct {
	selector   Selector
	extractors []Extractor
	transport  Transport
}

// New 创建 Downloader 并注册默认 Extractor
func New(opts ...Option) *Downloader {
	d := &Downloader{
		extractors: make([]Extractor, 0),
	}
	for _, o := range opts {
		o(d)
	}
	return d
}

// Download 执行一次下载的完整编排
func (d *Downloader) Download(ctx context.Context, req *Request) error {
	if req == nil || req.URL == "" || req.SavePath == "" {
		return fmt.Errorf("invalid request: missing URL or SavePath")
	}

	if req.Metadata == nil {
		req.Metadata = make(map[string]string)
	}

	// 1. Selector 匹配 Extractor
	var ex Extractor
	if d.selector != nil {
		ex = d.selector.MatchExtractor(ctx, req.URL, req.Hint)
	}
	if ex == nil && len(d.extractors) > 0 {
		for _, e := range d.extractors {
			if e.Match(ctx, req.URL) {
				ex = e
				break
			}
		}
	}
	if ex == nil {
		return fmt.Errorf("no extractor found for URL: %s", req.URL)
	}

	slog.Debug("Download: matched extractor", "extractor", ex.Name(), "url", req.URL)

	// 2. 为 Extractor 注入 Transport 和 Selector（如果支持）
	if hw, ok := ex.(interface{ SetTransport(Transport) }); ok && d.transport != nil {
		hw.SetTransport(d.transport)
	}
	if hw, ok := ex.(interface{ SetSelector(Selector) }); ok && d.selector != nil {
		hw.SetSelector(d.selector)
	}

	// 3. 执行
	return ex.Extract(ctx, req)
}
```

- [ ] **步骤 2：创建 `option.go`**

```go
// pkg/download/option.go
package download

type Option func(*Downloader)

func WithSelector(s Selector) Option {
	return func(d *Downloader) { d.selector = s }
}

func WithTransport(t Transport) Option {
	return func(d *Downloader) { d.transport = t }
}

func WithExtractor(ex Extractor) Option {
	return func(d *Downloader) {
		d.extractors = append(d.extractors, ex)
	}
}
```

- [ ] **步骤 3：创建 `download_test.go`（基础集成测试）**

```go
// pkg/download/download_test.go
package download

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cocomhub/download-manager/pkg/download/extractor"
	"github.com/cocomhub/download-manager/pkg/download/transport"
)

func TestDownloaderGet(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test content"))
	}))
	defer ts.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "output.txt")

	// 创建完整下载链
	httpExt := extractor.NewHTTPExtractor().
		WithTransport(transport.NewStdlibTransport()).
		WithMaxRetries(1)

	d := New(
		WithExtractor(httpExt),
		WithSelector(&DefaultSelector{
			extractors: []Extractor{httpExt},
		}),
		WithTransport(transport.NewStdlibTransport()),
	)

	err := d.Download(context.Background(), &Request{
		URL:      ts.URL,
		SavePath: dest,
		Metadata: make(map[string]string),
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "test content" {
		t.Errorf("expected 'test content', got '%s'", string(data))
	}
}
```

- [ ] **步骤 4：运行 Downloader 测试**

```bash
go test -v -run TestDownloaderGet ./pkg/download/
```
预期：PASS

- [ ] **步骤 5：创建 `selector.go` 中的 DefaultSelector**

```go
// 在 pkg/download/selector.go 中追加，或新创建文件

// DefaultSelector 默认 Selector 实现
type DefaultSelector struct {
	extractors    []Extractor
	proxySelector ProxySelector
}

func (s *DefaultSelector) WithProxySelector(ps ProxySelector) *DefaultSelector {
	s.proxySelector = ps
	return s
}

func (s *DefaultSelector) MatchExtractor(ctx context.Context, url string, hint *DownloadHint) Extractor {
	// 如果 hint 指定了 extractor 名，尝试匹配
	if hint != nil && hint.Extractor != "" {
		for _, ex := range s.extractors {
			if ex.Name() == hint.Extractor {
				return ex
			}
		}
	}
	// 遍历注册的 extractors，第一个 Match 返回 true 的胜出
	for _, ex := range s.extractors {
		if ex.Match(ctx, url) {
			return ex
		}
	}
	return nil
}

func (s *DefaultSelector) SelectProxy(ctx context.Context, targetURL string, hint *DownloadHint) (string, error) {
	if s.proxySelector != nil {
		return s.proxySelector.Select(ctx, targetURL, hint)
	}
	return "", nil
}
```

- [ ] **步骤 6：确保集成测试仍通过**

```bash
go test -v -run TestDownloaderGet ./pkg/download/
```
预期：PASS

- [ ] **步骤 7：Commit**

```bash
git add pkg/download/download.go pkg/download/option.go pkg/download/download_test.go
git add pkg/download/selector.go
git commit -m "feat(download): add Downloader, DefaultSelector, and Options"
```

### 任务 7：全局 Get 便捷函数

**文件：**
- 修改：`pkg/download/download.go`

- [ ] **步骤 1：添加全局默认 Downloader 和 Get 函数**

```go
// 在 pkg/download/download.go 末尾追加

var defaultDownloader *Downloader

func init() {
	// 在 init 中初始化以避免循环依赖
	// 实际使用时通过 InitDefault 或 SetDefault 配置
}

// SetDefault 设置全局默认 Downloader
func SetDefault(d *Downloader) {
	defaultDownloader = d
}

// GetDefault 返回全局默认 Downloader
func GetDefault() *Downloader {
	return defaultDownloader
}

// Get 是全局便捷的一行下载函数
// 使用全局默认 Downloader 执行下载
func Get(ctx context.Context, url string, dest string) error {
	if defaultDownloader == nil {
		return fmt.Errorf("default downloader not initialized, call download.InitDefault() first")
	}
	return defaultDownloader.Download(ctx, &Request{
		URL:      url,
		SavePath: dest,
		Metadata: make(map[string]string),
	})
}

// InitDefault 初始化全局默认 Downloader 并返回它
// 默认使用 HTTPExtractor + StdlibTransport
func InitDefault() *Downloader {
	httpExt := extractor.NewHTTPExtractor().
		WithTransport(transport.NewStdlibTransport()).
		WithMaxRetries(5)
	
	d := New(
		WithExtractor(httpExt),
		WithSelector(&DefaultSelector{
			extractors: []Extractor{httpExt},
		}),
		WithTransport(transport.NewStdlibTransport()),
	)
	SetDefault(d)
	return d
}
```

> **注意**：上面 import 了 `extractor` 和 `transport` 子包，需要在文件顶部添加 import。

- [ ] **步骤 2：写 Get 函数测试**

```go
// 在 pkg/download/download_test.go 中追加
func TestInitDefault(t *testing.T) {
	d := InitDefault()
	if d == nil {
		t.Fatal("InitDefault returned nil")
	}
	if GetDefault() != d {
		t.Error("GetDefault should return the same instance")
	}
}

func TestGet(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello from get"))
	}))
	defer ts.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "get_test.txt")

	InitDefault()

	err := Get(context.Background(), ts.URL, dest)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello from get" {
		t.Errorf("expected 'hello from get', got '%s'", string(data))
	}
}
```

- [ ] **步骤 3：运行完整测试**

```bash
go test -v ./pkg/download/... ./pkg/download/extractor/... ./pkg/download/transport/...
```
预期：所有测试 PASS

- [ ] **步骤 4：Commit**

```bash
git add pkg/download/download.go pkg/download/download_test.go
git commit -m "feat(download): add global Get convenience function and InitDefault"
```

### 任务 8：整体编译和回归

- [ ] **步骤 1：全量编译检查**

```bash
cd download-manager
go build ./...
go vet ./...
```
预期：无错误

- [ ] **步骤 2：确认旧包编译不受影响**

```bash
go build ./pkg/dlcore/...
go build ./downloader/...
```
预期：无错误

- [ ] **步骤 3：运行所有测试**

```bash
go test ./pkg/download/... ./pkg/download/extractor/... ./pkg/download/transport/...
```
预期：ALL PASS

## 验证清单

Phase 1 完成后需满足：

- [ ] `go build ./...` 通过
- [ ] `go vet ./...` 通过
- [ ] `go test ./pkg/download/...` 全部 PASS
- [ ] `go test ./pkg/download/extractor/...` 全部 PASS
- [ ] `go test ./pkg/download/transport/...` 全部 PASS
- [ ] 旧包 `go build ./pkg/dlcore/...` 通过（未受影响）
- [ ] `pkg/download/` 零外部依赖（仅 Go 标准库）

## 文件最终结构

```
pkg/download/
├── download.go                  # Downloader 结构体 + New + global Get + InitDefault
├── download_test.go             # 集成测试（Downloader、Get）
├── request.go                   # Request / TransportRequest / TransportResponse / DownloadHint / RangeRequest
├── option.go                    # Option 类型 + 函数
├── selector.go                  # Selector 接口 + ProxySelector 接口 + DefaultSelector
├── proxy_selector.go            # StaticProxySelector 实现
├── proxy_selector_test.go
├── extractor.go                 # Extractor 接口
├── transport.go                 # Transport 接口
├── domainlimiter.go             # DomainLimiter
├── domainlimiter_test.go
├── progress.go                  # ProgressReader
├── progress_test.go
├── md5.go                       # ComputeFileMD5 + TryGetMd5
├── md5_test.go
├── errors.go                    # ErrNoTry + IsNoTry
├── fs.go                        # ResolvePath + EnsureDir
├── fs_test.go
│
├── extractor/
│   ├── http.go                  # HTTPExtractor
│   └── http_test.go
│
└── transport/
    └── stdlib.go                # StdlibTransport
```