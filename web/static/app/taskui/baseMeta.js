// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * BaseMeta — 基础任务元数据显示
 * 通用信息：任务ID、类型、状态、存储配置
 * 通过 extraMeta 参数接收 task 类型特定扩展元数据的渲染函数
 */
;(function () {
  'use strict'

  function BaseMeta(h, task, extraMeta) {
    if (!task) return h('div', { class: 'text-xs text-gray-500' }, '无数据')
    return h('div', { class: 'grid grid-cols-1 md:grid-cols-3 gap-2 text-xs text-gray-600' }, [
      h('div', [
        h('span', { class: 'text-gray-400' }, '任务ID：'),
        h('span', task.id || '-')
      ]),
      h('div', [
        h('span', { class: 'text-gray-400' }, '类型：'),
        h('span', task.type || '-')
      ]),
      h('div', [
        h('span', { class: 'text-gray-400' }, '状态：'),
        h('span', task.status || 'unknown')
      ]),
      h('div', [
        h('span', { class: 'text-gray-400' }, '并发数：'),
        h('span', String(task.concurrency || '-'))
      ]),
      h('div', [
        h('span', { class: 'text-gray-400' }, '刷新间隔（秒）：'),
        h('span', String(task.refresh_interval || '-'))
      ]),
      h('div', { class: 'md:col-span-2' }, [
        h('span', { class: 'text-gray-400' }, '存储路径：'),
        h('span', (task.save_dir || '') || '未提供')
      ]),
      task.storage ? h('div', { class: 'md:col-span-2' }, [
        h('div', { class: 'text-gray-400' }, '存储配置'),
        h('div', { class: 'grid grid-cols-1 md:grid-cols-3 gap-2 text-xs text-gray-600' }, [
          h('div', [h('span', { class: 'text-gray-400' }, '类型：'), h('span', task.storage.type || '-')]),
          task.storage.config && task.storage.config.path ? h('div', [h('span', { class: 'text-gray-400' }, '路径：'), h('span', task.storage.config.path)]) : null,
          task.storage.config && task.storage.config.source ? h('div', [h('span', { class: 'text-gray-400' }, '源：'), h('span', task.storage.config.source)]) : null,
          task.storage.config && task.storage.config.database ? h('div', [h('span', { class: 'text-gray-400' }, '库：'), h('span', task.storage.config.database)]) : null,
          task.storage.config && task.storage.config.collection ? h('div', [h('span', { class: 'text-gray-400' }, '集合：'), h('span', task.storage.config.collection)]) : null,
        ])
      ]) : null,
      // Task 类型特定扩展元数据
      typeof extraMeta === 'function' ? extraMeta(h, task) : null,
    ])
  }

  window.TaskUI = window.TaskUI || {}
  window.TaskUI.BaseMeta = BaseMeta
})()