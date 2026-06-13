# Mobile Responsive UI 设计

> 2026-06-14

## 背景

当前 download-manager 的 Web UI 是桌面端优先的侧边栏+主内容布局（固定 256px 侧边栏 + 顶栏 + 网格/表格），在小屏设备上无法正常使用。需要在保持功能完整的前提下适配移动设备。

## 约束

1. 纯前端改动 — 不涉及 Go 后端
2. 无 build 步骤 — Vue 3 + Tailwind 从 CDN 加载
3. 保持单文件 HTML + 6 个 JS 模块的现有架构
4. `data-testid` 需要保留或适配
5. 增量落地，先适配再看测试结果

## 方案选型

**方案 A：纯 CSS 响应式 + 最小 JS 状态（推荐）**

- 利用 Tailwind 响应式前缀 (`sm:`、`md:`、`lg:`) 实现布局降级
- 仅新增 2 个 Vue data 状态：`mobileSidebarOpen`、`mobileToolbarOpen`
- 表格精简列使用 `hidden md:table-cell`
- 不新增独立移动端模板，不引入双份维护

## 设计细节

### 一、侧边栏 → 抽屉式导航

**当前问题：** 固定 `w-64` 256px 侧边栏在小屏占据 >60% 宽度。

**方案：**

- 桌面端（`>=lg`）：保持现有固定布局
- 移动端（`<lg`）：侧边栏改为 `fixed` 定位，从左侧滑入/滑出
- 新增 `mobileSidebarOpen: false` 状态
- 顶栏左侧新增 hamburger 按钮（`<lg` 可见）
- 点击任务后自动关闭侧边栏

```html
<aside class="
  w-64 bg-white shadow-md flex flex-col z-10
  fixed lg:relative inset-y-0 left-0 z-50
  transition-transform duration-300 ease-in-out
  lg:translate-x-0
  data-[open=false]:-translate-x-full
  lg:shadow-none shadow-2xl
">
```

### 二、顶栏操作按钮 → 响应式折叠

**当前问题：** 选中任务后顶栏展示约 10 个操作元素（重试/取消/排序/分页/搜索/视图切换/模式切换），一行放不下。

**方案：**

- 顶栏分两行：第一行 hamburger + 任务名 + `...` 展开按钮
- 第二行工具栏（桌面始终可见，移动端默认折叠）
- 新增 `mobileToolbarOpen: false` 状态
- 搜索框 `w-64` 改为 `w-full lg:w-64`

### 三、对象网格 → 自适应列数

**当前：** `grid-cols-1 sm:2 md:3 lg:4 xl:5 gap-6`

**方案：**

- 断点不变，gap 响应式：`gap-3 sm:4 md:6`
- 标签区块小屏隐藏：`hidden sm:flex`
- 操作按钮小屏缩小间距并支持换行：`gap-1 sm:gap-2 flex-wrap`

### 四、表格 → 移动端精简列

**当前：** 4 个表格（对象列表/聚合视图/指标/失败记录）全列展示，移动端水平溢出。

**方案：**

全部使用 CSS-only 精简列（不新增卡片模板）：

| 表格 | 桌面列 | 移动端列 |
|---|---|---|
| 对象列表 (801行) | Name + Info + Tags + Status + Progress + Actions | Name + Status + Actions |
| 聚合视图列表 (410行) | Name + Info + Tags + Status + Progress + Task + Actions | Name + Status + Task |
| 指标 (552行) | 任务 + 完成 + 失败 + 重试 + 延迟 + 队列 + 活跃 + 并发 | 任务 + 完成 + 失败 |
| 失败记录 (599行) | 时间 + 任务 + URL + 错误 + 尝试 + 永久 | 时间 + 任务 + 错误 |

实现：在 `<th>` 和 `<td>` 上添加 `hidden md:table-cell`。

### 五、模态框 → 移动端全屏化

涉及 4 个模态框：视频播放器/配置面板/新增任务/编辑配置。

**方案：**

- 桌面：居中弹窗（`max-w-lg`，`rounded-lg`，`mx-4`）
- 移动端：全屏铺满（`w-full`，`rounded-none`，`min-h-screen`）
- 视频播放器：`p-0 sm:p-4`

### 六、仪表盘 → 响应式

- 组件状态栏：`flex` → `flex flex-wrap` + `min-w-[80px]`
- 指标卡网格：`grid-cols-2 md:4`（已有，不变）

### 七、触摸交互增强

- 拖拽排序（714行）：触屏设备禁用，`[:draggable]` 条件中添加 `!isTouchDevice`
- hover 预览（718行）：移动端改为 `click` 切换
- 视频控制栏按钮：尺寸增大 `p-3` 替代 `p-2`

## 修改清单

| 区域 | 修改类型 | 涉及行数 |
|---|---|---|
| 侧边栏抽屉 | CSS class + 2 处 JS state | ~25 行 |
| 顶栏折叠 | CSS class + 1 处 JS state | ~30 行 |
| 对象网格间距 | CSS class 修改 | ~5 行 |
| 表格精简列 | `hidden md:table-cell` | ~40 行 |
| 模态框全屏 | CSS class 修改 | ~10 行 |
| 仪表盘 | CSS class 修改 | ~5 行 |
| 触摸适配 | CSS + JS 条件 | ~10 行 |
| **总计** | **纯 CSS + 3 个 JS state** | **~125 行** |

## 不修改的部分

- Go 后端：零改动
- 数据流/API：零改动
- data-testid：保留，在移动端定位器中适配
- 视频播放器逻辑：仅调整样式

## 规格自检

- [x] 占位符扫描：无
- [x] 内部一致性：方案间布局逻辑一致，无矛盾
- [x] 范围检查：聚焦前端 UI 适配，不涉及后端/API
- [x] 模糊性检查：表格精简列采用纯 CSS 方案而非双模板，已明确选定
