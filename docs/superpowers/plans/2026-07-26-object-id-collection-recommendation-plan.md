# 下载对象 ID 系统、合集与推荐功能 — 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 为 DownloadObject 添加数字 ID 字段，实现基于 URL 提取 ID 的标准化接口，提供合集系统（上一集/下一集导航）和推荐系统（基于 Tag 的推荐）。

**架构：** 三阶段增量推进：(1) 对象 ID 系统 — `Standardizer` 可选接口 + `DownloadObject.ID` + `StandardizationService`；(2) 合集系统 — `collection_id`/`collection_title` metadata + Collection API；(3) 推荐系统 — 扩展 `GET /api/aggregate` 支持 tags/exclude/random sort。

**技术栈：** Go 1.26, MongoDB, gorilla/mux, Vue 3 前端

---

## 文件结构总览

### 阶段一：对象 ID 系统

| 文件 | 操作 | 职责 |
|------|------|------|
| `model/object.go` | 修改 | 新增 `ID int64` 字段 |
| `core/interfaces.go` | 修改 | 新增 `Standardizer` 接口，扩展 `StorageFilter` |
| `storage/query.go` | 修改 | 新增 `MissingID`、`Tags`、`TagMode`、`ExcludeIDs` 过滤支持 |
| `storage/mongo_storage.go` | 修改 | 新增 `MissingID` 过滤、`id` 唯一索引、`random`/`tag_match_desc` 排序 |
| `task/base_task.go` | 修改 | 新增 `self` 字段 + `SetSelf` + `RememberRuntimeObject` 中调用 `Standardizer` |
| `task/option.go` | 修改 | 新增 `self` 相关（可选） |
| `task/hanime/task.go` | 修改 | 实现 `Standardizer` 接口（ID 提取） |
| `task/tktube/task.go` | 修改 | 实现 `Standardizer` 接口（ID 提取） |
| `task/vikacg/task.go` | 修改 | 实现 `Standardizer` 接口（ID 提取） |
| `manager/standardization.go` | **创建** | `StandardizationService` 定义 |
| `manager/manager.go` | 修改 | 新增 `UniqueTaskTypes()`、`FirstTaskOfType()`、启动时调用 `StandardizationService.Run()` |
| `manager/tasks.go` | 修改 | 新增 `GetObjectByTypeAndID()` |
| `api/server.go` | 修改 | 新增路由 `/api/objects/{type}/{id}` |
| `api/server_objects.go` | **创建** | 对象 API handler（`getObjectByTypeAndID`） |

### 阶段二：合集系统

| 文件 | 操作 | 职责 |
|------|------|------|
| `task/hanime/task.go` | 修改 | `Standardize` 中增加 `collection_id`/`collection_title` 设置 |
| `manager/aggregate.go` | 修改 | 新增 `GetObjectsByCollectionID()` |
| `manager/tasks.go` | 修改 | 新增 `GetObjectsByCollectionID()` |
| `api/server.go` | 修改 | 新增路由 `/api/objects/{type}/{id}/collection` |
| `api/server_objects.go` | 修改 | 新增 `getCollection` handler |
| `storage/mongo_storage.go` | 修改 | 新增 `collection_order` 索引 |

### 阶段三：推荐系统

| 文件 | 操作 | 职责 |
|------|------|------|
| `storage/query.go` | 修改 | 实现 `Tags`/`TagMode`/`ExcludeIDs` 过滤和 `random`/`tag_match_desc` 排序 |
| `storage/mongo_storage.go` | 修改 | 实现 `Tags`/`TagMode`/`ExcludeIDs` 的 MongoDB 查询 |
| `manager/aggregation_service.go` | 修改 | 传递 tags/exclude_ids/sort 参数到查询 |
| `api/server_metrics.go` | 修改 | 解析 `tags`/`tag_mode`/`exclude_ids` 查询参数 |
| `core/interfaces.go` | 修改 | 确认 `Sort` 支持 `random`/`tag_match_desc` |

---

## 任务

### 任务 1：模型变更 — ID 字段 + Standardizer 接口 + StorageFilter 扩展

**文件：**
- 修改：`model/object.go`
- 修改：`core/interfaces.go`

- [ ] **步骤 1：在 `DownloadObject` 中新增 `ID` 字段**

```go
// model/object.go — 在 URL 字段后添加
ID int64 `json:"id,omitempty" bson:"id,omitempty"`
// 0 = 未分配（兼容旧数据）
// 从 URL 提取，每个任务类型内部独立
```

在 `GetProgress()` 前添加 `GetID()` / `SetID()` 方法：

```go
func (o *DownloadObject) GetID() int64 {
    if o == nil {
        return 0
    }
    o.mu.RLock()
    defer o.mu.RUnlock()
    return o.ID
}

func (o *DownloadObject) SetID(id int64) {
    if o == nil {
        return
    }
    o.mu.Lock()
    defer o.mu.Unlock()
    o.ID = id
}
```

- [ ] **步骤 2：在 `core/interfaces.go` 中添加 `Standardizer` 接口**

```go
// core/interfaces.go — 在 SharedRegistry 后添加
// Standardizer 是可选接口，任务类型实现它以对下载对象执行标准化操作
// （如提取 ID、补全元数据等）。
type Standardizer interface {
    // Standardize 对下载对象执行标准化操作。
    // modified=true 表示对象被修改，需要持久化。
    Standardize(obj *model.DownloadObject) (modified bool, err error)
}
```

