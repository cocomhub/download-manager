# Web UI 任务类型插件化 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 将 `index.html` 中所有 task 类型特定的 `v-if` 条件分支（~15 处）抽象为 `TaskUI` 组件注册系统，每个 task 类型通过 `web/static/tasks/{type}/ui.js` 注册自己的 UI 组件。

**架构：** 前端新增 `web/static/app/taskui/` 框架（注册表 + 声明式工厂 + 基础模板），新增 `web/static/tasks/{type}/ui.js` 各 task 类型 UI 组件。后端扩展 `core/taskui.go`（`TaskUIAssets` 增加 `HasForm`/`HasViewer` 字段）和 `api/ui.go`（增强 UI 配置响应）。核心 UI 通过 `TaskUI.get(type)` 动态加载组件，Vue `h()` 函数渲染。

**技术栈：** Vue 3 (CDN) + Tailwind CSS (CDN) + vanilla JS IIFE 模块模式

---

## 文件结构

### 新建文件
| 文件 | 职责 |
|------|------|
| `web/static/app/taskui/registry.js` | TaskUI 注册表（register/get/list/hasForm/hasViewer） |
| `web/static/app/taskui/defineForm.js` | 声明式表单工厂 |
| `web/static/app/taskui/defineMeta.js` | 声明式元数据工厂 |
| `web/static/app/taskui/baseForm.js` | 基础表单模板（通用字段 + 插槽） |
| `web/static/app/taskui/baseMeta.js` | 基础元数据模板（通用信息 + 插槽） |
| `web/static/app/taskui/baseViewer.js` | 基础查看器模态框骨架 |
| `web/static/app/taskui/loader.js` | 动态加载 task UI 组件 |
| `web/static/tasks/urllist/ui.js` | url_list 类型 UI 组件 |
| `web/static/tasks/tktube/ui.js` | tktube 类型 UI 组件 |
| `web/static/tasks/hanime/ui.js` | hanime 类型 UI 组件 |
| `web/static/tasks/vikacg/ui.js` | vikacg 类型 UI 组件 |

### 修改文件
| 文件 | 职责 |
|------|------|
| `web/static/app/main.js` | 新增 TaskUI 集成方法，替换 `loadCustomUIFeatures` 等 |
| `web/static/app/helpers.js` | 移除 `isVikacg`/`getVikacgExcerpt`/`isCustomUI`/`getCustomUILabel`/`renderPluginCards`，`handleCardClick` 改为使用 TaskUI |
| `web/static/index.html` | 替换所有 `v-if="type.startsWith(...)"` 等条件分支为动态组件 |
| `core/taskui.go` | `TaskUIAssets` 增加 `HasForm`/`HasViewer`/`HasAggregate` 字段 |
| `api/ui.go` | `serveUIConfig` 返回增强的配置响应 |
| `task/urllist/ui/ui.go` | 注册 `HasForm: true` |
| `task/tktube/ui/ui.go` | 注册 `HasForm: true, HasViewer: true, HasAggregate: true` |
| `task/hanime/ui/ui.go` | 注册 `HasViewer: true` |
| `task/vikacg/ui/ui.go` | 注册 `HasViewer: true` |

---

### 任务 1：创建 `registry.js` — TaskUI 注册表核心

**文件：** 创建 `web/static/app/taskui/registry.js`

- [ ] **步骤 1：编写 registry.js 代码**

```javascript
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * TaskUI — 全局 task 类型 UI 组件注册表
 * 每个 task 类型通过 TaskUI.register(type, handler) 注册自己的 UI 组件。
 * 核心 UI 通过 TaskUI.get(type) 获取处理器，使用 Vue h() 函数渲染。
 */
;(function () {
  'use strict'

  var registry = {}

  /**
   * 规范化 handler，确保所有可选方法有默认值
   */
  function normalizeHandler(handler) {
    handler = handler || {}
    return {
      type: handler.type || '',
      label: handler.label || handler.type || '',
      icon: handler.icon || 'fa-cube',
      renderForm: typeof handler.renderForm === 'function' ? handler.renderForm : null,
      renderMeta: typeof handler.renderMeta === 'function' ? handler.renderMeta : null,
      renderCardExtra: typeof handler.renderCardExtra === 'function' ? handler.renderCardExtra : null,
      renderViewer: typeof handler.renderViewer === 'function' ? handler.renderViewer : null,
      renderAggregate: typeof handler.renderAggregate === 'function' ? handler.renderAggregate : null,
      shouldShowViewer: typeof handler.shouldShowViewer === 'function' ? handler.shouldShowViewer : defaultShouldShowViewer,
      viewerLabel: handler.viewerLabel || '查看',
    }
  }

  function defaultShouldShowViewer(obj) {
    return obj && obj.status === 'completed'
  }

  window.TaskUI = {
    register: function (type, handler) {
      if (!type || !handler) return
      registry[type] = normalizeHandler(handler)
    },
    get: function (type) {
      return registry[type] || null
    },
    list: function () {
      return Object.keys(registry)
    },
    hasForm: function (type) {
      var h = registry[type]
      return h && typeof h.renderForm === 'function'
    },
    hasViewer: function (type) {
      var h = registry[type]
      return h && typeof h.renderViewer === 'function'
    },
    hasMeta: function (type) {
      var h = registry[type]
      return h && typeof h.renderMeta === 'function'
    },
    hasAggregate: function (type) {
      var h = registry[type]
      return h && typeof h.renderAggregate === 'function'
    },
    hasCardExtra: function (type) {
      var h = registry[type]
      return h && typeof h.renderCardExtra === 'function'
    },
  }
})()
```

- [ ] **步骤 2：验证文件语法正确**

运行：`node --check web/static/app/taskui/registry.js` （预期：无输出）

---

### 任务 2：创建 `defineForm.js` — 声明式表单工厂

**文件：** 创建 `web/static/app/taskui/defineForm.js`

- [ ] **步骤 1：编写 defineForm.js 代码**

```javascript
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * defineForm — 声明式表单工厂
 * 简单 task 类型通过配置字段列表快速生成 renderForm 函数。
 *
 * 支持字段类型：text, number, select, textarea, checkbox
 * 字段验证：required, min, max, pattern
 */
;(function () {
  'use strict'

  var FIELD_TYPES = {
    text: renderTextField,
    number: renderNumberField,
    select: renderSelectField,
    textarea: renderTextareaField,
    checkbox: renderCheckboxField,
  }

  /**
   * 创建一个声明式表单渲染函数
   * @param {Object} config
   * @param {Array} config.fields - 字段定义数组
   * @returns {Function} renderForm(h, formData, formErrors)
   */
  function defineForm(config) {
    config = config || {}
    var fields = config.fields || []

    return function renderForm(h, formData, formErrors) {
      formErrors = formErrors || {}
      return h('div', { class: 'space-y-4' }, fields.map(function (field) {
        var renderer = FIELD_TYPES[field.type] || renderTextField
        return renderer(h, field, formData, formErrors)
      }))
    }
  }

  function renderTextField(h, field, data, errors) {
    return renderFieldWrapper(h, field, errors, [
      h('input', {
        class: 'w-full border border-gray-300 rounded-md p-2 focus:ring-2 focus:ring-blue-500 focus:border-blue-500' + (errors[field.key] ? ' border-red-500' : ''),
        attrs: { type: 'text', placeholder: field.placeholder || '', id: 'field-' + field.key },
        domProps: { value: data[field.key] || '' },
        on: { input: function (e) { data[field.key] = e.target.value } }
      })
    ])
  }

  function renderNumberField(h, field, data, errors) {
    return renderFieldWrapper(h, field, errors, [
      h('input', {
        class: 'w-full border border-gray-300 rounded-md p-2 focus:ring-2 focus:ring-blue-500 focus:border-blue-500' + (errors[field.key] ? ' border-red-500' : ''),
        attrs: { type: 'number', min: field.min, max: field.max, placeholder: field.placeholder || '', id: 'field-' + field.key },
        domProps: { value: data[field.key] !== undefined ? data[field.key] : field.default },
        on: { input: function (e) { data[field.key] = Number(e.target.value) } }
      })
    ])
  }

  function renderSelectField(h, field, data, errors) {
    return renderFieldWrapper(h, field, errors, [
      h('select', {
        class: 'w-full border border-gray-300 rounded-md p-2 focus:ring-2 focus:ring-blue-500 focus:border-blue-500' + (errors[field.key] ? ' border-red-500' : ''),
        attrs: { id: 'field-' + field.key },
        domProps: { value: data[field.key] || '' },
        on: { change: function (e) { data[field.key] = e.target.value } }
      }, (field.options || []).map(function (opt) {
        var optValue = typeof opt === 'string' ? opt : opt.value
        var optLabel = typeof opt === 'string' ? opt : opt.label
        return h('option', { attrs: { value: optValue } }, optLabel)
      }))
    ])
  }

  function renderTextareaField(h, field, data, errors) {
    return renderFieldWrapper(h, field, errors, [
      h('textarea', {
        class: 'w-full border border-gray-300 rounded-md p-2 font-mono text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500' + (errors[field.key] ? ' border-red-500' : ''),
        attrs: { rows: field.rows || 5, placeholder: field.placeholder || '', id: 'field-' + field.key },
        domProps: { value: data[field.key] || '' },
        on: { input: function (e) { data[field.key] = e.target.value } }
      })
    ])
  }

  function renderCheckboxField(h, field, data, errors) {
    return h('div', { class: 'flex items-center gap-2' }, [
      h('input', {
        attrs: { type: 'checkbox', id: 'field-' + field.key },
        domProps: { checked: !!data[field.key] },
        on: { change: function (e) { data[field.key] = e.target.checked } }
      }),
      h('label', { attrs: { for: 'field-' + field.key }, class: 'text-sm font-medium text-gray-700' }, field.label || '')
    ])
  }

  function renderFieldWrapper(h, field, errors, children) {
    var errorMsg = errors[field.key]
    return h('div', [
      field.label ? h('label', {
        attrs: { for: 'field-' + field.key },
        class: 'block text-sm font-medium text-gray-700 mb-1'
      }, field.label + (field.required ? ' *' : '')) : null,
      h('div', children),
      errorMsg ? h('p', { class: 'text-xs text-red-500 mt-1' }, errorMsg) : null,
      field.description ? h('p', { class: 'text-xs text-gray-500 mt-1' }, field.description) : null,
    ])
  }

  // 导出
  window.TaskUI = window.TaskUI || {}
  window.TaskUI.defineForm = defineForm
})()
```

