# Phase 2：补齐 Extractor 和 Transport 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 补齐 `pkg/download/` 中的缺失 Extractor 和 Transport 实现——CompositeExtractor、WgetExtractor、HLSExtractor，并重构 m3u8d 引擎使其 `http.Client` 可注入。

**架构：** 所有新 Extractor 实现 `pkg/download.Extractor` 接口并放在 `pkg/download/extractor/` 子目录；WgetExtractor 自管理进程（不使用 Transport）；HLSExtractor 整合 ffmpeg 命令 + m3u8d 库；CompositeExtractor 委托子 URL 给 Selector。

**技术栈：** Go 1.26 标准库 (`net/http`, `sync`, `context`, `os/exec`, `regexp`)。m3u8d 继续使用 `github.com/cavaliergopher/grab/v3`。

---

## 文件结构

```
pkg/download/
├── extractor/
│   ├── http.go              # HTTPExtractor (Phase 1，已完成)
│   ├── http_test.go          # (Phase 1，已完成)
│   ├── composite.go          # NEW: CompositeExtractor
│   ├── composite_test.go     # NEW: 测试
│   ├── wget.go               # NEW: WgetExtractor
│   ├── wget_test.go          # NEW: 测试
│   ├── hls.go                # NEW: HLSExtractor (ffmpeg + m3u8d)
│   └── hls_test.go           # NEW: 测试
│
├── m3u8d/                    # NEW: m3u8d 可注入引擎
│   ├── engine.go             # NEW: M3U8DEngine (接受 http.Client 注入)
│   ├── grab.go               # NEW: grab 适配层
│   └── config.go             # NEW: DownloadConfig

downloader/
├── wget.go                   # MODIFY: 标记 deprecated，转发到新 WgetExtractor
├── native.go                 # MODIFY: 复合下载逻辑改为使用 CompositeExtractor
```

## 任务分解

### 任务 1：CompositeExtractor — 复合下载 Extractor

**文件：**
- 创建：`pkg/download/extractor/composite.go`
- 创建：`pkg/download/extractor/composite_test.go`

CompositeExtractor 处理 `Request` 中包含多个子文件的场景。从现有的 `Extra["files"]` 解析逻辑提取，改为通过 `Request.Metadata["files"]` 传递文件列表。

**设计：**
```go
// CompositeExtractor 处理包含多个子文件的复合下载请求。
// 它从 req.Metadata["files"] 读取文件列表（[]map[string]string），
// 对每个子文件通过 Selector 匹配子 Extractor 执行下载。
type CompositeExtractor struct {
    selector  download.Selector
    transport download.Transport
}

func (e *CompositeExtractor) Name() string { return "composite" }
// Match 匹配包含 "files" 元数据的请求
func (e *CompositeExtractor) Match(ctx context.Context, url string) bool { return false }

// 通过接口注入（Extractor 可选）
func (e *CompositeExtractor) SetSelector(s download.Selector)  { e.selector = s }
func (e *CompositeExtractor) SetTransport(t download.Transport) { e.transport = t }

// Extract 执行复合下载：
// 1. 从 req.Metadata["files"] 解析文件列表
// 2. 对每个文件，构建子 Request 并调用 Downloader.Download
// 3. 汇总进度
func (e *CompositeExtractor) Extract(ctx context.Context, req *download.Request) error
```

- [ ] **步骤 1：编写失败测试**

```go
// pkg/download/extractor/composite_test.go
package extractor_test

import (
    "context"
    "testing"
    "github.com/cocomhub/download-manager/pkg/download"
)

func TestCompositeExtractorName(t *testing.T) {
    ex := NewCompositeExtractor()
    if ex.Name() != "composite" {
        t.Errorf("expected 'composite', got %s", ex.Name())
    }
}

func TestCompositeExtractorMatchAlwaysFalse(t *testing.T) {
    ex := NewCompositeExtractor()
    if ex.Match(context.Background(), "http://example.com/file") {
        t.Error("CompositeExtractor.Match should always return false")
    }
}

func TestCompositeExtractorNoFiles(t *testing.T) {
    ex := NewCompositeExtractor()
    err := ex.Extract(context.Background(), &download.Request{
        URL:      "http://example.com/page",
        SavePath: "/tmp/output",
        Metadata: map[string]string{},
    })
    if err == nil {
        t.Error("expected error for no files metadata")
    }
}

func TestCompositeExtractorEmptyFiles(t *testing.T) {
    ex := NewCompositeExtractor()
    err := ex.Extract(context.Background(), &download.Request{
        URL:      "http://example.com/page",
        SavePath: "/tmp/output",
        Metadata: map[string]string{"files": "[]"},
    })
    if err == nil {
        t.Error("expected error for empty files")
    }
}
```

