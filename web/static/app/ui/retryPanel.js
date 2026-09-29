// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * UiRetryPanel — 重试中心：失败分类聚合 + 一键/按类批量重试。
 * 纯函数模块，通过 state 与 Vue 应用交互（与 UiDashboard 同模式）。
 * state 必须包含：retryOverview, retryBusy
 */
;(function () {
  'use strict'

  var CATEGORIES = [
    { key: 'http_4xx', label: 'HTTP 4xx（永久）', color: 'red' },
    { key: 'http_5xx', label: 'HTTP 5xx（可重试）', color: 'orange' },
    { key: 'timeout', label: '超时' , color: 'yellow' },
    { key: 'other', label: '其它', color: 'gray' }
  ]

  function categoryLabel (key) {
    for (var i = 0; i < CATEGORIES.length; i++) {
      if (CATEGORIES[i].key === key) return CATEGORIES[i].label
    }
    return key || '其它'
  }

  function categoryColor (key) {
    for (var i = 0; i < CATEGORIES.length; i++) {
      if (CATEGORIES[i].key === key) return CATEGORIES[i].color
    }
    return 'gray'
  }

  function fetchOverview (state) {
    return (AppAPI.retryOverview ? AppAPI.retryOverview() : AppAPI.get('/api/retry/overview'))
      .then(function (data) {
        state.retryOverview = data
        return data
      })
      .catch(function (e) {
        console.error('Retry panel overview error:', e)
        state.retryOverview = null
      })
  }

  function retryCategory (state, category) {
    if (state.retryBusy) return Promise.resolve()
    state.retryBusy = true
    return AppAPI.post('/api/retry/retry-category', { category: category })
      .then(function (res) {
        if (!res.ok) throw new Error('分类重试失败')
        return fetchOverview(state)
      })
      .catch(function (e) {
        console.error('Retry category error:', e)
      })
      .finally(function () {
        state.retryBusy = false
      })
  }

  // retryableCategories 返回可一键重试的分类（5xx / timeout / other）。
  function retryableCategories (overview) {
    if (!overview || !overview.categories) return []
    return CATEGORIES.filter(function (c) {
      return c.key !== 'http_4xx'
    })
  }

  function categoryCount (overview, key) {
    if (!overview || !overview.categories) return 0
    return overview.categories[key] || 0
  }

  window.UiRetryPanel = {
    CATEGORIES: CATEGORIES,
    categoryLabel: categoryLabel,
    categoryColor: categoryColor,
    fetchOverview: fetchOverview,
    retryCategory: retryCategory,
    retryableCategories: retryableCategories,
    categoryCount: categoryCount
  }
})()