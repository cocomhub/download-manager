// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * UiVideoPlayer — 纯 DOM 视频播放器模块。
 * 从 videoPlayer.js 的 mixin 中提取，消除 Vue.mixin 依赖。
 *
 * 状态管理：使用一个独立的 state 对象，Vue 应用通过引用此对象来同步。
 */
;(function () {
  'use strict'

  // ---- 默认设置 ----
  var DEFAULT_SETTINGS = {
    skipInterval: 10,
    defaultSpeed: 1.0,
    defaultVolume: 0.1,
    autoPlay: true
  }

  // ---- 视频播放器状态 ----
  function createState () {
    return {
      currentVideo: null,
      isPlaying: false,
      isBuffering: false,
      currentTime: 0,
      duration: 0,
      buffered: 0,
      volume: 0.1,
      isMuted: false,
      playbackRate: 1.0,
      showControls: true,
      controlsTimer: null,
      showPlayIcon: false,
      hoverTime: null,
      hoverProgressPosition: 0,
      showVideoSettings: false,
      videoSettings: Object.assign({}, DEFAULT_SETTINGS),
      collectionList: []
    }
  }

  // ---- 视频元素引用 ----
  var _videoRef = null
  function getVideo () { return _videoRef || document.querySelector('#video-player') }
  function setVideo (el) { _videoRef = el }

  // ---- 设置持久化 ----
  function loadVideoSettings (state) {
    try {
      var saved = localStorage.getItem('dm_video_settings')
      if (saved) {
        state.videoSettings = Object.assign({}, state.videoSettings, JSON.parse(saved))
      }
    } catch (e) { console.error('Failed to load video settings', e) }
  }

  function saveVideoSettings (state) {
    try { localStorage.setItem('dm_video_settings', JSON.stringify(state.videoSettings)) }
    catch (e) { console.error('Failed to save video settings', e) }
  }

  function resetVideoSettings (state) {
    state.videoSettings = Object.assign({}, DEFAULT_SETTINGS)
    saveVideoSettings(state)
  }

  // ---- 播放器方法 ----
  function onLoadedMetadata (state) {
    var video = getVideo()
    if (!video) return
    state.duration = video.duration
    if (state.currentVideo) {
      video.playbackRate = state.videoSettings.defaultSpeed
      state.playbackRate = state.videoSettings.defaultSpeed
      video.volume = state.videoSettings.defaultVolume
      state.volume = state.videoSettings.defaultVolume
      if (state.videoSettings.autoPlay) {
        video.play().catch(function () {})
        state.isPlaying = true
      } else {
        video.pause()
        state.isPlaying = false
      }
    }
    state.isMuted = video.muted
  }

  function updateProgress (state) {
    var video = getVideo()
    if (!video) return
    state.currentTime = video.currentTime
    if (video.buffered.length > 0) {
      for (var i = 0; i < video.buffered.length; i++) {
        if (video.buffered.start(i) <= video.currentTime && video.buffered.end(i) >= video.currentTime) {
          state.buffered = video.buffered.end(i)
          break
        }
      }
    }
  }

  function onEnded (state) {
    state.isPlaying = false
    state.showControls = true
  }

  function togglePlay (state) {
    var video = getVideo()
    if (!video) return
    if (video.paused) { video.play(); state.isPlaying = true }
    else { video.pause(); state.isPlaying = false }
    state.showPlayIcon = true
    setTimeout(function () { state.showPlayIcon = false }, 500)
  }

  function seekClick (state, e) {
    var rect = e.currentTarget.getBoundingClientRect()
    var percent = (e.clientX - rect.left) / rect.width
    var video = getVideo()
    if (video) video.currentTime = percent * state.duration
  }

  function handleHoverProgress (state, e) {
    var rect = e.currentTarget.getBoundingClientRect()
    var percent = (e.clientX - rect.left) / rect.width
    state.hoverProgressPosition = Math.min(Math.max(percent * 100, 0), 100)
    var time = percent * state.duration
    state.hoverTime = Math.max(0, Math.min(time, state.duration))
  }

  function skip (state, seconds) {
    var video = getVideo()
    var s = Number(seconds)
    if (video && !isNaN(s)) video.currentTime += s
  }

  function setSpeed (state, rate) {
    var video = getVideo()
    if (video) { video.playbackRate = rate; state.playbackRate = rate }
  }

  function toggleMute (state) {
    var video = getVideo()
    if (!video) return
    video.muted = !video.muted
    state.isMuted = video.muted
    if (!state.isMuted && state.volume === 0) { state.volume = 1; video.volume = 1 }
  }

  function updateVolume (state) {
    var video = getVideo()
    if (video) {
      video.volume = state.volume
      state.isMuted = video.volume === 0
      video.muted = state.isMuted
    }
  }

  function toggleFullscreen () {
    var video = getVideo()
    if (!video) return
    var container = video.parentElement && video.parentElement.parentElement
    var target = container || video
    if (!document.fullscreenElement) {
      if (target.requestFullscreen) target.requestFullscreen()
      else if (video.requestFullscreen) video.requestFullscreen()
    } else {
      document.exitFullscreen()
    }
  }

  function onMouseMove (state) {
    state.showControls = true
    if (state.controlsTimer) clearTimeout(state.controlsTimer)
    state.controlsTimer = setTimeout(function () {
      if (state.isPlaying) state.showControls = false
    }, 3000)
  }

  function formatTime (seconds) {
    if (!seconds || isNaN(seconds)) return '00:00'
    var h = Math.floor(seconds / 3600)
    var m = Math.floor((seconds % 3600) / 60)
    var s = Math.floor(seconds % 60)
    if (h > 0) return h + ':' + m.toString().padStart(2, '0') + ':' + s.toString().padStart(2, '0')
    return m.toString().padStart(2, '0') + ':' + s.toString().padStart(2, '0')
  }

  function handleKeydown (state, e) {
    if (!state.currentVideo) return
    if (['Space', 'ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'KeyF', 'KeyK', 'KeyJ', 'KeyL', 'KeyM'].indexOf(e.code) >= 0) {
      e.preventDefault()
    }
    var key = e.key.toLowerCase()
    var code = e.code
    if (code === 'Space' || key === 'k') { togglePlay(state) }
    else if (code === 'ArrowRight' || key === 'l') { skip(state, state.videoSettings.skipInterval) }
    else if (code === 'ArrowLeft' || key === 'j') { skip(state, -state.videoSettings.skipInterval) }
    else if (code === 'ArrowUp') { state.volume = Math.min(1, state.volume + 0.1); updateVolume(state) }
    else if (code === 'ArrowDown') { state.volume = Math.max(0, state.volume - 0.1); updateVolume(state) }
    else if (key === 'f') { toggleFullscreen() }
    else if (key === 'm') { toggleMute(state) }
    else if (code === 'Escape') { closeVideo(state) }
  }

  function playPrev (state) {
    var idx = state.collectionList.findIndex(function (o) { return o.id === state.currentVideo.id })
    if (idx > 0) {
      switchToCollectionItem(state, state.collectionList[idx - 1])
    }
  }

  function playNext (state) {
    var idx = state.collectionList.findIndex(function (o) { return o.id === state.currentVideo.id })
    if (idx < state.collectionList.length - 1) {
      switchToCollectionItem(state, state.collectionList[idx + 1])
    }
  }

  function switchToCollectionItem (state, item) {
    var type = state.currentVideo && state.currentVideo.metadata && state.currentVideo.metadata.task_type
    if (!type) { type = 'hanime' }
    AppAPI.getObject(type, item.id).then(function (obj) {
      state.currentVideo = obj
    }).catch(function () {
      state.currentVideo = item
    })
  }

  function closeVideo (state) {
    state.currentVideo = null
    state.isPlaying = false
  }

  function playVideo (state, obj) {
    state.currentVideo = obj
    state.isPlaying = false
  }

  function isVideo (obj) {
    if (!obj) return false
    if (obj.extra && Array.isArray(obj.extra.files) && obj.extra.files.some(function (f) { return f && f.type === 'video' })) return true
    if (obj.save_path) {
      var sp = obj.save_path.toLowerCase()
      if (sp.indexOf('.mp4') > 0 || sp.indexOf('.webm') > 0 || sp.indexOf('.m3u8') > 0 || sp.indexOf('.mkv') > 0 || sp.indexOf('.ts') > 0) return true
    }
    if (obj.url) {
      var url = obj.url.toLowerCase()
      return url.indexOf('.mp4') > 0 || url.indexOf('.webm') > 0 || url.indexOf('.m3u8') > 0
    }
    return false
  }

  function getVideoUrl (obj) {
    if (!obj) return ''
    if (obj.extra && Array.isArray(obj.extra.files)) {
      for (var fi = 0; fi < obj.extra.files.length; fi++) {
        var f = obj.extra.files[fi]
        if (f && (f.type === 'video' || (f.path && /\.(mp4|webm|mkv|m3u8|ts)$/i.test(f.path)))) {
          if (f.path) return UiHelpers.pathToUrl(f.path, window.__dm_runtime)
        }
      }
    }
    if (obj.save_path) return UiHelpers.pathToUrl(obj.save_path, window.__dm_runtime)
    if (obj.url) return obj.url
    return ''
  }

  function getThumbImage (obj) {
    if (!obj) return ''
    if (obj.extra && obj.extra.thumb_url) return obj.extra.thumb_url
    if (obj.extra && obj.extra.local_cover) return UiHelpers.pathToUrl(obj.extra.local_cover, window.__dm_runtime)
    if (obj.extra && obj.extra.local_preview) return UiHelpers.pathToUrl(obj.extra.local_preview, window.__dm_runtime)
    if (obj.extra && obj.extra.cover_url) return obj.extra.cover_url
    if (obj.extra && obj.extra.cover) return obj.extra.cover
    if (obj.extra && obj.extra.preview_url) return obj.extra.preview_url
    if (obj.extra && obj.extra.local_url) return UiHelpers.pathToUrl(obj.extra.local_url, window.__dm_runtime)
    return getCoverImage(obj)
  }

  function getCoverImage (obj) {
    if (!obj) return ''
    var candidates = []
    function pushUrl (u) { if (typeof u === 'string' && u) candidates.push(u) }
    if (obj.extra) {
      if (obj.extra.local_cover) { return UiHelpers.pathToUrl(obj.extra.local_cover, window.__dm_runtime) }
      if (obj.extra.cover_url) return obj.extra.cover_url
      if (obj.extra.cover) return obj.extra.cover
      if (Array.isArray(obj.extra.files)) {
        for (var fi = 0; fi < obj.extra.files.length; fi++) {
          var f = obj.extra.files[fi]
          if (f && f.type === 'image' && f.path) {
            var fname = (f.name || f.path || '').toString().toLowerCase()
            if (fname.indexOf('cover') >= 0 || fname.indexOf('thumb') >= 0) {
              pushUrl(UiHelpers.pathToUrl(f.path, window.__dm_runtime))
            }
          }
        }
        if (candidates.length === 0) {
          for (var fi2 = 0; fi2 < obj.extra.files.length; fi2++) {
            var f2 = obj.extra.files[fi2]
            if (f2 && f2.type === 'image' && f2.path) {
              pushUrl(UiHelpers.pathToUrl(f2.path, window.__dm_runtime))
            }
          }
        }
      }
      if (obj.extra.cover_images && Array.isArray(obj.extra.cover_images)) obj.extra.cover_images.forEach(pushUrl)
      if (obj.extra.cover_urls && Array.isArray(obj.extra.cover_urls)) obj.extra.cover_urls.forEach(pushUrl)
      if (obj.extra.covers && Array.isArray(obj.extra.covers)) obj.extra.covers.forEach(pushUrl)
      if (obj.extra.thumb_url) pushUrl(obj.extra.thumb_url)
      if (obj.extra.preview_url) pushUrl(obj.extra.preview_url)
    }
    return candidates[0] || ''
  }

  function onCoverError (event) {
    event.target.style.display = 'none'
  }

  function getPreviewUrl (obj) {
    if (!obj) return ''
    if (obj.extra && obj.extra.local_preview) return UiHelpers.pathToUrl(obj.extra.local_preview, window.__dm_runtime)
    if (obj.extra && obj.extra.preview_url) return obj.extra.preview_url
    return ''
  }

  // ---- Export ----

  window.UiVideoPlayer = {
    createState: createState,
    setVideo: setVideo,
    loadVideoSettings: loadVideoSettings,
    saveVideoSettings: saveVideoSettings,
    resetVideoSettings: resetVideoSettings,
    onLoadedMetadata: onLoadedMetadata,
    updateProgress: updateProgress,
    onEnded: onEnded,
    togglePlay: togglePlay,
    seekClick: seekClick,
    handleHoverProgress: handleHoverProgress,
    skip: skip,
    setSpeed: setSpeed,
    toggleMute: toggleMute,
    updateVolume: updateVolume,
    toggleFullscreen: toggleFullscreen,
    onMouseMove: onMouseMove,
    formatTime: formatTime,
    handleKeydown: handleKeydown,
    playPrev: playPrev,
    playNext: playNext,
    switchToCollectionItem: switchToCollectionItem,
    closeVideo: closeVideo,
    playVideo: playVideo,
    isVideo: isVideo,
    getVideoUrl: getVideoUrl,
    getThumbImage: getThumbImage,
    getCoverImage: getCoverImage,
    onCoverError: onCoverError,
    getPreviewUrl: getPreviewUrl,
  }
})()