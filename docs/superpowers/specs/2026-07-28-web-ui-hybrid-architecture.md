# Web UI 架构重构：混合方案设计

## 概述

当前下载管理器 Web UI 使用 Vue 3 CDN + mixin 模式，存在以下问题：
- Mixin 模式导致 ~100 个方法倒入全局 `this` 命名空间，溯源困难、命名冲突、AI 生成代码易踩坑
- `this.$forceUpdate()` 反模式表明 Vue 响应式追踪断开了
- 动态 `Vue.createApp()` 子应用创建重量级开销，且无法正确绑定 `on:{}` 事件处理器
- `Vue.h()` 在 viewer 插件中是虚假契约（3 个 viewer 接收 h 但不使用）
- 两套并行渲染系统（Vue 模板 + 纯 DOM 操作）增加维护负担

本设计采用**混合方案**：保留 Vue 3 CDN **仅用于主模板层**的声明式渲染（`v-if`、`v-for`、`v-model`、`:class` 等），将所有业务逻辑从 mixin 中提取为独立纯函数模块。

## 架构

```
index.html (Vue 3 CDN — 仅用于模板渲染的状态容器)
  │
  ├── app/api.js               ← 纯函数 HTTP 封装（不变）
  │
  ├── app/taskui/               ← 插件注册系统（不变）
  │   ├── registry.js
  │   ├── defineForm.js / defineMeta.js
  │   ├── baseForm.js / baseMeta.js
  │   ├── baseViewer.js          ← 删除（无人使用）
  │   ├── loader.js / logger.js
  │   ├── collection.js / recommendation.js
  │   └── shared/                ← 纯函数模块（已实现）
  │       ├── data.js
  │       ├── dom.js
  │       └── modal.js
  │
  ├── app/ui/ (新建)             ← 从 mixin 提取的纯函数模块
  │   ├── helpers.js             ← 从 app/helpers.js mixin 提取
  │   ├── taskList.js            ← 从 app/taskList.js mixin 提取
  │   ├── videoPlayer.js         ← 从 app/videoPlayer.js mixin 提取
  │   └── dashboard.js           ← 从 app/dashboard.js mixin 提取
  │
  └── app/main.js               ← Vue 应用（大幅简化，仅状态容器）
```

## 设计原则

1. **Vue 只做模板渲染**：Vue 实例是"状态容器"，承载 `data()`、`computed`、`watch`，不承载业务逻辑方法
2. **业务逻辑是纯函数**：所有 mixin 方法提取为独立函数，接受显式参数，返回显式结果
3. **逐步迁移**：每步可独立验证，不破坏现有功能
4. **零新依赖**：不引入构建工具、不引入新框架

## 详细设计

### 1. Vue 保留的部分（index.html 模板 + main.js 状态容器）

**Vue 实例保留的 data() 属性**：
- 视图状态：`viewMode`、`selectedType`、`selectedTaskId`、`searchQuery`、`sortBy`、`statusFilter`、`pagination`
- 列表数据：`tasks`、`taskTypes`、`activeDownloads`、`selectedTask`、`selectedObjectUrls`
- 弹窗状态：`showConfigModal`、`showAddTaskModal`、`showGroupModal`、`showTaskTypeDefaultsModal`
- 仪表盘数据：`dashboardHealth`、`dashboardMetrics`、`dashboardFailures`
- 聚合视图：`aggObjects`、`aggSearchQuery`、`aggStatusFilter`、`aggGroupBy`、`aggViewMode`、`aggPagination`、`aggSortBy`
- TaskUI 集成：`customUIContainer`、`customUIError`

**Vue 实例保留的 computed 属性**：
- `filteredTasks` — 按类型过滤任务
- `filteredObjects` — 按状态过滤对象
- `aggFilteredObjects` / `aggPagedObjects` — 聚合视图分页
- `isWriteDisabled` — UI-only 模式判断
- `volumeIcon` — 音量图标
- `showTaskTypeFormFields` / `showTaskTypeMeta` — TaskUI 功能检测

