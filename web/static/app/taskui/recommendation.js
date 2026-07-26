// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * Recommendation panel component — renders tag-based recommendations in the viewer sidebar.
 * Pure DOM-based, no Vue dependency.
 *
 * Usage:
 *   var panel = RecommendationPanel.create({
 *     type: 'hanime',
 *     currentId: 407014,
 *     tags: ['action', 'comedy'],
 *     onPlayItem: function (item) { ... }
 *   })
 *   container.appendChild(panel.element)
 */
;(function () {
  'use strict'

  window.RecommendationPanel = {
    create: function (options) {
      var type = options.type
      var currentId = options.currentId
      var allTags = options.tags || []
      var onPlayItem = options.onPlayItem || function () {}
      var collapsed = false
      var selectedTags = allTags.slice() // default: all selected
      var tagMode = 'any'
      var sortBy = 'random'
      var element = document.createElement('div')
      element.className = 'recommendation-panel border-b border-gray-200'

      // Header
      var header = document.createElement('div')
      header.className = 'flex items-center justify-between px-3 py-2 bg-gray-50 cursor-pointer select-none hover:bg-gray-100'
      header.innerHTML = '<span class="text-sm font-semibold text-gray-700">推荐</span>' +
        '<button class="rec-toggle text-gray-400 hover:text-gray-600 text-xs"><i class="fas fa-chevron-up"></i></button>'

      var body = document.createElement('div')
      body.className = 'recommendation-body'

      // Controls
      var controls = document.createElement('div')
      controls.className = 'px-3 py-2 space-y-2 border-b border-gray-100'

      // Tag selector
      var tagContainer = document.createElement('div')
      tagContainer.className = 'flex flex-wrap gap-1'
      controls.appendChild(tagContainer)

      // Mode and sort dropdowns
      var modeRow = document.createElement('div')
      modeRow.className = 'flex gap-2 text-xs'
      modeRow.innerHTML =
        '<select class="rec-mode flex-1 bg-gray-100 border border-gray-300 rounded px-1 py-1 text-gray-700 outline-none">' +
          '<option value="any">任一匹配</option>' +
          '<option value="all">全部匹配</option>' +
        '</select>' +
        '<select class="rec-sort flex-1 bg-gray-100 border border-gray-300 rounded px-1 py-1 text-gray-700 outline-none">' +
          '<option value="random">随机</option>' +
          '<option value="date_desc">最新</option>' +
          '<option value="tag_match_desc">最相关</option>' +
        '</select>'
      controls.appendChild(modeRow)

      // Results container
      var results = document.createElement('div')
      results.className = 'recommendation-results max-h-64 overflow-y-auto'

      body.appendChild(controls)
      body.appendChild(results)
      element.appendChild(header)
      element.appendChild(body)

      // Render tags
      function renderTags () {
        tagContainer.innerHTML = ''
        // "全部" toggle
        var allBtn = document.createElement('button')
        allBtn.className = 'text-xs px-2 py-0.5 rounded-full border transition ' +
          (selectedTags.length === allTags.length ? 'bg-blue-600 text-white border-blue-600' : 'bg-white text-gray-500 border-gray-300')
        allBtn.textContent = '全部'
        allBtn.addEventListener('click', function () {
          if (selectedTags.length === allTags.length) {
            selectedTags = []
          } else {
            selectedTags = allTags.slice()
          }
          renderTags()
          fetchRecommendations()
        })
        tagContainer.appendChild(allBtn)

        allTags.forEach(function (tag) {
          var btn = document.createElement('button')
          var active = selectedTags.indexOf(tag) >= 0
          btn.className = 'text-xs px-2 py-0.5 rounded-full border transition ' +
            (active ? 'bg-blue-100 text-blue-700 border-blue-300' : 'bg-white text-gray-500 border-gray-300')
          btn.textContent = tag
          btn.addEventListener('click', function () {
            var idx = selectedTags.indexOf(tag)
            if (idx >= 0) {
              selectedTags.splice(idx, 1)
            } else {
              selectedTags.push(tag)
            }
            renderTags()
            fetchRecommendations()
          })
          tagContainer.appendChild(btn)
        })
      }

      // Fetch recommendations
      function fetchRecommendations () {
        if (selectedTags.length === 0) {
          results.innerHTML = '<div class="px-3 py-4 text-center text-gray-400 text-sm">请选择标签</div>'
          return
        }
        results.innerHTML = '<div class="px-3 py-4 text-center text-gray-400 text-sm"><i class="fas fa-spinner fa-spin"></i> 加载中...</div>'

        // 先获取合集列表，排除合集中所有对象
        AppAPI.getCollection(type, currentId).then(function (collData) {
          var collIds = (collData.objects || []).map(function (o) { return o.id }).filter(function (id) { return id })
          // 去重：当前对象 ID 已在合集中，避免重复
          var allExcludeIds = [String(currentId)]
          collIds.forEach(function (id) {
            if (id !== currentId && allExcludeIds.indexOf(String(id)) < 0) {
              allExcludeIds.push(String(id))
            }
          })

          AppAPI.aggregate({
            types: type,
            tags: selectedTags.join(','),
            tagMode: tagMode,
            excludeIds: allExcludeIds.join(','),
            sort: sortBy,
            limit: 20
          }).then(function (data) {
            var list = data.objects || []
            // 再去重：确保推荐结果不包含已排除的对象
            var seenIds = {}
            allExcludeIds.forEach(function (id) { seenIds[id] = true })
            list = list.filter(function (item) { return !seenIds[item.id] })

            if (list.length === 0) {
              results.innerHTML = '<div class="px-3 py-4 text-center text-gray-400 text-sm">暂无推荐</div>'
              return
            }
            results.innerHTML = ''
            list.forEach(function (item) {
              var row = document.createElement('div')
              row.className = 'flex items-center gap-2 px-3 py-2 cursor-pointer hover:bg-blue-50 border-b border-gray-100 last:border-b-0'

              // Cover thumbnail
              var img = document.createElement('img')
              img.className = 'w-10 h-7 object-cover rounded flex-shrink-0 bg-gray-200'
              img.src = ''
              img.alt = ''
              var coverUrl = window.__dm_getThumbImage ? window.__dm_getThumbImage(item) : ''
              if (coverUrl) {
                img.src = coverUrl
                img.onerror = function () {
                  var fallbackUrl = window.__dm_getCoverImage ? window.__dm_getCoverImage(item) : ''
                  if (fallbackUrl && fallbackUrl !== this.src) {
                    this.src = fallbackUrl
                    this.onerror = null
                  } else {
                    this.style.display = 'none'
                  }
                }
              }
              else img.style.display = 'none'
              row.appendChild(img)

              // Title
              var title = document.createElement('span')
              title.className = 'flex-1 truncate text-sm text-gray-700'
              title.textContent = item.metadata && item.metadata.title ? item.metadata.title : item.url
              row.appendChild(title)

              row.addEventListener('click', function () {
                onPlayItem(item)
              })
              results.appendChild(row)
            })
          }).catch(function () {
            results.innerHTML = '<div class="px-3 py-4 text-center text-gray-400 text-sm">加载失败</div>'
          })
        })
      }

      // Wire up mode/sort dropdowns
      var modeSelect = controls.querySelector('.rec-mode')
      var sortSelect = controls.querySelector('.rec-sort')
      modeSelect.addEventListener('change', function () {
        tagMode = modeSelect.value
        fetchRecommendations()
      })
      sortSelect.addEventListener('change', function () {
        sortBy = sortSelect.value
        fetchRecommendations()
      })

      // Toggle collapse
      header.addEventListener('click', function (e) {
        if (e.target.closest('.rec-toggle')) {
          collapsed = !collapsed
          body.style.display = collapsed ? 'none' : ''
          header.querySelector('.rec-toggle i').className = 'fas fa-chevron-' + (collapsed ? 'down' : 'up')
        }
      })

      // Initial render
      renderTags()
      fetchRecommendations()

      return {
        element: element,
        update: function (opts) {
          // Recommendation panel does NOT re-fetch on collection switch
        },
        destroy: function () {
          element.remove()
        }
      }
    }
  }
})()