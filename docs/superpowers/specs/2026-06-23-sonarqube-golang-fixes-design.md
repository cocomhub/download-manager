# SonarQube Go 问题修复与常量提取设计

## 概述

基于 SonarQube Cloud 扫描结果（1461 issues），对 `download-manager` 项目中的 Go 代码问题进行针对性修复，并将重复字符串提取为命名常量。修复拆分为 2 个 PR，共 12 个 commit，便于逐步 review 和合并。

## 修复策略

- **排除**：73 个 `go:S3776` 认知复杂度问题（需大规模重构，单独处理）、接口命名 `godre:S8196`（变更影响面大）、Context 字段 `godre:S8242`（跨包 API 变更）
- **排除**：非 Go 文件（JavaScript/Shell/CI YAML）问题
- **优先**：低风险、编译器可验证的问题（内联变量、命名返回值、重复分支、重复函数体）

### 按严重度分布

| 严重度 | 数量 | 本次处理 |
|--------|------|---------|
| BLOCKER | 1 | 否（JS 文件） |
| CRITICAL | 93 | 否（73 个 S3776 跳过） |
| MAJOR | 43 | 部分（仅 Go 代码） |
| MINOR | 36 | 部分（S8193/S8209/S4144/S8184/S1135 等） |

## PR 1：Go 代码简化修复（6 commits）

### Commit 1: 内联多余变量声明 (S8193)

**规则**：`godre:S8193` — Remove this unnecessary variable declaration

**修改文件**：

1. `downloader/native.go:137-139`
   ```go
   // 修改前
   var fileList []map[string]string
   if typeName := fmt.Sprintf("%T", filesVal); typeName == "primitive.A" {
   // 修改后
   if fmt.Sprintf("%T", filesVal) == "primitive.A" {
   ```

2. `pkg/download/http_extractor_test.go:198-199`
   ```go
   // 修改前
   var capturedEtag string
   ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
   // 修改后：删除 unused 声明
   ```

3. `model/object_meta_test.go:12`
   ```go
   // 修改前
   var o *DownloadObject
   if tags := o.GetTags(); tags != nil {
   // 修改后
   if tags := (*DownloadObject)(nil).GetTags(); tags != nil {
   ```

4. `storage/mongo_storage_test.go:41,44`
   ```go
   // 修改前
   if got := filter["task_id"]; got == nil {
   // 修改后
   if filter["task_id"] == nil {
   ```

### Commit 2: 命名未命名返回值参数 (S8209)

**规则**：`godre:S8209` — Group together these consecutive parameters of the same type

**修改文件**：

1. `downloader/scraper.go:21,38,55` — 给返回 `(string, error)` 命名
   ```go
   // 修改前
   func Scrape(url string, cookie string) (string, error) {
   // 修改后
   func Scrape(url string, cookie string) (body string, err error) {
   ```

2. `task/path_strategy.go:25`
   ```go
   // 修改前
   func (s *simplePathStrategy) Resolve(baseDir string, ...) (string, string) {
   // 修改后
   func (s *simplePathStrategy) Resolve(baseDir string, ...) (videoPath string, imagePath string) {
   ```

3. `api/server.go:142`
   ```go
   func coalesce(s string, def string) string {
   ```
   单返回值无需命名，用 `//nolint:godre` 抑制。

4. `core/extra_interfaces.go:14`
   ```go
   // 修改前
   Resolve(baseDir string, taskID string, title string, fileType string) (string, string)
   // 修改后
   Resolve(baseDir string, taskID string, title string, fileType string) (dir string, filename string)
   ```

### Commit 3: 删除重复的 switch 分支 (S1871)

**规则**：`go:S1871` — This branch's code block is the same as the block for the branch

**修改文件**：`main.go:83-88`

```go
// 修改前
res.RunMode = config.RunModeFull
switch strings.ToLower(runMode) {
case "full":
    res.RunMode = config.RunModeFull  // 重复赋值
case "ui":
    res.RunMode = config.RunModeUI
}

// 修改后
switch strings.ToLower(runMode) {
case "ui":
    res.RunMode = config.RunModeUI
default:
    res.RunMode = config.RunModeFull
}
```

