# Phase 4: Web UI 打磨 设计文档

> 日期：2026-06-04
> 项目：download-manager

## 综述

在前三阶段（可维护性重构、性能优化、可观测性）基础上，提升 Web UI 用户体验。核心目标：
1. 利用 Phase 3 新增的 API 端点构建运维仪表盘
2. 增强批量操作能力（跨页全选、批量重试）
3. 修复现有 UI bug

### 范围

| 维度 | 内容 | 状态 |
|------|------|------|
| A | 运维仪表盘 — 健康检查 / 指标 / 失败记录 UI | 设计确认 |
| C | 批量操作增强 — 跨页全选、批量重试 | 设计确认 |
| B | Bug 修复 — 现有 UI 问题修复 | 设计确认 |

---

## A. 运维仪表盘

### 新增 UI 入口

在左侧 Sidebar "Active Downloads" 和 "聚合视图" 按钮下方，新增"📊 仪表盘"按钮：

```html
<button @click="viewMode = 'dashboard'">
  <i class="fas fa-chart-bar"></i> 仪表盘
</button>
```

选中后 `viewMode = 'dashboard'`，主内容区替换为仪表盘视图。

### 仪表盘布局（三段式）

#### 第一段：健康状态摘要条

调用 `GET /api/healthz`，展示：

| 组件 | 显示方式 |
|------|---------|
| 整体状态 | 绿/黄/红圆点 + "正常/降级/异常" 文字 |
| Uptime | 顶部右侧显示 |
| 调度器 | 状态圆点 + 心跳时间 |
| Worker | 状态圆点 + 数量 |
| 事件总线 | 状态圆点 + 订阅数 |
| 任务 | 状态圆点 + 加载数 |

异常时（error/degraded）用红色/黄色背景高亮。

#### 第二段：运行时指标

调用 `GET /api/metrics`，分两部分：

**全局指标卡片**（4 个指标块横排）：
- 历史总下载数
- 当前活跃下载数
- Worker 数量
- 任务总数

**任务指标表格**：
| 任务 | 完成 | 失败 | 重试 | 平均延迟 | 队列深度 | 活跃数 | 并发度 |

每 10 秒自动刷新（或 SSE 触发时刷新）。

#### 第三段：失败记录

调用 `GET /api/metrics/failures?limit=20`，展示表格：

| 时间 | 任务 ID | URL | 错误信息 | 尝试次数 | 是否永久 |

支持按 `task_id` 过滤（输入框）、limit 下拉选择（20/50/100）。

### 数据刷新策略

- 仪表盘挂载时立即请求所有数据
- `GET /api/healthz` — 每 5 秒轮询
- `GET /api/metrics` — 每 10 秒轮询
- `GET /api/metrics/failures` — 每 15 秒轮询（或手动刷新）
- 组件卸载时清除所有定时器

### 实现文件

**修改：** `web/static/index.html` — 新增仪表盘视图模板
**修改：** `web/static/app/main.js` — 新增 `viewMode = 'dashboard'`、轮询定时器
**新增：** `web/static/app/dashboard.js` — 仪表盘方法模块（与 taskList.js 等注册方式一致）

---

## C. 批量操作增强

### 现状问题

当前 `toggleSelectAllObjects` 只选择**当前页**的对象（即 `this.filteredObjects`），UI 用 `selectedObjectUrls` 数组记录选中。但翻页后选中的对象会丢失，无法跨页批量操作。

### 增强方案

#### 1. 跨页全选

在批量操作栏增加"全选所有页"选项：

```html
<label><input type="radio" v-model="selectAllScope" value="all"> 全选所有页</label>
<label><input type="radio" v-model="selectAllScope" value="page"> 仅当前页</label>
```

- `selectAllScope = 'page'` — 行为与当前一致（选中当前页对象）
- `selectAllScope = 'all'` — 标记为"全选模式"，后续批量操作（取消/重试/撤销取消）时，API 请求带上 `apply_to_all=true` 或改用 `/api/tasks/{id}/retry` 等全量操作

**后端配合：** 批量取消/重试端点已支持全量操作（`RetryAllFailed`、`CancelTask`），无需新增后端逻辑。前端在 "全选模式" 下直接调用全量 API。

#### 2. 批量重试

在批量操作栏增加"批量重试"按钮：

