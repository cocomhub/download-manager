# Phase 4: Web UI 打磨 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 在 Phase 3 可观测性 API 的基础上，增强 Web UI：① 运维仪表盘（健康/指标/失败记录）、② 批量操作增强（跨页全选/批量重试/选中计数）、③ 修复 4 个现有 UI bug。

**架构：** 纯前端变更，无后端改动。新增 `web/static/app/dashboard.js` 模块（IIFE 模式），修改 `index.html`/`main.js`/`taskList.js`/`helpers.js`/`api.js`。仪表盘通过轮询 Phase 3 已实现的 `/api/healthz`、`/api/metrics`、`/api/metrics/failures` 端点获取数据。

**技术栈：** Vue 3 (CDN)、Tailwind CSS (CDN)、Font Awesome 6 (CDN)、原生 JS IIFE 模块模式。

**分支：** 从 `dev` 创建 `feature/phase4-ui-polish` 分支

---
**执行前先建分支：**
```bash
git checkout -b feature/phase4-ui-polish
```

---

## 文件变更清单

| 文件 | 操作 | 职责 |
|------|------|------|
| `web/static/app/dashboard.js` | **创建** | 仪表盘方法模块：fetchHealthz、fetchMetrics、fetchFailures、轮询定时器管理 |
| `web/static/app/api.js` | 修改 | 新增 `healthz()`、`metrics()`、`failures()` 三个 API 方法 |
| `web/static/index.html` | 修改 | Sidebar 仪表盘按钮、仪表盘三段式模板、批量操作栏增强、4 个 bug 修复 |
| `web/static/app/main.js` | 修改 | 新增 `viewMode='dashboard'`、仪表盘 data 字段、轮询定时器启动/停止、注册 dashboard 模块 |
| `web/static/app/taskList.js` | 修改 | 增强 `toggleSelectAllObjects`、新增 `retrySelectedObjects`、`cancelSelectAllObjects`、`selectAllScope` 相关逻辑 |
| `web/static/app/helpers.js` | 修改 | B1 修复：SSE `onopen` 回调中调用 `fetchTasks()` |

---

### 任务 1：Bug 修复（B1-B4）

**涉及文件：**
- 修改：`web/static/app/helpers.js:336-349`
- 修改：`web/static/index.html:399,618`（grid 视图进度条条件）
- 修改：`web/static/index.html:181-184`（分页空状态）
- 修改：`web/static/index.html:611`（progress 百分比显示）

- [ ] **步骤 1：B1 — SSE onopen 重连刷新**

在 `web/static/app/helpers.js` 的 `initSSE` 方法中，在 `eventSource.onerror` 之后添加 `eventSource.onopen` 回调：

```javascript
self.eventSource.onopen = function () {
  // SSE 重连成功后刷新任务列表，解决断连后数据陈旧的问题
  self.fetchTasks()
}
```

紧接在 `self.eventSource.onerror` 块之后（helpers.js:349）。注意 `onopen` 在首次连接时也会触发——`fetchTasks` 本身有去重保护（`loading` 标志和 `selectedTaskId` 判断），首次连接时多调一次无副作用。

- [ ] **步骤 2：B2 — cancelled 对象不显示进度条**

在 `web/static/index.html` 中，找到以下两处进度条显示条件，增加 `obj.status !== 'cancelled'` 判断：

**Grid 视图**（index.html:399）：
```html
<div v-if="obj.status === 'downloading' || (obj.status !== 'completed' && obj.status !== 'cancelled' && obj.progress > 0)" class="w-full bg-gray-100 rounded-full h-2 mb-1">
```

**Grid 视图（任务详情内）**（index.html:618）：
```html
<div v-if="obj.status === 'downloading' || (obj.status !== 'completed' && obj.status !== 'cancelled' && obj.progress > 0)" class="w-full bg-gray-100 rounded-full h-2 mb-1">
```

- [ ] **步骤 3：B3 — pagination total=0 时隐藏分页**

在 `web/static/index.html` 中，找到分页容器（约第 177 行）：

```html
<div class="flex items-center space-x-1" v-if="pagination.limit !== 'all'">
```

修改为：

```html
<div class="flex items-center space-x-1" v-if="pagination.limit !== 'all' && pagination.total > 0">
```

同时 tktube 聚合视图的分页控件（约第 344 行）也做同样修改：

```html
<div class="flex items-center space-x-1" v-if="tktubePagination.limit !== 'all' && tktubePagination.total > 0">
```

- [ ] **步骤 4：B4 — 优化 progress 百分比显示条件**

