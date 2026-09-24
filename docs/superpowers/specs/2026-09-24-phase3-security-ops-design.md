# 阶段 3：安全与运维 / 产品化 — 设计规格

> 日期：2026-09-24
> 状态：已由用户批准（2026-09-24）
> 前置：阶段 0-2 全部完成（P0 发布/工程对齐、P1 健康度/稳定性、P2 产品功能）

## 1. 背景与目标

1 年产品化 roadmap 最后阶段。目标：部署形态就绪、安全边界收敛、工程能力对齐 sproxy。

**已确认决策（用户 2026-09-24）**：
1. P3-1 鉴权默认策略：**仅 Docker/公网形态默认开启**（`DM_AUTH_ENABLED` 环境变量驱动），本地二进制保持 `auth.type=none`
2. P3-1 token 管理：**静态 token + 环境变量**（`DM_AUTH_TOKEN`），补 `expires_at` 可选过期校验，不新增签发/吊销 API
3. P3-2 /files/ 防护：**路径 Clean 校验防穿越 + 只读 + MIME 白名单**（拦截 html/js/svg）+ `X-Content-Type-Options: nosniff`
4. P3-2 SSE 边界：**Origin 同源校验**（配置 base URL 白名单）+ 鉴权已开启时要求 token
5. P3-3 docker-compose：**app + mongo 两容器**，环境变量强制鉴权 + 数据卷持久化 + 健康检查
6. P3-5 archcheck：**全套 8 项照搬 sproxy**
7. 收官：阶段 3 完成后打 **v0.3.0**；**另开阶段 4** 继续

## 2. 现状基线（实证）

| 维度 | 现状 |
|------|------|
| 鉴权 | `api/auth.go` 已有 basic/token 中间件（`crypto/subtle` 常量时间比较），默认 `auth.type=none` |
| token | 仅 `DM_AUTH_TOKEN` 环境变量覆盖，无过期校验 |
| SSE | `/api/events` 裸暴露（`api/server.go:117`），无 Origin/token 校验 |
| /files/ | `http.FileServer(http.Dir(GetDownloadRootDir()))` 裸暴露（`api/server.go:129`） |
| 部署 | 无 docker-compose / systemd / 部署文档 |
| sproxy 依赖 | `v0.18.0` 正式版已固定（✅ 完成），`replace` 指令已注释 |
| archcheck | `internal/archcheck/` 不存在 |
| notest 门禁 | `scripts/check-test-files.sh` 两处 bug（忽略逻辑反向 + 无空参 fail-closed），已修复（PR #81） |
| DomainLimiter | 已随 dlcore 退役迁移到 `pkg/download`（channel 等待队列），**非 spin-loop**（✅ P1-3 完成） |

## 3. P3-1 鉴权强化

### 3.1 默认策略：仅 Docker/公网形态默认开启

- 新增 `DM_AUTH_ENABLED` 环境变量（取值 `1`/`true`/`yes` 任意大小写视为启用）
- 环境变量驱动逻辑：`config.ValidateAndClamp()` 中，若 `DM_AUTH_ENABLED` 启用且 `auth.type` 未显式设置（空/none），则将 `auth.type` 设为 `basic`（用户名默认 `admin`，密码取 `DM_AUTH_PASSWORD` 或默认 `admin` 并打警告日志）
- 本地二进制（无 `DM_AUTH_ENABLED`）保持 `auth.type=none`，不破坏现有部署
- 默认值提示：`config/global.go` init() 中注释说明

### 3.2 token 过期校验

- `AuthConfig` 新增 `ExpiresAt string` 字段（yaml/json tag：`expires_at`，RFC3339 格式，空=永不过期）
- `validateTokenAuth` 中：`cfg.ExpiresAt` 非空时解析，若当前时间已过期则拒绝
- 环境变量 `DM_AUTH_TOKEN_EXPIRES` 可选覆盖（RFC3339，空=不覆盖）
- 不做签发/吊销 API（用户已确认静态 token 方案）

### 3.3 UI 登录态

- 401 响应时前端检测 `WWW-Authenticate` 头，显示登录提示（basic 模式浏览器原生弹窗；token 模式 UI 显示配置提示）
- 仅 web 静态资源与 API 响应头层面改动，不做登录页跳转（保持简单）

## 4. P3-2 SSE 与文件接口边界

### 4.1 /files/ 防护

- 替换裸 `http.FileServer` 为自定义 handler（`api/files.go` 新文件）：
  - **路径 Clean 校验**：`path.Clean` + 拒绝含 `..` 的 URL 路径（防穿越）
  - **只读**：仅 GET/HEAD，其他方法返回 405
  - **MIME 白名单**：以 `Content-Type` 判定，拦截 `text/html`、`application/xhtml+xml`、`text/javascript`、`application/javascript`、`image/svg+xml`（返回 403）；其余（视频/图片/文本等）放行
  - **安全头**：`X-Content-Type-Options: nosniff`
- 保持 `GET /files/` 目录列表？—— 不，关闭目录列表（`http.FileServer` 的 index 行为移除，仅精确文件 + 子路径）

### 4.2 SSE 边界

- `/api/events` 增加校验：
  - **Origin 同源**：`Origin` 头存在且不在白名单（`Server.BaseURLs []string` 配置或默认同源：请求 Host 与 Origin Host 一致）→ 403
  - **鉴权 token**：`auth.type=token` 时要求 `Authorization: Bearer <token>`（basic 时沿用基本认证）
  - 无 Origin 头（curl/非浏览器）放行（保持 API 兼容）

### 4.3 测试