- 选中对象中**仅对 status 为 `failed` 的对象**执行重试
- 调用 `POST /api/tasks/{id}/retry`（已存在）
- 跨页全选模式下直接调用 `RetryAllFailed`（等价于任务级重试所有失败）

#### 3. 选中计数增强

当前只在按钮旁显示 `已选 N` 的文字。增强为在操作栏显示：

```
已选 23 个对象（当前页 15 + 跨页 8）
```

### 实现文件

**修改：** `web/static/index.html` — 批量操作栏 UI 增强
**修改：** `web/static/app/main.js` — 新增 `selectAllScope`、`retrySelectedObjects` 方法
**修改：** `web/static/app/taskList.js` — 增强 `toggleSelectAllObjects`、新增 `retryMultipleObjects`

---

## B. Bug 修复清单

### B1. SSE 重连后丢失任务列表数据

**文件：** `web/static/app/helpers.js`

**问题：** `eventSource.onerror` 只打印错误和 toast 提示，没有在重连成功后重新拉取数据。浏览器 `EventSource` 会自动重连，但不会重新发送之前的事件。

**修复：** 在 `initSSE()` 中添加 `eventSource.onopen` 回调，重连成功后调用 `fetchTasks()` 拉取最新数据。

```javascript
this.eventSource.onopen = function () {
  self.fetchTasks()  // 重连后刷新任务列表
}
```

### B2. 任务详情中 cancelled 对象的进度条残留

**文件：** `web/static/index.html`

**问题：** 在 Grid View 中，`filteredObjects` 遍历时，`cancelled` 状态的对象会显示进度条（`obj.progress > 0` 时显示），但 cancelled 对象不应该显示进度条。

**修复：** 进度条显示条件增加 `obj.status !== 'cancelled'`：

```html
<div v-if="obj.status === 'downloading' || (obj.status !== 'completed' && obj.status !== 'cancelled' && obj.progress > 0)">
```

### B3. 分页页数选择器在 total=0 时显示 "1 / 1"

**文件：** `web/static/index.html:182-184`

**问题：** `Math.ceil(pagination.total / pagination.limit) || 1` 在 total=0 时返回 1。虽然没有实际影响（没有对象可显示），但 UI 上会看到一个孤立的 "1 / 1" 分页控件。

**修复：** 在分页容器上加 `v-if="pagination.total > 0"`。

### B4. 对象卡片的 progress 显示逻辑

**文件：** `web/static/index.html`

**问题：** `obj.progress` 在 SSR 中状态为 `pending` 时也可能有非零值（完全下载前的进度残留），此时显示进度条会让用户困惑。

**修复：** 进度百分比文字只在 `downloading` 或 `progress > 0 && progress < 100` 时显示。

---

## 文件清单

### 新增文件

| 文件 | 职责 |
|------|------|
| `web/static/app/dashboard.js` | 仪表盘数据获取与展示方法（健康、指标、失败记录） |

### 修改文件

| 文件 | 修改内容 |
|------|---------|
| `web/static/index.html` | Sidebar 新增"仪表盘"按钮；新增仪表盘三段式视图模板；批量操作栏跨页全选UI；4 个 bug 修复 |
| `web/static/app/main.js` | 新增 `viewMode` 状态和定时器管理；新增仪表盘相关 data |
| `web/static/app/taskList.js` | 增强 `toggleSelectAllObjects`；新增 `retrySelectedObjects`、`cancelSelectAllObjects` |
| `web/static/app/helpers.js` | SSE `onopen` 重连后刷新 |

### API 依赖（Phase 3 已实现，无需改动后端）

| 端点 | 用途 |
|------|------|
| `GET /api/healthz` | 仪表盘健康状态 |
| `GET /api/metrics` | 仪表盘运行时指标 |
| `GET /api/metrics/failures` | 仪表盘失败记录 |
| `POST /api/tasks/{id}/retry` | 批量重试（已有） |

---

## 工程规范

- 所有 JS 代码沿用现有 IIFE 模式 (`;(function () { 'use strict' ... })()`)
- 方法注册使用 `window.AppDashboard = { register: function(app) { ... } }`
- CSS 使用 Tailwind CDN 现有类，不新增自定义样式
- 所有修改后 `go build ./...` + `go vet ./...` + `go test ./...` 全部通过
- commit 信息格式：`feat(web): Phase 4 - <具体变更>`

---

## 变更日志

- 2026-06-04: 初版设计文档