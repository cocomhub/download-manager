/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

;(function () {
  'use strict'

  if (!window.__dm_uiBridge) return

  // ---- Data helpers ----

  function getTitle (obj) { return (obj && obj.metadata && obj.metadata.title) || '' }

  function getTags (obj) {
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

  function getArtist (obj) {
    if (obj && obj.extra && obj.extra.artist) return obj.extra.artist
    if (obj && obj.metadata && obj.metadata.artist) return obj.metadata.artist
    if (obj && obj.metadata && Array.isArray(obj.metadata.authors) && obj.metadata.authors.length) return obj.metadata.authors.join(', ')
    return ''
  }

  function getDescription (obj) {
    var s = ''
    if (obj && obj.extra && obj.extra.description) s = obj.extra.description
    else if (obj && obj.metadata && obj.metadata.description) s = obj.metadata.description
    else if (obj && obj.extra && obj.extra.content_text) s = obj.extra.content_text
    return (typeof s === 'string' ? s : '').replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim()
  }

  function getOriginLink (obj) {
    if (obj && obj.metadata && obj.metadata.page_url) return obj.metadata.page_url
    if (obj && obj.extra && obj.extra.origin_url) return obj.extra.origin_url
    return (obj && obj.url) || ''
  }

  function fileUrl (path) {
    if (!path) return ''
    var normalized = path.replace(/\\/g, '/')
    var root = typeof window.__dm_downloadRoot === 'string' ? window.__dm_downloadRoot : ''
    if (root && normalized.indexOf(root) === 0) {
      normalized = normalized.slice(root.length)
    }
    normalized = normalized.replace(/^\//, '')
    return '/files/' + normalized.split('/').filter(function(s){return s&&s!=='..'}).map(encodeURIComponent).join('/')
  }

  function getCoverImages (obj) {
    var imgs = []
    var push = function (u) { if (typeof u === 'string' && u) imgs.push(u) }
    // Local downloaded files take priority when object is completed
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
    // Fall back to remote origin URLs (even for non-completed objects)
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

  function getVideoURL (obj) {
    if (!obj) return ''
    var u = obj.metadata && obj.metadata.video_url || ''
    if (!u && obj.extra && obj.extra.video_url) u = obj.extra.video_url
    if (obj.status === 'completed') {
      if (obj.save_path) {
        var ext = obj.save_path.split('.').pop().toLowerCase()
        if (ext === 'mp4' || ext === 'webm' || ext === 'mkv' || ext === 'm3u8') {
          return fileUrl(obj.save_path)
        }
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

  function getDetails (obj) {
    var s = ''
    if (obj && obj.extra && obj.extra.details) s = obj.extra.details
    else if (obj && obj.metadata && obj.metadata.details) s = obj.metadata.details
    else if (obj && obj.metadata && obj.metadata.description) s = obj.metadata.description
    else if (obj && obj.extra && obj.extra.description) s = obj.extra.description
    return (typeof s === 'string' ? s : '').replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim()
  }

  function getDate (obj) {
    if (obj && obj.extra && obj.extra.date) return obj.extra.date
    if (obj && obj.metadata && obj.metadata.date) return obj.metadata.date
    return ''
  }

  function getPlaylist (obj) {
    var src = (obj && obj.extra && obj.extra.playlist) || (obj && obj.metadata && obj.metadata.playlist) || []
    var items = []
    function norm (it) {
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
    if (Array.isArray(src)) {
      src.forEach(function (x) { var n = norm(x); if (n) items.push(n) })
    } else { var n = norm(src); if (n) items.push(n) }
    var seen = {}, out = []
    items.forEach(function (it) {
      var k = (it.title || '') + '|' + (it.url || '') + '|' + (it.thumbnail || '')
      if (!seen[k]) { seen[k] = true; out.push(it) }
    })
    return out
  }

  function getGenres (obj) {
    var vals = []
    function pushVal (v) {
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
  }

  // ---- Modal ----

  var activeModal = null

  function closeModal () {
    if (activeModal) {
      // Clean up video element
      var video = activeModal.querySelector('video')
      if (video) { video.pause(); video.src = '' }
      document.body.removeChild(activeModal)
      activeModal = null
      document.body.style.overflow = ''
    }
  }

  function createModal (obj) {
    closeModal()

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

    // Check if we can play video natively (not HLS on non-Safari)
    var canPlay = !!videoUrl
    var isHLS = /\.m3u8(\?.*)?$/i.test(videoUrl)
    var isSafari = /safari/i.test(navigator.userAgent) && !/chrome|crios|chromium|edg/i.test(navigator.userAgent)
    var useVideo = canPlay && (!isHLS || isSafari)

    // Overlay
    var overlay = document.createElement('div')
    overlay.style.cssText = 'position:fixed;inset:0;background:rgba(0,0,0,0.7);z-index:50;display:flex;align-items:center;justify-content:center;padding:16px;-webkit-backdrop-filter:blur(4px);backdrop-filter:blur(4px)'

    // Panel
    var panel = document.createElement('div')
    panel.style.cssText = 'background:#fff;border-radius:8px;box-shadow:0 25px 50px rgba(0,0,0,0.25);width:100%;max-width:1200px;max-height:90vh;overflow:hidden;display:flex;flex-direction:column'
    overlay.appendChild(panel)

    // Header
    var header = document.createElement('div')
    header.style.cssText = 'padding:16px;border-bottom:1px solid #e5e7eb;display:flex;justify-content:space-between;align-items:center;background:#f9fafb'
    var hTitle = document.createElement('h3')
    hTitle.style.cssText = 'font-size:18px;font-weight:700;color:#1f2937;margin:0'
    hTitle.textContent = getTitle(obj) || 'Hanime'
    header.appendChild(hTitle)
    var hClose = document.createElement('button')
    hClose.innerHTML = '<i class="fas fa-times"></i>'
    hClose.style.cssText = 'color:#6b7280;cursor:pointer;background:none;border:none;font-size:18px'
    hClose.onclick = closeModal
    header.appendChild(hClose)
    panel.appendChild(header)

    // Body (scrollable)
    var body = document.createElement('div')
    body.style.cssText = 'flex:1;overflow-y:auto;padding:0'

    // Media area
    if (useVideo) {
      var videoWrap = document.createElement('div')
      videoWrap.style.cssText = 'background:#000;display:flex;align-items:center;justify-content:center'
      var video = document.createElement('video')
      video.src = videoUrl
      video.poster = firstPoster
      video.controls = true
      video.autoplay = true
      video.style.cssText = 'width:100%;max-height:55vh;outline:none'
      videoWrap.appendChild(video)
      body.appendChild(videoWrap)
    } else if (covers.length > 0) {
      var posterWrap = document.createElement('div')
      posterWrap.style.cssText = 'background:#000;display:flex;align-items:center;justify-content:center;padding:16px;min-height:200px'
      var posterImg = document.createElement('img')
      posterImg.src = firstPoster
      posterImg.style.cssText = 'max-width:100%;max-height:50vh;object-fit:contain'
      posterWrap.appendChild(posterImg)
      body.appendChild(posterWrap)

      if (!canPlay) {
        var noPlay = document.createElement('div')
        noPlay.style.cssText = 'text-align:center;padding:12px;background:#fef3c7;color:#92400e;font-size:14px'
        noPlay.textContent = '暂无可用视频源'
        body.appendChild(noPlay)
      } else if (isHLS && !isSafari) {
        var hlsMsg = document.createElement('div')
        hlsMsg.style.cssText = 'text-align:center;padding:8px;background:#fef3c7;color:#92400e;font-size:12px'
        // Safe DOM construction
          hlsMsg.textContent = 'HLS 流无法在此浏览器直接播放。'
          var downLink = document.createElement('a')
          // Only allow http/https schemes
          downLink.href = /^https?:\/\//i.test(videoUrl) ? videoUrl : "#"
          downLink.target = '_blank'
        downLink.rel = 'noopener noreferrer'
          downLink.style.cssText = 'color:#2563eb;text-decoration:underline'
          downLink.textContent = '下载视频文件'
          hlsMsg.appendChild(document.createTextNode(' '))
          hlsMsg.appendChild(downLink)
        body.appendChild(hlsMsg)
      }
    }

    // Origin link bar
    if (origin) {
      var originBar = document.createElement('div')
      originBar.style.cssText = 'display:flex;gap:8px;padding:12px 16px;background:#f9fafb;border-bottom:1px solid #e5e7eb;flex-wrap:wrap'
      var originBtn = document.createElement('a')
      // Only allow http/https schemes
          originBtn.href = /^https?:\/\//i.test(origin) ? origin : "#"
      originBtn.target = '_blank'
          originBtn.rel = 'noopener noreferrer'
      originBtn.style.cssText = 'padding:4px 12px;border-radius:6px;background:#2563eb;color:#fff;text-decoration:none;font-size:13px;display:inline-block'
      originBtn.textContent = '打开原页面'
      originBar.appendChild(originBtn)

      var copyBtn = document.createElement('button')
      copyBtn.style.cssText = 'padding:4px 12px;border-radius:6px;background:#fff;border:1px solid #d1d5db;cursor:pointer;font-size:13px'
      copyBtn.textContent = '复制标题'
      copyBtn.onclick = function () {
        var t = getTitle(obj)
        if (t && navigator.clipboard && navigator.clipboard.writeText) navigator.clipboard.writeText(t)
      }
      originBar.appendChild(copyBtn)

      var copyLinkBtn = document.createElement('button')
      copyLinkBtn.style.cssText = 'padding:4px 12px;border-radius:6px;background:#fff;border:1px solid #d1d5db;cursor:pointer;font-size:13px'
      copyLinkBtn.textContent = '复制链接'
      copyLinkBtn.onclick = function () {
        if (navigator.clipboard && navigator.clipboard.writeText) navigator.clipboard.writeText(origin)
      }
      originBar.appendChild(copyLinkBtn)

      body.appendChild(originBar)
    }

    // Content area
    var content = document.createElement('div')
    content.style.cssText = 'padding:16px'

    // Metadata row
    var metaRow = document.createElement('div')
    metaRow.style.cssText = 'font-size:13px;color:#6b7280;margin-bottom:12px;display:flex;flex-wrap:wrap;gap:4px'

    if (genres.length) {
      var genresSpan = document.createElement('span')
      genresSpan.textContent = genres.join(', ')
      metaRow.appendChild(genresSpan)
    }
    if (dateVal) {
      if (genres.length) {
        var sep = document.createElement('span')
        sep.textContent = ' · '
        metaRow.appendChild(sep)
      }
      var dateSpan = document.createElement('span')
      dateSpan.textContent = dateVal
      metaRow.appendChild(dateSpan)
    }
    if (tags.length) {
      if (genres.length || dateVal) {
        var sep2 = document.createElement('span')
        sep2.textContent = ' · '
        metaRow.appendChild(sep2)
      }
      var tagsSpan = document.createElement('span')
      tagsSpan.textContent = tags.join(', ')
      metaRow.appendChild(tagsSpan)
    }
    if (artist) {
      if (genres.length || dateVal || tags.length) {
        var sep3 = document.createElement('span')
        sep3.textContent = ' · '
        metaRow.appendChild(sep3)
      }
      var artistSpan = document.createElement('span')
      artistSpan.textContent = artist
      metaRow.appendChild(artistSpan)
    }
    content.appendChild(metaRow)

    // Details
    if (details) {
      var detailsEl = document.createElement('div')
      detailsEl.style.cssText = 'font-size:14px;color:#374151;line-height:1.6;white-space:pre-line;margin-bottom:12px'
      detailsEl.textContent = details
      content.appendChild(detailsEl)
    }

    // Tags chips
    if (tags.length > 0) {
      var tagWrap = document.createElement('div')
      tagWrap.style.cssText = 'display:flex;flex-wrap:wrap;gap:4px;margin-bottom:12px'
      tags.forEach(function (tag) {
        var t = document.createElement('span')
        t.style.cssText = 'font-size:11px;background:#f3f4f6;color:#4b5563;padding:2px 8px;border-radius:4px'
        t.textContent = '#' + tag
        tagWrap.appendChild(t)
      })
      content.appendChild(tagWrap)
    }

    body.appendChild(content)

    // Playlist
    if (playlist.length > 0) {
      var listSection = document.createElement('div')
      listSection.style.cssText = 'padding:12px 16px;border-top:1px solid #e5e7eb;background:#f9fafb'
      var listTitle = document.createElement('h4')
      listTitle.style.cssText = 'font-size:14px;font-weight:600;color:#374151;margin:0 0 8px'
      listTitle.textContent = '播放列表 (' + playlist.length + ')'
      listSection.appendChild(listTitle)

      var listWrap = document.createElement('div')
      listWrap.style.cssText = 'display:flex;flex-direction:column;gap:4px;max-height:200px;overflow-y:auto'

      playlist.forEach(function (item, idx) {
        var itemEl = document.createElement('div')
        itemEl.style.cssText = 'display:flex;align-items:center;gap:8px;padding:8px;border:1px solid #e5e7eb;border-radius:6px;cursor:pointer;font-size:13px'
        itemEl.style.cssText += ';background:#fff'
        itemEl.onmouseenter = function () { itemEl.style.background = '#f9fafb' }
        itemEl.onmouseleave = function () { itemEl.style.background = '#fff' }

        var numSpan = document.createElement('span')
        numSpan.style.cssText = 'width:20px;height:20px;border-radius:50%;background:#e5e7eb;display:flex;align-items:center;justify-content:center;font-size:11px;color:#6b7280;flex-shrink:0'
        numSpan.textContent = idx + 1
        itemEl.appendChild(numSpan)

        if (item.thumbnail) {
          var thumb = document.createElement('img')
          thumb.src = item.thumbnail
          thumb.style.cssText = 'width:40px;height:28px;object-fit:cover;border-radius:4px;flex-shrink:0'
          itemEl.appendChild(thumb)
        }

        var titleSpan = document.createElement('span')
        titleSpan.style.cssText = 'flex:1;color:#374151;overflow:hidden;text-overflow:ellipsis;white-space:nowrap'
        titleSpan.textContent = item.title || ('Item ' + (idx + 1))
        itemEl.appendChild(titleSpan)

        if (item.url) {
          itemEl.onclick = function () { window.open(item.url, '_blank', 'noopener,noreferrer') }
          var iconSpan = document.createElement('span')
          iconSpan.innerHTML = '<i class="fas fa-external-link-alt"></i>'
          iconSpan.style.cssText = 'color:#9ca3af;font-size:11px;flex-shrink:0'
          itemEl.appendChild(iconSpan)
        }

        listWrap.appendChild(itemEl)
      })

      listSection.appendChild(listWrap)
      body.appendChild(listSection)
    }

    panel.appendChild(body)

    // Footer
    var footer = document.createElement('div')
    footer.style.cssText = 'padding:12px 16px;border-top:1px solid #e5e7eb;background:#f9fafb;display:flex;justify-content:space-between;align-items:center'

    var fLeft = document.createElement('div')
    fLeft.style.cssText = 'display:flex;gap:8px'

    if (origin) {
      var openBtn = document.createElement('a')
      // Only allow http/https schemes
          openBtn.href = /^https?:\/\//i.test(origin) ? origin : "#"
      openBtn.target = '_blank'
      openBtn.rel = 'noopener noreferrer'
      openBtn.style.cssText = 'padding:6px 12px;border-radius:6px;background:#2563eb;color:#fff;text-decoration:none;font-size:14px;display:inline-block'
      openBtn.textContent = '打开原页面'
      fLeft.appendChild(openBtn)
    }

    var copyTitleBtn = document.createElement('button')
    copyTitleBtn.style.cssText = 'padding:6px 12px;border-radius:6px;background:#fff;border:1px solid #d1d5db;cursor:pointer;font-size:14px'
    copyTitleBtn.textContent = '复制标题'
    copyTitleBtn.onclick = function () {
      var t = getTitle(obj)
      if (t && navigator.clipboard && navigator.clipboard.writeText) navigator.clipboard.writeText(t)
    }
    fLeft.appendChild(copyTitleBtn)

    if (origin) {
      var copyLinkBtn = document.createElement('button')
      copyLinkBtn.style.cssText = 'padding:6px 12px;border-radius:6px;background:#fff;border:1px solid #d1d5db;cursor:pointer;font-size:14px'
      copyLinkBtn.textContent = '复制链接'
      copyLinkBtn.onclick = function () {
        if (navigator.clipboard && navigator.clipboard.writeText) navigator.clipboard.writeText(origin)
      }
      fLeft.appendChild(copyLinkBtn)
    }

    footer.appendChild(fLeft)

    var closeBtn = document.createElement('button')
    closeBtn.style.cssText = 'padding:6px 12px;border-radius:6px;background:#fff;border:1px solid #d1d5db;cursor:pointer;font-size:14px'
    closeBtn.textContent = '关闭'
    closeBtn.onclick = closeModal
    footer.appendChild(closeBtn)

    panel.appendChild(footer)

    // Backdrop click
    overlay.addEventListener('click', function (e) {
      if (e.target === overlay) closeModal()
    })

    // Escape
    function keyHandler (e) {
      if (e.key === 'Escape') closeModal()
    }
    document.addEventListener('keydown', keyHandler)

    // Override closeModal for cleanup
    closeModal = function () {
      document.removeEventListener('keydown', keyHandler)
      document.body.style.overflow = ''
      if (activeModal) {
        document.body.removeChild(activeModal)
        activeModal = null
      }
    }

    document.body.appendChild(overlay)
    activeModal = overlay
    document.body.style.overflow = 'hidden'
  }

  // Register with TaskUI (new plugin system)
  if (typeof TaskUI !== 'undefined' && TaskUI.register) {
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
            h('div', { class: 'p-4 border-b flex justify-between items-center bg-gray-50' }, [
              h('h3', { class: 'text-lg font-bold text-gray-800' }, getTitle(obj) || 'Hanime'),
              onClose ? h('button', { class: 'text-gray-500 hover:text-gray-700', on: { click: function (e) { e.stopPropagation(); onClose() } } }, [h('i', { class: 'fas fa-times' })]) : null,
            ]),
            h('div', { class: 'flex-1 overflow-y-auto' }, [
              useVideo ? h('div', { class: 'bg-black flex items-center justify-center' }, [
                h('video', {
                  attrs: { src: videoUrl, poster: firstPoster, controls: true, autoplay: true },
                  class: 'w-full max-h-[55vh] outline-none'
                })
              ]) : (covers.length > 0 ? h('div', { class: 'bg-black flex items-center justify-center p-4 min-h-[200px]' }, [
                h('img', { attrs: { src: firstPoster }, class: 'max-w-full max-h-[50vh] object-contain' })
              ]) : null),
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
  }

  // Register with the bridge (legacy compat)
  window.__dm_uiBridge.register('hanime', {
    label: '播放',
    open: function (obj) {
      createModal(obj)
    }
  })
})()
