# PikPak 混合下载方案设计 v2（分享直链前段 + 账号流量后段，分片并行版）

> **落地注记（2026-10）**：本设计描述的是 **sproxy 侧** 的 PikPak hybrid 下载策略（`pkg/volume/ext/pikpak`）。
> dm 侧对接的下载器**已更名为 `sproxy_cloud`**（原 `sproxy_hybrid`）：
> - 命名：dm 实际调用的是 **sproxy cloud download API**（服务维度），`hybrid` 只是 PikPak 这一后端的策略名，不应出现在 dm 侧；
> - 能力：**通用**——任意 URL 均可提交（sproxy 按 URL 自动发现后端），PikPak 分享链接仅为特化优先来源，**并非只做 PikPak**；
> - 产物：默认转存到目标卷 **并下载到本地**（`cloud_only: false` 默认；设 `true` 只留云端）。

> 状态：v1 可行性已验证（真实账号实测）+ v2 吸收评审（纯 Go 确认、metrics、降级、删除、并行分片）
> 版本：v2（设计稿，待评审）

---

## 1. 关键设计决策（v2 修订）

### 1.1 纯 Go 分享直链逻辑 ✅（已确认）
签名算法（gopeed 扩展 index.js 抽离）：
```
captchaSign = "1." + MD5链( WEB_CLIENT_ID + WEB_CLIENT_VERSION + WEB_PACKAGE_NAME + deviceId + ts,
                           × 15 个 WEB_ALGORITHMS 逐轮 MD5 )
```
- MD5 是标准（Joseph Myers 版）→ **Go `crypto/md5` 直接实现**
- `encodeURIComponent` → `url.QueryEscape`；`fetch` → `net/http`
- **无任何 JS 特有依赖（无 Buffer/window/WebCrypto）** → **纯 Go 100% 可移植** ✅
- 落地：sproxy 新增 `pkg/volume/ext/pikpak/resolver.go`（纯 Go ShareResolver），gopeed 扩展逻辑等价移植

### 1.2 失败 metric + 降级完整下载（v2 新增）
**原则：分享直链任何失败 → 记 metric → 降级为完整账号下载（sproxy 现有链），不阻断任务**

| 事件 | metric | 行为 |
|---|---|---|
| 分享直链 resolve 失败 | `pikpak_share_resolve_failed` | 降级全账号流量（转存整文件） |
| 分享段 Range 失败（非 416） | `pikpak_share_segment_failed` | 记失败 → 该段转账号流量续传 |
| 416 边界误判（实际 416 提前） | `pikpak_share_boundary_overshoot` | 记录 → 下次调低系数 |
| 账号段失败 | `pikpak_account_segment_failed` | 重试/换账号 |
| 降级发生 | `pikpak_downgrade_total` | 可观测：方案长期性评估依据 |
| 分享段节省字节 | `pikpak_share_bytes_saved` | 白嫖收益量化 |

- 全部指标进 `CloudMetrics`（sproxy 已有 Prometheus 基础），cloud download 任务状态可查
- **降级路径 = 现有 PikpakDownloader 完整下载（转存→CLI/网盘直链全段）**，实现上 hybrid 失败 → 直接委托现有 downloader，零重复

### 1.3 分享直链特殊兼容（opt 扩展）
- 分享直链无 ETag → Range 续传**不能**靠 If-Range 校验 → 需 opt：
  - `RangeResumeCompatible` 模式：无 ETag 时跳过 If-Range（信任 Range 续传，靠分段 md5 校验兜底）
  - 分享直链 **expire 签名**（短时有效）→ 续传用新链接时**必须校验内容一致性**（首段 md5 对比）
- 新增 `downloader.Opt`（sproxy pkg/downloader 已有 options 机制？确认）+ pikpak 专用 opt 组