- [ ] **步骤 3：扩展 `StorageFilter`**

```go
// core/interfaces.go — 在 StorageFilter 中添加
type StorageFilter struct {
    TaskIDs  []string
    URLs     []string
    Statuses []string
    Metadata map[string]string
    Search   string
    // 新增字段
    MissingID  *bool     // true = 只返回 ID == 0 的对象
    Tags       []string  // 推荐用标签（阶段三）
    TagMode    string    // "any" 或 "all"（阶段三）
    ExcludeIDs []int64   // 排除的对象 ID（阶段三）
}
```

- [ ] **步骤 4：运行测试验证编译通过**

```bash
go build ./...
go test ./model/... ./core/...
```

- [ ] **步骤 5：Commit**

```bash
git add model/object.go core/interfaces.go
git commit -m "feat: add ID field to DownloadObject, Standardizer interface, extend StorageFilter"
```

---

### 任务 2：存储层查询扩展 — MissingID 过滤 + MongoDB id 索引

**文件：**
- 修改：`storage/query.go`
- 修改：`storage/mongo_storage.go`

- [ ] **步骤 1：在 `storage/query.go` 的 `matchesFilterFields` 中添加 `MissingID` 过滤**

```go
// storage/query.go — 在 matchesFilterFields 函数末尾，return true 前添加
if filter.MissingID != nil {
    objID := obj.GetID()
    if *filter.MissingID && objID != 0 {
        return false
    }
    if !*filter.MissingID && objID == 0 {
        return false
    }
}
```

注意：`obj.GetID()` 需要加锁（`ID` 字段受 `mu` 保护）。修改如下：

```go
func matchesFilterFields(obj *model.DownloadObject, filter core.StorageFilter) bool {
    // ... 原有代码 ...
    if filter.MissingID != nil {
        obj.RLock()
        id := obj.ID
        obj.RUnlock()
        if *filter.MissingID && id != 0 {
            return false
        }
        if !*filter.MissingID && id == 0 {
            return false
        }
    }
    return true
}
```

- [ ] **步骤 2：在 `storage/mongo_storage.go` 的 `buildMongoFilter` 中添加 `MissingID` 过滤**

```go
// storage/mongo_storage.go — buildMongoFilter 函数中，在 filter["$or"] 之前添加
if query.Filter.MissingID != nil {
    if *query.Filter.MissingID {
        filter["$or"] = bson.A{
            bson.M{"id": bson.M{"$exists": false}},
            bson.M{"id": 0},
        }
    } else {
        // 只在 id 不存在或为 0 时添加 $or，否则什么也不加
        // 注意：这里不需要处理，因为 NormalMongoQuery 会处理
    }
}
```

注意：MongoDB 的 `$or` 可能与 `Search` 的 `$or` 冲突。需要合并：

```go
func buildMongoFilter(query *core.StorageQuery) bson.M {
    filter := bson.M{}
    if query == nil {
        return filter
    }
    // ... 原有 TaskIDs, URLs, Statuses, Metadata 过滤 ...

    var orConditions []bson.M

    // MissingID 过滤
    if query.Filter.MissingID != nil {
        if *query.Filter.MissingID {
            orConditions = append(orConditions,
                bson.M{"id": bson.M{"$exists": false}},
                bson.M{"id": 0},
            )
        }
    }

    // Search 过滤
    if query.Filter.Search != "" {
        pattern := regexp.QuoteMeta(query.Filter.Search)
        orConditions = append(orConditions,
            bson.M{"url": bson.M{opRegex: pattern, opOptions: "i"}},
            bson.M{fieldMetadataTitle: bson.M{opRegex: pattern, opOptions: "i"}},
            bson.M{"extra.tags": bson.M{opRegex: pattern, opOptions: "i"}},
        )
    }

    if len(orConditions) > 0 {
        filter["$or"] = orConditions
    }
    return filter
}
```

- [ ] **步骤 3：在 `ensureIndexes` 中添加 `id` 唯一索引**

```go
// storage/mongo_storage.go — ensureIndexes 的 models 数组中添加
{
    Keys:    bson.D{{Key: "id", Value: 1}},
    Options: options.Index().SetUnique(true).SetSparse(true).SetName("id_unique"),
},
```

- [ ] **步骤 4：在 `mongoSortField` 中添加 `random` 和 `tag_match_desc` 支持（阶段三预埋）**

```go
// storage/mongo_storage.go — mongoSortField 中 default 前添加
case "random":
    return "$sample"  // 使用 $sample 聚合，但 Find 不支持，需要特殊处理
case "tag_match_desc":
    return ""  // 内存排序，MongoDB 不直接支持
```

注意：`random` 在 MongoDB 中需要 `$sample` 聚合阶段，但 `Find` 不支持。有两种方案：
- 方案 A：对 `random` 排序，用 `$sample` 做聚合管道查询
- 方案 B：获取结果后内存随机打乱

我们先实现方案 B（简单），后续优化：

```go
func mongoSortField(field string) string {
    switch field {
    case "date":
        return "metadata.date"
    case "name":
        return fieldMetadataTitle
    case "duration":
        return "metadata.duration"
    case "status":
        return "status"
    case "url":
        return "url"
    case "random":
        return ""   // 内存随机
    case "tag_match_desc":
        return ""   // 内存排序
    default:
        return ""
    }
}
```

