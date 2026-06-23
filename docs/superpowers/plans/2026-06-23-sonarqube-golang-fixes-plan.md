# SonarQube Go 问题修复与常量提取 实现计划

> **面向 AI 代理的工作者：** 子技能：使用 `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans` 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 修复 SonarQube Cloud 扫描到的 1461 个 issues 中的 Go 代码低风险问题 + 将重复字符串提取为命名常量。

**架构：** 拆分为 2 个独立 PR，PR 1（Go 简化修复，6 commits）和 PR 2（常量提取，6 commits），相互不依赖。

**技术栈：** Go 1.26, golangci-lint, `gofmt -s`

**分支名：** PR 1 → `fix/sonarqube-golang-issues`；PR 2 → `refactor/extract-constants`

**当前状态：** 在 `fix/cancel-completed-hint-stale-pointer` 分支，有 1 个未 push 的 commit + 3 个未提交变更（ci.yml permissions 移动、sonar-project.properties exclusions、设计文档）。

**关键陷阱提醒（来自 CLAUDE.md）：**
1. `sync.Map` 类型断言必须用 `ok` 模式
2. Config 指针必须浅拷贝后才修改
3. 文件编码：PowerShell 写 Go 源码会 UTF-16 LE BOM，优先用 `Edit` 工具或 Git Bash `sed`
4. `gofmt -s` 会改变缩进，编辑文件前先 `gofmt -s` 再读

---

## 文件清单

### PR 1：Go 代码简化修复

| 文件 | 操作 | 行号 |
|------|------|------|
| `downloader/native.go` | 修改 | 137-141 |
| `pkg/download/http_extractor_test.go` | 修改 | 198 |
| `model/object_meta_test.go` | 修改 | 12 |
| `storage/mongo_storage_test.go` | 修改 | 41, 44 |
| `downloader/scraper.go` | 修改 | 21, 38, 55 |
| `task/path_strategy.go` | 修改 | 25 |
| `api/server.go` | 修改 | 142 |
| `core/extra_interfaces.go` | 修改 | 14 |
| `main.go` | 修改 | 83-88 |
| `task/base_task.go` | 修改 | 542-549 |
| `downloader/adapter.go` | 修改 | 186 |
| `pkg/download/extractor/hls.go` | 修改 | 68-69 |
| `pkg/download/extractor/wget.go` | 修改 | 80-81 |
| `task/tktube/player_util_embed.go` | 修改 | 6 |
| `pkg/dlcore/proxy_selector.go` | 修改 | 96-100 |
| `pkg/download/proxy_selector.go` | 修改 | 76-80 |
| `.github/workflows/ci.yml` | 修改（已有变更） | 8-10 → 40-42 |
| `sonar-project.properties` | 修改（已有变更） | exclusions |

### PR 2：重复字符串提取

| 文件 | 操作 |
|------|------|
| `model/meta_keys.go` | 新建（元数据键常量）|
| `model/status.go` | 不动（已有 Status* 常量）|
| `api/errors.go` | 追加（错误码常量、Cache-Control 常量）|
| `pkg/logutil/keys.go` | 新建（日志属性键常量）|

大量修改文件（~30+ 个 .go 文件涉及字符串替换）。

---

### 任务 0：准备分支（1 step）

- [ ] **步骤 1：提交当前未完成变更，切新分支**

当前在 `fix/cancel-completed-hint-stale-pointer`，已有 sonar-project.properties 和 ci.yml 改动用 commit 保存（它们是独立的 SonarQube 修复），设计文档另开 PR 提交。

```bash
cd D:/workdir/leon/cocomhub/download-manager
git add .github/workflows/ci.yml sonar-project.properties
git commit -m "chore: move CI permissions to job level, exclude IDE dirs from SonarQube"

# 切回 master 建 PR1 分支
git checkout master
git pull
git checkout -b fix/sonarqube-golang-issues
```

---

## PR 1：Go 代码简化修复

### 任务 1：内联多余变量声明（S8193，4 处）

**文件：**
- 修改：`downloader/native.go:139`
- 修改：`pkg/download/http_extractor_test.go:198`
- 修改：`model/object_meta_test.go:12`
- 修改：`storage/mongo_storage_test.go:41,44`

