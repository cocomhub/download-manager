# CI/CD 升级与 P0 治理 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 基于已有的 Docker/优雅停机/写保护/GoReleaser/CI 矩阵等基础设施，补充 Dockerfile HEALTHCHECK/非 root 用户、默认端口 8080、.codecov.yml 门禁与 Makefile 对齐、API 鉴权（none/basic/token）、下载路径一致性（FileRoot）

**架构：** 6 个独立可验证任务，按无依赖顺序组织。Sprint 1：Dockerfile 增强 + 端口默认值 + codecov 门禁对齐。Sprint 2：API 鉴权 + 路径一致性 + 差异检测

**技术栈：** Go 1.26、gorilla/mux、docker buildx、Codecov、环境变量覆盖

---

### 任务 1：Dockerfile 增强（HEALTHCHECK + 非 root + 私有模块认证）

**文件：**
- 修改：`Dockerfile`
- 新增：`Makefile` 中 docker-build / docker-build-secure 目标

- [ ] **步骤 1：为 Dockerfile runtime stage 添加 HEALTHCHECK 和非 root 用户**

```dockerfile
# 在 FROM alpine:3.21 之后，在 EXPOSE 之前插入
RUN adduser -D appuser && chown -R appuser /usr/local/bin/download-manager
USER appuser

# 将 EXPOSE 8080 之后替换为
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1:${DM_HTTP_PORT:-8080}/api/healthz || exit 1
```

修改后的完整 runtime stage：

```dockerfile
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata ffmpeg wget
COPY --from=builder /build/download-manager /usr/local/bin/download-manager
RUN adduser -D appuser && chown -R appuser /usr/local/bin/download-manager
USER appuser
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1:${DM_HTTP_PORT:-8080}/api/healthz || exit 1
ENTRYPOINT ["download-manager"]
CMD ["--config", "/etc/download-manager/config.yaml"]
```

- [ ] **步骤 2：在 Makefile 中添加 docker 构建目标**

```makefile
# --- Docker targets ---
.PHONY: docker-build docker-build-secure

docker-build:
	docker build -t download-manager:latest .

docker-build-secure:
	docker buildx build \
		--secret id=git_credentials,src=$(HOME)/.ssh/id_ed25519 \
		-t download-manager:latest .
```

（位置：放在现有 `notest` 目标之后，`run` 目标之前，注释用 `# ---` 分隔）

- [ ] **步骤 3：构建并验证 Docker 镜像**

运行：
```bash
cd D:/workdir/leon/cocomhub/download-manager && docker build -t dm-test .
docker run --rm dm-test --version
docker run --rm dm-test whoami | grep appuser
```

预期：三行输出——版本号、`appuser`、健康检查通过。

- [ ] **步骤 4：Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager
git add Dockerfile Makefile
git commit -m "feat: enhance Dockerfile with HEALTHCHECK, non-root user, and make targets"
```

---

### 任务 2：Config 默认端口 8080 + FilesDir + AuthConfig 结构体

**文件：**
- 修改：`config/config.go` — ValidateAndClamp 中加 HTTPPort 默认值；新增 `FilesDir` 字段、`AuthConfig` 结构体、`FileRoot()` 方法
- 修改：`config/config_diff.go` — 确保 FilesDir 和 Auth 变更在 Diff() 中被检测

- [ ] **步骤 1：在 Server 结构体中添加 FilesDir 和 Auth 字段**

```go
// config/config.go Server 结构体（约第 44-53 行）
type Server struct {
    HTTPPort        int        `yaml:"http_port" json:"http_port"`
    UIOnlyPort      int        `yaml:"ui_only_port" json:"ui_only_port"`
    WorkDir         string     `yaml:"work_dir" json:"work_dir"`
    LockFile        string     `yaml:"lock_file" json:"lock_file"`
    UIOnlyLockFile  string     `yaml:"ui_only_lock_file" json:"ui_only_lock_file"`
    ScraperPath     string     `yaml:"scraper_path" json:"scraper_path"`
    DownloadRootDir string     `yaml:"download_root_dir" json:"download_root_dir"`
    // 新增 2 个字段
    FilesDir        string     `yaml:"files_dir" json:"files_dir"`
    Auth            AuthConfig `yaml:"auth" json:"auth"`
    UIDefaults      UIDefaults `yaml:"ui_defaults" json:"ui_defaults"`
}
```

- [ ] **步骤 2：在 Config 文件底部添加 AuthConfig 结构体和 FileRoot 方法**

```go
// config/config.go 末尾，在 Config.Clone() 之前
// AuthConfig 定义 HTTP 鉴权方式。
type AuthConfig struct {
	Type string `yaml:"type" json:"type"` // "none" | "basic" | "token"
}

