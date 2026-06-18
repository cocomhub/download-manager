# Design Spec: Downloader 行为一致性测试套件

> **日期**: 2026-06-19
> **作者**: Claude Code
> **状态**: 设计稿

## 1. 背景与目标

### 1.1 问题

`pkg/dlcore/` 包已标记废弃，被 `pkg/download` 取代。当前通过 `downloader/adapter.go` 的 `DownloaderAdapter` 将新路径包装为 `core.Downloader` 接口，但**缺乏系统性的行为一致性验证**。

现有的测试覆盖：
- `pkg/dlcore/` 只有 2 个测试文件（HLS URL 检测 + 路径安全），**无下载行为测试**
- `downloader/downloader_test.go` 只测试工厂函数创建，**不测试实际下载**
- `pkg/download/` 有内部单元测试，但**不验证与 dlcore 的兼容性**（gapline）

### 1.2 目标

设计一套**分层测试套件**，以 `dlcore` 的行为为基准，验证 `pkg/download`（新路径）是完全替代品。核心原则：

- **dlcore 行为是 pkg/download 的子集**：pkg/download 必须能做一切 dlcore 能做的事
- **输入等价**：同一请求参数 → 相同失败/成功模式
- **输出等价**：文件内容一致、Metadata 关键字段一致
- **副作用等价**：进度回调、文件系统写入、日志行为一致
- **pkg/download 允许更多特性**：但不破坏 dlcore 已有的行为

### 1.3 不包含的范围

- 不修改 `pkg/dlcore/` 代码（标记废弃，只读）
- 不修改 `pkg/download/` 核心逻辑（只添加测试）
- 不测试外部进程依赖（wget、ffmpeg 子进程）
- 不依赖外部网络（全部使用 `httptest.Server`）

## 2. 架构

### 2.1 文件结构

```
downloader/
├── beacon_test.go                   # 测试夹具（httptest.Server + Comparator）
├── adapter_contract_test.go         # 契约级测试
├── adapter_functional_test.go       # 功能级对比测试
├── adapter_featuregap_test.go       # 特性差距测试
├── adapter_e2e_test.go              # 端到端测试
└── adapter_composite_test.go        # 复合下载测试
```

### 2.2 总体架构图

```ascii
┌──────────────────────────────────────────────────────────────┐
│                     Test File                                │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  Comparator.Run(name, url, savePath, checks...)       │   │
│  │  ┌────────────────┐  ┌──────────────────────────┐    │   │
│  │  │ oldDL (dlcore) │  │ newDL (pkg/download)     │    │   │
│  │  │ Download(obj)  │  │ Download(obj)            │    │   │
│  │  └────┬───────┬───┘  └───────┬──────────────────┘    │   │
│  │       │       │              │                       │    │
│  │       ▼       ▼              ▼                       │   │
│  │  ┌────────────────────────────────────────────────┐  │   │
│  │  │          Beacon (httptest.Server)               │  │   │
│  │  │  FileHandler / RangeHandler / ErrorHandler ...  │  │   │
│  │  └────────────────────────────────────────────────┘  │   │
│  │                                                       │   │
│  │  ┌────────────────────────────────────────────────┐  │   │
│  │  │          Check Verdicts                        │  │   │
│  │  │  ✓ CheckError  ✓ CheckFile  ✓ CheckMetadata    │  │   │
│  │  │  ✓ CheckProgress  ✓ CheckNoSideEffect          │  │   │
│  │  └────────────────────────────────────────────────┘  │   │
│  └──────────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────────┘
```

### 2.3 设计模式：统一对比运行器

核心思路：一个 `Comparator` 辅助结构体，同时构造 `NativeHTTPDownloader`（dlcore 后端）和 `DownloaderAdapter`（pkg/download 后端），对同一 HTTP handler 执行两次下载，逐字段比对待测试的维度。

优势：
- 一次测试覆盖两个实现
- 发现回归时立刻知道哪个实现有问题
- 减少重复代码
- 天然保证对比完整性

## 3. 基础设施组件

### 3.1 Beacon — 可编程 HTTP 测试服务器

> 文件：`beacon_test.go`

基于 `httptest.Server` 构建，提供预置的 handler 工厂，精确控制每个测试场景的 HTTP 响应。

**核心 API：**

