# Web UI 混合架构迁移设计

## 方案概述

下载管理器 Web UI 当前使用 Vue 3 CDN + mixin 模式，存在以下问题：
- Mixin 模式导致 ~100 个方法全部倒入全局 `this` 命名空间，溯源困难，命名冲突
- `this.$forceUpdate()` 反模式表示 Vue 响应式追踪断开了
- `Vue.createApp()` 子应用每次打开 viewer 创建全新 Vue 实例，有内存泄漏风险
- Vue 3 CDN 的 `createApp` 在插件函数返回 VNode 树时无法正确绑定 `on:{}` 事件处理器
- `h()` 参数传递是虚假契约 — 3 个 viewer 接收 `h` 但不使用，仅返回 `h('div')` 占位
- 三套并行渲染系统（Vue 模板、Vue.h()、纯 DOM）不同步

**方案 C（混合方案）**：保留 Vue 3 CDN 仅用于主模板层（`index.html` 的声明式渲染），将所有 JS 逻辑改为纯函数 + 模块化。消除 mixin、`$forceUpdate`、子应用 `createApp`、`h()` 虚假契约。

## 架构

```
index.html (Vue 3 CDN — 仅保留模板渲染：v-if, v-for, v-model, @click, :class)
  │
  ├── app/taskui/shared/        ← 纯函数模块（已实现）
  ├── app/taskui/               ← 保持现有（registry.js, defineForm.js, defineMeta.js, loader.js, collection.js, recommendation.js）
  ├── app/api.js                ← 纯函数（已实现）
  ├── app/ui/                   ← 新建：从 mixin 提取的纯函数
  │   ├── helpers.js            ← 从 AppHelpers mixin 提取
  │   ├── taskList.js           ← 从 AppTaskList mixin 提取
  │   ├── videoPlayer.js        ← 从 AppVideoPlayer mixin 提取
  │   └── dashboard.js          ← 从 AppDashboard mixin 提取
  └── app/main.js               ← Vue 应用（大幅简化，只做状态容器）
```

## 核心变更

### 1. Vue 保留的部分（index.html 模板）
- `v-if="viewMode === 'downloads'"` — 视图切换
- `v-for="task in filteredTasks"` — 任务列表渲染
- `v-model="searchQuery"` — 搜索输入
- `:class="..."` — 动态样式
- `@click="..."` — 事件绑定
- `:is="taskTypeFormComponent"` — 动态组件渲染

### 2. Vue 移除的部分
- 所有 `app.mixin()` 调用 → `window.UiHelpers = { ... }` 纯函数
- `this.$forceUpdate()` → 直接操作 DOM 或通过 `Vue.reactive` 状态对象
- `Vue.createApp()` 子应用 → `TaskUI.Modal.create()` 纯 DOM
- `Vue.h()` 虚假契约 → 删除 `normalizeHandler` 的 h 注入
- `baseViewer.js` → 删除（无人使用）

### 3. Vue 保留的 data 属性
- 视图状态：`viewMode`、`selectedType`、`searchQuery`、`sortBy`、`statusFilter`
- 数据：`tasks`、`selectedTask`、`activeDownloads`、`pagination`
- 弹窗：`showConfigModal`、`showAddTaskModal`、`showGroupModal`
- 筛选：`filteredTasks`、`filteredObjects`（computed）
- 仪表盘：`dashboardHealth`、`dashboardMetrics`、`dashboardFailures`

### 4. 从 mixin 提取的纯函数模块

**`window.UiHelpers`**（从 helpers.js mixin 提取）：
- `getTitle(obj)`, `getDate(obj)`, `getDuration(obj)`, `getTags(obj)`, `getObjId(obj)`
- `getTaskDisplayName(task)`, `getTaskTypeBadge(task)`
- `pathToUrl(path)`, `getFileUrl(obj)`
- `initSSE(url, handlers)`, `showToast(message, type)`
- `openAddTask()`, `saveNewTask()`, `openConfig()`, `saveConfig()`
- `handleCardClick(obj)`, `copyText(text)`
- `openGroupModal(obj)`, `closeGroupModal()`
- `openAggregateView()`, `fetchAggregateByType(type)`