### 1.4 直链失败重取逻辑（v2 关键修订）
```
分享段下载中直链失败（expire 过期/网络）：
  ① 重新 resolve 拿新直链（同 shareID + fileID）
  ② 用新直链从失败 offset 续传
  ③ 若新直链**再次同样失败**（同样错误，如 416/403 持续）→ 该段转账号流量下载
     否则继续用新链接续传
```
- **不轻易降级**：单次直链失败先重取重试；连续两次同样错误才转账号段（账号流量更贵，尽量分享段完成）

### 1.5 删除操作修复（v2 确认缺口）
- sproxy 现有 `Delete()` = `batchTrash`（移回收站）→ **不释放空间**（实测 usage 不降）
- 修复：`DeletePermanent()` = `/drive/v1/files:batchDelete`（永久删，实测 usage → 0）
- AutoDelete 默认改 `permanent`（6GB 空间必须及时释放，否则转存大文件失败）

### 1.6 分片并行下载（v2 核心重构，替代单次拼接）
**放弃"前段整块 + 后段整块 + 拼接校验"**，改为**分片（chunk）下载**：

```
文件 [0, total)
  ├─ 分享直链区 [0, 50%): 切成 N 个 chunk，并行下载（匿名，免账号配额）
  ├─ 账号直链区 [50%, total): 切成 M 个 chunk，并行下载（账号流量，全 Range）
  └─ 每 chunk：独立 Range 请求 → 独立文件（chunk_i.part）

写入策略（二选一，v2 推荐 B）：
  A. chunk 文件 → 抽象文件 writer 按顺序读下一分片（拼接在读取时透明）
  B. ★预分配整个文件空间，各 chunk 按偏移 os.WriteAt 写入 → 完成后整体就是完整文件
```

**方案 B（预分配 + WriteAt）评估**：
| 维度 | 评估 |
|---|---|
| 并发可靠性 | ✅ `os.File.WriteAt` 线程安全（按 offset 写，无共享状态）；并行 chunk 各写各的偏移 |
| 预分配 | `os.Truncate(size)` 或 sparse 文件（NTFS/EXT 支持），不占实际空间直到写入 |
| 数据正确性 | 每 chunk 独立下载 + 下载后**校验（Content-Range 对齐 + 可选 md5）** → 全部成功 = 文件完整 |
| 失败处理 | chunk 失败 → 重试该 chunk（重取直链/换源），不影响其它 chunk |
| 完成判定 | 所有 chunk 成功落盘 → `io.Copy(os.WriteAt)` 已内嵌 → 直接算总 md5 或 trust |
| 读取 | 最终文件就是标准文件（无拼接层），任何 reader 直接用 |

- **方案 B 优于 A**（无拼接层、无顺序读取耦合、崩溃后各 chunk 可独立续传）
- 备选：**C. 分段文件 + manifest**（类似 aria2）——复杂，不选

### 1.7 边界规则（v2 明确）
- **分享直链区 = 恒 ≤ 50%**（`min(探测边界, total×0.5)`，即使实测 55% 也不超上限）→ 防边界波动/限制收紧
- **账号区 = 从 50% 开始**（保守，保证与分享区重叠或衔接，不依赖 416 精确边界）
- 分享区失败 chunk → 转账号区下载（账号流量兜底，数据完整优先）
- 核心目标：**数据最终正确完整**（每 chunk 成功 + 全量校验），非最优节省

---

## 2. 目标架构（分片并行版）

```
输入：分享 URL
  │
  ├─ ① ShareResolver.Resolve（纯 Go 匿名）→ 直链 + 元信息 + total
  │
  ├─ ② 规划：分享区 [0, 50%) N chunk / 账号区 [50%, total) M chunk
  │        └ chunkSize 可配（默认 64MB，平衡并发与恢复粒度）
  │
  ├─ ③ 预分配 destFile（os.Truncate(total)）
  │
  ├─ ④ 并行下载（worker pool，限并发）：
  │        ├─ 分享 chunk：分享直链 Range → WriteAt(offset)
  │        │     └ 失败 → 重取直链重试 → 连续两次同样错误 → 转账号直链
  │        └─ 账号 chunk：转存后网盘直链 Range → WriteAt(offset)
  │              └ 失败 → 重试/换账号
  │
  ├─ ⑤ 每 chunk 完成即校验（下载字节数 == chunk 大小 + 偏移对齐）
  │
  ├─ ⑥ 全部 chunk 成功 → 可选全文件 md5/ffprobe 校验 → 完成
  │
  └─ ⑦ DeletePermanent（永久删转存，释放 6GB 空间）
```