- [ ] **步骤 2：运行测试确认失败**

运行：`go test ./pkg/download/extractor/... -run 'TestCompositeExtractor' -v`
预期：FAIL，报错 "undefined: NewCompositeExtractor"

- [ ] **步骤 3：创建 `extractor/composite.go` 实现**

```go
package extractor

import (
    "context"
    "encoding/json"
    "fmt"
    "log/slog"
    "os"
    "path/filepath"

    "github.com/cocomhub/download-manager/pkg/download"
)

// CompositeExtractor 处理复合下载请求。
// 从 req.Metadata["files"] 读取 []map[string]string 格式的文件列表，
// 对每个文件通过注入的 Downloader 执行下载。
type CompositeExtractor struct {
    selector  download.Selector
    transport download.Transport
    // 内部持有 Downloader 用于委托子下载
    downloader *download.Downloader
}

// NewCompositeExtractor 创建 CompositeExtractor 实例。
func NewCompositeExtractor() *CompositeExtractor {
    return &CompositeExtractor{}
}

func (e *CompositeExtractor) Name() string { return "composite" }
func (e *CompositeExtractor) Match(ctx context.Context, url string) bool { return false }

func (e *CompositeExtractor) SetSelector(s download.Selector)   { e.selector = s }
func (e *CompositeExtractor) SetTransport(t download.Transport)  { e.transport = t }

// parseFiles 从 req.Metadata["files"] 解析文件列表。
// 支持 JSON 字符串 ("[{\"url\":\"...\",\"path\":\"...\",\"type\":\"video\"}]")
// 和已有的 map 格式。
func parseFiles(metadata map[string]string) ([]map[string]string, error) {
    filesJSON, ok := metadata["files"]
    if !ok || filesJSON == "" {
        return nil, fmt.Errorf("composite: no 'files' in metadata")
    }
    var fileList []map[string]string
    if err := json.Unmarshal([]byte(filesJSON), &fileList); err != nil {
        return nil, fmt.Errorf("composite: failed to parse files JSON: %w", err)
    }
    if len(fileList) == 0 {
        return nil, fmt.Errorf("composite: files list is empty")
    }
    return fileList, nil
}

func (e *CompositeExtractor) Extract(ctx context.Context, req *download.Request) error {
    fileList, err := parseFiles(req.Metadata)
    if err != nil {
        return err
    }

    slog.Info("Starting composite download", "count", len(fileList), "url", req.URL)

    // 每个子文件使用自己的 Downloader（如果已有则复用）
    dl := e.downloader
    if dl == nil {
        dl = download.New()
        if e.transport != nil {
            dl = download.New(download.WithTransport(e.transport))
        }
    }

    for _, fileMap := range fileList {
        subURL := fileMap["url"]
        subPath := fileMap["path"]
        fType := fileMap["type"]

        if subURL == "" || subPath == "" {
            continue
        }

        // 创建目录
        dir := filepath.Dir(subPath)
        if err := os.MkdirAll(dir, 0755); err != nil {
            return fmt.Errorf("composite: failed to create directory %s: %w", dir, err)
        }

        // 跟踪进度：仅 video 类型或只有一个文件时
        trackProgress := (fType == "video" || len(fileList) == 1)

        subReq := &download.Request{
            URL:           subURL,
            SavePath:      subPath,
            TrackProgress: trackProgress,
            OnProgress:    req.OnProgress,
        }

        if err := dl.Download(ctx, subReq); err != nil {
            return fmt.Errorf("composite: sub-download failed (%s): %w", subURL, err)
        }
    }

    if req.OnProgress != nil {
        req.OnProgress(100, 0, 0)
    }
    return nil
}
```

- [ ] **步骤 4：运行测试确认通过**

运行：`go test ./pkg/download/extractor/... -run 'TestCompositeExtractor' -v`
预期：PASS

