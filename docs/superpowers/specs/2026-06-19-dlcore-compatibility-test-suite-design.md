# dlcore 兼容性测试套件设计

## 背景

`pkg/dlcore` 已标记废弃，由 `pkg/download` 取代。但缺乏完善的机制确保 `pkg/download` 的行为是 `dlcore` 的超集 ——
`dlcore` 的客户代码（如 `downloader/adapter.go`、`cmd/scraper_get`）依赖其行为边界，部分边缘行为在替换后可能发生
不符合预期的变化。

需要一套系统化的测试套件，以 `dlcore` 行为为基线，验证 `pkg/download` 的行为一致（或明确记录差异）。

## 设计目标

1. **行为基准**：以 `dlcore.Client` 的全部行为为参考标准，`pkg/download` 可以是超集，但 `dlcore` 能过的场景 `pkg/download` 也必须能过
2. **差异显式化**：每个已知差异（如 `maxRetries=0` 语义、`Metadata["status"]` 写入）有独立的测试用例标注为 dlcore-only
3. **覆盖率**：覆盖 dlcore 全部 65+ 导出/非导出符号的核心行为路径
4. **可维护性**：新增字段或行为时，测试框架能轻松扩充

## 架构

### 位置

在现有 `downloader/` 包内扩展，复用已有的 `Comparator` 基础设施和 `Beacon` HTTP 测试服务器。

```
downloader/
├── adapter_contract_test.go       ← 已有，补充用例
├── adapter_functional_test.go     ← 已有，补充用例
├── adapter_e2e_test.go            ← 已有，补充用例
├── adapter_compatibility_test.go  ← 新建：dlcore 兼容性专项
├── adapter_featuregap_test.go     ← 已有，补充用例
└── adapter_dlcore_only_test.go   ← 新建：dlcore 特有能力验证
```

### 测试分组（10 Groups，约 57 个用例）

#### Group A — 基础下载契约（Contract）— 6 用例

| 编号 | 测试 | 双方期望 | 说明 |
|------|------|----------|------|
| A1 | 空 URL / 空 SavePath 返回错误 | 双方返回 error | 输入验证 |
| A2 | 正常下载成功，文件内容一致 | 字节逐位相同 | 核心契约 |
| A3 | 文件大小一致 | 字节数相同 | 文件完整性 |
| A4 | Progress 回调到达 100% | `OnProgress(100,...)` 至少调用一次 | 进度报告 |
| A5 | Metadata 中包含 `total_size` | key 存在且值匹配实际字节数 | 元数据 |
| A6 | 对入参 `req.URL` / `req.SavePath` 无副作用 | 调用后值不变 | 副作用控制 |

#### Group B — MD5 校验 — 8 用例

| 编号 | 测试 | 双方期望 | 说明 |
|------|------|----------|------|
| B1 | `X-Amz-Meta-Md5chksum` → MD5 匹配 → 成功 | 下载完成 + metadata 有 md5 | 标准 MD5 |
| B2 | `Etag`（34 字符 hex）→ MD5 匹配 | 同上 | Etag 提取 |
| B3 | `Content-MD5` → MD5 匹配 | 同上 | Content-MD5 提取 |
| B4 | MD5 不匹配 → 截断 + 完整重新下载 | 最终文件匹配期望内容 | MD5 保护 |
| B5 | MD5 不匹配超上限 → 返回错误 | 返回错误 | 重试上限 |
| B6 | 空内容 + Etag 匹配 → 跳过 | 成功，不实际下载 | 条件跳过 |
| B7 | 断点续传 + MD5 验证（Content-Length == startOffset）| dlcore 跳过；pkg/download 可能跳过 | 续传 MD5 |
| B8 | 续传 + MD5 不匹配 → 截断重下 | 完整重新下载 | 续传 MD5 保护 |

#### Group C — 错误码与重试 — 10 用例

| 编号 | 测试 | 双方期望 | 说明 |
|------|------|----------|------|
| C1 | 403 → `ErrNoTry` | `errors.Is(err, ErrNoTry)` | 永久终止 |
| C2 | 404 → `ErrNoTry` | 同上 | 永久终止 |
| C3 | 500 → 可重试 | 非 `ErrNoTry` | 可重试 |
| C4 | 503 → 可重试 | 非 `ErrNoTry` | 可重试 |
| C5 | 416 → 重置 offset=0 重试 | 最终成功下载 | 范围不满足 |
| C6 | 不支持 Range → 全量重下 | 最终成功下载 | 回退 |
| C7 | `maxRetries=0` | **dlcore-only**: 无限重试；pkg/download: 不重试 | 差异测试 |
| C8 | `maxRetries=3` → 超上限 | 返回 error | 上限 |
| C9 | 502 → 返回可重试错误 | 非 `ErrNoTry` | 可重试 |
| C10 | 退避行为 | 两次尝试间有时间间隔 | 退避策略 |