### 2.1 关键接口（Go）
```go
// 纯 Go 匿名分享解析（pkg/volume/ext/pikpak/resolver.go）
type ShareResolver struct { /* deviceId, captcha token, algorithms */ }
func (r *ShareResolver) Resolve(ctx, shareURL) (*ShareMeta, error)        // total + 文件列表
func (r *ShareResolver) DirectLink(ctx, shareID, fileID) (string, error)  // 匿名直链
func (r *ShareResolver) RefreshCaptcha(ctx) error

// 分片并行下载器（pkg/volume/ext/pikpak/hybrid.go）
type HybridDownloader struct {
    resolver  *ShareResolver
    api       *API          // 转存 + FETCH 直链 + DeletePermanent
    pool      *AccountPool  // 账号空间/配额轮换
    httpDL    *downloader.HTTPDownloader  // 复用 Range 续传
    chunkSize int64         // 默认 64MB
    shareRatio float64      // 分享区比例（恒 ≤0.5）
    concurrency int
    metrics   *HybridMetrics
}
func (d *HybridDownloader) Download(ctx, source, destPath, onProgress) (*Result, error)

// 下载器 Opt（分享直链特殊兼容）
type Opt func(*HybridConfig)
func WithNoETagResume() Opt          // 无 ETag 时跳过 If-Range（分享直链）
func WithShareRatio(r float64) Opt    // 分享区比例（默认 0.5，上限 0.5）
func WithChunkSize(sz int64) Opt
func WithAutoDeletePermanent() Opt

// 指标（进 CloudMetrics）
type HybridMetrics struct {
    ShareResolveFailed  atomic.Int64
    ShareSegmentFailed  atomic.Int64
    AccountSegmentFailed atomic.Int64
    DowngradeTotal      atomic.Int64   // 分享段失败转账号
    ShareBytesSaved     atomic.Int64   // 分享区实际下载字节
    BoundaryOvershoot   atomic.Int64
}
```

### 2.2 失败重试状态机（chunk 级）
```
chunk (offset, length, source=share|account)
  │
  ├─ 下载尝试：
  │     ├─ 成功 → 校验 → done
  │     └─ 失败：
  │           ├─ 重取直链（expire 过期/网络错）→ 新链接续传
  │           ├─ 新链接再次同样错误 → source→account（降级 chunk）
  │           └─ account 失败 → 重试/换账号
  │
  └─ 所有 chunk done → 全量校验（可选）→ 完成
```
- chunk 状态机独立（互不阻塞），worker pool 并发执行
- 直链重取：`DirectLink(shareID, fileID)` 重新 resolve（新 expire 签名）
- 降级判定：**连续两次同样错误**（同 error code，如 416/403）才转账号段

---

## 3. sproxy 侧改造

### 3.1 新增（纯 Go）
| 文件 | 内容 |
|---|---|
| `pkg/volume/ext/pikpak/resolver.go` | 纯 Go ShareResolver（captcha 签名 + share detail + file_info） |
| `pkg/volume/ext/pikpak/hybrid.go` | HybridDownloader（分片并行 + 降级 + 指标） |
| `pkg/volume/ext/pikpak/hybrid_test.go` | fake API + 分片/降级/重取测试 |
| `pkg/downloader/opts.go` | 下载器 Opt 机制（NoETagResume 等） |