```go
type Beacon struct {
    t        *testing.T
    srv      *httptest.Server
    mu       sync.Mutex
    handlers map[string]beaconHandler // path -> handler
    requests []*http.Request          // 记录所有收到的请求
}

func NewBeacon(t *testing.T) *Beacon
func (b *Beacon) URL() string
func (b *Beacon) Reset()                    // 清空请求记录
func (b *Beacon) Requests() []*http.Request  // 返回副本
func (b *Beacon) Close()
```

**预置 Handler 工厂：**

| 工厂函数 | 用途 | HTTP 行为 |
|---|---|---|
| `FileHandler(content, contentType)` | 正常文件下载 | 200 OK, Content-Length |
| `RangeHandler(content)` | Range 支持 | 206 Partial, 解析 Range 头切片 |
| `ErrorHandler(statusCode)` | 错误码 | 403 / 404 / 416 / 500 |
| `ChunkedHandler(totalBytes, chunkSize, interval)` | 分块返回 | 逐步写入，触发进度回调 |
| `MD5Handler(content, md5Source, match)` | MD5 头 | 设置 X-Amz-Meta-Md5chksum / Etag / Content-MD5 |
| `ResumeHandler(content, breakPoint)` | 断点续传 | 首请求返回部分后中断，次请求带 Range 返回剩余 |
| `TextContentHandler()` | Content-Type 检测 | text/html，测试 mp4 URL 的检测逻辑 |
| `SlowHandler(content, delay)` | 慢响应 | 延迟 delay 后返回 |
| `ChunkedTransferHandler(content)` | 无 Content-Length | Transfer-Encoding: chunked |
| `ProxyHandler(proxyPath)` | 代理路径验证 | 记录请求路径，验证代理拼接 |

**被动记录：**
- 记录所有收到的 HTTP 请求（方法、路径、header、body）
- 测试可以通过 `Beacon.Requests()` 断言请求头、查询参数等

### 3.2 Comparator — 双实现对比运行器

> 文件：`beacon_test.go`

**核心 API：**

```go
type Check func(t *testing.T, oldResult, newResult *DownloadResult)

type Comparator struct {
    t         *testing.T
    beacon    *Beacon
    oldDL     core.Downloader  // NativeHTTPDownloader (dlcore)
    newDL     core.Downloader  // DownloaderAdapter (pkg/download)
    oldClient *dlcore.Client   // 底层 dlcore 引用
}

func NewComparator(t *testing.T, opts ...ComparatorOption) *Comparator
func (c *Comparator) Run(name string, obj *model.DownloadObject, headers map[string]string, checks ...Check)
```

**执行流程：**

1. 创建测试对象 `model.DownloadObject`（URL、SavePath、Metadata、Extra）
2. `t.Run("old_" + name, ...)` — 用 oldDL 下载，记录结果
3. `t.Run("new_" + name, ...)` — 用 newDL 下载，记录结果
4. 对每个 Check，执行对比断言

**预置 Check 函数：**

| Check | 对比内容 | 实现 |
|---|---|---|
| `CheckError()` | 错误类型一致 | `errors.Is` 比对新旧 error 是否同类型 |
| `CheckFileBytes()` | 文件内容完全一致 | `bytes.Equal` 比较写入磁盘的内容 |
| `CheckFileSize()` | 文件大小一致 | 比较 `os.Stat` 的 Size |
| `CheckMetadata(keys...)` | Metadata 指定 key 一致 | 按 key 比对新旧 Metadata value |
| `CheckProgress()` | 进度回调序列 | 记录回调参数，比较序列模式 |
| `CheckNoSideEffect()` | 无副作用 | URL / SavePath / Headers 未被修改 |
| `CheckAnyError()` | 新旧都返回 error（不要求相同） | 用于允许实现差异的场景 |

**Comparator 配置选项：**

```go
func WithConfig(cfg config.Downloader) ComparatorOption       // 共享配置
func WithRootDir(dir string) ComparatorOption                 // 文件根目录
func WithLogDir(dir string) ComparatorOption                  // 日志目录
func WithMaxRetries(n int) ComparatorOption                   // 最大重试
func WithDomainLimits(limits map[string]int) ComparatorOption  // 域名限制
```

Comparator 内部将同一配置同时应用于：
1. `NewNativeHTTPDownloader(cfg)` — 构造旧实现
2. `newDownloaderFromConfig(cfg)` — 构造新实现（通过 `NewDownloaderAdapter`）

### 3.3 测试对象工厂

```go
func makeTestObject(url, savePath string, metadata map[string]string, extra map[string]any) *model.DownloadObject {
    obj := model.NewDownloadObject("test-task", url, savePath)
    obj.Metadata = metadata
    obj.Extra = extra
    return &obj
}
```

