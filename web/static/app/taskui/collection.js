// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * Collection panel component — renders a list of collection items in the viewer sidebar.
 * Pure DOM-based, no Vue dependency. Can be used by any task type viewer.
 *
 * Usage:
 *   var panel = CollectionPanel.create({
 *     type: 'hanime',
 *     currentId: 407014,
 *     onPlayItem: function (item) { ... }
 *   })
 *   container.appendChild(panel.element)
 *   // To update highlight without re-fetching:
 *   panel.update({ currentId: 407015 })
 *   // To destroy:
 *   panel.destroy()
 */
;(function () {
  'use strict'

  window.CollectionPanel = {
    create: function (options) {
      var type = options.type
      var currentId = options.currentId
      var onPlayItem = options.onPlayItem || function () {}
      var collapsed = false
      var items = []
      var element = document.createElement('div')
      element.className = 'collection-panel border-b border-gray-200'

      // Header
      var header = document.createElement('div')
      header.className = 'flex items-center justify-between px-3 py-2 bg-gray-50 cursor-pointer select-none hover:bg-gray-100'
      header.innerHTML = '<span class="text-sm font-semibold text-gray-700">合集 <span class="collection-count text-gray-400">(0)</span></span>' +
        '<button class="collection-toggle text-gray-400 hover:text-gray-600 text-xs"><i class="fas fa-chevron-up"></i></button>'

      // Body
      var body = document.createElement('div')
      body.className = 'collection-body max-h-64 overflow-y-auto'

      // Fetch collection data
      if (type && currentId) {
        AppAPI.getCollection(type, currentId).then(function (data) {
          items = data.objects || []
          renderList(items)
        }).catch(function () {
          body.innerHTML = '<div class="px-3 py-4 text-center text-gray-400 text-sm">加载合集失败</div>'
        })
      } else {
        body.innerHTML = '<div class="px-3 py-4 text-center text-gray-400 text-sm">暂无合集</div>'
      }

      function renderList (list) {
        if (list.length === 0) {
          body.innerHTML = '<div class="px-3 py-4 text-center text-gray-400 text-sm">暂无合集</div>'
          header.querySelector('.collection-count').textContent = '(0)'
          return
        }
        header.querySelector('.collection-count').textContent = '(' + list.length + ')'
        body.innerHTML = ''
        list.forEach(function (item, idx) {
          var row = document.createElement('div')
          row.className = 'flex items-center gap-2 px-3 py-2 cursor-pointer hover:bg-blue-50 border-b border-gray-100 last:border-b-0 text-sm'
          row.dataset.id = item.id

          // Number badge
          var badge = document.createElement('span')
          badge.className = 'w-5 h-5 rounded-full flex items-center justify-center text-xs font-medium flex-shrink-0 ' +
            (item.id === currentId ? 'bg-blue-600 text-white' : 'bg-gray-200 text-gray-600')
          badge.textContent = idx + 1
          row.appendChild(badge)

          // Title
          var title = document.createElement('span')
          title.className = 'flex-1 truncate ' + (item.id === currentId ? 'text-blue-700 font-medium' : 'text-gray-700')
          title.textContent = item.metadata && item.metadata.collection_title
            ? item.metadata.collection_title
            : (item.metadata && item.metadata.title ? item.metadata.title : 'Item ' + item.id)
          row.appendChild(title)

          // Duration
          if (item.metadata && item.metadata.duration) {
            var dur = document.createElement('span')
            dur.className = 'text-xs text-gray-400 flex-shrink-0'
            dur.textContent = item.metadata.duration
            row.appendChild(dur)
          }

          // Current indicator
          if (item.id === currentId) {
            var indicator = document.createElement('span')
            indicator.className = 'text-blue-600 text-xs flex-shrink-0 ml-1'
            indicator.textContent = '▶'
            row.appendChild(indicator)
          }

          row.addEventListener('click', function () {
            if (item.id !== currentId) {
              onPlayItem(item)
            }
          })

          body.appendChild(row)
        })

        // Scroll to current item
        var currentRow = body.querySelector('[data-id="' + currentId + '"]')
        if (currentRow) {
          currentRow.scrollIntoView({ block: 'nearest' })
        }
      }

      // Toggle collapse
      header.addEventListener('click', function (e) {
        if (e.target.closest('.collection-toggle')) {
          collapsed = !collapsed
          body.style.display = collapsed ? 'none' : ''
          header.querySelector('.collection-toggle i').className = 'fas fa-chevron-' + (collapsed ? 'down' : 'up')
        }
      })

      element.appendChild(header)
      element.appendChild(body)

      return {
        element: element,
        update: function (opts) {
          if (opts.currentId !== undefined) {
            currentId = opts.currentId
            renderList(items)
          }
        },
        destroy: function () {
          element.remove()
        }
      }
    }
  }
})()