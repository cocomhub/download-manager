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
    if (!taskType || taskType === 'all') return
    if (loadedTypes[taskType]) {
      if (callback) callback()
      return
    }

    fetch('/api/ui/' + encodeURIComponent(taskType) + '/config')
      .then(function (r) { return r.json() })
      .then(function (cfg) {
        var pending = 0
        function onLoad() {
          pending--
          if (pending <= 0) {
            loadedTypes[taskType] = true
            if (callback) callback()
          }
        }

        // Load CSS
        if (cfg.css && cfg.css.length > 0) {
          cfg.css.forEach(function (p) {
            var href = '/api/ui/' + encodeURIComponent(taskType) + '/assets/' + encodeURIComponent(p)
            if (!document.querySelector('link[href="' + href + '"]')) {
              pending++
              var link = document.createElement('link')
              link.rel = 'stylesheet'
              link.href = href
              link.onload = onLoad
              link.onerror = onLoad
              document.head.appendChild(link)
            }
          })
        }

        // Load JS
        if (cfg.js && cfg.js.length > 0) {
          cfg.js.forEach(function (p) {
            var src = '/api/ui/' + encodeURIComponent(taskType) + '/assets/' + encodeURIComponent(p)
            if (!document.querySelector('script[src="' + src + '"]')) {
              pending++
              var script = document.createElement('script')
              script.src = src
              script.onload = onLoad
              script.onerror = onLoad
              document.body.appendChild(script)
            }
          })
        }

        // If no assets to load, mark as loaded
        if (pending === 0) {
          loadedTypes[taskType] = true
          if (callback) callback()
        }
      })
      .catch(function (e) {
        console.warn('loadTaskUI failed for', taskType, e)
        if (callback) callback()
      })
  }

  window.TaskUI = window.TaskUI || {}
  window.TaskUI.loadTaskUI = loadTaskUI
  window.TaskUI._loadedTypes = loadedTypes
})()