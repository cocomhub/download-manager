// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * UiTaskList — 纯函数版本的任务列表方法。
 * 从 taskList.js 的 mixin 中提取，消除 Vue.mixin 依赖。
 *
 * 所有方法通过 state 参数与 Vue 应用交互。
 * state 必须包含：tasks, selectedTaskId, selectedTask, selectedTaskIds, selectedObjectUrls,
 *   selectAllScope, pagination, searchQuery, sortBy, taskConfigForm, loading, abortController 等
 */
;(function () {
  'use strict'

  function fetchTasks (state) {
    Log.debug('fetchTasks started')
    AppAPI.tasks().then(function (data) {
      state.tasks = data || []
      if (typeof syncTaskTypes === 'function') {
        syncTaskTypes(state.tasks)
        if (typeof getAvailableTaskTypes === 'function') {
          state.taskTypes = getAvailableTaskTypes()
        }
      }
      AppAPI.activeDownloads().then(function (dl) { state.activeDownloads = dl || [] }).catch(function () {})
      if (state.selectedTaskId) fetchTaskDetails(state, state.selectedTaskId, true)
    }).catch(function (e) { console.error(e) }).finally(function () { state.loading = false })
  }

  function selectTask (state, id) {
    Log.info('selectTask', { id: id })
    state.mobileSidebarOpen = false
    state.selectedTaskId = id
    state.selectedTaskIds = state.selectedTaskIds.filter(function (x) { return x !== id })
    state.selectedObjectUrls = []
    state.selectAllScope = 'page'
    state.searchQuery = ''
    state.pagination.page = 1
    state.viewMode = 'grid'
    fetchTaskDetails(state, id, false)
    // Safety timeout: force-reset loading state after 15s
    setTimeout(function () {
      if (state.isLoadingTask && state.selectedTaskId === id) {
        state.isLoadingTask = false
        console.warn('fetchTaskDetails safety timeout for', id)
      }
    }, 15000)
    // Load task-type-specific UI
    var task = state.tasks.find(function (t) { return t.id === id })
    if (task && task.type) {
      Log.debug('selectTask loading task UI', { type: task.type })
      if (typeof state._loadTaskUI === 'function') {
        state._loadTaskUI(task.type)
      }
    }
  }

  function toggleSelectAll (state) {
    if (state.selectedTaskIds.length === state.tasks.length) {
      state.selectedTaskIds = []
    } else {
      state.selectedTaskIds = state.tasks.map(function (t) { return t.id })
    }
  }

  function toggleSelectAllObjects (state) {
    var urls = (state.filteredObjects || []).map(function (o) { return o.url })
    if (state.selectAllScope === 'all') {
      state.selectAllScope = 'page'
      state.selectedObjectUrls = [].concat(urls)
    } else {
      var allSelected = urls.length > 0 && urls.every(function (u) { return state.selectedObjectUrls.indexOf(u) >= 0 })
      if (allSelected) {
        state.selectedObjectUrls = state.selectedObjectUrls.filter(function (u) { return urls.indexOf(u) < 0 })
      } else {
        var set = {}
        state.selectedObjectUrls.forEach(function (u) { set[u] = true })
        urls.forEach(function (u) { set[u] = true })
        state.selectedObjectUrls = Object.keys(set)
        if (urls.length === state.filteredObjects.length && state.selectedObjectUrls.length > urls.length) {
          state.selectAllScope = 'all'
        }
      }
    }
  }

  function fetchTaskDetails (state, id, background) {
    if (!id) return
    Log.debug('fetchTaskDetails', { id: id, background: !!background, page: state.pagination.page })
    if (!background) {
      state.isLoadingTask = true
      if (state.abortController) state.abortController.abort()
      state.abortController = new AbortController()
    }
    var limit = state.pagination.limit
    var signal = state.abortController ? state.abortController.signal : null
    AppAPI.taskDetails(id, state.pagination.page, limit, state.searchQuery, state.sortBy, signal)
      .then(function (data) {
        state.selectedTask = data
        if (data.concurrency !== undefined) state.taskConfigForm.concurrency = data.concurrency
        if (data.refresh_interval !== undefined) state.taskConfigForm.refresh_interval = data.refresh_interval
        if (data.scrape_enabled !== undefined) state.taskConfigForm.scrape_enabled = data.scrape_enabled
        if (data.download_enabled !== undefined) state.taskConfigForm.download_enabled = data.download_enabled
        if (data.save_sub_dir !== undefined) state.taskConfigForm.save_sub_dir = data.save_sub_dir
        if (data.total !== undefined) {
          state.pagination.total = data.total
          state.pagination.page = data.page
          state.pagination.limit = (data.limit === -1 || data.limit === 0) ? 'all' : data.limit
        }
      }).catch(function (e) {
        if (e.name === 'AbortError') return
        console.error('fetchTaskDetails error:', e)
      }).finally(function () {
        if (!background) { state.isLoadingTask = false; state.abortController = null }
      })
  }

  function cancelCurrentTask (state) {
    if (state.isWriteDisabled) { UiHelpers.showToast('UI-Only 模式下已禁用', 'error'); return }
    if (!state.selectedTaskId) return
    AppAPI.post('/api/tasks/' + encodeURIComponent(state.selectedTaskId) + '/cancel', {}).then(function (res) {
      if (!res.ok) throw new Error('取消失败')
      UiHelpers.showToast('任务已取消', 'success')
      fetchTasks(state)
      fetchTaskDetails(state, state.selectedTaskId, true)
    }).catch(function (e) { UiHelpers.showToast('取消失败: ' + e.message, 'error') })
  }

  function cancelSelected (state) {
    if (state.isWriteDisabled) { UiHelpers.showToast('UI-Only 模式下已禁用', 'error'); return }
    if (state.selectedTaskIds.length === 0) return
    AppAPI.post('/api/tasks/cancel_batch', { ids: state.selectedTaskIds }).then(function (res) {
      if (!res.ok) throw new Error('批量取消失败')
      return res.json()
    }).then(function (result) {
      var failed = Object.entries(result).filter(function (kv) { return kv[1] !== 'ok' })
      if (failed.length === 0) UiHelpers.showToast('已取消选中任务', 'success')
      else UiHelpers.showToast('部分取消失败', 'error')
      state.selectedTaskIds = []
      fetchTasks(state)
      if (state.selectedTaskId) fetchTaskDetails(state, state.selectedTaskId, true)
    }).catch(function (e) { UiHelpers.showToast('批量取消失败: ' + e.message, 'error') })
  }

  function retryAllFailed (state) {
    if (state.isWriteDisabled) { UiHelpers.showToast('UI-Only 模式下已禁用', 'error'); return }
    if (!state.selectedTaskId) return
    AppAPI.post('/api/tasks/' + encodeURIComponent(state.selectedTaskId) + '/retry', {}).then(function (res) {
      if (!res.ok) throw new Error('重试失败')
      UiHelpers.showToast('已重试所有失败对象', 'success')
      fetchTaskDetails(state, state.selectedTaskId, true)
    }).catch(function (e) { UiHelpers.showToast('重试失败: ' + e.message, 'error') })
  }

  function changePage (state, p) {
    if (p < 1) return
    if (state.pagination.limit !== 'all' && p > Math.ceil(state.pagination.total / state.pagination.limit)) return
    state.pagination.page = p
    state.selectedObjectUrls = []
    state.selectAllScope = 'page'
    fetchTaskDetails(state, state.selectedTaskId)
  }

  function changeLimit (state) {
    state.pagination.page = 1
    state.selectedObjectUrls = []
    state.selectAllScope = 'page'
    fetchTaskDetails(state, state.selectedTaskId)
  }

  function retrySelectedObjects (state) {
    if (state.isWriteDisabled) { UiHelpers.showToast('UI-Only 模式下已禁用', 'error'); return }
    if (state.selectedObjectUrls.length === 0) return

    var isAllMode = state.selectAllScope === 'all'

    if (isAllMode) {
      if (!state.selectedTaskId) return
      AppAPI.post('/api/tasks/' + encodeURIComponent(state.selectedTaskId) + '/retry', {})
        .then(function (res) {
          if (!res.ok) throw new Error('批量重试失败')
          UiHelpers.showToast('已重试所有失败对象', 'success')
          state.selectedObjectUrls = []
          state.selectAllScope = 'page'
          fetchTaskDetails(state, state.selectedTaskId, true)
        }).catch(function (e) { UiHelpers.showToast('批量重试失败: ' + e.message, 'error') })
      return
    }

    var objs = (state.selectedTask && state.selectedTask.objects) || []
    var failedUrls = []
    objs.forEach(function (o) {
      if (state.selectedObjectUrls.indexOf(o.url) >= 0 && o.status === 'failed') {
        failedUrls.push(o.url)
      }
    })

    if (failedUrls.length === 0) {
      UiHelpers.showToast('选中的对象中没有可重试的失败项', 'info')
      return
    }

    var completed = 0
    var totalFailed = 0
    failedUrls.forEach(function (url) {
      AppAPI.post('/api/tasks/' + encodeURIComponent(state.selectedTaskId) + '/retry', { url: url })
        .then(function (res) {
          if (res.ok) {
            completed++
            var obj = (state.selectedTask && state.selectedTask.objects || []).find(function (o) { return o.url === url })
            if (obj) { obj.status = 'pending'; obj.progress = 0 }
          } else {
            totalFailed++
          }
        }).catch(function () { totalFailed++ })
        .finally(function () {
          if (completed + totalFailed === failedUrls.length) {
            if (totalFailed > 0) {
              UiHelpers.showToast('已重试 ' + completed + ' 个，失败 ' + totalFailed + ' 个', 'error')
            } else {
              UiHelpers.showToast('已重试 ' + completed + ' 个失败对象', 'success')
            }
            state.selectedObjectUrls = []
            fetchTaskDetails(state, state.selectedTaskId, true)
          }
        })
    })
  }

  function cancelSelectAllObjects (state) {
    if (state.isWriteDisabled) { UiHelpers.showToast('UI-Only 模式下已禁用', 'error'); return }
    if (state.selectedObjectUrls.length === 0) return

    if (state.selectAllScope === 'all') {
      if (!state.selectedTaskId) return
      AppAPI.post('/api/tasks/' + encodeURIComponent(state.selectedTaskId) + '/cancel', {})
        .then(function (res) {
          if (!res.ok) throw new Error('取消失败')
          UiHelpers.showToast('任务已取消', 'success')
          state.selectedObjectUrls = []
          state.selectAllScope = 'page'
          fetchTasks(state)
          fetchTaskDetails(state, state.selectedTaskId, true)
        }).catch(function (e) { UiHelpers.showToast('取消失败: ' + e.message, 'error') })
      return
    }

    AppAPI.post('/api/tasks/' + encodeURIComponent(state.selectedTaskId) + '/object/cancel_batch', { urls: state.selectedObjectUrls })
      .then(function (res) { if (!res.ok) throw new Error('批量取消失败'); return res.json() })
      .then(function (result) {
        var okList = Object.entries(result).filter(function (kv) { return kv[1] === 'ok' }).map(function (kv) { return kv[0] })
        if (state.selectedTask && state.selectedTask.objects && okList.length > 0) {
          state.selectedTask.objects.forEach(function (o) {
            if (okList.indexOf(o.url) >= 0) { o.status = 'cancelled'; o.progress = 0 }
          })
        }
        var failed = Object.entries(result).filter(function (kv) { return kv[1] !== 'ok' })
        if (failed.length === 0) UiHelpers.showToast('已取消选中对象', 'success')
        else UiHelpers.showToast('部分对象取消失败', 'error')
        state.selectedObjectUrls = []
      }).catch(function (e) { UiHelpers.showToast('批量取消失败: ' + e.message, 'error') })
  }

  function undoCancelSelectAllObjects (state) {
    if (state.isWriteDisabled) { UiHelpers.showToast('UI-Only 模式下已禁用', 'error'); return }
    if (state.selectedObjectUrls.length === 0) return

    if (state.selectAllScope === 'all') {
      UiHelpers.showToast('跨页全选模式不支持批量撤销取消，请切换为单页模式', 'info')
      return
    }

    AppAPI.post('/api/tasks/' + encodeURIComponent(state.selectedTaskId) + '/object/undo_cancel_batch', { urls: state.selectedObjectUrls })
      .then(function (res) { if (!res.ok) throw new Error('批量撤销失败'); return res.json() })
      .then(function (result) {
        var okList = Object.entries(result).filter(function (kv) { return kv[1] === 'ok' }).map(function (kv) { return kv[0] })
        if (state.selectedTask && state.selectedTask.objects && okList.length > 0) {
          state.selectedTask.objects.forEach(function (o) {
            if (okList.indexOf(o.url) >= 0) { o.status = 'pending'; o.progress = 0 }
          })
        }
        var failed = Object.entries(result).filter(function (kv) { return kv[1] !== 'ok' })
        if (failed.length === 0) UiHelpers.showToast('已撤销选中对象', 'success')
        else UiHelpers.showToast('部分对象撤销失败', 'error')
        state.selectedObjectUrls = []
      }).catch(function (e) { UiHelpers.showToast('批量撤销失败: ' + e.message, 'error') })
  }

  function cancelObject (state, obj) {
    if (state.isWriteDisabled) { UiHelpers.showToast('UI-Only 模式下已禁用', 'error'); return }
    if (!state.selectedTaskId || !obj || !obj.url) return
    AppAPI.post('/api/tasks/' + encodeURIComponent(state.selectedTaskId) + '/object/cancel', { url: obj.url })
      .then(function (res) {
        if (!res.ok) throw new Error('取消失败')
        obj.status = 'cancelled'
        UiHelpers.showToast('已取消该对象', 'success')
      }).catch(function (e) { UiHelpers.showToast('取消失败: ' + e.message, 'error') })
  }

  function undoCancelObject (state, obj) {
    if (state.isWriteDisabled) { UiHelpers.showToast('UI-Only 模式下已禁用', 'error'); return }
    if (!state.selectedTaskId || !obj || !obj.url) return
    AppAPI.post('/api/tasks/' + encodeURIComponent(state.selectedTaskId) + '/object/undo_cancel', { url: obj.url })
      .then(function (res) {
        if (!res.ok) throw new Error('撤销失败')
        obj.status = 'pending'
        UiHelpers.showToast('已撤销取消', 'success')
      }).catch(function (e) { UiHelpers.showToast('撤销失败: ' + e.message, 'error') })
  }

  function toggleTaskConfigPanel (state) {
    state.showTaskConfigPanel = !state.showTaskConfigPanel
  }

  function saveTaskConfig (state) {
    if (state.isWriteDisabled) { UiHelpers.showToast('UI-Only 模式下已禁用', 'error'); return }
    if (!state.selectedTaskId) return
    var payload = {
      concurrency: state.taskConfigForm.concurrency,
      refresh_interval: state.taskConfigForm.refresh_interval
    }
    if (state.taskConfigForm.scrape_enabled !== undefined) {
      payload.scrape_enabled = state.taskConfigForm.scrape_enabled
    }
    if (state.taskConfigForm.download_enabled !== undefined) {
      payload.download_enabled = state.taskConfigForm.download_enabled
    }
    if (state.taskConfigForm.save_sub_dir !== undefined && state.taskConfigForm.save_sub_dir !== '') {
      payload.save_sub_dir = state.taskConfigForm.save_sub_dir
    }
    AppAPI.patch('/api/tasks/' + encodeURIComponent(state.selectedTaskId) + '/runtime', payload).then(function (res) {
      if (!res.ok) throw new Error('保存失败')
      UiHelpers.showToast('任务配置已保存', 'success')
      fetchTaskDetails(state, state.selectedTaskId, true)
    }).catch(function (e) { UiHelpers.showToast('保存失败: ' + e.message, 'error') })
  }

  window.UiTaskList = {
    fetchTasks: fetchTasks,
    selectTask: selectTask,
    toggleSelectAll: toggleSelectAll,
    toggleSelectAllObjects: toggleSelectAllObjects,
    fetchTaskDetails: fetchTaskDetails,
    cancelCurrentTask: cancelCurrentTask,
    cancelSelected: cancelSelected,
    retryAllFailed: retryAllFailed,
    changePage: changePage,
    changeLimit: changeLimit,
    retrySelectedObjects: retrySelectedObjects,
    cancelSelectAllObjects: cancelSelectAllObjects,
    undoCancelSelectAllObjects: undoCancelSelectAllObjects,
    cancelObject: cancelObject,
    undoCancelObject: undoCancelObject,
    toggleTaskConfigPanel: toggleTaskConfigPanel,
    saveTaskConfig: saveTaskConfig,
  }
})()