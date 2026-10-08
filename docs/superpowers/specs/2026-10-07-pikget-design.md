# pikget —— 类 wget 下载器设计规格

> 状态：设计规格（待实现）
> 版本：v1
> 日期：2026-10-07
> 分支：feat/pikget-wget（基于 origin/master @ 3bb5de8）

---

## 1. 背景与目标

download-manager 现有一个完整下载能力栈，但缺少一个**独立的、wget 风格的命令行下载器**：
用户拿一个 URL（普通直链 / PikPak 分享链接），一条命令把它下到本地，自动选择合适的
下载实现。

目标：
- `pikget '<url>' -o <dest>` 一条命令下载完成，行为对齐 wget（进度、断点续传、退出码）。
- URL 自动分发：普通 `http(s)` 直链 → download-manager 自研 `pkg/download`（Range 续传、
  ETag、MD5 校验）；`mypikpak.com/s/`、`keepshare.org/` 分享链接 → sproxy 的
  `pikpak-hybrid` 混合下载器（分享直链前段 + 账号流量后段，分片并行，多账号轮换）。
- 零外部常驻服务：hybrid 能力通过 **本地内嵌 sproxy 的 pikpak 独立 module**（replace 指
  向 `../sproxy/pkg/volume/ext/pikpak`），不依赖运行中的 sproxy daemon。
- 非目标：磁力（`magnet:`/`bt:`）第一版不支持，报「暂不支持」；不做 Web UI；不做 daemon。

## 2. 用户决策（2026-10-07 确认）

| 决策点 | 结论 |
|---|---|
| PikPak 混合下载接入形态 | **本地内嵌 sproxy 模块**（go.mod replace → `../sproxy/pkg/volume/ext/pikpak`，进程内直接调用 HybridDownloader） |
| 下载引擎分发范围 | **直链 + PikPak 分享**（磁力第一版报暂不支持） |
| 配置存储 | **本地 YAML 配置文件**（`~/.config/pikget/config.yaml`）+ 账号凭据本地落盘 |
| 模块归属 | **download-manager 内新增子命令**（`cmd/pikget/`） |
| CLI 名称 | **pikget**（wget 的 P 版：直链/分享一条命令下完） |

## 3. 架构总览

```
pikget（download-manager/cmd/pikget/，子命令二进制）
   │
   ├─ main.go        flag 解析、装配、退出码
   ├─ cli.go         URL 分发决策（直链 vs 分享 vs 不支持）
   ├─ direct.go      直链后端：download-manager pkg/download（HTTPExtractor）
   ├─ hybrid.go      PikPak 分享后端：sproxy pikpak.HybridDownloader 装配
   ├─ config.go      ~/.config/pikget/config.yaml 加载 + 默认值
   └─ progress.go    进度渲染（-q 静默 / 默认逐行进度 / 完成摘要）

download-manager/ 根 go.mod
   require github.com/cocomhub/sproxy/pkg/volume/ext/pikpak v0.0.0
   replace  github.com/cocomhub/sproxy/pkg/volume/ext/pikpak => ../sproxy/pkg/volume/ext/pikpak

sproxy/pkg/volume/ext/pikpak（独立 Go module，**只复用不改**）
   ShareResolver（纯 Go 匿名分享解析：captcha 签名 → share detail → 直链）
   HybridDownloader（分享区 [0,50%) 匿名直链分片 + 账号区 [50%,total) FETCH 直链分片）
   AccountPool（多账号配额轮换 / 会话切换 / 冷却）
   API / Cli / FSSecretStore / DeletePermanent
```

复用边界：**pikget 不复制 sproxy 任何实现**；只 require + replace 该独立 module。
sproxy 根 module（含 netutil/sizex/downloader 接口）会作为间接依赖被拖入，属用户已确认的权衡。

## 4. URL 分发规则

| URL 特征 | 下载器 | 说明 |
|---|---|---|
| `http://` / `https://` 普通直链 | `pkg/download.New()`（HTTPExtractor + StdlibTransport） | 续传 / ETag / MD5 |
| `mypikpak.com/s/`、`mypikpak.net/s/`、`keepshare.org/`、`keepshare.cc/` 分享 | sproxy `HybridDownloader` | 混合下载 |
| `magnet:`、`bt:` 或其它未知 scheme | 报错 `pikget: unsupported URL scheme` | 第一版不支持 |