在 `web/static/index.html` 中，找到 grid 视图卡片的 progress 百分比文字（约第 611 行）：

```html
<span class="font-mono">{{ obj.progress ? (obj.progress + '%') : '' }}</span>
```

修改为仅在 `downloading` 状态或 `progress > 0 && progress < 100` 时显示：

```html
<span class="font-mono" v-if="obj.status === 'downloading' || (obj.progress > 0 && obj.progress < 100)">{{ obj.progress + '%' }}</span>
```

同时找到 list 视图对应的进度百分比列（约第 461-464 行），同样处理。list 视图目前只显示进度条，没有百分比文字——不需要改。

- [ ] **步骤 5：验证构建通过**

```bash
go build ./...
```

- [ ] **步骤 6：Commit**

```bash
git add web/static/index.html web/static/app/helpers.js
git commit -m "fix(web): Phase 4 - fix 4 UI bugs (SSE reconnect, cancelled progress, pagination empty, progress display)"
```

---

### 任务 2：Dashboard API 封装 + 模块创建

**涉及文件：**
- 创建：`web/static/app/dashboard.js`
- 修改：`web/static/app/api.js`

- [ ] **步骤 1：在 api.js 中新增仪表盘 API 方法**

在 `web/static/app/api.js` 的 `api` 对象末尾（`post` 方法之后）添加：

```javascript
healthz: function () {
  return fetch('/api/healthz').then(function (r) {
    if (!r.ok) throw new Error('Health check failed')
    return r.json()
  })
},

metrics: function () {
  return fetch('/api/metrics').then(function (r) {
    if (!r.ok) throw new Error('Metrics fetch failed')
    return r.json()
  })
},

failures: function (params) {
  var q = new URLSearchParams()
  if (params && params.limit) q.set('limit', params.limit)
  if (params && params.task_id) q.set('task_id', params.task_id)
  return fetch('/api/metrics/failures?' + q.toString()).then(function (r) {
    if (!r.ok) throw new Error('Failures fetch failed')
    return r.json()
  })
}
```

- [ ] **步骤 2：创建 dashboard.js 模块**

创建 `web/static/app/dashboard.js`，遵循现有 IIFE 模式：

```javascript
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * Dashboard methods — health check, metrics, failure records with polling.
 * Registered as Vue methods on the app.
 * Depends on: AppAPI
 */
;(function () {
  'use strict'

  window.AppDashboard = {
    register: function (app) {
      app.methods = Object.assign(app.methods || {}, {
        // --- Dashboard data fetching ---

        fetchDashboardData: function () {
          this.fetchHealthz()
          this.fetchMetrics()
          this.fetchFailures()
        },

        fetchHealthz: function () {
          var self = this
          AppAPI.healthz().then(function (data) {
            self.dashboardHealth = data
          }).catch(function (e) {
            console.error('Dashboard healthz error:', e)
            self.dashboardHealth = { status: 'error', components: {} }
          })
        },

        fetchMetrics: function () {
          var self = this
          AppAPI.metrics().then(function (data) {
            self.dashboardMetrics = data
          }).catch(function (e) {
            console.error('Dashboard metrics error:', e)
          })
        },

        fetchFailures: function () {
          var self = this
          var limit = self.dashboardFailuresLimit || 20
          var taskId = self.dashboardFailuresTaskId || ''
          AppAPI.failures({ limit: limit, task_id: taskId }).then(function (data) {
            self.dashboardFailures = data
          }).catch(function (e) {
            console.error('Dashboard failures error:', e)
          })
        },

        // --- Polling timer management ---

        startDashboardPolling: function () {
          var self = this
          this.stopDashboardPolling() // clear any existing timers

          // Healthz: 5s interval
          this.dashboardHealthzTimer = setInterval(function () {
            self.fetchHealthz()
          }, 5000)

          // Metrics: 10s interval
          this.dashboardMetricsTimer = setInterval(function () {
            self.fetchMetrics()
          }, 10000)

          // Failures: 15s interval
          this.dashboardFailuresTimer = setInterval(function () {
            self.fetchFailures()
          }, 15000)
        },

        stopDashboardPolling: function () {
          if (this.dashboardHealthzTimer) {
            clearInterval(this.dashboardHealthzTimer)
            this.dashboardHealthzTimer = null
          }
          if (this.dashboardMetricsTimer) {
            clearInterval(this.dashboardMetricsTimer)
            this.dashboardMetricsTimer = null
          }
          if (this.dashboardFailuresTimer) {
            clearInterval(this.dashboardFailuresTimer)
            this.dashboardFailuresTimer = null
          }
        },

        // --- Dashboard failure filter ---

        changeDashboardFailuresLimit: function () {
          this.fetchFailures()
        },

        searchDashboardFailures: function () {
          this.fetchFailures()
        }
      })
    }
  }
})()
```

