# 架构评审：download-manager 模块化与「下载服务 / 任务下发」隔离演进（2026-09-29）

> 定位：架构评审记录 + 演进路径设计。**当前仅评审，未改代码。**
> 基线数据为**代码实证**，采用**快照标注**：统一口径为 `master` @ `934c721`（2026-09-29）。
> 供 roadmap「解耦 / 下载服务独立化」议题引用评审依据；引用时须注明快照 commit，避免数据漂移。

## 一、现状基线（实证 · 快照 `934c721`）

> 口径：对非 `vendor/`、非 `build/` 的 `*.go` 实测。

| 维度 | 实测值 | 说明 |
|---|---|---|
| Go 代码量 | ~5.3 万行（53167） | `manager/` 74 文件 / 15015 行，为最大顶层包 |
| 测试函数 | `^func Test` 计 **930** 个 | `-race` 全绿；8 个 CI 必检（archcheck / cover-check×2 / notest / test / test-cover / bench / build） |
| 任务类型 | `urllist / booksite / mock`（仓库内） | tktube/hanime/vikacg 等已迁 **sdserver**；`core/tasktype.go` 仍有历史枚举（见 §九 Q3） |
| 存储后端 | memory / file / mongo | 工厂注册 |
| 下载器 | native / wget / scraper / composite + `pkg/download` 新引擎 | 热路径 = worker 池 + resolve 池 + small-object 池 |
| 分层门禁 | `internal/archcheck/layers.go` L0–L4 | 新增顶层包必须登记，否则 CI 红 |
| Manager 结构 | `Manager` 单 struct **60 字段**；`*Manager` 接收方法全仓 **124**（manager.go 内 22）；api→manager 调用点实测 **78**（去重 49 个方法名） | 见 §四状态归属表 |

### 依赖方向实测（非测试代码、仓库内顶层包）

```
api  ->  manager  task  web  config  core  pkg
manager  ->  downloader  task  storage  config  core  model  pkg
task  ->  storage  config  core  model  pkg
downloader  ->  config  core  model  pkg
storage  ->  core  model  pkg
core  ->  model
model  ->（零仓内依赖）
```

`internal/archcheck`（测试门禁）、`testutil`（测试）、`cmd/playwright-server`（独立 go.mod）、`scripts`（shell）不参与该图。**严格单向、无环**——由门禁 `TestLayeringDirection` 强制。

## 二、架构是否合理

### ✅ 合理之处
1. **依赖分层干净、无环**（L0 基础库 → L1 领域包 → L2 能力层 → L3 编排层 → L4 接口层），由 archcheck 强制。
2. **接口抽象到位**：`core.Task` / `core.Downloader` / `core.Storage` 插件化，`task`/`storage` 工厂 + TaskUI 前端插件系统走注册模式——新增站点任务只需 `adapter + ui.js`，不动核心（roadmap P5-3 已验证）。
3. **Manager 已开始文件级拆分**：`SchedulerService` / `AggregationService` / `ConfigService` / `ObjectController` / `runtime_mgr` 已从 struct 抽出。

### 结构问题 A：Manager = 上帝 struct
`Manager` 单 struct 60 字段、`*Manager` 方法 124 个，集编排、状态、事件、统计、依赖注入、生命周期于一体。**文件级拆分已到，模块级内聚未发生**——service 是 `*Manager` 的**方法**而非独立对象，共享同一 `m.mu` 与同一套状态 map。拆难点在 60 字段共享（见 §四 状态归属表）。

### 结构问题 B：api→manager 契约面过宽
api→manager 调用点 78 处；`OnMetadata` / `SetMetadataFlusher` / `eventBus` / `progressBatch` / `urlRegistry` 等横切关注点跨下载热路径——下载器返回一个对象，manager 要跑整套状态机 / 去重 / 广播。

## 三、高内聚低耦合评分

| 子系统 | 内聚 | 耦合 | 评语 |
|---|---|---|---|
| `pkg/download`（新引擎，含 m3u8d） | 高 | 低 | 零**编排**依赖（不 import manager/task/api）；但仍依赖 config / model / pkg/logutil / grab 等 L0/L1。可独立打包，需连带这些依赖 |
| `storage` | 高 | 低 | 纯接口实现，工厂隔离，几乎零业务依赖 |
| `downloader`（网络层） | 高 | 中 | 自含，但被 Manager 回调驱动（OnMetadata / OnProgress / SetMetadataFlusher） |
| `task`（下发层） | 中 | 中 | BaseTask 直连 storage；流转由 Manager 牵头 |
| `manager`（编排层） | 中 | 高 | 唯一超级节点，上行（api）+ 下行（downloader/task）全汇它 |

**结论**：底层三层质量好（尤其 storage、`pkg/download`）；堵点集中在 **manager**。

