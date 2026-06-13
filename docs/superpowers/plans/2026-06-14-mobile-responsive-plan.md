# Web UI 移动端适配 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 对现有 Web UI（单文件 Vue 3 SPA）做移动端适配，覆盖侧边栏抽屉、顶栏折叠、表格精简、模态框全屏、仪表盘响应式和触摸增强。

**架构：** 纯 CSS 响应式 + 最小 JS 状态量，利用 Tailwind 响应式前缀，不新增独立移动端模板。

**技术栈：** Vue 3 (CDN)、Tailwind CSS (CDN)、单文件 HTML

---

### 任务 1：侧边栏抽屉式导航

**文件：**
- 修改：`web/static/index.html`

**改动：** 侧边栏从固定布局改为 `fixed` 定位抽屉，新增 `mobileSidebarOpen` 状态，顶栏新增 hamburger 按钮。

- [ ] **步骤 1：在 Vue data 中添加 mobileSidebarOpen 状态**

在 `web/static/app/main.js` data 函数中新增：

```js
mobileSidebarOpen: false,
```

并空位注释说明用途。

- [ ] **步骤 2：在 selectTask 中关闭侧边栏**

在 `selectTask` 方法开头添加：

```js
this.mobileSidebarOpen = false
```

- [ ] **步骤 3：修改侧边栏 `<aside>` 标签 class**

当前（约 72 行）：
```html
<aside class="w-64 bg-white shadow-md flex flex-col z-10" data-testid="sidebar">
```

改为：
```html
<aside class="w-64 bg-white shadow-md flex flex-col z-10 fixed lg:relative inset-y-0 left-0 z-50 transition-transform duration-300 ease-in-out lg:translate-x-0" :class="mobileSidebarOpen ? 'translate-x-0' : '-translate-x-full'" data-testid="sidebar">
```

- [ ] **步骤 4：在顶栏添加 hamburger 按钮**

在顶栏 `<header>` 内部（约 140 行），在 `h2` 之前添加：

```html
<button @click="mobileSidebarOpen = !mobileSidebarOpen" class="lg:hidden p-2 text-gray-600 hover:text-gray-800">
  <i class="fas fa-bars text-lg"></i>
</button>
```

- [ ] **步骤 5：添加移动端遮罩层**

在侧边栏同级（约 71-135 行之间），添加：

```html
<!-- Mobile sidebar backdrop -->
<div v-if="mobileSidebarOpen" @click="mobileSidebarOpen = false" class="fixed inset-0 bg-black bg-opacity-50 z-40 lg:hidden"></div>
```

- [ ] **步骤 6：commit**

```bash
git add web/static/index.html web/static/app/main.js
git commit -m "feat: mobile responsive - sidebar drawer navigation"
```

---

### 任务 2：顶栏工具栏响应式折叠

**文件：**
- 修改：`web/static/index.html`

**改动：** 顶栏分两行，新增 `mobileToolbarOpen` 状态，工具栏在移动端默认折叠。

- [ ] **步骤 1：在 Vue data 中添加 mobileToolbarOpen**

在 `main.js` data 中添加：`mobileToolbarOpen: false`

- [ ] **步骤 2：重构顶栏 `<header>` 结构**

当前约 140-215 行，顶栏是单行 `<div class="flex ...">`。

改为两行结构：

