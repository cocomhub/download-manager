# 远程保存（download-manager → sproxy）

> 状态：A/B/C 与 UI 冲突面板已实现；包含云端冲突、镜像清理、重试上限与重启恢复。

## 目标

把下载管理器**已完成**对象的文件幂等同步到 sproxy 卷目录；同一对象的视频、封面、
缩略图、预览视频归入同一目录。云端下载对象直接转存到对应目录，并能生成有有效期的
sproxy 播放链接。

## 配置

### 全局 sproxy（唯一账号/连接配置）

```yaml
sproxy:
  api_url: https://sproxy.example.com
  access_key: ak-...
  access_key_secret: ...
  access_key_id: skey-...
  insecure_tls: false
  play_base_url: https://sproxy.example.com
  play_ttl: 1h
```

`downloader.sproxy_cloud` 的 `api_url/api_token/access_key*/insecure_tls` 是兼容别名：
顶层 `sproxy` 为空时自动回填；两者都配置时顶层优先。任务级不允许配置这些字段。

### 全局 remote_save 引擎默认值

```yaml
remote_save:
  concurrency: 2
  reconcile_interval: 10m
  deep_reconcile_interval: 24h
  settle_seconds: 30
  max_retries: 3
  max_files_per_object: 1000
  verify_checksum: false
```

- `reconcile_interval`：快速对账（manifest 命中时只核对远端 `Stat` 是否存在且 checksum 未变，
  不重算本地 sha256）。
- `deep_reconcile_interval`：深度对账（忽略 manifest，重算 sha256 + 校验远端）；上次执行时间
  持久化在 `remote_save/deep.state`，进程重启不会把 24h 计时清零。
- `max_retries`：失败对象自动重试上限；达到上限后自动对账停止重试，需人工触发/处理。
- `settle_seconds`：跳过刚写入/可能仍在下载的文件。
- `verify_checksum=true`：每次快速对账也重算本地 sha256。

### 任务级（只配路径）

```yaml
tasks:
  - id: hanime-2.5d
    type: hanime
    save_dir: /opt/download-manager/downloads/hanime/2.5D
    extra:
      remote_save:
        enabled: true
        volume: baidu
        dir: hanime/2.5D
        object_dirs: true    # B：远端按番号/唯一 id 分目录
        mirror_delete: false # 可选：镜像删除 manifest 记录过、且已不属于 completed 对象的远端文件
        exclude: []          # 可选 glob（相对 save_dir）
```

## 同步口径

- 只处理 `status=completed` 对象；收集主视频 + cover/thumb/preview + small objects；
  `object_dirs=true` 时远端路径为 `<dir>/<对象键>/<文件名>`，对象键优先级：
  `metadata.code` → `metadata.content_group` → 数字 ID → title。
- 逐文件 **sha256** 校验：本地 sha256 == 上传返回 checksum == sproxy `Stat.Checksum`；
  不一致或远端 checksum 不可得 → `conflict`，不自动覆盖。
- sproxy 侧 meta 保存原始 mtime + 完整 mongo `Metadata`/`Extra`/tags（`x_dm_object`）；
  download-manager 读取/展示的是 sproxy sidecar 的 mtime，不使用百度存储时间。
- 逐文件 sha256/远端 sha256 写入 mongo `DownloadObject.RemoteSave.Files`。
- 远端已存在且 sha256 一致 → `reused`；否则 `conflict`。
- 默认只上传/复用，不删除远端；`mirror_delete=true` 时，任务级对账会删除「manifest 记录过、
  当前 completed 文件集不再包含」的远端文件（未知远端文件永不删除；对象仍存在但非 completed
  时跳过；列目录/数据库错误时 fail-closed；云端对象只保留当前 transfer 目标；用户 rename 的
  保留文件不会被清理）。若同一 volume/dir 还有其它启用 remote_save 的任务，本任务跳过镜像
  删除，避免跨任务误删。

## API

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/tasks/{id}/remote_save` | 聚合状态（enabled/volume/dir/statuses/conflicts） |
| POST | `/api/tasks/{id}/remote_save` | 手动触发（body `{"force":true}` 可选） |
| GET | `/api/tasks/{id}/remote_save/conflicts` | 逐文件冲突列表 |
| POST | `/api/tasks/{id}/remote_save/conflicts/resolve` | 冲突处理：`{object_url,path,action}`；action=`reuse|overwrite|rename|skip` |
| GET | `/api/tasks/{id}/object/play_url?url=` | 生成/刷新签名播放 URL（返回 url + expires_at） |

## 云端直存（C）

- `cloud_download` 对象提交前由 `PrepareCloud` 写入 `transfer_volume`/`transfer_path`，
  路径为 `<dir>/<对象键>/<目标文件>`（`object_dirs=false` 时保留目标相对路径）。
- 目标已存在 → **fail-closed**，对象 `failed_permanent` + `RemoteSave=conflict`；冲突仍会出现在
  `/remote_save/conflicts`，可直接 reuse/overwrite/rename/skip。
- 转存完成后 `NotifyCloudCompleted` 回读 sproxy 的 sha256/mtime，写入 mongo 与播放链接；
  非 cloud_only 对象本地文件走正常对账（manifest 记录 origin=cloud，避免重复上传）。
- 云端冲突动作：
  - `reuse`：接受远端（不重新下载）；
  - `overwrite`：本地有文件则覆盖远端；cloud-only 则删除远端、清 cloud 坐标并重新入队下载；
  - `rename`：本地有文件则上传到新路径；cloud-only 则先把既有远端改名保留，再重新入队下载到
    原目标；
  - `skip`：保持跳过。

## UI 冲突处理

任务详情「远程保存」按钮打开冲突面板，逐文件可选 `复用 / 覆盖 / 改名 / 跳过`；
覆盖/改名有二次确认。UI 通过真实 API 读取/处理，Playwright 用例：
`test/playwright/specs/remote-save.spec.ts`。

## 真实端到端

`test/e2e/remote_save_e2e_test.go`（`-tags=e2e`）启动独立 in-process download-manager
（临时端口，不影响运行中的实例），连接真实 sproxy（不重启），覆盖：completed-only、
sha256、meta mtime/tags、冲突覆盖、签名播放 URL、mirror_delete。

```bash
SPROXY_E2E_URL=http://127.0.0.1:18085 SPROXY_E2E_AK=... SPROXY_E2E_SK=... SPROXY_E2E_SKID=... go test -tags=e2e -run TestRemoteSaveRealE2E -v ./test/e2e
```

## 已知限制

- 多实例同时写同一任务未做分布式锁（依赖单实例锁文件）。
- 外部卷（百度）列举/Stat 最终一致；快速对账会核对远端存在与 checksum，但本地内容变化且
  size+mtime 不变需开 `verify_checksum` 或等待深度对账。
- 云端多文件转存目前按 sproxy `transfer_url` + `files[]` 逐个记录；若 sproxy 只返回单个
  transfer_url，状态以该主文件为准。
- 覆盖/改名执行中途崩溃的恢复：删除已幂等（NotFound 视为成功）、改名目标已存在且 checksum
  相同会复用；但 overwrite 删除后、上传前崩溃仍需重新执行一次动作。
