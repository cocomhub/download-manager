# 依赖治理

本文档说明本项目的 Go 依赖管理策略，包括固定版本、本地开发 replace、私有 module 访问配置与升级流程。

## 固定版本策略

**所有直接依赖必须在 `go.mod` 中声明显式版本号，禁止使用未固定的 pseudo-version 作为常态。**

- 优先使用上游正式 release tag（`v1.2.3` 形态）。
- 上游没有发布任何 tag、且确有依赖需求时，才允许 `v0.0.0-<timestamp>-<commit>` 形态的 pseudo-version（例如 `github.com/dop251/goja`），并在升级流程中持续跟踪上游是否发布了正式版本。
- 禁止依赖浮动的 `master`/`main` 分支引用。

当前状态：

- `github.com/cocomhub/sproxy` 已固定为正式版 `v0.18.0`（`go.mod` 直接 require），不再依赖 pseudo-version 或本地 replace 参与构建。
- 依赖变更后运行 `go mod tidy` 并提交 `go.mod` + `go.sum`。

## 私有 module 访问

本项目依赖同组织的私有 module（`github.com/cocomhub/sproxy`）。Go 默认通过公共 proxy（`proxy.golang.org`）与公共 sum 校验服务拉取依赖，私有仓库需要额外配置。

### CI 配置

所有需要拉取依赖的 job 都包含 `Configure private module access` 步骤：

```yaml
- name: Configure private module access
  env:
    GH_PAT: ${{ secrets.GH_PAT }}
  run: |
    git config --global url."https://x-access-token:${GH_PAT}@github.com/".insteadOf "https://github.com/"
```

- `GH_PAT` 是仓库 secret，需要一个对该组织私有仓库有读权限的 GitHub Personal Access Token。
- `git config ... insteadOf` 让 go 工具链在拉取 `https://github.com/...` 时自动携带 token 认证。
- 测试矩阵的 `Verify dependencies` 步骤（仅 ubuntu）额外设置：

```yaml
env:
  GOPRIVATE: github.com/cocomhub/*
  GONOSUMDB: github.com/cocomhub/*
```

然后执行 `go mod verify`，确保依赖树与 `go.sum` 一致。

### 本地开发配置

本地机器执行：

```bash
go env -w GOPRIVATE=github.com/cocomhub/*
go env -w GONOSUMDB=github.com/cocomhub/*
```

- `GOPRIVATE` 表示这些 module 不走公共 proxy 也不做公共 sum 校验，已隐含设置 `GONOPROXY` 与 `GONOSUMDB`；显式设置 `GONOSUMDB` 仅为与 CI 对齐、语义清晰。
- 拉取私有 module 需要本地 git 已配置对 `github.com/cocomhub` 仓库的访问（SSH key 或 PAT）。
- 如需临时绕过公共 proxy 时可用 `GONOPROXY`，一般无需单独设置。

## 本地开发 replace 策略

本地需要同时调试 download-manager 与 sproxy 时，使用 replace 指向本地 checkout：

```go
// replace github.com/cocomhub/sproxy => ../sproxy
```

- 该行已以**注释形式**预留在 `go.mod` 末尾，防止误提交。
- 使用时：取消注释，并把 `../sproxy` 改成实际本地路径；`go mod tidy` 后本地构建即指向本地代码。
- **提交前必须恢复为注释形式**：replace 会改变构建语义，且相对路径在他人/CI 环境不可用，一旦进入主分支会导致构建失败或静默使用错误代码。

## 升级流程

**不使用 dependabot**（`dependabot.yml` 已删除），依赖升级采用手动批量 PR。

流程：

1. 定期检查依赖更新（或在上游发布新版本后）。
2. 本地执行升级并验证：

   ```bash
   go get github.com/cocomhub/sproxy@v0.19.0   # 单个依赖
   # 或批量：go get -u ./...
   go mod tidy
   make build-ci
   make test-cover
   ```

3. 提交一个集中的依赖升级 PR，提交信息使用 Conventional Commits，例如：

   ```text
   chore(deps): 升级 sproxy 至 v0.19.0
   ```

4. CI 全绿后合并。

注意：

- sproxy 的升级必须等待其正式版本发布（见上「固定版本策略」），不要引用其分支或 pseudo-version。
- 批量升级多个依赖时，如某个依赖引入破坏性变更，拆分为独立 PR 以便回滚定位。
