任务能力矩阵

- 核心抽象与通用组件
  - BaseTaskImpl：统一状态回填、关闭流程、可选能力注入（刷新器/分页/路径策略/请求头）
  - CommonRefresher：周期刷新器（可更新间隔）
  - CommonPager：分页抓取器（遇已知即短路）
  - PathStrategy：文件保存路径策略

- 能力接口（按需实现，工厂自动注入）
  - PathStrategyCap.SetPathStrategy
  - RefreshingCap.SetRefresher
  - PagingCap.SetPager
  - HeadersCap.SetHeaders

- 任务能力对比
  - tktube
    - 分页：CommonPager（已用）
    - 刷新：CommonRefresher（已用）
    - 缓存：JSON（Load/SaveCache）
    - 路径策略：支持（字段 pathStrategy），工厂可注入
    - 自定义头：无
  - vikacg
    - 分页：用户帖子 API 分页（内部实现）
    - 刷新：CommonRefresher（按 user_id>0 启用），工厂可注入
    - 缓存：JSON（Load/SaveCache）
    - 路径策略：无（图片按对象 SavePath 组织）
    - 自定义头：Cookie/User-Agent
  - hanime
    - 分页：CommonPager（已用）
    - 刷新：CommonRefresher（已用）
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

