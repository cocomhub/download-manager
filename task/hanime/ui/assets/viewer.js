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

  function fileUrl (path) { return window.__dm_pathToUrl ? window.__dm_pathToUrl(path) : fileUrl_impl(path) }
  function fileUrl_impl(path) {
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
    // Local downloaded files — cover before thumb
    if (obj && obj.status === 'completed' && obj.extra) {
      if (obj.extra.local_cover) push(fileUrl(obj.extra.local_cover))
      if (Array.isArray(obj.extra.files)) {
        // First pass: cover-named files
        obj.extra.files.forEach(function (f) {
          var name = (f.name || f.path || '').toString().toLowerCase()
          if (f.type === 'image' && name.indexOf('cover') >= 0) {
            if (f.path) push(fileUrl(f.path))
          }
        })
        // Second pass: thumb-named files (non-cover)
        obj.extra.files.forEach(function (f) {
          var name = (f.name || f.path || '').toString().toLowerCase()
          if (f.type === 'image' && name.indexOf('thumb') >= 0 && name.indexOf('cover') < 0) {
            if (f.path) push(fileUrl(f.path))
          }
        })
      }
    }
    // Fall back to remote origin URLs (cover fields before thumb fields)
    if (imgs.length === 0 && obj && obj.extra) {
      if (Array.isArray(obj.extra.cover_images)) obj.extra.cover_images.forEach(push)
      if (Array.isArray(obj.extra.cover_urls)) obj.extra.cover_urls.forEach(push)
      if (Array.isArray(obj.extra.covers)) obj.extra.covers.forEach(push)
      if (obj.extra.cover_url) push(obj.extra.cover_url)
      if (obj.extra.cover) push(obj.extra.cover)
      // thumb fields after cover fields
      if (obj.extra.thumb_url) push(obj.extra.thumb_url)
    }
    var uniq = [], seen = {}
    imgs.forEach(function (u) { if (u && !seen[u]) { seen[u] = true; uniq.push(u) } })
    return uniq
  }

  function getThumbImages (obj) {
    var imgs = []
    var push = function (u) { if (typeof u === 'string' && u) imgs.push(u) }
    // Local downloaded files — thumb before cover
    if (obj && obj.status === 'completed' && obj.extra) {
      if (obj.extra.local_preview) push(fileUrl(obj.extra.local_preview))
      if (obj.extra.local_cover) push(fileUrl(obj.extra.local_cover))
      if (Array.isArray(obj.extra.files)) {
        // First pass: thumb-named files (non-cover)
        obj.extra.files.forEach(function (f) {
          var name = (f.name || f.path || '').toString().toLowerCase()
          if (f.type === 'image' && name.indexOf('thumb') >= 0 && name.indexOf('cover') < 0) {
            if (f.path) push(fileUrl(f.path))
          }
        })
        // Second pass: cover-named files
        obj.extra.files.forEach(function (f) {
          var name = (f.name || f.path || '').toString().toLowerCase()
          if (f.type === 'image' && name.indexOf('cover') >= 0) {
            if (f.path) push(fileUrl(f.path))
          }
        })
      }
    }
    // Fall back to remote origin URLs (thumb fields before cover fields)
    if (imgs.length === 0 && obj && obj.extra) {
      if (obj.extra.thumb_url) push(obj.extra.thumb_url)
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

    // Add CSS for semi-transparent video controls
    var style = document.createElement('style')
    style.textContent = '.dm-video-player::-webkit-media-controls { opacity:0.6 !important; transition:opacity 0.3s } .dm-video-player::-webkit-media-controls:hover { opacity:1 !important } .dm-video-player::-webkit-media-controls-panel { background:rgba(0,0,0,0.3) !important }'
    document.head.appendChild(style)

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
    hTitle.classList.add('viewer-title')
    hTitle.textContent = getTitle(obj) || 'Hanime'
    header.appendChild(hTitle)
    var hClose = document.createElement('button')
    hClose.innerHTML = '<i class="fas fa-times"></i>'
    hClose.style.cssText = 'color:#6b7280;cursor:pointer;background:none;border:none;font-size:18px'
    hClose.onclick = closeModal
    header.appendChild(hClose)
    panel.appendChild(header)

    // Body - two-column layout: left = media + details, right = playlist
    var body = document.createElement('div')
    body.style.cssText = 'flex:1;overflow:hidden;padding:0;display:flex'

    // Left column: media + details
    var leftCol = document.createElement('div')
    leftCol.style.cssText = 'flex:1;overflow-y:auto'

    // Video area - 16:9 aspect ratio
    var mediaArea = document.createElement('div')
    mediaArea.style.cssText = 'background:#000;display:flex;align-items:center;justify-content:center;position:relative;aspect-ratio:16/9;overflow:hidden'
    mediaArea.classList.add('viewer-media-area')
    if (useVideo) {
      var posterImg = document.createElement('img')
      posterImg.src = firstPoster || videoUrl
      posterImg.style.cssText = 'width:100%;height:100%;object-fit:contain;cursor:pointer'
      posterImg.alt = getTitle(obj) || 'Hanime'
      mediaArea.appendChild(posterImg)

      var playOverlay = document.createElement('div')
      playOverlay.style.cssText = 'position:absolute;inset:0;display:flex;align-items:center;justify-content:center;background:rgba(0,0,0,0.2);cursor:pointer'
      playOverlay.innerHTML = '<i class="fas fa-play" style="font-size:48px;color:#fff;opacity:0.8;text-shadow:0 2px 8px rgba(0,0,0,0.5)"></i>'
      mediaArea.appendChild(playOverlay)

      var video = document.createElement('video')
      video.src = videoUrl
      video.poster = firstPoster
      video.controls = true
      video.style.cssText = 'width:100%;height:100%;outline:none;display:none'
      // Semi-transparent video controls to avoid blocking subtitles
      video.classList.add('dm-video-player')

      var playHandler = function () {
        posterImg.style.display = 'none'
        playOverlay.style.display = 'none'
        video.style.display = 'block'
        video.play().catch(function () {})
      }
      posterImg.onclick = playHandler
      playOverlay.onclick = playHandler
      mediaArea.appendChild(video)
    } else if (covers.length > 0) {
      var posterImg = document.createElement('img')
      posterImg.src = firstPoster
      posterImg.style.cssText = 'width:100%;height:100%;object-fit:contain'
      mediaArea.appendChild(posterImg)

      if (!canPlay) {
        var noPlay = document.createElement('div')
        noPlay.style.cssText = 'text-align:center;padding:12px;background:#fef3c7;color:#92400e;font-size:14px'
        noPlay.textContent = '暂无可用视频源'
        leftCol.appendChild(noPlay)
      } else if (isHLS && !isSafari) {
        var hlsMsg = document.createElement('div')
        hlsMsg.style.cssText = 'text-align:center;padding:8px;background:#fef3c7;color:#92400e;font-size:12px'
        hlsMsg.textContent = 'HLS 流无法在此浏览器直接播放。'
        var downLink = document.createElement('a')
        downLink.href = /^https?:\/\//i.test(videoUrl) ? videoUrl : "#"
        downLink.target = '_blank'
        downLink.rel = 'noopener noreferrer'
        downLink.style.cssText = 'color:#2563eb;text-decoration:underline'
        downLink.textContent = '下载视频文件'
        hlsMsg.appendChild(document.createTextNode(' '))
        hlsMsg.appendChild(downLink)
        leftCol.appendChild(hlsMsg)
      }
    }
    leftCol.appendChild(mediaArea)

    // Content area (tags + details below media)
    var content = document.createElement('div')
    content.style.cssText = 'padding:16px'

    // Metadata row (genres, date, artist only — tags shown as chips below)
    var metaRow = document.createElement('div')
    metaRow.style.cssText = 'font-size:13px;color:#6b7280;margin-bottom:12px;display:flex;flex-wrap:wrap;gap:4px'
    metaRow.classList.add('viewer-meta')

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
    if (artist) {
      if (genres.length || dateVal) {
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

    leftCol.appendChild(content)
    body.appendChild(leftCol)

    // Right column: collection + recommendation panels
    var rightCol = document.createElement('div')
    rightCol.style.cssText = 'width:320px;border-left:1px solid #e5e7eb;display:flex;flex-direction:column;overflow:hidden;flex-shrink:0;background:#fff'

    var collectionPanel = null
    var recommendationPanel = null
    var taskType = obj && obj.metadata && obj.metadata.task_type
    var objTags = []
    if (obj && obj.extra && Array.isArray(obj.extra.tags)) {
      objTags = obj.extra.tags
    }

    // Collection panel
    if (taskType && obj) {
      collectionPanel = CollectionPanel.create({
        type: taskType,
        currentId: obj.id,
        onPlayItem: function (item) {
          AppAPI.getObject(taskType, item.id).then(function (newObj) {
            closeModal()
            createModal(newObj)
          })
        }
      })
      rightCol.appendChild(collectionPanel.element)
    }

    // Recommendation panel
    if (taskType && obj) {
      recommendationPanel = RecommendationPanel.create({
        type: taskType,
        currentId: obj.id,
        tags: objTags,
        onPlayItem: function (item) {
          AppAPI.getObject(taskType, item.id).then(function (newObj) {
            closeModal()
            createModal(newObj)
          })
        }
      })
      rightCol.appendChild(recommendationPanel.element)
    }

    body.appendChild(rightCol)

    panel.appendChild(body)

    // Footer
    var footer = document.createElement('div')
    footer.style.cssText = 'padding:12px 16px;border-top:1px solid #e5e7eb;background:#f9fafb;display:flex;justify-content:space-between;align-items:center'

    var fLeft = document.createElement('div')
    fLeft.style.cssText = 'display:flex;gap:8px'

    if (origin) {
      var openBtn = document.createElement('a')
      openBtn.href = /^https?:\/\//i.test(origin) ? origin : "#"
      openBtn.target = '_blank'
      openBtn.rel = 'noopener noreferrer'
      openBtn.style.cssText = 'padding:6px 12px;border-radius:6px;background:#2563eb;color:#fff;text-decoration:none;font-size:14px;display:inline-block'
      openBtn.textContent = '打开原页面'
      fLeft.appendChild(openBtn)

      var copyLinkBtn = document.createElement('button')
      copyLinkBtn.style.cssText = 'padding:6px 12px;border-radius:6px;background:#fff;border:1px solid #d1d5db;cursor:pointer;font-size:14px'
      copyLinkBtn.textContent = '复制链接'
      copyLinkBtn.onclick = function () {
        if (navigator.clipboard && navigator.clipboard.writeText) navigator.clipboard.writeText(origin)
      }
      fLeft.appendChild(copyLinkBtn)
    }

    var copyTitleBtn = document.createElement('button')
    copyTitleBtn.style.cssText = 'padding:6px 12px;border-radius:6px;background:#fff;border:1px solid #d1d5db;cursor:pointer;font-size:14px'
    copyTitleBtn.textContent = '复制标题'
    copyTitleBtn.onclick = function () {
      var t = getTitle(obj)
      if (t && navigator.clipboard && navigator.clipboard.writeText) navigator.clipboard.writeText(t)
    }
    fLeft.appendChild(copyTitleBtn)

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
      if (collectionPanel) collectionPanel.destroy()
      if (recommendationPanel) recommendationPanel.destroy()
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
      viewerLabel: '查看',
      shouldShowViewer: function (obj) { return obj.status === 'completed' },
      onClick: function (obj, helpers) {
        if (obj.status !== 'completed') return false
        helpers.openTaskTypeViewer(obj)
        return true
      },
      renderViewer: function (h, obj, onClose) {
        // Shared clipboard helper
        function copyToClipboard(text) {
          if (!text) return
          if (navigator.clipboard && navigator.clipboard.writeText) {
            navigator.clipboard.writeText(text).catch(function () {})
          } else {
            var ta = document.createElement('textarea')
            ta.value = text
            ta.style.position = 'fixed'
            ta.style.opacity = '0'
            document.body.appendChild(ta)
            ta.select()
            try { document.execCommand('copy') } catch (e) {}
            document.body.removeChild(ta)
          }
        }
        // Use DOM-based approach (same as createModal) since Vue 3 CDN
        // createApp's render function cannot properly bind on:{} event
        // handlers when returning VNode trees from plugin functions.
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

        // Build DOM modal directly (same pattern as createModal)
        var overlay = document.createElement('div')
        overlay.className = 'fixed inset-0 bg-black bg-opacity-70 z-50 flex items-center justify-center p-4 backdrop-blur-sm'
        overlay.style.cssText = 'position:fixed;inset:0;background:rgba(0,0,0,0.7);z-index:50;display:flex;align-items:center;justify-content:center;padding:16px;-webkit-backdrop-filter:blur(4px);backdrop-filter:blur(4px)'

        var panel = document.createElement('div')
        panel.className = 'bg-white rounded-lg shadow-2xl w-full max-w-5xl max-h-[90vh] overflow-hidden flex flex-col'
        panel.style.cssText = 'background:#fff;border-radius:8px;box-shadow:0 25px 50px rgba(0,0,0,0.25);width:100%;max-width:1200px;max-height:90vh;overflow:hidden;display:flex;flex-direction:column'

        // Store reference for cleanup
        var modalRef = { overlay: overlay, panel: panel }

        // Override onClose to remove DOM elements
        var originalOnClose = onClose
        onClose = function () {
          if (modalRef.overlay && modalRef.overlay.parentNode) {
            modalRef.overlay.parentNode.removeChild(modalRef.overlay)
          }
          document.body.style.overflow = ''
          if (originalOnClose) originalOnClose()
        }
        overlay.appendChild(panel)

        // Header
        var header = document.createElement('div')
        header.style.cssText = 'padding:16px;border-bottom:1px solid #e5e7eb;display:flex;justify-content:space-between;align-items:center;background:#f9fafb'
        var hTitle = document.createElement('h3')
        hTitle.style.cssText = 'font-size:18px;font-weight:700;color:#1f2937;margin:0'
        hTitle.classList.add('viewer-title')
        hTitle.textContent = getTitle(obj) || 'Hanime'
        header.appendChild(hTitle)
        var hClose = document.createElement('button')
        hClose.innerHTML = '<i class="fas fa-times"></i>'
        hClose.style.cssText = 'color:#6b7280;cursor:pointer;background:none;border:none;font-size:18px'
        hClose.onclick = function (e) { e.stopPropagation(); onClose() }
        header.appendChild(hClose)
        panel.appendChild(header)

        // Body - two-column layout: left = media + details, right = playlist
        var body = document.createElement('div')
        body.style.cssText = 'flex:1;overflow:hidden;padding:0;display:flex'

        // Left column: media + details
        var leftCol = document.createElement('div')
        leftCol.style.cssText = 'flex:1;overflow-y:auto'

        // Media area — show poster with play overlay, click to play
        var mediaArea = document.createElement('div')
        mediaArea.style.cssText = 'background:#000;display:flex;align-items:center;justify-content:center;position:relative;aspect-ratio:16/9;overflow:hidden'
        mediaArea.classList.add('viewer-media-area')
        if (useVideo) {
          var posterImg = document.createElement('img')
          posterImg.src = firstPoster || videoUrl
          posterImg.style.cssText = 'width:100%;height:100%;object-fit:contain;cursor:pointer'
          posterImg.alt = getTitle(obj) || 'Hanime'
          mediaArea.appendChild(posterImg)

          var playOverlay = document.createElement('div')
          playOverlay.style.cssText = 'position:absolute;inset:0;display:flex;align-items:center;justify-content:center;background:rgba(0,0,0,0.2);cursor:pointer'
          playOverlay.innerHTML = '<i class="fas fa-play" style="font-size:48px;color:#fff;opacity:0.8;text-shadow:0 2px 8px rgba(0,0,0,0.5)"></i>'
          mediaArea.appendChild(playOverlay)

          var video = document.createElement('video')
          video.src = videoUrl
          video.poster = firstPoster
          video.controls = true
          video.style.cssText = 'width:100%;height:100%;outline:none;display:none'
          video.classList.add('dm-video-player')

          var playHandler = function () {
            posterImg.style.display = 'none'
            playOverlay.style.display = 'none'
            video.style.display = 'block'
            video.play().catch(function () {})
          }
          posterImg.onclick = playHandler
          playOverlay.onclick = playHandler
          mediaArea.appendChild(video)
        } else if (covers.length > 0) {
          var posterImg = document.createElement('img')
          posterImg.src = firstPoster
          posterImg.style.cssText = 'width:100%;height:100%;object-fit:contain'
          mediaArea.appendChild(posterImg)
        }
        leftCol.appendChild(mediaArea)

        // Content area (tags + details below media)
        var content = document.createElement('div')
        content.style.cssText = 'padding:16px'

        // Metadata row (genres, date, artist only)
        var metaRow = document.createElement('div')
        metaRow.style.cssText = 'font-size:13px;color:#6b7280;margin-bottom:12px;display:flex;flex-wrap:wrap;gap:4px'
        metaRow.classList.add('viewer-meta')
        if (genres.length) { var gs = document.createElement('span'); gs.textContent = genres.join(', '); metaRow.appendChild(gs) }
        if (dateVal) { if (genres.length) { var s1 = document.createElement('span'); s1.textContent = ' · '; metaRow.appendChild(s1) }; var ds = document.createElement('span'); ds.textContent = dateVal; metaRow.appendChild(ds) }
        if (artist) { if (genres.length || dateVal) { var s2 = document.createElement('span'); s2.textContent = ' · '; metaRow.appendChild(s2) }; var as = document.createElement('span'); as.textContent = artist; metaRow.appendChild(as) }
        content.appendChild(metaRow)

        // Details
        if (details) { var de = document.createElement('div'); de.style.cssText = 'font-size:14px;color:#374151;line-height:1.6;white-space:pre-line;margin-bottom:12px'; de.textContent = details; content.appendChild(de) }

        // Tags chips
        if (tags.length > 0) {
          var tagWrap = document.createElement('div'); tagWrap.style.cssText = 'display:flex;flex-wrap:wrap;gap:4px;margin-bottom:12px'
          tags.forEach(function (tag) { var t = document.createElement('span'); t.style.cssText = 'font-size:11px;background:#f3f4f6;color:#4b5563;padding:2px 8px;border-radius:4px'; t.textContent = '#' + tag; tagWrap.appendChild(t) })
          content.appendChild(tagWrap)
        }
        leftCol.appendChild(content)
        body.appendChild(leftCol)

        // Right column: collection + recommendation panels
        var rightCol = document.createElement('div')
        rightCol.style.cssText = 'width:320px;border-left:1px solid #e5e7eb;display:flex;flex-direction:column;overflow:hidden;flex-shrink:0;background:#fff'

        var collectionPanel = null
        var recommendationPanel = null
        var taskType = obj && obj.metadata && obj.metadata.task_type
        var objTags = []
        if (obj && obj.extra && Array.isArray(obj.extra.tags)) {
          objTags = obj.extra.tags
        }

        // Collection panel
        if (taskType && obj) {
          collectionPanel = CollectionPanel.create({
            type: taskType,
            currentId: obj.id,
            onPlayItem: function (item) {
              AppAPI.getObject(taskType, item.id).then(function (newObj) {
                onClose()
                createModal(newObj)
              })
            }
          })
          rightCol.appendChild(collectionPanel.element)
        }

        // Recommendation panel
        if (taskType && obj) {
          recommendationPanel = RecommendationPanel.create({
            type: taskType,
            currentId: obj.id,
            tags: objTags,
            onPlayItem: function (item) {
              AppAPI.getObject(taskType, item.id).then(function (newObj) {
                onClose()
                createModal(newObj)
              })
            }
          })
          rightCol.appendChild(recommendationPanel.element)
        }

        body.appendChild(rightCol)

        panel.appendChild(body)

        // Footer
        var footer = document.createElement('div')
        footer.style.cssText = 'padding:12px 16px;border-top:1px solid #e5e7eb;background:#f9fafb;display:flex;justify-content:space-between;align-items:center'
        var fLeft = document.createElement('div'); fLeft.style.cssText = 'display:flex;gap:8px'
        if (origin) {
          var openBtn = document.createElement('a'); openBtn.href = /^https?:\/\//i.test(origin) ? origin : '#'; openBtn.target = '_blank'; openBtn.rel = 'noopener noreferrer'; openBtn.style.cssText = 'padding:6px 12px;border-radius:6px;background:#2563eb;color:#fff;text-decoration:none;font-size:14px;display:inline-block'; openBtn.textContent = '打开原页面'; fLeft.appendChild(openBtn)
          var copyLinkBtn = document.createElement('button'); copyLinkBtn.style.cssText = 'padding:6px 12px;border-radius:6px;background:#fff;border:1px solid #d1d5db;cursor:pointer;font-size:14px'; copyLinkBtn.textContent = '复制链接'; copyLinkBtn.onclick = function () { copyToClipboard(origin) }; fLeft.appendChild(copyLinkBtn)
        }
        var copyTitleBtn = document.createElement('button'); copyTitleBtn.style.cssText = 'padding:6px 12px;border-radius:6px;background:#fff;border:1px solid #d1d5db;cursor:pointer;font-size:14px'; copyTitleBtn.textContent = '复制标题'; copyTitleBtn.onclick = function () { copyToClipboard(getTitle(obj)) }; fLeft.appendChild(copyTitleBtn)
        footer.appendChild(fLeft)
        var closeBtn = document.createElement('button'); closeBtn.style.cssText = 'padding:6px 12px;border-radius:6px;background:#fff;border:1px solid #d1d5db;cursor:pointer;font-size:14px'; closeBtn.textContent = '关闭'; closeBtn.onclick = function (e) { e.stopPropagation(); onClose() }; footer.appendChild(closeBtn)
        panel.appendChild(footer)

        // Backdrop click
        overlay.addEventListener('click', function (e) { if (e.target === overlay) onClose() })

        // Escape key
        var keyHandler = function(e) {
          if (e.key === 'Escape') onClose()
        }
        document.addEventListener('keydown', keyHandler)

        // Override onClose to clean up
        var origOnClose_ = onClose
        onClose = function () {
          document.removeEventListener('keydown', keyHandler)
          if (collectionPanel) collectionPanel.destroy()
          if (recommendationPanel) recommendationPanel.destroy()
          if (modalRef.overlay && modalRef.overlay.parentNode) {
            modalRef.overlay.parentNode.removeChild(modalRef.overlay)
          }
          document.body.style.overflow = ''
          if (origOnClose_) origOnClose_()
        }

        // Append to document
        document.body.appendChild(overlay)
        document.body.style.overflow = 'hidden'

        // Return empty VNode (the modal is already in the DOM)
        return h('div')
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