### 3.2 修改
| 文件 | 改动 |
|---|---|
| `api.go:Delete` | 新增 `DeletePermanent()`（batchDelete），AutoDelete 默认 permanent |
| `pkg/cloud/manager.go` | HybridMetrics 并入 CloudMetrics + Prometheus 导出 |
| `downloader_register` | HybridDownloader Priority 高于现有 PikpakDownloader |
| `config.go` | pikpak.hybrid/share_ratio/chunk_size/auto_delete=permanent |

### 3.3 测试（TDD）
- 纯 Go 签名 vs gopeed 扩展签名（同一 share 输出直链一致性）
- 分片并行：fake server 多 chunk 并发 + WriteAt 偏移正确性
- 降级：分享 chunk 连续两错 → 账号段
- 重取：expire 过期 → 新链接续传；再次同样错误 → 降级
- 删除：batchDelete 后 usage 归 0（mock quota）
- 416 边界：分享区恒 ≤50%（即使探测到 55%）

---

## 4. download-manager 侧改造

> **落地注记（2026-10）**：实际实现与本节初稿不同——最终采用「提交到 sproxy 服务端 cloud download」
> 方案（`downloader/sproxy_cloud.go` + 配置段 `downloader.sproxy_cloud`），**不是**本节所写的
> 本地直接调用 `pkg/volume/ext/pikpak` 库（`downloader/pikpak_hybrid.go` / `downloader.pikpak:`）。
> 原因：dm 只做任务解析/提交/轮询，hybrid 分片/账号池/转存都在 sproxy 侧；本地直调库的路径由
> 独立 CLI `cmd/pikget` 承担。下文保留为设计演进记录。

### 4.1 新增 `downloader/pikpak_hybrid.go`（初稿；最终为 `downloader/sproxy_cloud.go`）
- 复用 sproxy `pkg/volume/ext/pikpak`（require+replace）
- 适配 core.Downloader（obj → shareURL → HybridDownloader.Download → obj.SavePath）
- njavtv magnet_list 分流：keepshare 分享 → hybrid；纯磁力 → gopeed
- config `downloader.pikpak:`（resolver/hybrid/chunk_size/share_ratio/auto_delete）

### 4.2 数据正确性保障
- 每 chunk：Range 响应验证（206 + Content-Range 对齐 offset）
- 全量：可选 `ffprobe`/md5（下载后校验，复用 e2e 教训）
- 崩溃恢复：chunk 已完成部分落盘（WriteAt 幂等），重启后扫描 offset 续下未完成 chunk

---

## 5. 风险与对策
| 风险 | 对策 |
|---|---|
| 分享直链限制收紧（<50%） | share_ratio 恒 0.5 上限 + 边界探测 + 降级观测（DowngradeTotal metric） |
| 并发 WriteAt 可靠性 | Go 标准 `os.File.WriteAt` 线程安全（POSIX pwrite）；测试并发写入校验 |
| 转存空间不足（6GB） | 分片只转账号区（≤50% 文件）→ 大文件 >12GB 才超空间；多账号轮换 |
| 直链 expire 过期 | chunk 级重取直链（新签名）→ 连续两错才降级 |
| 无 ETag 续传一致性 | 分享区每 chunk 首段 md5 校验（新链接 vs 旧 chunk 重叠区） |
| usage_in_trash 计数异常 | 改用 batchDelete 永久删（usage 实测归 0），不依赖 trash 计数 |

## 6. 落地顺序
| 阶段 | 内容 | 验证 |
|---|---|---|
| P0 | sproxy resolver.go（纯 Go 签名）+ DeletePermanent | 签名输出 vs gopeed 一致 |
| P0.5 | HybridDownloader 分片并行 + 降级 + 指标 | fake API TDD + 真实 1.28GB |
| P1 | download-manager pikpak_hybrid.go + 分流 + config | TDD + 真实小文件 |
| P2 | 多账号空间调度 + 并发调优 | 多账号实测 |
| P3 | 崩溃恢复 + 边界动态学习 | 长期稳定性 |