#### Group D — Content-Type 检测 — 3 用例

| 编号 | 测试 | 双方期望 | 说明 |
|------|------|----------|------|
| D1 | `text/html` + `.mp4` URL → `ErrNoTry` | 双方一致 | 双方都拦截 |
| D2 | `text/html` + `.jpg` URL → `ErrNoTry` | 双方一致 | 双方都拦截 |
| D3 | `text/plain` + `.mp4` URL | **dlcore-only**: dlcore 拦截；pkg/download 不拦截 | 差异测试 |

#### Group E — 断点续传 — 5 用例

| 编号 | 测试 | 双方期望 | 说明 |
|------|------|----------|------|
| E1 | 支持 Range → 从 offset 续传 | 文件完整 | 标准续传 |
| E2 | 不支持 Range（200 无 Content-Range）→ 全量重下 | 最终文件完整 | 回退 |
| E3 | 续传时服务器内容变更（Content-Length < offset）→ 重置 | 完整重下 | 变更检测 |
| E4 | 续传 + Progress 从 offset 跟踪 | 进度值合理 | 进度准确 |
| E5 | 416 + 已下载部分 → 重置 | 最终成功 | 边界处理 |

#### Group F — 路径与文件系统 — 5 用例

| 编号 | 测试 | 双方期望 | 说明 |
|------|------|----------|------|
| F1 | 相对路径 → rootDir 内 | 正确位置 | 路径解析 |
| F2 | `../escape.txt` → 拒绝 | 双方拒绝 | 路径穿越保护 |
| F3 | 空 rootDir → 原样使用 | 路径按原样 | 无根路径 |
| F4 | 输出目录自动创建 | 目录存在 | 目录创建 |
| F5 | 日志文件创建（设 logDir） | 日志文件存在 | 日志 |

#### Group G — 元数据副作用 — 6 用例

| 编号 | 测试 | 双方期望 | 说明 |
|------|------|----------|------|
| G1 | `Metadata["total_size"]` == 实际字节数 | 双方一致 | 大小 |
| G2 | `Metadata["status"] == "completed"` | **dlcore-only**: dlcore 写入 | 差异测试 |
| G3 | `Metadata["md5_base64"]`（MD5 匹配时）| 双方一致 | MD5 元数据 |
| G4 | `Metadata["md5_hex"]`（MD5 匹配时）| 双方一致 | MD5 元数据 |
| G5 | `Metadata["mod_time"]`（Last-Modified 存在时）| 双方一致 | 时间 |
| G6 | 失败时 metadata 不变 | 不写入任何完成标记 | 失败安全 |

#### Group H — 并发与控制 — 6 用例

| 编号 | 测试 | 双方期望 | 说明 |
|------|------|----------|------|
| H1 | 取消活跃下载 | 上下文取消 + 文件部分存在 | 取消 |
| H2 | 取消不存在的下载 | 返回错误 | 取消不存在 |
| H3 | 域名限流 | 不超过并发上限 | 限流 |
| H4 | 多并发隔离 | 互不干扰 | 隔离 |
| H5 | 图片 URL 30s 超时 | **dlcore-only**: dlcore 有超时 | 差异测试 |
| H6 | huaacg.com 5s 超时 + ErrNoTry | **dlcore-only**: dlcore 有 | 差异测试 |

#### Group I — 请求处理 — 4 用例

| 编号 | 测试 | 双方期望 | 说明 |
|------|------|----------|------|
| I1 | 自定义请求头 | 全部发送 | 头注入 |
| I2 | 浏览器标头注入 | 含有 sec-ch-ua 等 | 浏览器标头 |
| I3 | 自定义头覆盖浏览器头 | 同名：自定义覆盖 | 优先级 |
| I4 | User-Agent 自定义值 | 使用指定 UA | UA 覆盖 |

#### Group J — 进度回调行为 — 4 用例

| 编号 | 测试 | 双方期望 | 说明 |
|------|------|----------|------|
| J1 | `TrackProgress=false` → 不触发 | 回调不被调用 | 关闭 |
| J2 | `OnProgress=nil` → 不 panic | 无 panic | nil 安全 |
| J3 | 零字节目标 → 至少触发一次 | 至少一次 0→100 | 零字节 |
| J4 | `total=0` 时触发 progress | **dlcore-only**: dlcore 仍触发；pkg/download 不触发 | 差异测试 |

