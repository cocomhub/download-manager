/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 *
 * {{TYPE}} Task UI 插件模板
 *
 * 依赖：
 *   - TaskUI.Data（通用数据访问器）
 *   - TaskUI.Dom（DOM 构建辅助）
 *   - TaskUI.Modal（Modal 构建器）
 *
 * 使用方式：
 *   1. 替换文件中所有 {{TYPE}} 为实际类型名
 *   2. 替换 {{LABEL}} 为显示标签
 *   3. 按需取消注释以下变体之一的代码块
 *   4. 移除不需要的变体
 */

;(function () {
  'use strict'

  // =============================================
  // 变体 1：视频播放器（参考 tktube / hanime）
  // =============================================
  // 取消注释以下代码块即可使用

  // if (typeof TaskUI !== 'undefined' && TaskUI.register) {
  //   TaskUI.register('{{TYPE}}', {
  //     type: '{{TYPE}}',
  //     label: '{{LABEL}}',
  //     icon: 'fa-video', // 图标：fa-video / fa-film / fa-image / fa-link 等
  //     viewerLabel: '查看',
  //
  //     // ---- 表单（可选） ----
  //     renderForm: TaskUI.defineForm({
  //       fields: [
  //         { type: 'text', key: 'keyword', label: '关键字', required: true, placeholder: '搜索关键词' },
  //         { type: 'number', key: 'max_concurrent', label: '并发数', min: 1, max: 10, default: 2 },
  //       ]
  //     }),
  //
  //     // ---- 元数据（可选） ----
  //     renderMeta: TaskUI.defineMeta({
  //       fields: [
  //         { type: 'text', key: 'keyword', label: '关键字', path: 'extra.keyword' },
  //         { type: 'text', key: 'max_concurrent', label: '并发', path: 'extra.max_concurrent' },
  //       ]
  //     }),
  //
  //     // ---- 表单数据映射 ----
  //     collectExtra: function (formData) {
  //       var extra = {}
  //       if (formData.keyword) extra.keyword = formData.keyword
  //       if (formData.max_concurrent) extra.max_concurrent = formData.max_concurrent
  //       return extra
  //     },
  //
  //     // ---- 查看器条件 ----
  //     shouldShowViewer: function (obj) { return obj.status === 'completed' },
  //
  //     // ---- 点击处理 ----
  //     onClick: function (obj, helpers) {
  //       if (obj.status !== 'completed') return false
  //       helpers.openTaskTypeViewer(obj)
  //       return true
  //     },
  //
  //     // ---- 查看器渲染 ----
  //     renderViewer: function (h, obj, onClose) {
  //       var D = TaskUI.Data, Dm = TaskUI.Dom, M = TaskUI.Modal
  //
  //       var videoUrl = D.getVideoUrl(obj)
  //       var coverUrl = D.getCoverImage(obj)
  //       var title = D.getTitle(obj) || '{{LABEL}}'
  //       var dur = D.getDuration(obj)
  //       var dateVal = D.getDate(obj)
  //       var origin = D.getOriginLink(obj)
  //       var details = D.getDetails(obj)
  //       var tags = D.getTags(obj)
  //       var fileUrlVal = D.getFileUrl(obj)
  //       var taskType = obj && obj.metadata && obj.metadata.task_type
  //       var objTags = (obj && obj.extra && Array.isArray(obj.extra.tags)) ? obj.extra.tags : []
  //
  //       var modal = M.create(obj, {
  //         title: title,
  //         mediaType: 'video',
  //         videoUrl: videoUrl,
  //         coverUrl: coverUrl,
  //         infoBar: [
  //           dur ? { icon: 'fas fa-clock', text: dur } : null,
  //           dateVal ? { icon: 'fas fa-calendar', text: dateVal } : null,
  //         ].filter(Boolean),
  //         contentRenderer: function (contentDiv) {
  //           if (details) {
  //             var de = document.createElement('div')
  //             de.style.cssText = 'font-size:14px;color:#374151;line-height:1.6;white-space:pre-line;margin-bottom:12px'
  //             de.textContent = details
  //             contentDiv.appendChild(de)
  //           }
  //           if (tags.length > 0) {
  //             var tagWrap = document.createElement('div')
  //             tagWrap.style.cssText = 'display:flex;flex-wrap:wrap;gap:4px;margin-bottom:12px'
  //             tagWrap.appendChild(Dm.createTagChips(tags))
  //             contentDiv.appendChild(tagWrap)
  //           }
  //         },
  //         sidebar: 'collection',
  //         type: taskType,
  //         currentId: obj.id,
  //         tags: objTags,
  //         onPlayItem: function (item) {
  //           AppAPI.getObject(taskType, item.id).then(function (newObj) {
  //             modal.close()
  //             TaskUI.get('{{TYPE}}').renderViewer(h, newObj, onClose)
  //           })
  //         },
  //         footerActions: [
  //           fileUrlVal ? Dm.createLink(fileUrlVal, '打开文件', { style: 'padding:6px 12px;border-radius:6px;background:#2563eb;color:#fff;text-decoration:none;font-size:14px;display:inline-block' }) : null,
  //           origin ? Dm.createLink(origin, '打开原页面') : null,
  //           Dm.createButton('复制标题', function () { D.copyToClipboard(D.getTitle(obj)) }),
  //           origin ? Dm.createButton('复制链接', function () { D.copyToClipboard(origin) }) : null,
  //         ].filter(Boolean),
  //         onClose: onClose
  //       })
  //
  //       return h('div')
  //     }
  //   })
  // }

  // =============================================
  // 变体 2：图片画廊（参考 vikacg）
  // =============================================
  // 取消注释以下代码块，实现 getImages()/getLinks() 数据函数后即可使用

  // if (typeof TaskUI !== 'undefined' && TaskUI.register) {
  //   TaskUI.register('{{TYPE}}', {
  //     type: '{{TYPE}}',
  //     label: '{{LABEL}}',
  //     icon: 'fa-image',
  //     viewerLabel: '浏览',
  //
  //     shouldShowViewer: function (obj) {
  //       return obj.status === 'completed' && obj.extra && Array.isArray(obj.extra.images)
  //     },
  //
  //     onClick: function (obj, helpers) {
  //       if (obj.status !== 'completed') return false
  //       helpers.openTaskTypeViewer(obj)
  //       return true
  //     },
  //
  //     renderViewer: function (h, obj, onClose) {
  //       var D = TaskUI.Data, Dm = TaskUI.Dom, M = TaskUI.Modal
  //       var images = getImages(obj) // 需实现
  //       if (images.length === 0) { if (onClose) onClose(); return h('div') }
  //
  //       var currentIdx = 0
  //       var overlay = M.createOverlay()
  //       var panel = M.createPanel('1200px')
  //       overlay.appendChild(panel)
  //
  //       // Header
  //       var header = M.createHeader({
  //         title: D.getTitle(obj) || '{{LABEL}}',
  //         onClose: onClose
  //       })
  //       panel.appendChild(header)
  //
  //       // Body
  //       var body = document.createElement('div')
  //       body.style.cssText = 'flex:1;overflow:hidden;padding:0;display:flex'
  //       // ... 图片画廊业务逻辑（左右箭头、缩略图、内容等）
  //       panel.appendChild(body)
  //
  //       // Footer
  //       var footer = M.createFooter({
  //         leftButtons: [ /* 操作按钮 */ ],
  //         onClose: onClose
  //       })
  //       panel.appendChild(footer)
  //
  //       // Close handlers
  //       M.setupCloseHandlers({ overlay: overlay, onClose: onClose })
  //       // 额外键盘事件（ArrowLeft/ArrowRight），需在 onClose 中清理
  //       function arrowHandler(e) {
  //         if (e.key === 'ArrowLeft' && images.length > 1) { ... }
  //         if (e.key === 'ArrowRight' && images.length > 1) { ... }
  //       }
  //       document.addEventListener('keydown', arrowHandler)
  //       // 在 onClose 中添加：document.removeEventListener('keydown', arrowHandler)
  //
  //       document.body.appendChild(overlay)
  //       document.body.style.overflow = 'hidden'
  //       return h('div')
  //     }
  //   })
  // }

  // =============================================
  // 变体 3：纯表单（参考 urllist）— 无 viewer
  // =============================================
  // 取消注释以下代码块即可使用

  // if (typeof TaskUI !== 'undefined' && TaskUI.register) {
  //   TaskUI.register('{{TYPE}}', {
  //     type: '{{TYPE}}',
  //     label: '{{LABEL}}',
  //     icon: 'fa-link',
  //
  //     renderForm: TaskUI.defineForm({
  //       fields: [
  //         { type: 'textarea', key: 'urls_text', label: 'URL 列表（每行一个）', rows: 10, required: true },
  //       ]
  //     }),
  //
  //     renderMeta: TaskUI.defineMeta({
  //       fields: [
  //         { type: 'count', key: 'URL 数量', path: 'extra.urls' },
  //       ]
  //     }),
  //
  //     collectExtra: function (formData) {
  //       return { urls_text: formData.urls_text }
  //     }
  //   })
  // }
})()