- [ ] **步骤 2：验证文件语法正确**

运行：`node --check web/static/app/taskui/defineForm.js` （预期：无输出）

---

### 任务 3：创建 `defineMeta.js` — 声明式元数据工厂

**文件：** 创建 `web/static/app/taskui/defineMeta.js`

- [ ] **步骤 1：编写 defineMeta.js 代码**

```javascript
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * defineMeta — 声明式元数据工厂
 * 简单 task 类型通过配置字段列表快速生成 renderMeta 函数。
 *
 * 支持字段类型：text, count, json
 * 数据路径：'extra.urls'、'save_dir' 等点号分隔路径
 */
;(function () {
  'use strict'

  /**
   * 根据点号路径获取嵌套值
   * 例：resolvePath({ extra: { urls: [...] } }, 'extra.urls') → [...]
   */
  function resolvePath(obj, path) {
    if (!obj || !path) return undefined
    var parts = path.split('.')
    var current = obj
    for (var i = 0; i < parts.length; i++) {
      if (current == null) return undefined
      current = current[parts[i]]
    }
    return current
  }

  var META_RENDERERS = {
    text: renderMetaText,
    count: renderMetaCount,
    json: renderMetaJson,
  }

  function defineMeta(config) {
    config = config || {}
    var fields = config.fields || []

    return function renderMeta(h, task) {
      if (!task) return h('div', { class: 'text-xs text-gray-500' }, '无数据')
      return h('div', { class: 'grid grid-cols-1 md:grid-cols-3 gap-2 text-xs text-gray-600' }, fields.map(function (field) {
        var renderer = META_RENDERERS[field.type] || renderMetaText
        return renderer(h, field, task)
      }))
    }
  }

  function renderMetaText(h, field, task) {
    var value = resolvePath(task, field.path)
    if (value === undefined || value === null) value = '-'
    return h('div', [
      h('span', { class: 'text-gray-400' }, field.label + '：'),
      h('span', String(value))
    ])
  }

  function renderMetaCount(h, field, task) {
    var value = resolvePath(task, field.path)
    var count = 0
    if (Array.isArray(value)) count = value.length
    else if (typeof value === 'number') count = value
    return h('div', [
      h('span', { class: 'text-gray-400' }, field.label + '：'),
      h('span', String(count))
    ])
  }

  function renderMetaJson(h, field, task) {
    var value = resolvePath(task, field.path)
    var text = '-'
    try { text = JSON.stringify(value, null, 2) } catch (e) {}
    return h('div', { class: 'break-all' }, [
      h('span', { class: 'text-gray-400' }, field.label + '：'),
      h('pre', { class: 'inline whitespace-pre-wrap' }, text)
    ])
  }

  // 导出
  window.TaskUI = window.TaskUI || {}
  window.TaskUI.defineMeta = defineMeta
})()
```

- [ ] **步骤 2：验证文件语法正确**

运行：`node --check web/static/app/taskui/defineMeta.js` （预期：无输出）

---

### 任务 4：创建基础模板文件

**文件：** 创建 `web/static/app/taskui/baseForm.js`、`baseMeta.js`、`baseViewer.js`、`loader.js`

- [ ] **步骤 1：创建 baseForm.js — 基础表单模板**

```javascript
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * BaseForm — 基础新建任务表单
 * 通用字段：任务ID、保存目录、存储类型
 * showExtraFields 插槽供 task 类型扩展
 */
;(function () {
  'use strict'

  function BaseForm(h, formData, formErrors, extraFields) {
    return h('div', { class: 'space-y-4' }, [
      // 任务ID
      h('div', [
        h('label', { class: 'block text-sm font-medium text-gray-700 mb-1' }, '任务ID *'),
        h('input', {
          class: 'w-full border border-gray-300 rounded-md p-2 focus:ring-2 focus:ring-blue-500 focus:border-blue-500',
          attrs: { type: 'text', placeholder: '例如：my_download_task' },
          domProps: { value: formData.id || '' },
          on: { input: function (e) { formData.id = e.target.value } }
        })
      ]),
      // 保存目录
      h('div', [
        h('label', { class: 'block text-sm font-medium text-gray-700 mb-1' }, '保存目录'),
        h('input', {
          class: 'w-full border border-gray-300 rounded-md p-2 focus:ring-2 focus:ring-blue-500 focus:border-blue-500',
          attrs: { type: 'text', placeholder: './downloads/mytask' },
          domProps: { value: formData.save_dir || '' },
          on: { input: function (e) { formData.save_dir = e.target.value } }
        })
      ]),
      // 存储类型
      h('div', { class: 'grid grid-cols-2 gap-4' }, [
        h('div', [
          h('label', { class: 'block text-sm font-medium text-gray-700 mb-1' }, '存储类型'),
          h('select', {
            class: 'w-full border border-gray-300 rounded-md p-2',
            domProps: { value: formData.storage_type || 'file' },
            on: { change: function (e) { formData.storage_type = e.target.value } }
          }, [
            h('option', { attrs: { value: 'file' } }, '文件'),
            h('option', { attrs: { value: 'mongo' } }, 'Mongo'),
            h('option', { attrs: { value: 'memory' } }, '内存'),
          ])
        ]),
        formData.storage_type === 'file' ? h('div', [
          h('label', { class: 'block text-sm font-medium text-gray-700 mb-1' }, '文件路径'),
          h('input', {
            class: 'w-full border border-gray-300 rounded-md p-2',
            attrs: { type: 'text', placeholder: formData.save_dir + '/' + (formData.id || 'task') + '_history.json' },
            domProps: { value: formData.storage_config && formData.storage_config.path || '' },
            on: { input: function (e) { formData.storage_config = formData.storage_config || {}; formData.storage_config.path = e.target.value } }
          })
        ]) : null,
        formData.storage_type === 'mongo' ? h('div', { class: 'col-span-2 grid grid-cols-3 gap-2' }, [
          h('div', [
            h('label', { class: 'block text-sm font-medium text-gray-700 mb-1' }, '来源'),
            h('input', {
              class: 'w-full border border-gray-300 rounded-md p-2',
              domProps: { value: formData.storage_config && formData.storage_config.source || '' },
              on: { input: function (e) { formData.storage_config = formData.storage_config || {}; formData.storage_config.source = e.target.value } }
            })
          ]),
          h('div', [
            h('label', { class: 'block text-sm font-medium text-gray-700 mb-1' }, '数据库'),
            h('input', {
              class: 'w-full border border-gray-300 rounded-md p-2',
              domProps: { value: formData.storage_config && formData.storage_config.database || '' },
              on: { input: function (e) { formData.storage_config = formData.storage_config || {}; formData.storage_config.database = e.target.value } }
            })
          ]),
          h('div', [
            h('label', { class: 'block text-sm font-medium text-gray-700 mb-1' }, '集合'),
            h('input', {
              class: 'w-full border border-gray-300 rounded-md p-2',
              domProps: { value: formData.storage_config && formData.storage_config.collection || '' },
              on: { input: function (e) { formData.storage_config = formData.storage_config || {}; formData.storage_config.collection = e.target.value } }
            })
          ])
        ]) : null,
      ]),
      // Task 类型特定扩展字段
      extraFields ? extraFields(h, formData) : null,
    ])
  }

  window.TaskUI = window.TaskUI || {}
  window.TaskUI.BaseForm = BaseForm
})()
```

- [ ] **步骤 2：创建 baseMeta.js — 基础元数据模板**

```javascript
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * BaseMeta — 基础任务元数据显示
 * 通用信息：任务ID、类型、状态、存储配置
 * showExtraMeta 插槽供 task 类型扩展
 */
;(function () {
  'use strict'

  function BaseMeta(h, task, extraMeta) {
    if (!task) return h('div', { class: 'text-xs text-gray-500' }, '无数据')
    return h('div', { class: 'grid grid-cols-1 md:grid-cols-3 gap-2 text-xs text-gray-600' }, [
      h('div', [
        h('span', { class: 'text-gray-400' }, '任务ID：'),
        h('span', task.id || '-')
      ]),
      h('div', [
        h('span', { class: 'text-gray-400' }, '类型：'),
        h('span', task.type || '-')
      ]),
      h('div', [
        h('span', { class: 'text-gray-400' }, '状态：'),
        h('span', task.status || 'unknown')
      ]),
      h('div', [
        h('span', { class: 'text-gray-400' }, '并发数：'),
        h('span', String(task.concurrency || '-'))
      ]),
      h('div', [
        h('span', { class: 'text-gray-400' }, '刷新间隔（秒）：'),
        h('span', String(task.refresh_interval || '-'))
      ]),
      h('div', { class: 'md:col-span-2' }, [
        h('span', { class: 'text-gray-400' }, '存储路径：'),
        h('span', (task.save_dir || '') || '未提供')
      ]),
      task.storage ? h('div', { class: 'md:col-span-2' }, [
        h('div', { class: 'text-gray-400' }, '存储配置'),
        h('div', { class: 'grid grid-cols-1 md:grid-cols-3 gap-2 text-xs text-gray-600' }, [
          h('div', [h('span', { class: 'text-gray-400' }, '类型：'), h('span', task.storage.type || '-')]),
          task.storage.config && task.storage.config.path ? h('div', [h('span', { class: 'text-gray-400' }, '路径：'), h('span', task.storage.config.path)]) : null,
          task.storage.config && task.storage.config.source ? h('div', [h('span', { class: 'text-gray-400' }, '源：'), h('span', task.storage.config.source)]) : null,
          task.storage.config && task.storage.config.database ? h('div', [h('span', { class: 'text-gray-400' }, '库：'), h('span', task.storage.config.database)]) : null,
          task.storage.config && task.storage.config.collection ? h('div', [h('span', { class: 'text-gray-400' }, '集合：'), h('span', task.storage.config.collection)]) : null,
        ])
      ]) : null,
      // Task 类型特定扩展元数据
      extraMeta ? extraMeta(h, task) : null,
    ])
  }

  window.TaskUI = window.TaskUI || {}
  window.TaskUI.BaseMeta = BaseMeta
})()
```

- [ ] **步骤 3：创建 baseViewer.js — 基础查看器模态框骨架**

