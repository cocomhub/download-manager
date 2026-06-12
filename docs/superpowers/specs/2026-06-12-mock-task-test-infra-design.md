# Mock 任务系统与测试基础设施设计

> 日期：2026-06-12
> 状态：已批准

## 背景与目标

对 download-manager 项目进行三项重构，按优先级排序：

1. **Mock 任务系统** — 支持基于规则自动生成 DownloadObject 的 mock 任务类型 + 可插拔的 MockDownloader 行为模式
2. **测试增强** — Manager 集成测试、API 测试、端到端场景测试（续传、取消、重试、并发限制）
3. **Viper 规范化重构** — 将配置加载从 `os.ReadFile + yaml.Unmarshal` 迁移到 Viper

## 工作顺序

```
Phase 1: Mock 任务系统
  └─ testutil/mockdl/ → task/mock/ → 单元测试
Phase 2: 测试增强
  └─ Manager 测试 → API 测试 → 端到端场景测试
Phase 3: Viper 重构
  └─ config.Load() 内部替换 → CLI flag 绑定 → 可选暴露
```

---

## 一、Mock 任务系统

### 文件结构

```
task/mock/
  config.go      — MockRule, MockBehavior 数据模型 + YAML 反序列化
  task.go        — MockTask 实现 core.Task，init() 注册 "mock" 类型

testutil/
  mockdl/
    downloader.go — MockDownloader 实现 core.Downloader，支持多种行为模式
    options.go    — Option 函数式选项
    new.go        — 程序化构造 API：NewTask, NewDownloader
```

### MockRule 数据模型

```go
// task/mock/config.go

type MockRule struct {
    URLTemplate    string            `yaml:"url_template" json:"url_template"`
    Count          int               `yaml:"count" json:"count"`
    Slugs          []string          `yaml:"slugs,omitempty" json:"slugs,omitempty"`
    FileSize       int64             `yaml:"file_size,omitempty" json:"file_size,omitempty"`
    InitialProgress int              `yaml:"initial_progress,omitempty" json:"initial_progress,omitempty"`
    Status         string            `yaml:"status,omitempty" json:"status,omitempty"`
    Metadata       map[string]string `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}
```

**字段说明：**
- `URLTemplate` — 支持 `{n}`（自增数字，从 0 开始）和 `{slug}`（从 Slugs 列表中取值）两种占位符
- `Count` — 生成对象数量。如果指定了 Slugs，则以 Slugs 长度为准
- `Slugs` — 显式 slug 列表，每 slug 生成一个对象
- `FileSize` — 模拟文件大小，影响进度模拟的字节计数
- `InitialProgress` — 初始进度百分比（模拟续传场景）
- `Status` — 初始状态，默认 `pending`。可选值：`pending` / `completed` / `failed` / `downloading`
- `Metadata` — 注入到对象的 Metadata 字段

### MockBehavior 数据模型

```go
// task/mock/config.go

type MockBehavior struct {
    Mode          string   `yaml:"mode" json:"mode"`
    FailRate      float64  `yaml:"fail_rate,omitempty" json:"fail_rate,omitempty"`
    FailOnURLs    []string `yaml:"fail_on_urls,omitempty" json:"fail_on_urls,omitempty"`
    TimeoutOnURLs []string `yaml:"timeout_on_urls,omitempty" json:"timeout_on_urls,omitempty"`
    DelayPerByte  float64  `yaml:"delay_per_byte,omitempty" json:"delay_per_byte,omitempty"`
}
```

### 行为模式

| 模式 | 值 | 行为 | 测试场景 |
|---|---|---|---|
| 始终成功 | `always_success` | 直接返回 nil，不阻塞 | 正常下载链路、端到端测试 |
| 始终失败 | `always_fail` | 返回 `ErrMockDownload` | 失败处理、重试计数 |
| 模拟进度 | `simulate_progress` | 逐步回调进度 0→100% | 进度上报、事件推送、SSE |
| 随机失败 | `random_fail` | 按 `FailRate` 概率失败 | 混合成功/失败场景 |
| 超时 | `timeout` | 阻塞直到 ctx.Done() | 超时取消、context 传播 |
| 首次失败 | `first_fail_then_success` | 第一次失败，后续成功 | 续传、重试成功 |
| 暂停在进度 | `pause_on_progress` | 在 `InitialProgress%` 处阻塞 | 下载中取消 |

### MockTask 实现

```go
// task/mock/task.go

type MockTask struct {
    *task.BaseTask
    rules    []MockRule
    behavior MockBehavior
    seeded   atomic.Bool    // 确保规则只生成一次
}

func (t *MockTask) Type() string { return "mock" }

