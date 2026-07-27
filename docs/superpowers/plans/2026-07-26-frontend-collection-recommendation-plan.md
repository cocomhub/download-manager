# 前端合集面板、推荐面板与播放器导航 — 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 实现前端合集面板、推荐面板和播放器导航功能，让查看器右栏展示合集列表和推荐结果，播放器支持上/下一集切换。

**架构：** 四个独立任务：API 层扩展 → 合集面板组件 → 推荐面板组件 → 播放器导航 + 查看器集成。

**技术栈：** JavaScript（ES5，无框架依赖），Vue 3（CDN），DOM 操作

---

## 文件结构总览

| 文件 | 操作 | 职责 |
|------|------|------|
| `web/static/app/api.js` | 修改 | 新增 `getObject`、`getCollection` 方法，扩展 `aggregate` 支持推荐参数 |
| `web/static/app/taskui/collection.js` | **创建** | 合集面板组件（DOM 渲染，无 Vue 依赖） |
| `web/static/app/taskui/recommendation.js` | **创建** | 推荐面板组件（DOM 渲染） |
| `web/static/app/videoPlayer.js` | 修改 | 新增 `playPrev`/`playNext`/`switchToCollectionItem` 方法 + `collectionList` 数据 |
| `web/static/index.html` | 修改 | 播放器控制栏新增上/下一集按钮 |
| `web/static/tasks/hanime/ui/assets/viewer.js` | 修改 | 集成合集+推荐面板到右栏 |
| `web/static/tasks/tktube/ui/assets/viewer.js` | 修改 | 替换占位符，集成合集+推荐面板到右栏 |

---

## 任务

### 任务 1：API 层扩展 — api.js

**文件：**
- 修改：`web/static/app/api.js`

- [ ] **步骤 1：新增 `getObject` 和 `getCollection` 方法**

在 `activeDownloads` 方法后添加：

```javascript
getObject: function (type, id) {
  return this.get('/api/objects/' + type + '/' + id)
},

getCollection: function (type, id) {
  return this.get('/api/objects/' + type + '/' + id + '/collection')
},
```

- [ ] **步骤 2：扩展 `aggregate` 方法支持推荐参数**

在 `aggregate` 方法中，在 `groupBy` 参数后添加：

```javascript
if (params.tags) { q.set('tags', params.tags) }
if (params.tagMode) { q.set('tag_mode', params.tagMode) }
if (params.excludeIds) { q.set('exclude_ids', params.excludeIds) }
```

- [ ] **步骤 3：Commit**

```bash
git add web/static/app/api.js
git commit -m "feat: add getObject/getCollection API methods, extend aggregate with recommendation params"
```

---

### 任务 2：合集面板组件 — collection.js

**文件：**
- 创建：`web/static/app/taskui/collection.js`

- [ ] **步骤 1：创建 `collection.js`**

