# CI/CD 升级与 P0 治理设计规格

> 状态: 待实现
> 说明: 经代码审查，原有 Docker/CI/GoReleaser/优雅停机/写保护等模块已完成实现，本文档仅覆盖**真正缺失**的部分，避免重复。

## 1. 当前实现状态

| 模块 | 文件 | 状态 |
|------|------|------|
| Docker 多阶段构建 | `Dockerfile` | ✅ 已有 2-stage (golang:1.26-alpine + alpine:3.21)，含 ffmpeg/wget |
| `.dockerignore` | `.dockerignore` | ✅ 已排除 .git/ node_modules/ build/ 等 |
| 优雅停机 | `manager/scheduler.go` `main.go` | ✅ `Stop(ctx)` + `WaitForShutdown(ctx)` + signal 处理 + ForceFlush |
| 写操作保护 | `api/server.go` | ✅ 全局 `writeMiddleware`，18+ 端点全覆盖 |
| 写保护测试 | `api/server_write_guard_test.go` | ✅ 表驱动测试 3 个场景（UI-only / Full+both-disabled / Full+single-enabled） |
| Codecov 配置 | `.codecov.yml` | ✅ target=60%, threshold=2%, patch target=70%（需将 project target 同步为 40%） |
| GoReleaser | `.goreleaser.yaml` | ✅ linux/darwin/windows × amd64/arm64，含 checksum + changelog |
| 跨平台 CI | `.github/workflows/ci.yml` | ✅ ubuntu/windows/macOS × go 1.26 矩阵 |
| Codecov 上传 CI | `.github/workflows/ci.yml` | ✅ unittests/no_mongo 双 flags |
| Release + Docker push | `.github/workflows/release.yml` | ✅ goreleaser + ghcr.io 推送 |

### 真正缺失的部分

| 模块 | 缺失内容 | 优先级 |
|------|----------|--------|
| Dockerfile 增强 | HEALTHCHECK、非 root 用户、默认端口、私有模块认证 | P1 |
| API 鉴权 | 无认证机制，直接对外暴露 | P0 |
| 下载路径一致性 | `/files/` 与下载落盘目录可能不一致 | P1 |
| codecov.yml 与 CI 对齐 | CI 门禁 20% 但 codecov.yml target=60%，需统一 | P1 |

---

## 2. Dockerfile 增强

### 改动清单

```dockerfile
# 当前 Dockerfile（已实现）
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git ca-certificates
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w -X main.Version=$(git describe --tags 2>/dev/null || echo dev) -X main.BuildAt=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o /build/download-manager .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata ffmpeg wget
COPY --from=builder /build/download-manager /usr/local/bin/download-manager
EXPOSE 8080
ENTRYPOINT ["download-manager"]
CMD ["--config", "/etc/download-manager/config.yaml"]
```

### 需要新增/修改

```dockerfile
# 1. 非 root 用户（runtime stage，builder stage 之后）
RUN adduser -D appuser && chown -R appuser /usr/local/bin/download-manager
USER appuser

# 2. HEALTHCHECK
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1:${DM_HTTP_PORT:-19199}/api/healthz || exit 1

# 3. 保持默认 8080 端口（config.yaml 默认值）
EXPOSE 8080
```

### 私有模块认证（仅 CI 构建需要）

通过 Docker BuildKit `--secret` 注入 SSH key 或 GitHub PAT：

```dockerfile
# 在 builder stage 的 COPY . . 之后新增
RUN --mount=type=secret,id=git_credentials \
    if [ -f /run/secrets/git_credentials ]; then \
        mkdir -p ~/.ssh && \
        cp /run/secrets/git_credentials ~/.ssh/id_ed25519 && \
        chmod 600 ~/.ssh/id_ed25519 && \
        ssh-keyscan github.com >> ~/.ssh/known_hosts 2>/dev/null; \
    fi && \
    CGO_ENABLED=0 go build -o /build/download-manager .
```

Makefile 新增目标：

```makefile
# Docker targets
docker-build:
	docker build -t download-manager:latest .

docker-build-secure:
	docker buildx build \
		--secret id=git_credentials,src=$(HOME)/.ssh/id_ed25519 \
		-t download-manager:latest .
```