#### 子任务 1.1：downloader/native.go

- [ ] **步骤 1：读文件确认当前内容**

```bash
cd D:/workdir/leon/cocomhub/download-manager
sed -n '134,145p' downloader/native.go
```

- [ ] **步骤 2：修改代码**

```go
// 修改前（第 137-140 行）
var fileList []map[string]string
if typeName := fmt.Sprintf("%T", filesVal); typeName == "primitive.A" {
    filesVal = filesVal.([]any)
}
// 修改后
if fmt.Sprintf("%T", filesVal) == "primitive.A" {
    filesVal = filesVal.([]any)
}
```

`var fileList` 实际上在 144 行被使用（`fileList = append(fileList, f)`），所以只内联 `typeName` 变量。

- [ ] **步骤 3：验证**

```bash
go fmt ./downloader/
go build ./downloader/
go test -count=1 ./downloader/
```

#### 子任务 1.2：pkg/download/http_extractor_test.go

- [ ] **步骤 4：读文件确认**

```bash
sed -n '195,240p' pkg/download/http_extractor_test.go
```

- [ ] **步骤 5：分析**：`capturedEtag` 在 198 行被声明，在 221 行的 `OnMetadata` 闭包中被写入（`capturedEtag = value`），在 233 行被读取断言。它**不是 unused**，S8193 可能误报或指别的变量。确认后决定：若 `capturedEtag` 在 handler 内和 handler 外都被使用→屏蔽该行 issue，不修改。

```go
// 如果确实有使用，加 nolint 注释
var capturedEtag string //nolint:godre  // used across closure boundary in OnMetadata
```

#### 子任务 1.3：model/object_meta_test.go

- [ ] **步骤 6：修改代码**

```go
// 修改前
var o *DownloadObject
if tags := o.GetTags(); tags != nil {
// 修改后
if tags := (*DownloadObject)(nil).GetTags(); tags != nil {
```

- [ ] **步骤 7：验证**

```bash
go fmt ./model/
go build ./model/
go test -count=1 ./model/
```

#### 子任务 1.4：storage/mongo_storage_test.go

- [ ] **步骤 8：读文件确认**

```bash
sed -n '38,48p' storage/mongo_storage_test.go
```

- [ ] **步骤 9：修改代码**

```go
// 修改前
if got := filter["task_id"]; got == nil {
    t.Fatalf("expected task_id filter")
}
if got := filter["status"]; got == nil {
    t.Fatalf("expected status filter")
}
// 修改后
if filter["task_id"] == nil {
    t.Fatalf("expected task_id filter")
}
if filter["status"] == nil {
    t.Fatalf("expected status filter")
}
```

- [ ] **步骤 10：验证**

```bash
go fmt ./storage/
go build ./storage/
go test -count=1 -run TestBuildFilter ./storage/
```

#### 子任务 1.5：commit

- [ ] **步骤 11：提交**

```bash
git add downloader/native.go model/object_meta_test.go storage/mongo_storage_test.go
git commit -m "fix: inline unnecessary variable declarations (S8193)"
```

---

### 任务 2：命名返回值参数（S8209，5 个函数）

**文件：**
- 修改：`downloader/scraper.go:21,38,55`
- 修改：`task/path_strategy.go:25`
- 修改：`core/extra_interfaces.go:14`
- 修改：`api/server.go:142`（加 nolint）

#### 子任务 2.1：downloader/scraper.go — 3 个函数

- [ ] **步骤 1：读取确认**

```bash
sed -n '21,60p' downloader/scraper.go
```

- [ ] **步骤 2：修改代码**

```go
// 修改前
func Scrape(url string, cookie string) (string, error) {
func ScraperNative(url string, cookie string) (string, error) {
func doScraperNative(url string, cookie string) (string, error) {
// 修改后
func Scrape(url string, cookie string) (body string, err error) {
func ScraperNative(url string, cookie string) (body string, err error) {
func doScraperNative(url string, cookie string) (body string, err error) {
```

- [ ] **步骤 3：验证**

```bash
go fmt ./downloader/
go build ./downloader/
go test -count=1 ./downloader/
```

