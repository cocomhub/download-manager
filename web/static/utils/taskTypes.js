/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

;(function (global) {
  var BUILTIN_TYPES = [
    { id: 'all', label: '全部' },
    { id: 'tktube', label: 'TKTube' },
    { id: 'vikacg', label: 'VikACG' },
    { id: 'hanime', label: 'Hanime' }
  ]

  var typeCache = null

  function getAvailableTaskTypes() {
    if (typeCache) return typeCache.slice()
    return BUILTIN_TYPES.slice()
  }

  function syncTaskTypes(tasks) {
    if (!tasks || tasks.length === 0) {
      typeCache = null
      return
    }
    var seen = {}
    BUILTIN_TYPES.forEach(function (t) { seen[t.id] = t })

    tasks.forEach(function (task) {
      if (!task || !task.type) return
      if (!seen[task.type]) {
        // 优先使用 TaskUI 注册的标签，回退到自动生成
        var handler = typeof TaskUI !== 'undefined' && TaskUI.get(task.type)
        var label = handler && handler.label ? handler.label : (
          task.type.replace(/_/g, ' ').replace(/\b\w/g, function (c) { return c.toUpperCase() })
        )
        seen[task.type] = { id: task.type, label: label }
      }
    })

    typeCache = Object.keys(seen).map(function (id) { return seen[id] })
  }

  global.getAvailableTaskTypes = getAvailableTaskTypes
  global.syncTaskTypes = syncTaskTypes
  global.TASK_TYPES = BUILTIN_TYPES
})(window)
