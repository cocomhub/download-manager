# Task UI 共享工具函数与模板 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 提取 4 个现有 task 类型 viewer.js 中的重复代码为 3 个共享模块（data.js、dom.js、modal.js），简化现有 viewer，并提供新任务模板 + 脚手架脚本。

**架构：**
- 新建 `web/static/app/taskui/shared/` 目录存放 3 个共享模块，挂载到 `TaskUI.Data`、`TaskUI.Dom`、`TaskUI.Modal` 命名空间
- 现有 viewer 文件逐步引用共享模块替代重复代码
- 提供 `task/TEMPLATE/` 模板 + `scripts/new-task-type.sh` 脚手架脚本

**技术栈：** 纯 JavaScript（IIFE 模式，无构建工具），Go（embed.FS + init() 注册），Shell 脚本

**设计文档：** `docs/superpowers/specs/2026-07-28-task-ui-shared-utilities-design.md`

---

## 文件变更清单

### 新建文件
| 文件 | 职责 |
|------|------|
| `web/static/app/taskui/shared/data.js` | 通用数据访问器（~120 行） |
| `web/static/app/taskui/shared/dom.js` | DOM 构建辅助（~80 行） |
| `web/static/app/taskui/shared/modal.js` | Modal 构建器（~250 行） |
| `task/TEMPLATE/ui/ui.go` | Go 端注册模板 |
| `task/TEMPLATE/ui/assets/viewer.js` | JS 端模板（3 种变体注释） |
| `scripts/new-task-type.sh` | 脚手架生成脚本 |
| `docs/new-task-checklist.md` | 新任务开发指南 |

### 修改文件
| 文件 | 改动量 |
|------|--------|
| `web/static/index.html` | +3 行（加载 shared 模块） |
| `task/tktube/ui/assets/viewer.js` | 减少 ~60% |
| `task/hanime/ui/assets/viewer.js` | 减少 ~60% |
| `task/vikacg/ui/assets/viewer.js` | 减少 ~50% |
| `task/urllist/ui/assets/viewer.js` | 减少 ~20% |

---

### 任务 1：创建 `shared/data.js` — 统一数据访问器

**文件：** 创建 `web/static/app/taskui/shared/data.js`

- [ ] **步骤 1：编写 data.js 文件**

```js
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * 共享数据访问器。
 * 挂载到 TaskUI.Data 命名空间，供所有 task 类型 viewer 使用。
 * 依赖：window.__dm_pathToUrl（定义在 api.js），window.__dm_downloadRoot
 */
;(function () {
  'use strict'

  var Data = {}

  // ---- 通用访问器 ----

  Data.getTitle = function (obj) {
    return (obj && obj.metadata && obj.metadata.title) || ''
  }

  Data.getDate = function (obj) {
    if (obj && obj.extra && obj.extra.date) return obj.extra.date
    if (obj && obj.metadata && obj.metadata.date) return obj.metadata.date
    return ''
  }

  Data.getDuration = function (obj) {
    return (obj && obj.metadata && obj.metadata.duration) || ''
  }

  Data.getResolution = function (obj) {
    return (obj && obj.metadata && obj.metadata.resolution) || ''
  }

  Data.getContentGroup = function (obj) {
    return (obj && obj.metadata && obj.metadata.content_group) || ''
  }

  Data.getTags = function (obj) {
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

  Data.getOriginLink = function (obj) {
    if (obj && obj.metadata && obj.metadata.page_url) return obj.metadata.page_url
    if (obj && obj.extra && obj.extra.origin_url) return obj.extra.origin_url
    return (obj && obj.url) || ''
  }

  Data.getDetails = function (obj) {
    var s = ''
    if (obj && obj.extra && obj.extra.description) s = obj.extra.description
    else if (obj && obj.metadata && obj.metadata.description) s = obj.metadata.description
    else if (obj && obj.metadata && obj.metadata.details) s = obj.metadata.details
    else if (obj && obj.extra && obj.extra.details) s = obj.extra.details
    return (typeof s === 'string' ? s : '').replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim()
  }

  // ---- 文件路径转 URL ----

  Data.fileUrl = function (path) {
    return window.__dm_pathToUrl ? window.__dm_pathToUrl(path) : Data.fileUrlImpl(path)
  }

  Data.fileUrlImpl = function (path) {
    if (!path) return ''
    var normalized = path.replace(/\\/g, '/')
    var root = typeof window.__dm_downloadRoot === 'string' ? window.__dm_downloadRoot : ''
    if (root && normalized.indexOf(root) === 0) {
      normalized = normalized.slice(root.length)
    }
    normalized = normalized.replace(/^\//, '')
    return '/files/' + normalized.split('/').filter(function(s){return s&&s!=='..'}).map(encodeURIComponent).join('/')
  }

  // ---- 媒体 URL 获取 ----

  Data.getVideoUrl = function (obj) {
    if (!obj) return ''
    if (obj.extra && Array.isArray(obj.extra.files)) {
      for (var fi = 0; fi < obj.extra.files.length; fi++) {
        var f = obj.extra.files[fi]
        if (f && (f.type === 'video' || (f.path && /\.(mp4|webm|mkv|m3u8|ts)$/i.test(f.path)))) {
          if (f.path) return Data.fileUrl(f.path)
        }
      }
    }
    if (obj.save_path) return Data.fileUrl(obj.save_path)
    if (obj.url) return obj.url
    return ''
  }

  Data.getCoverImage = function (obj) {
    if (!obj) return ''
    if (obj.extra && obj.extra.local_cover) return Data.fileUrl(obj.extra.local_cover)
    if (obj.extra && obj.extra.cover_url) return obj.extra.cover_url
    if (obj.extra && obj.extra.cover) return obj.extra.cover
    if (obj.extra && Array.isArray(obj.extra.files)) {
      for (var fi = 0; fi < obj.extra.files.length; fi++) {
        var f = obj.extra.files[fi]
        if (f && f.type === 'image' && f.path) {
          var fname = (f.name || f.path || '').toString().toLowerCase()
          if (fname.indexOf('cover') >= 0) return Data.fileUrl(f.path)
        }
      }
      for (var fi = 0; fi < obj.extra.files.length; fi++) {
        var f = obj.extra.files[fi]
        if (f && f.type === 'image' && f.path) {
          var fname = (f.name || f.path || '').toString().toLowerCase()
          if (fname.indexOf('thumb') >= 0) return Data.fileUrl(f.path)
        }
      }
      for (var fi2 = 0; fi2 < obj.extra.files.length; fi2++) {
        var f2 = obj.extra.files[fi2]
        if (f2 && f2.type === 'image' && f2.path) return Data.fileUrl(f2.path)
      }
    }
    return ''
  }

  Data.getThumbImage = function (obj) {
    if (!obj) return ''
    if (obj.extra && obj.extra.local_preview) return Data.fileUrl(obj.extra.local_preview)
    if (obj.extra && obj.extra.local_cover) return Data.fileUrl(obj.extra.local_cover)
    if (obj.extra && obj.extra.thumb_url) return obj.extra.thumb_url
    if (obj.extra && obj.extra.preview_url) return obj.extra.preview_url
    if (obj.extra && obj.extra.cover_url) return obj.extra.cover_url
    if (obj.extra && Array.isArray(obj.extra.files)) {
      for (var fi = 0; fi < obj.extra.files.length; fi++) {
        var f = obj.extra.files[fi]
        if (f && f.type === 'image' && f.path) {
          var fname = (f.name || f.path || '').toString().toLowerCase()
          if (fname.indexOf('thumb') >= 0 && fname.indexOf('cover') < 0) return Data.fileUrl(f.path)
        }
      }
      for (var fi = 0; fi < obj.extra.files.length; fi++) {
        var f = obj.extra.files[fi]
        if (f && f.type === 'image' && f.path) {
          var fname = (f.name || f.path || '').toString().toLowerCase()
          if (fname.indexOf('cover') >= 0) return Data.fileUrl(f.path)
        }
      }
      for (var fi2 = 0; fi2 < obj.extra.files.length; fi2++) {
        var f2 = obj.extra.files[fi2]
        if (f2 && f2.type === 'image' && f2.path) return Data.fileUrl(f2.path)
      }
    }
    return ''
  }

  Data.getFileUrl = function (obj) {
    if (obj && obj.save_path) return Data.fileUrl(obj.save_path)
    if (obj && obj.extra && Array.isArray(obj.extra.files)) {
      for (var fi = 0; fi < obj.extra.files.length; fi++) {
        var f = obj.extra.files[fi]
        if (f && f.path) return Data.fileUrl(f.path)
      }
    }
    return ''
  }

  // ---- 状态颜色 ----

  Data.statusColor = function (status) {
    switch (status) {
      case 'completed': return '#10b981'
      case 'downloading': return '#3b82f6'
      case 'failed': return '#ef4444'
      case 'cancelled': return '#9ca3af'
      default: return '#6b7280'
    }
  }

  Data.statusBg = function (status) {
    switch (status) {
      case 'completed': return '#d1fae5'
      case 'downloading': return '#dbeafe'
      case 'failed': return '#fee2e2'
      case 'cancelled': return '#f3f4f6'
      default: return '#f3f4f6'
    }
  }

  Data.priorityScore = function (obj) {
    if (obj && obj.extra) {
      if (obj.extra.variant_priority !== undefined) return obj.extra.variant_priority
      if (obj.extra.priority !== undefined) return obj.extra.priority
    }
    var r = Data.getResolution(obj)
    if (/1080/.test(r)) return 30
    if (/720/.test(r)) return 20
    if (/480/.test(r)) return 10
    return 0
  }

  // ---- 剪贴板 ----

  Data.copyToClipboard = function (text) {
    if (!text) return
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).catch(function () {})
    } else {
      var ta = document.createElement('textarea')
      ta.value = text
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.select()
      try { document.execCommand('copy') } catch (e) {}
      document.body.removeChild(ta)
    }
  }

  window.TaskUI = window.TaskUI || {}
  window.TaskUI.Data = Data
})()
```