func (t *MockTask) GetDownloadObjects() ([]*model.DownloadObject, error) {
    if !t.seeded.Load() {
        if err := t.seedObjects(); err != nil {
            return nil, err
        }
    }
    // 返回非 terminal 的对象
    var pending []*model.DownloadObject
    for _, obj := range t.GetAllObjects(true) {
        s := obj.GetStatus()
        if s != model.StatusCompleted && s != model.StatusCancelled {
            pending = append(pending, obj)
        }
    }
    return pending, nil
}

func (t *MockTask) Scrape(ctx context.Context) error {
    // refresh_interval <= 0 时 no-op
    // > 0 时重新生成规则对象（带递增后缀去重）
    return nil
}

func (t *MockTask) ResolveObject(_ context.Context, _ *model.DownloadObject) error {
    return nil // URL 本身即可下载
}
```

**对象生成逻辑（seedObjects）：**
1. 遍历 rules
2. 解析 URLTemplate：替换 `{n}` 为 0~Count-1，或从 Slugs 中取 slug
3. 构建 `model.DownloadObject`：填充 TaskID、URL、Status、Progress、Metadata
4. 调用 `t.PersistTaskObject(obj)` 持久化到存储和运行时列表

### MockDownloader 实现

```go
// testutil/mockdl/downloader.go

type MockDownloader struct {
    Mode        Mode
    FailError   error
    Delay       time.Duration
    FailRate    float64
    FailURLs    map[string]bool
    TimeoutURLs map[string]bool
    DelayPerByte float64

    // 可观测性钩子
    OnStart    func(url string)
    OnProgress func(url string, progress int)
    OnComplete func(url string)
    OnFail     func(url string, err error)
}

func (d *MockDownloader) Download(obj *model.DownloadObject, _ map[string]string) error {
    if d.OnStart != nil {
        d.OnStart(obj.URL)
    }

    switch d.Mode {
    case ModeAlwaysSuccess:
        return d.complete(obj)
    case ModeAlwaysFail:
        return d.fail(obj)
    case ModeSimulateProgress:
        return d.simulate(obj)
    case ModeRandomFail:
        if rand.Float64() < d.FailRate {
            return d.fail(obj)
        }
        return d.complete(obj)
    case ModeTimeout:
        <-obj.Done()  // 等待 ctx 取消
        return context.Canceled
    case ModeFirstFailThenSuccess:
        return d.firstFailThenSuccess(obj)
    case ModePauseOnProgress:
        return d.pauseAtProgress(obj)
    }
    return nil
}
```

### 程序化构造 API

```go
// testutil/mockdl/new.go

type MockTaskConfig struct {
    TaskID      string
    Rules       []MockRule
    Behavior    MockBehavior
    Objects     []*model.DownloadObject  // 直接预置对象（替代 Rules）
    Storage     core.Storage             // 默认 MemoryStorage
    Concurrency int
}

// NewTask 创建一个 mock 任务，不依赖 YAML 配置
func NewTask(t testing.TB, cfg MockTaskConfig) core.Task {
    // 内部构造 config.Task + 调用 task.NewTask("mock", ...)
    // 或直接构造 MockTask
}

// NewDownloader 创建 MockDownloader
func NewDownloader(mode Mode, opts ...Option) *MockDownloader
```

### YAML 配置示例

```yaml
tasks:
  - id: mock-test
    type: mock
    save_dir: downloads/mock
    storage:
      type: memory
    extra:
      max_concurrent: 3
      mock_rules:
        - url_template: "http://127.0.0.1:18080/files/file-{n}.bin"
          count: 5
          file_size: 1048576
        - url_template: "http://127.0.0.1:18080/videos/{slug}.mp4"
          slugs: ["ep1", "ep2"]
          metadata:
            content_group: "my-show"
      mock_behavior:
        mode: simulate_progress
        delay_per_byte: 0.000001
```

---

## 二、测试增强计划

### 层次一：Manager 集成测试

**文件：`manager/mock_integration_test.go`**

| 测试 | 验证点 |
|---|---|
| `TestManager_MockTask_FullLifecycle` | 创建 Manager → loadTasks → scan → processTask → download → complete |
| `TestManager_MockTask_RetryThenSuccess` | 首次失败 → 重试 → 成功 |
| `TestManager_MockTask_MaxRetriesExceeded` | 持续失败 → StatusFailedPermanent |
| `TestManager_MockTask_ConcurrencyLimit` | 多对象，验证 activeDownloads ≤ concurrency |
| `TestManager_MockTask_CancelDuringDownload` | 下载中取消 → StatusCancelled |
| `TestManager_MockTask_ResumeProgress` | initial_progress → 下载继续 → completed |

### 层次二：API 测试增强

**文件：`api/server_test.go`**

| 测试 | 路径 | 验证点 |
|---|---|---|
| 任务列表 | `GET /api/tasks` | 返回注册的任务列表 |
| 任务详情 | `GET /api/tasks/{id}` | 返回任务配置和状态 |
| 对象列表 | `GET /api/tasks/{id}/objects` | 分页、过滤状态、排序 |
| 对象操作 | `PUT /api/tasks/{id}/objects/{url}` 状态变更 | 取消、重试 |
| 配置读取 | `GET /api/config` | 当前配置 |
| 配置更新 | `POST /api/config` | 热更新（在 full 模式下） |
| 写保护 | 各 POST/PUT/DELETE | UI 模式拒绝写操作 |

### 层次三：端到端场景测试

**文件：`test/e2e/manager_e2e_test.go`**

```
mock task (rules) → Manager → scheduler → processTask → download → httptest server
                           ↓                          ↓
                       MemoryStorage            MockDownloader
                           ↓
                       API queries → assert statuses