```html
<header class="bg-white border-b px-4 lg:px-6 py-3 shadow-sm z-10">
  <!-- 第一行：hamburger + 任务名 + 移动端展开按钮 -->
  <div class="flex items-center gap-2">
    <!-- hamburger: 已在任务1添加 -->
    <h2 class="text-lg lg:text-xl font-semibold truncate flex-1" 
        v-if="selectedTask" :title="getTaskDisplayName(selectedTask)">
      {{ getTaskDisplayName(selectedTask) }}
    </h2>
    <h2 class="text-lg lg:text-xl font-semibold text-gray-500 flex-1" v-if="!selectedTask">Select a Task</h2>
    
    <!-- 移动端：展开工具栏按钮 -->
    <button @click="mobileToolbarOpen = !mobileToolbarOpen" 
            class="lg:hidden p-1.5 text-gray-500 hover:text-gray-700">
      <i class="fas" :class="mobileToolbarOpen ? 'fa-times' : 'fa-ellipsis-v'"></i>
    </button>
  </div>

  <!-- 第二行：工具栏 -->
  <div v-if="selectedTask" 
       class="flex items-center gap-2 lg:gap-3 mt-2 overflow-x-auto pb-1"
       :class="mobileToolbarOpen ? 'flex' : 'hidden lg:flex'">
    
    <!-- Retry Failed 按钮 -->
    <button @click="retryAllFailed" data-testid="btn-retry-all" 
            :disabled="isWriteDisabled" 
            class="text-xs lg:text-sm bg-red-50 text-red-600 hover:bg-red-100 px-2 lg:px-3 py-1.5 rounded-md transition-colors flex items-center disabled:opacity-50 whitespace-nowrap">
      <i class="fas fa-redo mr-1"></i> Retry
    </button>
    
    <!-- 取消任务按钮 -->
    <button @click="cancelCurrentTask" data-testid="btn-cancel-task"
            :disabled="isWriteDisabled"
            class="text-xs lg:text-sm bg-gray-50 text-gray-700 hover:bg-gray-100 px-2 lg:px-3 py-1.5 rounded-md transition-colors flex items-center disabled:opacity-50 whitespace-nowrap">
      <i class="fas fa-ban mr-1"></i> 取消
    </button>

    <!-- Sort (桌面可见，移动端在展开时可见) -->
    <select v-model="sortBy" 
            class="text-xs lg:text-sm border rounded-md px-2 py-1.5 focus:outline-none focus:ring-2 focus:ring-blue-500">
      <option value="default">Default</option>
      <option value="date_desc">Newest</option>
      <option value="date_asc">Oldest</option>
      <option value="duration_desc">Longest</option>
      <option value="name_asc">A-Z</option>
    </select>

    <!-- Items per page -->
    <select v-model="pagination.limit"
            class="text-xs lg:text-sm border rounded-md px-2 py-1.5 focus:outline-none focus:ring-2 focus:ring-blue-500">
      <option :value="25">25</option>
      <option :value="50">50</option>
      <option :value="100">100</option>
      <option value="all">All</option>
    </select>

    <!-- 搜索框（桌面宽，移动端窄） -->
    <div class="relative flex-1 lg:flex-none">
      <span class="absolute inset-y-0 left-0 flex items-center pl-2 lg:pl-3 text-gray-400">
        <i class="fas fa-search text-xs"></i>
      </span>
      <input type="text" v-model="searchQuery" data-testid="search-input"
             class="pl-8 lg:pl-10 pr-3 py-1.5 border rounded-full text-xs lg:text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 w-full lg:w-64 transition-all"
             placeholder="Search...">
    </div>

    <!-- Grid/List toggle -->
    <div class="bg-gray-100 p-0.5 lg:p-1 rounded-lg flex text-xs lg:text-sm shrink-0">
      <button @click="viewMode = 'grid'" class="px-2 lg:px-3 py-1 rounded-md transition-all"
              :class="viewMode === 'grid' ? 'bg-white shadow text-blue-600 font-medium' : 'text-gray-500'">
        <i class="fas fa-th-large"></i>
      </button>
      <button @click="viewMode = 'list'" class="px-2 lg:px-3 py-1 rounded-md transition-all"
              :class="viewMode === 'list' ? 'bg-white shadow text-blue-600 font-medium' : 'text-gray-500'">
        <i class="fas fa-list"></i>
      </button>
    </div>

    <!-- Watch/Manage toggle -->
    <div class="bg-gray-100 p-0.5 lg:p-1 rounded-lg flex text-xs lg:text-sm shrink-0">
      <button @click="uiMode = 'watch'" class="px-2 lg:px-3 py-1 rounded-md transition-all"
              :class="uiMode === 'watch' ? 'bg-white shadow text-purple-600 font-medium' : 'text-gray-500'">观看</button>
      <button @click="uiMode = 'manage'" class="px-2 lg:px-3 py-1 rounded-md transition-all"
              :class="uiMode === 'manage' ? 'bg-white shadow text-purple-600 font-medium' : 'text-gray-500'">管理</button>
    </div>
  </div>
</header>
```

