# Download Manager

面向多源任务的下载管理器。

## 工程约定
- 优先使用标准库：对 `errors`、`os/io`、`net/http`、`context`、`sync` 等常用能力，不再通过 pkg 层做薄封装
- 日志统一使用 `pkg/logutil` 初始化
- 单实例文件锁采用 `github.com/gofrs/flock`（Windows/macOS/Linux 兼容）

## 运行
```
go build
./download-manager --config config.yaml
```

UI 仅模式：
```
./download-manager --ui-only
```