```javascript
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * BaseViewer — 基础查看器模态框骨架
 * 提供模态框容器（header/body/footer），关闭按钮，ESC/背景点击关闭
 * contentRenderer 插槽供 task 类型自定义内容
 */
;(function () {
  'use strict'

  function BaseViewer(h, obj, onClose, contentRenderer) {
    var title = (obj && obj.metadata && obj.metadata.title) || ''
    return h('div', {
      class: 'fixed inset-0 bg-black bg-opacity-70 z-50 flex items-center justify-center p-4 backdrop-blur-sm',
      on: {
        click: function (e) { if (e.target === e.currentTarget && onClose) onClose() },
        keydown: function (e) { if (e.key === 'Escape' && onClose) onClose() }
      }
    }, [
      h('div', { class: 'bg-white rounded-lg shadow-2xl w-full max-w-5xl max-h-[90vh] overflow-hidden flex flex-col' }, [
        // Header
        h('div', { class: 'p-4 border-b flex justify-between items-center bg-gray-50' }, [
          h('h3', { class: 'text-lg font-bold text-gray-800' }, title),
          onClose ? h('button', {
            class: 'text-gray-500 hover:text-gray-700',
            on: { click: onClose }
          }, [h('i', { class: 'fas fa-times' })]) : null,
        ]),
        // Body
        h('div', { class: 'flex-1 overflow-y-auto' }, [
          contentRenderer ? contentRenderer(h, obj) : null,
        ]),
        // Footer
        h('div', { class: 'p-3 border-t bg-gray-50 flex justify-end' }, [
          onClose ? h('button', {
            class: 'px-3 py-1.5 rounded bg-white border hover:bg-gray-100 text-sm',
            on: { click: onClose }
          }, '关闭') : null,
        ]),
      ])
    ])
  }

  window.TaskUI = window.TaskUI || {}
  window.TaskUI.BaseViewer = BaseViewer
})()
```

- [ ] **步骤 4：创建 loader.js — 动态加载 task UI 组件**

```javascript
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * Loader — 动态加载 task 类型 UI 组件
 * 通过 /api/ui/{type}/config 获取配置，加载 JS/CSS 资产
 * 依赖现有后端资产服务机制
 */
;(function () {
  'use strict'

  var loadedTypes = {}

  function loadTaskUI(taskType, callback) {
    if (!taskType || taskType === 'all') return
    if (loadedTypes[taskType]) {
      if (callback) callback()
      return
    }

    fetch('/api/ui/' + encodeURIComponent(taskType) + '/config')
      .then(function (r) { return r.json() })
      .then(function (cfg) {
        var pending = 0
        function onLoad() {
          pending--
          if (pending <= 0) {
            loadedTypes[taskType] = true
            if (callback) callback()
          }
        }

        // Load CSS
        if (cfg.css && cfg.css.length > 0) {
          cfg.css.forEach(function (p) {
            var href = '/api/ui/' + encodeURIComponent(taskType) + '/assets/' + encodeURIComponent(p)
            if (!document.querySelector('link[href="' + href + '"]')) {
              pending++
              var link = document.createElement('link')
              link.rel = 'stylesheet'
              link.href = href
              link.onload = onLoad
              link.onerror = onLoad
              document.head.appendChild(link)
            }
          })
        }

        // Load JS
        if (cfg.js && cfg.js.length > 0) {
          cfg.js.forEach(function (p) {
            var src = '/api/ui/' + encodeURIComponent(taskType) + '/assets/' + encodeURIComponent(p)
            if (!document.querySelector('script[src="' + src + '"]')) {
              pending++
              var script = document.createElement('script')
              script.src = src
              script.onload = onLoad
              script.onerror = onLoad
              document.body.appendChild(script)
            }
          })
        }

        // If no assets to load, mark as loaded
        if (pending === 0) {
          loadedTypes[taskType] = true
          if (callback) callback()
        }
      })
      .catch(function (e) {
        console.warn('loadTaskUI failed for', taskType, e)
        if (callback) callback()
      })
  }

  window.TaskUI = window.TaskUI || {}
  window.TaskUI.loadTaskUI = loadTaskUI
  window.TaskUI._loadedTypes = loadedTypes
})()
```

- [ ] **步骤 5：验证所有文件语法正确**

运行：`node --check web/static/app/taskui/baseForm.js && node --check web/static/app/taskui/baseMeta.js && node --check web/static/app/taskui/baseViewer.js && node --check web/static/app/taskui/loader.js` （预期：无输出）

---

### 任务 5：创建 urllist 类型 UI 组件

**文件：** 创建 `web/static/tasks/urllist/ui.js`

- [ ] **步骤 1：编写 ui.js**

```javascript
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

;(function () {
  'use strict'

  TaskUI.register('url_list', {
    type: 'url_list',
    label: 'URL 列表',
    icon: 'fa-link',
    // 使用声明式表单配置
    renderForm: TaskUI.defineForm({
      fields: [
        {
          type: 'textarea',
          key: 'urls_text',
          label: 'URL 列表（每行一个）',
          rows: 10,
          required: true,
          placeholder: 'https://example.com/file1.zip\nhttps://example.com/file2.zip'
        }
      ]
    }),
    // 使用声明式元数据配置
    renderMeta: TaskUI.defineMeta({
      fields: [
        { type: 'count', key: 'urls', label: 'URL 数量', path: 'extra.urls' }
      ]
    })
  })
})()
```

- [ ] **步骤 2：验证文件语法正确**

运行：`node --check web/static/tasks/urllist/ui.js` （预期：无输出）

---

### 任务 6：创建 tktube 类型 UI 组件

**文件：** 创建 `web/static/tasks/tktube/ui.js`（注意：此文件替换现有 `viewer.js` 的功能，并在 `__dm_uiBridge` 基础上迁移到 `TaskUI`）

- [ ] **步骤 1：编写 ui.js**

