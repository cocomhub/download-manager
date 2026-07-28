// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * Vue app initialization — wires together all modules.
 * Must be loaded AFTER all other app/*.js files and Vue CDN.
 * Depends on: AppAPI, UiHelpers, UiTaskList, UiVideoPlayer, UiDashboard (global)
 */
;(function () {
  'use strict'

  var app = Vue.createApp({
    data: function () {
      return {
        runtime: { mode: 'full', features: { download: true, scheduler: true } },
        tasks: [],
        taskTypes: (typeof getAvailableTaskTypes === 'function' ? getAvailableTaskTypes() : [{ id: 'all', label: '全部' }]),
        selectedType: 'all',
        activeDownloads: [],
        selectedTaskId: null,
        selectedTask: null,
        selectedTaskIds: [],
        selectedObjectUrls: [],
        selectAllScope: 'page',
        viewMode: 'grid',
        searchQuery: '',
        sortBy: 'default',
        pagination: { page: 1, limit: 50, total: 0 },
        timer: null,
        searchTimer: null,
        draggedItem: null,
        eventSource: null,
        abortController: null,

        // Config & Modals
        fullConfig: null,
        showConfigModal: false,
        showAddTaskModal: false,
        showHistoryModal: false,
        showTaskConfigPanel: false,
        uiDefaults: null,
        showEditTaskModal: false,
        editTask: null,
        configForm: {
          proxies: '', scan_interval: 10, global_concurrent: 5,
          log_level: 'info', log_filename: '', log_max_size: 100,
          log_max_backups: 3, log_max_age: 7, log_compress: false, log_console: true,
          domain_limits_text: '', status_style: 'pill'
        },
        newTask: {
          id: '', type: 'tktube', save_dir: './downloads', save_sub_dir: '', scrape_enabled: true, download_enabled: true,
          urls_text: '', keyword: '',
          subtype: 'tag', max_concurrent: 1, refresh_interval: 3600,
          storage_type: 'file', storage_config: { path: '', source: '', database: '', collection: '' }
        },
        taskConfigForm: { concurrency: 1, refresh_interval: 3600, scrape_enabled: true, download_enabled: true, save_sub_dir: '' },
        statusFilter: 'all',
        configHistory: [],
        diffForm: { left: 'current', right: 'current' },
        configDiff: null,
        lineDiff: [],
        collapsedLineDiff: [],
        diffOptions: { ignore_ws: false, ignore_comments: false, side_by_side: false },
        pathFilter: '',
        showRollbackConfirm: false,
        rollbackTarget: null,
        rollbackDiff: null,
        rollbackLineDiff: [],
        tagForm: { tag: '', message: '' },
        selectedBackupTags: [],
        selectedBackupNotes: [],
        noteForm: { message: '', author: '', messageText: '' },
        loading: false,
        isLoadingTask: false,
        aggObjects: [],
        aggSearchQuery: '',
        aggStatusFilter: 'all',
        aggLoading: false,
        aggGroupBy: false,
        aggViewMode: 'grid',
        uiMode: 'manage',
        aggPagination: { page: 1, limit: 50, total: 0 },
        aggSortBy: 'date_desc',
        aggConcurrency: 2,
        aggDelayMs: 200,
        lastAggFetchTs: 0,
        aggMinIntervalMs: 3000,
        showGroupModal: false,
        groupModal: { title: '', list: [], repObj: null, taskId: '', taskType: '' },

        // Hover
        hoverObj: null,
        hoverTimer: null,
        enablePreview: true,
        previewTimer: null,

        // Dashboard
        dashboardHealth: null,
        dashboardMetrics: null,
        dashboardFailures: null,
        dashboardFailuresLimit: 20,
        dashboardFailuresTaskId: '',
        dashboardHealthzTimer: null,
        dashboardMetricsTimer: null,
        dashboardFailuresTimer: null,

        // Mobile responsive
        mobileSidebarOpen: false,
        mobileToolbarOpen: false,

        // External task UI
        // _registeredUITypes 已移除，由 TaskUI 注册表替代
        // showCustomUIModal、customUITitle、customUIData 已移除，由 TaskUI 查看器替代
        // showCustomTaskView 已移除，由 TaskUI 替代
        // 保留 loadTaskUI 别名，兼容 taskList.js 等模块通过 this.loadTaskUI() 调用
        // 实际实现在 taskList.js 中已改为直接调用 TaskUI.loadTaskUI()

        // Task type defaults modal
        showTaskTypeDefaultsModal: false,
        taskTypeDefaultsData: {},
        // Video player (from old mixin, now pure state)
        currentVideo: null,
        isPlaying: false,
        isBuffering: false,
        currentTime: 0,
        duration: 0,
        buffered: 0,
        volume: 0.1,
        isMuted: false,
        playbackRate: 1.0,
        showControls: true,
        controlsTimer: null,
        showPlayIcon: false,
        hoverTime: null,
        hoverProgressPosition: 0,
        showVideoSettings: false,
        videoSettings: {
          skipInterval: 10,
          defaultSpeed: 1.0,
          defaultVolume: 0.1,
          autoPlay: true
        },
        collectionList: [],
      }
    },

    computed: {
      isWriteDisabled: function () {
        var rt = this.runtime || {}
        var f = rt.features || {}
        return rt.mode === 'ui' || (!f.download && !f.scheduler)
      },
      volumeIcon: function () {
        if (this.isMuted || this.volume === 0) return 'fa-volume-mute'
        if (this.volume < 0.5) return 'fa-volume-down'
        return 'fa-volume-up'
      },
      filteredTasks: function () {
        if (this.selectedType === 'all') return this.tasks || []
        var type = this.selectedType
        return (this.tasks || []).filter(function (t) { return t && String(t.type || '').toLowerCase() === String(type).toLowerCase() })
      },
      filteredObjects: function () {
        if (!this.selectedTask || !this.selectedTask.objects) return []
        var list = this.selectedTask.objects
        if (this.statusFilter === 'all') return list
        return list.filter(function (o) { return o.status === this.statusFilter }.bind(this))
      },
      aggFilteredObjects: function () { return this.aggObjects || [] },
      aggPagedObjects: function () { return this.aggFilteredObjects || [] },
      groupModalSafety: function () {
        var list = Array.isArray(this.groupModal.list) ? this.groupModal.list : []
        var priorityCounts = {}
        var hasDownloading = false
        list.forEach(function (it) {
          if (!it) return
          var priority = this.getObjectVariantPriority(it)
          priorityCounts[priority] = (priorityCounts[priority] || 0) + 1
          if (it.status === 'downloading') hasDownloading = true
        }.bind(this))
        var hasPriorityConflict = Object.values(priorityCounts).some(function (c) { return c > 1 })
        var pendingTargets = hasPriorityConflict ? [] : list.filter(function (it) { return this.isGroupCancelTarget(it) }.bind(this))
        return {
          hasPriorityConflict: hasPriorityConflict,
          hasDownloading: hasDownloading,
          pendingTargets: pendingTargets,
          pendingCancelableCount: pendingTargets.length,
          taskId: (this.groupModal.taskId || '').trim(),
          taskType: (this.groupModal.taskType || '').trim()
        }
      },
      filteredChanges: function () {
        if (!this.configDiff || !this.configDiff.changes) return []
        if (!this.pathFilter) return this.configDiff.changes
        return this.configDiff.changes.filter(function (c) { return c.path.startsWith(this.pathFilter) }.bind(this))
      },

      // ---- TaskUI integration ----
      showTaskTypeFormFields: function () {
        var handler = TaskUI.get(this.newTask.type)
        return handler && handler.renderForm !== null
      },
      taskTypeFormComponent: function () {
        var handler = TaskUI.get(this.newTask.type)
        if (handler && handler.renderForm) {
          var self = this
          return {
            render: function () {
              return handler.renderForm(self.newTask, {})
            }
          }
        }
        return null
      },
      showTaskTypeMeta: function () {
        if (!this.selectedTask || !this.selectedTask.extra) return false
        var handler = TaskUI.get(this.selectedTask.type)
        return handler && handler.renderMeta !== null
      },
      taskTypeMetaComponent: function () {
        var handler = TaskUI.get(this.selectedTask.type)
        if (handler && handler.renderMeta) {
          var task = this.selectedTask
          return {
            render: function () {
              return handler.renderMeta(task)
            }
          }
        }
        return null
      }
    },

    watch: {
      currentVideo: function (val) {
        if (val) {
          var self = this
          this.$nextTick(function () {
            self._videoKeyHandler = function(e){UiVideoPlayer.handleKeydown(self,e)}
            window.addEventListener('keydown', self._videoKeyHandler)
            self.isPlaying = true
            self.playbackRate = 1.0
            self.showControls = true
            if (self.$refs.videoModal) self.$refs.videoModal.focus()
          })
        } else {
          if (this._videoKeyHandler) window.removeEventListener('keydown', this._videoKeyHandler)
        }
      },
      sortBy: function () { this.pagination.page = 1; this.fetchTaskDetails(this.selectedTaskId) },
      searchQuery: function (newVal) {
        var self = this
        if (this.searchTimer) clearTimeout(this.searchTimer)
        this.searchTimer = setTimeout(function () { self.pagination.page = 1; self.fetchTaskDetails(self.selectedTaskId) }, 500)
      },
      aggSearchQuery: function () { this.aggPagination.page = 1; this.fetchAggregateByType(this.selectedType) },
      aggStatusFilter: function () { this.aggPagination.page = 1; this.fetchAggregateByType(this.selectedType) },
      aggGroupBy: function () { this.aggPagination.page = 1; this.fetchAggregateByType(this.selectedType) },
      selectedType: function () {
        if (typeof window.__dm_updateURLWithType === 'function') window.__dm_updateURLWithType(this.selectedType)
        if (this.viewMode === 'aggregate') { this.aggPagination.page = 1; this.fetchAggregateByType(this.selectedType) }
        this.loadTaskUIForType(this.selectedType)
      },
      selectedTask: function () {
        this.$nextTick(function () {
          if (this.uiMode === 'watch' && this.selectedTask) {
            // Task-specific watch mode rendering is handled by TaskUI plugins
          }
        }.bind(this))
      },
      viewMode: function (val) {
        if (val === 'dashboard') {
          this.fetchDashboardData()
          this.startDashboardPolling()
        } else {
          this.stopDashboardPolling()
        }
        this.$nextTick(function () {
          // Task-specific watch mode rendering is handled by TaskUI plugins
        }.bind(this))
      }
    },

    mounted: function () {
      this.initTypeFromURL()
      this.initRuntime()
      this.fetchTasks()
      this.initSSE()
      this.loadVideoSettings()
      this.initUiDefaults()
      this.showAddTaskModal = false
    },

    beforeUnmount: function () {
      if (this.timer) clearInterval(this.timer)
      if (this.eventSource) this.eventSource.close()
      if (this.abortController) this.abortController.abort()
      this.stopDashboardPolling()
      if (this._videoKeyHandler) window.removeEventListener('keydown', this._videoKeyHandler)
    },

    methods: {
      // ---- TaskUI integration ----
      // ---- Delegates to UiHelpers/UiTaskList/UiVideoPlayer/UiDashboard ----
      openConfig: function() { UiHelpers.openConfig(this) },
      fetchTasks: function() { UiTaskList.fetchTasks(this) },
      openAggregateView: function() { UiHelpers.openAggregateView(this) },
      openAddTask: function(e) { UiHelpers.openAddTask(this, e) },
      toggleSelectAll: function() { UiTaskList.toggleSelectAll(this) },
      cancelSelected: function() { UiTaskList.cancelSelected(this) },
      selectTask: function(id) { UiTaskList.selectTask(this, id) },
      retryAllFailed: function() { UiTaskList.retryAllFailed(this) },
      cancelCurrentTask: function() { UiTaskList.cancelCurrentTask(this) },
      changePage: function(p) { UiTaskList.changePage(this, p) },
      changeLimit: function() { UiTaskList.changeLimit(this) },
      fetchTaskDetails: function(id, background) { UiTaskList.fetchTaskDetails(this, id, background) },
      saveTaskConfig: function() { UiTaskList.saveTaskConfig(this) },
      toggleTaskConfigPanel: function() { UiTaskList.toggleTaskConfigPanel(this) },
      toggleSelectAllObjects: function() { UiTaskList.toggleSelectAllObjects(this) },
      cancelSelectAllObjects: function() { UiTaskList.cancelSelectAllObjects(this) },
      retrySelectedObjects: function() { UiTaskList.retrySelectedObjects(this) },
      undoCancelSelectAllObjects: function() { UiTaskList.undoCancelSelectAllObjects(this) },
      cancelObject: function(obj) { UiTaskList.cancelObject(this, obj) },
      undoCancelObject: function(obj) { UiTaskList.undoCancelObject(this, obj) },
      hasOnClick: function(obj) { return UiHelpers.hasOnClick(obj) },
      saveConfig: function() { UiHelpers.saveConfig(this) },
      openConfigHistory: function() { UiHelpers.openConfigHistory(this) },
      saveNewTask: function() { UiHelpers.saveNewTask(this) },
      fetchAggregateByType: function(type) { UiHelpers.fetchAggregateByType(this, type) },
      cancelAggObject: function(obj) { UiHelpers.cancelAggObject(this, obj) },
      changeAggPage: function(p) { UiHelpers.changeAggPage(this, p) },
      copyText: function(text) { UiHelpers.copyText(text) },
      initTypeFromURL: function() { UiHelpers.initTypeFromURL(this) },
      initRuntime: function() { UiHelpers.initRuntime(this) },
      initUiDefaults: function() { UiHelpers.initUiDefaults(this) },
      // Group helpers
      getScopedTaskInfo: function(obj) { return UiHelpers.getScopedTaskInfo(obj) },
      getObjectVariantPriority: function(obj) { return UiHelpers.getObjectVariantPriority(obj) },
      isGroupCancelTarget: function(obj) { return UiHelpers.isGroupCancelTarget(obj) },
      getObjectVariantLabel: function(obj) { return UiHelpers.getObjectVariantLabel(obj) },
      metadataContentGroup: function(obj) { return UiHelpers.metadataContentGroup(obj) },
      // Object display helpers
      getTitle: function(obj) { return UiHelpers.getTitle(obj) },
      getDate: function(obj) { return UiHelpers.getDate(obj) },
      getDuration: function(obj) { return UiHelpers.getDuration(obj) },
      getTags: function(obj) { return UiHelpers.getTags(obj) },
      getObjId: function(obj) { return UiHelpers.getObjId(obj) },
      getTaskTypeForObj: function(obj) { return UiHelpers.getTaskTypeForObj(obj) },
      isTouchDevice: function() { return UiHelpers.isTouchDevice() },
      pathToUrl: function(path) { return UiHelpers.pathToUrl(path, this.runtime) },
      getFileUrl: function(obj) { return UiHelpers.getFileUrl(obj, this.runtime) },
      getTaskDisplayName: function(task) { return UiHelpers.getTaskDisplayName(task) },
      getTaskTypeBadge: function(task) { return UiHelpers.getTaskTypeBadge(task) },
      // Video player delegates
      handleKeydown: function(e) { UiVideoPlayer.handleKeydown(this, e) },
      loadVideoSettings: function() { UiVideoPlayer.loadVideoSettings(this) },
      saveVideoSettings: function() { UiVideoPlayer.saveVideoSettings(this) },
      resetVideoSettings: function() { UiVideoPlayer.resetVideoSettings(this) },
      onLoadedMetadata: function() { UiVideoPlayer.onLoadedMetadata(this) },
      updateProgress: function() { UiVideoPlayer.updateProgress(this) },
      onEnded: function() { UiVideoPlayer.onEnded(this) },
      togglePlay: function() { UiVideoPlayer.togglePlay(this) },
      seekClick: function(e) { UiVideoPlayer.seekClick(this, e) },
      handleHoverProgress: function(e) { UiVideoPlayer.handleHoverProgress(this, e) },
      skip: function(s) { UiVideoPlayer.skip(this, s) },
      setSpeed: function(r) { UiVideoPlayer.setSpeed(this, r) },
      toggleMute: function() { UiVideoPlayer.toggleMute(this) },
      updateVolume: function() { UiVideoPlayer.updateVolume(this) },
      toggleFullscreen: function() { UiVideoPlayer.toggleFullscreen() },
      onMouseMove: function() { UiVideoPlayer.onMouseMove(this) },
      formatTime: function(s) { return UiVideoPlayer.formatTime(s) },
      playPrev: function() { UiVideoPlayer.playPrev(this) },
      playNext: function() { UiVideoPlayer.playNext(this) },
      switchToCollectionItem: function(item) { UiVideoPlayer.switchToCollectionItem(this, item) },
      closeVideo: function() { UiVideoPlayer.closeVideo(this) },
      playVideo: function(obj) { UiVideoPlayer.playVideo(this, obj) },
      isVideo: function(obj) { return UiVideoPlayer.isVideo(obj) },

      // Missing methods from original mixin
      setHoverObj: function(obj) {
        var self = this
        if (this.hoverTimer) clearTimeout(this.hoverTimer)
        this.hoverTimer = setTimeout(function () { self.hoverObj = obj }, 600)
      },
      clearHoverObj: function() {
        if (this.hoverTimer) clearTimeout(this.hoverTimer)
        this.hoverObj = null
      },
      initSSE: function() {
        if (this.eventSource) this.eventSource.close()
        Log.debug('initSSE connecting')
        var self = this
        this.eventSource = new EventSource('/api/events')
        this.eventSource.onmessage = function (event) {
          try {
            var data = JSON.parse(event.data)
            Log.trace('SSE event', { type: data.type, taskId: data.payload && data.payload.task_id })
            self.handleSSEEvent(data)
          } catch (e) { Log.error('SSE Parse Error', { error: e.message }) }
        }
        this.eventSource.onerror = function () {
          Log.warn('SSE connection error, reconnecting...')
          UiHelpers.showToast('Connection lost. Reconnecting...', 'error')
        }
        this.eventSource.onopen = function () {
          Log.debug('SSE connected')
          self.fetchTasks()
        }
      },
      handleSSEEvent: function(event) {
        var self = this
        if (event.type === 'object_update' || event.type === 'shared_object_update') {
          var obj = event.payload
          if (obj.status === 'downloading') {
            var idx = this.activeDownloads.findIndex(function (d) { return d.url === obj.url })
            if (idx >= 0) {
              this.activeDownloads[idx] = { task_id: obj.task_id, url: obj.url, title: (obj.metadata && obj.metadata.title) || obj.url, progress: obj.progress, status: obj.status }
            } else {
              this.activeDownloads.push({ task_id: obj.task_id, url: obj.url, title: (obj.metadata && obj.metadata.title) || obj.url, progress: obj.progress, status: obj.status })
            }
          } else {
            var idx2 = this.activeDownloads.findIndex(function (d) { return d.url === obj.url })
            if (idx2 >= 0) this.activeDownloads.splice(idx2, 1)
            if (obj.status === 'completed') UiHelpers.showToast('Download completed: ' + UiHelpers.getTitle(obj), 'success')
            else if (obj.status === 'failed') UiHelpers.showToast('Download failed: ' + UiHelpers.getTitle(obj), 'error')
            else if (obj.status === 'cancelled') UiHelpers.showToast('已取消: ' + UiHelpers.getTitle(obj), 'info')
          }
          if (this.selectedTask && this.selectedTask.objects) {
            var currentObj = this.selectedTask.objects.find(function (o) { return o.url === obj.url })
            if (currentObj) { currentObj.status = obj.status; currentObj.progress = obj.progress; if (obj.metadata) currentObj.metadata = obj.metadata }
          }
          if (this.viewMode === 'aggregate' && Array.isArray(this.aggObjects) && this.aggObjects.length > 0) {
            var objType = (obj && typeof obj.type === 'string') ? obj.type : null
            if (!objType) {
              var task = this.tasks.find(function (t) { return t.id === obj.task_id })
              if (task && typeof task.type === 'string') objType = task.type
            }
            if (this.selectedType !== 'all' && objType && objType !== this.selectedType) return
            var idxAgg = this.aggObjects.findIndex(function (o) { return o.url === obj.url && o.task_id === obj.task_id })
            if (idxAgg >= 0) {
              var existing = this.aggObjects[idxAgg]
              existing.status = obj.status
              existing.progress = obj.progress
              if (obj.metadata) existing.metadata = obj.metadata
              this.aggObjects.splice(idxAgg, 1, existing)
            }
          }
        } else if (event.type === 'task_update') {
          var summary = event.payload
          var ti = this.tasks.findIndex(function (t) { return t.id === summary.id })
          if (ti >= 0) { this.tasks[ti] = Object.assign({}, this.tasks[ti], summary) }
        } else if (event.type === 'task_list_change') {
          this.fetchTasks()
        } else if (event.type === 'progress_batch') {
          var updates = event.payload.updates
          if (updates && updates.length > 0) {
            for (var pi = 0; pi < updates.length; pi++) {
              var item = updates[pi]
              var aidx = this.activeDownloads.findIndex(function (d) { return d.url === item.url })
              if (aidx >= 0) { this.activeDownloads[aidx].progress = item.progress }
              if (this.selectedTask && this.selectedTask.objects) {
                var currentObj = this.selectedTask.objects.find(function (o) { return o.url === item.url })
                if (currentObj) { currentObj.progress = item.progress }
              }
              if (this.viewMode === 'aggregate' && Array.isArray(this.aggObjects) && this.aggObjects.length > 0) {
                var idxAgg = this.aggObjects.findIndex(function (o) { return o.url === item.url && o.task_id === item.task_id })
                if (idxAgg >= 0) { this.aggObjects[idxAgg].progress = item.progress }
              }
            }
          }
        }
      },
      openGroupModal: function(obj) {
        var info = UiHelpers.getScopedTaskInfo(obj)
        var groupId = obj && obj.metadata && obj.metadata.content_group
        if (!groupId) {
          this.groupModal.taskId = info.taskId
          this.groupModal.taskType = info.taskType
          this.showGroupModal = true
          return
        }
        var self = this
        AppAPI.groupObjects(groupId, info.taskId, info.taskType).then(function(data) {
          self.groupModal.title = groupId
          self.groupModal.list = data.objects || []
          self.groupModal.taskId = info.taskId
          self.groupModal.taskType = info.taskType
          self.showGroupModal = true
        }).catch(function(e) {
          UiHelpers.showToast('加载分组失败: ' + e.message, 'error')
        })
      },
      closeGroupModal: function() {
        this.showGroupModal = false
        this.groupModal = { taskId: '', taskType: '' }
      },
      changeAggLimit: function() { UiHelpers.changeAggLimit(this) },
      handleCardClick: function(obj) {
        if (!obj) return
        Log.debug('handleCardClick', { url: obj.url, status: obj.status, taskType: obj.metadata && obj.metadata.task_type })
        if (obj.status === 'cancelled' && obj.extra && obj.extra.redirect_url) {
          this.openObjectInfoViewer(obj)
          return
        }
        if (obj.status === 'completed') {
          var type = obj.metadata && obj.metadata.task_type
          if (type && !TaskUI.get(type)) {
            Log.info('handleCardClick: plugin not loaded yet, loading and retrying', { type: type })
            var self = this
            this.loadTaskUIForType(type, function () { self.handleCardClick(obj) })
            return
          }
          var handler = type ? TaskUI.get(type) : null
          if (handler && handler.onClick) {
            var helpers = {
              openTaskTypeViewer: this.openTaskTypeViewer.bind(this),
              playVideo: this.playVideo.bind(this),
              getFileUrl: this.getFileUrl.bind(this),
              pathToUrl: this.pathToUrl.bind(this),
              getTitle: this.getTitle.bind(this)
            }
            if (handler.onClick(obj, helpers)) return
          }
          if (this.isVideo(obj)) { this.playVideo(obj); return }
          this.openObjectInfoViewer(obj)
        }
      },
      getVideoUrl: function(obj) { return UiVideoPlayer.getVideoUrl(obj) },
      getThumbImage: function(obj) { return UiVideoPlayer.getThumbImage(obj) },
      getCoverImage: function(obj) { return UiVideoPlayer.getCoverImage(obj) },
      onCoverError: function(e) { UiVideoPlayer.onCoverError(e) },
      getPreviewUrl: function(obj) { return UiVideoPlayer.getPreviewUrl(obj) },
      // ---- Missing template methods (delegates and stubs) ----
      isGroupRepresentative: function(obj) { return UiHelpers.isGroupRepresentative(obj) },
      retryObject: function(obj) {
        if (this.isWriteDisabled) { UiHelpers.showToast('UI-Only 模式下已禁用', 'error'); return }
        if (!this.selectedTaskId || !obj || !obj.url) return
        var self = this
        AppAPI.post('/api/tasks/' + encodeURIComponent(this.selectedTaskId) + '/retry', { url: obj.url })
          .then(function(res) { if (!res.ok) throw new Error('重试失败'); obj.status = 'pending'; obj.progress = 0; UiHelpers.showToast('已重试', 'success') })
          .catch(function(e) { UiHelpers.showToast('重试失败: ' + e.message, 'error') })
      },
      dragStart: function($event, obj) { this.draggedItem = obj; $event.dataTransfer.effectAllowed = 'move' },
      drop: function($event, obj) { $event.preventDefault(); this.draggedItem = null },
      undoCancelAggObject: function(obj) {
        if (this.isWriteDisabled) { UiHelpers.showToast('UI-Only 模式下已禁用', 'error'); return }
        if (!obj || !obj.task_id) return
        var self = this
        AppAPI.post('/api/tasks/' + encodeURIComponent(obj.task_id) + '/object/undo_cancel', { url: obj.url })
          .then(function(res) { if (!res.ok) throw new Error('撤销失败'); obj.status = 'pending'; UiHelpers.showToast('已撤销取消', 'success') })
          .catch(function(e) { UiHelpers.showToast('撤销失败: ' + e.message, 'error') })
      },
      cancelLowPriorityInGroup: function() {
        if (this.isWriteDisabled) { UiHelpers.showToast('UI-Only 模式下已禁用', 'error'); return }
        var targets = this.groupModalSafety.pendingTargets
        if (!targets || targets.length === 0) return
        var taskId = this.groupModal.taskId
        if (!taskId) { UiHelpers.showToast('无法获取任务ID', 'error'); return }
        var urls = targets.map(function(t) { return t.url })
        var self = this
        AppAPI.post('/api/tasks/' + encodeURIComponent(taskId) + '/object/cancel_batch', { urls: urls })
          .then(function(res) { if (!res.ok) throw new Error('批量取消失败'); UiHelpers.showToast('已取消 ' + urls.length + ' 个低优先级对象', 'success'); self.closeGroupModal() })
          .catch(function(e) { UiHelpers.showToast('批量取消失败: ' + e.message, 'error') })
      },
      fmt: function(val) {
        if (val === null || val === undefined) return ''
        if (typeof val === 'object') return JSON.stringify(val, null, 2)
        return String(val)
      },
      viewConfigDiff: function() {
        var self = this
        AppAPI.post('/api/config/diff', { left: this.diffForm.left, right: this.diffForm.right, options: this.diffOptions })
          .then(function(data) { self.configDiff = data; self.lineDiff = (data && data.lineDiff) || []; self.collapsedLineDiff = (data && data.collapsedLineDiff) || [] })
          .catch(function(e) { UiHelpers.showToast('加载差异失败: ' + e.message, 'error') })
      },
      clearConfigDiff: function() { this.configDiff = null; this.lineDiff = []; this.collapsedLineDiff = [] },
      prepareRollback: function(filename) {
        this.rollbackTarget = filename
        this.showRollbackConfirm = true
        var self = this
        AppAPI.post('/api/config/diff', { left: 'current', right: filename })
          .then(function(data) { self.rollbackDiff = data; self.rollbackLineDiff = (data && data.lineDiff) || [] })
          .catch(function(e) { UiHelpers.showToast('加载差异失败: ' + e.message, 'error') })
      },
      confirmRollback: function() {
        var self = this
        AppAPI.post('/api/config/rollback', { target: this.rollbackTarget })
          .then(function(res) { if (!res.ok) throw new Error('回滚失败'); UiHelpers.showToast('配置已回滚', 'success'); self.showRollbackConfirm = false; self.fetchTasks() })
          .catch(function(e) { UiHelpers.showToast('回滚失败: ' + e.message, 'error') })
      },
      addConfigTagRow: function(filename) {
        var tag = prompt('输入标签名称:')
        if (!tag || !tag.trim()) return
        var self = this
        AppAPI.post('/api/config/backup/' + encodeURIComponent(filename) + '/tag', { tag: tag.trim() })
          .then(function() { UiHelpers.showToast('标签已添加', 'success') })
          .catch(function(e) { UiHelpers.showToast('添加标签失败: ' + e.message, 'error') })
      },
      addConfigNoteRow: function(filename) {
        var note = prompt('输入记录内容:')
        if (!note || !note.trim()) return
        var self = this
        AppAPI.post('/api/config/backup/' + encodeURIComponent(filename) + '/note', { message: note.trim() })
          .then(function() { UiHelpers.showToast('记录已添加', 'success') })
          .catch(function(e) { UiHelpers.showToast('添加记录失败: ' + e.message, 'error') })
      },
      deleteConfigBackupRow: function(filename) {
        if (!confirm('确定要删除备份 ' + filename + ' 吗？')) return
        var self = this
        AppAPI.del('/api/config/backup/' + encodeURIComponent(filename))
          .then(function() { UiHelpers.showToast('备份已删除', 'success'); self.fetchTasks() })
          .catch(function(e) { UiHelpers.showToast('删除失败: ' + e.message, 'error') })
      },
      addConfigTag: function() {
        if (!this.tagForm.tag || !this.tagForm.tag.trim()) return
        var self = this
        AppAPI.post('/api/config/backup/' + encodeURIComponent(this.diffForm.right) + '/tag', { tag: this.tagForm.tag.trim() })
          .then(function() { self.tagForm.tag = ''; self.tagForm.message = '标签已添加'; UiHelpers.showToast('标签已添加', 'success') })
          .catch(function(e) { UiHelpers.showToast('添加标签失败: ' + e.message, 'error') })
      },
      addConfigNote: function() {
        if (!this.noteForm.message || !this.noteForm.message.trim()) return
        var self = this
        AppAPI.post('/api/config/backup/' + encodeURIComponent(this.diffForm.right) + '/note', { message: this.noteForm.message.trim(), author: this.noteForm.author || '' })
          .then(function() { self.noteForm.message = ''; self.noteForm.messageText = '记录已添加'; UiHelpers.showToast('记录已添加', 'success') })
          .catch(function(e) { UiHelpers.showToast('添加记录失败: ' + e.message, 'error') })
      },
      // Dashboard delegates
      fetchDashboardData: function() { UiDashboard.fetchDashboardData(this) },
      fetchHealthz: function() { UiDashboard.fetchHealthz(this) },
      fetchMetrics: function() { UiDashboard.fetchMetrics(this) },
      fetchFailures: function() { UiDashboard.fetchFailures(this) },
      startDashboardPolling: function() { UiDashboard.startDashboardPolling(this) },
      stopDashboardPolling: function() { UiDashboard.stopDashboardPolling() },
      changeDashboardFailuresLimit: function() { UiDashboard.changeDashboardFailuresLimit(this) },
      searchDashboardFailures: function() { UiDashboard.searchDashboardFailures(this) },

      loadTaskUIForType: function (taskType, callback) {
        var self = this
        var handler = TaskUI.get(taskType)
        if (handler) {
          Log.info('loadTaskUIForType ALREADY REGISTERED — using plugin', { type: taskType, features: { form: !!handler.renderForm, meta: !!handler.renderMeta, viewer: !!handler.renderViewer, cardExtra: !!handler.renderCardExtra } })
          if (callback) callback()
          return
        }
        Log.info('loadTaskUIForType NOT REGISTERED — loading viewer.js', { type: taskType })
        TaskUI.loadTaskUI(taskType, function () {
          var h = TaskUI.get(taskType)
          if (h) {
            Log.info('loadTaskUIForType LOADED — plugin now available', { type: taskType, features: { form: !!h.renderForm, meta: !!h.renderMeta, viewer: !!h.renderViewer, cardExtra: !!h.renderCardExtra } })
          } else {
            Log.warn('loadTaskUIForType LOADED — but no plugin registered', { type: taskType })
          }
          if (callback) callback()
        })
      },
      // 保留 loadTaskUI 别名，兼容 taskList.js 等通过 mixin 共享 this 的模块
      loadTaskUI: function (taskType) {
        this.loadTaskUIForType(taskType)
      },
      showTaskTypeViewer: function (obj) {
        if (!obj) return false
        var type = obj.metadata && obj.metadata.task_type
        var handler = TaskUI.get(type)
        var result = handler && typeof handler.renderViewer === 'function' && handler.shouldShowViewer(obj)
        Log.debug('showTaskTypeViewer', { type: type, hasHandler: !!handler, result: result })
        return result
      },
      taskTypeViewerLabel: function (obj) {
        var type = obj && obj.metadata && obj.metadata.task_type
        var handler = TaskUI.get(type)
        var label = (handler && handler.viewerLabel) || '查看'
        return label
      },
      openTaskTypeViewer: function (obj) {
        var type = obj && obj.metadata && obj.metadata.task_type
        var handler = TaskUI.get(type)
        Log.info('openTaskTypeViewer', { type: type, hasHandler: !!handler, renderViewer: !!(handler && handler.renderViewer), title: obj && obj.metadata && obj.metadata.title })
        if (handler && handler.renderViewer) {
          try {
            var container = document.createElement('div')
            document.body.appendChild(container)
            var onClose = function () {
              try { if (container.parentNode) container.parentNode.removeChild(container) } catch (e) {}
            }
            var result = handler.renderViewer(null, obj, onClose)
            // The viewer already appends its DOM to document.body,
            // so we just need to clean up the container on close.
            // result is a dummy VNode (h('div')) that we ignore.
          } catch (e) {
            Log.error('openTaskTypeViewer renderViewer error', { error: e.message, stack: e.stack })
            // Show error modal using pure DOM
            var overlay = TaskUI.Modal.createOverlay()
            var panel = TaskUI.Modal.createPanel('500px')
            overlay.appendChild(panel)
            var header = TaskUI.Modal.createHeader({ title: '查看器加载失败', onClose: function () { if (overlay.parentNode) overlay.parentNode.removeChild(overlay); document.body.style.overflow = '' } })
            panel.appendChild(header)
            var body = document.createElement('div')
            body.style.cssText = 'padding:24px;text-align:center'
            var msg = document.createElement('p')
            msg.style.cssText = 'color:#dc2626;margin-bottom:16px'
            msg.textContent = e.message || '未知错误'
            body.appendChild(msg)
            var closeBtn = document.createElement('button')
            closeBtn.style.cssText = 'padding:6px 12px;border-radius:6px;background:#2563eb;color:#fff;border:none;cursor:pointer;font-size:14px'
            closeBtn.textContent = '关闭'
            closeBtn.onclick = function () { if (overlay.parentNode) overlay.parentNode.removeChild(overlay); document.body.style.overflow = '' }
            body.appendChild(closeBtn)
            panel.appendChild(body)
            document.body.appendChild(overlay)
            document.body.style.overflow = 'hidden'
          }
        } else {
          Log.warn('openTaskTypeViewer no renderViewer', { type: type, hasHandler: !!handler })
        }
      },
      showTaskTypeCardExtra: function (obj) {
        if (!obj) return false
        var type = obj.metadata && obj.metadata.task_type
        var handler = TaskUI.get(type)
        return handler && typeof handler.renderCardExtra === 'function'
      },
      taskTypeCardExtraComponent: function (obj) {
        var type = obj.metadata && obj.metadata.task_type
        var handler = TaskUI.get(type)
        if (handler && handler.renderCardExtra) {
          return {
            render: function () {
              return handler.renderCardExtra(obj)
            }
          }
        }
        return null
      },
      closeCustomUI: function () {
        var el = document.getElementById('custom-ui-content')
        if (el) el.innerHTML = ''
      },
      // 已废弃 — custom-ui-content 元素已从 UI 中移除，保留此方法仅为兼容外部调用
      // closeCustomUI 已无实际作用，将在下次清理中移除

      // ---- Task type defaults modal ----
      openTaskTypeDefaults: function () {
        var self = this
        AppAPI.getTaskTypeDefaults().then(function (data) {
          // Convert extra JSON to editable text area format
          Object.keys(data || {}).forEach(function (typ) {
            var def = data[typ]
            if (def.extra && typeof def.extra === 'object') {
              def.extra_text = JSON.stringify(def.extra, null, 2)
            } else {
              def.extra_text = ''
            }
          })
          self.taskTypeDefaultsData = data || {}
          self.showTaskTypeDefaultsModal = true
        })
      },
      saveTaskTypeDefaults: function () {
        var self = this
        // Convert extra_text back to JSON
        var data = {}
        Object.keys(self.taskTypeDefaultsData).forEach(function (typ) {
          var def = self.taskTypeDefaultsData[typ]
          data[typ] = {
            storage: def.storage || null,
            save_root_dir: def.save_root_dir || '',
            scrape_enabled: !!def.scrape_enabled,
            download_enabled: !!def.download_enabled,
          }
          // Parse extra_text to JSON
          if (def.extra_text && def.extra_text.trim()) {
            try {
              data[typ].extra = JSON.parse(def.extra_text)
            } catch (e) {
              UiHelpers.showToast('类型 ' + typ + ' 的扩展配置 JSON 格式错误', 'error')
              return
            }
          }
        })
        AppAPI.setTaskTypeDefaults(data).then(function () {
          self.showTaskTypeDefaultsModal = false
          self.fetchTasks()
        }).catch(function (e) {
          UiHelpers.showToast('保存失败: ' + e.message, 'error')
        })
      },

      // ---- Default object info viewer (no-task-type fallback) ----
      openObjectInfoViewer: function (obj) {
        if (!obj) return
        Log.info('openObjectInfoViewer', { url: obj.url, status: obj.status, title: UiHelpers.getTitle(obj) })
        var title = UiHelpers.getTitle(obj) || obj.url || ''
        var fileUrl = UiHelpers.getFileUrl(obj, this.runtime)
        var tags = UiHelpers.getTags(obj).slice()
        var dateVal = UiHelpers.getDate(obj)
        var duration = UiHelpers.getDuration(obj)
        var tagState = { items: tags, adding: false, inputValue: '' }
        function saveTags () {
          var taskType = UiHelpers.getTaskTypeForObj(obj)
          var objId = UiHelpers.getObjId(obj)
          AppAPI.updateObjectTags(taskType, objId, tagState.items).then(function (res) {
            if (!res.ok) throw new Error('save failed')
            obj.extra = obj.extra || {}
            obj.extra.tags = tagState.items.slice()
          }).catch(function (e) {
            UiHelpers.showToast('保存标签失败: ' + e.message, 'error')
          })
        }
        var overlay = document.createElement('div')
        overlay.style.cssText = 'position:fixed;inset:0;background:rgba(0,0,0,0.7);z-index:50;display:flex;align-items:center;justify-content:center;padding:16px;-webkit-backdrop-filter:blur(4px);backdrop-filter:blur(4px)'
        var panel = document.createElement('div')
        panel.style.cssText = 'background:#fff;border-radius:8px;box-shadow:0 25px 50px rgba(0,0,0,0.25);width:100%;max-width:672px;max-height:90vh;overflow:hidden;display:flex;flex-direction:column'
        overlay.appendChild(panel)
        function onClose () {
          if (overlay.parentNode) overlay.parentNode.removeChild(overlay)
          document.body.style.overflow = ''
        }
        var header = document.createElement('div')
        header.style.cssText = 'padding:16px;border-bottom:1px solid #e5e7eb;display:flex;justify-content:space-between;align-items:center;background:#f9fafb'
        var hTitle = document.createElement('h3')
        hTitle.style.cssText = 'font-size:18px;font-weight:700;color:#1f2937;margin:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap'
        hTitle.textContent = title || '对象信息'
        header.appendChild(hTitle)
        var hClose = document.createElement('button')
        hClose.innerHTML = '<i class="fas fa-times"></i>'
        hClose.style.cssText = 'color:#6b7280;cursor:pointer;background:none;border:none;font-size:18px'
        hClose.onclick = function (e) { e.stopPropagation(); onClose() }
        header.appendChild(hClose)
        panel.appendChild(header)
        var body = document.createElement('div')
        body.style.cssText = 'flex:1;overflow-y:auto;padding:16px'
        function addRow(l, v) {
          if (!v) return
          var r = document.createElement('div'); r.style.cssText = 'font-size:12px'
          var lb = document.createElement('span'); lb.style.cssText = 'color:#6b7280'; lb.textContent = l + ': '
          r.appendChild(lb)
          var vl = document.createElement('span'); vl.style.cssText = 'color:#374151;word-break:break-all'; vl.textContent = v
          r.appendChild(vl); body.appendChild(r)
        }
        var sr = document.createElement('div'); sr.style.cssText = 'display:flex;align-items:center;gap:8px'
        var sl = document.createElement('span'); sl.style.cssText = 'font-size:12px;color:#6b7280'; sl.textContent = '状态：'
        sr.appendChild(sl)
        var sb = document.createElement('span')
        sb.style.cssText = 'padding:2px 8px;font-size:12px;font-weight:600;border-radius:9999px;background:' + (obj.status === 'completed' ? '#d1fae5' : obj.status === 'failed' ? '#fee2e2' : obj.status === 'downloading' ? '#fef3c7' : '#f3f4f6') + ';color:' + (obj.status === 'completed' ? '#065f46' : obj.status === 'failed' ? '#991b1b' : obj.status === 'downloading' ? '#92400e' : '#374151')
        sb.textContent = obj.status || ''
        sr.appendChild(sb); body.appendChild(sr)
        addRow('URL', obj.url)
        addRow('文件路径', obj.save_path)
        addRow('日期', dateVal)
        addRow('时长', duration)
        var ts = document.createElement('div'); ts.style.cssText = 'margin-top:8px'
        var tw = document.createElement('div'); tw.style.cssText = 'display:flex;flex-wrap:wrap;gap:4px;align-items:center'
        function renderTags() {
          tw.innerHTML = ''
          tagState.items.forEach(function(tag,i) {
            var t = document.createElement('span')
            t.style.cssText = 'font-size:12px;background:#f3f4f6;color:#4b5563;padding:2px 8px;border-radius:4px;display:inline-flex;align-items:center;gap:4px'
            t.textContent = '#' + tag
            var d = document.createElement('button'); d.textContent = 'x'; d.style.cssText = 'color:#f87171;cursor:pointer;background:none;border:none;font-size:12px;margin-left:4px'
            d.onclick = function(e) { e.stopPropagation(); tagState.items.splice(i,1); renderTags(); saveTags() }
            t.appendChild(d); tw.appendChild(t)
          })
          if (!tagState.adding) {
            var ab = document.createElement('button'); ab.textContent = '+'; ab.style.cssText = 'color:#3b82f6;cursor:pointer;background:none;border:none;font-size:12px;margin-left:4px'
            ab.onclick = function(e) { e.stopPropagation(); tagState.adding=true; tagState.inputValue=''; renderTags() }
            tw.appendChild(ab)
          } else {
            var inp = document.createElement('input'); inp.type='text'; inp.style.cssText='border:1px solid #d1d5db;border-radius:4px;padding:2px 4px;font-size:12px;width:96px'
            inp.value = tagState.inputValue
            inp.onkeydown = function(e) {
              if (e.key==='Enter') { e.preventDefault(); var v=inp.value.trim(); if(v&&tagState.items.indexOf(v)<0){tagState.items.push(v);saveTags()} tagState.adding=false; renderTags() }
              else if (e.key==='Escape') { tagState.adding=false; renderTags() }
            }
            tw.appendChild(inp)
            var cb = document.createElement('button'); cb.textContent = 'v'; cb.style.cssText = 'color:#22c55e;cursor:pointer;background:none;border:none;font-size:12px;margin-left:4px'
            cb.onclick = function(e) { e.stopPropagation(); var v=inp.value.trim(); if(v&&tagState.items.indexOf(v)<0){tagState.items.push(v);saveTags()} tagState.adding=false; renderTags() }
            tw.appendChild(cb)
            setTimeout(function(){inp.focus()},0)
          }
          ts.appendChild(tw)
        }
        renderTags(); body.appendChild(ts)
        if (obj.metadata) {
          var md = document.createElement('div'); md.style.cssText = 'border-top:1px solid #e5e7eb;padding-top:12px;margin-top:8px'
          var mt = document.createElement('p'); mt.style.cssText = 'font-size:12px;font-weight:600;color:#6b7280;margin:0 0 4px'; mt.textContent = '元数据'
          md.appendChild(mt)
          Object.keys(obj.metadata).filter(function(k){return ['title','date','duration'].indexOf(k)<0}).forEach(function(k){
            var r=document.createElement('div');r.style.cssText='font-size:12px;color:#4b5563';r.textContent=k+': '+(obj.metadata[k]||'');md.appendChild(r)
          })
          body.appendChild(md)
        }
        if (obj.extra) {
          var ed = document.createElement('div'); ed.style.cssText = 'border-top:1px solid #e5e7eb;padding-top:12px;margin-top:8px'
          var et = document.createElement('p'); et.style.cssText = 'font-size:12px;font-weight:600;color:#6b7280;margin:0 0 4px'; et.textContent = '扩展信息'
          ed.appendChild(et)
          Object.keys(obj.extra).filter(function(k){return k!=='tags'&&k!=='images'&&k!=='files'}).forEach(function(k){
            var r=document.createElement('div');r.style.cssText='font-size:12px;color:#4b5563;word-break:break-all';var v=obj.extra[k];if(typeof v==='object')v=JSON.stringify(v,null,2);r.textContent=k+': '+v;ed.appendChild(r)
          })
          body.appendChild(ed)
        }
        panel.appendChild(body)
        var footer = document.createElement('div')
        footer.style.cssText = 'padding:12px;border-top:1px solid #e5e7eb;background:#f9fafb;display:flex;justify-content:space-between;align-items:center'
        var fl = document.createElement('div'); fl.style.cssText = 'display:flex;gap:8px'
        if (fileUrl) {
          var of = document.createElement('a'); of.href=fileUrl; of.target='_blank'; of.rel='noopener noreferrer'; of.style.cssText='padding:6px 12px;border-radius:6px;background:#2563eb;color:#fff;text-decoration:none;font-size:14px;display:inline-block'; of.textContent='打开文件'; fl.appendChild(of)
        }
        if (obj.metadata&&obj.metadata.page_url) {
          var op = document.createElement('a'); op.href=obj.metadata.page_url; op.target='_blank'; op.rel='noopener noreferrer'; op.style.cssText='padding:6px 12px;border-radius:6px;background:#fff;border:1px solid #d1d5db;text-decoration:none;cursor:pointer;font-size:14px;color:#374151;display:inline-block'; op.textContent='打开原页面'; fl.appendChild(op)
        }
        footer.appendChild(fl)
        var cb2 = document.createElement('button'); cb2.style.cssText='padding:6px 12px;border-radius:6px;background:#fff;border:1px solid #d1d5db;cursor:pointer;font-size:14px;color:#374151'; cb2.textContent='关闭'; cb2.onclick=function(e){e.stopPropagation();onClose()}; footer.appendChild(cb2)
        panel.appendChild(footer)
        overlay.addEventListener('click', function(e){if(e.target===overlay)onClose()})
        function keyHandler(e){if(e.key==='Escape')onClose()}
        document.addEventListener('keydown',keyHandler)
        var origOnClose=onClose; onClose=function(){document.removeEventListener('keydown',keyHandler);document.body.style.overflow='';if(origOnClose)origOnClose()}
        document.body.appendChild(overlay); document.body.style.overflow='hidden'
      }
    }
  })

  app.mount('#app')
})()