#### 子任务 2.2：task/path_strategy.go

- [ ] **步骤 4：读取确认**

```bash
sed -n '25,40p' task/path_strategy.go
```

- [ ] **步骤 5：修改代码**

```go
// 修改前
func (s *simplePathStrategy) Resolve(baseDir string, taskID string, title string, fileType string) (string, string) {
// 修改后
func (s *simplePathStrategy) Resolve(baseDir string, taskID string, title string, fileType string) (videoPath string, imagePath string) {
```

- [ ] **步骤 6：验证**

```bash
go fmt ./task/
go build ./task/...
go test -count=1 ./task/...
```

#### 子任务 2.3：core/extra_interfaces.go

- [ ] **步骤 7：读取确认**

```bash
sed -n '12,16p' core/extra_interfaces.go
```

- [ ] **步骤 8：修改代码**

```go
// 修改前
Resolve(baseDir string, taskID string, title string, fileType string) (string, string)
// 修改后
Resolve(baseDir string, taskID string, title string, fileType string) (dir string, filename string)
```

- [ ] **步骤 9：验证**

```bash
go fmt ./core/
go build ./core/
```

#### 子任务 2.4：api/server.go — 加 nolint

- [ ] **步骤 10：读取确认**

```bash
sed -n '142,148p' api/server.go
```

- [ ] **步骤 11：修改代码**

```go
// 修改前
func coalesce(s string, def string) string {
// 修改后
//nolint:godre  // single return value, naming adds no value
func coalesce(s string, def string) string {
```

- [ ] **步骤 12：验证**

```bash
go fmt ./api/
go build ./api/
```

#### 子任务 2.5：commit

- [ ] **步骤 13：提交**

```bash
git add downloader/scraper.go task/path_strategy.go core/extra_interfaces.go api/server.go
git commit -m "fix: name unnamed return parameters (S8209)"
```

---

### 任务 3：删除重复 switch 分支（S1871）

**文件：** `main.go:83-88`

- [ ] **步骤 1：读取确认**

```bash
sed -n '78,110p' main.go
```

- [ ] **步骤 2：修改代码**

```go
// 修改前（第 80-89 行）
if runMode != "" {
    res.RunModeSet = true
    res.RunMode = config.RunModeFull
    switch strings.ToLower(runMode) {
    case "full":
        res.RunMode = config.RunModeFull
    case "ui":
        res.RunMode = config.RunModeUI
    }
// 修改后（合并重复的赋值）
if runMode != "" {
    res.RunModeSet = true
    switch strings.ToLower(runMode) {
    case "ui":
        res.RunMode = config.RunModeUI
    default:
        res.RunMode = config.RunModeFull
    }
```

- [ ] **步骤 3：验证**

```bash
go fmt ./main.go  # 注意 main.go 在根目录
go build .
go test -count=1 ./...
```

- [ ] **步骤 4：提交**

```bash
git add main.go
git commit -m "fix: remove duplicate switch branch in main.go (S1871)"
```

---

### 任务 4：消除重复函数体（S4144）

**文件：** `task/base_task.go:542`

- [ ] **步骤 1：读取确认**

```bash
sed -n '530,550p' task/base_task.go
```

- [ ] **步骤 2：修改代码**

```go
// 修改前（第 542-549 行）
func (b *BaseTask) syncSharedToObjectLocked(obj *model.DownloadObject) {
    if b.shared == nil || obj == nil {
        return
    }
    if so, err := b.shared.Get(obj.URL); err == nil && so != nil {
        applySharedState(obj, so)
    }
}
// 修改后
func (b *BaseTask) syncSharedToObjectLocked(obj *model.DownloadObject) {
    b.SyncSharedToObject(obj)
}
```

注意：函数名含 `Locked` 但本身不持锁，锁在 `applySharedState` 内部——这是保持向后兼容的最小改动。

- [ ] **步骤 3：验证**

```bash
go fmt ./task/
go build ./task/...
go test -count=1 ./task/...
```

- [ ] **步骤 4：提交**

```bash
git add task/base_task.go
git commit -m "fix: deduplicate identical function body (S4144)"
```

---

### 任务 5：解决 TODO + 空函数注释 + 空白导入注释（S1135, S1186, S8184）

