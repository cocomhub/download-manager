% Tasks

* [x] Task 1: 收敛并统一内容分组键生成

  * [x] SubTask 1.1: 在 `pkg/titlegroup` 新增统一分组键函数，成功解析返回合法组名，失败返回 `unknown+title`，空标题回退为 `unknown+url`

  * [x] SubTask 1.2: 替换现有 tktube 任务对象创建与启动回填逻辑，统一通过该函数生成 content_group

  * [x] SubTask 1.3: 单测覆盖正常解析、未知标题、空标题、大小写与前缀场景

* [x] Task 2: 启动时全量重算并纠偏保存

  * [x] SubTask 2.1: 将启动回填从“仅缺失时更新”改为“每次启动全量重算”

  * [x] SubTask 2.2: 当新旧 content_group 不一致时持久化更新并记录统计日志

  * [x] SubTask 2.3: 单测覆盖已有错误分组、已有正确分组、未知分组纠偏

* [x] Task 3: 加固组策略隔离与冲突保护

  * [x] SubTask 3.1: 将组内策略作用域收敛为 `task_id + task_type + content_group`

  * [x] SubTask 3.2: 定义四档优先级层级（HQ+C / HQ / Plain+C / Plain）及唯一性检测

  * [x] SubTask 3.3: 若同组同优先级存在多个对象，则跳过该组自动取消并记录冲突日志

  * [x] SubTask 3.4: 自动取消仅作用于 `pending` 等未开始状态，不得改写 `downloading`

  * [x] SubTask 3.5: 单测覆盖跨任务同名组、同优先级冲突组、downloading 对象保护

* [x] Task 4: 修正聚合与展开接口的安全边界

  * [x] SubTask 4.1: `group_by=content` 聚合按 `task_id + task_type + content_group` 分组

  * [x] SubTask 4.2: 未知对象按 `unknown+title` / `unknown+url` 独立分组，避免错误折叠

  * [x] SubTask 4.3: 组展开接口增加任务维度约束，避免跨任务同名组串组

  * [x] SubTask 4.4: 单测覆盖未知分组不合并、不同任务同名组不串联

* [x] Task 5: 收敛前端默认行为，避免误操作

  * [x] SubTask 5.1: 前端“查看分组”与批量取消请求附带任务维度信息

  * [x] SubTask 5.2: 冲突组或非安全组在 UI 中仅展示，不默认提供自动取消语义

  * [x] SubTask 5.3: 对 `downloading` 对象禁用“低优先级自动取消”相关操作提示

  * [x] SubTask 5.4: 端到端或前端逻辑验证上述保护生效

* [x] Task 6: 显式收紧组策略作用域

  * [x] SubTask 6.1: 在自动取消策略中显式校验 `task_id + task_type + content_group`，不得依赖存储天然隔离

  * [x] SubTask 6.2: 补充跨任务共用存储的测试，验证同名组不会被错误参与自动取消

  * [x] SubTask 6.3: 重新核验并补齐 checklist 第 3 条

% Task Dependencies

* [Task 2] depends on [Task 1]

* [Task 3] depends on [Task 1], [Task 2]

* [Task 4] depends on [Task 1], [Task 3]

* [Task 5] depends on [Task 4]

* [Task 6] depends on [Task 3]