// FileRoot 返回 HTTP /files/ 服务使用的根目录。
// 优先使用 FilesDir（如果设置），否则回退到 Downloader.OutputDir。
func (c *Config) FileRoot() string {
	if c.Server.FilesDir != "" {
		return c.Server.FilesDir
	}
	return c.Downloader.OutputDir
}
```

- [ ] **步骤 3：在 ValidateAndClamp 中 HTTPPort 和 UIOnlyPort 默认值**

```go
// config/config.go ValidateAndClamp() 中，在现有的 "Server defaults" 段之后（约第 309 行）
// 现有代码：
// if c.Server.DownloadRootDir == "" {
//     c.Server.DownloadRootDir = filepath.Join(c.Server.WorkDir, "downloads")
// }

// 在其后添加：
if c.Server.HTTPPort <= 0 {
	c.Server.HTTPPort = 8080
}
if c.Server.UIOnlyPort <= 0 {
	c.Server.UIOnlyPort = 8091
}
```

- [ ] **步骤 4：编写配置默认值测试**

```go
// config/config_test.go 中新增测试函数

func TestConfig_DefaultHTTPPort(t *testing.T) {
	t.Parallel()
	cfg := &Config{}
	cfg.ValidateAndClamp()
	if cfg.Server.HTTPPort != 8080 {
		t.Errorf("HTTPPort = %d, want 8080", cfg.Server.HTTPPort)
	}
	if cfg.Server.UIOnlyPort != 8091 {
		t.Errorf("UIOnlyPort = %d, want 8091", cfg.Server.UIOnlyPort)
	}
}

func TestConfig_FileRoot(t *testing.T) {
	t.Parallel()

	t.Run("default to output dir", func(t *testing.T) {
		cfg := &Config{}
		cfg.Downloader.OutputDir = "/data/downloads"
		if got := cfg.FileRoot(); got != "/data/downloads" {
			t.Errorf("FileRoot() = %q, want %q", got, "/data/downloads")
		}
	})

	t.Run("use files_dir when set", func(t *testing.T) {
		cfg := &Config{}
		cfg.Server.FilesDir = "/data/files"
		cfg.Downloader.OutputDir = "/data/downloads"
		if got := cfg.FileRoot(); got != "/data/files" {
			t.Errorf("FileRoot() = %q, want %q", got, "/data/files")
		}
	})
}
```

- [ ] **步骤 5：运行配置测试**

运行：
```bash
cd D:/workdir/leon/cocomhub/download-manager && go test -v -run 'TestConfig_DefaultHTTPPort|TestConfig_FileRoot' ./config/
```

预期：两个测试 PASS。

- [ ] **步骤 6：更新 config_diff.go 确保 FilesDir 和 Auth 变更被检测**

```go
// config/config_diff.go — 检查是否有 Diff() 方法。如果 Diff() 使用
// reflect.DeepEqual 或逐个字段比较，检查新字段是否被覆盖。
// 如果 Diff() 是手动逐个字段比较，添加新增字段的比较逻辑。
```

```go
// 如果 Diff() 使用自动比较（如 reflect.DeepEqual），则无需改动。
// 如果是手动比较，在 Server 比较段添加：
// sa.Server.FilesDir != sb.Server.FilesDir {
//     changes = append(changes, ConfigChange{Field: "server.files_dir", ...})
// }
// sa.Server.Auth.Type != sb.Server.Auth.Type {
//     changes = append(changes, ConfigChange{Field: "server.auth.type", ...})
// }
```

读取 `config_diff.go` 确认比较方式，补充必要字段。

- [ ] **步骤 7：Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager
git add config/config.go config/config_test.go config/config_diff.go
git commit -m "feat: add default ports 8080/8091, FilesDir/FileRoot, AuthConfig struct"
```