- [ ] **步骤 5：运行测试验证**

```bash
go test ./storage/... -run TestQuery
go build ./...
```

- [ ] **步骤 6：Commit**

```bash
git add storage/query.go storage/mongo_storage.go
git commit -m "feat: add MissingID filter, id unique index, random/tag_match_desc sort support"
```

---

### 任务 3：BaseTask 标准化钩子 — self 引用 + RememberRuntimeObject 集成

**文件：**
- 修改：`task/base_task.go`
- 修改：`task/option.go`

- [ ] **步骤 1：在 `BaseTask` 结构体中添加 `self` 字段**

```go
// task/base_task.go — BaseTask 结构体中，在 scrapeDriver 后添加
self any  // embedding task reference, set by embedding task's constructor
```

- [ ] **步骤 2：添加 `SetSelf` 方法**

```go
// task/base_task.go — 在 SetHeaders 后添加
func (b *BaseTask) SetSelf(self any) {
    b.self = self
}
```

- [ ] **步骤 3：在 `RememberRuntimeObject` 中插入标准化调用**

```go
// task/base_task.go — RememberRuntimeObject 函数中，在 upsert 之前添加
func (b *BaseTask) RememberRuntimeObject(obj *model.DownloadObject, lock bool) {
    if obj == nil {
        return
    }
    if lock {
        b.mu.Lock()
        defer b.mu.Unlock()
    }

    // Standardize: 如果 embedding task 实现了 Standardizer，调用它
    if std, ok := b.self.(core.Standardizer); ok {
        if modified, err := std.Standardize(obj); err != nil {
            b.logger.Error("Failed to standardize object", logutil.LogKeyURL, obj.URL, logutil.LogKeyError, err)
        } else if modified {
            b.logger.Debug("Object standardized", logutil.LogKeyURL, obj.URL, "id", obj.ID)
        }
    }

    b.objects = upsertRuntimeObject(b.objects, obj)
    b.knownURLs = rememberRuntimeURLs(b.objects)
}
```

- [ ] **步骤 4：运行测试验证**

```go build ./task/...
go test ./task/... -run TestBaseTask -count=1
```

- [ ] **步骤 5：Commit**

```bash
git add task/base_task.go
git commit -m "feat: add self reference to BaseTask, integrate Standardizer in RememberRuntimeObject"
```

---

### 任务 4：各任务类型实现 Standardizer 接口

**文件：**
- 修改：`task/hanime/task.go`
- 修改：`task/tktube/task.go`
- 修改：`task/vikacg/task.go`

- [ ] **步骤 1：hanime 实现 Standardizer — ID 提取**

```go
// task/hanime/task.go — 在 Type() 方法后添加
func (t *Task) Standardize(obj *model.DownloadObject) (bool, error) {
    modified := false

    // 提取 ID: https://hanime1.me/watch?v=407014
    if obj.ID == 0 {
        if vid := extractVideoIDFromURL(obj.URL); vid > 0 {
            obj.ID = vid
            modified = true
        }
    }

    return modified, nil
}

// 需要提取 extractVideoIDFromURL 为独立函数（当前内联在 parseHanimeTitle 中）
// 在 task/hanime/task.go 中查找 extractVideoIDFromURL 并确保它可从 Standardize 调用
```

检查 `extractVideoIDFromURL` 函数是否存在：

```go
// 若不存在，添加：
func extractVideoIDFromURL(pageURL string) int64 {
    u, err := url.Parse(pageURL)
    if err != nil {
        return 0
    }
    v := u.Query().Get("v")
    if v == "" {
        return 0
    }
    id, err := strconv.ParseInt(v, 10, 64)
    if err != nil {
        return 0
    }
    return id
}
```

注意：`extractVideoIDFromURL` 可能已存在于 `task/hanime/task.go` 中（用于标题格式化），确认其签名并复用。

- [ ] **步骤 2：在 hanime 的 `NewTask` 中调用 `SetSelf`**

```go
// task/hanime/task.go — NewTask 函数末尾，return t, nil 之前
bt.SetSelf(t)
```

- [ ] **步骤 3：tktube 实现 Standardizer — ID 提取**

```go
// task/tktube/task.go — 在 Type() 方法后添加
func (t *Task) Standardize(obj *model.DownloadObject) (bool, error) {
    modified := false

    // 提取 ID: https://tktube.com/videos/297910/nhdtb-995c/ → 297910
    if obj.ID == 0 {
        if id := extractTktubeVideoID(obj.URL); id > 0 {
            obj.ID = id
            modified = true
        }
    }

    return modified, nil
}

// 辅助函数
func extractTktubeVideoID(rawURL string) int64 {
    // /videos/297910/nhdtb-995c/
    u, err := url.Parse(rawURL)
    if err != nil {
        return 0
    }
    parts := strings.Split(strings.Trim(u.Path, "/"), "/")
    for i, p := range parts {
        if p == "videos" && i+1 < len(parts) {
            id, err := strconv.ParseInt(parts[i+1], 10, 64)
            if err != nil {
                return 0
            }
            return id
        }
    }
    return 0
}
```

- [ ] **步骤 4：在 tktube 的 `NewTask` 中调用 `SetSelf`**

