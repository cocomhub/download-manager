// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * API helpers — pure fetch wrappers, no Vue dependency.
 * Exposed as window.AppAPI.
 */
;(function () {
  'use strict'

  var api = {
    runtime: function () {
      return fetch('/api/runtime').then(function (r) { return r.json() })
    },

    tasks: function () {
      return fetch('/api/tasks').then(function (r) { return r.json() })
    },

    taskDetails: function (id, page, limit, search, sortBy) {
      var url = '/api/tasks/' + encodeURIComponent(id) + '?page=' + page
      if (limit === 'all') { url += '&limit=all' } else { url += '&limit=' + (limit || 50) }
      if (search) { url += '&search=' + encodeURIComponent(search) }
      if (sortBy && sortBy !== 'default') { url += '&sort=' + sortBy }
      return fetch(url).then(function (r) {
        if (!r.ok) throw new Error('Failed to fetch task details')
        return r.json()
      })
    },

    activeDownloads: function () {
      return fetch('/api/downloads').then(function (r) { return r.json() })
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
    }
  }

  window.AppAPI = api
})()