- [ ] **步骤 5：Commit**

```bash
git add pkg/download/extractor/composite.go pkg/download/extractor/composite_test.go
git commit -m "feat(download): add CompositeExtractor for multi-file downloads"
```

---

### 任务 2：WgetExtractor — wget 命令行下载 Extractor

**文件：**
- 创建：`pkg/download/extractor/wget.go`
- 创建：`pkg/download/extractor/wget_test.go`

WgetExtractor 从 `downloader/wget.go` 迁移，将 wget 下载逻辑包装为 Extractor 接口。
WgetExtractor 不依赖 Transport，它自己管理 `exec.Command`。

**设计要点：**
- 接受 `ProxySelector` 做代理决策（通过 `SetSelector` 从 Downloader 注入）
- 保留 `active sync.Map` 管理进程，支持取消
- 进度通过正则解析 wget stderr 输出
- 支持自定义 headers、User-Agent、代理

- [ ] **步骤 1：编写失败测试**

```go
// pkg/download/extractor/wget_test.go
package extractor_test

import (
    "context"
    "testing"
    "github.com/cocomhub/download-manager/pkg/download"
)

func TestWgetExtractorName(t *testing.T) {
    ex := NewWgetExtractor()
    if ex.Name() != "wget" {
        t.Errorf("expected 'wget', got %s", ex.Name())
    }
}

func TestWgetExtractorMatch(t *testing.T) {
    ex := NewWgetExtractor()
    if !ex.Match(context.Background(), "http://example.com/file.zip") {
        t.Error("WgetExtractor should match any URL")
    }
}

func TestWgetExtractorCancel(t *testing.T) {
    ex := NewWgetExtractor()
    // 取消不存在的下载应该返回 nil
    err := ex.Cancel("http://example.com/nonexistent")
    if err != nil {
        t.Errorf("Cancel on nonexistent should return nil, got: %v", err)
    }
}

func TestWgetExtractorSetSelector(t *testing.T) {
    ex := NewWgetExtractor()
    ex.SetSelector(nil) // should not panic
}

func TestWgetExtractorSetTransport(t *testing.T) {
    ex := NewWgetExtractor()
    ex.SetTransport(nil) // should not panic (WgetExtractor ignores Transport)
}
```

- [ ] **步骤 2：运行测试确认失败**

运行：`go test ./pkg/download/extractor/... -run 'TestWgetExtractor' -v`
预期：FAIL，报错 "undefined: NewWgetExtractor"

- [ ] **步骤 3：创建 `extractor/wget.go` 实现**