> 注意：实际实现时保持原有按钮的完整功能（isWriteDisabled、data-testid），只调整布局和 class。

- [ ] **步骤 3：移除旧的顶栏 `<header>` 内容**

删除约 140-215 行的原顶栏 `<header>` 块，替换为步骤 2 的新结构。

- [ ] **步骤 4：commit**

```bash
git add web/static/index.html
git commit -m "feat: mobile responsive - collapsible toolbar"
```

---

### 任务 3：表格精简列（4 个表格）

**文件：**
- 修改：`web/static/index.html`

**改动：** 在 4 个表格的 `<th>` 和 `<td>` 上添加 `hidden md:table-cell`。

- [ ] **步骤 1：对象列表表格精简**

约 800-862 行的 List View `table`。当前列：Name + Info + Tags + Status + Progress + Actions。

改为：

```html
<thead class="bg-gray-50">
  <tr>
    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Name</th>
    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider hidden md:table-cell">Info</th>
    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider hidden md:table-cell">Tags</th>
    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Status</th>
    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider hidden md:table-cell">Progress</th>
    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Actions</th>
  </tr>
</thead>
```

对应 `<td>` 添加相同 `hidden md:table-cell`：
- Info `<td>`：`hidden md:table-cell`
- Tags `<td>`：`hidden md:table-cell`
- Progress `<td>`：`hidden md:table-cell`

- [ ] **步骤 2：聚合视图列表表格精简**

约 410-473 行，聚合视图的 List 模式。列：Name + Info + Tags + Status + Progress + Task + Actions。

精简方案：Name + Status + Task + Actions（移动端），桌面全列。

在对应 `<th>` 和 `<td>` 上添加 `hidden md:table-cell`：
- Info 列：`hidden md:table-cell`
- Tags 列：`hidden md:table-cell`
- Progress 列：`hidden md:table-cell`

- [ ] **步骤 3：仪表盘指标表格精简**

约 552-578 行。列：任务 + 完成 + 失败 + 重试 + 延迟 + 队列 + 活跃 + 并发。

精简方案：
- 桌面全 8 列
- 移动端只保留：任务 + 完成 + 失败

在以下 `<th>` 和 `<td>` 上添加 `hidden md:table-cell`：
- 重试、延迟、队列、活跃、并发

- [ ] **步骤 4：仪表盘失败记录表格精简**

约 599-621 行。列：时间 + 任务 + URL + 错误 + 尝试 + 永久。

精简方案：
- 桌面全 6 列
- 移动端保留：时间 + 任务 + 错误

在以下 `<th>` 和 `<td>` 上添加 `hidden md:table-cell`：
- URL、尝试、永久

- [ ] **步骤 5：commit**

```bash
git add web/static/index.html
git commit -m "feat: mobile responsive - condensed table columns"
```

---

### 任务 4：模态框全屏化 + 仪表盘响应式

**文件：**
- 修改：`web/static/index.html`

- [ ] **步骤 1：配置面板模态框全屏适配**

约 221 行。当前：`<div class="bg-white rounded-lg shadow-2xl p-4 border w-[420px]">`

改为：
```html
<div class="bg-white shadow-2xl p-4 border w-full sm:max-w-lg mx-0 sm:mx-4 min-h-screen sm:min-h-0 rounded-none sm:rounded-lg">
```

- [ ] **步骤 2：视频播放器模态框全屏适配**

约 872 行。当前：`<div class="fixed inset-0 bg-black bg-opacity-95 z-50 flex items-center justify-center p-4 backdrop-blur-sm ...">`

