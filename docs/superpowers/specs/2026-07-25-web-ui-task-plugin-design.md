# Web UI 任务类型插件化设计

## 背景

当前 Web UI 中，任务类型的 UI 表现通过 `index.html` 中的大量 `v-if` 条件分支实现：

| 条件 | 位置 | 用途 |
|------|------|------|
| `type.startsWith('tktube')` | 任务元数据 | 显示关键字/子类型/并发数 |
| `type === 'url_list'` | 任务元数据 | 显示 URL 数量 |
| `type === 'url_list'` | 新建表单 | 显示 URL 文本域 |
| `type === 'tktube'` | 新建表单 | 显示关键字/子类型/并发数 |
| `isVikacg(obj)` | 对象卡片 | 文字摘录 |
| `isCustomUI(obj)` | 对象卡片 | 自定义按钮 |
| `showVikacgModal` | 模态框 | 图片浏览器 |
| `showHanimeModal` | 模态框 | 视频播放器 |

**问题**：每新增一个 task 类型，就要在 `index.html` 的多个位置添加 `v-if` 分支，核心 UI 与 task UI 高度耦合。

## 设计目标

1. **去条件化** — `index.html` 中不再出现 `v-if="type.startsWith(...)"` 等 task 类型特定判断
2. **类型自管理** — 每个 task 类型通过 JS 模块注册自己的 UI 组件（新建表单、元数据、卡片、查看器）
3. **分层模板** — 简单 task 使用声明式配置快速生成 UI，复杂 task 完全自定义
4. **向后兼容** — 现有 task UI 采集器（`__dm_uiBridge`）逐步迁移，不破坏现有功能
5. **后端桥接复用** — 仍使用现有 `core.RegisterTaskUI()` + `api/ui.go` 资产服务机制

## 架构设计

### 组件注册接口

每个 task 类型注册一个 `TaskUIHandler` 对象，包含以下可选方法：

```javascript
// 一个 task 类型完整的 UI 注册对象
{
  // === 基本信息（必填） ===
  type: 'my_task',           // 匹配后端 task.Type()
  label: '我的任务',          // 类型显示名称
  icon: 'fa-star',           // 类型图标，用于筛选下拉框等

  // === 1. 新建任务表单（可选） ===
  // 不提供则此类型不能在 UI 中新建
  renderForm: function(h, formData, formErrors) { return VNode },

  // === 2. 任务详情元数据（可选） ===
  // 不提供则显示 JSON.stringify(task.extra)
  renderMeta: function(h, task) { return VNode },

  // === 3. 对象卡片额外内容（可选） ===
  // 不提供则无额外内容
  renderCardExtra: function(h, obj) { return VNode },

  // === 4. 查看器模态框（可选） ===
  // 不提供则无模态框按钮
  renderViewer: function(h, obj, onClose) { return VNode },
  shouldShowViewer: function(obj) { return obj.status === 'completed' },
  viewerLabel: '查看',        // 按钮文字

  // === 5. 聚合视图（可选） ===
  // 不提供则使用默认聚合视图
  renderAggregate: function(h, objects, params) { return VNode },
}
```

### 声明式配置工厂

对简单 task 类型，提供 `defineForm` / `defineMeta` / `defineCard` 工厂函数：

```javascript
// 简单 task 通过声明式配置生成 UI，无需写渲染函数
TaskUI.defineForm({
  fields: [
    { type: 'textarea', key: 'urls', label: 'URL 列表（每行一个）', rows: 10, required: true },
    { type: 'text', key: 'save_dir', label: '保存目录' },
  ]
})

TaskUI.defineMeta({
  fields: [
    { type: 'count', key: 'urls', label: 'URL 数量', path: 'extra.urls' },
    { type: 'text', key: 'save_dir', label: '保存路径', path: 'save_dir' },
  ]
})

TaskUI.defineCard({
  // 卡片额外信息，默认无
})
```

### 分层使用模式

```
                          TaskUI 注册系统
                                │
         ┌──────────────────────┼──────────────────────┐
         │                      │                      │
   基础层（默认模板）      配置层（声明式）         自定义层（渲染函数）
         │                      │                      │
    ┌────┴────┐           ┌─────┴─────┐          ┌─────┴─────┐
    │ BaseForm │           │defineForm │          │renderForm │
    │ BaseMeta │           │defineMeta │          │renderMeta │
    │ BaseCard │           │defineCard │          │renderCard │
    │ BaseView │           │           │          │renderView │
    └─────────┘           └───────────┘          └───────────┘
```