- [ ] **步骤 2：验证 data.js 加载正常**

```
确认文件语法正确，无拼写错误。可在浏览器控制台执行 `TaskUI.Data.getTitle({metadata:{title:'test'}})` 验证返回 'test'。
```

- [ ] **步骤 3：Commit**

```bash
git add web/static/app/taskui/shared/data.js
git commit -m "feat: add shared data accessor module (TaskUI.Data)"
```

---

### 任务 2：创建 `shared/dom.js` — DOM 构建辅助

**文件：** 创建 `web/static/app/taskui/shared/dom.js`

- [ ] **步骤 1：编写 dom.js 文件**

```js
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * 共享 DOM 构建辅助函数。
 * 挂载到 TaskUI.Dom 命名空间。
 */
;(function () {
  'use strict'

  var Dom = {}

  /**
   * 创建标签 chip 集合
   * @param {string[]} tags
   * @returns {DocumentFragment}
   */
  Dom.createTagChips = function (tags) {
    var frag = document.createDocumentFragment()
    if (!tags || tags.length === 0) return frag
    tags.forEach(function (tag) {
      var t = document.createElement('span')
      t.style.cssText = 'font-size:11px;background:#f3f4f6;color:#4b5563;padding:2px 8px;border-radius:4px'
      t.textContent = '#' + tag
      frag.appendChild(t)
    })
    return frag
  }

  /**
   * 创建信息条（图标 + 文本）
   * @param {Array<{icon: string, text: string}>} items
   * @returns {HTMLElement}
   */
  Dom.createInfoBar = function (items) {
    var bar = document.createElement('div')
    bar.style.cssText = 'display:flex;gap:16px;padding:12px 16px;background:#f9fafb;border-bottom:1px solid #e5e7eb;font-size:13px;color:#6b7280;flex-wrap:wrap'
    if (!items || items.length === 0) return bar
    items.forEach(function (item) {
      if (!item.text) return
      var el = document.createElement('span')
      el.innerHTML = '<i class="' + (item.icon || 'fas fa-circle') + '" style="margin-right:4px"></i> ' + item.text
      bar.appendChild(el)
    })
    return bar
  }

  /**
   * 创建按钮
   * @param {string} text
   * @param {function} onClick
   * @param {object} opts — { primary?: boolean, style?: string, className?: string }
   * @returns {HTMLElement}
   */
  Dom.createButton = function (text, onClick, opts) {
    opts = opts || {}
    var btn = document.createElement('button')
    if (opts.primary) {
      btn.style.cssText = 'padding:6px 12px;border-radius:6px;background:#2563eb;color:#fff;border:none;cursor:pointer;font-size:14px'
    } else {
      btn.style.cssText = 'padding:6px 12px;border-radius:6px;background:#fff;border:1px solid #d1d5db;cursor:pointer;font-size:14px;color:#374151'
    }
    if (opts.style) btn.style.cssText = opts.style
    btn.textContent = text
    if (onClick) btn.onclick = onClick
    return btn
  }

  /**
   * 创建链接
   * @param {string} href
   * @param {string} text
   * @param {object} opts — { style?: string }
   * @returns {HTMLElement}
   */
  Dom.createLink = function (href, text, opts) {
    opts = opts || {}
    var a = document.createElement('a')
    a.href = /^https?:\/\//i.test(href) ? href : '#'
    a.target = '_blank'
    a.rel = 'noopener noreferrer'
    a.style.cssText = opts.style || 'padding:6px 12px;border-radius:6px;background:#fff;border:1px solid #d1d5db;text-decoration:none;cursor:pointer;font-size:14px;color:#374151;display:inline-block'
    a.textContent = text || href
    return a
  }

  /**
   * 创建徽章
   * @param {string} text
   * @param {string} bgColor — 背景色 CSS 值
   * @param {string} textColor — 文字色 CSS 值
   * @returns {HTMLElement}
   */
  Dom.createBadge = function (text, bgColor, textColor) {
    var badge = document.createElement('span')
    badge.style.cssText = 'font-size:11px;padding:2px 8px;border-radius:4px;background:' + (bgColor || '#f3f4f6') + ';color:' + (textColor || '#4b5563')
    badge.textContent = text
    return badge
  }

  /**
   * 创建图标元素
   * @param {string} iconClass — 如 'fa-video'
   * @returns {HTMLElement}
   */
  Dom.createIcon = function (iconClass) {
    var i = document.createElement('i')
    i.className = 'fas ' + (iconClass || 'fa-cube')
    return i
  }

  window.TaskUI = window.TaskUI || {}
  window.TaskUI.Dom = Dom
})()
```

- [ ] **步骤 2：Commit**

```bash
git add web/static/app/taskui/shared/dom.js
git commit -m "feat: add shared DOM helper module (TaskUI.Dom)"
```

---

### 任务 3：创建 `shared/modal.js` — Modal 构建器

**文件：** 创建 `web/static/app/taskui/shared/modal.js`

- [ ] **步骤 1：编写 modal.js 文件**