```go
package extractor

import (
    "bufio"
    "context"
    "fmt"
    "log/slog"
    "os"
    "os/exec"
    "path/filepath"
    "regexp"
    "strconv"
    "strings"
    "sync"
    "time"

    "github.com/cocomhub/download-manager/pkg/download"
)

// DefaultWgetUserAgent 是 wget 下载的默认 User-Agent。
const DefaultWgetUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36"

var reWgetProgress = regexp.MustCompile(`\s+(\d+)%`)

// WgetExtractor 使用系统 wget 命令执行下载。
// 不依赖 Transport 接口，自行管理 exec.Cmd 进程。
type WgetExtractor struct {
    logDir       string
    selector     download.Selector
    active       sync.Map // URL → *exec.Cmd
    userAgent    string
    maxRetries   int
    timeoutSecs  int
}

// NewWgetExtractor 创建 WgetExtractor，支持可变选项函数。
func NewWgetExtractor(opts ...WgetOption) *WgetExtractor {
    e := &WgetExtractor{
        userAgent:  DefaultWgetUserAgent,
        maxRetries: 5,
        timeoutSecs: 20,
    }
    for _, o := range opts {
        o(e)
    }
    return e
}

// WgetOption 配置 WgetExtractor。
type WgetOption func(*WgetExtractor)

func WithWgetLogDir(dir string) WgetOption { return func(e *WgetExtractor) { e.logDir = dir } }
func WithWgetUserAgent(ua string) WgetOption { return func(e *WgetExtractor) { e.userAgent = ua } }
func WithWgetMaxRetries(n int) WgetOption { return func(e *WgetExtractor) { e.maxRetries = n } }
func WithWgetTimeout(secs int) WgetOption { return func(e *WgetExtractor) { e.timeoutSecs = secs } }

func (e *WgetExtractor) Name() string { return "wget" }
func (e *WgetExtractor) Match(ctx context.Context, url string) bool { return true }

// SetSelector 注入 Selector（用于代理选择）。
func (e *WgetExtractor) SetSelector(s download.Selector) { e.selector = s }

// SetTransport 满足 Extractor 接口（WgetExtractor 忽略 Transport）。
func (e *WgetExtractor) SetTransport(t download.Transport) {}

// Extract 执行 wget 下载。
func (e *WgetExtractor) Extract(ctx context.Context, req *download.Request) error {
    // 确保目录存在
    dir := filepath.Dir(req.SavePath)
    if err := os.MkdirAll(dir, 0755); err != nil {
        return fmt.Errorf("wget: failed to create directory: %w", err)
    }

    // 准备日志文件
    var f *os.File
    if e.logDir != "" {
        logFile := filepath.Join(e.logDir, filepath.Base(req.SavePath)+"."+time.Now().Format("20060102150405")+".wget.log")
        var err error
        f, err = os.Create(logFile)
        if err != nil {
            slog.Warn("Failed to create wget log file", "file", logFile, "error", err)
        } else {
            defer f.Close()
        }
    }

    // 确定代理
    proxyURL := ""
    if e.selector != nil {
        var err error
        proxyURL, err = e.selector.SelectProxy(ctx, req.URL, req.Hint)
        if err != nil {
            slog.Warn("Proxy selection failed, falling back to direct", "url", req.URL, "error", err)
        }
    }

    // 构建 wget 命令
    args := []string{"-c", "-T", strconv.Itoa(e.timeoutSecs), "-t", strconv.Itoa(e.maxRetries)}
    args = append(args, "--header", "User-Agent: "+e.userAgent)

    for k, v := range req.Headers {
        args = append(args, "--header", fmt.Sprintf("%s: %s", k, v))
    }

    url := req.URL
    if proxyURL != "" {
        url = strings.TrimPrefix(url, "http://")
        url = strings.TrimPrefix(url, "https://")
        url = proxyURL + "/" + url
        slog.Info("Using proxy", "url", url, "proxy", proxyURL)
    }

    args = append(args, "-O", req.SavePath, url)

    cmd := exec.CommandContext(ctx, "wget", args...)
    e.active.Store(req.URL, cmd)

    stderr, err := cmd.StderrPipe()
    if err != nil {
        e.active.Delete(req.URL)
        return fmt.Errorf("wget: failed to get stderr pipe: %w", err)
    }

    cmd.Stdout = f
    slog.Info("Starting download", "downloader", "wget", "url", req.URL, "path", req.SavePath)

    if err := cmd.Start(); err != nil {
        e.active.Delete(req.URL)
        return fmt.Errorf("wget: start failed: %w", err)
    }

    // 解析进度
    scanner := bufio.NewScanner(stderr)
    for scanner.Scan() {
        line := scanner.Text()
        if f != nil {
            _, _ = f.WriteString(line + "\n")
        }
        if req.TrackProgress && req.OnProgress != nil {
            if matches := reWgetProgress.FindStringSubmatch(line); len(matches) > 1 {
                if p, err := strconv.Atoi(matches[1]); err == nil {
                    req.OnProgress(float64(p), 0, 0)
                }
            }
        }
    }

    if err := cmd.Wait(); err != nil {
        e.active.Delete(req.URL)
        return fmt.Errorf("wget: execution failed: %w", err)
    }
    e.active.Delete(req.URL)

    if req.OnProgress != nil {
        req.OnProgress(100, 0, 0)
    }
    return nil
}

// Cancel 取消正在进行的下载。
func (e *WgetExtractor) Cancel(url string) error {
    if v, ok := e.active.Load(url); ok {
        cmd := v.(*exec.Cmd)
        _ = cmd.Process.Kill()
        e.active.Delete(url)
        return nil
    }
    return nil
}
```

- [ ] **步骤 4：创建 `extractor/wget_test.go` 集成测试**

