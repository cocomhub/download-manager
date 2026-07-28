// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * 共享 DOM 构建辅助函数。
 * 挂载到 TaskUI.Dom 命名空间。
 */
;(function () {
  'use strict'

  var Dom = {}

  /**
   * 创建标签 chip 集合
   * @param {string[]} tags
   * @returns {DocumentFragment}
   */
  Dom.createTagChips = function (tags) {
    var frag = document.createDocumentFragment()
    if (!tags || tags.length === 0) return frag
    tags.forEach(function (tag) {
      var t = document.createElement('span')
      t.style.cssText = 'font-size:11px;background:#f3f4f6;color:#4b5563;padding:2px 8px;border-radius:4px'
      t.textContent = '#' + tag
      frag.appendChild(t)
    })
    return frag
  }

  /**
   * 创建信息条（图标 + 文本）
   * @param {Array<{icon: string, text: string}>} items
   * @returns {HTMLElement}
   */
  Dom.createInfoBar = function (items) {
    var bar = document.createElement('div')
    bar.style.cssText = 'display:flex;gap:16px;padding:12px 16px;background:#f9fafb;border-bottom:1px solid #e5e7eb;font-size:13px;color:#6b7280;flex-wrap:wrap'
    if (!items || items.length === 0) return bar
    items.forEach(function (item) {
      if (!item || !item.text) return
      var el = document.createElement('span')
      var icon = document.createElement('i')
      icon.className = item.icon || 'fas fa-circle'
      icon.style.cssText = 'margin-right:4px'
      el.appendChild(icon)
      el.appendChild(document.createTextNode(' ' + item.text))
      bar.appendChild(el)
    })
    return bar
  }

  /**
   * 创建按钮
   * @param {string} text
   * @param {function} onClick
   * @param {object} [opts] — { primary?: boolean, style?: string }
   * @returns {HTMLElement}
   */
  Dom.createButton = function (text, onClick, opts) {
    opts = opts || {}
    var btn = document.createElement('button')
    if (opts.style) {
      btn.style.cssText = opts.style
    } else if (opts.primary) {
      btn.style.cssText = 'padding:6px 12px;border-radius:6px;background:#2563eb;color:#fff;border:none;cursor:pointer;font-size:14px'
    } else {
      btn.style.cssText = 'padding:6px 12px;border-radius:6px;background:#fff;border:1px solid #d1d5db;cursor:pointer;font-size:14px;color:#374151'
    }
    btn.textContent = text
    if (onClick) btn.onclick = onClick
    return btn
  }

  /**
   * 创建链接
   * @param {string} href
   * @param {string} text
   * @param {object} [opts] — { style?: string, primary?: boolean }
   * @returns {HTMLElement}
   */
  Dom.createLink = function (href, text, opts) {
    opts = opts || {}
    var a = document.createElement('a')
    a.href = /^https?:\/\//i.test(href) ? href : '#'
    a.target = '_blank'
    a.rel = 'noopener noreferrer'
    if (opts.style) {
      a.style.cssText = opts.style
    } else if (opts.primary) {
      a.style.cssText = 'padding:6px 12px;border-radius:6px;background:#2563eb;color:#fff;text-decoration:none;font-size:14px;display:inline-block'
    } else {
      a.style.cssText = 'padding:6px 12px;border-radius:6px;background:#fff;border:1px solid #d1d5db;text-decoration:none;cursor:pointer;font-size:14px;color:#374151;display:inline-block'
    }
    a.textContent = text || href
    return a
  }

  /**
   * 创建徽章
   * @param {string} text
   * @param {string} [bgColor]
   * @param {string} [textColor]
   * @returns {HTMLElement}
   */
  Dom.createBadge = function (text, bgColor, textColor) {
    var badge = document.createElement('span')
    badge.style.cssText = 'font-size:11px;padding:2px 8px;border-radius:4px;background:' + (bgColor || '#f3f4f6') + ';color:' + (textColor || '#4b5563')
    badge.textContent = text
    return badge
  }

  window.TaskUI = window.TaskUI || {}
  window.TaskUI.Dom = Dom
})()