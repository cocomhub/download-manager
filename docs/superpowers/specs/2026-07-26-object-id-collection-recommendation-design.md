# 下载对象 ID 系统、合集与推荐功能设计

> 日期：2026-07-26
> 状态：设计稿
> 关联项目：download-manager

## 1. 背景与目标

### 1.1 当前问题

- `DownloadObject` 没有独立 ID 字段，URL 是唯一标识符
- 所有 API 操作（cancel/retry/reorder）都通过 URL 标识对象
- 前端无法按稳定 ID 精确定位对象
- 播放列表、关联视频等功能行为模糊，缺乏统一的设计
- 不同任务类型间 ID 不相关，需隔离

### 1.2 目标

1. **对象 ID 系统**：每个下载对象有稳定数字 ID，按任务类型隔离
2. **合集系统**：对象间紧密关联关系，支持上一集/下一集导航
3. **推荐系统**：基于 Tag 的推荐，支持多种推荐模式
4. **兼容旧数据**：存量数据逐步补全 ID

## 2. 整体架构

### 2.1 三阶段划分

| 阶段 | 内容 | 核心变更 |
|------|------|---------|
| 阶段一 | 对象 ID 系统 | `DownloadObject.ID` + `Standardizer` 接口 + 标准化服务 |
| 阶段二 | 合集系统 | `collection_id`/`collection_title` + Collection API |
| 阶段三 | 推荐系统 | 扩展 `GET /api/aggregate` 支持 tags/exclude/sort=random |

### 2.2 数据流总览

```
新对象创建路径
  ↓
创建 DownloadObject → 设置 URL/TaskID/Metadata/Extra
  ↓
检查 Task 是否实现 Standardizer
  ↓ 是
Standardize(obj)
  ├─ 提取 ID（从 URL）
  ├─ 设置 collection_id（hanime: 从 playlist 最小 ID）
  └─ 设置 collection_title（hanime: 从 playlist 对应标题）
  ↓
Storage.Update(obj) → MongoDB（含 id 唯一索引）

存量旧数据（定时 Run）
  ↓
每种任务类型取一个 Task 实例
  ↓
查 Storage.Search(MissingID=true) → 获取 ID=0 的对象
  ↓
调用 Standardize(obj) → 补全 ID/collection_id
  ↓
Storage.Update(obj)

用户打开查看器（前端）
  ↓
GET /api/objects/{type}/{id} → 对象详情
GET /api/objects/{type}/{id}/collection → 合集列表（首次，前端缓存）
GET /api/aggregate?types=...&tags=...&tag_mode=...&exclude_ids=...&sort=... → 推荐（可多次）
  ↓
渲染：播放器 + 合集面板（左侧）+ 推荐面板（右侧）
```

## 3. 阶段一：对象 ID 系统

### 3.1 数据模型变更

```go
// model/object.go — DownloadObject 新增字段
ID int64 `json:"id,omitempty" bson:"id,omitempty"`
// 0 = 未分配（兼容旧数据）
// 从 URL 提取，每个任务类型内部独立
// 不同任务类型使用不同 MongoDB collection，ID 互不干扰
```

### 3.2 通用标准化接口

```go
// core/interfaces.go — 新增可选接口
type Standardizer interface {
    // Standardize 对下载对象执行标准化操作
    // modified=true 表示对象被修改，需要持久化
    Standardize(obj *model.DownloadObject) (modified bool, err error)
}
```

各任务类型实现该接口，ID 提取只是第一个标准化操作。未来可扩展：
- 补全缺失的 title
- 填充默认值
- 清理无效 metadata

### 3.3 StorageFilter 扩展

```go
// core/interfaces.go
type StorageFilter struct {
    // ... 原有字段
    MissingID  *bool              // true = 只返回 ID == 0 的对象
    Tags       []string           // 推荐用标签
    TagMode    string             // "any" 或 "all"
    ExcludeIDs []int64            // 排除的对象 ID
}
```

**MongoDB 实现**：
- `MissingID=true` → `$or: [{id: {$exists: false}}, {id: 0}]`
- `Tags` → `extra.tags` 字段匹配
- `ExcludeIDs` → `id: {$nin: [...]}`

### 3.4 各任务类型 ID 提取规则

