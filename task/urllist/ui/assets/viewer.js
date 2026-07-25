// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * url_list 类型 UI 资产
 * 注册到 TaskUI 注册表，使用声明式表单/元数据配置。
 */
;(function () {
  'use strict'

  TaskUI.register('url_list', {
    type: 'url_list',
    label: 'URL 列表',
    icon: 'fa-link',
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
    renderMeta: TaskUI.defineMeta({
      fields: [
        { type: 'count', key: 'urls', label: 'URL 数量', path: 'extra.urls' }
      ]
    }),
    collectExtra: function (formData) {
      var extra = {}
      if (formData.urls_text) extra.urls_text = formData.urls_text
      return extra
    }
  })
})()