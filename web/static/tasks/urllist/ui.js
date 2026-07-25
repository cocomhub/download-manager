// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * url_list 类型 UI 组件
 * 使用声明式配置工厂，无需自定义渲染函数
 */
;(function () {
  'use strict'

  TaskUI.register('url_list', {
    type: 'url_list',
    label: 'URL 列表',
    icon: 'fa-link',
    // 使用声明式表单配置
    renderForm: TaskUI.defineForm({
      fields: [
        {
          type: 'textarea',
          key: 'urls_text',
          label: 'URL 列表（每行一个）',
          rows: 10,
          required: true,
          placeholder: 'https://example.com/file1.zip\nhttps://example.com/file2.zip'
        }
      ]
    }),
    // 使用声明式元数据配置
    renderMeta: TaskUI.defineMeta({
      fields: [
        { type: 'count', key: 'urls', label: 'URL 数量', path: 'extra.urls' }
      ]
    })
  })
})()