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
    if (obj && obj.extra && typeof obj.extra.tags === 'string') tags.push(obj.extra.tags)
    if (obj && obj.metadata && Array.isArray(obj.metadata.tags)) tags.push.apply(tags, obj.metadata.tags)
    if (obj && obj.metadata && typeof obj.metadata.tags === 'string') tags.push(obj.metadata.tags)
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
    // 优先级：extra.details > metadata.details > metadata.description > extra.description
    if (obj && obj.extra && obj.extra.details) s = obj.extra.details
    else if (obj && obj.metadata && obj.metadata.details) s = obj.metadata.details
    else if (obj && obj.metadata && obj.metadata.description) s = obj.metadata.description
    else if (obj && obj.extra && obj.extra.description) s = obj.extra.description
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