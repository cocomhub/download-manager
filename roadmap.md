# download-manager — 项目发展规划（2026-09 起，1 年窗口）

> 本文档基于代码实证编写，标记 ✅ 的条目表示**当前已完成**，避免重复规划。
> 配套精简版见 `docs/implementation-roadmap.md`；两者以本文档为准。

## 一、现状基线（2026-09-23 实证）

### 规模与健康度

| 维度 | 数值 | 说明 |
|------|------|------|
| Go 代码量 | ~47K 行 | 不含 build/、tmp/、playwright-server |
| 测试函数 | 469 个 | `go test ./...` 全绿（含 race 路径） |
| 任务类型 | 4 种 | urllist / tktube / hanime / vikacg |
| 存储后端 | 3 种 | memory / file / mongo（v2 + testcontainers 集成测试） |
| 下载器 | 多形态 | native / wget / scraper / composite / multi |
| CI | 5 jobs | test / test-no-mongo / lint / playwright / sonar |
| 覆盖率门禁 | project 40% / patch 80% | `make cover-check` + Codecov |
| 发布状态 | **0 个 tag** | 从未正式发版，GoReleaser 配置就绪但未跑通过 |

### 已完成能力（✅ 不再重复规划）

- ✅ 写保护统一：全局 `writeMiddleware`（UI 模式 / 双功能关闭时拦截非 GET/HEAD），auth 中间件先于写保护执行
- ✅ 优雅停机：`main.go` 5s 超时 context + `mgr.Stop()` + `srv.Shutdown()` + `WaitForShutdown()`（worker/force-download） + `ForceFlush()` + Mongo client 关闭，二次信号强制退出
- ✅ 下载根目录收敛：`config.GetDownloadRootDir()`（FilesDir 优先回退 DownloadRootDir），`/files/` 与落盘共用同一逻辑
- ✅ 鉴权基础：AuthConfig 支持 basic / token 两种方案，`crypto/subtle` 常量时间比较
- ✅ Manager 部分拆分：`AggregationService`（只读聚合/分组查询）、`ConfigService`（配置治理）已抽出
- ✅ 对象扩展字段结构化：`model/object_meta.go` 提供 `ObjectMeta` / `ObjectMetadata`（Extra / Metadata 的结构化视图）
- ✅ 前端模块化：TaskUI 插件系统（registry/defineForm/defineMeta/baseViewer/loader）+ `web/static/app/` 多模块（api.js / main.js / taskui/），index.html 已从 4050 行降至 1686 行
- ✅ mongo-driver v1→v2 迁移 + testcontainers 集成测试
- ✅ Playwright e2e：10+ specs（dashboard / aggregate / config / accessibility / cross-browser / fault-injection 等）

### 积压与债务（本次规划要处理的）

- ⚠️ 最近主提交 2026-08-07，**约 1.5 个月无活跃提交**
- ⚠️ 110 个遗留分支（大量已合并分支未清理，含 worktree-agent-* 临时分支）
- ⚠️ 10 个 dependabot PR 积压（含已过期的 #10 v1.17.9，v1→v2 迁移后应 close）
- ⚠️ `pkg/dlcore` 已标记 Deprecated，但 `downloader/native.go` 仍在使用；替代 `pkg/download` 已有 6 项行为对齐 + Comparator 测试套件
- ⚠️ `pkg/dlcore/client.go:227` DomainLimiter 存在忙等待 spin-loop（`for max != 0 && cur >= max`）
- ⚠️ Manager 仍偏重：对象操作（ObjectController）、调度（Scheduler）职责未完全拆出
- ⚠️ 聚合查询仍偏向全量拉取后内存过滤排序分页，未下推 storage 层
- ⚠️ 配置接口未覆盖全部配置面（filesystem / http / proxy / progress / ffmpeg 读取编辑不完整）
- ⚠️ 无 pr-title 门禁、无 archcheck 门禁体系（工程能力落后于 sproxy）
- ⚠️ 上游依赖 `github.com/cocomhub/sproxy` 为 pseudo-version，依赖 CI 私有 module 访问