| 任务类型 | URL 示例 | 提取逻辑 |
|---------|---------|---------|
| hanime | `https://hanime1.me/watch?v=407014` | 解析查询参数 `v` |
| tktube | `https://tktube.com/videos/297910/nhdtb-995c/` | 提取路径段 `/videos/{id}/` |
| vikacg | `https://www.vikacg.com/p/209067` | 提取路径段 `/p/{id}` |
| url_list | 不实现 Standardizer | 跳过 |

### 3.5 标准化服务

```go
// manager/standardization.go
type StandardizationService struct {
    mgr *Manager
}

// Run 扫描存量旧数据，每种任务类型只处理一次
func (s *StandardizationService) Run(ctx context.Context) {
    for _, taskType := range s.mgr.UniqueTaskTypes() {
        task := s.mgr.FirstTaskOfType(taskType)
        std, ok := task.(Standardizer)
        if !ok { continue }

        objects, _ := task.Storage().Search(&StorageQuery{
            Filter: StorageFilter{MissingID: ptr(true)},
            Limit:  0,  // 不限量
        })

        for _, obj := range objects {
            if modified, _ := std.Standardize(obj); modified {
                task.Storage().Update(obj)
            }
        }
    }
}
```

### 3.6 新对象创建时立即标准化

在 `BaseTask` 创建对象后的公共路径中插入：

```go
// 在 RememberRuntimeObject 或 UpdateStatus 首次写入时
if std, ok := any(b).(Standardizer); ok {
    if modified, _ := std.Standardize(obj); modified {
        // ID 已赋值，后续 Storage.Update 写入
    }
}
```

### 3.7 触发时机

| 时机 | 方式 |
|------|------|
| 新对象创建 | 立即执行（在创建路径中） |
| 应用启动 | 一次完整 Run() |
| 定时扫描 | 复用 Manager 现有定时器 |

### 3.8 MongoDB 索引

```go
// 每个 collection 创建时（按任务类型隔离）
{Keys: {"id": 1}, Unique: true, Sparse: true, Name: "id_unique"}
```

`Sparse: true` 确保旧数据（无 `id` 字段）不触发唯一约束冲突。

### 3.9 API 端点

```
GET /api/objects/{type}/{id}
```
- 按任务类型 + ID 精确查找单个对象
- 返回完整 `DownloadObject` JSON
- 404 未找到

## 4. 阶段二：合集系统

### 4.1 数据模型

Metadata 中新增两个键：

| 键 | 类型 | 说明 | 示例 |
|----|------|------|------|
| `collection_id` | string | 合集标识，同一合集共享此值 | `"407013"` |
| `collection_title` | string | 本对象在合集内的标题 | `"EP 1"` |

### 4.2 hanime 合集实现

在 `Standardize` 中实现：

```go
// 1. 从 extra.playlist 或 metadata.playlist 获取播放列表
// playlist 项: {title: "EP 1", url: "https://hanime1.me/watch?v=407014"}

// 2. 按 URL 去重
seen := map[string]bool{}
var unique []playlistItem
for _, item := range playlist {
    if seen[item.url] { continue }
    seen[item.url] = true
    unique = append(unique, item)
}

// 3. 从 URL 提取 ID，找最小 ID 作为合集 ID
minID := int64(0)
for _, item := range unique {
    if id := extractID(item.url); id > 0 {
        if minID == 0 || id < minID { minID = id }
    }
}
if minID > 0 {
    obj.Metadata["collection_id"] = strconv.FormatInt(minID, 10)
}

// 4. 从 playlist 中找本对象 URL 对应的标题
for _, item := range playlist {
    if extractID(item.url) == obj.ID {
        obj.Metadata["collection_title"] = item.title
        break
    }
}
```

### 4.3 tktube / vikacg 合集实现

两个任务类型暂时返回空字符串（`collection_id` 不设置），留待后续实现。

### 4.4 Collection API

```
GET /api/objects/{type}/{id}/collection
```

**逻辑**：
1. 按 `type` + `id` 定位对象
2. 读取 `Metadata["collection_id"]`
3. 为空 → 返回 `{ objects: [], total: 0 }`
4. 在同一 Storage 中查询 `metadata.collection_id == collectionID` 的所有对象
5. 按 `collection_title` 排序
6. 返回完整列表