---

## 3. API 鉴权

### 配置段

```yaml
# config.yaml 新增
server:
  auth:
    type: none      # none | basic | token
```

### Config 结构体

```go
// config/config.go
type Server struct {
    Auth AuthConfig `yaml:"auth"`
}

type AuthConfig struct {
    Type string `yaml:"type"` // "none" | "basic" | "token"
}
```

环境变量覆盖：
- `DM_AUTH_TYPE` — 覆盖 `server.auth.type`
- `DM_AUTH_USERNAME` — Basic Auth 用户名（默认 `admin`）
- `DM_AUTH_PASSWORD` — Basic Auth 密码
- `DM_AUTH_TOKEN` — Token Auth 的 bearer token

### 中间件

```go
// api/auth.go（新增）
func authMiddleware(cfg *config.AuthConfig) mux.MiddlewareFunc {
    switch cfg.Type {
    case "none":
        return nopMiddleware
    case "basic":
        return basicAuthMiddleware(cfg.Username, cfg.Password)
    case "token":
        return tokenAuthMiddleware(cfg.Token)
    default:
        return nopMiddleware
    }
}
```

鉴权中间件在 `writeMiddleware` **之前**执行：先验证身份，再检查写权限。

### 注册到 Router

```go
// api/server.go
func (s *Server) Router() *mux.Router {
    r := mux.NewRouter()
    
    // Auth middleware first, then write guard
    r.Use(s.authMiddleware())
    r.Use(s.writeMiddleware)
    // ... routes
}
```

### 测试

```
TestAuth_None_Pass              → type=none + GET → 200
TestAuth_None_WriteGuard        → type=none + POST (UI mode) → 405（仍受写保护）
TestAuth_Basic_Valid            → basic + 正确凭据 → 200
TestAuth_Basic_Invalid          → basic + 错误凭据 → 401
TestAuth_Basic_Missing          → basic + 无凭据 → 401
TestAuth_Token_Valid            → token + 正确 token → 200
TestAuth_Token_Invalid          → token + 错误 token → 401
TestAuth_Token_Empty            → token + 空 token（env 未设置）→ 等价 none
```

---

## 4. 下载路径一致性

### 问题分析

当前实现：
- 下载引擎使用 `config.Downloader.OutputDir` 作为文件落盘目录
- `/files/` HTTP handler 直接暴露指定目录（默认可能不同）

需要验证 `api/server.go` 中 `/files/` handler 使用哪个目录。

### 设计

```go
// config/config.go
func (c *Config) FileRoot() string {
    if c.Server.FilesDir != "" {
        return c.Server.FilesDir
    }
    return c.Downloader.OutputDir
}
```

`/files/` handler 通过 `cfg.FileRoot()` 获取根目录，确保路径一致。

### Config 结构体扩展

```go
type Server struct {
    // ... 现有字段 ...
    Auth     AuthConfig `yaml:"auth"`
    FilesDir string     `yaml:"files_dir"` // 可选，覆盖默认文件服务根目录
}
```

### 变更文件

| 文件 | 动作 |
|------|------|
| `config/config.go` | 新增 `FilesDir` 字段 + `FileRoot()` 方法 |
| `api/server.go` | `/files/` handler 使用 `cfg.FileRoot()` |
| `config/config_diff.go` | 确保 `FilesDir` 变更在 Diff() 中被检测 |

### 测试

```
TestFileRoot_Default_UseOutputDir → FilesDir 未设置 → FileRoot() == OutputDir
TestFileRoot_Explicit_UseFilesDir → FilesDir 设置 → FileRoot() == FilesDir
TestFileRoot_Clone_RaceFree       → Clone() 后 FileRoot() 正确
```

---

## 5. 默认端口统一为 8080

### 当前状态

| 位置 | 值 | 来源 |
|------|-----|------|
| `config.yaml` | `http_port: 8080` | ✅ 已正确 |
| `config.go ValidateAndClamp()` | **缺少默认值** | ⚠️ HTTPPort 零值（0）时未回退，无配置文件时端口为 0 |
| `Dockerfile` | `EXPOSE 8080` | ✅ 已正确 |

