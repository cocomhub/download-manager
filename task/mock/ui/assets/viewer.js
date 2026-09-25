// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
// mock 任务查看器：视频播放（模拟视频任务，供 e2e 使用）。
(function () {
  'use strict'
  window.TaskUI = window.TaskUI || {}
  window.TaskUI.register('mock', {
    shouldShowViewer: function (obj) {
      return !!(obj && obj.extra && obj.extra.files && obj.extra.files.some(function (f) {
        return f && (f.type === 'video' || (f.path && /\.(mp4|webm|m3u8|mkv)$/i.test(f.path)))
      }))
    },
    renderViewer: function (h, obj) {
      var files = (obj.extra && obj.extra.files) || []
      var videoUrl = ''
      var cover = ''
      for (var i = 0; i < files.length; i++) {
        var f = files[i]
        if (f && f.type === 'video' && f.path) {
          videoUrl = window.__dm_pathToUrl ? window.__dm_pathToUrl(f.path) : f.path
        }
        if (f && f.type === 'image' && f.path && !cover) {
          cover = window.__dm_pathToUrl ? window.__dm_pathToUrl(f.path) : f.path
        }
      }
      if (!videoUrl && obj.extra && obj.extra.preview_url) videoUrl = obj.extra.preview_url
      if (!videoUrl && obj.url) videoUrl = obj.url
      var el = []
      if (videoUrl) {
        // 视频 + poster（有 cover 用 cover，否则灰色占位模拟 poster）
        el.push(h('img', {
          attrs: { src: cover || undefined, style: cover ? 'max-width:100%;border-radius:8px;margin-bottom:8px' : 'width:100%;height:200px;background:#e5e7eb;border-radius:8px;margin-bottom:8px' }
        }))
        el.push(h('video', {
          attrs: { src: videoUrl, controls: true, poster: cover || undefined, style: 'width:100%;max-height:60vh;border-radius:8px;background:#000' }
        }))
      } else if (cover) {
        el.push(h('img', { attrs: { src: cover, style: 'max-width:100%;border-radius:8px' } }))
      }
      var meta = obj.metadata || {}
      var rows = []
      if (meta.title) rows.push(['标题', meta.title])
      if (meta.content_group) rows.push(['分组', meta.content_group])
      if (meta.resolution) rows.push(['清晰度', meta.resolution])
      rows.forEach(function (r) {
        el.push(h('div', { style: 'font-size:13px;color:#374151;margin-top:6px' }, [
          h('span', { style: 'color:#6b7280' }, r[0] + ': '),
          h('span', {}, r[1])
        ]))
      })
      return h('div', {}, el)
    }
  })
})()