- **基础层**：核心 UI 提供默认表单/元数据/卡片实现，task 不注册时使用默认
- **配置层**：简单 task 传配置对象（字段列表），工厂函数生成渲染函数
- **自定义层**：复杂 task 写渲染函数，完全控制视图

### 注册表（`registry.js`）

```javascript
window.TaskUI = (function() {
  var registry = {}

  return {
    // 注册一个 task 类型的 UI 处理器
    register: function(type, handler) {
      registry[type] = normalizeHandler(handler)
    },

    // 获取指定类型的 UI 处理器
    get: function(type) { return registry[type] || null },

    // 获取所有已注册类型
    list: function() { return Object.keys(registry) },

    // 检查类型是否有特定功能
    hasForm: function(type) {
      var h = registry[type]; return h && typeof h.renderForm === 'function'
    },
    hasViewer: function(type) {
      var h = registry[type]; return h && typeof h.renderViewer === 'function'
    },
    hasMeta: function(type) {
      var h = registry[type]; return h && typeof h.renderMeta === 'function'
    },
    hasAggregate: function(type) {
      var h = registry[type]; return h && typeof h.renderAggregate === 'function'
    },
  }
})()
```

### 核心 UI 集成方式

核心 UI 通过 `TaskUI.get(type)` 获取处理器，使用 Vue 的 `h()` 函数渲染：

```html
<!-- 新建任务表单（替换 v-if="type === 'url_list'" 等） -->
<div v-if="showAddTaskModal">
  <component :is="addTaskFormComponent" />
</div>
```

```javascript
// 计算属性：根据选中的 task 类型获取表单组件
addTaskFormComponent: function() {
  var handler = TaskUI.get(this.newTask.type)
  if (handler && handler.renderForm) {
    var self = this
    return {
      render: function(h) {
        return handler.renderForm(h, self.newTask, self.formErrors)
      }
    }
  }
  return null
}
```

### 文件结构变化

```
web/static/
├── index.html              # 核心 UI — 移除所有 v-if 分支
├── ui.json
├── app/
│   ├── main.js             # 初始化 + 核心 Vue 应用
│   ├── taskList.js         # 任务列表（通用，不变）
│   ├── api.js              # API 封装（不变）
│   ├── helpers.js          # 通用辅助函数（简化）
│   ├── dashboard.js        # 仪表盘（不变）
│   ├── videoPlayer.js      # 保留（供外部引用）
│   └── taskui/             # ★ 新增：task UI 框架
│       ├── registry.js     #   注册表：register / get / list
│       ├── baseForm.js     #   基础表单模板
│       ├── baseMeta.js     #   基础元数据模板
│       ├── baseCard.js     #   基础卡片额外内容模板
│       ├── baseViewer.js   #   基础查看器模板（模态框骨架）
│       ├── defineForm.js   #   声明式表单工厂
│       ├── defineMeta.js   #   声明式元数据工厂
│       └── loader.js       #   动态加载 task UI 组件
├── utils/
│   └── taskTypes.js        # 简化（仅保留类型发现）
└── tasks/                  # ★ 新增：各 task 类型 UI
    ├── urllist/
    │   └── ui.js
    ├── tktube/
    │   └── ui.js
    ├── hanime/
    │   └── ui.js
    └── vikacg/
        └── ui.js
```

### 数据流

```
用户选择 task 类型
  → loader.js 动态加载 tasks/{type}/ui.js（通过现有 /api/ui/{type}/assets/）
  → ui.js 调用 TaskUI.register(type, handler) 注册组件
  → 核心 UI 通过 TaskUI.get(type) 获取处理器
  → 使用 Vue h() 函数渲染对应组件

            ┌──────────────┐
            │  index.html  │
            │  核心 Vue 应用 │
            └──────┬───────┘
                   │ TaskUI.get(type)
         ┌─────────▼──────────┐
         │  taskui/registry.js │
         │    注册表           │
         └─────────┬──────────┘
                   │ register(type, handler)
         ┌─────────▼──────────┐
         │  tasks/{type}/ui.js │
         │  类型特定 UI 组件    │
         └────────────────────┘
                   │
                   │ 通过 /api/ui/{type}/assets/ 加载
                   │（复用现有后端机制）
         ┌─────────▼──────────┐
         │  core.RegisterTaskUI│
         │  + api/ui.go       │
         │  后端资产服务       │
         └────────────────────┘
```

## 迁移计划

### Phase 1：框架搭建（`taskui/` 目录）

**文件**：`web/static/app/taskui/registry.js`