## 7. 代码锚点（更新）
| 能力 | 位置 |
|---|---|
| 签名算法（JS 源） | `gopeed-web/.../TeamBreakerr@gopeed-extension-pikpak/index.js`（captchaSign/WEB_ALGORITHMS） |
| sproxy 现有分享 API | `sproxy/pkg/volume/ext/pikpak/api.go`（RestoreShare/ShareDetail/ListShareRecursive） |
| sproxy 网盘直链 | `sproxy/pkg/volume/ext/pikpak/api.go:DownloadLink`（FETCH） |
| sproxy 账号池 | `sproxy/pkg/volume/ext/pikpak/account.go`（Select/Use） |
| sproxy Delete（改永久） | `sproxy/pkg/volume/ext/pikpak/api.go:396`（batchTrash → +batchDelete） |
| sproxy Range 续传 | `sproxy/pkg/downloader/http_downloader.go`（.partial + If-Range + 416） |
| sproxy CloudMetrics | `sproxy/pkg/cloud/manager.go:221` |
| dm gopeed 分流 | `download-manager/downloader/gopeed.go:Download` |
| dm 自研 Range | `download-manager/pkg/download/http_extractor.go` |
| njavtv 磁力元数据 | `sdserver/internal/task/njavtv/detail_upgrade.go:magnet_list` |

---

## 8. 实测结论补充（2026-10-04 真实链路，v3）

### 8.1 带宽实测（所有通道）

| 通道 | 单线程速率 | 并发 | 结论 |
|---|---|---|---|
| 分享直链（匿名） | ~1.2MB/s | **不加速**（每连接独立限速，总吞吐不增） | 免账号配额，Range 0-55% |
| 账号 FETCH | ~1.3MB/s | 不加速 | 全 Range，需转存+token |
| CLI 官方通道 | ~1.15MB/s | 单线程 | OAuth，无并发选项 |
| CLI play URL | ~1.2MB/s | 不加速 | 同 CDN 流媒体 |

**关键**：免费账号/CDN 全局限速 ~1.2MB/s（单连接），**任何通道并发都不加速**。
之前误判"账号直链 0.2MB/s 慢"= 链接过期/网络抖动（expire 签名短时有效）。

### 8.2 hybrid 慢的原因（40min vs CLI 19min）
- 16MB chunk 每请求 30-60s（CDN 限速下 16MB 需 ~13s，实际 + 握手/慢启动）
- 分池并发 2 太低 + HTTP 连接复用不足
- **总吞吐仍受账号 1.2MB/s 限制** → 分片并发无法突破

### 8.3 真实优化空间
1. **多账号轮换**（每账号独立 1.2MB/s）→ 2 账号 2.4MB/s（sproxy AccountPool 已支持）——**唯一真实提速手段**
2. 连接复用 + 小 chunk + 高并发 → 减少单请求时长（总吞吐不变）
3. **分享区免配额是核心价值**（省 6GB 空间/日流量），速度非卖点

### 8.4 CLI 能力挖掘
- `task add`：磁力云端下载（绕分享 416，占网盘空间）——大文件空间不够时不可用
- `play --url-only`：同 CDN 限速，无特殊
- `download`：单文件无并发/续传（中断从头）
- `mcp`：CLI 可作 MCP server（未来集成）
- CLI 不支持 Range 续传 → 直链（Range）才是续传方案

### 8.5 转存幂等（已实现）
- restore 前 FindInDrive（同名+同大小精确匹配）→ 命中则跳过 RestoreShare 直接复用
- 避免多次 restore 累积同名副本 (1)(2)(3) 占满 6GB
- 测试：TestHybridDownload_RestoreIdempotent

### 8.6 性能优化后续（P2）
- [ ] hybrid 连接复用（http.Transport keep-alive 调优）
- [ ] 多账号并行下载账号区（每账号独立 chunk 池）
- [ ] 分享区免配额收益 metrics（ShareBytesSaved 已加）
- [ ] 分享直链 416 边界动态探测（避免超限重试）
