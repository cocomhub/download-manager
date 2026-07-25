// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * Loader — 动态加载 task 类型 UI 组件
 * 通过 /api/ui/{type}/config 获取配置，加载 JS/CSS 资产
 * 依赖现有后端资产服务机制
 */
;(function () {
  'use strict'

  var loadedTypes = {}

  function loadTaskUI(taskType, callback) {
    if (!taskType || taskType === 'all') {
      Log.trace('loadTaskUI skipped', { taskType: taskType })
      return
    }
    if (loadedTypes[taskType]) {
      Log.trace('loadTaskUI already loaded', { type: taskType })
      if (callback) callback()
      return
    }

    Log.debug('loadTaskUI fetching config', { type: taskType })
    fetch('/api/ui/' + encodeURIComponent(taskType) + '/config')
      .then(function (r) {
        if (!r.ok) {
          // 404 is normal for task types with no registered UI assets
          Log.debug('loadTaskUI no assets registered', { type: taskType, status: r.status })
          loadedTypes[taskType] = true
          if (callback) callback()
          return null
        }
        Log.debug('loadTaskUI config received', { type: taskType })
        return r.json()
      })
      .then(function (cfg) {
        if (!cfg) return

        var pending = 0
        function onLoad() {
          pending--
          if (pending <= 0) {
            Log.debug('loadTaskUI all assets loaded', { type: taskType })
            loadedTypes[taskType] = true
            if (callback) callback()
          }
        }

        // Load CSS
        if (cfg.css && cfg.css.length > 0) {
          Log.debug('loadTaskUI loading CSS', { type: taskType, paths: cfg.css })
          cfg.css.forEach(function (p) {
            var href = '/api/ui/' + encodeURIComponent(taskType) + '/assets/' + encodeURIComponent(p)
            if (!document.querySelector('link[href="' + href + '"]')) {
              pending++
              var link = document.createElement('link')
              link.rel = 'stylesheet'
              link.href = href
              link.onload = onLoad
              link.onerror = function () { Log.warn('loadTaskUI CSS load error', { type: taskType, path: p }); onLoad() }
              document.head.appendChild(link)
            }
          })
        }

        // Load JS
        if (cfg.js && cfg.js.length > 0) {
          Log.debug('loadTaskUI loading JS', { type: taskType, paths: cfg.js })
          cfg.js.forEach(function (p) {
            var src = '/api/ui/' + encodeURIComponent(taskType) + '/assets/' + encodeURIComponent(p)
            if (!document.querySelector('script[src="' + src + '"]')) {
              pending++
              var script = document.createElement('script')
              script.src = src
              script.onload = function () { Log.debug('loadTaskUI JS loaded', { type: taskType, path: p }); onLoad() }
              script.onerror = function () { Log.warn('loadTaskUI JS load error', { type: taskType, path: p }); onLoad() }
              document.body.appendChild(script)
            }
          })
        }

        // If no assets to load, mark as loaded
        if (pending === 0) {
          Log.debug('loadTaskUI no assets to load', { type: taskType })
          loadedTypes[taskType] = true
          if (callback) callback()
        }
      })
      .catch(function (e) {
        Log.warn('loadTaskUI fetch failed', { type: taskType, error: e.message })
        loadedTypes[taskType] = true
        if (callback) callback()
      })
  }

  window.TaskUI = window.TaskUI || {}
  window.TaskUI.loadTaskUI = loadTaskUI
  window.TaskUI._loadedTypes = loadedTypes
})()