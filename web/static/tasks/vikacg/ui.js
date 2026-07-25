// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * vikacg 类型 UI 组件
 * 自定义查看器 — 图片浏览器 + 标签 + 链接
 */
;(function () {
  'use strict'

  function getImages(obj) {
    var imgs = []
    if (obj && obj.status === 'completed' && obj.extra && Array.isArray(obj.extra.files)) {
      obj.extra.files.forEach(function (f) {
        if (f.type === 'image' && f.path) imgs.push(fileUrl(f.path))
      })
    }
    if (imgs.length === 0 && obj && obj.extra && Array.isArray(obj.extra.images)) {
      obj.extra.images.forEach(function (u) { if (typeof u === 'string' && u) imgs.push(u) })
    }
    return imgs
  }

  function getLinks(obj) {
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
  }

  function getExcerpt(obj) {
    var s = (obj && obj.extra && obj.extra.content_text) || ''
    if (!s) return ''
    var t = s.replace(/\s+/g, ' ').trim()
    return t.length > 200 ? t.slice(0, 200) + '...' : t
  }

  function getContentHtml(obj) {
    var s = (obj && obj.extra && obj.extra.content_html) || ''
    return typeof s === 'string' ? s.trim() : ''
  }

  function getTags(obj) {
    if (obj && obj.extra && Array.isArray(obj.extra.tags)) return obj.extra.tags
    if (obj && obj.extra && typeof obj.extra.tags === 'string') return [obj.extra.tags]
    return []
  }

  function fileUrl(path) {
    if (!path) return ''
    var normalized = path.replace(/\\/g, '/')
    var root = typeof window.__dm_downloadRoot === 'string' ? window.__dm_downloadRoot : ''
    if (root && normalized.indexOf(root) === 0) { normalized = normalized.slice(root.length) }
    normalized = normalized.replace(/^\//, '')
    return '/files/' + normalized.split('/').filter(function(s){return s&&s!=='..'}).map(encodeURIComponent).join('/')
  }

  function getTitle(obj) { return (obj && obj.metadata && obj.metadata.title) || '' }
  function getDate(obj) { return (obj && obj.metadata && obj.metadata.date) || '' }

  // ---- 注册 vikacg UI ----
  TaskUI.register('vikacg', {
    type: 'vikacg',
    label: 'VikACG',
    icon: 'fa-image',
    viewerLabel: '浏览',
    shouldShowViewer: function (obj) {
      return obj.status === 'completed' && obj.extra && (Array.isArray(obj.extra.images) || Array.isArray(obj.extra.files))
    },
    renderViewer: function (h, obj, onClose) {
      var images = getImages(obj)
      var links = getLinks(obj)
      var title = getTitle(obj) || 'VikACG'
      var dateVal = getDate(obj)
      var tags = getTags(obj)
      var html = getContentHtml(obj)
      var excerpt = getExcerpt(obj)
      var section = obj && obj.metadata && obj.metadata.section
      var pageUrl = obj && obj.metadata && obj.metadata.page_url

      // 使用闭包管理当前图片索引
      var state = { currentIdx: 0 }

      // 更新图片函数
      function updateMainImage(el) {
        if (el && images[state.currentIdx]) {
          el.src = images[state.currentIdx]
        }
      }

      return h('div', {
        class: 'fixed inset-0 bg-black bg-opacity-70 z-50 flex items-center justify-center p-4 backdrop-blur-sm',
        on: {
          click: function (e) { if (e.target === e.currentTarget && onClose) onClose() },
        }
      }, [
        h('div', { class: 'bg-white rounded-lg shadow-2xl w-full max-w-5xl max-h-[90vh] overflow-hidden flex flex-col' }, [
          // Header
          h('div', { class: 'p-4 border-b flex justify-between items-center bg-gray-50' }, [
            h('div', { class: 'flex items-center gap-3' }, [
              h('h3', { class: 'text-lg font-bold text-gray-800' }, title),
              section ? h('span', { class: 'px-2 py-0.5 text-xs rounded bg-blue-50 text-blue-600' }, section) : null,
              dateVal ? h('span', { class: 'text-xs text-gray-500' }, dateVal) : null,
            ]),
            onClose ? h('button', { class: 'text-gray-500 hover:text-gray-700', on: { click: function (e) { e.stopPropagation(); onClose() } } }, [h('i', { class: 'fas fa-times' })]) : null,
          ]),
          // Body
          h('div', { class: 'flex-1 overflow-y-auto' }, [
            images.length > 0 ? h('div', { class: 'bg-gray-100' }, [
              // Main image
              h('div', { class: 'aspect-[16/9] bg-black flex items-center justify-center' }, [
                h('img', {
                  ref: 'mainImage',
                  attrs: { src: images[0] },
                  class: 'w-full h-full object-contain'
                })
              ]),
              // Navigation
              images.length > 1 ? h('div', { class: 'p-2 flex items-center justify-between' }, [
                h('div', { class: 'text-xs text-gray-500' }, function () {
                  return (state.currentIdx + 1) + ' / ' + images.length
                }),
                h('div', { class: 'flex gap-2' }, [
                  h('button', {
                    class: 'px-2 py-1 text-xs bg-white border rounded hover:bg-gray-50',
                    on: { click: function () {
                      state.currentIdx = (state.currentIdx - 1 + images.length) % images.length
                      var imgEl = document.querySelector('[ref="mainImage"]')
                      if (imgEl) imgEl.src = images[state.currentIdx]
                    } }
                  }, '上一张'),
                  h('button', {
                    class: 'px-2 py-1 text-xs bg-white border rounded hover:bg-gray-50',
                    on: { click: function () {
                      state.currentIdx = (state.currentIdx + 1) % images.length
                      var imgEl = document.querySelector('[ref="mainImage"]')
                      if (imgEl) imgEl.src = images[state.currentIdx]
                    } }
                  }, '下一张'),
                ])
              ]) : null,
              // Thumbnails
              images.length > 1 ? h('div', { class: 'p-2 grid grid-cols-6 gap-2' }, images.map(function (img, idx) {
                return h('img', {
                  attrs: { src: img },
                  class: 'w-full h-16 object-cover rounded cursor-pointer border',
                  style: { borderColor: idx === 0 ? '#3b82f6' : '#e5e7eb' },
                  on: { click: function () {
                    state.currentIdx = idx
                    var imgEl = document.querySelector('[ref="mainImage"]')
                    if (imgEl) imgEl.src = images[state.currentIdx]
                  } }
                })
              })) : null,
            ]) : null,
            // Content
            h('div', { class: 'p-4 space-y-3' }, [
              // HTML or excerpt
              html ? h('div', { class: 'text-sm text-gray-700 whitespace-pre-line' }, html.replace(/<[^>]+>/g, ' ')) :
              excerpt ? h('div', { class: 'text-sm text-gray-700 whitespace-pre-line' }, excerpt) : null,
              // Tags
              tags.length ? h('div', [
                h('div', { class: 'text-xs text-gray-500 mb-1' }, '标签'),
                h('div', { class: 'flex flex-wrap gap-1' }, tags.map(function (tag) {
                  return h('span', { class: 'text-xs bg-gray-100 text-gray-600 px-2 py-0.5 rounded' }, '#' + tag)
                }))
              ]) : null,
              // Links
              links.length ? h('div', [
                h('div', { class: 'text-xs text-gray-500 mb-1' }, '相关链接'),
                h('div', { class: 'flex flex-col gap-1' }, links.map(function (l) {
                  return h('a', {
                    attrs: { href: /^https?:\/\//i.test(l.href) ? l.href : '#', target: '_blank', rel: 'noopener noreferrer' },
                    class: 'text-xs text-blue-600 hover:text-blue-800 break-all'
                  }, l.text || l.href)
                }))
              ]) : null,
            ]),
          ]),
          // Footer
          h('div', { class: 'p-3 border-t bg-gray-50 flex justify-between items-center' }, [
            h('div', { class: 'flex gap-2' }, [
              pageUrl ? h('a', {
                attrs: { href: /^https?:\/\//i.test(pageUrl) ? pageUrl : '#', target: '_blank', rel: 'noopener noreferrer' },
                class: 'px-3 py-1.5 rounded bg-blue-600 text-white text-sm hover:bg-blue-700'
              }, '打开原页面') : null,
              pageUrl ? h('button', {
                class: 'px-3 py-1.5 rounded bg-white border text-sm hover:bg-gray-100',
                on: { click: function () { if (navigator.clipboard && navigator.clipboard.writeText) navigator.clipboard.writeText(pageUrl) } }
              }, '复制链接') : null,
            ]),
            onClose ? h('button', {
              class: 'px-3 py-1.5 rounded bg-white border text-sm hover:bg-gray-100',
              on: { click: function (e) { e.stopPropagation(); onClose() } }
            }, '关闭') : null,
          ]),
        ])
      ])
    }
  })

  // 兼容：保留 __dm_uiBridge 注册
  if (window.__dm_uiBridge) {
    window.__dm_uiBridge.register('vikacg', {
      label: '浏览',
      open: function (obj) {
        var handler = TaskUI.get('vikacg')
        if (handler && handler.renderViewer) {
          var container = document.getElementById('custom-ui-content')
          if (container) container.innerHTML = ''
          var vm = Vue.createApp({
            render: function(h) {
              return handler.renderViewer(h, obj, function() {
                if (container) container.innerHTML = ''
                vm.unmount()
              })
            }
          }).mount(container)
        }
      }
    })
  }
})()