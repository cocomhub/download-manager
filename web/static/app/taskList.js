// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * Task list methods — fetch, select, cancel, paginate, etc.
 * Depends on: AppAPI
 */
;(function () {
  'use strict'

  window.AppTaskList = {
    register: function (app) {
      app.mixin({methods: {
        fetchTasks: function () {
          var self = this
          AppAPI.tasks().then(function (data) {
            self.tasks = data || []
            AppAPI.activeDownloads().then(function (dl) { self.activeDownloads = dl || [] }).catch(function () {})
            if (self.selectedTaskId) self.fetchTaskDetails(self.selectedTaskId, true)
          }).catch(function (e) { console.error(e) }).finally(function () { self.loading = false })
        },

        selectTask: function (id) {
          this.selectedTaskId = id
          this.selectedTaskIds = this.selectedTaskIds.filter(function (x) { return x !== id })
          this.selectedObjectUrls = []
          this.selectAllScope = 'page'
          this.searchQuery = ''
          this.pagination.page = 1
          this.viewMode = 'grid'
          this.fetchTaskDetails(id, false)
        },

        toggleSelectAll: function () {
          if (this.selectedTaskIds.length === this.tasks.length) {
            this.selectedTaskIds = []
          } else {
            this.selectedTaskIds = this.tasks.map(function (t) { return t.id })
          }
        },

        toggleSelectAllObjects: function () {
          var urls = (this.filteredObjects || []).map(function (o) { return o.url })
          // Toggle between page-only and all-pages mode
          if (this.selectAllScope === 'all') {
            // Currently in 'all' mode — switch to 'page' mode
            this.selectAllScope = 'page'
            // Select only current page objects
            this.selectedObjectUrls = [].concat(urls)
          } else {
            // Check if all current page objects are already selected
            var allSelected = urls.length > 0 && urls.every(function (u) { return this.selectedObjectUrls.indexOf(u) >= 0 }.bind(this))
            if (allSelected) {
              // Deselect current page objects
              this.selectedObjectUrls = this.selectedObjectUrls.filter(function (u) { return urls.indexOf(u) < 0 })
            } else {
              // Select current page objects (union)
              var set = {}
              this.selectedObjectUrls.forEach(function (u) { set[u] = true })
              urls.forEach(function (u) { set[u] = true })
              this.selectedObjectUrls = Object.keys(set)
              // Enter 'all' mode if selections span beyond current page
              if (urls.length === this.filteredObjects.length && this.selectedObjectUrls.length > urls.length) {
                this.selectAllScope = 'all'
              }
            }
          }
        },

        fetchTaskDetails: function (id, background) {
          if (!id) return
          if (!background) {
            this.isLoadingTask = true
            if (this.abortController) this.abortController.abort()
            this.abortController = new AbortController()
          }
          var self = this
          var limit = this.pagination.limit
          AppAPI.taskDetails(id, this.pagination.page, limit, this.searchQuery, this.sortBy)
            .then(function (data) {
              self.selectedTask = data
              if (data.concurrency !== undefined) self.taskConfigForm.concurrency = data.concurrency
              if (data.refresh_interval !== undefined) self.taskConfigForm.refresh_interval = data.refresh_interval
              if (data.total !== undefined) {
                self.pagination.total = data.total
                self.pagination.page = data.page
                self.pagination.limit = (data.limit === -1 || data.limit === 0) ? 'all' : data.limit
              }
            }).catch(function (e) {
              if (e.name === 'AbortError') return
              console.error(e)
            }).finally(function () {
              if (!background) { self.isLoadingTask = false; self.abortController = null }
            })
        },

        cancelCurrentTask: function () {
          if (this.isWriteDisabled) { this.showToast('UI-Only 模式下已禁用', 'error'); return }
          if (!this.selectedTaskId) return
          var self = this
          AppAPI.post('/api/tasks/' + encodeURIComponent(this.selectedTaskId) + '/cancel', {}).then(function (res) {
            if (!res.ok) throw new Error('取消失败')
            self.showToast('任务已取消', 'success')
            self.fetchTasks()
            self.fetchTaskDetails(self.selectedTaskId, true)
          }).catch(function (e) { self.showToast('取消失败: ' + e.message, 'error') })
        },

        cancelSelected: function () {
          if (this.isWriteDisabled) { this.showToast('UI-Only 模式下已禁用', 'error'); return }
          if (this.selectedTaskIds.length === 0) return
          var self = this
          AppAPI.post('/api/tasks/cancel_batch', { ids: this.selectedTaskIds }).then(function (res) {
            if (!res.ok) throw new Error('批量取消失败')
            return res.json()
          }).then(function (result) {
            var failed = Object.entries(result).filter(function (kv) { return kv[1] !== 'ok' })
            if (failed.length === 0) self.showToast('已取消选中任务', 'success')
            else self.showToast('部分取消失败', 'error')
            self.selectedTaskIds = []
            self.fetchTasks()
            if (self.selectedTaskId) self.fetchTaskDetails(self.selectedTaskId, true)
          }).catch(function (e) { self.showToast('批量取消失败: ' + e.message, 'error') })
        },

        retryAllFailed: function () {
          if (this.isWriteDisabled) { this.showToast('UI-Only 模式下已禁用', 'error'); return }
          if (!this.selectedTaskId) return
          var self = this
          AppAPI.post('/api/tasks/' + encodeURIComponent(this.selectedTaskId) + '/retry', {}).then(function (res) {
            if (!res.ok) throw new Error('重试失败')
            self.showToast('已重试所有失败对象', 'success')
            self.fetchTaskDetails(self.selectedTaskId, true)
          }).catch(function (e) { self.showToast('重试失败: ' + e.message, 'error') })
        },

        changePage: function (p) {
          if (p < 1) return
          if (this.pagination.limit !== 'all' && p > Math.ceil(this.pagination.total / this.pagination.limit)) return
          this.pagination.page = p
          this.selectedObjectUrls = []
          this.selectAllScope = 'page'
          this.fetchTaskDetails(this.selectedTaskId)
        },

        changeLimit: function () {
          this.pagination.page = 1
          this.selectedObjectUrls = []
          this.selectAllScope = 'page'
          this.fetchTaskDetails(this.selectedTaskId)
        },

        retrySelectedObjects: function () {
          if (this.isWriteDisabled) { this.showToast('UI-Only 模式下已禁用', 'error'); return }
          if (this.selectedObjectUrls.length === 0) return

          var self = this
          var isAllMode = this.selectAllScope === 'all'

          if (isAllMode) {
            // Cross-page mode: retry all failed for this task
            if (!this.selectedTaskId) return
            AppAPI.post('/api/tasks/' + encodeURIComponent(this.selectedTaskId) + '/retry', {})
              .then(function (res) {
                if (!res.ok) throw new Error('批量重试失败')
                self.showToast('已重试所有失败对象', 'success')
                self.selectedObjectUrls = []
                self.selectAllScope = 'page'
                self.fetchTaskDetails(self.selectedTaskId, true)
              }).catch(function (e) { self.showToast('批量重试失败: ' + e.message, 'error') })
            return
          }

          // Page mode: only retry selected failed objects individually
          var objs = (self.selectedTask && self.selectedTask.objects) || []
          var failedUrls = []
          objs.forEach(function (o) {
            if (self.selectedObjectUrls.indexOf(o.url) >= 0 && o.status === 'failed') {
              failedUrls.push(o.url)
            }
          })

          if (failedUrls.length === 0) {
            self.showToast('选中的对象中没有可重试的失败项', 'info')
            return
          }

          var completed = 0
          var totalFailed = 0
          failedUrls.forEach(function (url) {
            AppAPI.post('/api/tasks/' + encodeURIComponent(self.selectedTaskId) + '/retry', { url: url })
              .then(function (res) {
                if (res.ok) {
                  completed++
                  var obj = (self.selectedTask && self.selectedTask.objects || []).find(function (o) { return o.url === url })
                  if (obj) { obj.status = 'pending'; obj.progress = 0 }
                } else {
                  totalFailed++
                }
              }).catch(function () { totalFailed++ })
              .finally(function () {
                if (completed + totalFailed === failedUrls.length) {
                  if (totalFailed > 0) {
                    self.showToast('已重试 ' + completed + ' 个，失败 ' + totalFailed + ' 个', 'error')
                  } else {
                    self.showToast('已重试 ' + completed + ' 个失败对象', 'success')
                  }
                  self.selectedObjectUrls = []
                  self.fetchTaskDetails(self.selectedTaskId, true)
                }
              })
          })
        },

        cancelSelectAllObjects: function () {
          if (this.isWriteDisabled) { this.showToast('UI-Only 模式下已禁用', 'error'); return }
          if (this.selectedObjectUrls.length === 0) return

          var self = this
          if (this.selectAllScope === 'all') {
            // Cross-page mode: cancel the entire task
            if (!this.selectedTaskId) return
            AppAPI.post('/api/tasks/' + encodeURIComponent(this.selectedTaskId) + '/cancel', {})
              .then(function (res) {
                if (!res.ok) throw new Error('取消失败')
                self.showToast('任务已取消', 'success')
                self.selectedObjectUrls = []
                self.selectAllScope = 'page'
                self.fetchTasks()
                self.fetchTaskDetails(self.selectedTaskId, true)
              }).catch(function (e) { self.showToast('取消失败: ' + e.message, 'error') })
            return
          }

          // Page mode: batch cancel selected objects
          AppAPI.post('/api/tasks/' + encodeURIComponent(self.selectedTaskId) + '/object/cancel_batch', { urls: self.selectedObjectUrls })
            .then(function (res) { if (!res.ok) throw new Error('批量取消失败'); return res.json() })
            .then(function (result) {
              var okList = Object.entries(result).filter(function (kv) { return kv[1] === 'ok' }).map(function (kv) { return kv[0] })
              if (self.selectedTask && self.selectedTask.objects && okList.length > 0) {
                self.selectedTask.objects.forEach(function (o) {
                  if (okList.indexOf(o.url) >= 0) { o.status = 'cancelled'; o.progress = 0 }
                })
              }
              var failed = Object.entries(result).filter(function (kv) { return kv[1] !== 'ok' })
              if (failed.length === 0) self.showToast('已取消选中对象', 'success')
              else self.showToast('部分对象取消失败', 'error')
              self.selectedObjectUrls = []
            }).catch(function (e) { self.showToast('批量取消失败: ' + e.message, 'error') })
        },

        undoCancelSelectAllObjects: function () {
          if (this.isWriteDisabled) { this.showToast('UI-Only 模式下已禁用', 'error'); return }
          if (this.selectedObjectUrls.length === 0) return

          var self = this
          if (this.selectAllScope === 'all') {
            self.showToast('跨页全选模式不支持批量撤销取消，请切换为单页模式', 'info')
            return
          }

          // Page mode: batch undo cancel selected objects
          AppAPI.post('/api/tasks/' + encodeURIComponent(self.selectedTaskId) + '/object/undo_cancel_batch', { urls: self.selectedObjectUrls })
            .then(function (res) { if (!res.ok) throw new Error('批量撤销失败'); return res.json() })
            .then(function (result) {
              var okList = Object.entries(result).filter(function (kv) { return kv[1] === 'ok' }).map(function (kv) { return kv[0] })
              if (self.selectedTask && self.selectedTask.objects && okList.length > 0) {
                self.selectedTask.objects.forEach(function (o) {
                  if (okList.indexOf(o.url) >= 0) { o.status = 'pending'; o.progress = 0 }
                })
              }
              var failed = Object.entries(result).filter(function (kv) { return kv[1] !== 'ok' })
              if (failed.length === 0) self.showToast('已撤销选中对象', 'success')
              else self.showToast('部分对象撤销失败', 'error')
              self.selectedObjectUrls = []
            }).catch(function (e) { self.showToast('批量撤销失败: ' + e.message, 'error') })
        },

        // ---- Single-object cancel/undo ----
        cancelObject: function (obj) {
          if (this.isWriteDisabled) { this.showToast('UI-Only 模式下已禁用', 'error'); return }
          if (!this.selectedTaskId || !obj || !obj.url) return
          var self = this
          AppAPI.post('/api/tasks/' + encodeURIComponent(this.selectedTaskId) + '/object/cancel', { url: obj.url })
            .then(function (res) {
              if (!res.ok) throw new Error('取消失败')
              obj.status = 'cancelled'
              self.showToast('已取消该对象', 'success')
            }).catch(function (e) { self.showToast('取消失败: ' + e.message, 'error') })
        },
        undoCancelObject: function (obj) {
          if (this.isWriteDisabled) { this.showToast('UI-Only 模式下已禁用', 'error'); return }
          if (!this.selectedTaskId || !obj || !obj.url) return
          var self = this
          AppAPI.post('/api/tasks/' + encodeURIComponent(this.selectedTaskId) + '/object/undo_cancel', { url: obj.url })
            .then(function (res) {
              if (!res.ok) throw new Error('撤销失败')
              obj.status = 'pending'
              self.showToast('已撤销取消', 'success')
            }).catch(function (e) { self.showToast('撤销失败: ' + e.message, 'error') })
        },

        // ---- Task config panel ----
        toggleTaskConfigPanel: function () {
          this.showTaskConfigPanel = !this.showTaskConfigPanel
        },
        saveTaskConfig: function () {
          if (this.isWriteDisabled) { this.showToast('UI-Only 模式下已禁用', 'error'); return }
          if (!this.selectedTaskId) return
          var self = this
          AppAPI.patch('/api/tasks/' + encodeURIComponent(this.selectedTaskId) + '/runtime', {
            concurrency: this.taskConfigForm.concurrency,
            refresh_interval: this.taskConfigForm.refresh_interval
          }).then(function (res) {
            if (!res.ok) throw new Error('保存失败')
            self.showToast('任务配置已保存', 'success')
            self.fetchTaskDetails(self.selectedTaskId, true)
          }).catch(function (e) { self.showToast('保存失败: ' + e.message, 'error') })
        }  // end saveTaskConfig
      }})  // end app.mixin
    }  // end register
  }  // end AppTaskList
})()