```

- httptest 服务器提供真实 HTTP 文件下载
- MockDownloader 使用 `always_success` 模式验证完整流程
- `simulate_progress` 模式验证进度事件
- `first_fail_then_success` 模式验证续传

---

## 三、Viper 配置重构

### 现状

```go
// config/global.go
func Load(configFile string) (*Config, error) {
    data, err := os.ReadFile(configFile)   // 纯文件读取
    cfg := Config{ /* defaults */ }
    err = yaml.Unmarshal(data, &cfg)        // 直接反序列化
    cfg.ValidateAndClamp()
    return &cfg, nil
}
```

```go
// main.go
fs := flag.NewFlagSet("download-manager", flag.ContinueOnError)
fs.StringVar(&cfgPath, "config", "config.yaml", "")
fs.StringVar(&runMode, "run-mode", "", "")
fs.BoolVar(&uiOnly, "ui-only", false, "")
// ... env fallback 手动实现
```

### Phase 1：config.Load() 内部替换

```go
// config/viper.go (新增)

func LoadViper(configFile string) (*Config, error) {
    v := viper.New()
    v.SetConfigFile(configFile)
    v.SetEnvPrefix("DM")
    v.AutomaticEnv()
    // 环境变量到配置键的映射
    v.BindEnv("runtime.mode", "DM_RUN_MODE")
    v.BindEnv("server.http_port", "DM_HTTP_PORT")

    if err := v.ReadInConfig(); err != nil {
        return nil, fmt.Errorf("read config: %w", err)
    }

    var cfg Config
    if err := v.Unmarshal(&cfg); err != nil {
        return nil, fmt.Errorf("unmarshal config: %w", err)
    }

    cfg.ValidateAndClamp()
    return &cfg, nil
}
```

**对外保持 `config.Load(filepath) (*Config, error)` 不变**，内部实现切换到 Viper。

### Phase 2：CLI flag 绑定

```go
// main.go

func main() {
    v := viper.New()
    v.SetEnvPrefix("DM")
    v.AutomaticEnv()

    // 解析 flag 并绑定到 Viper
    cfgPath := v.GetString("config")  // 默认 "config.yaml"
    runMode := v.GetString("run-mode")
    uiOnly := v.GetBool("ui-only")

    // flag 覆盖（CLI 优先级最高）
    flag.StringVar(&cfgPath, "config", cfgPath, "")
    flag.StringVar(&runMode, "run-mode", runMode, "")
    flag.BoolVar(&uiOnly, "ui-only", uiOnly, "")
    flag.Parse()

    cfg, err := config.Load(cfgPath)
}
```

**环境变量层级：** CLI flag > 环境变量（`DM_*`）> 配置文件 > 默认值

### 向后兼容

- `config.Load()` 签名不变
- 所有 `config_test.go` 中的测试继续通过
- `Diff()` 方法操作 `Config` 结构体，不受 Viper 影响
- `main.go` 的环境变量手动解析逻辑逐步淘汰

---

## 关键风险

| 风险 | 影响 | 缓解措施 |
|---|---|---|
| MockDownloader 在 `pause_on_progress` 模式下 goroutine 泄漏 | 测试不稳定 | 每个测试用 `t.Cleanup()` 确保超时取消 |
| Viper Unmarshal 与现有 yaml 标签不兼容 | 配置加载失败 | Phase 1 保留旧的 `yaml.Unmarshal` 作为 fallback |
| `config_test.go` 中的 `Diff` 测试依赖结构体指针 | 测试失败 | 运行全量测试套件确保一致 |

---

## 验证方法

1. `go test ./task/mock/...` — Mock 任务单元测试
2. `go test ./testutil/mockdl/...` — MockDownloader 单元测试
3. `go test ./manager/... -run TestManager_Mock -v` — Manager 集成测试
4. `go test ./api/... -v` — API 测试
5. `go test ./config/... -v` — Viper 迁移后配置测试
6. `go test ./...` — 全量回归
