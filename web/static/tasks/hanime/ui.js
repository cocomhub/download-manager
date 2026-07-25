// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * hanime 类型 UI 组件
 * 自定义查看器 — 视频播放器 + 播放列表 + 元数据
 */
;(function () {
  'use strict'

  // ---- Helper functions ----
  function getTitle(obj) { return (obj && obj.metadata && obj.metadata.title) || '' }

  function getTags(obj) {
    var tags = []
    if (obj && obj.extra && Array.isArray(obj.extra.tags)) tags.push.apply(tags, obj.extra.tags)
    if (obj && obj.metadata && Array.isArray(obj.metadata.tags)) tags.push.apply(tags, obj.metadata.tags)
    var set = {}, out = []
    tags.forEach(function (t) {
      var s = (t || '').toString().trim()
      if (s && !set[s]) { set[s] = true; out.push(s) }
    })
    return out
  }

  function getArtist(obj) {
    if (obj && obj.extra && obj.extra.artist) return obj.extra.artist
    if (obj && obj.metadata && obj.metadata.artist) return obj.metadata.artist
    if (obj && obj.metadata && Array.isArray(obj.metadata.authors) && obj.metadata.authors.length) return obj.metadata.authors.join(', ')
    return ''
  }

  function getDescription(obj) {
    var s = ''
    if (obj && obj.extra && obj.extra.description) s = obj.extra.description
    else if (obj && obj.metadata && obj.metadata.description) s = obj.metadata.description
    else if (obj && obj.extra && obj.extra.content_text) s = obj.extra.content_text
    return (typeof s === 'string' ? s : '').replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim()
  }

  function getOriginLink(obj) {
    if (obj && obj.metadata && obj.metadata.page_url) return obj.metadata.page_url
    if (obj && obj.extra && obj.extra.origin_url) return obj.extra.origin_url
    return (obj && obj.url) || ''
  }

  function fileUrl(path) {
    if (!path) return ''
    var normalized = path.replace(/\\/g, '/')
    var root = typeof window.__dm_downloadRoot === 'string' ? window.__dm_downloadRoot : ''
    if (root && normalized.indexOf(root) === 0) { normalized = normalized.slice(root.length) }
    normalized = normalized.replace(/^\//, '')
    return '/files/' + normalized.split('/').filter(function(s){return s&&s!=='..'}).map(encodeURIComponent).join('/')
  }

  function getCoverImages(obj) {
    var imgs = []
    var push = function (u) { if (typeof u === 'string' && u) imgs.push(u) }
    if (obj && obj.status === 'completed' && obj.extra) {
      if (obj.extra.local_cover) push(fileUrl(obj.extra.local_cover))
      if (Array.isArray(obj.extra.files)) {
        obj.extra.files.forEach(function (f) {
          var name = (f.name || f.path || '').toString().toLowerCase()
          if (f.type === 'image' && (name.indexOf('cover') >= 0 || name.indexOf('thumb') >= 0)) {
            if (f.path) push(fileUrl(f.path))
          }
        })
      }
    }
    if (imgs.length === 0 && obj && obj.extra) {
      if (Array.isArray(obj.extra.cover_images)) obj.extra.cover_images.forEach(push)
      if (Array.isArray(obj.extra.cover_urls)) obj.extra.cover_urls.forEach(push)
      if (Array.isArray(obj.extra.covers)) obj.extra.covers.forEach(push)
      if (obj.extra.cover_url) push(obj.extra.cover_url)
      if (obj.extra.cover) push(obj.extra.cover)
    }
    var uniq = [], seen = {}
    imgs.forEach(function (u) { if (u && !seen[u]) { seen[u] = true; uniq.push(u) } })
    return uniq
  }

  function getVideoURL(obj) {
    if (!obj) return ''
    var u = obj.metadata && obj.metadata.video_url || ''
    if (!u && obj.extra && obj.extra.video_url) u = obj.extra.video_url
    if (obj.status === 'completed') {
      if (obj.save_path) {
        var ext = obj.save_path.split('.').pop().toLowerCase()
        if (ext === 'mp4' || ext === 'webm' || ext === 'mkv' || ext === 'm3u8') { return fileUrl(obj.save_path) }
      }
      if (obj.extra && Array.isArray(obj.extra.files)) {
        for (var fi = 0; fi < obj.extra.files.length; fi++) {
          var f = obj.extra.files[fi]
          if (f && (f.type === 'video' || (f.path && /\.(mp4|webm|mkv|m3u8|ts)$/i.test(f.path)))) {
            if (f.path) return fileUrl(f.path)
          }
        }
      }
      if (obj.extra && obj.extra.local_url) return fileUrl(obj.extra.local_url)
      if (obj.extra && obj.extra.file_url) return fileUrl(obj.extra.file_url)
      if (obj.path) return fileUrl(obj.path)
      if (obj.save_path && /\.(mp4|webm|mkv|m3u8|ts)$/i.test(obj.save_path)) return fileUrl(obj.save_path)
    }
    if (typeof u === 'string' && u) return u
    return ''
  }

  function getDetails(obj) {
    var s = ''
    if (obj && obj.extra && obj.extra.details) s = obj.extra.details
    else if (obj && obj.metadata && obj.metadata.details) s = obj.metadata.details
    else if (obj && obj.metadata && obj.metadata.description) s = obj.metadata.description
    else if (obj && obj.extra && obj.extra.description) s = obj.extra.description
    return (typeof s === 'string' ? s : '').replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim()
  }

  function getDate(obj) {
    if (obj && obj.extra && obj.extra.date) return obj.extra.date
    if (obj && obj.metadata && obj.metadata.date) return obj.metadata.date
    return ''
  }

  function getPlaylist(obj) {
    var src = (obj && obj.extra && obj.extra.playlist) || (obj && obj.metadata && obj.metadata.playlist) || []
    var items = []
    function norm(it) {
      if (!it) return null
      if (typeof it === 'string') {
        var s = it.trim()
        if (!s) return null
        if (/^https?:\/\//.test(s) || s.startsWith('/')) return { title: '', thumbnail: '', url: s }
        if (obj && obj.status === 'completed') return { title: '', thumbnail: '', url: fileUrl(s) }
        return { title: s, thumbnail: '', url: '' }
      }
      if (typeof it === 'object') {
        var title = it.title || it.name || it.label || ''
        var url = it.url || it.href || it.link || it.src || ''
        var thumb = it.thumbnail || it.thumb || it.image || it.cover || ''
        if (!url) {
          if (it.path && obj && obj.status === 'completed') url = fileUrl(it.path)
          else if (it.local_url && obj && obj.status === 'completed') url = fileUrl(it.local_url)
          else if (it.file_url && obj && obj.status === 'completed') url = fileUrl(it.file_url)
        } else {
          var pLike = typeof url === 'string' && url && !/^https?:\/\//.test(url) && !url.startsWith('/')
          if (pLike && obj && obj.status === 'completed') url = fileUrl(url)
        }
        if (typeof title !== 'string') title = ''
        if (typeof url !== 'string') url = ''
        if (typeof thumb !== 'string') thumb = ''
        if (!title && !url) return null
        return { title: title, thumbnail: thumb, url: url }
      }
      return null
    }
    if (Array.isArray(src)) { src.forEach(function (x) { var n = norm(x); if (n) items.push(n) }) }
    else { var n = norm(src); if (n) items.push(n) }
    var seen = {}, out = []
    items.forEach(function (it) {
      var k = (it.title || '') + '|' + (it.url || '') + '|' + (it.thumbnail || '')
      if (!seen[k]) { seen[k] = true; out.push(it) }
    })
    return out
  }

  function getGenres(obj) {
    var vals = []
    function pushVal(v) {
      if (Array.isArray(v)) { v.forEach(function (s) { pushVal(s) }); return }
      if (typeof v === 'string') { v.split(/[，、,|/]/).forEach(function (x) { var t = x.trim(); if (t) vals.push(t) }) }
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
    vals.forEach(function (s) { var t = (s || '').toString().trim(); if (t && !set[t]) { set[t] = true; out.push(t) } })
    return out
  }

  // ---- 注册 hanime UI ----
  TaskUI.register('hanime', {
    type: 'hanime',
    label: 'Hanime',
    icon: 'fa-film',
    viewerLabel: '播放',
    shouldShowViewer: function (obj) { return obj.status === 'completed' },
    renderViewer: function (h, obj, onClose) {
      var videoUrl = getVideoURL(obj)
      var covers = getCoverImages(obj)
      var firstPoster = covers.length > 0 ? covers[0] : ''
      var playlist = getPlaylist(obj)
      var details = getDetails(obj)
      var genres = getGenres(obj)
      var dateVal = getDate(obj)
      var tags = getTags(obj)
      var artist = getArtist(obj)
      var origin = getOriginLink(obj)

      var isHLS = /\.m3u8(\?.*)?$/i.test(videoUrl)
      var isSafari = /safari/i.test(navigator.userAgent) && !/chrome|crios|chromium|edg/i.test(navigator.userAgent)
      var useVideo = !!videoUrl && (!isHLS || isSafari)

      return h('div', {
        class: 'fixed inset-0 bg-black bg-opacity-70 z-50 flex items-center justify-center p-4 backdrop-blur-sm',
        on: {
          click: function (e) { if (e.target === e.currentTarget && onClose) onClose() },
        }
      }, [
        h('div', { class: 'bg-white rounded-lg shadow-2xl w-full max-w-5xl max-h-[90vh] overflow-hidden flex flex-col' }, [
          // Header
          h('div', { class: 'p-4 border-b flex justify-between items-center bg-gray-50' }, [
            h('h3', { class: 'text-lg font-bold text-gray-800' }, getTitle(obj) || 'Hanime'),
            onClose ? h('button', { class: 'text-gray-500 hover:text-gray-700', on: { click: function (e) { e.stopPropagation(); onClose() } } }, [h('i', { class: 'fas fa-times' })]) : null,
          ]),
          // Body
          h('div', { class: 'flex-1 overflow-y-auto' }, [
            // Video/Cover area
            useVideo ? h('div', { class: 'bg-black flex items-center justify-center' }, [
              h('video', {
                attrs: { src: videoUrl, poster: firstPoster, controls: true, autoplay: true },
                class: 'w-full max-h-[55vh] outline-none'
              })
            ]) : (covers.length > 0 ? h('div', { class: 'bg-black flex items-center justify-center p-4 min-h-[200px]' }, [
              h('img', { attrs: { src: firstPoster }, class: 'max-w-full max-h-[50vh] object-contain' })
            ]) : null),

            // Origin link bar
            origin ? h('div', { class: 'flex gap-2 p-3 bg-gray-50 border-b flex-wrap' }, [
              h('a', {
                attrs: { href: /^https?:\/\//i.test(origin) ? origin : '#', target: '_blank', rel: 'noopener noreferrer' },
                class: 'px-2 py-1 rounded bg-blue-600 text-white text-xs hover:bg-blue-700'
              }, '打开原页面'),
              h('button', {
                class: 'px-2 py-1 rounded bg-white border text-xs hover:bg-gray-100',
                on: { click: function () { if (navigator.clipboard && navigator.clipboard.writeText) navigator.clipboard.writeText(getTitle(obj)) } }
              }, '复制标题'),
              h('button', {
                class: 'px-2 py-1 rounded bg-white border text-xs hover:bg-gray-100',
                on: { click: function () { if (navigator.clipboard && navigator.clipboard.writeText) navigator.clipboard.writeText(origin) } }
              }, '复制链接'),
            ]) : null,

            // Metadata
            h('div', { class: 'p-4' }, [
              (genres.length || dateVal || tags.length || artist) ? h('div', { class: 'text-xs text-gray-500 mb-3 flex flex-wrap gap-1' }, [
                genres.length ? h('span', genres.join(', ')) : null,
                dateVal ? h('span', (genres.length ? ' · ' : '') + dateVal) : null,
                tags.length ? h('span', ((genres.length || dateVal) ? ' · ' : '') + tags.join(', ')) : null,
                artist ? h('span', ((genres.length || dateVal || tags.length) ? ' · ' : '') + artist) : null,
              ]) : null,
              details ? h('div', { class: 'text-sm text-gray-700 whitespace-pre-line mb-3' }, details) : null,
              tags.length ? h('div', { class: 'flex flex-wrap gap-1 mb-3' }, tags.map(function (tag) {
                return h('span', { class: 'text-xs bg-gray-100 text-gray-600 px-2 py-0.5 rounded' }, '#' + tag)
              })) : null,
            ]),

            // Playlist
            playlist.length ? h('div', { class: 'p-3 border-t bg-gray-50' }, [
              h('h4', { class: 'text-sm font-semibold text-gray-700 mb-2' }, '播放列表 (' + playlist.length + ')'),
              h('div', { class: 'flex flex-col gap-1 max-h-[200px] overflow-y-auto' }, playlist.map(function (item, idx) {
                return h('div', {
                  class: 'flex items-center gap-2 p-2 border rounded bg-white text-xs hover:bg-gray-50 cursor-pointer',
                  on: { click: function () { if (item.url) window.open(item.url, '_blank', 'noopener,noreferrer') } }
                }, [
                  h('span', { class: 'w-5 h-5 rounded-full bg-gray-200 flex items-center justify-center text-xs text-gray-600 flex-shrink-0' }, String(idx + 1)),
                  item.thumbnail ? h('img', { attrs: { src: item.thumbnail }, class: 'w-10 h-7 object-cover rounded flex-shrink-0' }) : null,
                  h('span', { class: 'flex-1 truncate text-gray-700' }, item.title || 'Item ' + (idx + 1)),
                ])
              }))
            ]) : null,
          ]),
          // Footer
          h('div', { class: 'p-3 border-t bg-gray-50 flex justify-between items-center' }, [
            h('div', { class: 'flex gap-2' }, [
              origin ? h('a', {
                attrs: { href: /^https?:\/\//i.test(origin) ? origin : '#', target: '_blank', rel: 'noopener noreferrer' },
                class: 'px-3 py-1.5 rounded bg-blue-600 text-white text-sm hover:bg-blue-700'
              }, '打开原页面') : null,
              h('button', {
                class: 'px-3 py-1.5 rounded bg-white border text-sm hover:bg-gray-100',
                on: { click: function () { if (navigator.clipboard && navigator.clipboard.writeText) navigator.clipboard.writeText(getTitle(obj)) } }
              }, '复制标题'),
              origin ? h('button', {
                class: 'px-3 py-1.5 rounded bg-white border text-sm hover:bg-gray-100',
                on: { click: function () { if (navigator.clipboard && navigator.clipboard.writeText) navigator.clipboard.writeText(origin) } }
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
    window.__dm_uiBridge.register('hanime', {
      label: '播放',
      open: function (obj) {
        var handler = TaskUI.get('hanime')
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