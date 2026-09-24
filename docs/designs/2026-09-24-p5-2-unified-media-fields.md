# P5-2 unified-media-fields 合入评估设计（2026-09-24）

## 背景 / 目标

- 现状证据：保留分支 `feature/unified-media-fields`（基于旧 master 8dee6ec）含 4 个功能提交：
  - `f4a592f` 统一 cover/thumb/preview 固定字段 + 内容分组书橱视图
  - `91a3ae2` content 分组聚合下推存储层 + 封面小对象只下载一次/回填补下载
  - `464810c` 对象数据结构版本升级机制 + 卡片封面按类型比例
  - `7604fa9` 下载期 obj.Metadata/Extra 并发 map 读写 fatal 修复
- 与 master diff 151 文件（含 pkg/dlcore 13 个已删除文件 = 过期部分）
- **关键发现**：master 已有 `manager/small_object.go`、`manager/standardization.go`、`model/object.go`——但内容落后于分支（分支版本有 `smallObjectKey` 去重 + `fileExistsNonEmpty` + `drainPendingSO` 自愈 + `Snapshot()` 深拷贝）
- 目标：评估并合入分支独有价值（小对象自愈、媒体字段回填、版本升级、并发修复）

## 组件与接口

- `manager/small_object.go`（改）：分支版增强（smallObjectKey 按 taskID 前缀、fileExistsNonEmpty 启发式、finalizeSmallObject 写回媒体字段、drainPendingSO 补下载自愈）
- `model/object.go`（改）：`Snapshot()` 深拷贝（cloneExtraValue 覆盖 []string/[]map/slice-of-any/map）
- `model/object_meta.go`（改）：SetMedia/GetMedia 固定字段访问器（cover_url/cover_path/thumb_url/thumb_path/preview_url/preview_path）
- `core/interfaces.go`（改）：SmallObjectInfo / SmallObjectProvider / ObjectVersioner 接口
- `manager/aggregate.go`（改）：内容分组聚合下推
- `web/static/app/`（改）：书橱视图 + 卡片封面按类型比例

## 数据流

1. 对象下载完成 → `enqueueSmallObjects` 检查 SmallObjectProvider → 小对象入队
2. 队列满 → `drainPendingSO` 记录待补 → worker 空闲/下次入队时补下载
3. 小对象下载成功 → `finalizeSmallObject` 写回父对象固定媒体字段
4. 启动标准化 → ObjectVersioner 检查 version < LatestVersion → 逐级升级

## 错误处理

- 小对象队列满：不阻塞主下载，记录 pending + 写回媒体字段（不静默丢失）
- 下载失败：最多 3 次指数退避，失败不写媒体字段
- TOCTOU：enqueue 与 worker 取出之间再次查文件存在

## 测试 + 变异点

- 分支已有测试可移植：`manager/media_backfill_test.go`、`model/object_race_test.go`、`model/version_test.go`、`api/server_media_test.go`、`manager/content_group_reps_test.go`
- **变异点**：删 drainPendingSO → 队列满时小对象丢失（测试红）；删 Snapshot 深拷贝 → race 测试红；删 finalizeSmallObject → 媒体字段缺失（测试红）

## 片划分

- P1（核心）：small_object 增强 + Snapshot 深拷贝 + 媒体字段访问器
- P2（分组）：聚合下推 + 书橱视图
- P3（版本）：ObjectVersioner 升级机制

## 风险与零回归

- 与 master 重叠部分（small_object.go 已有基础版）→ 需仔细 diff 合并而非覆盖
- pkg/dlcore 相关 diff 全部丢弃（master 已删）
- 行为零回归：默认任务类型无 SmallObjectProvider 时路径不变；版本升级仅对 version < Latest 的对象生效（旧数据迁移路径）
- web 改动遵守 Web UI 硬要求（node --test + Playwright e2e）
