# Tasks

* [x] 设计配置模型与默认值

  * [x] 在 config.Config.Downloader 中新增字段分组：filesystem、http、proxy、io、progress、ffmpeg

  * [x] 定义默认值与 ValidateAndClamp 逻辑，保持与现有行为兼容

  * [x] 更新示例 config.yaml 与注释

* [ ] 引入 RootFS 路径沙盒

  * [x] 在 dlcore 内部新增 Root 解析器（基于 os.Root）与路径校验/Join

  * [x] 新增 Option：WithRootDir / WithConfig（承载上述配置）

  * [x] downloader/native 初始化时派生 root\_dir（默认 \<server.work\_dir>/downloads）并传入

* [ ] 替换文件系统操作

  * [x] 将 pkg/dlcore/client.go 的 MkdirAll/Stat/OpenFile/Rename/Chtimes/WriteFile 等切换为 RootFS

  * [x] 将 pkg/dlcore/ffmpeg.go 的日志文件写入与输出文件名解析切换为 RootFS

  * [x] 将代理缓存目录迁移到 root 内的 cache\_dir（不再使用 os.TempDir）

* [ ] 外部化硬编码参数

  * [x] 将默认 UA/headers、HTTP 超时与连接池、探测超时与 TTL、缓冲区大小、权限掩码、进度阈值参数化

  * [x] 为 ffmpeg 提供 extra\_args（以当前默认参数为默认值），并外部化 HLS 相关开关

  * [x] 删除环境专属绝对路径逻辑，改为可选配置（在 root 内）

* [ ] 兼容与迁移

  * [x] 继续支持既有 Option（WithLoggerDir、WithFFmpegPath 等），内部映射到新配置

  * [x] 若未配置 root\_dir，则自动采用 \<server.work\_dir>/downloads

  * [ ] 文档化迁移注意事项

* [ ] 测试与验证

  * [x] 新增路径越界单测：包含 ".."、绝对路径均被正确处理

  * [x] 新增配置默认值回归测试，确保行为与现状一致

# Task Dependencies

* \[引入 RootFS 路径沙盒] 依赖 \[设计配置模型与默认值]

* \[替换文件系统操作] 依赖 \[引入 RootFS 路径沙盒]

* \[外部化硬编码参数] 依赖 \[设计配置模型与默认值]

* \[兼容与迁移] 依赖 \[设计配置模型与默认值]

* \[测试与验证] 依赖全部实现任务完成后进行