```js
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * 共享 Modal 构建器。
 * 挂载到 TaskUI.Modal 命名空间，提供全功能的 DOM Modal 骨架。
 * 依赖：TaskUI.Data, TaskUI.Dom, CollectionPanel, RecommendationPanel, AppAPI
 */
;(function () {
  'use strict'

  var Modal = {}

  /**
   * 创建半透明黑色背景 overlay
   * @returns {HTMLElement}
   */
  Modal.createOverlay = function () {
    var overlay = document.createElement('div')
    overlay.className = 'fixed inset-0 bg-black bg-opacity-70 z-50 flex items-center justify-center p-4 backdrop-blur-sm'
    overlay.style.cssText = 'position:fixed;inset:0;background:rgba(0,0,0,0.7);z-index:50;display:flex;align-items:center;justify-content:center;padding:16px;-webkit-backdrop-filter:blur(4px);backdrop-filter:blur(4px)'
    return overlay
  }

  /**
   * 创建白色圆角面板
   * @param {string} maxWidth — 如 '1400px'
   * @returns {HTMLElement}
   */
  Modal.createPanel = function (maxWidth) {
    var panel = document.createElement('div')
    panel.className = 'bg-white rounded-lg shadow-2xl w-full max-h-[90vh] overflow-hidden flex flex-col'
    panel.style.cssText = 'background:#fff;border-radius:8px;box-shadow:0 25px 50px rgba(0,0,0,0.25);width:100%;max-width:' + (maxWidth || '1400px') + ';max-height:90vh;overflow:hidden;display:flex;flex-direction:column'
    return panel
  }

  /**
   * 创建 header
   * @param {object} config
   * @param {string} config.title — 标题
   * @param {function} config.onClose — 关闭回调
   * @param {Array<{text:string, bg:string, color:string}>} config.badges — 右侧徽章列表
   * @returns {HTMLElement}
   */
  Modal.createHeader = function (config) {
    var header = document.createElement('div')
    header.style.cssText = 'padding:16px;border-bottom:1px solid #e5e7eb;display:flex;justify-content:space-between;align-items:center;background:#f9fafb'

    // Title
    var hTitle = document.createElement('h3')
    hTitle.style.cssText = 'font-size:18px;font-weight:700;color:#1f2937;margin:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap'
    hTitle.className = 'viewer-title'
    hTitle.textContent = config.title || ''
    header.appendChild(hTitle)

    // Right side: badges + close button
    var right = document.createElement('div')
    right.style.cssText = 'display:flex;align-items:center;gap:12px;flex-shrink:0'

    // Badges
    if (config.badges && config.badges.length > 0) {
      config.badges.forEach(function (b) {
        var badge = document.createElement('span')
        badge.style.cssText = 'font-size:11px;padding:2px 8px;border-radius:4px;background:' + (b.bg || '#f3f4f6') + ';color:' + (b.color || '#4b5563')
        badge.textContent = b.text || ''
        right.appendChild(badge)
      })
    }

    // Close button
    if (config.onClose) {
      var hClose = document.createElement('button')
      hClose.innerHTML = '<i class="fas fa-times"></i>'
      hClose.style.cssText = 'color:#6b7280;cursor:pointer;background:none;border:none;font-size:18px;margin-left:8px'
      hClose.onclick = function (e) { e.stopPropagation(); config.onClose() }
      right.appendChild(hClose)
    }

    header.appendChild(right)
    return header
  }

  /**
   * 创建 footer
   * @param {object} config
   * @param {Array<HTMLElement>} config.leftButtons — 左侧按钮数组
   * @param {string} config.closeText — 关闭按钮文字，默认 '关闭'
   * @param {function} config.onClose — 关闭回调
   * @returns {HTMLElement}
   */
  Modal.createFooter = function (config) {
    var footer = document.createElement('div')
    footer.style.cssText = 'padding:12px 16px;border-top:1px solid #e5e7eb;background:#f9fafb;display:flex;justify-content:space-between;align-items:center'

    // Left side: action buttons
    var fLeft = document.createElement('div')
    fLeft.style.cssText = 'display:flex;gap:8px'
    if (config.leftButtons && config.leftButtons.length > 0) {
      config.leftButtons.forEach(function (btn) {
        fLeft.appendChild(btn)
      })
    }
    footer.appendChild(fLeft)

    // Close button
    if (config.onClose) {
      var closeBtn = document.createElement('button')
      closeBtn.style.cssText = 'padding:6px 12px;border-radius:6px;background:#fff;border:1px solid #d1d5db;cursor:pointer;font-size:14px;color:#374151'
      closeBtn.textContent = config.closeText || '关闭'
      closeBtn.onclick = function (e) { e.stopPropagation(); config.onClose() }
      footer.appendChild(closeBtn)
    }

    return footer
  }

  /**
   * 创建视频区域（16:9 比例，poster + play overlay + video）
   * @param {object} config
   * @param {string} config.videoUrl — 视频 URL
   * @param {string} config.coverUrl — 封面 URL
   * @param {string} config.title — 标题
   * @returns {HTMLElement} 视频区域 div
   */
  Modal.createVideoArea = function (config) {
    var area = document.createElement('div')
    area.style.cssText = 'background:#000;display:flex;align-items:center;justify-content:center;position:relative;aspect-ratio:16/9;overflow:hidden'
    area.className = 'viewer-media-area'

    var isHLS = /\.m3u8(\?.*)?$/i.test(config.videoUrl || '')
    var isSafari = /safari/i.test(navigator.userAgent) && !/chrome|crios|chromium|edg/i.test(navigator.userAgent)
    var useVideo = !!config.videoUrl && (!isHLS || isSafari)

    if (useVideo) {
      // Poster image
      var posterImg = document.createElement('img')
      posterImg.src = config.coverUrl || config.videoUrl
      posterImg.style.cssText = 'width:100%;height:100%;object-fit:contain;cursor:pointer'
      posterImg.alt = config.title || ''
      area.appendChild(posterImg)

      // Play overlay
      var playOverlay = document.createElement('div')
      playOverlay.style.cssText = 'position:absolute;inset:0;display:flex;align-items:center;justify-content:center;background:rgba(0,0,0,0.2);cursor:pointer'
      playOverlay.innerHTML = '<i class="fas fa-play" style="font-size:48px;color:#fff;opacity:0.8;text-shadow:0 2px 8px rgba(0,0,0,0.5)"></i>'
      area.appendChild(playOverlay)

      // Video element
      var video = document.createElement('video')
      video.src = config.videoUrl
      video.poster = config.coverUrl || ''
      video.controls = true
      video.style.cssText = 'width:100%;height:100%;outline:none;display:none'
      video.classList.add('dm-video-player')

      var playHandler = function () {
        posterImg.style.display = 'none'
        playOverlay.style.display = 'none'
        video.style.display = 'block'
        video.play().catch(function () {})
      }
      posterImg.onclick = playHandler
      playOverlay.onclick = playHandler
      area.appendChild(video)
    } else if (config.videoUrl) {
      // HLS or unsupported — show poster only
      var posterImg = document.createElement('img')
      posterImg.src = config.coverUrl || ''
      posterImg.style.cssText = 'width:100%;height:100%;object-fit:contain'
      posterImg.alt = config.title || ''
      if (posterImg.src) area.appendChild(posterImg)
    } else {
      // No video — placeholder
      var placeholder = document.createElement('div')
      placeholder.style.cssText = 'display:flex;align-items:center;justify-content:center;height:100%;color:#9ca3af;font-size:14px'
      placeholder.innerHTML = '<i class="fas fa-video" style="font-size:48px;margin-right:12px;opacity:0.5"></i> 无可用视频'
      area.appendChild(placeholder)
    }

    // Add CSS for semi-transparent controls
    if (!document.getElementById('dm-video-player-style')) {
      var style = document.createElement('style')
      style.id = 'dm-video-player-style'
      style.textContent = '.dm-video-player::-webkit-media-controls { opacity:0.6 !important; transition:opacity 0.3s } .dm-video-player::-webkit-media-controls:hover { opacity:1 !important } .dm-video-player::-webkit-media-controls-panel { background:rgba(0,0,0,0.3) !important }'
      document.head.appendChild(style)
    }

    return area
  }

  /**
   * 创建右侧合集/推荐面板侧栏
   * @param {object} config
   * @param {string} config.type — 任务类型
   * @param {string|number} config.currentId — 当前对象 ID
   * @param {string[]} config.tags — 标签列表
   * @param {function} config.onPlayItem — 播放/切换回调
   * @returns {{ element: HTMLElement, collectionPanel: object|null, recommendationPanel: object|null }}
   */
  Modal.createSidebar = function (config) {
    var rightCol = document.createElement('div')
    rightCol.style.cssText = 'width:380px;border-left:1px solid #e5e7eb;display:flex;flex-direction:column;overflow:hidden;flex-shrink:0;background:#fff'

    var result = { element: rightCol, collectionPanel: null, recommendationPanel: null }

    if (!config.type || !config.currentId) return result

    // Collection panel
    result.collectionPanel = window.CollectionPanel && window.CollectionPanel.create({
      type: config.type,
      currentId: config.currentId,
      onPlayItem: function (item) {
        if (config.onPlayItem) config.onPlayItem(item, 'collection')
      }
    })
    if (result.collectionPanel) {
      rightCol.appendChild(result.collectionPanel.element)
    }

    // Recommendation panel
    result.recommendationPanel = window.RecommendationPanel && window.RecommendationPanel.create({
      type: config.type,
      currentId: config.currentId,
      tags: config.tags || [],
      onPlayItem: function (item) {
        if (config.onPlayItem) config.onPlayItem(item, 'recommendation')
      }
    })
    if (result.recommendationPanel) {
      rightCol.appendChild(result.recommendationPanel.element)
    }

    return result
  }

  /**
   * 设置关闭事件处理器（backdrop 点击 + ESC 键盘）
   * @param {object} config
   * @param {HTMLElement} config.overlay — overlay 元素
   * @param {function} config.onClose — 关闭回调
   * @param {function} config.cleanup — 额外清理回调
   * @returns {function} keyHandler — 供后续移除
   */
  Modal.setupCloseHandlers = function (config) {
    // Backdrop click
    config.overlay.addEventListener('click', function (e) {
      if (e.target === config.overlay && config.onClose) config.onClose()
    })

    // ESC key
    function keyHandler(e) {
      if (e.key === 'Escape' && config.onClose) config.onClose()
    }
    document.addEventListener('keydown', keyHandler)
    return keyHandler
  }

  /**
   * 创建完整 modal（全功能构建器）
   * 返回 { overlay, panel, close, cleanup }
   */
  Modal.create = function (obj, config) {
    config = config || {}
    var D = TaskUI.Data
    var title = config.title || D.getTitle(obj) || ''

    var overlay = Modal.createOverlay()
    var panel = Modal.createPanel(config.maxWidth || '1400px')
    overlay.appendChild(panel)

    // Header
    var header = Modal.createHeader({
      title: title,
      onClose: config.onClose,
      badges: config.badges || []
    })
    panel.appendChild(header)

    // Body (two-column layout)
    var body = document.createElement('div')
    body.style.cssText = 'flex:1;overflow:hidden;padding:0;display:flex'

    // Left column
    var leftCol = document.createElement('div')
    leftCol.style.cssText = 'flex:1;overflow-y:auto'

    // Media area (if configured)
    if (config.mediaType === 'video' && config.videoUrl) {
      leftCol.appendChild(Modal.createVideoArea({
        videoUrl: config.videoUrl,
        coverUrl: config.coverUrl,
        title: title
      }))
    }

    // Info bar
    if (config.infoBar && config.infoBar.length > 0) {
      leftCol.appendChild(TaskUI.Dom.createInfoBar(config.infoBar))
    }

    // Content renderer
    if (config.contentRenderer) {
      var contentDiv = document.createElement('div')
      contentDiv.style.cssText = 'padding:16px'
      config.contentRenderer(contentDiv, obj)
      leftCol.appendChild(contentDiv)
    }

    body.appendChild(leftCol)

    // Right sidebar
    var sidebarResult = null
    if (config.sidebar === 'collection') {
      sidebarResult = Modal.createSidebar({
        type: config.type,
        currentId: config.currentId,
        tags: config.tags,
        onPlayItem: config.onPlayItem
      })
      body.appendChild(sidebarResult.element)
    }

    panel.appendChild(body)

    // Footer
    var footer = Modal.createFooter({
      leftButtons: config.footerActions || [],
      closeText: config.closeText || '关闭',
      onClose: config.onClose
    })
    panel.appendChild(footer)

    // Close handlers
    var keyHandler = Modal.setupCloseHandlers({
      overlay: overlay,
      onClose: config.onClose
    })

    // Store references for cleanup
    var preCleanup = config.onClose
    config.onClose = function () {
      document.removeEventListener('keydown', keyHandler)
      if (sidebarResult) {
        if (sidebarResult.collectionPanel) sidebarResult.collectionPanel.destroy()
        if (sidebarResult.recommendationPanel) sidebarResult.recommendationPanel.destroy()
      }
      if (overlay && overlay.parentNode) overlay.parentNode.removeChild(overlay)
      document.body.style.overflow = ''
      if (preCleanup) preCleanup()
    }

    // Mount
    document.body.appendChild(overlay)
    document.body.style.overflow = 'hidden'

    return { overlay: overlay, panel: panel, close: config.onClose }
  }

  window.TaskUI = window.TaskUI || {}
  window.TaskUI.Modal = Modal
})()
```

