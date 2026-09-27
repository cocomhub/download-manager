# 独立 UI cmd 设计（前后端分离）

> 日期：2026-09-27
> 状态：设计稿（待用户审查）

## 背景 / 目标

- 现状：`main.go` 单二进制 = 下载引擎（Manager）+ HTTP API + 内嵌 `web/static` UI（`api/server.go` Router 的 `PathPrefix("/")` 静态路由）。
- 痛点：修改 `web/static/**`（UI 代码）必须 `go build` 重建**同一个二进制**并重启 → **中断正在进行的下载**。
- 目标：新增独立 `cmd/ui/` 二进制，**只负责 UI 展示**（内嵌 `web/static` + 反向代理 API/文件到下载进程）。
  - 更新 UI = 只重建 ui 二进制（秒级），**下载进程零重启**。
  - 下载进程保留原 UI（8080 也能访问），9000 是可选新入口（用户已确认）。

## 架构

```
浏览器 ──► :9000 (cmd/ui 独立二进制)
            ├─ /            → 本进程内嵌 web/static（http.FileServer）
            ├─ /app/*       → 本进程（同上）
            ├─ /libs/*      → 本进程（同上）
            ├─ /ui.json     → 本进程（同上）
            ├─ /api/*       → 反向代理 → :8080（下载进程）
            ├─ /files/*     → 反向代理 → :8080（下载进程本地文件服务）
            ├─ /api/ui/*    → 反向代理 → :8080（TaskUI 插件资产，如 sdserver njavtv/tktube viewer）
            └─ /api/events  → 反向代理（SSE 流式透传）
                                 │ http
                                 ▼
                     :8080 (下载进程, 原 full 模式)
                      ├─ 原 UI（保留，可访问）
                      ├─ API / SSE / /files/
                      └─ 下载 + 调度引擎
```

## 组件与文件

### 1. `cmd/ui/main.go`（新增，独立二进制）

- import：`web`（L0 叶子，纯 embed）、`net/http`、`net/http/httputil`、`net/url`、`flag`、`log/slog`。
- **不 import** `manager` / `api` / `task` 任何包 → 不触发下载引擎、不注册任务 → archcheck 分层安全（cmd/ 豁免登记，但 import 图里只依赖 L0）。
- 参数：
  - `--port`（默认 9000）：UI 监听端口
  - `--api-url`（默认 `http://127.0.0.1:8080`）：下载进程 API 地址
  - `--version`：打印版本后退出
- 版本注入：`-ldflags "-X main.Version=... -X main.BuildAt=..."`（与主二进制同模式）。

### 2. 路由

```go
mux := http.NewServeMux()
// 静态资源（本进程 embed）
subFS, _ := fs.Sub(web.StaticFS, "static")
mux.Handle("/", http.FileServer(http.FS(subFS)))          // 兜底：/、/app/*、/libs/*、/ui.json
// 反代到下载进程
mux.Handle("/api/", proxy)    // 含 /api/events、/api/ui/*
mux.Handle("/files/", proxy)
```

反代用 `httputil.ReverseProxy` + **Rewrite 钩子**（Go 1.27 支持）：

```go
proxy := &httputil.ReverseProxy{
    Rewrite: func(pr *httputil.ProxyRequest) {
        pr.SetURL(apiURL)        // 重写 scheme/host/path
        pr.SetXForwarded()       // 设置 X-Forwarded-For/Host/Proto
        // Host 头：SetURL 后 Out.Host="" → 转发用后端 host（127.0.0.1:8080）
        // Authorization 头默认透传（前端 authFetch 附加）
        // Origin 头默认透传（浏览器发的 http://localhost:9000）
    },
    FlushInterval: 100 * time.Millisecond,  // SSE 流式透传
}
```

### 3. `api/server_task.go` 改动（最小，兼容反代）

`handleEvents` 的 SSE 同源检查目前是 `oh.Host != r.Host` 直接 403。反代时后端收到的 `r.Host = 127.0.0.1:8080`，而浏览器 `Origin = http://localhost:9000` → 被拒。

放宽为同时信任标准反代头 `X-Forwarded-Host`：

```go
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

> 兼容性：无反代时 `X-Forwarded-Host` 为空 → `oh.Host != r.Host` 原逻辑不变；有反代（本 cmd 或 nginx）时通过标准头放行。已存在的 `handleEvents` 测试需补一个 X-Forwarded-Host 用例。

### 4. Makefile

```makefile
.PHONY: build-ui
build-ui: fmt
	$(GO) build $(GOBUILD_EXTRA) $(GO_BUILD_FLAGS) -ldflags "$(GO_LDFLAGS)" -o $(BIN_DIR)/download-manager-ui$(EXE) ./cmd/ui
```

产物 `build/bin/download-manager-ui(.exe)`。**goreleaser 暂不加**（用户只需本地构建；后续需要再扩 builds）。

## 数据流

1. 用户访问 `http://localhost:9000` → `cmd/ui` 返回内嵌 `index.html` + JS/CSS。
2. 前端所有请求走**同源相对路径**（`/api/*`、`/files/*`、`/api/events`，见 `api.js`/`helpers.js`/`main.js` 实证）→ 落到 `cmd/ui`。
3. `cmd/ui` 把 `/api/*`、`/files/*` 反代到 `:8080` 下载进程：
   - GET/POST/PUT/PATCH 原样透传（含 `Authorization`、`Origin`、body）；
   - `/api/events` SSE：`FlushInterval` 逐块转发，浏览器 EventSource 正常接收实时事件。
4. 下载进程执行真实读写（含鉴权、写保护、/files/ 文件服务）。

## 错误处理

- **下载进程不可达**（未启动/端口错）：反代返回 502。前端 `authFetch` 会看到网络错误 → 保持现有错误提示（不改前端）。
- **`--api-url` 格式非法**：启动时报错退出（fail-fast）。
- **端口占用**：`ListenAndServe` 报错退出（与主二进制一致）。

## 测试

1. **单元**：`cmd/ui/proxy_test.go`
   - 起一个本地 `httptest.Server` 模拟下载进程（记录收到的 Host/X-Forwarded-Host/Authorization/Origin）；
   - 断言反代正确设置 `X-Forwarded-Host`、`Authorization` 透传、`/api/` 与 `/files/` 前缀转发、静态 `/` 由本进程服务。
2. **单元**：`api/server_task_test.go` 补 `handleEvents` 用例：
   - `X-Forwarded-Host` 与 Origin.Host 匹配 → 200（SSE 头发出）；
   - 不匹配 → 403（原逻辑）。
3. **E2E（可选）**：Playwright 起 `cmd/ui`（`--api-url` 指向 playwright-server）→ 页面可加载、播放本地视频。

## 验收标准

- [ ] `make build-ui` 产出 `build/bin/download-manager-ui(.exe)`，启动后 `:9000` 可访问完整 UI。
- [ ] 下载进程（8080）仍可访问原 UI（零回归）。
- [ ] 修改 `web/static/**` 后只重建 ui 二进制，下载进程不重启即可看到新 UI。
- [ ] SSE 实时事件（任务/对象更新）经反代正常推送。
- [ ] 鉴权（若启用）经反代正常（Authorization 透传 + 401 流程）。
- [ ] `/files/*` 视频/图片经反代可播放。
- [ ] CI 门禁全绿（archcheck 分层、notest、web-test、e2e 不回归）。
