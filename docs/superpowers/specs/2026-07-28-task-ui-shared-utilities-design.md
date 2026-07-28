# Task UI 共享工具函数与模板设计

## 概述

下载管理器 Web UI 的 TaskUI 插件系统中，4 个现有任务类型（tktube、hanime、vikacg、urllist）的 viewer.js 文件存在大量重复代码。本设计旨在：

1. 提取重复代码为可复用共享模块
2. 简化现有 viewer 实现
3. 提供新任务类型模板 + 脚手架脚本

## 现有问题

| 重复片段 | 出现次数 | 说明 |
|---------|---------|------|
| `fileUrl_impl()` 路径转 URL | 4 处 | 全部 viewer 文件各自定义，逻辑完全一致 |
| `copyToClipboard()` | 3 处 | tktube/hanime/vikacg 的 renderViewer 中内联定义 |
| Modal 骨架（overlay/panel/header/footer/backdrop/ESC） | 3 处 | 每个 viewer 约 60 行重复 |
| 标签 chip 渲染 | 3 处 | 完全相同的 `<span>#tag</span>` 模式 |
| `getTitle()` / `getDate()` 等简单访问器 | 3-4 处 | 简单的 `obj.metadata.xxx` 取值 |
| 合集 + 推荐面板创建 | 2 处 | tktube 和 hanime 完全相同的模式 |
| Go 端 `init()` + `embed.FS` + `RegisterTaskUI` | 4 处 | 全部相同模式，仅参数不同 |

## 共享模块设计

### 1. `web/static/app/taskui/shared/data.js` — 统一数据访问器

挂载到 `TaskUI.Data` 命名空间，从现有 viewer 提取所有 `getXxx()` 函数。

**通用访问器（所有任务类型通用）：**

```
TaskUI.Data.getTitle(obj)         → obj.metadata.title || ''
TaskUI.Data.getTags(obj)          → 合并去重 extra.tags + metadata.tags
TaskUI.Data.getDate(obj)          → metadata.date || extra.date
TaskUI.Data.getDuration(obj)      → metadata.duration
TaskUI.Data.getResolution(obj)    → metadata.resolution
TaskUI.Data.getContentGroup(obj)  → metadata.content_group
TaskUI.Data.getOriginLink(obj)    → metadata.page_url → extra.origin_url → obj.url
TaskUI.Data.getDetails(obj)       → extra.description → metadata.description → details
TaskUI.Data.getCoverImage(obj)    → 多层回退：local_cover → extra.files(cover) → cover_url → 等
TaskUI.Data.getThumbImage(obj)    → 多层回退：local_preview → local_cover → extra.files(thumb) → 等
TaskUI.Data.getVideoUrl(obj)      → extra.files(video) → save_path → obj.url
TaskUI.Data.getFileUrl(obj)       → save_path → extra.files → ''
TaskUI.Data.fileUrl(path)         → 委托给 window.__dm_pathToUrl 或 _impl 回退
TaskUI.Data.statusColor(status)   → 状态颜色映射
TaskUI.Data.statusBg(status)      → 状态背景色映射
TaskUI.Data.priorityScore(obj)    → 变体优先级评分
TaskUI.Data.copyToClipboard(text) → 统一剪贴板（navigator.clipboard + fallback）
```

**特定类型扩展（在 viewer 中专有，不进共享模块）：**
- hanime: `getArtist()`, `getGenres()`, `getPlaylist()`, `getCoverImages()`, `getThumbImages()`, `getVideoURL()`
- vikacg: `getImages()`, `getLinks()`, `getExcerpt()`, `getContentHtml()`

### 2. `web/static/app/taskui/shared/dom.js` — DOM 构建辅助

挂载到 `TaskUI.Dom` 命名空间，提供纯 DOM 构建函数。

```
TaskUI.Dom.createTagChips(tags)            → DocumentFragment，每个 <span>#tag</span>
TaskUI.Dom.createInfoBar(items)            → 信息条 div，每项 <i> + text
TaskUI.Dom.createButton(text, onClick, css) → <button> 元素
TaskUI.Dom.createLink(href, text, css)      → <a> 元素，target=_blank
TaskUI.Dom.createBadge(text, bgColor)       → 徽章 <span>
TaskUI.Dom.createIcon(iconClass)            → <i class="fas fa-xxx">
```

### 3. `web/static/app/taskui/shared/modal.js` — Modal 构建器

挂载到 `TaskUI.Modal` 命名空间，提供完整的 DOM Modal 骨架。

**核心函数：**

```
TaskUI.Modal.createOverlay()         → 半透明黑色背景 overlay div
TaskUI.Modal.createPanel(maxWidth)   → 白色圆角面板 div
TaskUI.Modal.createHeader(title, { badges, onClose, extraButtons }) → header div
TaskUI.Modal.createFooter({ leftButtons, closeText, onClose }) → footer div
TaskUI.Modal.createVideoArea({ videoUrl, coverUrl, title, isHLS }) → 16:9 视频区域 div
TaskUI.Modal.createSidebar({ type, currentId, tags, onPlayItem }) → 右侧面板 div
TaskUI.Modal.setupCloseHandlers({ overlay, onClose, cleanup }) → { keyHandler }
TaskUI.Modal.create(obj, config)     → 全功能 Modal 构建器（视频/图片/通用）
```

**`create()` 配置对象：**