```javascript
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * Collection panel component — renders a list of collection items in the viewer sidebar.
 * Pure DOM-based, no Vue dependency. Can be used by any task type viewer.
 *
 * Usage:
 *   var panel = CollectionPanel.create({
 *     type: 'hanime',
 *     currentId: 407014,
 *     onPlayItem: function (item) { ... }
 *   })
 *   container.appendChild(panel.element)
 *   // To update highlight without re-fetching:
 *   panel.update({ currentId: 407015 })
 *   // To destroy:
 *   panel.destroy()
 */
;(function () {
  'use strict'

  window.CollectionPanel = {
    create: function (options) {
      var type = options.type
      var currentId = options.currentId
      var onPlayItem = options.onPlayItem || function () {}
      var collapsed = false
      var items = []
      var element = document.createElement('div')
      element.className = 'collection-panel border-b border-gray-200'

      // Header
      var header = document.createElement('div')
      header.className = 'flex items-center justify-between px-3 py-2 bg-gray-50 cursor-pointer select-none hover:bg-gray-100'
      header.innerHTML = '<span class="text-sm font-semibold text-gray-700">合集 <span class="collection-count text-gray-400">(0)</span></span>' +
        '<button class="collection-toggle text-gray-400 hover:text-gray-600 text-xs"><i class="fas fa-chevron-up"></i></button>'

      // Body
      var body = document.createElement('div')
      body.className = 'collection-body max-h-64 overflow-y-auto'

      // Fetch collection data
      if (type && currentId) {
        AppAPI.getCollection(type, currentId).then(function (data) {
          items = data.objects || []
          renderList(items)
        }).catch(function () {
          body.innerHTML = '<div class="px-3 py-4 text-center text-gray-400 text-sm">加载合集失败</div>'
        })
      } else {
        body.innerHTML = '<div class="px-3 py-4 text-center text-gray-400 text-sm">暂无合集</div>'
      }

      function renderList (list) {
        if (list.length === 0) {
          body.innerHTML = '<div class="px-3 py-4 text-center text-gray-400 text-sm">暂无合集</div>'
          header.querySelector('.collection-count').textContent = '(0)'
          return
        }
        header.querySelector('.collection-count').textContent = '(' + list.length + ')'
        body.innerHTML = ''
        list.forEach(function (item, idx) {
          var row = document.createElement('div')
          row.className = 'flex items-center gap-2 px-3 py-2 cursor-pointer hover:bg-blue-50 border-b border-gray-100 last:border-b-0 text-sm'
          row.dataset.id = item.id

          // Number badge
          var badge = document.createElement('span')
          badge.className = 'w-5 h-5 rounded-full flex items-center justify-center text-xs font-medium flex-shrink-0 ' +
            (item.id === currentId ? 'bg-blue-600 text-white' : 'bg-gray-200 text-gray-600')
          badge.textContent = idx + 1
          row.appendChild(badge)

          // Title
          var title = document.createElement('span')
          title.className = 'flex-1 truncate ' + (item.id === currentId ? 'text-blue-700 font-medium' : 'text-gray-700')
          title.textContent = item.metadata && item.metadata.collection_title ? item.metadata.collection_title : (item.metadata && item.metadata.title ? item.metadata.title : 'Item ' + item.id)
          row.appendChild(title)

          // Duration
          if (item.metadata && item.metadata.duration) {
            var dur = document.createElement('span')
            dur.className = 'text-xs text-gray-400 flex-shrink-0'
            dur.textContent = item.metadata.duration
            row.appendChild(dur)
          }

          // Current indicator
          if (item.id === currentId) {
            var indicator = document.createElement('span')
            indicator.className = 'text-blue-600 text-xs flex-shrink-0 ml-1'
            indicator.textContent = '▶'
            row.appendChild(indicator)
          }

          row.addEventListener('click', function () {
            if (item.id !== currentId) {
              onPlayItem(item)
            }
          })

          body.appendChild(row)
        })

        // Scroll to current item
        var currentRow = body.querySelector('[data-id="' + currentId + '"]')
        if (currentRow) {
          currentRow.scrollIntoView({ block: 'nearest' })
        }
      }

      // Toggle collapse
      header.addEventListener('click', function (e) {
        if (e.target.closest('.collection-toggle')) {
          collapsed = !collapsed
          body.style.display = collapsed ? 'none' : ''
          header.querySelector('.collection-toggle i').className = 'fas fa-chevron-' + (collapsed ? 'down' : 'up')
        }
      })

      element.appendChild(header)
      element.appendChild(body)

      return {
        element: element,
        update: function (opts) {
          if (opts.currentId !== undefined) {
            currentId = opts.currentId
            // Re-render with existing data, no re-fetch
            renderList(items)
          }
        },
        destroy: function () {
          element.remove()
        }
      }
    }
  }
})()
```

- [ ] **步骤 2：Commit**

```bash
git add web/static/app/taskui/collection.js
git commit -m "feat: add collection panel component for viewer sidebar"
```

---

### 任务 3：推荐面板组件 — recommendation.js

**文件：**
- 创建：`web/static/app/taskui/recommendation.js`

- [ ] **步骤 1：创建 `recommendation.js`**

