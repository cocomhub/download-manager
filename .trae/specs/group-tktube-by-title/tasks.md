% Tasks

* [x] Task 1: 提供 tktube 内容组名解析器

  * [x] SubTask 1.1: 新增解析函数，输入 title 字符串，输出 content\_group

  * [x] SubTask 1.2: 实现去除前缀标签、正则匹配 (\[A-Z]+-\d{2,4})(?:C)?\b，返回捕获组 1 的大写

  * [x] SubTask 1.3: 匹配失败返回空字符串，并为边界情况编写单测

* [ ] Task 2: 在 tktube 任务构建流程中填充 content\_group

  * [x] SubTask 2.1: 在 tktube 任务对象或其元数据结构中新增字段 content\_group（string）

  * [x] SubTask 2.2: 于创建/装配下载对象时调用解析器写入字段

  * [x] SubTask 2.3: 确保序列化/持久化/接口返回包含该字段（只读，不影响现有调用）

* [x] Task 3: 单元测试与样例覆盖

  * [x] SubTask 3.1: 覆盖以下标题样例：CLUB-100、CLUB-100C、【高画质】CLUB-100、【高画质】CLUB-100C

  * [x] SubTask 3.2: 覆盖其他系列如 SSIS-123、ABP-456C，验证大写归一与 C 版去除

  * [x] SubTask 3.3: 覆盖不匹配标题，返回空字符串

* [x] Task 4: 文档与注释

  * [x] SubTask 4.1: 在解析函数与字段处写明规则与示例

  * [x] SubTask 4.2: 更新 README/开发说明（若存在）描述内容分组使用方式与限制

% Task Dependencies

* \[Task 2] depends on \[Task 1]

* \[Task 3] depends on \[Task 1], \[Task 2]

* \[Task 4] depends on \[Task 1], \[Task 2]

