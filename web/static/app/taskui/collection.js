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
      body.className = 'collection-body max-h-80 overflow-y-auto'

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
          row.className = 'flex items-start gap-2 px-3 py-2 cursor-pointer hover:bg-blue-50 border-b border-gray-100 last:border-b-0'
          row.dataset.id = item.id

          // Thumbnail (larger: 120x68)
          var thumbWrap = document.createElement('div')
          thumbWrap.className = 'flex-shrink-0 relative'
          thumbWrap.style.cssText = 'width:120px;height:68px;border-radius:6px;overflow:hidden;background:#f3f4f6'

          var thumb = document.createElement('img')
          thumb.className = 'w-full h-full object-cover'
          thumb.src = ''
          thumb.alt = ''
          var thumbUrl = window.__dm_getThumbImage ? window.__dm_getThumbImage(item) : ''
          if (thumbUrl) {
            thumb.src = thumbUrl
            thumb.onerror = function () {
              var coverUrl = window.__dm_getCoverImage ? window.__dm_getCoverImage(item) : ''
              if (coverUrl && coverUrl !== this.src) {
                this.src = coverUrl
                this.onerror = null
              } else {
                this.style.display = 'none'
              }
            }
          } else {
            thumb.style.display = 'none'
          }
          thumbWrap.appendChild(thumb)

          // Duration overlay
          if (item.metadata && item.metadata.duration) {
            var dur = document.createElement('span')
            dur.className = 'absolute bottom-0.5 right-0.5 bg-black/70 text-white text-[10px] px-1 rounded'
            dur.textContent = item.metadata.duration
            thumbWrap.appendChild(dur)
          }

          // Current indicator
          if (item.id === currentId) {
            var cur = document.createElement('span')
            cur.className = 'absolute top-0.5 left-0.5 bg-blue-600 text-white text-[10px] px-1 rounded'
            cur.textContent = '▶ 当前'
            thumbWrap.appendChild(cur)
          }

          row.appendChild(thumbWrap)

          // Info
          var info = document.createElement('div')
          info.className = 'flex-1 min-w-0'

          var title = document.createElement('div')
          title.className = 'text-sm font-medium leading-tight ' + (item.id === currentId ? 'text-blue-700' : 'text-gray-800')
          title.textContent = item.metadata && item.metadata.collection_title
            ? item.metadata.collection_title
            : (item.metadata && item.metadata.title ? item.metadata.title : 'Item ' + item.id)
          title.style.cssText = 'display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden'
          info.appendChild(title)

          // Meta row
          var meta = document.createElement('div')
          meta.className = 'text-xs text-gray-400 mt-1'
          meta.textContent = item.metadata && item.metadata.date ? item.metadata.date : ''
          if (item.metadata && item.metadata.resolution) {
            meta.textContent += (meta.textContent ? ' · ' : '') + item.metadata.resolution
          }
          if (meta.textContent) info.appendChild(meta)

          row.appendChild(info)

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