**Vue 实例保留的 watch**：
- `selectedType` — 加载 TaskUI + 更新 URL
- `searchQuery` — 防抖搜索
- `sortBy` — 重置分页
- `viewMode` — 启动/停止仪表盘轮询
- `currentVideo` — 键盘事件注册/移除

**Vue 实例保留的模板绑定**：
- 所有 `v-if`/`v-for`/`v-model`/`:class`/`@click` 保留不变
- `@click` 事件绑定改为调用 `window.UiHelpers.xxx()`

### 2. 从 mixin 提取的纯函数模块

**`app/ui/helpers.js`** — 从 `app/helpers.js` 的 mixin 提取：

```js
window.UiHelpers = {
  getTitle(obj) { ... },
  getTags(obj) { ... },
  getDate(obj) { ... },
  getDuration(obj) { ... },
  getObjId(obj) { ... },
  getTaskTypeForObj(obj) { ... },
  getTaskDisplayName(task) { ... },
  getTaskTypeBadge(task) { ... },
  getFileUrl(obj) { ... },
  getScopedTaskInfo(obj) { ... },
  getObjectVariantPriority(obj) { ... },
  isGroupRepresentative(obj) { ... },
  isGroupCancelTarget(obj) { ... },
  getObjectVariantLabel(obj) { ... },
  metadataContentGroup(obj) { ... },
  isTouchDevice() { ... },
  pathToUrl(path) { ... },
  copyText(text) { ... },
  showToast(message, type) { ... },

  // 操作函数（需要访问 Vue 实例的通过参数传入）
  initSSE(vueInstance) { ... },
  handleEvent(event, vueInstance) { ... },
  openAddTask(vueInstance) { ... },
  saveNewTask(formData, vueInstance) { ... },
  openConfig(vueInstance) { ... },
  saveConfig(config, vueInstance) { ... },
  handleCardClick(obj, vueInstance) { ... },
  openGroupModal(obj, vueInstance) { ... },
  openAggregateView(vueInstance) { ... },
  fetchAggregateByType(type, vueInstance) { ... },
  cancelAggObject(obj, vueInstance) { ... },
  changeAggPage(page, vueInstance) { ... },
}
```

**`app/ui/taskList.js`** — 从 `app/taskList.js` 的 mixin 提取：

```js
window.UiTaskList = {
  fetchTasks(vueInstance) { ... },
  selectTask(id, vueInstance) { ... },
  fetchTaskDetails(id, background, vueInstance) { ... },
  cancelCurrentTask(vueInstance) { ... },
  cancelSelected(vueInstance) { ... },
  retryAllFailed(vueInstance) { ... },
  retrySelectedObjects(vueInstance) { ... },
  cancelObject(obj, vueInstance) { ... },
  undoCancelObject(obj, vueInstance) { ... },
  toggleTaskConfigPanel(vueInstance) { ... },
  saveTaskConfig(vueInstance) { ... },
  changePage(p, vueInstance) { ... },
  changeLimit(vueInstance) { ... },
}
```

**`app/ui/videoPlayer.js`** — 从 `app/videoPlayer.js` 的 mixin 提取：

```js
window.UiVideoPlayer = {
  // 播放器状态管理
  state: {
    currentVideo: null,
    isPlaying: false,
    isBuffering: false,
    currentTime: 0,
    duration: 0,
    volume: 1,
    isMuted: false,
    playbackRate: 1,
    collectionList: [],
  },

  // 方法
  loadVideoSettings() { ... },
  saveVideoSettings() { ... },
  resetVideoSettings() { ... },
  playVideo(obj, vueInstance) { ... },
  closeVideo(vueInstance) { ... },
  togglePlay(vueInstance) { ... },
  seekClick(e, vueInstance) { ... },
  skip(seconds, vueInstance) { ... },
  setSpeed(rate, vueInstance) { ... },
  toggleMute(vueInstance) { ... },
  updateVolume(vueInstance) { ... },
  toggleFullscreen(vueInstance) { ... },
  playPrev(vueInstance) { ... },
  playNext(vueInstance) { ... },
  switchToCollectionItem(item, vueInstance) { ... },
  isVideo(obj) { ... },
  formatTime(seconds) { ... },
}
```

**`app/ui/dashboard.js`** — 从 `app/dashboard.js` 的 mixin 提取：