## 四、状态归属表（Step1 交付物——从「文件级」到「模块级」）

> 依据 `manager/manager.go` 的 `type Manager struct` 实测（**60 字段**，字段名照抄源码）。**归属后每个 service 自持独立锁，`m.mu` 收窄为仅保护真正共享的注册表与队列元数据。** 本表是 Step1 的 **验收清单**：完成后除下表中「Manager（薄门面）」列外，字段不应再被 `Manager` 方法直接读写。

| 归属 | 字段（源码原名） | 说明 |
|---|---|---|
| **Manager（薄门面，全局共享）** | `cfg` `cfgVal` `tasks` `mu` `downloader` `downloaderMu` `stopChan` `initializedCh` `startedAt` `forceWg` `drainMode` `drainDone` `drainOnce` `drainOnceStarted` | 生命周期、任务注册表、下载器引用、停机/排空；`m.mu` 仅存这里 |
| **SchedulerService** | `schedSvc` `schedulerEnabled` `schedulerStop` `schedulerSignal` `taskQueues` `scanRunning` `processingTask` `scrapingTask` `schedulerHeartbeat` `workerHeartbeat` | 调度/扫描编排、任务队列、心跳 |
| **ExecutorService（新抽：执行态）** | `workerStop` `workerCount` `workerWg` `downloadQueue` `activeDownloads` `downloadingObj` `inflight` `lastProgress` `totalDownloads` `workersEnabled` | 下载热路径：worker 池、去重、进度 |
| **ResolveService** | `resolveQueue` `resolveCtx` `resolveCancel` `resolveWg` `resolveCache` `compositeResolveCount` | 详情解析，3 并发 |
| **SmallObjectService（已有 small_object.go）** | `soQueue` `soCtx` `soCancel` `soWg` `soTracker` `soInflight` `soPending` | 关联小对象 |
| **RetryService（新抽）** | `failureRecords` `failureMu` `failureWriteIdx` `maxFailures` `failedCount` | 失败环形缓冲 + 永久标记 |
| **EventService（已有 events.go）** | `subscribers` `eventMu` | 进程内总线；Step3 演为广播扇出 |
| **MetricsService** | `metrics` | 健康检查 / 上报 |
| **ObjectController / AggregationService / ConfigService（已有）** | `objectCtrl` `aggSvc` `configSvc` | 已独立，仅迁剩余字段依赖 |

**迁移方式**：service 由「写在 `*Manager` 上的方法」外提为独立 struct，构造函数以**最小接口注入**依赖（不传 `*Manager` 整对象），service 内部自持互斥。一个服务一条 PR，配 `-race` 单测。

## 五、演进路径（三步，各自独立可交付，各含测试与零回归）

### Step 1：进程内可视化拆分
- service 提升为独立 struct，显式 DI，字段按 §四 迁走，`m.mu` 收窄。
- **不改变部署边界、不改 API/SSE 契约、不改对象字段模型。**
- **零回归**：`go test -race ./manager/...` + `make archcheck` 保持绿。

### Step 2：定义下载服务边界
- 定义 `pkg/job` 契约（§六）。Manager→Executor 只经该接口，底层 downloader 实现之。
- **门禁**：契约表驱动（完成/重试/取消/幂等/状态机）+ 行为对照（与 v0 `pkg/download` 语义对齐）。

### Step 3：拆两个二进制
- **下发端 + API + UI + SSE 事件端**（一个二进制）
- **下载执行 worker**（独立二进制 / 服务）
- 通信默认 **HTTP/REST**（stdlib 优先、零新增基础设施）；Redis 队列为可选。状态经共享 **storage（file/mongo）** 同步。
- 落实并下沉进 job 层：节流 / 重试 / 幂等 / 状态恢复。
- **不要把 storage 与 downloader 再拆碎**：「下载服务 = job executor + 下层存储」；「任务下发」= 定义任务 / 扫描 / 抓取 / 生成对象 / 编排。

## 六、download-job 契约（Step2 预先定版，Step3 直接复用）

```go
// pkg/job（L1 层，只依赖 L0/L1，不依赖 manager/api）
type Kind int
const (
    KindRaw Kind = iota
    KindVideo
    KindImage
    KindAudio
    KindComposite // 多个子文件
)

type File struct { URL, Path, Type string }

type Job struct {
    ID        string // 幂等键；建议 = 对象 URL（或 URL/SavePath），重启可重放
    URL       string
    Headers   map[string]string
    SavePath  string
    Kind      Kind
    Files     []File // KindComposite 时使用
    MaxRetries int
    RetryBackoff time.Duration
}

type JobStatus int
const (
    JobPending JobStatus = iota
    JobDownloading
    JobCompleted
    JobFailed
    JobCanceled
)

type Result struct {
    Status   JobStatus
    Progress int64 // 0..100
    Metadata map[string]string // 落库合并
    Err      error
}

// Executor：进程内实现（Step2），跨进程时由中间层实现（Step3）
type Executor interface {
    Submit(ctx context.Context, jobs []*Job) ([]string, error) // 返回 jobID
    Status(ctx context.Context, jobID string) (Result, error)
    Cancel(ctx context.Context, jobID string) error // 幂等
}
```