- [ ] **步骤 3：在 index.html 中加载 dashboard.js**

在 `web/static/index.html` 的 `<head>` 中，在 `taskList.js` 之后、`main.js` 之前添加：

```html
<script src="app/dashboard.js"></script>
```

加载顺序：`api.js` → `videoPlayer.js` → `helpers.js` → `taskList.js` → **`dashboard.js`** → `main.js`

- [ ] **步骤 4：验证构建**

```bash
go build ./...
```

- [ ] **步骤 5：Commit**

```bash
git add web/static/app/api.js web/static/app/dashboard.js web/static/index.html
git commit -m "feat(web): Phase 4 - add dashboard API methods and module"
```

---

### 任务 3：仪表盘视图 Sidebar 按钮 + 模板 + 主 App 集成

**涉及文件：**
- 修改：`web/static/index.html` — Sidebar 按钮 + 仪表盘三段式模板
- 修改：`web/static/app/main.js` — data 字段 + viewMode 切换 + 轮询生命周期

- [ ] **步骤 1：在 Sidebar 中添加仪表盘按钮**

在 `web/static/index.html` 的 Sidebar 中（约第 98~103 行，在"聚合视图"和"新建任务"按钮之间），添加仪表盘按钮：

```html
<button @click="viewMode = 'dashboard'" class="w-full py-1.5 rounded shadow-sm text-sm font-medium transition flex items-center justify-center gap-2" :class="viewMode === 'dashboard' ? 'bg-green-500 text-white hover:bg-green-600' : 'bg-gray-100 hover:bg-gray-200 text-gray-700'">
  <i class="fas fa-chart-bar"></i> 仪表盘
</button>
```

按钮放在"聚合视图"按钮之后（不要覆盖现有按钮）。

- [ ] **步骤 2：在 main.js 中添加 dashboard 相关 data 字段**

在 `web/static/app/main.js` 的 `data` 对象中（`hoverObj` 之后、`enablePreview` 之前或之后），添加：

```javascript
// Dashboard
dashboardHealth: null,
dashboardMetrics: null,
dashboardFailures: null,
dashboardFailuresLimit: 20,
dashboardFailuresTaskId: '',
dashboardHealthzTimer: null,
dashboardMetricsTimer: null,
dashboardFailuresTimer: null,
```

- [ ] **步骤 3：在 main.js 中注册 dashboard 模块**

在 `web/static/app/main.js` 的模块注册区（约第 209~213 行），添加：

```javascript
if (typeof AppDashboard !== 'undefined') AppDashboard.register(app)
```

放在 `AppDownloadView` 注册之后。

- [ ] **步骤 4：在 index.html 中添加仪表盘视图模板**

在 `web/static/index.html` 的主内容区域，在 tktube 聚合视图结束之后、任务详情视图开始之前（约第 479 行），添加仪表盘模板：

