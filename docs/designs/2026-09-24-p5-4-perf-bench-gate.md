# P5-4 性能压测 + 基准门禁设计（2026-09-24）

## 背景 / 目标

- 现状证据：`make bench`（`go test -bench=. -benchmem -count=5 -run=^$`）+ `make bench-compare`（benchstat baseline vs current）存在；CI benchmark-action `continue-on-error: true`（**无失败 gate**，基准回归不拦 CI）
- 已有基准：`manager/bench_test.go`、`storage/bench_test.go`、`storage/bench_pushdown_test.go`、`model/model_bench_test.go`、`pkg/titlegroup/bench_test.go`
- 目标：大样本（5000+ 对象）聚合/存储压测报告 + bench-compare 加失败 gate（阈值比较）+ CI 门禁接线

## 组件与接口

- `storage/bench_test.go`（改）：新增 5000/10000 对象基准（SeedLarge 方法 + AggregateObjects 场景）
- `scripts/bench-gate.sh`（新）：benchstat 输出解析 + 阈值比较
  - 用法：`./scripts/bench-gate.sh baseline.txt current.txt [threshold_pct]`
  - 默认阈值：p50 +5% / p90 +10%（ns/op 与 allocs/op 独立判断）
  - 输出：超阈值函数清单 + 退出码 1
- `Makefile`（改）：`bench-gate` 目标（跑 bench + 与 baseline 比较 + 门禁判定）
- `.github/workflows/ci.yml`（改）：`continue-on-error: true` → 视阈值判定红/绿（或单独 job 做 gate）
- `docs/performance.md`（新）：压测报告模板 + 复现步骤 + 历史数据

## 数据流

1. 开发/CI 跑 `make bench` → bench.txt
2. 与 baseline.txt（最近一次稳定基准）比较 → benchstat 差异
3. bench-gate.sh 解析 → 超阈值 → 红（CI 失败）

## 错误处理

- baseline.txt 不存在：提示先跑 `make bench` 生成 baseline，不失败（首次）
- 无基准匹配：跳过该函数比较（不误报）
- 环境差异（CI 慢于本地）：阈值需按 CI 实测校准，避免 flaky

## 测试 + 变异点

- bench-gate.sh 用 `bash -n` 语法检查 + 手工构造超阈值样本验证退出码
- **变异点**：改回 `continue-on-error: true` → 门禁不生效（脚本测试红）；删阈值判断 → 超阈值仍绿（脚本测试红）

## 片划分

- P1（基准数据）：5000+ 对象压测基准 + 报告
- P2（门禁脚本）：bench-gate.sh + Makefile 目标
- P3（CI 接线）：ci.yml 门禁 + 校准阈值

## 风险与零回归

- CI 环境波动是主要 flaky 风险 → 阈值保守（p50 +5% / p90 +10%）+ 可配置
- 基准不改变任何生产行为，纯测量
- 门禁失败可临时放宽阈值（配置化）而非 disable
