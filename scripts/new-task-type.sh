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
		FS:        assets,
		JSPaths:   []string{"assets/viewer.js"},
		Label:     "$LABEL",
		HasForm:   $HAS_FORM_GO,
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
    cat > "$ASSETS_DIR/viewer.js" << 'JSEOF_IMAGE'
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
      overlay.addEventListener('click', function (e) { if (e.target === overlay) onClose() })
      function keyHandler(e) {
        if (e.key === 'Escape') { onClose(); return }
        if (e.key === 'ArrowLeft' && images.length > 1) { currentIdx = (currentIdx - 1 + images.length) % images.length; imgEl.src = images[currentIdx] }
        if (e.key === 'ArrowRight' && images.length > 1) { currentIdx = (currentIdx + 1) % images.length; imgEl.src = images[currentIdx] }
      }
      document.addEventListener('keydown', keyHandler)

      // Note: onClose should call document.removeEventListener('keydown', keyHandler)

      document.body.appendChild(overlay)
      document.body.style.overflow = 'hidden'
      return h('div')
    }
  })
})()
JSEOF_IMAGE
    # 替换占位符
    sed -i "s/__TYPE__/$TYPE/g; s/__LABEL__/$LABEL/g" "$ASSETS_DIR/viewer.js"
    echo "  ✓ 已创建: task/$TYPE/ui/assets/viewer.js (图片画廊)"
    ;;

  *)
    # 仅表单
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