```javascript
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

;(function () {
  'use strict'

  // ---- Helper functions ----
  function getTitle(obj) { return (obj && obj.metadata && obj.metadata.title) || '' }
  function getContentGroup(obj) { return (obj && obj.metadata && obj.metadata.content_group) || '' }
  function getResolution(obj) { return (obj && obj.metadata && obj.metadata.resolution) || '' }
  function getDate(obj) { return (obj && obj.metadata && obj.metadata.date) || '' }
  function getDuration(obj) { return (obj && obj.metadata && obj.metadata.duration) || '' }

  function fileUrl(path) {
    if (!path) return ''
    var normalized = path.replace(/\\/g, '/')
    var root = typeof window.__dm_downloadRoot === 'string' ? window.__dm_downloadRoot : ''
    if (root && normalized.indexOf(root) === 0) {
      normalized = normalized.slice(root.length)
    }
    normalized = normalized.replace(/^\//, '')
    return '/files/' + normalized.split('/').filter(function(s){return s&&s!=='..'}).map(encodeURIComponent).join('/')
  }

  function getFileUrl(obj) {
    if (obj && obj.save_path) return fileUrl(obj.save_path)
    return ''
  }

  function statusColor(status) {
    switch (status) {
      case 'completed': return '#10b981'
      case 'downloading': return '#3b82f6'
      case 'failed': return '#ef4444'
      case 'cancelled': return '#9ca3af'
      default: return '#6b7280'
    }
  }

  function statusBg(status) {
    switch (status) {
      case 'completed': return '#d1fae5'
      case 'downloading': return '#dbeafe'
      case 'failed': return '#fee2e2'
      case 'cancelled': return '#f3f4f6'
      default: return '#f3f4f6'
    }
  }

  function priorityScore(obj) {
    if (obj && obj.extra) {
      if (obj.extra.variant_priority !== undefined) return obj.extra.variant_priority
      if (obj.extra.priority !== undefined) return obj.extra.priority
    }
    var r = getResolution(obj)
    if (/1080/.test(r)) return 30
    if (/720/.test(r)) return 20
    if (/480/.test(r)) return 10
    return 0
  }

  var THEME = {
    primary: '#3b82f6', primaryDark: '#2563eb', bg: '#ffffff',
    bgAlt: '#f9fafb', border: '#e5e7eb', text: '#1f2937',
    textSecondary: '#6b7280', textMuted: '#9ca3af'
  }

  function createObjectCard(obj) {
    var card = document.createElement('div')
    card.style.cssText = 'border:1px solid ' + THEME.border + ';border-radius:8px;overflow:hidden;background:' + THEME.bg + ';transition:box-shadow 0.2s'
    card.onmouseenter = function () { card.style.boxShadow = '0 4px 12px rgba(0,0,0,0.1)' }
    card.onmouseleave = function () { card.style.boxShadow = 'none' }

    var coverArea = document.createElement('div')
    coverArea.style.cssText = 'position:relative;background:#f3f4f6;aspect-ratio:16/9;display:flex;align-items:center;justify-content:center;overflow:hidden'

    var coverUrl = ''
    if (obj && obj.status === 'completed') {
      if (obj.extra && obj.extra.local_preview) coverUrl = fileUrl(obj.extra.local_preview)
      else if (obj.extra && obj.extra.local_cover) coverUrl = fileUrl(obj.extra.local_cover)
      else if (obj.save_path) coverUrl = fileUrl(obj.save_path)
    }
    if (!coverUrl && obj && obj.extra) {
      if (obj.extra.thumb_url) coverUrl = obj.extra.thumb_url
      else if (obj.extra.preview_url) coverUrl = obj.extra.preview_url
      else if (obj.extra.cover_url) coverUrl = obj.extra.cover_url
    }

    if (coverUrl) {
      var img = document.createElement('img')
      img.src = coverUrl
      img.alt = ''
      img.style.cssText = 'width:100%;height:100%;object-fit:cover'
      img.onerror = function () { img.style.display = 'none' }
      coverArea.appendChild(img)
    } else {
      var icon = document.createElement('i')
      icon.className = 'fas fa-video'
      icon.style.cssText = 'font-size:32px;color:' + THEME.textMuted
      coverArea.appendChild(icon)
    }

    var badge = document.createElement('span')
    badge.style.cssText = 'position:absolute;top:6px;right:6px;font-size:10px;padding:2px 6px;border-radius:4px;font-weight:600;background:' + statusBg(obj.status) + ';color:' + statusColor(obj.status)
    badge.textContent = obj.status || 'unknown'
    coverArea.appendChild(badge)

    var res = getResolution(obj)
    if (res) {
      var resBadge = document.createElement('span')
      resBadge.style.cssText = 'position:absolute;bottom:6px;right:6px;font-size:10px;padding:2px 6px;border-radius:4px;background:rgba(0,0,0,0.7);color:#fff'
      resBadge.textContent = res
      coverArea.appendChild(resBadge)
    }
    card.appendChild(coverArea)

    var info = document.createElement('div')
    info.style.cssText = 'padding:10px 12px'
    var titleEl = document.createElement('div')
    titleEl.style.cssText = 'font-size:13px;font-weight:600;color:' + THEME.text + ';line-height:1.3;overflow:hidden;text-overflow:ellipsis;white-space:nowrap'
    titleEl.textContent = getTitle(obj) || obj.url || 'Untitled'
    info.appendChild(titleEl)

    var metaRow = document.createElement('div')
    metaRow.style.cssText = 'display:flex;gap:8px;font-size:11px;color:' + THEME.textSecondary + ';margin-top:4px'
    var dur = getDuration(obj)
    if (dur) { var durEl = document.createElement('span'); durEl.textContent = dur; metaRow.appendChild(durEl) }
    var dateVal = getDate(obj)
    if (dateVal) {
      if (dur) { var sep = document.createElement('span'); sep.textContent = '|'; metaRow.appendChild(sep) }
      var dateEl = document.createElement('span'); dateEl.textContent = dateVal; metaRow.appendChild(dateEl)
    }
    if (dur || dateVal) info.appendChild(metaRow)

    if (obj.progress > 0 && obj.progress < 100 && obj.status === 'downloading') {
      var progOuter = document.createElement('div')
      progOuter.style.cssText = 'margin-top:8px;height:4px;background:#f3f4f6;border-radius:2px;overflow:hidden'
      var progInner = document.createElement('div')
      progInner.style.cssText = 'height:100%;background:' + THEME.primary + ';border-radius:2px;transition:width 0.3s'
      progInner.style.width = (obj.progress || 0) + '%'
      progOuter.appendChild(progInner)
      info.appendChild(progOuter)
    }
    card.appendChild(info)

    card.style.cursor = 'pointer'
    card.onclick = function () {
      if (obj.status === 'completed') {
        var url = getFileUrl(obj)
        if (url) {
          var ext = (obj.save_path || '').split('.').pop().toLowerCase()
          if (ext === 'mp4' || ext === 'webm' || ext === 'mkv') {
            var videoWin = window.open('', '_blank')
            if (videoWin) {
              var doc = videoWin.document
              doc.body.style.margin = '0'; doc.body.style.background = '#000'
              doc.body.style.display = 'flex'; doc.body.style.alignItems = 'center'
              doc.body.style.justifyContent = 'center'; doc.body.style.height = '100vh'
              var video = doc.createElement('video')
              video.src = url; video.controls = true; video.autoplay = true
              video.style.maxWidth = '100%'; video.style.maxHeight = '100%'
              doc.body.appendChild(video); doc.close()
            }
          } else { window.open(url, '_blank') }
        }
      }
    }
    return card
  }

  function renderTaskView(task) {
    var container = document.getElementById('custom-task-container')
    if (!container) return
    container.innerHTML = ''

    var objects = (task && task.objects) || []
    if (objects.length === 0) {
      container.innerHTML = '<div style="padding:32px;text-align:center;color:' + THEME.textSecondary + ';font-size:14px">暂无对象</div>'
      return
    }

    var groups = {}, ungrouped = []
    objects.forEach(function (obj) {
      var g = getContentGroup(obj)
      if (g) { if (!groups[g]) groups[g] = []; groups[g].push(obj) }
      else { ungrouped.push(obj) }
    })

    var groupNames = Object.keys(groups).sort(function (a, b) {
      var aMax = Math.max.apply(null, groups[a].map(priorityScore))
      var bMax = Math.max.apply(null, groups[b].map(priorityScore))
      return bMax - aMax
    })

    var wrapper = document.createElement('div')
    wrapper.style.cssText = 'padding:16px;overflow-y:auto;height:100%'

    if (groupNames.length > 0) {
      groupNames.forEach(function (groupName) {
        var items = groups[groupName]
        items.sort(function (a, b) { return priorityScore(b) - priorityScore(a) })
        var section = document.createElement('div')
        section.style.cssText = 'margin-bottom:24px'
        var headerRow = document.createElement('div')
        headerRow.style.cssText = 'display:flex;align-items:center;gap:12px;margin-bottom:12px;padding-bottom:8px;border-bottom:2px solid ' + THEME.primary
        var groupTitle = document.createElement('h3')
        groupTitle.style.cssText = 'font-size:16px;font-weight:600;color:' + THEME.text + ';margin:0'
        groupTitle.textContent = groupName
        headerRow.appendChild(groupTitle)
        var countBadge = document.createElement('span')
        countBadge.style.cssText = 'font-size:11px;background:' + THEME.primary + ';color:#fff;border-radius:10px;padding:2px 8px'
        countBadge.textContent = items.length + ' 项'
        headerRow.appendChild(countBadge)
        section.appendChild(headerRow)
        var grid = document.createElement('div')
        grid.style.cssText = 'display:grid;grid-template-columns:repeat(auto-fill,minmax(180px,1fr));gap:12px'
        items.forEach(function (obj) { grid.appendChild(createObjectCard(obj)) })
        section.appendChild(grid)
        wrapper.appendChild(section)
      })
    }

    if (ungrouped.length > 0) {
      ungrouped.sort(function (a, b) { return priorityScore(b) - priorityScore(a) })
      var section = document.createElement('div')
      section.style.cssText = 'margin-bottom:24px'
      var headerRow = document.createElement('div')
      headerRow.style.cssText = 'display:flex;align-items:center;gap:12px;margin-bottom:12px;padding-bottom:8px;border-bottom:2px solid ' + THEME.textMuted
      var groupTitle = document.createElement('h3')
      groupTitle.style.cssText = 'font-size:16px;font-weight:600;color:' + THEME.text + ';margin:0'
      groupTitle.textContent = '未分组'
      headerRow.appendChild(groupTitle)
      var countBadge = document.createElement('span')
      countBadge.style.cssText = 'font-size:11px;background:' + THEME.textMuted + ';color:#fff;border-radius:10px;padding:2px 8px'
      countBadge.textContent = ungrouped.length + ' 项'
      headerRow.appendChild(countBadge)
      section.appendChild(headerRow)
      var grid = document.createElement('div')
      grid.style.cssText = 'display:grid;grid-template-columns:repeat(auto-fill,minmax(180px,1fr));gap:12px'
      ungrouped.forEach(function (obj) { grid.appendChild(createObjectCard(obj)) })
      section.appendChild(grid)
      wrapper.appendChild(section)
    }
    container.appendChild(wrapper)
  }

  // ---- 注册 tktube UI ----
  TaskUI.register('tktube', {
    type: 'tktube',
    label: 'TKTube',
    icon: 'fa-video',
    renderForm: TaskUI.defineForm({
      fields: [
        { type: 'text', key: 'keyword', label: '关键字', required: true, placeholder: '例如：RCTD' },
        {
          type: 'select', key: 'subtype', label: '子类型',
          options: [
            { value: 'tag', label: '标签' },
            { value: 'model', label: '模特' },
            { value: 'search', label: '搜索' }
          ]
        },
        { type: 'number', key: 'max_concurrent', label: '并发数', min: 1, max: 10, default: 2 },
        { type: 'number', key: 'refresh_interval', label: '刷新间隔（秒）', min: 10, default: 3600 },
      ]
    }),
    renderMeta: TaskUI.defineMeta({
      fields: [
        { type: 'text', key: 'keyword', label: '关键字', path: 'extra.keyword' },
        { type: 'text', key: 'subtype', label: '子类型', path: 'extra.subtype' },
        { type: 'text', key: 'max_concurrent', label: '并发', path: 'extra.max_concurrent' },
        { type: 'text', key: 'refresh_interval', label: '刷新间隔', path: 'extra.refresh_interval' },
      ]
    }),
    viewerLabel: '播放',
    shouldShowViewer: function (obj) { return obj.status === 'completed' },
    renderViewer: function (h, obj, onClose) {
      // 使用 BaseViewer 骨架 + 视频播放
      return TaskUI.BaseViewer(h, obj, onClose, function (h, obj) {
        var videoUrl = (obj && obj.save_path) ? fileUrl(obj.save_path) : ''
        return h('div', { class: 'p-4' }, [
          videoUrl ? h('video', {
            attrs: { src: videoUrl, controls: true, autoplay: true },
            class: 'w-full rounded-lg bg-black max-h-[60vh]'
          }) : h('p', { class: 'text-gray-500 text-center py-8' }, '暂无可用视频'),
        ])
      })
    },
    renderAggregate: function (h, objects, params) {
      // 返回 null 表示使用 DOM 渲染方式（通过 renderTaskView）
      return null
    }
  })

  // 兼容：保留 __dm_uiBridge 注册，确保旧代码调用不中断
  if (window.__dm_uiBridge) {
    window.__dm_uiBridge.registerTaskView('tktube', {
      render: function (task) { renderTaskView(task) }
    })
  }
})()
```

- [ ] **步骤 2：验证文件语法正确**

运行：`node --check web/static/tasks/tktube/ui.js` （预期：无输出）

---

### 任务 7：创建 hanime 类型 UI 组件

**文件：** 创建 `web/static/tasks/hanime/ui.js`（此文件替换 `viewer.js` 的模块功能，迁移到 `TaskUI`）

- [ ] **步骤 1：编写 ui.js 查看器部分**