```js
TaskUI.Modal.create(obj, {
  title: string,                    // 标题（默认 getTitle）
  mediaType: 'video' | 'image' | null,  // 媒体类型
  infoBar: [{ icon, text }],        // 顶部信息条
  contentRenderer: (obj) => VNode,  // 主体内容
  sidebar: 'collection' | 'links' | null,  // 右侧面板类型
  links: [{ text, href }],          // 用于 links 侧栏
  footerActions: [{ text, onClick, primary }],  // 底部操作按钮
  copyButtons: ['title', 'link'],   // 复制按钮
  onClose: callback,                // 关闭回调
})
```

## 改造范围

### 新建文件

| 文件 | 行数估计 | 内容 |
|------|---------|------|
| `web/static/app/taskui/shared/data.js` | ~120 | 统一数据访问器 |
| `web/static/app/taskui/shared/dom.js` | ~80 | DOM 构建辅助 |
| `web/static/app/taskui/shared/modal.js` | ~250 | Modal 构建器 |
| `task/TEMPLATE/ui/ui.go` | ~15 | Go 端注册模板 |
| `task/TEMPLATE/ui/assets/viewer.js` | ~150 | JS 端模板（含注释） |
| `scripts/new-task-type.sh` | ~80 | 脚手架生成脚本 |
| `docs/new-task-checklist.md` | ~30 | 新任务开发指南 |

### 修改文件

| 文件 | 改动量 | 说明 |
|------|-------|------|
| `web/static/index.html` | +3 行 | 在 `loader.js` 后加载 shared 模块 |
| `task/tktube/ui/assets/viewer.js` | 减少 ~60% | 引用共享模块替代重复代码 |
| `task/hanime/ui/assets/viewer.js` | 减少 ~60% | 引用共享模块替代重复代码 |
| `task/vikacg/ui/assets/viewer.js` | 减少 ~50% | 图片画廊业务逻辑保留 |
| `task/urllist/ui/assets/viewer.js` | 减少 ~20% | 可选择性使用共享 data 访问器 |

### 不改动文件

- `web/static/app/helpers.js` — Vue mixin 方法保持不受影响
- `web/static/app/api.js` — `__dm_pathToUrl` 等保持为共享模块的委托目标
- `web/static/app/taskui/` 下现有文件（registry.js、defineForm.js 等）
- 所有 Go 后端文件（`core/` 接口不变）

## 模板代码

### Go 端模板 (`task/TEMPLATE/ui/ui.go`)

```go
package ui

import (
    "embed"
    "github.com/cocomhub/download-manager/core"
)

//go:embed assets/viewer.js
var assets embed.FS

func init() {
    core.RegisterTaskUI("{{TYPE}}", core.TaskUIAssets{
        FS:      assets,
        JSPaths: []string{"assets/viewer.js"},
        Label:   "{{LABEL}}",
        // HasForm: true,      // 是否有扩展表单
        // HasViewer: true,    // 是否有自定义查看器
        // HasAggregate: true, // 是否有聚合视图
    })
}
```

### JS 端模板 — 3 种变体

**变体 1：视频播放器（参考 tktube/hanime）**
```js
TaskUI.register('{{TYPE}}', {
    type: '{{TYPE}}', label: '{{LABEL}}', icon: 'fa-video',
    renderForm: TaskUI.defineForm({
        fields: [
            { type: 'text', key: 'keyword', label: '关键字', required: true },
        ]
    }),
    renderMeta: TaskUI.defineMeta({
        fields: [
            { type: 'text', key: 'keyword', label: '关键字', path: 'extra.keyword' },
        ]
    }),
    collectExtra: function(formData) {
        return { keyword: formData.keyword }
    },
    shouldShowViewer: function(obj) { return obj.status === 'completed' },
    onClick: function(obj, helpers) {
        if (obj.status !== 'completed') return false
        helpers.openTaskTypeViewer(obj)
        return true
    },
    renderViewer: function(h, obj, onClose) {
        var D = TaskUI.Data, M = TaskUI.Modal
        var videoUrl = D.getVideoUrl(obj), coverUrl = D.getCoverImage(obj)
        // 使用 M.create() 构建全功能 Modal
        // ...
    }
})
```

**变体 2：图片画廊（参考 vikacg）** — 类似结构，mediaType 为 'image'

**变体 3：纯表单（参考 urllist）** — 只有表单 + 元数据，无 viewer

### 脚手架脚本 (`scripts/new-task-type.sh`)

交互式脚本，接受参数：
- `type`（任务类型标识，如 `mytype`）
- `label`（显示标签，如 `My Type`）
- `hasForm`（y/n）
- `hasViewer`（y/n）
- `viewerType`（video/image/none）

自动创建目录结构，替换模板中的 `{{TYPE}}`/`{{LABEL}}` 占位符。

## 实施步骤

1. 创建 `shared/data.js` — 提取通用数据访问器
2. 创建 `shared/dom.js` — 提取 DOM 构建辅助
3. 创建 `shared/modal.js` — 提取 Modal 构建器
4. 修改 `index.html` — 加载新模块
5. 简化 `tktube/viewer.js` — 引用共享模块
6. 简化 `hanime/viewer.js` — 引用共享模块
7. 简化 `vikacg/viewer.js` — 引用共享模块
8. 简化 `urllist/viewer.js` — 引用共享 data 访问器
9. 创建 `task/TEMPLATE/` — 模板文件
10. 创建 `scripts/new-task-type.sh` — 脚手架脚本
11. 创建 `docs/new-task-checklist.md` — 开发指南
12. 测试验证

## 验证方式

1. `go build ./...` — 编译通过
2. `make run` — 启动 Web UI
3. 逐一测试 4 个任务类型的 viewer 功能正常
4. 验证 `__dm_pathToUrl` 委托路径正确
5. 验证 `scripts/new-task-type.sh` 生成的文件结构正确