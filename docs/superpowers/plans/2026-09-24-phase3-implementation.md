# 阶段 3：安全运维/产品化 — 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 subagent-driven-development 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法跟踪进度。

**目标：** 完成 1 年 roadmap 最后阶段——鉴权强化、SSE/文件边界、部署形态、依赖治理文档、archcheck 门禁。

**架构：** 5 个独立任务（P3-1 至 P3-5），互不冲突的文件边界，可并行执行。P3-1（config.go/auth.go）与 P3-3（compose 环境变量）有配置依赖，P3-3 的文档部分引用 P3-1 已定的环境变量名。

**技术栈：** Go 1.27、gorilla/mux、config YAML、docker-compose、systemd、golangci-lint v2。

**规格：** `docs/superpowers/specs/2026-09-24-phase3-security-ops-design.md`（已批准）

## 全局约束

- 新文件必须带 SPDX 头：`Copyright 2026 The Cocomhub Authors. All rights reserved.` / `SPDX-License-Identifier: Apache-2.0`
- 文件编码 UTF-8 without BOM；gofmt 干净（`gofmt -s`）
- 测试只绑定 `127.0.0.1`；测试用 `t.Context()`；断言用 `testutil/assert.MustEventually`（避免 time.Sleep）
- 提交信息 Conventional Commits：`feat(scope): 标题`
- 禁止删除操作（删除文件/分支需用户确认）
- 所有环境变量名以 `DM_` 前缀：`DM_AUTH_ENABLED`、`DM_AUTH_TYPE`、`DM_AUTH_USERNAME`、`DM_AUTH_PASSWORD`、`DM_AUTH_TOKEN`、`DM_AUTH_TOKEN_EXPIRES`
- `AuthConfig.ExpiresAt` 字段格式 RFC3339（如 `2027-01-01T00:00:00Z`），空 = 永不过期
- 现有测试必须保持全绿（28 包）

---
## 任务 1（P3-1）：鉴权强化

**文件：**
- 修改：`config/config.go`（AuthConfig 加 `ExpiresAt`、ValidateAndClamp 加 `DM_AUTH_ENABLED` 驱动）
- 修改：`config/global.go`（init() 默认值注释）
- 修改：`api/auth.go`（validateTokenAuth 加过期校验）
- 修改：`config/config_diff.go`（Diff() 补 ExpiresAt 比较）
- 测试：`api/auth_test.go` 扩展 + `config/config_test.go` 扩展

- [ ] **步骤 1：写失败的测试**

```go
// config 测试：DM_AUTH_ENABLED=1 时 auth.type 默认 basic
func TestAuthEnabledEnvDrivesDefault(t *testing.T) {
    t.Setenv("DM_AUTH_ENABLED", "1")
    t.Setenv("DM_AUTH_PASSWORD", "secret")
    cfg := config.Default()
    // ValidateAndClamp 后 Auth.Type == "basic" 且 Password == "secret"
}

// api 测试：ExpiresAt 过期 token 拒绝
func TestTokenAuthExpired(t *testing.T) {
    cfg := config.AuthConfig{Type: "token", Token: "abc", ExpiresAt: "2020-01-01T00:00:00Z"}
    // validateTokenAuth("Bearer abc") == false
}
```

- [ ] **步骤 2：运行确认失败**

运行：`go test ./config/... -run TestAuthEnabled && go test ./api/... -run TestTokenAuthExpired`
预期：FAIL（编译失败或断言失败）

- [ ] **步骤 3：实现**

`config/config.go`：
- `AuthConfig` 加字段：`ExpiresAt string \`yaml:"expires_at" json:"expires_at"\``
- `ValidateAndClamp()` 开头加：
```go
if env := os.Getenv("DM_AUTH_ENABLED"); isTruthy(env) {
    if ac.Type == "" || ac.Type == "none" {
        ac.Type = "basic"
        if ac.Username == "" { ac.Username = "admin" }
        if p := os.Getenv("DM_AUTH_PASSWORD"); p != "" { ac.Password = p }
    }
}
if p := os.Getenv("DM_AUTH_PASSWORD"); p != "" { cfg.Server.Auth.Password = p }
if t := os.Getenv("DM_AUTH_TOKEN"); t != "" { cfg.Server.Auth.Token = t }
if e := os.Getenv("DM_AUTH_TOKEN_EXPIRES"); e != "" { cfg.Server.Auth.ExpiresAt = e }
```
- `isTruthy`：`"1"`/`"true"`/`"yes"`（大小写不敏感）为真