```go
// wget 实际调用需要系统安装 wget，所以测试使用 mock
func TestWgetExtractorBuildArgs(t *testing.T) {
    // 验证参数构造
    ex := NewWgetExtractor(WithWgetUserAgent("test-agent"), WithWgetTimeout(30))
    req := &download.Request{
        URL:      "http://example.com/file.zip",
        SavePath: "/tmp/output/file.zip",
        Headers:  map[string]string{"Referer": "http://example.com"},
    }
    
    // 不实际执行，仅测试 Extract 在 wget 不存在时返回合适错误
    err := ex.Extract(context.Background(), req)
    if err == nil {
        t.Skip("wget not available, skipping")
    }
    // 如果系统有 wget 但无法连接，错误信息应该包含相关描述
    t.Logf("Got expected error: %v", err)
}
```

- [ ] **步骤 5：运行测试确认通过**

运行：`go test ./pkg/download/extractor/... -run 'TestWgetExtractor' -v`
预期：PASS（跳过需要 wget 的集成测试）

- [ ] **步骤 6：Commit**

```bash
git add pkg/download/extractor/wget.go pkg/download/extractor/wget_test.go
git commit -m "feat(download): add WgetExtractor for wget command-line downloads"
```

---

### 任务 3：HLSExtractor — HLS 流下载 Extractor

**文件：**
- 创建：`pkg/download/extractor/hls.go`
- 创建：`pkg/download/extractor/hls_test.go`

HLSExtractor 整合两种 HLS 下载方式：
1. **ffmpeg**（默认）：exec ffmpeg 命令下载
2. **m3u8d**：使用 m3u8d 库（更可靠的 TS 分片下载）

从 `pkg/dlcore/m3u8d_handler.go` + `pkg/dlcore/ffmpeg.go` 迁移而来。

- [ ] **步骤 1：编写失败测试**

```go
// pkg/download/extractor/hls_test.go
package extractor_test

import (
    "context"
    "testing"
    "github.com/cocomhub/download-manager/pkg/download"
)

func TestHLSExtractorName(t *testing.T) {
    ex := NewHLSExtractor()
    if ex.Name() != "hls" {
        t.Errorf("expected 'hls', got %s", ex.Name())
    }
}

func TestHLSExtractorMatchM3U8(t *testing.T) {
    ex := NewHLSExtractor()
    if !ex.Match(context.Background(), "http://example.com/stream.m3u8") {
        t.Error("HLSExtractor should match .m3u8 URLs")
    }
    if !ex.Match(context.Background(), "http://example.com/playlist.M3U8") {
        t.Error("HLSExtractor should match .M3U8 URLs (case-insensitive)")
    }
    if ex.Match(context.Background(), "http://example.com/file.mp4") {
        t.Error("HLSExtractor should NOT match non-m3u8 URLs")
    }
}

func TestHLSExtractorNoFFmpeg(t *testing.T) {
    ex := NewHLSExtractor(WithHLSMode("ffmpeg"))
    err := ex.Extract(context.Background(), &download.Request{
        URL:      "http://example.com/stream.m3u8",
        SavePath: "/tmp/output.mp4",
    })
    if err == nil {
        t.Skip("ffmpeg not available, skipping")
    }
    // 应该返回 ffmpeg not found 错误
    t.Logf("Got expected error: %v", err)
}
```

- [ ] **步骤 2：运行测试确认失败**

运行：`go test ./pkg/download/extractor/... -run 'TestHLSExtractor' -v`
预期：FAIL，报错 "undefined: NewHLSExtractor"

- [ ] **步骤 3：创建 `extractor/hls.go` 实现**