判定实现：参考 download-manager `downloader/sproxy_hybrid.go:isShareURL` 的 host 匹配
（`strings.Contains(low, "mypikpak.com/s/")` 等），pikget 内自建 `dispatch(url) kind` 函数。

## 5. CLI 形态

```
pikget [flags] <URL>

Flags:
  -o, --output <path>      输出文件路径或目录（默认：取 URL 文件名到当前目录）
  -c, --concurrency <n>    直链并发/分享分片并发（默认 4）
  -q, --quiet              静默模式（仅错误输出，无进度）
  -v, --verbose            详细日志（slog debug 级）
  -h, --help               用法
      --version            版本
      --config <path>      配置文件路径（默认 ~/.config/pikget/config.yaml）
      --no-resume          禁用断点续传
```

直链特有（复用 pkg/download）：
```
      --user-agent <ua>    UA（默认 chromium 桌面 UA）
      --proxy <url>        HTTP 代理（http/socks5）
      --header 'K: V'      自定义请求头（可重复）
      --timeout <sec>      HTTP 超时（默认 300）
      --retry <n>          重试次数（默认 3）
```

分享特有（装配 sproxy hybrid）：
```
      --share-ratio <r>    分享区比例（默认 0.5，恒 ≤0.5）
      --chunk-size <bytes> 分片大小（默认 64MiB）
      --auto-delete        完成后永久删除转存副本（默认 false；大文件建议 true 释放空间）
      --pikpak-accounts-dir <dir>  账号会话凭据目录（默认 ~/.pikpak/）
      --pikpak-state-dir <dir>     账号配额状态目录（默认 <config>/pikpak-account-state）
```

退出码：
- `0` 下载成功（含分享降级成功）
- `1` 下载失败 / 不支持 URL
- `2` 参数错误

## 6. 配置（config.yaml）

位置：`~/.config/pikget/config.yaml`（可 `--config` 覆盖）。`~/.config` 取
`os.UserConfigDir()`。文件不存在时用默认值 + 首次运行时落盘示例（可选项）。

```yaml
downloader:
  http:
    user_agent: "Mozilla/5.0 ... Chrome/145"
    proxy: ""
    timeout_secs: 300
    max_retries: 3
    headers:
      - "Referer: https://example.com"
  pikpak:
    share_ratio: 0.5        # 分享区比例（恒 ≤0.5）
    chunk_size: 67108864    # 64MiB
    concurrency: 4
    auto_delete: false      # 完成后永久删转存副本
    accounts_dir: ""        # 默认 ~/.pikpak/
    state_dir: ""           # 默认 <UserConfigDir>/pikget/pikpak-account-state
    secrets_dir: ""         # 账号凭据落盘目录（FSSecretStore 底层）
```

账号凭据：复用 `pikpak.FSSecretStore`（封装一个本地目录为 SecretStore）+ `AccountPool`
（Select/Use/RecordUsage/MarkFailed）。本地 CLI 工具场景**直接落盘明文 JSON 到
`~/.pikpak/`** 即可（与 sproxy CLI 同目录语义）；加密卷（shardseal）非 CLI 场景必须，
可后续接入——配置留 `secrets_dir` 字段。

## 7. 直链后端（direct.go）

复用 download-manager `pkg/download`：

```go
func downloadDirect(ctx context.Context, cfg config.HTTP, url, dest string, onProg func(float64, int64, int64)) error {
    dl := download.New()
    // StdlibTransport + 可选代理 + UA + headers
    req := &download.Request{
        URL: url, SavePath: dest,
        Headers: cfg.headers,
        TrackProgress: true,
        OnProgress: func(p float64, downloaded, total int64) { onProg(p, downloaded, total) },
    }
    return dl.Download(ctx, req)
}
```

能力继承：Range 续传（`prepareDownloadOffset`）、弱 ETag + If-None-Match、MD5 校验
（`computeFileMD5`）、ResponseCheck 钩子、域名限流（可选装配）。

## 8. PikPak 分享后端（hybrid.go）

装配 sproxy `pikpak.HybridDownloader`：

