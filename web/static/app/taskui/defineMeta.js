// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * defineMeta — 声明式元数据工厂
 * 简单 task 类型通过配置字段列表快速生成 renderMeta 函数。
 *
 * 支持字段类型：text, count, json
 * 数据路径：'extra.urls'、'save_dir' 等点号分隔路径
 */
;(function () {
  'use strict'

  /**
   * 根据点号路径获取嵌套值
   * 例：resolvePath({ extra: { urls: [...] } }, 'extra.urls') → [...]
   */
  function resolvePath(obj, path) {
    if (!obj || !path) return undefined
    var parts = path.split('.')
    var current = obj
    for (var i = 0; i < parts.length; i++) {
      if (current == null) return undefined
      current = current[parts[i]]
    }
    return current
  }

  var META_RENDERERS = {
    text: renderMetaText,
    count: renderMetaCount,
    json: renderMetaJson,
  }

  function defineMeta(config) {
    config = config || {}
    var fields = config.fields || []

    return function renderMeta(h, task) {
      if (!task) return h('div', { class: 'text-xs text-gray-500' }, '无数据')
      return h('div', { class: 'grid grid-cols-1 md:grid-cols-3 gap-2 text-xs text-gray-600' }, fields.map(function (field) {
        var renderer = META_RENDERERS[field.type] || renderMetaText
        return renderer(h, field, task)
      }))
    }
  }

  function renderMetaText(h, field, task) {
    var value = resolvePath(task, field.path)
    if (value === undefined || value === null) value = '-'
    return h('div', [
      h('span', { class: 'text-gray-400' }, field.label + '：'),
      h('span', String(value))
    ])
  }

  function renderMetaCount(h, field, task) {
    var value = resolvePath(task, field.path)
    var count = 0
    if (Array.isArray(value)) count = value.length
    else if (typeof value === 'number') count = value
    return h('div', [
      h('span', { class: 'text-gray-400' }, field.label + '：'),
      h('span', String(count))
    ])
  }

  function renderMetaJson(h, field, task) {
    var value = resolvePath(task, field.path)
    var text = '-'
    try { text = JSON.stringify(value, null, 2) } catch (e) {}
    return h('div', { class: 'break-all' }, [
      h('span', { class: 'text-gray-400' }, field.label + '：'),
      h('pre', { class: 'inline whitespace-pre-wrap' }, text)
    ])
  }

  // 导出
  window.TaskUI = window.TaskUI || {}
  window.TaskUI.defineMeta = defineMeta
})()