// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

/**
 * UiDashboard — 纯函数版本的仪表盘方法。
 * 从 dashboard.js 的 mixin 中提取，消除 Vue.mixin 依赖。
 *
 * 所有方法通过 state 参数与 Vue 应用交互。
 * state 必须包含：dashboardHealth, dashboardMetrics, dashboardFailures,
 *   dashboardFailuresLimit, dashboardFailuresTaskId
 */
;(function () {
  'use strict'

  // ---- Polling timer IDs (stored externally) ----
  var _timers = { healthz: null, metrics: null, failures: null }

  function fetchDashboardData (state) {
    fetchHealthz(state)
    fetchMetrics(state)
    fetchFailures(state)
  }

  function fetchHealthz (state) {
    AppAPI.healthz().then(function (data) {
      state.dashboardHealth = data
    }).catch(function (e) {
      console.error('Dashboard healthz error:', e)
      state.dashboardHealth = { status: 'error', components: {} }
    })
  }

  function fetchMetrics (state) {
    AppAPI.metrics().then(function (data) {
      state.dashboardMetrics = data
    }).catch(function (e) {
      console.error('Dashboard metrics error:', e)
    })
  }

  function fetchFailures (state) {
    var limit = state.dashboardFailuresLimit || 20
    var taskId = state.dashboardFailuresTaskId || ''
    AppAPI.failures({ limit: limit, task_id: taskId }).then(function (data) {
      state.dashboardFailures = data
    }).catch(function (e) {
      console.error('Dashboard failures error:', e)
    })
  }

  function startDashboardPolling (state) {
    stopDashboardPolling()

    _timers.healthz = setInterval(function () { fetchHealthz(state) }, 5000)
    _timers.metrics = setInterval(function () { fetchMetrics(state) }, 10000)
    _timers.failures = setInterval(function () { fetchFailures(state) }, 15000)
  }

  function stopDashboardPolling () {
    if (_timers.healthz) { clearInterval(_timers.healthz); _timers.healthz = null }
    if (_timers.metrics) { clearInterval(_timers.metrics); _timers.metrics = null }
    if (_timers.failures) { clearInterval(_timers.failures); _timers.failures = null }
  }

  function changeDashboardFailuresLimit (state) {
    fetchFailures(state)
  }

  function searchDashboardFailures (state) {
    fetchFailures(state)
  }

  window.UiDashboard = {
    fetchDashboardData: fetchDashboardData,
    fetchHealthz: fetchHealthz,
    fetchMetrics: fetchMetrics,
    fetchFailures: fetchFailures,
    startDashboardPolling: startDashboardPolling,
    stopDashboardPolling: stopDashboardPolling,
    changeDashboardFailuresLimit: changeDashboardFailuresLimit,
    searchDashboardFailures: searchDashboardFailures,
  }
})()