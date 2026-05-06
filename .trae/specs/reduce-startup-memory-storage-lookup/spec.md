# 启动期内存收敛与 Storage 查找 Spec

## Why
当前服务启动时会创建并读取任务级 `cache` 文件，同时对 `Storage` 执行全量加载与冷启动回灌，导致对象数据、状态数据和派生索引在内存中重复驻留。对于 `mongo` 场景，`Search(nil)` 与 `cursor.All` 会一次性把大集合拉到进程内，直接放大启动耗时与内存占用。

## What Changes
- 移除任务级 `cache` 文件机制，不再依赖 `<work_dir>/cache/*.json` 作为启动或查询的数据来源
- 调整启动流程：启动阶段不得把任务对象或存储内容全量加载到内存；仅初始化任务元数据、运行时组件与必要连接
- 将对象列表、详情、搜索、聚合、分组展开、重试/取消定位等内容查询路径改为按需通过 `Storage` 获取
- 扩展 `Storage` 查询能力，支持精确查找、过滤、排序、分页、计数，以及必要的轻量批量查询
- 优化 `mongo` 存储实现：常规请求必须在数据库侧完成过滤、排序与分页，禁止通过全量拉取后在内存中再筛选
- 将冷启动期间的全量对齐、全量回填、全量共享状态回灌改为按需查询、分批处理或显式维护任务
- **BREAKING** 任务与管理器不得再假设“所有历史对象在启动完成后已常驻内存”；依赖 `GetAllObjects()` 的读取路径需要迁移为 `Storage` 驱动

## Impact
- Affected specs:
  - Manager 启动生命周期与任务装载流程
  - Task 持久化、对象查找与增量刷新流程
  - API 对象列表、详情、聚合与分组查询流程
  - Mongo 存储查询模型与索引策略
- Affected code:
  - `manager/manager.go`
  - `manager/task_loader.go`
  - `api/server.go`
  - `core/interfaces.go`
  - `storage/mongo_storage.go`
  - `storage/file_storage.go`
  - `task/tktube_task.go`
  - `task/vikacg_task.go`

## ADDED Requirements
### Requirement: 无缓存冷启动
系统 SHALL 移除任务级 `cache` 文件读写逻辑；服务启动时不得创建、读取或刷新 `<work_dir>/cache/*.json`，也不得依赖该目录恢复对象状态或对象列表。

#### Scenario: 启动时不再读取任务缓存
- **WHEN** 服务启动且历史工作目录下存在旧的 `cache` 文件
- **THEN** 系统不会读取这些文件恢复任务对象
- **AND** 系统不会因缺失 `cache` 目录而创建新的任务缓存文件

#### Scenario: 关闭时不再写入任务缓存
- **WHEN** 服务正常停止
- **THEN** 系统不会调用任务级缓存持久化逻辑
- **AND** 任务状态仅通过 `Storage` 或显式运行时状态完成保留

### Requirement: Storage 成为对象查询事实来源
系统 SHALL 将 `Storage` 作为历史对象与状态对象的事实来源。对象列表、详情、搜索、分组展开、聚合与单对象定位必须通过 `Storage` 的按需查询完成，而不是依赖任务内存切片或启动期预加载结果。

#### Scenario: 任务详情按页查询
- **WHEN** 用户查看某个任务的对象列表并传入分页、搜索或排序参数
- **THEN** 系统仅查询当前请求所需的数据页
- **AND** 不会先把该任务全部对象加载到内存再进行筛选

#### Scenario: 单对象操作按需定位
- **WHEN** 用户对单个对象执行重试、取消或查看详情
- **THEN** 系统通过 `Storage` 精确查找目标对象
- **AND** 不要求任务在内存中持有全量对象集合

### Requirement: 启动阶段禁止全量回灌
系统 SHALL 禁止在启动阶段执行针对所有任务对象的全量 `Storage.Search(nil)`、全量共享状态回灌或全量内容分组回填。若确有必要，必须改为显式维护任务、后台分批任务或首访时懒处理。

#### Scenario: 大集合冷启动
- **WHEN** `mongo` 存储中存在大量历史对象
- **THEN** 服务启动仅建立连接与必要运行时结构
- **AND** 启动完成不依赖把所有对象解码到内存

### Requirement: Mongo 查询必须服务端分页过滤
`mongo` 存储 SHALL 支持服务端过滤、排序、分页与计数；常规请求不得使用“查询全集后在 Go 内存中过滤”的方式完成列表、搜索、聚合或分组查询。

#### Scenario: Mongo 任务对象分页
- **WHEN** 用户查询某个任务的第 N 页对象列表
- **THEN** `mongo` 仅返回该页所需记录
- **AND** Go 进程不会通过 `cursor.All` 拉取整个集合

#### Scenario: Mongo 条件查询
- **WHEN** 用户按 `task_id`、状态、分组键或关键字执行对象筛选
- **THEN** 过滤条件在数据库侧执行
- **AND** 常用条件具备对应索引或明确的限流边界

### Requirement: 增量刷新通过 Storage 去重
任务增量刷新 SHALL 通过 `Storage` 精确查重或轻量批量查重判断对象是否已存在，不再通过启动期构建的全量 `knownURLs` / `objects` 常驻集合承担历史去重职责。

#### Scenario: 刷新新页面时查重
- **WHEN** 任务抓取到一批候选 URL
- **THEN** 系统仅对当前批次 URL 执行存在性检查
- **AND** 不需要提前把历史全部 URL 装入内存

## MODIFIED Requirements
### Requirement: 对象读取接口语义
现有对象读取路径修改为：
- 任务实例可保留少量运行时热数据，但不得承诺持有完整历史对象集
- Manager 与 API 读取对象时优先走 `Storage` 查询接口
- 仅正在下载、刚更新或当前请求命中的对象允许进入短期内存缓存

### Requirement: 共享状态与内容分组校正
现有共享状态对齐、内容分组回填与聚合逻辑修改为：
- 不再依赖启动期全表扫描
- 若需要校正历史数据，必须通过显式维护入口或后台分批执行
- 请求路径上的分组与聚合优先使用 `Storage` 查询结果，避免为查询而构建全量常驻副本

## REMOVED Requirements
### Requirement: 任务级 JSON Cache 作为启动数据源
**Reason**: 与 `Storage` 持久化重复，且会在启动期造成对象数据的二次常驻与高峰内存放大。
**Migration**:
- 启动恢复统一改为 `Storage` 按需查询
- 旧 `cache` 文件视为废弃数据，不再读取、不再写回

### Requirement: 启动时全量预热 Storage 到内存
**Reason**: 在大数据量尤其是 `mongo` 场景下会导致不可接受的启动内存与耗时。
**Migration**:
- 列表、详情、搜索、聚合改为分页/过滤查询
- 全量校正类工作改为显式维护任务或后台分批流程