**契约语义（必须写在接口层，而非落进 Manager）**
- **幂等**：同 jobID 重复提交→不重复下载，命中已存在对象并恢复未终态。
- **进度**：进程内走 `OnProgress / OnMetadata`（现状）；跨进程由执行端**推**事件、下发端**扇出**给 SSE。
- **重试 / 节流 / 恢复**：`MaxRetries` / `RetryBackoff` 随 job 下发，终态落共享存储，崩溃后按 jobID 回放未终态。

## 七、跨进程事件 / 进度契约（Step3）

现状：SSE 由 Manager 进程内 event 总线直接驱动（`/api/events` → subscribe → publish）；进度经 `OnProgress / OnMetadata` 回调写入。拆分后 **SSE 只在「下发端（API + UI + SSE）进程」**：

```
执行 worker ─(HTTP POST /internal/events, 共享 token)─▶ 下发端 event sink
   ▲   push(对象状态+进度)              │  merge 进本地总线
   │                                   └─▶ 本进程 event 总线 ─▶ SSE /api/events
   └─(终态/未终态回放) 轮询共享 storage ─┘
```

1. 执行端把「对象状态 + 进度」持久化到共享 storage，并**推**增量到下发端 internal 端点；
2. 下发端经 `POST /internal/events`（用 `DM_INTERNAL_TOKEN`，独立于用户鉴权）merge 进本地总线，再广播到 SSE；
3. **内部鉴权与用户鉴权分离**：`/internal/*` 不经 Web 用户鉴权中间件。
4. 事件单一、可幂等合并、可重试；SSE 永不直连执行 worker。

## 八、测试与可验证性（各 step 门禁）

| 步骤 | 可测试动作 |
|---|---|
| **Step1** | 1) `TestStateOwnership`（字段归属断言）；2) 每 service `-race` 并发回归（≥3 用例）；3) 事件序列回归 v0↔v1 一致；4) `make archcheck` 保持绿 |
| **Step2** | 1) `pkg/job` 契约表测（完成/重试/取消/幂等/状态机）；2) `mockExecutor` 注入测试；3) 行为对照 v0 `pkg/download` |
| **Step3** | 1) 测试矩阵起两个子进程（httptest 挂载两二进制）；2) 崩溃注入 → job 幂等重放；3) bench 节流/负载；4) Playwright SSE 全链路（事件按序、扇到多个订阅端） |

> 门禁统一接 `make check-ci`；新增测试不得低于当前 `-race` 全绿基线。

## 九、决策点（Q1–Q4，待用户决议）

- **Q1 立项**：是否收录为 roadmap 阶段议题？若是，**与既有 P2#8「拆分 Manager 职责」合并立项**，避免两份计划重复；P2#8 不含拆二进制，把 Step3 作为随行子议题即可。
- **Q2 通信选型**：Step3 用 **HTTP/REST（默认，零新增基础设施）** 还是 Redis 队列？决定 Step3 测试注入形状。切换栅栏：除非 §8 bench 显示 HTTP 轮询成瓶颈才评估 Redis，且另开评审不默认选。
- **Q3 历史引用**：`core/tasktype.go` 的历史枚举、`cmd/playwright-server/fixture` 依赖站点任务的用例是否随 sdserver 迁移归档或剔除？**推荐保留 foster**：枚举未被工厂误用、删除破坏编译面；fixture 保留为「历史回归」快照。是否剔除留给各拆分 PR 自然收敛。
- **Q4 分层表**：`layers.go` 已是 L0–L4 五层并附描述；进程边界是 Step3 才引入的部署概念，**推荐暂不新增 L5**，Step3 落地、确认「独立执行 worker」时再按 archcheck 增补 L5（`pkg/job` + 契约测试），仅作 append。

## 结论

- 架构方向健康、分层无环、扩展性好（尤其 storage / `pkg/download` / task 工厂 / TaskUI 插件）。
- 低内聚高耦合的中心在 **manager**：文件级已拆、服务级仍巨型单例；拆难点在 60 字段共享——本文已给出可执行的状态归属表。
- 「下载服务 + 任务下发」独立程序**可行且是正确演进**：`pkg/download` 引擎与 `core.Task` 任务接口均已预埋；但分工是纵向（领域分层）而非横向（部署切分），直接拆二进制需谨慎，建议按 Step1→2→3 先接口后拆机、全程保持单二进制可运行。