```javascript
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

;(function () {
  'use strict'

  // ---- Helper functions ----
  function getTitle(obj) { return (obj && obj.metadata && obj.metadata.title) || '' }

  function getTags(obj) {
    var tags = []
    if (obj && obj.extra && Array.isArray(obj.extra.tags)) tags.push.apply(tags, obj.extra.tags)
    if (obj && obj.metadata && Array.isArray(obj.metadata.tags)) tags.push.apply(tags, obj.metadata.tags)
    var set = {}, out = []
    tags.forEach(function (t) {
      var s = (t || '').toString().trim()
      if (s && !set[s]) { set[s] = true; out.push(s) }
    })
    return out
  }

  function getArtist(obj) {
    if (obj && obj.extra && obj.extra.artist) return obj.extra.artist
    if (obj && obj.metadata && obj.metadata.artist) return obj.metadata.artist
    if (obj && obj.metadata && Array.isArray(obj.metadata.authors) && obj.metadata.authors.length) return obj.metadata.authors.join(', ')
    return ''
  }

  function getDescription(obj) {
    var s = ''
    if (obj && obj.extra && obj.extra.description) s = obj.extra.description
    else if (obj && obj.metadata && obj.metadata.description) s = obj.metadata.description
    else if (obj && obj.extra && obj.extra.content_text) s = obj.extra.content_text
    return (typeof s === 'string' ? s : '').replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim()
  }

  function getOriginLink(obj) {
    if (obj && obj.metadata && obj.metadata.page_url) return obj.metadata.page_url
    if (obj && obj.extra && obj.extra.origin_url) return obj.extra.origin_url
    return (obj && obj.url) || ''
  }

  function fileUrl(path) {
    if (!path) return ''
    var normalized = path.replace(/\\/g, '/')
    var root = typeof window.__dm_downloadRoot === 'string' ? window.__dm_downloadRoot : ''
    if (root && normalized.indexOf(root) === 0) { normalized = normalized.slice(root.length) }
    normalized = normalized.replace(/^\//, '')
    return '/files/' + normalized.split('/').filter(function(s){return s&&s!=='..'}).map(encodeURIComponent).join('/')
  }

  function getCoverImages(obj) {
    var imgs = []
    var push = function (u) { if (typeof u === 'string' && u) imgs.push(u) }
    if (obj && obj.status === 'completed' && obj.extra) {
      if (obj.extra.local_cover) push(fileUrl(obj.extra.local_cover))
      if (Array.isArray(obj.extra.files)) {
        obj.extra.files.forEach(function (f) {
          var name = (f.name || f.path || '').toString().toLowerCase()
          if (f.type === 'image' && (name.indexOf('cover') >= 0 || name.indexOf('thumb') >= 0)) {
            if (f.path) push(fileUrl(f.path))
          }
        })
      }
    }
    if (imgs.length === 0 && obj && obj.extra) {
      if (Array.isArray(obj.extra.cover_images)) obj.extra.cover_images.forEach(push)
      if (Array.isArray(obj.extra.cover_urls)) obj.extra.cover_urls.forEach(push)
      if (Array.isArray(obj.extra.covers)) obj.extra.covers.forEach(push)
      if (obj.extra.cover_url) push(obj.extra.cover_url)
      if (obj.extra.cover) push(obj.extra.cover)
    }
    var uniq = [], seen = {}
    imgs.forEach(function (u) { if (u && !seen[u]) { seen[u] = true; uniq.push(u) } })
    return uniq
  }

  function getVideoURL(obj) {
    if (!obj) return ''
    var u = obj.metadata && obj.metadata.video_url || ''
    if (!u && obj.extra && obj.extra.video_url) u = obj.extra.video_url
    if (obj.status === 'completed') {
      if (obj.save_path) {
        var ext = obj.save_path.split('.').pop().toLowerCase()
        if (ext === 'mp4' || ext === 'webm' || ext === 'mkv' || ext === 'm3u8') { return fileUrl(obj.save_path) }
      }
      if (obj.extra && Array.isArray(obj.extra.files)) {
        for (var fi = 0; fi < obj.extra.files.length; fi++) {
          var f = obj.extra.files[fi]
          if (f && (f.type === 'video' || (f.path && /\.(mp4|webm|mkv|m3u8|ts)$/i.test(f.path)))) {
            if (f.path) return fileUrl(f.path)
          }
        }
      }
      if (obj.extra && obj.extra.local_url) return fileUrl(obj.extra.local_url)
      if (obj.extra && obj.extra.file_url) return fileUrl(obj.extra.file_url)
      if (obj.path) return fileUrl(obj.path)
      if (obj.save_path && /\.(mp4|webm|mkv|m3u8|ts)$/i.test(obj.save_path)) return fileUrl(obj.save_path)
    }
    if (typeof u === 'string' && u) return u
    return ''
  }

  function getDetails(obj) {
    var s = ''
    if (obj && obj.extra && obj.extra.details) s = obj.extra.details
    else if (obj && obj.metadata && obj.metadata.details) s = obj.metadata.details
    else if (obj && obj.metadata && obj.metadata.description) s = obj.metadata.description
    else if (obj && obj.extra && obj.extra.description) s = obj.extra.description
    return (typeof s === 'string' ? s : '').replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim()
  }

  function getDate(obj) {
    if (obj && obj.extra && obj.extra.date) return obj.extra.date
    if (obj && obj.metadata && obj.metadata.date) return obj.metadata.date
    return ''
  }

  function getPlaylist(obj) {
    var src = (obj && obj.extra && obj.extra.playlist) || (obj && obj.metadata && obj.metadata.playlist) || []
    var items = []
    function norm(it) {
      if (!it) return null
      if (typeof it === 'string') {
        var s = it.trim()
        if (!s) return null
        if (/^https?:\/\//.test(s) || s.startsWith('/')) return { title: '', thumbnail: '', url: s }
        if (obj && obj.status === 'completed') return { title: '', thumbnail: '', url: fileUrl(s) }
        return { title: s, thumbnail: '', url: '' }
      }
      if (typeof it === 'object') {
        var title = it.title || it.name || it.label || ''
        var url = it.url || it.href || it.link || it.src || ''
        var thumb = it.thumbnail || it.thumb || it.image || it.cover || ''
        if (!url) {
          if (it.path && obj && obj.status === 'completed') url = fileUrl(it.path)
          else if (it.local_url && obj && obj.status === 'completed') url = fileUrl(it.local_url)
          else if (it.file_url && obj && obj.status === 'completed') url = fileUrl(it.file_url)
        } else {
          var pLike = typeof url === 'string' && url && !/^https?:\/\//.test(url) && !url.startsWith('/')
          if (pLike && obj && obj.status === 'completed') url = fileUrl(url)
        }
        if (typeof title !== 'string') title = ''
        if (typeof url !== 'string') url = ''
        if (typeof thumb !== 'string') thumb = ''
        if (!title && !url) return null
        return { title: title, thumbnail: thumb, url: url }
      }
      return null
    }
    if (Array.isArray(src)) { src.forEach(function (x) { var n = norm(x); if (n) items.push(n) }) }
    else { var n = norm(src); if (n) items.push(n) }
    var seen = {}, out = []
    items.forEach(function (it) {
      var k = (it.title || '') + '|' + (it.url || '') + '|' + (it.thumbnail || '')
      if (!seen[k]) { seen[k] = true; out.push(it) }
    })
    return out
  }

  function getGenres(obj) {
    var vals = []
    function pushVal(v) {
      if (Array.isArray(v)) { v.forEach(function (s) { pushVal(s) }); return }
      if (typeof v === 'string') { v.split(/[，、,|/]/).forEach(function (x) { var t = x.trim(); if (t) vals.push(t) }) }
    }
    if (obj && obj.extra) {
      if (obj.extra.genre) pushVal(obj.extra.genre)
      if (obj.extra.genres) pushVal(obj.extra.genres)
      if (obj.extra.categories) pushVal(obj.extra.categories)
      if (obj.extra.tags) pushVal(obj.extra.tags)
    }
    if (obj && obj.metadata) {
      if (obj.metadata.genre) pushVal(obj.metadata.genre)
      if (obj.metadata.genres) pushVal(obj.metadata.genres)
      if (obj.metadata.categories) pushVal(obj.metadata.categories)
      if (obj.metadata.tags) pushVal(obj.metadata.tags)
    }
    var out = [], set = {}
    vals.forEach(function (s) { var t = (s || '').toString().trim(); if (t && !set[t]) { set[t] = true; out.push(t) } })
    return out
  }

  // ---- 注册 hanime UI ----
  TaskUI.register('hanime', {
    type: 'hanime',
    label: 'Hanime',
    icon: 'fa-film',
    viewerLabel: '播放',
    shouldShowViewer: function (obj) { return obj.status === 'completed' },
    renderViewer: function (h, obj, onClose) {
      var videoUrl = getVideoURL(obj)
      var covers = getCoverImages(obj)
      var firstPoster = covers.length > 0 ? covers[0] : ''
      var playlist = getPlaylist(obj)
      var details = getDetails(obj)
      var genres = getGenres(obj)
      var dateVal = getDate(obj)
      var tags = getTags(obj)
      var artist = getArtist(obj)
      var origin = getOriginLink(obj)

      var isHLS = /\.m3u8(\?.*)?$/i.test(videoUrl)
      var isSafari = /safari/i.test(navigator.userAgent) && !/chrome|crios|chromium|edg/i.test(navigator.userAgent)
      var useVideo = !!videoUrl && (!isHLS || isSafari)

      return h('div', {
        class: 'fixed inset-0 bg-black bg-opacity-70 z-50 flex items-center justify-center p-4 backdrop-blur-sm',
        on: {
          click: function (e) { if (e.target === e.currentTarget && onClose) onClose() },
          keydown: function (e) { if (e.key === 'Escape' && onClose) onClose() }
        }
      }, [
        h('div', { class: 'bg-white rounded-lg shadow-2xl w-full max-w-5xl max-h-[90vh] overflow-hidden flex flex-col' }, [
          // Header
          h('div', { class: 'p-4 border-b flex justify-between items-center bg-gray-50' }, [
            h('h3', { class: 'text-lg font-bold text-gray-800' }, getTitle(obj) || 'Hanime'),
            onClose ? h('button', { class: 'text-gray-500 hover:text-gray-700', on: { click: onClose } }, [h('i', { class: 'fas fa-times' })]) : null,
          ]),
          // Body
          h('div', { class: 'flex-1 overflow-y-auto' }, [
            // Video/Cover area
            useVideo ? h('div', { class: 'bg-black flex items-center justify-center' }, [
              h('video', {
                attrs: { src: videoUrl, poster: firstPoster, controls: true, autoplay: true },
                class: 'w-full max-h-[55vh] outline-none'
              })
            ]) : (covers.length > 0 ? h('div', { class: 'bg-black flex items-center justify-center p-4 min-h-[200px]' }, [
              h('img', { attrs: { src: firstPoster }, class: 'max-w-full max-h-[50vh] object-contain' })
            ]) : null),

            // Origin link bar
            origin ? h('div', { class: 'flex gap-2 p-3 bg-gray-50 border-b flex-wrap' }, [
              h('a', {
                attrs: { href: /^https?:\/\//i.test(origin) ? origin : '#', target: '_blank', rel: 'noopener noreferrer' },
                class: 'px-2 py-1 rounded bg-blue-600 text-white text-xs hover:bg-blue-700'
              }, '打开原页面'),
              h('button', {
                class: 'px-2 py-1 rounded bg-white border text-xs hover:bg-gray-100',
                on: { click: function () { if (navigator.clipboard && navigator.clipboard.writeText) navigator.clipboard.writeText(getTitle(obj)) } }
              }, '复制标题'),
              h('button', {
                class: 'px-2 py-1 rounded bg-white border text-xs hover:bg-gray-100',
                on: { click: function () { if (navigator.clipboard && navigator.clipboard.writeText) navigator.clipboard.writeText(origin) } }
              }, '复制链接'),
            ]) : null,

            // Metadata
            h('div', { class: 'p-4' }, [
              (genres.length || dateVal || tags.length || artist) ? h('div', { class: 'text-xs text-gray-500 mb-3 flex flex-wrap gap-1' }, [
                genres.length ? h('span', genres.join(', ')) : null,
                dateVal ? h('span', (genres.length ? ' · ' : '') + dateVal) : null,
                tags.length ? h('span', ((genres.length || dateVal) ? ' · ' : '') + tags.join(', ')) : null,
                artist ? h('span', ((genres.length || dateVal || tags.length) ? ' · ' : '') + artist) : null,
              ]) : null,
              details ? h('div', { class: 'text-sm text-gray-700 whitespace-pre-line mb-3' }, details) : null,
              tags.length ? h('div', { class: 'flex flex-wrap gap-1 mb-3' }, tags.map(function (tag) {
                return h('span', { class: 'text-xs bg-gray-100 text-gray-600 px-2 py-0.5 rounded' }, '#' + tag)
              })) : null,
            ]),

            // Playlist
            playlist.length ? h('div', { class: 'p-3 border-t bg-gray-50' }, [
              h('h4', { class: 'text-sm font-semibold text-gray-700 mb-2' }, '播放列表 (' + playlist.length + ')'),
              h('div', { class: 'flex flex-col gap-1 max-h-[200px] overflow-y-auto' }, playlist.map(function (item, idx) {
                return h('div', {
                  class: 'flex items-center gap-2 p-2 border rounded bg-white text-xs hover:bg-gray-50 cursor-pointer',
                  on: { click: function () { if (item.url) window.open(item.url, '_blank', 'noopener,noreferrer') } }
                }, [
                  h('span', { class: 'w-5 h-5 rounded-full bg-gray-200 flex items-center justify-center text-xs text-gray-600 flex-shrink-0' }, String(idx + 1)),
                  item.thumbnail ? h('img', { attrs: { src: item.thumbnail }, class: 'w-10 h-7 object-cover rounded flex-shrink-0' }) : null,
                  h('span', { class: 'flex-1 truncate text-gray-700' }, item.title || 'Item ' + (idx + 1)),
                ])
              }))
            ]) : null,
          ]),
          // Footer
          h('div', { class: 'p-3 border-t bg-gray-50 flex justify-between items-center' }, [
            h('div', { class: 'flex gap-2' }, [
              origin ? h('a', {
                attrs: { href: /^https?:\/\//i.test(origin) ? origin : '#', target: '_blank', rel: 'noopener noreferrer' },
                class: 'px-3 py-1.5 rounded bg-blue-600 text-white text-sm hover:bg-blue-700'
              }, '打开原页面') : null,
              h('button', {
                class: 'px-3 py-1.5 rounded bg-white border text-sm hover:bg-gray-100',
                on: { click: function () { if (navigator.clipboard && navigator.clipboard.writeText) navigator.clipboard.writeText(getTitle(obj)) } }
              }, '复制标题'),
              origin ? h('button', {
                class: 'px-3 py-1.5 rounded bg-white border text-sm hover:bg-gray-100',
                on: { click: function () { if (navigator.clipboard && navigator.clipboard.writeText) navigator.clipboard.writeText(origin) } }
              }, '复制链接') : null,
            ]),
            onClose ? h('button', {
              class: 'px-3 py-1.5 rounded bg-white border text-sm hover:bg-gray-100',
              on: { click: onClose }
            }, '关闭') : null,
          ]),
        ])
      ])
    }
  })

  // 兼容：保留 __dm_uiBridge 注册
  if (window.__dm_uiBridge) {
    window.__dm_uiBridge.register('hanime', {
      label: '播放',
      open: function (obj) {
        // 通过 TaskUI 获取查看器并渲染到模态框容器
        var handler = TaskUI.get('hanime')
        if (handler && handler.renderViewer) {
          var container = document.getElementById('custom-ui-content')
          if (container) container.innerHTML = ''
          // 创建临时 Vue 应用渲染查看器
          var vm = Vue.createApp({
            render: function(h) {
              return handler.renderViewer(h, obj, function() {
                if (container) container.innerHTML = ''
                vm.unmount()
              })
            }
          }).mount(container)
        }
      }
    })
  }
})()
```