`api/auth.go` `validateTokenAuth`：
```go
if cfg.ExpiresAt != "" {
    exp, err := time.Parse(time.RFC3339, cfg.ExpiresAt)
    if err == nil && time.Now().After(exp) {
        return false
    }
}
```

`config/config_diff.go`：Diff() 比较补 `ExpiresAt` 字段

- [ ] **步骤 4：运行确认通过**

运行：`go test ./config/... ./api/... -count=1`
预期：PASS

- [ ] **步骤 5：Commit**

```bash
git add config/config.go config/global.go config/config_diff.go api/auth.go config/config_test.go api/auth_test.go
git commit -m "feat(auth): DM_AUTH_ENABLED 环境变量驱动默认 basic + token 过期校验"
```

- [ ] **步骤 6：自审**（实现者执行）

---
## 任务 2（P3-2）：SSE 与文件接口边界

**文件：**
- 创建：`api/files.go`（自定义文件 handler）
- 修改：`api/server.go`（注册 files handler + SSE 校验）
- 测试：`api/files_test.go`、`api/sse_test.go`

- [ ] **步骤 1：写失败的测试**

```go
// files_test.go
func TestFiles_PathTraversalRejected(t *testing.T) {
    // GET /files/../../etc/passwd → 403
}
func TestFiles_MIMETextHTMLRejected(t *testing.T) {
    // GET /files/x.html → 403
}
func TestFiles_ReadOnly(t *testing.T) {
    // POST /files/x.mp4 → 405
}
func TestFiles_NoSniffHeader(t *testing.T) {
    // 正常文件响应带 X-Content-Type-Options: nosniff
}

// sse_test.go
func TestSSE_SameOriginOK(t *testing.T) {
    // Origin 与 Host 一致 → 200
}
func TestSSE_CrossOriginRejected(t *testing.T) {
    // Origin 不同 → 403
}
func TestSSE_TokenRequired(t *testing.T) {
    // auth=token 无 Authorization → 401
}
```

- [ ] **步骤 2：运行确认失败**

运行：`go test ./api/... -run TestFiles_ -count=1` 等
预期：FAIL

- [ ] **步骤 3：实现**

`api/files.go`：
```go
func (s *Server) filesHandler() http.Handler {
    root := filepath.Clean(s.mgr.GetDownloadRootDir())
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodGet && r.Method != http.MethodHead {
            writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "read-only files")
            return
        }
        upath := r.URL.Path // 已由 mux 剥离前缀
        if strings.Contains(upath, "..") {
            writeJSONError(w, http.StatusForbidden, "forbidden", "invalid path")
            return
        }
        clean := filepath.Clean(filepath.Join(root, filepath.FromSlash(upath)))
        if !strings.HasPrefix(clean, root) {
            writeJSONError(w, http.StatusForbidden, "forbidden", "path traversal")
            return
        }
        f, err := os.Open(clean)
        if err != nil {
            writeJSONError(w, http.StatusNotFound, "not_found", "file not found")
            return
        }
        defer f.Close()
        stat, err := f.Stat()
        if err != nil || stat.IsDir() {
            writeJSONError(w, http.StatusNotFound, "not_found", "not a file")
            return
        }
        // 用 http.DetectContentType 读前 512 字节判定 MIME
        buf := make([]byte, 512)
        n, _ := f.Read(buf)
        ct := http.DetectContentType(buf[:n])
        if isBlockedMIME(ct) {
            writeJSONError(w, http.StatusForbidden, "forbidden", "content type blocked")
            return
        }
        f.Seek(0, io.SeekStart)
        w.Header().Set("Content-Type", ct)
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
        if r.Method == http.MethodHead { return }
        http.ServeContent(w, r, filepath.Base(clean), stat.ModTime(), f)
    })
}

func isBlockedMIME(ct string) bool {
    switch {
    case strings.HasPrefix(ct, "text/html"), strings.HasPrefix(ct, "text/javascript"),
        ct == "application/javascript", strings.HasPrefix(ct, "image/svg+xml"):
        return true
    }
    return false
}
```