```go
// task/tktube/task.go — NewTask 中，bt 创建后
bt.SetSelf(t)
```

- [ ] **步骤 5：vikacg 实现 Standardizer — ID 提取**

```go
// task/vikacg/task.go — 在 Type() 方法后添加
func (t *Task) Standardize(obj *model.DownloadObject) (bool, error) {
    modified := false

    // 提取 ID: https://www.vikacg.com/p/209067 → 209067
    if obj.ID == 0 {
        if id := extractVikacgID(obj.URL); id > 0 {
            obj.ID = id
            modified = true
        }
    }

    return modified, nil
}

func extractVikacgID(rawURL string) int64 {
    u, err := url.Parse(rawURL)
    if err != nil {
        return 0
    }
    parts := strings.Split(strings.Trim(u.Path, "/"), "/")
    for i, p := range parts {
        if p == "p" && i+1 < len(parts) {
            id, err := strconv.ParseInt(parts[i+1], 10, 64)
            if err != nil {
                return 0
            }
            return id
        }
    }
    return 0
}
```

- [ ] **步骤 6：在 vikacg 的 `NewTask` 中调用 `SetSelf`**

```go
// task/vikacg/task.go — NewTask 中
bt.SetSelf(t)
```

- [ ] **步骤 7：运行测试验证**

```bash
go build ./...
go test ./task/hanime/... ./task/tktube/... ./task/vikacg/... -count=1
```

- [ ] **步骤 8：Commit**

```bash
git add task/hanime/task.go task/tktube/task.go task/vikacg/task.go
git commit -m "feat: implement Standardizer for hanime, tktube, vikacg tasks"
```

---

### 任务 5：StandardizationService — 定时扫描存量数据

**文件：**
- 创建：`manager/standardization.go`
- 修改：`manager/manager.go`
- 修改：`manager/tasks.go`

- [ ] **步骤 1：创建 `manager/standardization.go`**

```go
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
    "context"
    "log/slog"

    "github.com/cocomhub/download-manager/core"
    "github.com/cocomhub/download-manager/model"
    "github.com/cocomhub/download-manager/pkg/logutil"
)

// StandardizationService 扫描存量旧数据，执行标准化操作（如 ID 提取）。
// 每种任务类型只处理一次，与任务实例数量无关。
type StandardizationService struct {
    mgr *Manager
}

func NewStandardizationService(mgr *Manager) *StandardizationService {
    return &StandardizationService{mgr: mgr}
}

// Run 执行一次标准化扫描。
// 遍历所有任务类型，取一个 Task 实例，检查是否实现 Standardizer，
// 对 MissingID=true 的对象执行标准化。
func (s *StandardizationService) Run(ctx context.Context) {
    for _, taskType := range s.mgr.UniqueTaskTypes() {
        task := s.mgr.FirstTaskOfType(taskType)
        if task == nil {
            continue
        }
        std, ok := task.(core.Standardizer)
        if !ok {
            // 该任务类型不支持标准化
            slog.Debug("Standardization: task type does not implement Standardizer",
                logutil.LogKeyTaskID, task.ID(), "task_type", taskType)
            continue
        }

        st := task.Storage()
        if st == nil {
            continue
        }

        missingID := true
        objects, err := st.Search(&core.StorageQuery{
            Filter: core.StorageFilter{
                MissingID: &missingID,
            },
            Limit: 0, // 不限量
        })
        if err != nil {
            slog.Error("Standardization: search failed", logutil.LogKeyTaskID, task.ID(),
                "task_type", taskType, logutil.LogKeyError, err)
            continue
        }

        if len(objects) == 0 {
            continue
        }

        count := 0
        for _, obj := range objects {
            if modified, err := std.Standardize(obj); err != nil {
                slog.Error("Standardization: failed", logutil.LogKeyTaskID, task.ID(),
                    logutil.LogKeyURL, obj.URL, logutil.LogKeyError, err)
            } else if modified {
                if err := st.Update(obj); err != nil {
                    slog.Error("Standardization: update failed", logutil.LogKeyTaskID, task.ID(),
                        logutil.LogKeyURL, obj.URL, logutil.LogKeyError, err)
                } else {
                    count++
                }
            }
        }

        slog.Info("Standardization completed", "task_type", taskType, "processed", count)
    }
}
```

- [ ] **步骤 2：在 `manager/tasks.go` 中添加 `UniqueTaskTypes` 和 `FirstTaskOfType` 方法**

```go
// manager/tasks.go — 在 getTask 后添加

// UniqueTaskTypes 返回所有已注册任务的不重复类型列表。
func (m *Manager) UniqueTaskTypes() []string {
    seen := make(map[string]bool)
    var types []string
    m.tasks.Range(func(_, value any) bool {
        t := value.(core.Task)
        tt := t.Type()
        if !seen[tt] {
            seen[tt] = true
            types = append(types, tt)
        }
        return true
    })
    return types
}

// FirstTaskOfType 返回指定类型的第一个 Task 实例。
func (m *Manager) FirstTaskOfType(taskType string) core.Task {
    var found core.Task
    m.tasks.Range(func(_, value any) bool {
        t := value.(core.Task)
        if t.Type() == taskType {
            found = t
            return false
        }
        return true
    })
    return found
}
```

- [ ] **步骤 3：在 `Manager.Start()` 中调用标准化服务**

