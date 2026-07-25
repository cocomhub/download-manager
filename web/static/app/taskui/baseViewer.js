// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * BaseViewer — 基础查看器模态框骨架
 * 提供模态框容器（header/body/footer），关闭按钮，ESC/背景点击关闭
 * 通过 contentRenderer 参数接收 task 类型自定义内容
 */
;(function () {
  'use strict'

  function BaseViewer(h, obj, onClose, contentRenderer) {
    var title = (obj && obj.metadata && obj.metadata.title) || ''
    return h('div', {
      class: 'fixed inset-0 bg-black bg-opacity-70 z-50 flex items-center justify-center p-4 backdrop-blur-sm',
      attrs: { tabindex: '0' },
      on: {
        click: function (e) { if (e.target === e.currentTarget && onClose) onClose() },
      }
    }, [
      h('div', { class: 'bg-white rounded-lg shadow-2xl w-full max-w-5xl max-h-[90vh] overflow-hidden flex flex-col' }, [
        // Header
        h('div', { class: 'p-4 border-b flex justify-between items-center bg-gray-50' }, [
          h('h3', { class: 'text-lg font-bold text-gray-800' }, title),
          onClose ? h('button', {
            class: 'text-gray-500 hover:text-gray-700',
            on: { click: function (e) { e.stopPropagation(); onClose() } }
          }, [h('i', { class: 'fas fa-times' })]) : null,
        ]),
        // Body
        h('div', { class: 'flex-1 overflow-y-auto' }, [
          typeof contentRenderer === 'function' ? contentRenderer(h, obj) : null,
        ]),
        // Footer
        h('div', { class: 'p-3 border-t bg-gray-50 flex justify-end' }, [
          onClose ? h('button', {
            class: 'px-3 py-1.5 rounded bg-white border hover:bg-gray-100 text-sm',
            on: { click: function (e) { e.stopPropagation(); onClose() } }
          }, '关闭') : null,
        ]),
      ])
    ])
  }

  window.TaskUI = window.TaskUI || {}
  window.TaskUI.BaseViewer = BaseViewer
})()