- [ ] **步骤 2：验证 modal.js 依赖可用**

```
确认 TaskUI.Data、TaskUI.Dom、CollectionPanel、RecommendationPanel 在运行时已加载。
Modal.create() 返回的 { overlay, panel, close } 结构正确。
```

- [ ] **步骤 3：Commit**

```bash
git add web/static/app/taskui/shared/modal.js
git commit -m "feat: add shared modal builder (TaskUI.Modal)"
```

---

### 任务 4：修改 `index.html` 加载共享模块

**文件：** 修改 `web/static/index.html`

- [ ] **步骤 1：在 `index.html` 的 `loader.js` 后添加 shared 模块加载**

找到 `web/static/index.html` 中 `app/taskui/loader.js` 的 `<script>` 标签，在其后添加：

```html
    <!-- Shared UI utilities -->
    <script src="app/taskui/shared/data.js"></script>
    <script src="app/taskui/shared/dom.js"></script>
    <script src="app/taskui/shared/modal.js"></script>
```

加载顺序：data.js（无依赖）→ dom.js（无依赖）→ modal.js（依赖 Data + Dom）

- [ ] **步骤 2：Commit**

```bash
git add web/static/index.html
git commit -m "feat: load shared UI utility modules in index.html"
```

---

### 任务 5：简化 `tktube/viewer.js` — 引用共享模块

**文件：** 修改 `task/tktube/ui/assets/viewer.js`

- [ ] **步骤 1：删除重复的辅助函数，替换为 TaskUI.Data 引用**

删除以下重复函数（共 ~175 行）：
- `getTitle()` → `TaskUI.Data.getTitle(obj)`
- `getContentGroup()` → `TaskUI.Data.getContentGroup(obj)`
- `getResolution()` → `TaskUI.Data.getResolution(obj)`
- `getDate()` → `TaskUI.Data.getDate(obj)`
- `getDuration()` → `TaskUI.Data.getDuration(obj)`
- `getOriginLink()` → `TaskUI.Data.getOriginLink(obj)`
- `getTags()` → `TaskUI.Data.getTags(obj)`
- `getDetails()` → `TaskUI.Data.getDetails(obj)`
- `fileUrl()` / `fileUrl_impl()` → `TaskUI.Data.fileUrl(path)`
- `getVideoUrl()` → `TaskUI.Data.getVideoUrl(obj)`
- `getCoverImage()` → `TaskUI.Data.getCoverImage(obj)`
- `getThumbImage()` → `TaskUI.Data.getThumbImage(obj)`
- `getFileUrl()` → `TaskUI.Data.getFileUrl(obj)`
- `statusColor()` / `statusBg()` → `TaskUI.Data.statusColor()` / `statusBg()`
- `priorityScore()` → `TaskUI.Data.priorityScore(obj)`
- `THEME` 常量 → 删除（未在外部使用）

更新 `renderTaskView()` 和 `createObjectCard()` 中的引用：`getTitle(obj)` → `TaskUI.Data.getTitle(obj)`。

- [ ] **步骤 2：简化 renderViewer 中的重复代码**

在 `renderViewer` 中：
- 删除内联 `copyToClipboard()` → 使用 `TaskUI.Data.copyToClipboard(text)`
- 删除 Modal 骨架 DOM 构建（overlay/panel/header/body/footer/backdrop/ESC 的 ~200 行）→ 使用 `TaskUI.Modal.create()` 替代
- 保留：视频区域构建逻辑（可复用 `TaskUI.Modal.createVideoArea()`）
- 保留：合集/推荐面板创建（可复用 `TaskUI.Modal.createSidebar()`）

简化后的 `renderViewer` 代码结构：

```js
renderViewer: function (h, obj, onClose) {
  var D = TaskUI.Data, Dm = TaskUI.Dom, M = TaskUI.Modal

  var videoUrl = D.getVideoUrl(obj)
  var coverUrl = D.getCoverImage(obj)
  var title = D.getTitle(obj) || 'TKTube'
  var res = D.getResolution(obj)
  var dur = D.getDuration(obj)
  var dateVal = D.getDate(obj)
  var contentGroup = D.getContentGroup(obj)
  var origin = D.getOriginLink(obj)
  var details = D.getDetails(obj)
  var tags = D.getTags(obj)
  var fileUrlVal = D.getFileUrl(obj)
  var taskType = obj && obj.metadata && obj.metadata.task_type
  var objTags = (obj && obj.extra && Array.isArray(obj.extra.tags)) ? obj.extra.tags : []

  var modal = M.create(obj, {
    title: title,
    badges: [
      contentGroup ? { text: contentGroup, bg: '#eff6ff', color: '#2563eb' } : null,
      res ? { text: res, bg: '#f3f4f6', color: '#4b5563' } : null
    ].filter(Boolean),
    mediaType: 'video',
    videoUrl: videoUrl,
    coverUrl: coverUrl,
    infoBar: [
      dur ? { icon: 'fas fa-clock', text: dur } : null,
      dateVal ? { icon: 'fas fa-calendar', text: dateVal } : null,
      res ? { icon: 'fas fa-expand', text: res } : null,
      contentGroup ? { icon: 'fas fa-folder', text: contentGroup } : null
    ].filter(Boolean),
    contentRenderer: function (contentDiv) {
      // Details
      if (details) {
        var de = document.createElement('div')
        de.style.cssText = 'font-size:14px;color:#374151;line-height:1.6;white-space:pre-line;margin-bottom:12px'
        de.textContent = details
        contentDiv.appendChild(de)
      }
      // Tags
      if (tags.length > 0) {
        var tagWrap = document.createElement('div')
        tagWrap.style.cssText = 'display:flex;flex-wrap:wrap;gap:4px;margin-bottom:12px'
        tagWrap.appendChild(Dm.createTagChips(tags))
        contentDiv.appendChild(tagWrap)
      }
    },
    sidebar: 'collection',
    type: taskType,
    currentId: obj.id,
    tags: objTags,
    onPlayItem: function (item) {
      AppAPI.getObject(taskType, item.id).then(function (newObj) {
        modal.close()
        // Re-create modal with new object
        // (in practice, this is handled by the viewer's own onPlayItem)
      })
    },
    footerActions: [
      fileUrlVal ? Dm.createLink(fileUrlVal, '打开文件', { style: 'padding:6px 12px;border-radius:6px;background:#2563eb;color:#fff;text-decoration:none;font-size:14px;display:inline-block' }) : null,
      origin ? Dm.createLink(origin, '打开原页面') : null,
      Dm.createButton('复制标题', function () { D.copyToClipboard(D.getTitle(obj)) }),
      origin ? Dm.createButton('复制链接', function () { D.copyToClipboard(origin) }) : null
    ].filter(Boolean),
    onClose: onClose
  })

  return h('div')
}
```

- 保留 `renderTaskView()` 和 `createObjectCard()` 函数（这些是 tktube 特有的内容分组视图，不在共享模块范围内）
- 保留 `TaskUI.register()` 注册块

- [ ] **步骤 3：验证简化后功能正常**

```
启动 Web UI，打开 tktube 类型的 completed 对象，确认：
- Modal 正常打开，header/footer 正常
- 视频区域 poster + play overlay 正常
- 点击播放后视频正常播放
- 合集/推荐面板正常显示
- ESC/backdrop 关闭正常
- 复制标题/链接功能正常
```

- [ ] **步骤 4：Commit**

```bash
git add task/tktube/ui/assets/viewer.js
git commit -m "refactor: simplify tktube viewer with shared TaskUI.Data/Dom/Modal"
```

---

### 任务 6：简化 `hanime/viewer.js` — 引用共享模块

**文件：** 修改 `task/hanime/ui/assets/viewer.js`

- [ ] **步骤 1：删除重复的通用辅助函数，替换为 TaskUI.Data 引用**

删除以下重复函数（共 ~170 行）：
- `getTitle()` → `TaskUI.Data.getTitle(obj)`
- `getTags()` → `TaskUI.Data.getTags(obj)`
- `getArtist()` → **保留**（hanime 特有）
- `getDescription()` → `TaskUI.Data.getDetails(obj)`（注意：hanime 的 fallback 顺序不同，保留 `getDescription` 作为包装）
- `getOriginLink()` → `TaskUI.Data.getOriginLink(obj)`
- `getDetails()` → `TaskUI.Data.getDetails(obj)`
- `getDate()` → `TaskUI.Data.getDate(obj)`
- `fileUrl()` / `fileUrl_impl()` → `TaskUI.Data.fileUrl(path)`
- `getVideoURL()` → `TaskUI.Data.getVideoUrl(obj)`（注意 hanime 的额外回退逻辑：local_url/file_url/path，需确认共享版是否覆盖）
- `getCoverImages()` → **保留**（hanime 特有，返回数组）
- `getThumbImages()` → **保留**（hanime 特有，返回数组）
- `getGenres()` → **保留**（hanime 特有）
- `getPlaylist()` → **保留**（hanime 特有）
- `statusColor()` / `statusBg()` → `TaskUI.Data.statusColor() / statusBg()`

