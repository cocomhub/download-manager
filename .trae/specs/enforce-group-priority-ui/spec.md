% 内容分组回填 + 优先级 + 自动取消 + 分组展示开关 Spec

## Why
同一内容常以多版本存在（如【高画质】、后缀 C）。已具备分组名抽取能力，但存量对象可能缺失分组字段，且缺乏同组版本的优先级与自动取消机制以及按组聚合的 UI。需要完善数据一致性与用户体验。

## What Changes
- 新增：启动时对存量对象回填 content_group（按任务类型调用对应解析器；首期覆盖 tktube）
- 新增：分组内优先级模型（tktube）：【高画质】 > 无前缀；同前缀下 C 后缀 > 无 C
- 新增：当组内存在“已完成”的高优先级对象时，自动取消低优先级对象的下载，并记录跳转链接
- 新增：播放/打开低优先级对象时自动跳转至高优先级对象
- 新增：UI 增加“按内容分组展示”开关；聚合视图仅展示每组最高优先级对象，可展开查看同组全部对象；支持一键删除低优先级并置为取消状态
- 接口扩展：对象列表/聚合接口支持按 content_group 分组返回与展开（不破坏旧接口，新增 query 参数）
- 数据兼容：新增信息落在对象的 Metadata/Extra 字段中，保持后向兼容

## Impact
- Affected specs: 任务/对象一致性、聚合/展示、下载策略
- Affected code: manager（启动流程/聚合）、storage（批量更新）、task/tktube_task.go（可复用已有组名生成）、api/server.go（列表与展开接口）、web/static（分组开关与展开 UI）

## ADDED Requirements
### Requirement: 回填内容分组
系统 SHALL 在服务启动时扫描存量 DownloadObject，若缺失 Metadata.content_group，则基于任务类型的规则计算并落库（首期 tktube 使用 TKTGroupNameFromTitle）。

#### Scenario: 存量缺失字段
- WHEN 服务启动
- THEN 所有对象均具备 content_group（若无法解析则为空字符串）

### Requirement: 分组优先级（tktube）
系统 SHALL 在同一 content_group 内计算对象优先级，规则：
- has_hq = 标题是否含开头“【…】”标签，含则 1，否则 0
- has_c = 标题是否以可选后缀 C（在编号之后）表示，含则 1，否则 0
- 排序：先 has_hq，后 has_c，降序；平级保持现有顺序
- 系统 MAY 将计算结果存入 Extra.priority（数值或结构化标记）

#### Scenario: 比较
- WHEN “【高画质】CLUB-100” vs “CLUB-100C”
- THEN 前者优先级更高
- WHEN “【高画质】CLUB-100C” vs “【高画质】CLUB-100”
- THEN 前者优先级更高

### Requirement: 自动取消低优先级
系统 SHALL 在同组存在已完成的最高优先级对象时，将该组内其他未完成对象自动置为“canceled”，并记录 Extra.redirect_url 指向最高优先级对象的 URL。

#### Scenario: 完成高优先级
- WHEN 组内某对象状态变更为 completed
- THEN 同组更低优先级对象状态自动更新为 canceled，且保存 redirect_url

### Requirement: 播放跳转
系统 SHALL 在 UI 打开（或播放）被取消的低优先级对象时，自动跳转至 redirect_url 所指向的高优先级对象。

#### Scenario: 点击被取消对象
- WHEN 用户点击已取消的低优先级对象
- THEN UI 自动导航到高优先级对象对应的详情/播放

### Requirement: 分组展示开关与批量操作
系统 SHALL 在 UI 提供“按内容分组展示”开关；开启时：
- 列表仅展示每组最高优先级对象，并显示组内对象数量
- 点击代表项可展开弹出同组全部对象
- 系统 SHALL 提供“删除低优先级并取消下载”的按钮，批量将低优先级对象置为 canceled 并可选删除其本地文件（可配置）

#### Scenario: 开关与展开
- WHEN 开关开启
- THEN 列表按组聚合展示代表对象；用户可展开查看并进行批量取消/删除

## MODIFIED Requirements
### Requirement: 对象列表接口
接口新增可选参数 group_by=content，返回聚合后的代表对象列表，并附 group_size 与代表对象标识；提供展开接口 /api/groups/{group}/objects 获取该组全部对象。

## REMOVED Requirements
无