### dlcore-only 测试机制

使用 `DlcoreOnlyRun` 辅助方法，仅构造 `dlcore.Client` 执行测试，同时收集 `pkg/download` 的表现用于日志记录：

```go
func (c *Comparator) DlcoreOnlyRun(t *testing.T, name string, obj *model.DownloadObject,
    headers map[string]string, expectedErrCheck func(error) bool) {
    t.Run(name+"_[dlcore-only]", func(t *testing.T) {
        // 1. 运行 dlcore
        oldResult := c.runOld(obj, headers)
        // 2. 断言 dlcore 行为满足预期
        if !expectedErrCheck(oldResult.Err) { t.Errorf("dlcore: unexpected error: %v", oldResult.Err) }
        // 3. 运行 pkg/download 记录参考
        newResult := c.runNew(obj, headers)
        t.Logf("pkg/download behavior for reference: err=%v, size=%d", newResult.Err, newResult.FileSize)
    })
}
```

### 已知差异表

最终交付附带以下差异表（写在文档中，也作为测试的 `TestKnownDifferences` 用例验证）：

| 差异 | dlcore | pkg/download | 测试引用 |
|------|--------|-------------|----------|
| `maxRetries=0` | 无限重试 | 不重试 | C7 |
| `Metadata["status"]` | 写入 `"completed"` | 不写入 | G2 |
| `text/plain` + `.mp4` | `ErrNoTry` | 不拦截 | D3 |
| 图片 URL 30s 超时 | 启用 | 不启用 | H5 |
| huaacg.com 5s 超时 | 启用 | 不启用 | H6 |
| progress 在 total=0 时 | 仍触发 | 不触发 | J4 |
| `Metadata["status"]` == `"completed"` | 写入 | 不写入 | G2 |

## Commit & PR 策略

按测试 Group 的逻辑关联度分 3 批提交和 PR，每批可独立审查和合并：

### PR 1：核心契约（Groups A + C + F + I）— ~25 用例
- A 组（基础下载契约）：新增文件 `adapter_contract_ext_test.go`，覆盖 A1-A6
- C 组（错误码与重试）：在 `adapter_functional_test.go` 中补充 C1-C10
- F 组（路径与文件系统）：在 `adapter_functional_test.go` 中补充 F1-F5
- I 组（请求处理）：在 `adapter_functional_test.go` 中补充 I1-I4
- 需要新增 `DlcoreOnlyRun` 辅助方法、扩展 ComparatorOptions 支持 `WithMaxRetries(0)` 模式
- **定位**：验证下载核心路径一致，风险最低，优先合并

### PR 2：数据完整性（Groups B + D + E + G）— ~22 用例
- B 组（MD5 校验）：在 `adapter_functional_test.go` 中补充 B1-B8
- D 组（Content-Type 检测）：在 `adapter_functional_test.go` 中补充 D1-D3
- E 组（断点续传）：在 `adapter_functional_test.go` 中补充 E1-E5
- G 组（元数据副作用）：在 `adapter_functional_test.go` 中补充 G1-G6
- 扩展 Comparator 支持 MD5 头部注入的 Beacon `HandleWithMD5`
- **定位**：验证文件完整性保障一致，涉及较多 Beacon 注入模式

### PR 3：并发 + dlcore-only 边缘（Groups H + J + 差异表）— ~10 用例
- H 组（并发与控制）：在 `adapter_contract_test.go` 中补充 H1-H6
- J 组（进度回调）：在 `adapter_e2e_test.go` 中补充 J1-J4
- dlcore-only 特有能力：新建 `adapter_dlcore_only_test.go`，覆盖 C7/D3/G2/H5/H6/J4
- **定位**：并发测试耗时较长（需 `go test -race`），dlcore-only 测试不阻塞主线合并

### 提交流程
每批 PR 遵循 CLAUDE.md 的提交前标准化流程：
1. `go fix ./...`
2. `go fmt ./...`
3. `addlicense -c "The Cocomhub Authors. All rights reserved." -s=only .`
4. `go build ./...`
5. `go test -race -count=1 -timeout=180s ./downloader/...`

### 成功标准

1. **所有共享测试**（非 dlcore-only）双方 pass
2. **所有 dlcore-only 测试** dlcore 端 pass，pkg/download 端记录表现
3. **无 panic** 在任何一条测试路径上
4. **构建通过**：`go build ./downloader/...`
5. **无 data race**：`go test -race -count=1 ./downloader/...`