`api/server.go`：
- `/files/` 路由：`r.PathPrefix("/files/").Handler(http.StripPrefix("/files/", s.filesHandler()))`
- SSE 校验中间件（仅 `/api/events`）：
```go
func (s *Server) sseOriginMiddleware() mux.MiddlewareFunc {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            origin := r.Header.Get("Origin")
            if origin != "" {
                oh, err := url.Parse(origin)
                if err != nil || oh.Host != r.Host {
                    writeJSONError(w, http.StatusForbidden, "forbidden", "cross-origin denied")
                    return
                }
            }
            ac := s.mgr.GetConfig().Server.Auth
            if ac.Type == "token" {
                token := r.Header.Get("Authorization")
                if !validateTokenAuth(ac, token) {
                    writeJSONError(w, http.StatusUnauthorized, "unauthorized", "token required")
                    return
                }
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

- [ ] **步骤 4：运行确认通过**

运行：`go test ./api/... -count=1`
预期：PASS

- [ ] **步骤 5：Commit**

```bash
git add api/files.go api/server.go api/files_test.go api/sse_test.go
git commit -m "feat(api): /files/ 只读 + MIME 白名单 + 防穿越；SSE Origin 同源 + token"
```

- [ ] **步骤 6：自审**

---
## 任务 3（P3-3）：部署形态

**文件：**
- 创建：`docker-compose.yml`（根目录）
- 创建：`deploy/download-manager.service`
- 创建：`docs/deploy.md`
- 修改：无

- [ ] **步骤 1：写验收测试（配置校验）**

由于 compose/systemd 是配置+文档，测试重点：
- `deploy/compose_test.go`？—— 不，compose 验证用 `docker compose config`（CI 有 docker）
- 实际：文档/配置无 Go 测试；验证 = `docker compose config --quiet` 语法 + `systemd-analyze verify`

```bash
# 验证命令
docker compose config --quiet   # 语法正确退出 0
systemd-analyze verify deploy/download-manager.service   # 语法正确
```

- [ ] **步骤 2：实现 docker-compose.yml**

内容见规格 §5.1（app + mongo 两容器 + 环境变量强制鉴权 + 卷持久化 + 健康检查 + `DM_AUTH_PASSWORD` 必填 fail-closed）

- [ ] **步骤 3：实现 systemd unit**

`deploy/download-manager.service`（规格 §5.2）：
```ini
[Unit]
Description=download-manager daemon
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=download-manager
WorkingDirectory=/var/lib/download-manager
ExecStart=/usr/local/bin/download-manager --config /etc/download-manager/config.yaml
EnvironmentFile=/etc/download-manager/env
Restart=on-failure
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true

[Install]
WantedBy=multi-user.target
```

- [ ] **步骤 4：实现 docs/deploy.md**

覆盖：Docker Compose 部署步骤、systemd 部署步骤、环境变量清单（DM_AUTH_*）、安全前提（公网必须开启鉴权）、数据持久化说明

- [ ] **步骤 5：验证**

运行：`docker compose config --quiet`（有 docker 时）+ `systemd-analyze verify deploy/download-manager.service`（有 systemd 时）
预期：退出 0

- [ ] **步骤 6：Commit**

```bash
git add docker-compose.yml deploy/download-manager.service docs/deploy.md
git commit -m "feat(deploy): docker-compose app+mongo + systemd unit + 部署文档"
```

- [ ] **步骤 7：自审**

---
## 任务 4（P3-4）：依赖治理文档

**文件：**
- 创建：`docs/dependency-management.md`
- 修改：`go.mod`（确认 replace 注释格式）
- 修改：无

- [ ] **步骤 1：确认现状**

运行：`grep "cocomhub/sproxy" go.mod`
预期：`github.com/cocomhub/sproxy v0.18.0`（已固定）

- [ ] **步骤 2：实现 docs/dependency-management.md**

覆盖：
1. 固定版本策略：所有依赖显式版本，不用 pseudo-version（sproxy 已 v0.18.0）
2. 本地开发 replace：`// replace github.com/cocomhub/sproxy => ../sproxy`（注释掉，避免误提交）
3. 私有 module 访问：CI `Configure private module access` 步骤说明 + 本地 `GOPRIVATE`/`GONOSUMDB` 配置
4. 升级流程：手动批量 PR（不用 dependabot）

