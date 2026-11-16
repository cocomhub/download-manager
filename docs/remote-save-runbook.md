# 远程保存运行手册

> 适用范围：download-manager remote-save（A 本地上传 / B 对象目录 / C 云端直存）。
> 当前主干已包含冲突处理、镜像清理、失败重试、manifest 对象隔离与 cloud_only auto-clean 恢复。

## 1. 上线前检查

```bash
# download-manager
go test ./...
go test -race ./downloader ./remotesave ./manager ./api
golangci-lint run ./...

# 真实 sproxy（独立端口/独立 root，避免影响生产）
SPROXY_E2E_URL=... SPROXY_E2E_AK=... SPROXY_E2E_SK=... SPROXY_E2E_SKID=... \
  go test -count=1 -tags=e2e -run 'TestRemoteSaveRealE2E|TestSproxyCloudOnlyAutoCleanRealE2E' ./test/e2e
```

确认：

- 顶层 `sproxy` 账号配置可用，`access_key/access_key_secret/access_key_id` 三件套齐全；
- 任务 `remote_save.volume/dir` 指向预期卷目录；
- `mirror_delete=true` 时同一 `volume+dir` 不应被其它启用 remote_save 的任务共享；
- cloud_only 任务必须能从 `transfer_volume/transfer_path` 找到转存目标。

## 2. 常用操作

### 查看状态

```http
GET /api/tasks/{task_id}/remote_save
GET /api/tasks/{task_id}/remote_save/objects
GET /api/tasks/{task_id}/remote_save/migration
GET /api/tasks/{task_id}/remote_save/conflicts
```

### 手动触发对账

```http
POST /api/tasks/{task_id}/remote_save
{"force": true, "url": "可选；空=全任务"}
```

`force=true` 会忽略 manifest 成功缓存，重新校验本地 sha256 与远端 checksum。

### 处理冲突

```http
POST /api/tasks/{task_id}/remote_save/conflicts/resolve
{"object_url":"...","path":"...","action":"reuse|overwrite|rename|skip"}

# 批量（同一动作，单条失败不阻断其它）
POST /api/tasks/{task_id}/remote_save/conflicts/resolve_batch
{"action":"reuse","items":[{"object_url":"...","path":"..."}]}
```

- `reuse`：接受远端内容；
- `overwrite`：用本地覆盖远端（显式破坏性）；
- `rename`：保留远端并将本地写入新路径；
- `skip`：用户显式跳过，后续对账不应自动改回。

### 刷新播放 URL

```http
GET /api/tasks/{task_id}/object/play_url?url={object_url}
```

## 3. 故障处理

### 远端 Stat/上传失败

- 文件状态会写 manifest 为 failed；
- 快速对账会重试，受 `remote_save.max_retries` 限制；
- 达到上限后需人工触发 `force=true` 或处理根因。

### 远端文件被删除

快速对账会对 manifest 命中文件做远端 `Stat`；缺失会重新校验并上传。

### manifest 条目冲突或旧版本升级

manifest 以 `objectURL + "\x00" + rel` 为 key；旧版 `rel` key 会自动迁移。
不要手工编辑 manifest；损坏文件会被隔离后重建。

### sproxy cloud_only auto-clean

`cloud_only=true`（save=false）时 sproxy 转存成功后会立即删除 cloud task。
download-manager 在 task 404 时按 `transfer_volume/transfer_path` 回读目标：

- 目标存在：视为成功，记录 `transfer_url`；
- 目标不存在：保持失败，检查转存路径/卷配置。

### 对象目录键变化

`GET /api/tasks/{task_id}/remote_save/migration` 返回 manifest 旧路径与当前
`object_dirs` 计算路径的差异；当前只生成报告，不自动移动/删除远端文件。
确认后可用 sproxy rename/备份工具人工迁移，或让 mirror_delete 清理旧路径。

### mirror_delete 未删除

可能原因：

- 同一 volume/dir 存在其它启用 remote_save 的任务（安全跳过）；
- 对象不在 completed 列表或对象仍存在；
- 文件仍属于当前 completed 文件集；
- 用户 rename 的保留文件（有意保留）。

### 进程重启

- manifest 从磁盘加载；
- 失败/冲突状态保留；
- deep reconcile 时间从 `remote_save/deep.state` 恢复；
- 大文件分块上传使用确定性 upload ID，可在 sproxy 未清理会话时续传。

## 4. 日志与指标建议

关键日志字段：

- `task_id`
- `url`
- `file`
- `remote`
- `status`
- `reason`

建议监控：

- remote-save 对象/文件状态计数；
- failed/conflict/retry 数量；
- mirror delete 次数；
- 远端 Stat 错误率；
- synced 对象数与文件总字节。

## 5. 已知边界

- Mongo 存储带任务级租约（`<collection>_leases`，TTL 5m，自动续租）；memory 存储无跨进程保护；
- overwrite 删除远端后、上传前进程崩溃需要再次执行动作；
- cloud_only 没有 transfer 目标时，sproxy auto-clean 后无法恢复；
- 单对象文件上传目前串行；大任务可考虑后续文件级并发；
- `mirror_delete` 默认关闭，开启前必须确认目录独占。
