# 远程保存生产部署策略

> 适用：download-manager remote-save（本地上传、云端直存、mirror、迁移）。
> 本文是推荐的生产参数与上线检查，不替代代码/配置的最终事实源。

## 1. 部署拓扑

- 推荐**单个 full 实例**处理下载与 remote-save。
- 需要多实例时：
  - 必须使用共享 Mongo 存储；
  - remote-save 依赖 `TaskLeaseStore`（`<collection>_leases`，TTL 5m）；
  - 每个任务同一时刻只允许一个实例执行（任务级租约）；
  - UI-only 实例不执行 remote-save。
- 不要用多个 full 实例共享同一 `work_dir` 但使用不同存储。

## 2. 关键配置建议

```yaml
remote_save:
  concurrency: 2
  file_concurrency: 1
  reconcile_interval: 10m
  deep_reconcile_interval: 24h
  settle_seconds: 30
  max_retries: 3
  retry_backoff_base: 1m
  retry_backoff_max: 1h
  max_files_per_object: 1000
  verify_checksum: false
  batch_stat: false
```

### `batch_stat`

- 默认 **false**：保持逐文件 `Stat`，行为最保守。
- 大任务、目标目录文件多、sproxy `List` 稳定后可设为 `true`。
- 开启前验证：
  - sproxy `List` 返回完整；
  - 目录内文件量可控；
  - 失败时自动回退 `Stat` 的日志无异常。
- 不建议对超大目录（数万文件）直接开启，先按任务/目录分批评估。

### `file_concurrency`

- 默认 `1`，不建议全量直接调到 8。
- 小文件多、对象目录文件数 >20 时可尝试 `2` 或 `4`。
- 观察：
  - sproxy RPS；
  - 本地磁盘/CPU；
  - 百度上传并发；
  - 失败率。
- 单对象并发只影响文件，不改变对象级 worker 数。

### 重试

- `max_retries` 控制自动重试次数；
- 退避默认 `1m → 2m → 4m … 1h`；
- 生产建议：
  - 短时 sproxy/网络故障：保持默认；
  - 外部盘长期不可用：降低 `max_retries`，靠人工 force；
  - 告警阈值建议：连续 3 次 reconcile 后 `failed` 对象数持续增长即告警。

### mirror_delete

- 默认关闭；
- 开启前确认同一 `volume + dir` 不被其它启用 remote_save 的任务共享；
- 建议先运行只读迁移报告/冲突检查，再开启；
- 删除只针对 manifest 记录过、当前 completed 文件集之外的路径。

## 3. 上线检查

```bash
go test ./...
go test -tags=no_mongo ./...
go test -race ./remotesave ./manager ./api ./storage
golangci-lint run ./...
```

真实 sproxy（独立端口/独立 root）：

```bash
SPROXY_E2E_URL=http://127.0.0.1:18086 \
SPROXY_E2E_AK=... SPROXY_E2E_SK=... SPROXY_E2E_SKID=... \
  go test -count=1 -tags=e2e \
  -run 'TestRemoteSaveRealE2E|TestSproxyCloudOnlyAutoCleanRealE2E' ./test/e2e
```

检查：

- sproxy 版本与 dm 依赖一致；
- `sproxy.play_base_url` 可访问；
- 签名 TTL ≤ sproxy 服务端限制；
- `work_dir` 可写，manifest/审计文件不与他人冲突；
- Mongo 租约集合可创建/索引可建立；
- mirror 目录独占确认完成。

## 4. 监控与告警建议

remote-save 指标：

- `files_uploaded` / `files_reused` / `files_failed`
- `conflicts`
- `bytes_uploaded`
- `mirror_deleted`
- `cloud_files`
- `lease_acquire_failures` / `lease_renew_failures`
- 分钟级 `trend`

建议告警：

- `conflicts` 持续增长；
- `files_failed` 增长且重试耗尽；
- `lease_acquire_failures` 非零且持续；
- `lease_renew_failures` 非零；
- confirmed 对象数长时间不增长（同步卡住）；
- mirror 删除量异常增长（先确认目录独占）。

## 5. 运维流程

- 冲突：先查看对象状态表 → 批量/单条处理 → 复核。
- 迁移：先看迁移报告 → 确认旧/新路径 → 执行迁移 → 查看 `migration-audit.jsonl`。
- 失败：查看 `last_error`/retries → 修复根因 → 对象级 force 或等待退避。
- mirror：默认关闭；变更目录配置前先核对报告与独占性。

## 6. 已知边界

- 覆盖动作删除远端后、上传前崩溃需要再次执行动作；删除/改名本身幂等。
- cloud_only 必须有可用 `transfer_volume/transfer_path` 才能在 auto-clean 后恢复。
- 任务租约依赖 Mongo；memory 存储无跨进程保护。
- `batch_stat` 默认关闭，性能收益取决于目录规模与 sproxy 稳定性。
