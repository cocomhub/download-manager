# 外部化 dlcore 硬编码与 RootFS 防护 Spec

## Why
当前 pkg/dlcore 存在多处硬编码（路径、超时、UA、缓冲区大小、权限等），且包含环境专属绝对路径（/Volumes…、/opt…）。这导致难以跨环境复用，并带来路径安全隐患。通过将硬编码外部化为配置，并在文件系统层面采用 Go 1.24 的 os.Root 构建“根目录”，可统一路径约束、防御目录遍历并降低配置负担。

## What Changes
- 新增 Downloader 配置结构，系统化管理 HTTP、代理、I/O、进度、FFmpeg、路径相关设置
- 新增 downloader.root_dir，并使用 os.Root 将 dlcore 的文件操作限制在该根目录下
- 将默认 UA、默认首部、HTTP/探测超时、缓存 TTL、缓冲区大小、权限掩码、进度阈值等由硬编码改为配置项
- 统一日志与缓存相对路径：log_dir、cache_dir 等相对于 root_dir 解析
- 移除平台专属绝对路径逻辑；如确需“已存在文件迁移/外部 HLS 引导日志”，改为在 root_dir 下的可选配置
- 兼容现有 Option：既有 WithLoggerDir/WithFFmpegPath 等不变，内部映射到新配置；未显式提供 root_dir 时，默认采用 <server.work_dir>/downloads
- 与 Web 文件服务保持一致：默认 root_dir 与 Manager 的下载根目录一致，避免歧义

## Impact
- Affected specs:
  - 配置模型：config.downloader.* 新增与调整
  - 路径与安全：所有 dlcore 文件写读受 RootFS 约束，越界拒绝
  - 下载体验：默认行为与现状保持一致，允许通过配置精细化调整
- Affected code:
  - pkg/dlcore/client.go（文件写读、默认参数、首部/UA、缓冲/权限、探测 TTL 等）
  - pkg/dlcore/ffmpeg.go（ffmpeg 可执行与参数、日志路径、绝对路径去除）
  - pkg/dlcore/option.go（新增 WithRootDir/WithConfig，兼容旧 Option）
  - downloader/native.go（从配置派生并传入 RootDir 与扩展项）
  - config/config.go & config/global.go（downloader 字段与默认值、校验）
  - 示例配置 config.yaml（新增/调整字段）

## ADDED Requirements
### Requirement: RootFS 目录沙盒
系统 SHALL 使用 Go 1.24 os.Root 基于 downloader.root_dir 获取受限文件系统；dlcore 的所有路径操作（含保存文件、日志、缓存、重命名等）必须在该根内进行，越界一律拒绝并返回明确错误。

#### Scenario: 成功保存文件
- WHEN 任务提供 SavePath="videos/abc.mp4"
- AND root_dir 已设置为 "<work_dir>/downloads"
- THEN 系统在 "<work_dir>/downloads/videos/abc.mp4" 内创建/写入文件并成功完成

#### Scenario: 阻止目录遍历
- WHEN 任务提供 SavePath="../../etc/passwd"
- THEN 路径校验失败并拒绝写入（返回“越界路径”错误）

### Requirement: 外部化默认参数
系统 SHALL 通过配置项提供如下默认值并允许覆盖：
- HTTP：timeout_seconds、idle_conn_timeout_seconds、max_idle_conns、max_idle_conns_per_host、default_user_agent、default_headers（可禁用/启用浏览器风格首部集）
- 代理策略：proxies、force_proxy、decision_cache_ttl_seconds、direct_probe_timeout_seconds、bandwidth_path_suffix
- I/O：buffer_size、file_mode、dir_mode
- 进度：min_percent_step、max_interval_seconds
- FFmpeg：path、extra_args、hls_auto_mark_as_fail、可选的“已存在文件迁移”与“外部 HLS 引导日志”均需显式启用且路径在 root_dir 内

### Requirement: 相对路径解析一致性
系统 SHALL 将 downloader.log_dir、downloader.cache_dir、FFmpeg 日志目录等相对路径统一视为相对于 root_dir 的子路径；若提供绝对路径且不在根内，系统必须拒绝并报错。

## MODIFIED Requirements
### Requirement: 现有 Downloader 配置语义
- 原有 downloader.log_dir 若为空则按默认相对路径 "logs" 解析到 <root_dir>/logs
- 原有 downloader.ffmpeg_path 保持兼容；未设置时使用 "ffmpeg"
- 默认 UA/首部从硬编码调整为配置派生的默认集；调用方仍可在请求级别覆盖
- 域名并发限制沿用现有下发通道不变

## REMOVED Requirements
### Requirement: 环境专属绝对路径
**Reason**: 不可移植且破坏 RootFS 沙盒
**Migration**:
- “已存在文件迁移”替换为可选配置：downloader.ffmpeg.move_if_exists.enabled=true 与 .dir="completed"（相对 root_dir）
- “外部 HLS 引导日志”替换为可选配置：downloader.ffmpeg.external_hls_log.enabled=true 与 .path="logs/hls-bootstrap.log"

