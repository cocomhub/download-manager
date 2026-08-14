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
      renderForm: typeof handler.renderForm === 'function'
        ? function (formData, formErrors) { return handler.renderForm(Vue.h, formData, formErrors) }
        : null,
      renderMeta: typeof handler.renderMeta === 'function'
        ? function (task) { return handler.renderMeta(Vue.h, task) }
        : null,
      renderCardExtra: typeof handler.renderCardExtra === 'function'
        ? function (obj) { return handler.renderCardExtra(Vue.h, obj) }
        : null,
      renderViewer: typeof handler.renderViewer === 'function'
        ? function (h, obj, onClose) { return handler.renderViewer(Vue.h, obj, onClose) }
        : null,
      renderAggregate: typeof handler.renderAggregate === 'function' ? handler.renderAggregate : null,
      onClick: typeof handler.onClick === 'function' ? handler.onClick : null,
      shouldShowViewer: typeof handler.shouldShowViewer === 'function' ? handler.shouldShowViewer : defaultShouldShowViewer,
      viewerLabel: handler.viewerLabel || '查看',
      // groupBy: 任务类型希望任务详情按指定字段分组（如 mxs → "book"），
      // 前端据此为 taskDetails 附加 &group_by=...，实现「一书一卡」书橱视图。
      groupBy: handler.groupBy || '',
      // cardCoverAspect: 卡片封面区宽高比（如 mxs 竖版封面 "5/7"），默认 "16/9"。
      // 仅改变封面区比例（卡片宽度不变，行内高度取最高者自动对齐），供封面为竖版的任务类型使用。
      cardCoverAspect: handler.cardCoverAspect || '',
      // collectExtra: 将 formData 映射为 API 请求的 payload 字段
      // 返回 { urls_text, keyword, ... } 或 { extra: { ... } } 等
      collectExtra: typeof handler.collectExtra === 'function' ? handler.collectExtra : defaultCollectExtra,
    }
  }

  function defaultCollectExtra(formData) {
    return {}
  }

  function defaultShouldShowViewer(obj) {
    return obj && obj.status === 'completed'
  }

  window.TaskUI = {
    register: function (type, handler) {
      if (!type || !handler) return
      registry[type] = normalizeHandler(handler)
      Log.debug('TaskUI.register', { type: type, hasForm: !!handler.renderForm, hasViewer: !!handler.renderViewer, hasMeta: !!handler.renderMeta, hasCardExtra: !!handler.renderCardExtra, hasOnClick: !!handler.onClick })
    },
    get: function (type) {
      var h = registry[type] || null
      if (h) {
        Log.trace('TaskUI.get', { type: type, found: true })
      } else {
        Log.trace('TaskUI.get', { type: type, found: false })
      }
      return h
    },
    list: function () {
      var keys = Object.keys(registry)
      Log.debug('TaskUI.list', { count: keys.length, types: keys })
      return keys
    },
    hasForm: function (type) {
      var h = registry[type]
      var result = h && typeof h.renderForm === 'function'
      Log.trace('TaskUI.hasForm', { type: type, result: result })
      return result
    },
    hasViewer: function (type) {
      var h = registry[type]
      var result = h && typeof h.renderViewer === 'function'
      Log.trace('TaskUI.hasViewer', { type: type, result: result })
      return result
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
    hasOnClick: function (type) {
      var h = registry[type]
      return h && typeof h.onClick === 'function'
    },
  }
})()