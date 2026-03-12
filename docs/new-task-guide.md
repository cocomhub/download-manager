新任务开发指南

- 最小实现点
  - 解析分页入口（可选，若需要分页：实现 CommonPager 所需 PageFuncs）
  - 解析详情/对象构建（ParseDetail/BuildObject 的等价逻辑）
  - 返回对象列表：实现 core.Task.GetDownloadObjects

- 建议组合
  - 通过实现能力接口让工厂自动注入：
    - PathStrategyCap（SetPathStrategy）
    - RefreshingCap（SetRefresher）
    - HeadersCap（SetHeaders，可选）
  - 直接复用 CommonPager/Refresher 与 PathStrategy

- 状态回填与去重
  - 在生成对象时调用存储/共享注册表恢复最新状态；未完成对象重启时需将 Downloading → Pending，并清理延迟解析的 files 字段

- 命名规范
  - 优先通过 PathStrategy 统一生成路径，避免各任务分散
  - 文件名避开非法字符：/、\\、结尾点等

- 测试建议
  - 单元测试覆盖：分页终止条件、刷新增量、状态回填、共享对齐
  - 使用本地示例 HTML 做快照/解析测试，尽量避免对线上站点施压

