% 内容分组安全修复与隔离策略 Spec

## Why
现有内容分组与组内策略已具备基础能力，但审查发现仍存在误聚合、误取消、运行中取消不生效、跨任务误操作等风险，可能影响线上数据完整性与对象可见性。需要补充严格的分组、隔离与幂等约束，确保策略只在足够安全时生效。

## What Changes
- 新增：未解析到合法组名时，使用 `unknown+title` 作为兜底分组键，避免未知对象落入同一组
- 新增：每次启动都重新计算 content_group；若与存量值不一致则强制更新保存
- 新增：组策略严格按 `task_id + task_type + content_group` 隔离，禁止跨任务、跨类型联动
- 新增：同组内每个优先级只能存在一个对象；若某优先级存在多个对象，则该组整体忽略自动取消策略
- 新增：自动取消仅作用于未开始下载对象；运行中的对象不得仅通过状态覆盖方式“假取消”
- 新增：分组聚合与组展开接口遵循上述隔离与兜底规则
- 新增：单元测试覆盖安全边界与冲突场景

## Impact
- Affected specs: 内容分组、自动取消、聚合展示、安全策略
- Affected code: `pkg/titlegroup`、`manager/task_loader.go`、`manager/manager.go`、`api/server.go`、`web/static/index.html`、相关测试

## ADDED Requirements
### Requirement: 严谨分组键
系统 SHALL 为每个对象生成稳定且非空的内容分组键。

#### 规则
- 若按任务类型规则成功解析出合法组名，则使用该组名
- 若未解析出合法组名，则使用 `unknown+title` 作为分组键
- `title` 使用对象当前标题原文，经空白裁剪后参与兜底；空标题时使用 `unknown+url`
- 组键生成结果必须非空

#### Scenario: 无法解析组名
- WHEN tktube 对象标题无法匹配合法编号
- THEN content_group = `unknown+<trimmed-title>`，而不是空字符串或通用 unknown

### Requirement: 启动时全量重算并纠偏
系统 SHALL 在每次启动时对全部对象重新计算 content_group；当新计算值与存量值不一致时，必须更新并持久化保存。

#### Scenario: 存量值错误
- WHEN 对象原有 content_group 与当前规则计算结果不一致
- THEN 启动时自动改写为新值并持久化

### Requirement: 组策略严格隔离
系统 SHALL 仅在相同 `task_id`、相同 `task_type`、相同 `content_group` 的对象之间应用组内策略。

#### Scenario: 不同任务同名分组
- WHEN 两个任务下均存在 `CLUB-100`
- THEN 任何自动取消、分组展开、代表选择都只能在各自任务内生效

### Requirement: 优先级唯一性保护
系统 SHALL 要求同一组内每个优先级层级最多只有一个对象；若存在重复优先级对象，则该组视为冲突组并忽略自动取消策略。

#### 优先级层级
- HQ+C
- HQ
- Plain+C
- Plain

#### Scenario: 同优先级重复
- WHEN 同一组内出现两个 `HQ` 对象或两个 `Plain+C` 对象
- THEN 系统不得自动取消该组任何对象，并记录冲突日志

### Requirement: 安全自动取消
系统 SHALL 仅在以下条件全部满足时才自动取消低优先级对象：
- 组内存在唯一的最高优先级已完成对象
- 组内不存在同优先级冲突
- 被取消对象状态为 `pending` 或其他“未开始”状态

#### 限制
- 对于 `downloading` 状态对象，系统 SHALL NOT 仅通过状态改写视为取消成功
- 若未来支持真正运行中取消，必须通过可中断下载上下文或取消句柄实现

#### Scenario: 运行中对象
- WHEN 低优先级对象已处于 downloading
- THEN 当前版本不得自动将其改为 cancelled

### Requirement: 安全聚合与展开
系统 SHALL 在分组聚合与组展开时遵循相同隔离维度与分组键规则。

#### Scenario: 未知分组对象
- WHEN 多个对象均无法解析合法组名
- THEN 它们按各自 `unknown+title` 或 `unknown+url` 独立分组，不得被聚合为同一组

## MODIFIED Requirements
### Requirement: 内容分组回填
原“缺失时回填”规则修改为“每次启动全量重算并纠偏保存”，以保证规则升级后存量数据持续收敛。

### Requirement: 自动取消低优先级
原“同组存在已完成最高优先级对象时自动取消低优先级对象”修改为“仅对严格隔离后的安全组生效，且不得影响 downloading 或冲突组对象”。

### Requirement: 对象列表接口
`group_by=content` 的聚合逻辑修改为：
- 分组维度为 `task_id + task_type + content_group`
- 未知对象使用 `unknown+title` / `unknown+url`
- 冲突组仍可聚合展示，但不得附带可误导的自动取消默认语义

## REMOVED Requirements
无