```go
func newHybrid(cfg config.Pikpak) (*pikpak.HybridDownloader, error) {
    resolver := pikpak.NewShareResolver(pikpak.ShareResolverConfig{ /* UA 等 */ })
    api := pikpak.NewAPI(pikpak.APIConfig{ /* host/token 可空走 CLI 会话 */ }, cli)
    // 账号池（可选）：secrets store + 会话/状态目录
    var pool *pikpak.AccountPool
    if cfg.hasAccounts() {
        store := pikpak.NewFSSecretStore(someFS)
        pool, _ = pikpak.NewAccountPool(pikpak.AccountPoolConfig{
            Secrets: store, CredentialsDir: cfg.AccountsDir, StateDir: cfg.StateDir,
        })
    }
    return pikpak.NewHybridDownloader(pikpak.HybridConfig{
        Resolver: resolver, API: api, AccountPool: pool,
        ChunkSize: cfg.ChunkSize, ShareRatio: cfg.ShareRatio,
        Concurrency: cfg.Concurrency, AutoDelete: cfg.AutoDelete,
    })
}
```

进度适配：`HybridDownloader.Download(ctx, source, destPath, onProgress)` 的
`onProgress` 为 `downloader.ProgressFunc`（sproxy 侧类型），包装为 pikget 进度回调
（下载字节 / 总字节）。

降级语义（sproxy 内建，pikget 透传）：匿名分享 resolve 失败 / 分享段连续两错 → 降级
账号段或整任务委托 fallback（未装配 fallback 时如实报错）。pikget 不重复实现。

## 9. 错误处理

| 场景 | 行为 |
|---|---|
| 不支持 scheme | stderr `pikget: unsupported URL: <url>`，退出 1 |
| 直链失败（网络/校验） | 包装错误 + `-v` 详情，退出 1 |
| 分享降级后成功 | 成功，`-v` 记录降级 metric 摘要 |
| 分享全失败 | 包装错误（含 sproxy 错误链），退出 1 |
| 中断（Ctrl-C / SIGTERM） | ctx cancel → 保留 `.partial`（直链）/ 分片中间态（hybrid），退出 130 |

## 10. 测试计划（TDD）

| 层 | 用例 | 技术 |
|---|---|---|
| cli 分发 | `dispatch()` 对 直链/3 种分享 host/磁力/未知 scheme 的分类 | 纯函数单测 |
| 直链 e2e | 本地 `httptest` server：Range 续传、ETag、MD5、失败重试 | `pkg/download` 已有能力 + 小 mock |
| 分享装配 | `newHybrid` 无账号 / 有账号（fake SecretStore）构造成功 | fake + 参数断言 |
| hybrid 真实链路 | （可选，需真分享 URL + 账号）标记 `manual`，CI 跳过 | 手测验证 |
| 进度 | onProgress 回调收到 0→100 序列 | mock 后端回调断言 |
| 退出码 | 支持/不支持/失败三种退出码黑盒测试（`exec` 子进程或 `RunE` 返回） | cmd 测试 |

## 11. 落点文件清单

```
download-manager/
├── go.mod                                  # + require/replace sproxy pikpak 子模块
├── go.sum                                  # 新增间接依赖
├── cmd/pikget/
│   ├── main.go                             # 装配 + 退出码
│   ├── cli.go                              # flags + URL 分发
│   ├── direct.go                           # 直链后端
│   ├── hybrid.go                           # PikPak 分享后端装配
│   ├── config.go                           # config.yaml 加载
│   ├── progress.go                         # 进度渲染
│   ├── cli_test.go                         # 分发/退出码黑盒
│   ├── direct_test.go                      # 直链 e2e
│   └── hybrid_test.go                      # 装配测试
└── docs/superpowers/specs/2026-10-07-pikget-design.md   # 本文档
```

注意：`cmd/pikget` 是普通子目录命令（非独立 go.mod），沿用 download-manager 单一 module；
新增依赖会进根 go.mod。`.notestignore` 无需改动（cmd/pikget 带测试）。

## 12. 风险与后续

- **依赖连锁**：根 go.mod require sproxy 子模块 → 间接引入 sproxy 根 module（netutil/
  sizex/downloader 接口）。`make build-all`（遍历子 module）可能受影响——CI 验证时确认。
- **worktree 复用**：工作区 `dm-wt-pikget` 与 sproxy 同级（`../sproxy`），replace 相对路径
  在 go build 时解析到仓库内 sproxy，可正常构建。
- **后续片**（非本期）：磁力支持（接 Gopeed）、账号加密卷（shardseal）、分享目录递归
  打包下载、`-r` 递归下载分享文件夹。
