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
          try {
            var container = document.createElement('div')
            document.body.appendChild(container)
            var onClose = function () {
              try { if (container.parentNode) container.parentNode.removeChild(container) } catch (e) {}
            }
            var result = handler.renderViewer(obj, onClose)
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
            UiHelpers.showToast('save tags failed: ' + e.message, 'error')
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
        hTitle.textContent = title || 'object info'
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
        var sl = document.createElement('span'); sl.style.cssText = 'font-size:12px;color:#6b7280'; sl.textContent = 'status: '
        sr.appendChild(sl)
        var sb = document.createElement('span')
        sb.style.cssText = 'padding:2px 8px;font-size:12px;font-weight:600;border-radius:9999px;background:' + (obj.status === 'completed' ? '#d1fae5' : obj.status === 'failed' ? '#fee2e2' : obj.status === 'downloading' ? '#fef3c7' : '#f3f4f6') + ';color:' + (obj.status === 'completed' ? '#065f46' : obj.status === 'failed' ? '#991b1b' : obj.status === 'downloading' ? '#92400e' : '#374151')
        sb.textContent = obj.status || ''
        sr.appendChild(sb); body.appendChild(sr)
        addRow('URL', obj.url)
        addRow('path', obj.save_path)
        addRow('date', dateVal)
        addRow('duration', duration)
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
          var mt = document.createElement('p'); mt.style.cssText = 'font-size:12px;font-weight:600;color:#6b7280;margin:0 0 4px'; mt.textContent = 'metadata'
          md.appendChild(mt)
          Object.keys(obj.metadata).filter(function(k){return ['title','date','duration'].indexOf(k)<0}).forEach(function(k){
            var r=document.createElement('div');r.style.cssText='font-size:12px;color:#4b5563';r.textContent=k+': '+(obj.metadata[k]||'');md.appendChild(r)
          })
          body.appendChild(md)
        }
        if (obj.extra) {
          var ed = document.createElement('div'); ed.style.cssText = 'border-top:1px solid #e5e7eb;padding-top:12px;margin-top:8px'
          var et = document.createElement('p'); et.style.cssText = 'font-size:12px;font-weight:600;color:#6b7280;margin:0 0 4px'; et.textContent = 'extra'
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
          var of = document.createElement('a'); of.href=fileUrl; of.target='_blank'; of.rel='noopener noreferrer'; of.style.cssText='padding:6px 12px;border-radius:6px;background:#2563eb;color:#fff;text-decoration:none;font-size:14px;display:inline-block'; of.textContent='open file'; fl.appendChild(of)
        }
        if (obj.metadata&&obj.metadata.page_url) {
          var op = document.createElement('a'); op.href=obj.metadata.page_url; op.target='_blank'; op.rel='noopener noreferrer'; op.style.cssText='padding:6px 12px;border-radius:6px;background:#fff;border:1px solid #d1d5db;text-decoration:none;cursor:pointer;font-size:14px;color:#374151;display:inline-block'; op.textContent='open page'; fl.appendChild(op)
        }
        footer.appendChild(fl)
        var cb2 = document.createElement('button'); cb2.style.cssText='padding:6px 12px;border-radius:6px;background:#fff;border:1px solid #d1d5db;cursor:pointer;font-size:14px;color:#374151'; cb2.textContent='close'; cb2.onclick=function(e){e.stopPropagation();onClose()}; footer.appendChild(cb2)
        panel.appendChild(footer)
        overlay.addEventListener('click', function(e){if(e.target===overlay)onClose()})
        function keyHandler(e){if(e.key==='Escape')onClose()}
        document.addEventListener('keydown',keyHandler)
        var origOnClose=onClose; onClose=function(){document.removeEventListener('keydown',keyHandler);document.body.style.overflow='';if(origOnClose)origOnClose()}
        document.body.appendChild(overlay); document.body.style.overflow='hidden'
      },

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