- `api/files_test.go`：穿越防护（`../`、`%2e%2e` 编码）、只读方法、MIME 拦截（html/js/svg）、nosniff 头、正常文件放行
- `api/sse_test.go`：同源放行、跨源 403、token 缺失 401、带 token 放行

## 5. P3-3 部署形态

### 5.1 docker-compose.yml（根目录新建）

```yaml
services:
  app:
    build: .
    ports: ["8080:8080"]
    environment:
      - DM_AUTH_ENABLED=1
      - DM_AUTH_TYPE=basic
      - DM_AUTH_USERNAME=${DM_AUTH_USERNAME:-admin}
      - DM_AUTH_PASSWORD=${DM_AUTH_PASSWORD:?must set}
      - DM_MONGO_URI=mongodb://mongo:27017/download_manager
    volumes:
      - dm-data:/data
    depends_on:
      mongo:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:8080/api/healthz"]
      interval: 30s
      timeout: 5s
      retries: 3
  mongo:
    image: mongo:7
    volumes:
      - mongo-data:/data/db
    healthcheck:
      test: ["CMD", "mongosh", "--eval", "db.adminCommand('ping')"]
      interval: 30s
      timeout: 5s
      retries: 3
volumes:
  dm-data:
  mongo-data:
```

- `DM_AUTH_PASSWORD` 必填（`${VAR:?}` 语法强制），未设置则 compose 启动失败（fail-closed）
- 数据卷 `dm-data`（下载根目录）/ `mongo-data`（mongo 持久化）
- `.dockerignore` 已有；Dockerfile 检查 `work_dir` 与下载目录默认值匹配卷挂载

### 5.2 systemd unit

- `deploy/download-manager.service`：
  - `ExecStart=/usr/local/bin/download-manager --config /etc/download-manager/config.yaml`
  - `WorkingDirectory=/var/lib/download-manager`
  - `User=download-manager`（新建系统用户）
  - `Restart=on-failure`
  - `EnvironmentFile=/etc/download-manager/env`（可选注入 `DM_AUTH_*`）
  - `ProtectSystem=strict`、`NoNewPrivileges=true`、`PrivateTmp=true` 等加固

### 5.3 部署文档

- `docs/deploy.md`：Docker Compose 步骤、systemd 步骤、环境变量清单、安全前提（公网必须开鉴权）

## 6. P3-4 sproxy 依赖治理（已完成，补文档）

- `go.mod` 已固定 `github.com/cocomhub/sproxy v0.18.0`（PR #64）
- 补 `docs/dependency-management.md`：固定版本策略、本地 `replace` 开发策略（`// replace github.com/cocomhub/sproxy => ../sproxy` 注释）、私有 module 访问配置（CI 已有 Configure private module access 步骤）

## 7. P3-5 archcheck 门禁对齐 sproxy

### 7.1 门禁清单（全套 8 项照搬）

| # | 门禁 | 对应 sproxy 文件 | dm 落点 |
|---|------|-----------------|---------|
| 1 | layers 方向性 | `arch_test.go` + `layers.go` | `internal/archcheck/layers.go`：登记 dm 各包层级（config → model → storage → core → task → manager → api → downloader → pkg/*） |
| 2 | notest 门禁 | `notest_gate_test.go` | 已验证修复（PR #81），门禁守卫 Makefile notest 不空转 |
| 3 | makefile 一致性 | `makefile_target_dup_test.go` 等 | 守卫 Makefile 目标无重复、check-ci 含全部必需目标 |
| 4 | release 策略 | `release_policy_test.go` | 守卫 release-please 配置 + pr-title + CHANGELOG 策略 |
| 5 | build_flags 对齐 | `build_flags_alignment_test.go` | 守卫 ldflags 版本注入（download-manager 用 `-X main.Version`） |
| 6 | dead_symbols | `dead_symbols_test.go` | 可配置 `.deadcodeignore`（按 dm 规模裁剪） |
| 7 | docs_rules | `docs_rules_test.go` | 文档命名/索引规则（按 dm 规模裁剪） |
| 8 | duplication | `duplication_test.go` | 重复代码检测（按 dm 规模裁剪） |

### 7.2 Makefile 对齐

- 新增 `archcheck` 目标：`go test ./internal/archcheck/...`
- `check-ci` 加入 `archcheck`
- 对齐 `web-test`（前端 JS 单测挂 CI）、`deadcode-check`（按 dm 的 main 包调整）

### 7.3 无测试包登记

- `.notestignore` 已登记 8 个包（PR #81）：task/*/ui、cmd/playwright-server(+fixture)、testutil/assert、web、cmd/scraper_get/tunnel

## 8. 收官与发布

- 阶段 3 完成后：合并全部 PR → `make check-ci` 全绿 → release-please 自动 bump v0.3.0（feat 类提交）
- 同步 roadmap.md 标记 M3 里程碑达成
- 另开阶段 4（内容待定）

## 9. 验收标准汇总

| 条目 | 验收 |
|------|------|
| P3-1 | `DM_AUTH_ENABLED=1` 时 API 401；`DM_AUTH_TOKEN_EXPIRES` 过期拒绝；本地无环境变量时行为不变 |
| P3-2 | `/files/../etc/passwd` 403；`/files/x.html` 403；SSE 跨源 403；token 缺失 401 |
| P3-3 | `docker compose up` 可复现；systemd unit 语法正确（`systemd-analyze verify`） |
| P3-4 | 依赖版本显式（go.mod 已固定），文档覆盖 replace 策略 |
| P3-5 | `make check-ci` 全绿，archcheck 8 项通过，门禁变异验证命中 |