实现注册表核心功能：
- `TaskUI.register(type, handler)` — 注册
- `TaskUI.get(type)` — 获取
- `TaskUI.list()` — 列出所有
- `TaskUI.hasForm(type)` / `hasViewer(type)` / `hasMeta(type)`

**文件**：`web/static/app/taskui/baseForm.js`

基础表单模板：
- 通用字段：任务 ID、保存目录、存储类型
- 预留 `taskExtraFields` 插槽给 task 类型

**文件**：`web/static/app/taskui/baseMeta.js`

基础元数据模板：
- 通用信息：任务 ID、类型、状态、存储配置
- 预留 `taskExtraMeta` 插槽

**文件**：`web/static/app/taskui/defineForm.js`

声明式表单工厂：
- 支持字段类型：`text`、`number`、`select`、`textarea`、`checkbox`
- 字段验证：`required`、`min`、`max`、`pattern`
- 生成 `renderForm` 函数

**文件**：`web/static/app/taskui/defineMeta.js`

声明式元数据工厂：
- 支持字段类型：`text`、`count`、`json`
- 数据路径：`extra.urls`、`save_dir` 等
- 生成 `renderMeta` 函数

**文件**：`web/static/app/taskui/baseViewer.js`

基础查看器模态框骨架：
- 模态框容器（header/body/footer）
- 关闭按钮、ESC 关闭、背景点击关闭
- 预留 `viewerContent` 插槽

### Phase 2：task 类型迁移

**urllist**（最简单，使用声明式配置）：

```javascript
// web/static/tasks/urllist/ui.js
TaskUI.register('url_list', {
  type: 'url_list',
  label: 'URL 列表',
  icon: 'fa-link',
  renderForm: TaskUI.defineForm({
    fields: [
      { type: 'textarea', key: 'urls', label: 'URL 列表（每行一个）', rows: 10, required: true }
    ]
  }),
  renderMeta: TaskUI.defineMeta({
    fields: [
      { type: 'count', key: 'urls', label: 'URL 数量', path: 'extra.urls' }
    ]
  })
})
```

**tktube**（中等复杂，自定义表单 + 元数据 + 查看器 + 聚合视图）：

```javascript
// web/static/tasks/tktube/ui.js
TaskUI.register('tktube', {
  type: 'tktube',
  label: 'TKTube',
  icon: 'fa-video',
  renderForm: function(h, formData) {
    // 自定义表单渲染：关键字、子类型、并发数、刷新间隔
    return h('div', { class: 'space-y-4' }, [
      h('div', [h('label', '关键字'), h('input', { ... })]),
      h('div', [h('label', '子类型'), h('select', { ... }, [
        h('option', { value: 'tag' }, '标签'),
        h('option', { value: 'model' }, '模特'),
        h('option', { value: 'search' }, '搜索'),
      ])]),
      // ...
    ])
  },
  renderMeta: function(h, task) {
    return h('div', { class: 'grid grid-cols-3 gap-2' }, [
      h('div', [h('span', { class: 'text-gray-400' }, '关键字：'), h('span', task.extra.keyword || '-')]),
      h('div', [h('span', { class: 'text-gray-400' }, '子类型：'), h('span', task.extra.subtype || '-')]),
      h('div', [h('span', { class: 'text-gray-400' }, '并发：'), h('span', task.extra.max_concurrent || '-')]),
    ])
  },
  renderViewer: function(h, obj, onClose) {
    // 视频播放器查看器
    // ...
  },
  shouldShowViewer: function(obj) { return obj.status === 'completed' },
  viewerLabel: '播放',
  renderAggregate: function(h, objects, params) {
    // 内容分组聚合视图
    // ...
  }
})
```

**hanime**（复杂，自定义查看器 — 视频播放器 + 播放列表）：

```javascript
// web/static/tasks/hanime/ui.js
TaskUI.register('hanime', {
  type: 'hanime',
  label: 'Hanime',
  icon: 'fa-film',
  viewerLabel: '播放',
  shouldShowViewer: function(obj) { return obj.status === 'completed' },
  renderViewer: function(h, obj, onClose) {
    // 使用 h() 构建视频播放器 + 播放列表 + 元数据
    // 复用 web/static/app/videoPlayer.js 中的播放器逻辑
    // ...
  }
})
```

**vikacg**（复杂，自定义查看器 — 图片浏览器）：

