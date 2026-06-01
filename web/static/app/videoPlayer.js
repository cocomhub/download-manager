// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * Video player module — registers player methods on the Vue app.
 * Usage: AppVideoPlayer.register(app)
 * Depends on: AppAPI
 */
;(function () {
  'use strict'

  var player = {
    register: function (app) {
      // ---- data ----
      app.data(function () {
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
          videoSettings: {
            skipInterval: 10,
            defaultSpeed: 1.0,
            defaultVolume: 0.1,
            autoPlay: true
          }
        }
      })

      // ---- methods ----
      app.methods = Object.assign(app.methods || {}, {
        loadVideoSettings: function () {
          try {
            var saved = localStorage.getItem('dm_video_settings')
            if (saved) {
              this.videoSettings = Object.assign({}, this.videoSettings, JSON.parse(saved))
            }
          } catch (e) { console.error('Failed to load video settings', e) }
        },
        saveVideoSettings: function () {
          try { localStorage.setItem('dm_video_settings', JSON.stringify(this.videoSettings)) }
          catch (e) { console.error('Failed to save video settings', e) }
        },
        resetVideoSettings: function () {
          this.videoSettings = { skipInterval: 10, defaultSpeed: 1.0, defaultVolume: 0.1, autoPlay: true }
          this.saveVideoSettings()
        },

        onLoadedMetadata: function () {
          var video = this.$refs.videoPlayer
          if (!video) return
          this.duration = video.duration
          if (this.currentVideo) {
            video.playbackRate = this.videoSettings.defaultSpeed
            this.playbackRate = this.videoSettings.defaultSpeed
            video.volume = this.videoSettings.defaultVolume
            this.volume = this.videoSettings.defaultVolume
            if (this.videoSettings.autoPlay) {
              video.play().catch(function () {})
              this.isPlaying = true
            } else {
              video.pause()
              this.isPlaying = false
            }
          }
          this.isMuted = video.muted
        },

        updateProgress: function () {
          var video = this.$refs.videoPlayer
          if (!video) return
          this.currentTime = video.currentTime
          if (video.buffered.length > 0) {
            for (var i = 0; i < video.buffered.length; i++) {
              if (video.buffered.start(i) <= video.currentTime && video.buffered.end(i) >= video.currentTime) {
                this.buffered = video.buffered.end(i)
                break
              }
            }
          }
        },

        onEnded: function () {
          this.isPlaying = false
          this.showControls = true
        },

        togglePlay: function () {
          var video = this.$refs.videoPlayer
          if (!video) return
          if (video.paused) { video.play(); this.isPlaying = true }
          else { video.pause(); this.isPlaying = false }
          this.showPlayIcon = true
          var self = this
          setTimeout(function () { self.showPlayIcon = false }, 500)
        },

        seekClick: function (e) {
          var rect = e.currentTarget.getBoundingClientRect()
          var percent = (e.clientX - rect.left) / rect.width
          var video = this.$refs.videoPlayer
          if (video) video.currentTime = percent * this.duration
        },

        handleHoverProgress: function (e) {
          var rect = e.currentTarget.getBoundingClientRect()
          var percent = (e.clientX - rect.left) / rect.width
          this.hoverProgressPosition = Math.min(Math.max(percent * 100, 0), 100)
          var time = percent * this.duration
          this.hoverTime = Math.max(0, Math.min(time, this.duration))
          if (this.enablePreview && this.$refs.previewVideo) {
            if (this.previewTimer) clearTimeout(this.previewTimer)
            var self = this
            this.previewTimer = setTimeout(function () {
              if (self.$refs.previewVideo) self.$refs.previewVideo.currentTime = self.hoverTime
            }, 50)
          }
        },

        skip: function (seconds) {
          var video = this.$refs.videoPlayer
          var s = Number(seconds)
          if (video && !isNaN(s)) video.currentTime += s
        },

        setSpeed: function (rate) {
          var video = this.$refs.videoPlayer
          if (video) { video.playbackRate = rate; this.playbackRate = rate }
        },

        toggleMute: function () {
          var video = this.$refs.videoPlayer
          if (!video) return
          video.muted = !video.muted
          this.isMuted = video.muted
          if (!this.isMuted && this.volume === 0) { this.volume = 1; video.volume = 1 }
        },

        updateVolume: function () {
          var video = this.$refs.videoPlayer
          if (video) {
            video.volume = this.volume
            this.isMuted = video.volume === 0
            video.muted = this.isMuted
          }
        },

        toggleFullscreen: function () {
          var video = this.$refs.videoPlayer
          if (!video) return
          var container = video.parentElement.parentElement
          var target = container || video
          if (!document.fullscreenElement) {
            if (target.requestFullscreen) target.requestFullscreen()
            else if (video.requestFullscreen) video.requestFullscreen()
          } else {
            document.exitFullscreen()
          }
        },

        onMouseMove: function () {
          this.showControls = true
          if (this.controlsTimer) clearTimeout(this.controlsTimer)
          var self = this
          this.controlsTimer = setTimeout(function () {
            if (self.isPlaying) self.showControls = false
          }, 3000)
        },

        formatTime: function (seconds) {
          if (!seconds || isNaN(seconds)) return '00:00'
          var h = Math.floor(seconds / 3600)
          var m = Math.floor((seconds % 3600) / 60)
          var s = Math.floor(seconds % 60)
          if (h > 0) return h + ':' + m.toString().padStart(2, '0') + ':' + s.toString().padStart(2, '0')
          return m.toString().padStart(2, '0') + ':' + s.toString().padStart(2, '0')
        },

        handleKeydown: function (e) {
          if (!this.currentVideo) return
          if (['Space', 'ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'KeyF', 'KeyK', 'KeyJ', 'KeyL', 'KeyM'].indexOf(e.code) >= 0) {
            e.preventDefault()
          }
          var key = e.key.toLowerCase()
          var code = e.code
          if (code === 'Space' || key === 'k') { this.togglePlay() }
          else if (code === 'ArrowRight' || key === 'l') { this.skip(this.videoSettings.skipInterval) }
          else if (code === 'ArrowLeft' || key === 'j') { this.skip(-this.videoSettings.skipInterval) }
          else if (code === 'ArrowUp') { this.volume = Math.min(1, this.volume + 0.1); this.updateVolume() }
          else if (code === 'ArrowDown') { this.volume = Math.max(0, this.volume - 0.1); this.updateVolume() }
          else if (key === 'f') { this.toggleFullscreen() }
          else if (key === 'm') { this.toggleMute() }
          else if (code === 'Escape') { this.closeVideo() }
        },

        closeVideo: function () {
          this.currentVideo = null
          this.isPlaying = false
        },

        playVideo: function (obj) {
          this.currentVideo = obj
          this.isPlaying = false
          var self = this
          this.$nextTick(function () {
            window.addEventListener('keydown', self.handleKeydown)
          })
        },

        isVideo: function (obj) {
          if (!obj || !obj.url) return false
          var url = obj.url.toLowerCase()
          return url.indexOf('.mp4') > 0 || url.indexOf('.webm') > 0 || url.indexOf('.m3u8') > 0
        },

        getVideoUrl: function (obj) {
          if (obj && obj.save_path) return this.pathToUrl(obj.save_path)
          if (obj && obj.url) return obj.url
          return ''
        },

        getCoverImage: function (obj) {
          if (obj && obj.extra && obj.extra.preview_url) return obj.extra.preview_url
          if (obj && obj.extra && obj.extra.local_preview) return this.pathToUrl(obj.extra.local_preview)
          if (obj && obj.metadata && obj.metadata.page_url) {
            var u = obj.metadata.page_url
            if (u.indexOf('hanime1') > 0 && u.indexOf('/watch/') > 0) return 'https://i1.hanime1.me/thumbnails/' + u.split('/watch/').pop() + '.jpg'
          }
          return ''
        },

        getPreviewUrl: function (obj) {
          if (obj && obj.extra && obj.extra.preview_url) return obj.extra.preview_url
          if (obj && obj.extra && obj.extra.local_preview) return this.pathToUrl(obj.extra.local_preview)
          return ''
        }
      })
    }
  }

  window.AppVideoPlayer = player
})()