```go
// manager/manager.go — 在 Start() 中找到 close(initializedCh) 之前的初始化位置，添加：
// 启动标准化服务（异步，不阻塞启动）
go func() {
    stdSvc := NewStandardizationService(m)
    // 启动时执行一次完整扫描
    stdSvc.Run(context.Background())
    slog.Info("Initial standardization complete")
}()
```

注意：`Start()` 末尾是 `for { select {} }`，`close(initializedCh)` 在 for 之前。在 close 后启动标准化 goroutine。

- [ ] **步骤 4：运行测试验证**

```bash
go build ./...
go test ./manager/... -run TestManager -count=1
```

- [ ] **步骤 5：Commit**

```bash
git add manager/standardization.go manager/tasks.go manager/manager.go
git commit -m "feat: add StandardizationService with UniqueTaskTypes/FirstTaskOfType support"
```

---

### 任务 6：API 单对象查询 — GET /api/objects/{type}/{id}

**文件：**
- 创建：`api/server_objects.go`
- 修改：`api/server.go`
- 修改：`manager/tasks.go`

- [ ] **步骤 1：在 `manager/tasks.go` 中添加 `GetObjectByTypeAndID`**

```go
// manager/tasks.go — 在 FirstTaskOfType 后添加

// GetObjectByTypeAndID 按任务类型和数字 ID 查找单个下载对象。
// 返回 nil 表示未找到。
func (m *Manager) GetObjectByTypeAndID(taskType string, id int64) (*model.DownloadObject, error) {
    task := m.FirstTaskOfType(taskType)
    if task == nil {
        return nil, fmt.Errorf("%w: task type %q not found", errTaskNotFound, taskType)
    }
    st := task.Storage()
    if st == nil {
        return nil, nil
    }
    objects, err := st.Search(&core.StorageQuery{
        Filter: core.StorageFilter{
            TaskIDs: []string{task.ID()},
        },
        Limit: 0, // 不限量，内存过滤
    })
    if err != nil {
        return nil, err
    }
    for _, obj := range objects {
        if obj.GetID() == id {
            return obj, nil
        }
    }
    return nil, nil
}
```

- [ ] **步骤 2：创建 `api/server_objects.go`**

```go
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
    "encoding/json"
    "fmt"
    "net/http"
    "strconv"

    "github.com/gorilla/mux"
)

// getObjectByTypeAndID 返回单个下载对象详情。
// GET /api/objects/{type}/{id}
func (s *Server) getObjectByTypeAndID(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    taskType := vars["type"]
    idStr := vars["id"]

    id, err := strconv.ParseInt(idStr, 10, 64)
    if err != nil || id <= 0 {
        writeJSONError(w, http.StatusBadRequest, "invalid_id", "id must be a positive integer")
        return
    }

    obj, err := s.mgr.GetObjectByTypeAndID(taskType, id)
    if err != nil {
        writeJSONError(w, http.StatusNotFound, "not_found",
            fmt.Sprintf("object not found: %v", err))
        return
    }
    if obj == nil {
        writeJSONError(w, http.StatusNotFound, "not_found", "object not found")
        return
    }

    // 确保 task_type metadata 存在
    obj.EnsureTaskType(taskType)

    w.Header().Set(hdrContentType, "application/json")
    json.NewEncoder(w).Encode(obj)
}
```

- [ ] **步骤 3：在 `api/server.go` 的 `Router()` 中添加路由**

```go
// api/server.go — 在 /api/groups/{group}/objects 路由后添加
r.HandleFunc("/api/objects/{type}/{id}", s.getObjectByTypeAndID).Methods("GET")
```

- [ ] **步骤 4：运行测试验证**

```bash
go build ./...
go test ./api/... -run TestAPI -count=1
```

- [ ] **步骤 5：Commit**

```bash
git add api/server_objects.go api/server.go manager/tasks.go
git commit -m "feat: add GET /api/objects/{type}/{id} single object lookup API"
```

---

### 任务 7：hanime 合集逻辑 — Standardize 中设置 collection_id/collection_title

**文件：**
- 修改：`task/hanime/task.go`

- [ ] **步骤 1：在 `Standardize` 中添加合集逻辑**

```go
// task/hanime/task.go — Standardize 方法中，在 ID 提取后添加

// 设置合集信息（从 playlist 获取）
if obj.Metadata == nil {
    obj.Metadata = make(map[string]string)
}

// 从 Extra 或 Metadata 读取 playlist
playlist := getPlaylistFromObject(obj)
if len(playlist) > 0 && obj.Metadata["collection_id"] == "" {
    // 按 URL 去重
    seen := make(map[string]bool)
    var unique []hanimeItem
    for _, item := range playlist {
        if seen[item.href] {
            continue
        }
        seen[item.href] = true
        unique = append(unique, item)
    }

    // 找最小 ID 作为合集 ID
    var minID int64
    for _, item := range unique {
        if id := extractVideoIDFromURL(item.href); id > 0 {
            if minID == 0 || id < minID {
                minID = id
            }
        }
    }
    if minID > 0 {
        obj.Metadata["collection_id"] = strconv.FormatInt(minID, 10)
        modified = true
    }
}

// 设置本对象的合集标题
if obj.Metadata["collection_title"] == "" {
    for _, item := range playlist {
        if extractVideoIDFromURL(item.href) == obj.ID {
            obj.Metadata["collection_title"] = item.title
            modified = true
            break
        }
    }
}
```

