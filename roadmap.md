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
> ✅ **已完成**（2026-09-24）：鉴权强化、SSE/文件边界、docker-compose+systemd、依赖治理；**v0.3.0 收官发布**
> ⚠️ **补漏**（2026-09-24 审计发现）：P3-5 archcheck 门禁曾被空合并（PR #86 仅 docs）→ 阶段 4 重做落地（PR #98，8 项门禁 + tunnel 迁移 + build 对齐）

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

## 七、阶段 4：收官审计与补漏（2026-09-24）

> 用户要求：检查 roadmap 所有任务是否完整实现，避免遗漏或假实现。

### P4-1 archcheck 门禁落地（P3-5 补漏）
- 审计发现：PR #86 是空提交（tree 与父相同），archcheck 8 项门禁从未进入 master
- 重做：`internal/archcheck/` 8 项门禁（layers/notest_gate/makefile_dup/release_policy/build_flags/dead_symbols/docs_rules/duplication）
- 架构倒置修复：downloader → `cmd/scraper_get/tunnel` 迁移到 `pkg/scraper_tunnel`
- build 对齐：VERSION `--match 'v[0-9]*'` + `GO_BUILD_FLAGS=-trimpath` + goreleaser `flags: -trimpath` + ci.yml 接入
- 状态：✅ PR #98

### P4-2 m3u8d 收敛
- `pkg/m3u8d` 与 `pkg/download/m3u8d` 同源重复 → 收敛：cmd/m3u8d 改用新引擎（M3U8DEngine），删除旧 pkg/m3u8d
- **保留 cmd/m3u8d**（CLI 入口，用户确认）
- 状态：✅ PR #97

### P4-3 roadmap 修正
- 阶段 3 标注补漏说明（P3-5 曾空合并 → 重做）
- 状态：✅ 本文档

## 八、阶段 5：产品收尾与生态扩展（2026-10 起）

> 规划日期：2026-09-24。基于代码实证盘点（`web/static/` 无 401/login、bench gate 缺失、TEMPLATE 0 站点落地、README 25 行过时、unified-media-fields 分支保留 4 功能提交）。
> 用户已确认方向（2026-09-24）：全部纳入。

### P5-1 UI 登录态落地（P3-1 遗留子项）
- 盘点证据：`api/auth.go` 中间件返回 401，但 `web/static/` 全仓 **0 处** 401/login 处理 → 启用鉴权后 UI 完全不可用（白屏/报错）
- 内容：401 统一拦截 + 登录页（basic/token）+ token 持久化（localStorage）+ 请求自动附加 Authorization 头 + 登出
- 测试：纯函数 node --test + Playwright e2e（真浏览器，Web UI 硬要求）
- 验收：启用鉴权后 UI 可登录使用，401 时自动跳转登录页

### P5-2 unified-media-fields 分支合入
- 盘点证据：保留分支含 4 个功能提交（统一 cover/thumb/preview 固定字段 + 内容分组书橱视图 / content 分组聚合下推 / 对象数据版本升级 + 卡片封面比例 / 下载期并发修复）
- 内容：与 master 差异评估（151 文件 diff，基于旧 8dee6ec）→ cherry-pick 有价值部分 → 冲突解决 + 行为对齐
- 验收：有价值功能合入且全量测试绿，过期部分明确丢弃

### P5-3 新站点任务落地
- 盘点证据：`task/TEMPLATE` + `scripts/new-task-type.sh` 就绪，P2-1 目标「规划落地 1-2 个新站点任务」未达（0 个新站点）
- 内容：候选站点调研（需用户确认）→ 落地 1-2 个站点任务（仅需 site-specific adapter + ui.js，不动核心）
- 验收：新任务类型经模板创建、注册、UI 展示、下载全链路验证
- **方向已确认**（2026-09-24）：漫画/图片站 + 视频站各 1 个

### P5-4 性能压测 + 基准门禁
- 盘点证据：`make bench` + `bench-compare`（benchstat）存在；CI benchmark-action `continue-on-error: true`（无失败 gate）
- 内容：大样本（5000+ 对象）聚合/存储压测报告 + bench-compare 加失败 gate（阈值比较）+ CI 门禁接线
- 验收：基准回归有护栏（超阈值 CI 红），压测报告可复现

### P5-5 文档收敛
- 盘点证据：README 仅 25 行（无安装/发布/功能/配置）；无 config.md
- 内容：README 重写（安装/发布/功能总览/快速开始）+ 新增 docs/config.md（配置参考，对齐 sproxy 文档体系）
- 验收：新用户按文档可完成安装配置启动

## 优先级矩阵（阶段 5）

| 优先级 | 项 | 理由 |
|--------|----|------|
| S1 正确性 | P5-1 | 鉴权启用后 UI 不可用 = 功能正确性缺陷 |
| S2 高价值 | P5-3、P5-4 | 新站点扩展产品价值；基准门禁防回归 |
| S3 架构 | P5-2 | 数据模型协议统一，影响后续 UI 开发 |
| S4 并行 | P5-1/P5-4 独立可并行 | 无共享状态 |
| S5 长尾 | P5-5 | 文档贯穿各阶段 |

## 推进顺序（阶段 5）

1. **P5-2 分支评估先行**：决定合入内容（可能影响 P5-1 的模型基础）
2. **P5-1 + P5-4 并行**：独立无依赖
3. **P5-3**：候选站点需用户确认后调研落地
4. **P5-5**：贯穿期完成（README 最后统一收口）

## 九、阶段 6：Web UI 增强 + 代理抓取（2026-10 起）

> 规划日期：2026-09-24。用户已确认方向（视频快捷键/全屏 + sproxy+sclient 代理下载）。
> API 文档不纳入；booksite 保留 xkcd 现状。

### P6-1 Web UI 视频/图片增强
- 视频快捷键补全：倍速（[ / ] 步进 0.25）+ OSD 显示（现有已有 播放暂停/快进/音量/全屏/静音）
- 图片全屏观看（baseViewer 加 lightbox/全屏）
- 测试：node --test 纯函数 + Playwright e2e（Web UI 硬要求）
- 验收：视频倍速/图片全屏真浏览器可用

### P6-2 站点抓取走代理（sproxy+sclient）
- booksite RunScraper 改经代理（复用 downloader.proxy.list + force_proxy），用户通过
  `curl -x http://127.0.0.1:1080` 模式经 sproxy+sclient 加密隧道访问外网，避免管理员发现访问记录
- 代理故障 fail-closed（不静默回退直连）
- 文档：docs/proxy-setup.md（sproxy+sclient 配置指南）
- 测试：mock 代理服务器验证请求经代理
- 验收：抓取流量经代理，管理员只见出口节点

### P6-3 新站点任务（移至外部库 sdserver）
- 用户需求（2026-09-25）：新增外部站点任务（经 127.0.0.1:1080 代理访问，防访问记录暴露）
- **站点具体信息不存储在 download-manager**——实现移至外部库 `leon/cocomhub/sdserver`（internal/task/）
- 支持按番号、厂商、标签、类型爬取；支持番号提取
- 番号库机制（JSON 持久化），含 tktube 已抓取番号构建
- 去重：已知番号跳过；未知番号不去重但打 metadata.special_review=true（人工复核标记）
- 代理故障 fail-closed（不静默回退直连）

## 优先级矩阵（阶段 6）

| 优先级 | 项 | 理由 |
|--------|----|------|
| S1 高价值 | P6-2 | 用户核心诉求（防访问暴露） |
| S2 体验 | P6-1 | 视频/图片观看体验增强 |