### Commit 4: 消除重复函数体 (S4144)

**规则**：`go:S4144` — Update this function so that its implementation is not identical

**修改文件**：`task/base_task.go:542`

```go
// 删除 syncSharedToObjectLocked 的重复体，改为调用 SyncSharedToObject
func (b *BaseTask) syncSharedToObjectLocked(obj *model.DownloadObject) {
    b.SyncSharedToObject(obj)  // delegate
}
```

注意：该函数名含 "Locked" 但函数体本身并未持锁，锁在 `applySharedState` 内部。这是保持向后兼容的最小修改。

### Commit 5: 解决 TODO + 空函数注释 + 空白导入注释 (S1135, S1186, S8184)

**规则**：
- `go:S1135` — Complete the task associated to this TODO comment
- `go:S1186` — Add a nested comment explaining why this function is empty
- `godre:S8184` — Add a comment explaining why this blank import is needed

**修改文件**：

1. `downloader/adapter.go:186`
   ```go
   // TODO(MigrationCleanup): remove after dlcore deprecation — compatibility field
   ```
   评估：该兼容字段在 dlcore 废弃期间仍有价值，将 TODO 升级为带有编号/目标版本的说明注释：
   ```go
   // Compatibility shim: removed when dlcore is fully removed.
   ```

2. `pkg/download/extractor/hls.go:69` — 空函数加注释说明计划实现
   ```go
   // Placeholder: HLS-specific cleanup will be added when HLS extraction
   // is fully migrated from pkg/dlcore.
   ```

3. `pkg/download/extractor/wget.go:81` — 同上

4. `task/tktube/player_util_embed.go:6`
   ```go
   // blank import registers the embed of player_util.js at compile time.
   ```

### Commit 6: 使用安全的缓存目录替代 TempDir (S5445)

**规则**：`go:S5445` — Make sure to not create this file in a predictable and publicly writable path

**修改文件**：

1. `pkg/dlcore/proxy_selector.go:98`
2. `pkg/download/proxy_selector.go:78`

```go
// 修改前
cacheBase = filepath.Join(os.TempDir(), ".dm-proxy-cache")

// 修改后
cacheDir, err := os.UserCacheDir()
if err != nil {
    cacheDir = os.TempDir()
}
cacheBase = filepath.Join(cacheDir, "dm-proxy-cache")
_, _ = os.Stat(cacheBase) // 原有调用不变
```

注意：目录创建权限已在原有代码的 `os.MkdirAll` 中处理，只需确认其配置了 `0700`。

## PR 2：重复字符串提取为命名常量（6 commits）

### 通用原则

1. **先查后用**：提取前检查是否已有现成常量可用
2. **语义命名**：常量名需清晰表达用途，如 `MetadataKeyStatus` 而非 `KStatus`
3. **就近放置**：优先放在最相关的包中，避免创建无意义的 `const.go` 文件
4. **不用 const 文件**：如 `download-manager` 自建约定所示，不建全局 `const.go`，按职责放

### Commit 7: 替换 status 字面量为 model.Status* 常量

**分析**：`model/status.go` 已有：
```go
StatusCompleted = "completed"
StatusFailed    = "failed"
```

**修改文件**（~11 处）：

- `downloader/adapter.go` — `obj.Metadata["status"] = "completed"` → `model.StatusCompleted`
- `manager/events.go` — `summary["completed"]` → `model.StatusCompleted`
- `manager/manager.go` — `summary["completed"]` → `model.StatusCompleted`
- `manager/metrics.go` — metrics key（不更改，metrics key 不是 status）
- `downloader/adapter.go` — Metadata["status"] 写入

注意：`api/server_metrics.go` 和 `cmd/playwright-server/fixture/datasets.go` 中用作 JSON 键或 fixture 数据的 `"status"` 不替换。

### Commit 8: 提取元数据键常量

**新建文件**：`model/meta_keys.go`

```go
package model

// Metadata keys used across task types.
const (
    MetadataKeyTitle       = "title"
    MetadataKeyContentGroup = "content_group"
    MetadataKeyType        = "type"
    MetadataKeyStatus      = "status"
)
```