## 4. 测试用例设计

### 4.1 层次1：契约级测试（adapter_contract_test.go）

**目标**：验证 `core.Downloader` 接口的基本契约，确保新旧实现都符合接口约定。

共 **9 个测试**：

| # | 测试 | 场景 | Check 维度 |
|---|---|---|---|
| 1 | `TestDLContract_EmptyURL` | URL="" | Error (non-nil), 文件未创建 |
| 2 | `TestDLContract_EmptySavePath` | SavePath="" | Error (non-nil), 文件未创建 |
| 3 | `TestDLContract_Success` | 正常下载，文件内容"hello world" | Error=nil, FileBytes 正确 |
| 4 | `TestDLContract_ProgressCalled` | TrackProgress 启用 | Progress 起始 0, 最终 100, 至少 2 次回调 |
| 5 | `TestDLContract_MetadataPopulated` | 下载完成 | Metadata 含 md5_base64, md5_hex, total_size, status |
| 6 | `TestDLContract_NoSideEffect` | 下载前后检查入参 | URL, SavePath, Headers 不变 |
| 7 | `TestDLContract_Cancel` | 下载中取消 | 返回 error |
| 8 | `TestDLContract_DomainLimit` | 域名并发限制 | 同一域名并发 2 个下载，Acquire/Release 正确 |
| 9 | `TestDLContract_ConcurrentDownload` | 不同域名并发 | 全部成功 |

### 4.2 层次2：功能级对比测试（adapter_functional_test.go）

**目标**：逐功能对比 dlcore 和 pkg/download 的行为一致性。

共 **18 个测试**，分 6 组：

#### 组A：请求头（3 tests）

| 测试 | 场景 | 验证方法 |
|---|---|---|
| `TestFunc_HeaderInjection` | 默认浏览器头注入 | Beacon.Requests() 含 sec-ch-ua, sec-fetch-* 等 |
| `TestFunc_HeaderInjectionDisabled` | DisableInjectBrowserLikeHeaders | 请求无浏览器类头 |
| `TestFunc_CustomHeaders` | req.Headers 覆盖 | 自定义头在请求中存在且覆盖默认值 |

#### 组B：断点续传（4 tests）

| 测试 | 场景 | Check 维度 |
|---|---|---|
| `TestFunc_ResumeNormal` | 文件存在 50%，Range 续传剩余 | FileBytes, FileSize, Beacon 收到 Range 头 |
| `TestFunc_ResumeCompleted` | 文件已完整，Content-Length == startOffset | 跳过 (error=nil)，无 HTTP 请求 |
| `TestFunc_ResumeServerNoSupport` | 服务器返回 200（非 206） | 从头下载，FileBytes 正确 |
| `TestFunc_Resume416` | 416 Range Not Satisfiable | 重置 offset，重试成功 |

#### 组C：重试（2 tests）

| 测试 | 场景 | Check 维度 |
|---|---|---|
| `TestFunc_MaxRetries` | 超过 maxRetries 次失败 | 返回 ErrNoTry 包装错误 |
| `TestFunc_RetryOnMD5Mismatch` | 首次 MD5 不匹配，重试成功 | 最终 FileBytes 正确 |

#### 组D：MD5 校验（4 tests）

| 测试 | 场景 | 验证方法 |
|---|---|---|
| `TestFunc_MD5_XAmzMeta` | X-Amz-Meta-Md5chksum 头 | Metadata 含 md5_base64, md5_hex |
| `TestFunc_MD5_Etag` | Etag 头（34 字符弱 etag） | Metadata 含 md5_hex |
| `TestFunc_MD5_ContentMD5` | Content-MD5 头（32 字符 hex） | Metadata 含 md5_hex |
| `TestFunc_MD5_Mismatch` | MD5 头与内容不匹配 | 触发截断重试 |

#### 组E：错误码 + 路径安全（4 tests）

| 测试 | 场景 | Check 维度 |
|---|---|---|
| `TestFunc_403NoRetry` | 服务器返回 403 | ErrNoTry |
| `TestFunc_404NoRetry` | 服务器返回 404 | ErrNoTry |
| `TestFunc_TextContentType` | text/html + URL 含 .mp4 | ErrNoTry |
| `TestFunc_PathTraversal` | SavePath="../evil" | Error (路径越界拒绝) |

#### 组F：日志（1 test）