## 二、阶段规划

### 阶段 0：重新激活与发布启动（第 1-2 月）
> ✅ **已完成**（2026-09-23）：v0.1.0 发布、pr-title/release-please 落地、dlcore 退役、分支清理

**目标：仓库恢复健康活跃状态，完成首个正式发布 v0.1.0。**

#### P0-1 仓库治理（清理积压）
- 清理 110 个遗留分支：确认已合并分支后删远端 + 本地（`git remote prune origin` + `git branch -D`），保留仍在用的功能分支
- 消化 dependabot 积压：合入可合并的版本升级 PR（actions/setup-go、checkout、sonarqube-scan 等），**close 过期的 #10（mongo-driver v1.17.9）**——v1→v2 迁移后该分支已无意义
- 决策未跟踪文件：AGENTS.md（应提交，作为 Codex 指南）、`.pi/`（评估是否 gitignore）

#### P0-2 dlcore 废弃迁移收尾
- `downloader/native.go` 从 `pkg/dlcore` 切换到 `pkg/download`（6 项行为差异已有对齐方案 + Comparator 测试套件保障，见 AGENTS.md「dlcore → pkg/download 行为差异」）
- 删除 `pkg/dlcore` 包及其 dlcore-only 测试（`DlcoreOnlyRun` 类），移除 `//nolint:staticcheck` 兼容注释
- 验收：`grep -rn "pkg/dlcore"` 归零，全量测试 + lint 通过

#### P0-3 首个正式发布 v0.1.0
- 审查 `.goreleaser.yaml`（当前目标平台/制品/签名是否就绪）
- 本地 `make build` + `goreleaser snapshot` 预演
- 打 tag `v0.1.0`，验证 GoReleaser workflow + Docker 镜像推送 ghcr.io
- 验收：GitHub Release 页面出现 v0.1.0 制品，Docker 镜像可拉取

#### P0-4 发布流程建立（对齐 sproxy）
- 评估 release-please 自动化（sproxy 已有 release-please.yml + 子 module tag 流程）vs 保持手动 tag 触发 release.yml
- 无论选哪种，补齐：CHANGELOG 策略、版本号规则（Conventional Commits 驱动）、发布文档
- **对齐项**：引入 pr-title workflow（校验 PR 标题符合 Conventional Commits，对齐 sproxy `pr-title.yml`）

### 阶段 1：工程健康度与稳定性（第 3-6 月）
> ✅ **已完成**（2026-09-24）：Manager 拆分、聚合下推、配置接口、race 清扫、停机边界、notest 门禁修复

**目标：消除已知技术债，Manager 拆分到位，性能与并发安全可度量。**

#### P1-1 Manager 职责残余拆分
- 继续拆分：`ObjectController`（对象操作：cancel/undo/retry/reorder/batch）、`SchedulerService`（调度/worker 池）
- Manager 保留：生命周期、组装、事件总线、健康检查
- 顺序：先 ObjectController（边界清晰）→ SchedulerService
- 验收：Manager 核心文件行数显著下降，各服务有独立单测

#### P1-2 聚合查询下推
- `storage.Search()` 接口扩展：支持 limit / offset / sort / filter 下推（file / mongo 后端各自实现）
- `AggregationService` 不再全量拉取后内存过滤
- 5000+ 对象场景基准测试（`manager/bench_test.go` 已有基础）
- 验收：大样本下聚合接口响应时间与内存占用显著下降，基准有回归护栏

#### P1-3 DomainLimiter spin-loop 修复
- `pkg/dlcore/client.go:227` 忙等待 → `sync.Cond` 或 channel-based semaphore
- 验收：无 spin-loop（代码审查 + 测试覆盖并发等待场景）