```html
<!-- Dashboard View -->
<div v-if="!isLoadingTask && viewMode === 'dashboard'" class="space-y-6">
  <h2 class="text-xl font-bold text-gray-800 mb-2">📊 仪表盘</h2>

  <!-- 第一段：健康状态摘要条 -->
  <div class="bg-white rounded-xl shadow-sm p-6 border">
    <div class="flex justify-between items-start">
      <div>
        <div class="text-xs text-gray-500 uppercase tracking-wider mb-1">系统状态</div>
        <div class="flex items-center gap-2">
          <span class="w-3 h-3 rounded-full inline-block" :class="{
            'bg-green-500': dashboardHealth && dashboardHealth.status === 'ok',
            'bg-yellow-500': dashboardHealth && dashboardHealth.status === 'degraded',
            'bg-red-500': !dashboardHealth || dashboardHealth.status === 'error'
          }"></span>
          <span class="text-xl font-bold" :class="{
            'text-green-700': dashboardHealth && dashboardHealth.status === 'ok',
            'text-yellow-700': dashboardHealth && dashboardHealth.status === 'degraded',
            'text-red-700': !dashboardHealth || dashboardHealth.status === 'error'
          }">
            {{ dashboardHealth ? (dashboardHealth.status === 'ok' ? '正常' : dashboardHealth.status === 'degraded' ? '降级' : '异常') : '加载中...' }}
          </span>
        </div>
      </div>
      <span class="text-xs text-gray-400" v-if="dashboardHealth && dashboardHealth.uptime">运行中 {{ dashboardHealth.uptime }}</span>
    </div>

    <!-- 组件状态 -->
    <div class="flex gap-6 mt-4 pt-4 border-t">
      <div v-for="(comp, name) in (dashboardHealth && dashboardHealth.components) || {}" :key="name" class="text-center">
        <div class="text-xs text-gray-500 mb-1">{{ name }}</div>
        <div class="flex items-center gap-1 text-sm font-semibold" :class="{
          'text-green-600': comp.status === 'ok',
          'text-yellow-600': comp.status === 'degraded',
          'text-red-600': comp.status === 'error' || comp.status === 'stopped'
        }">
          <span class="w-2 h-2 rounded-full inline-block" :class="{
            'bg-green-500': comp.status === 'ok',
            'bg-yellow-500': comp.status === 'degraded',
            'bg-red-500': comp.status === 'error' || comp.status === 'stopped'
          }"></span>
          {{ comp.status === 'ok' ? '正常' : comp.status === 'stopped' ? '已停' : comp.status === 'degraded' ? '降级' : '异常' }}
          <span v-if="comp.detail" class="text-xs text-gray-400 font-normal ml-1">({{ comp.detail }})</span>
        </div>
      </div>
      <div v-if="!dashboardHealth" class="text-sm text-gray-400">加载中...</div>
    </div>
  </div>

  <!-- 第二段：运行时指标 -->
  <div class="bg-white rounded-xl shadow-sm p-6 border">
    <div class="text-xs text-gray-500 uppercase tracking-wider mb-4">运行时指标</div>
    <!-- 全局指标卡片 -->
    <div class="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
      <div class="bg-gray-50 rounded-lg p-4 text-center">
        <div class="text-2xl font-bold text-blue-600">{{ (dashboardMetrics && dashboardMetrics.global && dashboardMetrics.global.total_downloads) || 0 }}</div>
        <div class="text-xs text-gray-500 mt-1">历史总下载</div>
      </div>
      <div class="bg-gray-50 rounded-lg p-4 text-center">
        <div class="text-2xl font-bold text-purple-600">{{ (dashboardMetrics && dashboardMetrics.global && dashboardMetrics.global.active_downloads) || 0 }}</div>
        <div class="text-xs text-gray-500 mt-1">当前活跃</div>
      </div>
      <div class="bg-gray-50 rounded-lg p-4 text-center">
        <div class="text-2xl font-bold text-amber-600">{{ (dashboardMetrics && dashboardMetrics.global && dashboardMetrics.global.worker_count) || 0 }}</div>
        <div class="text-xs text-gray-500 mt-1">Worker</div>
      </div>
      <div class="bg-gray-50 rounded-lg p-4 text-center">
        <div class="text-2xl font-bold text-green-600">{{ dashboardMetrics && dashboardMetrics.tasks ? Object.keys(dashboardMetrics.tasks).length : 0 }}</div>
        <div class="text-xs text-gray-500 mt-1">任务总数</div>
      </div>
    </div>

    <!-- 任务指标表格 -->
    <div class="text-xs text-gray-500 mb-2">任务指标</div>
    <div v-if="dashboardMetrics && dashboardMetrics.tasks && Object.keys(dashboardMetrics.tasks).length > 0" class="overflow-x-auto">
      <table class="min-w-full text-sm">
        <thead>
          <tr class="border-b text-xs text-gray-500">
            <th class="text-left py-2 pr-4">任务</th>
            <th class="text-right py-2 px-2">完成</th>
            <th class="text-right py-2 px-2">失败</th>
            <th class="text-right py-2 px-2">重试</th>
            <th class="text-right py-2 px-2">平均延迟</th>
            <th class="text-right py-2 px-2">队列深度</th>
            <th class="text-right py-2 px-2">活跃数</th>
            <th class="text-right py-2 px-2">并发度</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(tm, tid) in dashboardMetrics.tasks" :key="tid" class="border-b hover:bg-gray-50">
            <td class="py-2 pr-4 font-medium text-gray-800">{{ tid }}</td>
            <td class="py-2 px-2 text-right">{{ tm.completed || 0 }}</td>
            <td class="py-2 px-2 text-right" :class="tm.failures > 0 ? 'text-red-600' : ''">{{ tm.failures || 0 }}</td>
            <td class="py-2 px-2 text-right">{{ tm.retried || 0 }}</td>
            <td class="py-2 px-2 text-right">{{ tm.avg_latency_ms ? (tm.avg_latency_ms + 'ms') : '-' }}</td>
            <td class="py-2 px-2 text-right">{{ tm.queue_depth || 0 }}</td>
            <td class="py-2 px-2 text-right">{{ tm.active || 0 }}</td>
            <td class="py-2 px-2 text-right">{{ tm.concurrency || '-' }}</td>
          </tr>
        </tbody>
      </table>
    </div>
    <div v-else class="text-sm text-gray-400 py-4 text-center">
      {{ dashboardMetrics ? '暂无任务指标数据' : '加载中...' }}
    </div>
  </div>

  <!-- 第三段：失败记录 -->
  <div class="bg-white rounded-xl shadow-sm p-6 border">
    <div class="flex justify-between items-center mb-4">
      <div class="text-xs text-gray-500 uppercase tracking-wider">失败记录</div>
      <div class="flex items-center gap-3">
        <input type="text" v-model="dashboardFailuresTaskId" @input="searchDashboardFailures" placeholder="过滤 task_id..." class="text-sm border rounded-md px-2 py-1.5 w-40 focus:outline-none focus:ring-2 focus:ring-blue-500">
        <select v-model="dashboardFailuresLimit" @change="changeDashboardFailuresLimit" class="text-sm border rounded-md px-2 py-1.5 focus:outline-none focus:ring-2 focus:ring-blue-500">
          <option :value="20">最近 20 条</option>
          <option :value="50">最近 50 条</option>
          <option :value="100">最近 100 条</option>
        </select>
        <button @click="fetchFailures" class="text-sm bg-white border text-gray-700 hover:bg-gray-50 px-3 py-1.5 rounded-md">刷新</button>
      </div>
    </div>
    <div v-if="dashboardFailures && dashboardFailures.failures && dashboardFailures.failures.length > 0" class="overflow-x-auto">
      <table class="min-w-full text-sm">
        <thead>
          <tr class="border-b text-xs text-gray-500">
            <th class="text-left py-2 pr-4">时间</th>
            <th class="text-left py-2 pr-4">任务</th>
            <th class="text-left py-2 pr-4">URL</th>
            <th class="text-left py-2 pr-4">错误信息</th>
            <th class="text-center py-2 px-2">尝试</th>
            <th class="text-center py-2 px-2">永久</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(f, idx) in dashboardFailures.failures" :key="idx" class="border-b hover:bg-gray-50">
            <td class="py-2 pr-4 text-xs text-gray-500 whitespace-nowrap">{{ f.timestamp ? new Date(f.timestamp * 1000).toLocaleString() : '-' }}</td>
            <td class="py-2 pr-4 font-medium text-gray-800">{{ f.task_id || '-' }}</td>
            <td class="py-2 pr-4 max-w-xs truncate text-gray-600" :title="f.url">{{ f.url || '-' }}</td>
            <td class="py-2 pr-4 text-red-600 max-w-md truncate" :title="f.error">{{ f.error || '-' }}</td>
            <td class="py-2 px-2 text-center">{{ f.attempt || 0 }}</td>
            <td class="py-2 px-2 text-center">{{ f.permanent ? '✅' : '❌' }}</td>
          </tr>
        </tbody>
      </table>
    </div>
    <div v-else class="text-sm text-gray-400 py-4 text-center">
      {{ dashboardFailures ? '暂无失败记录' : '加载中...' }}
    </div>
  </div>
</div>
```

