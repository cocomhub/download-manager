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
        getTaskDisplayName: function (task) {
          if (!task) return ''
          if (task.name && task.name !== task.id) return task.name
          return task.id
        },
        getTaskTypeBadge: function (task) {
          if (!task || !task.type) return ''
          return task.type.length > 12 ? task.type.slice(0, 12) + '…' : task.type
        },

        // Group helpers
        getScopedTaskInfo: function (obj) {
          if (!obj) return { taskId: '', taskType: '' }
          return { taskId: obj.task_id || '', taskType: (obj.metadata && obj.metadata.task_type) || '' }
        },
        getObjectVariantPriority: function (obj) {
          if (!obj || !obj.extra) return 0
          return obj.extra.variant_priority || 0
        },
        isGroupCancelTarget: function (obj) {
          return obj && (obj.status === 'pending' || obj.status === 'failed') &&
            !this.getObjectVariantPriority(obj) === 0
        },
        metadataContentGroup: function (obj) {
          return (obj && obj.metadata && obj.metadata.content_group) || ''
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
          var u = ((obj && obj.metadata && obj.metadata.page_url) || (obj && obj.url) || '').toLowerCase()
          return u.indexOf('hanime.tv') >= 0 || u.indexOf('hanime1') >= 0
        },
        getHanimeTitle: function (obj) {
          if (obj && obj.metadata && obj.metadata.title) return obj.metadata.title
          if (obj && obj.extra && obj.extra.title) return obj.extra.title
          return this.getTitle(obj)
        },
        getHanimeTags: function (obj) {
          var tags = []
          if (obj && obj.extra && Array.isArray(obj.extra.tags)) tags.push.apply(tags, obj.extra.tags)
          if (obj && obj.metadata && Array.isArray(obj.metadata.tags)) tags.push.apply(tags, obj.metadata.tags)
          var set = {}, out = []
          tags.forEach(function (t) {
            var s = (t || '').toString().trim()
            if (s && !set[s]) { set[s] = true; out.push(s) }
          })
          return out
        },
        getHanimeArtist: function (obj) {
          if (obj && obj.extra && obj.extra.artist) return obj.extra.artist
          if (obj && obj.metadata && obj.metadata.artist) return obj.metadata.artist
          if (obj && obj.metadata && Array.isArray(obj.metadata.authors) && obj.metadata.authors.length > 0) return obj.metadata.authors.join(', ')
          return ''
        },
        getHanimeDescription: function (obj) {
          var s = ''
          if (obj && obj.extra && obj.extra.description) s = obj.extra.description
          else if (obj && obj.metadata && obj.metadata.description) s = obj.metadata.description
          else if (obj && obj.extra && obj.extra.content_text) s = obj.extra.content_text
          if (typeof s !== 'string') s = ''
          return s
        },
        getHanimeOriginLink: function (obj) {
          if (obj && obj.metadata && obj.metadata.page_url) return obj.metadata.page_url
          if (obj && obj.extra && obj.extra.origin_url) return obj.extra.origin_url
          return (obj && obj.url) || ''
        },
        getHanimeCover: function (obj) {
          var imgs = []
          var pushUrl = function (u) { if (typeof u === 'string' && u) imgs.push(u) }
          if (obj && obj.extra) {
            if (Array.isArray(obj.extra.cover_images)) obj.extra.cover_images.forEach(pushUrl)
            if (Array.isArray(obj.extra.cover_urls)) obj.extra.cover_urls.forEach(pushUrl)
            if (Array.isArray(obj.extra.covers)) obj.extra.covers.forEach(pushUrl)
            if (obj.extra.cover_url) pushUrl(obj.extra.cover_url)
            if (obj.extra.cover) pushUrl(obj.extra.cover)
            if (obj.extra.local_cover) pushUrl(this.pathToUrl(obj.extra.local_cover))
            if (Array.isArray(obj.extra.files)) {
              obj.extra.files.forEach(function (f) {
                var name = (f.name || f.path || '').toString().toLowerCase()
                if (f.type === 'image' && (name.indexOf('cover') >= 0 || name.indexOf('thumb') >= 0)) {
                  if (f.path) imgs.push(this.pathToUrl(f.path))
                }
              }.bind(this))
            }
            if (imgs.length === 0 && Array.isArray(obj.extra.images)) obj.extra.images.forEach(pushUrl)
          }
          var uniq = [], seen = {}
          imgs.forEach(function (u) { if (u && !seen[u]) { seen[u] = true; uniq.push(u) } })
          return uniq
        },
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
        },
        canPlayHanimeVideo: function (obj) {
          var u = this.getHanimeVideoURL(obj)
          if (!u) return false
          if (/\.m3u8(\?.*)?$/i.test(u)) {
            var ua = navigator.userAgent || ''
            var isSafari = /safari/i.test(ua) && !/chrome|crios|chromium|edg/i.test(ua)
            return isSafari
          }
          return true
        },
        getHanimeVideoURL: function (obj) {
          if (!obj) return ''
          var u = ''
          if (obj.metadata && obj.metadata.video_url) u = obj.metadata.video_url
          if (!u && obj.extra && obj.extra.video_url) u = obj.extra.video_url
          if (obj.status === 'completed') {
            if (this.isVideo(obj)) return this.getVideoUrl(obj)
            if (obj.extra && Array.isArray(obj.extra.files)) {
              var f = obj.extra.files.find(function (x) { return x && (x.type === 'video' || (x.path && /\.(mp4|webm|mkv|m3u8|ts)$/i.test(x.path.toString()))) })
              if (f && f.path) return this.pathToUrl(f.path)
            }
            if (obj.extra && obj.extra.local_url) return this.pathToUrl(obj.extra.local_url)
            if (obj.extra && obj.extra.file_url) return this.pathToUrl(obj.extra.file_url)
            if (obj.path) return this.pathToUrl(obj.path)
            if (obj.save_path && /\.(mp4|webm|mkv|m3u8|ts)$/i.test(obj.save_path.toString())) return this.pathToUrl(obj.save_path)
          }
          if (typeof u === 'string' && u) return u
          return ''
        },
        getHanimeDetails: function (obj) {
          var s = ''
          if (obj && obj.extra && obj.extra.details) s = obj.extra.details
          else if (obj && obj.metadata && obj.metadata.details) s = obj.metadata.details
          else if (obj && obj.metadata && obj.metadata.description) s = obj.metadata.description
          else if (obj && obj.extra && obj.extra.description) s = obj.extra.description
          if (typeof s !== 'string') s = ''
          return s.replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim()
        },
        getHanimeDate: function (obj) {
          if (obj && obj.extra && obj.extra.date) return obj.extra.date
          if (obj && obj.metadata && obj.metadata.date) return obj.metadata.date
          return ''
        },
        getHanimePlaylist: function (obj) {
          var src = (obj && obj.extra && obj.extra.playlist) || (obj && obj.metadata && obj.metadata.playlist) || []
          var items = []
          var norm = function (it) {
            if (!it) return null
            if (typeof it === 'string') {
              var s = it.trim()
              if (!s) return null
              if (/^https?:\/\//i.test(s) || s.startsWith('/')) return { title: '', thumbnail: '', url: s }
              if (obj && obj.status === 'completed') return { title: '', thumbnail: '', url: this.pathToUrl(s) }
              return { title: s, thumbnail: '', url: '' }
            }
            if (typeof it === 'object') {
              var title = it.title || it.name || it.label || ''
              var url = it.url || it.href || it.link || it.src || ''
              var thumb = it.thumbnail || it.thumb || it.image || it.cover || ''
              if (!url) {
                if (it.path && obj && obj.status === 'completed') url = this.pathToUrl(it.path)
                else if (it.local_url && obj && obj.status === 'completed') url = this.pathToUrl(it.local_url)
                else if (it.file_url && obj && obj.status === 'completed') url = this.pathToUrl(it.file_url)
              } else {
                var pLike = typeof url === 'string' && url && !/^https?:\/\//i.test(url) && !url.startsWith('/')
                if (pLike && obj && obj.status === 'completed') url = this.pathToUrl(url)
              }
              if (typeof title !== 'string') title = ''
              if (typeof url !== 'string') url = ''
              if (typeof thumb !== 'string') thumb = ''
              if (!title && !url) return null
              return { title: title, thumbnail: thumb, url: url }
            }
            return null
          }.bind(this)
          if (Array.isArray(src)) {
            src.forEach(function (x) { var n = norm(x); if (n) items.push(n) })
          } else { var n = norm(src); if (n) items.push(n) }
          var seen = {}, out = []
          items.forEach(function (it) {
            var k = (it.title || '') + '|' + (it.url || '') + '|' + (it.thumbnail || '')
            if (!seen[k]) { seen[k] = true; out.push(it) }
          })
          return out
        },
        getHanimeGenres: function (obj) {
          var vals = []
          var pushVal = function (v) {
            if (Array.isArray(v)) { v.forEach(function (s) { pushVal(s) }); return }
            if (typeof v === 'string') {
              v.split(/[，、,|/]/).forEach(function (x) {
                var t = x.trim()
                if (t) vals.push(t)
              })
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
})()