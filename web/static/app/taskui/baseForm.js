// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * BaseForm — 基础新建任务表单
 * 通用字段：任务ID、保存目录、存储类型
 * 通过 extraFields 参数接收 task 类型特定扩展字段的渲染函数
 */
;(function () {
  'use strict'

  function BaseForm(h, formData, formErrors, extraFields) {
    formErrors = formErrors || {}
    return h('div', { class: 'space-y-4' }, [
      // 任务ID
      h('div', [
        h('label', { class: 'block text-sm font-medium text-gray-700 mb-1' }, '任务ID *'),
        h('input', {
          class: 'w-full border border-gray-300 rounded-md p-2 focus:ring-2 focus:ring-blue-500 focus:border-blue-500',
          attrs: { type: 'text', placeholder: '例如：my_download_task' },
          domProps: { value: formData.id || '' },
          on: { input: function (e) { formData.id = e.target.value } }
        })
      ]),
      // 保存目录
      h('div', [
        h('label', { class: 'block text-sm font-medium text-gray-700 mb-1' }, '保存目录'),
        h('input', {
          class: 'w-full border border-gray-300 rounded-md p-2 focus:ring-2 focus:ring-blue-500 focus:border-blue-500',
          attrs: { type: 'text', placeholder: './downloads/mytask' },
          domProps: { value: formData.save_dir || '' },
          on: { input: function (e) { formData.save_dir = e.target.value } }
        })
      ]),
      // 存储类型
      h('div', { class: 'grid grid-cols-2 gap-4' }, [
        h('div', [
          h('label', { class: 'block text-sm font-medium text-gray-700 mb-1' }, '存储类型'),
          h('select', {
            class: 'w-full border border-gray-300 rounded-md p-2',
            domProps: { value: formData.storage_type || 'file' },
            on: { change: function (e) { formData.storage_type = e.target.value } }
          }, [
            h('option', { attrs: { value: 'file' } }, '文件'),
            h('option', { attrs: { value: 'mongo' } }, 'Mongo'),
            h('option', { attrs: { value: 'memory' } }, '内存'),
          ])
        ]),
        formData.storage_type === 'file' ? h('div', [
          h('label', { class: 'block text-sm font-medium text-gray-700 mb-1' }, '文件路径'),
          h('input', {
            class: 'w-full border border-gray-300 rounded-md p-2',
            attrs: { type: 'text', placeholder: formData.save_dir + '/' + (formData.id || 'task') + '_history.json' },
            domProps: { value: formData.storage_config && formData.storage_config.path || '' },
            on: { input: function (e) { formData.storage_config = formData.storage_config || {}; formData.storage_config.path = e.target.value } }
          })
        ]) : null,
        formData.storage_type === 'mongo' ? h('div', { class: 'col-span-2 grid grid-cols-3 gap-2' }, [
          h('div', [
            h('label', { class: 'block text-sm font-medium text-gray-700 mb-1' }, '来源'),
            h('input', {
              class: 'w-full border border-gray-300 rounded-md p-2',
              domProps: { value: formData.storage_config && formData.storage_config.source || '' },
              on: { input: function (e) { formData.storage_config = formData.storage_config || {}; formData.storage_config.source = e.target.value } }
            })
          ]),
          h('div', [
            h('label', { class: 'block text-sm font-medium text-gray-700 mb-1' }, '数据库'),
            h('input', {
              class: 'w-full border border-gray-300 rounded-md p-2',
              domProps: { value: formData.storage_config && formData.storage_config.database || '' },
              on: { input: function (e) { formData.storage_config = formData.storage_config || {}; formData.storage_config.database = e.target.value } }
            })
          ]),
          h('div', [
            h('label', { class: 'block text-sm font-medium text-gray-700 mb-1' }, '集合'),
            h('input', {
              class: 'w-full border border-gray-300 rounded-md p-2',
              domProps: { value: formData.storage_config && formData.storage_config.collection || '' },
              on: { input: function (e) { formData.storage_config = formData.storage_config || {}; formData.storage_config.collection = e.target.value } }
            })
          ])
        ]) : null,
      ]),
      // Task 类型特定扩展字段
      typeof extraFields === 'function' ? extraFields(h, formData) : null,
    ])
  }

  window.TaskUI = window.TaskUI || {}
  window.TaskUI.BaseForm = BaseForm
})()