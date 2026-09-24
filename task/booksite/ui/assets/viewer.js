/**
 * Copyright 2026 The Cocomhub Authors. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

;(function () {
  'use strict'

  if (typeof TaskUI === 'undefined' || !TaskUI.register) return

  TaskUI.register('booksite', {
    type: 'booksite',
    label: 'Book Site',
    icon: 'fa-link',

    renderForm: TaskUI.defineForm({
      fields: [
        { type: 'textarea', key: 'urls_text', label: 'URL 列表（每行一个）', rows: 10, required: true },
      ]
    }),

    renderMeta: TaskUI.defineMeta({
      fields: [
        { type: 'count', key: 'URL 数量', path: 'extra.urls' },
      ]
    }),

    collectExtra: function (formData) {
      return { urls_text: formData.urls_text }
    }
  })
})()