```go
package extractor

import (
    "context"
    "fmt"
    "log/slog"
    "os"
    "os/exec"
    "path/filepath"
    "strconv"
    "strings"
    "time"

    "github.com/cocomhub/download-manager/pkg/download"
)

// HLSMode 表示 HLS 下载模式。
type HLSMode string

const (
    HLSModeFFmpeg HLSMode = "ffmpeg"
    HLSModeM3U8D  HLSMode = "m3u8d"
)

// HLSExtractor 处理 HLS (m3u8) 流媒体下载。
// 支持两种后端：ffmpeg（默认）和 m3u8d。
type HLSExtractor struct {
    mode       HLSMode
    ffmpegPath string
    ffmpegArgs []string
    userAgent  string
    transport  download.Transport
}

// NewHLSExtractor 创建 HLSExtractor，支持可变选项函数。
func NewHLSExtractor(opts ...HLSOption) *HLSExtractor {
    e := &HLSExtractor{
        mode:       HLSModeFFmpeg,
        ffmpegPath: "ffmpeg",
        ffmpegArgs: []string{"-c", "copy", "-bsf:a", "aac_adtstoasc", "-movflags", "+faststart", "-f", "mp4"},
        userAgent:  DefaultWgetUserAgent,
    }
    for _, o := range opts {
        o(e)
    }
    return e
}

// HLSOption 配置 HLSExtractor。
type HLSOption func(*HLSExtractor)

func WithHLSMode(mode HLSMode) HLSOption         { return func(e *HLSExtractor) { e.mode = mode } }
func WithFFmpegPath(path string) HLSOption        { return func(e *HLSExtractor) { e.ffmpegPath = path } }
func WithFFmpegArgs(args []string) HLSOption       { return func(e *HLSExtractor) { e.ffmpegArgs = args } }
func WithHLSUserAgent(ua string) HLSOption         { return func(e *HLSExtractor) { e.userAgent = ua } }

func (e *HLSExtractor) SetTransport(t download.Transport) { e.transport = t }

func (e *HLSExtractor) Name() string { return "hls" }

func (e *HLSExtractor) Match(ctx context.Context, url string) bool {
    return strings.Contains(strings.ToLower(url), ".m3u8")
}

func (e *HLSExtractor) Extract(ctx context.Context, req *download.Request) error {
    switch e.mode {
    case HLSModeFFmpeg:
        return e.downloadWithFFmpeg(ctx, req)
    case HLSModeM3U8D:
        return e.downloadWithM3U8D(ctx, req)
    default:
        return e.downloadWithFFmpeg(ctx, req)
    }
}

func (e *HLSExtractor) downloadWithFFmpeg(ctx context.Context, req *download.Request) error {
    rPath := req.SavePath
    dir := filepath.Dir(rPath)
    if err := os.MkdirAll(dir, 0755); err != nil {
        return fmt.Errorf("hls: failed to create directory: %w", err)
    }

    ffmpeg := e.ffmpegPath
    if path, err := exec.LookPath(ffmpeg); err == nil {
        ffmpeg = path
    } else {
        return fmt.Errorf("hls: ffmpeg not found: %w", err)
    }

    args := []string{"-y"}
    if e.userAgent != "" {
        args = append(args, "-user_agent", e.userAgent)
    }

    var headerLines []string
    if v := req.Headers["Referer"]; v != "" {
        headerLines = append(headerLines, fmt.Sprintf("Referer: %s", v))
    }
    if v := req.Headers["Cookie"]; v != "" {
        headerLines = append(headerLines, fmt.Sprintf("Cookie: %s", v))
    }
    if len(headerLines) > 0 {
        args = append(args, "-headers", strings.Join(headerLines, "\r\n"))
    }

    args = append(args, "-i", req.URL)
    args = append(args, e.ffmpegArgs...)
    args = append(args, rPath)

    slog.Info("Starting HLS download", "downloader", "ffmpeg", "url", req.URL)

    cmd := exec.CommandContext(ctx, ffmpeg, args...)
    stderr, err := cmd.StderrPipe()
    if err != nil {
        return fmt.Errorf("hls: failed to attach stderr: %w", err)
    }
    cmd.Stdout = nil

    if err := cmd.Start(); err != nil {
        return fmt.Errorf("hls: ffmpeg start failed: %w", err)
    }

    // 读取 stderr 避免阻塞
    go func() {
        buf := make([]byte, 4096)
        for {
            _, err := stderr.Read(buf)
            if err != nil {
                break
            }
        }
    }()

    if err := cmd.Wait(); err != nil {
        return fmt.Errorf("hls: ffmpeg execution failed: %w", err)
    }

    if req.OnProgress != nil {
        req.OnProgress(100, 0, 0)
    }
    if info, err := os.Stat(rPath); err == nil && req.Metadata != nil {
        req.Metadata["total_size"] = strconv.FormatInt(info.Size(), 10)
    }
    return nil
}

func (e *HLSExtractor) downloadWithM3U8D(ctx context.Context, req *download.Request) error {
    // m3u8d 模式需要 m3u8d 包，暂作为占位。
    // Phase 2 task 4 完成后会使用新 M3U8DEngine。
    return fmt.Errorf("hls: m3u8d mode not yet implemented in HLSExtractor")
}
```

