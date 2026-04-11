% Tasks

* [x] Task 1: 存量对象分组字段回填

  * [x] SubTask 1.1: 在启动流程添加一次性回填流程（可幂等），扫描所有对象

  * [x] SubTask 1.2: 对 tktube 类型对象，调用 TKTGroupNameFromTitle 填充 Metadata.content_group（缺失时）

  * [x] SubTask 1.3: 持久化更新并记录回填统计；新增单测验证内存存储/文件存储的回填

* [x] Task 2: 分组内优先级模型（tktube）

  * [x] SubTask 2.1: 新增解析函数，抽取 has_hq/has_c 两个标记（title 前缀与后缀）

  * [x] SubTask 2.2: 定义比较器：先 has_hq 再 has_c，降序；平级保持稳定排序；将结果以 Extra.priority 或 Extra.variant 标记存储

  * [x] SubTask 2.3: 单测覆盖多种组合比较（含同组多对象）

* [x] Task 3: 自动取消低优先级与跳转记录

  * [x] SubTask 3.1: 在对象状态更新为 completed 时触发同组检查（manager 或 task 层），选出最高优先级对象

  * [x] SubTask 3.2: 将低优先级的未完成对象置为 canceled，并写入 Extra.redirect_url 指向最高优先级对象 URL

  * [x] SubTask 3.3: 单测验证状态变更与 redirect_url 的正确性与幂等

* [ ] Task 4: 播放/打开跳转逻辑

  * [ ] SubTask 4.1: API/前端在打开对象时，如检测 status=canceled 且存在 redirect_url，执行前端导航至该链接

  * [ ] SubTask 4.2: 单测/端到端验证点击被取消对象可跳转到高优先级对象

* [x] Task 5: API 聚合与展开

  * [x] SubTask 5.1: 对象列表接口新增 group_by=content 参数，返回各组代表对象与 group_size

  * [x] SubTask 5.2: 新增 /api/groups/{group}/objects 接口返回指定组内的全部对象

  * [x] SubTask 5.3: 单测覆盖聚合与展开接口（内存存储）

* [ ] Task 6: UI 分组开关与批量操作

  * [ ] SubTask 6.1: 在页面加入“按内容分组”开关（默认关闭以保持兼容）

  * [ ] SubTask 6.2: 聚合视图展示代表对象，显示 group_size，点击展开弹出同组对象列表

  * [ ] SubTask 6.3: 在展开视图提供“删除低优先级并取消下载”按钮，批量调用接口更新状态（可选删除文件）

  * [ ] SubTask 6.4: 端到端验证开关、展开、批量取消流程

% Task Dependencies

* [Task 2] depends on [Task 1]

* [Task 3] depends on [Task 2]

* [Task 4] depends on [Task 3]

* [Task 5] depends on [Task 1], [Task 2]

* [Task 6] depends on [Task 5]
