# P6-2 站点抓取走代理（sproxy+sclient 模式）设计（2026-09-24）

## 背景 / 目标

- 用户需求：新增网站任务时通过 `curl -x http://127.0.0.1:1080` 代理访问，而非直连网站——避免被管理员发现访问记录
- 现状：下载器 native 已支持代理（NewStaticProxySelector + http.Transport.Proxy）；但 **booksite 的 RunScraper 用 http.DefaultClient 直连**（task/booksite/adapter.go:71）
- scraper 下载器（downloader/scraper.go）已支持 proxies + forceProxy
- 目标：站点抓取（RunScraper）统一走代理（HTTP CONNECT 模式，sproxy+sclient 加密隧道对外）

## 组件与接口

- `task/booksite/adapter.go`（改）：RunScraper 用可注入的 http.Client（带 Proxy 配置），而非 DefaultClient
- `pkg/scraper_tunnel/tunnel.go`（改）：若支持代理注入，同样接入
- `downloader/scraper.go`（改）：确认代理选择器传递到抓取请求
- 配置：复用 `downloader.proxy.list` + `force_proxy`（用户已在 config.yaml 配 127.0.0.1:1080）
- 文档：`docs/proxy-setup.md`（sproxy+sclient 加密出口配置指南）

## 数据流

1. config.yaml `downloader.proxy.list: [http://127.0.0.1:1080]` + `force_proxy: true`
2. booksite RunScraper 构造 http.Client（Transport.Proxy 指向 127.0.0.1:1080）
3. 抓取 xkcd.com → 经本地 sproxy → sclient 加密隧道 → 出口节点 → 目标站
4. 管理员看到的是出口节点的访问，而非本地 IP

## 错误处理

- 代理不可达：抓取失败返回错误（fail-closed，不静默回退直连——用户明确要防暴露）
- 代理选择器健康检查：故障代理冷却后切换

## 测试 + 变异点

- 单测：booksite RunScraper 用 mock 代理服务器（httptest）验证请求经代理
- **变异点**：改回 DefaultClient 直连 → 测试红（断言请求必须经代理）

## 片划分

- P1：booksite RunScraper 代理注入 + 测试
- P2：scraper 下载器/隧道确认代理传递
- P3：docs/proxy-setup.md 文档

## 风险与零回归

- 仅 booksite 新站点走代理，现有任务（urllist/tktube 等）行为不变（它们已有下载代理机制）
- force_proxy=false 时行为不变（直连）
- 代理故障 fail-closed（不暴露直连）