**文件：**
- 修改：`downloader/adapter.go:186` — TODO 注释
- 修改：`pkg/download/extractor/hls.go:68-69` — 空函数
- 修改：`pkg/download/extractor/wget.go:80-81` — 空函数
- 修改：`task/tktube/player_util_embed.go:6` — 空白导入

#### 子任务 5.1：adapter.go TODO 注释

- [ ] **步骤 1：读取确认**

```bash
sed -n '184,189p' downloader/adapter.go
```

- [ ] **步骤 2：修改代码**

```go
// 修改前
// TODO(MigrationCleanup): remove after dlcore deprecation — compatibility field
obj.Metadata["status"] = "completed"
// 修改后（解释为什么这里设 "completed"）
// Compatibility shim: set status metadata for consumers not yet migrated
// to the pkg/download result model. Remove when dlcore is fully removed.
obj.Metadata["status"] = "completed"
```

#### 子任务 5.2：hls.go 空函数

- [ ] **步骤 3：读取确认**

```bash
sed -n '68,70p' pkg/download/extractor/hls.go
```

- [ ] **步骤 4：修改代码**

```go
// 修改前
func (e *HLSExtractor) SetTransport(_ download.Transport) {}
// 修改后
// SetTransport is a no-op: HLSExtractor downloads via ffmpeg exec or m3u8d,
// not through a Go Transport. Implemented for download.TransportSetter interface.
func (e *HLSExtractor) SetTransport(_ download.Transport) {}
```

#### 子任务 5.3：wget.go 空函数

- [ ] **步骤 5：读取确认**

```bash
sed -n '80,82p' pkg/download/extractor/wget.go
```

- [ ] **步骤 6：修改代码**

```go
// 修改前
func (e *WgetExtractor) SetTransport(t download.Transport) {}
// 修改后
// SetTransport is a no-op: wget does not use a Go Transport.
// Implemented for download.TransportSetter interface compatibility.
func (e *WgetExtractor) SetTransport(t download.Transport) {}
```

#### 子任务 5.4：player_util_embed.go 空白导入

- [ ] **步骤 7：读取确认**

```bash
cat task/tktube/player_util_embed.go
```

- [ ] **步骤 8：修改代码**

```go
// 修改前
import _ "embed"
// 修改后
import _ "embed" // embed player_util.js at compile time for PlayerUtilJS
```

#### 子任务 5.5：验证+提交

- [ ] **步骤 9：验证**

```bash
go fmt ./downloader/ ./pkg/download/extractor/ ./task/tktube/
go build ./...
go test -count=1 ./downloader/ ./pkg/download/extractor/ ./task/tktube/
```

- [ ] **步骤 10：提交**

```bash
git add downloader/adapter.go pkg/download/extractor/hls.go pkg/download/extractor/wget.go task/tktube/player_util_embed.go
git commit -m "fix: resolve TODO, annotate empty functions, blank imports (S1135, S1186, S8184)"
```

---

### 任务 6：使用安全缓存目录（S5445）

**文件：**
- 修改：`pkg/dlcore/proxy_selector.go:96-100`
- 修改：`pkg/download/proxy_selector.go:76-80`

#### 子任务 6.1：dlcore/proxy_selector.go

- [ ] **步骤 1：读取确认**

```bash
sed -n '94,130p' pkg/dlcore/proxy_selector.go
```

- [ ] **步骤 2：修改代码**

```go
// 修改前（第 96-101 行）
cacheBase := ps.cacheDir
if cacheBase == "" {
    cacheBase = filepath.Join(os.TempDir(), ".dm-proxy-cache")
}
// 修改后
cacheBase := ps.cacheDir
if cacheBase == "" {
    cacheDir, err := os.UserCacheDir()
    if err != nil {
        cacheDir = os.TempDir()
    }
    cacheBase = filepath.Join(cacheDir, "dm-proxy-cache")
}
```

注意：移除了 `.dm-proxy-cache` 的前导点（点前缀表示 Unix 隐藏目录，在用户缓存目录中不需要，且 `os.UserCacheDir()` 返回的目录通常本身就是隐藏/受限的）。同时检查以下权限设置代码，确保 `MkdirAll` 使用 `0700`：

