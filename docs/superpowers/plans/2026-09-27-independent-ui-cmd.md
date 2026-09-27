# 独立 UI cmd 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 subagent-driven-development（推荐）或 executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 新增独立 `cmd/ui/` 二进制：内嵌 `web/static` 静态 UI + 反向代理 `/api/*`、`/files/*`、`/api/events` 到下载进程，实现「更新 UI 只重建 ui 二进制、下载进程零重启」。

**架构：** `cmd/ui` 只 import `web`（L0 叶子 embed）+ 标准库（`net/http`、`net/http/httputil`、`flag`、`log/slog`）。静态资源本进程提供；API/文件/SSE 经 `httputil.ReverseProxy` + `Rewrite` 钩子（`SetURL` + `SetXForwarded`）转发到 `--api-url` 指定的下载进程。唯一对下载进程的改动：`api/server_task.go` 的 `handleEvents` SSE 同源检查放宽到信任 `X-Forwarded-Host`。

**技术栈：** Go 1.27（`httputil.ReverseProxy.Rewrite`）、标准库 `http.ServeMux`、`http.FileServer`、`embed`。

**规格：** `docs/superpowers/specs/2026-09-27-independent-ui-cmd-design.md`

## 全局约束

- 前端代码（`web/static/**`）**零改动**——所有请求保持同源相对路径（`/api/*`、`/files/*`、`/api/events`）。
- `cmd/ui` 不得 import `manager`、`api`、`task`、`downloader`、`storage`（不触发下载引擎、不注册任务）；只允许 `web` + 标准库 + `github.com/cocomhub/download-manager/web`。
- 下载进程（8080）保留原 UI 与 API，行为零回归。
- SSE 反代必须流式透传（`FlushInterval` 非零），浏览器 EventSource 实时接收。
- 鉴权头 `Authorization` 必须原样透传（前端 `authFetch` 附加，下载进程 `authMiddleware` 校验）。
- 所有新建 Go 文件带 SPDX 头（`Copyright 2026 The Cocomhub Authors. All rights reserved.` / `SPDX-License-Identifier: Apache-2.0`）。
- 测试只用 `127.0.0.1` 回环；`t.Context()`；表驱动测试带 `name` 字段。

---

### 任务 1：`handleEvents` SSE 同源检查兼容反代

**文件：**
- 修改：`api/server_task.go:499-512`（`handleEvents` 的 Origin 检查）
- 测试：`api/sse_test.go`（补 2 个用例）

- [ ] **步骤 1：编写失败的测试**

在 `api/sse_test.go` 追加两个用例：

```go
// TestSSE_ForwardedHostMatchesOrigin 反代场景：Origin.Host 与 X-Forwarded-Host 匹配 → 放行。
func TestSSE_ForwardedHostMatchesOrigin(t *testing.T) {
	ts := sseTestSetup(t, "none", "")

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, ts.URL+"/api/events", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Origin", "http://localhost:9000")
	req.Header.Set("X-Forwarded-Host", "localhost:9000") // 反代设置的标准头
	// 让后端 r.Host 与 Origin 不同：直接改请求 Host 头模拟反代后的 r.Host
	req.Host = ts.URL // 后端的 r.Host 将是 ts.URL 的 host:port

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 (X-Forwarded-Host 与 Origin 匹配)", resp.StatusCode)
	}
}

// TestSSE_ForwardedHostMismatchRejected 反代场景：X-Forwarded-Host 与 Origin 不匹配 → 403。
func TestSSE_ForwardedHostMismatchRejected(t *testing.T) {
	ts := sseTestSetup(t, "none", "")

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, ts.URL+"/api/events", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Origin", "http://localhost:9000")
	req.Header.Set("X-Forwarded-Host", "evil.example:9000")
	req.Host = ts.URL

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (X-Forwarded-Host 与 Origin 不匹配)", resp.StatusCode)
	}
}
```

> 注意：`sseTestSetup` 用 `httptest.NewServer`，`ts.URL` 形如 `http://127.0.0.1:PORT`。设置 `req.Host = ts.URL`（含 `http://` 前缀）会让后端 `r.Host` 变成 `http://127.0.0.1:PORT`——这恰好与 `Origin`（`http://localhost:9000`）不同，能触发旧逻辑 403；新逻辑用 `X-Forwarded-Host` 放行/拒绝。

- [ ] **步骤 2：运行测试确认失败**

运行：`go test -run 'TestSSE_ForwardedHost' ./api/`
预期：两个用例都 FAIL（旧逻辑 `oh.Host != r.Host` 直接 403，ForwardedHostMatchesOrigin 期望 200 实际 403）。