**`window.UiTaskList`**（从 taskList.js mixin 提取）：
- `fetchTasks()`, `selectTask(id)`, `fetchTaskDetails(id)`
- `cancelCurrentTask()`, `cancelSelected()`, `retryAllFailed()`
- `changePage(p)`, `changeLimit()`
- `retrySelectedObjects()`, `cancelObject(obj)`, `undoCancelObject(obj)`
- `toggleTaskConfigPanel()`, `saveTaskConfig()`

**`window.UiVideoPlayer`**（从 videoPlayer.js mixin 提取）：
- `playVideo(obj)`, `closeVideo()`
- `togglePlay()`, `seekClick(e)`, `skip(seconds)`, `setSpeed(rate)`
- `toggleMute()`, `updateVolume()`, `toggleFullscreen()`
- `playPrev()`, `playNext()`, `switchToCollectionItem(item)`
- `handleKeydown(e)`, `formatTime(seconds)`
- `isVideo(obj)`, `getVideoUrl(obj)`, `getThumbImage(obj)`, `getCoverImage(obj)`

**`window.UiDashboard`**（从 dashboard.js mixin 提取）：
- `fetchDashboardData()`, `fetchHealthz()`, `fetchMetrics()`, `fetchFailures()`
- `startDashboardPolling()`, `stopDashboardPolling()`

### 5. main.js 简化

Vue 应用不再承载业务逻辑，只做模板渲染的"状态容器"。

```js
// 简化后的 main.js
var app = Vue.createApp({
  data: function() {
    return {
      viewMode: 'grid',
      selectedType: 'all',
      tasks: [],
      selectedTask: null,
      searchQuery: '',
      // ... 状态属性
    }
  },
  computed: {
    filteredTasks: function() { ... },
    filteredObjects: function() { ... },
  },
  watch: {
    selectedType: function(type) { ... },
    searchQuery: function(q) { ... },
  },
  mounted: function() { ... },
  beforeUnmount: function() { ... },
  methods: {
    // 仅模板直接调用的方法，委托给 UiHelpers
    handleCardClick: function(obj) { UiHelpers.handleCardClick(obj) },
    openAddTask: function(e) { UiHelpers.openAddTask(e) },
    // ...
  }
})
app.mount('#app')
```

## 实施步骤

### 第一阶段：准备工作（不影响现有功能）

1. 创建 `web/static/app/ui/` 目录
2. 创建 `app/ui/helpers.js` — 从 helpers.js mixin 提取 pure functions
3. 创建 `app/ui/taskList.js` — 从 taskList.js mixin 提取 pure functions
4. 创建 `app/ui/videoPlayer.js` — 从 videoPlayer.js mixin 提取 pure functions
5. 创建 `app/ui/dashboard.js` — 从 dashboard.js mixin 提取 pure functions

### 第二阶段：逐步替换

6. 修改 `index.html` — 加载新模块，保持旧模块仍加载（双轨运行）
7. 修改 `main.js` — 逐步替换 mixin 注册为 UiHelpers 调用
8. 修改 `index.html` 模板 — 将 `@click="this.xxx()"` 替换为 `@click="UiHelpers.xxx()"`

### 第三阶段：清理

9. 删除 `app/mixin/*.js` 中的 mixin 注册（确认无误后）
10. 删除 `normalizeHandler` 的 h 注入
11. 删除 `baseViewer.js`
12. 删除 `this.$forceUpdate()` 调用
13. 删除 `Vue.createApp()` 子应用（viewer 已全部使用纯 DOM）

## 验证方式

1. `go build ./...` — 编译通过
2. 启动 Web UI，逐一测试所有功能：任务列表、搜索、筛选、分页
3. 测试 4 个任务类型的 viewer 功能正常
4. 测试 SSE 实时更新正常
5. 测试仪表盘轮询正常
6. 测试视频播放器正常
7. 测试配置保存正常