- [ ] **步骤 2：添加 `getPlaylistFromObject` 辅助函数**

```go
// task/hanime/task.go — 在 Standardize 附近添加
func getPlaylistFromObject(obj *model.DownloadObject) []hanimeItem {
    // 尝试从 Extra.playlist 读取
    if obj.Extra != nil {
        if raw, ok := obj.Extra["playlist"]; ok {
            if items, ok := raw.([]hanimeItem); ok {
                return items
            }
            // 尝试从 []any 转换
            if list, ok := raw.([]any); ok {
                result := make([]hanimeItem, 0, len(list))
                for _, item := range list {
                    if m, ok := item.(map[string]any); ok {
                        href, _ := m["url"].(string)
                        title, _ := m["title"].(string)
                        thumb, _ := m["thumbnail"].(string)
                        result = append(result, hanimeItem{href: href, title: title, thumbURL: thumb})
                    }
                }
                return result
            }
        }
    }
    // 尝试从 Metadata.playlist 读取
    if obj.Metadata != nil {
        if raw, ok := obj.Metadata["playlist"]; ok {
            // playlist 在 metadata 中是 JSON 字符串
            var items []hanimeItem
            if err := json.Unmarshal([]byte(raw), &items); err == nil {
                return items
            }
        }
    }
    return nil
}
```

注意：需要导入 `encoding/json`。

- [ ] **步骤 3：运行测试验证**

```bash
go build ./task/hanime/...
go test ./task/hanime/... -count=1
```

- [ ] **步骤 4：Commit**

```bash
git add task/hanime/task.go
git commit -m "feat: add collection_id/collection_title extraction in hanime Standardize"
```

---

### 任务 8：Collection API — GET /api/objects/{type}/{id}/collection

**文件：**
- 修改：`api/server_objects.go`
- 修改：`api/server.go`
- 修改：`manager/tasks.go`

- [ ] **步骤 1：在 `manager/tasks.go` 中添加 `GetCollectionByID`**

```go
// manager/tasks.go — 在 GetObjectByTypeAndID 后添加

// GetCollectionByID 返回指定对象所在合集的所有对象。
// 按 collection_title 排序。
func (m *Manager) GetCollectionByID(taskType string, id int64) ([]*model.DownloadObject, error) {
    // 先查找对象
    obj, err := m.GetObjectByTypeAndID(taskType, id)
    if err != nil {
        return nil, err
    }
    if obj == nil {
        return nil, nil
    }

    // 读取 collection_id
    obj.RLock()
    collectionID := obj.Metadata["collection_id"]
    obj.RUnlock()

    if collectionID == "" {
        return []*model.DownloadObject{}, nil
    }

    // 查询同一 collection 的所有对象
    task := m.FirstTaskOfType(taskType)
    if task == nil {
        return nil, fmt.Errorf("%w: task type %q not found", errTaskNotFound, taskType)
    }
    st := task.Storage()
    if st == nil {
        return nil, nil
    }

    // 使用 metadata 精确匹配
    objects, err := st.Search(&core.StorageQuery{
        Filter: core.StorageFilter{
            TaskIDs:  []string{task.ID()},
            Metadata: map[string]string{"collection_id": collectionID},
        },
        Limit: 0, // 不限量
    })
    if err != nil {
        return nil, err
    }

    // 按 collection_title 排序
    sort.Slice(objects, func(i, j int) bool {
        objects[i].RLock()
        objects[j].RLock()
        ti := objects[i].Metadata["collection_title"]
        tj := objects[j].Metadata["collection_title"]
        objects[i].RUnlock()
        objects[j].RUnlock()
        return ti < tj
    })

    return objects, nil
}
```

需要导入 `sort` 包。

- [ ] **步骤 2：在 `api/server_objects.go` 中添加 `getCollection` handler**

```go
// api/server_objects.go — 在 getObjectByTypeAndID 后添加

// getCollection 返回指定对象所在合集的所有对象。
// GET /api/objects/{type}/{id}/collection
func (s *Server) getCollection(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    taskType := vars["type"]
    idStr := vars["id"]

    id, err := strconv.ParseInt(idStr, 10, 64)
    if err != nil || id <= 0 {
        writeJSONError(w, http.StatusBadRequest, "invalid_id", "id must be a positive integer")
        return
    }

    objects, err := s.mgr.GetCollectionByID(taskType, id)
    if err != nil {
        writeJSONError(w, http.StatusNotFound, "not_found",
            fmt.Sprintf("collection not found: %v", err))
        return
    }
    if objects == nil {
        writeJSONError(w, http.StatusNotFound, "not_found", "object not found")
        return
    }

    // 确保 task_type metadata
    for _, o := range objects {
        o.EnsureTaskType(taskType)
    }

    w.Header().Set(hdrContentType, "application/json")
    json.NewEncoder(w).Encode(map[string]any{
        "objects": objects,
        "total":   len(objects),
    })
}
```

- [ ] **步骤 3：在 `api/server.go` 中添加路由**

```go
// api/server.go — 在 /api/objects/{type}/{id} 路由后添加
r.HandleFunc("/api/objects/{type}/{id}/collection", s.getCollection).Methods("GET")
```

- [ ] **步骤 4：运行测试验证**

```bash
go build ./...
go test ./api/... -count=1
```

