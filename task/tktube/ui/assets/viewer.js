/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

;(function () {
  'use strict'

  if (!window.__dm_uiBridge) return

  var D = TaskUI.Data
  var Dm = TaskUI.Dom
  var M = TaskUI.Modal

  // ---- Theme colors ----
  var THEME = {
    primary: '#3b82f6',
    primaryDark: '#2563eb',
    bg: '#ffffff',
    bgAlt: '#f9fafb',
    border: '#e5e7eb',
    text: '#1f2937',
    textSecondary: '#6b7280',
    textMuted: '#9ca3af'
  }

  // ---- Sorting ----
  function priorityScore (obj) {
    if (obj && obj.extra) {
      if (obj.extra.variant_priority !== undefined) return obj.extra.variant_priority
      if (obj.extra.priority !== undefined) return obj.extra.priority
    }
    var r = D.getResolution(obj)
    if (/1080/.test(r)) return 30
    if (/720/.test(r)) return 20
    if (/480/.test(r)) return 10
    return 0
  }

  // ---- Task view (legacy) ----

  function renderTaskView (task) {
    var container = document.getElementById('custom-task-container')
    if (!container) return

    container.innerHTML = ''

    var objects = (task && task.objects) || []
    if (objects.length === 0) {
      container.innerHTML = '<div style="padding:32px;text-align:center;color:' + THEME.textSecondary + ';font-size:14px">暂无对象</div>'
      return
    }

    // Group by content_group
    var groups = {}
    var ungrouped = []
    objects.forEach(function (obj) {
      var g = D.getContentGroup(obj)
      if (g) {
        if (!groups[g]) groups[g] = []
        groups[g].push(obj)
      } else {
        ungrouped.push(obj)
      }
    })

    // Sort groups by highest priority
    var groupNames = Object.keys(groups).sort(function (a, b) {
      var aMax = Math.max.apply(null, groups[a].map(priorityScore))
      var bMax = Math.max.apply(null, groups[b].map(priorityScore))
      return bMax - aMax
    })

    var wrapper = document.createElement('div')
    wrapper.style.cssText = 'padding:16px;overflow-y:auto;height:100%'

    if (groupNames.length > 0) {
      groupNames.forEach(function (groupName) {
        var items = groups[groupName]
        items.sort(function (a, b) { return priorityScore(b) - priorityScore(a) })

        var section = document.createElement('div')
        section.style.cssText = 'margin-bottom:24px'

        var headerRow = document.createElement('div')
        headerRow.style.cssText = 'display:flex;align-items:center;gap:12px;margin-bottom:12px;padding-bottom:8px;border-bottom:2px solid ' + THEME.primary

        var groupTitle = document.createElement('h3')
        groupTitle.style.cssText = 'font-size:16px;font-weight:600;color:' + THEME.text + ';margin:0'
        groupTitle.textContent = groupName
        headerRow.appendChild(groupTitle)

        var countBadge = document.createElement('span')
        countBadge.style.cssText = 'font-size:11px;background:' + THEME.primary + ';color:#fff;border-radius:10px;padding:2px 8px'
        countBadge.textContent = items.length + ' 项'
        headerRow.appendChild(countBadge)

        section.appendChild(headerRow)

        var grid = document.createElement('div')
        grid.style.cssText = 'display:grid;grid-template-columns:repeat(auto-fill,minmax(180px,1fr));gap:12px'

        items.forEach(function (obj) {
          var card = createObjectCard(obj)
          grid.appendChild(card)
        })

        section.appendChild(grid)
        wrapper.appendChild(section)
      })
    }

    if (ungrouped.length > 0) {
      ungrouped.sort(function (a, b) { return priorityScore(b) - priorityScore(a) })

      var section = document.createElement('div')
      section.style.cssText = 'margin-bottom:24px'

      var headerRow = document.createElement('div')
      headerRow.style.cssText = 'display:flex;align-items:center;gap:12px;margin-bottom:12px;padding-bottom:8px;border-bottom:2px solid ' + THEME.textMuted

      var groupTitle = document.createElement('h3')
      groupTitle.style.cssText = 'font-size:16px;font-weight:600;color:' + THEME.text + ';margin:0'
      groupTitle.textContent = '未分组'
      headerRow.appendChild(groupTitle)

      var countBadge = document.createElement('span')
      countBadge.style.cssText = 'font-size:11px;background:' + THEME.textMuted + ';color:#fff;border-radius:10px;padding:2px 8px'
      countBadge.textContent = ungrouped.length + ' 项'
      headerRow.appendChild(countBadge)

      section.appendChild(headerRow)

      var grid = document.createElement('div')
      grid.style.cssText = 'display:grid;grid-template-columns:repeat(auto-fill,minmax(180px,1fr));gap:12px'

      ungrouped.forEach(function (obj) {
        var card = createObjectCard(obj)
        grid.appendChild(card)
      })

      section.appendChild(grid)
      wrapper.appendChild(section)
    }

    container.appendChild(wrapper)
  }

  function createObjectCard (obj) {
    var card = document.createElement('div')
    card.style.cssText = 'border:1px solid ' + THEME.border + ';border-radius:8px;overflow:hidden;background:' + THEME.bg + ';transition:box-shadow 0.2s'
    card.onmouseenter = function () { card.style.boxShadow = '0 4px 12px rgba(0,0,0,0.1)' }
    card.onmouseleave = function () { card.style.boxShadow = 'none' }

    var coverArea = document.createElement('div')
    coverArea.style.cssText = 'position:relative;background:#f3f4f6;aspect-ratio:16/9;display:flex;align-items:center;justify-content:center;overflow:hidden'

    var coverUrl = D.getThumbImage(obj)
    if (coverUrl) {
      var img = document.createElement('img')
      img.src = coverUrl
      img.alt = ''
      img.style.cssText = 'width:100%;height:100%;object-fit:cover'
      img.onerror = function () { img.style.display = 'none' }
      coverArea.appendChild(img)
    } else {
      var icon = document.createElement('i')
      icon.className = 'fas fa-video'
      icon.style.cssText = 'font-size:32px;color:' + THEME.textMuted
      coverArea.appendChild(icon)
    }

    var badge = document.createElement('span')
    badge.style.cssText = 'position:absolute;top:6px;right:6px;font-size:10px;padding:2px 6px;border-radius:4px;font-weight:600;background:' + D.statusBg(obj.status) + ';color:' + D.statusColor(obj.status)
    badge.textContent = obj.status || 'unknown'
    coverArea.appendChild(badge)

    var res = D.getResolution(obj)
    if (res) {
      var resBadge = document.createElement('span')
      resBadge.style.cssText = 'position:absolute;bottom:6px;right:6px;font-size:10px;padding:2px 6px;border-radius:4px;background:rgba(0,0,0,0.7);color:#fff'
      resBadge.textContent = res
      coverArea.appendChild(resBadge)
    }

    card.appendChild(coverArea)

    var info = document.createElement('div')
    info.style.cssText = 'padding:10px 12px'

    var titleEl = document.createElement('div')
    titleEl.style.cssText = 'font-size:13px;font-weight:600;color:' + THEME.text + ';line-height:1.3;overflow:hidden;text-overflow:ellipsis;white-space:nowrap'
    titleEl.textContent = D.getTitle(obj) || obj.url || 'Untitled'
    info.appendChild(titleEl)

    var metaRow = document.createElement('div')
    metaRow.style.cssText = 'display:flex;gap:8px;font-size:11px;color:' + THEME.textSecondary + ';margin-top:4px'

    var dur = D.getDuration(obj)
    if (dur) {
      var durEl = document.createElement('span')
      durEl.textContent = dur
      metaRow.appendChild(durEl)
    }

    var dateVal = D.getDate(obj)
    if (dateVal) {
      if (dur) {
        var sep = document.createElement('span')
        sep.textContent = '|'
        metaRow.appendChild(sep)
      }
      var dateEl = document.createElement('span')
      dateEl.textContent = dateVal
      metaRow.appendChild(dateEl)
    }

    if (dur || dateVal) info.appendChild(metaRow)

    if (obj.progress > 0 && obj.progress < 100 && obj.status === 'downloading') {
      var progOuter = document.createElement('div')
      progOuter.style.cssText = 'margin-top:8px;height:4px;background:#f3f4f6;border-radius:2px;overflow:hidden'
      var progInner = document.createElement('div')
      progInner.style.cssText = 'height:100%;background:' + THEME.primary + ';border-radius:2px;transition:width 0.3s'
      progInner.style.width = (obj.progress || 0) + '%'
      progOuter.appendChild(progInner)
      info.appendChild(progOuter)
    }

    card.appendChild(info)

    card.style.cursor = 'pointer'
    card.onclick = function () {
      if (obj.status === 'completed') {
        // Use TaskUI renderViewer if available
        var handler = TaskUI.get('tktube')
        if (handler && handler.renderViewer) {
          handler.renderViewer(null, obj, function () {})
        }
      }
    }

    return card
  }

  // ---- Register with TaskUI ----
  if (typeof TaskUI !== 'undefined' && TaskUI.register) {
    TaskUI.register('tktube', {
      type: 'tktube',
      label: 'TKTube',
      icon: 'fa-video',
      viewerLabel: '查看',
      renderForm: TaskUI.defineForm({
        fields: [
          { type: 'text', key: 'keyword', label: '关键字', required: true, placeholder: '例如：RCTD' },
          {
            type: 'select', key: 'subtype', label: '子类型',
            options: [
              { value: 'tag', label: '标签' },
              { value: 'model', label: '模特' },
              { value: 'search', label: '搜索' }
            ]
          },
          { type: 'number', key: 'max_concurrent', label: '并发数', min: 1, max: 10, default: 2 },
          { type: 'number', key: 'refresh_interval', label: '刷新间隔（秒）', min: 10, default: 3600 },
        ]
      }),
      renderMeta: TaskUI.defineMeta({
        fields: [
          { type: 'text', key: 'keyword', label: '关键字', path: 'extra.keyword' },
          { type: 'text', key: 'subtype', label: '子类型', path: 'extra.subtype' },
          { type: 'text', key: 'max_concurrent', label: '并发', path: 'extra.max_concurrent' },
          { type: 'text', key: 'refresh_interval', label: '刷新间隔', path: 'extra.refresh_interval' },
        ]
      }),
      collectExtra: function (formData) {
        var extra = {}
        if (formData.keyword) extra.keyword = formData.keyword
        if (formData.subtype) extra.subtype = formData.subtype
        if (formData.max_concurrent) extra.max_concurrent = formData.max_concurrent
        if (formData.refresh_interval) extra.refresh_interval = formData.refresh_interval
        return extra
      },
      shouldShowViewer: function (obj) { return obj.status === 'completed' },
      onClick: function (obj, helpers) {
        if (obj.status !== 'completed') return false
        helpers.openTaskTypeViewer(obj)
        return true
      },
      renderViewer: function (h, obj, onClose) {
        var videoUrl = D.getVideoUrl(obj)
        var coverUrl = D.getCoverImage(obj)
        var title = D.getTitle(obj) || 'TKTube'
        var res = D.getResolution(obj)
        var dur = D.getDuration(obj)
        var dateVal = D.getDate(obj)
        var contentGroup = D.getContentGroup(obj)
        var origin = D.getOriginLink(obj)
        var details = D.getDetails(obj)
        var tags = D.getTags(obj)
        var fileUrlVal = D.getFileUrl(obj)
        var taskType = obj && obj.metadata && obj.metadata.task_type
        var objTags = (obj && obj.extra && Array.isArray(obj.extra.tags)) ? obj.extra.tags : []

        var modal = M.create(obj, {
          title: title,
          maxWidth: '1400px',
          badges: [
            contentGroup ? { text: contentGroup, bg: '#eff6ff', color: '#2563eb' } : null,
            res ? { text: res, bg: '#f3f4f6', color: '#4b5563' } : null
          ].filter(Boolean),
          mediaType: 'video',
          videoUrl: videoUrl,
          coverUrl: coverUrl,
          infoBar: [
            dur ? { icon: 'fas fa-clock', text: dur } : null,
            dateVal ? { icon: 'fas fa-calendar', text: dateVal } : null,
            res ? { icon: 'fas fa-expand', text: res } : null,
            contentGroup ? { icon: 'fas fa-folder', text: contentGroup } : null
          ].filter(Boolean),
          contentRenderer: function (contentDiv) {
            if (details) {
              var de = document.createElement('div')
              de.style.cssText = 'font-size:14px;color:#374151;line-height:1.6;white-space:pre-line;margin-bottom:12px'
              de.textContent = details
              contentDiv.appendChild(de)
            }
            if (tags.length > 0) {
              var tagWrap = document.createElement('div')
              tagWrap.style.cssText = 'display:flex;flex-wrap:wrap;gap:4px;margin-bottom:12px'
              tagWrap.appendChild(Dm.createTagChips(tags))
              contentDiv.appendChild(tagWrap)
            }
          },
          sidebar: 'collection',
          type: taskType,
          currentId: obj.id,
          tags: objTags,
          onPlayItem: function (item) {
            AppAPI.getObject(taskType, item.id).then(function (newObj) {
              modal.close()
              // Re-open with new object
              modal = M.create(newObj, {
                title: D.getTitle(newObj) || 'TKTube',
                maxWidth: '1400px',
                badges: [
                  D.getContentGroup(newObj) ? { text: D.getContentGroup(newObj), bg: '#eff6ff', color: '#2563eb' } : null,
                  D.getResolution(newObj) ? { text: D.getResolution(newObj), bg: '#f3f4f6', color: '#4b5563' } : null
                ].filter(Boolean),
                mediaType: 'video',
                videoUrl: D.getVideoUrl(newObj),
                coverUrl: D.getCoverImage(newObj),
                infoBar: [
                  D.getDuration(newObj) ? { icon: 'fas fa-clock', text: D.getDuration(newObj) } : null,
                  D.getDate(newObj) ? { icon: 'fas fa-calendar', text: D.getDate(newObj) } : null,
                  D.getResolution(newObj) ? { icon: 'fas fa-expand', text: D.getResolution(newObj) } : null,
                  D.getContentGroup(newObj) ? { icon: 'fas fa-folder', text: D.getContentGroup(newObj) } : null
                ].filter(Boolean),
                contentRenderer: function (contentDiv) {
                  var d = D.getDetails(newObj)
                  if (d) {
                    var de = document.createElement('div')
                    de.style.cssText = 'font-size:14px;color:#374151;line-height:1.6;white-space:pre-line;margin-bottom:12px'
                    de.textContent = d
                    contentDiv.appendChild(de)
                  }
                  var t = D.getTags(newObj)
                  if (t.length > 0) {
                    var tagWrap = document.createElement('div')
                    tagWrap.style.cssText = 'display:flex;flex-wrap:wrap;gap:4px;margin-bottom:12px'
                    tagWrap.appendChild(Dm.createTagChips(t))
                    contentDiv.appendChild(tagWrap)
                  }
                },
                sidebar: 'collection',
                type: taskType,
                currentId: item.id,
                tags: (newObj && newObj.extra && Array.isArray(newObj.extra.tags)) ? newObj.extra.tags : [],
                onPlayItem: function (nextItem) {
                  AppAPI.getObject(taskType, nextItem.id).then(function (nextObj) {
                    modal.close()
                    // Delegate to renderViewer again
                    TaskUI.get('tktube').renderViewer(h, nextObj, onClose)
                  })
                },
                footerActions: [
                  D.getFileUrl(newObj) ? Dm.createLink(D.getFileUrl(newObj), '打开文件', { style: 'padding:6px 12px;border-radius:6px;background:#2563eb;color:#fff;text-decoration:none;font-size:14px;display:inline-block' }) : null,
                  D.getOriginLink(newObj) ? Dm.createLink(D.getOriginLink(newObj), '打开原页面') : null,
                  Dm.createButton('复制标题', function () { D.copyToClipboard(D.getTitle(newObj)) }),
                  D.getOriginLink(newObj) ? Dm.createButton('复制链接', function () { D.copyToClipboard(D.getOriginLink(newObj)) }) : null,
                ].filter(Boolean),
                onClose: onClose
              })
            })
          },
          footerActions: [
            fileUrlVal ? Dm.createLink(fileUrlVal, '打开文件', { style: 'padding:6px 12px;border-radius:6px;background:#2563eb;color:#fff;text-decoration:none;font-size:14px;display:inline-block' }) : null,
            origin ? Dm.createLink(origin, '打开原页面') : null,
            Dm.createButton('复制标题', function () { D.copyToClipboard(D.getTitle(obj)) }),
            origin ? Dm.createButton('复制链接', function () { D.copyToClipboard(origin) }) : null,
          ].filter(Boolean),
          onClose: onClose
        })

        return h ? h('div') : null
      }
    })
  }

  // Register as task view (legacy compat)
  window.__dm_uiBridge.registerTaskView('tktube', {
    render: function (task) {
      renderTaskView(task)
    }
  })
})()