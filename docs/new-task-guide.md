# 新任务开发指南

> 完整清单见 `docs/new-task-checklist.md`；本文档为「最小实现点」速查。

## 推荐接入路径（三件套）

新任务类型推荐使用 **PagingScanner + SiteAdapter + UI 插件** 三件套：

1. **task.go** — 注册工厂 + `NewTask`（从 `cfg.Extra` 读站点参数）+ 组装 PagingScanner/SiteAdapter
2. **adapter.go** — 实现 `task.SiteAdapter`：分页生命周期 + 对象构建（缓存优先）
3. **ui/** — 前端插件（`task/TEMPLATE/ui/` 模板 + 脚手架生成）

脚手架生成 Go 骨架：

```bash
TYPE=mytype LABEL="My Type" ./scripts/new-task-type.sh
# → task/mytype/task.go + adapter.go + ui/ui.go + ui/assets/viewer.js
```

## 最小实现点

- **分页列表**（可选）：实现 `SiteAdapter` 的 `BuildPageURL`/`RunScraper`/`ParseTotalPages`/`ParsePage`，
  由 `PagingScanner`（`task/scanner.go`）驱动分页抓取 → 去重 → 构建 → 持久化。
- **对象构建**：`BuildObject` 构建 DownloadObject；**缓存优先** 先查 `BaseTask.GetCachedObject(url)`。
- **详情解析**（可选）：实现 `ResolveObject` 填充 `Metadata`（title/date/tags）与 `Extra`（files/images）。
- **返回对象列表**：`BaseTask` 已提供 `GetDownloadObjects`（从 runtime + storage 收集非终态对象）。

## 建议组合

- 通过 `NewBaseTask(cfg, opts)` 自动获得：`PathStrategy`（路径策略）、状态管理、去重缓存、共享注册表。
- `Scrape` 默认委托给 PagingScanner（`BaseTask.Scrape`）；直连型任务（如 urllist）可覆盖返回 nil。

## 状态回填与去重

- `PagingScanner.processItems` 自动调用 `CheckAndRestoreStatus`（共享注册表 + 存储恢复状态）。
- `BuildObject` 缓存命中时复用已存在对象，未完成对象重启时状态自动恢复。

## 命名规范

- 优先通过 `PathStrategy` 统一生成路径，避免各任务分散。
- 文件名避开非法字符：`/`、`\`、结尾点等。

## 测试建议

- 单元测试覆盖：分页终止条件（空页熔断）、对象构建、缓存优先、状态回填。
- 使用本地示例 HTML 做解析测试，避免对线上站点施压；测试只绑 127.0.0.1。