- [ ] **步骤 5：Commit**

```bash
git add api/server_objects.go api/server.go manager/tasks.go
git commit -m "feat: add GET /api/objects/{type}/{id}/collection API"
```

---

### 任务 9：MongoDB collection 索引（合集查询优化）

**文件：**
- 修改：`storage/mongo_storage.go`

- [ ] **步骤 1：在 `ensureIndexes` 中添加合集索引**

```go
// storage/mongo_storage.go — ensureIndexes 的 models 数组中添加
{
    Keys:    bson.D{{Key: "metadata.collection_id", Value: 1}, {Key: "metadata.collection_title", Value: 1}},
    Options: options.Index().SetName("collection_order"),
},
```

- [ ] **步骤 2：Commit**

```bash
git add storage/mongo_storage.go
git commit -m "feat: add collection order index for MongoDB"
```

---

### 任务 10：扩展 aggregate API 支持 tags、exclude_ids、random sort

**文件：**
- 修改：`api/server_metrics.go`
- 修改：`manager/aggregation_service.go`
- 修改：`manager/aggregate.go`
- 修改：`storage/query.go`（tags 过滤、random 排序）
- 修改：`storage/mongo_storage.go`（tags 过滤）

- [ ] **步骤 1：在 `api/server_metrics.go` 的 `aggregateObjects` 中解析新参数**

```go
// api/server_metrics.go — aggregateObjects 函数中，在 types 解析后添加
tags := r.URL.Query().Get("tags")
tagMode := r.URL.Query().Get("tag_mode")
if tagMode == "" {
    tagMode = "any"
}
excludeIDsStr := r.URL.Query().Get("exclude_ids")
var excludeIDs []int64
if excludeIDsStr != "" {
    for _, s := range strings.Split(excludeIDsStr, ",") {
        s = strings.TrimSpace(s)
        if id, err := strconv.ParseInt(s, 10, 64); err == nil {
            excludeIDs = append(excludeIDs, id)
        }
    }
}
```

- [ ] **步骤 2：将新参数传递给 `AggregateObjects`**

```go
// 修改调用处
if groupBy == "content" {
    res, err = s.mgr.AggregateByContent(page, limit, search, sortBy, status, types)
} else {
    // 传递 tags/tagMode/excludeIDs
    res, err = s.mgr.AggregateObjects(page, limit, search, sortBy, status, types)
    // 需要扩展 AggregateObjects 签名
}
```

扩展 `AggregateObjects` 签名比较麻烦（改了接口）。更好的方式：在 `buildBaseQuery` 中组合所有过滤条件。

```go
// 修改为：将 tags/tagMode/excludeIDs 注入到查询中
func buildBaseQuery(search, status string, tags []string, tagMode string, excludeIDs []int64) *core.StorageQuery {
    q := &core.StorageQuery{
        Filter: core.StorageFilter{
            Search:     search,
            Tags:       tags,
            TagMode:    tagMode,
            ExcludeIDs: excludeIDs,
        },
    }
    if status != "" && status != "all" {
        q.Filter.Statuses = []string{status}
    }
    return q
}
```

- [ ] **步骤 3：在 `storage/query.go` 的 `matchesFilterFields` 中添加 Tags/ExcludeIDs 过滤**

```go
// storage/query.go — matchesFilterFields 中，在 MissingID 检查后添加

// Tags 过滤
if len(filter.Tags) > 0 {
    obj.RLock()
    extra := obj.Extra
    obj.RUnlock()
    if !matchTags(extra, filter.Tags, filter.TagMode) {
        return false
    }
}

// ExcludeIDs 过滤
if len(filter.ExcludeIDs) > 0 {
    obj.RLock()
    id := obj.ID
    obj.RUnlock()
    for _, eid := range filter.ExcludeIDs {
        if id == eid {
            return false
        }
    }
}
```

- [ ] **步骤 4：添加 `matchTags` 辅助函数**

```go
// storage/query.go — 在 extraTagsContain 后添加

func matchTags(extra map[string]any, tags []string, mode string) bool {
    if len(extra) == 0 || len(tags) == 0 {
        return true
    }
    raw, ok := extra["tags"]
    if !ok {
        return false
    }
    var objTags []string
    switch t := raw.(type) {
    case []string:
        objTags = t
    case []any:
        for _, tag := range t {
            if s, ok := tag.(string); ok {
                objTags = append(objTags, s)
            }
        }
    }

    objTagSet := make(map[string]bool, len(objTags))
    for _, t := range objTags {
        objTagSet[strings.ToLower(t)] = true
    }

    if mode == "all" {
        for _, tag := range tags {
            if !objTagSet[strings.ToLower(tag)] {
                return false
            }
        }
        return true
    }
    // default: "any"
    for _, tag := range tags {
        if objTagSet[strings.ToLower(tag)] {
            return true
        }
    }
    return false
}
```

- [ ] **步骤 5：在 `storage/query.go` 的 `applySort` 中添加 `random` 和 `tag_match_desc` 支持**