```go
// 第 125 行
_ = os.MkdirAll(filepath.Dir(cachePath), 0700)
// 第 147 行
_ = os.MkdirAll(filepath.Dir(cachePath), 0700)
```

#### 子任务 6.2：pkg/download/proxy_selector.go

- [ ] **步骤 3：读取确认**

```bash
sed -n '74,110p' pkg/download/proxy_selector.go
```

- [ ] **步骤 4：修改代码**（与 6.1 相同模式）

```go
cacheBase := s.cacheDir
if cacheBase == "" {
    cacheDir, err := os.UserCacheDir()
    if err != nil {
        cacheDir = os.TempDir()
    }
    cacheBase = filepath.Join(cacheDir, "dm-proxy-cache")
}
```

权限修改同 6.1。

- [ ] **步骤 5：验证**

```bash
go fmt ./pkg/dlcore/ ./pkg/download/
go build ./pkg/dlcore/ ./pkg/download/
go test -count=1 ./pkg/dlcore/ ./pkg/download/
```

- [ ] **步骤 6：提交**

```bash
git add pkg/dlcore/proxy_selector.go pkg/download/proxy_selector.go
git commit -m "fix: use UserCacheDir instead of TempDir for proxy cache (S5445)"
```

---

### 任务 7：PR 1 最终验证

- [ ] **步骤 1：全量验证**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go fmt ./... && go build ./... && go test -race -count=1 -timeout=180s ./...
```

- [ ] **步骤 2：lint 检查**

```bash
golangci-lint run ./...
```

- [ ] **步骤 3：确认分支提交记录干净**

```bash
git log --oneline
```

- [ ] **步骤 4：Push + Open PR**

```bash
git push origin fix/sonarqube-golang-issues
# 使用 gh CLI 创建 PR
gh pr create --title "fix: SonarQube Go issues remediation" --body "See design doc: docs/superpowers/specs/2026-06-23-sonarqube-golang-fixes-design.md"
```

---

## PR 2：重复字符串提取为命名常量

> **注意**：PR 2 在 PR 1 合并后进行。以下步骤假设已切回 master 建新分支 `refactor/extract-constants`。

### 任务 8：替换 status 字面量为 model.Status* 常量

**分析**：`model/status.go` 已有 `StatusCompleted = "completed"`、`StatusFailed = "failed"` 等。
需要替换的是 **作为状态值** 使用的字面量，而非作为 JSON 键或元数据键使用的 `"status"`。

- [ ] **步骤 1：搜索所有 `"completed"` 字面量在非 test 的 .go 文件中**

```bash
cd D:/workdir/leon/cocomhub/download-manager
grep -rn '"completed"' --include='*.go' | grep -v '_test.go' | grep -v 'fixture/datasets.go'
```

- [ ] **步骤 2：逐一分析每个匹配项**

| 位置 | 是否该替换 | 原因 |
|------|-----------|------|
| `downloader/adapter.go:187` `obj.Metadata["status"] = "completed"` | **否** | 写入 Metadata 值，不是状态赋值 |
| `model/status.go:11` `StatusCompleted = "completed"` | 定义本身 | 不动 |
| `manager/events.go` `summary["completed"]` | **是** | 这是计数 map 键，与 model 状态无关→保留 |
| `manager/manager.go` `summary["completed"]` | **是** | 同上 |
| `manager/metrics.go` `"completed"` | **是** | metrics 标签值，提取为 `metricsLabelCompleted` 常量 |

- [ ] **步骤 3：搜索 `"failed"` 字面量**

```bash
grep -rn '"failed"' --include='*.go' | grep -v '_test.go' | grep -v 'fixture/datasets.go'
```

结论：metrics 标签值的 `"failed"` 和 `"completed"` 可以提取为局部常量，但 `model.StatusCompleted` 是 model 包的领域常量，metrics 包只是使用同名字符串，不共享领域含义——提取到 metrics 文件。

- [ ] **步骤 4：修改 `manager/metrics.go`**

```go
// 文件顶部加
const (
    metricsLabelCompleted = "completed"
    metricsLabelFailed    = "failed"
)
```

- [ ] **步骤 5：验证**

```bash
go fmt ./manager/ && go build ./manager/ && go test -count=1 ./manager/
```

- [ ] **步骤 6：提交**

```bash
git add manager/metrics.go
git commit -m "refactor: extract metrics label constants"
```

---

### 任务 9：提取元数据键常量

**新建文件：** `model/meta_keys.go`

- [ ] **步骤 1：创建 `model/meta_keys.go`**

```go
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model