注意：hanime 的 `getVideoURL` 比共享版 `getVideoUrl` 多了 `local_url`、`file_url`、`path` 回退。在共享版 `getVideoUrl` 确认覆盖这些回退后，方可删除 hanime 版本。

- [ ] **步骤 2：简化 renderViewer 中的重复代码**

类似 tktube，替换 Modal 骨架和复制功能为共享模块。

- 替换 `copyToClipboard()` → `TaskUI.Data.copyToClipboard(text)`
- 替换 Modal 骨架 DOM 构建 → `TaskUI.Modal.create()` + `createVideoArea()`
- 保留：genres 元数据行、artist 显示、playlist 等 hanime 特有内容

- [ ] **步骤 3：验证简化后功能正常**

```
启动 Web UI，打开 hanime 类型的 completed 对象，确认：
- Modal 正常打开，header/footer 正常
- 视频区域正常
- 合集/推荐面板正常
- ESC/backdrop 关闭正常
```

- [ ] **步骤 4：Commit**

```bash
git add task/hanime/ui/assets/viewer.js
git commit -m "refactor: simplify hanime viewer with shared TaskUI.Data/Dom/Modal"
```

---

### 任务 7：简化 `vikacg/viewer.js` — 引用共享模块

**文件：** 修改 `task/vikacg/ui/assets/viewer.js`

- [ ] **步骤 1：删除重复的通用辅助函数，替换为 TaskUI.Data 引用**

删除以下重复函数：
- `getTags()` → `TaskUI.Data.getTags(obj)`
- `getTitle()` → `TaskUI.Data.getTitle(obj)`
- `getDate()` → `TaskUI.Data.getDate(obj)`
- `fileUrl()` / `fileUrl_impl()` → `TaskUI.Data.fileUrl(path)`
- `copyToClipboard()` → `TaskUI.Data.copyToClipboard(text)`

保留（vikacg 特有）：
- `getImages()` — 特有逻辑（local files + remote images）
- `getLinks()` — 特有逻辑
- `getExcerpt()` — 特有逻辑
- `getContentHtml()` — 特有逻辑

- [ ] **步骤 2：简化 Modal 骨架**

- 替换 Modal 骨架 DOM 构建 → `TaskUI.Modal.createOverlay()` + `createPanel()` + `createHeader()` + `createFooter()`
- 保留：图片画廊特有的左右箭头导航、缩略图条、currentIdx 管理
- 替换 backdrop/ESC 关闭 → `TaskUI.Modal.setupCloseHandlers()`
- 替换左侧主体内容中的标签 chip → `TaskUI.Dom.createTagChips()`

- [ ] **步骤 3：验证简化后功能正常**

```
启动 Web UI，打开 vikacg 类型的 completed 对象，确认：
- Modal 正常打开，header/footer 正常
- 图片画廊正常显示，左右箭头切换正常
- 缩略图条正常，点击切换正常
- 右侧相关链接正常
- 键盘 ArrowLeft/ArrowRight 导航正常
- ESC 关闭正常
```

- [ ] **步骤 4：Commit**

```bash
git add task/vikacg/ui/assets/viewer.js
git commit -m "refactor: simplify vikacg viewer with shared TaskUI.Data/Dom/Modal"
```

---

### 任务 8：简化 `urllist/viewer.js` — 引用共享 data 访问器

**文件：** 修改 `task/urllist/ui/assets/viewer.js`

- [ ] **步骤 1：删除重复的 fileUrl 函数，替换为共享模块**

urllist 的 viewer 只有 `renderForm` 和 `renderMeta`，无 viewer。但文件开头定义了 `fileUrl_impl()`。删除该函数，使用 `TaskUI.Data.fileUrl()`（如果确实被引用）。

- [ ] **步骤 2：确认 urllist 的 `collectExtra` 和 `renderForm` 不需要修改**

urllist 的 `collectExtra` 是 `{ urls_text: formData.urls_text }`，完全独立，无需修改。

- [ ] **步骤 3：Commit**

```bash
git add task/urllist/ui/assets/viewer.js
git commit -m "refactor: simplify urllist viewer with shared TaskUI.Data.fileUrl"
```

---

### 任务 9：创建 `task/TEMPLATE/` 模板文件

**文件：** 创建 `task/TEMPLATE/ui/ui.go` 和 `task/TEMPLATE/ui/assets/viewer.js`

- [ ] **步骤 1：创建 `task/TEMPLATE/ui/ui.go`**

```go
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package ui registers {{TYPE}} custom UI assets via the TaskUIAssets framework.
//
// 使用方式：将本文件复制到 task/<your-type>/ui/ui.go，替换 {{TYPE}} 和 {{LABEL}}。
// 同时创建 task/<your-type>/ui/assets/viewer.js 编写 UI 插件代码。
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
		// HasForm: true,      // 是否有扩展表单（新建任务弹窗中的额外字段）
		// HasViewer: true,    // 是否有自定义查看器（点击对象时弹窗）
		// HasAggregate: true, // 是否有聚合视图
	})
}
```

- [ ] **步骤 2：创建 `task/TEMPLATE/ui/assets/viewer.js` — 3 种变体模板**