```go
// storage/query.go — compareByField 中，default 前添加
case "random":
    return 0  // 随机排序不由 compareByField 处理，在 applySort 中特殊处理

// 在 applySort 函数开头添加
func applySort(objects []*model.DownloadObject, query *core.StorageQuery) {
    if len(objects) < 2 || query == nil || len(query.Sort) == 0 {
        return
    }

    // 检查是否有 random 排序规则
    for _, rule := range query.Sort {
        if rule.Field == "random" {
            // 随机打乱
            rand.Shuffle(len(objects), func(i, j int) {
                objects[i], objects[j] = objects[j], objects[i]
            })
            return
        }
    }

    // 原有排序逻辑
    sort.SliceStable(objects, func(i, j int) bool {
        // ... 原有代码 ...
    })
}
```

需要导入 `math/rand`。

- [ ] **步骤 6：在 `storage/query.go` 的 `compareByField` 中添加 `tag_match_desc` 排序**

```go
// 在 compareByField 的 default 前添加
case "tag_match_desc":
    leftTags := countMatchingTags(left, tagsFromQuery)
    rightTags := countMatchingTags(right, tagsFromQuery)
    if leftTags != rightTags {
        return leftTags > rightTags
    }
    return 0
```

需要将 `tagsFromQuery` 传递到 `compareByField`。更好的方式：在 `applySort` 中预处理。

简化方案：`tag_match_desc` 排序需要 context（匹配的 tags），在 `applySort` 中处理：

```go
// 在随机排序之后，检查 tag_match_desc
if len(query.Sort) > 0 && query.Sort[0].Field == "tag_match_desc" {
    // 不需要 tags 上下文时，按 extra.tags 数组长度降序
    sort.SliceStable(objects, func(i, j int) bool {
        // 使用 extra.tags 数组长度作为代理
        iTags := countTags(objects[i])
        jTags := countTags(objects[j])
        if iTags != jTags {
            return iTags > jTags
        }
        return false
    })
    return
}
```

临时简化：`tag_match_desc` 按 `extra.tags` 数组长度降序排列（后续可优化为按匹配标签数）。

- [ ] **步骤 7：在 MongoDB 的 `buildMongoFilter` 中添加 `Tags` 和 `ExcludeIDs` 过滤**

```go
// storage/mongo_storage.go — buildMongoFilter 中，在 MissingID 后添加

// Tags 过滤
if len(query.Filter.Tags) > 0 {
    pattern := ""
    if query.Filter.TagMode == "all" {
        // 所有标签都要匹配
        for _, tag := range query.Filter.Tags {
            orConditions = append(orConditions,
                bson.M{"extra.tags": bson.M{opRegex: regexp.QuoteMeta(tag), opOptions: "i"}},
            )
        }
    } else {
        // 任一标签匹配
        tagConditions := bson.A{}
        for _, tag := range query.Filter.Tags {
            tagConditions = append(tagConditions,
                bson.M{"extra.tags": bson.M{opRegex: regexp.QuoteMeta(tag), opOptions: "i"}},
            )
        }
        if len(tagConditions) > 0 {
            orConditions = append(orConditions, bson.M{"$or": tagConditions})
        }
    }
}

// ExcludeIDs 过滤
if len(query.Filter.ExcludeIDs) > 0 {
    filter["id"] = bson.M{"$nin": query.Filter.ExcludeIDs}
}
```

- [ ] **步骤 8：更新 `AggregationService` 接口传递新参数**

```go
// manager/aggregation_service.go — 更新 AggregateObjects 签名
func (svc *AggregationService) AggregateObjects(page, limit int64, search, sortBy, status string, types []string, tags []string, tagMode string, excludeIDs []int64) (map[string]any, error) {
    // ... 使用 tags/tagMode/excludeIDs ...
}
```

更新所有调用处，包括 `manager/aggregate.go` 中的 `AggregateObjects` 方法。

- [ ] **步骤 9：运行测试验证**

```bash
go build ./...
go test ./storage/... ./manager/... ./api/... -count=1
```

- [ ] **步骤 10：Commit**

```bash
git add api/server_metrics.go manager/aggregation_service.go manager/aggregate.go storage/query.go storage/mongo_storage.go
git commit -m "feat: extend aggregate API with tags/tag_mode/exclude_ids/random sort support"
```

---

## 验证

### 单元测试
```bash
# 全量测试（不含 MongoDB）
go test -count=1 ./model/... ./core/... ./storage/... ./task/... ./manager/... ./api/...
```

### 端到端验证
1. 启动服务：`go run . --config build/config.yaml`
2. 创建 hanime 任务，确认新对象自动获得 ID
3. `curl http://127.0.0.1:19199/api/objects/hanime/407014` 返回对象详情
4. `curl http://127.0.0.1:19199/api/objects/hanime/407014/collection` 返回合集
5. `curl "http://127.0.0.1:19199/api/aggregate?types=hanime&tags=tag1&tag_mode=any&exclude_ids=407014&sort=random&limit=20"` 返回推荐

### 旧数据兼容验证
1. 停止服务，清空存储，启动服务
2. 创建一批对象（模拟旧数据，无 ID 字段）
3. 确认启动后标准化服务自动补全 ID
4. 确认 `GET /api/objects/hanime/407014` 返回数据

---

## 执行交接

计划已完成并保存到 `docs/superpowers/plans/2026-07-26-object-id-collection-recommendation-plan.md`。两种执行方式：

**1. 子代理驱动（推荐）** — 每个任务调度一个新的子代理，任务间进行审查，快速迭代

**2. 内联执行** — 在当前会话中使用 executing-plans 执行任务，批量执行并设有检查点

选哪种方式？