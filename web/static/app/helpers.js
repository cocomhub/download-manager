// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * Helper methods for object display, URL routing, etc.
 * Registered as Vue methods on the app.
 * Depends on: AppAPI
 */
;(function () {
  'use strict'

  // ---- External Task UI Bridge ----
  // Each task type can register:
  //   - label: button text ("播放", "浏览")
  //   - open(obj): called when object is clicked
  //   - renderCard(obj, containerEl): optional — full card DOM rendering
  //   - registerTaskView: full task detail view (tktube style)
  window.__dm_uiBridge = (function () {
    var plugins = {}
    var taskViews = {}
    var cardRenderers = {}
    return {
      register: function (taskType, handler) {
        handler.label = handler.label || '浏览'
        plugins[taskType] = handler
        if (typeof handler.renderCard === 'function') {
          cardRenderers[taskType] = handler.renderCard
        }
      },
      hasPlugin: function (taskType) { return !!plugins[taskType] },
      hasCardRenderer: function (taskType) { return !!cardRenderers[taskType] },
      getLabel: function (taskType) { var p = plugins[taskType]; return p ? p.label : '浏览' },
      open: function (taskType, obj) {
        var p = plugins[taskType]
        if (p && p.open) p.open(obj)
      },
      renderCard: function (taskType, obj, el) {
        var r = cardRenderers[taskType]
        if (r) r(obj, el)
      },
      registerTaskView: function (taskType, handler) {
        taskViews[taskType] = handler
      },
      getTaskView: function (taskType) {
        return taskViews[taskType] || null
      }
    }
  })()

  window.AppHelpers = {
    register: function (app) {
      app.mixin({methods: {
        // URL / Type routing
        initTypeFromURL: function () {
          try {
            var cand = (typeof window.__dm_readTypeFromURL === 'function') ? window.__dm_readTypeFromURL() : null
            var ids = (typeof getAvailableTaskTypes === 'function') ? (getAvailableTaskTypes() || []).map(function (t) { return t.id }) : []
            this.selectedType = (cand && ids.indexOf(cand) >= 0) ? cand : 'all'
          } catch (e) { this.selectedType = 'all' }
        },
        initRuntime: function () {
          var self = this
          AppAPI.runtime().then(function (d) {
            if (d && typeof d === 'object') {
              self.runtime = d
              // Expose download root globally for plugin JS files
              if (d.download_root) window.__dm_downloadRoot = d.download_root.replace(/\\/g, '/')
              // Initialize frontend logger with server-configured log level
              if (d.log_level && typeof Log !== 'undefined' && Log.setLevel) {
                Log.setLevel(d.log_level)
              }
            }
          }).catch(function () {})
        },

        // Hover
        setHoverObj: function (obj) {
          var self = this
          if (this.hoverTimer) clearTimeout(this.hoverTimer)
          this.hoverTimer = setTimeout(function () { self.hoverObj = obj }, 600)
        },
        clearHoverObj: function () {
          if (this.hoverTimer) clearTimeout(this.hoverTimer)
          this.hoverObj = null
        },

        // Object display helpers
        getTitle: function (obj) {
          return (obj && obj.metadata && obj.metadata.title) || ''
        },
        getDate: function (obj) {
          return (obj && obj.metadata && obj.metadata.date) || ''
        },
        getDuration: function (obj) {
          return (obj && obj.metadata && obj.metadata.duration) || ''
        },
        getTags: function (obj) {
          if (obj && obj.extra && Array.isArray(obj.extra.tags)) return obj.extra.tags
          if (obj && obj.extra && typeof obj.extra.tags === 'string') return [obj.extra.tags]
          return []
        },
        // Touch detection — method instead of direct window access in template
        // to avoid Vue `with(this)` scope issues.
        isTouchDevice: function () {
          try { return 'ontouchstart' in window } catch (e) { return false }
        },
        pathToUrl: function (path) {
          if (!path) return ''
          // Encode special URL characters (? # [ ] etc.) that would break path parsing
          var normalized = path.replace(/\\/g, '/')
          // Strip download root prefix if the path is absolute (e.g. /opt/.../downloads/hanime/... → hanime/...)
          var downloadRoot = this.runtime && this.runtime.download_root
          if (downloadRoot && normalized.indexOf(downloadRoot.replace(/\\/g, '/')) === 0) {
            normalized = normalized.slice(downloadRoot.replace(/\\/g, '/').length)
          }
          // Strip leading slash for relative path
          normalized = normalized.replace(/^\//, '')
          return '/files/' + normalized.split('/').map(function (seg) {
            return encodeURIComponent(seg)
          }).join('/')
        },
        getFileUrl: function (obj) {
          if (obj && obj.save_path) return this.pathToUrl(obj.save_path)
          return ''
        },
        getTaskDisplayName: function (task) {
          if (!task) return ''
          if (task.display_name) return task.display_name
          if (task.name && task.name !== task.id) return task.name
          return task.id
        },
        getTaskTypeBadge: function (task) {
          if (!task || !task.type) return ''
          // 优先从 TaskUI 注册表获取标签
          var handler = TaskUI.get(task.type)
          if (handler && handler.label) return handler.label
          // 回退硬编码映射
          var known = {
            'tktube': 'TKTube',
            'hanime': 'Hanime',
            'vikacg': 'VikACG',
            'url_list': 'URL',
            'mxs': '漫小肆'
          }
          return known[task.type] || (task.type.length > 12 ? task.type.slice(0, 12) + '…' : task.type)
        },

        // Group helpers
        getScopedTaskInfo: function (obj) {
          if (!obj) return { taskId: '', taskType: '' }
          return { taskId: obj.task_id || '', taskType: (obj.metadata && obj.metadata.task_type) || '' }
        },
        getObjectVariantPriority: function (obj) {
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
        },
        isGroupRepresentative: function (obj) {
          return !!(obj && obj.extra && (obj.extra.group_rep || obj.extra.is_representative))
        },
        isGroupCancelTarget: function (obj) {
          return obj && obj.status === 'pending' && !this.isGroupRepresentative(obj) &&
            (obj.extra && obj.extra.group_size)
        },
        getObjectVariantLabel: function (obj) {
          if (obj && obj.metadata && obj.metadata.resolution) return obj.metadata.resolution
          if (obj && obj.metadata && obj.metadata.variant_label) return obj.metadata.variant_label
          return 'standard'
        },
        metadataContentGroup: function (obj) {
          return (obj && obj.metadata && obj.metadata.content_group) || ''
        },

        // SSE
        initSSE: function () {
          if (this.eventSource) this.eventSource.close()
          Log.debug('initSSE connecting')
          var self = this
          this.eventSource = new EventSource('/api/events')
          this.eventSource.onmessage = function (event) {
            try {
              var data = JSON.parse(event.data)
              Log.trace('SSE event', { type: data.type, taskId: data.payload && data.payload.task_id })
              self.handleEvent(data)
            } catch (e) { Log.error('SSE Parse Error', { error: e.message }) }
          }
          this.eventSource.onerror = function () {
            Log.warn('SSE connection error, reconnecting...')
            self.showToast('Connection lost. Reconnecting...', 'error')
          }
          this.eventSource.onopen = function () {
            Log.debug('SSE connected')
            self.fetchTasks()
          }
        },
        handleEvent: function (event) {
          var self = this
          Log.trace('handleEvent', { type: event.type, taskId: event.payload && event.payload.task_id, url: event.payload && event.payload.url })
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
              if (obj.status === 'completed') this.showToast('Download completed: ' + this.getTitle(obj), 'success')
              else if (obj.status === 'failed') this.showToast('Download failed: ' + this.getTitle(obj), 'error')
              else if (obj.status === 'cancelled') this.showToast('已取消: ' + this.getTitle(obj), 'info')
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
                if (aidx >= 0) {
                  this.activeDownloads[aidx].progress = item.progress
                }
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

        showToast: function (message, type) {
          type = type || 'info'
          var toast = document.createElement('div')
          toast.className = 'fixed bottom-4 left-4 px-4 py-2 rounded shadow-lg text-white text-sm z-50 transition-opacity duration-300 ' + (type === 'error' ? 'bg-red-500' : 'bg-green-500')
          toast.textContent = message
          document.body.appendChild(toast)
          setTimeout(function () {
            toast.style.opacity = '0'
            setTimeout(function () { toast.remove() }, 300)
          }, 3000)
        },

        // UI defaults
        initUiDefaults: function () {
          var self = this
          AppAPI.serverConfig().then(function (svr) {
            var svrUi = (svr && svr.ui_defaults) || {}
            var localUi = {}
            try { localUi = JSON.parse(localStorage.getItem('dm_ui_defaults') || '{}') } catch (e) {}
            var merged = Object.assign({}, svrUi, localUi)
            self.uiDefaults = merged
            if (merged.default_save_dir) self.newTask.save_dir = merged.default_save_dir
            if (typeof merged.diff_side_by_side === 'boolean') self.diffOptions.side_by_side = merged.diff_side_by_side
            if (typeof merged.diff_ignore_ws === 'boolean') self.diffOptions.ignore_ws = merged.diff_ignore_ws
            if (typeof merged.diff_ignore_comment === 'boolean') self.diffOptions.ignore_comments = merged.diff_ignore_comment
          }).catch(function () {})
        },

        // ---- Create task modal ----
        openAddTask: function ($event) {
          if ($event) $event.preventDefault()
          Log.debug('openAddTask', { currentType: this.newTask.type })
          this.showAddTaskModal = true
        },
        saveNewTask: function () {
          var payload = {
            id: this.newTask.id,
            type: this.newTask.type,
            save_dir: this.newTask.save_dir,
            storage: { type: this.newTask.storage_type },
            extra: {}
          }
          Log.info('saveNewTask', { type: payload.type, id: payload.id, storage: payload.storage.type })
          if (this.newTask.storage_type === 'file' && this.newTask.storage_config.path) {
            payload.storage.path = this.newTask.storage_config.path
          }
          if (this.newTask.storage_type === 'mongo') {
            if (this.newTask.storage_config.source) payload.storage.source = this.newTask.storage_config.source
            if (this.newTask.storage_config.database) payload.storage.database = this.newTask.storage_config.database
            if (this.newTask.storage_config.collection) payload.storage.collection = this.newTask.storage_config.collection
          }
          // 通过 task 插件收集类型特定字段
          var handler = TaskUI.get(this.newTask.type)
          if (handler && handler.collectExtra) {
            var extra = handler.collectExtra(this.newTask)
            if (extra) {
              payload.extra = Object.assign(payload.extra, extra)
            }
          }
          if (!payload.id || !payload.type) {
            this.showToast('请填写任务ID和类型', 'error')
            return
          }
          var self = this
          AppAPI.post('/api/tasks', payload).then(function (res) {
            if (!res.ok) throw new Error('创建失败')
            self.showToast('任务创建成功', 'success')
            self.showAddTaskModal = false
            self.newTask = { id: '', type: 'url_list', save_dir: '', storage_type: 'file', storage_config: {}, urls_text: '', keyword: '', subtype: 'tag', max_concurrent: 2, refresh_interval: 300 }
            self.fetchTasks()
          }).catch(function (e) { self.showToast('创建失败: ' + e.message, 'error') })
        },

        // ---- Config panel ----
        openConfig: function () {
          Log.info('openConfig')
          this.showConfigModal = true
          var self = this
          AppAPI.serverConfig().then(function (data) {
            self.configForm = data || {}
            Log.debug('openConfig loaded', { log_level: data && data.log_level })
          }).catch(function () {})
        },
        saveConfig: function () {
          var self = this
          AppAPI.post('/api/config/server', this.configForm).then(function (res) {
            if (!res.ok) throw new Error('保存失败')
            self.showToast('配置已保存', 'success')
            self.showConfigModal = false
            self.initUiDefaults()
            // Apply frontend log level immediately
            if (self.configForm.log_level !== undefined && typeof Log !== 'undefined' && Log.setLevel) {
              Log.setLevel(self.configForm.log_level)
            }
          }).catch(function (e) { self.showToast('保存失败: ' + e.message, 'error') })
        },
        openConfigHistory: function () {
          this.showConfigHistoryModal = true
        },

        // ---- TaskUI helper predicates ----
        hasOnClick: function (obj) {
          if (!obj) return false
          var type = obj.metadata && obj.metadata.task_type
          return type && TaskUI.hasOnClick(type)
        },

        // ---- Card / group modal ----
        handleCardClick: function (obj) {
          if (!obj) return
          Log.debug('handleCardClick', { url: obj.url, status: obj.status, taskType: obj.metadata && obj.metadata.task_type })
          // cancelled + redirect_url → show info modal with link
          if (obj.status === 'cancelled' && obj.extra && obj.extra.redirect_url) {
            this.openObjectInfoViewer(obj)
            return
          }
          if (obj.status === 'completed') {
            // Delegate to task-type plugin onClick if registered
            var type = obj.metadata && obj.metadata.task_type
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
            // Default: play video or show object info modal
            if (this.isVideo(obj)) {
              this.playVideo(obj)
              return
            }
            this.openObjectInfoViewer(obj)
          }
        },
        openGroupModal: function (obj) {
          var info = this.getScopedTaskInfo(obj)
          this.groupModal.taskId = info.taskId
          this.groupModal.taskType = info.taskType
          this.showGroupModal = true
        },
        closeGroupModal: function () {
          this.showGroupModal = false
          this.groupModal = { taskId: '', taskType: '' }
        },

        // ---- Aggregate view ----
        openAggregateView: function () {
          Log.info('openAggregateView', { selectedType: this.selectedType, viewMode: this.viewMode })
          this.viewMode = 'aggregate'
          this.fetchAggregateByType(this.selectedType || 'all')
        },
        fetchAggregateByType: function (type) {
          if (this.aggLoading) return
          Log.debug('fetchAggregateByType', { type: type, page: this.aggPagination.page, sort: this.aggSortBy, groupBy: this.aggGroupBy })
          this.aggLoading = true
          var self = this
          AppAPI.aggregate({
            types: type || 'all',
            sort: this.aggSortBy || '',
            groupBy: this.aggGroupBy || false,
            page: this.aggPagination.page,
            limit: this.aggPagination.limit
          }).then(function (data) {
            self.aggObjects = (data && data.objects) || (Array.isArray(data) ? data : [])
            self.aggPagination.total = (data && data.total) || self.aggObjects.length
            self.showAggView = true
          }).catch(function () {
            self.showToast('加载聚合视图失败', 'error')
          }).finally(function () {
            self.aggLoading = false
          })
        },
        cancelAggObject: function (obj) {
          if (!obj || !obj.task_id) return
          var self = this
          AppAPI.post('/api/tasks/' + encodeURIComponent(obj.task_id) + '/object/cancel', { url: obj.url }).then(function (res) {
            if (res && !res.ok) throw new Error('取消失败')
            obj.status = 'cancelled'
            self.showToast('已取消: ' + (obj.metadata && obj.metadata.title || obj.url), 'info')
          }).catch(function (e) { self.showToast('取消失败: ' + e.message, 'error') })
        },
        changeAggPage: function (page) {
          this.aggPagination.page = page
          this.fetchAggregateByType(this.selectedType || 'all')
        },
        changeAggLimit: function () {
          this.aggPagination.page = 1
          this.fetchAggregateByType(this.selectedType || 'all')
        },

        // ---- Clipboard ----
        copyText: function (text) {
          var self = this
          if (navigator.clipboard && navigator.clipboard.writeText) {
            navigator.clipboard.writeText(text).then(function () {
              self.showToast('已复制到剪贴板', 'success')
            }).catch(function () {
              self.showToast('复制失败', 'error')
            })
          } else {
            // Fallback
            var ta = document.createElement('textarea')
            ta.value = text
            ta.style.position = 'fixed'
            ta.style.opacity = '0'
            document.body.appendChild(ta)
            ta.select()
            try { document.execCommand('copy'); self.showToast('已复制到剪贴板', 'success') }
            catch (e) { self.showToast('复制失败', 'error') }
            document.body.removeChild(ta)
          }
        },

        // ---- Custom UI / External Task Plugin methods ----
        // isCustomUI / getCustomUILabel / openCustomUI / renderPluginCards
        // 已迁移到 TaskUI 注册表，保留兼容性 shim

      }})
    }
  }
})()