| 测试 | 场景 | 验证方法 |
|---|---|---|
| `TestFunc_LogFileCreated` | 配置 logDir | 日志文件存在且非空 |

### 4.3 层次3：特性差距测试（adapter_featuregap_test.go）

**目标**：
- 验证配置迁移正确性
- 验证 pkg/download 的额外特性不破坏 dlcore 行为
- 验证 pkg/download 能做 dlcore 所有事

共 **6 个测试**：

| # | 测试 | 场景 | 验证 |
|---|---|---|---|
| 1 | `TestFeatureGap_ConfigTypeMigration` | native_http → native_old | ValidateAndClamp 正确转换类型 |
| 2 | `TestFeatureGap_ConfigFieldMigration` | 旧字段→新子结构 | Filesystem.LogDir, FFmpeg.Path 正确填充 |
| 3 | `TestFeatureGap_ProgressTuning` | 进度调优参数 | 新旧实现进度行为一致 |
| 4 | `TestFeatureGap_DownloadAllDlcoreTypes` | dlcore 支持的所有 HTTP 状态码/错误 | pkg/download 有相同处理 |
| 5 | `TestFeatureGap_ExtraMetrics` | pkg/download 暴露 Metrics | 不报错，不影响下载结果 |
| 6 | `TestFeatureGap_MetadataFlusher` | 立即持久化回调 | 注册回调后下载成功，回调被触发 |

### 4.4 层次4：端到端测试（adapter_e2e_test.go）

**目标**：完整下载流程验证。

共 **6 个测试**：

| # | 测试 | 场景 | 步骤 |
|---|---|---|---|
| 1 | `TestE2E_NormalDownload` | 完整流程 | 构建 beacon → Comparator.Run → 验证文件/Metadata/Progress |
| 2 | `TestE2E_ResumeInterrupted` | 中断恢复 | 首下载中断 → 检查部分文件 → 续传 → 验证完整 |
| 3 | `TestE2E_ZeroByteFile` | 空文件 | Content-Length:0 → 创建空文件，Metadata total_size=0 |
| 4 | `TestE2E_ChunkedTransfer` | 分块传输编码 | Transfer-Encoding: chunked → 文件完整 |
| 5 | `TestE2E_ServerErrorRecovery` | 临时错误恢复 | 前 2 次 500 → 第 3 次 200 → 最终成功 |
| 6 | `TestE2E_AuthHeaders` | 认证头传递 | 传入 Bearer token → 服务器收到 Authorization 头 |

### 4.5 复合下载测试（adapter_composite_test.go）

**目标**：测试 `obj.Extra["files"]` 逻辑（仅 pkg/download 路径，dlcore 的 NativeHTTPDownloader 也有此能力）。

共 **5 个测试**：

| # | 测试 | 场景 | 验证 |
|---|---|---|---|
| 1 | `TestComposite_SingleVideo` | 1 个 video 子文件 | 进度追踪，文件完整 |
| 2 | `TestComposite_MultipleFiles` | 3 个子文件（cover+video+subtitle） | 全部下载成功，内容一致 |
| 3 | `TestComposite_EmptyFilesList` | 空列表 | ErrNoTry |
| 4 | `TestComposite_PartialFail` | 第 2 个子文件失败 | 返回 error, 第 1 个文件保留 |
| 5 | `TestComposite_MetadataPrefix` | 子文件前缀元数据 | cover_etag, video_checksum 等正确存储 |

## 5. 配置与构造

### 5.1 Comparator 构造细节

```go
func NewComparator(t *testing.T, beacon *Beacon, opts ...ComparatorOption) *Comparator {
    t.Helper()
    
    cfg := config.Downloader{
        Type: "native",          // 新路径（默认）
        MaxRetries: 0,
        Filesystem: config.DcFilesystem{
            RootDir:  t.TempDir(),
            LogDir:   t.TempDir(),
            CacheDir: t.TempDir(),
        },
        HTTP: config.DcHTTP{
            TimeoutSeconds: 30,
            DefaultUserAgent: "TestAgent/1.0",
        },
    }
    // 应用 opts...
    
    // 构造旧路径：NativeHTTPDownloader（内部使用 dlcore.Client）
    oldDL := NewNativeHTTPDownloader(cfg)
    
    // 构造新路径：DownloaderAdapter + pkg/download
    newDL := newDownloaderFromConfig(cfg)
    adapter := NewDownloaderAdapter(newDL)
    
    return &Comparator{
        t: t, beacon: beacon,
        oldDL: oldDL,
        newDL: adapter,
    }
}
```