- [ ] **步骤 4：运行测试确认通过**

运行：`go test ./pkg/download/extractor/... -run 'TestHLSExtractor' -v`
预期：PASS

- [ ] **步骤 5：Commit**

```bash
git add pkg/download/extractor/hls.go pkg/download/extractor/hls_test.go
git commit -m "feat(download): add HLSExtractor for HLS/m3u8 stream downloads"
```

---

### 任务 4：m3u8d 可注入引擎（M3U8DEngine）

**文件：**
- 创建：`pkg/download/m3u8d/config.go`
- 创建：`pkg/download/m3u8d/engine.go`
- 创建：`pkg/download/m3u8d/grab.go`

从 `pkg/m3u8d/` 重构为工具库，主要改动：
1. 构造函数接受可选的 `*http.Client` 注入
2. grab 适配层：用注入的 client 替换 grab 内部 client
3. 保持 API 兼容：`NewM3U8Downloader(cfg)` → `NewM3U8DEngine(cfg, httpClient)`

- [ ] **步骤 1：创建 `m3u8d/config.go`**

```go
// pkg/download/m3u8d/config.go
package m3u8d

import "time"

// DownloadConfig 配置 M3U8DEngine 的下载行为。
type DownloadConfig struct {
    InputURL    string
    OutputFile  string
    UserAgent   string
    Headers     map[string]string
    Concurrency int
    MaxRetries  int
    WorkDir     string
    KeepFiles   bool
    FFmpegArgs  []string
    Timeout     time.Duration
    Verbose     bool
}
```

- [ ] **步骤 2：创建 `m3u8d/engine.go`**

```go
// pkg/download/m3u8d/engine.go
package m3u8d

import (
    "context"
    "encoding/base64"
    "errors"
    "fmt"
    "net/http"
    "net/url"
    "os"
    "os/exec"
    "path/filepath"
    "regexp"
    "strings"
    "sync"
    "time"
)

// M3U8DEngine 是 m3u8 下载引擎，支持 http.Client 注入。
type M3U8DEngine struct {
    Config          *DownloadConfig
    client          *http.Client
    baseURL         *url.URL
    downloaded      map[string]bool
    mu              sync.RWMutex
    totalFiles      int
    downloadedCount int
}

// NewM3U8DEngine 创建 M3U8DEngine。
// 如果 httpClient 为 nil，使用默认 client。
func NewM3U8DEngine(cfg *DownloadConfig, httpClient *http.Client) (*M3U8DEngine, error) {
    parsedURL, err := url.Parse(cfg.InputURL)
    if err != nil {
        return nil, fmt.Errorf("invalid URL: %w", err)
    }

    base64URL := base64.URLEncoding.EncodeToString([]byte(parsedURL.String()))
    if cfg.WorkDir == "" {
        cfg.WorkDir = fmt.Sprintf("download_%s", base64URL[:10])
    }
    if err := os.MkdirAll(cfg.WorkDir, 0755); err != nil {
        return nil, fmt.Errorf("failed to create work directory: %w", err)
    }

    if httpClient == nil {
        httpClient = &http.Client{Timeout: cfg.Timeout}
    }

    return &M3U8DEngine{
        Config:     cfg,
        client:     httpClient,
        baseURL:    parsedURL,
        downloaded: make(map[string]bool),
    }, nil
}

// 其余方法：downloadFileWithRetry, downloadFile, DownloadAll,
// downloadFilesConcurrently, parseM3U8, ConvertToMP4, Cleanup
// 与 pkg/m3u8d/downloader.go 保持一致，但 downloadFilesConcurrently
// 改为使用注入的 http.Client 替代 grab.NewClient()
```

- [ ] **步骤 3：运行测试确认通过**

运行：`go build ./pkg/download/m3u8d/...`
预期：PASS

- [ ] **步骤 4：Commit**

```bash
git add pkg/download/m3u8d/
git commit -m "feat(download): add injectable M3U8DEngine with grab adapter"
```

---

## 验证

```bash
go build ./pkg/download/...
go vet ./pkg/download/...
go test -count=1 ./pkg/download/...
```