```javascript
// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * Recommendation panel component — renders tag-based recommendations in the viewer sidebar.
 * Pure DOM-based, no Vue dependency.
 *
 * Usage:
 *   var panel = RecommendationPanel.create({
 *     type: 'hanime',
 *     currentId: 407014,
 *     tags: ['action', 'comedy'],
 *     onPlayItem: function (item) { ... }
 *   })
 *   container.appendChild(panel.element)
 */
;(function () {
  'use strict'

  window.RecommendationPanel = {
    create: function (options) {
      var type = options.type
      var currentId = options.currentId
      var allTags = options.tags || []
      var onPlayItem = options.onPlayItem || function () {}
      var collapsed = false
      var selectedTags = allTags.slice() // default: all selected
      var tagMode = 'any'
      var sortBy = 'random'
      var element = document.createElement('div')
      element.className = 'recommendation-panel border-b border-gray-200'

      // Header
      var header = document.createElement('div')
      header.className = 'flex items-center justify-between px-3 py-2 bg-gray-50 cursor-pointer select-none hover:bg-gray-100'
      header.innerHTML = '<span class="text-sm font-semibold text-gray-700">推荐</span>' +
        '<button class="rec-toggle text-gray-400 hover:text-gray-600 text-xs"><i class="fas fa-chevron-up"></i></button>'

      var body = document.createElement('div')
      body.className = 'recommendation-body'

      // Controls
      var controls = document.createElement('div')
      controls.className = 'px-3 py-2 space-y-2 border-b border-gray-100'

      // Tag selector
      var tagContainer = document.createElement('div')
      tagContainer.className = 'flex flex-wrap gap-1'
      controls.appendChild(tagContainer)

      // Mode and sort dropdowns
      var modeRow = document.createElement('div')
      modeRow.className = 'flex gap-2 text-xs'
      modeRow.innerHTML =
        '<select class="rec-mode flex-1 bg-gray-100 border border-gray-300 rounded px-1 py-1 text-gray-700 outline-none">' +
          '<option value="any">任一匹配</option>' +
          '<option value="all">全部匹配</option>' +
        '</select>' +
        '<select class="rec-sort flex-1 bg-gray-100 border border-gray-300 rounded px-1 py-1 text-gray-700 outline-none">' +
          '<option value="random">随机</option>' +
          '<option value="date_desc">最新</option>' +
          '<option value="tag_match_desc">最相关</option>' +
        '</select>'
      controls.appendChild(modeRow)

      // Results container
      var results = document.createElement('div')
      results.className = 'recommendation-results max-h-64 overflow-y-auto'

      body.appendChild(controls)
      body.appendChild(results)
      element.appendChild(header)
      element.appendChild(body)

      // Render tags
      function renderTags () {
        tagContainer.innerHTML = ''
        // "全部" toggle
        var allBtn = document.createElement('button')
        allBtn.className = 'text-xs px-2 py-0.5 rounded-full border transition ' +
          (selectedTags.length === allTags.length ? 'bg-blue-600 text-white border-blue-600' : 'bg-white text-gray-500 border-gray-300')
        allBtn.textContent = '全部'
        allBtn.addEventListener('click', function () {
          if (selectedTags.length === allTags.length) {
            selectedTags = []
          } else {
            selectedTags = allTags.slice()
          }
          renderTags()
          fetchRecommendations()
        })
        tagContainer.appendChild(allBtn)

        allTags.forEach(function (tag) {
          var btn = document.createElement('button')
          var active = selectedTags.indexOf(tag) >= 0
          btn.className = 'text-xs px-2 py-0.5 rounded-full border transition ' +
            (active ? 'bg-blue-100 text-blue-700 border-blue-300' : 'bg-white text-gray-500 border-gray-300')
          btn.textContent = tag
          btn.addEventListener('click', function () {
            var idx = selectedTags.indexOf(tag)
            if (idx >= 0) {
              selectedTags.splice(idx, 1)
            } else {
              selectedTags.push(tag)
            }
            renderTags()
            fetchRecommendations()
          })
          tagContainer.appendChild(btn)
        })
      }

      // Fetch recommendations
      function fetchRecommendations () {
        if (selectedTags.length === 0) {
          results.innerHTML = '<div class="px-3 py-4 text-center text-gray-400 text-sm">请选择标签</div>'
          return
        }
        results.innerHTML = '<div class="px-3 py-4 text-center text-gray-400 text-sm"><i class="fas fa-spinner fa-spin"></i> 加载中...</div>'
        AppAPI.aggregate({
          types: type,
          tags: selectedTags.join(','),
          tagMode: tagMode,
          excludeIds: String(currentId),
          sort: sortBy,
          limit: 20
        }).then(function (data) {
          var list = data.objects || []
          if (list.length === 0) {
            results.innerHTML = '<div class="px-3 py-4 text-center text-gray-400 text-sm">暂无推荐</div>'
            return
          }
          results.innerHTML = ''
          list.forEach(function (item) {
            var row = document.createElement('div')
            row.className = 'flex items-center gap-2 px-3 py-2 cursor-pointer hover:bg-blue-50 border-b border-gray-100 last:border-b-0'

            // Cover thumbnail
            var img = document.createElement('img')
            img.className = 'w-10 h-7 object-cover rounded flex-shrink-0 bg-gray-200'
            img.src = ''
            img.alt = ''
            // Try to get cover from various sources
            var coverUrl = ''
            if (item.extra && item.extra.local_cover) coverUrl = '/files/' + item.extra.local_cover.replace(/\\/g, '/')
            else if (item.extra && item.extra.cover_url) coverUrl = item.extra.cover_url
            else if (item.extra && item.extra.thumb_url) coverUrl = item.extra.thumb_url
            if (coverUrl) img.src = coverUrl
            else img.style.display = 'none'
            row.appendChild(img)

            // Title
            var title = document.createElement('span')
            title.className = 'flex-1 truncate text-sm text-gray-700'
            title.textContent = item.metadata && item.metadata.title ? item.metadata.title : item.url
            row.appendChild(title)

            row.addEventListener('click', function () {
              onPlayItem(item)
            })
            results.appendChild(row)
          })
        }).catch(function () {
          results.innerHTML = '<div class="px-3 py-4 text-center text-gray-400 text-sm">加载失败</div>'
        })
      }

      // Wire up mode/sort dropdowns
      var modeSelect = controls.querySelector('.rec-mode')
      var sortSelect = controls.querySelector('.rec-sort')
      modeSelect.addEventListener('change', function () {
        tagMode = modeSelect.value
        fetchRecommendations()
      })
      sortSelect.addEventListener('change', function () {
        sortBy = sortSelect.value
        fetchRecommendations()
      })

      // Toggle collapse
      header.addEventListener('click', function (e) {
        if (e.target.closest('.rec-toggle')) {
          collapsed = !collapsed
          body.style.display = collapsed ? 'none' : ''
          header.querySelector('.rec-toggle i').className = 'fas fa-chevron-' + (collapsed ? 'down' : 'up')
        }
      })

      // Initial render
      renderTags()
      fetchRecommendations()

      return {
        element: element,
        update: function (opts) {
          // Recommendation panel does NOT re-fetch on collection switch
          // Only update if explicitly needed
        },
        destroy: function () {
          element.remove()
        }
      }
    }
  }
})()
```