```javascript
// web/static/tasks/vikacg/ui.js
TaskUI.register('vikacg', {
  type: 'vikacg',
  label: 'VikACG',
  icon: 'fa-image',
  viewerLabel: '浏览',
  shouldShowViewer: function(obj) {
    return obj.status === 'completed' && obj.extra && obj.extra.images
  },
  renderViewer: function(h, obj, onClose) {
    // 图片浏览器：主图、导航、缩略图网格、标签、链接
    // ...
  }
})
```

### Phase 3：核心 UI 清理

1. **`index.html` 移除**：
   - 新建任务表单中的 `v-if="type === 'url_list'"` 和 `v-if="type === 'tktube'"`
   - 任务元数据中的 `v-if="type.startsWith('tktube')"`、`v-if="type === 'url_list'"`、`v-if="type !== 'url_list' && !type.startsWith('tktube')"`
   - 对象卡片中的 `v-if="isVikacg(obj)"`、`v-if="isCustomUI(obj)"`
   - 整个 vikacg/hanime 模态框（`showVikacgModal`、`showHanimeModal`、`showCustomUIModal`）

2. **替换为动态组件**：
   - 新建表单：`<component :is="addTaskFormComponent" />`
   - 任务元数据：`<component :is="taskMetaComponent" />`
   - 卡片额外内容：`<component :is="cardExtraComponent" />`
   - 查看器：`<component :is="viewerComponent" />`

3. **`main.js` 简化**：
   - 移除 `loadCustomUIFeatures`、`loadTaskUI`、`renderCustomTaskView` 等方法
   - 替换为统一的 `loadTaskUIComponent(type)` 方法

4. **`helpers.js` 简化**：
   - `__dm_uiBridge` 保留兼容，逐步迁移到 `TaskUI` 注册表

### 后端变化

**`core/taskui.go` 扩展**：

```go
type TaskUIAssets struct {
    FS                    embed.FS
    JSPaths               []string // JS 文件路径
    CSSPaths              []string // CSS 文件路径
    Label                 string   // 按钮标签
    HasForm               bool     // 是否支持 UI 新建
    HasViewer             bool     // 是否有查看器
    HasAggregate          bool     // 是否有聚合视图
    DefaultFormFields     []string // 可选：默认表单字段列表
}
```

**`api/ui.go` 扩展**：

```go
// 增强的 UI 配置响应
func (s *Server) serveUIConfig(w http.ResponseWriter, r *http.Request) {
    taskType := mux.Vars(r)["type"]
    assets, ok := core.GetTaskUI(taskType)
    if !ok {
        writeJSONError(w, http.StatusNotFound, "not_found", "no UI assets for type: "+taskType)
        return
    }
    w.Header().Set(hdrContentType, "application/json")
    _ = json.NewEncoder(w).Encode(map[string]any{
        "js":           assets.JSPaths,
        "css":          assets.CSSPaths,
        "label":        assets.Label,
        "has_form":     assets.HasForm,
        "has_viewer":   assets.HasViewer,
        "has_aggregate": assets.HasAggregate,
    })
}
```

**`task/{type}/ui/ui.go` 更新**：

```go
// urllist 示例—标记 HasForm 支持 UI 新建
func init() {
    core.RegisterTaskUI("url_list", core.TaskUIAssets{
        FS:      assets,
        JSPaths: []string{"assets/viewer.js"},
        Label:   "URL 列表",
        HasForm: true,
    })
}
```

## 错误处理

### 组件加载失败
- 如果 task 类型的 JS 加载失败，核心 UI 应优雅降级
- 使用 `onerror` 回调捕获加载错误，显示默认视图

### 缺失处理器
- 如果 `TaskUI.get(type)` 返回 null，使用默认实现（BaseForm / BaseMeta / BaseCard）
- 确保核心 UI 在所有情况下都有合理的默认行为

### 向后兼容
- `__dm_uiBridge` 在过渡期内保留，新 task 优先使用 `TaskUI` 注册表
- 现有的 `viewer.js` 文件（hanime/vikacg/tktube）逐步迁移，迁移完成前仍可工作

## 验证方式

1. **功能等价**：现有 4 种 task 类型 UI 功能不变
   - 新建 urllist/tktube 任务
   - 查看任务元数据
   - 对象卡片渲染
   - VikACG 图片浏览器、hanime 视频播放器
2. **条件分支消除**：`index.html` 中不再有 `v-if="type.startsWith(...)"` 等条件
3. **新增 task 类型**：只需在 `web/static/tasks/` 下新建目录和 `ui.js`，无需修改核心 UI
4. **Playwright E2E 测试**：通过现有测试套件验证
5. **降级测试**：加载失败时显示默认视图，不崩溃