### 5.2 Config 构造注意事项

- `cfg.Type` 对旧路径固定为 `"native_old"`，对新路径固定为 `"native"`
- 日志目录使用独立的 `t.TempDir()` 避免新旧实现日志相互干扰
- `MaxRetries` 在所有重试测试中显式设置，避免默认无限重试导致测试超时
- 域名并发测试设置 `DomainLimits` 确保限流生效

## 6. 实施计划

### 6.1 执行顺序

```
Phase 1: 基础设施
  beacon_test.go  (约 150 行)
  └── 无依赖，最先实现

Phase 2: 核心契约
  adapter_contract_test.go  (约 200 行)
  └── 依赖 Phase 1，最快验证基础设施可用

Phase 3: 功能对比（工作主体）
  adapter_functional_test.go  (约 400 行)
  └── 依赖 Phase 1 + 2，最复杂的部分

Phase 4: 特性差距
  adapter_featuregap_test.go  (约 150 行)
  └── 独立于 Phase 3

Phase 5: 端到端 + 复合下载
  adapter_e2e_test.go  (约 200 行)
  adapter_composite_test.go  (约 150 行)
  └── 依赖 Phase 1
```

### 6.2 涉及文件

- 仅创建新文件：`downloader/` 下 6 个 `_test.go` 文件
- 不修改任何非测试文件

### 6.3 验证方法

```bash
# 逐层验证
go test -v -count=1 -run 'TestDLContract_' ./downloader/        # 契约
go test -v -count=1 -run 'TestFunc_' ./downloader/              # 功能
go test -v -count=1 -run 'TestFeatureGap_' ./downloader/        # 特性差距
go test -v -count=1 -run 'TestE2E_' ./downloader/               # 端到端
go test -v -count=1 -run 'TestComposite_' ./downloader/         # 复合下载

# 全部 + race
go test -race -count=1 -timeout=180s ./downloader/

# 不破坏现有测试
go test -race -count=1 ./...
```

### 6.4 已知注意事项

1. **文件编码**：PowerShell 可能默认 UTF-16 LE + BOM，优先使用 bash sed 或 Edit 工具编辑测试文件
2. **metadata 写锁**：测试中读 `obj.Metadata` 需要 `obj.RLock()/RUnlock()`，写需要 `obj.Lock()/Unlock()`
3. **dlcore 和 pkg/download 的 SavePath 解析差异**：dlcore 使用 `ResolvePath(rootDir, SavePath)`，pkg/download 在 extractor 内也做相似操作。测试需确保 rootDir 一致
4. **maxRetries=0 语义差异**：dlcore 中 maxRetries=0 表示"无限制"，pkg/download 可能表示"不重试"
5. **进度回调签名一致**：新旧实现的 OnProgress 签名都是 `(progress float64, downloaded int64, total int64)`，对比时直接比较参数值
6. **Metadata 的 key 不完全一致**：dlcore 写入 `md5_base64`、`md5_hex`，pkg/download 也有类似字段。对比使用 `CheckMetadata("md5_hex", "total_size", "status")` 指定子集
7. **断点续传策略差异**：dlcore 先发 HEAD 探测再发 GET，pkg/download 直接用 GET+Range。这是实现细节差异，只要最终效果一致即可

## 7. 测试用例覆盖率矩阵

| dlcore 功能 | 契约测试 | 功能测试 | 特性差距 | E2E | 复合 |
|---|---|---|---|---|---|
| URL/SavePath 空值验证 | ✓ | | | | |
| 图片 URL 30s 超时 | | (跳过—依赖网络) | | | |
| Metadata nil 初始化 | ✓ | | | | |
| Status=Completed 跳过 | | ✓(ResumeCompleted) | | | |
| handler 分发 | | (内部实现细节) | | | |
| 路径安全拼接 | | ✓ | | | |
| 日志文件写入 | | ✓ | | | |
| 代理拼接 | | (Proxy path 验证) | | ✓ | |
| 断点续传 | | ✓ | | ✓ | |
| 重试循环 | | ✓ | | ✓ | |
| 域名并发限制 | ✓ | | | | |
| 浏览器头注入 | | ✓ | | | |
| 403/404 → ErrNoTry | | ✓ | | | |
| Content-Type 检测 | | ✓ | | | |
| 416 → 重置 offset | | ✓ | | | |
| 进度回调 | ✓ | | | | |
| MD5 校验+重试 | | ✓ | | | |
| Metadata 写入 | ✓ | ✓ | | ✓ | |
| FFmpeg HLS 下载 | | (跳过—外部进程) | | | |
| m3u8d HLS 下载 | | (跳过—外部依赖) | | | |
| moveIfExists | | | ✓ | | |
| ExternalHLSLog | | | ✓ | | |
| 代理选择器 | | ✓ | | | |
| 取消 (Cancel) | ✓ | | | | |
| 复合下载 (Extra.files) | | | | | ✓ |
| 配置迁移 | | | ✓ | | |
| 额外特性 (ETag/Metrics) | | | ✓ | | |