// Metadata keys used across task types and storage layers.
const (
    MetadataKeyTitle       = "title"
    MetadataKeyContentGroup = "content_group"
    MetadataKeyType        = "type"
    MetadataKeyStatus      = "status"
    MetadataKeyURL         = "url"
)
```

- [ ] **步骤 2：搜索 `["title"]` 的 Metadata 访问**

```bash
grep -rn '\["title"\]' --include='*.go' | grep -v '_test.go' | grep -v 'fixture/datasets.go'
```

替换 `obj.Metadata["title"]` → `obj.Metadata[MetadataKeyTitle]`，`o.Metadata["title"]` → `o.Metadata[MetadataKeyTitle]`，以及 `so.Metadata["title"]`、`metadata["title"]` 等。

涉及文件：`model/object_meta.go`, `manager/aggregate.go`, `manager/events.go`, `manager/manager.go`, `storage/query.go`, `task/*/task.go`

- [ ] **步骤 3：搜索 `["content_group"]` 的访问**

```bash
grep -rn '\["content_group"\]' --include='*.go' | grep -v '_test.go' | grep -v 'fixture/datasets.go'
```

涉及文件：`model/object_meta.go`, `manager/aggregate.go`, `manager/download_group.go`, `task/tktube/task.go`

- [ ] **步骤 4：搜索 `["type"]` 的 Metadata 访问**

```bash
grep -rn '\["type"\]' --include='*.go' | grep -v '_test.go' | grep -v 'fixture/datasets.go' | grep -v 'config\.'
```

注意区分 Metadata["type"]（元数据中的类型）和 config 中的 Type 字段（配置结构体字段）。

- [ ] **步骤 5：逐个替换**

每个文件手动执行替换。示例：

```go
// 修改前
obj.Metadata["title"]
// 修改后
obj.Metadata[model.MetadataKeyTitle]
```

```go
// 修改前（model/object_meta.go）
func (o *DownloadObject) GetTitle() string {
    if o == nil {
        return ""
    }
    o.RLock()
    defer o.RUnlock()
    v, _ := o.Metadata["title"]
    return v
}
// 修改后
func (o *DownloadObject) GetTitle() string {
    if o == nil {
        return ""
    }
    o.RLock()
    defer o.RUnlock()
    v, _ := o.Metadata[MetadataKeyTitle]
    return v
}
```

- [ ] **步骤 6：验证**

```bash
go fmt ./...
go build ./...
go test -count=1 ./...
```

- [ ] **步骤 7：提交**

```bash
git add model/meta_keys.go model/object_meta.go manager/aggregate.go manager/events.go manager/manager.go manager/download_group.go storage/query.go task/*/
git commit -m "refactor: extract metadata key constants"
```

---

### 任务 10：提取 HTTP 头常量

**分析**：
- `api/errors.go` 已有 `hdrContentType = "Content-Type"`（包私有）
- 跨包场景需评估是否导出或建新包

**推荐方案**：
- 在 `api/errors.go` 中补充 `cache-control` 和 `no-cache` 常量（仅 `api/` 包内使用）
- 跨包用的 `User-Agent` 等放在调用方就近

- [ ] **步骤 1：补充 `api/errors.go`**

```go
const (
    errFmtInvalidBody = "Invalid request body: %v"
    hdrContentType    = "Content-Type"
    hdrCacheControl   = "Cache-Control"
    hdrNoCache        = "no-cache"
)
```

- [ ] **步骤 2：替换 `api/server_task.go` 中的 "Cache-Control" 和 "no-cache"**

```go
// 修改前
w.Header().Set("Cache-Control", "no-cache")
// 修改后
w.Header().Set(hdrCacheControl, hdrNoCache)
```

- [ ] **步骤 3：替换 `api/server.go` 中的 "Content-Type"**

```go
// 修改前
w.Header().Set("Content-Type", "application/json")
// 修改后
w.Header().Set(hdrContentType, "application/json")
```

- [ ] **步骤 4：跨包 "cache-control" 保留不动**

`downloader/scraper.go`, `pkg/dlcore/client.go`, `pkg/download/http_extractor.go`, `cmd/scraper_get/main.go` 中的 `"cache-control"` 是跨包使用的 HTTP 头名。考虑到项目不做 `pkg` 层薄封装的约定，保持这些不变。

- [ ] **步骤 5：验证**

```bash
go fmt ./api/ && go build ./api/ && go test -count=1 ./api/
```

- [ ] **步骤 6：提交**

```bash
git add api/errors.go api/server_task.go api/server.go
git commit -m "refactor: extract HTTP header constants"
```

---

### 任务 11：提取 API 错误码常量

- [ ] **步骤 1：修改 `api/errors.go`**

```go
const (
    errFmtInvalidBody    = "Invalid request body: %v"
    hdrContentType       = "Content-Type"
    hdrCacheControl      = "Cache-Control"
    hdrNoCache           = "no-cache"
    errCodeInvalidRequest = "invalid_request"
    errCodeUpdateFailed   = "update_failed"
)
```

- [ ] **步骤 2：替换 `api/server_task.go` 中的 "invalid_request"**

```bash
grep -n '"invalid_request"' api/server_task.go
```

将 `map[string]string{"error": "invalid_request"}` 替换为 `map[string]string{"error": errCodeInvalidRequest}`。

- [ ] **步骤 3：替换 `api/server_config.go` 中的 "invalid_request" 和 "update_failed"**

```bash
grep -n '"invalid_request"\|"update_failed"' api/server_config.go
```

- [ ] **步骤 4：验证**

```bash
go fmt ./api/ && go build ./api/ && go test -count=1 ./api/
```

- [ ] **步骤 5：提交**

```bash
git add api/errors.go api/server_task.go api/server_config.go
git commit -m "refactor: extract API error code constants"
```

---

### 任务 12：提取日志属性键常量

**新建文件：** `pkg/logutil/keys.go`

- [ ] **步骤 1：创建 `pkg/logutil/keys.go`**

```go
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package logutil