- [ ] **步骤 2：Commit**

```bash
git add web/static/app/taskui/recommendation.js
git commit -m "feat: add recommendation panel component with tag selector and mode/sort switching"
```

---

### 任务 4：播放器导航 — videoPlayer.js + index.html

**文件：**
- 修改：`web/static/app/videoPlayer.js`
- 修改：`web/static/index.html`

- [ ] **步骤 1：在 `videoPlayer.js` 中添加 `collectionList` 数据和导航方法**

在 `data()` 的 `return` 对象中，`videoSettings` 后添加：
```javascript
collectionList: [],
collectionIndex: -1,
onCollectionSwitch: null
```

在 `methods` 中，`closeVideo` 前添加 `playPrev`、`playNext`、`switchToCollectionItem`：

```javascript
playPrev: function () {
  var idx = this.collectionList.findIndex(function (o) { return o.id === this.currentVideo.id }.bind(this))
  if (idx > 0) {
    this.switchToCollectionItem(this.collectionList[idx - 1])
  }
},

playNext: function () {
  var idx = this.collectionList.findIndex(function (o) { return o.id === this.currentVideo.id }.bind(this))
  if (idx < this.collectionList.length - 1) {
    this.switchToCollectionItem(this.collectionList[idx + 1])
  }
},

switchToCollectionItem: function (item) {
  var self = this
  var type = this.currentVideo && this.currentVideo.metadata && this.currentVideo.metadata.task_type
  if (!type) {
    // Fallback: try to get type from currentVideo
    type = 'hanime'
  }
  AppAPI.getObject(type, item.id).then(function (obj) {
    // Preserve collection context
    var list = self.collectionList
    var callback = self.onCollectionSwitch
    self.currentVideo = obj
    self.collectionList = list
    self.onCollectionSwitch = callback
    // Notify collection panel to update highlight
    if (self.onCollectionSwitch) {
      self.onCollectionSwitch(item.id)
    }
  }).catch(function () {
    // If getObject fails, use the item data directly
    var list = self.collectionList
    var callback = self.onCollectionSwitch
    self.currentVideo = item
    self.collectionList = list
    self.onCollectionSwitch = callback
    if (self.onCollectionSwitch) {
      self.onCollectionSwitch(item.id)
    }
  })
},
```

- [ ] **步骤 2：在 `index.html` 播放器控制栏中添加导航按钮**

在 `index.html` 第 993 行 `<div class="flex justify-between items-center text-white">` 下，`<!-- Left Controls -->` 区域中，在 `togglePlay` 按钮后添加：