```js
/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 *
 * {{TYPE}} Task UI 插件
 *
 * 依赖：
 *   - TaskUI.Data（通用数据访问器）
 *   - TaskUI.Dom（DOM 构建辅助）
 *   - TaskUI.Modal（Modal 构建器）
 *
 * 使用方式：
 *   1. 替换文件中所有 {{TYPE}} 为实际类型名
 *   2. 替换 {{LABEL}} 为显示标签
 *   3. 按需取消注释以下变体之一的代码块
 *   4. 移除不需要的变体
 */

;(function () {
  'use strict'

  // =============================================
  // 变体 1：视频播放器（参考 tktube / hanime）
  // =============================================
  // 取消注释以下代码块即可使用

  // if (typeof TaskUI !== 'undefined' && TaskUI.register) {
  //   TaskUI.register('{{TYPE}}', {
  //     type: '{{TYPE}}',
  //     label: '{{LABEL}}',
  //     icon: 'fa-video', // 图标：fa-video / fa-film / fa-image / fa-link 等
  //     viewerLabel: '查看',
  //
  //     // ---- 表单（可选） ----
  //     renderForm: TaskUI.defineForm({
  //       fields: [
  //         { type: 'text', key: 'keyword', label: '关键字', required: true, placeholder: '搜索关键词' },
  //         { type: 'number', key: 'max_concurrent', label: '并发数', min: 1, max: 10, default: 2 },
  //       ]
  //     }),
  //
  //     // ---- 元数据（可选） ----
  //     renderMeta: TaskUI.defineMeta({
  //       fields: [
  //         { type: 'text', key: 'keyword', label: '关键字', path: 'extra.keyword' },
  //         { type: 'text', key: 'max_concurrent', label: '并发', path: 'extra.max_concurrent' },
  //       ]
  //     }),
  //
  //     // ---- 表单数据映射 ----
  //     collectExtra: function (formData) {
  //       var extra = {}
  //       if (formData.keyword) extra.keyword = formData.keyword
  //       if (formData.max_concurrent) extra.max_concurrent = formData.max_concurrent
  //       return extra
  //     },
  //
  //     // ---- 查看器条件 ----
  //     shouldShowViewer: function (obj) { return obj.status === 'completed' },
  //
  //     // ---- 点击处理 ----
  //     onClick: function (obj, helpers) {
  //       if (obj.status !== 'completed') return false
  //       helpers.openTaskTypeViewer(obj)
  //       return true
  //     },
  //
  //     // ---- 查看器渲染 ----
  //     renderViewer: function (h, obj, onClose) {
  //       var D = TaskUI.Data, Dm = TaskUI.Dom, M = TaskUI.Modal
  //
  //       var videoUrl = D.getVideoUrl(obj)
  //       var coverUrl = D.getCoverImage(obj)
  //       var title = D.getTitle(obj) || '{{LABEL}}'
  //       var dur = D.getDuration(obj)
  //       var dateVal = D.getDate(obj)
  //       var origin = D.getOriginLink(obj)
  //       var details = D.getDetails(obj)
  //       var tags = D.getTags(obj)
  //       var fileUrlVal = D.getFileUrl(obj)
  //       var taskType = obj && obj.metadata && obj.metadata.task_type
  //       var objTags = (obj && obj.extra && Array.isArray(obj.extra.tags)) ? obj.extra.tags : []
  //
  //       var modal = M.create(obj, {
  //         title: title,
  //         mediaType: 'video',
  //         videoUrl: videoUrl,
  //         coverUrl: coverUrl,
  //         infoBar: [
  //           dur ? { icon: 'fas fa-clock', text: dur } : null,
  //           dateVal ? { icon: 'fas fa-calendar', text: dateVal } : null,
  //         ].filter(Boolean),
  //         contentRenderer: function (contentDiv) {
  //           if (details) {
  //             var de = document.createElement('div')
  //             de.style.cssText = 'font-size:14px;color:#374151;line-height:1.6;white-space:pre-line;margin-bottom:12px'
  //             de.textContent = details
  //             contentDiv.appendChild(de)
  //           }
  //           if (tags.length > 0) {
  //             var tagWrap = document.createElement('div')
  //             tagWrap.style.cssText = 'display:flex;flex-wrap:wrap;gap:4px;margin-bottom:12px'
  //             tagWrap.appendChild(Dm.createTagChips(tags))
  //             contentDiv.appendChild(tagWrap)
  //           }
  //         },
  //         sidebar: 'collection',
  //         type: taskType,
  //         currentId: obj.id,
  //         tags: objTags,
  //         onPlayItem: function (item) {
  //           AppAPI.getObject(taskType, item.id).then(function (newObj) {
  //             modal.close()
  //             // 重新打开新对象的 viewer
  //             TaskUI.get('{{TYPE}}').renderViewer(h, newObj, onClose)
  //           })
  //         },
  //         footerActions: [
  //           fileUrlVal ? Dm.createLink(fileUrlVal, '打开文件', { style: 'padding:6px 12px;border-radius:6px;background:#2563eb;color:#fff;text-decoration:none;font-size:14px;display:inline-block' }) : null,
  //           origin ? Dm.createLink(origin, '打开原页面') : null,
  //           Dm.createButton('复制标题', function () { D.copyToClipboard(D.getTitle(obj)) }),
  //           origin ? Dm.createButton('复制链接', function () { D.copyToClipboard(origin) }) : null,
  //         ].filter(Boolean),
  //         onClose: onClose
  //       })
  //
  //       return h('div')
  //     }
  //   })
  // }

  // =============================================
  // 变体 2：图片画廊（参考 vikacg）
  // =============================================
  // 取消注释以下代码块，实现 getImages()/getLinks() 数据函数后即可使用

  // if (typeof TaskUI !== 'undefined' && TaskUI.register) {
  //   TaskUI.register('{{TYPE}}', {
  //     type: '{{TYPE}}',
  //     label: '{{LABEL}}',
  //     icon: 'fa-image',
  //     viewerLabel: '浏览',
  //
  //     shouldShowViewer: function (obj) {
  //       return obj.status === 'completed' && obj.extra && Array.isArray(obj.extra.images)
  //     },
  //
  //     onClick: function (obj, helpers) {
  //       if (obj.status !== 'completed') return false
  //       helpers.openTaskTypeViewer(obj)
  //       return true
  //     },
  //
  //     renderViewer: function (h, obj, onClose) {
  //       var D = TaskUI.Data, Dm = TaskUI.Dom, M = TaskUI.Modal
  //       var images = getImages(obj) // 需实现
  //       if (images.length === 0) { if (onClose) onClose(); return h('div') }
  //
  //       var currentIdx = 0
  //       var overlay = M.createOverlay()
  //       var panel = M.createPanel('1200px')
  //       overlay.appendChild(panel)
  //
  //       // Header
  //       var header = M.createHeader({
  //         title: D.getTitle(obj) || '{{LABEL}}',
  //         onClose: onClose
  //       })
  //       panel.appendChild(header)
  //
  //       // Body
  //       var body = document.createElement('div')
  //       body.style.cssText = 'flex:1;overflow:hidden;padding:0;display:flex'
  //       // ... 图片画廊业务逻辑（左右箭头、缩略图、内容等）
  //       panel.appendChild(body)
  //
  //       // Footer
  //       var footer = M.createFooter({
  //         leftButtons: [ /* 操作按钮 */ ],
  //         onClose: onClose
  //       })
  //       panel.appendChild(footer)
  //
  //       // Close handlers
  //       var keyHandler = M.setupCloseHandlers({ overlay: overlay, onClose: onClose })
  //       // ... 额外键盘事件（ArrowLeft/ArrowRight）
  //
  //       document.body.appendChild(overlay)
  //       document.body.style.overflow = 'hidden'
  //       return h('div')
  //     }
  //   })
  // }

  // =============================================
  // 变体 3：纯表单（参考 urllist）— 无 viewer
  // =============================================
  // 取消注释以下代码块即可使用

  // if (typeof TaskUI !== 'undefined' && TaskUI.register) {
  //   TaskUI.register('{{TYPE}}', {
  //     type: '{{TYPE}}',
  //     label: '{{LABEL}}',
  //     icon: 'fa-link',
  //
  //     renderForm: TaskUI.defineForm({
  //       fields: [
  //         { type: 'textarea', key: 'urls_text', label: 'URL 列表（每行一个）', rows: 10, required: true },
  //       ]
  //     }),
  //
  //     renderMeta: TaskUI.defineMeta({
  //       fields: [
  //         { type: 'count', key: 'URL 数量', path: '{{PATH}}' },
  //       ]
  //     }),
  //
  //     collectExtra: function (formData) {
  //       return { urls_text: formData.urls_text }
  //     }
  //   })
  // }
})()
```

- [ ] **步骤 3：Commit**

```bash
git add task/TEMPLATE/
git commit -m "feat: add task type template (TEMPLATE)"
```

---

### 任务 10：创建 `scripts/new-task-type.sh` 脚手架脚本

**文件：** 创建 `scripts/new-task-type.sh`

- [ ] **步骤 1：编写脚本**