- [ ] **步骤 5：在 main.js 中添加 viewMode 切换逻辑（mounted/beforeUnmount）**

在 `web/static/app/main.js` 的 `mounted` 生命周期末尾（`this.showAddTaskModal = false` 之后），添加 viewMode watch 逻辑。由于 Vue 3 Options API 中 watch 是独立字段，我们通过在 `watch` 对象中添加一个 `viewMode` 处理器来管理轮询生命周期：

在 `web/static/app/main.js` 的 `watch` 对象中（约第 185 行 `selectedType` 之后），添加：

```javascript
viewMode: function (val) {
  if (val === 'dashboard') {
    this.fetchDashboardData()
    this.startDashboardPolling()
  } else {
    this.stopDashboardPolling()
  }
}
```

同时确保 `beforeUnmount` 中清理轮询定时器（在 `web/static/app/main.js` 的 `beforeUnmount` 方法中，在原有清理之后添加）：

```javascript
this.stopDashboardPolling()
```

- [ ] **步骤 6：验证构建**

```bash
go build ./...
```

- [ ] **步骤 7：Commit**

```bash
git add web/static/index.html web/static/app/main.js
git commit -m "feat(web): Phase 4 - add dashboard view with health, metrics, failures panels"
```

---

### 任务 4：批量操作增强（跨页全选/批量重试/选中计数）