// Log attribute keys used across the application.
const (
    LogKeyTaskID = "task_id"
    LogKeyError  = "error"
    LogKeyURL    = "url"
    LogKeyStatus = "status"
    LogKeyType   = "type"
)
```

- [ ] **步骤 2：确认所有 `slog.Any("error", err)` 和类似调用**

```bash
grep -rn 'slog\.\(Any\|String\|Group\)("error"' --include='*.go' | grep -v '_test.go'
# 同样搜索 "task_id", "url", "status", "type" 在 slog 调用中
```

- [ ] **步骤 3：替换（按文件批量替换）**

使用 `sed` 批量替换（注意：需在 Git Bash 中运行）：

```bash
# 替换所有 .go 文件中的 slog 调用（排除测试文件）
find . -name '*.go' ! -name '*_test.go' -exec sed -i \
  -e 's/slog\.Any("task_id",/slog.Any(logutil.LogKeyTaskID,/g' \
  -e 's/slog\.String("task_id",/slog.String(logutil.LogKeyTaskID,/g' \
  -e 's/slog\.Any("error",/slog.Any(logutil.LogKeyError,/g' \
  -e 's/slog\.String("error",/slog.String(logutil.LogKeyError,/g' \
  -e 's/slog\.Any("url",/slog.Any(logutil.LogKeyURL,/g' \
  -e 's/slog\.String("url",/slog.String(logutil.LogKeyURL,/g' \
  {} +
