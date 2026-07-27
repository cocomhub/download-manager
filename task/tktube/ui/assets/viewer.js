/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

;(function () {
  'use strict'

  if (!window.__dm_uiBridge) return

  // ---- Helper functions ----

  function getTitle (obj) { return (obj && obj.metadata && obj.metadata.title) || '' }

  function getContentGroup (obj) {
    return (obj && obj.metadata && obj.metadata.content_group) || ''
  }

  function getResolution (obj) {
    return (obj && obj.metadata && obj.metadata.resolution) || ''
  }

  function getDate (obj) {
    return (obj && obj.metadata && obj.metadata.date) || ''
  }

  function getDuration (obj) {
    return (obj && obj.metadata && obj.metadata.duration) || ''
  }

  function getOriginLink (obj) {
    if (obj && obj.metadata && obj.metadata.page_url) return obj.metadata.page_url
    if (obj && obj.extra && obj.extra.origin_url) return obj.extra.origin_url
    return (obj && obj.url) || ''
  }

  function getTags (obj) {
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

  function getDetails (obj) {
    var s = ''
    if (obj && obj.extra && obj.extra.description) s = obj.extra.description
    else if (obj && obj.metadata && obj.metadata.description) s = obj.metadata.description
    else if (obj && obj.metadata && obj.metadata.details) s = obj.metadata.details
    return (typeof s === 'string' ? s : '').replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim()
  }

  function fileUrl (path) { return window.__dm_pathToUrl ? window.__dm_pathToUrl(path) : fileUrl_impl(path) }
  function fileUrl_impl(path) {
    if (!path) return ''
    var normalized = path.replace(/\\/g, '/')
    var root = typeof window.__dm_downloadRoot === 'string' ? window.__dm_downloadRoot : ''
    if (root && normalized.indexOf(root) === 0) {
      normalized = normalized.slice(root.length)
    }
    normalized = normalized.replace(/^\//, '')
    return '/files/' + normalized.split('/').filter(function(s){return s&&s!=='..'}).map(encodeURIComponent).join('/')
  }

  function getVideoUrl (obj) {
    if (!obj) return ''
    // Check extra.files for video type
    if (obj.extra && Array.isArray(obj.extra.files)) {
      for (var fi = 0; fi < obj.extra.files.length; fi++) {
        var f = obj.extra.files[fi]
        if (f && (f.type === 'video' || (f.path && /\.(mp4|webm|mkv|m3u8|ts)$/i.test(f.path)))) {
          if (f.path) return fileUrl(f.path)
        }
      }
    }
    // Check save_path
    if (obj.save_path) return fileUrl(obj.save_path)
    // Fall back to URL
    if (obj.url) return obj.url
    return ''
  }

  function getCoverImage (obj) {
    if (!obj) return ''
    // Local files — cover (local_preview is a video, not an image)
    if (obj.extra && obj.extra.local_cover) return fileUrl(obj.extra.local_cover)
    // Remote URLs — cover fields first
    if (obj.extra && obj.extra.cover_url) return obj.extra.cover_url
    if (obj.extra && obj.extra.cover) return obj.extra.cover
    // Check extra.files for image-type — cover before thumb
    if (obj.extra && Array.isArray(obj.extra.files)) {
      // First pass: cover-named files
      for (var fi = 0; fi < obj.extra.files.length; fi++) {
        var f = obj.extra.files[fi]
        if (f && f.type === 'image' && f.path) {
          var fname = (f.name || f.path || '').toString().toLowerCase()
          if (fname.indexOf('cover') >= 0) {
            return fileUrl(f.path)
          }
        }
      }
      // Second pass: thumb-named files (non-cover)
      for (var fi = 0; fi < obj.extra.files.length; fi++) {
        var f = obj.extra.files[fi]
        if (f && f.type === 'image' && f.path) {
          var fname = (f.name || f.path || '').toString().toLowerCase()
          if (fname.indexOf('thumb') >= 0) {
            return fileUrl(f.path)
          }
        }
      }
      // Fallback: first image
      for (var fi2 = 0; fi2 < obj.extra.files.length; fi2++) {
        var f2 = obj.extra.files[fi2]
        if (f2 && f2.type === 'image' && f2.path) return fileUrl(f2.path)
      }
    }
    return ''
  }

  function getThumbImage (obj) {
    if (!obj) return ''
    // Local files — preview/thumb before cover
    if (obj.extra && obj.extra.local_preview) return fileUrl(obj.extra.local_preview)
    if (obj.extra && obj.extra.local_cover) return fileUrl(obj.extra.local_cover)
    // Remote URLs — thumb fields before cover fields
    if (obj.extra && obj.extra.thumb_url) return obj.extra.thumb_url
    if (obj.extra && obj.extra.preview_url) return obj.extra.preview_url
    if (obj.extra && obj.extra.cover_url) return obj.extra.cover_url
    // Check extra.files for image-type — thumb before cover
    if (obj.extra && Array.isArray(obj.extra.files)) {
      // First pass: thumb-named files (non-cover)
      for (var fi = 0; fi < obj.extra.files.length; fi++) {
        var f = obj.extra.files[fi]
        if (f && f.type === 'image' && f.path) {
          var fname = (f.name || f.path || '').toString().toLowerCase()
          if (fname.indexOf('thumb') >= 0 && fname.indexOf('cover') < 0) {
            return fileUrl(f.path)
          }
        }
      }
      // Second pass: cover-named files
      for (var fi = 0; fi < obj.extra.files.length; fi++) {
        var f = obj.extra.files[fi]
        if (f && f.type === 'image' && f.path) {
          var fname = (f.name || f.path || '').toString().toLowerCase()
          if (fname.indexOf('cover') >= 0) {
            return fileUrl(f.path)
          }
        }
      }
      // Fallback: first image
      for (var fi2 = 0; fi2 < obj.extra.files.length; fi2++) {
        var f2 = obj.extra.files[fi2]
        if (f2 && f2.type === 'image' && f2.path) return fileUrl(f2.path)
      }
    }
    return ''
  }

  function getFileUrl (obj) {
    if (obj && obj.save_path) return fileUrl(obj.save_path)
    if (obj && obj.extra && Array.isArray(obj.extra.files)) {
      for (var fi = 0; fi < obj.extra.files.length; fi++) {
        var f = obj.extra.files[fi]
        if (f && f.path) return fileUrl(f.path)
      }
    }
    return ''
  }

  function statusColor (status) {
    switch (status) {
      case 'completed': return '#10b981'
      case 'downloading': return '#3b82f6'
      case 'failed': return '#ef4444'
      case 'cancelled': return '#9ca3af'
      default: return '#6b7280'
    }
  }

  function statusBg (status) {
    switch (status) {
      case 'completed': return '#d1fae5'
      case 'downloading': return '#dbeafe'
      case 'failed': return '#fee2e2'
      case 'cancelled': return '#f3f4f6'
      default: return '#f3f4f6'
    }
  }

  // ---- Theme colors ----
  var THEME = {
    primary: '#3b82f6',
    primaryDark: '#2563eb',
    bg: '#ffffff',
    bgAlt: '#f9fafb',
    border: '#e5e7eb',
    text: '#1f2937',
    textSecondary: '#6b7280',
    textMuted: '#9ca3af'
  }

  // ---- Modal ----

  var activeModal = null

  function closeModal () {
    if (activeModal) {
      var video = activeModal.querySelector('video')
      if (video) { video.pause(); video.src = '' }
      document.body.removeChild(activeModal)
      activeModal = null
      document.body.style.overflow = ''
    }
  }

  function createModal (obj) {
    closeModal()

    var videoUrl = getVideoUrl(obj)
    var coverUrl = getCoverImage(obj)
    var title = getTitle(obj) || 'TKTube'
    var res = getResolution(obj)
    var dur = getDuration(obj)
    var dateVal = getDate(obj)
    var contentGroup = getContentGroup(obj)
    var origin = getOriginLink(obj)
    var details = getDetails(obj)
    var tags = getTags(obj)
    var fileUrlVal = getFileUrl(obj)

    var isHLS = /\.m3u8(\?.*)?$/i.test(videoUrl)
    var isSafari = /safari/i.test(navigator.userAgent) && !/chrome|crios|chromium|edg/i.test(navigator.userAgent)
    var useVideo = !!videoUrl && (!isHLS || isSafari)

    // Add CSS for semi-transparent video controls
    var style = document.createElement('style')
    style.textContent = '.dm-video-player::-webkit-media-controls { opacity:0.6 !important; transition:opacity 0.3s } .dm-video-player::-webkit-media-controls:hover { opacity:1 !important } .dm-video-player::-webkit-media-controls-panel { background:rgba(0,0,0,0.3) !important }'
    document.head.appendChild(style)

    // Overlay
    var overlay = document.createElement('div')
    overlay.style.cssText = 'position:fixed;inset:0;background:rgba(0,0,0,0.7);z-index:50;display:flex;align-items:center;justify-content:center;padding:16px;-webkit-backdrop-filter:blur(4px);backdrop-filter:blur(4px)'

    // Panel
    var panel = document.createElement('div')
    panel.style.cssText = 'background:#fff;border-radius:8px;box-shadow:0 25px 50px rgba(0,0,0,0.25);width:100%;max-width:1400px;max-height:90vh;overflow:hidden;display:flex;flex-direction:column'
    overlay.appendChild(panel)

    // Header
    var header = document.createElement('div')
    header.style.cssText = 'padding:16px;border-bottom:1px solid #e5e7eb;display:flex;justify-content:space-between;align-items:center;background:#f9fafb'
    var hTitle = document.createElement('h3')
    hTitle.style.cssText = 'font-size:18px;font-weight:700;color:#1f2937;margin:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap'
    hTitle.className = 'viewer-title'
    hTitle.textContent = title
    header.appendChild(hTitle)

    var headerRight = document.createElement('div')
    headerRight.style.cssText = 'display:flex;align-items:center;gap:12px;flex-shrink:0'

    if (contentGroup) {
      var groupBadge = document.createElement('span')
      groupBadge.style.cssText = 'font-size:11px;background:#eff6ff;color:#2563eb;padding:2px 8px;border-radius:4px'
      groupBadge.textContent = contentGroup
      headerRight.appendChild(groupBadge)
    }

    if (res) {
      var resBadge = document.createElement('span')
      resBadge.style.cssText = 'font-size:11px;background:#f3f4f6;color:#4b5563;padding:2px 8px;border-radius:4px'
      resBadge.textContent = res
      headerRight.appendChild(resBadge)
    }

    var hClose = document.createElement('button')
    hClose.innerHTML = '<i class="fas fa-times"></i>'
    hClose.style.cssText = 'color:#6b7280;cursor:pointer;background:none;border:none;font-size:18px;margin-left:8px'
    hClose.onclick = closeModal
    headerRight.appendChild(hClose)

    header.appendChild(headerRight)
    panel.appendChild(header)

    // Body - two-column layout: left = video + info, right = related videos (placeholder)
    var body = document.createElement('div')
    body.style.cssText = 'flex:1;overflow:hidden;padding:0;display:flex'

    // Left column: video + info
    var leftCol = document.createElement('div')
    leftCol.style.cssText = 'flex:1;overflow-y:auto'

    // Video area - 16:9 aspect ratio
    var mediaArea = document.createElement('div')
    mediaArea.style.cssText = 'background:#000;display:flex;align-items:center;justify-content:center;position:relative;aspect-ratio:16/9;overflow:hidden'
    mediaArea.className = 'viewer-media-area'

    if (useVideo) {
      var posterImg = document.createElement('img')
      posterImg.src = coverUrl || videoUrl
      posterImg.style.cssText = 'width:100%;height:100%;object-fit:contain;cursor:pointer'
      posterImg.alt = title
      mediaArea.appendChild(posterImg)

      var playOverlay = document.createElement('div')
      playOverlay.style.cssText = 'position:absolute;inset:0;display:flex;align-items:center;justify-content:center;background:rgba(0,0,0,0.2);cursor:pointer'
      playOverlay.innerHTML = '<i class="fas fa-play" style="font-size:48px;color:#fff;opacity:0.8;text-shadow:0 2px 8px rgba(0,0,0,0.5)"></i>'
      mediaArea.appendChild(playOverlay)

      var video = document.createElement('video')
      video.src = videoUrl
      video.poster = coverUrl
      video.controls = true
      video.style.cssText = 'width:100%;height:100%;outline:none;display:none'
      // Semi-transparent video controls to avoid blocking subtitles
      video.classList.add('dm-video-player')

      var playHandler = function () {
        posterImg.style.display = 'none'
        playOverlay.style.display = 'none'
        video.style.display = 'block'
        video.play().catch(function () {})
      }
      posterImg.onclick = playHandler
      playOverlay.onclick = playHandler
      mediaArea.appendChild(video)
    } else if (videoUrl) {
      // HLS or unsupported format — show poster
      var posterImg = document.createElement('img')
      posterImg.src = coverUrl || ''
      posterImg.style.cssText = 'width:100%;height:100%;object-fit:contain'
      posterImg.alt = title
      if (posterImg.src) mediaArea.appendChild(posterImg)
    } else {
      // No video — show placeholder
      var placeholder = document.createElement('div')
      placeholder.style.cssText = 'display:flex;align-items:center;justify-content:center;height:100%;color:#9ca3af;font-size:14px'
      placeholder.innerHTML = '<i class="fas fa-video" style="font-size:48px;margin-right:12px;opacity:0.5"></i> 无可用视频'
      mediaArea.appendChild(placeholder)
    }
    leftCol.appendChild(mediaArea)

    // Info bar
    var infoBar = document.createElement('div')
    infoBar.style.cssText = 'display:flex;gap:16px;padding:12px 16px;background:#f9fafb;border-bottom:1px solid #e5e7eb;font-size:13px;color:#6b7280;flex-wrap:wrap'
    infoBar.className = 'viewer-meta'

    if (dur) {
      var durEl = document.createElement('span')
      durEl.innerHTML = '<i class="fas fa-clock" style="margin-right:4px"></i> ' + dur
      infoBar.appendChild(durEl)
    }
    if (dateVal) {
      var dateEl = document.createElement('span')
      dateEl.innerHTML = '<i class="fas fa-calendar" style="margin-right:4px"></i> ' + dateVal
      infoBar.appendChild(dateEl)
    }
    if (res) {
      var resEl = document.createElement('span')
      resEl.innerHTML = '<i class="fas fa-expand" style="margin-right:4px"></i> ' + res
      infoBar.appendChild(resEl)
    }
    if (contentGroup) {
      var groupEl = document.createElement('span')
      groupEl.innerHTML = '<i class="fas fa-folder" style="margin-right:4px"></i> ' + contentGroup
      infoBar.appendChild(groupEl)
    }
    if (infoBar.children.length > 0) {
      leftCol.appendChild(infoBar)
    }

    // Content area (no origin bar — buttons in footer)
    var content = document.createElement('div')
    content.style.cssText = 'padding:16px'

    // Details
    if (details) {
      var detailsEl = document.createElement('div')
      detailsEl.style.cssText = 'font-size:14px;color:#374151;line-height:1.6;white-space:pre-line;margin-bottom:12px'
      detailsEl.textContent = details
      content.appendChild(detailsEl)
    }

    // Tags chips
    if (tags.length > 0) {
      var tagWrap = document.createElement('div')
      tagWrap.style.cssText = 'display:flex;flex-wrap:wrap;gap:4px;margin-bottom:12px'
      tags.forEach(function (tag) {
        var t = document.createElement('span')
        t.style.cssText = 'font-size:11px;background:#f3f4f6;color:#4b5563;padding:2px 8px;border-radius:4px'
        t.textContent = '#' + tag
        tagWrap.appendChild(t)
      })
      content.appendChild(tagWrap)
    }

    leftCol.appendChild(content)
    body.appendChild(leftCol)

    // Right column: collection + recommendation panels
    var rightCol = document.createElement('div')
    rightCol.style.cssText = 'width:380px;border-left:1px solid #e5e7eb;display:flex;flex-direction:column;overflow:hidden;flex-shrink:0;background:#fff'

    var collectionPanel = null
    var recommendationPanel = null
    var taskType = obj && obj.metadata && obj.metadata.task_type
    var objTags = []
    if (obj && obj.extra && Array.isArray(obj.extra.tags)) {
      objTags = obj.extra.tags
    }

    // 合集面板
    if (taskType && obj) {
      collectionPanel = CollectionPanel.create({
        type: taskType,
        currentId: obj.id,
        onPlayItem: function (item) {
          AppAPI.getObject(taskType, item.id).then(function (newObj) {
            if (collectionPanel) collectionPanel.update({ currentId: item.id })
            // 更新媒体区域
            var mediaArea = rightCol.parentElement && rightCol.parentElement.querySelector('.viewer-media-area')
            if (mediaArea) {
              var posterImg = mediaArea.querySelector('img')
              if (posterImg) {
                posterImg.src = getCoverImage(newObj) || getVideoUrl(newObj)
                posterImg.alt = (newObj.metadata && newObj.metadata.title) || newObj.url
              }
              var video = mediaArea.querySelector('video')
              if (video) {
                video.src = getVideoUrl(newObj)
                video.poster = getCoverImage(newObj)
                video.load()
              }
            }
            // 更新标题
            var titleEl = rightCol.parentElement && rightCol.parentElement.querySelector('.viewer-title')
            if (titleEl) titleEl.textContent = (newObj.metadata && newObj.metadata.title) || newObj.url
            // 更新元数据
            var metaEl = rightCol.parentElement && rightCol.parentElement.querySelector('.viewer-meta')
            if (metaEl) {
              metaEl.innerHTML = ''
              if (newObj.metadata) {
                if (newObj.metadata.duration) {
                  var durEl = document.createElement('span')
                  durEl.innerHTML = '<i class="fas fa-clock" style="margin-right:4px"></i> ' + newObj.metadata.duration
                  metaEl.appendChild(durEl)
                }
                if (newObj.metadata.date) {
                  var dateEl = document.createElement('span')
                  dateEl.innerHTML = '<i class="fas fa-calendar" style="margin-right:4px"></i> ' + newObj.metadata.date
                  metaEl.appendChild(dateEl)
                }
                if (newObj.metadata.resolution) {
                  var resEl = document.createElement('span')
                  resEl.innerHTML = '<i class="fas fa-tag" style="margin-right:4px"></i> ' + newObj.metadata.resolution
                  metaEl.appendChild(resEl)
                }
              }
            }
          })
        }
      })
      rightCol.appendChild(collectionPanel.element)
    }

    // 推荐面板
    if (taskType && obj) {
      recommendationPanel = RecommendationPanel.create({
        type: taskType,
        currentId: obj.id,
        tags: objTags,
        onPlayItem: function (item) {
          AppAPI.getObject(taskType, item.id).then(function (newObj) {
            closeModal()
            createModal(newObj)
          })
        }
      })
      rightCol.appendChild(recommendationPanel.element)
    }

    body.appendChild(rightCol)

    panel.appendChild(body)

    // Footer
    var footer = document.createElement('div')
    footer.style.cssText = 'padding:12px 16px;border-top:1px solid #e5e7eb;background:#f9fafb;display:flex;justify-content:space-between;align-items:center'

    var fLeft = document.createElement('div')
    fLeft.style.cssText = 'display:flex;gap:8px'

    if (fileUrlVal) {
      var openFileBtn = document.createElement('a')
      openFileBtn.href = fileUrlVal
      openFileBtn.target = '_blank'
      openFileBtn.rel = 'noopener noreferrer'
      openFileBtn.style.cssText = 'padding:6px 12px;border-radius:6px;background:#2563eb;color:#fff;text-decoration:none;font-size:14px;display:inline-block'
      openFileBtn.textContent = '打开文件'
      fLeft.appendChild(openFileBtn)
    }

    if (origin) {
      var originFooterBtn = document.createElement('a')
      originFooterBtn.href = /^https?:\/\//i.test(origin) ? origin : '#'
      originFooterBtn.target = '_blank'
      originFooterBtn.rel = 'noopener noreferrer'
      originFooterBtn.style.cssText = 'padding:6px 12px;border-radius:6px;background:#fff;border:1px solid #d1d5db;text-decoration:none;cursor:pointer;font-size:14px;color:#374151;display:inline-block'
      originFooterBtn.textContent = '打开原页面'
      fLeft.appendChild(originFooterBtn)
    }

    var copyTitleBtn = document.createElement('button')
    copyTitleBtn.style.cssText = 'padding:6px 12px;border-radius:6px;background:#fff;border:1px solid #d1d5db;cursor:pointer;font-size:14px'
    copyTitleBtn.textContent = '复制标题'
    copyTitleBtn.onclick = function () {
      var t = getTitle(obj)
      if (t && navigator.clipboard && navigator.clipboard.writeText) navigator.clipboard.writeText(t)
    }
    fLeft.appendChild(copyTitleBtn)

    if (origin) {
      var copyLinkBtn = document.createElement('button')
      copyLinkBtn.style.cssText = 'padding:6px 12px;border-radius:6px;background:#fff;border:1px solid #d1d5db;cursor:pointer;font-size:14px'
      copyLinkBtn.textContent = '复制链接'
      copyLinkBtn.onclick = function () {
        if (navigator.clipboard && navigator.clipboard.writeText) navigator.clipboard.writeText(origin)
      }
      fLeft.appendChild(copyLinkBtn)
    }

    footer.appendChild(fLeft)

    var closeBtn = document.createElement('button')
    closeBtn.style.cssText = 'padding:6px 12px;border-radius:6px;background:#fff;border:1px solid #d1d5db;cursor:pointer;font-size:14px'
    closeBtn.textContent = '关闭'
    closeBtn.onclick = closeModal
    footer.appendChild(closeBtn)

    panel.appendChild(footer)

    // Backdrop click
    overlay.addEventListener('click', function (e) {
      if (e.target === overlay) closeModal()
    })

    // Escape
    function keyHandler (e) {
      if (e.key === 'Escape') closeModal()
    }
    document.addEventListener('keydown', keyHandler)

    // Override closeModal for cleanup
    closeModal = function () {
      document.removeEventListener('keydown', keyHandler)
      document.body.style.overflow = ''
      if (collectionPanel) collectionPanel.destroy()
      if (recommendationPanel) recommendationPanel.destroy()
      if (activeModal) {
        document.body.removeChild(activeModal)
        activeModal = null
      }
    }

    document.body.appendChild(overlay)
    activeModal = overlay
    document.body.style.overflow = 'hidden'
  }

  // ---- Sorting ----
  function priorityScore (obj) {
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

  // ---- Task view (legacy) ----

  function renderTaskView (task) {
    var container = document.getElementById('custom-task-container')
    if (!container) return

    // Clear
    container.innerHTML = ''

    var objects = (task && task.objects) || []
    if (objects.length === 0) {
      container.innerHTML = '<div style="padding:32px;text-align:center;color:' + THEME.textSecondary + ';font-size:14px">暂无对象</div>'
      return
    }

    // Group by content_group
    var groups = {}
    var ungrouped = []
    objects.forEach(function (obj) {
      var g = getContentGroup(obj)
      if (g) {
        if (!groups[g]) groups[g] = []
        groups[g].push(obj)
      } else {
        ungrouped.push(obj)
      }
    })

    // Sort groups by highest priority
    var groupNames = Object.keys(groups).sort(function (a, b) {
      var aMax = Math.max.apply(null, groups[a].map(priorityScore))
      var bMax = Math.max.apply(null, groups[b].map(priorityScore))
      return bMax - aMax
    })

    // Create wrapper
    var wrapper = document.createElement('div')
    wrapper.style.cssText = 'padding:16px;overflow-y:auto;height:100%'

    // If we have groups, show group-based layout
    if (groupNames.length > 0) {
      groupNames.forEach(function (groupName) {
        var items = groups[groupName]
        items.sort(function (a, b) { return priorityScore(b) - priorityScore(a) })

        var section = document.createElement('div')
        section.style.cssText = 'margin-bottom:24px'

        // Group header
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

        // Items grid
        var grid = document.createElement('div')
        grid.style.cssText = 'display:grid;grid-template-columns:repeat(auto-fill,minmax(180px,1fr));gap:12px'

        items.forEach(function (obj) {
          var card = createObjectCard(obj)
          grid.appendChild(card)
        })

        section.appendChild(grid)
        wrapper.appendChild(section)
      })
    }

    // Ungrouped objects
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

      ungrouped.forEach(function (obj) {
        var card = createObjectCard(obj)
        grid.appendChild(card)
      })

      section.appendChild(grid)
      wrapper.appendChild(section)
    }

    container.appendChild(wrapper)
  }

  function createObjectCard (obj) {
    var card = document.createElement('div')
    card.style.cssText = 'border:1px solid ' + THEME.border + ';border-radius:8px;overflow:hidden;background:' + THEME.bg + ';transition:box-shadow 0.2s'
    card.onmouseenter = function () { card.style.boxShadow = '0 4px 12px rgba(0,0,0,0.1)' }
    card.onmouseleave = function () { card.style.boxShadow = 'none' }

    // Cover / placeholder
    var coverArea = document.createElement('div')
    coverArea.style.cssText = 'position:relative;background:#f3f4f6;aspect-ratio:16/9;display:flex;align-items:center;justify-content:center;overflow:hidden'

    var coverUrl = getThumbImage(obj)
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

    // Status badge overlay
    var badge = document.createElement('span')
    badge.style.cssText = 'position:absolute;top:6px;right:6px;font-size:10px;padding:2px 6px;border-radius:4px;font-weight:600;background:' + statusBg(obj.status) + ';color:' + statusColor(obj.status)
    badge.textContent = obj.status || 'unknown'
    coverArea.appendChild(badge)

    // Resolution badge if available
    var res = getResolution(obj)
    if (res) {
      var resBadge = document.createElement('span')
      resBadge.style.cssText = 'position:absolute;bottom:6px;right:6px;font-size:10px;padding:2px 6px;border-radius:4px;background:rgba(0,0,0,0.7);color:#fff'
      resBadge.textContent = res
      coverArea.appendChild(resBadge)
    }

    card.appendChild(coverArea)

    // Info area
    var info = document.createElement('div')
    info.style.cssText = 'padding:10px 12px'

    var titleEl = document.createElement('div')
    titleEl.style.cssText = 'font-size:13px;font-weight:600;color:' + THEME.text + ';line-height:1.3;overflow:hidden;text-overflow:ellipsis;white-space:nowrap'
    titleEl.textContent = getTitle(obj) || obj.url || 'Untitled'
    info.appendChild(titleEl)

    // Metadata row
    var metaRow = document.createElement('div')
    metaRow.style.cssText = 'display:flex;gap:8px;font-size:11px;color:' + THEME.textSecondary + ';margin-top:4px'

    var dur = getDuration(obj)
    if (dur) {
      var durEl = document.createElement('span')
      durEl.textContent = dur
      metaRow.appendChild(durEl)
    }

    var dateVal = getDate(obj)
    if (dateVal) {
      if (dur) {
        var sep = document.createElement('span')
        sep.textContent = '|'
        metaRow.appendChild(sep)
      }
      var dateEl = document.createElement('span')
      dateEl.textContent = dateVal
      metaRow.appendChild(dateEl)
    }

    if (dur || dateVal) info.appendChild(metaRow)

    // Progress bar
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

    // Click to open modal
    card.style.cursor = 'pointer'
    card.onclick = function () {
      if (obj.status === 'completed') {
        createModal(obj)
      }
    }

    return card
  }

  // Register with TaskUI (new plugin system)
  if (typeof TaskUI !== 'undefined' && TaskUI.register) {
    TaskUI.register('tktube', {
      type: 'tktube',
      label: 'TKTube',
      icon: 'fa-video',
      viewerLabel: '查看',
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
      collectExtra: function (formData) {
        var extra = {}
        if (formData.keyword) extra.keyword = formData.keyword
        if (formData.subtype) extra.subtype = formData.subtype
        if (formData.max_concurrent) extra.max_concurrent = formData.max_concurrent
        if (formData.refresh_interval) extra.refresh_interval = formData.refresh_interval
        return extra
      },
      shouldShowViewer: function (obj) { return obj.status === 'completed' },
      onClick: function (obj, helpers) {
        if (obj.status !== 'completed') return false
        helpers.openTaskTypeViewer(obj)
        return true
      },
      renderViewer: function (h, obj, onClose) {
        // Shared clipboard helper
        function copyToClipboard(text) {
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

        var videoUrl = getVideoUrl(obj)
        var coverUrl = getCoverImage(obj)
        var title = getTitle(obj) || 'TKTube'
        var res = getResolution(obj)
        var dur = getDuration(obj)
        var dateVal = getDate(obj)
        var contentGroup = getContentGroup(obj)
        var origin = getOriginLink(obj)
        var details = getDetails(obj)
        var tags = getTags(obj)
        var fileUrlVal = getFileUrl(obj)

        var isHLS = /\.m3u8(\?.*)?$/i.test(videoUrl)
        var isSafari = /safari/i.test(navigator.userAgent) && !/chrome|crios|chromium|edg/i.test(navigator.userAgent)
        var useVideo = !!videoUrl && (!isHLS || isSafari)

        // Build DOM modal directly
        var overlay = document.createElement('div')
        overlay.className = 'fixed inset-0 bg-black bg-opacity-70 z-50 flex items-center justify-center p-4 backdrop-blur-sm'
        overlay.style.cssText = 'position:fixed;inset:0;background:rgba(0,0,0,0.7);z-index:50;display:flex;align-items:center;justify-content:center;padding:16px;-webkit-backdrop-filter:blur(4px);backdrop-filter:blur(4px)'

        var panel = document.createElement('div')
        panel.className = 'bg-white rounded-lg shadow-2xl w-full max-w-5xl max-h-[90vh] overflow-hidden flex flex-col'
        panel.style.cssText = 'background:#fff;border-radius:8px;box-shadow:0 25px 50px rgba(0,0,0,0.25);width:100%;max-width:1400px;max-height:90vh;overflow:hidden;display:flex;flex-direction:column'

        var modalRef = { overlay: overlay, panel: panel }

        var originalOnClose = onClose
        onClose = function () {
          if (modalRef.overlay && modalRef.overlay.parentNode) {
            modalRef.overlay.parentNode.removeChild(modalRef.overlay)
          }
          document.body.style.overflow = ''
          if (originalOnClose) originalOnClose()
        }
        overlay.appendChild(panel)

        // Header
        var header = document.createElement('div')
        header.style.cssText = 'padding:16px;border-bottom:1px solid #e5e7eb;display:flex;justify-content:space-between;align-items:center;background:#f9fafb'
        var hTitle = document.createElement('h3')
        hTitle.style.cssText = 'font-size:18px;font-weight:700;color:#1f2937;margin:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap'
        hTitle.className = 'viewer-title'
        hTitle.textContent = title
        header.appendChild(hTitle)

        var headerRight = document.createElement('div')
        headerRight.style.cssText = 'display:flex;align-items:center;gap:12px;flex-shrink:0'
        if (contentGroup) {
          var groupBadge = document.createElement('span')
          groupBadge.style.cssText = 'font-size:11px;background:#eff6ff;color:#2563eb;padding:2px 8px;border-radius:4px'
          groupBadge.textContent = contentGroup
          headerRight.appendChild(groupBadge)
        }
        if (res) {
          var resBadge = document.createElement('span')
          resBadge.style.cssText = 'font-size:11px;background:#f3f4f6;color:#4b5563;padding:2px 8px;border-radius:4px'
          resBadge.textContent = res
          headerRight.appendChild(resBadge)
        }
        var hClose = document.createElement('button')
        hClose.innerHTML = '<i class="fas fa-times"></i>'
        hClose.style.cssText = 'color:#6b7280;cursor:pointer;background:none;border:none;font-size:18px;margin-left:8px'
        hClose.onclick = function (e) { e.stopPropagation(); onClose() }
        headerRight.appendChild(hClose)
        header.appendChild(headerRight)
        panel.appendChild(header)

        // Body - two-column layout: left = video + info, right = related videos (placeholder)
        var body = document.createElement('div')
        body.style.cssText = 'flex:1;overflow:hidden;padding:0;display:flex'

        // Left column: video + info
        var leftCol = document.createElement('div')
        leftCol.style.cssText = 'flex:1;overflow-y:auto'

        // Video area - fixed height
        var mediaArea = document.createElement('div')
        mediaArea.style.cssText = 'background:#000;display:flex;align-items:center;justify-content:center;position:relative;aspect-ratio:16/9;overflow:hidden'
        mediaArea.className = 'viewer-media-area'

        if (useVideo) {
          var posterImg = document.createElement('img')
          posterImg.src = coverUrl || videoUrl
          posterImg.style.cssText = 'width:100%;height:100%;object-fit:contain;cursor:pointer'
          posterImg.alt = title
          mediaArea.appendChild(posterImg)

          var playOverlay = document.createElement('div')
          playOverlay.style.cssText = 'position:absolute;inset:0;display:flex;align-items:center;justify-content:center;background:rgba(0,0,0,0.2);cursor:pointer'
          playOverlay.innerHTML = '<i class="fas fa-play" style="font-size:48px;color:#fff;opacity:0.8;text-shadow:0 2px 8px rgba(0,0,0,0.5)"></i>'
          mediaArea.appendChild(playOverlay)

          var video = document.createElement('video')
          video.src = videoUrl
          video.poster = coverUrl
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
          mediaArea.appendChild(video)
        } else if (videoUrl) {
          var posterImg = document.createElement('img')
          posterImg.src = coverUrl || ''
          posterImg.style.cssText = 'width:100%;height:100%;object-fit:contain'
          posterImg.alt = title
          if (posterImg.src) mediaArea.appendChild(posterImg)
        } else {
          var placeholder = document.createElement('div')
          placeholder.style.cssText = 'display:flex;align-items:center;justify-content:center;height:100%;color:#9ca3af;font-size:14px'
          placeholder.innerHTML = '<i class="fas fa-video" style="font-size:48px;margin-right:12px;opacity:0.5"></i> 无可用视频'
          mediaArea.appendChild(placeholder)
        }
        leftCol.appendChild(mediaArea)

        // Info bar
        var infoBar = document.createElement('div')
        infoBar.style.cssText = 'display:flex;gap:16px;padding:12px 16px;background:#f9fafb;border-bottom:1px solid #e5e7eb;font-size:13px;color:#6b7280;flex-wrap:wrap'
        infoBar.className = 'viewer-meta'
        if (dur) { var durEl = document.createElement('span'); durEl.innerHTML = '<i class="fas fa-clock" style="margin-right:4px"></i> ' + dur; infoBar.appendChild(durEl) }
        if (dateVal) { var dateEl = document.createElement('span'); dateEl.innerHTML = '<i class="fas fa-calendar" style="margin-right:4px"></i> ' + dateVal; infoBar.appendChild(dateEl) }
        if (res) { var resEl = document.createElement('span'); resEl.innerHTML = '<i class="fas fa-expand" style="margin-right:4px"></i> ' + res; infoBar.appendChild(resEl) }
        if (contentGroup) { var groupEl = document.createElement('span'); groupEl.innerHTML = '<i class="fas fa-folder" style="margin-right:4px"></i> ' + contentGroup; infoBar.appendChild(groupEl) }
        if (infoBar.children.length > 0) leftCol.appendChild(infoBar)

        // Content area (no origin bar — buttons in footer)
        var content = document.createElement('div')
        content.style.cssText = 'padding:16px'
        if (details) { var de = document.createElement('div'); de.style.cssText = 'font-size:14px;color:#374151;line-height:1.6;white-space:pre-line;margin-bottom:12px'; de.textContent = details; content.appendChild(de) }
        if (tags.length > 0) {
          var tagWrap = document.createElement('div'); tagWrap.style.cssText = 'display:flex;flex-wrap:wrap;gap:4px;margin-bottom:12px'
          tags.forEach(function (tag) { var t = document.createElement('span'); t.style.cssText = 'font-size:11px;background:#f3f4f6;color:#4b5563;padding:2px 8px;border-radius:4px'; t.textContent = '#' + tag; tagWrap.appendChild(t) })
          content.appendChild(tagWrap)
        }
        leftCol.appendChild(content)
        body.appendChild(leftCol)

        // Right column: collection + recommendation panels
        var rightCol = document.createElement('div')
        rightCol.style.cssText = 'width:380px;border-left:1px solid #e5e7eb;display:flex;flex-direction:column;overflow:hidden;flex-shrink:0;background:#fff'

        var collectionPanel = null
        var recommendationPanel = null
        var taskType = obj && obj.metadata && obj.metadata.task_type
        var objTags = []
        if (obj && obj.extra && Array.isArray(obj.extra.tags)) {
          objTags = obj.extra.tags
        }

        // 合集面板
        if (taskType && obj) {
          collectionPanel = CollectionPanel.create({
            type: taskType,
            currentId: obj.id,
            onPlayItem: function (item) {
              AppAPI.getObject(taskType, item.id).then(function (newObj) {
                closeModal()
                createModal(newObj)
              })
            }
          })
          rightCol.appendChild(collectionPanel.element)
        }

        // 推荐面板
        if (taskType && obj) {
          recommendationPanel = RecommendationPanel.create({
            type: taskType,
            currentId: obj.id,
            tags: objTags,
            onPlayItem: function (item) {
              AppAPI.getObject(taskType, item.id).then(function (newObj) {
                closeModal()
                createModal(newObj)
              })
            }
          })
          rightCol.appendChild(recommendationPanel.element)
        }

        body.appendChild(rightCol)

        panel.appendChild(body)

        // Footer
        var footer = document.createElement('div')
        footer.style.cssText = 'padding:12px 16px;border-top:1px solid #e5e7eb;background:#f9fafb;display:flex;justify-content:space-between;align-items:center'
        var fLeft = document.createElement('div'); fLeft.style.cssText = 'display:flex;gap:8px'
        if (fileUrlVal) {
          var openFileBtn = document.createElement('a'); openFileBtn.href = fileUrlVal; openFileBtn.target = '_blank'; openFileBtn.rel = 'noopener noreferrer'; openFileBtn.style.cssText = 'padding:6px 12px;border-radius:6px;background:#2563eb;color:#fff;text-decoration:none;font-size:14px;display:inline-block'; openFileBtn.textContent = '打开文件'; fLeft.appendChild(openFileBtn)
        }
        if (origin) {
          var originFooterBtn = document.createElement('a'); originFooterBtn.href = /^https?:\/\//i.test(origin) ? origin : '#'; originFooterBtn.target = '_blank'; originFooterBtn.rel = 'noopener noreferrer'; originFooterBtn.style.cssText = 'padding:6px 12px;border-radius:6px;background:#fff;border:1px solid #d1d5db;text-decoration:none;cursor:pointer;font-size:14px;color:#374151;display:inline-block'; originFooterBtn.textContent = '打开原页面'; fLeft.appendChild(originFooterBtn)
        }
        var copyTitleBtn = document.createElement('button'); copyTitleBtn.style.cssText = 'padding:6px 12px;border-radius:6px;background:#fff;border:1px solid #d1d5db;cursor:pointer;font-size:14px'; copyTitleBtn.textContent = '复制标题'; copyTitleBtn.onclick = function () { copyToClipboard(getTitle(obj)) }; fLeft.appendChild(copyTitleBtn)
        if (origin) { var copyLinkBtn = document.createElement('button'); copyLinkBtn.style.cssText = 'padding:6px 12px;border-radius:6px;background:#fff;border:1px solid #d1d5db;cursor:pointer;font-size:14px'; copyLinkBtn.textContent = '复制链接'; copyLinkBtn.onclick = function () { copyToClipboard(origin) }; fLeft.appendChild(copyLinkBtn) }
        footer.appendChild(fLeft)
        var closeBtn = document.createElement('button'); closeBtn.style.cssText = 'padding:6px 12px;border-radius:6px;background:#fff;border:1px solid #d1d5db;cursor:pointer;font-size:14px'; closeBtn.textContent = '关闭'; closeBtn.onclick = function (e) { e.stopPropagation(); onClose() }; footer.appendChild(closeBtn)
        panel.appendChild(footer)

        // Backdrop click
        overlay.addEventListener('click', function (e) { if (e.target === overlay) onClose() })

        // Escape key
        function keyHandler(e) {
          if (e.key === 'Escape') onClose()
        }
        document.addEventListener('keydown', keyHandler)

        // Override onClose to clean up key listener
        var origOnClose_ = onClose
        onClose = function () {
          document.removeEventListener('keydown', keyHandler)
          if (collectionPanel) collectionPanel.destroy()
          if (recommendationPanel) recommendationPanel.destroy()
          if (modalRef.overlay && modalRef.overlay.parentNode) {
            modalRef.overlay.parentNode.removeChild(modalRef.overlay)
          }
          document.body.style.overflow = ''
          if (origOnClose_) origOnClose_()
        }

        // Mount
        document.body.appendChild(overlay)
        document.body.style.overflow = 'hidden'

        return h('div')
      }
    })
  }

  // Register as task view (legacy compat)
  window.__dm_uiBridge.registerTaskView('tktube', {
    render: function (task) {
      renderTaskView(task)
    }
  })
})()