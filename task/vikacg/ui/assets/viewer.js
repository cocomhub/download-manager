/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

;(function () {
  'use strict'

  var D = TaskUI.Data
  var Dm = TaskUI.Dom
  var M = TaskUI.Modal

  /**
   * Extract images from a VikACG download object.
   */
  function getImages (obj) {
    var imgs = []
    if (obj && obj.status === 'completed' && obj.extra && Array.isArray(obj.extra.files)) {
      obj.extra.files.forEach(function (f) {
        if (f.type === 'image' && f.path) imgs.push(D.fileUrl(f.path))
      })
    }
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

  function getExcerpt (obj) {
    var s = (obj && obj.extra && obj.extra.content_text) || ''
    if (!s) return ''
    var t = s.replace(/\s+/g, ' ').trim()
    return t.length > 200 ? t.slice(0, 200) + '...' : t
  }

  function getContentHtml (obj) {
    var s = (obj && obj.extra && obj.extra.content_html) || ''
    return typeof s === 'string' ? s.trim() : ''
  }

  // ---- Modal ----

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
      var pageUrl = obj && obj.metadata && obj.metadata.page_url
      if (pageUrl) window.open(pageUrl, '_blank', 'noopener,noreferrer')
      return
    }

    var currentIdx = 0
    var overlay = M.createOverlay()
    var panel = M.createPanel('1200px')
    overlay.appendChild(panel)

    // Header
    var header = document.createElement('div')
    header.style.cssText = 'padding:16px;border-bottom:1px solid #e5e7eb;display:flex;justify-content:space-between;align-items:center;background:#f9fafb'

    var titleGroup = document.createElement('div')
    titleGroup.style.cssText = 'display:flex;align-items:center;gap:12px'

    var titleEl = document.createElement('h3')
    titleEl.style.cssText = 'font-size:18px;font-weight:700;color:#1f2937;margin:0'
    titleEl.textContent = D.getTitle(obj) || 'VikACG'
    titleGroup.appendChild(titleEl)

    var section = obj && obj.metadata && obj.metadata.section
    if (section) {
      var sectionBadge = document.createElement('span')
      sectionBadge.style.cssText = 'padding:2px 8px;font-size:12px;border-radius:4px;background:#eff6ff;color:#2563eb'
      sectionBadge.textContent = section
      titleGroup.appendChild(sectionBadge)
    }

    var dateVal = D.getDate(obj)
    if (dateVal) {
      var dateEl = document.createElement('span')
      dateEl.style.cssText = 'font-size:12px;color:#6b7280'
      dateEl.textContent = dateVal
      titleGroup.appendChild(dateEl)
    }

    header.appendChild(titleGroup)

    var closeBtn = document.createElement('button')
    closeBtn.innerHTML = '<i class="fas fa-times"></i>'
    closeBtn.style.cssText = 'color:#6b7280;cursor:pointer;background:none;border:none;font-size:18px'
    closeBtn.onclick = closeModal
    header.appendChild(closeBtn)
    panel.appendChild(header)

    // Body - two-column layout
    var body = document.createElement('div')
    body.style.cssText = 'flex:1;overflow:hidden;padding:0;display:flex'

    var leftCol = document.createElement('div')
    leftCol.style.cssText = 'flex:1;overflow-y:auto'

    // Image area
    var imgArea = document.createElement('div')
    imgArea.style.cssText = 'position:relative;background:#000;display:flex;align-items:center;justify-content:center;min-height:300px'

    var imgEl = document.createElement('img')
    imgEl.style.cssText = 'width:100%;height:100%;object-fit:contain;max-height:60vh'
    imgEl.src = images[0]
    imgEl.alt = D.getTitle(obj)
    imgArea.appendChild(imgEl)

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
    leftCol.appendChild(imgArea)

    // Counter
    var counterEl = document.createElement('div')
    counterEl.style.cssText = 'font-size:12px;color:#6b7280;text-align:center;padding:4px 0;background:#f9fafb'
    function updateCounter () { counterEl.textContent = (currentIdx + 1) + ' / ' + images.length }
    updateCounter()
    leftCol.appendChild(counterEl)

    // Thumbnails
    if (images.length > 1) {
      var thumbRow = document.createElement('div')
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
        thumb.style.cssText = 'width:64px;height:64px;object-fit:cover;border-radius:4px;cursor:pointer;border:2px solid ' + (idx === 0 ? '#3b82f6' : '#e5e7eb')
        thumb.onclick = function () { currentIdx = idx; imgEl.src = images[currentIdx]; updateCounter(); updateThumbs() }
        thumbRow.appendChild(thumb)
      })
      leftCol.appendChild(thumbRow)
    }

    // Content area
    var contentDiv = document.createElement('div')
    contentDiv.style.cssText = 'padding:16px'

    var html = getContentHtml(obj)
    if (html) {
      var prose = document.createElement('div')
      prose.style.cssText = 'font-size:14px;color:#374151;line-height:1.6'
      prose.textContent = html.replace(/<[^>]+>/g, ' ')
      contentDiv.appendChild(prose)
    } else {
      var excerpt = getExcerpt(obj)
      if (excerpt) {
        var excerptEl = document.createElement('div')
        excerptEl.style.cssText = 'font-size:14px;color:#374151;white-space:pre-line;line-height:1.6'
        excerptEl.textContent = excerpt
        contentDiv.appendChild(excerptEl)
      }
    }

    var tags = D.getTags(obj)
    if (tags.length > 0) {
      var tagWrap = document.createElement('div')
      tagWrap.style.cssText = 'display:flex;flex-wrap:wrap;gap:4px;margin-bottom:12px;margin-top:12px'
      tagWrap.appendChild(Dm.createTagChips(tags))
      contentDiv.appendChild(tagWrap)
    }
    leftCol.appendChild(contentDiv)
    body.appendChild(leftCol)

    // Right column: related links
    var rightCol = document.createElement('div')
    rightCol.style.cssText = 'width:320px;border-left:1px solid #e5e7eb;overflow-y:auto;background:#f9fafb;flex-shrink:0'

    var links = getLinks(obj)
    if (links.length > 0) {
      var linkSection = document.createElement('div'); linkSection.style.cssText = 'padding:16px'
      var linkTitle = document.createElement('h4'); linkTitle.style.cssText = 'font-size:14px;font-weight:600;color:#374151;margin:0 0 8px'; linkTitle.textContent = '相关链接 (' + links.length + ')'; linkSection.appendChild(linkTitle)
      var linkList = document.createElement('ul'); linkList.style.cssText = 'list-style:none;padding:0;margin:0;display:flex;flex-direction:column;gap:4px'
      links.forEach(function (l) {
        var li = document.createElement('li')
        var a = document.createElement('a')
        a.href = /^https?:\/\//i.test(l.href) ? l.href : '#'
        a.target = '_blank'; a.rel = 'noopener noreferrer'
        a.style.cssText = 'font-size:12px;color:#2563eb;word-break:break-all;display:block;padding:6px 8px;border:1px solid #e5e7eb;border-radius:6px;text-decoration:none'
        a.textContent = l.text || l.href
        a.onmouseenter = function () { a.style.background = '#f3f4f6' }
        a.onmouseleave = function () { a.style.background = 'transparent' }
        li.appendChild(a); linkList.appendChild(li)
      })
      linkSection.appendChild(linkList); rightCol.appendChild(linkSection)
    } else {
      var emptySection = document.createElement('div')
      emptySection.style.cssText = 'padding:16px;text-align:center;color:#9ca3af;font-size:13px'
      emptySection.textContent = '暂无关联内容'
      rightCol.appendChild(emptySection)
    }
    body.appendChild(rightCol)
    panel.appendChild(body)

    // Footer
    var footer = M.createFooter({
      leftButtons: [
        (obj && obj.metadata && obj.metadata.page_url) ? Dm.createButton('打开原页面', function () { window.open(obj.metadata.page_url, '_blank', 'noopener,noreferrer') }, { primary: true }) : null,
        (obj && obj.metadata && obj.metadata.page_url) ? Dm.createButton('复制链接', function () { D.copyToClipboard(obj.metadata.page_url) }) : null,
      ].filter(Boolean),
      onClose: closeModal
    })
    panel.appendChild(footer)

    // Backdrop click
    overlay.addEventListener('click', function (e) {
      if (e.target === overlay) closeModal()
    })

    // Escape + arrow keys
    function keyHandler (e) {
      if (e.key === 'Escape') closeModal()
      if (e.key === 'ArrowLeft' && images.length > 1) {
        currentIdx = (currentIdx - 1 + images.length) % images.length
        imgEl.src = images[currentIdx]; updateCounter(); if (typeof updateThumbs === 'function') updateThumbs()
      }
      if (e.key === 'ArrowRight' && images.length > 1) {
        currentIdx = (currentIdx + 1) % images.length
        imgEl.src = images[currentIdx]; updateCounter(); if (typeof updateThumbs === 'function') updateThumbs()
      }
    }
    document.addEventListener('keydown', keyHandler)

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

  // ---- Register with TaskUI ----
  if (typeof TaskUI !== 'undefined' && TaskUI.register) {
    TaskUI.register('vikacg', {
      type: 'vikacg',
      label: 'VikACG',
      icon: 'fa-image',
      viewerLabel: '浏览',
      shouldShowViewer: function (obj) {
        return obj.status === 'completed' && obj.extra && (Array.isArray(obj.extra.images) || Array.isArray(obj.extra.files))
      },
      onClick: function (obj, helpers) {
        if (obj.status !== 'completed') return false
        var images = getImages(obj)
        if (images.length > 0) {
          helpers.openTaskTypeViewer(obj)
          return true
        }
        var pageUrl = obj && obj.metadata && obj.metadata.page_url
        if (pageUrl) {
          window.open(pageUrl, '_blank', 'noopener,noreferrer')
          return true
        }
        return false
      },
      renderViewer: function (h, obj, onClose) {
        var images = getImages(obj)
        if (images.length === 0) {
          if (onClose) onClose()
          return h ? h('div') : null
        }

        var currentIdx = 0
        var overlay = M.createOverlay()
        var panel = M.createPanel('1200px')
        overlay.appendChild(panel)

        var keyHandler = null

        // Build the final closeHandler before any component captures it
        var closeHandler = function () {
          if (keyHandler) document.removeEventListener('keydown', keyHandler)
          if (overlay && overlay.parentNode) overlay.parentNode.removeChild(overlay)
          document.body.style.overflow = ''
          if (onClose) onClose()
        }

        // Header
        var header = M.createHeader({
          title: D.getTitle(obj) || 'VikACG',
          onClose: closeHandler,
          badges: [
            (obj && obj.metadata && obj.metadata.section) ? { text: obj.metadata.section, bg: '#eff6ff', color: '#2563eb' } : null,
          ].filter(Boolean)
        })
        panel.appendChild(header)

        // Body
        var body = document.createElement('div')
        body.style.cssText = 'flex:1;overflow:hidden;padding:0;display:flex'

        var leftCol = document.createElement('div')
        leftCol.style.cssText = 'flex:1;overflow-y:auto'

        var imgArea = document.createElement('div')
        imgArea.style.cssText = 'position:relative;background:#000;display:flex;align-items:center;justify-content:center;min-height:300px'
        var imgEl = document.createElement('img')
        imgEl.src = images[0]
        imgEl.style.cssText = 'width:100%;object-fit:contain;max-height:60vh'
        imgEl.onerror = function (e) { e.target.style.display = 'none' }
        imgArea.appendChild(imgEl)

        function updateCounter() { counterEl.textContent = (currentIdx + 1) + ' / ' + images.length }
        function updateThumbs() { var thumbs = thumbRow.querySelectorAll('img'); thumbs.forEach(function (img, idx) { img.style.borderColor = idx === currentIdx ? '#3b82f6' : '#e5e7eb'; img.style.boxShadow = idx === currentIdx ? '0 0 0 2px #3b82f6' : 'none' }) }

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
        leftCol.appendChild(imgArea)

        var counterEl = document.createElement('div')
        counterEl.style.cssText = 'font-size:12px;color:#6b7280;text-align:center;padding:4px 0;background:#f9fafb'
        updateCounter()
        leftCol.appendChild(counterEl)

        var thumbRow = document.createElement('div')
        thumbRow.style.cssText = 'display:flex;gap:8px;padding:8px;overflow-x:auto;background:#f3f4f6'
        if (images.length > 1) {
          images.forEach(function (src, idx) {
            var thumb = document.createElement('img')
            thumb.src = src; thumb.alt = 'Thumb ' + (idx + 1)
            thumb.style.cssText = 'width:64px;height:64px;object-fit:cover;border-radius:4px;cursor:pointer;border:2px solid ' + (idx === 0 ? '#3b82f6' : '#e5e7eb')
            thumb.onclick = function () { currentIdx = idx; imgEl.src = images[currentIdx]; updateCounter(); updateThumbs() }
            thumbRow.appendChild(thumb)
          })
          leftCol.appendChild(thumbRow)
        }

        // Content
        var contentDiv = document.createElement('div')
        contentDiv.style.cssText = 'padding:16px'
        var html = getContentHtml(obj)
        if (html) {
          var prose = document.createElement('div'); prose.style.cssText = 'font-size:14px;color:#374151;line-height:1.6'
          prose.textContent = html.replace(/<[^>]+>/g, ' '); contentDiv.appendChild(prose)
        } else {
          var excerpt = getExcerpt(obj)
          if (excerpt) { var excerptEl = document.createElement('div'); excerptEl.style.cssText = 'font-size:14px;color:#374151;white-space:pre-line;line-height:1.6'; excerptEl.textContent = excerpt; contentDiv.appendChild(excerptEl) }
        }
        var tags = D.getTags(obj)
        if (tags.length > 0) {
          var tagWrap = document.createElement('div'); tagWrap.style.cssText = 'display:flex;flex-wrap:wrap;gap:4px;margin-bottom:12px;margin-top:12px'
          tagWrap.appendChild(Dm.createTagChips(tags)); contentDiv.appendChild(tagWrap)
        }
        leftCol.appendChild(contentDiv)
        body.appendChild(leftCol)

        // Right: links
        var rightCol = document.createElement('div')
        rightCol.style.cssText = 'width:320px;border-left:1px solid #e5e7eb;overflow-y:auto;background:#f9fafb;flex-shrink:0'
        var links = getLinks(obj)
        if (links.length > 0) {
          var linkSection = document.createElement('div'); linkSection.style.cssText = 'padding:16px'
          var linkTitle = document.createElement('h4'); linkTitle.style.cssText = 'font-size:14px;font-weight:600;color:#374151;margin:0 0 8px'; linkTitle.textContent = '相关链接 (' + links.length + ')'; linkSection.appendChild(linkTitle)
          var linkList = document.createElement('ul'); linkList.style.cssText = 'list-style:none;padding:0;margin:0;display:flex;flex-direction:column;gap:4px'
          links.forEach(function (l) {
            var li = document.createElement('li'); var a = document.createElement('a')
            a.href = /^https?:\/\//i.test(l.href) ? l.href : '#'; a.target = '_blank'; a.rel = 'noopener noreferrer'
            a.style.cssText = 'font-size:12px;color:#2563eb;word-break:break-all;display:block;padding:6px 8px;border:1px solid #e5e7eb;border-radius:6px;text-decoration:none'
            a.textContent = l.text || l.href
            a.onmouseenter = function () { a.style.background = '#f3f4f6' }; a.onmouseleave = function () { a.style.background = 'transparent' }
            li.appendChild(a); linkList.appendChild(li)
          })
          linkSection.appendChild(linkList); rightCol.appendChild(linkSection)
        }
        body.appendChild(rightCol)
        panel.appendChild(body)

        // Footer
        var pageUrl = obj && obj.metadata && obj.metadata.page_url
        var footer = M.createFooter({
          leftButtons: pageUrl ? [
            Dm.createButton('打开原页面', function () { window.open(pageUrl, '_blank', 'noopener,noreferrer') }, { primary: true }),
            Dm.createButton('复制链接', function () { D.copyToClipboard(pageUrl) }),
          ] : [],
          onClose: closeHandler
        })
        panel.appendChild(footer)

        // Close handlers
        overlay.addEventListener('click', function (e) { if (e.target === overlay) closeHandler() })

        keyHandler = function(e) {
          if (e.key === 'Escape') { closeHandler(); return }
          if (e.key === 'ArrowLeft' && images.length > 1) { currentIdx = (currentIdx - 1 + images.length) % images.length; imgEl.src = images[currentIdx]; updateCounter(); if (typeof updateThumbs === 'function') updateThumbs() }
          if (e.key === 'ArrowRight' && images.length > 1) { currentIdx = (currentIdx + 1) % images.length; imgEl.src = images[currentIdx]; updateCounter(); if (typeof updateThumbs === 'function') updateThumbs() }
        }
        document.addEventListener('keydown', keyHandler)

        document.body.appendChild(overlay)
        document.body.style.overflow = 'hidden'
        return h ? h('div') : null
      }
    })
  }

})()