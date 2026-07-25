// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * TaskUI — 全局 task 类型 UI 组件注册表
 * 每个 task 类型通过 TaskUI.register(type, handler) 注册自己的 UI 组件。
 * 核心 UI 通过 TaskUI.get(type) 获取处理器，使用 Vue h() 函数渲染。
 */
;(function () {
  'use strict'

  var registry = {}

  /**
   * 规范化 handler，确保所有可选方法有默认值
   */
  function normalizeHandler(handler) {
    handler = handler || {}
    return {
      type: handler.type || '',
      label: handler.label || handler.type || '',
      icon: handler.icon || 'fa-cube',
      renderForm: typeof handler.renderForm === 'function' ? handler.renderForm : null,
      renderMeta: typeof handler.renderMeta === 'function' ? handler.renderMeta : null,
      renderCardExtra: typeof handler.renderCardExtra === 'function' ? handler.renderCardExtra : null,
      renderViewer: typeof handler.renderViewer === 'function' ? handler.renderViewer : null,
      renderAggregate: typeof handler.renderAggregate === 'function' ? handler.renderAggregate : null,
      shouldShowViewer: typeof handler.shouldShowViewer === 'function' ? handler.shouldShowViewer : defaultShouldShowViewer,
      viewerLabel: handler.viewerLabel || '查看',
    }
  }

  function defaultShouldShowViewer(obj) {
    return obj && obj.status === 'completed'
  }

  window.TaskUI = {
    register: function (type, handler) {
      if (!type || !handler) return
      registry[type] = normalizeHandler(handler)
    },
    get: function (type) {
      return registry[type] || null
    },
    list: function () {
      return Object.keys(registry)
    },
    hasForm: function (type) {
      var h = registry[type]
      return h && typeof h.renderForm === 'function'
    },
    hasViewer: function (type) {
      var h = registry[type]
      return h && typeof h.renderViewer === 'function'
    },
    hasMeta: function (type) {
      var h = registry[type]
      return h && typeof h.renderMeta === 'function'
    },
    hasAggregate: function (type) {
      var h = registry[type]
      return h && typeof h.renderAggregate === 'function'
    },
    hasCardExtra: function (type) {
      var h = registry[type]
      return h && typeof h.renderCardExtra === 'function'
    },
  }
})()