---

### 任务 3：codecov.yml + Makefile 门禁对齐（40%）

**文件：**
- 修改：`.codecov.yml` — project target 60% → 40%，patch 70% → 60%
- 修改：`Makefile` — COVER_THRESHOLD 20 → 40

- [ ] **步骤 1：修改 .codecov.yml**

```yaml
# codecov.yml 第 13-17 行
coverage:
  status:
    project:
      default:
        target: 40%       # 与 make cover-check 的 COVER_THRESHOLD 一致
        threshold: 2%
    patch:
      default:
        target: 60%       # 新代码期望 60%
```

- [ ] **步骤 2：修改 Makefile 覆盖率门禁**

```makefile
# Makefile 第 30 行
COVER_THRESHOLD ?= 40
```

- [ ] **步骤 3：运行 cover-check 验证新阈值**

```bash
cd D:/workdir/leon/cocomhub/download-manager && make cover-check
```

预期：覆盖率报告输出，确认使用 40% 阈值。

- [ ] **步骤 4：Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager
git add .codecov.yml Makefile
git commit -m "fix: align codecov and make cover-check thresholds to 40%"
```

---

### 任务 4：API 鉴权中间件（none/basic/token）

**文件：**
- 创建：`api/auth.go` — 鉴权中间件
- 创建：`api/auth_test.go` — 表驱动测试
- 修改：`api/server.go` — 注册鉴权中间件

- [ ] **步骤 1：创建 api/auth.go**

```go
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"crypto/subtle"
	"net/http"

	"github.com/cocomhub/download-manager/config"
)

// authMiddleware returns an HTTP middleware that enforces the configured
// authentication scheme. It runs before writeMiddleware so that auth failures
// return 401 before write-protection checks.
func (s *Server) authMiddleware() mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var cfg config.AuthConfig
			if cp := s.mgr.GetConfig(); cp != nil {
				cfg = cp.Server.Auth
			}

			switch cfg.Type {
			case "basic":
				user, pass, ok := r.BasicAuth()
				if !ok || !s.validateBasicAuth(user, pass) {
					w.Header().Set("WWW-Authenticate", `Basic realm="download-manager"`)
					writeJSONError(w, http.StatusUnauthorized, "unauthorized", "invalid credentials")
					return
				}
			case "token":
				token := r.Header.Get("Authorization")
				if token == "" || !s.validateTokenAuth(token) {
					writeJSONError(w, http.StatusUnauthorized, "unauthorized", "invalid token")
					return
				}
			}
			// case "none" — pass through
			next.ServeHTTP(w, r)
		})
	}
}

// validateBasicAuth checks username/password against config with env override.
func (s *Server) validateBasicAuth(user, pass string) bool {
	cfg := s.mgr.GetConfig()
	if cfg == nil {
		return false
	}
	expectedUser := cfg.Server.Auth.Username
	if expectedUser == "" {
		expectedUser = "admin"
	}
	expectedPass := cfg.Server.Auth.Password
	// 环境变量 DM_AUTH_PASSWORD 优先
	if envPass := os.Getenv("DM_AUTH_PASSWORD"); envPass != "" {
		expectedPass = envPass
	}
	return subtle.ConstantTimeCompare([]byte(user), []byte(expectedUser)) == 1 &&
		subtle.ConstantTimeCompare([]byte(pass), []byte(expectedPass)) == 1
}