### 改动

```go
// config/config.go ValidateAndClamp() 中加一段
// Server defaults
if c.Server.HTTPPort <= 0 {
    c.Server.HTTPPort = 8080
}
if c.Server.UIOnlyPort <= 0 {
    c.Server.UIOnlyPort = 8091
}
```

测试中端口通过 `httptest.NewServer` 或配置显式传入随机端口，不受此默认值影响。

当前状态：
- CI 门禁：`make cover-check` 检查 `COVER_THRESHOLD=20%`（local + CI）
- Codecov：`.codecov.yml` 配置 `project.target=60%`

矛盾点：CI 会同时跑 `make cover-check`（检查 20%）和 Codecov（期望 60%）。当前项目的实际覆盖率在 20-30% 区间，Codecov 的 60% target 会导致 PR 始终标红。

### 方案

同步 `.codecov.yml` 与 CI 门禁一致：

```yaml
# .codecov.yml 修改
coverage:
  status:
    project:
      default:
        target: 40%       # 与 make cover-check 的 COVER_THRESHOLD 一致
        threshold: 2%     # PR 允许 2% 波动
    patch:
      default:
        target: 60%       # 新代码期望 60%（比 project 高但留有余地）
```

同步更新 CI 中 make cover-check 的阈值引用，确保一致性。

---

## 6. 文件变更总览

| 文件 | 动作 | 归属 Sprint |
|------|------|-------------|
| `Dockerfile` | 修改：HEALTHCHECK + USER nonroot + EXPOSE 19199 + secret mount | Sprint 1 |
| `Makefile` | 修改：新增 docker-build / docker-build-secure 目标 | Sprint 1 |
| `codecov.yml` | 修改：project target 60% → 40%，patch 70% → 60% | Sprint 1 |
| `.github/workflows/ci.yml` | 可能的微调：验证 codecov.yml 与 make cover-check 不冲突 | Sprint 1 |
| `api/auth.go` | **新增**：auth 中间件（none/basic/token） | Sprint 2 |
| `api/auth_test.go` | **新增**：7 个表驱动测试用例 | Sprint 2 |
| `config/config.go` | 修改：新增 `Server.Auth` + `Server.FilesDir` + `FileRoot()` | Sprint 2 |
| `api/server.go` | 修改：注册 auth middleware；`/files/` 使用 `FileRoot()` | Sprint 2 |
| `config/config_diff.go` | 验证：FilesDir 在 Diff() 中被检测 | Sprint 2 |

---

## 7. 验证策略

### Sprint 1 验证

| 项 | 命令/方式 |
|----|-----------|
| Docker 构建 | `docker build -t dm-test . && docker run --rm dm-test --version` |
| Docker 健康检查 | `docker run -d -p 19199:19199 dm-test && sleep 3 && curl http://127.0.0.1:19199/api/healthz` |
| Docker 非 root | `docker run --rm dm-test whoami` → 应输出 `appuser` |
| codecov.yml 验证 | `curl -s https://codecov.io/validate -d @codecov.yml` |

### Sprint 2 验证

| 项 | 命令/方式 |
|----|-----------|
| Auth none | `curl http://127.0.0.1:19199/api/healthz` → 200 |
| Auth basic | `curl -u admin:pass http://...` → 200；`curl http://...` → 401 |
| Auth token | `curl -H "Authorization: Bearer xxx" http://...` → 200；无 header → 401 |
| 路径一致性 | `/files/` 返回内容与 `config.Downloader.OutputDir` 中一致 |
| 差异检测 | 修改 FilesDir 后 `GET /api/config/diff` 能检测到变更 |

### 通用测试要求

- 所有测试使用 `t.Run` 子测试 + 表驱动模式
- 使用 `t.Setenv` / `t.TempDir` / `t.Context`（Go 1.24+）提升测试可靠性
- 辅助函数调用 `t.Helper()`
- CI 中必跑 `go test -race -count=1 -timeout=180s`