**修改文件**：
- `model/object_meta.go`, `task/*/task.go`, `manager/aggregate.go`, `storage/query.go`, `manager/events.go`, `manager/manager.go`

逐个替换 `obj.Metadata["title"]` → `obj.Metadata[MetadataKeyTitle]`。

### Commit 9: 提取 HTTP 头常量

**分析**：`api/errors.go` 已有 `hdrContentType = "Content-Type"`（包私有）。

**方案**：保持 `hdrContentType` 私有，在 `api/errors.go` 补充：
```go
const (
    hdrCacheControl = "Cache-Control"
    hdrNoCache      = "no-cache"
)
```

跨包使用场景较多，考虑建 `pkg/httputil/headers.go`：
```go
package httputil

const (
    HeaderUserAgent    = "User-Agent"
    HeaderContentType  = "Content-Type"
    HeaderCacheControl = "Cache-Control"
    CacheNoCache       = "no-cache"
    CacheNoStore       = "no-store"
)
```

但项目已约定不做 `pkg` 层薄封装（来自 CLAUDE.md 架构约定）。因此将 HTTP 头常量放在 `api/errors.go` 或就近。

**修改文件**：
- `api/server_task.go` — `"Cache-Control"` / `"no-cache"`
- `downloader/scraper.go` — `"cache-control"`（小写 → 统一使用常量，但保留 header 大小写规范，HTTP 头不区分大小写）
- `pkg/dlcore/client.go`, `pkg/download/http_extractor.go` — 同上

### Commit 10: 提取 API 错误码常量

**分析**：`api/server_task.go` 和 `api/server_config.go` 中大量使用：
- `"invalid_request"` — 18 次
- `"update_failed"` — 6 次

**方案**：在 `api/errors.go` 中补充：
```go
const (
    errCodeInvalidRequest = "invalid_request"
    errCodeUpdateFailed   = "update_failed"
)
```

### Commit 11: 提取日志属性键常量

**新建文件**：`pkg/logutil/keys.go`

```go
package logutil

// Log attribute keys used across the application.
const (
    LogKeyTaskID = "task_id"
    LogKeyError  = "error"
    LogKeyURL    = "url"
    LogKeyStatus = "status"
)
```

**修改文件**（~148 处）：

这涉及大量文件，通过脚本 + manual review 处理。主要在以下文件中替换：
- `api/server_metrics.go`, `server_task.go`, `server_config.go`
- `downloader/adapter.go`, `native.go`, `wget.go`
- `manager/download.go`, `aggregate.go`, `manager.go`
- `pkg/download/http_extractor.go`
- `pkg/dlcore/handler.go`
- 以及其他 `slog.Any("task_id", ...)` / `slog.String("error", ...)` 调用

**注意**：这是变更量最大的 commit，需要确认是否值得做，因为 `slog` 属性键字符串常量的好处主要是防拼写错误。

### Commit 12: 提取时间格式和 URL 路径常量

**分析**：

1. `"20060102150405"` — 日志文件名时间戳格式，5 处使用
   - 该格式`YYYYMMDDHHMMSS`无分隔符，可读性差
   - 提取为 `const logTimestampFormat = "20060102150405"` 并附注释说明这是 Go 参考时间格式

2. `"/bandwidth"` — 代理带宽探测 URL 路径后缀，7 处使用
   - 提取为 `const ProxyBandwidthPath = "/bandwidth"` 放 `config/config.go`

3. `sec-ch-ua` 完整值字符串 "Google Chrome"... — 4 处使用
   - 提取为浏览器常量

## 验证方法

### PR 1 验证
```bash
# 每个 commit 提交前运行
cd download-manager
go fmt ./...
go build ./...
golangci-lint run ./...
go test -race -count=1 ./...
```

### PR 2 验证
```bash
# 同上 + 确认无拼写错误
go vet ./...
go test -race -count=1 ./...
```

### 特别关注
- S8193 内联变量：确认编译通过
- S8209 命名返回值：确认现有调用方不受影响（Go 中命名/未命名在调用方无差别）
- S5445 路径变更：确认 Windows 和 Linux 都走通
- 日志属性键：134 处替换，用 `grep` 确认无遗漏
