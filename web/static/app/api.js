// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * API helpers — pure fetch wrappers, no Vue dependency.
 * Exposed as window.AppAPI.
 */
;(function () {
  'use strict'

  // ---- Standalone utility functions (no Vue dependency) ----

  // pathToUrl 将本地路径转换为可访问的 URL 路径
  window.__dm_pathToUrl = function (path) {
    if (!path) return ''
    var normalized = path.replace(/\\/g, '/')
    // Strip download root prefix if the path is absolute
    // (e.g. /opt/.../downloads/hanime/... → hanime/...)
    var downloadRoot = typeof window.__dm_downloadRoot === 'string' ? window.__dm_downloadRoot : ''
    if (downloadRoot && normalized.indexOf(downloadRoot) === 0) {
      normalized = normalized.slice(downloadRoot.length)
    }
    // Strip leading slash for relative path
    normalized = normalized.replace(/^\//, '')
    return '/files/' + normalized.split('/').map(function (seg) {
      return encodeURIComponent(seg)
    }).join('/')
  }

  // getThumbImage 获取缩略图（小尺寸，用于合集、推荐列表）
  window.__dm_getThumbImage = function (obj) {
    if (!obj) return ''
    if (obj.extra && obj.extra.thumb_url) return obj.extra.thumb_url
    if (obj.extra && obj.extra.local_cover) return window.__dm_pathToUrl(obj.extra.local_cover)
    if (obj.extra && obj.extra.cover_url) return obj.extra.cover_url
    if (obj.extra && obj.extra.cover) return obj.extra.cover
    if (obj.extra && obj.extra.preview_url) return obj.extra.preview_url
    if (obj.extra && obj.extra.local_url) return window.__dm_pathToUrl(obj.extra.local_url)
    // 从 extra.files 中找第一个图片
    if (obj.extra && Array.isArray(obj.extra.files)) {
      for (var fi = 0; fi < obj.extra.files.length; fi++) {
        var f = obj.extra.files[fi]
        if (f && f.type === 'image' && f.path) return window.__dm_pathToUrl(f.path)
      }
    }
    return ''
  }

  // getCoverImage 获取封面图（大尺寸，用于视频播放器海报）
  window.__dm_getCoverImage = function (obj) {
    if (!obj) return ''
    if (obj.extra && obj.extra.local_cover) return window.__dm_pathToUrl(obj.extra.local_cover)
    if (obj.extra && obj.extra.cover_url) return obj.extra.cover_url
    if (obj.extra && obj.extra.cover) return obj.extra.cover
    // 从 extra.files 中找 cover/thumb 命名的图片
    if (obj.extra && Array.isArray(obj.extra.files)) {
      for (var fi = 0; fi < obj.extra.files.length; fi++) {
        var f = obj.extra.files[fi]
        if (f && f.type === 'image' && f.path) {
          var fname = (f.name || f.path || '').toString().toLowerCase()
          if (fname.indexOf('cover') >= 0 || fname.indexOf('thumb') >= 0) return window.__dm_pathToUrl(f.path)
        }
      }
      // 回退到第一个图片
      for (var fi = 0; fi < obj.extra.files.length; fi++) {
        var f = obj.extra.files[fi]
        if (f && f.type === 'image' && f.path) return window.__dm_pathToUrl(f.path)
      }
    }
    if (obj.extra && obj.extra.thumb_url) return obj.extra.thumb_url
    if (obj.extra && obj.extra.preview_url) return obj.extra.preview_url
    return ''
  }

  // getPreviewUrl 获取预览视频 URL（用于鼠标悬停预览）
  window.__dm_getPreviewUrl = function (obj) {
    if (!obj) return ''
    if (obj.extra && obj.extra.local_preview) return window.__dm_pathToUrl(obj.extra.local_preview)
    if (obj.extra && obj.extra.preview_url) return obj.extra.preview_url
    return ''
  }

  var api = {
    runtime: function () {
      return fetch('/api/runtime').then(function (r) { return r.json() })
    },

    tasks: function () {
      return fetch('/api/tasks').then(function (r) { return r.json() })
    },

    taskDetails: function (id, page, limit, search, sortBy, signal) {
      var url = '/api/tasks/' + encodeURIComponent(id) + '?page=' + page
      if (limit === 'all') { url += '&limit=all' } else { url += '&limit=' + (limit || 50) }
      if (search) { url += '&search=' + encodeURIComponent(search) }
      if (sortBy && sortBy !== 'default') { url += '&sort=' + sortBy }
      var opts = { method: 'GET' }
      if (signal) opts.signal = signal
      return fetch(url, opts).then(function (r) {
        if (!r.ok) throw new Error('Failed to fetch task details')
        return r.json()
      })
    },

    activeDownloads: function () {
      return fetch('/api/downloads').then(function (r) { return r.json() })
    },

    getObject: function (type, id) {
      return this.get('/api/objects/' + type + '/' + id)
    },

    getCollection: function (type, id) {
      return this.get('/api/objects/' + type + '/' + id + '/collection')
    },

    aggregate: function (params) {
      var q = new URLSearchParams()
      q.set('page', params.page || 1)
      if (params.limit === 'all') { q.set('limit', 'all') } else { q.set('limit', params.limit || 50) }
      if (params.search) { q.set('search', params.search) }
      if (params.sort) { q.set('sort', params.sort) }
      if (params.status && params.status !== 'all') { q.set('status', params.status) }
      if (params.types && params.types !== 'all') { q.set('types', params.types) }
      if (params.groupBy) { q.set('group_by', 'content') }
      if (params.tags) { q.set('tags', params.tags) }
      if (params.tagMode) { q.set('tag_mode', params.tagMode) }
      if (params.excludeIds) { q.set('exclude_ids', params.excludeIds) }
      return fetch('/api/aggregate?' + q.toString()).then(function (r) {
        if (!r.ok) throw new Error('Aggregate request failed')
        return r.json()
      })
    },

    groupObjects: function (groupId, taskId, taskType) {
      var params = new URLSearchParams()
      if (taskId) params.set('task_id', taskId)
      if (taskType) params.set('task_type', taskType)
      var query = params.toString()
      return fetch('/api/groups/' + encodeURIComponent(groupId) + '/objects' + (query ? '?' + query : '')).then(function (r) {
        if (!r.ok) throw new Error('Failed to load group')
        return r.json()
      })
    },

    serverConfig: function () {
      return fetch('/api/config/server').then(function (r) { return r.json() })
    },

    logConfig: function () {
      return fetch('/api/config/log').then(function (r) { return r.json() })
    },

    healthz: function () {
      return fetch('/api/healthz').then(function (r) {
        if (!r.ok) throw new Error('Health check failed')
        return r.json()
      })
    },

    metrics: function () {
      return fetch('/api/metrics').then(function (r) {
        if (!r.ok) throw new Error('Metrics fetch failed')
        return r.json()
      })
    },

    failures: function (params) {
      var q = new URLSearchParams()
      if (params && params.limit) q.set('limit', params.limit)
      if (params && params.task_id) q.set('task_id', params.task_id)
      return fetch('/api/metrics/failures?' + q.toString()).then(function (r) {
        if (!r.ok) throw new Error('Failures fetch failed')
        return r.json()
      })
    },

    get: function (url) {
      return fetch(url, { method: 'GET' }).then(function (r) {
        if (!r.ok) throw new Error('GET request failed: ' + url)
        return r.json()
      })
    },

    post: function (url, body) {
      return fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
      })
    },

    put: function (url, body) {
      return fetch(url, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
      })
    },

    patch: function (url, body) {
      return fetch(url, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
      })
    },

    updateObjectTags: function (type, id, tags) {
      return this.post('/api/objects/' + type + '/' + id + '/tags', { tags: tags })
    },

    getTaskTypeDefaults: function () {
      return fetch('/api/config/task-type-defaults').then(function (r) { return r.json() })
    },
    setTaskTypeDefaults: function (data) {
      return fetch('/api/config/task-type-defaults', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data)
      })
    }
  }

  window.AppAPI = api
})()