**涉及文件：**
- 修改：`web/static/index.html` — 批量操作栏 UI 增强
- 修改：`web/static/app/taskList.js` — 新增跨页全选、批量重试、批量取消方法
- 修改：`web/static/app/main.js` — 新增 `selectAllScope` data 字段

- [ ] **步骤 1：在 main.js 中添加 selectAllScope data 字段**

在 `web/static/app/main.js` 的 `data` 对象中，在 `selectedObjectUrls` 之后添加：

```javascript
selectAllScope: 'page', // 'page' | 'all'
```

- [ ] **步骤 2：在 taskList.js 中增强 toggleSelectAllObjects 方法**

替换 `web/static/app/taskList.js` 中的 `toggleSelectAllObjects` 方法（约第 41-52 行）为增强版本：

```javascript
toggleSelectAllObjects: function () {
  var urls = (this.filteredObjects || []).map(function (o) { return o.url })
  // Toggle between page-only and all-pages mode
  if (this.selectAllScope === 'all') {
    // Currently in 'all' mode — switch to 'page' mode
    this.selectAllScope = 'page'
    // Select only current page objects
    this.selectedObjectUrls = [].concat(urls)
  } else {
    // Check if all current page objects are already selected
    var allSelected = urls.length > 0 && urls.every(function (u) { return this.selectedObjectUrls.indexOf(u) >= 0 }.bind(this))
    if (allSelected) {
      // Deselect current page objects
      this.selectedObjectUrls = this.selectedObjectUrls.filter(function (u) { return urls.indexOf(u) < 0 })
    } else {
      // Select current page objects (union)
      var set = {}
      this.selectedObjectUrls.forEach(function (u) { set[u] = true })
      urls.forEach(function (u) { set[u] = true })
      this.selectedObjectUrls = Object.keys(set)
      // Enter 'all' mode if the select-all checkbox was clicked with nothing selected yet
      if (urls.length === this.filteredObjects.length && this.selectedObjectUrls.length > urls.length) {
        this.selectAllScope = 'all'
      }
    }
  }
},
```

- [ ] **步骤 3：在 taskList.js 中新增批量操作方法**

在 `web/static/app/taskList.js` 的 `methods` 中，在 `undoCancelSelectedObjects` 之后添加：