```html
<!-- Collection Navigation -->
<div v-if="collectionList.length > 1" class="flex items-center gap-1">
  <button @click="playPrev" class="hover:text-blue-400 transition text-sm px-1" :class="{ 'opacity-30 cursor-not-allowed': collectionList.findIndex(o => o.id === currentVideo.id) <= 0 }" :disabled="collectionList.findIndex(o => o.id === currentVideo.id) <= 0" title="上一集">
    <i class="fas fa-step-backward"></i>
  </button>
  <span class="text-xs text-gray-400 select-none">
    {{ collectionList.findIndex(o => o.id === currentVideo.id) + 1 }} / {{ collectionList.length }}
  </span>
  <button @click="playNext" class="hover:text-blue-400 transition text-sm px-1" :class="{ 'opacity-30 cursor-not-allowed': collectionList.findIndex(o => o.id === currentVideo.id) >= collectionList.length - 1 }" :disabled="collectionList.findIndex(o => o.id === currentVideo.id) >= collectionList.length - 1" title="下一集">
    <i class="fas fa-step-forward"></i>
  </button>
</div>
```

- [ ] **步骤 3：Commit**

```bash
git add web/static/app/videoPlayer.js web/static/index.html
git commit -m "feat: add collection navigation (prev/next) to video player"
```

---

### 任务 5：hanime 查看器集成

**文件：**
- 修改：`web/static/tasks/hanime/ui/assets/viewer.js`

- [ ] **步骤 1：在 hanime 查看器右栏中集成合集+推荐面板**

找到 hanime 查看器的右栏创建代码（约第 316 行 body 部分）。在右栏 div 中，移除旧的播放列表渲染，替换为：

```javascript
// Right column: collection panel + recommendation panel
var rightCol = document.createElement('div')
rightCol.className = 'viewer-right-col'
rightCol.style.cssText = 'width:320px;border-left:1px solid #e5e7eb;display:flex;flex-direction:column;overflow:hidden;flex-shrink:0;background:#fff'

// Collection panel
var collectionPanel = null
var recommendationPanel = null

// Get task type from object
var taskType = obj && obj.metadata && obj.metadata.task_type

// Get tags from object
var objTags = []
if (obj && obj.extra && Array.isArray(obj.extra.tags)) {
  objTags = obj.extra.tags
}

// Helper to switch collection item
function handleCollectionPlay (item) {
  var type = taskType
  AppAPI.getObject(type, item.id).then(function (newObj) {
    // Update collection panel highlight
    if (collectionPanel) {
      collectionPanel.update({ currentId: item.id })
    }
    // Update video area
    updateVideoArea(newObj)
    // Update metadata
    updateMetaArea(newObj)
  })
}

// Create collection panel
if (taskType && obj && obj.id) {
  collectionPanel = CollectionPanel.create({
    type: taskType,
    currentId: obj.id,
    onPlayItem: handleCollectionPlay
  })
  rightCol.appendChild(collectionPanel.element)
}

// Create recommendation panel
if (taskType && obj && obj.id) {
  recommendationPanel = RecommendationPanel.create({
    type: taskType,
    currentId: obj.id,
    tags: objTags,
    onPlayItem: function (item) {
      // Open viewer for recommended item
      if (window.__dm_uiBridge && typeof window.__dm_uiBridge.open === 'function') {
        window.__dm_uiBridge.open(taskType, item)
      }
    }
  })
  rightCol.appendChild(recommendationPanel.element)
}

// Helper to update video area
function updateVideoArea (newObj) {
  // Find the video container and update poster and video source
  var viewerBody = rightCol.parentElement
  if (viewerBody) {
    var mediaArea = viewerBody.querySelector('.viewer-media-area')
    if (mediaArea) {
      // Re-render media area with new object
      mediaArea.innerHTML = ''
      mediaArea.appendChild(createMediaElement(newObj))
    }
    // Update title
    var titleEl = viewerBody.querySelector('.viewer-title')
    if (titleEl) {
      titleEl.textContent = (newObj.metadata && newObj.metadata.title) || newObj.url
    }
  }
}

function updateMetaArea (newObj) {
  // Update metadata display in left column
  var viewerBody = rightCol.parentElement
  if (viewerBody) {
    var metaEl = viewerBody.querySelector('.viewer-meta')
    if (metaEl) {
      // Update meta content
      var metaHtml = ''
      if (newObj.metadata) {
        if (newObj.metadata.artist) metaHtml += '<span class="text-sm text-gray-600">' + newObj.metadata.artist + '</span>'
        if (newObj.metadata.date) metaHtml += '<span class="text-sm text-gray-400"> · ' + newObj.metadata.date + '</span>'
        if (newObj.metadata.duration) metaHtml += '<span class="text-sm text-gray-400"> · ' + newObj.metadata.duration + '</span>'
      }
      metaEl.innerHTML = metaHtml
    }
  }
}
```