- [ ] **步骤 2：验证文件语法正确**

运行：`node --check web/static/tasks/hanime/ui.js` （预期：无输出）

---

### 任务 8：创建 vikacg 类型 UI 组件

**文件：** 创建 `web/static/tasks/vikacg/ui.js`

- [ ] **步骤 1：编写 ui.js**

```javascript
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

;(function () {
  'use strict'

  function getImages(obj) {
    var imgs = []
    if (obj && obj.status === 'completed' && obj.extra && Array.isArray(obj.extra.files)) {
      obj.extra.files.forEach(function (f) {
        if (f.type === 'image' && f.path) imgs.push(fileUrl(f.path))
      })
    }
    if (imgs.length === 0 && obj && obj.extra && Array.isArray(obj.extra.images)) {
      obj.extra.images.forEach(function (u) { if (typeof u === 'string' && u) imgs.push(u) })
    }
    return imgs
  }

  function getLinks(obj) {
    var links = []
    var base = (obj && obj.metadata && obj.metadata.page_url) || ''
    if (obj && obj.extra && Array.isArray(obj.extra.links)) {
      obj.extra.links.forEach(function (l) {
        var href = (l && l.href) || ''
        var text = (l && l.text) || href
        if (!href) return
        var abs = href
        try { abs = new URL(href, base).toString() } catch (e) {}
        links.push({ text: text, href: abs })
      })
    }
    return links
  }

  function getExcerpt(obj) {
    var s = (obj && obj.extra && obj.extra.content_text) || ''
    if (!s) return ''
    var t = s.replace(/\s+/g, ' ').trim()
    return t.length > 200 ? t.slice(0, 200) + '...' : t
  }

  function getContentHtml(obj) {
    var s = (obj && obj.extra && obj.extra.content_html) || ''
    return typeof s === 'string' ? s.trim() : ''
  }

  function getTags(obj) {
    if (obj && obj.extra && Array.isArray(obj.extra.tags)) return obj.extra.tags
    if (obj && obj.extra && typeof obj.extra.tags === 'string') return [obj.extra.tags]
    return []
  }

  function fileUrl(path) {
    if (!path) return ''
    var normalized = path.replace(/\\/g, '/')
    var root = typeof window.__dm_downloadRoot === 'string' ? window.__dm_downloadRoot : ''
    if (root && normalized.indexOf(root) === 0) { normalized = normalized.slice(root.length) }
    normalized = normalized.replace(/^\//, '')
    return '/files/' + normalized.split('/').filter(function(s){return s&&s!=='..'}).map(encodeURIComponent).join('/')
  }

  function getTitle(obj) { return (obj && obj.metadata && obj.metadata.title) || '' }
  function getDate(obj) { return (obj && obj.metadata && obj.metadata.date) || '' }

  // ---- 注册 vikacg UI ----
  TaskUI.register('vikacg', {
    type: 'vikacg',
    label: 'VikACG',
    icon: 'fa-image',
    viewerLabel: '浏览',
    shouldShowViewer: function (obj) {
      return obj.status === 'completed' && obj.extra && (Array.isArray(obj.extra.images) || Array.isArray(obj.extra.files))
    },
    renderViewer: function (h, obj, onClose) {
      var images = getImages(obj)
      var links = getLinks(obj)
      var title = getTitle(obj) || 'VikACG'
      var dateVal = getDate(obj)
      var tags = getTags(obj)
      var html = getContentHtml(obj)
      var excerpt = getExcerpt(obj)
      var section = obj && obj.metadata && obj.metadata.section
      var pageUrl = obj && obj.metadata && obj.metadata.page_url

      // 状态管理
      var currentIdx = 0

      return h('div', {
        class: 'fixed inset-0 bg-black bg-opacity-70 z-50 flex items-center justify-center p-4 backdrop-blur-sm',
        on: {
          click: function (e) { if (e.target === e.currentTarget && onClose) onClose() },
          keydown: function (e) { if (e.key === 'Escape' && onClose) onClose() }
        }
      }, [
        h('div', { class: 'bg-white rounded-lg shadow-2xl w-full max-w-5xl max-h-[90vh] overflow-hidden flex flex-col' }, [
          // Header
          h('div', { class: 'p-4 border-b flex justify-between items-center bg-gray-50' }, [
            h('div', { class: 'flex items-center gap-3' }, [
              h('h3', { class: 'text-lg font-bold text-gray-800' }, title),
              section ? h('span', { class: 'px-2 py-0.5 text-xs rounded bg-blue-50 text-blue-600' }, section) : null,
              dateVal ? h('span', { class: 'text-xs text-gray-500' }, dateVal) : null,
            ]),
            onClose ? h('button', { class: 'text-gray-500 hover:text-gray-700', on: { click: onClose } }, [h('i', { class: 'fas fa-times' })]) : null,
          ]),
          // Body
          h('div', { class: 'flex-1 overflow-y-auto' }, [
            images.length > 0 ? h('div', { class: 'bg-gray-100' }, [
              // Main image (using v-for simulation - simple approach)
              h('div', { class: 'aspect-[16/9] bg-black flex items-center justify-center' }, [
                h('img', {
                  ref: 'mainImage',
                  attrs: { src: images[0] },
                  class: 'w-full h-full object-contain',
                  key: 'main-img'
                })
              ]),
              // Navigation
              images.length > 1 ? h('div', { class: 'p-2 flex items-center justify-between' }, [
                h('div', { class: 'text-xs text-gray-500' }, '1 / ' + images.length),
                h('div', { class: 'flex gap-2' }, [
                  h('button', {
                    class: 'px-2 py-1 text-xs bg-white border rounded hover:bg-gray-50',
                    on: { click: function () { currentIdx = (currentIdx - 1 + images.length) % images.length; /* update image */ } }
                  }, '上一张'),
                  h('button', {
                    class: 'px-2 py-1 text-xs bg-white border rounded hover:bg-gray-50',
                    on: { click: function () { currentIdx = (currentIdx + 1) % images.length; /* update image */ } }
                  }, '下一张'),
                ])
              ]) : null,
              // Thumbnails
              images.length > 1 ? h('div', { class: 'p-2 grid grid-cols-6 gap-2' }, images.map(function (img, idx) {
                return h('img', {
                  attrs: { src: img },
                  class: 'w-full h-16 object-cover rounded cursor-pointer border',
                  class: 'border-' + (idx === 0 ? 'blue-500' : 'gray-200'),
                  on: { click: function () { currentIdx = idx; /* update image */ } }
                })
              })) : null,
            ]) : null,
            // Content
            h('div', { class: 'p-4 space-y-3' }, [
              // HTML or excerpt
              html ? h('div', { class: 'text-sm text-gray-700 prose max-w-none' }, [h('div', { attrs: { innerHTML: html } })]) :
              excerpt ? h('div', { class: 'text-sm text-gray-700 whitespace-pre-line' }, excerpt) : null,
              // Tags
              tags.length ? h('div', [
                h('div', { class: 'text-xs text-gray-500 mb-1' }, '标签'),
                h('div', { class: 'flex flex-wrap gap-1' }, tags.map(function (tag) {
                  return h('span', { class: 'text-xs bg-gray-100 text-gray-600 px-2 py-0.5 rounded' }, '#' + tag)
                }))
              ]) : null,
              // Links
              links.length ? h('div', [
                h('div', { class: 'text-xs text-gray-500 mb-1' }, '相关链接'),
                h('div', { class: 'flex flex-col gap-1' }, links.map(function (l) {
                  return h('a', {
                    attrs: { href: /^https?:\/\//i.test(l.href) ? l.href : '#', target: '_blank', rel: 'noopener noreferrer' },
                    class: 'text-xs text-blue-600 hover:text-blue-800 break-all'
                  }, l.text || l.href)
                }))
              ]) : null,
            ]),
          ]),
          // Footer
          h('div', { class: 'p-3 border-t bg-gray-50 flex justify-between items-center' }, [
            h('div', { class: 'flex gap-2' }, [
              pageUrl ? h('a', {
                attrs: { href: /^https?:\/\//i.test(pageUrl) ? pageUrl : '#', target: '_blank', rel: 'noopener noreferrer' },
                class: 'px-3 py-1.5 rounded bg-blue-600 text-white text-sm hover:bg-blue-700'
              }, '打开原页面') : null,
              pageUrl ? h('button', {
                class: 'px-3 py-1.5 rounded bg-white border text-sm hover:bg-gray-100',
                on: { click: function () { if (navigator.clipboard && navigator.clipboard.writeText) navigator.clipboard.writeText(pageUrl) } }
              }, '复制链接') : null,
            ]),
            onClose ? h('button', {
              class: 'px-3 py-1.5 rounded bg-white border text-sm hover:bg-gray-100',
              on: { click: onClose }
            }, '关闭') : null,
          ]),
        ])
      ])
    }
  })

  // 兼容：保留 __dm_uiBridge 注册
  if (window.__dm_uiBridge) {
    window.__dm_uiBridge.register('vikacg', {
      label: '浏览',
      open: function (obj) {
        var handler = TaskUI.get('vikacg')
        if (handler && handler.renderViewer) {
          var container = document.getElementById('custom-ui-content')
          if (container) container.innerHTML = ''
          var vm = Vue.createApp({
            render: function(h) {
              return handler.renderViewer(h, obj, function() {
                if (container) container.innerHTML = ''
                vm.unmount()
              })
            }
          }).mount(container)
        }
      }
    })
  }
})()
```

