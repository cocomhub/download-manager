/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

;(function () {
  'use strict'

  if (!window.__dm_uiBridge) return

  /**
   * Extract images from a VikACG download object.
   * Returns an array of absolute URLs (string).
   */
  function getImages (obj) {
    var imgs = []
    // Local downloaded files take priority when object is completed
    if (obj && obj.status === 'completed' && obj.extra && Array.isArray(obj.extra.files)) {
      obj.extra.files.forEach(function (f) {
        if (f.type === 'image' && f.path) imgs.push(fileUrl(f.path))
      })
    }
    // Fall back to remote origin URLs
    if (imgs.length === 0 && obj && obj.extra && Array.isArray(obj.extra.images)) {
      obj.extra.images.forEach(function (u) {
        if (typeof u === 'string' && u) imgs.push(u)
      })
    }
    return imgs
  }

  /**
   * Extract links from a VikACG download object.
   */
  function getLinks (obj) {
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

  /**
   * Extract excerpt from a VikACG download object.
   */
  function getExcerpt (obj) {
    var s = (obj && obj.extra && obj.extra.content_text) || ''
    if (!s) return ''
    var t = s.replace(/\s+/g, ' ').trim()
    return t.length > 200 ? t.slice(0, 200) + '...' : t
  }

  /**
   * Extract content HTML from a VikACG download object.
   */
  function getContentHtml (obj) {
    var s = (obj && obj.extra && obj.extra.content_html) || ''
    return typeof s === 'string' ? s.trim() : ''
  }

  /**
   * Extract tags from a download object.
   */
  function getTags (obj) {
    if (obj && obj.extra && Array.isArray(obj.extra.tags)) return obj.extra.tags
    if (obj && obj.extra && typeof obj.extra.tags === 'string') return [obj.extra.tags]
    return []
  }

  /**
   * Extract title from a download object.
   */
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

  function getTitle (obj) {
    return (obj && obj.metadata && obj.metadata.title) || ''
  }

  /**
   * Extract date from a download object.
   */
  function getDate (obj) {
    return (obj && obj.metadata && obj.metadata.date) || ''
  }

  // ---- Modal creation ----

  var activeModal = null

  function closeModal () {
    if (activeModal) {
      document.body.removeChild(activeModal)
      activeModal = null
      document.body.style.overflow = ''
    }
  }

  function createModal (obj) {
    closeModal()
    var images = getImages(obj)
    if (images.length === 0) {
      // No images to display — open the origin page instead
      var pageUrl = obj && obj.metadata && obj.metadata.page_url
      if (pageUrl) window.open(pageUrl, '_blank', 'noopener,noreferrer')
      return
    }

    var currentIdx = 0
    var overlay = document.createElement('div')
    overlay.className = 'fixed inset-0 bg-black bg-opacity-70 z-50 flex items-center justify-center p-4 backdrop-blur-sm'
    overlay.style.cssText = 'position:fixed;inset:0;background:rgba(0,0,0,0.7);z-index:50;display:flex;align-items:center;justify-content:center;padding:16px;-webkit-backdrop-filter:blur(4px);backdrop-filter:blur(4px)'

    // Main panel
    var panel = document.createElement('div')
    panel.className = 'bg-white rounded-lg shadow-2xl w-full max-w-6xl max-h-[90vh] overflow-hidden flex flex-col'
    panel.style.cssText = 'background:#fff;border-radius:8px;box-shadow:0 25px 50px rgba(0,0,0,0.25);width:100%;max-width:1200px;max-height:90vh;overflow:hidden;display:flex;flex-direction:column'
    overlay.appendChild(panel)

    // Header
    var header = document.createElement('div')
    header.className = 'p-4 border-b flex justify-between items-center bg-gray-50'
    header.style.cssText = 'padding:16px;border-bottom:1px solid #e5e7eb;display:flex;justify-content:space-between;align-items:center;background:#f9fafb'

    var titleGroup = document.createElement('div')
    titleGroup.className = 'flex items-center gap-3'
    titleGroup.style.cssText = 'display:flex;align-items:center;gap:12px'

    var titleEl = document.createElement('h3')
    titleEl.className = 'text-lg font-bold text-gray-800'
    titleEl.style.cssText = 'font-size:18px;font-weight:700;color:#1f2937'
    titleEl.textContent = getTitle(obj) || 'VikACG'
    titleGroup.appendChild(titleEl)

    var section = obj && obj.metadata && obj.metadata.section
    if (section) {
      var sectionBadge = document.createElement('span')
      sectionBadge.className = 'px-2 py-0.5 text-xs rounded bg-blue-50 text-blue-600'
      sectionBadge.style.cssText = 'padding:2px 8px;font-size:12px;border-radius:4px;background:#eff6ff;color:#2563eb'
      sectionBadge.textContent = section
      titleGroup.appendChild(sectionBadge)
    }

    var dateVal = getDate(obj)
    if (dateVal) {
      var dateEl = document.createElement('span')
      dateEl.className = 'text-xs text-gray-500'
      dateEl.style.cssText = 'font-size:12px;color:#6b7280'
      dateEl.textContent = dateVal
      titleGroup.appendChild(dateEl)
    }

    header.appendChild(titleGroup)

    var closeBtn = document.createElement('button')
    closeBtn.innerHTML = '<i class="fas fa-times"></i>'
    closeBtn.className = 'text-gray-500 hover:text-gray-700'
    closeBtn.style.cssText = 'color:#6b7280;cursor:pointer;background:none;border:none;font-size:18px'
    closeBtn.onclick = closeModal
    header.appendChild(closeBtn)

    panel.appendChild(header)

    // Body
    var body = document.createElement('div')
    body.className = 'flex-1 overflow-y-auto p-0'
    body.style.cssText = 'flex:1;overflow-y:auto;padding:0'

    // Image area
    var imgArea = document.createElement('div')
    imgArea.className = 'relative bg-black flex items-center justify-center'
    imgArea.style.cssText = 'position:relative;background:#000;display:flex;align-items:center;justify-content:center;min-height:300px'

    var imgEl = document.createElement('img')
    imgEl.className = 'w-full h-full object-contain'
    imgEl.style.cssText = 'width:100%;height:100%;object-fit:contain;max-height:60vh'
    imgEl.src = images[0]
    imgEl.alt = getTitle(obj)
    imgArea.appendChild(imgEl)

    // Navigation arrows
    if (images.length > 1) {
      var prevBtn = document.createElement('button')
      prevBtn.innerHTML = '<i class="fas fa-chevron-left"></i>'
      prevBtn.style.cssText = 'position:absolute;left:8px;top:50%;transform:translateY(-50%);background:rgba(255,255,255,0.8);border:none;border-radius:50%;width:36px;height:36px;font-size:16px;cursor:pointer;display:flex;align-items:center;justify-content:center;color:#374151'
      prevBtn.onclick = function (e) { e.stopPropagation(); currentIdx = (currentIdx - 1 + images.length) % images.length; imgEl.src = images[currentIdx]; updateCounter(); updateThumbs() }
      imgArea.appendChild(prevBtn)

      var nextBtn = document.createElement('button')
      nextBtn.innerHTML = '<i class="fas fa-chevron-right"></i>'
      nextBtn.style.cssText = 'position:absolute;right:8px;top:50%;transform:translateY(-50%);background:rgba(255,255,255,0.8);border:none;border-radius:50%;width:36px;height:36px;font-size:16px;cursor:pointer;display:flex;align-items:center;justify-content:center;color:#374151'
      nextBtn.onclick = function (e) { e.stopPropagation(); currentIdx = (currentIdx + 1) % images.length; imgEl.src = images[currentIdx]; updateCounter(); updateThumbs() }
      imgArea.appendChild(nextBtn)
    }

    // Counter
    var counterEl = document.createElement('div')
    counterEl.className = 'text-xs text-gray-500 text-center py-1 bg-gray-50'
    counterEl.style.cssText = 'font-size:12px;color:#6b7280;text-align:center;padding:4px 0;background:#f9fafb'
    function updateCounter () { counterEl.textContent = (currentIdx + 1) + ' / ' + images.length }
    updateCounter()
    body.appendChild(imgArea)
    body.appendChild(counterEl)

    // Thumbnails
    if (images.length > 1) {
      var thumbRow = document.createElement('div')
      thumbRow.className = 'flex gap-2 p-2 overflow-x-auto bg-gray-100'
      thumbRow.style.cssText = 'display:flex;gap:8px;padding:8px;overflow-x:auto;background:#f3f4f6'

      function updateThumbs () {
        var thumbs = thumbRow.querySelectorAll('img')
        thumbs.forEach(function (img, idx) {
          img.style.borderColor = idx === currentIdx ? '#3b82f6' : '#e5e7eb'
          img.style.boxShadow = idx === currentIdx ? '0 0 0 2px #3b82f6' : 'none'
        })
      }

      images.forEach(function (src, idx) {
        var thumb = document.createElement('img')
        thumb.src = src
        thumb.alt = 'Thumb ' + (idx + 1)
        thumb.className = 'cursor-pointer rounded border'
        thumb.style.cssText = 'width:64px;height:64px;object-fit:cover;border-radius:4px;cursor:pointer;border:2px solid ' + (idx === 0 ? '#3b82f6' : '#e5e7eb') + ';box-shadow:' + (idx === 0 ? '0 0 0 2px #3b82f6' : 'none')
        thumb.onclick = function () { currentIdx = idx; imgEl.src = images[currentIdx]; updateCounter(); updateThumbs() }
        thumbRow.appendChild(thumb)
      })

      body.appendChild(thumbRow)
    }

    // Content area
    var contentDiv = document.createElement('div')
    contentDiv.className = 'p-4'
    contentDiv.style.cssText = 'padding:16px'

    // Content HTML or excerpt
    var html = getContentHtml(obj)
    if (html) {
      var prose = document.createElement('div')
      prose.className = 'prose max-w-none text-sm text-gray-700'
      prose.style.cssText = 'font-size:14px;color:#374151;line-height:1.6'
      prose.textContent = html.replace(/<[^>]+>/g, ' ')
      contentDiv.appendChild(prose)
    } else {
      var excerpt = getExcerpt(obj)
      if (excerpt) {
        var excerptEl = document.createElement('div')
        excerptEl.className = 'text-sm text-gray-700 whitespace-pre-line'
        excerptEl.style.cssText = 'font-size:14px;color:#374151;white-space:pre-line;line-height:1.6'
        excerptEl.textContent = excerpt
        contentDiv.appendChild(excerptEl)
      }
    }

    // Tags
    var tags = getTags(obj)
    if (tags.length > 0) {
      var tagWrap = document.createElement('div')
      tagWrap.className = 'flex flex-wrap gap-1 mb-3'
      tagWrap.style.cssText = 'display:flex;flex-wrap:wrap;gap:4px;margin-bottom:12px'
      tags.forEach(function (tag) {
        var t = document.createElement('span')
        t.className = 'text-xs bg-gray-100 text-gray-600 px-2 py-0.5 rounded'
        t.style.cssText = 'font-size:11px;background:#f3f4f6;color:#4b5563;padding:2px 8px;border-radius:4px'
        t.textContent = '#' + tag
        tagWrap.appendChild(t)
      })
      contentDiv.appendChild(tagWrap)
    }

    // Links
    var links = getLinks(obj)
    if (links.length > 0) {
      var linkTitle = document.createElement('p')
      linkTitle.className = 'text-xs font-semibold text-gray-500 mb-1'
      linkTitle.style.cssText = 'font-size:12px;font-weight:600;color:#6b7280;margin-bottom:4px'
      linkTitle.textContent = '相关链接'
      contentDiv.appendChild(linkTitle)

      var linkList = document.createElement('ul')
      linkList.className = 'space-y-1'
      linkList.style.cssText = 'list-style:none;padding:0;margin:0'
      links.forEach(function (l) {
        var li = document.createElement('li')
        var a = document.createElement('a')
        // Only allow http/https schemes
          a.href = /^https?:\/\//i.test(l.href) ? l.href : "#"
        a.target = '_blank'
        a.rel = 'noopener noreferrer'
        a.className = 'text-xs text-blue-600 hover:text-blue-800 break-all'
        a.style.cssText = 'font-size:12px;color:#2563eb;word-break:break-all'
        a.textContent = l.text || l.href
        li.appendChild(a)
        linkList.appendChild(li)
      })
      contentDiv.appendChild(linkList)
    }

    body.appendChild(contentDiv)
    panel.appendChild(body)

    // Footer
    var footer = document.createElement('div')
    footer.className = 'p-3 border-t bg-gray-50 flex justify-between items-center'
    footer.style.cssText = 'padding:12px;border-top:1px solid #e5e7eb;background:#f9fafb;display:flex;justify-content:space-between;align-items:center'

    var footerLeft = document.createElement('div')
    footerLeft.className = 'flex gap-2'
    footerLeft.style.cssText = 'display:flex;gap:8px'

    var pageUrl = obj && obj.metadata && obj.metadata.page_url
    if (pageUrl) {
      var originBtn = document.createElement('button')
      originBtn.className = 'px-3 py-1.5 rounded bg-blue-600 text-white hover:bg-blue-700 text-sm'
      originBtn.style.cssText = 'padding:6px 12px;border-radius:6px;background:#2563eb;color:#fff;border:none;cursor:pointer;font-size:14px'
      originBtn.textContent = '打开原页面'
      originBtn.onclick = function () { window.open(pageUrl, '_blank', 'noopener,noreferrer') }
      footerLeft.appendChild(originBtn)

      var copyLinkBtn = document.createElement('button')
      copyLinkBtn.className = 'px-3 py-1.5 rounded bg-white border hover:bg-gray-100 text-sm'
      copyLinkBtn.style.cssText = 'padding:6px 12px;border-radius:6px;background:#fff;border:1px solid #d1d5db;cursor:pointer;font-size:14px'
      copyLinkBtn.textContent = '复制链接'
      copyLinkBtn.onclick = function () {
        if (navigator.clipboard && navigator.clipboard.writeText) {
          navigator.clipboard.writeText(pageUrl)
        }
      }
      footerLeft.appendChild(copyLinkBtn)
    }

    footer.appendChild(footerLeft)

    var footerClose = document.createElement('button')
    footerClose.className = 'px-3 py-1.5 rounded bg-white border hover:bg-gray-100 text-sm'
    footerClose.style.cssText = 'padding:6px 12px;border-radius:6px;background:#fff;border:1px solid #d1d5db;cursor:pointer;font-size:14px'
    footerClose.textContent = '关闭'
    footerClose.onclick = closeModal
    footer.appendChild(footerClose)

    panel.appendChild(footer)

    // Click outside to close
    overlay.addEventListener('click', function (e) {
      if (e.target === overlay) closeModal()
    })

    // Escape key
    function keyHandler (e) {
      if (e.key === 'Escape') closeModal()
      if (e.key === 'ArrowLeft' && images.length > 1) {
        currentIdx = (currentIdx - 1 + images.length) % images.length
        imgEl.src = images[currentIdx]
        updateCounter()
        if (typeof updateThumbs === 'function') updateThumbs()
      }
      if (e.key === 'ArrowRight' && images.length > 1) {
        currentIdx = (currentIdx + 1) % images.length
        imgEl.src = images[currentIdx]
        updateCounter()
        if (typeof updateThumbs === 'function') updateThumbs()
      }
    }
    document.addEventListener('keydown', keyHandler)

    // Cleanup on close
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

  // Register with the bridge
  window.__dm_uiBridge.register('vikacg', {
    label: '浏览',
    open: function (obj) {
      createModal(obj)
    }
  })
})()
