任务能力矩阵

- 核心抽象与通用组件
  - BaseTask：统一状态回填、关闭流程、去重缓存、共享注册表、路径策略、抓取驱动
  - PagingScanner：分页抓取管线（scrape → 去重 → 构建 → 持久化）
  - SiteAdapter：站点分页生命周期接口（BuildPageURL/RunScraper/ParseTotalPages/ParsePage/BuildObject）
  - PathStrategy：文件保存路径策略
  - GetCachedObject：缓存优先（跨任务共享 + 重启恢复）

- 任务能力对比
  - booksite（模板验证站点：xkcd 漫画站）
    - 分页：PagingScanner + SiteAdapter（按漫画 ID 递增，合成 URL 模式）
    - 刷新：PagingScanner 增量抓取
    - 缓存：JSON（Load/SaveCache）
    - 路径策略：无（图片按对象 SavePath 组织）
    - 自定义头：User-Agent
  - url_list
    - 无分页/爬取（URL 固定，直接下载）
    - 无详情解析（ResolveObject 空实现）
    - 自定义头：无
  - mock
    - 测试用：按 mock_rules 生成对象（fixture/Playwright e2e）
    - 自定义头：无
  - 站点任务（tktube / hanime / vikacg / njavtv / mxs）已外迁 **sdserver**，见 `github.com/cocomhub/sdserver/docs/deployment-guide.md`

- 聚合与事件
  - 聚合：manager.AggregateObjects（分页、筛选、排序、搜索）
  - 事件：EventObjectUpdate、EventTaskUpdate、EventSharedObjectUpdate（SSE /api/events）

- 配置要点
  - tasks[].extra.path_strategy：first_fixed 等
  - tasks[].extra.refresh_interval：整数秒
  - tasks[].extra.headers：字典，按需传入