> **跳过说明**：图片 URL 超时依赖特定域名模式、FFmpeg 和 m3u8d 依赖外部二进制，不适合在纯单元测试中验证。可由 E2E 测试框架（Playwright）覆盖。

## 8. 测试文件模板

### 8.1 beacon_test.go 结构

```go
package downloader_test

// Beacon: programmable HTTP test server
type Beacon struct {
    srv      *httptest.Server
    t        *testing.T
    mu       sync.Mutex
    handlers map[string]http.HandlerFunc
    requests []*http.Request
}

func NewBeacon(t *testing.T) *Beacon { ... }
func (b *Beacon) ServeHTTP(w http.ResponseWriter, r *http.Request) { ... }
func (b *Beacon) URL() string { return b.srv.URL }
func (b *Beacon) Reset() { ... }
func (b *Beacon) Requests() []*http.Request { ... }
func (b *Beacon) Close() { ... }

// Handler factories
func (b *Beacon) HandleFile(path, content, contentType string) { ... }
// ... 其余工厂函数

// Comparator
type Comparator struct { ... }
func NewComparator(t *testing.T, beacon *Beacon, opts ...ComparatorOption) *Comparator { ... }
func (c *Comparator) Run(name string, obj *model.DownloadObject, headers map[string]string, checks ...Check) { ... }

// Pre-built checks
func CheckError() Check { ... }
func CheckFileBytes() Check { ... }
// ... 其余 Check 工厂
```

### 8.2 契约测试结构示例

```go
func TestDLContract_Success(t *testing.T) {
    t.Parallel()
    beacon := NewBeacon(t)
    defer beacon.Close()
    beacon.HandleFile("/file.txt", "hello world", "text/plain")

    cmp := NewComparator(t, beacon)
    obj := makeTestObject(beacon.URL()+"/file.txt", "out/file.txt", nil, nil)
    cmp.Run("success", obj, nil, CheckError(), CheckFileBytes())
}
```

## 9. 附录

### 9.1 dlcore Option → pkg/download 配置映射

| dlcore Option | pkg/download 等价配置 |
|---|---|
| `WithHTTPClient(c)` | `HTTP.*` 字段 + StdlibTransport 构造 |
| `WithRootDir(dir)` | `Filesystem.RootDir` |
| `WithLogDir(dir)` | `Filesystem.LogDir` |
| `WithCacheDir(dir)` | `Filesystem.CacheDir` |
| `WithProxies(proxies)` | `Proxy.List` |
| `WithForceProxy(force)` | `Proxy.Force` |
| `WithMaxRetries(n)` | `MaxRetries` |
| `WithFFmpegPath(path)` | `FFmpeg.Path` |
| `WithHLSAutoMarkAsFail(v)` | `FFmpeg.HLSAutoMarkAsFail` |
| `WithDefaultUserAgent(ua)` | `HTTP.DefaultUserAgent` |
| `WithDisableInjectBrowserLikeHeaders(v)` | `HTTP.DisableInjectBrowserLikeHeaders` |
| `WithProxyTuning(ttl, probe, suffix)` | `Proxy.{DecisionCacheTTL,DirectProbeTimeout,BandwidthPathSuffix}` |
| `WithProgressTuning(step, interval)` | `Progress.{MinPercentStep,MaxIntervalSeconds}` |
| `WithFFmpegExtraArgs(args)` | `FFmpeg.ExtraArgs` |

### 9.2 dlcore Metadata 字段清单

| Key | 来源 | 格式 |
|---|---|---|
| `status` | dlcore httpHandler.Download | `StatusCompleted` |
| `md5_base64` | 响应头 > computeFileMD5 | Base64 编码 |
| `md5_hex` | 响应头 > computeFileMD5 | 32 字符 hex |
| `mod_time` | Last-Modified 响应头 | RFC1123 格式 |
| `total_size` | os.Stat(file).Size() | 字符串形式数字 |
