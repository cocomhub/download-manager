# Download Manager

面向多源任务的下载管理器：采集、排队、下载、状态追踪、聚合浏览与配置治理。

## 功能总览

- **多任务类型**：urllist（通用 URL 列表）、tktube / hanime（视频站）、vikacg（图片/漫画站），任务接入标准化（SiteAdapter + TaskUI 插件）
- **多存储后端**：memory / file / mongo（v2 + testcontainers 集成测试）
- **多下载器**：native（HTTP，域名限流 + 代理池 + 重试 + 断点续传）、wget、scraper、composite / multi
- **HLS 链路**：m3u8 下载 + ffmpeg 合并（`cmd/m3u8d` 独立 CLI）
- **对象生命周期**：pending → downloading → completed / failed / cancelled，支持取消/撤销/批量/重试/重排
- **聚合视图**：跨任务搜索、筛选、排序、分页、内容分组、合集与推荐
- **实时事件**：SSE 推送对象/任务状态变更
- **配置治理**：在线更新、备份、回滚、diff、标签与备注
- **鉴权**：basic / token（`DM_AUTH_ENABLED` 驱动，Docker/公网默认开启），UI 登录页
- **运行模式**：full（下载 + 管理） / ui（仅管理界面，只读）

## 快速开始

### 本地构建

```bash
make build          # 构建到 build/bin/download-manager
./build/bin/download-manager --config config.yaml
```

UI 仅模式：

```bash
./build/bin/download-manager --ui-only
```

### Docker Compose（推荐生产）

```bash
cp deploy/docker/config.yaml.example config.yaml   # 若存在
docker compose up -d                               # app + mongo 两容器
```

> 容器部署需设置 `DM_AUTH_PASSWORD`（必填，fail-closed）；详见 [部署文档](./docs/deploy.md)。

### 配置

- 完整配置参考：[docs/config.md](./docs/config.md)
- 示例配置：仓库根 `config.yaml`（本地测试用）

## 环境变量

| 变量 | 说明 |
|------|------|
| `DM_AUTH_ENABLED` | 1 时默认开启 basic 鉴权（用户名 admin + `DM_AUTH_PASSWORD`） |
| `DM_AUTH_PASSWORD` / `DM_AUTH_TOKEN` / `DM_AUTH_TOKEN_EXPIRES` | 鉴权凭据与过期时间（RFC3339） |
| `DM_MONGO_URI` | 容器部署指向 mongo 服务（如 `mongodb://mongo:27017/...`） |
| `DM_RUN_MODE` / `DM_UI_ONLY` | 运行模式（full / ui），优先级低于 CLI flag |

## 工程约定

- 优先使用标准库：`errors` / `os/io` / `net/http` / `context` / `sync`
- 日志统一 `pkg/logutil`（slog + lumberjack 轮转）
- 单实例文件锁 `github.com/gofrs/flock`
- 提交遵循 Conventional Commits（`type(scope): subject`，pr-title 门禁）
- CI 门禁：Test × 4 / Lint / Playwright UI E2E / SonarQube + archcheck（分层/notest/构建对齐）

## 文档

- [当前功能特性总览](./docs/current-capabilities.md)
- [配置参考](./docs/config.md)
- [任务能力矩阵](./docs/tasks-capabilities.md)
- [新任务开发指南](./docs/new-task-guide.md)
- [部署指南（Docker / systemd）](./docs/deploy.md)
- [依赖治理](./docs/dependency-management.md)
- [实施 Roadmap](./docs/implementation-roadmap.md)

## 发布

版本由 release-please 自动管理（Conventional Commits 驱动），tag 推送触发 GoReleaser 构建 6 平台制品（darwin/linux/windows × amd64/arm64）+ checksums。