// validateTokenAuth checks bearer token against config with env override.
func (s *Server) validateTokenAuth(token string) bool {
	cfg := s.mgr.GetConfig()
	if cfg == nil {
		return false
	}
	expected := cfg.Server.Auth.Token
	if envToken := os.Getenv("DM_AUTH_TOKEN"); envToken != "" {
		expected = envToken
	}
	if expected == "" {
		return true // token 为空等价于无鉴权
	}
	// 支持 "Bearer xxx" 格式
	if len(token) > 7 && token[:7] == "Bearer " {
		token = token[7:]
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}
```

需要补充 import：

```go
import (
	"crypto/subtle"
	"net/http"
	"os"

	"github.com/cocomhub/download-manager/config"
	"github.com/gorilla/mux"
)
```

- [ ] **步骤 2：在 api/server.go 中注册 authMiddleware**

```go
// api/server.go Router() 方法中，在 r.Use(s.writeMiddleware) 之前添加
func (s *Server) Router() *mux.Router {
	r := mux.NewRouter()

	// Auth middleware first, then write guard
	r.Use(s.authMiddleware())
	r.Use(s.writeMiddleware)

	// ... 现有路由保持不变 ...
}
```

- [ ] **步骤 3：创建 api/auth_test.go**

```go
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"net/http"
	"testing"

	"github.com/cocomhub/download-manager/config"
)

func TestAuthMiddleware(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		authType   string
		password   string
		token      string
		setupReq   func(r *http.Request)
		wantStatus int
	}{
		{
			name:       "none passes through",
			authType:   "none",
			setupReq:   func(r *http.Request) {},
			wantStatus: http.StatusNotFound, // 404 because route doesn't exist
		},
		{
			name:       "basic valid credentials",
			authType:   "basic",
			password:   "secret",
			setupReq:   func(r *http.Request) { r.SetBasicAuth("admin", "secret") },
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "basic invalid password",
			authType:   "basic",
			password:   "secret",
			setupReq:   func(r *http.Request) { r.SetBasicAuth("admin", "wrong") },
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "basic missing credentials",
			authType:   "basic",
			password:   "secret",
			setupReq:   func(r *http.Request) {},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "token valid",
			authType:   "token",
			token:      "mytoken",
			setupReq:   func(r *http.Request) { r.Header.Set("Authorization", "Bearer mytoken") },
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "token invalid",
			authType:   "token",
			token:      "mytoken",
			setupReq:   func(r *http.Request) { r.Header.Set("Authorization", "Bearer wrong") },
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "token missing",
			authType:   "token",
			token:      "mytoken",
			setupReq:   func(r *http.Request) {},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "token empty is pass-through",
			authType:   "token",
			token:      "",
			setupReq:   func(r *http.Request) {},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// 使用 memory storage 的最小配置
			cfg := &config.Config{
				Server: config.Server{
					HTTPPort: 0, // 测试用 0 表示随机
					WorkDir:  t.TempDir(),
					Auth: config.AuthConfig{
						Type:     tc.authType,
						Password: tc.password,
						Token:    tc.token,
					},
				},
			}
			mgr := newTestManager(t, cfg)
			srv := NewServer(mgr)
			router := srv.Router()

			req := mustNewRequest(t, http.MethodGet, "/api/nonexistent", nil)
			tc.setupReq(req)
			rr := mustServeHTTP(t, router, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rr.Code, tc.wantStatus)
			}
		})
	}
}
```

需要 helper 函数（如果尚无复用）：

```go
// api/auth_test.go 中（或使用 api/server_test.go 中已有的 helper）
func newTestManager(t *testing.T, cfg *config.Config) *testManager {
	t.Helper()
	// 使用 memory storage 创建最小 manager
	// 具体实现参考 server_test.go 中的 TestManager 构造方式
}

