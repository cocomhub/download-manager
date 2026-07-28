// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * UiHelpers — 纯函数版本的 UI 辅助方法。
 * 从 helpers.js 的 mixin 中提取，消除 Vue.mixin 依赖。
 * 所有方法通过 state 参数与 Vue 应用交互。
 *
 * state 对象包含 Vue data 中的响应式属性：
 *   { selectedType, newTask, showAddTaskModal, showConfigModal, ... }
 */
;(function () {
  'use strict'

  // ---- URL / Type routing ----

  function initTypeFromURL (state) {
    try {
      var cand = (typeof window.__dm_readTypeFromURL === 'function') ? window.__dm_readTypeFromURL() : null
      var ids = (typeof getAvailableTaskTypes === 'function') ? (getAvailableTaskTypes() || []).map(function (t) { return t.id }) : []
      state.selectedType = (cand && ids.indexOf(cand) >= 0) ? cand : 'all'
    } catch (e) { state.selectedType = 'all' }
  }

  function initRuntime (state) {
    AppAPI.runtime().then(function (d) {
      if (d && typeof d === 'object') {
        state.runtime = d
        if (d.download_root) window.__dm_downloadRoot = d.download_root.replace(/\\/g, '/')
        if (d.log_level && typeof Log !== 'undefined' && Log.setLevel) {
          Log.setLevel(d.log_level)
        }
      }
    }).catch(function () {})
  }

  // ---- Object display helpers ----

  function getTitle (obj) {
    return (obj && obj.metadata && obj.metadata.title) || ''
  }

  function getDate (obj) {
    return (obj && obj.metadata && obj.metadata.date) || ''
  }

  function getDuration (obj) {
    return (obj && obj.metadata && obj.metadata.duration) || ''
  }

  function getTags (obj) {
    if (obj && obj.extra && Array.isArray(obj.extra.tags)) return obj.extra.tags
    if (obj && obj.extra && typeof obj.extra.tags === 'string') return [obj.extra.tags]
    return []
  }

  function getObjId (obj) {
    return (obj && obj.id) || (obj && obj.ID) || 0
  }

  function getTaskTypeForObj (obj) {
    return (obj && obj.metadata && obj.metadata.task_type) || (obj && obj.extra && obj.extra.task_type) || ''
  }

  function isTouchDevice () {
    try { return 'ontouchstart' in window } catch (e) { return false }
  }

  function pathToUrl (path, runtime) {
    if (!path) return ''
    var normalized = path.replace(/\\/g, '/')
    var downloadRoot = runtime && runtime.download_root
    if (downloadRoot && normalized.indexOf(downloadRoot.replace(/\\/g, '/')) === 0) {
      normalized = normalized.slice(downloadRoot.replace(/\\/g, '/').length)
    }
    normalized = normalized.replace(/^\//, '')
    return '/files/' + normalized.split('/').map(function (seg) {
      return encodeURIComponent(seg)
    }).join('/')
  }

  function getFileUrl (obj, runtime) {
    if (obj && obj.save_path) return pathToUrl(obj.save_path, runtime)
    if (obj && obj.extra && Array.isArray(obj.extra.files)) {
      for (var fi = 0; fi < obj.extra.files.length; fi++) {
        var f = obj.extra.files[fi]
        if (f && f.path) return pathToUrl(f.path, runtime)
      }
    }
    return ''
  }

  function getTaskDisplayName (task) {
    if (!task) return ''
    if (task.display_name) return task.display_name
    if (task.name && task.name !== task.id) return task.name
    return task.id
  }

  function getTaskTypeBadge (task) {
    if (!task || !task.type) return ''
    var handler = TaskUI.get(task.type)
    if (handler && handler.label) return handler.label
    var known = {
      'tktube': 'TKTube',
      'hanime': 'Hanime',
      'vikacg': 'VikACG',
      'url_list': 'URL',
      'mxs': '漫小肆'
    }
    return known[task.type] || (task.type.length > 12 ? task.type.slice(0, 12) + '…' : task.type)
  }

  // ---- Group helpers ----

  function getScopedTaskInfo (obj) {
    if (!obj) return { taskId: '', taskType: '' }
    return { taskId: obj.task_id || '', taskType: (obj.metadata && obj.metadata.task_type) || '' }
  }

  function getObjectVariantPriority (obj) {
    if (!obj || !obj.extra) return 0
    if (obj.extra.variant_priority !== undefined) return obj.extra.variant_priority
    if (obj.extra.priority !== undefined) return obj.extra.priority
    if (obj.metadata && obj.metadata.resolution) {
      var r = obj.metadata.resolution
      if (/1080/.test(r)) return 30
      if (/720/.test(r)) return 20
      if (/480/.test(r)) return 10
    }
    return 0
  }

  function isGroupRepresentative (obj) {
    return !!(obj && obj.extra && (obj.extra.group_rep || obj.extra.is_representative))
  }

  function isGroupCancelTarget (obj) {
    return obj && obj.status === 'pending' && !isGroupRepresentative(obj) &&
      (obj.extra && obj.extra.group_size)
  }

  function getObjectVariantLabel (obj) {
    if (obj && obj.metadata && obj.metadata.resolution) return obj.metadata.resolution
    if (obj && obj.metadata && obj.metadata.variant_label) return obj.metadata.variant_label
    return 'standard'
  }

  function metadataContentGroup (obj) {
    return (obj && obj.metadata && obj.metadata.content_group) || ''
  }

  // ---- Toast ----

  function showToast (message, type) {
    type = type || 'info'
    var toast = document.createElement('div')
    toast.className = 'fixed bottom-4 left-4 px-4 py-2 rounded shadow-lg text-white text-sm z-50 transition-opacity duration-300 ' + (type === 'error' ? 'bg-red-500' : type === 'info' ? 'bg-blue-500' : 'bg-green-500')
    toast.textContent = message
    document.body.appendChild(toast)
    setTimeout(function () {
      toast.style.opacity = '0'
      setTimeout(function () { toast.remove() }, 300)
    }, 3000)
  }

  // ---- UI defaults ----

  function initUiDefaults (state) {
    AppAPI.serverConfig().then(function (svr) {
      var svrUi = (svr && svr.ui_defaults) || {}
      var localUi = {}
      try { localUi = JSON.parse(localStorage.getItem('dm_ui_defaults') || '{}') } catch (e) {}
      var merged = Object.assign({}, svrUi, localUi)
      state.uiDefaults = merged
      if (merged.default_save_dir) state.newTask.save_dir = merged.default_save_dir
      if (typeof merged.diff_side_by_side === 'boolean') state.diffOptions.side_by_side = merged.diff_side_by_side
      if (typeof merged.diff_ignore_ws === 'boolean') state.diffOptions.ignore_ws = merged.diff_ignore_ws
      if (typeof merged.diff_ignore_comment === 'boolean') state.diffOptions.ignore_comments = merged.diff_ignore_comment
    }).catch(function () {})
  }

  // ---- Create task modal ----

  function openAddTask (state, $event) {
    if ($event) $event.preventDefault()
    Log.debug('openAddTask', { currentType: state.newTask.type })
    AppAPI.getTaskTypeDefaults().then(function (defaults) {
      var typeDef = defaults && defaults[state.newTask.type]
      if (typeDef) {
        if (typeDef.save_root_dir && !state.newTask.save_dir) {
          state.newTask.save_dir = typeDef.save_root_dir
        }
        if (state.newTask.scrape_enabled === undefined) {
          state.newTask.scrape_enabled = typeDef.scrape_enabled !== false
        }
        if (state.newTask.download_enabled === undefined) {
          state.newTask.download_enabled = typeDef.download_enabled !== false
        }
      }
      state.showAddTaskModal = true
    }).catch(function () {
      state.showAddTaskModal = true
    })
  }

  function saveNewTask (state) {
    var payload = {
      id: state.newTask.id,
      type: state.newTask.type,
      save_dir: state.newTask.save_dir,
      storage: { type: state.newTask.storage_type },
      extra: {}
    }
    Log.info('saveNewTask', { type: payload.type, id: payload.id, storage: payload.storage.type })
    if (state.newTask.save_sub_dir) {
      payload.save_sub_dir = state.newTask.save_sub_dir
    }
    if (state.newTask.scrape_enabled !== undefined) {
      payload.scrape_enabled = state.newTask.scrape_enabled
    }
    if (state.newTask.download_enabled !== undefined) {
      payload.download_enabled = state.newTask.download_enabled
    }
    if (payload.save_dir && payload.save_sub_dir) {
      Log.warn('saveNewTask: both save_dir and save_sub_dir configured, save_dir takes precedence')
    }
    if (state.newTask.storage_type === 'file' && state.newTask.storage_config.path) {
      payload.storage.path = state.newTask.storage_config.path
    }
    if (state.newTask.storage_type === 'mongo') {
      if (state.newTask.storage_config.source) payload.storage.source = state.newTask.storage_config.source
      if (state.newTask.storage_config.database) payload.storage.database = state.newTask.storage_config.database
      if (state.newTask.storage_config.collection) payload.storage.collection = state.newTask.storage_config.collection
    }
    var handler = TaskUI.get(state.newTask.type)
    if (handler && handler.collectExtra) {
      var extra = handler.collectExtra(state.newTask)
      if (extra) {
        payload.extra = Object.assign(payload.extra, extra)
      }
    }
    if (!payload.id || !payload.type) {
      showToast('请填写任务ID和类型', 'error')
      return
    }
    AppAPI.post('/api/tasks', payload).then(function (res) {
      if (!res.ok) throw new Error('创建失败')
      showToast('任务创建成功', 'success')
      state.showAddTaskModal = false
      state.newTask = { id: '', type: 'url_list', save_dir: '', save_sub_dir: '', scrape_enabled: true, download_enabled: true, storage_type: 'file', storage_config: {}, urls_text: '', keyword: '', subtype: 'tag', max_concurrent: 2, refresh_interval: 300 }
      // Fetch tasks via callback
      if (typeof state._fetchTasks === 'function') state._fetchTasks()
    }).catch(function (e) { showToast('创建失败: ' + e.message, 'error') })
  }

  // ---- Config panel ----

  function openConfig (state) {
    Log.info('openConfig')
    state.showConfigModal = true
    AppAPI.serverConfig().then(function (data) {
      state.configForm = data || {}
      Log.debug('openConfig loaded', { log_level: data && data.log_level })
    }).catch(function () {})
  }

  function saveConfig (state) {
    AppAPI.post('/api/config/server', state.configForm).then(function (res) {
      if (!res.ok) throw new Error('保存失败')
      showToast('配置已保存', 'success')
      state.showConfigModal = false
      initUiDefaults(state)
      if (state.configForm.log_level !== undefined && typeof Log !== 'undefined' && Log.setLevel) {
        Log.setLevel(state.configForm.log_level)
      }
    }).catch(function (e) { showToast('保存失败: ' + e.message, 'error') })
  }

  function openConfigHistory (state) {
    state.showConfigHistoryModal = true
  }

  // ---- TaskUI helper predicates ----

  function hasOnClick (obj) {
    if (!obj) return false
    var type = obj.metadata && obj.metadata.task_type
    return type && TaskUI.hasOnClick(type)
  }

  // ---- Aggregate view ----

  function openAggregateView (state) {
    Log.info('openAggregateView', { selectedType: state.selectedType, viewMode: state.viewMode })
    state.viewMode = 'aggregate'
    fetchAggregateByType(state, state.selectedType || 'all')
  }

  function fetchAggregateByType (state, type) {
    if (state.aggLoading) return
    state.aggLoading = true
    AppAPI.aggregate({
      types: type || 'all',
      sort: state.aggSortBy || '',
      groupBy: state.aggGroupBy || false,
      search: state.aggSearchQuery || '',
      page: state.aggPagination.page,
      limit: state.aggPagination.limit
    }).then(function (data) {
      state.aggObjects = (data && data.objects) || (Array.isArray(data) ? data : [])
      state.aggPagination.total = (data && data.total) || state.aggObjects.length
      state.showAggView = true
    }).catch(function () {
      showToast('加载聚合视图失败', 'error')
    }).finally(function () {
      state.aggLoading = false
    })
  }

  function cancelAggObject (state, obj) {
    if (!obj || !obj.task_id) return
    AppAPI.post('/api/tasks/' + encodeURIComponent(obj.task_id) + '/object/cancel', { url: obj.url }).then(function (res) {
      if (res && !res.ok) throw new Error('取消失败')
      obj.status = 'cancelled'
      showToast('已取消: ' + (obj.metadata && obj.metadata.title || obj.url), 'info')
    }).catch(function (e) { showToast('取消失败: ' + e.message, 'error') })
  }

  function changeAggPage (state, page) {
    state.aggPagination.page = page
    fetchAggregateByType(state, state.selectedType || 'all')
  }

  function changeAggLimit (state) {
    state.aggPagination.page = 1
    fetchAggregateByType(state, state.selectedType || 'all')
  }

  // ---- Clipboard ----

  function copyText (text) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).then(function () {
        showToast('已复制到剪贴板', 'success')
      }).catch(function () {
        showToast('复制失败', 'error')
      })
    } else {
      var ta = document.createElement('textarea')
      ta.value = text
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.select()
      try { document.execCommand('copy'); showToast('已复制到剪贴板', 'success') }
      catch (e) { showToast('复制失败', 'error') }
      document.body.removeChild(ta)
    }
  }

  // ---- Export ----

  window.UiHelpers = {
    // State init
    initTypeFromURL: initTypeFromURL,
    initRuntime: initRuntime,
    initUiDefaults: initUiDefaults,

    // Display helpers (pure, no state)
    getTitle: getTitle,
    getDate: getDate,
    getDuration: getDuration,
    getTags: getTags,
    getObjId: getObjId,
    getTaskTypeForObj: getTaskTypeForObj,
    isTouchDevice: isTouchDevice,
    pathToUrl: pathToUrl,
    getFileUrl: getFileUrl,
    getTaskDisplayName: getTaskDisplayName,
    getTaskTypeBadge: getTaskTypeBadge,

    // Group helpers
    getScopedTaskInfo: getScopedTaskInfo,
    getObjectVariantPriority: getObjectVariantPriority,
    isGroupRepresentative: isGroupRepresentative,
    isGroupCancelTarget: isGroupCancelTarget,
    getObjectVariantLabel: getObjectVariantLabel,
    metadataContentGroup: metadataContentGroup,

    // Actions (accept state)
    openAddTask: openAddTask,
    saveNewTask: saveNewTask,
    openConfig: openConfig,
    saveConfig: saveConfig,
    openConfigHistory: openConfigHistory,
    hasOnClick: hasOnClick,
    openAggregateView: openAggregateView,
    fetchAggregateByType: fetchAggregateByType,
    cancelAggObject: cancelAggObject,
    changeAggPage: changeAggPage,
    changeAggLimit: changeAggLimit,

    // Utilities
    showToast: showToast,
    copyText: copyText,
  }
})()