- [ ] **步骤 3：修改 `handleEvents`**

`api/server_task.go` 的 `handleEvents` Origin 检查段：

```go
	// Same-origin check: reject cross-origin requests before any SSE headers
	// are written, unless no Origin header is present (curl/CLI clients).
	// 兼容反向代理：允许 Origin.Host 与 r.Host 或标准反代头 X-Forwarded-Host 匹配
	// （独立 UI cmd / nginx 均设置该头；无反代时为空，逻辑与原来一致）。
	origin := r.Header.Get("Origin")
	if origin != "" {
		oh, err := url.Parse(origin)
		fwdHost := r.Header.Get("X-Forwarded-Host")
		if err != nil || (oh.Host != r.Host && oh.Host != fwdHost) {
			writeJSONError(w, http.StatusForbidden, "forbidden", "cross-origin denied")
			return
		}
	}
```

- [ ] **步骤 4：运行测试确认通过**

运行：`go test -run 'TestSSE' ./api/`
预期：全部 PASS（含原有 SameOriginOK / CrossOriginRejected / NoOriginPasses 等）。

- [ ] **步骤 5：Commit**

```bash
git add api/server_task.go api/sse_test.go
git commit -m "fix(api): SSE 同源检查兼容反向代理(X-Forwarded-Host)"
```

---

### 任务 2：`cmd/ui` 主程序（静态服务 + 反代）

**文件：**
- 创建：`cmd/ui/main.go`
- 创建：`cmd/ui/main_test.go`

- [ ] **步骤 1：编写失败的测试**

`cmd/ui/main_test.go`：

```go
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestUI 构造 UI handler（复用 buildHandler），backend 为模拟下载进程。
func newTestUI(t *testing.T, backend *httptest.Server) http.Handler {
	t.Helper()
	h, err := buildHandler(backend.URL)
	if err != nil {
		t.Fatalf("buildHandler: %v", err)
	}
	return h
}

func TestUI_ServesStaticIndex(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("backend should not be called for static /, got %s", r.URL.Path)
	}))
	defer backend.Close()

	h := newTestUI(t, backend)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "<!DOCTYPE html") {
		t.Errorf("body should contain HTML, got: %s", rr.Body.String()[:100])
	}
}

func TestUI_ProxiesAPIAndSetsForwardedHost(t *testing.T) {
	var gotHost, gotFwdHost, gotAuth, gotOrigin string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		gotFwdHost = r.Header.Get("X-Forwarded-Host")
		gotAuth = r.Header.Get("Authorization")
		gotOrigin = r.Header.Get("Origin")
		w.Header().Set("X-Backend", "yes")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer backend.Close()

	h := newTestUI(t, backend)
	req := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	req.Host = "localhost:9000"
	req.Header.Set("Authorization", "Bearer abc")
	req.Header.Set("Origin", "http://localhost:9000")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if rr.Header().Get("X-Backend") != "yes" {
		t.Errorf("backend response header not propagated")
	}
	if gotAuth != "Bearer abc" {
		t.Errorf("Authorization = %q, want Bearer abc (透传)", gotAuth)
	}
	if gotOrigin != "http://localhost:9000" {
		t.Errorf("Origin = %q, want http://localhost:9000 (透传)", gotOrigin)
	}
	if gotFwdHost != "localhost:9000" {
		t.Errorf("X-Forwarded-Host = %q, want localhost:9000", gotFwdHost)
	}
	if gotHost == "localhost:9000" {
		t.Errorf("Host should be rewritten to backend, got %q", gotHost)
	}
}

func TestUI_ProxiesFilesPrefix(t *testing.T) {
	var gotPath string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "video/mp4")
		w.Write([]byte("fake-video"))
	}))
	defer backend.Close()

	h := newTestUI(t, backend)
	req := httptest.NewRequest(http.MethodGet, "/files/njavtv/abc.mp4", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if gotPath != "/files/njavtv/abc.mp4" {
		t.Errorf("backend path = %q, want /files/njavtv/abc.mp4", gotPath)
	}
}

func TestUI_ProxiesSSE(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// 立即返回（SSE 长连接在测试中只验证头 + 首块）
		io.WriteString(w, "data: {\"type\":\"task_update\"}\n\n")
	}))
	defer backend.Close()

	h := newTestUI(t, backend)
	req := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	req.Header.Set("Origin", "http://localhost:9000")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
}
```

- [ ] **步骤 2：运行测试确认失败**

运行：`go test ./cmd/ui/`
预期：编译错误 `undefined: buildHandler`（函数未定义）。

