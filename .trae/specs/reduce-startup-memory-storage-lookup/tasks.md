# Tasks
- [x] Task 1: 定义 Storage 查询契约并收敛对象访问入口
  - [x] SubTask 1.1: 为 `Storage` 设计分页、过滤、排序、计数与批量存在性检查能力，覆盖任务详情、聚合、分组与单对象定位场景
  - [x] SubTask 1.2: 识别并替换 Manager / API / Task 中对 `GetAllObjects()`、`Search(nil)`、内存切片全量遍历的依赖点
  - [x] SubTask 1.3: 明确 file / mongo / memory 三类存储的兼容语义，保证调用方统一走 `Storage`

- [x] Task 2: 移除 cache 文件并改造冷启动流程
  - [x] SubTask 2.1: 删除任务级 `LoadCache` / `SaveCache` 启动与停机路径，不再创建或依赖 `<work_dir>/cache`
  - [x] SubTask 2.2: 移除启动时对各任务 `Storage.Search(nil)` 的全量冷启动回灌
  - [x] SubTask 2.3: 将共享状态对齐、内容分组回填等全量启动动作改为按需、分批或显式维护流程

- [x] Task 3: 将对象列表与操作迁移为 Storage 驱动
  - [x] SubTask 3.1: 将任务详情、全局聚合、内容分组、组展开接口改为使用 `Storage` 分页查询
  - [x] SubTask 3.2: 将重试、取消、撤销取消等单对象操作改为通过 `Storage` 精确定位，不依赖全量内存对象
  - [x] SubTask 3.3: 仅对正在下载或刚更新对象保留轻量运行时缓存，避免重新引入全量常驻副本

- [x] Task 4: 优化任务刷新与去重路径
  - [x] SubTask 4.1: 将任务增量刷新时的历史对象判断从全量 `knownURLs`/`objects` 改为 `Storage` 精确查重或批量查重
  - [x] SubTask 4.2: 梳理 `tktube`、`vikacg` 等任务的状态恢复与对象构造流程，确保懒加载后行为一致
  - [x] SubTask 4.3: 为需要的短期缓存增加容量边界与失效策略，避免查询路径再次堆积内存

- [x] Task 5: 强化 Mongo 场景
  - [x] SubTask 5.1: 为 `mongo` 存储实现服务端过滤、排序、分页与计数，禁止列表请求走全集解码
  - [x] SubTask 5.2: 设计并补齐常用查询索引，重点覆盖 `task_id`、`status`、`metadata.content_group`、时间排序及 URL 精确查找
  - [x] SubTask 5.3: 为大集合请求设置显式边界，避免无过滤全表扫描进入请求主路径

- [x] Task 6: 验证行为与回归风险
  - [x] SubTask 6.1: 补充冷启动回归验证，确认大数据量下不再出现全量加载与 cache 文件依赖
  - [x] SubTask 6.2: 补充 `mongo` 分页/过滤/计数测试，确认不会通过 `cursor.All` 拉全量数据
  - [x] SubTask 6.3: 补充任务详情、聚合、分组、单对象操作回归测试，确认功能行为与现有接口保持兼容

# Task Dependencies
- [Task 2] depends on [Task 1]
- [Task 3] depends on [Task 1], [Task 2]
- [Task 4] depends on [Task 1], [Task 2]
- [Task 5] depends on [Task 1]
- [Task 6] depends on [Task 3], [Task 4], [Task 5]