- [ ] **步骤 2：验证文件语法正确**

运行：`node --check web/static/tasks/vikacg/ui.js` （预期：无输出）

---

### 任务 9：修改 `index.html` 中的新建任务表单

**文件：** 修改 `web/static/index.html` 第 1524-1615 行

- [ ] **步骤 1：替换新建任务表单中的条件分支**

将原第 1577-1607 行（`v-if="newTask.type === 'url_list'"` 和 `v-if="newTask.type === 'tktube'"` 条件渲染）替换为动态组件：

原代码（第 1576-1607 行）：
```html
                        <!-- Simple URL Fields -->
                        <div v-if="newTask.type === 'url_list'">
                            <label class="block text-sm font-medium text-gray-700 mb-1">URL 列表（每行一个）</label>
                            <textarea v-model="newTask.urls_text" ...></textarea>
                        </div>

                        <!-- TKTube Fields -->
                        <div v-if="newTask.type === 'tktube'" class="space-y-4 border-t pt-4 mt-2">
                            ...
                        </div>
```

替换为：
```html
                        <!-- Task Type Specific Fields -->
                        <div v-if="showTaskTypeFormFields" class="space-y-4 border-t pt-4 mt-2">
                            <component :is="taskTypeFormComponent" />
                        </div>
```

同时在 `main.js` 中新增 `showTaskTypeFormFields` 计算属性和 `taskTypeFormComponent` 计算属性。

- [ ] **步骤 2：修改 index.html 新建任务表单区域**

具体修改：在 `index.html` 第 1576 行处，移除 `v-if="newTask.type === 'url_list'"` 和 `v-if="newTask.type === 'tktube'"` 的整块内容，替换为动态组件：

```html
                        <!-- Task Type Specific Fields -->
                        <div v-if="showTaskTypeFormFields" class="space-y-4 border-t pt-4 mt-2">
                            <component :is="taskTypeFormComponent" />
                        </div>
```

- [ ] **步骤 3：修改 main.js 中的 `saveNewTask` 方法**

原 `saveNewTask` 方法（第 304-340 行）中，第 319-327 行包含 task 类型特定的 payload 构建逻辑。替换为通过 TaskUI 获取类型特定的字段收集逻辑：

```javascript
saveNewTask: function () {
  var payload = {
    id: this.newTask.id,
    type: this.newTask.type,
    save_dir: this.newTask.save_dir,
    storage: { type: this.newTask.storage_type }
  }
  if (this.newTask.storage_type === 'file' && this.newTask.storage_config.path) {
    payload.storage.path = this.newTask.storage_config.path
  }
  if (this.newTask.storage_type === 'mongo') {
    if (this.newTask.storage_config.source) payload.storage.source = this.newTask.storage_config.source
    if (this.newTask.storage_config.database) payload.storage.database = this.newTask.storage_config.database
    if (this.newTask.storage_config.collection) payload.storage.collection = this.newTask.storage_config.collection
  }
  // 通过 TaskUI 收集类型特定字段（由 task 的 defineForm 处理）
  // 字段已绑定到 this.newTask，payload 中直接使用
  if (this.newTask.type === 'url_list') {
    payload.urls_text = this.newTask.urls_text
  }
  if (this.newTask.type === 'tktube') {
    if (this.newTask.keyword) payload.keyword = this.newTask.keyword
    if (this.newTask.subtype) payload.subtype = this.newTask.subtype
    if (this.newTask.max_concurrent) payload.max_concurrent = this.newTask.max_concurrent
    if (this.newTask.refresh_interval) payload.refresh_interval = this.newTask.refresh_interval
  }
  // ... 后续保持不变
}
```

注意：此步骤保留现有 payload 构建逻辑（因为 `saveNewTask` 需要序列化 `newTask` 数据到 API 请求，而 `renderForm` 已通过双向绑定将数据写入 `newTask`）。后续可进一步抽象，但当前保持兼容。

---

### 任务 10：修改 `main.js` — 新增 TaskUI 集成方法

**文件：** 修改 `web/static/app/main.js`

- [ ] **步骤 1：新增计算属性和方法**

在 `main.js` 的 `computed` 中新增：

```javascript
// 是否显示 task 类型特定表单字段
showTaskTypeFormFields: function () {
  var handler = TaskUI.get(this.newTask.type)
  return handler && handler.renderForm !== null
},
// Task 类型特定的表单组件
taskTypeFormComponent: function () {
  var handler = TaskUI.get(this.newTask.type)
  if (handler && handler.renderForm) {
    var self = this
    return {
      render: function (h) {
        return handler.renderForm(h, self.newTask, {})
      }
    }
  }
  return null
},
// 是否显示 task 类型特定元数据
showTaskTypeMeta: function () {
  if (!this.selectedTask || !this.selectedTask.extra) return false
  var handler = TaskUI.get(this.selectedTask.type)
  return handler && handler.renderMeta !== null
},
// Task 类型特定的元数据组件
taskTypeMetaComponent: function () {
  var handler = TaskUI.get(this.selectedTask.type)
  if (handler && handler.renderMeta) {
    var task = this.selectedTask
    return {
      render: function (h) {
        return handler.renderMeta(h, task)
      }
    }
  }
  return null
},
// 是否显示 task 类型特定查看器
showTaskTypeViewer: function (obj) {
  if (!obj) return false
  var type = obj.metadata && obj.metadata.task_type
  var handler = TaskUI.get(type)
  return handler && handler.renderViewer !== null && handler.shouldShowViewer(obj)
},
// 查看器按钮文字
taskTypeViewerLabel: function (obj) {
  var type = obj && obj.metadata && obj.metadata.task_type
  var handler = TaskUI.get(type)
  return (handler && handler.viewerLabel) || '查看'
},
```

在 `methods` 中新增：

```javascript
// 打开 task 类型特定查看器
openTaskTypeViewer: function (obj) {
  var type = obj && obj.metadata && obj.metadata.task_type
  var handler = TaskUI.get(type)
  if (handler && handler.renderViewer) {
    var self = this
    // 创建临时挂载点
    var container = document.createElement('div')
    document.body.appendChild(container)
    var vm = Vue.createApp({
      render: function (h) {
        return handler.renderViewer(h, obj, function () {
          vm.unmount()
          if (container.parentNode) container.parentNode.removeChild(container)
        })
      }
    }).mount(container)
  }
},
```

- [ ] **步骤 2：替换 `loadCustomUIFeatures` 等旧方法**

将 `loadCustomUIFeatures`、`loadTaskUI`、`renderCustomTaskView` 方法替换为：

```javascript
// 加载 task 类型 UI 组件
loadTaskUIForType: function (taskType) {
  var self = this
  TaskUI.loadTaskUI(taskType, function () {
    // 触发 Vue 重新渲染
    self.$forceUpdate()
  })
},
```

---

### 任务 11：修改 `index.html` 中的任务元数据区域

**文件：** 修改 `web/static/index.html` 第 708-722 行

- [ ] **步骤 1：替换任务元数据中的条件分支**

原代码（第 708-722 行）：
```html
<div class="md:col-span-2" v-if="selectedTask.extra">
  <div class="text-gray-500 text-xs">任务扩展</div>
  <div class="grid grid-cols-1 md:grid-cols-3 gap-2 text-xs text-gray-600" v-if="selectedTask.type.startsWith('tktube')">
    ...tktube特定字段...
  </div>
  <div class="text-xs text-gray-600" v-if="selectedTask.type==='url_list'">
    ...url_list特定字段...
  </div>
  <div class="text-xs text-gray-600 break-all" v-if="selectedTask.type !== 'url_list' && !selectedTask.type.startsWith('tktube')">
    {{ JSON.stringify(selectedTask.extra, null, 2) }}
  </div>
</div>
```