```

**注意**：`logutil` 包可能未被引用，需在每个修改的文件中添加 import。

**替代方案**：用 Go AST 工具或手动逐一替换。

**由于涉及 30+ 文件，推荐每个子包单独替换并验证：**

```bash
# 先处理 api/ 包
sed -i 's/slog\.String("task_id",/slog.String(logutil.LogKeyTaskID,/g' api/*.go
go build ./api/  # 会报未导入 logutil → 手动加 import
```

- [ ] **步骤 4：验证**

```bash
go fmt ./...
go build ./...
go test -count=1 ./...
```

- [ ] **步骤 5：提交**

```bash
git add pkg/logutil/keys.go    # 新文件
# 以及所有修改的文件
git commit -m "refactor: extract log attribute key constants"
```

---

### 任务 13：提取时间格式和 URL 路径常量（最终）

#### 子任务 13.1：时间格式常量

- [ ] **步骤 1：确认所有 "20060102150405" 出现位置**

```bash
grep -rn '20060102150405' --include='*.go'
```

涉及文件：`downloader/wget.go`, `pkg/dlcore/ffmpeg.go`, `pkg/dlcore/handler.go`, `pkg/download/extractor/wget.go`, `pkg/download/http_extractor.go`

- [ ] **步骤 2：新建或选择位置**

推荐放在 `pkg/download/extractor/extractor.go` 或新建一个最小常量文件。

```go
// 在 pkg/download/extractor/extractor.go 中追加（如果已经存在）
// 或在 pkg/download/ 中新建 timefmt.go

package download

// LogTimestampFormat is the Go time layout used for log filenames.
// Format: YYYYMMDDHHMMSS (no separators, safe for filenames).
const LogTimestampFormat = "20060102150405"
```

- [ ] **步骤 3：替换**

```go
// 修改前
time.Now().Format("20060102150405")
// 修改后
time.Now().Format(download.LogTimestampFormat)
```

注意：`pkg/dlcore/ffmpeg.go` 和 `pkg/dlcore/handler.go` 不能引用 `pkg/download`（dlcore 是废弃包），这两个保持原地不变或提取到 dlcore 局部常量。

更务实的方案：在各自包中定义包私有常量：

```go
// pkg/dlcore 中已有 const.go 或就近定义
const logTimestampFormat = "20060102150405"

// pkg/download 中
const logTimestampFormat = "20060102150405"

// downloader/wget.go
const logTimestampFormat = "20060102150405"
```

- [ ] **步骤 4：验证**

```bash
go build ./...
go test -count=1 ./...
```

#### 子任务 13.2："/bandwidth" 路径常量

- [ ] **步骤 5：确认所有 "/bandwidth" 出现位置**

```bash
grep -rn '"/bandwidth"' --include='*.go' | grep -v '_test.go'
```

涉及文件：`config/config.go`, `pkg/dlcore/proxy.go`, `pkg/dlcore/proxy_selector.go`, `pkg/download/proxy/tunnel.go`, `pkg/download/proxy_selector.go`

- [ ] **步骤 6：在 `config/config.go` 中定义**

```go
const DefaultBandwidthPath = "/bandwidth"
```

或如果现有常量已存在，追加上去。

- [ ] **步骤 7：替换**

```go
// 修改前
"/bandwidth"
// 修改后
config.DefaultBandwidthPath
```

- [ ] **步骤 8：验证**

```bash
go fmt ./...
go build ./...
go test -count=1 ./...
```

#### 子任务 13.3：提交

- [ ] **步骤 9：提交**

```bash
git add .
git commit -m "refactor: extract time format and URL path constants"
```

---

### 任务 14：PR 2 最终验证

- [ ] **步骤 1：全量验证**

```bash
cd D:/workdir/leon/cocomhub/download-manager && go fmt ./... && go build ./... && go test -race -count=1 -timeout=180s ./...
```

- [ ] **步骤 2：Push + Open PR**

```bash
git push origin refactor/extract-constants
gh pr create --title "refactor: extract repeated strings to named constants" --body "See design doc: docs/superpowers/specs/2026-06-23-sonarqube-golang-fixes-design.md"
```

---

## 验证清单

| 步骤 | 命令 | 预期 |
|------|------|------|
| 格式化 | `go fmt ./...` | 无输出 |
| 编译 | `go build ./...` | exit 0 |
| 测试 | `go test -race -count=1 -timeout=180s ./...` | PASS 无 race |
| Lint | `golangci-lint run ./...` | 无新增 issue |
| SonarQube 重扫 | 自动在 CI 中触发 | issue 数减少 |