- [ ] **步骤 3：Commit**

```bash
git add docs/dependency-management.md
git commit -m "docs(deps): 依赖治理文档（固定版本 + replace 策略 + 私有 module 访问）"
```

- [ ] **步骤 4：自审**

---
## 任务 5（P3-5）：archcheck 门禁

**文件：**
- 创建：`internal/archcheck/layers.go`（包层级登记）
- 创建：`internal/archcheck/arch_test.go`（门禁测试：layers/notest/makefile/release/build_flags/dead_symbols/docs_rules/duplication 8 项）
- 修改：`Makefile`（archcheck 目标 + check-ci 加入）
- 修改：`.notestignore`（如需要）
- 创建：`.deadcodeignore`（如需）

- [ ] **步骤 1：写门禁测试（先红）**

```go
// internal/archcheck/arch_test.go
package archcheck

// TestLayers_NoIllegalDependency：layers 方向性
// TestNotestGate_WiredAndFailsClosed：Makefile notest 不空转 + 空参 fail-closed
// TestMakefile_NoDupTargets：Makefile 目标无重复
// TestReleasePolicy：release-please 配置存在且 pr-title.yml 存在
// TestBuildFlags：ldflags 版本注入格式
// TestDocsRules：docs/ 命名规则
// TestDuplication：重复代码阈值
// TestDeadSymbols：.deadcodeignore 存在
```

- [ ] **步骤 2：运行确认失败**

运行：`go test ./internal/archcheck/... -count=1`
预期：FAIL（门禁未实现）

- [ ] **步骤 3：实现 layers.go**

参考 sproxy `internal/archcheck/layers.go` 模式（包路径 → 层级），登记 dm 包：
```go
var Managed = map[string]bool{
    "github.com/cocomhub/download-manager/api": true,
    "github.com/cocomhub/download-manager/config": true,
    "github.com/cocomhub/download-manager/core": true,
    "github.com/cocomhub/download-manager/downloader": true,
    "github.com/cocomhub/download-manager/manager": true,
    "github.com/cocomhub/download-manager/model": true,
    "github.com/cocomhub/download-manager/pkg/download": true,
    "github.com/cocomhub/download-manager/storage": true,
    "github.com/cocomhub/download-manager/task": true,
}
```
层级规则（简版）：model/config 不依赖 manager/api；task 不依赖 manager；storage 不依赖 task 等

- [ ] **步骤 4：实现各门禁测试**

按 sproxy 对应文件裁剪实现（见规格 §7.1 表），注意 dm 差异：
- ldflags 用 `-X main.Version`（sproxy 用 pkg/version）
- main 包是 `./`（根），deadcode 目标调整

- [ ] **步骤 5：Makefile 对齐**

```make
archcheck:
	$(GO) test $(GOTAGS) ./internal/archcheck/...
```
`check-ci` 追加 `archcheck`

- [ ] **步骤 6：运行确认通过**

运行：`make archcheck && make check-ci`
预期：PASS（可能需迭代修复门禁暴露的真实问题）

- [ ] **步骤 7：Commit**

```bash
git add internal/archcheck/ Makefile .notestignore .deadcodeignore
git commit -m "feat(archcheck): 门禁对齐 sproxy（layers/notest/makefile/release/build_flags/dead_symbols/docs_rules/duplication）"
```

- [ ] **步骤 8：自审**