#### P1-4 配置接口补齐
- GET / POST `/api/config` 覆盖全部配置面：filesystem / http / proxy / progress / ffmpeg
- UI 配置面板补全（web/static 改动需遵守 Web UI 测试硬要求：node --test 单测 + Playwright e2e）
- 验收：配置面 100% 可通过 API 读写，导入前校验与错误提示完善

#### P1-5 数据竞争清扫
- 按已知模式清单全量复查：stale pointer（download cancel 竞争）、sync.Map 类型断言、Metadata/Extra 加锁、downloader 专用锁封装
- race 测试补强：`go test -race` 全量 + 既有 race_test.go / worker_stop_test.go 扩展
- 验收：`make check-ci` 全绿，race detector 零报警

#### P1-6 停机边界强化
- `shutdown_test.go` 扩展：in-flight 下载、force-download goroutine、Mongo flush、二次信号强制退出
- 验收：覆盖停机各分支的测试通过

### 阶段 2：产品功能增强（第 6-9 月）
> ✅ **已完成**（2026-09-24）：任务模板沉淀、代理池增强、跨任务搜索、ObjectMeta 访问器

**目标：任务接入标准化、下载能力扩展、对象协议稳定。**

#### P2-1 新任务类型接入标准化
- 利用已有 `task/TEMPLATE` + `scripts/new-task-type.sh` + TaskUI 插件系统
- 沉淀站点 adapter 模板：paging / detail / cache / ready 判定通用逻辑（对应旧 roadmap「任务模板与站点 Adapter」）
- 规划落地 1-2 个新站点任务（候选站点需调研后确认）
- 验收：新增任务仅需编写 site-specific adapter + ui.js，无需改核心

#### P2-2 下载能力扩展
- 候选方向（需先需求调研取舍）：aria2 后端（多连接加速）、m3u8 清晰度/变体选择、代理池增强
- 任何下载器改动必须通过 downloader 包既有契约测试（adapter_contract_test.go / functional / e2e）
- 验收：新能力有契约测试覆盖 + 基准数据

#### P2-3 跨任务搜索与预览增强
- 跨任务统一对象搜索
- 播放器 / 图片预览增强、合集 / 推荐面板打磨（现有 collection.js / recommendation.js 基础）
- 验收：Playwright e2e 覆盖新增交互

#### P2-4 ObjectMeta 协议推广
- `model/object_meta.go` 结构化字段逐步取代隐式 Extra / Metadata map 协议
- 建立字段清单与命名约束文档（对齐旧 roadmap「对象扩展字段协议治理」）
- 验收：高频字段全部走结构化访问层，前后端契约稳定

### 阶段 3：安全与运维 / 产品化（第 9-12 月）
> ✅ **已完成**（2026-09-24）：鉴权强化、SSE/文件边界、docker-compose+systemd、依赖治理、archcheck 门禁；**v0.3.0 收官发布**

**目标：部署形态就绪、安全边界收敛、工程能力对齐 sproxy。**

#### P3-1 鉴权默认开启策略
- 明确 auth 默认值决策（默认关闭 + 文档警示 vs 默认 basic），README 与部署文档标注安全前提
- token 方案完善（签发/吊销/过期）
- UI 登录态（401 时跳转登录页）
- 验收：默认部署下风险可控，安全前提文档化

#### P3-2 SSE 与文件接口边界收敛
- SSE 来源校验（Origin / token）
- `/files/` 路径穿越防护、MIME 白名单
- 验收：边界测试覆盖（Playwright + API 测试）

#### P3-3 部署形态
- Docker Compose（app + mongo）、systemd unit、部署文档
- 验收：文档化部署步骤可复现

#### P3-4 上游依赖治理
- `github.com/cocomhub/sproxy` pseudo-version → 固定版本策略或本地 replace 开发策略
- 私有 module 访问配置文档化（CI 已有 Configure private module access 步骤，补本地说明）
- 验收：构建可复现，依赖版本显式