注意：需要找到 `createMediaElement` 函数（hanime 查看器中已有创建视频区域的逻辑），将其提取为可复用的函数以支持 `updateVideoArea`。

- [ ] **步骤 2：在查看器关闭时销毁面板**

```javascript
// 在查看器的关闭逻辑中添加：
if (collectionPanel) collectionPanel.destroy()
if (recommendationPanel) recommendationPanel.destroy()
```

- [ ] **步骤 3：Commit**

```bash
git add web/static/tasks/hanime/ui/assets/viewer.js
git commit -m "feat: integrate collection and recommendation panels into hanime viewer"
```

---

### 任务 6：tktube 查看器集成

**文件：**
- 修改：`web/static/tasks/tktube/ui/assets/viewer.js`

- [ ] **步骤 1：在 tktube 查看器右栏中替换占位符**

找到 tktube 查看器的右栏创建代码（约第 403 行）。替换"关联视频"占位符为合集+推荐面板，逻辑与 hanime 查看器类似。

```javascript
// 替换原有的占位符代码：
// var relatedPlaceholder = document.createElement('div')
// relatedPlaceholder.innerHTML = '<i class="fas fa-film" ...></i> 关联视频列表<br>（后续实现）'

// 改为：
// Collection panel
var collectionPanel = null
var recommendationPanel = null
var taskType = obj && obj.metadata && obj.metadata.task_type
var objTags = []
if (obj && obj.extra && Array.isArray(obj.extra.tags)) {
  objTags = obj.extra.tags
}

if (taskType && obj && obj.id) {
  collectionPanel = CollectionPanel.create({
    type: taskType,
    currentId: obj.id,
    onPlayItem: function (item) {
      AppAPI.getObject(taskType, item.id).then(function (newObj) {
        if (collectionPanel) collectionPanel.update({ currentId: item.id })
        // Update video area
        updateViewerContent(newObj)
      })
    }
  })
  rightCol.appendChild(collectionPanel.element)
}

if (taskType && obj && obj.id) {
  recommendationPanel = RecommendationPanel.create({
    type: taskType,
    currentId: obj.id,
    tags: objTags,
    onPlayItem: function (item) {
      if (window.__dm_uiBridge && typeof window.__dm_uiBridge.open === 'function') {
        window.__dm_uiBridge.open(taskType, item)
      }
    }
  })
  rightCol.appendChild(recommendationPanel.element)
}

// Helper to update viewer content
function updateViewerContent (newObj) {
  // Update media area, title, metadata
  // Reuse createMediaElement pattern from tktube viewer
}
```

- [ ] **步骤 2：在查看器关闭时销毁面板**

```javascript
if (collectionPanel) collectionPanel.destroy()
if (recommendationPanel) recommendationPanel.destroy()
```

- [ ] **步骤 3：Commit**

```bash
git add web/static/tasks/tktube/ui/assets/viewer.js
git commit -m "feat: integrate collection and recommendation panels into tktube viewer"
```

---

## 验证

### 功能验证
1. 启动服务：`go run . --config build/config.yaml`
2. 打开 hanime 任务，点击已完成对象进入查看器
3. 确认右栏显示合集面板，列表项高亮当前视频
4. 确认合集列表自动滚动到当前项
5. 点击合集列表中的其他项，确认视频切换播放
6. 确认推荐面板显示标签选择器，默认全选
7. 切换标签、模式、排序，确认推荐结果重新加载
8. 点击推荐结果项，确认打开查看器
9. 确认播放器底部显示上/下一集按钮
10. 点击上一集/下一集，确认切换播放
11. 确认折叠按钮正常工作
12. 确认 tktube 查看器同样工作

### 构建验证
```bash
# 前端代码无需构建，直接嵌入
# 如果修改了后端代码
go build ./...
```

### Playwright 测试确认
```bash
make playwright-test
```

---

## 执行交接

计划已完成。两种执行方式：

**1. 子代理驱动（推荐）** — 每个任务调度一个新的子代理，任务间进行审查，快速迭代

**2. 内联执行** — 在当前会话中逐步执行任务，批量执行并设有检查点

选哪种方式？