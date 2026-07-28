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

  // ---- Hanime-specific data helpers ----

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

  function getCoverImages (obj) {
    var imgs = []
    var push = function (u) { if (typeof u === 'string' && u) imgs.push(u) }
    if (obj && obj.status === 'completed' && obj.extra) {
      if (obj.extra.local_cover) push(D.fileUrl(obj.extra.local_cover))
      if (Array.isArray(obj.extra.files)) {
        obj.extra.files.forEach(function (f) {
          var name = (f.name || f.path || '').toString().toLowerCase()
          if (f.type === 'image' && name.indexOf('cover') >= 0) {
            if (f.path) push(D.fileUrl(f.path))
          }
        })
        obj.extra.files.forEach(function (f) {
          var name = (f.name || f.path || '').toString().toLowerCase()
          if (f.type === 'image' && name.indexOf('thumb') >= 0 && name.indexOf('cover') < 0) {
            if (f.path) push(D.fileUrl(f.path))
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
      if (obj.extra.thumb_url) push(obj.extra.thumb_url)
    }
    var uniq = [], seen = {}
    imgs.forEach(function (u) { if (u && !seen[u]) { seen[u] = true; uniq.push(u) } })
    return uniq
  }

  function getThumbImages (obj) {
    var imgs = []
    var push = function (u) { if (typeof u === 'string' && u) imgs.push(u) }
    if (obj && obj.status === 'completed' && obj.extra) {
      if (obj.extra.local_preview) push(D.fileUrl(obj.extra.local_preview))
      if (obj.extra.local_cover) push(D.fileUrl(obj.extra.local_cover))
      if (Array.isArray(obj.extra.files)) {
        obj.extra.files.forEach(function (f) {
          var name = (f.name || f.path || '').toString().toLowerCase()
          if (f.type === 'image' && name.indexOf('thumb') >= 0 && name.indexOf('cover') < 0) {
            if (f.path) push(D.fileUrl(f.path))
          }
        })
        obj.extra.files.forEach(function (f) {
          var name = (f.name || f.path || '').toString().toLowerCase()
          if (f.type === 'image' && name.indexOf('cover') >= 0) {
            if (f.path) push(D.fileUrl(f.path))
          }
        })
      }
    }
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
          return D.fileUrl(obj.save_path)
        }
      }
      if (obj.extra && Array.isArray(obj.extra.files)) {
        for (var fi = 0; fi < obj.extra.files.length; fi++) {
          var f = obj.extra.files[fi]
          if (f && (f.type === 'video' || (f.path && /\.(mp4|webm|mkv|m3u8|ts)$/i.test(f.path)))) {
            if (f.path) return D.fileUrl(f.path)
          }
        }
      }
      if (obj.extra && obj.extra.local_url) return D.fileUrl(obj.extra.local_url)
      if (obj.extra && obj.extra.file_url) return D.fileUrl(obj.extra.file_url)
      if (obj.path) return D.fileUrl(obj.path)
      if (obj.save_path && /\.(mp4|webm|mkv|m3u8|ts)$/i.test(obj.save_path)) return D.fileUrl(obj.save_path)
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

  function getPlaylist (obj) {
    var src = (obj && obj.extra && obj.extra.playlist) || (obj && obj.metadata && obj.metadata.playlist) || []
    var items = []
    function norm (it) {
      if (!it) return null
      if (typeof it === 'string') {
        var s = it.trim()
        if (!s) return null
        if (/^https?:\/\//.test(s) || s.startsWith('/')) return { title: '', thumbnail: '', url: s }
        if (obj && obj.status === 'completed') return { title: '', thumbnail: '', url: D.fileUrl(s) }
        return { title: s, thumbnail: '', url: '' }
      }
      if (typeof it === 'object') {
        var title = it.title || it.name || it.label || ''
        var url = it.url || it.href || it.link || it.src || ''
        var thumb = it.thumbnail || it.thumb || it.image || it.cover || ''
        if (!url) {
          if (it.path && obj && obj.status === 'completed') url = D.fileUrl(it.path)
          else if (it.local_url && obj && obj.status === 'completed') url = D.fileUrl(it.local_url)
          else if (it.file_url && obj && obj.status === 'completed') url = D.fileUrl(it.file_url)
        } else {
          var pLike = typeof url === 'string' && url && !/^https?:\/\//.test(url) && !url.startsWith('/')
          if (pLike && obj && obj.status === 'completed') url = D.fileUrl(url)
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
    var dateVal = D.getDate(obj)
    var tags = D.getTags(obj)
    var artist = getArtist(obj)
    var origin = D.getOriginLink(obj)

    var canPlay = !!videoUrl
    var isHLS = /\.m3u8(\?.*)?$/i.test(videoUrl)
    var isSafari = /safari/i.test(navigator.userAgent) && !/chrome|crios|chromium|edg/i.test(navigator.userAgent)
    var useVideo = canPlay && (!isHLS || isSafari)

    // Add CSS for semi-transparent video controls
    var style = document.createElement('style')
    style.textContent = '.dm-video-player::-webkit-media-controls { opacity:0.6 !important; transition:opacity 0.3s } .dm-video-player::-webkit-media-controls:hover { opacity:1 !important } .dm-video-player::-webkit-media-controls-panel { background:rgba(0,0,0,0.3) !important }'
    document.head.appendChild(style)

    var overlay = M.createOverlay()
    var panel = M.createPanel('1400px')
    overlay.appendChild(panel)

    // Header
    var header = M.createHeader({ title: D.getTitle(obj) || 'Hanime', onClose: closeModal })
    panel.appendChild(header)

    // Body - two-column layout
    var body = document.createElement('div')
    body.style.cssText = 'flex:1;overflow:hidden;padding:0;display:flex'

    var leftCol = document.createElement('div')
    leftCol.style.cssText = 'flex:1;overflow-y:auto'

    // Video area
    var mediaArea = document.createElement('div')
    mediaArea.style.cssText = 'background:#000;display:flex;align-items:center;justify-content:center;position:relative;aspect-ratio:16/9;overflow:hidden'
    mediaArea.classList.add('viewer-media-area')
    if (useVideo) {
      var posterImg = document.createElement('img')
      posterImg.src = firstPoster || videoUrl
      posterImg.style.cssText = 'width:100%;height:100%;object-fit:contain;cursor:pointer'
      posterImg.alt = D.getTitle(obj) || 'Hanime'
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
        downLink.href = /^https?:\/\//i.test(videoUrl) ? videoUrl : '#'
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

    // Content area
    var content = document.createElement('div')
    content.style.cssText = 'padding:16px'

    // Metadata row
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

    if (details) {
      var detailsEl = document.createElement('div')
      detailsEl.style.cssText = 'font-size:14px;color:#374151;line-height:1.6;white-space:pre-line;margin-bottom:12px'
      detailsEl.textContent = details
      content.appendChild(detailsEl)
    }

    if (tags.length > 0) {
      var tagWrap = document.createElement('div')
      tagWrap.style.cssText = 'display:flex;flex-wrap:wrap;gap:4px;margin-bottom:12px'
      tagWrap.appendChild(Dm.createTagChips(tags))
      content.appendChild(tagWrap)
    }

    leftCol.appendChild(content)
    body.appendChild(leftCol)

    // Right column: collection + recommendation
    var rightCol = document.createElement('div')
    rightCol.style.cssText = 'width:380px;border-left:1px solid #e5e7eb;display:flex;flex-direction:column;overflow:hidden;flex-shrink:0;background:#fff'

    var collectionPanel = null
    var recommendationPanel = null
    var taskType = obj && obj.metadata && obj.metadata.task_type
    var objTags = []
    if (obj && obj.extra && Array.isArray(obj.extra.tags)) {
      objTags = obj.extra.tags
    }

    if (taskType && obj) {
      collectionPanel = window.CollectionPanel && window.CollectionPanel.create({
        type: taskType,
        currentId: obj.id,
        onPlayItem: function (item) {
          AppAPI.getObject(taskType, item.id).then(function (newObj) {
            closeModal()
            createModal(newObj)
          })
        }
      })
      if (collectionPanel) rightCol.appendChild(collectionPanel.element)
    }

    if (taskType && obj) {
      recommendationPanel = window.RecommendationPanel && window.RecommendationPanel.create({
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
      if (recommendationPanel) rightCol.appendChild(recommendationPanel.element)
    }

    body.appendChild(rightCol)
    panel.appendChild(body)

    // Footer
    var footer = M.createFooter({
      leftButtons: [
        origin ? Dm.createLink(origin, '打开原页面', { primary: true }) : null,
        origin ? Dm.createButton('复制链接', function () { D.copyToClipboard(origin) }) : null,
        Dm.createButton('复制标题', function () { D.copyToClipboard(D.getTitle(obj)) }),
      ].filter(Boolean),
      onClose: closeModal
    })
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
      // Clean up video element
      if (activeModal) {
        var video = activeModal.querySelector('video')
        if (video) { video.pause(); video.src = '' }
      }
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

  // ---- Register with TaskUI ----
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
        var videoUrl = getVideoURL(obj)
        var covers = getCoverImages(obj)
        var firstPoster = covers.length > 0 ? covers[0] : ''
        var details = getDetails(obj)
        var genres = getGenres(obj)
        var dateVal = D.getDate(obj)
        var tags = D.getTags(obj)
        var artist = getArtist(obj)
        var origin = D.getOriginLink(obj)

        var isHLS = /\.m3u8(\?.*)?$/i.test(videoUrl)
        var isSafari = /safari/i.test(navigator.userAgent) && !/chrome|crios|chromium|edg/i.test(navigator.userAgent)
        var useVideo = !!videoUrl && (!isHLS || isSafari)

        var overlay = M.createOverlay()
        var panel = M.createPanel('1400px')
        overlay.appendChild(panel)

        var collectionPanel = null
        var recommendationPanel = null
        var keyHandler = null

        // Build the final onClose before any component captures it
        var originalOnClose = onClose
        onClose = function () {
          if (keyHandler) document.removeEventListener('keydown', keyHandler)
          if (collectionPanel) collectionPanel.destroy()
          if (recommendationPanel) recommendationPanel.destroy()
          if (overlay && overlay.parentNode) overlay.parentNode.removeChild(overlay)
          document.body.style.overflow = ''
          if (originalOnClose) originalOnClose()
        }

        // Header
        var header = M.createHeader({ title: D.getTitle(obj) || 'Hanime', onClose: onClose })
        panel.appendChild(header)

        // Body
        var body = document.createElement('div')
        body.style.cssText = 'flex:1;overflow:hidden;padding:0;display:flex'

        var leftCol = document.createElement('div')
        leftCol.style.cssText = 'flex:1;overflow-y:auto'

        var mediaArea = document.createElement('div')
        mediaArea.style.cssText = 'background:#000;display:flex;align-items:center;justify-content:center;position:relative;aspect-ratio:16/9;overflow:hidden'
        mediaArea.classList.add('viewer-media-area')
        if (useVideo) {
          var posterImg = document.createElement('img')
          posterImg.src = firstPoster || videoUrl
          posterImg.style.cssText = 'width:100%;height:100%;object-fit:contain;cursor:pointer'
          posterImg.alt = D.getTitle(obj) || 'Hanime'
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

        // Content
        var content = document.createElement('div')
        content.style.cssText = 'padding:16px'

        var metaRow = document.createElement('div')
        metaRow.style.cssText = 'font-size:13px;color:#6b7280;margin-bottom:12px;display:flex;flex-wrap:wrap;gap:4px'
        metaRow.classList.add('viewer-meta')
        if (genres.length) { var gs = document.createElement('span'); gs.textContent = genres.join(', '); metaRow.appendChild(gs) }
        if (dateVal) { if (genres.length) { var s1 = document.createElement('span'); s1.textContent = ' · '; metaRow.appendChild(s1) }; var ds = document.createElement('span'); ds.textContent = dateVal; metaRow.appendChild(ds) }
        if (artist) { if (genres.length || dateVal) { var s2 = document.createElement('span'); s2.textContent = ' · '; metaRow.appendChild(s2) }; var as = document.createElement('span'); as.textContent = artist; metaRow.appendChild(as) }
        content.appendChild(metaRow)

        if (details) { var de = document.createElement('div'); de.style.cssText = 'font-size:14px;color:#374151;line-height:1.6;white-space:pre-line;margin-bottom:12px'; de.textContent = details; content.appendChild(de) }

        if (tags.length > 0) {
          var tagWrap = document.createElement('div'); tagWrap.style.cssText = 'display:flex;flex-wrap:wrap;gap:4px;margin-bottom:12px'
          tagWrap.appendChild(Dm.createTagChips(tags))
          content.appendChild(tagWrap)
        }
        leftCol.appendChild(content)
        body.appendChild(leftCol)

        // Right column
        var rightCol = document.createElement('div')
        rightCol.style.cssText = 'width:380px;border-left:1px solid #e5e7eb;display:flex;flex-direction:column;overflow:hidden;flex-shrink:0;background:#fff'

        var collectionPanel = null
        var recommendationPanel = null
        var taskType = obj && obj.metadata && obj.metadata.task_type
        var objTags = []
        if (obj && obj.extra && Array.isArray(obj.extra.tags)) { objTags = obj.extra.tags }

        if (taskType && obj) {
          collectionPanel = window.CollectionPanel && window.CollectionPanel.create({
            type: taskType, currentId: obj.id,
            onPlayItem: function (item) {
              AppAPI.getObject(taskType, item.id).then(function (newObj) { onClose(); createModal(newObj) })
            }
          })
          if (collectionPanel) rightCol.appendChild(collectionPanel.element)
        }
        if (taskType && obj) {
          recommendationPanel = window.RecommendationPanel && window.RecommendationPanel.create({
            type: taskType, currentId: obj.id, tags: objTags,
            onPlayItem: function (item) {
              AppAPI.getObject(taskType, item.id).then(function (newObj) { onClose(); createModal(newObj) })
            }
          })
          if (recommendationPanel) rightCol.appendChild(recommendationPanel.element)
        }
        body.appendChild(rightCol)
        panel.appendChild(body)

        // Footer
        var footer = M.createFooter({
          leftButtons: [
            origin ? Dm.createLink(origin, '打开原页面', { primary: true }) : null,
            origin ? Dm.createButton('复制链接', function () { D.copyToClipboard(origin) }) : null,
            Dm.createButton('复制标题', function () { D.copyToClipboard(D.getTitle(obj)) }),
          ].filter(Boolean),
          onClose: onClose
        })
        panel.appendChild(footer)

        // Close handlers
        overlay.addEventListener('click', function (e) { if (e.target === overlay) onClose() })
        keyHandler = function(e) { if (e.key === 'Escape') onClose() }
        document.addEventListener('keydown', keyHandler)

        document.body.appendChild(overlay)
        document.body.style.overflow = 'hidden'
        return h ? h('div') : null
      }
    })
  }

  // Register with bridge (legacy compat)
  window.__dm_uiBridge.register('hanime', {
    label: '播放',
    open: function (obj) {
      createModal(obj)
    }
  })
})()