- [ ] **步骤 3：编写 `cmd/ui/main.go`**

```go
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package main 独立 UI 进程：内嵌 web/static 静态资源 + 反向代理 API/文件到下载进程。
// 更新 UI 只需重建本二进制，下载进程零重启。
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"

	"github.com/cocomhub/download-manager/web"
)

var (
	Version = "dev"
	BuildAt = "unknown"
)

// buildHandler 构造 UI 路由：/api/*、/files/* 反代到 backendURL，其余静态资源本进程提供。
func buildHandler(backendURL string) (http.Handler, error) {
	apiURL, err := url.Parse(backendURL)
	if err != nil {
		return nil, fmt.Errorf("invalid --api-url %q: %w", backendURL, err)
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(apiURL)
			pr.SetXForwarded() // 设置 X-Forwarded-For/Host/Proto（后端 SSE 同源检查依赖）
			// Host 头由 SetURL 置空 → 转发用后端 host
			// Authorization / Origin / body 默认透传
		},
		FlushInterval: 100 * time.Millisecond, // SSE 流式透传
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", proxy)
	mux.Handle("/files/", proxy)

	subFS, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		return nil, fmt.Errorf("failed to embed static files: %w", err)
	}
	mux.Handle("/", http.FileServer(http.FS(subFS)))

	return mux, nil
}

func main() {
	var (
		port    = flag.Int("port", 9000, "UI listen port")
		apiURL  = flag.String("api-url", "http://127.0.0.1:8080", "downloader backend API URL")
		showVer = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVer {
		fmt.Printf("Version: %s, Build At: %s\n", Version, BuildAt)
		os.Exit(0)
	}

	handler, err := buildHandler(*apiURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: handler,
	}
	slog.Info("UI server started", "port", *port, "api_url", *apiURL, "version", Version, "build_at", BuildAt)
	slog.Info("Web UI available", "url", fmt.Sprintf("http://localhost:%d", *port))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("UI server failed", "error", err)
		os.Exit(1)
	}
}
```

- [ ] **步骤 4：运行测试确认通过**

运行：`go test ./cmd/ui/`
预期：全部 PASS。

- [ ] **步骤 5：Commit**

```bash
git add cmd/ui/main.go cmd/ui/main_test.go
git commit -m "feat(ui): 独立 UI 进程 cmd/ui——内嵌静态 + 反代 API/files/SSE 到下载进程"
```

---

### 任务 3：Makefile `build-ui` target

**文件：**
- 修改：`Makefile`（build 目标旁新增）

- [ ] **步骤 1：修改 Makefile**

在 `build-ci` 目标后新增：

```makefile
.PHONY: build-ui
build-ui: fmt
	$(GO) build $(GOBUILD_EXTRA) $(GO_BUILD_FLAGS) -ldflags "$(GO_LDFLAGS)" -o $(BIN_DIR)/download-manager-ui$(EXE) ./cmd/ui
```

- [ ] **步骤 2：验证构建**

运行：`make build-ui`
预期：`build/bin/download-manager-ui(.exe)` 产出成功。

- [ ] **步骤 3：冒烟验证（可选项，手动）**

```bash
# 终端 1：起下载进程（full 模式）
go run . --config config.yaml --run-mode full   # 或用户实际 sdserver
# 终端 2：起独立 UI
./build/bin/download-manager-ui --port 9000 --api-url http://127.0.0.1:8080
# 浏览器打开 http://localhost:9000 → 完整 UI；播放视频；看实时任务更新
```

- [ ] **步骤 4：Commit**

```bash
git add Makefile
git commit -m "build(ui): Makefile 新增 build-ui target 构建独立 UI 二进制"
```

---

### 任务 4：全量验证与门禁

- [ ] **步骤 1：单元测试全量**

运行：`go test ./api/ ./cmd/ui/`
预期：全部 PASS。

- [ ] **步骤 2：CI 门禁**

运行：`make vet && make lint && make archcheck && make web-test && make notest`
预期：全部通过（`cmd/ui` 只依赖 `web` L0 + 标准库 → 分层安全；`cmd/` 豁免 Levels 登记）。

- [ ] **步骤 3：E2E 冒烟（可选，Playwright）**

若 `test/playwright` 可用：起 playwright-server + `cmd/ui`（`--api-url` 指向 playwright-server 端口）→ 页面加载、任务列表渲染、SSE 实时更新。

- [ ] **步骤 4：Commit（如有收尾改动）**

```bash
git add -A
git commit -m "chore(ui): 独立 UI cmd 收尾验证"
```