#### P3-5 工程能力对齐 sproxy（贯穿性条目）
- 引入 `internal/archcheck` 门禁子集（按 dm 规模裁剪）：layers 方向性、notest_gate、makefile 一致性、release 策略门禁（对齐 sproxy arch_test.go / notest_gate_test.go / makefile_*_test.go / release_policy_test.go）
- Makefile 目标对齐：web-test（前端 JS 单测挂 CI）、deadcode-check
- 验收：门禁全绿，防回归能力与 sproxy 同级

## 三、里程碑与验收

| 里程碑 | 时间 | 验收标准 |
|--------|------|----------|
| **M0 重新激活** ✅ | 第 2 月末 | 仓库无遗留分支积压、dependabot 归零、pkg/dlcore 退役、**v0.1.0 已发布**（Release + Docker）、pr-title 门禁生效 |
| **M1 稳定** ✅ | 第 6 月末 | Manager 拆分完成、聚合下推生效（基准达标）、spin-loop 清零、配置面完整、race 零报警 |
| **M2 增强** ✅ | 第 9 月末 | 新任务类型接入验证通过、下载能力扩展落地、ObjectMeta 协议主导、跨任务搜索可用 |
| **M3 产品化** ✅ | 第 12 月末 | 鉴权默认开启、Docker Compose / systemd 部署就绪、sproxy 依赖治理完成、archcheck 门禁与 sproxy 同级、部署与安全文档完整 |

## 四、放弃项（明确不做，防止路线漂移）

- **分布式多实例集群**：保持单实例架构；多部署 = 各自独立实例 + 共享外部存储，不做任务级分布式协调
- **BT / 磁力 / P2P 下载协议**：聚焦 HTTP / HLS 链路（含 wget / aria2 候选），不引入 BitTorrent 协议栈
- **插件市场 / 第三方生态**：TaskUI 插件系统仅服务本仓库任务类型，不开放对外市场
- **多租户 / 计费 / 用户体系**：定位本机 / 内网 / 个人使用，不做商业化账号系统
- **桌面客户端 / 浏览器插件**：保持 Web UI 单一形态
- **全量兼容 youtube-dl 类站点**：保持站点 adapter 模式，不承诺任意站点可下载

## 五、风险与依赖

| 风险 | 影响 | 缓解 |
|------|------|------|
| sproxy 依赖为 pseudo-version，主仓变更可能破坏构建 | 高 | 阶段 3 固定版本；阶段 0-2 期间 CI 持续监控；本地 replace 策略备案 |
| Playwright CI 稳定性（浏览器安装 / flaky） | 中 | 已有 fault-injection / cross-browser 专项；沿用 MustEventually 轮询纪律 |
| 覆盖率门禁维持（40% / 80%） | 中 | 新代码随提交补测试；发布前 `make check-ci` 强制 |
| dlcore 迁移引发行为回归 | 中 | Comparator 测试套件（dlcore-only + 对齐断言）作为迁移护栏 |
| 用户优先级变化（roadmap 是活文档） | 低 | 每季度复审，里程碑按需调整 |

## 六、推进顺序建议

1. **第 1 月**：P0-1（分支清理 + dependabot）→ P0-2（dlcore 迁移，可并行）
2. **第 2 月**：P0-3（v0.1.0 发布）→ P0-4（发布流程 + pr-title）
3. **第 3-4 月**：P1-1（Manager 拆分）→ P1-3（spin-loop）→ P1-5（race 清扫，穿插进行）
4. **第 5-6 月**：P1-2（聚合下推）→ P1-4（配置接口）→ P1-6（停机边界）
5. **第 7-9 月**：P2-1 → P2-2 → P2-4 → P2-3（依赖顺序：先任务标准化，再下载能力，协议治理穿插）
6. **第 10-12 月**：P3-1 → P3-2 → P3-3 → P3-4，P3-5（archcheck 对齐）贯穿第 6-12 月
