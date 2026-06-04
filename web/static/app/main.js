// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * Vue app initialization — wires together all modules.
 * Must be loaded AFTER all other app/*.js files and Vue CDN.
 * Depends on: AppAPI, AppVideoPlayer, AppHelpers (global)
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
          id: '', type: 'tktube', save_dir: './downloads', urls_text: '', keyword: '',
          subtype: 'tag', max_concurrent: 1, refresh_interval: 3600,
          storage_type: 'file', storage_config: { path: '', source: '', database: '', collection: '' }
        },
        taskConfigForm: { concurrency: 1, refresh_interval: 3600 },
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
        tktubeObjects: [],
        tktubeSearchQuery: '',
        tktubeStatusFilter: 'all',
        tktubeLoading: false,
        tktubeGroupBy: false,
        aggViewMode: 'grid',
        uiMode: 'manage',
        tktubePagination: { page: 1, limit: 50, total: 0 },
        tktubeSortBy: 'date_desc',
        tktubeAggConcurrency: 2,
        tktubeAggDelayMs: 200,
        lastAggFetchTs: 0,
        tktubeAggMinIntervalMs: 3000,
        showVikacgModal: false,
        vikacgModalObj: null,
        vikacgActiveImgIdx: 0,
        showHanimeModal: false,
        hanimeModalObj: null,
        hanimeActiveCoverIdx: 0,
        hanimeActivePosterIdx: 0,
        hanimeVideoError: false,
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
        dashboardFailuresTimer: null
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
      selectedObjectCount: function () { return this.selectedObjectUrls.length },
      tktubeFilteredObjects: function () { return this.tktubeObjects || [] },
      tktubePagedObjects: function () { return this.tktubeFilteredObjects || [] },
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
      }
    },

    watch: {
      currentVideo: function (val) {
        if (val) {
          var self = this
          this.$nextTick(function () {
            window.addEventListener('keydown', self.handleKeydown)
            self.isPlaying = true
            self.playbackRate = 1.0
            self.showControls = true
            if (self.$refs.videoModal) self.$refs.videoModal.focus()
          })
        } else {
          window.removeEventListener('keydown', this.handleKeydown)
        }
      },
      sortBy: function () { this.pagination.page = 1; this.fetchTaskDetails(this.selectedTaskId) },
      searchQuery: function (newVal) {
        var self = this
        if (this.searchTimer) clearTimeout(this.searchTimer)
        this.searchTimer = setTimeout(function () { self.pagination.page = 1; self.fetchTaskDetails(self.selectedTaskId) }, 500)
      },
      tktubeSearchQuery: function () { this.tktubePagination.page = 1; this.fetchAggregateByType(this.selectedType) },
      tktubeStatusFilter: function () { this.tktubePagination.page = 1; this.fetchAggregateByType(this.selectedType) },
      tktubeGroupBy: function () { this.tktubePagination.page = 1; this.fetchAggregateByType(this.selectedType) },
      selectedType: function () {
        if (typeof window.__dm_updateURLWithType === 'function') window.__dm_updateURLWithType(this.selectedType)
        if (this.viewMode === 'tktube') { this.tktubePagination.page = 1; this.fetchAggregateByType(this.selectedType) }
      },
      viewMode: function (val) {
        if (val === 'dashboard') {
          this.fetchDashboardData()
          this.startDashboardPolling()
        } else {
          this.stopDashboardPolling()
        }
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
    }
  })

  // Register video player module
  AppVideoPlayer.register(app)

  // Register helpers and rest methods (loaded from separate modules or inline below)
  if (typeof AppTaskList !== 'undefined') AppTaskList.register(app)
  if (typeof AppAggregateView !== 'undefined') AppAggregateView.register(app)
  if (typeof AppConfigPanel !== 'undefined') AppConfigPanel.register(app)
  if (typeof AppDownloadView !== 'undefined') AppDownloadView.register(app)
  if (typeof AppDashboard !== 'undefined') AppDashboard.register(app)

  app.mount('#app')
})()