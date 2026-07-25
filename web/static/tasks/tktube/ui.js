// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * tktube 类型 UI 组件
 * 自定义表单、元数据、查看器，保留聚合视图通过 __dm_uiBridge 兼容
 */
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
      var videoUrl = (obj && obj.save_path) ? fileUrl(obj.save_path) : ''
      return TaskUI.BaseViewer(h, obj, onClose, function (h, obj) {
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