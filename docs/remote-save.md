# 远程保存（download-manager → sproxy）

> 状态：A/B/C 已实现；UI 冲突处理面板待接入（后端接口已就绪）。

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

- `reconcile_interval`：快速对账（manifest 命中不访问远端）。
- `deep_reconcile_interval`：深度对账（忽略 manifest，重算 sha256 + 校验远端）。
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
- 只上传/复用，不删除远端；本地改名/删除不会传播删除。

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
- 目标已存在 → **fail-closed**，对象 `failed_permanent` + `RemoteSave=conflict`，等待人工处理。
- 转存完成后 `NotifyCloudCompleted` 回读 sproxy 的 sha256/mtime，写入 mongo 与播放链接；
  非 cloud_only 对象本地文件走正常对账（manifest 记录 origin=cloud，避免重复上传）。

## 已知限制

- 多实例同时写同一任务未做分布式锁（依赖单实例锁文件）。
- 外部卷（百度）列举/Stat 最终一致；快速对账不访问远端，深度对账周期内可能短暂不一致。
- 本地内容变化但 size+mtime 不变默认检测不到；开 `verify_checksum` 或深度对账可发现。
- 云端冲突的 reuse/overwrite/rename 动作与 UI 面板待接入；当前 API 以 fail-closed 为主。