```js
window.UiDashboard = {
  fetchDashboardData(vueInstance) { ... },
  fetchHealthz(vueInstance) { ... },
  fetchMetrics(vueInstance) { ... },
  fetchFailures(vueInstance) { ... },
  startDashboardPolling(vueInstance) { ... },
  stopDashboardPolling(vueInstance) { ... },
}
```

### 3. Vue 与纯函数的桥接

在 `main.js` 中，Vue 实例通过 `methods` 调用纯函数模块：

```js
// main.js 的 methods 中
methods: {
  // 不再定义业务逻辑，只做桥接
  handleCardClick: function(obj) {
    return window.UiHelpers.handleCardClick(obj, this)
  },
  fetchTasks: function() {
    return window.UiTaskList.fetchTasks(this)
  },
  // ...
}
```

但更进一步的简化方式是：**在模板中直接调用 `window.UiHelpers.xxx()`**，跳过 Vue methods 层：

```html
<!-- index.html 模板中 -->
<button @click="UiHelpers.handleCardClick(obj, this)">
  查看
</button>
```

但这需要 Vue 实例能访问 `window.UiHelpers`。更务实的做法是保留 `methods` 桥接但自动生成：

```js
// main.js — 自动桥接
var UI_MODULES = [window.UiHelpers, window.UiTaskList, window.UiVideoPlayer, window.UiDashboard]
var appMethods = {}
UI_MODULES.forEach(function(mod) {
  Object.keys(mod).forEach(function(key) {
    if (typeof mod[key] === 'function') {
      appMethods[key] = function() {
        var args = Array.prototype.slice.call(arguments)
        return mod[key].apply(null, args.concat([this]))
      }
    }
  })
})

Vue.createApp({
  data: function() { return { ... } },
  computed: { ... },
  watch: { ... },
  methods: appMethods
}).mount('#app')
```

### 4. 删除 `Vue.h()` 虚假契约

**`normalizeHandler` 简化**：删除 `h` 注入包装，所有 render 函数由 handler 直接管理：

```js
// registry.js — 简化后
function normalizeHandler(handler) {
  return {
    type: handler.type,
    label: handler.label || handler.type,
    icon: handler.icon || 'fa-cube',
    renderForm: handler.renderForm || null,   // 不再包装 h
    renderMeta: handler.renderMeta || null,
    renderViewer: handler.renderViewer || null,
    // ...
  }
}
```

**删除 baseViewer.js**：`TaskUI.BaseViewer` 基于 `Vue.h()` 实现，但没有任何 viewer 使用它。

### 5. 迁移步骤

| 步骤 | 内容 | 工作量 |
|------|------|--------|
| 1 | 创建 `app/ui/` 目录结构 | 小 |
| 2 | 提取 `helpers.js` 到 `app/ui/helpers.js` | 中 |
| 3 | 提取 `taskList.js` 到 `app/ui/taskList.js` | 中 |
| 4 | 提取 `videoPlayer.js` 到 `app/ui/videoPlayer.js` | 中 |
| 5 | 提取 `dashboard.js` 到 `app/ui/dashboard.js` | 中 |
| 6 | 简化 `main.js`：删除 mixin 注册，改为纯函数桥接 | 中 |
| 7 | 删除 `baseViewer.js`，简化 `registry.js` 的 h 注入 | 小 |
| 8 | 删除 `this.$forceUpdate()` 调用 | 小 |
| 9 | 更新 `index.html` 加载顺序 | 小 |
| 10 | 测试验证 | 中 |

## 验证方式

1. `go build ./...` 编译通过
2. 启动 Web UI，逐一测试所有功能：
   - 任务列表（加载、筛选、搜索、分页）
   - 任务操作（创建、取消、重试、配置）
   - 视频播放器（播放、暂停、进度、音量、全屏）
   - 查看器（tktube/hanime/vikacg 的 Modal 打开/关闭）
   - 仪表盘（健康检查、指标、失败记录）
   - 配置面板（加载、保存、差异对比）
3. 浏览器控制台无 JavaScript 错误
4. 确认 `$forceUpdate` 不再被调用