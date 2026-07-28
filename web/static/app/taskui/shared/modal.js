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
   * @param {string} [maxWidth] — 如 '1400px'
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
   * @param {function} [config.onClose] — 关闭回调
   * @param {Array<{text:string, bg?:string, color?:string}>} [config.badges] — 右侧徽章列表
   * @returns {HTMLElement}
   */
  Modal.createHeader = function (config) {
    var header = document.createElement('div')
    header.style.cssText = 'padding:16px;border-bottom:1px solid #e5e7eb;display:flex;justify-content:space-between;align-items:center;background:#f9fafb'

    var hTitle = document.createElement('h3')
    hTitle.style.cssText = 'font-size:18px;font-weight:700;color:#1f2937;margin:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap'
    hTitle.className = 'viewer-title'
    hTitle.textContent = config.title || ''
    header.appendChild(hTitle)

    var right = document.createElement('div')
    right.style.cssText = 'display:flex;align-items:center;gap:12px;flex-shrink:0'

    if (config.badges && config.badges.length > 0) {
      config.badges.forEach(function (b) {
        if (!b || !b.text) return
        var badge = document.createElement('span')
        badge.style.cssText = 'font-size:11px;padding:2px 8px;border-radius:4px;background:' + (b.bg || '#f3f4f6') + ';color:' + (b.color || '#4b5563')
        badge.textContent = b.text
        right.appendChild(badge)
      })
    }

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
   * @param {Array<HTMLElement>} [config.leftButtons] — 左侧按钮数组
   * @param {string} [config.closeText] — 关闭按钮文字，默认 '关闭'
   * @param {function} [config.onClose] — 关闭回调
   * @returns {HTMLElement}
   */
  Modal.createFooter = function (config) {
    var footer = document.createElement('div')
    footer.style.cssText = 'padding:12px 16px;border-top:1px solid #e5e7eb;background:#f9fafb;display:flex;justify-content:space-between;align-items:center'

    var fLeft = document.createElement('div')
    fLeft.style.cssText = 'display:flex;gap:8px'
    if (config.leftButtons && config.leftButtons.length > 0) {
      config.leftButtons.forEach(function (btn) {
        if (btn) fLeft.appendChild(btn)
      })
    }
    footer.appendChild(fLeft)

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
   * @param {string} [config.videoUrl] — 视频 URL
   * @param {string} [config.coverUrl] — 封面 URL
   * @param {string} [config.title] — 标题
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
      var posterImg = document.createElement('img')
      posterImg.src = config.coverUrl || config.videoUrl
      posterImg.style.cssText = 'width:100%;height:100%;object-fit:contain;cursor:pointer'
      posterImg.alt = config.title || ''
      area.appendChild(posterImg)

      var playOverlay = document.createElement('div')
      playOverlay.style.cssText = 'position:absolute;inset:0;display:flex;align-items:center;justify-content:center;background:rgba(0,0,0,0.2);cursor:pointer'
      playOverlay.innerHTML = '<i class="fas fa-play" style="font-size:48px;color:#fff;opacity:0.8;text-shadow:0 2px 8px rgba(0,0,0,0.5)"></i>'
      area.appendChild(playOverlay)

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
      var posterImg = document.createElement('img')
      posterImg.src = config.coverUrl || ''
      posterImg.style.cssText = 'width:100%;height:100%;object-fit:contain'
      posterImg.alt = config.title || ''
      if (posterImg.src) area.appendChild(posterImg)
    } else {
      var placeholder = document.createElement('div')
      placeholder.style.cssText = 'display:flex;align-items:center;justify-content:center;height:100%;color:#9ca3af;font-size:14px'
      placeholder.innerHTML = '<i class="fas fa-video" style="font-size:48px;margin-right:12px;opacity:0.5"></i> 无可用视频'
      area.appendChild(placeholder)
    }

    // Inject video controls CSS once
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
   * @param {string[]} [config.tags] — 标签列表
   * @param {function} [config.onPlayItem] — 播放/切换回调
   * @returns {{ element: HTMLElement, collectionPanel: object|null, recommendationPanel: object|null }}
   */
  Modal.createSidebar = function (config) {
    var rightCol = document.createElement('div')
    rightCol.style.cssText = 'width:380px;border-left:1px solid #e5e7eb;display:flex;flex-direction:column;overflow:hidden;flex-shrink:0;background:#fff'

    var result = { element: rightCol, collectionPanel: null, recommendationPanel: null }

    if (!config.type || !config.currentId) return result

    if (window.CollectionPanel) {
      result.collectionPanel = window.CollectionPanel.create({
        type: config.type,
        currentId: config.currentId,
        onPlayItem: function (item) {
          if (config.onPlayItem) config.onPlayItem(item, 'collection')
        }
      })
      if (result.collectionPanel) {
        rightCol.appendChild(result.collectionPanel.element)
      }
    }

    if (window.RecommendationPanel) {
      result.recommendationPanel = window.RecommendationPanel.create({
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
    }

    return result
  }

  /**
   * 设置关闭事件处理器（backdrop 点击 + ESC 键盘）
   * @param {object} config
   * @param {HTMLElement} config.overlay — overlay 元素
   * @param {function} config.onClose — 关闭回调
   * @returns {function} keyHandler — 供后续移除
   */
  Modal.setupCloseHandlers = function (config) {
    config.overlay.addEventListener('click', function (e) {
      if (e.target === config.overlay && config.onClose) config.onClose()
    })

    function keyHandler(e) {
      if (e.key === 'Escape' && config.onClose) config.onClose()
    }
    document.addEventListener('keydown', keyHandler)
    return keyHandler
  }

  /**
   * 创建完整 modal（全功能构建器）
   * @param {object} obj — 下载对象
   * @param {object} config
   * @param {string} [config.title] — 标题，默认 TaskUI.Data.getTitle(obj)
   * @param {string} [config.maxWidth] — 最大宽度，默认 '1400px'
   * @param {Array} [config.badges] — 顶部徽章
   * @param {string} [config.mediaType] — 'video' 时显示视频区域
   * @param {string} [config.videoUrl] — 视频 URL
   * @param {string} [config.coverUrl] — 封面 URL
   * @param {Array} [config.infoBar] — 信息条项目
   * @param {function} [config.contentRenderer] — 主体内容渲染回调 function(contentDiv, obj)
   * @param {string} [config.sidebar] — 'collection' 时显示合集/推荐面板
   * @param {string} [config.type] — 任务类型（sidebar 用）
   * @param {string|number} [config.currentId] — 当前对象 ID（sidebar 用）
   * @param {string[]} [config.tags] — 标签（sidebar 用）
   * @param {function} [config.onPlayItem] — 合集/推荐切换回调
   * @param {Array} [config.footerActions] — 底部操作按钮
   * @param {function} [config.onClose] — 关闭回调
   * @returns {{ overlay: HTMLElement, panel: HTMLElement, close: function }}
   */
  Modal.create = function (obj, config) {
    config = config || {}
    var D = TaskUI.Data
    var title = config.title || D.getTitle(obj) || ''

    var overlay = Modal.createOverlay()
    var panel = Modal.createPanel(config.maxWidth || '1400px')
    overlay.appendChild(panel)

    // Save user's onClose before any wrapping
    var userOnClose = config.onClose
    var sidebarResult = null

    // Build the wrapped onClose FIRST, before header/footer capture it
    var wrappedOnClose = function () {
      document.removeEventListener('keydown', keyHandler)
      if (sidebarResult) {
        if (sidebarResult.collectionPanel) sidebarResult.collectionPanel.destroy()
        if (sidebarResult.recommendationPanel) sidebarResult.recommendationPanel.destroy()
      }
      if (overlay && overlay.parentNode) overlay.parentNode.removeChild(overlay)
      document.body.style.overflow = ''
      if (userOnClose) userOnClose()
    }
    config.onClose = wrappedOnClose

    var header = Modal.createHeader({
      title: title,
      onClose: wrappedOnClose,
      badges: config.badges || []
    })
    panel.appendChild(header)

    var body = document.createElement('div')
    body.style.cssText = 'flex:1;overflow:hidden;padding:0;display:flex'

    var leftCol = document.createElement('div')
    leftCol.style.cssText = 'flex:1;overflow-y:auto'

    if (config.mediaType === 'video' && config.videoUrl) {
      leftCol.appendChild(Modal.createVideoArea(config))
    }

    if (config.infoBar && config.infoBar.length > 0) {
      leftCol.appendChild(TaskUI.Dom.createInfoBar(config.infoBar))
    }

    if (config.contentRenderer) {
      var contentDiv = document.createElement('div')
      contentDiv.style.cssText = 'padding:16px'
      config.contentRenderer(contentDiv, obj)
      leftCol.appendChild(contentDiv)
    }

    body.appendChild(leftCol)

    if (config.sidebar === 'collection') {
      sidebarResult = Modal.createSidebar({
        type: config.type,
        currentId: config.currentId,
        tags: config.tags,
        onPlayItem: config.onPlayItem
      })
      if (sidebarResult) {
        body.appendChild(sidebarResult.element)
      }
    }

    panel.appendChild(body)

    var footer = Modal.createFooter({
      leftButtons: config.footerActions || [],
      closeText: config.closeText || '关闭',
      onClose: wrappedOnClose
    })
    panel.appendChild(footer)

    // Backdrop click — closure references config.onClose which is now wrappedOnClose
    overlay.addEventListener('click', function (e) {
      if (e.target === overlay && config.onClose) config.onClose()
    })

    // ESC key
    function keyHandler(e) {
      if (e.key === 'Escape' && config.onClose) config.onClose()
    }
    document.addEventListener('keydown', keyHandler)

    document.body.appendChild(overlay)
    document.body.style.overflow = 'hidden'

    return { overlay: overlay, panel: panel, close: wrappedOnClose }
  }

  window.TaskUI = window.TaskUI || {}
  window.TaskUI.Modal = Modal
})()