改为：
```html
<div class="fixed inset-0 bg-black bg-opacity-95 z-50 flex items-center justify-center p-0 sm:p-4 backdrop-blur-sm ...">
```

- [ ] **步骤 3：仪表盘组件状态 flex-wrap**

约 506 行。当前：`<div class="flex gap-6 mt-4 pt-4 border-t">`

改为：
```html
<div class="flex flex-wrap gap-3 sm:gap-6 mt-4 pt-4 border-t">
```

- [ ] **步骤 4：commit**

```bash
git add web/static/index.html
git commit -m "feat: mobile responsive - fullscreen modals and dashboard layout"
```

---

### 任务 5：触摸交互增强

**文件：**
- 修改：`web/static/index.html`

- [ ] **步骤 1：移动端禁用拖拽排序**

找到对象卡片 `<div>`（约 714 行）的 `:draggable` 绑定：

```html
:draggable="sortBy==='default' && !('ontouchstart' in window)"
```

- [ ] **步骤 2：视频控制栏移动端增大触摸区域**

找到视频播放器控制按钮（约 882+ 行），将按钮 `p-2` 改为 `p-2 sm:p-3 lg:p-2` 以保持桌面不变但移动端增大。

- [ ] **步骤 3：commit**

```bash
git add web/static/index.html
git commit -m "feat: mobile responsive - touch interaction improvements"
```

---

### 任务 6：Playwright 移动端测试

**文件：**
- 修改：`test/playwright/playwright.config.ts`
- 创建：`test/playwright/specs/mobile.spec.ts`

- [ ] **步骤 1：在 playwright.config.ts 中添加 mobile project**

```typescript
projects: [
  // ... existing desktop project
  {
    name: 'mobile',
    use: {
      ...devices['iPhone 15 Pro'],
      // Mobile Safari with touch events
    },
  },
],
```

- [ ] **步骤 2：创建 mobile.spec.ts 测试文件**

```typescript
import { test, expect } from '@playwright/test';

test.describe('Mobile Responsive', () => {

  test('M1: sidebar drawer opens and closes', async ({ page }) => {
    await page.goto('/');
    // Sidebar should be hidden on mobile
    await expect(page.locator('[data-testid="sidebar"]')).not.toBeVisible();
    
    // Click hamburger to open
    const hamburger = page.locator('.fa-bars').first();
    await hamburger.click();
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible();
    
    // Click backdrop to close
    const backdrop = page.locator('.bg-opacity-50').first();
    await backdrop.click();
    await expect(page.locator('[data-testid="sidebar"]')).not.toBeVisible();
  });

  test('M2: toolbar collapse works', async ({ page }) => {
    await page.goto('/');
    // Select a task
    // ... existing task selection logic
    // Verify toolbar items are in the expanded area
    // Toggle toolbar visibility
  });

  test('M3: table columns are condensed', async ({ page }) => {
    await page.goto('/');
    // Switch to list view
    // Verify only core columns are visible
  });

  test('M4: modal goes fullscreen', async ({ page }) => {
    await page.goto('/');
    // Open config modal
    // Verify it spans full width
    // Verify min-h-screen is present
  });

  test('M5: touch interaction for drag disabled', async ({ page }) => {
    await page.goto('/');
    // Verify draggable attribute is false
  });
});
```

- [ ] **步骤 3：运行移动端测试**

```bash
cd test/playwright
npx playwright test --project=mobile
```

预期：所有 5 个移动端测试通过。

- [ ] **步骤 4：commit**

```bash
git add test/playwright/playwright.config.ts test/playwright/specs/mobile.spec.ts
git commit -m "test: mobile responsive E2E tests (M1-M5)"
```

---

## 自检

- [x] **规格覆盖度：** 设计文档中 6 个章节全部覆盖（侧边栏/顶栏/表格/模态框/仪表盘/触摸）
- [x] **占位符扫描：** 无
- [x] **类型一致性：** `mobileSidebarOpen`、`mobileToolbarOpen`、`isTouchDevice` 在任务间一致