```javascript
retrySelectedObjects: function () {
  if (this.isWriteDisabled) { this.showToast('UI-Only 模式下已禁用', 'error'); return }
  if (this.selectedObjectUrls.length === 0) return

  var self = this
  var isAllMode = this.selectAllScope === 'all'
  // Get the selected objects from current page data
  var objs = (this.selectedTask && this.selectedTask.objects) || []
  var failedUrls = []

  if (isAllMode) {
    // Cross-page mode: retry all failed for this task (calls RetryAllFailed endpoint)
    if (!this.selectedTaskId) return
    AppAPI.post('/api/tasks/' + encodeURIComponent(this.selectedTaskId) + '/retry', {})
      .then(function (res) {
        if (!res.ok) throw new Error('批量重试失败')
        self.showToast('已重试所有失败对象', 'success')
        self.selectedObjectUrls = []
        self.selectAllScope = 'page'
        self.fetchTaskDetails(self.selectedTaskId, true)
      }).catch(function (e) { self.showToast('批量重试失败: ' + e.message, 'error') })
    return
  }

  // Page mode: only retry selected failed objects individually
  objs.forEach(function (o) {
    if (self.selectedObjectUrls.indexOf(o.url) >= 0 && o.status === 'failed') {
      failedUrls.push(o.url)
    }
  })

  if (failedUrls.length === 0) {
    self.showToast('选中的对象中没有可重试的失败项', 'info')
    return
  }

  // Retry each failed object one by one (individual retry API)
  var completed = 0
  failedUrls.forEach(function (url) {
    AppAPI.post('/api/tasks/' + encodeURIComponent(self.selectedTaskId) + '/retry', { url: url })
      .then(function (res) {
        if (res.ok) {
          completed++
          // Update local status
          var obj = (self.selectedTask && self.selectedTask.objects || []).find(function (o) { return o.url === url })
          if (obj) { obj.status = 'pending'; obj.progress = 0 }
        }
      }).catch(function () {})
      .finally(function () {
        if (completed === failedUrls.length) {
          self.showToast('已重试 ' + completed + ' 个失败对象', 'success')
          self.selectedObjectUrls = []
          self.fetchTaskDetails(self.selectedTaskId, true)
        }
      })
  })
},

cancelSelectAllObjects: function () {
  if (this.isWriteDisabled) { this.showToast('UI-Only 模式下已禁用', 'error'); return }
  if (this.selectedObjectUrls.length === 0) return

  var self = this
  if (this.selectAllScope === 'all') {
    // Cross-page mode: cancel the entire task
    if (!this.selectedTaskId) return
    AppAPI.post('/api/tasks/' + encodeURIComponent(this.selectedTaskId) + '/cancel', {})
      .then(function (res) {
        if (!res.ok) throw new Error('取消失败')
        self.showToast('任务已取消', 'success')
        self.selectedObjectUrls = []
        self.selectAllScope = 'page'
        self.fetchTasks()
        self.fetchTaskDetails(self.selectedTaskId, true)
      }).catch(function (e) { self.showToast('取消失败: ' + e.message, 'error') })
    return
  }

  // Page mode: batch cancel selected objects
  AppAPI.post('/api/tasks/' + encodeURIComponent(self.selectedTaskId) + '/object/cancel_batch', { urls: self.selectedObjectUrls })
    .then(function (res) { if (!res.ok) throw new Error('批量取消失败'); return res.json() })
    .then(function (result) {
      var okList = Object.entries(result).filter(function (kv) { return kv[1] === 'ok' }).map(function (kv) { return kv[0] })
      if (self.selectedTask && self.selectedTask.objects && okList.length > 0) {
        self.selectedTask.objects.forEach(function (o) {
          if (okList.indexOf(o.url) >= 0) { o.status = 'cancelled'; o.progress = 0 }
        })
      }
      var failed = Object.entries(result).filter(function (kv) { return kv[1] !== 'ok' })
      if (failed.length === 0) self.showToast('已取消选中对象', 'success')
      else self.showToast('部分对象取消失败', 'error')
      self.selectedObjectUrls = []
    }).catch(function (e) { self.showToast('批量取消失败: ' + e.message, 'error') })
},

undoCancelSelectAllObjects: function () {
  if (this.isWriteDisabled) { this.showToast('UI-Only 模式下已禁用', 'error'); return }
  if (this.selectedObjectUrls.length === 0) return

  var self = this
  if (this.selectAllScope === 'all') {
    // Cross-page mode is not applicable for undo-cancel — revert to page mode
    self.showToast('跨页全选模式不支持批量撤销取消，请切换为单页模式', 'info')
    return
  }

  // Page mode: batch undo cancel selected objects
  AppAPI.post('/api/tasks/' + encodeURIComponent(self.selectedTaskId) + '/object/undo_cancel_batch', { urls: self.selectedObjectUrls })
    .then(function (res) { if (!res.ok) throw new Error('批量撤销失败'); return res.json() })
    .then(function (result) {
      var okList = Object.entries(result).filter(function (kv) { return kv[1] === 'ok' }).map(function (kv) { return kv[0] })
      if (self.selectedTask && self.selectedTask.objects && okList.length > 0) {
        self.selectedTask.objects.forEach(function (o) {
          if (okList.indexOf(o.url) >= 0) { o.status = 'pending'; o.progress = 0 }
        })
      }
      var failed = Object.entries(result).filter(function (kv) { return kv[1] !== 'ok' })
      if (failed.length === 0) self.showToast('已撤销选中对象', 'success')
      else self.showToast('部分对象撤销失败', 'error')
      self.selectedObjectUrls = []
    }).catch(function (e) { self.showToast('批量撤销失败: ' + e.message, 'error') })
}
```

- [ ] **步骤 4：在 index.html 中增强批量操作栏 UI**

找到任务详情区域的批量操作栏（约第 499~503 行），替换为增强版本：