func mustNewRequest(t *testing.T, method, target string, body any) *http.Request {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, target, r)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func mustServeHTTP(t *testing.T, handler http.Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}
```

- [ ] **步骤 4：运行鉴权测试**

运行：
```bash
cd D:/workdir/leon/cocomhub/download-manager && go test -v -run 'TestAuthMiddleware' ./api/
```

预期：8 个子测试全部 PASS。

- [ ] **步骤 5：补充 AuthConfig 的 Username/Token 字段**

```go
// config/config.go 中 AuthConfig 结构体补充字段
type AuthConfig struct {
	Type     string `yaml:"type" json:"json"`        // "none" | "basic" | "token"
	Username string `yaml:"username" json:"username"` // basic auth 用户名，默认 "admin"
	Password string `yaml:"password" json:"password"` // 环境变量 DM_AUTH_PASSWORD 优先
	Token    string `yaml:"token" json:"token"`       // 环境变量 DM_AUTH_TOKEN 优先
}
```

- [ ] **步骤 6：运行全部构建和测试**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go build ./... && go test -race -count=1 -timeout=180s ./api/... ./config/...
```

预期：全部 PASS。

- [ ] **步骤 7：Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager
git add api/auth.go api/auth_test.go api/server.go config/config.go
git commit -m "feat: add API auth middleware (none/basic/token) with env override"
```

---

### 任务 5：下载路径一致性 — /files/ 使用 FileRoot()

**文件：**
- 修改：`api/server.go` — /files/ handler 使用 `s.mgr.FileRoot()`（通过 config 的 FileRoot 方法）

- [ ] **步骤 1：确认 Manager 暴露 FileRoot 方法**

```go
// manager/manager.go — 确认已有 GetDownloadRootDir() 方法
// 或新增 FileRoot() 方法委托给 config
func (m *Manager) GetDownloadRootDir() string {
	cfg := m.currentCfg()
	if cfg == nil {
		return ""
	}
	return cfg.FileRoot() // 改用 FileRoot()
}
```

- [ ] **步骤 2：修改 api/server.go 中 /files/ handler**

```go
// api/server.go 第 115 行
// 修改前：
r.PathPrefix("/files/").Handler(http.StripPrefix("/files/", http.FileServer(http.Dir(s.mgr.GetDownloadRootDir()))))

// 修改后（如 GetDownloadRootDir 已使用 FileRoot 则无需改动）
// 验证 s.mgr.GetDownloadRootDir() 内部是否使用 cfg.FileRoot()
```

- [ ] **步骤 3：验证构建通过**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go build ./...
```

预期：编译成功。

- [ ] **步骤 4：Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager
git add api/server.go manager/manager.go
git commit -m "feat: unify /files/ path resolution via config.FileRoot()"
```

---

### 任务 6：验证全量 CI 流程

**文件：**
- 运行：全量构建、测试、覆盖率、lint、docker

- [ ] **步骤 1：全量测试**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go test -race -count=1 -timeout=180s ./...
```

预期：所有测试 PASS。

- [ ] **步骤 2：覆盖率检查**

```bash
cd D:/workdir/leon/cocomhub/download-manager && make cover-check
```

预期：覆盖率 >= 40%。

- [ ] **步骤 3：Docker 构建验证**

```bash
cd D:/workdir/leon/cocomhub/download-manager && docker build -t dm-verify .
docker run --rm dm-verify --version
docker run --rm dm-verify whoami
```

预期：版本号输出 + `appuser` 用户。

- [ ] **步骤 4：CLAUDE.md 更新**

检查 CLAUDE.md 中是否需同步更新：
- 端口默认值 8080 → 更新文档
- 鉴权配置 → 添加到架构概览
- COVER_THRESHOLD 40 → 更新文档

- [ ] **步骤 5：最终 Commit**

```bash
cd D:/workdir/leon/cocomhub/download-manager
git add CLAUDE.md
git commit -m "docs: sync CLAUDE.md with auth, port, and threshold changes"
```