**响应**：
```json
{
  "objects": [
    {"id": 407013, "title": "EP 0", "collection_title": "EP 0", ...},
    {"id": 407014, "title": "EP 1", "collection_title": "EP 1", ...},
    {"id": 407015, "title": "EP 2", "collection_title": "EP 2", ...}
  ],
  "total": 3
}
```

### 4.5 前端交互

- 首次打开查看器时调用一次 Collection API
- 收到完整列表后，根据当前对象 ID 在列表中定位
- 本地维护 `currentIndex`，切换时更新索引
- 用户切换合集对象时，调用 `GET /api/objects/{type}/{id}` 更新详情
- 合集列表和推荐列表保持不变

### 4.6 MongoDB 索引

```go
{Keys: {"metadata.collection_id": 1, "metadata.collection_title": 1}, Name: "collection_order"}
```

## 5. 阶段三：推荐系统

### 5.1 设计思路

不创建专用推荐端点，而是**扩展现有 `GET /api/aggregate`**，使其支持基于 tag 的推荐能力。这样更通用，前端可以灵活组合参数。

### 5.2 API 扩展

```
GET /api/aggregate
```

**新增查询参数**：

| 参数 | 类型 | 默认 | 说明 |
|---------|------|------|------|
| `tags` | string | — | 标签列表，逗号分隔，如 `"tag1,tag2"` |
| `tag_mode` | string | `any` | `any`=满足任一标签, `all`=满足全部标签 |
| `exclude_ids` | string | — | 排除的对象 ID，逗号分隔，如 `"407014,407015"` |
| `sort` | string | — | **新增 `random`、`tag_match_desc` 值** |

**sort 值完整列表**：

| 值 | 说明 |
|----|------|
| `date_desc` | 按日期降序（默认） |
| `date_asc` | 按日期升序 |
| `name_asc` | 按标题升序 |
| `duration_desc` | 按时长降序 |
| `random` | 随机排序 |
| `tag_match_desc` | 按匹配标签数降序（推荐场景） |

### 5.3 推荐场景调用示例

```
# 满足任意 tag，排除当前对象，随机排序
GET /api/aggregate?types=hanime&tags=tag1,tag2&tag_mode=any&exclude_ids=407014&sort=random&limit=20

# 满足全部 tag，按匹配度排序
GET /api/aggregate?types=hanime&tags=tag1,tag2&tag_mode=all&exclude_ids=407014&sort=tag_match_desc&limit=20

# 用户手动选择部分 tag
GET /api/aggregate?types=hanime&tags=tag1&tag_mode=any&exclude_ids=407014,407015&sort=date_desc&limit=20
```

### 5.4 前端交互

- 查看器打开时，从对象 `Extra.Tags` 获取标签列表
- 在推荐面板中展示标签选择器（默认全选）
- 首次请求：`GET /api/aggregate?types=hanime&tags=tag1,tag2&tag_mode=any&exclude_ids=407014&sort=random&limit=20`
- 用户切换推荐模式（any/all/随机/排序）→ 重新请求
- 用户取消勾选某些 tag → 重新请求
- 切换合集对象时，推荐列表保持不变（不重新请求）

## 6. 兼容性与迁移

### 6.1 旧数据兼容

- 新增 `ID int64` 字段，默认值 0 表示未分配
- `omitempty` 确保旧数据 JSON/BSON 中不包含 `id` 字段
- 启动时和定时扫描中，`StandardizationService.Run()` 补全存量数据
- 新创建的对象立即有 ID

### 6.2 任务类型隔离

- 不同任务类型使用不同的 MongoDB collection
- ID 唯一索引在各自 collection 内独立，不会跨类型冲突
- 查询时不需要 `task_type` 过滤条件

### 6.3 向后兼容

- 所有现有 API 端点不变
- 新增端点不影响旧 UI
- 旧前端不调用新 API 时，功能不受影响

## 7. 未在此设计中涵盖的内容

- 合集 `collection_title` 在 tktube/vikacg 中的具体实现
- 推荐系统的具体排序算法（`tag_match_desc` 的计分公式）
- 前端 UI 的具体实现细节（标签选择器样式、合集面板布局等）
- 性能优化（大数据量下的推荐查询缓存等）