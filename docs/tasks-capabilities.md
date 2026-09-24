任务能力矩阵

- 核心抽象与通用组件
  - BaseTask：统一状态回填、关闭流程、去重缓存、共享注册表、路径策略、抓取驱动
  - PagingScanner：分页抓取管线（scrape → 去重 → 构建 → 持久化）
  - SiteAdapter：站点分页生命周期接口（BuildPageURL/RunScraper/ParseTotalPages/ParsePage/BuildObject）
  - PathStrategy：文件保存路径策略
  - GetCachedObject：缓存优先（跨任务共享 + 重启恢复）

- 任务能力对比
  - tktube
    - 分页：PagingScanner + SiteAdapter
    - 刷新：PagingScanner 增量抓取
    - 缓存：JSON（Load/SaveCache）
    - 路径策略：支持（字段 pathStrategy），工厂可注入
    - 内容分组：根据标题提取内容组名，写入对象 Metadata.content_group（示例：CLUB-100、CLUB-100C、【高画质】CLUB-100* 均归为 CLUB-100）
    - 自定义头：无
  - vikacg
    - 分页：用户帖子 API 分页（内部实现）
    - 刷新：PagingScanner 增量抓取（按 user_id>0 启用）
    - 缓存：JSON（Load/SaveCache）
    - 路径策略：无（图片按对象 SavePath 组织）
    - 自定义头：Cookie/User-Agent
  - hanime
    - 分页：PagingScanner + SiteAdapter
    - 刷新：PagingScanner 增量抓取
    - 缓存：JSON（Load/SaveCache）
    - 路径策略：支持（字段 pathStrategy），工厂可注入
    - 自定义头：Cookie

- 聚合与事件
  - 聚合：manager.AggregateObjects（分页、筛选、排序、搜索）
  - 事件：EventObjectUpdate、EventTaskUpdate、EventSharedObjectUpdate（SSE /api/events）

- 配置要点
  - tasks[].extra.path_strategy：first_fixed 等
  - tasks[].extra.refresh_interval：整数秒
  - tasks[].extra.headers：字典，按需传入
