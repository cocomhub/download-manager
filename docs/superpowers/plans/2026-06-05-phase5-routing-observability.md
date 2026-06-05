# Phase 5：路由规则 + 可观测性 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 为 `pkg/download/` 添加 URL 模式路由规则、Per-handler 下载指标和中间件链。

**架构：** 规则引擎在 Selector 层上方添加 URL 匹配优先级；中间件链在 Download() 流程中嵌入拦截点；metrics 通过原子计数器收集。Manager 通过适配器暴露 metrics 到 `/api/metrics`。

**技术栈：** Go 1.26 标准库 (`path.Match`, `sync/atomic`, `regexp`)。无第三方依赖。

---

## 文件结构

```
pkg/download/
├── rule.go               # NEW: URL 路由规则引擎
├── middleware.go          # NEW: 下载中间件链
├── metrics.go             # NEW: Per-handler 指标收集
├── download.go            # MODIFY: 集成 middleware + metrics
├── option.go              # MODIFY: 新增 WithRule/WithMetricRegistry

api/
├── server.go              # MODIFY: 新增 /api/download/metrics 端点
```

## 任务分解

### 任务 1：Rule — URL 路由规则引擎

**文件：**
- 创建：`pkg/download/rule.go`
- 创建：`pkg/download/rule_test.go`

Rule 允许基于 URL 模式选择特定 Extractor 或 Transport，覆盖 Selector 的默认匹配。

- [ ] **步骤 1：创建 `rule.go`**

```go
package download

import (
    "fmt"
    "path"
    "strings"
)

// Rule 描述一个 URL 路由规则。
type Rule struct {
    // Pattern 是 URL 模式（支持 path.Match 语法，如 "*.m3u8"、"*huge*"）
    Pattern string
    // Extractor 指定匹配后使用的 Extractor 名称（可选）
    Extractor string
    // MinSize 最小文件大小（字节），0 表示不限制
    MinSize int64
    // MaxSize 最大文件大小（字节），0 表示不限制
    MaxSize int64
}

// Matcher 检查 URL 是否匹配此规则。
func (r *Rule) Matcher(url string) bool {
    return matchPattern(r.Pattern, url)
}

// matchPattern 使用 path.Match 或后缀匹配。
func matchPattern(pattern, url string) bool {
    // Try glob match
    if ok, err := path.Match(pattern, url); err == nil && ok {
        return true
    }
    // Try suffix match (e.g. "*.m3u8")
    if strings.HasPrefix(pattern, "*") {
        suffix := strings.TrimPrefix(pattern, "*")
        if strings.HasSuffix(url, suffix) {
            return true
        }
    }
    // Try prefix match
    if strings.HasSuffix(pattern, "*") {
        prefix := strings.TrimSuffix(pattern, "*")
        if strings.HasPrefix(url, prefix) {
            return true
        }
    }
    // Try contains match
    if strings.Count(pattern, "*") == 2 && strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
        substr := strings.TrimSuffix(strings.TrimPrefix(pattern, "*"), "*")
        if strings.Contains(url, substr) {
            return true
        }
    }
    return false
}

// RuleSet 管理一组规则。
type RuleSet struct {
    rules []*Rule
}

// NewRuleSet 创建 RuleSet。
func NewRuleSet(rules ...*Rule) *RuleSet {
    return &RuleSet{rules: rules}
}

// AddRule 添加规则。
func (rs *RuleSet) AddRule(r *Rule) {
    rs.rules = append(rs.rules, r)
}

// Match 返回第一个匹配的规则，若无匹配返回 nil。
func (rs *RuleSet) Match(url string, hint *DownloadHint) *Rule {
    for _, r := range rs.rules {
        if r.Matcher(url) {
            return r
        }
    }
    return nil
}
```

- [ ] **步骤 2：创建 `rule_test.go`**

test cases: suffix match, prefix match, contains match, no match.

- [ ] **步骤 3：Commit**

### 任务 2：Metrics — Per-handler 指标收集

**文件：**
- 创建：`pkg/download/metrics.go`
- 创建：`pkg/download/metrics_test.go`

- [ ] **步骤 1：创建 `metrics.go`**

```go
package download

import (
    "sync/atomic"
    "time"
)

// Metrics 记录单个 Extractor/Transport 的下载统计。
type Metrics struct {
    Name           string
    TotalRequests  atomic.Int64
    TotalBytes     atomic.Int64
    SuccessCount   atomic.Int64
    FailureCount   atomic.Int64
    TotalDurationMs atomic.Int64
    LastRequestAt  atomic.Int64 // unix timestamp
}

// MetricRegistry 管理所有 Metrics 实例。
type MetricRegistry struct {
    metrics map[string]*Metrics
}

func NewMetricRegistry() *MetricRegistry { ... }
func (r *MetricRegistry) Get(name string) *Metrics { ... }
func (r *MetricRegistry) Record(name string, bytes int64, duration time.Duration, success bool) { ... }
func (r *MetricRegistry) Snapshot() map[string]map[string]int64 { ... }
```

- [ ] **步骤 2：Commit**

### 任务 3：Middleware — 下载中间件链

**文件：**
- 创建：`pkg/download/middleware.go`
- 创建：`pkg/download/middleware_test.go`

- [ ] **步骤 1：创建 `middleware.go`**

```go
package download

import "context"

// Middleware 是下载中间件。在 Extract 前后执行额外逻辑。
type Middleware func(ctx context.Context, req *Request, next Extractor) error

// MetricsMiddleware 创建记录指标的中间件。
func MetricsMiddleware(registry *MetricRegistry) Middleware { ... }

// RuleMiddleware 创建应用路由规则的中间件。
func RuleMiddleware(ruleSet *RuleSet, selector Selector) Middleware { ... }

// ChainMiddleware 将多个中间件组合为链。
func ChainMiddleware(mws ...Middleware) Middleware { ... }
```

- [ ] **步骤 2：修改 `download.go` 集成 middleware**

```go
type Downloader struct {
    selector   Selector
    extractors []Extractor
    transport  Transport
    middleware Middleware
    metrics    *MetricRegistry
}

func (d *Downloader) Download(ctx context.Context, req *Request) error {
    // ... existing matching logic ...
    // Wrap extractor with middleware chain
    executor := ex
    if d.middleware != nil {
        executor = &middlewareExtractor{base: ex, mw: d.middleware}
    }
    return executor.Extract(ctx, req)
}
```

- [ ] **步骤 3：修改 `option.go` 添加 `WithRuleSet` 和 `WithMetricRegistry`**

- [ ] **步骤 4：Commit**

### 任务 4：Manager + API 集成

- [ ] **步骤 1：在 Manager 创建时配置 RuleSet + MetricRegistry 并注入**

- [ ] **步骤 2：添加 `/api/download/metrics` 端点暴露 `MetricRegistry.Snapshot()`**

- [ ] **步骤 3：全量回归**

## 验证

```bash
go build ./...
go test -run 'Rule|Metrics|Middleware' ./pkg/download/...
go test ./...
```