```bash
#!/bin/bash
# new-task-type.sh — 创建新任务类型的脚手架
# 自动生成 Go 端注册代码 + JS 端 UI 插件模板
#
# 用法：
#   ./scripts/new-task-type.sh
#   或带参数：
#   TYPE=mytype LABEL="My Type" HAS_FORM=y HAS_VIEWER=y VIEWER_TYPE=video ./scripts/new-task-type.sh
#
# 交互式模式会提示输入以下参数：
#   - TYPE: 任务类型标识（小写字母+下划线，如 vikacg）
#   - LABEL: 显示标签（如 "My Type"）
#   - HAS_FORM: 是否有扩展表单 (y/n)
#   - HAS_VIEWER: 是否有自定义查看器 (y/n)
#   - VIEWER_TYPE: 查看器类型 (video/image/none)，仅 HAS_VIEWER=y 时生效
#
# 输出：
#   - task/<TYPE>/ui/ui.go — Go 注册代码
#   - task/<TYPE>/ui/assets/viewer.js — JS UI 插件

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# ---- 交互式参数输入 ----
if [ -z "${TYPE:-}" ]; then
  read -p "任务类型标识 (如 mytype): " TYPE
fi
TYPE="${TYPE,,}"  # 转小写
TYPE="${TYPE// /_}"  # 空格转下划线

if [ -z "${LABEL:-}" ]; then
  read -p "显示标签 (如 My Type): " LABEL
fi

if [ -z "${HAS_FORM:-}" ]; then
  read -p "是否有扩展表单? (y/n): " HAS_FORM
fi

if [ -z "${HAS_VIEWER:-}" ]; then
  read -p "是否有自定义查看器? (y/n): " HAS_VIEWER
fi

VIEWER_TYPE="none"
if [ "$HAS_VIEWER" = "y" ] || [ "$HAS_VIEWER" = "Y" ]; then
  if [ -z "${VIEWER_TYPE:-}" ]; then
    echo "查看器类型:"
    echo "  1) video — 视频播放器"
    echo "  2) image — 图片画廊"
    echo "  3) none — 仅表单"
    read -p "选择 (1/2/3): " v_choice
    case "$v_choice" in
      1) VIEWER_TYPE="video" ;;
      2) VIEWER_TYPE="image" ;;
      *) VIEWER_TYPE="none" ;;
    esac
  fi
fi

# 设置 Go 端布尔值
HAS_FORM_GO="false"
HAS_VIEWER_GO="false"
if [ "$HAS_FORM" = "y" ] || [ "$HAS_FORM" = "Y" ]; then HAS_FORM_GO="true"; fi
if [ "$HAS_VIEWER" = "y" ] || [ "$HAS_VIEWER" = "Y" ]; then HAS_VIEWER_GO="true"; fi

# ---- 创建目录 ----
UI_DIR="$PROJECT_DIR/task/$TYPE/ui"
ASSETS_DIR="$UI_DIR/assets"
mkdir -p "$ASSETS_DIR"

if [ -f "$UI_DIR/ui.go" ] || [ -f "$ASSETS_DIR/viewer.js" ]; then
  echo "错误: task/$TYPE/ 已存在文件！"
  exit 1
fi

# ---- 生成 Go 文件 ----
cat > "$UI_DIR/ui.go" << GOEOF
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package ui registers $TYPE custom UI assets via the TaskUIAssets framework.
package ui

import (
	"embed"

	"github.com/cocomhub/download-manager/core"
)

//go:embed assets/viewer.js
var assets embed.FS

func init() {
	core.RegisterTaskUI("$TYPE", core.TaskUIAssets{
		FS:      assets,
		JSPaths: []string{"assets/viewer.js"},
		Label:   "$LABEL",
		HasForm: $HAS_FORM_GO,
		HasViewer: $HAS_VIEWER_GO,
	})
}
GOEOF

echo "  ✓ 已创建: task/$TYPE/ui/ui.go"

# ---- 生成 JS 文件 ----
case "$VIEWER_TYPE" in
  video)
    cat > "$ASSETS_DIR/viewer.js" << JSEOF
/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

;(function () {
  'use strict'

  if (typeof TaskUI === 'undefined' || !TaskUI.register) return

  TaskUI.register('$TYPE', {
    type: '$TYPE',
    label: '$LABEL',
    icon: 'fa-video',
    viewerLabel: '查看',

    renderForm: TaskUI.defineForm({
      fields: [
        { type: 'text', key: 'keyword', label: '关键字', required: true, placeholder: '搜索关键词' },
        { type: 'number', key: 'max_concurrent', label: '并发数', min: 1, max: 10, default: 2 },
      ]
    }),

    renderMeta: TaskUI.defineMeta({
      fields: [
        { type: 'text', key: 'keyword', label: '关键字', path: 'extra.keyword' },
        { type: 'text', key: 'max_concurrent', label: '并发', path: 'extra.max_concurrent' },
      ]
    }),

    collectExtra: function (formData) {
      var extra = {}
      if (formData.keyword) extra.keyword = formData.keyword
      if (formData.max_concurrent) extra.max_concurrent = formData.max_concurrent
      return extra
    },

    shouldShowViewer: function (obj) { return obj.status === 'completed' },

    onClick: function (obj, helpers) {
      if (obj.status !== 'completed') return false
      helpers.openTaskTypeViewer(obj)
      return true
    },

    renderViewer: function (h, obj, onClose) {
      var D = TaskUI.Data, Dm = TaskUI.Dom, M = TaskUI.Modal

      var videoUrl = D.getVideoUrl(obj)
      var coverUrl = D.getCoverImage(obj)
      var title = D.getTitle(obj) || '$LABEL'
      var dur = D.getDuration(obj)
      var dateVal = D.getDate(obj)
      var origin = D.getOriginLink(obj)
      var details = D.getDetails(obj)
      var tags = D.getTags(obj)
      var fileUrlVal = D.getFileUrl(obj)
      var taskType = obj && obj.metadata && obj.metadata.task_type
      var objTags = (obj && obj.extra && Array.isArray(obj.extra.tags)) ? obj.extra.tags : []

      var modal = M.create(obj, {
        title: title,
        mediaType: 'video',
        videoUrl: videoUrl,
        coverUrl: coverUrl,
        infoBar: [
          dur ? { icon: 'fas fa-clock', text: dur } : null,
          dateVal ? { icon: 'fas fa-calendar', text: dateVal } : null,
        ].filter(Boolean),
        contentRenderer: function (contentDiv) {
          if (details) {
            var de = document.createElement('div')
            de.style.cssText = 'font-size:14px;color:#374151;line-height:1.6;white-space:pre-line;margin-bottom:12px'
            de.textContent = details
            contentDiv.appendChild(de)
          }
          if (tags.length > 0) {
            var tagWrap = document.createElement('div')
            tagWrap.style.cssText = 'display:flex;flex-wrap:wrap;gap:4px;margin-bottom:12px'
            tagWrap.appendChild(Dm.createTagChips(tags))
            contentDiv.appendChild(tagWrap)
          }
        },
        sidebar: 'collection',
        type: taskType,
        currentId: obj.id,
        tags: objTags,
        onPlayItem: function (item) {
          AppAPI.getObject(taskType, item.id).then(function (newObj) {
            modal.close()
            TaskUI.get('$TYPE').renderViewer(h, newObj, onClose)
          })
        },
        footerActions: [
          fileUrlVal ? Dm.createLink(fileUrlVal, '打开文件', { style: 'padding:6px 12px;border-radius:6px;background:#2563eb;color:#fff;text-decoration:none;font-size:14px;display:inline-block' }) : null,
          origin ? Dm.createLink(origin, '打开原页面') : null,
          Dm.createButton('复制标题', function () { D.copyToClipboard(D.getTitle(obj)) }),
          origin ? Dm.createButton('复制链接', function () { D.copyToClipboard(origin) }) : null,
        ].filter(Boolean),
        onClose: onClose
      })

      return h('div')
    }
  })
})()
JSEOF
    echo "  ✓ 已创建: task/$TYPE/ui/assets/viewer.js (视频播放器)"
    ;;

  image)
    cat > "$ASSETS_DIR/viewer.js" << 'JSEOF'
/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

;(function () {
  'use strict'

  if (typeof TaskUI === 'undefined' || !TaskUI.register) return

  // ---- 数据访问器（需适配实际数据模型） ----

  function getImages(obj) {
    var imgs = []
    if (obj && obj.status === 'completed' && obj.extra && Array.isArray(obj.extra.files)) {
      obj.extra.files.forEach(function (f) {
        if (f.type === 'image' && f.path) imgs.push(TaskUI.Data.fileUrl(f.path))
      })
    }
    if (imgs.length === 0 && obj && obj.extra && Array.isArray(obj.extra.images)) {
      obj.extra.images.forEach(function (u) {
        if (typeof u === 'string' && u) imgs.push(u)
      })
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
        try { href = new URL(href, base).toString() } catch (e) {}
        links.push({ text: text, href: href })
      })
    }
    return links
  }

  // ---- 注册 ----

  TaskUI.register('__TYPE__', {
    type: '__TYPE__',
    label: '__LABEL__',
    icon: 'fa-image',
    viewerLabel: '浏览',

    shouldShowViewer: function (obj) {
      return obj.status === 'completed' && obj.extra && (Array.isArray(obj.extra.images) || Array.isArray(obj.extra.files))
    },

    onClick: function (obj, helpers) {
      if (obj.status !== 'completed') return false
      helpers.openTaskTypeViewer(obj)
      return true
    },

    renderViewer: function (h, obj, onClose) {
      var images = getImages(obj)
      if (images.length === 0) { if (onClose) onClose(); return h('div') }

      var currentIdx = 0
      var D = TaskUI.Data, Dm = TaskUI.Dom, M = TaskUI.Modal
      var overlay = M.createOverlay()
      var panel = M.createPanel('1200px')
      overlay.appendChild(panel)

      // Header
      var header = M.createHeader({ title: D.getTitle(obj) || '__LABEL__', onClose: onClose })
      panel.appendChild(header)

      // Body
      var body = document.createElement('div')
      body.style.cssText = 'flex:1;overflow:hidden;padding:0;display:flex'

      var leftCol = document.createElement('div')
      leftCol.style.cssText = 'flex:1;overflow-y:auto'

      // Image
      var imgArea = document.createElement('div')
      imgArea.style.cssText = 'position:relative;background:#000;display:flex;align-items:center;justify-content:center;min-height:300px'
      var imgEl = document.createElement('img')
      imgEl.src = images[0]
      imgEl.style.cssText = 'width:100%;object-fit:contain;max-height:60vh'
      imgArea.appendChild(imgEl)

      if (images.length > 1) {
        var prevBtn = document.createElement('button')
        prevBtn.innerHTML = '<i class="fas fa-chevron-left"></i>'
        prevBtn.style.cssText = 'position:absolute;left:8px;top:50%;transform:translateY(-50%);background:rgba(255,255,255,0.8);border:none;border-radius:50%;width:36px;height:36px;font-size:16px;cursor:pointer;display:flex;align-items:center;justify-content:center;color:#374151'
        prevBtn.onclick = function (e) { e.stopPropagation(); currentIdx = (currentIdx - 1 + images.length) % images.length; imgEl.src = images[currentIdx]; }
        imgArea.appendChild(prevBtn)

        var nextBtn = document.createElement('button')
        nextBtn.innerHTML = '<i class="fas fa-chevron-right"></i>'
        nextBtn.style.cssText = 'position:absolute;right:8px;top:50%;transform:translateY(-50%);background:rgba(255,255,255,0.8);border:none;border-radius:50%;width:36px;height:36px;font-size:16px;cursor:pointer;display:flex;align-items:center;justify-content:center;color:#374151'
        nextBtn.onclick = function (e) { e.stopPropagation(); currentIdx = (currentIdx + 1) % images.length; imgEl.src = images[currentIdx]; }
        imgArea.appendChild(nextBtn)
      }
      leftCol.appendChild(imgArea)

      // Tags
      var tags = D.getTags(obj)
      if (tags.length > 0) {
        var tagWrap = document.createElement('div')
        tagWrap.style.cssText = 'display:flex;flex-wrap:wrap;gap:4px;padding:16px'
        tagWrap.appendChild(Dm.createTagChips(tags))
        leftCol.appendChild(tagWrap)
      }
      body.appendChild(leftCol)

      // Links sidebar
      var links = getLinks(obj)
      var rightCol = document.createElement('div')
      rightCol.style.cssText = 'width:320px;border-left:1px solid #e5e7eb;overflow-y:auto;background:#f9fafb;flex-shrink:0'
      if (links.length > 0) {
        var linkSection = document.createElement('div'); linkSection.style.cssText = 'padding:16px'
        var linkTitle = document.createElement('h4'); linkTitle.style.cssText = 'font-size:14px;font-weight:600;color:#374151;margin:0 0 8px'; linkTitle.textContent = '相关链接 (' + links.length + ')'; linkSection.appendChild(linkTitle)
        var linkList = document.createElement('ul'); linkList.style.cssText = 'list-style:none;padding:0;margin:0;display:flex;flex-direction:column;gap:4px'
        links.forEach(function (l) {
          var li = document.createElement('li')
          var a = document.createElement('a')
          a.href = /^https?:\/\//i.test(l.href) ? l.href : '#'; a.target = '_blank'; a.rel = 'noopener noreferrer'
          a.style.cssText = 'font-size:12px;color:#2563eb;word-break:break-all;display:block;padding:6px 8px;border:1px solid #e5e7eb;border-radius:6px;text-decoration:none'
          a.textContent = l.text || l.href
          li.appendChild(a); linkList.appendChild(li)
        })
        linkSection.appendChild(linkList); rightCol.appendChild(linkSection)
      }
      body.appendChild(rightCol)
      panel.appendChild(body)

      // Footer
      var pageUrl = obj && obj.metadata && obj.metadata.page_url
      var footer = M.createFooter({
        leftButtons: pageUrl ? [
          Dm.createButton('打开原页面', function () { window.open(pageUrl, '_blank', 'noopener,noreferrer') }, { primary: true }),
          Dm.createButton('复制链接', function () { D.copyToClipboard(pageUrl) }),
        ] : [],
        onClose: onClose
      })
      panel.appendChild(footer)

      // Close handlers
      var keyHandler = M.setupCloseHandlers({ overlay: overlay, onClose: onClose })
      document.addEventListener('keydown', function arrowHandler(e) {
        if (e.key === 'ArrowLeft' && images.length > 1) { currentIdx = (currentIdx - 1 + images.length) % images.length; imgEl.src = images[currentIdx] }
        if (e.key === 'ArrowRight' && images.length > 1) { currentIdx = (currentIdx + 1) % images.length; imgEl.src = images[currentIdx] }
      })

      document.body.appendChild(overlay)
      document.body.style.overflow = 'hidden'
      return h('div')
    }
  })
})()
JSEOF
    # 替换占位符
    sed -i "s/__TYPE__/$TYPE/g; s/__LABEL__/$LABEL/g" "$ASSETS_DIR/viewer.js"
    echo "  ✓ 已创建: task/$TYPE/ui/assets/viewer.js (图片画廊)"
    ;;

  *)
    # 仅表单
    if [ "$HAS_VIEWER_GO" = "false" ]; then
      cat > "$ASSETS_DIR/viewer.js" << JSEOF
/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

;(function () {
  'use strict'

  if (typeof TaskUI === 'undefined' || !TaskUI.register) return

  TaskUI.register('$TYPE', {
    type: '$TYPE',
    label: '$LABEL',
    icon: 'fa-link',

    renderForm: TaskUI.defineForm({
      fields: [
        { type: 'textarea', key: 'urls_text', label: 'URL 列表（每行一个）', rows: 10, required: true },
      ]
    }),

    renderMeta: TaskUI.defineMeta({
      fields: [
        { type: 'count', key: 'URL 数量', path: 'extra.urls' },
      ]
    }),

    collectExtra: function (formData) {
      return { urls_text: formData.urls_text }
    }
  })
})()
JSEOF
      echo "  ✓ 已创建: task/$TYPE/ui/assets/viewer.js (仅表单)"
    fi
    ;;
esac

echo ""
echo "==== 新任务类型 '$TYPE' 已创建 ===="
echo "  下一步:"
echo "    1. 实现 task/$TYPE/ 下的 Go 后端逻辑（task 接口、下载逻辑等）"
echo "    2. 修改 viewer.js 适配实际数据模型"
echo "    3. go build ./... 确认编译通过"
echo "    4. make run 启动后测试"
echo ""
```

