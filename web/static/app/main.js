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
            var view = window.__dm_uiBridge && window.__dm_uiBridge.getTaskView(this.selectedTask.type)
            if (view) {
              view.render(this.selectedTask)
            }
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
          if (this.selectedTask && this.uiMode === 'watch') {
            var view = window.__dm_uiBridge && window.__dm_uiBridge.getTaskView(this.selectedTask.type)
            if (view) view.render(this.selectedTask)
          }
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
    },

    methods: {
      // ---- TaskUI integration ----
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
          self.$forceUpdate()
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
        var result = handler && handler.renderViewer !== null && handler.shouldShowViewer(obj)
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
          var self = this
          var container = document.createElement('div')
          document.body.appendChild(container)
          try {
            var vm = Vue.createApp({
              render: function () {
                var h = Vue.h
                try {
                  var result = handler.renderViewer(obj, function () {
                    try { vm.unmount() } catch (e) { Log.error('openTaskTypeViewer unmount error', { error: e.message }) }
                    try { if (container.parentNode) container.parentNode.removeChild(container) } catch (e) {}
                  })
                  Log.info('openTaskTypeViewer renderViewer returned', { type: typeof result, isVNode: !!(result && result.type) })
                  return result
                } catch (e) {
                  Log.error('openTaskTypeViewer renderViewer error', { error: e.message, stack: e.stack })
                  return h('div', { class: 'fixed inset-0 bg-black bg-opacity-70 z-50 flex items-center justify-center p-4' }, [
                    h('div', { class: 'bg-white rounded-lg p-6 text-center' }, [
                      h('p', { class: 'text-red-600 mb-2' }, '查看器加载失败: ' + e.message),
                      h('button', { class: 'px-3 py-1.5 rounded bg-blue-600 text-white text-sm', on: { click: function () { try { vm.unmount() } catch (e) {}; try { if (container.parentNode) container.parentNode.removeChild(container) } catch (e) {} } } }, '关闭')
                    ])
                  ])
                }
              }
            }).mount(container)
          } catch (e) {
            Log.error('openTaskTypeViewer createApp error', { error: e.message, stack: e.stack })
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
              self.showToast('类型 ' + typ + ' 的扩展配置 JSON 格式错误', 'error')
              return
            }
          }
        })
        AppAPI.setTaskTypeDefaults(data).then(function () {
          self.showTaskTypeDefaultsModal = false
          self.fetchTasks()
        }).catch(function (e) {
          self.showToast('保存失败: ' + e.message, 'error')
        })
      },

      // ---- Default object info viewer (no-task-type fallback) ----
      openObjectInfoViewer: function (obj) {
        if (!obj) return
        Log.info('openObjectInfoViewer', { url: obj.url, status: obj.status, title: this.getTitle(obj) })
        var self = this
        var container = document.createElement('div')
        document.body.appendChild(container)

        // ---- Tag editing state ----
        var tagState = Vue.reactive({
          items: self.getTags(obj).slice(),
          adding: false,
          inputValue: ''
        })

        function saveTags () {
          var taskType = self.getTaskTypeForObj(obj)
          var objId = self.getObjId(obj)
          AppAPI.updateObjectTags(taskType, objId, tagState.items).then(function (res) {
            if (!res.ok) throw new Error('保存标签失败')
            obj.extra = obj.extra || {}
            obj.extra.tags = tagState.items.slice()
          }).catch(function (e) {
            self.showToast('保存标签失败: ' + e.message, 'error')
            Log.error('Failed to save tags', e)
          })
        }

        var vm = Vue.createApp({
          render: function () {
            var h = Vue.h
            var title = self.getTitle(obj) || obj.url || ''
            var fileUrl = self.getFileUrl(obj)
            var tags = self.getTags(obj)
            var dateVal = self.getDate(obj)
            var duration = self.getDuration(obj)

            function onClose () {
              vm.unmount()
              if (container.parentNode) container.parentNode.removeChild(container)
            }

            return h('div', {
              class: 'fixed inset-0 bg-black bg-opacity-70 z-50 flex items-center justify-center p-4 backdrop-blur-sm',
              on: { click: function (e) { if (e.target === e.currentTarget) onClose() } }
            }, [
              h('div', { class: 'bg-white rounded-lg shadow-2xl w-full max-w-2xl max-h-[90vh] overflow-hidden flex flex-col' }, [
                // Header
                h('div', { class: 'p-4 border-b flex justify-between items-center bg-gray-50' }, [
                  h('h3', { class: 'text-lg font-bold text-gray-800 truncate' }, title || '对象信息'),
                  h('button', { class: 'text-gray-500 hover:text-gray-700', on: { click: function (e) { e.stopPropagation(); onClose() } } }, [
                    h('i', { class: 'fas fa-times' })
                  ]),
                ]),
                // Body
                h('div', { class: 'flex-1 overflow-y-auto p-4 space-y-3' }, [
                  // Status badge
                  h('div', { class: 'flex items-center gap-2' }, [
                    h('span', { class: 'text-xs text-gray-500' }, '状态：'),
                    h('span', {
                      class: 'px-2 py-0.5 text-xs font-semibold rounded-full',
                      style: {
                        backgroundColor: obj.status === 'completed' ? '#d1fae5' : obj.status === 'failed' ? '#fee2e2' : obj.status === 'downloading' ? '#fef3c7' : '#f3f4f6',
                        color: obj.status === 'completed' ? '#065f46' : obj.status === 'failed' ? '#991b1b' : obj.status === 'downloading' ? '#92400e' : '#374151'
                      }
                    }, obj.status || '')
                  ]),
                  // URL
                  obj.url ? h('div', { class: 'text-xs' }, [
                    h('span', { class: 'text-gray-500' }, 'URL：'),
                    h('span', { class: 'text-gray-700 break-all' }, obj.url)
                  ]) : null,
                  // Save path
                  obj.save_path ? h('div', { class: 'text-xs' }, [
                    h('span', { class: 'text-gray-500' }, '文件路径：'),
                    h('span', { class: 'text-gray-700 break-all' }, obj.save_path)
                  ]) : null,
                  // Date
                  dateVal ? h('div', { class: 'text-xs' }, [
                    h('span', { class: 'text-gray-500' }, '日期：'),
                    h('span', { class: 'text-gray-700' }, dateVal)
                  ]) : null,
                  // Duration
                  duration ? h('div', { class: 'text-xs' }, [
                    h('span', { class: 'text-gray-500' }, '时长：'),
                    h('span', { class: 'text-gray-700' }, duration)
                  ]) : null,
                  // Tags (editable)
                  h('div', { class: '' }, [
                    h('div', { class: 'flex flex-wrap gap-1 items-center' }, [
                      // Existing tags
                      tagState.items.map(function (tag, i) {
                        return h('span', { class: 'text-xs bg-gray-100 text-gray-600 px-2 py-0.5 rounded inline-flex items-center gap-1' }, [
                          h('span', null, '#' + tag),
                          h('button', {
                            class: 'text-red-400 hover:text-red-600 text-xs ml-1 cursor-pointer',
                            on: { click: function (e) {
                              e.stopPropagation()
                              tagState.items.splice(i, 1)
                              saveTags()
                            }}
                          }, '✕')
                        ])
                      }),
                      // Add button
                      !tagState.adding ? h('button', {
                        class: 'text-blue-500 hover:text-blue-700 text-xs ml-1 cursor-pointer',
                        on: { click: function (e) {
                          e.stopPropagation()
                          tagState.adding = true
                          tagState.inputValue = ''
                        }}
                      }, '+') : null,
                      // Input field
                      tagState.adding ? h('div', { class: 'inline-flex items-center gap-1' }, [
                        h('input', {
                          attrs: { type: 'text', placeholder: '添加标签', class: 'border rounded px-1 py-0.5 text-xs w-24' },
                          domProps: { value: tagState.inputValue },
                          on: {
                            input: function (e) { tagState.inputValue = e.target.value },
                            keydown: function (e) {
                              if (e.key === 'Enter') {
                                e.preventDefault()
                                var val = tagState.inputValue.trim()
                                if (val && tagState.items.indexOf(val) < 0) {
                                  tagState.items.push(val)
                                  saveTags()
                                }
                                tagState.adding = false
                                tagState.inputValue = ''
                              } else if (e.key === 'Escape') {
                                tagState.adding = false
                                tagState.inputValue = ''
                              }
                            }
                          }
                        }),
                        h('button', {
                          class: 'text-green-500 hover:text-green-700 text-xs ml-1 cursor-pointer',
                          on: { click: function (e) {
                            e.stopPropagation()
                            var val = tagState.inputValue.trim()
                            if (val && tagState.items.indexOf(val) < 0) {
                              tagState.items.push(val)
                              saveTags()
                            }
                            tagState.adding = false
                            tagState.inputValue = ''
                          }}
                        }, '✓'),
                        h('button', {
                          class: 'text-gray-400 hover:text-gray-600 text-xs ml-1 cursor-pointer',
                          on: { click: function (e) {
                            e.stopPropagation()
                            tagState.adding = false
                            tagState.inputValue = ''
                          }}
                        }, '✕')
                      ]) : null
                    ])
                  ]),
                  // Metadata
                  obj.metadata ? h('div', { class: 'border-t pt-3 mt-2' }, [
                    h('p', { class: 'text-xs font-semibold text-gray-500 mb-1' }, '元数据'),
                    Object.keys(obj.metadata).filter(function (k) { return !['title', 'date', 'duration'].includes(k) }).map(function (k) {
                      return h('div', { class: 'text-xs text-gray-600' }, k + ': ' + (obj.metadata[k] || ''))
                    })
                  ]) : null,
                  // Extra
                  obj.extra ? h('div', { class: 'border-t pt-3 mt-2' }, [
                    h('p', { class: 'text-xs font-semibold text-gray-500 mb-1' }, '扩展信息'),
                    Object.keys(obj.extra).filter(function (k) { return k !== 'tags' && k !== 'images' && k !== 'files' }).map(function (k) {
                      var v = obj.extra[k]
                      if (typeof v === 'object') v = JSON.stringify(v, null, 2)
                      return h('div', { class: 'text-xs text-gray-600 break-all' }, k + ': ' + v)
                    })
                  ]) : null,
                ]),
                // Footer
                h('div', { class: 'p-3 border-t bg-gray-50 flex justify-between items-center' }, [
                  h('div', { class: 'flex gap-2' }, [
                    fileUrl ? h('a', {
                      attrs: { href: fileUrl, target: '_blank', rel: 'noopener noreferrer' },
                      class: 'px-3 py-1.5 rounded bg-blue-600 text-white text-sm hover:bg-blue-700'
                    }, '打开文件') : null,
                    obj.metadata && obj.metadata.page_url ? h('a', {
                      attrs: { href: obj.metadata.page_url, target: '_blank', rel: 'noopener noreferrer' },
                      class: 'px-3 py-1.5 rounded bg-white border text-sm hover:bg-gray-100'
                    }, '打开原页面') : null,
                  ]),
                  h('button', {
                    class: 'px-3 py-1.5 rounded bg-white border text-sm hover:bg-gray-100',
                    on: { click: function (e) { e.stopPropagation(); onClose() } }
                  }, '关闭'),
                ]),
              ])
            ])
          }
        }).mount(container)
      }
    }
  })

  // Register helpers module
  if (typeof AppHelpers !== 'undefined') AppHelpers.register(app)

  // Register video player module
  AppVideoPlayer.register(app)

  // Register task list and dashboard modules
  if (typeof AppTaskList !== 'undefined') AppTaskList.register(app)
  if (typeof AppDashboard !== 'undefined') AppDashboard.register(app)

  // Optional aggregate/config/download view modules (loaded from separate files)
  if (typeof AppAggregateView !== 'undefined') AppAggregateView.register(app)
  if (typeof AppConfigPanel !== 'undefined') AppConfigPanel.register(app)
  if (typeof AppDownloadView !== 'undefined') AppDownloadView.register(app)

  app.mount('#app')
})()