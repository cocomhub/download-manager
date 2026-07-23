// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * Helper methods for object display, URL routing, etc.
 * Registered as Vue methods on the app.
 * Depends on: AppAPI
 */
;(function () {
  'use strict'

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
            if (d && typeof d === 'object') self.runtime = d
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
        pathToUrl: function (path) {
          return '/files/' + (path || '').replace(/\\/g, '/')
        },
        getFileUrl: function (obj) {
          if (obj && obj.save_path) return this.pathToUrl(obj.save_path)
          return ''
        },
        getScopedTaskInfo: function (obj) {
          return { taskId: obj.task_id || '', taskType: (obj.metadata && obj.metadata.task_type) || '' }
        },

        // ---- Type detection ----
        isVideo: function (obj) {
          if (!obj || !obj.save_path) return false
          var ext = obj.save_path.split('.').pop()
          return ext === 'mp4' || ext === 'webm' || ext === 'mkv'
        },
        getVideoUrl: function (obj) {
          if (!obj) return ''
          if (obj.save_path) return this.pathToUrl(obj.save_path)
          return obj.url || ''
        },

        // Vikacg helpers
        isVikacg: function (obj) {
          var u = (obj && obj.metadata && obj.metadata.page_url) || (obj && obj.url) || ''
          return u.indexOf('vikacg.com') >= 0
        },
        getVikacgImages: function (obj) {
          var imgs = []
          if (obj && obj.extra && Array.isArray(obj.extra.files)) {
            obj.extra.files.forEach(function (f) {
              if (f.type === 'image' && f.path) imgs.push(this.pathToUrl(f.path))
            }.bind(this))
          }
          if (imgs.length === 0 && obj && obj.extra && Array.isArray(obj.extra.images)) {
            obj.extra.images.forEach(function (u) { if (typeof u === 'string' && u) imgs.push(u) })
          }
          return imgs
        },
        getVikacgLinks: function (obj) {
          var links = []
          var base = (obj && obj.metadata && obj.metadata.page_url) || ''
          if (obj && obj.extra && Array.isArray(obj.extra.links)) {
            obj.extra.links.forEach(function (l) {
              var href = (l && l.href) || ''
              var text = (l && l.text) || href
              if (!href) return
              var abs = href
              try { abs = new URL(href, base).toString() } catch (e) {}
              links.push({ text: text, href: abs })
            })
          }
          return links
        },
        getVikacgExcerpt: function (obj) {
          var s = (obj && obj.extra && obj.extra.content_text) || ''
          if (!s) return ''
          var t = s.replace(/\s+/g, ' ').trim()
          return t.length > 200 ? t.slice(0, 200) + '...' : t
        },
        getVikacgHtml: function (obj) {
          var s = (obj && obj.extra && obj.extra.content_html) || ''
          if (typeof s !== 'string') return ''
          return s.trim()
        },
        openVikacg: function (obj) {
          this.vikacgModalObj = obj
          this.vikacgActiveImgIdx = 0
          this.showVikacgModal = true
        },
        closeVikacg: function () {
          this.showVikacgModal = false
          this.vikacgModalObj = null
          this.vikacgActiveImgIdx = 0
        },

        // Hanime helpers
        isHanime: function (obj) {
          var u = (obj && obj.metadata && obj.metadata.page_url) || (obj && obj.url) || ''
          return u.indexOf('hanime1.me') >= 0
        },
        getHanimeTitle: function (obj) {
          return (obj && obj.metadata && obj.metadata.title) || ''
        },
        getHanimeTags: function (obj) {
          if (obj && obj.extra && Array.isArray(obj.extra.tags)) return obj.extra.tags
          return []
        },
        getHanimeArtist: function (obj) {
          return (obj && obj.metadata && obj.metadata.artist) || ''
        },
        getHanimeDescription: function (obj) {
          return (obj && obj.extra && obj.extra.content_text) || ''
        },
        getHanimeOriginLink: function (obj) {
          return (obj && obj.metadata && obj.metadata.page_url) || ''
        },
        getHanimeCover: function (obj) {
          var srcs = this.getHanimePoster(obj)
          return srcs.length ? srcs[0] : ''
        },
        getHanimePoster: function (obj) {
          if (obj && obj.extra && Array.isArray(obj.extra.images)) return obj.extra.images
          if (obj && obj.extra && Array.isArray(obj.extra.files)) {
            var imgs = []
            obj.extra.files.forEach(function (f) {
              if (f.type === 'image' && f.path) imgs.push(this.pathToUrl(f.path))
            }.bind(this))
            return imgs
          }
          return []
        },
        getHanimeVideoURL: function (obj) {
          if (obj && obj.extra && obj.extra.video_url) return obj.extra.video_url
          if (obj && obj.save_path && this.isVideo(obj)) return this.pathToUrl(obj.save_path)
          return ''
        },
        canPlayHanimeVideo: function (obj) {
          return !!this.getHanimeVideoURL(obj)
        },
        getHanimeDetails: function (obj) {
          return (obj && obj.metadata && obj.metadata.detail) || ''
        },
        getHanimeDate: function (obj) {
          return (obj && obj.metadata && obj.metadata.date) || ''
        },
        getHanimePlaylist: function (obj) {
          if (obj && obj.extra && Array.isArray(obj.extra.playlist)) return obj.extra.playlist
          if (obj && obj.extra && Array.isArray(obj.extra.links)) return obj.extra.links
          return []
        },
        getHanimeGenres: function (obj) {
          if (obj && obj.metadata && obj.metadata.genre) return [obj.metadata.genre]
          return []
        },
        canOpenVikacg: function (obj) { return this.isVikacg(obj) },
        canOpenHanime: function (obj) { return this.isHanime(obj) },
        openHanime: function (obj) {
          this.hanimeModalObj = obj
          this.hanimeActiveCoverIdx = 0
          this.hanimeActivePosterIdx = 0
          this.hanimeVideoError = false
          this.showHanimeModal = true
        },
        closeHanime: function () {
          this.showHanimeModal = false
          this.hanimeModalObj = null
          this.hanimeVideoError = false
        },

        // ---- Task type badge / display name ----
        getTaskTypeBadge: function (task) {
          var known = {
            'tktube': 'TKTube',
            'hanime': 'Hanime',
            'vikacg': 'VikACG',
            'url_list': 'URL',
            'mxs': '漫小肆'
          }
          return known[task && task.type] || (task && task.type) || '?'
        },
        getTaskDisplayName: function (task) {
          if (!task) return ''
          // Try task summary display_name or extra label first
          if (task.display_name) return task.display_name
          // Fall back to id
          return task.id || ''
        },

        // ---- External Task UI (custom JS/CSS registered by task types) ----
        isCustomUI: function (obj) {
          return !!(
            obj && obj.metadata && obj.metadata.task_type &&
            window.__dm_uiBridge && window.__dm_uiBridge.hasPlugin(obj.metadata.task_type)
          )
        },
        getCustomUILabel: function (obj) {
          var type = obj && obj.metadata && obj.metadata.task_type
          return (type && window.__dm_uiBridge && window.__dm_uiBridge.getLabel(type)) || '浏览'
        },
        openCustomUI: function (obj) {
          var type = obj && obj.metadata && obj.metadata.task_type
          if (type && window.__dm_uiBridge) window.__dm_uiBridge.open(type, obj)
        },

        // Object variant / content group helpers
        getObjectVariantPriority: function (obj) {
          if (obj && obj.extra) {
            if (obj.extra.variant_priority !== undefined) return obj.extra.variant_priority
            if (obj.extra.priority !== undefined) return obj.extra.priority
          }
          if (obj && obj.metadata && obj.metadata.resolution) {
            var r = obj.metadata.resolution
            if (/1080/.test(r)) return 30
            if (/720/.test(r)) return 20
            if (/480/.test(r)) return 10
          }
          return 0
        },
        isGroupRepresentative: function (obj) { return !!(obj && obj.extra && (obj.extra.group_rep || obj.extra.is_representative)) },
        isGroupCancelTarget: function (obj) {
          return obj && obj.status === 'pending' && !this.isGroupRepresentative(obj) && (obj.extra && obj.extra.group_size)
        },
        getObjectVariantLabel: function (obj) {
          if (obj && obj.metadata && obj.metadata.resolution) return obj.metadata.resolution
          if (obj && obj.metadata && obj.metadata.variant_label) return obj.metadata.variant_label
          return 'standard'
        },
        getCoverImage: function (obj) {
          if (this.isVikacg(obj)) {
            var images = this.getVikacgImages(obj)
            return images.length ? images[0] : ''
          }
          if (this.isHanime(obj)) return this.getHanimeCover(obj)
          if (obj && obj.extra) {
            if (obj.extra.thumb_url) return obj.extra.thumb_url
            if (obj.extra.preview_url) return obj.extra.preview_url
            if (obj.extra.cover_url) return obj.extra.cover_url
            if (Array.isArray(obj.extra.images) && obj.extra.images.length) return obj.extra.images[0]
            if (Array.isArray(obj.extra.files)) {
              for (var i = 0; i < obj.extra.files.length; i++) {
                var f = obj.extra.files[i]
                if (f.type === 'image' && f.path) return this.pathToUrl(f.path)
              }
            }
          }
          return ''
        },
        getPreviewUrl: function (obj) {
          if (obj && obj.extra && obj.extra.preview_url) return obj.extra.preview_url
          return ''
        },

        // ---- Meta helpers (non-standard keys from vikacg, etc.) ----
        getVariantTagText: function (obj) {
          return this.getObjectVariantLabel(obj)
        },
        getObjectByUrl: function (url) {
          if (this.selectedTask && this.selectedTask.objects) {
            return this.selectedTask.objects.find(function (o) { return o.url === url })
          }
          return null
        },

        // ---- Generic multi-source tag aggregator ----
        getAllTags: function (obj) {
          var vals = []
          function pushVal(x) {
            if (typeof x === 'string') { vals.push(x); return }
            if (Array.isArray(x)) {
              x.forEach(function (v) { if (typeof v === 'string' && v.trim()) vals.push(v.trim()) })
              return
            }
            if (typeof x === 'object') {
              var xs = String(x)
              var parts = xs.split(/[,，、;；\s]+/)
              parts.forEach(function (v) { if (v.trim()) vals.push(v.trim()) })
            }
          }
          if (obj && obj.extra) {
            if (obj.extra.genre) pushVal(obj.extra.genre)
            if (obj.extra.genres) pushVal(obj.extra.genres)
            if (obj.extra.categories) pushVal(obj.extra.categories)
            if (obj.extra.tags) pushVal(obj.extra.tags)
          }
          if (obj && obj.metadata) {
            if (obj.metadata.genre) pushVal(obj.metadata.genre)
            if (obj.metadata.genres) pushVal(obj.metadata.genres)
            if (obj.metadata.categories) pushVal(obj.metadata.categories)
            if (obj.metadata.tags) pushVal(obj.metadata.tags)
          }
          var out = [], set = {}
          vals.forEach(function (s) {
            var t = (s || '').toString().trim()
            if (t && !set[t]) { set[t] = true; out.push(t) }
          })
          return out
        },

        // SSE
        initSSE: function () {
          if (this.eventSource) this.eventSource.close()
          var self = this
          this.eventSource = new EventSource('/api/events')
          this.eventSource.onmessage = function (event) {
            try {
              var data = JSON.parse(event.data)
              self.handleEvent(data)
            } catch (e) { console.error('SSE Parse Error', e) }
          }
          this.eventSource.onerror = function () {
            console.error('SSE Error')
            self.showToast('Connection lost. Reconnecting...', 'error')
          }
          this.eventSource.onopen = function () {
            // SSE 重连成功后刷新任务列表，解决断连后数据陈旧的问题
            self.fetchTasks()
          }
        },
        handleEvent: function (event) {
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
              if (obj.status === 'completed') this.showToast('Download completed: ' + this.getTitle(obj), 'success')
              else if (obj.status === 'failed') this.showToast('Download failed: ' + this.getTitle(obj), 'error')
              else if (obj.status === 'cancelled') this.showToast('已取消: ' + this.getTitle(obj), 'info')
            }
            if (this.selectedTask && this.selectedTask.objects) {
              var currentObj = this.selectedTask.objects.find(function (o) { return o.url === obj.url })
              if (currentObj) { currentObj.status = obj.status; currentObj.progress = obj.progress; if (obj.metadata) currentObj.metadata = obj.metadata }
            }
            if (this.viewMode === 'tktube' && Array.isArray(this.tktubeObjects) && this.tktubeObjects.length > 0) {
              var objType = (obj && typeof obj.type === 'string') ? obj.type : null
              if (!objType) {
                var task = this.tasks.find(function (t) { return t.id === obj.task_id })
                if (task && typeof task.type === 'string') objType = task.type
              }
              if (this.selectedType !== 'all' && objType && objType !== this.selectedType) return
              var idxAgg = this.tktubeObjects.findIndex(function (o) { return o.url === obj.url && o.task_id === obj.task_id })
              if (idxAgg >= 0) {
                var existing = this.tktubeObjects[idxAgg]
                existing.status = obj.status
                existing.progress = obj.progress
                if (obj.metadata) existing.metadata = obj.metadata
                this.tktubeObjects.splice(idxAgg, 1, existing)
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
                if (this.viewMode === 'tktube' && Array.isArray(this.tktubeObjects) && this.tktubeObjects.length > 0) {
                  var idxAgg = this.tktubeObjects.findIndex(function (o) { return o.url === item.url && o.task_id === item.task_id })
                  if (idxAgg >= 0) { this.tktubeObjects[idxAgg].progress = item.progress }
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
          this.showAddTaskModal = true
        },
        saveNewTask: function () {
          var payload = {
            id: this.newTask.id,
            type: this.newTask.type,
            save_dir: this.newTask.save_dir,
            storage: { type: this.newTask.storage_type }
          }
          if (this.newTask.storage_type === 'file' && this.newTask.storage_config.path) {
            payload.storage.path = this.newTask.storage_config.path
          }
          if (this.newTask.storage_type === 'mongo') {
            payload.storage = { type: 'mongo' }
            if (this.newTask.storage_config.source) payload.storage.source = this.newTask.storage_config.source
            if (this.newTask.storage_config.database) payload.storage.database = this.newTask.storage_config.database
            if (this.newTask.storage_config.collection) payload.storage.collection = this.newTask.storage_config.collection
          }
          if (this.newTask.type === 'url_list') {
            payload.urls_text = this.newTask.urls_text
          }
          if (this.newTask.type === 'tktube') {
            if (this.newTask.keyword) payload.keyword = this.newTask.keyword
            if (this.newTask.subtype) payload.subtype = this.newTask.subtype
            if (this.newTask.max_concurrent) payload.max_concurrent = this.newTask.max_concurrent
            if (this.newTask.refresh_interval) payload.refresh_interval = this.newTask.refresh_interval
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
          this.showConfigModal = true
          var self = this
          AppAPI.serverConfig().then(function (data) {
            self.configForm = data || {}
          }).catch(function () {})
        },
        saveConfig: function () {
          var self = this
          AppAPI.put('/api/config/server', this.configForm).then(function (res) {
            if (!res.ok) throw new Error('保存失败')
            self.showToast('配置已保存', 'success')
            self.showConfigModal = false
            self.initUiDefaults()
          }).catch(function (e) { self.showToast('保存失败: ' + e.message, 'error') })
        },
        openConfigHistory: function () {
          this.showConfigHistoryModal = true
        },

        // ---- Card / group modal ----
        handleCardClick: function (obj) {
          if (!obj) return
          if (obj.status === 'completed' && this.isVideo(obj)) {
            this.playVideo(obj)
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

        // ---- Tktube / Aggregate view ----
        openTktubeAggregate: function () {
          this.viewMode = 'tktube'
          this.fetchAggregateByType(this.selectedType || 'all')
        },
        fetchAggregateByType: function (type) {
          if (this.tktubeLoading) return
          this.tktubeLoading = true
          var self = this
          var sortBy = this.tktubeSortBy || ''
          var groupBy = this.tktubeGroupBy || false
          var url = '/api/tasks/objects?type=' + encodeURIComponent(type || 'all')
          if (sortBy) url += '&sort=' + encodeURIComponent(sortBy)
          if (groupBy) url += '&group=' + encodeURIComponent(groupBy)
          AppAPI.get(url).then(function (data) {
            self.tktubeObjects = (data && data.objects) || (Array.isArray(data) ? data : [])
            self.tktubePagination.total = (data && data.total) || self.tktubeObjects.length
            self.showTktubeView = true
          }).catch(function () {
            self.showToast('加载聚合视图失败', 'error')
          }).finally(function () {
            self.tktubeLoading = false
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
        changeTktubePage: function (page) {
          this.tktubePagination.page = page
          this.fetchAggregateByType(this.selectedType || 'all')
        },
        changeTktubeLimit: function () {
          this.tktubePagination.page = 1
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
        }
      }})
    }
  }

  // ---- External Task UI Bridge ----
  window.__dm_uiBridge = (function () {
    var plugins = {}
    var taskViews = {}
    return {
      register: function (taskType, handler) {
        handler.label = handler.label || '浏览'
        plugins[taskType] = handler
      },
      hasPlugin: function (taskType) { return !!plugins[taskType] },
      getLabel: function (taskType) { var p = plugins[taskType]; return p ? p.label : '浏览' },
      open: function (taskType, obj) {
        var p = plugins[taskType]
        if (p && p.open) p.open(obj)
      },
      // Task-level view: replaces the default grid for a task type
      registerTaskView: function (taskType, handler) {
        taskViews[taskType] = handler
      },
      getTaskView: function (taskType) {
        return taskViews[taskType] || null
      }
    }
  })()
})()