- [ ] **步骤 2：添加执行权限**

```bash
chmod +x scripts/new-task-type.sh
```

- [ ] **步骤 3：测试脚本**

```bash
# 交互式测试
echo -e "testtype\nTest Type\nn\nn" | bash scripts/new-task-type.sh
# 确认生成的文件正确
rm -rf task/testtype/
```

- [ ] **步骤 4：Commit**

```bash
git add scripts/new-task-type.sh
git commit -m "feat: add new-task-type.sh scaffold script"
```

---

### 任务 11：创建 `docs/new-task-checklist.md` 开发指南

**文件：** 创建 `docs/new-task-checklist.md`

- [ ] **步骤 1：编写文档**

```markdown
# 新任务类型开发指南

## 概览

在下载管理器中添加新任务类型需要以下步骤：

1. **Go 后端** — 实现 `core.Task` 接口
2. **Go UI 注册** — `task/<type>/ui/ui.go`
3. **JS UI 插件** — `task/<type>/ui/assets/viewer.js`
4. **配置** — 在 `config.yaml` 中添加任务配置

## 快速开始

使用脚手架脚本自动生成模板：

```bash
./scripts/new-task-type.sh
```

交互式提示输入参数，或直接传参：

```bash
TYPE=mytype LABEL="My Type" HAS_FORM=y HAS_VIEWER=y VIEWER_TYPE=video ./scripts/new-task-type.sh
```

## 手动创建步骤

### 1. 创建 Go 后端

在 `task/<type>/` 下创建 Go 包，实现 `core.Task` 接口。

参考现有实现：
- `task/urllist/` — 简单 URL 列表下载
- `task/tktube/` — 视频网站下载
- `task/hanime/` — 动漫网站下载
- `task/vikacg/` — 图片网站下载

### 2. 创建 UI 注册

```
task/<type>/ui/
├── ui.go            # Go 注册代码
└── assets/
    └── viewer.js    # JS UI 插件
```

模板文件位置：`task/TEMPLATE/`

### 3. 编写 JS UI 插件

#### 3.1 共享模块

| 模块 | 命名空间 | 用途 |
|------|---------|------|
| `data.js` | `TaskUI.Data` | 通用数据访问器（getTitle、getTags、getVideoUrl 等） |
| `dom.js` | `TaskUI.Dom` | DOM 构建辅助（createTagChips、createButton、createLink 等） |
| `modal.js` | `TaskUI.Modal` | Modal 构建器（createOverlay、createPanel、createVideoArea 等） |

#### 3.2 注册方式

```js
TaskUI.register('mytype', {
  type: 'mytype',
  label: 'My Type',
  icon: 'fa-video',           // FontAwesome 图标类
  viewerLabel: '查看',         // 查看器按钮文字

  // 表单（可选）
  renderForm: TaskUI.defineForm({ fields: [...] }),
  renderMeta: TaskUI.defineMeta({ fields: [...] }),
  collectExtra: function(formData) { ... },

  // 查看器（可选）
  shouldShowViewer: function(obj) { return obj.status === 'completed' },
  onClick: function(obj, helpers) { ... },
  renderViewer: function(h, obj, onClose) { ... },
})
```

#### 3.3 查看器类型选择

| 类型 | 适用场景 | 参考实现 |
|------|---------|---------|
| 视频播放器 | 视频/动画内容 | tktube、hanime |
| 图片画廊 | 图片/漫画内容 | vikacg |
| 纯表单 | 无查看器，仅任务创建 | urllist |

### 4. 配置

在 `config.yaml` 中添加任务配置段：

```yaml
tasks:
  mytype:
    enabled: true
    # 类型特定配置...
```

## 验证清单

- [ ] `go build ./...` 编译通过
- [ ] JS 文件语法正确（无控制台错误）
- [ ] 新建任务弹窗显示扩展表单（如有）
- [ ] 任务详情页显示扩展元数据（如有）
- [ ] 点击 completed 对象打开查看器（如有）
- [ ] 查看器 ESC/backdrop 关闭正常
- [ ] 合集/推荐面板正常显示（如有）
- [ ] `make run` 启动后功能正常
```

- [ ] **步骤 2：Commit**

```bash
git add docs/new-task-checklist.md
git commit -m "docs: add new task type development guide"
```

---

### 任务 12：最终验证

**前置条件：** 所有任务 1-11 完成

- [ ] **步骤 1：编译验证**

```bash
cd download-manager && go build ./...
```

预期：无编译错误。

- [ ] **步骤 2：启动 Web UI 测试**

```bash
make run
```

在浏览器中打开 `http://127.0.0.1:19199`，逐一验证：

1. tktube 类型：点击 completed 对象 → 视频播放器 Modal 正常（header/footer/视频/合集/推荐）
2. hanime 类型：点击 completed 对象 → 视频播放器 Modal 正常
3. vikacg 类型：点击 completed 对象 → 图片画廊 Modal 正常（箭头/缩略图/键盘导航）
4. urllist 类型：新建任务 → 表单正常显示

- [ ] **步骤 3：验证新增任务类型流程**

```bash
# 测试脚手架脚本
echo -e "testtype\nTest Type\nn\nn" | bash scripts/new-task-type.sh
ls task/testtype/ui/ui.go
ls task/testtype/ui/assets/viewer.js
rm -rf task/testtype/
```

预期：脚本正常生成文件，目录结构正确。

- [ ] **步骤 4：最终 Commit**

```bash
git add -A
git commit -m "feat: complete task UI shared utilities and templates"
```

---

## 注意事项

1. **Modal 关闭清理**：`TaskUI.Modal.setupCloseHandlers()` 返回的 `keyHandler` 必须在关闭时移除，避免内存泄漏。`Modal.create()` 已自动处理。

2. **视频控制样式**：`Modal.createVideoArea()` 通过 `document.getElementById('dm-video-player-style')` 检查避免重复注入 CSS。

3. **依赖性**：`modal.js` 依赖 `TaskUI.Data` 和 `TaskUI.Dom`，必须在 `data.js` 和 `dom.js` 之后加载。`index.html` 中的加载顺序已保证。

4. **现有 viewer 兼容性**：简化后的 viewer 仍然通过 `TaskUI.register()` 注册完全相同的 handler 结构，API 层不受影响。`renderViewer` 仍然接收 `(h, obj, onClose)` 签名。

5. **hanime 特有函数保留**：`getArtist()`、`getGenres()`、`getPlaylist()`、`getCoverImages()`、`getThumbImages()` 是 hanime 特有，保留在 viewer 文件中，不进入共享模块。