```html
<div class="flex items-center gap-2">
  <button @click="toggleSelectAllObjects" class="text-xs bg-white text-gray-700 hover:bg-gray-100 px-2 py-1 rounded-md transition-colors">
    {{ selectAllScope === 'all' ? '取消全选' : '全选当前页' }}
  </button>
  <label class="flex items-center gap-1 text-xs text-gray-500 cursor-pointer" v-if="selectedObjectUrls.length > 0">
    <input type="radio" v-model="selectAllScope" value="page" class="accent-blue-500"> 仅当前页
  </label>
  <label class="flex items-center gap-1 text-xs text-gray-500 cursor-pointer" v-if="selectedObjectUrls.length > 0">
    <input type="radio" v-model="selectAllScope" value="all" class="accent-blue-500"> 全选所有页
  </label>
  <button @click="cancelSelectAllObjects" :disabled="isWriteDisabled || selectedObjectUrls.length===0" :title="isWriteDisabled ? 'UI-Only 模式下已禁用' : ''" class="text-xs px-2 py-1 rounded-md transition-colors" :class="selectedObjectUrls.length>0 ? 'bg-red-50 text-red-600 hover:bg-red-100' : 'bg-gray-100 text-gray-400 cursor-not-allowed'">取消选中</button>
  <button @click="retrySelectedObjects" :disabled="isWriteDisabled || selectedObjectUrls.length===0" :title="isWriteDisabled ? 'UI-Only 模式下已禁用' : ''" class="text-xs px-2 py-1 rounded-md transition-colors" :class="selectedObjectUrls.length>0 ? 'bg-orange-50 text-orange-600 hover:bg-orange-100' : 'bg-gray-100 text-gray-400 cursor-not-allowed'">批量重试</button>
  <button @click="undoCancelSelectAllObjects" :disabled="isWriteDisabled || selectedObjectUrls.length===0" :title="isWriteDisabled ? 'UI-Only 模式下已禁用' : ''" class="text-xs px-2 py-1 rounded-md transition-colors" :class="selectedObjectUrls.length>0 ? 'bg-blue-50 text-blue-600 hover:bg-blue-100' : 'bg-gray-100 text-gray-400 cursor-not-allowed'">撤销取消</button>
  <span class="text-[10px] px-2 py-0.5 rounded-full bg-gray-100 text-gray-500" v-if="selectedObjectUrls.length>0">
    已选 {{ selectedObjectUrls.length }} 个对象<span v-if="selectAllScope === 'all'">（跨页全选）</span>
  </span>
</div>
```

注意：原有方法 `cancelSelectedObjects` 和 `undoCancelSelectedObjects` 被新的 `cancelSelectAllObjects` 和 `undoCancelSelectAllObjects` 取代。HTML 中的 `@click` 绑定需要改为新方法名。同时原有 `selectedObjectCount > 0` 条件改为 `selectedObjectUrls.length > 0`。

- [ ] **步骤 5：在 main.js 的 methods 中添加 selectAllScope 重置**

为了确保切换任务时 selectAllScope 重置，在 `taskList.js` 的 `selectTask` 方法中添加：

在 `taskList.js` 的 `selectTask` 方法中（约第 23-31 行），在 `this.selectedObjectUrls = []` 之后（第 27 行）添加：

```javascript
this.selectAllScope = 'page'
```

- [ ] **步骤 6：验证构建**

```bash
go build ./...
```

- [ ] **步骤 7：Commit**

```bash
git add web/static/index.html web/static/app/taskList.js web/static/app/main.js
git commit -m "feat(web): Phase 4 - enhance batch ops (cross-page select, batch retry, selected count)"
```

---

## 自检清单

1. **规格覆盖度：**
   - [x] A. 运维仪表盘 — Sidebar 按钮、三段式视图、轮询策略、dashboard.js 模块
   - [x] C. 跨页全选 — `selectAllScope` 状态、全选/取消逻辑、API 路由
   - [x] C. 批量重试 — `retrySelectedObjects` 方法、跨页模式走任务级 retry API
   - [x] C. 选中计数 — 显示 `已选 N 个对象（跨页全选）`
   - [x] B1. SSE 重连刷新 — `onopen` 回调中调用 `fetchTasks()`
   - [x] B2. cancelled 进度条残留 — 增加 `obj.status !== 'cancelled'`
   - [x] B3. 分页 total=0 — 增加 `v-if="pagination.total > 0"`
   - [x] B4. progress 显示逻辑 — 增加 `v-if="obj.status === 'downloading' || (obj.progress > 0 && obj.progress < 100)"`

2. **占位符扫描：** 无 TODO、无"待定"、无"后续实现"。

3. **类型一致性：**
   - `dashboardHealth`、`dashboardMetrics`、`dashboardFailures` — 在 main.js data 中定义，dashboard.js 中使用
   - `selectAllScope` — 在 main.js data 中定义（'page'|'all'），taskList.js 的方法中使用
   - `AppDashboard.register(app)` — 遵循与 `AppTaskList`、`AppVideoPlayer` 相同的注册模式
   - 新方法名与 HTML 模板中的 `@click` 绑定一致