# 配置参考（config.yaml）

> 配置结构定义见 `config/config.go`；`ValidateAndClamp` 处理默认值与旧字段迁移。

## 顶层结构

```yaml
server:            # HTTP 服务与工作目录
log:               # 日志（slog + lumberjack）
mongo:             # MongoDB 存储源列表
downloader:        # 下载器配置（native/wget）
task_scan:         # 任务扫描周期
runtime:           # 运行模式与功能开关
contexts:          # 命名任务上下文（storage 等）
tasks:             # 任务定义列表
task_type_defaults:# 任务类型默认配置
```

## server

| 字段 | 类型 | 说明 |
|------|------|------|
| `http_port` | int | full 模式 HTTP 端口 |
| `ui_only_port` | int | UI 模式端口 |
| `work_dir` | string | 工作目录（缓存等） |
| `lock_file` | string | full 模式单实例锁文件 |
| `ui_only_lock_file` | string | UI 模式锁文件 |
| `download_root_dir` | string | 下载根目录（落盘） |
| `files_dir` | string | HTTP `/files/` 服务根目录（优先于 download_root_dir） |
| `auth` | object | 鉴权（见下） |
| `ui_defaults` | object | UI 默认值（保存目录/窗口/样式） |

### auth

```yaml
auth:
  type: none        # none | basic | token
  username: admin   # basic 用户名（默认 admin）
  password: ""      # basic 密码（DM_AUTH_PASSWORD 优先）
  token: ""         # token 值（DM_AUTH_TOKEN 优先）
  expires_at: ""    # token 过期（RFC3339，空 = 不过期；DM_AUTH_TOKEN_EXPIRES 优先）
```

- `DM_AUTH_ENABLED=1` 且 type 未设/none → 自动 basic（密码缺省 "admin" 并告警）
- `/api/healthz`、`/api/runtime`、`/api/auth/verify` 与静态资源豁免鉴权；`/api/` 与 `/files/` 受保护

## downloader

| 字段 | 说明 |
|------|------|
| `type` | native / wget / gopeed / sproxy_cloud |
| `global_concurrent` | 全局并发 worker 数 |
| `sequential` | true=每任务同时只下载 1 个对象（任务内串行，按返回顺序逐个），默认 false=保持并发 |
| `max_retries` | 失败重试次数 |
| `log_dir` | 下载器日志目录 |
| `force_proxy` | 强制走代理 |
| `proxies` | 代理列表（轮换 + 故障切换） |
| `domain_limits` | 域名限流（`domain: 并发数`） |
| `ffmpeg_path` | ffmpeg 路径（HLS 合并） |
| `hls_auto_mark_as_fail` | HLS 下载失败自动标记 |
| `filesystem` | 文件系统：`root_dir` / `log_dir` / `cache_dir` / `allow_paths` / `follow_symlinks` |
| `http` | HTTP 客户端：`timeout_seconds` / `idle_conn_timeout_seconds` / `max_idle_conns` / `max_idle_conns_per_host` / `default_user_agent` / `disable_inject_browser_like_headers` |
| `proxy` | 代理决策：`force` / `list` / `decision_cache_ttl_secs` / `direct_probe_timeout_secs` / `bandwidth_path_suffix` |
| `progress` | 进度回调：`min_percent_step` / `max_interval_seconds` |
| `ffmpeg` | `path` / `extra_args` / `move_if_exists` / `external_hls_log` |

### downloader.sproxy_cloud

将 URL 提交给 sproxy 云端下载服务（sproxy 侧完成下载/转存；PikPak 分享链接走其 hybrid 策略）；
download-manager 只做任务解析/提交/轮询。**通用**：任意 URL 均可交 sproxy（其按 URL 自动发现后端），
PikPak 分享链接（keepshare / mypikpak）为特化优先来源。认证优先 SproxySig（推荐），未配置时回落 Bearer。

| 字段 | 说明 |
|------|------|
| `api_url` | sproxy cloud download API（默认 `http://127.0.0.1:8080/api/cloud/download`） |
| `access_key` | SproxySig AccessKey |
| `access_key_secret` | SproxySig AccessKeySecret（仅本地算签名，永不上线） |
| `access_key_id` | SproxySig SK 条目 ID（`skey-id`，v2 必传） |
| `api_token` | 旧 Bearer token（未配 SproxySig 三件套时使用，向后兼容） |
| `transfer_volume` | 转存目标卷（非空则下载完成后转存到该卷）；留空 = 产物仅留 cloud 桶 |
| `transfer_path` | 转存目标路径（卷内相对路径，可含子目录，如 `xxx/xxxx.mp4`）；仅 `transfer_volume` 非空时生效 |
| `cloud_only` | `false`（默认）= 完成后把原始文件下载到本地 `SavePath`；`true` = 仅留云端不下载（只转存不拉回） |
| `poll_every` | 任务轮询间隔（默认 5s） |
| `timeout` | 单任务总超时（默认 3h） |
| `client_timeout` | 显式覆盖单请求 HTTP 超时（默认沿用 sproxy 的 300s） |

#### 下载项级「云端下载」选项

是否走云端下载由**每个下载项**控制（而非全局下载器类型）：`DownloadObject.cloud_download`（bool）。
任务在创建/解析对象时自行决定是否置位；置位的下载项由 Manager 路由到 sproxy_cloud（未配置 `api_url`
时回落默认下载器并告警）。切换方式：

- API：`POST /api/tasks/{id}/object/cloud_download`，body `{"url": "...", "enabled": true}`
- Web UI：对象列表的「云端」按钮（写入受 `uiMode` / 写保护约束）

说明：
- SproxySig 三件套（`access_key` / `access_key_secret` / `access_key_id`）需同时配置；启动时会带外验证签名链路，失败则该后端拒绝任务（fail-closed）。
- 转存成功后产物引用写入对象 `Extra`：`transfer_url`（`sproxy://<卷>/<路径>`）、`cloud_task_id`、`cloud_task_filename`。
- 对象取消/服务停机通过 `Cancel` / 注入上下文中断在途轮询。

## tasks

```yaml
tasks:
  - id: my-task          # 唯一 ID
    type: url_list         # url_list | booksite | mock（站点任务见 sdserver）
    save_dir: ./downloads
    save_sub_dir: ""     # 可选子目录
    storage:             # 覆盖默认存储
      type: file         # memory | file | mongo
      config:
        path: ./data
    storage_context: ""  # 引用 contexts 下的命名配置
    scrape_enabled: true
    download_enabled: true
    extra:               # 任务类型特定字段
      path_strategy: first_fixed
      refresh_interval: 3600
      headers: { Cookie: "..." }
```

### extra 常见字段

- `path_strategy`：`first_fixed`（首段固定）等
- `refresh_interval`：抓取刷新周期（秒）
- `headers`：自定义请求头（Cookie / User-Agent）

## task_type_defaults

```yaml
task_type_defaults:
  urllist:
    storage: { type: file, config: { path: ./data } }
    save_root_dir: ./downloads
    scrape_enabled: true
    download_enabled: true
    extra: { ... }
```

## contexts

```yaml
contexts:
  mongo-main:
    storage:
      type: mongo
      config:
        uri: mongodb://root:root123@host:27017/dm?authSource=admin
        database: dm
        collection: objects
```

## mongo

```yaml
mongo:
  - name: default
    uri: mongodb://root:root123@host:27017/download_manager?authSource=admin
```

## 运行模式

```yaml
runtime:
  mode: full           # full | ui
  log_level: info
  download: { enabled: true }
  scheduler: { enabled: true }
```

## task_scan

```yaml
task_scan:
  disable: false
  interval: 60         # 秒
```