替换为：
```html
<div class="md:col-span-2" v-if="selectedTask.extra">
  <div class="text-gray-500 text-xs">任务扩展</div>
  <div v-if="showTaskTypeMeta">
    <component :is="taskTypeMetaComponent" />
  </div>
  <div v-else class="text-xs text-gray-600 break-all">
    {{ JSON.stringify(selectedTask.extra, null, 2) }}
  </div>
</div>
```

---

### 任务 12：修改 `index.html` 中的对象卡片额外内容

**文件：** 修改 `web/static/index.html`

- [ ] **步骤 1：替换卡片中的 `v-if="isVikacg(obj)"` 和 `v-if="isCustomUI(obj)"`**

原代码（第 781 行）：
```html
<div v-if="isVikacg(obj)" class="text-xs text-gray-500 line-clamp-3 mb-2">{{ getVikacgExcerpt(obj) }}</div>
```

替换为：
```html
<div v-if="showTaskTypeCardExtra(obj)" class="mb-2">
  <component :is="taskTypeCardExtraComponent(obj)" />
</div>
```

原代码（第 809 行，第 874 行）：
```html
<button v-if="isCustomUI(obj) && obj.status === 'completed'" @click.stop="openCustomUI(obj)" class="...">
  <i class="fas fa-book-open mr-1"></i> {{ getCustomUILabel(obj) }}
</button>
```

替换为：
```html
<button v-if="showTaskTypeViewer(obj) && obj.status === 'completed'" @click.stop="openTaskTypeViewer(obj)" class="text-xs px-2 py-1 rounded bg-white text-green-600 hover:bg-green-50 transition">
  <i class="fas fa-book-open mr-1"></i> {{ taskTypeViewerLabel(obj) }}
</button>
```

---

### 任务 13：替换查看器模态框

**文件：** 修改 `web/static/index.html`

- [ ] **步骤 1：移除 vikacg/hanime/customUI 模态框**

删除以下块：
- `showVikacgModal` 模态框（第 1122-1173 行）
- `showHanimeModal` 模态框（第 1175-1234 行）
- `showCustomUIModal` 模态框（第 1236-1246 行）

- [ ] **步骤 2：移除 `showCustomTaskView` 条件**

删除 `showCustomTaskView` 相关条件（第 727 行）。

- [ ] **步骤 3：在 `main.js` 中移除相关数据和方法**

从 `main.js` 的 `data` 中移除：
- `showVikacgModal`、`vikacgModalObj`、`vikacgActiveImgIdx`
- `showHanimeModal`、`hanimeModalObj`、`hanimeVideoError`
- `showCustomUIModal`、`customUITitle`、`customUIData`
- `showCustomTaskView`
- `_registeredUITypes`

从 `methods` 中移除：
- `closeVikacg`、`closeHanime`、`closeCustomUI`
- `renderPluginCards`

---

### 任务 14：简化 `helpers.js`

**文件：** 修改 `web/static/app/helpers.js`

- [ ] **步骤 1：移除 `isVikacg`、`getVikacgExcerpt` 方法**

删除 `helpers.js` 第 173-183 行：
```javascript
isVikacg: function (obj) { ... },
getVikacgExcerpt: function (obj) { ... },
```

- [ ] **步骤 2：移除 `isCustomUI`、`getCustomUILabel`、`openCustomUI`、`renderPluginCards` 方法**

删除 `helpers.js` 第 457-495 行：
```javascript
isCustomUI: function (obj) { ... },
getCustomUILabel: function (obj) { ... },
openCustomUI: function (obj) { ... },
renderPluginCards: function () { ... },
```

- [ ] **步骤 3：修改 `handleCardClick` 方法**

原 `handleCardClick`（第 364-377 行）：
```javascript
handleCardClick: function (obj) {
  if (!obj) return
  if (obj.status === 'completed') {
    var type = obj.metadata && obj.metadata.task_type
    if (type && window.__dm_uiBridge && window.__dm_uiBridge.hasPlugin(type)) {
      window.__dm_uiBridge.open(type, obj)
      return
    }
    if (this.isVideo(obj)) {
      this.playVideo(obj)
    }
  }
},
```

替换为：
```javascript
handleCardClick: function (obj) {
  if (!obj) return
  if (obj.status === 'completed') {
    var type = obj.metadata && obj.metadata.task_type
    if (type && TaskUI.hasViewer(type)) {
      this.openTaskTypeViewer(obj)
      return
    }
    if (this.isVideo(obj)) {
      this.playVideo(obj)
    }
  }
},
```

- [ ] **步骤 4：修改 `index.html` 中的 `isVikacg` 引用**

在 `index.html` 中，搜索 `!isVikacg(obj)` 模式（第 418、482、808、873 行）：
```html
<a v-if="obj.status === 'completed' && !isVideo(obj) && !isVikacg(obj)" ...>
```

替换为：
```html
<a v-if="obj.status === 'completed' && !isVideo(obj) && !showTaskTypeViewer(obj)" ...>
```

---

### 任务 15：扩展后端 `core/taskui.go` — 增加 `HasForm` 等字段

**文件：** 修改 `core/taskui.go`

- [ ] **步骤 1：扩展 `TaskUIAssets` 结构体**

```go
type TaskUIAssets struct {
	FS       embed.FS // embedded filesystem containing the assets
	JSPaths  []string // file paths relative to FS root (e.g. ["reader.js"])
	CSSPaths []string // CSS file paths relative to FS root
	Label    string   // button label shown in the UI, e.g. "阅读"

	// 新增字段
	HasForm      bool   // 此类型是否支持在 UI 中新建
	HasViewer    bool   // 此类型是否有查看器
	HasAggregate bool   // 此类型是否有聚合视图
}
```

- [ ] **步骤 2：运行 `go build ./...` 验证编译通过**

运行：`cd D:/workdir/leon/cocomhub/download-manager && go build ./...` （预期：无错误）

---

### 任务 16：扩展后端 `api/ui.go` — 增强 UI 配置响应

**文件：** 修改 `api/ui.go`

- [ ] **步骤 1：修改 `serveUIConfig` 响应**

在 `serveUIConfig` 中增加 `has_form`、`has_viewer`、`has_aggregate` 字段：

```go
func (s *Server) serveUIConfig(w http.ResponseWriter, r *http.Request) {
	taskType := mux.Vars(r)["type"]
	assets, ok := core.GetTaskUI(taskType)
	if !ok {
		writeJSONError(w, http.StatusNotFound, "not_found", "no UI assets for type: "+taskType)
		return
	}
	w.Header().Set(hdrContentType, "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"js":            assets.JSPaths,
		"css":           assets.CSSPaths,
		"label":         assets.Label,
		"has_form":      assets.HasForm,
		"has_viewer":    assets.HasViewer,
		"has_aggregate": assets.HasAggregate,
	})
}
```

- [ ] **步骤 2：运行 `go build ./...` 验证编译通过**

---

### 任务 17：更新各 task 类型的 `ui/ui.go` 注册

**文件：** 修改 `task/urllist/ui/ui.go`、`task/tktube/ui/ui.go`、`task/hanime/ui/ui.go`、`task/vikacg/ui/ui.go`

- [ ] **步骤 1：更新 `task/urllist/ui/ui.go`**

```go
func init() {
	core.RegisterTaskUI("url_list", core.TaskUIAssets{
		FS:      assets,
		JSPaths: []string{"assets/viewer.js"},
		Label:   "URL 列表",
		HasForm: true,
	})
}
```

- [ ] **步骤 2：更新 `task/tktube/ui/ui.go`**

```go
func init() {
	core.RegisterTaskUI("tktube", core.TaskUIAssets{
		FS:           assets,
		JSPaths:      []string{"assets/viewer.js"},
		Label:        "",
		HasForm:      true,
		HasViewer:    true,
		HasAggregate: true,
	})
}
```

- [ ] **步骤 3：更新 `task/hanime/ui/ui.go`**

```go
func init() {
	core.RegisterTaskUI("hanime", core.TaskUIAssets{
		FS:        assets,
		JSPaths:   []string{"assets/viewer.js"},
		Label:     "播放",
		HasViewer: true,
	})
}
```

- [ ] **步骤 4：更新 `task/vikacg/ui/ui.go`**

```go
func init() {
	core.RegisterTaskUI("vikacg", core.TaskUIAssets{
		FS:        assets,
		JSPaths:   []string{"assets/viewer.js"},
		Label:     "浏览",
		HasViewer: true,
	})
}
```

- [ ] **步骤 5：运行 `go build ./...` 验证编译通过**

运行：`cd D:/workdir/leon/cocomhub/download-manager && go build ./...` （预期：无错误）

---

### 任务 18：添加 `index.html` 中所需 script 加载

**文件：** 修改 `web/static/index.html`

- [ ] **步骤 1：在 `index.html` 的 `</body>` 前添加 taskui 框架加载**

找到 `index.html` 末尾（第 1617 行之后）的 `<script>` 加载顺序。在 `main.js` 之前添加 taskui 框架的加载：

```html
    <script src="app/taskui/registry.js"></script>
    <script src="app/taskui/defineForm.js"></script>
    <script src="app/taskui/defineMeta.js"></script>
    <script src="app/taskui/baseForm.js"></script>
    <script src="app/taskui/baseMeta.js"></script>
    <script src="app/taskui/baseViewer.js"></script>
    <script src="app/taskui/loader.js"></script>
```

---

### 任务 19：验证

- [ ] **步骤 1：运行 `go build ./...` 确认后端编译通过**

- [ ] **步骤 2：运行 Playwright E2E 测试验证 UI 功能不变**

```bash
cd D:/workdir/leon/cocomhub/download-manager
make playwright-test
```

- [ ] **步骤 3：手动检查 `index.html` 确认不再有 `type.startsWith('tktube')` 等条件分支**

```bash
grep -n "type\.startsWith\|type === 'url_list'\|isVikacg\|isCustomUI\|showVikacgModal\|showHanimeModal\|showCustomUIModal" web/static/index.html
```

预期：无输出（或仅剩 `selectedType === 'all'` 等通用判断）

- [ ] **步骤 4：运行 `go test ./...` 确认功能测试通过**

```bash
cd D:/workdir/leon/cocomhub/download-manager
go test -race -count=1 -timeout=180s ./...
```